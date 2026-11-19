// Package app is the composition root of the core. It reads the
// configuration, constructs every part in dependency order, serves them, and
// stops them in reverse.
//
// # Startup order
//
//  1. the configuration, because a rejected one must bind no port, write no
//     file and start no goroutine
//  2. the logger, whose level and format come from that configuration, so every
//     line after this point is the operator's choice
//  3. the authorization graph, migrated and bootstrapped, before the hub that
//     asks it who may subscribe and before the store that asks it who may see
//     an instance. A first run creates it empty; nothing is fetched
//  4. the event hub, before anything that publishes to it, because the
//     supervision plane's status hook publishes an instance's first transition
//     while it is still starting
//  5. the type registry, which the supervision plane resolves a declared type
//     against
//  6. the supervision plane, running, before the store loads anything.
//     resourcehost.Reconcile hands its plan to the goroutine Run owns and waits
//     for it, so a Reload against a plane that is not running blocks
//  7. the device plane, restored, with the connectors mounted onto it, and
//     both bound onto the supervision plane as the doors a module reads the
//     fleet through. It is before the reconcile because the reconcile starts
//     the modules, and a module reads the doors as it starts
//  8. the resource store, and its first Reload, which is the startup reconcile.
//     A resources block that does not parse fails here, and the instances it
//     declares are registered in the graph's directory as part of it
//  9. the identity layer, which reaches the issuer: a server whose configured
//     issuer cannot be discovered refuses to start, because it would refuse
//     every login afterwards
//  10. the listener, bound before anything is announced, so a port already in
//     use is a startup failure and not a silent one
//
// # Shutdown order
//
//  1. the listener drains first, so no request can reach the store while its
//     instances are being torn down. An event stream is told to end as the
//     drain starts, because a response that never finishes would otherwise hold
//     the drain open to its whole budget
//  2. the supervision plane stops, and every instance with it. The supervisor
//     bounds each Close with its own StopTimeout and names a resource that did
//     not come back
//  3. the wait for the plane is bounded too, because the sum of those budgets
//     is not
//  4. the event hub closes last, so an instance publishing its way out of
//     service still reaches whoever was watching the shutdown
//  5. the authorization graph's handle closes after all of them, because every
//     one of those steps can still ask it a question
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/hostschema"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/betaportal"
	"github.com/prusa3d/prusa-connect/internal/discovery"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/kernel/sqlstore"
	"github.com/prusa3d/prusa-connect/internal/kernel/sysmem"
	"github.com/prusa3d/prusa-connect/internal/platform/auth/dbstore"
	"github.com/prusa3d/prusa-connect/internal/platform/devicestore"
	"github.com/prusa3d/prusa-connect/internal/platform/instanceid"
	"github.com/prusa3d/prusa-connect/internal/platform/observability"
	"github.com/prusa3d/prusa-connect/internal/timeserver"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/loginguard"
	"github.com/prusa3d/prusa-connect/internal/transport/stream"
	"github.com/prusa3d/prusa-connect/internal/transport/webrtc"
)

// Run starts the core and returns when it has stopped. A caller cancels ctx to
// ask it to stop. The only such caller is the signal handler in cmd.
//
// Every failure it reports names what could not be done. A part Run builds
// itself stops the start: the database, the sign-in provider, a listener. The
// process then binds no port.
//
// A resource instance does not stop the start. The supervisor keeps retrying
// one that will not start, and an instance declared required: true makes
// GET /api/v1/readiness answer 503 until it serves. The server serves
// meanwhile.
//
// A first-run wizard confirmed while it runs stops it, and Run then returns
// ErrRestart so the command starts it again on the file the wizard wrote.
func Run(ctx context.Context, opts Options) error {
	ctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)

	opts.restart = func() { stop(ErrRestart) }
	opts.upgradeApply = func() { stop(ErrUpgradeApply) }

	core, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	core = opts.applyTo(core)

	if err = core.Validate(); err != nil {
		return err
	}
	opts.backupRestore = func(id string) error {
		if writeErr := recordBackupRestore(core, id); writeErr != nil {
			return writeErr
		}
		stop(ErrBackupRestore)

		return nil
	}

	if err = requireBuiltParts(core); err != nil {
		return err
	}

	// Before the logger, because the logger counts its own lines into the
	// registry, and a registry built afterwards would leave every line before
	// it uncounted.
	metrics, err := newObserved()
	if err != nil {
		return err
	}

	counting, err := metrics.logging()
	if err != nil {
		return err
	}

	ring := diagnosticsRing(core)

	logger, err := newLogger(core.Log, counting, ring)
	if err != nil {
		return err
	}

	metrics.attach(logger)
	metrics.stamp(opts.version())

	// After applyTo, because the marker sits beside the overlay it resolves.
	opts.safe = takeSafeMode(ctx, logger, core.Overlay, opts.SafeMode, time.Now())
	core = opts.safe.hold(core)

	crashes := keepCrashes(ctx, core, logger)

	applyMemoryLimit(ctx, core.MemoryLimit, logger, debug.SetMemoryLimit)

	started := time.Now()

	logger.InfoContext(ctx, "starting",
		"version", opts.version(),
		"config", opts.ConfigPath, "overlay", core.Overlay)
	warnMissingConfiguration(ctx, logger, opts.ConfigPath)

	err = serve(ctx, core, opts, logger, metrics, ring, crashes, started)

	logger.InfoContext(ctx, "stopped", "uptime", time.Since(started).Round(time.Millisecond).String())

	cause := context.Cause(ctx)
	if errors.Is(cause, ErrRestart) && err != nil && restartsPast(err) {
		logger.WarnContext(ctx, "a supervised resource did not stop in time; restarting without it", "error", err)
	}

	return afterServe(cause, err)
}

