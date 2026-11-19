package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// The grants an approved install writes, and a removal takes back.
//
// The permissions a manifest declares are an upper limit on what the module may
// do as itself, and they are not a grant. The authorization graph decides what
// the module actually reaches, and it decides it for the module's own
// principal, which is `sink:` and the instance's name. Nothing writes a grant
// for that principal on its own, so a module an operator has approved sees no
// printer until somebody writes one. Approving the install writes it.

// grantWriter is the authorization graph as an install and a removal write it.
// The store behind it is the same one the server reads.
//
// Grants is here for the standing check rather than for its rows. Reading the
// grants on an object takes exactly the permission writing one takes, so asking
// for them is how this asks whether the write would be refused, without writing
// anything.
type grantWriter interface {
	Grants(ctx context.Context, who authz.Subject, object authz.Object) ([]authz.Tuple, error)
	WriteGrant(ctx context.Context, who authz.Subject, grant authz.Tuple) (bool, error)
	WithdrawGrant(ctx context.Context, who authz.Subject, grant authz.Tuple) error
}

// moduleAuthority writes a module's grants as the principal the operator's
// configuration names. On an ordinary deployment that principal holds the admin
// role on this server's organization, because the first grant is written for
// it, and written again at the first start after it changes. On a deployment
// whose administrator withdrew those grants it holds nothing, which is what
// mayWrite is for.
type moduleAuthority struct {
	graph grantWriter
	who   authz.Subject
}

// moduleGrant is one grant an approved install writes: the edge itself, and the
// actions of the manifest it answers. Two actions the same role reaches are one
// grant and one line.
type moduleGrant struct {
	tuple   authz.Tuple
	answers []authz.Action
}

// moduleGrantPlan is every grant an approved install will write, and every
// permission it will not write one for.
type moduleGrantPlan struct {
	write []moduleGrant
	// apart is each action the organization cannot confer on a module's own
	// principal. A module holds the viewer role on the organization and no role
	// above it, so an operator writes anything above that on each printer.
	apart []authz.Action
	// unplanned is each permission this cannot turn into a grant at all: a name
	// the authorization graph does not hold, and an action no role reaches.
	//
	// It is carried rather than returned as an error because the two callers
	// answer it differently. An install refuses, so a manifest asking for
	// something nobody can grant never reaches the disk. A removal says so and
	// carries on, because a declaration nobody can plan the withdrawal of still
	// has files, a declaration and grants to take away, and refusing would
	// strand all three.
	unplanned []unplannedPermission
}

// unplannedPermission is one permission of a manifest that becomes no grant,
// and the sentence that says why.
type unplannedPermission struct {
	name string
	why  string
}

// planModuleGrants maps the actions a manifest asks for onto the grants an
// approved install writes for the module's own principal.
//
// It decides nothing and writes nothing. A permission it cannot turn into a
// grant goes into the plan's unplanned list, and the caller says what that
// means: refusable is the install's answer and the removal has its own.
//
// The error it returns is not a permission at all. It is this server unable to
// name its own organization or its own instance in the graph, which is a broken
// deployment rather than a manifest.
func planModuleGrants(instance ref.Ref, permissions []string) (moduleGrantPlan, error) {
	principal, err := authz.ParseSubject(sinkPrincipal(instance))
	if err != nil {
		return moduleGrantPlan{}, fmt.Errorf(
			"name %s in the authorization graph: %w", instance, err)
	}

	org, err := authz.NewObject(authz.TypeOrg, bootstrapOrg)
	if err != nil {
		return moduleGrantPlan{}, fmt.Errorf(
			"name the organization in the authorization graph: %w", err)
	}

	var plan moduleGrantPlan

	for _, name := range permissions {
		action := authz.Action(strings.TrimSpace(name))
		if !action.Valid() {
			plan.unplanned = append(plan.unplanned, unplannedPermission{
				name: name,
				why: "it is not an action the authorization graph holds. The actions are " +
					strings.Join(authz.ActionNames(authz.Actions()), ", "),
			})

			continue
		}

		relation, conferred := authz.OrgRoleConferring(action)
		if !conferred {
			plan.unplanned = append(plan.unplanned, unplannedPermission{
				name: name,
				why: "no role on the organization confers it. A device's own credential takes it, " +
					"and nothing else can be granted it",
			})

			continue
		}

		if !authz.Grantable(authz.TypeOrg, relation, principal.Type()) {
			// A file action the organization cannot confer is granted on the
			// storage kind, which is where the storage routes check it. A
			// module that writes G-code needs write_storage, and no role on
			// the organization a module may hold reaches it.
			if tuple, onStorage := storageGrant(action, relation, principal); onStorage {
				plan.add(tuple, action)

				continue
			}

			plan.apart = append(plan.apart, action)

			continue
		}

		tuple, tupleErr := authz.NewTuple(org, relation, principal)
		if tupleErr != nil {
			return moduleGrantPlan{}, fmt.Errorf("build the grant for %s: %w", action, tupleErr)
		}

		plan.add(tuple, action)
	}

	return plan, nil
}

