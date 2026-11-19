package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/internal/app/builtin"
	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/admission"
	"github.com/prusa3d/prusa-connect/internal/kernel/version"
	"github.com/prusa3d/prusa-connect/internal/printer/capability"
	"github.com/prusa3d/prusa-connect/internal/printer/profile"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/event"
)

// connectorPrefix is where a driver goes when its devices can be told where to
// dial. The instance name follows it, and the prefix is stripped before the
// driver sees a request, which is what connectorapi.Inbound promises.
const connectorPrefix = "/connectors/"

// dialsAbsolutePaths is the half of an inbound driver this host needs when the
// device cannot be told where to dial.
//
// connectorapi.Inbound lets the host pick a prefix and strip it, which is right
// for anything configurable. Some firmware in the field has its paths compiled
// in and is not updated on this project's schedule. A driver serving such
// devices says which paths it needs and is mounted on them verbatim. It is
// declared here because this is where it is consumed.
// It is an alias of the published interface and not a second declaration of
// the same method set. With a second declaration, an out-of-tree author could
// learn the shape only by reading this file, and a rename here would stop
// every module outside the core from being mounted on its own paths, with no
// error. With one published name the compiler checks it.
type dialsAbsolutePaths = connectorapi.DialsAbsolutePaths

// takesLogger is the half of a driver that accepts the host's logger.
//
// A driver that implements it is given the logger before its handler is
// mounted and before any device can reach it. The same logger is also in the
// connectorapi.Spec the driver is built from, so a driver that does not
// implement this can still log.
//
// It is declared here because this is where it is consumed.
// It is an alias of the published interface: see dialsAbsolutePaths above for
// why these are aliases and not second declarations.
type takesLogger = connectorapi.TakesLogger

// servesItsOwnSocket is the half of an inbound driver that asks the host for a
// listener of its own.
//
// A dialect whose devices cannot be told where to dial, and cannot be served
// on the socket everything else is served on, says so here. One example is a
// firmware that speaks TLS 1.2 with one cipher suite and verifies one
// certificate: a browser on the same socket would be held to the same
// limits. The address, the certificate and the handshake budget are the
// dialect's, because each of the three is a property of the firmware. The
// host binds the socket, serves it and drains it.
//
// It is an alias of the published interface: see dialsAbsolutePaths above for
// why these are aliases and not second declarations.
type servesItsOwnSocket = connectorapi.ServesItsOwnSocket

// admits is the half of a driver that holds a registration table an operator
// decides on. A dialect whose devices pair by displaying a code implements it,
// and the host publishes that table at /api/v1/registrations.
//
// It is an alias of connectorapi.Admits, and the alias matters. An interface
// written out here would have methods returning
// internal/kernel/admission.Registration, a type in the core's internal tree.
// A module outside the core cannot name that type, so it could not satisfy
// the interface. /api/v1/registrations would then be closed to every
// out-of-tree dialect with no error, and a dialect whose devices pair by
// displaying a code could not publish its table. The published type is the
// same interface, so an out-of-tree dialect can implement it.
type admits = connectorapi.Admits

// keepsRegistrations is the half of a driver whose registration table outlives
// the process. A dialect that implements it is handed the host's store before
// its handler is mounted, and reads back the table the last process left.
//
// It is declared here because this is where it is consumed. It is optional: a
// dialect that does not implement it keeps its table in memory and loses it
// when the process ends.
type keepsRegistrations interface {
	// UseRegistrations hands the driver the store and the names its rows are
	// written under. The host calls it once, before anything can reach the
	// driver.
	UseRegistrations(ctx context.Context, connector, kind string, store admission.Store) error
}

// notifiesApprovals is the half of a driver that approves a registration
// without being asked, and says so.
//
// It is an alias of connectorapi.NotifiesApprovals, and it is declared here for
// the reason takesLogger is: this is where it is consumed. A dialect that
// approves on arrival decides inside its own table, so the host's approval path
// never runs, and the device id that path binds is never bound. Such a driver
// is handed the notice to call; one that only approves through admits does not
// implement this, because the host is already the caller there.
type notifiesApprovals = connectorapi.NotifiesApprovals