// portalTimeout bounds one Beta Portal request, so a portal that stops
// answering cannot hold a pairing or update request open.
const portalTimeout = 30 * time.Second

// serve runs the core over a supervision plane, and stops the plane last
// whatever happened above it. Joining the two errors is deliberate: a shutdown
// that abandoned a wedged resource is worth reporting even when everything else
// went cleanly.
func serve(
	ctx context.Context, core config.Core, opts Options, logger *slog.Logger, metrics *observed,
	ring *observability.LogRing, crashes string, started time.Time,
) error {
	host, closeGraph, err := openGraph(ctx, core, opts.version(), logger)
	if err != nil {
		return err
	}

	defer closeGraph()

	graph := host.graph

	// The connectors whose admission exchange a subscriber may watch. It is
	// bound once the connectors are mounted, which is after the hub exists.
	admission := &admissionOrigins{}

	hub, err := stream.New(stream.Options{
		Site: core.Site,
		Authorizer: visibility{
			graph: graph, admission: admission, dashboardsOff: !core.Dashboards.On(),
			serverID: host.instance, serverAdmin: graph,
		},
		Logger:          logger,
		MaxPerPrincipal: core.Events.MaxStreamsPerPrincipal,
	})
	if err != nil {
		return fmt.Errorf("build the event hub: %w", err)
	}

	// The hub outlives the plane. Every instance publishes its way out of
	// service as it stops, and a hub closed first would drop the last thing an
	// operator watching a shutdown wanted to see.
	defer hub.Close()

	entitlement, portal, err := openPortal(ctx, logger, core, opts.ConfigPath, host.instance)
	if err != nil {
		return err
	}

	// The history provider is not built here. It is a supervised instance like
	// every other resource, declared under `history:` and reconciled by the
	// plane's own feed. A deployment that declares none opens no file, runs no
	// migration and holds no subscription, and that needs no branch here.
	supervision, err := startPlane(
		ctx,
		logger,
		hub,
		metrics,
		moduleDataRoot(core),
		!core.ModulesOn(),
		core.SafeMode,
		core.ModulesSwitchOff,
		opts.Reaper,
	)
	if err != nil {
		return err
	}

	supervision.admission = admission

	served := serveOver(
		ctx, core, opts, logger, supervision, hub, host, metrics, ring, crashes, started, entitlement, portal,
	)

	return errors.Join(served, supervision.shutdown(ctx, logger, core.ShutdownTimeout))
}

