package resource

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/kernel/atomicfile"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// The functions the install command uses to write the overlay.
//
// `module add` writes a declaration for a server that is not running, as
// `admin create` writes an account for one. It writes through this package
// because this package defines the overlay format, and a second writer with
// its own copy of the format could write a file this package cannot read.
//
// These functions check no version token. The token protects one file from
// two writers at the same time, and the install command runs while the server
// does not. A running core reads the overlay at its next start, and the
// command says so when it finishes. A running server calls them too, from its
// installer, and overlayWriting keeps them from overlapping its own writes.

// InstallModule writes one module's declaration into the overlay, replacing
// the declaration under that name, and returns what it wrote.
//
// The block is checked by the same rules as the operator's own file before
// anything is written, so the next load does not refuse a declaration the
// install command wrote.
//
// The next load creates the identity. A module installed here has no data
// rows yet, so it needs no UID before that load, and the load creates one as
// it does for a block an operator wrote.
//
// left is the change the install makes to the list of databases left behind.
// It is written in the same file write as the declaration.
func InstallModule(
	overlayPath string, instance ref.Ref, entry install.Entry, left LeftChange,
) (Declaration, error) {
	mode, err := install.Declared(instance, entry)
	if err != nil {
		return Declaration{}, fmt.Errorf("%w: %s: %w", ErrConfigInvalid, instance, err)
	}

	if mode.Runtime == install.RuntimeBuiltin {
		return Declaration{}, fmt.Errorf("%w: %s declares runtime %s, and this file holds installed "+
			"modules. An instance this binary compiles in is written through the resource API",
			ErrConfigInvalid, instance, install.RuntimeBuiltin)
	}

	settings, err := canonicalJSON(entry.Config)
	if err != nil {
		return Declaration{}, fmt.Errorf("%w: %s: the settings are not JSON: %w",
			ErrConfigInvalid, instance, err)
	}

	runtime, service, script, asset := declaredMode(mode)
	declared := Declaration{
		Ref: instance,
		Spec: Spec{
			Type:     mode.Type,
			Enabled:  mode.State == install.StateRuntime,
			Required: entry.Required,
			Runtime:  runtime,
			Service:  service,
			Script:   script,
			Asset:    asset,
			Module:   entry.Module,
			Config:   settings,
		},
		Source: SourceOverlay,
	}

	overlayWriting.Lock()
	defer overlayWriting.Unlock()

	overlay, err := readOverlayFile(overlayPath)
	if err != nil {
		return Declaration{}, err
	}

	next := withInstance(overlay, instance, ref.UID{}, declared.Spec)
	next.Databases = left.applied(overlay.Databases)

	if err = writeOverlayFile(overlayPath, next); err != nil {
		return Declaration{}, err
	}

	return declared, nil
}

// RemoveModule removes one module's declaration and its identity from the
// overlay, and reports whether the overlay held one under that name.
//
// A name the overlay does not hold returns false and no error. A module an
// operator declared in their own file is not removed, because this function
// never writes that file.
//
// left is the module's own database that stays after the removal. It is
// recorded in the same file write, and it is nil for a module granted none.
func RemoveModule(overlayPath string, instance ref.Ref, left *LeftDatabase) (bool, error) {
	overlayWriting.Lock()
	defer overlayWriting.Unlock()

	overlay, err := readOverlayFile(overlayPath)
	if err != nil {
		return false, err
	}

	held := false

	for _, entry := range overlay.Instances {
		if entry.Kind == instance.Kind().String() && entry.Name == instance.ID() {
			held = true
		}
	}

	if !held {
		return false, nil
	}

	next := withoutInstance(overlay, instance)
	next.Databases = LeftChange{Leave: left}.applied(overlay.Databases)

	return true, writeOverlayFile(overlayPath, next)
}

// RestoreModule writes a declaration back as it was before an install
// replaced it. An update whose module did not start is undone with it. The
// instance keeps the identity the overlay holds for it.
//
// left is the list of databases left behind as it was before the install. The
// same write restores it.
func RestoreModule(overlayPath string, declared Declaration, left []LeftDatabase) error {
	overlayWriting.Lock()
	defer overlayWriting.Unlock()

	overlay, err := readOverlayFile(overlayPath)
	if err != nil {
		return err
	}

	next := withInstance(overlay, declared.Ref, ref.UID{}, declared.Spec)
	next.Databases = slices.Clone(left)

	return writeOverlayFile(overlayPath, next)
}

// Installed is every module either input declares, in the order of the merged
// document. `module list` reads it when it reads the files and not a running
// server.
func Installed(paths Paths) ([]Declaration, error) {
	doc, err := Load(paths)
	if err != nil {
		return nil, err
	}

	var out []Declaration

	for _, declared := range doc.Instances {
		if _, isModule := declared.ModuleNamespace(); isModule {
			out = append(out, declared)
		}
	}

	return out, nil
}

