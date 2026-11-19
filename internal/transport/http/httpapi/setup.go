package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// firstRunPath is the first run. It is one path because it is one question:
// what this deployment does about identity. A GET says whether it has been
// answered, a POST answers it.
const firstRunPath = "/api/v1/setup"

// SetupPath is where the first run is answered, for the startup line that tells
// an operator where to take the decision. It is read from the constant the
// operations are registered under, so the line and the route cannot drift apart.
func SetupPath() string { return firstRunPath }

// The two states the first run reports and the two decisions it accepts. They
// are on the wire, so a page reads a word and does not infer a state from a
// status code.
const (
	// SetupPending is an install that has recorded no decision.
	SetupPending = "pending"
	// SetupDone is an install that has recorded one.
	SetupDone = "done"
	// SetupRestarting is the answer to a decision the wizard confirmed: it is
	// written down and the server restarts to record it.
	SetupRestarting = "restarting"
	// SetupModeOpen serves every request as the configured principal and asks
	// for no credential.
	SetupModeOpen = "open"
	// SetupModeRequired asks every caller for a credential.
	SetupModeRequired = "required"
	// SetupAdmissionAsk makes a new printer wait: it registers, shows a code on
	// its screen, and an operator approves that code. It is the default,
	// because a network nobody has vouched for is the ordinary case.
	SetupAdmissionAsk = "ask"
	// SetupAdmissionOpen admits a printer the moment it dials, which is what a
	// trusted local wire means.
	SetupAdmissionOpen = "open"
)

// The steps the first run reports. A page reads the key, and a person reads
// the sentence beside it.
const (
	// SetupStepMode is the decision about identity.
	SetupStepMode = "mode"
	// SetupStepAdministrator is the first administrator.
	SetupStepAdministrator = "administrator"
	// SetupStepAdmission is how a new printer joins.
	SetupStepAdmission = "admission"
	// SetupStepModulePrefix begins the step of one declared module. The rest of
	// the key is the instance, in kind:name spelling, so a page finds the row
	// it drew the switch for.
	SetupStepModulePrefix = "module:"
)

// SetupStep is one thing the first run does, and whether it has been done.
//
// done is the step's finished state and not "somebody pressed something": the
// mode is recorded, an administrator exists, the admission choice is written,
// a module is switched on. A module the first run left off on purpose is
// done false with a detail that says so, and a page prints that detail beside
// its switch.
type SetupStep struct {
	// Step is the key. It is mode, administrator, admission, or module: and
	// the instance.
	Step string `json:"step" doc:"Which step this is: mode, administrator, admission, or module: and the instance in kind:name spelling." example:"mode"`
	// Done is whether the step has reached its finished state.
	Done bool `json:"done" doc:"Whether this step is finished."`
	// Detail is one short English sentence about what was done.
	Detail string `json:"detail" doc:"One short sentence saying what was recorded." example:"Every caller presents a credential."`
}

// Setup is the first run as this surface uses it. It is declared here because
// this is where it is consumed, and satisfied by the composition root, which is
// the only thing that may name the machine-owned overlay, the account store and
// the login at once.
type Setup interface {
	// Pending reports whether this install has recorded no decision yet. Every
	// request passes through it, so it reads memory and never the disk.
	Pending() bool
	// State is what the first-run page reads: whether a decision has been
	// recorded, and what the page needs to offer one.
	State(ctx context.Context) (SetupState, error)
	// Record takes the decision. It creates the first administrator when the
	// decision is required, and signs the caller in, so the page continues
	// without a second step.
	Record(ctx context.Context, decision SetupDecision) (SetupResult, error)
	// FirstAccount runs create while this server holds no account, once the
	// caller has sent the one-time setup code. POST /api/v1/auth/setup is the
	// other way to create the first administrator, and it asks for the same
	// code.
	FirstAccount(ctx context.Context, client SetupClient, code string, create func(context.Context) error) error
	// Configuration is the configuration contract the first-run wizard draws
	// its fields from, behind the same code.
	Configuration(ctx context.Context, client SetupClient, code string) (SetupConfiguration, error)
	// CheckStorage reports whether this process could keep uploads in a
	// folder, behind the same code, and creates nothing.
	CheckStorage(ctx context.Context, client SetupClient, code, root string) (SetupStorageCheck, error)
	// CheckOIDC tries the sign-in provider the wizard would configure, behind
	// the same code, and signs nobody in.
	CheckOIDC(ctx context.Context, client SetupClient, code string, probe SetupOIDCProbe) (SetupOIDCCheck, error)
	// CreateCertificates makes the certificates the wizard encrypts with,
	// behind the same code.
	CreateCertificates(ctx context.Context, client SetupClient, code string, names []string) (SetupCertificates, error)
}