// serveOver builds everything that reads or writes the configured inventory and
// serves it. The plane is already running, so the first Reload reconciles. A
// Reload against a plane that is not running would block.
func serveOver(
	ctx context.Context,
	core config.Core,
	opts Options,
	logger *slog.Logger,
	supervision *plane,
	hub *stream.Hub,
	host hostStore,
	metrics *observed,
	ring *observability.LogRing,
	crashes string,
	started time.Time,
	entitlement *betaportal.Store,
	portal httpapi.BetaPortalPairing,
) error {
	graph, driverKinds := host.graph, host.kinds

	// The device models the drivers declared are placed first, so a refused
	// one is logged at the top of the start and not among the connectors.
	models, err := declaredModels(ctx, logger)
	if err != nil {
		return err
	}

	store, err := resource.New(resource.Config{
		Paths:    resource.Paths{File: opts.ConfigPath, Overlay: core.Overlay},
		Plane:    supervision.host,
		Graph:    graph,
		Declared: declaredSettings{types: supervision.types},
		Logger:   logger,

		ModulesOff: !core.ModulesOn(),
		SafeMode:   core.SafeMode,
		Pool:       resource.PoolOf(core.ModulesMemory, sysmem.Total),
		SwitchOff:  core.ModulesSwitchOff,
	})
	if err != nil {
		return fmt.Errorf("build the resource store: %w", err)
	}

	supervision.switchOffs.bind(store)

	// The device plane is built, restored and bound onto the supervision plane
	// before the first Reload, because the reconcile starts a declared module
	// and a module reads the fleet doors from its own start goroutine. The
	// doors are plain fields with no lock, so only this order puts the writes
	// before the reads.
	fleet := newDevices(graph, host.devices, hub, logger, bootstrapOrg, staging{host: supervision.host},
		metrics.meters(), core.DeviceStateBudget)

	// One cache for every picture read out of a file, wherever the file is,
	// and the answers of the modules that read files for the core beside it.
	// The two share core.preview_cache_bytes, so together they hold no more
	// than that.
	analysisBytes := core.PreviewCacheBytes / analysisCacheShare
	previews := newPreviewCache(core.PreviewCacheBytes - analysisBytes)
	fleet.previews = previews
	fleet.listings = newListingCache(core.DeviceListingCacheBytes)
	fleet.models = models

	fleet.prefetchThumbnails = *core.DeviceThumbnailPrefetch

	// The files a person sent to a printer without keeping them. The storage
	// routes write them and a send deletes them; the sweep started below
	// removes what a failed send or a restart left behind.
	incoming := newIncomingCopies(stores{host: supervision.host}, logger)
	fleet.incoming = incoming

	fleet.thumbnails = thumbnailDiskOf(ctx, core, logger)

	// The transfer projection, resolved per read for the reason the history
	// surface is. It is set here and not passed to the constructor, because
	// the supervision plane owns the history instances and the device plane
	// is built beside it, not under it.
	fleet.past = supervision.transferRecords()

	// The write path beside it. A command's and a transfer's own frames go
	// into the store as they are published and do not wait for the feed's
	// next flush. A process that dies without draining then still leaves the
	// row that says what was asked of the machine.
	fleet.durable = supervision.durableWrites()

	// Every printer's session is closed before the hub is, so a printer
	// publishing its way out of service still reaches whoever was watching the
	// shutdown. The listener has already drained by the time this runs.
	defer fleet.Close()

	// The fleet the last run left. It happens before any module starts and
	// before the listener binds, so no caller can read a listing that is half
	// restored, and it puts every stored device down first: a socket the last
	// process held is not one this one has.
	if err = fleet.restore(ctx); err != nil {
		return fmt.Errorf("read the fleet back from the record store: %w", err)
	}

	// A transfer that was in flight when the last process stopped is over: the
	// device has forgotten it and the bytes it holds are a prefix nobody can
	// name. The row is closed as interrupted and nothing is resumed.
	//
	// The sweep runs when the store starts storing and not at this point in
	// the sequence. The declared history instance is not ready here, so a
	// call made here would find no provider and would read nothing. The
	// command sweep uses the same hook.
	//
	// The cost is a short time at the start in which a caller can read a
	// transfer row the last run left open and this run has not closed. The
	// other choice is a sweep that never runs.
	supervision.past.whenStoring(ctx, fleet.sweepInterruptedTransfers)

	// The mounts are everything this server answers that is not an API
	// operation, and the printer routes are the whole of that list. It is one
	// list because it is one origin: httpapi.Server.Handler is all of it, and a
	// route added outside the sealed package is one chi.Walk cannot enumerate
	// and the OpenAPI document will never publish, which is what
	// TestNothingOutsideTheSealedPackageMountsAServeMux refuses.
	// The fleet is the announcer: a code written down for the first time is a
	// frame on the connector's origin, which is what a page adding a printer
	// waits on instead of asking the listing every second.
	registrations, err := devicestore.NewRegistrations(host.db, host.engine, fleet)
	if err != nil {
		return err
	}

	// How a new printer joins, as the first run recorded it. It is read here
	// because a connector's mode is fixed when the driver is built, and this is
	// the last moment before that. A choice recorded while the process runs
	// takes effect at the next start, and the first-run page says so.
	// A confirmed first-run wizard may have left a decision for this start.
	// Its admission choice is written before the read below, and the rest
	// after the identity layer is built.
	pending, err := admitPending(opts.ConfigPath, store)
	if err != nil {
		return err
	}

	admissionChoice, err := store.AdmissionMode()
	if err != nil {
		return fmt.Errorf("read how a new printer joins this server: %w", err)
	}

	built, err := mountConnectors(ctx, core, opts.PrinterAddr, logger, fleet,
		driverKinds, registrations, admissionChoice)
	if err != nil {
		return err
	}

	// The state sets the mounted dialects declared, served in the device-state
	// vocabulary.
	states, err := placedStates(built.states)
	if err != nil {
		return err
	}

	// Which devices are waiting to be let onto this server, over every
	// connector that holds a registration table. A dialect that admits on
	// arrival contributes nothing and the surface is empty. The API and a
	// module admit through one value, so an approval from either lands on the
	// same tables and re-binds through the same fleet.
	admissions := newRegistrations(built.admissions, fleet)

	// The origins the registration events are published on. Without this a
	// subscriber holding admit_device would be offered no connector to watch,
	// and the three frames would reach nobody.
	supervision.admission.bind(admissionNames(built.admissions))

	// A module reads the fleet through its own principal, and this is where the
	// plane learns what to read. Both doors are bound here, after the
	// connectors, because the registration tables are the connectors' and the
	// connectors were mounted onto the fleet a moment ago. Nothing that reads
	// them runs until the Reload below.
	supervision.fleets.bind(fleet)
	supervision.fleets.bindStates(states)
	supervision.fleets.bindAdmission(&admissions, graph)

	// A sink that takes samples is handed the whole tree of each printer it
	// can read, read from the fleet.
	supervision.feed.readTrees(fleet.treeOf)

	// The one way into what a storage holds. The HTTP routes and a module's
	// storage.read both go through it, so both take one permission check and
	// one confinement rule.
	// A module that reads a file type for the core reads that one file
	// through a handle the host makes per call.
	handles := newFileHandles()
	storageReader := storagePlane{
		graph: graph, stores: stores{host: supervision.host}, previews: previews, incoming: incoming,
		analysis: newFileAnalysis(supervision.host, handles, analysisBytes),
	}

	supervision.storages.bind(storageReader)
	supervision.storages.bindHandles(stores{host: supervision.host}, handles)

	// The camera plane, which an HTTP push and a module's camera.accept both
	// go through, so both take one permission check and land in one driver.
	// It is built here, before the first reconcile starts a module.
	relay, err := webrtc.New(webrtc.Config{Logger: logger, LoopbackOnly: opts.LoopbackOnly})
	if err != nil {
		return fmt.Errorf("build the media relay: %w", err)
	}

	cameraPlane := newCameras(graph, supervision.host, relay, logger)
	cameraPlane.hub = hub
	supervision.cameraFrames.bind(cameraPlane)

	cameraBook := &cameraSettings{
		rows: host.devices, graph: graph, plane: supervision.host,
		types: declaredSettings{types: supervision.types}, org: bootstrapOrg, log: logger,
	}
	fleet.dropCameras = cameraBook.withdraw

	// The graph a module asks about somebody it has served. It is the same
	// store every other permission check reaches.
	supervision.accessors.bind(graph)

	// The graph a module's call in a person's name to another module is
	// decided by, the one the HTTP mount decides the person's own request by.
	supervision.routes.bind(graph)

	// What the connectors mounted beside the API, so a module's root mount is
	// refused where a printer dials. Bound here for the reason the fleet is:
	// the mounts exist and no module has started yet.
	supervision.beside.bind(httpapi.PatternsOf(built.mounts))

	// The first-run decision, read before the identity layer is wired, because
	// it picks which resolver this process runs with. An install that has
	// recorded none serves only the probes, the page and the first run itself
	// until one is there.
	//
	// It is read before the reconcile, because the fleet reads it when a
	// printer arrives and a module may bring a printer as it starts.
	recorded, err := store.SetupMode()
	if err != nil {
		return fmt.Errorf("read what this server does about identity: %w", err)
	}

	mode := newSetupMode(recorded)
	fleet.openOwner = openOwnerOf(core, mode)

	if err = reconcileDeclared(ctx, logger, core, store, cameraBook, opts.ConfigPath); err != nil {
		return err
	}

	who, err := identityFor(ctx, core, host, supervision, mode, logger)
	if err != nil {
		return err
	}

	// The credential table is swept while the server runs, and the sweep is
	// waited for on the way out, before the database closes.
	defer startCredentialSweep(ctx, who.expired, logger)()

	code, shown, err := newSetupGate(core.Auth.SetupCode, logger, time.Now)
	if err != nil {
		return err
	}

	first := newFirstRun(core, store, mode, code, who.accounts, opts.version(), logger)
	first.configPath, first.restart = opts.ConfigPath, opts.restart
	first.apiFlag, first.listening = opts.APIAddr, core.API.Addr

	// The decision a confirmed wizard left is recorded first. Then an install
	// that predates this key and already had a login, an account or a
	// principal the operator chose has its decision written, so that install
	// keeps working as it did.
	if err = first.settleWith(ctx, pending); err != nil {
		return err
	}

	announce, err := announceOrSpend(ctx, first, code, shown, core, opts, logger)
	if err != nil {
		return err
	}

	// The dial loops, and the endpoint list that feeds them. They stop before
	// the device plane does, because the defer above was registered first: a
	// loop still running while fleet.Close tears sessions down would dial a
	// printer into a plane that is shutting.
	dials := startDialling(ctx, logger, built.dialled)
	defer dials.stop()

	// The store hands it the list as it stands and again whenever a write moves
	// it, so adding a printer needs a call and no restart.
	store.WatchEndpoints(ctx, dials)

	// Where the web app stands, decided once: this build either carries it or
	// does not, and core.frontend.enabled was read from the file at load. The
	// readiness report and the startup line below both say the same word.
	mounts, appState := withCoreApp(built.mounts, core.Frontend,
		modulePlane{host: supervision.host, store: store}.IsolatedTab, opts.appMounter())

	// What this server tells the local network about itself. It is built here
	// and started in listen, because the record names the port bind was
	// granted. Nothing it carries comes from a configuration key the record
	// does not name. The link is the one mDNS socket of the process.
	link := discovery.NewLink(logger, metrics.counter("pcl_mdns_packets_dropped_total"))
	announcer := discovery.New(discovery.Config{
		Enabled:    core.Discovery.On(),
		Services:   core.Discovery.Services,
		Instance:   core.Frontend.Title,
		Version:    opts.version(),
		ID:         host.instance,
		TLS:        core.API.TLS.Enabled(),
		SystemHost: core.Discovery.SystemHost(),
	}, logger, link)

	// The listener hears the other printers on the same network. It is
	// switched off with the announcer, and it listens for the two names a
	// printer and a server answer to, whatever this server announces itself as.
	listener := discovery.NewListener(discovery.ListenerConfig{
		Enabled:  core.Discovery.On(),
		Services: config.NearbyServices(),
		Self:     announcer.Host,
	}, logger, link)

	nearby, err := newLocalNetwork(ctx, core, announcer, listener, store)
	if err != nil {
		return err
	}

	nearby.peers = fleet.peers

	clock := newTimeService(ctx, core, timeserver.New(timeserver.Addr, logger), timeserver.Port, store, logger)
	defer clock.stop()

	// The store hands it the origins of the modules that are switched on, as
	// they stand and again with every write, so an origin needs no restart.
	origins := newBrowserOrigins(ctx, core, store, logger)
	store.WatchModuleOrigins(ctx, origins)

	probes := readiness{store: store, host: supervision.host, app: appState, announcer: announcer}

	// The limit on login attempts, which the credential store cannot enforce for
	// itself: it is handed a username and a password, and a limit that counts
	// needs to know who is asking. Validate has already refused a value this
	// cannot read.
	trusted, err := core.TrustedPrefixes()
	if err != nil {
		return err
	}

	// The periodic capture, so a job can be replayed with pictures later. It
	// records nothing unless a camera's declaration switches it on. Its
	// goroutines are cancelled with this context, so no capture writes into a
	// storage the plane below has already torn down.
	recorder := newCameraHistory(supervision.host, hub, stores{host: supervision.host}, logger)
	recordCtx, stopRecording := context.WithCancel(ctx)
	recording := make(chan struct{})

	go func() {
		defer close(recording)

		recorder.Run(recordCtx)
	}()

	// It stops on the way out whatever ended the run, and not only on a
	// cancelled context: serving stops for reasons of its own, and a capture
	// left running would be writing into a storage the plane below is tearing
	// down.
	defer func() {
		stopRecording()
		<-recording
	}()

	// The sweep of the temporary copies, stopped for the reason the capture
	// above is: it deletes inside storages the plane below tears down.
	sweepCtx, stopSweeping := context.WithCancel(ctx)
	sweeping := make(chan struct{})

	go func() {
		defer close(sweeping)

		incoming.Run(sweepCtx)
	}()

	defer func() {
		stopSweeping()
		<-sweeping
	}()

	// The pages of cards the home page draws, kept in the host database beside
	// the graph that decides who sees which.
	boards, boardsOff, err := servedDashboards(ctx, core, host, hub, logger)
	if err != nil {
		return err
	}

	maintenance, err := newMaintenance(
		ctx, core, opts.ConfigPath, opts.version(), fleet, supervision.host, hub, host.instance,
		entitlement, opts.upgradeApply, opts.backupRestore,
	)
	if err != nil {
		return fmt.Errorf("build upgrade service: %w", err)
	}

	installer := newModuleInstaller(
		core, opts.ConfigPath, graph, supervision.host, store, graph, logger, entitlement)
	maintenance.requestRestart, maintenance.logger = opts.restart, logger
	// A core upgrade's module plan reads the same index an install does.
	maintenance.catalog = installer

	api, err := httpapi.New(httpapi.Config{
		Version:     opts.version(),
		InstanceID:  host.instance,
		Logger:      logger,
		Principals:  who.principals,
		Sessions:    who.sessions,
		Accounts:    who.accounts,
		Profiles:    who.profiles,
		Setup:       first,
		Credentials: who.credentials,
		Pairings:    who.pairings,
		AppPairings: who.appPairings,
		Logins:      loginguard.New(loginguard.Options{TrustedProxies: trusted, Logger: logger}),
		Readiness:   probes,
		Diagnostics: servedDiagnostics(diagnosticsBundles{
			version: opts.version(), started: started, core: core, setupCode: shown,
			ring: ring, crashes: crashes, devices: fleet, store: store, cameras: cameraBook, probes: probes,
			accounts: who.accounts,
			mode:     mode, sources: supervision.host.DiagnosticsSources,
		}),
		Resources:       store,
		Presets:         store,
		Dashboards:      boards,
		DashboardsOff:   boardsOff,
		ModulesOff:      !core.ModulesOn(),
		Discovery:       nearby,
		TimeServer:      clock,
		Links:           newLinkStore(store, supervision.host, graph, instanceEmitter(hub, metrics.meters())),
		Endpoints:       store,
		Grants:          graph,
		Verbs:           graph,
		Access:          graph,
		Devices:         fleet,
		Cameras:         cameraPlane,
		CameraSettings:  cameraBook,
		Stores:          storageReader,
		Connectors:      fleet,
		Events:          hub,
		History:         supervision.historyStore(graph),
		Maintenance:     maintenance,
		UpgradesOff:     maintenance.upgradesOff(),
		BetaPortal:      portal,
		Metrics:         metrics.served(core.Observability.On()),
		MetricsListener: core.Observability.Addr != "",
		FleetTelemetry:  fleetTelemetry{graph: graph},
		Served:          metrics.timeRequest,
		Heartbeat:       core.Events.Heartbeat,
		Retry:           core.Events.Retry,
		InsecureCookies: !who.secureCookies,
		Origins:         origins,
		RequestBudget:   core.API.RequestTimeout,
		StreamBudget:    core.API.StreamTimeout,
		Mounts:          mounts,
		App: withPrinterAdmission(
			withSafeMode(appSettings(core.Frontend, core.Diagnostics, appState), opts.safe),
			built.admissions,
		),
		Registrations:    admissions,
		PrinterAuthority: printerAuthority(core.Frontend, opts.ConfigPath),
		DeviceStates:     states,
		// A module's own HTTP surface, under /api/v1/modules/<namespace>/, and
		// the graph that decides who may reach one. They are passed together
		// because httpapi refuses a module plane with no graph behind it: a
		// module route is authorized on the module's instance the way every
		// other path under /api/v1 is authorized.
		Modules:      modulePlane{host: supervision.host, store: store},
		ModuleAccess: graph,
		// Installing one, which is a write to this server's own configuration
		// and not a call into a module. It runs the functions `module add` and
		// `module remove` run, so the command and the API check the same things.
		// The host goes with it: an uninstall stops the module's process before
		// it deletes the module's files, because a running process holds its
		// own executable open.
		//
		// The store goes with it too. An install writes the overlay without
		// going through the store, so the store is asked to read it again and
		// converge. That starts the module, serves its bundle and puts it on
		// the resources listing in the same request, with no restart.
		//
		// The graph goes with it as the writer too, so an approved install
		// writes the module's grants and a removal takes them back, exactly as
		// the commands do.
		ModuleInstall: installer,
		// What those modules are keeping. The same plane and the same graph
		// decide who may see it.
		State: supervision.records.stateStore(),
		// The same backend, read one key at a time for the signed-in person.
		// A module's settings pane writes through it.
		ModuleState:        supervision.records.settingsStore(),
		MaxStateValueBytes: int64(core.ModuleSettingsBytes),
		DeviceModels:       models,
	})
	if err != nil {
		return fmt.Errorf("build the API surface: %w", err)
	}

	logger.InfoContext(ctx, "event hub ready", "site", core.Site, "epoch", hub.Epoch())

	// One line an operator greps for when the origin root answers 404. "off"
	// is core.frontend.enabled; "absent" is a build that carries no app at all,
	// which no key in the file can change.
	logger.InfoContext(ctx, "the web app", "state", appState,
		"title", core.Frontend.Title, "language", core.Frontend.DefaultLanguage,
		"theme", core.Frontend.Theme)

	// The metrics the file asked for. `make build` reads the same key to decide
	// whether the client is linked, so one line says what this binary was built
	// for and what this run was asked for.
	logger.InfoContext(ctx, "the metrics",
		"served", core.Observability.On(), "addr", core.Observability.Addr)

	letGo := first.letPendingGo(ctx)

	return listen(ctx, core, built.listeners, logger, api, probes, nearby, func() {
		letGo()
		announce()
	})
}

