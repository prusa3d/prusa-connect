package httpapi

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The paths this package publishes. They are constants because the contract
// gate, the reference page and the allowlist all name the same strings, and a
// path written twice in source can come to differ.
const (
	healthPath    = "/api/v1/health"
	principalPath = "/api/v1/principal"
)

// bearerScheme is the key of the security scheme in the published document, and
// the requirement every operation carries unless it declares noSecurity. The
// server issues per-user credentials and a cross-origin client presents one as
// a bearer token. A same-origin browser session uses a cookie, and the client
// writes no code for it.
const bearerScheme = "bearer"

// cookieScheme is the key of the second root requirement. The two are
// alternatives: OpenAPI 3.1 reads the root list as OR, so a request that
// satisfies either one is authorized. TokenFrom in
// internal/platform/auth/session does the same. A Bearer header wins where
// both arrive.
const cookieScheme = "session"

// noSecurity is OpenAPI's empty security requirement: the document saying this
// operation needs no identity. principalMiddleware reads it, so the document
// says what is public and no second list is kept beside the router.
var noSecurity = []map[string][]string{}

// unlimitedBody switches off Huma's per-operation body limiter. The bound is
// held once, by bodyLimitMiddleware, which is on the router and therefore
// covers the document and the reference as well. Huma's copy defaults to 1 MiB
// whatever Config.MaxBodyBytes says, fires first for any larger configured
// bound, and reports the breach with an error carrying no limit for the client
// to read.
const unlimitedBody = -1

// register is the only way an operation is mounted. It exists so the body bound
// and the query strictness are not fields every registration has to remember;
// TestEveryOperationDefersTheBodyLimit and
// TestOnlyAStateChangingOperationRefusesAnUnpublishedQueryParameter read the
// registered operations and fail on one that forgot.
func register[I, O any](api huma.API, op huma.Operation, handler func(context.Context, *I) (*O, error)) {
	op.MaxBodyBytes = unlimitedBody
	op.RejectUnknownQueryParameters = changesState(op.Method)

	// Huma gives a raw body the same 5 s read deadline it gives a typed one. A
	// raw body here can be a camera frame of up to 8 MiB sent over a mobile
	// network, so it keeps the server's own ReadTimeout instead.
	if _, raw := reflect.TypeFor[I]().FieldByName("RawBody"); raw {
		op.BodyReadTimeout = readTimeout
	}

	// huma.Register appends 422 and 500 to op.Errors, so it must be handed a
	// slice nothing else holds. The tables below are shared between operations
	// and carry spare capacity, so without this copy two servers built at once
	// write the same array. The copy is here, in the one place every operation
	// passes, so a table added later is covered too.
	op.Errors = slices.Clone(op.Errors)

	// A route whose answer waits on a machine says so in the document as well
	// as in the budget table. The length of the budget is this deployment's
	// setting and stays out of the contract. That the answer waits on a
	// printer is part of the contract: a client that puts its own deadline on
	// such a call cuts off a transfer that is still running.
	if class, _ := op.Metadata[budgetKey].(budgetClass); class == budgetStream {
		if op.Extensions == nil {
			op.Extensions = map[string]any{}
		}
		op.Extensions[longRunningExtension] = true
	}

	// huma.Register reads huma.NewError to build the error schema, and the
	// installer writes it once. Going through the once here gives the read a
	// happens-before edge, so a probe that registers without a Server does
	// not race a parallel test that builds one.
	installErrorModel()

	huma.Register(api, op, handler)
}

// changesState reports whether a method may alter the configuration, which is
// what decides whether an operation refuses a query parameter it does not
// publish.
//
// A misspelt parameter must never change what a write
// does: ?dryRun=true reads to its sender as a successful dry run and commits the
// write, on the endpoint that rewrites the operator's configuration. On a safe
// method the same typo costs the caller a wrong answer they can see, and the
// strictness costs something an operator cannot see: GET /api/v1/health?_=17245
// is the cache-buster a probe, a proxy or an uptime monitor appends to a public
// liveness URL, and answering it 422 takes the instance out of service over a
// parameter nothing reads. Huma's own switch is process-wide, so the narrowing
// has to be here.
func changesState(method string) bool {
	return method != http.MethodGet && method != http.MethodHead
}