// SetupState is the first run as a page reads it.
type SetupState struct {
	// State is pending or done.
	State string `json:"state" enum:"pending,done" doc:"Whether this install has recorded a decision about identity."`
	// Mode is the recorded decision, and empty while the state is pending.
	Mode string `json:"mode,omitempty" enum:"open,required" doc:"The recorded decision. Absent while the state is pending."`
	// Version is the build this process is running, so the first-run page names
	// what it is setting up.
	Version string `json:"version" doc:"The build this process is running."`
	// Issuer is true when an OIDC issuer is configured. The page then offers no
	// password, because a deployment serves one identity provider.
	Issuer bool `json:"issuer" doc:"Whether an OIDC issuer is configured. A deployment with one serves no password login."`
	// Admission is how a new printer joins, and empty while nothing has been
	// recorded.
	Admission string `json:"admission,omitempty" enum:"ask,open" doc:"How a new printer joins. Absent while nothing has been recorded."`
	// Modules are the declared modules and whether each is enabled, so the
	// first run is where an operator switches on what they installed.
	Modules []SetupModule `json:"modules" doc:"The declared modules and whether each is enabled."`
	// Steps are what the first run has done, read from what is on disk. A
	// person who closed the browser halfway resumes where they stopped, and a
	// settled server reports what its first run decided.
	Steps []SetupStep `json:"steps" doc:"What the first run has done, read from the recorded decision, the account store and the declared modules."`
	// Documentation is where this build's documentation is read, so the
	// first-run page links to it before anybody has signed in.
	Documentation string `json:"documentation,omitempty" doc:"The address of the documentation written for this build, ending in a slash. Absent in a build that names none."`
	// LastError is the start that failed after the wizard was last confirmed.
	// The server went back to the configuration it had, and the page shows
	// the error beside the answers that caused it.
	LastError *SetupFailure `json:"last_error,omitempty" doc:"The start that failed after the wizard was last confirmed. The server went back to its previous configuration. Absent when nothing failed."`
}

// SetupModule is one declared module as the first-run page lists it.
type SetupModule struct {
	Instance string `json:"instance" doc:"The instance, in kind:name spelling, which is what a decision names it by."                        example:"sink:planner"`
	Type     string `json:"type"     doc:"The module this instance is built from."`
	Enabled  bool   `json:"enabled"  doc:"Whether the supervisor starts it."`
	Writable bool   `json:"writable" doc:"Whether the first run may switch it. An instance the operator declared by hand is read-only here."`
}

// SetupClient is who sent a setup code.
type SetupClient struct {
	// Address is the client address the login limit reads: the peer, or the
	// forwarded address behind a proxy core.trusted_proxies lists.
	Address string
	// Forwarded says the request carried X-Forwarded-For or Forwarded and the
	// address is still loopback. A proxy on this machine sent it, and
	// core.trusted_proxies does not list that proxy, so every client behind
	// it counts as this machine.
	Forwarded bool
}

// setupClientOf reads who sent a request, for the setup code.
func setupClientOf(cfg Config, r *http.Request) SetupClient {
	client := SetupClient{Address: cfg.Logins.Address(r)}

	if r == nil {
		return client
	}

	if addr, err := netip.ParseAddr(client.Address); err == nil && addr.Unmap().IsLoopback() {
		client.Forwarded = r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Forwarded") != ""
	}

	return client
}