// refusable is the install's answer to a permission nothing can grant. An
// install writes files and a declaration, so it refuses before either, and the
// operator reads the reason with the deployment exactly as it was.
func (p *moduleGrantPlan) refusable() error {
	if len(p.unplanned) == 0 {
		return nil
	}

	held := p.unplanned[0]

	return fmt.Errorf("the manifest asks for the permission %q, and %s", held.name, held.why)
}

// without is this plan's grants that the other plan does not write. It is what
// a second install takes back: a manifest that dropped a permission leaves a
// grant behind that nothing in the new declaration asks for.
func (p *moduleGrantPlan) without(kept moduleGrantPlan) moduleGrantPlan {
	var left moduleGrantPlan

	for _, grant := range p.write {
		if slices.ContainsFunc(kept.write, func(held moduleGrant) bool {
			return held.tuple == grant.tuple
		}) {
			continue
		}

		left.write = append(left.write, grant)
	}

	return left
}

// add records one grant, and folds a second action the same edge answers into
// the line already there.
func (p *moduleGrantPlan) add(tuple authz.Tuple, action authz.Action) {
	for i := range p.write {
		if p.write[i].tuple == tuple {
			p.write[i].answers = append(p.write[i].answers, action)

			return
		}
	}

	p.write = append(p.write, moduleGrant{tuple: tuple, answers: []authz.Action{action}})
}

// moduleGrantWork is everything one install does to the authorization graph:
// the grants it writes, the grants of the declaration it replaces that nothing
// asks for any more, and the graph it does both through.
type moduleGrantWork struct {
	plan      moduleGrantPlan
	stale     moduleGrantPlan
	authority *moduleAuthority
	close     func()
}

// grantWorkFor works out what an install owes the authorization graph, opens
// the graph when it owes it anything, and refuses before the caller writes a
// file if the write would not be allowed.
//
// asking is what the release's manifest asks for, and declared is what this
// install replaces. Both are read, because a module holds only the grants its
// declaration justifies. A release that dropped a permission has to take its
// grant with it, or the graph holds a grant no declaration explains.
func grantWorkFor(
	ctx context.Context, install moduleInstall, instance ref.Ref,
	asking manifest.Manifest, declared resource.Declaration,
) (moduleGrantWork, error) {
	work := moduleGrantWork{close: func() {}}

	// An approved install records what the module may do as itself, and a
	// permission recorded with no grant behind it still bounds what the module
	// asks. checks lets it ask about an action it does not take,
	// check_absent_people lets it ask about any person, list_people lets it
	// list them, and a public route lets anybody reach it. allowed_origins lets
	// a page of another site call the API in the name of whoever opens it. All
	// of them are administering the organization, so all need the same standing
	// a grant needs, even when no tuple is planned.
	ceiling := install.request.approve &&
		(len(asking.Permissions) > 0 || len(asking.Checks) > 0 || asking.CheckAbsentPeople || asking.ListPeople ||
			len(asking.Calls) > 0 || len(asking.Routes.Public) > 0 || len(asking.AllowedOrigins) > 0)

	if install.request.approve {
		plan, err := planModuleGrants(instance, asking.Permissions)
		if err != nil {
			return moduleGrantWork{}, err
		}

		if err = plan.refusable(); err != nil {
			return moduleGrantWork{}, err
		}

		work.plan = plan
	}

	before, err := planModuleGrants(instance, declaredPermissions(declared))
	if err != nil {
		return moduleGrantWork{}, err
	}

	work.stale = before.without(work.plan)

	objects := grantObjects(work.plan, work.stale)

	if ceiling {
		org, orgErr := authz.NewObject(authz.TypeOrg, bootstrapOrg)
		if orgErr != nil {
			return moduleGrantWork{}, fmt.Errorf("name the organization in the authorization graph: %w", orgErr)
		}

		objects = appendObject(objects, org)
	}

	if install.grants == nil || len(objects) == 0 {
		return work, nil
	}

	authority, closeGraph, err := install.grants()
	if err != nil {
		return moduleGrantWork{}, err
	}

	work.authority, work.close = authority, closeGraph

	// The grants taken back are checked with the ones written, and both before
	// the caller writes a file. A withdrawal refused after the declaration was
	// replaced would leave a grant no declaration asks for.
	if err = authority.mayWrite(ctx, objects); err != nil {
		closeGraph()

		return moduleGrantWork{}, err
	}

	return work, nil
}