// overlayWriting makes one read and the write that follows it a single step
// for every writer of the overlay in this process. The store holds its own
// lock as well, but the install functions below run under the installer's
// lock in a running server, and without this one an install and a settings
// write that overlap would each write back the file as it was before the
// other. A version token cannot replace it, because these functions take
// none.
var overlayWriting sync.Mutex

// updateOverlay reads the overlay, applies change and writes the result as
// one step. An error from change writes nothing. The caller holds s.mu.
func (s *Store) updateOverlay(change func(*overlayFile) error) error {
	overlayWriting.Lock()
	defer overlayWriting.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return err
	}

	if err = change(&overlay); err != nil {
		return err
	}

	return s.writeOverlay(overlay)
}

// readOverlayFile reads the machine-owned file, and returns an empty overlay
// when the file does not exist.
func readOverlayFile(path string) (overlayFile, error) {
	content, err := readOptional(path)
	if err != nil {
		return overlayFile{}, err
	}

	return parseOverlay(content)
}

// writeOverlayFile renders and writes the overlay. It creates the overlay's
// directory when it does not exist.
func writeOverlayFile(path string, overlay overlayFile) error {
	encoded, err := renderOverlay(overlay)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("create the overlay directory: %w", err)
	}

	return atomicfile.Write(path, encoded, overlayPerm)
}

// moduleFactsOf renders what the module's manifest said for the wire, and nil
// for a declaration whose install recorded none.
//
// It is published for an instance that is switched off too, because the web
// app still shows a module the operator switched off.
func moduleFactsOf(spec Spec) *httpapi.ResourceModule {
	if spec.Module == nil {
		return nil
	}

	facts := &httpapi.ResourceModule{
		Title:   spec.Module.Title,
		Version: spec.Module.Version,
		Summary: spec.Module.Summary,
		Author:  spec.Module.Author,
	}

	if asked := spec.Module.Asked; asked != nil {
		granted := RequestsOf(*asked)
		facts.Asked = &granted
	}

	facts.AllowedOrigins = slices.Clone(spec.Module.AllowedOrigins)
	facts.Listening = listeningOf(spec)
	facts.Database = databaseOf(spec)

	for _, required := range spec.Module.Requires {
		facts.Requires = append(facts.Requires, httpapi.ResourceRequirement{
			Capability: required.Capability,
			Core:       required.Core,
			Module:     required.Module,
			Version:    required.Version,
			Since:      required.Since,
			Header:     required.Header,
		})
	}

	for _, suggested := range spec.Module.Suggests {
		facts.Suggests = append(facts.Suggests, httpapi.ResourceRequirement{
			Module: suggested.Module, Reason: suggested.Reason,
		})
	}

	for _, conflict := range spec.Module.Conflicts {
		facts.Conflicts = append(facts.Conflicts, httpapi.ResourceRequirement{
			Module: conflict.Module, Version: conflict.Version,
		})
	}

	facts.Pulled = spec.Module.Pulled

	for _, option := range spec.Module.Options {
		published := httpapi.ResourceOption{
			Key:      option.Key,
			Type:     option.Type.String(),
			Required: option.Required,
			Default:  option.Default,
			Summary:  option.Summary,
		}

		// The default of a credential is a credential, so it is not
		// published.
		if option.Type == extension.OptionSecret {
			published.Default = ""
		}

		facts.Options = append(facts.Options, published)
	}

	return facts
}

// serviceEntryOf renders a module's service block as the overlay carries it,
// and nil for a declaration that has none. typeName is the instance's
// qualified type. Each event type in the spec starts with the type's
// namespace. The overlay keeps the event name without the namespace, as the
// operator wrote it, and the next load adds the namespace again.
func serviceEntryOf(service *install.Service, typeName string) *install.ServiceEntry {
	if service == nil {
		return nil
	}

	var events map[string]string

	if service.Events != nil {
		namespace, _, _ := strings.Cut(typeName, extension.QualifiedSeparator)
		events = make(map[string]string, len(service.Events))

		for endpoint, eventType := range service.Events {
			events[endpoint] = install.EventNameOf(namespace, eventType)
		}
	}

	major := service.ContractMajor
	permissions := make([]string, 0, len(service.Permissions))

	for _, action := range service.Permissions {
		permissions = append(permissions, action.String())
	}

	checks := make([]string, 0, len(service.Checks))

	for _, action := range service.Checks {
		checks = append(checks, action.String())
	}

	entry := &install.ServiceEntry{
		Path:              service.Path,
		Checksum:          service.Checksum,
		ContractMajor:     &major,
		Events:            events,
		CheckAbsentPeople: service.CheckAbsentPeople,
		ListPeople:        service.ListPeople,
		Calls:             service.Calls,
		PublicRoutes:      routeLines(service.PublicRoutes),
		StagedRoutes:      slices.Clone(service.StagedRoutes),
		RequestHeaders:    slices.Clone(service.RequestHeaders),
		RecordsMiB:        service.RecordsMiB,
		MemoryMiB:         service.MemoryMiB,
		OnOverMemory:      service.OnOverMemory,
		SwitchOff:         service.SwitchOff.Entry(),
		Listen:            listenEntryOf(service.Listen),
		Data:              service.Data,
		Database:          service.Database.Entry(),
		Mount:             service.Mount,
		Assets:            assetEntryOf(service.Assets),
	}

	if len(permissions) > 0 {
		entry.Permissions = permissions
	}

	if len(checks) > 0 {
		entry.Checks = checks
	}

	return entry
}