// SetupDecision is what a caller records.
type SetupDecision struct {
	// Code is the one-time setup code the caller sent, as they typed it.
	Code string
	// Client is who sent the request, so wrong codes are counted per client.
	Client SetupClient
	// Mode is open or required.
	Mode string
	// Account is the first administrator, required when the mode is required
	// and refused otherwise.
	Account *NewAccount
	// Admission is ask or open. It is never empty here: a body that named none
	// takes ask, so the composition root writes a choice and never guesses one
	// later.
	Admission string
	// Modules are the instances to switch on or off in the same call.
	Modules []SetupModuleChoice
	// Configuration is the wizard's answers, a dotted path to a value. When it
	// is set, the decision is written down and a restart applies it. This call
	// does not.
	Configuration map[string]any
	// StorageRoot is the folder a new storage keeps its files in, taken only
	// with Configuration. Empty declares no storage.
	StorageRoot string
	// MQTT is the broker the printers' state is sent to, taken only with
	// Configuration. Nil declares no sink.
	MQTT *SetupMQTT
	// Printers is the listener printers dial, apart from the API, taken only
	// with Configuration. Nil keeps printers on the API's own listener.
	Printers *SetupPrinters
	// History is how long the printers' history is kept, taken only with
	// Configuration. Nil keeps none.
	History *SetupHistory
}

// SetupHistory is the history store the first run declares.
type SetupHistory struct {
	// Retention is how long a sample is kept, as a Go duration; "0" keeps
	// every sample.
	Retention string
}

// SetupPrinters is the printers' own listener.
type SetupPrinters struct {
	Addr        string
	Certificate string
	Key         string
}

// SetupMQTT is the broker a sink the first run declares sends to.
type SetupMQTT struct {
	URL      string
	Username string
	Password string
}

// SetupModuleChoice is one module the decision switches.
type SetupModuleChoice struct {
	Instance string
	Enabled  bool
}

// SetupResult is what recording the decision produced. Session is set when an
// administrator was created and signed in, and empty otherwise.
type SetupResult struct {
	Mode      string
	Admission string
	Session   Session
	SignedIn  bool
	// Steps are what this call did, in the same shape a read reports.
	Steps []SetupStep
	// Restarting says the decision was written down and the server restarts
	// to apply it. Nothing is recorded until the new process starts.
	Restarting bool
}

// setupAccountBody is the first administrator as the first run takes them. The
// bounds are the account store's, restated so an oversized field is refused
// before anything is hashed.
type setupAccountBody struct {
	Username string `json:"username" doc:"The name this person logs in with, compared case-folded."         maxLength:"254" minLength:"1"`
	Password string `json:"password" doc:"At least 8 bytes and at most 72, which is what the hasher takes." maxLength:"72"  minLength:"8"`
}

// setupModuleBody is one module the decision switches.
type setupModuleBody struct {
	Instance string `json:"instance" doc:"The instance, in kind:name spelling, as the state listed it." maxLength:"127" minLength:"3"`
	Enabled  bool   `json:"enabled"  doc:"Whether the supervisor starts it."`
}

// setupStorageBody is the storage the wizard declares.
type setupStorageBody struct {
	Root string `json:"root" doc:"The folder the files a person uploads are kept in. A relative path is read from the folder the server was started in." maxLength:"4096" minLength:"1"`
}

// setupMQTTBody is the broker the wizard declares a sink for.
type setupMQTTBody struct {
	URL      string `json:"url"                doc:"The broker, such as tcp://broker.local:1883. ssl:// or mqtts:// connects over TLS." maxLength:"2048" minLength:"1"`
	Username string `json:"username,omitempty" doc:"The user the server signs in to the broker as."                                     maxLength:"256"`
	Password string `json:"password,omitempty" doc:"That user's password."                                                              maxLength:"1024"`
}

// setupPrintersBody is the printers' own listener.
type setupPrintersBody struct {
	Addr        string `json:"addr"                  doc:"The host and port printers dial, such as :8443. It may not be the API's own."                                  maxLength:"255"  minLength:"1"`
	Certificate string `json:"certificate,omitempty" doc:"An ECDSA certificate in PEM. With it and the key the listener speaks TLS 1.2, which is what a printer speaks." maxLength:"4096"`
	Key         string `json:"key,omitempty"         doc:"The certificate's key in PEM."                                                                                 maxLength:"4096"`
}

// setupHistoryBody is the history store the wizard declares.
type setupHistoryBody struct {
	Retention string `json:"retention" doc:"How long a sample is kept, such as 48h or 720h. 0 keeps every sample." maxLength:"32" minLength:"1"`
}

