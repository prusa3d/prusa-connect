package app

// What narrows a fleet listing and what orders it.
//
// Every clause here narrows and none of them widens. The permission was
// resolved inside the query that produced the rows, and nothing
// in this file reaches the authorization graph: a filter decides what a caller
// is shown out of what they may already see. A tag is one such filter.
//
// The order is applied over the rows the filters left, in this process, and not
// in SQL. The listing cannot page in SQL at all: which devices this process has
// met is process state and no statement joins against it, so the walk already
// produces the caller's whole authorized population and the window is cut
// afterwards. Sorting there costs the comparison and nothing else.

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/printer/profile"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The fields a caller may order the fleet by. The set is closed, because an
// open one would be a way to ask this server to sort on something it does not
// hold and get an order it cannot explain.
const (
	sortByUrgency = "urgency"
	sortByName    = "name"
	sortByModel   = "model"
	sortByState   = "state"
	sortBySeen    = "last_seen"
	sortByID      = "id"
)

// The issue severities a listing may be narrowed to, the device-issue
// vocabulary's closed set.
const (
	severityError   = "error"
	severityWarning = "warning"
	severityInfo    = "info"
)

// maxSearchLength is the longest search text the listing accepts. The request
// schema enforces it; the filters advertise it.
const maxSearchLength = 96

// The two directions.
const (
	orderAscending  = "asc"
	orderDescending = "desc"
)

// What a listing does with the devices an operator has retired.
const (
	retiredExclude = "exclude"
	retiredInclude = "include"
	retiredOnly    = "only"
)

// deviceFilter is one listing request's clauses, read once before the walk so
// that nothing is parsed per device.
type deviceFilter struct {
	tags   []string
	labels []labelClause
	// states, models and search are the compat surface's state, printerType and
	// search. Each is compared without regard to case, because a machine's own
	// spelling of its model and its state is the machine's and a person typing
	// one should not have to match it exactly.
	states []string
	models []string
	search string
	// searchModels is what a reported model must be for the search to find it
	// by the name its card shows. A person types "Core One" off a card, and the
	// machine reports "CoreOneL".
	searchModels namedModels
	// severities keeps the devices carrying an issue of any of these.
	severities []string
	// retired is what to do with the devices an operator retired: leave them
	// out, put them in, or answer with those alone.
	retired string
	sort    string
	order   string
}

// newDeviceFilter reads one query into the clauses the walk applies. A value
// outside a closed set is refused here rather than ignored, because a caller
// who misspelled a sort field and was answered in the default order cannot see
// that anything went wrong.
//
// The catalogue is the rows the cards show, the core's and the drivers'. Its
// zero value holds the core's only.
func newDeviceFilter(query httpapi.DeviceQuery, catalogue profile.Catalogue) (deviceFilter, error) {
	tags, err := parseTagFilter(query.Tags)
	if err != nil {
		return deviceFilter{}, err
	}

	labels, err := parseLabelFilter(query.Labels)
	if err != nil {
		return deviceFilter{}, err
	}

	filter := deviceFilter{
		tags:       tags,
		labels:     labels,
		states:     lowered(query.States),
		models:     lowered(query.Models),
		severities: lowered(query.Severities),
		search:     strings.ToLower(strings.TrimSpace(query.Search)),
		retired:    query.Retired,
		sort:       query.Sort,
		order:      query.Order,
	}

	filter.searchModels = modelsNamed(catalogue, filter.search)

	if filter.retired == "" {
		filter.retired = retiredExclude
	}

	if filter.order == "" {
		filter.order = orderAscending
	}

	if err = oneOf("query.retired", filter.retired,
		retiredExclude, retiredInclude, retiredOnly); err != nil {
		return deviceFilter{}, err
	}

	if err = oneOf("query.order", filter.order, orderAscending, orderDescending); err != nil {
		return deviceFilter{}, err
	}

	for _, severity := range filter.severities {
		if err = oneOf("query.severity", severity,
			severityError, severityWarning, severityInfo); err != nil {
			return deviceFilter{}, err
		}
	}

	if filter.sort == "" {
		filter.sort = sortByUrgency
	}

	if err = oneOf("query.sort", filter.sort,
		sortByUrgency, sortByName, sortByModel, sortByState, sortBySeen, sortByID); err != nil {
		return deviceFilter{}, err
	}

	return filter, nil
}