// bootstrapOrg is the org the first grant is written on. A resource instance
// does not federate and the graph is node-local, so one name
// serves every deployment and a reader of the tuple table can tell the grant
// this server wrote itself from one an operator wrote.
const bootstrapOrg = "local"

// openGraph opens the authorization graph, brings its schema up to date, writes
// the first grant when it has none, and returns it with the function that
// closes the handle.
//
// A first run creates the file and starts empty. Nothing is fetched: an
// air-gapped install has no network, and a server that needs one to decide who
// may read a printer is a server that stops working when the link does.
func openGraph(
	ctx context.Context, core config.Core, version string, logger *slog.Logger,
) (hostStore, func(), error) {
	db, dialect, where, err := openDatabase(ctx, core)
	if err != nil {
		return hostStore{}, nil, err
	}

	closeGraph := func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.ErrorContext(ctx, "closing the authorization graph", "error", closeErr.Error())
		}
	}

	if dialect == authz.DialectPostgres {
		if err = awaitDatabase(ctx, db.PingContext, where, logger, databaseBackoff(), databaseWait); err != nil {
			closeGraph()

			return hostStore{}, nil, err
		}
	}

	// The migration covers the whole host schema and not the graph alone, so
	// every owner's tables arrive before anything reads one.
	migrator, err := hostschema.Migrator(db, engineOf(dialect), version)
	if err == nil {
		err = migrator.Migrate(ctx)
	}

	if err != nil {
		closeGraph()

		return hostStore{}, nil, fmt.Errorf("migrate the host database at %s: %w", where, err)
	}

	graph, err := authz.Open(db, dialect, authz.WithOrgAdminHandover(core.Authz.OrgAdminHandover))
	if err != nil {
		closeGraph()

		return hostStore{}, nil, fmt.Errorf("read the authorization graph at %s: %w", where, err)
	}

	// The schema knows no legal relation shape until the vocabulary has been
	// written into it, and the tuple table's foreign key points at that.
	if err = graph.Reconcile(ctx); err != nil {
		closeGraph()

		return hostStore{}, nil, fmt.Errorf("write the vocabulary into the authorization graph: %w", err)
	}

	// The per-build half of the verb table, installed once, from the
	// declarations compiled into this binary. It is after Reconcile because
	// the pin it reads is in a migrated table, and before anything serves,
	// because a verb the table does not classify reaches no device.
	drivers, err := installDeviceKinds(ctx, logger, graph)
	if err != nil {
		closeGraph()

		return hostStore{}, nil, err
	}

	if err = bootstrapGraph(ctx, core, logger, graph, where); err != nil {
		closeGraph()

		return hostStore{}, nil, err
	}

	logger.InfoContext(ctx, "authorization graph ready",
		"engine", core.Persistence.Type, "database", where, "epoch", graph.Epoch())

	records, err := devicestore.New(db, engineOf(dialect))
	if err != nil {
		closeGraph()

		return hostStore{}, nil, err
	}

	// The id a client knows this server by, written on the first start and read
	// back on every later one.
	instance, err := instanceid.Ensure(ctx, db, engineOf(dialect), time.Now())
	if err != nil {
		closeGraph()

		return hostStore{}, nil, err
	}

	return hostStore{
		graph: graph, devices: records, db: db, engine: engineOf(dialect), kinds: drivers, instance: instance,
	}, closeGraph, nil
}

