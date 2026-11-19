package httpapi

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/prusa3d/prusa-connect/internal/kernel/devicestate"
	"github.com/prusa3d/prusa-connect/internal/printer/profile"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/internal/transport/http/loginguard"
)

// Errors New reports. Callers inspect them with errors.Is.
var (
	// ErrNoPrincipals reports a Config with no way to identify a caller. There
	// is no default: an unauthenticated server is a decision somebody makes on
	// purpose, in their own Principals implementation.
	ErrNoPrincipals = errors.New("httpapi: Config.Principals is required")
	// ErrNoResourceStore reports a Config with no configured inventory. The
	// resource operations are published unconditionally, so the document does
	// not depend on how the process was wired.
	ErrNoResourceStore = errors.New("httpapi: Config.Resources is required")
	// ErrNoGCodePresetStore reports a Config with no store behind the gcode
	// presets. They are published unconditionally, like every other operation,
	// so what a client generates from must not depend on how this process was
	// wired.
	ErrNoGCodePresetStore = errors.New("httpapi: Config.Presets is required")
	// ErrNoEndpointStore reports a Config with no store behind the addresses
	// this server dials. They are published unconditionally, like every other
	// operation, so what a client generates from must not depend on how this
	// process was wired.
	ErrNoEndpointStore = errors.New("httpapi: Config.Endpoints is required")
	// ErrNoGrants reports a Config with no authorization graph behind the grant
	// operations. They are published unconditionally, like every other
	// operation, so a server that cannot answer who may grant what must not
	// start rather than publish paths that answer nothing.
	ErrNoGrants = errors.New("httpapi: Config.Grants is required")
	// ErrNoSessions reports a Config with no login ceremony. There is no
	// default for the same reason there is none for Principals: a deployment
	// with no issuer wires an implementation that refuses to begin a login,
	// rather than one that is missing.
	ErrNoSessions = errors.New("httpapi: Config.Sessions is required")
	// ErrNoAccounts reports a Config with no local credential store. There is no
	// default for the same reason there is none for Sessions: a deployment that
	// holds no accounts wires an implementation that refuses to create one and
	// refuses every password, rather than one that is missing.
	ErrNoAccounts = errors.New("httpapi: Config.Accounts is required")
	// ErrNoReadiness reports a Config that cannot answer whether the process is
	// ready. The operation is published unconditionally, so a server that
	// cannot compute the verdict must not start rather than publish a path that
	// answers nothing.
	ErrNoReadiness = errors.New("httpapi: Config.Readiness is required")
	// ErrNoDeviceFleet reports a Config with no device plane behind the printer
	// operations. They are published unconditionally, like every other
	// operation, so a server that cannot answer which printers exist must not
	// start rather than publish paths that answer nothing.
	ErrNoDeviceFleet = errors.New("httpapi: Config.Devices is required")
	// ErrNoStorages reports a Config with no storage plane. The routes are
	// published whatever a deployment declared, so something has to answer them.
	ErrNoStorages = errors.New("httpapi: Config.Stores is required")
	// ErrNoEventStream reports a Config with no hub behind the event stream. The
	// operation is published unconditionally for the same reason the others are:
	// what a client generates from must not depend on how this process was
	// wired.
	ErrNoEventStream = errors.New("httpapi: Config.Events is required")
	// ErrUndocumentedRoute reports a route the mux will serve that the OpenAPI
	// document does not publish, or the reverse.
	ErrUndocumentedRoute = errors.New("httpapi: route and OpenAPI document disagree")
	// ErrMountRefused reports a Mount this server will not serve: one with no
	// handler, one with no reason, or one claiming a path this server publishes
	// itself.
	ErrMountRefused = errors.New("httpapi: mount refused")
	// ErrNoModuleAccess reports a Config carrying a module plane and no
	// authorization graph to gate it. There is no default: a module surface
	// that authorized on nothing would be reachable by anybody who can reach
	// the port.
	ErrNoModuleAccess = errors.New("httpapi: Config.ModuleAccess is required whenever Config.Modules is set")
)

