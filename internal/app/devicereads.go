package app

// The three answers a caller reads out of the fleet: one page of printers, the
// rollup of them by connector, and one printer on its own.
//
// They sit apart from the register in devices.go because they only read what
// the register holds. Each one walks the caller's own authorized population
// rather than the process's whole fleet, and none of them changes a thing.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/platform/observability/health"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// List returns one page of the printers the caller may view telemetry on and
// that this process has been dialled by, and how many there are over every
// page. A printer whose session has ended is on it, carrying the last state it
// reported and why it went.
//
// Every row carries what the host has found wrong with that printer, and the
// page carries the rollup of those issues over the caller's whole authorized
// population, retired printers apart. No filter narrows the rollup, so a
// client that asked for the problems still learns how large the fleet behind
// them is. The page also describes itself: the filters it offers with the
// values present in the caller's fleet, and the orders it may be read in. All
// of it is read out of the entries this process already holds, so it costs no
// statement the listing did not already cost.
//
// The permission is resolved inside the graph's own query and never by
// filtering rows afterwards. What is joined here is existence,
// not permission: a printer in the directory with no entry at all is one this
// process has not been dialled by since it started, which is what a directory
// that survives a restart leaves behind.
//
// That join is why the caller's page cannot be the query's page. Which printers
// this process has met is process state and no statement can join against it,
// so a page cut in SQL and then filtered here comes back short: fifty
// authorized rows of which thirty are unknown would answer as twenty items,
// which a client cannot tell from a fleet of twenty. The
// directory is walked in windows instead, the join is applied as each window
// arrives, and the caller's limit and offset are applied to what is left.
func (d *devices) List(
	ctx context.Context, principal string, query httpapi.DeviceQuery,
) (httpapi.DevicePage, error) {
	who, err := authz.ParseSubject(principal)
	if err != nil {
		return httpapi.DevicePage{}, fmt.Errorf("read the caller's principal: %w", err)
	}

	filter, err := newDeviceFilter(query, d.models)
	if err != nil {
		return httpapi.DevicePage{}, err
	}

	limit, offset := authz.Page{Limit: query.Limit, Offset: query.Offset}.Bounds()

	// One reading for the whole page, taken before the walk. Every row then
	// carries the generation its action set was read under, and a grant written
	// during the walk moves the epoch a client compares against rather than
	// leaving half the page stamped older than the other half.
	epoch := d.graph.Epoch()

	// The dangerous verbs the caller reaches, on every device at once, so the
	// page costs one more query and not one per row.
	conferred, err := d.graph.VerbsOfSubject(ctx, who)
	if err != nil {
		return httpapi.DevicePage{}, err
	}

	answer := httpapi.DevicePage{
		Items:  make([]httpapi.Device, 0, min(limit, fleetScanWindow)),
		Limit:  limit,
		Offset: offset,
		Issues: emptyIssueSummary(),
	}

	// The rows the clauses left, held until the walk ends. The caller may ask
	// for an order, and an order cannot be applied to a page that was cut while
	// the rows were still arriving. The slice holds one pointer and a few words
	// per device over the caller's authorized population, which this product
	// bounds at the fleet it claims, and nothing is rendered until the window
	// is known.
	var matched []candidate

	facetsOf := newFacets(filter, query)

	err = d.walkVisible(ctx, who, func(row authz.DeviceRow, entry *device) {
		facts := entry.filterFacts(row.Name)

		// The retired clause decides whether a device is a candidate at all, but
		// not whether the fleet counts it: a retired printer is a record of a
		// machine that has left, so its issues are nobody's to attend to, and
		// a live printer counts whatever the retired clause chose.
		candidateRow := filter.admitsRetired(facts)
		if facts.retired && !candidateRow {
			return
		}

		// Assessed once and used twice. A second reading for the rendered row
		// would let the rollup and the item beside it disagree about the same
		// printer, and the two are read together in one screen.
		issues, link := entry.issues()

		if !facts.retired {
			addToRollup(&answer.Issues, issues)
		}

		if !candidateRow {
			return
		}

		tags := entry.tagList()
		match := clauseMatch{
			state:    len(filter.states) == 0 || matchesAny(filter.states, facts.listedState()),
			model:    len(filter.models) == 0 || matchesAny(filter.models, facts.model),
			search:   filter.matchesSearch(facts),
			tags:     entry.carriesTags(filter.tags) && entry.matchesLabels(filter.labels),
			severity: filter.admitsSeverity(issues, query.WithIssues),
		}

		facetsOf.count(match, facts, tags, issues)

		if !match.all() {
			return
		}

		matched = append(matched, candidate{
			row: row, entry: entry, facts: facts, issues: issues, link: link,
		})
	})
	if err != nil {
		return httpapi.DevicePage{}, err
	}

	answer.Filters = facetsOf.describe()
	answer.Sorts = slices.Clone(listingSorts)
	answer.Sort = filter.sort
	answer.Order = filter.order

	filter.orderDevices(matched)

	answer.Total = len(matched)

	for index := offset; index < len(matched) && len(answer.Items) < limit; index++ {
		taken := matched[index]

		answer.Items = append(answer.Items,
			taken.entry.render(taken.row.Name, taken.row.Actions, conferred[taken.row.ID], epoch,
				taken.issues, taken.link, d.prints, d.isResident(taken.entry.driver)))
	}

	return answer, nil
}