// admissionTable is one connector's registration table, under the instance name
// that tells two connectors apart on the operator surface.
type admissionTable struct {
	ref    connectorapi.Ref
	driver admits
}

// profiled is the half of a driver that ships capability profiles of its own.
//
// The registry a resolver is built over is the driver scope: it is the union
// that bounds a device none of its profiles cover, and nothing in an identity
// names a driver and no profile records one. A driver that implements this says
// which profiles are its own; one that does not gets an empty registry, whose
// coverage is absent and narrows nothing, so its whole baseline stands. It is
// declared here because this is where it is consumed.
type profiled interface {
	// Profiles are the capability profiles this driver ships.
	Profiles() (profile.Registry, error)
}

// mountConnectors builds the drivers configuration names and returns the routes
// a printer dials.
//
// A driver is built here and not supervised as a resource instance, because
// its routes are mounted on the sealed router, which is built once when the
// server is. A handler swapped underneath a mounted route would change the
// device wire while a printer was on it. A connector therefore lives as long
// as the process, and changing one is a restart. For the same reason the
// settings are a core key: see config.Connector.
//
// An outbound driver mounts no route and is returned instead, for the dial
// supervisor to reach the addresses the endpoint list holds. A driver may be
// both, and one that is gets its routes mounted and is dialled as well.
//
// Two connectors declaring one path are refused by the sealed router, which
// names the pattern. Nothing checks for it here: a second check would be a
// second answer to one question.
func mountConnectors(
	ctx context.Context, core config.Core, deviceAddr string, logger *slog.Logger, plane *devices,
	driverKinds map[string]authz.DeviceKind, registrations admission.Store, admissionChoice string,
) (connectorPlane, error) {
	types, err := connectorapi.NewRegistry()
	if err != nil {
		return connectorPlane{}, fmt.Errorf("build the connector type registry: %w", err)
	}

	declared := core.DeclaredConnectors()
	if len(declared) == 0 {
		// Which dialect ships is composition, so builtin answers it and this
		// file never names one.
		shipping, defaultErr := builtin.DefaultConnector(types)
		if defaultErr != nil {
			return connectorPlane{}, fmt.Errorf("no connector is declared: %w", defaultErr)
		}

		declared = []config.NamedConnector{{
			Name:      string(shipping),
			Connector: config.Connector{Type: string(shipping)},
		}}

		logger.InfoContext(ctx, "no connector is declared, so the shipping dialect is mounted",
			"connector", string(shipping))
	}

	if deviceAddr != "" && len(declared) > 1 {
		return connectorPlane{}, fmt.Errorf("%w: -printer-addr sets the listener of the connector this "+
			"server mounts, and %d are declared. Write the address under the connector that serves "+
			"those printers, at core.connectors.<name>.config.listener.addr",
			config.ErrInvalid, len(declared))
	}

	var built connectorPlane

	for _, connector := range declared {
		one, mountErr := mountOne(ctx, types, logger, plane, connector,
			driverKinds, registrations, admissionChoice, deviceAddr)
		if mountErr != nil {
			return connectorPlane{}, mountErr
		}

		built.mounts = append(built.mounts, one.mounts...)

		if one.dialled != nil {
			built.dialled = append(built.dialled, *one.dialled)
		}

		if one.admission != nil {
			built.admissions = append(built.admissions, *one.admission)
		}

		if one.listener != nil {
			built.listeners = append(built.listeners, *one.listener)
		}

		if one.states != nil {
			built.states = append(built.states, *one.states)
		}
	}

	built.mounts = append(built.mounts, reservations(built.mounts)...)

	return built, nil
}