const (
	defaultTitle       = "Prusa Connect"
	defaultVersion     = "0.0.0"
	defaultMaxBodySize = 1 << 20
	// defaultMaxUploadSize bounds a body that is a file.
	//
	// The wire could carry more: a printer transfer describes its length in a
	// uint32, so its limit is 4 GiB. But a deployment sits behind a proxy with
	// a body limit of its own. With different limits in one path, a person
	// waits out an upload and is refused by the lowest one. The frontend
	// (packages/svelte/src/data/services/storage.ts) and the nginx manifests
	// refuse at this same number.
	defaultMaxUploadSize = 200 << 20

	// Bounds on the wire. The zero value of each is no timeout at all.
	// ReadHeaderTimeout matters on the way in: without it a connection that
	// never finishes its headers holds a goroutine forever. WriteTimeout
	// matters on the way out: /docs/scalar.js is several MiB, and a reader
	// that never drains it would hold the goroutine and the buffer.
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second

	// The budgets of a Config that sets none, which is a test or an embedded
	// build. They repeat the two defaults of internal/app/config, and the
	// reasons for both numbers are on the keys there.
	defaultRequestBudget = 30 * time.Second
	defaultStreamBudget  = 6 * time.Hour
)

// Config is what a server needs. New refuses a Config that leaves a required
// field nil, with one of the ErrNo errors above.
type Config struct {
	// Title and Version identify the API in the OpenAPI document.
	Title   string
	Version string
	// InstanceID is the id this server answers to on every address. The health
	// answer carries it.
	InstanceID string
	// Logger is where a panic and a refused identity are reported. Discarded
	// when nil.
	Logger *slog.Logger
	// Principals resolves the caller of a request.
	Principals Principals
	// Sessions runs the login ceremony and owns the cookie a session rides in.
	Sessions Sessions
	// Accounts is the local credential store behind the password login and the
	// first-run path. A deployment with an issuer wires one that refuses both.
	Accounts Accounts
	// Profiles is what the identity provider said about the people who signed
	// in there, which this server keeps no account for. Nil on a deployment
	// with no issuer.
	Profiles Profiles
	// Setup is the first run: whether this install has recorded what it does
	// about identity, and the one call that records it. Nil is a server that
	// has, which is what a test and an embedded build take: an install whose
	// decision is already in its configuration must not have to record a second
	// one to answer anything.
	Setup Setup
	// Credentials mints and revokes api credentials. Nil is a server that mints
	// none and refuses to, which is what a deployment that authenticates nobody
	// wires: a token nothing would ever verify is not minted.
	Credentials Credentials
	// AppPairings is how an app asks a person for a credential. Nil is a server
	// that pairs no app, which is what a server that mints no credential wires.
	AppPairings AppPairings
	// Pairings maps the subjects an identity provider names onto local
	// principals, for a bearer token another issuer signed. Nil is a server
	// that verifies none and holds no pairing.
	Pairings Pairings
	// Logins holds a caller in front of the password check for as long as the
	// attempts already charged to their account and their address say. It is
	// the limit on attempts that the credential store cannot enforce, because
	// the store is handed a username and a password and this surface is handed
	// the request. Nil takes the shipping numbers and trusts no proxy.
	Logins *loginguard.Guard
	// Readiness is the process's own verdict, served at /api/v1/readiness.
	Readiness Readiness
	// Diagnostics builds the support bundle served at /api/v1/diagnostics. Nil
	// answers 404, and so does a build without the diagnostics tag.
	Diagnostics Diagnostics
	// Maintenance prepares core upgrades and protects their data, and reports
	// the running version and restarts the server. Nil publishes the contract
	// and answers not found from each maintenance operation.
	Maintenance Maintenance
	// UpgradesOff is core.upgrade.enabled switched off: each upgrade and backup
	// operation answers not found, and get-ui-settings says so. The server
	// version and the restart are still served. The zero value keeps upgrades
	// and backups on.
	UpgradesOff bool
	// BetaPortal runs OAuth device pairing without exposing the device code,
	// token or proof key. Nil publishes the same contract and answers not found.
	BetaPortal BetaPortalPairing
	// App is core.frontend as the page reads it: the name, the language and the
	// theme, served unauthenticated at /api/v1/ui. The zero
	// value is a deployment that serves no app, which is what a caller reading
	// it is told.
	App AppSettings
	// AllowedOrigins are the browser origins a page may call this API from,
	// each exact and matched byte for byte against the Origin header. Empty
	// sends no CORS header at all, so a page from another origin cannot read a
	// response. A wildcard is refused by New: the API takes a credential, and a
	// browser sends none to an origin that answers every page with *.
	//
	// It applies to the API listener alone. The device listener serves wires a
	// printer dials, and a printer is not a browser.
	AllowedOrigins []string
	// InsecureCookies drops the Secure attribute from the session cookie. It is
	// set from the scheme of the redirect URI the issuer sends the browser back
	// to, because a Secure cookie on an origin a browser reaches over http is
	// one the browser never sends back. It is not a switch of its own: nothing
	// else reads it and no credential check consults it.
	InsecureCookies bool
	// Resources is the configured inventory the resource operations read and
	// write. Its implementation owns the machine-owned overlay and merges the
	// operator's hand-authored file over it.
	Resources ResourceStore
	// Presets is the stored gcode sequences the preset operations read and
	// write. It shares the machine-owned overlay with Resources, so one
	// implementation satisfies both and a write to either fences the other.
	Presets GCodePresetStore
	// Dashboards is the pages of cards the home page draws. The paths are
	// published unless DashboardsOff is set or the build leaves them out. Nil,
	// with the paths published, answers an empty listing and a 404 for any id.
	Dashboards DashboardStore
	// DashboardsOff is core.dashboards.enabled switched off: no dashboard
	// operation is served, and get-ui-settings says so. A build without the
	// dashboards serves none whatever this says.
	DashboardsOff bool
	// ModulesOff is core.modules_enabled switched off: get-ui-settings says
	// so, and the app hides module installation. The zero value keeps modules
	// on.
	ModulesOff bool
	// Links is the configured links and what each running link observes of the
	// nodes around it. Nil answers an empty listing and a 404 for any name,
	// which is a build carrying no link type, and the paths are published
	// either way because the document is a contract.
	Links LinkStore
	// Discovery is what this server hears and announces on the local network.
	// Nil is a build that wires neither, and the list it serves is empty.
	Discovery Discovery
	// TimeServer is the SNTP server on the local network. Nil reports off and
	// answers not found to a change.
	TimeServer TimeServer
	// Endpoints is the addresses this server dials, which the endpoint
	// operations read and write. It shares the machine-owned overlay with
	// Resources, so one implementation satisfies both and a write to either
	// fences the other.
	Endpoints EndpointStore
	// Grants is the authorization graph the grant operations read and write.
	Grants GrantStore
	// Verbs is the dangerous tier and the first-seen record: the rows that
	// reach a verb no class, role or level confers, and the acceptances that
	// move what the host is holding. The paths are published either way,
	// because the document is a contract and a client expects every path of
	// the release.
	Verbs VerbGrantStore
	// Access is the same graph as the permission review reads it: what the
	// caller may do, rather than which tuples say so. Nil publishes no check
	// path at all.
	Access AccessReview
	// Stores is the server's own storage, which the storage operations browse
	// and change.
	Stores Storages
	// Devices is the device plane the device operations read and command. It
	// is what a connector's Arrivals registered, not anything configuration
	// declared: a device dials in and is admitted.
	Devices DeviceFleet
	// Cameras is the configured cameras the media operations read. Nil
	// publishes no camera path at all, which is a build with no media plane.
	Cameras CameraFleet
	// CameraSettings is the cameras people add and change. Nil publishes none
	// of their paths.
	CameraSettings CameraSettings
	// Registrations is the devices waiting to be let onto this server, over
	// every connector that holds a registration table. Nil answers an empty
	// listing and a 404 for any code, which is a deployment whose connectors
	// admit a device the moment it arrives.
	Registrations DeviceRegistrations
	// PrinterAuthority reads the DER certificate of this server's own authority,
	// the file a printer loads from its USB drive as connect.der. It returns
	// fs.ErrNotExist when the deployment makes none. Nil answers 404.
	PrinterAuthority func() ([]byte, error)
	// Events is the hub the event stream serves from.
	Events EventStream
	// History resolves the stored past the history operations read, per
	// request. Nil, and a resolver that answers none, are a deployment with no
	// serving provider, and every one of those paths answers the refusal that
	// says so rather than an empty page: a caller cannot tell "nothing
	// happened" from "nothing is kept".
	//
	// It is a resolver and not a value because a provider is a supervised
	// instance. One that is restarting, one that was declared after boot and
	// one that was never declared must all read the same, and a value captured
	// at wiring time cannot say any of that. The error is typed because no
	// serving instance and two instances claiming the asked verb are different
	// answers, and a boolean could not tell them apart.
	History HistoryResolver
	// Metrics is the process's metric registry as an exposition handler,
	// published at /metrics. Nil answers 404 there, which is what a build
	// carrying no metrics client says.
	Metrics http.Handler
	// MetricsListener says the deployment gave the exposition a socket of its
	// own, which core.observability.addr is. The API listener then publishes no
	// /metrics operation and answers 404 at that path, and MetricsHandler is
	// what the second listener serves.
	//
	// It is a boolean and not the address, because this package does not bind
	// the socket. What it decides is which of the two routers the exposition
	// goes on, and that is the same decision whatever port the operator chose.
	MetricsListener bool
	// FleetTelemetry decides who may read the exposition. Nil refuses every
	// caller rather than serving one, because the exposition names every device
	// this process has seen and a missing decider is not a permission.
	FleetTelemetry FleetTelemetryAccess
	// Served records one request this origin answered. Nil records nothing.
	//
	// It is a function and not a metric handle because internal/transport may
	// not import internal/platform. This package may not name a registry, so
	// the composition root passes in the one call it wants made.
	// route is the template a request matched and never the path it was written
	// as, because a path is one series per printer id.
	Served func(method, route string, status int, took time.Duration)
	// Connectors rolls up device health per connector, served beside the
	// liveness probe and never inside it. Nil answers an empty list, which is a
	// deployment with no connector plane.
	Connectors ConnectorHealth
	// Heartbeat is how often an idle event stream writes a comment frame, so an
	// intermediary that reaps a silent connection does not take a reader off a
	// stream that is merely quiet.
	Heartbeat time.Duration
	// Retry is the reconnection delay an event stream hands its reader, so a
	// browser that drops comes back on this deployment's number rather than on
	// whatever its own engine defaults to.
	Retry time.Duration
	// RequestBudget is how long one request may take before this server stops
	// waiting for its own handler and answers 504. It is the other half of the
	// context: a client that goes away already stops the work, and this is what
	// bounds a client that stays.
	RequestBudget time.Duration
	// StreamBudget is the same bound for the routes whose registration declares
	// the stream class: the event stream, the camera stream, the two printer
	// transfers and the storage content routes. Those keep a window per chunk,
	// so this is the ceiling over the whole response rather than what notices a
	// peer that stopped.
	StreamBudget time.Duration
	// MaxBodyBytes bounds a request body. Nothing here allocates in proportion
	// to a body; this stops a client trying to make it.
	MaxBodyBytes int64
	// MaxUploadBytes bounds a request body on the routes whose body is a file.
	// It is separate because the two numbers answer different questions: a
	// document over a megabyte is a client doing something wrong, and a print
	// file over a megabyte is normal.
	MaxUploadBytes int64
	// Mounts are the paths this origin serves that are not operations: a frozen
	// device wire, and the frontend at the root. Each one carries the reason it
	// is not published, and the route-and-document diff is told to expect
	// exactly these patterns and no others.
	Mounts []Mount
	// Modules answers which module claims one namespace, behind
	// /api/v1/modules/{namespace}/. Nil is a build with no module plane, and
	// every namespace answers as an unrouted path.
	//
	// A module's routes are not operations and are left out of the OpenAPI
	// document on purpose: an installed module must not change what this
	// server publishes, so uninstalling it is not a breaking change. The one
	// operation under the group is the core's own,
	// GET /api/v1/modules/{namespace}/openapi.json, which serves the document a
	// module ships for its routes.
	Modules ModulePlane
	// ModuleInstall installs a module, removes one and says what there is to
	// install, behind POST /api/v1/modules, DELETE /api/v1/modules/{namespace}
	// and GET /api/v1/modules/index. Nil serves the three paths and answers
	// core.resource/INSTALL_NOT_CONFIGURED, which is a build holding no
	// writable configuration of its own.
	//
	// It is separate from Modules because the two answer different questions.
	// That one routes a request into a module that is already running; this one
	// writes the declaration a module runs from, which is this server's
	// configuration and not a module's surface.
	ModuleInstall ModuleInstaller
	// ModuleAccess decides who may reach a module's surface. It is required
	// whenever Modules is set, because a module route is authorized on the
	// module's own instance. A route that authorized on nothing would be open
	// to anybody who can reach the port.
	ModuleAccess ModuleAccess
	// State is the module record store, for the operator listing at
	// /api/v1/state/namespaces. Nil is a deployment holding no record store,
	// and the listing is empty rather than absent: the document is a contract
	// and a build without a store publishes the same route.
	//
	// It answers accounting and never a stored value. The permission is decided
	// per namespace against Modules and ModuleAccess, so a deployment carrying
	// no module plane lists nothing.
	State StateResolver
	// ModuleState is the same record backend, read one key at a time for the
	// signed-in person, behind /api/v1/modules/{namespace}/state. Nil is a
	// deployment running no record backend, and every operation there answers
	// 501 core.resource/STATE_NOT_CONFIGURED.
	//
	// It is a second port and not a method on State because the two answer
	// different questions. State publishes the accounting an operator reads and
	// returns no value at all. This one returns a value, to the person who
	// wrote it.
	ModuleState ModuleStateResolver
	// MaxStateValueBytes bounds one record a module's browser half writes. It
	// is separate from MaxBodyBytes because the two answer different questions:
	// a settings pane keeps a few choices, and a bound on the whole body would
	// refuse the request rather than the setting.
	MaxStateValueBytes int64
	// DeviceStates is the state set each device kind's adapter declared, as the
	// host placed it at install. The zero value declares nothing, which is
	// every deployment whose adapters declare no state set, and the vocabulary
	// answers an empty list rather than a refusal.
	//
	// It is a value and not an interface because it is a table the host built
	// once and never a store: it is read from what was installed and never from
	// a running module.
	DeviceStates devicestate.Installed
	// DeviceModels is the device-model catalogue as the host placed it at
	// start: the core's rows and the rows the drivers in this build declared.
	// The zero value serves the core's rows alone.
	DeviceModels profile.Catalogue
}