// setupBody is the decision.
type setupBody struct {
	Code      string            `json:"code,omitempty"      maxLength:"128" doc:"The one-time setup code this server printed in its log when it started, or the value of core.auth.setup_code. Hyphens, spaces and case do not matter."`
	Mode      string            `json:"mode"                                doc:"open serves every request as core.principal. required asks every caller for a credential."                                                                                                  enum:"open,required"`
	Account   *setupAccountBody `json:"account,omitempty"                   doc:"The first administrator. Required when mode is required and configuration names no core.auth.issuer, and refused when it is open."`
	Admission string            `json:"admission,omitempty"                 doc:"How a new printer joins. ask holds it at the door with a code an operator approves, and is what an absent key takes. open admits a printer the moment it dials, which is a wire you trust." enum:"ask,open"`
	Modules   []setupModuleBody `json:"modules,omitempty"                   doc:"Declared modules to switch on or off in the same call."`

	Configuration map[string]any     `json:"configuration,omitempty" doc:"The first-run wizard's answers, each a dotted path from get-setup-configuration and its value. With it, the server writes the answers to config.d/setup.yaml, answers 202 and restarts, and the decision is recorded by the new process."`
	Storage       *setupStorageBody  `json:"storage,omitempty"       doc:"A storage for the files a person uploads, declared by the restart. Taken only with configuration."`
	MQTT          *setupMQTTBody     `json:"mqtt,omitempty"          doc:"An MQTT broker the printers' state is sent to, as a sink declared by the restart. Taken only with configuration."`
	Printers      *setupPrintersBody `json:"printers,omitempty"      doc:"A listener of their own for printers, apart from the API: the one a printer dials over TLS. Taken only with configuration."`
	History       *setupHistoryBody  `json:"history,omitempty"       doc:"A history store for the printers' charts, events, command log and finished transfers, in a SQLite file under config.d. Taken only with configuration."`
}

type setupStateOutput struct {
	Body SetupState
}

type recordSetupInput struct {
	Body setupBody
}

// setupOutput sets the session credential when one was minted, so the page that
// created the administrator continues as them.
type setupOutput struct {
	Status    int
	SetCookie http.Cookie `header:"Set-Cookie" doc:"The session credential, HttpOnly and SameSite=Lax. Absent when the mode is open, and when the server restarts."`

	Body struct {
		State      string      `json:"state"      enum:"done,restarting" doc:"done when the decision is recorded before this answer is written. restarting when the body carried configuration: the server restarts and the new process records it."`
		Restarting bool        `json:"restarting" doc:"Whether the server restarts after this answer. Read get-health and then get-setup until the state is done."`
		Mode       string      `json:"mode"      enum:"open,required" doc:"The decision that was recorded."`
		Admission  string      `json:"admission" enum:"ask,open" doc:"The admission choice that was recorded."`
		SignedIn   bool        `json:"signed_in" doc:"Whether this answer carries a session for the administrator it created."`
		Steps      []SetupStep `json:"steps"     doc:"What this call did: the mode, the administrator, the admission choice and each module it switched."`
	}
}

// registerFirstRun publishes the first run. Both operations are registered
// whatever the process was wired with, so the document a client generates from
// does not depend on whether this install has been set up.
func registerFirstRun(api huma.API, cfg Config) {
	registerSetupState(api, cfg)
	registerRecordSetup(api, cfg)
	registerSetupConfiguration(api, cfg)
	registerSetupStorage(api, cfg)
	registerSetupOIDC(api, cfg)
	registerSetupCertificates(api, cfg)
}

func registerSetupState(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "get-setup",
		Method:      http.MethodGet,
		Path:        firstRunPath,
		Summary:     "Whether this server has been set up",
		Description: "Reports whether this install has recorded a decision about identity, and what the " +
			"first-run page needs to offer one: the build, whether an OIDC issuer is configured, and the " +
			"declared modules. It takes no credential, because until the decision is recorded there is " +
			"nobody to hold one.\n\n" +
			"`steps` is what the first run has done, read from what is on disk rather than from what this " +
			"process remembers: the recorded mode, whether this server holds an account, the recorded " +
			"admission choice, and each declared module. A person who closed the browser halfway resumes " +
			"where they stopped, and a server that was set up long ago still reports what its first run " +
			"decided.",
		Tags:     []string{"auth"},
		Security: noSecurity,
		Errors:   []int{http.StatusInternalServerError},
	}, func(ctx context.Context, _ *struct{}) (*setupStateOutput, error) {
		state, err := cfg.Setup.State(ctx)
		if err != nil {
			return nil, setupFault(err, "read whether this server has been set up")
		}

		if state.Modules == nil {
			state.Modules = []SetupModule{}
		}

		if state.Steps == nil {
			state.Steps = []SetupStep{}
		}

		return &setupStateOutput{Body: state}, nil
	})
}