// oneOf refuses a value the set does not carry, naming the set so a caller can
// correct the request without reading the document.
func oneOf(parameter, value string, allowed ...string) error {
	if slices.Contains(allowed, value) {
		return nil
	}

	return httpfault.RequestInvalid(parameter,
		"one of "+strings.Join(allowed, ", "), "the request carries something else")
}

// lowered is one repeated parameter with every value cased down and every empty
// one dropped, so a comparison below is a string equality.
func lowered(values []string) []string {
	out := make([]string, 0, len(values))

	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}

// namedModels is the catalogue rows whose display name holds the search text,
// in the two forms a card finds its row by. A model is compared folded, as
// connectorapi.FoldModel folds it. A device type code is compared as written
// with the case taken off, because folding drops the dots that tell two codes
// apart.
type namedModels struct {
	folded []string
	codes  []string
}

// modelsNamed reads the catalogue once per listing and not once per device.
func modelsNamed(catalogue profile.Catalogue, search string) namedModels {
	if search == "" {
		return namedModels{}
	}

	var out namedModels

	for _, doc := range catalogue.Docs() {
		if !strings.Contains(strings.ToLower(doc.DisplayName), search) {
			continue
		}

		if folded := connectorapi.FoldModel(doc.Model); folded != "" {
			out.folded = append(out.folded, folded)
		}

		if doc.DeviceTypeID != "" {
			out.codes = append(out.codes, strings.ToLower(doc.DeviceTypeID))
		}
	}

	return out
}

func (n namedModels) holds(model string) bool {
	return slices.Contains(n.codes, strings.ToLower(model)) ||
		slices.Contains(n.folded, connectorapi.FoldModel(model))
}

// deviceFacts is what the filters and the order read off one entry, taken in
// one pass under the entry's lock. Reading each field through its own accessor
// would take that lock five times per device per listing.
type deviceFacts struct {
	name     string
	model    string
	serial   string
	unitUID  string
	state    string
	lastSeen time.Time
	retired  bool
	// connected is what the row's connected field says. A listing ranks on it
	// because the last state a departed machine reported is not its state now.
	connected bool
}

// offlineState is the state a listing files a machine under while it is not
// connected. A printer card already reads that way.
const offlineState = "OFFLINE"

// listedState is the state the state filter, its counts and the state order
// read. A machine that is not connected is offline, whatever it reported last.
// The row still carries that last report, because the departure record keeps it.
func (f deviceFacts) listedState() string {
	if !f.connected {
		return offlineState
	}

	return f.state
}

// filterFacts reads the values a listing filters and orders by.
func (e *device) filterFacts(directory string) deviceFacts {
	connected := e.connected()

	e.mu.Lock()
	defer e.mu.Unlock()

	name := e.name
	if name == "" {
		name = directory
	}

	if name == "" {
		name = displayName(e.id, e.identity)
	}

	return deviceFacts{
		name:      name,
		model:     e.identity.Model,
		serial:    serialOf(e.identity),
		unitUID:   e.identity.UnitUID,
		state:     jobText(e.state[printerStatePath]),
		lastSeen:  e.lastSeen,
		retired:   e.retired != "",
		connected: connected,
	}
}

// admitsRetired is the retired clause alone, the one that decides whether a
// device is a candidate at all.
func (f deviceFilter) admitsRetired(facts deviceFacts) bool {
	switch f.retired {
	case retiredExclude:
		return !facts.retired
	case retiredOnly:
		return facts.retired
	default:
		return true
	}
}

// clauseMatch is which of the narrowing clauses one device passes. The facets
// need each answer on its own: a dimension's counts apply every clause but
// that dimension's own.
type clauseMatch struct {
	state, model, search, tags, severity bool
}

func (m clauseMatch) all() bool {
	return m.state && m.model && m.search && m.tags && m.severity
}

// matchesAny reports whether the value is one of the wanted spellings, compared
// with the case already taken off both sides.
func matchesAny(wanted []string, value string) bool {
	return slices.Contains(wanted, strings.ToLower(value))
}

