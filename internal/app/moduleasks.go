package app

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// What a module asks the operator for, held to what the operator was shown
// before they approved it, and to what they decided about the release it
// replaces.

// listPeople is the manifest key, and the request line, of the permission to
// list the people this server holds.
const listPeople = "list_people"

// checkRequest is the request line of an action the module asks access.check
// about without taking it as itself.
func checkRequest(action string) string { return "check " + action }

// originRequest is the request line of a browser origin the module asks the
// API to answer.
func originRequest(origin string) string { return "origin " + origin }

// callRequest is the request line of a call to another module's routes.
func callRequest(namespace string) string { return "call " + namespace }

// publicRequest is the request line of a route the module answers without a
// caller, such as "public POST /hooks/announce/" (ADR-20261120-xys rule 2).
func publicRequest(route string) string { return "public " + route }

// stagedRequest is the request line of a route whose body the host writes to
// a file, such as "upload PUT /files/{name} up to 1024 MiB" (ADR-20261120-xys
// rule 5).
func stagedRequest(route install.StagedRoute) string { return "upload " + route.String() }

// headerRequest is the request line of a request header the host passes to
// the module, with the reason the module gives, such as
// "header Print-After-Upload (PrusaSlicer 2 says whether to print the file it
// uploads)" (ADR-20261118-nnm).
func headerRequest(header install.RequestHeader) string {
	if header.Required {
		return "header " + header.Name + ", required (" + header.Reason + ")"
	}

	return "header " + header.Name + " (" + header.Reason + ")"
}

// asksOf is what a manifest asks for, before anything is decided about it.
func asksOf(declared manifest.Manifest) install.ModuleAsks {
	return install.ModuleAsks{
		Permissions:       slices.Clone(declared.Permissions),
		Checks:            slices.Clone(declared.Checks),
		Events:            maps.Clone(declared.Events),
		CheckAbsentPeople: declared.CheckAbsentPeople,
		ListPeople:        declared.ListPeople,
		Calls:             slices.Clone(declared.Calls),
		PublicRoutes:      routeLines(declared.Routes.Public),
		StagedRoutes:      install.StagedRoutesOf(declared.Routes.Staged),
		RequestHeaders:    install.RequestHeadersOf(declared.Routes.Headers),
		RecordsMiB:        declared.RecordsMiB,
		MemoryMiB:         declared.Needs.MemoryMiB,
		Listen:            listenAsksOf(declared.Needs.Listen),
		Data:              declared.Needs.Data,
		Database:          declared.Needs.Database,
		AllowedOrigins:    slices.Clone(declared.AllowedOrigins),
	}
}

// beyondAsks is each request of asked that the ceiling does not hold, in the
// words requests uses, and empty when asked stays within it.
func beyondAsks(asked, ceiling install.ModuleAsks, namespace string) []string {
	out := linesBeyond(asked.Permissions, ceiling.Permissions, func(action string) string {
		return "permission " + action
	})
	out = append(out, linesBeyond(asked.Checks, ceiling.Checks, checkRequest)...)

	for endpoint, name := range asked.Events {
		if held, listed := ceiling.Events[endpoint]; !listed || held != name {
			out = append(out, "event "+endpoint+" as "+namespace+"."+name)
		}
	}

	if asked.CheckAbsentPeople && !ceiling.CheckAbsentPeople {
		out = append(out, "permission "+checkAbsentPeople)
	}

	if asked.ListPeople && !ceiling.ListPeople {
		out = append(out, "permission "+listPeople)
	}

	out = append(out, callsBeyond(asked.Calls, ceiling.Calls)...)
	out = append(out, linesBeyond(asked.AllowedOrigins, ceiling.AllowedOrigins, originRequest)...)
	out = append(out, linesBeyond(asked.PublicRoutes, ceiling.PublicRoutes, publicRequest)...)
	out = append(out, stagedBeyond(asked.StagedRoutes, ceiling.StagedRoutes)...)
	out = append(out, headersBeyond(asked.RequestHeaders, ceiling.RequestHeaders)...)

	if asked.RecordsMiB > ceiling.RecordsMiB {
		out = append(out, recordsRequest(asked.RecordsMiB))
	}

	if asked.MemoryMiB > ceiling.MemoryMiB {
		out = append(out, memoryRequest(asked.MemoryMiB))
	}

	for _, listen := range asked.Listen {
		if !slices.Contains(ceiling.Listen, listen) {
			out = append(out, listen.Request())
		}
	}

	if asked.Data && !ceiling.Data {
		out = append(out, dataRequest)
	}

	if asked.Database && !ceiling.Database {
		out = append(out, databaseRequest)
	}

	slices.Sort(out)

	return out
}