// grantObjects is every object the plans write a grant on or take one from,
// each once.
func grantObjects(plans ...moduleGrantPlan) []authz.Object {
	var objects []authz.Object

	for _, plan := range plans {
		for _, grant := range plan.write {
			objects = appendObject(objects, grant.tuple.Object())
		}
	}

	return objects
}

// appendObject adds one object to the list unless it is already there.
func appendObject(objects []authz.Object, object authz.Object) []authz.Object {
	if slices.Contains(objects, object) {
		return objects
	}

	return append(objects, object)
}

// grantsMoved is what one install did to the authorization graph: a line for
// each grant it wrote and each it took back, and the actions of the manifest
// that a written grant answers.
type grantsMoved struct {
	granted   []string
	withdrawn []string
	// gave is each action a grant was written for. An action the plan held
	// and no grant was written for is not in it, which is what the answer's
	// approved list is built from.
	gave []authz.Action
}

// move writes the grants this install owes and takes back the ones the new
// declaration does not justify, and answers a line for each.
func (w moduleGrantWork) move(ctx context.Context) (grantsMoved, error) {
	var moved grantsMoved

	var err error

	moved.granted, moved.gave, err = w.authority.write(ctx, w.plan)
	if err != nil {
		return moved, err
	}

	moved.withdrawn, err = w.authority.withdraw(ctx, w.stale)

	return moved, err
}

// mayWrite refuses an install whose grants this principal cannot write or take
// back, before the install writes anything.
//
// Without it the files and the declaration land, the grant is refused, and the
// deployment is left with a module switched on, its permissions recorded, and
// nothing granted.
//
// The question is asked by reading the grants on the object. Reading them needs
// the admin role on the organisation, and so does writing one. Writing also
// checks that the role stays below admin and that the writer keeps its own
// standing, and every tuple this plan writes is the viewer role for a module,
// so an admin that can read can write it too. Nothing is written either way.
func (a *moduleAuthority) mayWrite(ctx context.Context, objects []authz.Object) error {
	if a == nil {
		return nil
	}

	for _, object := range objects {
		if _, err := a.graph.Grants(ctx, a.who, object); err != nil {
			return fmt.Errorf("core.principal is %q, and that principal may not write the grants on "+
				"%s. Nothing was installed. Set core.principal to the principal this server wrote its "+
				"first grant for, or install without -approve and write the grants yourself: %w",
				a.who.ID(), object, err)
		}
	}

	return nil
}

// write stores the plan's grants and answers one line for each, with the
// actions the written grants answer.
//
// A grant the graph already holds is written again and stores nothing, so a
// second install of the same module leaves one row and prints the same line.
//
// A nil authority writes nothing. That is a caller with no graph open in this
// process, and it answers no line and no action.
//
// A failure answers what was written before it, so the caller reports those
// grants too and not only the one that failed.
func (a *moduleAuthority) write(ctx context.Context, plan moduleGrantPlan) ([]string, []authz.Action, error) {
	if a == nil {
		return nil, nil, nil
	}

	lines := make([]string, 0, len(plan.write))

	var gave []authz.Action

	for _, grant := range plan.write {
		if _, err := a.graph.WriteGrant(ctx, a.who, grant.tuple); err != nil {
			return lines, gave, fmt.Errorf("the module is installed and the grant %s was not written: %w",
				grant.tuple, err)
		}

		lines = append(lines, describeModuleGrant(grant))
		gave = append(gave, grant.answers...)
	}

	return lines, gave, nil
}

// withdraw takes the plan's grants away and answers one line for each it took.
// A grant the graph does not hold is not an error, so removing a module twice
// says the same thing twice.
//
// Every grant is tried, and the failures are answered together. A withdrawal
// that stopped at the first failure would leave the rest of a removed module's
// grants standing, with nothing left declared to tell anybody they are there.
func (a *moduleAuthority) withdraw(ctx context.Context, plan moduleGrantPlan) ([]string, error) {
	if a == nil {
		return nil, nil
	}

	lines := make([]string, 0, len(plan.write))

	var failures []error

	for _, grant := range plan.write {
		if err := a.graph.WithdrawGrant(ctx, a.who, grant.tuple); err != nil {
			failures = append(failures, fmt.Errorf("take the grant %s away: %w", grant.tuple, err))

			continue
		}

		lines = append(lines, describeModuleGrant(grant))
	}

	return lines, errors.Join(failures...)
}