// connectorPlane is every declared connector as the composition root receives
// it: the routes a device dials, the outbound halves the dial supervisor
// drives, and the registration tables an operator decides.
type connectorPlane struct {
	mounts     []httpapi.Mount
	dialled    []dialledConnector
	admissions []admissionTable
	// listeners are the sockets the declared dialects asked for, in declaration
	// order. Most deployments have none: a dialect asks for one only when its
	// devices cannot be served on the socket everything else is on.
	listeners []deviceListener
	// states are the state sets the mounted dialects declared, one per
	// mounted connector, in declaration order. placedStates turns them into
	// what the device-state vocabulary serves.
	states []mountedStates
}

// mountedStates is one mounted connector's declared state set, with what the
// host needs to place it: the instance a device names under driver, the type
// that declared it, and the device kind its machines are registered under.
type mountedStates struct {
	instance     connectorapi.Ref
	registration connectorapi.Registration
	kind         authz.DeviceKind
}

// deviceListener is one socket a dialect asked for, under the instance name
// that says which connector to look at when the bind fails.
type deviceListener struct {
	connector connectorapi.Ref
	spec      connectorapi.DeviceListener
}

// builtConnector is one declared connector. Each of the three is empty for a
// half the dialect does not implement, and a dialect that implements none of
// them is refused earlier by connectorapi.Directions.
type builtConnector struct {
	mounts    []httpapi.Mount
	dialled   *dialledConnector
	admission *admissionTable
	listener  *deviceListener
	states    *mountedStates
}

// reservations are the wildcards under the directories a frozen dialect's own
// paths sit in, so a path beside them is refused by this list and not by
// anything the origin may later mount above it.
//
// When a dialect's paths are /p/register and /p/ws, they are the whole of /p,
// and a printer at /p/anything has the wrong address. A handler claiming the
// directory above would answer it 200 with a body the firmware never reads,
// and neither end would see a failure.
//
// A driver mounted under a prefix this host chose already claims everything
// below it and needs none of this. So does a route directly under the root: its
// directory is the origin root, and a wildcard there would claim every path on
// the server.
func reservations(mounts []httpapi.Mount) []httpapi.Mount {
	claimed := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		claimed = append(claimed, mount.Pattern)
	}

	var out []httpapi.Mount

	for _, mount := range mounts {
		pattern := directoryOf(mount.Pattern)
		if pattern == "" || slices.Contains(claimed, pattern) {
			continue
		}

		claimed = append(claimed, pattern)
		out = append(out, httpapi.Mount{
			Pattern: pattern,
			Handler: httpapi.NotRouted(),
			Reason: "the paths beside " + mount.Pattern + ", which no connector claims. A device " +
				"that dials one has the wrong address and is told so, rather than being answered " +
				"by whatever claims the directory above",
		})
	}

	return out
}

// directoryOf is the wildcard covering the directory one route sits in, and is
// empty for a route that needs none: one that already ends in a wildcard, and
// one directly under the origin root.
func directoryOf(route string) string {
	if strings.HasSuffix(route, "/*") {
		return ""
	}

	cut := strings.LastIndex(route, "/")
	if cut <= 0 {
		return ""
	}

	return route[:cut] + "/*"
}