// matchesSearch is the free-text clause. It looks in the four values a person
// types when they are looking for one machine: what it is called, what model it
// is, and the two numbers that identify it. The model matches by its own
// spelling and by the name the catalogue shows for it.
//
// It is a substring match and not a prefix one, because a person searching for
// a serial types the last digits they can read off the sticker. The id is
// deliberately not searched: it is minted here and means nothing to anybody
// typing.
func (f deviceFilter) matchesSearch(facts deviceFacts) bool {
	if f.search == "" {
		return true
	}

	for _, value := range []string{facts.name, facts.model, facts.serial, facts.unitUID} {
		if strings.Contains(strings.ToLower(value), f.search) {
			return true
		}
	}

	return f.searchModels.holds(facts.model)
}

// candidate is one device that passed every clause, held until the whole
// population is known so that the order can be applied before the page is cut.
//
// It carries the issues and the link the walk already assessed. Assessing them
// again at render time would let a row disagree with the rollup printed beside
// it, and link.observe rolls a window as it answers, so reading it twice for
// one record is two readings of one accumulator.
type candidate struct {
	row    authz.DeviceRow
	entry  *device
	facts  deviceFacts
	issues []httpapi.DeviceIssue
	link   httpapi.DeviceLink
}

// The tiers of the urgency order, worst first. rankSilent is where a machine
// sits when nothing it says can be trusted.
const (
	rankError = iota
	rankAttention
	rankWaiting
	rankSilent
	rankWorking
)

// urgencyRank is where a machine state sits when the fleet is read worst
// first: broken, asking for a person, waiting for hands, silent, working. A
// state this table does not name reads as silent, because a machine reporting
// a word nobody knows is not one a person can trust to be fine. A machine that
// is not connected is silent whatever it last said.
func urgencyRank(state string, connected bool) int {
	if !connected {
		return rankSilent
	}

	switch strings.ToUpper(state) {
	case "ERROR":
		return rankError
	case "ATTENTION":
		return rankAttention
	case "PAUSED", "FINISHED", "STOPPED":
		return rankWaiting
	case "PRINTING", "BUSY", "MANIPULATING", "IDLE", "READY":
		return rankWorking
	default:
		return rankSilent
	}
}

// admitsSeverity is the issue clause: with severities named, a device carrying
// an issue of any of them; with with_issues, a device carrying any issue.
func (f deviceFilter) admitsSeverity(issues []httpapi.DeviceIssue, withIssues bool) bool {
	if withIssues && len(issues) == 0 {
		return false
	}

	if len(f.severities) == 0 {
		return true
	}

	return slices.ContainsFunc(issues, func(issue httpapi.DeviceIssue) bool {
		return slices.Contains(f.severities, issue.Severity)
	})
}

// orderDevices puts the candidates in the order the caller asked for, urgency
// when none was asked for.
//
// The sort is stable and two machines equal on the sorted field are ordered by
// id, ascending in either direction, so a page boundary never moves between
// two requests.
func (f deviceFilter) orderDevices(items []candidate) {
	descending := f.order == orderDescending

	sort.SliceStable(items, func(a, b int) bool {
		first, second := items[a], items[b]
		if descending {
			first, second = second, first
		}

		if f.less(first, second) {
			return true
		}

		if f.less(second, first) {
			return false
		}

		return items[a].row.ID < items[b].row.ID
	})
}

// less compares two candidates on the field the caller named.
func (f deviceFilter) less(a, b candidate) bool {
	switch f.sort {
	case sortByUrgency:
		return urgencyRank(a.facts.state, a.facts.connected) < urgencyRank(b.facts.state, b.facts.connected)
	case sortByName:
		return foldedLess(a.facts.name, b.facts.name)
	case sortByModel:
		return foldedLess(a.facts.model, b.facts.model)
	case sortByState:
		return foldedLess(a.facts.listedState(), b.facts.listedState())
	case sortBySeen:
		return a.facts.lastSeen.Before(b.facts.lastSeen)
	default:
		return a.row.ID < b.row.ID
	}
}

// foldedLess compares two values the way a person reads a list, with the case
// off, and falls back to the exact spelling so that two values differing only
// in case have one order rather than none.
func foldedLess(a, b string) bool {
	lowerA, lowerB := strings.ToLower(a), strings.ToLower(b)
	if lowerA != lowerB {
		return lowerA < lowerB
	}

	return a < b
}