func registerRecordSetup(api huma.API, cfg Config) {
	register(api, huma.Operation{
		OperationID: "record-setup",
		Method:      http.MethodPost,
		Path:        firstRunPath,
		Summary:     "Record what this server does about identity",
		Description: "Records whether this deployment asks every caller for a credential. required creates " +
			"the first administrator and answers with their session, so the page continues as them. With " +
			"core.auth.issuer in configuration it takes no account: people sign in at the issuer, and " +
			"core.principal names the first administrator by their subject there. open " +
			"serves every request as core.principal and takes no account. Either way the modules named in " +
			"the body are switched on or off in the same call. It is refused once a decision is recorded: " +
			"reopening the first run over the API would let whoever reaches the port choose again.\n\n" +
			"`admission` says how a new printer joins, and an absent key takes `ask`: the printer " +
			"registers, shows a code on its screen and an operator approves it. `open` admits a printer " +
			"the moment it dials, which is what a wire you trust means. It is written into the same " +
			"machine-owned file the mode is, so it survives a restart and a read reports it back.\n\n" +
			"`steps` is what this call did, one row per thing recorded, in the shape a read of this " +
			"address reports them.\n\n" +
			"`code` is the one-time setup code. The server prints it in its log at every start while " +
			"nothing is recorded, or takes it from core.auth.setup_code. A missing or wrong code answers " +
			"403, and an address that sent five wrong codes in fifteen minutes answers 429 with " +
			"Retry-After. No operation returns the code.",
		Tags:     []string{"auth"},
		Security: noSecurity,
		Errors: []int{
			http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusConflict,
			http.StatusTooManyRequests, http.StatusInternalServerError,
		},
	}, func(ctx context.Context, in *recordSetupInput) (*setupOutput, error) {
		decision, err := setupDecisionOf(in.Body)
		if err != nil {
			return nil, err
		}

		decision.Client = setupClientOf(cfg, requestFrom(ctx))

		result, err := cfg.Setup.Record(ctx, decision)
		if err != nil {
			return nil, withRetryAfter(setupFault(err, "record what this server does about identity"))
		}

		out := &setupOutput{Status: http.StatusOK}
		out.Body.State = SetupDone

		if result.Restarting {
			out.Status = http.StatusAccepted
			out.Body.State = SetupRestarting
			out.Body.Restarting = true
		}

		out.Body.Mode = result.Mode
		out.Body.Admission = result.Admission
		out.Body.SignedIn = result.SignedIn
		out.Body.Steps = result.Steps

		if out.Body.Steps == nil {
			out.Body.Steps = []SetupStep{}
		}

		if result.SignedIn {
			out.SetCookie = sessionCookie(cfg, result.Session)
		}

		return out, nil
	})
}

// namesIssuer reports whether the wizard's answers name an OpenID Connect
// issuer. People then sign in there, and core.principal names the first
// administrator by their subject, so a required mode needs no account.
func namesIssuer(configuration map[string]any) bool {
	issuer, _ := configuration["core.auth.issuer"].(string)

	return strings.TrimSpace(issuer) != ""
}