// mountOne builds one declared connector and returns the routes it answers and
// the dialling half the endpoint list feeds. A connector that is switched off
// returns neither, and each half is nil for a dialect that does not implement
// it.
func mountOne(
	ctx context.Context,
	types *connectorapi.Registry,
	logger *slog.Logger,
	plane *devices,
	declared config.NamedConnector,
	driverKinds map[string]authz.DeviceKind,
	registrations admission.Store,
	admissionChoice string,
	deviceAddr string,
) (builtConnector, error) {
	key := "core.connectors." + declared.Name
	instance := connectorapi.Ref(declared.Name)

	if !declared.Connector.On() {
		logger.InfoContext(ctx, "a declared connector is switched off and is not mounted",
			"connector", instance.String(), "type", declared.Connector.Type)

		return builtConnector{}, nil
	}

	registration, err := types.Lookup(connectorapi.TypeName(declared.Connector.Type))
	if err != nil {
		return builtConnector{}, fmt.Errorf("%s.type: %w", key, err)
	}

	settings, err := declared.Connector.Settings()
	if err != nil {
		return builtConnector{}, fmt.Errorf("%s.config: %w", key, err)
	}

	settings, err = withAdmission(ctx, logger, key,
		registration.Dialect().Admission, settings, admissionChoice)
	if err != nil {
		return builtConnector{}, err
	}

	settings, err = withListenerAddr(ctx, logger, key, settings, deviceAddr)
	if err != nil {
		return builtConnector{}, err
	}

	driver, err := registration.Build(connectorapi.Spec{
		Ref: instance, Type: registration.Qualified(), Config: settings,
		Logger: logger.With("connector", instance.String()),
	})
	if err != nil {
		return builtConnector{}, fmt.Errorf("%s: %w", key, err)
	}

	inbound, outbound, err := connectorapi.Directions(driver)
	if err != nil {
		return builtConnector{}, fmt.Errorf("%s: %w", key, err)
	}

	if takes, accepts := driver.(takesLogger); accepts {
		takes.UseLogger(logger.With("connector", instance.String()))
	}

	// The device kind this driver's machines are registered under, read here
	// because the registration table is written under it and a row has to say
	// what is waiting before anything has dialled.
	kind := driverKinds[registration.Qualified().String()]

	// The table the last process left is restored before the routes are
	// mounted. A printer that dials the moment the listener binds must meet the
	// registrations an operator already approved, not an empty table.
	if keeps, accepts := driver.(keepsRegistrations); accepts && registrations != nil {
		if err = keeps.UseRegistrations(ctx, instance.String(), kind.String(), registrations); err != nil {
			return builtConnector{}, fmt.Errorf("%s: %w", key, err)
		}
	}

	// After the table is restored, so a stored approval is not reported as a
	// new one, and before the routes are mounted, so no printer can register
	// or be refused without the host hearing about it.
	if plane != nil {
		hearDecisions(driver, plane, instance.String())
		forgetsRetired(ctx, driver, plane, instance.String())

		if registration.Dialect().Resident {
			plane.useResident(instance.String())
		}
	}

	resolver, err := resolverFor(driver, logger)
	if err != nil {
		return builtConnector{}, fmt.Errorf("read the capability profiles of %s: %w", key, err)
	}

	// A driver type no declaration names is absent from the map, so its kind
	// is empty. A device with an empty kind is registered as a printer, which
	// is the default kind.
	arrivals := plane.Arrivals(instance, driver.Baseline(), resolver, kind)

	built := builtConnector{admission: admissionOf(instance, driver), listener: listenerOf(instance, driver)}

	// A dialect that declared no state set has nothing to place, and the
	// vocabulary then carries no entry for it.
	if !registration.Dialect().States.IsZero() {
		built.states = &mountedStates{instance: instance, registration: registration, kind: kind}
	}

	if outbound != nil {
		built.dialled = &dialledConnector{
			ref: instance, driver: outbound, baseline: driver.Baseline(), arrivals: arrivals,
		}

		logger.InfoContext(ctx, "connector dials the endpoints named for it",
			"connector", instance.String(), "type", registration.Qualified().String())
	}

	if inbound == nil {
		return built, nil
	}

	handler, err := inbound.Handler(arrivals)
	if err != nil {
		return builtConnector{}, fmt.Errorf("mount %s: %w", key, err)
	}

	built.mounts = mountsFor(instance, driver, handler)

	logger.InfoContext(ctx, "connector mounted",
		"connector", instance.String(),
		"type", registration.Qualified().String(),
		"routes", httpapi.PatternsOf(built.mounts),
		"capabilities", len(driver.Baseline()))

	return built, nil
}

// listenerKey is the settings key a dialect writes its own socket under. It is
// the same word in every dialect that asks for one, because it is the word the
// published interface uses.
const listenerKey = "listener"