// requestsChangedError is a release that asks for more than the operator was
// shown: what the marketplace listed for it, or what the page sent as expect.
//
// It is a type for the reason handDeclaredError is: the command prints the
// sentence, and the API answers the refusal with the lines in its detail.
type requestsChangedError struct {
	shown  string
	beyond []string
}

func (e requestsChangedError) Error() string {
	return fmt.Sprintf("the release asks for more than %s: %s. Nothing was installed. Read what it asks "+
		"for and approve it again", e.shown, strings.Join(e.beyond, ", "))
}

// The two things an operator may have been shown before approving.
const (
	shownByMarketplace = "the marketplace listed for it"
	shownByRequest     = "the request expected"
)

// heldToShown refuses an approved install whose manifest asks for more than a
// ceiling the operator read. A nil ceiling is one nobody showed, which the
// command is.
func heldToShown(asked install.ModuleAsks, namespace string, ceilings map[string]*install.ModuleAsks) error {
	for _, shown := range []string{shownByMarketplace, shownByRequest} {
		ceiling := ceilings[shown]
		if ceiling == nil {
			continue
		}

		if beyond := beyondAsks(asked, *ceiling, namespace); len(beyond) > 0 {
			return requestsChangedError{shown: shown, beyond: beyond}
		}
	}

	return nil
}

// nameTakenError is a release whose instance name another module already
// holds. The instance and its directory under the modules directory are named
// after the manifest's name alone, so installing it would replace the files
// and the declaration of a module with another path.
type nameTakenError struct {
	instance ref.Ref
	held     string
	wanted   string
}

func (e nameTakenError) Error() string {
	return fmt.Sprintf("the name %s is taken by %s, declared as %s, and this release is %s. "+
		"Remove that module first, or install a release with another name",
		e.instance.ID(), e.instance, e.held, e.wanted)
}

// nameFree refuses a release whose name another declaration holds: the same
// instance declared for another type, or a module of another kind under the
// same name, whose files live in the same directory.
func nameFree(doc resource.Document, instance ref.Ref, typeName string) error {
	for _, declared := range doc.Instances {
		if declared.Ref.ID() != instance.ID() {
			continue
		}

		_, module := declared.ModuleNamespace()

		switch {
		case declared.Ref == instance && declared.Spec.Type != typeName:
			return nameTakenError{instance: declared.Ref, held: declared.Spec.Type, wanted: typeName}
		case declared.Ref != instance && module:
			return nameTakenError{instance: declared.Ref, held: declared.Spec.Type, wanted: typeName}
		}
	}

	return nil
}

// declaredAlready is the declaration this install replaces, and the zero value
// when the instance is new, with the document it was read from. The install
// reads it for what the last one was granted, so a permission the manifest has
// dropped takes its grant with it, and the document for the ports the other
// instances hold.
//
// It refuses to install over an instance the operator declared in their own
// file. That file is theirs, this command never writes it, and an overlay entry
// under the same name would be shadowed by it and never run. It also refuses a
// name another module holds, so a release with another module path cannot
// replace that module's files and declaration.
func declaredAlready(
	core config.Core, configPath string, instance ref.Ref, typeName string,
) (resource.Declaration, resource.Document, error) {
	doc, err := resource.Load(resource.Paths{File: configPath, Overlay: core.Overlay})
	if err != nil {
		return resource.Declaration{}, resource.Document{}, err
	}

	declared, found := doc.Find(instance)
	if found && declared.Source == resource.SourceFile {
		return resource.Declaration{}, resource.Document{}, handDeclaredError{instance: instance, file: configPath}
	}

	if err = nameFree(doc, instance, typeName); err != nil {
		return resource.Declaration{}, resource.Document{}, err
	}

	if !found {
		return resource.Declaration{}, doc, nil
	}

	return declared, doc, nil
}

// handDeclaredError reports an instance the operator declared in their own file.
//
// It is a type because two callers say different things about it. The command
// prints the sentence. The API answers the refusal that names the instance and
// the file, which a page renders itself.
type handDeclaredError struct {
	instance ref.Ref
	file     string
	// installsOff leaves out the advice to install under another name, which
	// the server refuses while core.modules_enabled is false.
	installsOff bool
}