// setupDecisionOf reads the body as the composition root takes it, and refuses
// the two pairings that cannot mean anything: a required mode with nobody to
// sign in, neither an account nor an issuer, and an open one carrying a
// password nothing would ever check.
func setupDecisionOf(body setupBody) (SetupDecision, error) {
	decision := SetupDecision{Code: body.Code, Mode: body.Mode, Admission: body.Admission}

	// An absent key is the careful answer. A deployment nobody said anything
	// about must not be the one that lets any machine on the wire in.
	if decision.Admission == "" {
		decision.Admission = SetupAdmissionAsk
	}

	if decision.Admission != SetupAdmissionAsk && decision.Admission != SetupAdmissionOpen {
		return SetupDecision{}, httpfault.RequestInvalid("body.admission",
			SetupAdmissionAsk+" or "+SetupAdmissionOpen,
			"a printer either waits for an operator to approve its code or is let in when it dials")
	}

	switch {
	case body.Mode == SetupModeRequired && body.Account == nil && !namesIssuer(body.Configuration):
		return SetupDecision{}, httpfault.RequestInvalid("body.account",
			"a username and a password", "a deployment that requires a credential needs somebody to hold one")

	case body.Mode == SetupModeOpen && body.Account != nil:
		return SetupDecision{}, httpfault.RequestInvalid("body.account",
			"nothing", "an open deployment asks for no credential, so an account here would never be checked")
	}

	if body.Account != nil {
		decision.Account = &NewAccount{
			Username: body.Account.Username,
			Password: body.Account.Password,
		}
	}

	for _, module := range body.Modules {
		decision.Modules = append(decision.Modules, SetupModuleChoice(module))
	}

	if body.Storage != nil {
		if body.Configuration == nil {
			return SetupDecision{}, httpfault.RequestInvalid("body.storage",
				"nothing, or configuration beside it", "a storage is declared by the restart the wizard asks for")
		}

		decision.StorageRoot = body.Storage.Root
	}

	if body.MQTT != nil {
		if body.Configuration == nil {
			return SetupDecision{}, httpfault.RequestInvalid("body.mqtt",
				"nothing, or configuration beside it", "a sink is declared by the restart the wizard asks for")
		}

		decision.MQTT = &SetupMQTT{URL: body.MQTT.URL, Username: body.MQTT.Username, Password: body.MQTT.Password}
	}

	if body.Printers != nil {
		if body.Configuration == nil {
			return SetupDecision{}, httpfault.RequestInvalid(
				"body.printers",
				"nothing, or configuration beside it",
				"the printers' listener is declared by the restart the wizard asks for",
			)
		}

		decision.Printers = &SetupPrinters{
			Addr: body.Printers.Addr, Certificate: body.Printers.Certificate, Key: body.Printers.Key,
		}
	}

	if body.History != nil {
		if body.Configuration == nil {
			return SetupDecision{}, httpfault.RequestInvalid(
				"body.history",
				"nothing, or configuration beside it",
				"the history store is declared by the restart the wizard asks for",
			)
		}

		decision.History = &SetupHistory{Retention: body.History.Retention}
	}

	decision.Configuration = body.Configuration

	return decision, nil
}

// RetryAfterParam is the param of core.auth/SETUP_CODE_LIMITED and
// core.auth/LOGIN_LIMITED that says how many seconds the address waits. Whoever
// raises the fault fills it and this surface copies it into Retry-After, so
// both read this one name.
const RetryAfterParam = "retry_after_seconds"

// withRetryAfter copies the wait a limited first run carries into Retry-After,
// the header a client reads without parsing the body.
func withRetryAfter(err error) error {
	raised, ok := errors.AsType[*fault.Fault](err)
	if !ok || !fault.HasCode(raised, fault.AuthSetupCodeLimited.Code()) {
		return err
	}

	wait, ok := raised.Params().Get(RetryAfterParam)
	if !ok {
		return err
	}

	return huma.ErrorWithHeaders(err, http.Header{"Retry-After": []string{wait.String()}})
}

// setupFault keeps a refusal the implementation raised and turns anything else
// into this server's own failure. An overlay that could not be written is not
// the caller's mistake, and reporting it as one sends them rewriting a password
// that was fine.
func setupFault(err error, doing string) error {
	if raised, ok := errors.AsType[*fault.Fault](err); ok {
		return raised
	}

	return httpfault.Unexpected(doing)
}

// setupOpenOperations are the operations a pending install still answers. They
// are the probes an orchestrator sends whatever the process is doing, and the
// first run itself. Everything else answers SETUP_PENDING, because this server
// cannot say whether a caller needs a credential until the decision is there.
//
// The frontend at the origin root is not here: it is a mount and not an
// operation, so it is served whatever this reports, and a person can reach the
// page that records the decision. get-ui-settings is here because the page
// reads the title, the language and the theme before its first paint. A
// pending server that refused it would draw the setup screen with no name and
// in the wrong colours.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var setupOpenOperations = map[string]bool{
	"get-health":                true,
	"get-readiness":             true,
	"get-setup":                 true,
	"get-setup-configuration":   true,
	"check-setup-storage":       true,
	"check-setup-oidc":          true,
	"create-setup-certificates": true,
	"record-setup":              true,
	"get-ui-settings":           true,
}