// withListenerAddr lays the -printer-addr flag over one connector's settings
// block.
//
// The flag exists because an address is the one thing about this socket a
// person changes without editing the file: a second instance on one machine, a
// bench run, a container told which port to bind. It writes the address only,
// so a certificate and a handshake budget the file carries are still the
// operator's.
//
// The key is written into the dialect's own block and is not read by this
// host, for the reason config.Connector gives: what is inside that block
// belongs to the driver, and only the driver can say whether it is right. A
// dialect that reads no listener block refuses the key by name. That is
// correct for a flag that asks it for a socket it does not serve.
func withListenerAddr(
	ctx context.Context, logger *slog.Logger, key string, settings json.RawMessage, addr string,
) (json.RawMessage, error) {
	if addr == "" {
		return settings, nil
	}

	block := map[string]any{}

	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &block); err != nil {
			return nil, fmt.Errorf("%s.config: read the settings block: %w", key, err)
		}
	}

	listener, written := block[listenerKey].(map[string]any)
	if !written {
		if _, present := block[listenerKey]; present {
			return nil, fmt.Errorf("%s.config.%s is not a block, and -printer-addr writes an address "+
				"into it", key, listenerKey)
		}

		listener = map[string]any{}
	}

	listener["addr"] = addr
	block[listenerKey] = listener

	merged, err := json.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("%s.config: write the listener address the flag carried: %w", key, err)
	}

	logger.InfoContext(ctx, "the command line sets this connector's own listener",
		"connector", key, "addr", addr)

	return merged, nil
}

// listenerOf is the socket one driver asks for, and nil for a dialect that asks
// for none or asks with no address. A dialect without one has its device routes
// served on this server's own listener, which is the shipping default.
func listenerOf(instance connectorapi.Ref, driver connectorapi.Driver) *deviceListener {
	asks, own := driver.(servesItsOwnSocket)
	if !own {
		return nil
	}

	spec := asks.DeviceListener()
	if spec.Addr == "" {
		return nil
	}

	return &deviceListener{connector: instance, spec: spec}
}

// withAdmission lays the recorded admission choice over one connector's
// settings block.
//
// The choice is written in the machine-owned overlay by the first run, because
// core.connectors is the operator's own file and this server never writes
// that file (config.Connector). So the connector reads its mode from the
// operator's block where there is one, and from the overlay where there is
// not: an operator who wrote the key meant it, and the first run fills in for
// the deployment that was never edited by hand.
//
// words is the dialect's own declaration (connectorapi.Admission): the settings
// key, and what it calls each of the two choices. A dialect that declared none
// is left exactly as its configuration has it. `ask` and `open` are the core's
// words, and the core cannot guess what another wire calls waiting for an
// operator.
//
// An install that has recorded no choice asks first (resource.EffectiveAdmission).
// A dialect's own default is what it admits with no host above it, and this
// host has an operator to ask.
func withAdmission(
	ctx context.Context, logger *slog.Logger, key string,
	words connectorapi.Admission, settings json.RawMessage, choice string,
) (json.RawMessage, error) {
	recorded := choice != ""
	choice = resource.EffectiveAdmission(choice)

	word, known := admissionWord(words, choice)
	if !known {
		return settings, nil
	}

	block := map[string]any{}

	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &block); err != nil {
			return nil, fmt.Errorf("%s.config: read the settings block: %w", key, err)
		}
	}

	if _, written := block[words.Key]; written {
		return settings, nil
	}

	block[words.Key] = word

	merged, err := json.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("%s.config: write the recorded admission choice: %w", key, err)
	}

	if !recorded {
		logger.InfoContext(ctx, "no admission choice is recorded, so this connector asks before a printer joins",
			"connector", key, "admission", choice, words.Key, word)

		return merged, nil
	}

	logger.InfoContext(ctx, "the first run's admission choice is what this connector admits on",
		"connector", key, "admission", choice, words.Key, word)

	return merged, nil
}