// walkVisible calls visit for every printer the subject may view telemetry on
// that this process holds an entry for, live or departed, in the directory's
// own order.
//
// The permission is resolved inside the query and the window is the query's, so
// nothing here holds more than one window of rows however large the deployment
// is. It is shared by the listing and the connector rollup because both answer
// a question about the caller's own population, and two walks would be two
// chances for one of them to answer about somebody else's.
//
// The store takes each window by where the last one ended rather than by an
// offset, so a printer registering during the walk cannot move a row out of it.
func (d *devices) walkVisible(
	ctx context.Context, who authz.Subject, visit func(authz.DeviceRow, *device),
) error {
	return d.graph.WalkDevices(ctx, who, authz.ActionObserve, fleetScanWindow,
		func(row authz.DeviceRow) error {
			entry, seen := d.known(row.ID)
			if !seen {
				return nil
			}

			visit(row, entry)

			return nil
		})
}

// Connectors rolls up the printers this caller may see, one entry per connector
// they hold at least one printer on.
//
// The population is the caller's rather than the process's, and that is a
// security property rather than a convenience: a count over the whole fleet
// would tell a caller who may see twenty printers that there are four hundred,
// which is the fact the listing refuses them. It walks the same authorized
// directory the listing walks, for the same reason.
//
// The plane records no transition time for a printer going away, so Since is
// left zero and the wire reports it absent. The order inside a connector then
// falls back to the identifier, which is stable across polls, and the counts
// are what the report is for.
//
// LastSeen is the entry's own instant and never the stored record's. The record
// is written by one goroutine on a two second timer, so a device this process
// is holding would report that nothing has ever seen it for as long as a flush
// is owed, and a device this process is holding has been seen by definition. It
// is also what health.Roll reads to decide that a connector has gone quiet, so
// a zero here is the answer "unknown" under any grace period at all.
//
// A retired printer is not in the population. It is the record of a machine
// that has left: the listing leaves it out by default and the issue
// rollup never counts it. Counting it would report a connector as failing
// because of registrations nobody can see on the Printers page, since one live
// printer beside two retired rows is three devices with one connected, which
// is below the floor.
func (d *devices) Connectors(ctx context.Context, principal string) ([]httpapi.ConnectorRollup, error) {
	who, err := authz.ParseSubject(principal)
	if err != nil {
		return nil, fmt.Errorf("read the caller's principal: %w", err)
	}

	populations := map[string][]health.Device{}

	err = d.walkVisible(ctx, who, func(row authz.DeviceRow, entry *device) {
		if _, _, retired := entry.retirement(); retired {
			return
		}

		populations[entry.driver] = append(populations[entry.driver], health.Device{
			ID:       row.ID,
			Name:     entry.displayedName(row.Name),
			State:    presenceState(entry.connected()),
			LastSeen: entry.seenAt(),
		})
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	rollups := make([]httpapi.ConnectorRollup, 0, len(populations))

	for _, connector := range slices.Sorted(maps.Keys(populations)) {
		rollups = append(rollups,
			projectRollup(health.Roll(connector, now, health.Policy{}, populations[connector])))
	}

	return rollups, nil
}

// Get returns one printer, whether its session is open or has ended. A caller
// who holds nothing on it and an id this server has never been dialled by are
// the same refusal, so a listing is the only thing that tells anybody which
// printers exist.
func (d *devices) Get(ctx context.Context, principal string, id string) (httpapi.Device, error) {
	entry, held, err := d.reachHolding(ctx, principal, id, authz.ActionObserve)
	if err != nil {
		return httpapi.Device{}, err
	}

	who, err := authz.ParseSubject(principal)
	if err != nil {
		return httpapi.Device{}, fmt.Errorf("read the caller's principal: %w", err)
	}

	conferred, err := d.graph.VerbsOfSubject(ctx, who)
	if err != nil {
		return httpapi.Device{}, err
	}

	issues, link := entry.issues()

	return entry.render("", held, conferred[id], d.graph.Epoch(), issues, link, d.prints,
		d.isResident(entry.driver)), nil
}

// Capabilities returns the descriptor one printer resolved to. It takes the
// action Get takes, because it is the capability list Get already answers with
// the arguments written out. A printer whose session has ended has no resolved
// set, and answers an empty one under epoch zero, as its record does.
func (d *devices) Capabilities(
	ctx context.Context, principal string, id string,
) (httpapi.DeviceCapabilitySet, error) {
	entry, err := d.reach(ctx, principal, id, authz.ActionObserve)
	if err != nil {
		return httpapi.DeviceCapabilitySet{}, err
	}

	if entry.session == nil {
		return httpapi.DeviceCapabilitySet{Set: connectorapi.Set{}}, nil
	}

	set, epoch := entry.session.Capabilities()

	return httpapi.DeviceCapabilitySet{Set: set, Epoch: epoch, ProfileMatched: entry.session.ProfileMatched()}, nil
}