// hostStore is the host's own database as everything above it reads it: the
// authorization graph, the handle the other host-owned tables are read through,
// which engine that handle speaks, and the device kinds this build's drivers
// registered.
//
// The handle is kept beside the graph because the accounts, the credentials
// and the registrations are tables in the same database under the same
// ledger. One handle, one migration, one backup.
type hostStore struct {
	graph   *authz.Store
	devices *devicestore.Store
	db      *sql.DB
	engine  dbstore.Engine
	kinds   map[string]authz.DeviceKind
	// instance is the id this server answers to on every address it has.
	instance string
}

// identityFor builds the identity layer and hands its list of people to the
// supervision plane. The two are one step, so the plane cannot run with an
// identity layer whose accounts it never learned about. The plane's modules
// have already started, and the plane keeps nobody before the bind.
func identityFor(
	ctx context.Context, core config.Core, host hostStore,
	supervision *plane, mode *setupMode, logger *slog.Logger,
) (identity, error) {
	who, err := buildIdentity(ctx, core, host, supervision.host, mode, logger)
	if err != nil {
		return identity{}, err
	}

	supervision.people.bind(who.people)

	return who, nil
}

// engineOf names the graph's dialect the way the stores beside it spell it. The
// two enumerations are separate because neither package may own the other's
// vocabulary, and this is the one place a value crosses.
func engineOf(dialect authz.Dialect) dbstore.Engine {
	if dialect == authz.DialectPostgres {
		return dbstore.EnginePostgres
	}

	return dbstore.EngineSQLite
}