// routeLines writes each route as the declaration spells it, and nil for
// none.
func routeLines(routes []manifest.Route) []string {
	if len(routes) == 0 {
		return nil
	}

	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.String())
	}

	return out
}

// assetEntryOf renders a bundle as the overlay carries it, and nil for a
// declaration that has none.
func assetEntryOf(asset *install.Asset) *install.AssetEntry {
	if asset == nil {
		return nil
	}

	return &install.AssetEntry{Path: asset.Path, Checksum: asset.Checksum}
}

// listenEntryOf renders the granted ports in the text form the declaration
// uses, and nil for none.
func listenEntryOf(granted map[string]install.ListenPort) map[string]string {
	if len(granted) == 0 {
		return nil
	}

	out := make(map[string]string, len(granted))
	for name, port := range granted {
		out[name] = port.String()
	}

	return out
}

// StagedRoutesOf is each staged route in the API's spelling, and nil for none.
func StagedRoutesOf(routes []install.StagedRoute) []httpapi.ModuleStagedRoute {
	var out []httpapi.ModuleStagedRoute
	for _, route := range routes {
		out = append(out, httpapi.ModuleStagedRoute{Method: route.Method, Path: route.Path, MaxMiB: route.MaxMiB})
	}

	return out
}

// stagedAsksOf is the API's spelling of staged routes read back into the one
// the declaration records.
func stagedAsksOf(routes []httpapi.ModuleStagedRoute) []install.StagedRoute {
	var out []install.StagedRoute
	for _, route := range routes {
		out = append(out, install.StagedRoute{Method: route.Method, Path: route.Path, MaxMiB: route.MaxMiB})
	}

	return out
}

// RequestHeadersOf is each request header in the API's spelling, and nil for
// none.
func RequestHeadersOf(headers []install.RequestHeader) []httpapi.ModuleRequestHeader {
	var out []httpapi.ModuleRequestHeader
	for _, header := range headers {
		out = append(out, httpapi.ModuleRequestHeader{
			Name: header.Name, Reason: header.Reason, Required: header.Required,
		})
	}

	return out
}

// headerAsksOf is the API's spelling of request headers read back into the
// one the declaration records.
func headerAsksOf(headers []httpapi.ModuleRequestHeader) []install.RequestHeader {
	var out []install.RequestHeader
	for _, header := range headers {
		out = append(out, install.RequestHeader{Name: header.Name, Reason: header.Reason, Required: header.Required})
	}

	return out
}

// RequestsOf converts a recorded ask to the type the API returns.
func RequestsOf(asked install.ModuleAsks) httpapi.ModuleRequests {
	out := httpapi.ModuleRequests{
		Permissions:       asked.Permissions,
		Checks:            asked.Checks,
		Events:            asked.Events,
		CheckAbsentPeople: asked.CheckAbsentPeople,
		ListPeople:        asked.ListPeople,
		Calls:             asked.Calls,
		PublicRoutes:      asked.PublicRoutes,
		StagedRoutes:      StagedRoutesOf(asked.StagedRoutes),
		RequestHeaders:    RequestHeadersOf(asked.RequestHeaders),
		RecordsMiB:        asked.RecordsMiB,
		MemoryMiB:         asked.MemoryMiB,
		Data:              asked.Data,
		Database:          asked.Database,
		AllowedOrigins:    asked.AllowedOrigins,
	}

	for _, listen := range asked.Listen {
		out.Listen = append(out.Listen, httpapi.ModuleListen{
			Name: listen.Name, Network: string(listen.Network), Port: listen.Port, Why: listen.Why,
		})
	}

	return out
}