// setupMiddleware refuses every other operation while the first run has taken
// no decision.
//
// It runs before identity is resolved, because on a pending install there is no
// answer to who the caller is: that is the decision that has not been taken.
func setupMiddleware(api huma.API, setup Setup) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		op := ctx.Operation()
		if op != nil && setupOpenOperations[op.OperationID] {
			next(ctx)

			return
		}

		if !setup.Pending() {
			next(ctx)

			return
		}

		refusal := fault.AuthSetupPending.New(
			"this server has not recorded what it does about identity, so it serves nothing else yet")

		// The response is the last thing this request does. A failure to write
		// it is already on stderr, and no caller is left to report it to.
		_ = huma.WriteErr(api, ctx, refusal.Status(), "", refusal)
	}
}

// settledSetup is the first run of a process wired without one: a server that
// is never pending and whose decision is the one its configuration already
// carries. It is what a test and an embedded build take, so no caller has to
// record a decision to get a server.
type settledSetup struct {
	mode    string
	version string
}

var _ Setup = settledSetup{}

func (settledSetup) Pending() bool { return false }

// State reports the one step such a process can answer for: the mode its
// configuration already carries. It holds no account store and no overlay, so
// there is nothing on disk to read the other steps from.
func (s settledSetup) State(context.Context) (SetupState, error) {
	return SetupState{
		State:   SetupDone,
		Mode:    s.mode,
		Version: s.version,
		Modules: []SetupModule{},
		Steps: []SetupStep{{
			Step: SetupStepMode, Done: s.mode != "",
			Detail: SetupModeDetail(s.mode),
		}},
	}, nil
}

// SetupModeDetail is the sentence a mode step carries. It is exported because
// the composition root writes the same sentence, and one source keeps a page
// from showing two different answers for one decision.
func SetupModeDetail(mode string) string {
	switch mode {
	case SetupModeRequired:
		return "Every caller presents a credential."
	case SetupModeOpen:
		return "Every request is served as the configured principal."
	}

	return "Nothing has been recorded about identity yet."
}

// SetupAdmissionDetail is the sentence an admission step carries, for the
// reason SetupModeDetail is exported.
func SetupAdmissionDetail(admission string) string {
	switch admission {
	case SetupAdmissionAsk:
		return "A new printer shows a code and waits for you to approve it."
	case SetupAdmissionOpen:
		return "A new printer is let in the moment it dials."
	}

	return "Nothing has been recorded about how a new printer joins."
}

// FirstAccount runs create with no code, because a process wired without a
// first run has no code to ask for. The account store still refuses a second
// administrator.
func (settledSetup) FirstAccount(
	ctx context.Context,
	_ SetupClient,
	_ string,
	create func(context.Context) error,
) error {
	return create(ctx)
}

func (settledSetup) Configuration(context.Context, SetupClient, string) (SetupConfiguration, error) {
	return SetupConfiguration{}, fault.AuthSetupClosed.New(
		"this server has already recorded what it does about identity")
}

func (settledSetup) CreateCertificates(context.Context, SetupClient, string, []string) (SetupCertificates, error) {
	return SetupCertificates{}, fault.AuthSetupClosed.New(
		"this server has already recorded what it does about identity")
}

func (settledSetup) CheckStorage(context.Context, SetupClient, string, string) (SetupStorageCheck, error) {
	return SetupStorageCheck{}, fault.AuthSetupClosed.New(
		"this server has already recorded what it does about identity")
}

func (settledSetup) CheckOIDC(context.Context, SetupClient, string, SetupOIDCProbe) (SetupOIDCCheck, error) {
	return SetupOIDCCheck{}, fault.AuthSetupClosed.New(
		"this server has already recorded what it does about identity")
}

func (settledSetup) Record(context.Context, SetupDecision) (SetupResult, error) {
	return SetupResult{}, fault.AuthSetupClosed.New(
		"this server has already recorded what it does about identity")
}