func (e handDeclaredError) Error() string {
	if e.installsOff {
		return fmt.Sprintf("%s is declared in %s, which this server never writes. "+
			"Remove the block there", e.instance, e.file)
	}

	return fmt.Sprintf("%s is declared in %s, which this server never writes. "+
		"Remove the block there, or install this module under another name", e.instance, e.file)
}

// shownAsks is what the manifest asks for, refused when an approved install
// asks for more than the operator was shown.
func shownAsks(request moduleRequest, declared manifest.Manifest, namespace string) (install.ModuleAsks, error) {
	asked := asksOf(declared)

	if !request.approve {
		return asked, nil
	}

	return asked, heldToShown(asked, namespace, request.shown)
}

// replaced is what an install decided against the declaration it replaces.
type replaced struct {
	declared resource.Declaration
	// added is the lines this release asks for that the one before did not.
	added []string
	// memory is the operator's memory settings the install writes.
	memory operatorMemory
}

// operatorMemory is what the operator set for the module's memory and its
// switch-off, which an install keeps from the declaration it replaces.
type operatorMemory struct {
	// mib is the budget, and zero to follow the author's recommendation.
	mib int
	// onOverMemory is the service block's on_over_memory.
	onOverMemory string
	// switchOff is the service block's switch_off.
	switchOff install.SwitchOff
}

// replacing reads the declaration this install replaces, and writes what the
// install grants into the manifest the rest of the install reads, so the
// declaration, the grants and the answer all carry it.
func replacing(
	core config.Core, configPath string, instance ref.Ref, namespace string, request moduleRequest,
	asked install.ModuleAsks, held *manifest.Manifest,
) (replaced, error) {
	declared, doc, err := declaredAlready(core, configPath, instance,
		namespace+extension.QualifiedSeparator+held.Name)
	if err != nil {
		return replaced{}, err
	}

	given, added, err := decidedAsks(request, asked, declared, namespace, portsInUse(core, doc, instance))
	if err != nil {
		return replaced{}, err
	}

	memory, err := grantedMemory(request, declared)
	if err != nil {
		return replaced{}, err
	}

	// Before anything is written, so a module that would leave a contract
	// unmet, its own or another module's, changes nothing.
	next := moduleRecordOf(instance, namespace, *held, request.approve)
	if err = resource.CheckChange(doc.Instances, resource.Replaced(doc.Instances, next), instance); err != nil {
		return replaced{}, err
	}

	grantAsks(held, given)

	return replaced{declared: declared, added: added, memory: memory}, nil
}

// memoryRequest is the request line of a manifest that recommends a memory
// budget for its process.
func memoryRequest(mib int) string {
	return fmt.Sprintf("memory %d MiB", mib)
}

// grantedMemory is the operator's memory settings an install writes: the
// budget the request names, else everything the declaration it replaces holds.
// The budget is never copied from the author's recommendation, so a release
// that lowers its recommendation lowers the budget of a module the operator
// set none for.
func grantedMemory(request moduleRequest, declared resource.Declaration) (operatorMemory, error) {
	var kept operatorMemory
	if declared.Spec.Service != nil {
		kept = operatorMemory{
			mib:          declared.Spec.Service.MemoryMiB,
			onOverMemory: declared.Spec.Service.OnOverMemory,
			switchOff:    declared.Spec.Service.SwitchOff,
		}
	}

	if request.memoryMiB == 0 {
		return kept, nil
	}

	if !request.approve {
		return operatorMemory{}, fmt.Errorf("%w: memory_mib is set only with approve", ErrUsage)
	}

	if err := install.CheckMemoryMiB("memory_mib", request.memoryMiB); err != nil {
		return operatorMemory{}, fmt.Errorf("%w: %w", ErrUsage, err)
	}

	kept.mib = request.memoryMiB

	return kept, nil
}