// AsksOf converts an ask from the API's type to the type the declaration
// records.
func AsksOf(requests httpapi.ModuleRequests) install.ModuleAsks {
	out := install.ModuleAsks{
		Permissions:       requests.Permissions,
		Checks:            requests.Checks,
		Events:            requests.Events,
		CheckAbsentPeople: requests.CheckAbsentPeople,
		ListPeople:        requests.ListPeople,
		Calls:             requests.Calls,
		PublicRoutes:      requests.PublicRoutes,
		StagedRoutes:      stagedAsksOf(requests.StagedRoutes),
		RequestHeaders:    headerAsksOf(requests.RequestHeaders),
		RecordsMiB:        requests.RecordsMiB,
		MemoryMiB:         requests.MemoryMiB,
		Data:              requests.Data,
		Database:          requests.Database,
		AllowedOrigins:    requests.AllowedOrigins,
	}

	for _, listen := range requests.Listen {
		out.Listen = append(out.Listen, install.ListenAsk{
			Name: listen.Name, Network: manifest.Network(listen.Network), Port: listen.Port, Why: listen.Why,
		})
	}

	return out
}

// databaseOf is the database the declaration grants, without its credential,
// and nil for none.
func databaseOf(spec Spec) *httpapi.ResourceDatabase {
	if spec.Service == nil || spec.Service.Database == nil {
		return nil
	}

	held := spec.Service.Database

	return &httpapi.ResourceDatabase{
		Engine: string(held.Engine), Provisioned: held.Provisioned, Server: held.Server(),
	}
}

// withDatabaseSchema adds what the running module reported about its own
// database to the grant the listing publishes.
func (s *Store) withDatabaseSchema(instance *httpapi.ResourceInstance, named ref.Ref) {
	if instance.Module == nil || instance.Module.Database == nil {
		return
	}

	if state, reported := s.plane.Database(named); reported {
		instance.Module.Database.Schema = DatabaseSchemaOf(state)
	}
}

// DatabaseSchemaOf converts what a module reported about its own database to
// the type the listing and the install answer return.
func DatabaseSchemaOf(state wirerpc.DatabaseState) *httpapi.ResourceDatabaseSchema {
	return &httpapi.ResourceDatabaseSchema{
		Version: state.Version, Compat: state.Compat, Applied: slices.Clone(state.Applied),
	}
}

// listeningOf is the ports the declaration grants, in the order the recorded
// ask lists them, each with the reason the manifest gave.
func listeningOf(spec Spec) []httpapi.ModuleListen {
	if spec.Service == nil || len(spec.Service.Listen) == 0 {
		return nil
	}

	why := map[string]string{}
	order := []string{}

	if spec.Module != nil && spec.Module.Asked != nil {
		for _, asked := range spec.Module.Asked.Listen {
			why[asked.Name] = asked.Why
			order = append(order, asked.Name)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(spec.Service.Listen)) {
		if !slices.Contains(order, name) {
			order = append(order, name)
		}
	}

	var out []httpapi.ModuleListen

	for _, name := range order {
		port, granted := spec.Service.Listen[name]
		if !granted {
			continue
		}

		out = append(out, httpapi.ModuleListen{
			Name: name, Network: string(port.Network), Port: port.Port, Why: why[name],
		})
	}

	return out
}

// withDependencies writes, for each module this one depends on, whether the
// dependency is met and the release, instance and state this server has of
// it, and the installed modules that require this one. A module the caller
// may not see is left out, and the dependency still says whether it is met.
func withDependencies(
	facts *httpapi.ResourceModule, resolved ResolvedModule, dependants []ResolvedModule,
	sees func(ref.Ref) bool, stateOf func(ref.Ref) string,
) {
	fillDependencies(facts.Requires, resolved.Requires, sees, stateOf)
	fillDependencies(facts.Conflicts, resolved.Conflicts, sees, stateOf)

	for _, dependant := range dependants {
		if !sees(dependant.Instance) {
			continue
		}

		for _, dependency := range dependant.Requires {
			if dependency.Module == resolved.Namespace {
				facts.RequiredBy = append(facts.RequiredBy, httpapi.ModuleDependant{
					Instance: dependant.Instance.String(), Module: dependant.Namespace, Version: dependency.Version,
					Enabled: dependant.Enabled,
				})
			}
		}
	}
}

// fillDependencies writes the resolved state of each dependency into the rows
// that name it.
func fillDependencies(
	rows []httpapi.ResourceRequirement, held []ResolvedDependency,
	sees func(ref.Ref) bool, stateOf func(ref.Ref) string,
) {
	for index := range rows {
		at := slices.IndexFunc(held, func(one ResolvedDependency) bool {
			return rows[index].Module != "" && one.Module == rows[index].Module
		})
		if at < 0 {
			continue
		}

		dependency := held[at]
		rows[index].Met = dependency.Met

		if dependency.Installed != "" && sees(dependency.Instance) {
			rows[index].Installed = dependency.Installed
			rows[index].Instance = dependency.Instance.String()
			rows[index].State = stateOf(dependency.Instance)
		}
	}
}