// openDatabase opens the engine the configuration names and returns the handle,
// the dialect the store renders its SQL for, and the name a message may print.
//
// One selector decides it, because the graph, the records and the queue share a
// database. On Postgres the printable name is the DSN with its password taken
// out; on SQLite it is the file.
func openDatabase(ctx context.Context, core config.Core) (*sql.DB, authz.Dialect, string, error) {
	pool := poolOf(core.Persistence)

	if core.Persistence.Type == config.EnginePostgres {
		where := core.Persistence.Postgres.RedactedDSN()

		db, err := authz.OpenPostgres(core.Persistence.Postgres.DSN, pool)
		if err != nil {
			return nil, 0, "", fmt.Errorf("open the database at %s: %w", where, err)
		}

		return db, authz.DialectPostgres, where, nil
	}

	db, err := authz.OpenSQLite(ctx, core.Authz.Database, pool)
	if err != nil {
		return nil, 0, "", err
	}

	return db, authz.DialectSQLite, core.Authz.Database, nil
}

// poolOf is the operator's database block as the store layer takes it. The
// keys are filled in by WithDefaults, so every field here is a real bound.
func poolOf(persistence config.Persistence) sqlstore.Pool {
	return sqlstore.Pool{
		MaxOpen:          persistence.MaxOpenConns,
		MaxIdle:          persistence.MaxIdleConns,
		MaxLifetime:      persistence.ConnMaxLifetime,
		StatementTimeout: persistence.Postgres.StatementTimeout,
		PageCacheBytes:   persistence.SQLite.PageCacheBytes,
	}
}