// admissionWord translates one of the core's two choices into the word the
// dialect declared for it, and reports false when there is nothing to write.
//
// Nothing here knows which dialect it is translating for. A dialect that
// declared no words, and a choice outside the two the first run offers, both
// leave the settings block alone.
func admissionWord(words connectorapi.Admission, choice string) (string, bool) {
	if words.IsZero() {
		return "", false
	}

	switch choice {
	case resource.AdmissionOpen:
		return words.Open, true
	case resource.AdmissionAsk:
		return words.Ask, true
	}

	return "", false
}

// admissionOf is one driver's registration table, and nil for a dialect that
// holds none. A dialect without one admits a device the moment it arrives, so
// there is nothing for an operator to decide and no row to publish.
func admissionOf(instance connectorapi.Ref, driver connectorapi.Driver) *admissionTable {
	table, holds := driver.(admits)
	if !holds {
		return nil
	}

	return &admissionTable{ref: instance, driver: table}
}

// hearDecisions hands a driver the notices for what it decides on its own wire:
// the approvals it makes for itself, and the machines it turns away, so a
// person waiting for one learns why it is not joining.
func hearDecisions(driver connectorapi.Driver, plane *devices, connector string) {
	if notifies, accepts := driver.(notifiesApprovals); accepts {
		notifies.UseApprovals(func(
			ctx context.Context, registration connectorapi.DeviceRegistration, by string,
		) error {
			if err := plane.registrationApproved(ctx, registration, by); err != nil {
				return err
			}

			plane.approvedOnArrival(ctx, connector, registration)

			return nil
		})
	}

	if reports, accepts := driver.(connectorapi.ReportsRefusals); accepts {
		admission := admissionWordOf(driver)

		reports.UseRefusals(func(ctx context.Context, refusal connectorapi.DeviceRefusal) {
			plane.registrationRefused(ctx, connector, admission, refusal)
		})
	}
}

// forgetsRetired hands the fleet a driver that can take back the credential it
// issued a device, and has it take back every one an operator already removed.
// It runs after the table is restored, so the rows it removes are the ones the
// last process left.
func forgetsRetired(ctx context.Context, driver connectorapi.Driver, plane *devices, connector string) {
	forgets, accepts := driver.(connectorapi.ForgetsDevices)
	if !accepts {
		return
	}

	plane.useForgetter(connector, forgets)
	plane.withdrawRetiredCredentials(ctx, connector, forgets)
}

// admissionWordOf is how one driver lets a machine in, in the two words the app
// settings and the refused frame use. A driver that does not say is taken to
// ask a person, because that answer never skips a decision.
func admissionWordOf(driver any) string {
	reports, says := driver.(connectorapi.ReportsAdmission)
	if says && reports.AdmitsOnArrival() {
		return event.AdmissionAutomatic
	}

	return event.AdmissionApproval
}

// printerAdmission is how a new device joins this server, as the add-printer
// page reads it from GET /api/v1/ui.
//
// It is automatic only when every connector holding a registration table
// approves on arrival. One connector that asks a person is enough to make it
// approval: the page then offers the approval step, and a machine that reaches
// a connector approving on its own passes through that step without stopping.
// A server with no table at all is automatic, because nothing there waits for
// a decision.
func printerAdmission(tables []admissionTable) string {
	for _, table := range tables {
		if admissionWordOf(table.driver) == event.AdmissionApproval {
			return event.AdmissionApproval
		}
	}

	return event.AdmissionAutomatic
}

// resolverFor is one driver's capability resolver, over the profiles that
// driver ships and no others.
//
// A driver with none gets an empty registry and not the whole shipped file.
// An empty registry covers nothing, and a coverage tier that is absent narrows
// nothing. That is correct for a dialect this build has no data about:
// holding it to another dialect's matrix would remove every verb that
// dialect's firmware does not have.
//
// Each resolver keeps its own per-unit epoch table. A device belongs to the
// driver that admitted it, so no unit is counted by two of them.
func resolverFor(driver connectorapi.Driver, log *slog.Logger) (*capability.Resolver, error) {
	shipped, carries := driver.(profiled)
	if !carries {
		return capability.NewResolver(profile.Registry{}, log), nil
	}

	profiles, err := shipped.Profiles()
	if err != nil {
		return nil, err
	}

	return capability.NewResolver(profiles, log), nil
}