// decidedAsks is what an approved install grants: what the manifest asks for,
// less what the operator took away from the release it replaces, and the
// lines this release asks for that the one before did not.
//
// The ask the operator approved last is recorded in the declaration. A
// permission in that ask and missing from the declaration is one the operator
// took away, and it stays away. A records room that differs from that ask is
// one the operator set, and it stays unless the request names another. A
// declaration that records no ask has no decision to keep, and the install
// grants what the manifest asks.
func decidedAsks(
	request moduleRequest, asked install.ModuleAsks, declared resource.Declaration, namespace string,
	inUse map[install.ListenPort]string,
) (install.ModuleAsks, []string, error) {
	records, err := grantedRecords(request, asked.RecordsMiB)
	if err != nil {
		return install.ModuleAsks{}, nil, err
	}

	given := asked
	given.RecordsMiB = records

	// The ports are decided before anything else, because a port is granted
	// only with its own approval, on a first install as on an update.
	before := previousAsk(declared)
	ports := portsHeld{granted: heldListen(declared), before: before, inUse: inUse}

	if given.Listen, err = grantedListen(request, asked.Listen, ports); err != nil {
		return install.ModuleAsks{}, nil, err
	}

	if !request.approve || before == nil {
		return given, nil, nil
	}

	// A module with no process holds its origins too, so they are decided
	// before the service block is asked for.
	var added []string

	given.AllowedOrigins, added = keptOrigins(asked.AllowedOrigins, before.AllowedOrigins,
		declared.Spec.Module.AllowedOrigins, added)

	if declared.Spec.Service == nil {
		slices.Sort(added)

		return given, added, nil
	}

	held := declared.Spec.Service

	var fresh []string

	given.Permissions, fresh = keptPermissions(asked.Permissions, before.Permissions, declaredPermissions(declared))
	added = append(added, fresh...)
	given.Checks, added = keptChecks(asked.Checks, before.Checks, authz.ActionNames(held.Checks), added)
	given.Events, added = keptEvents(asked.Events, before.Events, held.Events, namespace, added)

	freshAbsent := asked.CheckAbsentPeople && !before.CheckAbsentPeople
	given.CheckAbsentPeople = asked.CheckAbsentPeople && (freshAbsent || held.CheckAbsentPeople)

	if freshAbsent {
		added = append(added, "permission "+checkAbsentPeople)
	}

	freshList := asked.ListPeople && !before.ListPeople
	given.ListPeople = asked.ListPeople && (freshList || held.ListPeople)

	if freshList {
		added = append(added, "permission "+listPeople)
	}

	given.Calls, added = keptCalls(asked.Calls, before.Calls, held.Calls, added)
	given.PublicRoutes, added = keptLines(asked.PublicRoutes, before.PublicRoutes,
		routeLines(held.PublicRoutes), added, publicRequest)
	given.StagedRoutes, added = keptStaged(asked.StagedRoutes, before.StagedRoutes, held.StagedRoutes, added)
	given.RequestHeaders, added = keptHeaders(asked.RequestHeaders, before.RequestHeaders, held.RequestHeaders, added)

	if missing := requiredMissing(asked.RequestHeaders, given.RequestHeaders); missing != "" {
		return install.ModuleAsks{}, nil, fmt.Errorf("%w: the module requires the request header %s, and the "+
			"operator took it away from service.request_headers. Grant it there again, or remove the module",
			ErrUsage, missing)
	}

	given.RecordsMiB, added = keptRoom(request, asked, *before, held, given.RecordsMiB, added)

	added = append(added, ports.added(given.Listen)...)

	if asked.Data && !before.Data {
		added = append(added, dataRequest)
	}

	if asked.Database && !before.Database {
		added = append(added, databaseRequest)
	}

	slices.Sort(added)

	return given, added, nil
}

// keptRoom is the records room an approved update grants, and added with a
// line for each room this release asks more of than the one before: a records
// room the operator did not set, and the memory recommendation.
func keptRoom(
	request moduleRequest, asked, before install.ModuleAsks, held *install.Service, records int, added []string,
) (int, []string) {
	if request.recordsMiB == 0 && held.RecordsMiB != before.RecordsMiB {
		records = held.RecordsMiB
	} else if request.recordsMiB == 0 && asked.RecordsMiB > before.RecordsMiB {
		added = append(added, recordsRequest(asked.RecordsMiB))
	}

	if asked.MemoryMiB > before.MemoryMiB {
		added = append(added, memoryRequest(asked.MemoryMiB))
	}

	return records, added
}

// previousAsk is the ask the declaration records, and nil for none.
func previousAsk(declared resource.Declaration) *install.ModuleAsks {
	if declared.Ref.IsZero() || declared.Spec.Module == nil {
		return nil
	}

	return declared.Spec.Module.Asked
}