// bootstrapGraph writes the first grant into a graph that holds no tuple, and
// says at startup whether the principal this deployment serves can do anything.
//
// The first owner is core.principal, which is who an unauthenticated deployment
// serves every request as and who the first person to log in through a
// configured issuer is expected to be. It is a write into the graph and not a
// rule about an empty one: the emptiness is read once, inside the transaction
// that ends it, and every decision afterwards reads the projection like any
// other. There is no later state in which it is consulted again. A later start
// with a different core.principal writes the first grant again for that
// principal, once.
func bootstrapGraph(
	ctx context.Context, core config.Core, logger *slog.Logger, graph *authz.Store, where string,
) error {
	owner, err := authz.NewSubject(humanSubjectType, principalID(core))
	if err != nil {
		return fmt.Errorf("core.principal is %q, which the authorization graph cannot key on: %w",
			core.Principal, err)
	}

	// An issuer added to config.yaml by hand, without the wizard, leaves the
	// default here. Nobody at the issuer has that subject, so nobody who signs
	// in administers the server, and only the server can say so.
	if core.Auth.Enabled() && core.Principal == config.DefaultPrincipal {
		logger.WarnContext(ctx, "core.principal is still the default, so nobody who signs in at the identity "+
			"provider administers this server, and the local username and password login is refused while an "+
			"issuer is set; set it to the administrator's subject, which the sign-in log line names, and restart",
			"principal", core.Principal, "issuer", core.Auth.Issuer)
	}

	written, err := graph.Bootstrap(ctx, bootstrapOrg, owner)
	if err != nil {
		return fmt.Errorf("write the first grant into the authorization graph: %w", err)
	}

	if written {
		logger.InfoContext(ctx, "the authorization graph was empty; the first grant was written",
			"owner", owner.String(), "org", bootstrapOrg)

		return nil
	}

	// An operator who changes core.principal after the first run, which the
	// setup wizard does when it names the administrator at an issuer, gets
	// the first grant written again for the new one.
	handed, err := graph.HandOver(ctx, bootstrapOrg, owner)
	if err != nil {
		return fmt.Errorf("make core.principal the administrator of the authorization graph: %w", err)
	}

	if handed {
		logger.InfoContext(ctx, "core.principal changed; it was made the administrator of the org",
			"owner", owner.String(), "org", bootstrapOrg)
	}

	// A graph whose org edges an operator withdrew still reaches nothing.
	// The warning at startup tells the operator what is missing, so the
	// server does not only look broken.
	held, err := holdsAResourceKind(ctx, graph, owner)
	if err != nil {
		return err
	}

	if !held {
		logger.WarnContext(ctx, "the configured principal holds no grant on any resource kind",
			"principal", owner.String(), "database", where)
	}

	return nil
}