// healthOutput reports that the process is up and what it is. It says nothing
// about a printer, a database or a principal: a probe that fails when a
// dependency is degraded takes the whole instance out of service for something
// a client could have been told about instead.
type healthOutput struct {
	Body struct {
		Status  string `json:"status"  enum:"ok"   doc:"Always ok. A degraded dependency is reported by the resource it belongs to."`
		Version string `json:"version"             doc:"The build this process is running."`
		// InstanceID is never omitted: a client keys a server by it, and a
		// field that comes and goes is one a client has to guess about.
		InstanceID string `json:"instance_id" doc:"The id of this server: random, written once on the first start, and the same after a restart or an address change. The mDNS record of every service this server announces carries it as the id key."`
	}
}

// principalOutput names the caller as the authorization graph knows them.
type principalOutput struct {
	Body struct {
		Principal   string `json:"principal" doc:"The subject the authorization graph is keyed by for this caller."`
		DisplayName string `json:"display_name,omitempty" doc:"The signed-in person's display name: a local account's, or the name the identity provider last sent."`
		Username    string `json:"username,omitempty" doc:"The signed-in person's login name. Present only when the subject is a local user account that still exists."`
		Email       string `json:"email,omitempty" doc:"The address the identity provider last sent for the signed-in person. Absent for a local account."`
	}
}

// registerOperations mounts every operation this binary serves. It is the only
// place a route is added, and verifyDocumented refuses a server whose routes
// and document disagree afterwards.
func registerOperations(api huma.API, cfg Config, closing <-chan struct{}) {
	// First, because it answers with the fleet the printer operations are then
	// registered over: a command sent through this server is recorded on its way
	// to the printer.
	cfg.Devices = registerDevtools(api, cfg)

	// Then the raw gcode surface, because it returns the fleet that holds every
	// path reaching a raw gcode verb to the extra action it takes. Registering
	// the printer operations over anything else leaves the generic command
	// endpoint able to name the verb.
	cfg.Devices = registerGCode(api, cfg)

	registerHealth(api, cfg.Version, cfg.InstanceID)
	registerAppSettings(
		api, cfg.App, dashboardsCarried && !cfg.DashboardsOff, !cfg.ModulesOff, upgradesServed(cfg),
		func() bool { return diagnosticsCarried && cfg.Diagnostics != nil && cfg.Diagnostics.Served() },
	)
	registerConnectorHealth(api, cfg.Connectors)
	registerReadiness(api, cfg.Readiness, cfg.Principals, cfg.Grants)
	registerDiagnostics(api, cfg)
	registerMaintenance(api, cfg)
	registerBetaPortalPairing(api, cfg)
	registerPrincipal(api, cfg)
	registerFirstRun(api, cfg)
	registerAuth(api, cfg)
	registerAccounts(api, cfg)
	registerCredentials(api, cfg)
	registerAppPairings(api, cfg)
	registerPairings(api, cfg)
	registerResources(api, cfg.Resources)
	registerGCodePresets(api, cfg.Presets)

	if dashboardsCarried && !cfg.DashboardsOff {
		registerDashboards(api, cfg.Dashboards)
	}

	registerConnectorEndpoints(api, cfg.Endpoints)
	registerLinks(api, cfg.Links)
	registerDiscovery(api, cfg)
	registerTimeServer(api, cfg)
	registerAllowedOrigins(api, cfg)
	registerGrants(api, cfg.Grants)
	registerVerbGrants(api, cfg.Verbs)

	registerAccess(api, cfg.Access)
	registerDevices(api, cfg.Devices)
	registerCameras(api, cfg, closing)
	registerCameraSettings(api, cfg.CameraSettings)
	registerRegistrations(api, cfg)
	registerPrinterAuthority(api, cfg)

	// Always, and never conditionally on what an operator declared: the document
	// is a published contract and a deployment with no storage declared serves
	// the same routes, answering with an empty listing and a 404 for any name.
	registerStorages(api, cfg.Stores)
	registerStateNamespaces(api, cfg)
	registerModuleState(api, cfg)
	registerModuleOpenAPI(api, cfg)
	registerModuleInstall(api, cfg)
	registerModuleMemory(api, cfg)
	describeModuleUI(api)

	registerEvents(api, cfg, closing)

	// After the device operations, so the device lookup the per-device history
	// paths resolve through is the same fleet those operations were registered
	// over, raw gcode wrapper and command log included.
	registerHistory(api, cfg)
	registerCommands(api, cfg)

	registerVocabularies(api, cfg.DeviceStates, cfg.DeviceModels)

	// Unless the exposition has a listener of its own, where it is served
	// instead of here and this document names no operation for it.
	if !cfg.MetricsListener {
		registerMetrics(api, cfg.Metrics, cfg.FleetTelemetry)
	}
}