// grantAsks writes what was decided into the manifest the rest of the install
// reads, so the declaration, the grants and the answer all carry it.
func grantAsks(declared *manifest.Manifest, given install.ModuleAsks) {
	declared.Permissions = given.Permissions
	declared.Checks = given.Checks
	declared.Events = given.Events
	declared.CheckAbsentPeople = given.CheckAbsentPeople
	declared.ListPeople = given.ListPeople
	declared.Calls = given.Calls
	declared.AllowedOrigins = given.AllowedOrigins
	declared.Routes.Public = slices.DeleteFunc(declared.Routes.Public, func(route manifest.Route) bool {
		return !slices.Contains(given.PublicRoutes, route.String())
	})
	declared.Routes.Staged = stagedGiven(declared.Routes.Staged, given.StagedRoutes)
	granted := install.RequestHeaderNames(given.RequestHeaders)
	declared.Routes.Headers = slices.DeleteFunc(declared.Routes.Headers, func(header manifest.RequestHeader) bool {
		return !slices.Contains(granted, header.Name)
	})
	declared.RecordsMiB = given.RecordsMiB
	declared.Needs = manifest.Needs{
		Listen: manifestListen(given.Listen), Data: given.Data, Database: given.Database,
		MemoryMiB: given.MemoryMiB,
	}
}

// callsBeyond is the request line of each call asked that the ceiling does
// not hold.
func callsBeyond(asked, ceiling []string) []string { return linesBeyond(asked, ceiling, callRequest) }

// linesBeyond is the request line of each entry asked that the ceiling does
// not hold.
func linesBeyond(asked, ceiling []string, line func(string) string) []string {
	var out []string

	for _, entry := range asked {
		if !slices.Contains(ceiling, entry) {
			out = append(out, line(entry))
		}
	}

	return out
}

// keptCalls is each call asked that the declaration still holds, or that the
// ask before did not have, with the lines for the second kind added to added.
// A call the operator took away from the release before stays away.
func keptCalls(asked, before, held, added []string) ([]string, []string) {
	return keptLines(asked, before, held, added, callRequest)
}

// keptLines is each entry asked that the declaration still holds, or that the
// ask before did not have, with the request line of the second kind added to
// added. An entry the operator took away from the release before stays away.
func keptLines(asked, before, held, added []string, line func(string) string) ([]string, []string) {
	var kept []string

	for _, entry := range asked {
		fresh := !slices.Contains(before, entry)
		if fresh {
			added = append(added, line(entry))
		}

		if fresh || slices.Contains(held, entry) {
			kept = append(kept, entry)
		}
	}

	return kept, added
}

// keptHeaders is each request header asked that the declaration still holds,
// or that the ask before did not have, with the request line of the second
// kind added to added. A header the operator took away stays away.
func keptHeaders(asked, before []install.RequestHeader, held, added []string) ([]install.RequestHeader, []string) {
	var kept []install.RequestHeader

	for _, header := range asked {
		fresh := !slices.Contains(install.RequestHeaderNames(before), header.Name)
		if fresh {
			added = append(added, headerRequest(header))
		}

		if fresh || slices.Contains(held, header.Name) {
			kept = append(kept, header)
		}
	}

	return kept, added
}

// requiredMissing is the first request header asked that the module marks
// required and given leaves out, and the empty string when there is none.
func requiredMissing(asked, given []install.RequestHeader) string {
	for _, header := range asked {
		if header.Required && !slices.Contains(install.RequestHeaderNames(given), header.Name) {
			return header.Name
		}
	}

	return ""
}

// headersBeyond is the request line of each request header asked that the
// ceiling does not name.
func headersBeyond(asked, ceiling []install.RequestHeader) []string {
	var out []string

	for _, header := range asked {
		if !slices.Contains(install.RequestHeaderNames(ceiling), header.Name) {
			out = append(out, headerRequest(header))
		}
	}

	return out
}

// stagedBeyond is the request line of each staged route asked that the
// ceiling does not hold, or holds with a smaller limit.
func stagedBeyond(asked, ceiling []install.StagedRoute) []string {
	var out []string

	for _, route := range asked {
		held, found := stagedNamed(ceiling, route)
		if !found || route.MaxMiB > held.MaxMiB {
			out = append(out, stagedRequest(route))
		}
	}

	return out
}