// describeModuleGrant is one grant as an operator reads it: the edge the graph
// holds, the permission of the manifest it answers, and whatever else the role
// carries.
//
// The last part is the one somebody has to read. A role is not one action: the
// viewer role written to answer a request for observe also lets the module read
// the configuration of every resource kind and the contents of every storage.
// An operator who is told only the action they approved has not been told what
// they approved.
//
// A grant with no action is one read back from the graph rather than planned
// from a manifest, and it is only the edge.
func describeModuleGrant(grant moduleGrant) string {
	if len(grant.answers) == 0 {
		return grant.tuple.String()
	}

	line := grant.tuple.String() + " for " + strings.Join(authz.ActionNames(grant.answers), " and ")

	var besides []authz.Action

	for _, action := range authz.OrgRoleConfers(grant.tuple.Relation()) {
		// A role on a resource kind carries the resource actions of that
		// role on that kind, and nothing on the devices.
		if grant.tuple.Object().Type() == authz.TypeResourceKind && !slices.Contains(resourceActions, action) {
			continue
		}

		if !slices.Contains(grant.answers, action) {
			besides = append(besides, action)
		}
	}

	if len(besides) == 0 {
		return line
	}

	return line + ". The " + grant.tuple.Relation().String() + " role also carries " +
		strings.Join(authz.ActionNames(besides), ", ")
}

// describeGrantApart is one action the organization cannot confer on a module,
// as an operator reads it.
func describeGrantApart(action authz.Action) string {
	relation, _ := authz.OrgRoleConferring(action)

	return fmt.Sprintf("%s. It needs the %s role on the organization, and a module holds the viewer "+
		"role there and no role above it. It may still ask whether a person may %s", action, relation, action)
}

// openModuleGrants opens the authorization graph so an install can write the
// module's grants and a removal can take them back.
//
// The write runs as the principal the operator's configuration names, which is
// the principal this server wrote its first grant for and the one that
// administers the organization. There is no second door: the command writes
// through the same rules an administrator writes through over the API.
//
// A core that is already running reads a grant written here on the next request
// it answers, because it resolves every request against the projection in this
// same database. The one thing that does not move is an event stream that is
// already open: a subscription keeps the access it was opened with until the
// epoch that core holds in memory moves, and another process's write does not
// move it. So an open stream widens or narrows at that core's next start.
func openModuleGrants(ctx context.Context, core config.Core) (*moduleAuthority, func(), error) {
	who, err := authz.NewSubject(humanSubjectType, principalID(core))
	if err != nil {
		return nil, nil, fmt.Errorf(
			"core.principal is %q, which the authorization graph cannot key on: %w", core.Principal, err)
	}

	host, closeHost, err := openGraph(ctx, core, "", quiet())
	if err != nil {
		return nil, nil, err
	}

	return &moduleAuthority{graph: host.graph, who: who}, closeHost, nil
}

// removalAuthority is the same authority over the graph this process already
// holds, for the server rather than for a command. It is what takes a removed
// module's grants away when the removal came over HTTP.
//
// A configuration whose principal the graph cannot key on answers nil. The
// server is already running by then and refusing to serve over it would be a
// deployment down for a key that only a removal reads, so the removal leaves
// the grants written and this says once, at startup, that it will.
func removalAuthority(core config.Core, graph grantWriter, logger *slog.Logger) *moduleAuthority {
	who, err := authz.NewSubject(humanSubjectType, principalID(core))
	if err != nil {
		logger.Warn("core.principal cannot be named in the authorization graph, so removing a module "+
			"over the API will leave its grants written", "principal", core.Principal)

		return nil
	}

	return &moduleAuthority{graph: graph, who: who}
}

// resourceActions are the actions a role on a resource kind carries.
//
//nolint:gochecknoglobals // a closed table read when an install is described.
var resourceActions = []authz.Action{
	authz.ActionViewResource, authz.ActionOperateResource, authz.ActionConfigureResource,
	authz.ActionReadStorage, authz.ActionWriteStorage,
}

// storageGrant is the grant of a file action on the storage kind, for a role
// the organization cannot give a module's principal, and false for any other
// action or a role the storage kind cannot give it either.
func storageGrant(action authz.Action, relation authz.Relation, principal authz.Subject) (authz.Tuple, bool) {
	if action != authz.ActionReadStorage && action != authz.ActionWriteStorage {
		return authz.Tuple{}, false
	}

	if !authz.Grantable(authz.TypeResourceKind, relation, principal.Type()) {
		return authz.Tuple{}, false
	}

	kind, err := authz.NewObject(authz.TypeResourceKind, ref.KindStorage.String())
	if err != nil {
		return authz.Tuple{}, false
	}

	tuple, err := authz.NewTuple(kind, relation, principal)
	if err != nil {
		return authz.Tuple{}, false
	}

	return tuple, true
}
