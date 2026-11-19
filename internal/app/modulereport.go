package app

import (
	"fmt"
	"io"
	"slices"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// moduleReport is what one install prints: where the files landed, what the
// module asked for, and whether it was switched on.
type moduleReport struct {
	instance  ref.Ref
	typeName  string
	namespace string
	version   string
	runtime   string
	root      string
	enabled   bool
	checked   int
	requests  []string
	// requires is what the module cannot run without, one line each. It is
	// printed whether or not the operator approved the install, because an
	// instance that is switched on and missing one of these does not start.
	requires []string
	// dependencies is the modules the install added because the release
	// requires them, as module@version.
	dependencies []string
	// suggests is the modules the module works better with, one line each.
	suggests []string
	// settings is the keys the module reads, one line each: the key, its type,
	// its default and what it does. A key with no default is one the operator
	// has to write before the instance starts.
	settings []string
	// connects is every connection the module makes outside Prusa Connect, one
	// line each, as its manifest declares it under egress. It is printed
	// whether or not the operator approved, so they read it before they do.
	connects []string
	// granted is every grant the install wrote into the authorization graph,
	// one line each. An operator who reads the install has then read what the
	// module may reach, and can take any of it away again.
	granted []string
	// withdrawn is every grant the install took back, one line each. It is the
	// grants of the declaration this install replaced that the new one does not
	// justify.
	withdrawn []string
	// apart is every permission the module asked for that the organization
	// cannot confer on it, one action each.
	apart []authz.Action
	// asksAbsent is an approved install of a module that may ask about a
	// person it is not serving. It grants no action, so no grant line says it.
	asksAbsent bool
	// listsPeople is an approved install of a module that may list the people
	// this server holds.
	listsPeople bool
	// calls is each module the install lets this one call the routes of.
	calls []string
	// public is each route the install lets anybody reach, with no caller.
	public []string
	// staged is each route whose body the install lets the host write to a
	// file, with its limit.
	staged []install.StagedRoute
	// headers is each request header the install lets the host pass to the
	// module, with the reason the module gives.
	headers []manifest.RequestHeader
	// checks is each action the install lets this one ask about for a person
	// without taking it as itself.
	checks []string
	// origins is each browser origin the install lets this one add to the
	// origins the API answers.
	origins []string
	// recordsMiB is the room an approved install granted the module's records,
	// in mebibytes, and zero for one that kept the default.
	recordsMiB int
	// memory is the memory budget of an approved service module, and the zero
	// value for any other.
	memory resource.Budget
	// approved is what an approved install gave the module, and withheld is
	// each permission it asked for that no grant was written for. Both are in
	// the words requests uses, so the answer over HTTP says what was given.
	approved []string
	withheld []string
	// added is what an approved update asks for that the release it replaced
	// did not, and that it granted, in the words requests uses.
	added []string
	// ports is each port the manifest asks for that the install did not
	// grant, because the operator did not approve the ports.
	ports []string
	// listens is each port the install granted, as name and network/port.
	listens []string
	// database is where the module's own database is, as the install output
	// names it, and empty for none.
	database string
	// replaced is the declaration this install replaced, and the zero value
	// for a first install. written is the one it wrote. An update the module
	// does not start under is rolled back to replaced.
	replaced resource.Declaration
	written  resource.Declaration
	// previous is where the release this update replaced is kept, and empty
	// when nothing was kept.
	previous string
	// leftBefore is the databases left behind as they were before this
	// install, which a rollback writes back.
	leftBefore []resource.LeftDatabase
	// databaseLeft is the database this install gave up and left behind, as
	// the output names it, and empty for none. databaseBack says the granted
	// database is one an earlier install left behind, so its data is back.
	databaseLeft string
	databaseBack bool
}

// peopleRequests is the request line of each permission about people a
// manifest asks for: asking about the absent and listing them.
func peopleRequests(declared manifest.Manifest) []string {
	var out []string

	if declared.CheckAbsentPeople {
		out = append(out, "permission "+checkAbsentPeople)
	}

	if declared.ListPeople {
		out = append(out, "permission "+listPeople)
	}

	return out
}

// reportOf gathers what the install did, so the printing is one function with
// no decisions left in it.
func reportOf(
	instance ref.Ref, written resource.Declaration, landed installed,
	held release, checked int, approve bool,
) moduleReport {
	namespace, _ := written.ModuleNamespace()

	out := moduleReport{
		instance:  instance,
		typeName:  written.Spec.Type,
		namespace: namespace,
		version:   held.manifest.Version,
		runtime:   written.Spec.Runtime.String(),
		root:      landed.root,
		previous:  landed.previous,
		enabled:   written.Spec.Enabled,
		checked:   checked,
	}

	for _, required := range recordedRequirements(held.manifest) {
		out.requires = append(out.requires, required.Describe())
	}

	for _, suggested := range recordedSuggests(held.manifest.Suggests) {
		out.suggests = append(out.suggests, suggested.Module+": "+suggested.Reason)
	}

	for _, option := range held.manifest.Options {
		out.settings = append(out.settings, describeOption(option))
	}

	for _, egress := range held.manifest.Egress {
		out.connects = append(out.connects, egress.Line())
	}

	out.asksAbsent = approve && held.manifest.CheckAbsentPeople
	out.listsPeople = approve && held.manifest.ListPeople

	if approve {
		out.calls = held.manifest.Calls
		out.public = routeLines(held.manifest.Routes.Public)
		out.staged = install.StagedRoutesOf(held.manifest.Routes.Staged)
		out.headers = held.manifest.Routes.Headers
		out.checks = held.manifest.Checks
		out.origins = held.manifest.AllowedOrigins
	}

	if approve {
		out.recordsMiB = held.manifest.RecordsMiB
		out.memory, _ = resource.BudgetOf(written.Spec)

		out.listens = listensGranted(held.manifest.Needs.Listen)

		if written.Spec.Service != nil {
			out.database = databaseGranted(written.Spec.Service.Database)
		}

		return out
	}

	out.requests = requestLines(held.manifest, namespace)

	return out
}

// requestLines is every request of a manifest nobody approved, one line each
// in the words requests uses, sorted.
func requestLines(asking manifest.Manifest, namespace string) []string {
	var lines []string

	if asking.Needs.Data {
		lines = append(lines, dataRequest)
	}

	if asking.Needs.Database {
		lines = append(lines, databaseRequest)
	}

	for _, action := range asking.Permissions {
		lines = append(lines, "permission "+action)
	}

	for _, action := range asking.Checks {
		lines = append(lines, checkRequest(action))
	}

	lines = append(lines, peopleRequests(asking)...)

	for _, namespace := range asking.Calls {
		lines = append(lines, callRequest(namespace))
	}

	lines = append(lines, routeRequests(asking.Routes)...)

	for _, origin := range asking.AllowedOrigins {
		lines = append(lines, originRequest(origin))
	}

	if asking.RecordsMiB > 0 {
		lines = append(lines, recordsRequest(asking.RecordsMiB))
	}

	if asking.Needs.MemoryMiB > 0 {
		lines = append(lines, memoryRequest(asking.Needs.MemoryMiB))
	}

	for endpoint, name := range asking.Events {
		lines = append(lines, "event "+endpoint+" as "+namespace+"."+name)
	}

	slices.Sort(lines)

	return lines
}

// memoryGranted is what `module add -approve` prints for the module's memory
// budget, with who set it.
func memoryGranted(budget resource.Budget) string {
	const bytesPerMiB = 1 << 20

	if budget.Source == resource.BudgetDefault {
		return fmt.Sprintf("%d MiB, the default (memory_mib)", budget.Bytes/bytesPerMiB)
	}

	return fmt.Sprintf("%d MiB, set by the %s (memory_mib)", budget.Bytes/bytesPerMiB, budget.Source)
}

// report prints what was installed. It says what the module asked for and was
// not given. Without that, an operator finds a module that is switched off and
// silent, and has no reason for it.
func report(out io.Writer, held moduleReport) {
	fmt.Fprintf(out, "installed %s as %s\n", held.typeName, held.instance)
	fmt.Fprintf(out, "  files       %s\n", held.root)
	fmt.Fprintf(out, "  namespace   %s\n", held.namespace)

	if held.checked > 0 {
		fmt.Fprintf(out, "  checksums   %d files verified against %s\n", held.checked, checksumsName)
	}

	for _, required := range held.requires {
		fmt.Fprintf(out, "  needs       %s\n", required)
	}

	for _, added := range held.dependencies {
		fmt.Fprintf(out, "  added       %s, which it requires\n", added)
	}

	for _, suggested := range held.suggests {
		fmt.Fprintf(out, "  suggests    %s\n", suggested)
	}

	for _, setting := range held.settings {
		fmt.Fprintf(out, "  setting     %s\n", setting)
	}

	for _, connection := range held.connects {
		fmt.Fprintf(out, "  connects to %s\n", connection)
	}

	for _, grant := range held.granted {
		fmt.Fprintf(out, "  granted     %s\n", grant)
	}

	for _, grant := range held.withdrawn {
		fmt.Fprintf(out, "  withdrawn   %s\n", grant)
	}

	for _, action := range held.apart {
		fmt.Fprintf(out, "  not granted %s\n", describeGrantApart(action))
	}

	for _, action := range held.checks {
		fmt.Fprintf(out, "  may check   whether a person may %s (checks)\n", action)
	}

	if held.asksAbsent {
		fmt.Fprintf(out, "  may ask     about people it is not serving (%s)\n", checkAbsentPeople)
	}

	if held.listsPeople {
		fmt.Fprintf(out, "  may list    the people this server holds, with their names (%s)\n", listPeople)
	}

	for _, namespace := range held.calls {
		fmt.Fprintf(out, "  may call    the routes of %s (calls)\n", namespace)
	}

	reportRoutes(out, held)

	for _, origin := range held.origins {
		fmt.Fprintf(out, "  answers     pages of %s that call the API from a browser (allowed_origins)\n", origin)
	}

	if held.recordsMiB > 0 {
		fmt.Fprintf(out, "  records     %s\n", recordsGranted(held.recordsMiB))
	}

	if held.memory.Bytes > 0 {
		fmt.Fprintf(out, "  memory      %s\n", memoryGranted(held.memory))
	}

	reportListens(out, held.listens)

	reportDatabase(out, held)

	for _, request := range held.added {
		fmt.Fprintf(out, "  new         %s, which the release it replaced did not ask for\n", request)
	}

	if held.enabled {
		fmt.Fprintln(out, "  enabled     yes")

		reportPortsOpen(out, held.ports)

		return
	}

	fmt.Fprintln(out, "  enabled     no. Run the same line with -approve, or switch it on through the API")

	for _, request := range held.requests {
		fmt.Fprintf(out, "  asks for    %s\n", request)
	}
}

// routeRequests is the request line of each route and each header of the
// routes block: a route with no caller, a staged route and a request header.
func routeRequests(routes manifest.Routes) []string {
	var out []string

	for _, route := range routes.Public {
		out = append(out, publicRequest(route.String()))
	}

	for _, route := range install.StagedRoutesOf(routes.Staged) {
		out = append(out, stagedRequest(route))
	}

	for _, header := range install.RequestHeadersOf(routes.Headers) {
		out = append(out, headerRequest(header))
	}

	return out
}

// reportRoutes writes what an approved install granted of the routes block.
func reportRoutes(out io.Writer, held moduleReport) {
	for _, route := range held.public {
		fmt.Fprintf(out, "  public      %s, answered without a signed-in caller (routes.public)\n", route)
	}

	for _, route := range held.staged {
		fmt.Fprintf(out, "  upload      %s, up to %d MiB (routes.staged)\n", route.Route(), route.MaxMiB)
	}

	for _, header := range held.headers {
		fmt.Fprintf(out, "  header      %s, passed to the module (%s) (routes.headers)\n",
			header.Name, header.Reason)
	}
}