func registerHealth(api huma.API, version, instance string) {
	register(api, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        healthPath,
		Summary:     "Liveness of this process",
		Description: "Reports that the process is serving, with the build version and the id of this server. " +
			"The id is random, is written once on the first start, and stays the same across restarts and " +
			"address changes, so a client can tell the server again at a new address. It takes no identity " +
			"and reads no dependency.",
		Tags:     []string{"system"},
		Security: noSecurity,
		Errors:   []int{http.StatusInternalServerError},
	}, func(_ context.Context, _ *struct{}) (*healthOutput, error) {
		out := &healthOutput{}
		out.Body.Status = "ok"
		out.Body.Version = version
		out.Body.InstanceID = instance

		return out, nil
	})
}

// withProfile adds what the identity provider said about a person this server
// keeps no account for. A profile that cannot be read leaves the answer with
// the subject alone: the caller is still who they are.
func withProfile(ctx context.Context, profiles Profiles, id string, out *principalOutput) (*principalOutput, error) {
	if profiles == nil {
		return out, nil
	}

	profile, found, err := profiles.Profile(ctx, id)
	if err != nil || !found {
		return out, nil //nolint:nilerr // the subject is the answer, with or without a name
	}

	out.Body.DisplayName = profile.DisplayName
	out.Body.Email = profile.Email

	return out, nil
}

func registerPrincipal(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "get-principal",
		Method:      http.MethodGet,
		Path:        principalPath,
		Summary:     "The caller this server resolved",
		Description: "Returns the subject the authorization graph is keyed by, so a client can see " +
			"which identity its credentials produced before it acts on a printer. When that identity is a " +
			"local user account it also carries the person's display name and username, and for a person " +
			"who signed in at the identity provider the name and the address it last sent, so a page can " +
			"show who is signed in without the permission to list accounts.",
		Tags:   []string{"system"},
		Errors: []int{http.StatusUnauthorized},
	}, func(ctx context.Context, _ *struct{}) (*principalOutput, error) {
		principal := principalFrom(ctx)
		if principal == "" {
			return nil, fault.AuthTokenInvalid.New("the request carries no usable identity")
		}

		out := &principalOutput{}
		out.Body.Principal = principal

		id, isUser := strings.CutPrefix(principal, authz.TypeUser.String()+":")
		if !isUser {
			return out, nil
		}

		account, err := cfg.Accounts.Get(ctx, id)
		if fl, ok := errors.AsType[*fault.Fault](err); ok && fl.Code() == httpfault.CodeResourceNotFound() {
			return withProfile(ctx, cfg.Profiles, id, out)
		}

		if err != nil {
			return nil, accountFault(err, "read the signed-in account")
		}

		out.Body.DisplayName = account.DisplayName
		out.Body.Username = account.Username

		return out, nil
	})
}