// Server is the HTTP surface. The mux is unexported and never handed out: a
// caller holding one could register a route Huma would never see.
type Server struct {
	mux    *chi.Mux
	api    huma.API
	spec   *specCache
	logger *slog.Logger

	// spending is how long each route may take, read off the registrations
	// once they are all in. It is held here so that what a route was given is
	// a fact a test can read, rather than something only a slow handler shows.
	spending *budgets

	// devices carries only the mounts marked Device. It is built here and not
	// by a composition root, because chi.Walk cannot enumerate a router
	// assembled outside this package.
	devices *chi.Mux

	// unserved limits the line written when a printer dials a route no
	// connector serves here.
	unserved unservedPrinterRoutes

	// metrics carries the exposition and nothing else, for the listener
	// core.observability.addr asks for. It is built whether or not a
	// deployment binds that socket, because a router costs one allocation and
	// a nil one would panic in the first deployment that does.
	metrics *chi.Mux

	// closing is how a draining listener reaches a response that never ends.
	// http.Server.Shutdown waits for an in-flight request, and an event stream
	// is in flight for as long as its reader wants it to be.
	closing   chan struct{}
	closeOnce sync.Once
}

// New builds the router, registers every operation, mounts the document and the
// reference, and then refuses to return a server whose routes and OpenAPI
// document disagree. A server that will not start is the runtime form of a
// build failure, and it is the only form available for routes that an
// extension registers after compilation.
func New(cfg Config) (*Server, error) {
	installErrorModel()

	cfg = cfg.withDefaults()

	if cfg.Principals == nil {
		return nil, ErrNoPrincipals
	}

	if cfg.Resources == nil {
		return nil, ErrNoResourceStore
	}

	if cfg.Presets == nil {
		return nil, ErrNoGCodePresetStore
	}

	if cfg.Endpoints == nil {
		return nil, ErrNoEndpointStore
	}

	if cfg.Grants == nil {
		return nil, ErrNoGrants
	}

	if cfg.Sessions == nil {
		return nil, ErrNoSessions
	}

	if cfg.Accounts == nil {
		return nil, ErrNoAccounts
	}

	if cfg.Readiness == nil {
		return nil, ErrNoReadiness
	}

	if cfg.Events == nil {
		return nil, ErrNoEventStream
	}

	if cfg.Devices == nil {
		return nil, ErrNoDeviceFleet
	}

	if cfg.Stores == nil {
		return nil, ErrNoStorages
	}

	if cfg.Modules != nil && cfg.ModuleAccess == nil {
		return nil, ErrNoModuleAccess
	}

	if err := validateMounts(cfg.Mounts); err != nil {
		return nil, err
	}

	if slices.Contains(cfg.AllowedOrigins, "*") {
		return nil, ErrWildcardOrigin
	}

	mux := chi.NewMux()
	// First, and on this router only. A preflight is answered here before it
	// is counted, traced or routed, and the device router never sees the
	// allow-list; see corsMiddleware.
	mux.Use(corsMiddleware(cfg.AllowedOrigins))
	mux.Use(middlewareStack(cfg)...)
	// Last of the router's own, so the deadline is the nearest bound to the
	// handler and the refusal it leads to is recovered, counted and traced like
	// any other answer. The table behind it is empty until the operations are
	// registered, which is why it is a handle and not a value: chi takes its
	// middleware before it takes a route.
	spending := newBudgets()
	mux.Use(budgetMiddleware(mux, spending))
	mux.NotFound(notFound)
	mux.MethodNotAllowed(methodNotAllowed)

	api := humachi.New(mux, humaConfig(cfg))
	registerErrorSchemas(api.OpenAPI().Components.Schemas)
	// The first run comes before identity. On an install that has recorded no
	// decision there is no answer to who the caller is, because that is the
	// decision nobody has taken yet.
	api.UseMiddleware(requestMiddleware,
		setupMiddleware(api, cfg.Setup),
		principalMiddleware(api, cfg.Principals, cfg.Logger))

	server := &Server{
		mux:      mux,
		api:      api,
		logger:   cfg.Logger,
		spending: spending,
		closing:  make(chan struct{}),
	}

	registerOperations(api, cfg, server.closing)

	// After the operations, because the table is read off their registrations,
	// and before the server is returned, because nothing serves until then.
	spending.readFrom(api, cfg.RequestBudget, cfg.StreamBudget)

	// After every operation and before the document can be rendered. It refuses an
	// example that does not fit the operation it names; the gate on an
	// operation carrying none at all is a test.
	if err := attachExamples(api); err != nil {
		return nil, err
	}

	server.spec = newSpecCache(api)
	server.mountInfrastructure(cfg)
	server.mountModules(cfg)
	server.mountBeside(cfg)
	server.devices = deviceRouter(cfg)
	server.metrics = metricsRouter(cfg)

	// A path nothing above routed is a module's root mount or nothing. It is
	// set here and not before the operations so that it reads as what it is:
	// the last thing asked, after every route of this server's own.
	mux.NotFound(server.rootModules(cfg, server.unroutedPrinter(http.HandlerFunc(notFound))).ServeHTTP)

	if err := verifyDocumented(mux, api, PatternsOf(cfg.Mounts)); err != nil {
		return nil, err
	}

	// Every root this router serves is one a module's mount is refused. It is
	// held here, on the built router, so the reserved list cannot fall behind
	// a route somebody registered later.
	if err := verifyRootsReserved(mux, PatternsOf(cfg.Mounts)); err != nil {
		return nil, err
	}

	return server, nil
}