// holdsAResourceKind reports whether this principal reaches any of the kinds the
// first grant is written on.
//
// The question is asked of the kinds and not of the configured instances. This
// runs before the first Reload, so the instance directory holds what the last
// run registered, and on a deployment that declares no instance it holds
// nothing on every run. Reading it would warn about a graph that is correct,
// on every start after the first.
func holdsAResourceKind(ctx context.Context, graph *authz.Store, owner authz.Subject) (bool, error) {
	for _, kind := range ref.ConfigurableKinds() {
		object, err := authz.NewObject(authz.TypeResourceKind, kind.String())
		if err != nil {
			return false, fmt.Errorf("name the %s resource kind in the authorization graph: %w", kind, err)
		}

		allowed, err := graph.Can(ctx, owner, authz.ActionViewResource, object)
		if err != nil {
			return false, fmt.Errorf("read what %s may see: %w", owner, err)
		}

		if allowed {
			return true, nil
		}
	}

	return false, nil
}

// logOverCommit warns when the enabled modules' budgets add up to more than
// the pool. The start never refuses one, so this is where an operator learns
// that the machine was promised more than it holds.
func logOverCommit(ctx context.Context, logger *slog.Logger, store *resource.Store) {
	const mib = 1 << 20

	promised, pool := store.PoolUse()
	if !pool.Known || promised.Bytes <= pool.Bytes {
		return
	}

	logger.WarnContext(ctx, "modules promise more memory than the pool holds",
		"promised_mib", promised.Bytes/mib, "pool_mib", pool.Bytes/mib)
}