// setupExamples are the worked examples for the first run. The state sample is
// the pending one, because that is the answer the page is written against and
// the only one an operator ever acts on.
func setupExamples() map[string]operationExample {
	return map[string]operationExample{
		"get-setup": {Response: SetupState{
			State:   SetupPending,
			Version: "0.1.0",
			Modules: []SetupModule{
				{Instance: "sink:planner", Type: "github.com/acme/planner", Enabled: false, Writable: true},
			},
			Steps: []SetupStep{
				{Step: SetupStepMode, Done: false, Detail: SetupModeDetail("")},
				{Step: SetupStepAdministrator, Done: false, Detail: "No account has been created yet."},
				{Step: SetupStepAdmission, Done: false, Detail: SetupAdmissionDetail("")},
				{Step: SetupStepModulePrefix + "sink:planner", Done: false, Detail: "sink:planner is switched off."},
			},
		}},

		"get-setup-configuration": {Response: SetupConfiguration{
			ConfigFile: SetupFile{Path: "/srv/prusa-connect/config/config.yaml", Exists: false},
			SetupFile:  SetupFile{Path: "/srv/prusa-connect/config/config.d/setup.yaml", Exists: false},
			Keys: []SetupKey{{
				Path: "core.api.addr", Group: "api", Level: "basic", Kind: "text", Default: ":8080",
				Choices:   []string{},
				Doc:       "The host and port the HTTP API, the web app and the event stream are served on.",
				Reference: "config/network/#key-core-api-addr",
			}, {
				Path: "core.log.level", Group: "log", Level: "advanced", Kind: "text", Default: "info",
				Choices: []string{"debug", "info", "warn", "error"},
				Doc:     "How much the process writes to its log: debug, info, warn or error.",
				Locked:  true, Value: "debug",
				Reference: "config/logging/#key-core-log-level", Guide: "logging/",
			}},
			Storage:  SetupStorage{Root: "/srv/prusa-connect/storage", Declared: false},
			Hosts:    []string{"printserver", "printserver.local", "192.168.1.50"},
			Printers: true,
		}},

		"create-setup-certificates": {
			Request: map[string]any{"names": []string{"printserver", "printserver.local", "192.168.1.50"}},
			Response: SetupCertificates{
				Folder:             "/srv/prusa-connect/config/config.d/certs",
				Names:              []string{"printserver", "printserver.local", "192.168.1.50"},
				WebCertificate:     "/srv/prusa-connect/config/config.d/certs/web.crt",
				WebKey:             "/srv/prusa-connect/config/config.d/certs/web.key",
				PrinterCertificate: "/srv/prusa-connect/config/config.d/certs/printer.crt",
				PrinterKey:         "/srv/prusa-connect/config/config.d/certs/printer.key",
				Authority:          "/srv/prusa-connect/config/config.d/certs/ca.crt",
				PrinterAuthority:   []byte{0x30, 0x82},
				Expires:            "2029-01-18T10:00:00Z",
			},
		},

		"check-setup-storage": {
			Request:  map[string]any{"root": "/srv/prusa-connect/storage"},
			Response: SetupStorageCheck{Root: "/srv/prusa-connect/storage", Exists: false, Writable: true},
		},

		"check-setup-oidc": {
			Request: SetupOIDCProbe{
				Issuer:       "https://sso.example.com/realms/home",
				ClientID:     "prusa-connect",
				ClientSecret: "a client secret",
				RedirectURI:  "http://printserver.local:8080/api/v1/auth/callback",
			},
			Response: SetupOIDCCheck{
				Issuer:    "https://sso.example.com/realms/home",
				Discovery: SetupOIDCStep{Status: "passed"},
				Client:    SetupOIDCStep{Status: "passed", Detail: "invalid_grant: Code not valid"},
				Redirect:  SetupOIDCStep{Status: "failed", Detail: "status 400: Invalid parameter: redirect_uri"},
			},
		},

		"record-setup": {
			Request: setupBody{
				Code: "K7QD-3M9X-TP2A",
				Mode: SetupModeRequired,
				Account: &setupAccountBody{
					Username: "owner@example.com",
					Password: "a password of at least eight bytes",
				},
				Admission: SetupAdmissionAsk,
				Modules:   []setupModuleBody{{Instance: "sink:planner", Enabled: true}},
			},
			Response: map[string]any{
				"state": SetupDone, "mode": SetupModeRequired,
				"admission": SetupAdmissionAsk, "signed_in": true, "restarting": false,
				"steps": []SetupStep{
					{Step: SetupStepMode, Done: true, Detail: SetupModeDetail(SetupModeRequired)},
					{Step: SetupStepAdministrator, Done: true, Detail: "owner@example.com is the first administrator."},
					{Step: SetupStepAdmission, Done: true, Detail: SetupAdmissionDetail(SetupAdmissionAsk)},
					{Step: SetupStepModulePrefix + "sink:planner", Done: true, Detail: "sink:planner is switched on."},
				},
			},
		},
	}
}