// Handler returns the surface to serve. It is an http.HandlerFunc rather than
// the mux, so a caller cannot assert their way back to the router.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.mux.ServeHTTP) }

// DeviceHandler is the subset of this origin a device-facing listener serves:
// the mounts marked Device, and nothing else. Every other path answers the same
// refusal an unmounted server answers with.
//
// It is a subset and never a superset, so a route it serves is a route the main
// handler serves too and the route-and-document diff already covers.
func (s *Server) DeviceHandler() http.Handler { return http.HandlerFunc(s.devices.ServeHTTP) }

// DeviceRoutes are the patterns DeviceHandler answers, for the line that says
// at startup where a device may dial.
func (s *Server) DeviceRoutes() []string {
	patterns := make([]string, 0, len(s.devices.Routes()))
	for _, route := range s.devices.Routes() {
		patterns = append(patterns, route.Pattern)
	}

	return patterns
}

// MetricsHandler is what the metrics listener serves: the Prometheus exposition
// at /metrics, to a caller holding no credential, and a refusal at every other
// path. metricsRouter argues why the credential is the socket and not a grant.
//
// It answers whether or not core.observability.addr was set. Serving it is the
// composition root's decision, taken by binding the socket or not binding it,
// and this package does not hold a second copy of that decision.
func (s *Server) MetricsHandler() http.Handler { return http.HandlerFunc(s.metrics.ServeHTTP) }