// mountsFor is where one driver's routes go on this origin.
func mountsFor(
	instance connectorapi.Ref, driver connectorapi.Driver, handler http.Handler,
) []httpapi.Mount {
	absolute, frozen := driver.(dialsAbsolutePaths)
	if !frozen {
		prefix := connectorPrefix + instance.String()

		return []httpapi.Mount{{
			Pattern: prefix + "/*",
			Handler: http.StripPrefix(prefix, handler),
			Device:  true,
			Reason: "the " + instance.String() + " connector, under the prefix this host chose for it. " +
				"A device dials it and is admitted; nothing here is an operation a client calls",
		}}
	}

	routes := absolute.Routes()
	out := make([]httpapi.Mount, 0, len(routes))

	for _, route := range routes {
		out = append(out, httpapi.Mount{
			Pattern: route,
			Handler: handler,
			Device:  true,
			Reason: "the " + instance.String() + " connector: a path this dialect's firmware has " +
				"compiled in, frozen before this API existed and not updatable on our schedule",
		})
	}

	return out
}

// appSettings is core.frontend as the page reads it at GET /api/v1/ui.
//
// Enabled is where the app stands and not what the file says. A client
// reading it from a build that carries no app is told the page is not here.
// The three appearance keys are sent whatever the answer, because a deployment
// serving its own page still reads the name and the theme this server was
// configured with.
func appSettings(frontend config.Frontend, diagnostics config.Diagnostics, state string) httpapi.AppSettings {
	return httpapi.AppSettings{
		Enabled:         state == httpapi.AppServed,
		Title:           frontend.Title,
		DefaultLanguage: frontend.DefaultLanguage,
		Theme:           frontend.Theme,
		PrinterAddress:  frontend.PrinterAddress,
		PrinterCustomCA: frontend.PrinterCustomCA,
		Documentation:   version.Documentation(),
		SupportURL:      frontend.SupportURL,
		// The address alone; whether the bundle is served is the diagnostics
		// field, which the page reads first.
		DiagnosticsSendURL: diagnostics.SendTarget(),
	}
}

// withSafeMode adds how this run entered safe mode, when it did.
func withSafeMode(settings httpapi.AppSettings, safe safeMode) httpapi.AppSettings {
	if safe.on() {
		settings.SafeMode = &httpapi.SafeModeState{Reason: safe.Reason, Since: safe.Since}
	}

	return settings
}

// withPrinterAdmission adds how a new printer joins to the app settings. It is
// read from the connectors this process built, because a connector's mode is
// fixed when the driver is built and a change takes effect at the next start.
func withPrinterAdmission(settings httpapi.AppSettings, tables []admissionTable) httpapi.AppSettings {
	settings.PrinterAdmission = printerAdmission(tables)

	return settings
}

// withCoreApp adds the mount that serves the core's own app at the origin root,
// for a build that carries one and a configuration that asks for it. It
// returns the mounts and where the app stands. The readiness report and the
// startup line both say where the app stands.
//
// It is added here and not to the connector list, because the two are used
// differently. A module's root mount is refused on the connector patterns. The
// app claims no root: it answers whatever no other mount did, after every
// operation, every connector and every module root.
//
// core.frontend.enabled off leaves the mount out and does not mount a handler
// that refuses. With no mount the root falls through to a module's root mount
// and then to this server's own refusal, which is also what a build carrying
// no app answers there. A deployment that puts its own page in front of this
// API needs that.
func withCoreApp(mounts []httpapi.Mount, frontend config.Frontend) ([]httpapi.Mount, string) {
	app, carried := httpapi.AppMount()
	if !carried {
		return mounts, httpapi.AppAbsent
	}

	if !frontend.On() {
		return mounts, httpapi.AppOff
	}

	return append(slices.Clone(mounts), app), httpapi.AppServed
}