// keptStaged is each staged route asked that the declaration still holds, or
// that the ask before did not have, with the request line of each route new
// to this release or asking more than before added to added. A route the
// operator took away stays away. A limit that differs from the ask before is
// the operator's and stays, as a records room the operator set does (keptRoom).
// A route that asks for more than before and more than that limit is a new
// request even then (ADR-20261120-xys rule 5), and approving it gives the new
// ask.
func keptStaged(asked, before, held []install.StagedRoute, added []string) ([]install.StagedRoute, []string) {
	var kept []install.StagedRoute

	for _, route := range asked {
		previous, wasAsked := stagedNamed(before, route)
		granted, isHeld := stagedNamed(held, route)

		switch {
		case !wasAsked:
			added = append(added, stagedRequest(route))
			kept = append(kept, route)
		case !isHeld:
			continue
		case granted.MaxMiB != previous.MaxMiB:
			if route.MaxMiB > previous.MaxMiB && route.MaxMiB > granted.MaxMiB {
				added = append(added, stagedRequest(route))
				kept = append(kept, route)

				continue
			}

			kept = append(kept, granted)
		default:
			if route.MaxMiB > previous.MaxMiB {
				added = append(added, stagedRequest(route))
			}

			kept = append(kept, route)
		}
	}

	return kept, added
}

// stagedNamed is the route of routes with the method and the path of route.
func stagedNamed(routes []install.StagedRoute, route install.StagedRoute) (install.StagedRoute, bool) {
	for _, held := range routes {
		if held.Route() == route.Route() {
			return held, true
		}
	}

	return install.StagedRoute{}, false
}

// stagedGiven is each staged route of the manifest that was given, with the
// limit it was given.
func stagedGiven(declared []manifest.StagedRoute, given []install.StagedRoute) []manifest.StagedRoute {
	var out []manifest.StagedRoute

	for _, route := range declared {
		granted, found := stagedNamed(given, install.StagedRoute{Method: route.Method, Path: route.Path})
		if found {
			out = append(out, manifest.StagedRoute{Route: route.Route, MaxMiB: granted.MaxMiB})
		}
	}

	return out
}

// routeLines is each route as a request names it.
func routeLines(routes []manifest.Route) []string {
	var out []string
	for _, route := range routes {
		out = append(out, route.String())
	}

	return out
}

// keptOrigins is each origin asked that the declaration still holds, or that
// the ask before did not have, with the lines for the second kind added to
// added. An origin the operator took away from the release before stays away.
func keptOrigins(asked, before, held, added []string) ([]string, []string) {
	var kept []string

	for _, origin := range asked {
		fresh := !slices.Contains(before, origin)
		if fresh {
			added = append(added, originRequest(origin))
		}

		if fresh || slices.Contains(held, origin) {
			kept = append(kept, origin)
		}
	}

	return kept, added
}

// keptChecks is each check asked that the declaration still holds, or that the
// ask before did not have, with the lines for the second kind added to added.
// A check the operator took away from the release before stays away.
func keptChecks(asked, before, held, added []string) ([]string, []string) {
	var kept []string

	for _, action := range asked {
		fresh := !slices.Contains(before, action)
		if fresh {
			added = append(added, checkRequest(action))
		}

		if fresh || slices.Contains(held, action) {
			kept = append(kept, action)
		}
	}

	return kept, added
}

// keptPermissions is each permission asked that the declaration still holds,
// or that the ask before did not have, and the lines for the second kind.
func keptPermissions(asked, before, held []string) ([]string, []string) {
	var kept, added []string

	for _, action := range asked {
		fresh := !slices.Contains(before, action)
		if fresh {
			added = append(added, "permission "+action)
		}

		if fresh || slices.Contains(held, action) {
			kept = append(kept, action)
		}
	}

	return kept, added
}

// keptEvents is each event asked that the declaration still holds, or that
// the ask before did not have, and added with a line for the second kind.
func keptEvents(
	asked, before, held map[string]string, namespace string, added []string,
) (map[string]string, []string) {
	var kept map[string]string

	for endpoint, name := range asked {
		fresh := before[endpoint] != name
		if fresh {
			added = append(added, "event "+endpoint+" as "+namespace+"."+name)
		}

		// The declaration holds the whole type, and the release asks by name.
		if fresh || install.EventNameOf(namespace, held[endpoint]) == name {
			if kept == nil {
				kept = map[string]string{}
			}

			kept[endpoint] = name
		}
	}

	return kept, added
}