// MetricsServer returns a server for the metrics listener, bound to addr, with
// the same four timeouts the API listener carries.
//
// There is no TLS and no handshake budget of its own. The listener is reached
// from inside the deployment's own network, by a collector that holds no
// account here, so there is no credential on the wire for a certificate to
// protect.
func (s *Server) MetricsServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.MetricsHandler(),
		ErrorLog:          s.errorLog("metrics"),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// DeviceServer returns a server for the device-facing listener, bound to addr,
// with handshake as the budget for a printer's TLS handshake.
//
// Go takes the handshake budget from the smallest positive value among
// ReadHeaderTimeout, ReadTimeout and WriteTimeout (net/http.(*Server).
// tlsHandshakeTimeout), so all three carry it: leaving the API listener's ten
// seconds on any one of them would cap the handshake at ten seconds whatever
// the operator configured.
func (s *Server) DeviceServer(addr string, handshake time.Duration) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.DeviceHandler(),
		ErrorLog:          s.errorLog("printer"),
		ReadHeaderTimeout: handshake,
		ReadTimeout:       handshake,
		WriteTimeout:      handshake,
		IdleTimeout:       idleTimeout,
	}
}

// errorLog routes what net/http reports about a connection into the logger this
// process was configured with, named by the listener it happened on.
//
// The zero value is log.Printf onto raw stderr, which honours neither
// core.log.level nor core.log.format and names no listener. A failed TLS
// handshake is the line that matters: it is the only report a server gets that
// a client refused the certificate it was presented, and with two sockets
// bound, which one refused is the first thing an operator needs.
//
// Warn, because every one of these is a connection that failed and none of them
// is this process failing.
func (s *Server) errorLog(listener string) *log.Logger {
	return slog.NewLogLogger(s.logger.With("listener", listener).Handler(), slog.LevelWarn)
}

// deviceRouter is the device-facing router: the same middleware every request
// passes through, and only the mounts that carry a device wire.
func deviceRouter(cfg Config) *chi.Mux {
	mux := chi.NewMux()
	mux.Use(middlewareStack(cfg)...)
	mux.NotFound(notFound)
	mux.MethodNotAllowed(methodNotAllowed)

	for _, mount := range cfg.Mounts {
		if mount.Device {
			mux.Handle(mount.Pattern, guard(mount.Handler))
		}
	}

	return mux
}

// HTTPServer returns a server bound to addr with the four timeouts set, because
// every one of them is unbounded by default.
func (s *Server) HTTPServer(addr string) *http.Server {
	server := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ErrorLog:          s.errorLog("api"),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Shutdown waits for in-flight requests and closes idle connections. An
	// event stream is neither: it is one request that ends when its reader says
	// so, so without this the drain runs to its budget and reports a timeout.
	server.RegisterOnShutdown(s.beginShutdown)

	return server
}

// beginShutdown tells every open stream that the listener is draining. It is
// safe to call more than once, because a server may be shut down twice.
func (s *Server) beginShutdown() { s.closeOnce.Do(func() { close(s.closing) }) }

func (s *Server) mountInfrastructure(cfg Config) {
	s.mux.Get(specJSONPath, s.spec.serveJSON)
	s.mux.Get(specYAMLPath, s.spec.serveYAML)
	s.mux.Get(docsPath, s.referenceHandler(cfg))
	s.mux.Get(docsScriptPath, serveReferenceScript)
	s.mountDevPage()
}

// mountBeside registers the handlers that answer paths this server does not
// publish. They go on after the operations and the infrastructure, so a mount
// can never shadow one: chi matches a static segment before a wildcard, and
// guard refuses this server's own paths whatever the pattern was.
//
// The catch-all at the origin root is asked last of all. A module's root mount
// stands in front of it, so /graphql reaches the module and not a page saying
// the route is fine, and the frontend still answers every address nothing
// claimed.
func (s *Server) mountBeside(cfg Config) {
	for _, mount := range cfg.Mounts {
		handler := mount.Handler
		if mount.Pattern == rootCatchAll {
			handler = s.rootModules(cfg, s.unroutedPrinter(navigations(handler)))
		}

		s.mux.Handle(mount.Pattern, guard(handler))
	}
}

func (c Config) withDefaults() Config {
	if c.Title == "" {
		c.Title = defaultTitle
	}

	if c.Version == "" {
		c.Version = defaultVersion
	}

	if c.Logger == nil {
		c.Logger = slog.New(slog.DiscardHandler)
	}

	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = defaultMaxBodySize
	}

	if c.MaxUploadBytes <= 0 {
		c.MaxUploadBytes = defaultMaxUploadSize
	}

	if c.RequestBudget <= 0 {
		c.RequestBudget = defaultRequestBudget
	}

	if c.StreamBudget < c.RequestBudget {
		// Raised rather than refused. A server built with one of the two set is
		// a test or an embedded build, and a stream budget under the request
		// budget would be the longer bound shortening a response. The
		// configuration key that carries the same pair refuses it instead,
		// where an operator is reading the message.
		c.StreamBudget = max(defaultStreamBudget, c.RequestBudget)
	}

	if c.MaxStateValueBytes <= 0 {
		c.MaxStateValueBytes = defaultMaxStateValueBytes
	}

	if c.Heartbeat <= 0 {
		c.Heartbeat = defaultHeartbeat
	}

	if c.Retry <= 0 {
		c.Retry = defaultRetry
	}

	if c.Logins == nil {
		c.Logins = loginguard.New(loginguard.Options{Logger: c.Logger})
	}

	if c.Credentials == nil {
		c.Credentials = noCredentials{}
	}

	if c.Pairings == nil {
		c.Pairings = noPairings{}
	}

	if c.AppPairings == nil {
		c.AppPairings = noAppPairings{}
	}

	if c.Setup == nil {
		c.Setup = settledSetup{mode: SetupModeRequired, version: c.Version}
	}

	return c
}

// humaConfig takes over the three routes Huma would otherwise mount itself.
// Its documentation page loads Scalar from unpkg and sets a Content-Security-
// Policy that omits 'self', so no locally served copy can replace it, and its
// document handler caches the marshalled bytes in an unsynchronised closure.
func humaConfig(cfg Config) huma.Config {
	humaCfg := huma.DefaultConfig(cfg.Title, cfg.Version)
	// What a reader sees before they have read an operation: the landing prose,
	// the base URL a request goes to, and a description under every group in the
	// sidebar. See reference.go for what each one is for.
	humaCfg.Info.Description = apiDescription
	humaCfg.Servers = apiServers()
	humaCfg.Tags = apiTags()
	humaCfg.OpenAPIPath = ""
	humaCfg.DocsPath = ""
	humaCfg.SchemasPath = ""
	// The only default hook installs the schema-link transformer, which puts a
	// $schema member and a Link header on every response pointing at
	// SchemasPath. With that path disabled the pointers resolve to nothing, so
	// the hook goes with it rather than publishing a URL that answers 404.
	humaCfg.CreateHooks = nil
	// Not here. Huma ORs this process-wide switch with the per-operation one,
	// and register sets the per-operation one for the methods that need it; see
	// changesState for which and why.
	humaCfg.RejectUnknownQueryParameters = false
	// A member named twice in one object is refused rather than resolved
	// last-wins, because type decides which implementation is built.
	humaCfg.Formats = strictJSONFormats()

	humaCfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		bearerScheme: {
			Type:   "http",
			Scheme: "bearer",
			Description: "A credential this server issued, sent as a bearer token. It is what a " +
				"script, a service or a cross-origin client uses. An operation that publishes an " +
				"empty security requirement takes no credential at all.",
		},
	}
	// The root requirement. Without it OpenAPI 3.1 says every operation is
	// unauthenticated, and a generated client sends no credentials at all.
	humaCfg.Security = []map[string][]string{{bearerScheme: {}}}

	// The second requirement is an alternative rather than an addition.
	// OpenAPI 3.1 section 4.8.30 reads the root list as OR, so either
	// credential authorizes a request, which is what session.TokenFrom accepts:
	// a Bearer header first, then the cookie.
	if cookie := sessionCookieName(cfg); cookie != "" {
		humaCfg.Components.SecuritySchemes[cookieScheme] = &huma.SecurityScheme{
			Type: "apiKey",
			In:   "cookie",
			Name: cookie,
			Description: "The session a browser holds after the login ceremony. It is HttpOnly, so " +
				"no client code reads or sends it: the browser attaches it to a same-origin request " +
				"on its own. Use the bearer scheme from anywhere else.",
		}

		humaCfg.Security = append(humaCfg.Security, map[string][]string{cookieScheme: {}})
	}

	return humaCfg
}

// sessionCookieName is the cookie a session rides in, as the document names it.
// It is read from the login ceremony rather than written out here, so the name
// the document publishes and the name the server sets cannot drift apart.
//
// A Config with no login ceremony has no session cookie to publish. New refuses
// one, so the empty answer is reachable only from a caller building the huma
// configuration on its own.
func sessionCookieName(cfg Config) string {
	if cfg.Sessions == nil {
		return ""
	}

	return cfg.Sessions.Cookie()
}

// middlewareStack is what every request passes through, whatever it is for.
// Identity is not here: it is a Huma middleware, so it runs for operations and
// not for the document or the reference.
func middlewareStack(cfg Config) []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		headMiddleware,
		foldListHeaderMiddleware,
		traceMiddleware,
		loggerMiddleware(cfg.Logger),
		// Outside the recovery, so a panicking handler is still one request
		// this origin took time over and answered 500 to.
		servedMiddleware(cfg.Served),
		recoverMiddleware(),
		bodyLimitMiddleware(cfg.MaxBodyBytes, cfg.MaxUploadBytes),
	}
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, httpfault.RouteNotFound(r.Method, r.URL.Path))
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, httpfault.MethodNotAllowed(r.Method, r.URL.Path))
}

// DiagnosticsChoice is what the admin chose to include beyond the core's own
// files.
type DiagnosticsChoice struct {
	// Modules includes what module processes wrote to standard error.
	Modules bool
	// Extensions includes the files modules add through diagnostics.collect.
	Extensions bool
}

// Diagnostics builds the redacted bundle an operator attaches to a support
// ticket. frontend is what the browser sent, unparsed.
type Diagnostics interface {
	Bundle(ctx context.Context, principal string, choice DiagnosticsChoice, frontend []byte) ([]byte, error)
	// Extensions is the title of each running module that adds files to a
	// bundle.
	Extensions() []string
	// Served reports whether the bundle is offered. It can change while
	// the process runs, when the first run records open access.
	Served() bool
}
