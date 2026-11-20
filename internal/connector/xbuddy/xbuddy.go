// Package xbuddy is the printer dialect Prusa's xBuddy and Buddy boards speak,
// ported to sdk/connectorapi.
//
// It is the only dialect with machines in the field, so its wire is frozen
// rather than chosen: the registration handshake, the nine-byte framing and the
// verb rewrite are what firmware that cannot be updated on our schedule already
// speaks, and they are recorded in earlier research
// sections 1 to 4.
//
// It is inbound. Nothing in configuration names a printer: the
// printer registers, waits to be approved, and then dials, so the host learns
// about one when it arrives and admits it through connectorapi.Arrivals.
//
// # The wire
//
//	POST /p/register   {sn, fingerprint, printer_type, firmware} in, a Code header out
//	GET  /p/register   Code header in; 202 while nobody has approved it, 200 and a Token when they have
//	GET  /p/auth       Fingerprint and Token headers in, the host's device id out
//	GET  /p/ws         Fingerprint, Token and Sec-WebSocket-Protocol: prusa-connect
//
// Those four paths are absolute in shipping firmware, so a host serving field
// printers mounts this driver at the root and strips nothing. The handler
// itself routes from its own root and encodes no prefix, so a host is free to
// mount it elsewhere for anything that can be told where to dial.
//
// # What the profile data does not carry
//
// 33 of the 57 verbs never reach the socket under the name the capability
// vocabulary gives them: twenty-one become G frames of gcode and twelve become a
// single SET_VALUE. That mapping is in verbs.go, and it is the one thing a
// driver generated from internal/printer/profile alone would get wrong. One
// command can also travel in another form than its row: a chamber target of
// 0 goes as M141 S0 when the printer reports IDLE, READY, STOPPED or FINISHED.
// See chamberOffForm.
package xbuddy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/prusa3d/prusa-connect/internal/kernel/admission"
	"github.com/prusa3d/prusa-connect/internal/printer/profile"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// Type is the value of a connector's `type` in configuration that selects this
// driver.
const Type connectorapi.TypeName = "xbuddy"

// The frozen endpoints and subprotocol.
const (
	registerPath  = "/p/register"
	authPath      = "/p/auth"
	websocketPath = "/p/ws"
	subprotocol   = "prusa-connect"

	codeHeader        = "Code"
	tokenHeader       = "Token"
	fingerprintHeader = "Fingerprint"
	modelHeader       = "User-Agent-Printer"
	firmwareHeader    = "User-Agent-Version"
)

// fingerprintHeaderLength is how much of its fingerprint a printer puts in the
// Fingerprint header, and it is the firmware's number rather than this
// server's.
//
// Buddy computes a 50 character fingerprint once at boot, out of the OTP unit
// id, the MAC address and the serial (printer_common.cpp, printerHash with
// FINGERPRINT_SIZE 50). It sends all 50 of them in the body of
// POST /p/register and in the INFO event, and it sends the first 16 in every
// Fingerprint header, /p/ws included: the header is emitted with a hard
// precision of FINGERPRINT_HDR_SIZE (connect.cpp UpgradeRequest, httpc.cpp
// "%s: %.*s"). So one machine has two spellings on this wire and neither route
// can be made to send the other one.
//
// Every table this registrar keys by a printer stays keyed on the full spelling
// a registration body carried. A header spelling reaches an entry through
// resolveLocked, which takes the prefix index only for a value of exactly this
// length and then checks that the entry's own fingerprint starts with it, and
// answers with the entry so that every credential check compares full spellings.
//
// Keying the tables on the prefix instead would make sixteen characters the
// credential. A printer puts those sixteen in the clear on every request it
// makes, so anything that saw one of them could register as that machine and be
// handed its code and its token.
//
// The retired server met the same asymmetry and answered it by checking the
// token row before the static allowlist (legacy/connector/xbuddy/auth.go),
// which worked only for the token mode and left a duplicate row behind whenever
// the ordering was wrong. Resolving the spelling answers every mode at once.
const fingerprintHeaderLength = 16

// headerSpelling reports whether a presented fingerprint is a header spelling
// rather than a full one: exactly fingerprintHeaderLength characters, which is
// what the firmware truncates to and what no registration body ever carries.
//
// The length test is the whole of the safety here. A prefix match on any
// presented length would mean sixteen characters of a printer's fingerprint,
// which every one of its requests puts on the wire in clear, were enough to be
// treated as that printer: the credential tables would answer to a prefix, and
// anything that saw one request could claim the machine. So the tables stay
// keyed on the full spelling a registration carried, and a prefix reaches an
// entry only through resolveLocked, only at this exact length, and only after
// the entry's own fingerprint is checked to start with it.
func headerSpelling(fingerprint string) bool {
	return len(fingerprint) == fingerprintHeaderLength
}

// Vendor is what every printer this dialect serves is made by. The handshake
// does not carry it, because a printer that speaks this wire is a Prusa.
const Vendor = "Prusa"

const (
	defaultQueueDepth    = 32
	defaultStorage       = "/usb"
	maxRegistrationBytes = 1 << 14

	// retryAfterFull is how long a printer refused a code is told to wait. It is
	// the shipping ping interval, so a printer that retries on it is retrying at
	// the rate this dialect already talks at.
	retryAfterFull = "54"
)

// ErrInvalidConfig reports a config block this driver cannot act on.
var ErrInvalidConfig = errors.New("xbuddy: invalid configuration")

//nolint:gochecknoinits // registration has to happen before main; the host reaches it through a blank import.
func init() { connectorapi.Register(Type, doc(), New, dialect()) }

// Config is what an operator writes under a connector's `config` block.
type Config struct {
	// Mode is the credential a printer presents on the websocket: none, token,
	// fingerprint or both. Empty means none, which is what the shipping default
	// is.
	Mode string `json:"mode,omitempty"`
	// AutoRegister approved a registration the moment it was made, with no
	// operator. It is no longer a setting: mode none approves on arrival and
	// every other mode waits for an operator, so the only values this takes are
	// the one its mode already does. It is still read, so that a file carrying
	// the other one is refused with a sentence rather than obeyed. See
	// checkAutoRegister.
	AutoRegister *bool `json:"auto_register,omitempty"`
	// MaxRegistrations is how many printers this driver keeps a registration
	// for. Zero or absent takes 400, the fleet this product claims. Over the
	// bound the oldest registration no printer completed is dropped.
	MaxRegistrations int `json:"max_registrations,omitempty"`
	// QueueDepth is how many commands may wait for one printer, whether it is
	// connected or away.
	QueueDepth int `json:"queue_depth,omitempty"`
	// ChunkWriteCeiling is how many bytes of the file one transfer chunk may
	// carry, whatever chunk size the printer asks for. Zero or absent takes
	// defaultChunkWriteCeiling, which is the largest size a board in the field
	// was observed to survive.
	ChunkWriteCeiling int `json:"chunk_write_ceiling,omitempty"`
	// OfflineGrace closes a session whose printer has been away that long, as
	// time.ParseDuration reads it. Empty or zero means a session outlives any
	// absence and only the host ends it, which is what keeps a queue for a
	// printer that is switched off overnight.
	OfflineGrace string `json:"offline_grace,omitempty"`
	// DefaultStorage is the firmware-side mountpoint a path with no storage
	// segment is resolved against. Empty means /usb.
	DefaultStorage string `json:"default_storage,omitempty"`
	// ReadDeadline is how long a printer's connection may say nothing before it
	// is reported unreachable, as time.ParseDuration reads it. Empty takes the
	// shipping 60s. It and the two keys after it are spelled as the shipping
	// connector spells them under connectors.prusa-xbuddy.timeouts.
	ReadDeadline string `json:"read_deadline,omitempty"`
	// PingInterval is how often the host asks a printer to prove it is there.
	// Empty takes the shipping 54s.
	PingInterval string `json:"ping_interval,omitempty"`
	// PingTimeout bounds writing one ping. Empty takes the shipping 10s.
	PingTimeout string `json:"ping_timeout,omitempty"`
	// CommandDeadline is how long one command waits for the printer to answer
	// it, as time.ParseDuration reads it. Empty takes defaultCommandDeadline.
	// It is read the same way the three keys above are, because an operator who
	// wrote 0s asked for a connector that answers every command before it is
	// sent.
	CommandDeadline string `json:"command_deadline,omitempty"`
	// Listener is the device-facing socket this dialect asks the host for: its
	// address, its certificate and its handshake budget. An absent block, which
	// is the shipping default, asks for no socket, and printers then dial the
	// host's own listener.
	Listener Listener `json:"listener,omitzero"`
}

// Driver is the connectorapi.Inbound this package registers.
type Driver struct {
	registrar       *registrar
	queueDepth      int
	grace           time.Duration
	storage         string
	chunkCeiling    int
	live            Liveness
	commandDeadline time.Duration
	listener        connectorapi.DeviceListener
	log             *slog.Logger
	upgrader        websocket.Upgrader
	refusals        refusalLog

	mu       sync.Mutex
	sessions map[string]*Session
}

// New builds a Driver from its spec. It is the connectorapi.Factory for Type.
func New(spec connectorapi.Spec) (connectorapi.Driver, error) {
	cfg := Config{}
	if err := spec.DecodeConfig(&cfg); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	mode := ModeNone
	if cfg.Mode != "" {
		mode = Mode(cfg.Mode)
	}

	switch mode {
	case ModeNone, ModeToken, ModeFingerprint, ModeBoth:
	default:
		return nil, fmt.Errorf("%w: %w: %q", ErrInvalidConfig, ErrUnknownMode, cfg.Mode)
	}

	if err := checkAutoRegister(mode, cfg.AutoRegister); err != nil {
		return nil, err
	}

	depth := defaultQueueDepth
	if cfg.QueueDepth != 0 {
		depth = cfg.QueueDepth
	}

	if depth < 1 {
		return nil, fmt.Errorf("%w: queue_depth must be at least 1, and it is %d", ErrInvalidConfig, depth)
	}

	if cfg.MaxRegistrations < 0 {
		return nil, fmt.Errorf("%w: max_registrations must not be negative, and it is %d",
			ErrInvalidConfig, cfg.MaxRegistrations)
	}

	grace, err := parseGrace(cfg.OfflineGrace)
	if err != nil {
		return nil, err
	}

	live, err := parseTimeouts(cfg)
	if err != nil {
		return nil, err
	}

	ceiling, err := parseChunkCeiling(cfg.ChunkWriteCeiling)
	if err != nil {
		return nil, err
	}

	deadline, err := parsePositive("command_deadline", cfg.CommandDeadline, defaultCommandDeadline)
	if err != nil {
		return nil, err
	}

	// The socket this dialect asks the host for, read and checked here rather
	// than by the host: the certificate has to satisfy this firmware, and the
	// dialect is the only thing that knows what this firmware accepts.
	listener, err := parseListener(cfg.Listener, spec.Logger)
	if err != nil {
		return nil, err
	}

	return NewDriver(mode, depth, grace, cfg.DefaultStorage, spec.Logger,
		WithLiveness(live), WithMaxRegistrations(cfg.MaxRegistrations),
		WithChunkWriteCeiling(ceiling), WithCommandDeadline(deadline),
		WithDeviceListener(listener)), nil
}

// checkAutoRegister refuses an auto_register setting that contradicts the
// admission mode, and there is no other setting: whether a registration is
// approved the moment it is made is the mode and nothing else
// (registrar.approvesOnArrival).
//
// Mode none admits every printer that dials, so a registration waiting for an
// operator there stalls a handshake whose answer is already yes: it approves on
// arrival, always. In mode token, fingerprint and both, the registration is the
// decision, and approving one without an operator hands out the credential that
// mode checks to anything that can reach POST /p/register. That is how a Core
// One was admitted twice on the bench on 2026-09-03 with mode token written in
// the file (BUG-011). In mode token a printer is never admitted without an
// operator.
//
// The key is kept, and refused rather than ignored, because it is in
// deployments in the field and in the retired server's configuration. An
// operator who wrote the setting that no longer exists is told which mode does
// what they asked for, instead of finding out from a printer.
func checkAutoRegister(mode Mode, asked *bool) error {
	if asked == nil {
		return nil
	}

	if *asked && mode != ModeNone {
		return fmt.Errorf("%w: auto_register: true is refused in mode %s, where every "+
			"registration is what an operator decides on. Only mode none approves on arrival",
			ErrInvalidConfig, mode)
	}

	if !*asked && mode == ModeNone {
		return fmt.Errorf("%w: auto_register: false is refused in mode none, which admits "+
			"every printer that dials, so its registrations cannot wait for an operator. Use "+
			"mode token to have them decided by an operator", ErrInvalidConfig)
	}

	return nil
}

// parseChunkCeiling reads the chunk ceiling. Zero or absent takes the measured
// default, and a value the wire cannot carry is refused here rather than
// clamped: an operator who wrote a megabyte asked for something this connector
// will not do to a board.
func parseChunkCeiling(value int) (int, error) {
	if value == 0 {
		return defaultChunkWriteCeiling, nil
	}

	if value < 1 || value > maxChunkBytes {
		return 0, fmt.Errorf("%w: chunk_write_ceiling is %d, expected between 1 and %d",
			ErrInvalidConfig, value, maxChunkBytes)
	}

	return value, nil
}

// Option adjusts a driver beyond what NewDriver takes positionally.
type Option func(*Driver)

// WithLiveness sets how fast a printer that went away silently is noticed. A
// zero field takes the shipping value.
func WithLiveness(live Liveness) Option {
	return func(d *Driver) { d.live = live.orShipping() }
}

// WithChunkWriteCeiling bounds one transfer chunk's payload. Zero takes
// defaultChunkWriteCeiling, and so does a negative number, because a session
// that wrote no bytes per chunk would serve no file at all.
func WithChunkWriteCeiling(ceiling int) Option {
	return func(d *Driver) { d.chunkCeiling = ceiling }
}

// WithCommandDeadline bounds how long one command waits for the printer to
// answer it. Zero takes defaultCommandDeadline, and so does a negative
// duration, because a session that gave a command no time at all would fail
// every command it sent.
func WithCommandDeadline(deadline time.Duration) Option {
	return func(d *Driver) { d.commandDeadline = deadline }
}

// WithMaxRegistrations bounds the registration table. Zero takes
// defaultMaxRegistrations.
func WithMaxRegistrations(limit int) Option {
	return func(d *Driver) { d.registrar = newRegistrar(d.registrar.mode, limit) }
}

// NewDriver returns a driver with its settings given directly, for a host that
// wires its own built-ins and for tests.
//
// log may be nil, which is a driver that logs nothing. The contract hands a
// factory nothing but its spec, so a driver built through configuration has no
// logger until the host passes one here.
func NewDriver(
	mode Mode, queueDepth int, grace time.Duration, storage string, log *slog.Logger,
	options ...Option,
) *Driver {
	if storage == "" {
		storage = defaultStorage
	}

	driver := &Driver{
		registrar:       newRegistrar(mode, defaultMaxRegistrations),
		queueDepth:      queueDepth,
		grace:           grace,
		storage:         storage,
		live:            Liveness{}.orShipping(),
		commandDeadline: defaultCommandDeadline,
		log:             log,
		upgrader:        websocket.Upgrader{Subprotocols: []string{subprotocol}},
		sessions:        make(map[string]*Session),
	}

	for _, option := range options {
		option(driver)
	}

	return driver
}

// sendable is what this driver may put on the wire for one printer inside an
// exchange of its own, which is not the same set as what a caller may ask for.
//
// The two transfer starts are kept out of the published baseline on purpose: a
// caller-started download leaves the device asking for ranges no Send is
// holding (see sentNotInvokable). The host resolves a printer's capabilities
// against that published baseline, so the set it hands back can never carry
// them, and a session that read only that set could never send a file at all.
// This is where they come back, gated on the firmware's own profile rather than
// on what a caller may invoke.
//
// A printer no profile covers gets them. The profile tier exists to narrow what
// a caller may ask a machine for, and this exchange is not something a caller
// asks for: whether the file can be sent is still decided by
// core.feature.file_upload in the resolved set, which the profile does govern.
func (d *Driver) sendable(identity connectorapi.Identity) connectorapi.Set {
	sends := make(connectorapi.Set, len(sentNotInvokable))

	matched, covered := transferProfiles().Match(identity.Model, identity.Firmware)

	for key := range sentNotInvokable {
		if covered && !matched.Capabilities.Has(key) {
			continue
		}

		sends[key] = connectorapi.Value{Flag: true}
	}

	return sends
}

// transferProfiles is the shipped firmware matrix, read once. Match is pure and
// the registry is immutable, so one value serves every arrival.
//
//nolint:gochecknoglobals // one immutable value, built once.
var transferProfiles = sync.OnceValue(func() profile.Registry {
	built, err := profile.Builtin()
	if err != nil {
		// A build whose own profile file will not parse has a bigger problem
		// than this, and it is reported where the file is loaded for the
		// resolver. An empty registry covers nothing, which is the answer that
		// lets a printer be sent a file rather than the one that refuses every
		// printer on a data error.
		return profile.Registry{}
	}

	return built
})

// UseLogger takes the logger this process writes through.
//
// connectorapi.Factory is handed a spec and nothing else, so a driver built
// from configuration reaches no logger on its own and every line this package
// writes goes nowhere. The host calls this once, before the handler is mounted
// and before any printer can reach it, which is why the field needs no guard.
func (d *Driver) UseLogger(log *slog.Logger) { d.log = log }

// Baseline is what a caller may ask this driver for, on any printer. A profile
// subtracts from it and nothing can add to it.
//
// It is the wire table less the transfer starts this driver sends for itself: a
// key here is invokable through the command endpoint, and those two are not
// (ADR-20260826-ezr rule 7, sentNotInvokable). A file is sent through Transfers.Send,
// which is the whole exchange rather than one frame.
func (d *Driver) Baseline() connectorapi.Set { return published().Clone() }

// Handler returns the routes a printer dials. Every path is written from this
// driver's own root, so a host is free to mount it under a prefix it strips.
//
// It routes by hand rather than through a mux. Two frozen paths do not need a
// router, and internal/transport/http/httpapi's contract test refuses a
// net/http mux anywhere under internal/: a route registered on one is a route
// chi.Walk cannot enumerate and the OpenAPI document never publishes.
func (d *Driver) Handler(arrivals connectorapi.Arrivals) (http.Handler, error) {
	if arrivals == nil {
		return nil, errors.New("xbuddy: the host passed no Arrivals, so a printer that dials in reaches nothing")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == registerPath && r.Method == http.MethodPost:
			d.handleBeginRegistration(w, r)
		case r.URL.Path == registerPath && r.Method == http.MethodGet:
			d.handlePollRegistration(w, r)
		case r.URL.Path == authPath && r.Method == http.MethodGet:
			d.handleAuth(w, r, arrivals)
		case r.URL.Path == websocketPath && r.Method == http.MethodGet:
			d.handleWebsocket(w, r, arrivals)
		case r.URL.Path == registerPath || r.URL.Path == authPath || r.URL.Path == websocketPath:
			w.Header().Set("Allow", allowedMethods(r.URL.Path))
			http.Error(w, "that method is not one this route answers", http.StatusMethodNotAllowed)
		default:
			http.NotFound(w, r)
		}
	}), nil
}

// Routes are the paths this dialect's firmware dials, absolute, in the order
// the handshake uses them.
//
// connectorapi.Inbound lets a host mount a driver under a prefix of its own and
// strip it. This dialect cannot take that offer: the paths are compiled into
// firmware in the field that is not updated on this project's schedule,
// so a host serving those printers has to put the
// handler exactly here. A host that only serves printers it can configure is
// free to ignore this and mount the handler wherever it likes.
func (d *Driver) Routes() []string { return []string{registerPath, authPath, websocketPath} }

// Profiles are the capability profiles this dialect ships.
//
// The file internal/printer/profile embeds is this dialect's firmware matrix
// and nothing else: it was extracted from the shipping xBuddy connector,
// and every model in it is a board this driver drives. A host
// resolves each driver against its own profiles, because the union of a
// registry is what bounds a device none of its profiles cover, and holding one
// dialect's unknown devices to another's data subtracts capabilities the device
// really has.
func (d *Driver) Profiles() (profile.Registry, error) { return profile.Builtin() }

// Approve lets one registration through. It is the operator's half of the
// handshake, and it is on the driver because the driver is what holds the code
// the printer is polling with.
//
// by is who decided, as the host names them. It is recorded against the
// registration and never checked here: whether this caller may decide anything
// was answered before the call reached a dialect.
func (d *Driver) Approve(ctx context.Context, code, by string) (admission.Registration, error) {
	return d.registrar.Approve(ctx, code, by)
}

// UseApprovals takes the notice this driver calls for an approval it made
// itself, which is what mode none makes on every registration.
//
// The host binds a device id at an approval, and an approval it never hears
// about is a machine that gets its id at the first handshake instead: on the
// bench that gave one CORE One two ids, because the two were bound under the
// two spellings its firmware puts on the wire (BUG-011).
//
// The host calls it once, before the handler is mounted and before any printer
// can register, which is why the field needs no guard beyond the table's own.
func (d *Driver) UseApprovals(notify connectorapi.ApprovalNotice) {
	d.registrar.mu.Lock()
	defer d.registrar.mu.Unlock()

	d.registrar.notify = notify
}

var (
	_ connectorapi.ReportsRefusals  = (*Driver)(nil)
	_ connectorapi.ReportsAdmission = (*Driver)(nil)
)

// UseRefusals takes the notice this driver calls for a printer it turned away
// on /p/ws. It is called at most once a minute per printer, on the same clock
// as the log line.
func (d *Driver) UseRefusals(notify connectorapi.RefusalNotice) {
	d.refusals.use(notify)
}

// AdmitsOnArrival reports whether this driver approves every registration the
// moment it arrives, which is mode none.
func (d *Driver) AdmitsOnArrival() bool {
	return d.registrar.approvesOnArrival()
}

// Reject forgets one registration and every credential it held. A code nothing
// answers to is admission.ErrUnknownCode.
func (d *Driver) Reject(ctx context.Context, code string) error {
	return d.registrar.Reject(ctx, code)
}

var _ connectorapi.ForgetsDevices = (*Driver)(nil)

// ForgetDevice withdraws the registration, the token and the approved
// fingerprint this driver holds for one printer, when it was issued before
// issuedBefore. The host calls it when the printer's record is retired. The
// printer is then refused on /p/ws in modes token, fingerprint and both, and
// gets back in only by registering again and, where a person approves, being
// approved again. Mode none checks no credential, so there this keeps the
// registration and the printer joins again by itself.
func (d *Driver) ForgetDevice(
	ctx context.Context, identity connectorapi.Identity, issuedBefore time.Time,
) error {
	return d.registrar.forgetDevice(ctx, identity.UnitUID, issuedBefore)
}

// UseRegistrations gives this driver the store its registration table survives
// a restart in, and reads back the table the last process left.
//
// The host calls it once, before the handler is mounted and before any printer
// can reach it. A driver that is never handed one keeps its table in memory and
// behaves exactly as it did before the store existed.
func (d *Driver) UseRegistrations(
	ctx context.Context, connector, kind string, store admission.Store,
) error {
	return d.registrar.restore(ctx, connector, kind, store)
}

// Pending returns the registrations waiting for an operator to decide, in code
// order.
func (d *Driver) Pending() []admission.Registration { return d.registrar.pending() }

// registerRequest is what a printer says about itself. The field names are
// frozen: firmware in the field sends exactly these.
type registerRequest struct {
	SerialNumber string `json:"sn"`
	Fingerprint  string `json:"fingerprint"`
	PrinterType  string `json:"printer_type"`
	Firmware     string `json:"firmware"`
}

// handleBeginRegistration answers POST /p/register with the code the printer
// polls on.
func (d *Driver) handleBeginRegistration(w http.ResponseWriter, r *http.Request) {
	var body registerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRegistrationBytes)).Decode(&body); err != nil {
		http.Error(w, "the registration payload is not readable", http.StatusBadRequest)

		return
	}

	if body.Fingerprint == "" {
		http.Error(w, "the registration carries no fingerprint", http.StatusBadRequest)

		return
	}

	code, dropped, err := d.registrar.begin(r.Context(), Registration{
		Fingerprint: body.Fingerprint,
		Serial:      body.SerialNumber,
		PrinterType: body.PrinterType,
		Firmware:    body.Firmware,
	})

	switch {
	case errors.Is(err, ErrRegistrationsFull):
		d.logWarn(r.Context(), "the registration table is full, so a printer was refused a code",
			slog.String("fingerprint", body.Fingerprint), slog.String("error", err.Error()))
		w.Header().Set("Retry-After", retryAfterFull)
		http.Error(w, "this server is holding as many registrations as it may", http.StatusServiceUnavailable)

		return
	case err != nil:
		d.logError(r.Context(), "a registration could not be issued", slog.String("error", err.Error()))
		http.Error(w, "the registration could not be issued", http.StatusInternalServerError)

		return
	}

	if dropped != "" {
		d.logWarn(r.Context(), "a registration was dropped to make room for another",
			slog.String("dropped", dropped), slog.String("fingerprint", body.Fingerprint))
	}

	w.Header().Set(codeHeader, code)
	w.WriteHeader(http.StatusOK)
}

// handlePollRegistration answers GET /p/register. 202 is the printer's cue to
// keep polling, which is what it does for as long as nobody has decided.
func (d *Driver) handlePollRegistration(w http.ResponseWriter, r *http.Request) {
	code := r.Header.Get(codeHeader)
	if code == "" {
		http.Error(w, "the poll carries no code", http.StatusBadRequest)

		return
	}

	entry, err := d.registrar.complete(r.Context(), code)

	switch {
	case errors.Is(err, ErrRegistrationPending):
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, ErrUnknownCode):
		http.Error(w, "no registration answers to that code", http.StatusNotFound)
	case err != nil:
		http.Error(w, "the registration could not be completed", http.StatusInternalServerError)
	default:
		w.Header().Set(tokenHeader, entry.Token)
		w.WriteHeader(http.StatusOK)
	}
}

// handleWebsocket is the whole of the join: the printer proves it is one this
// driver registered, the host decides whether it wants it, and the session is
// either created or reattached.
//
// Admit runs on every connection and not only the first. A host that has
// stopped wanting a printer says so there, and a printer that reconnects
// through a driver that never asked again would be a printer nothing
// supervises.
//
// A session is reattached only while the printer is still what that session was
// built for. A capability set is fixed for a session's life here, so a machine
// that comes back reflashed gets a session of its own rather than a set decided
// for the firmware of the connection before it.
func (d *Driver) handleWebsocket(w http.ResponseWriter, r *http.Request, arrivals connectorapi.Arrivals) {
	fingerprint, err := d.registrar.authenticate(r.Header.Get(fingerprintHeader), r.Header.Get(tokenHeader))
	if err != nil {
		d.reportRefusedWebsocket(r, http.StatusUnauthorized, err)
		http.Error(w, "the printer's credentials were refused", http.StatusUnauthorized)

		return
	}

	identity := d.identityOf(fingerprint, r)

	profile, err := arrivals.Admit(r.Context(), identity)
	if err != nil {
		d.reportRefusedWebsocket(r, http.StatusForbidden, err)
		http.Error(w, "the host does not accept this printer", http.StatusForbidden)

		return
	}

	if existing, found := d.lookup(fingerprint); found {
		if existing.Identity() == identity {
			d.reattach(w, r, existing)

			return
		}

		// Whatever was queued for the old machine is dropped with it. A
		// command composed against one firmware's capability set is not a
		// command to carry out on another's.
		//
		// CloseReplaced and not Close: each dropped command answers its caller
		// with core.printer/PRINTER_CHANGED and the two firmware labels, so a
		// person who sent a command while the machine was away learns it was
		// not sent and why, rather than reading a failure the server does not
		// describe.
		_ = existing.CloseReplaced(identity)
	}

	d.join(w, r, arrivals, identity, fingerprint, profile)
}

// join builds a session for a printer the host has not seen, announces it, and
// puts the connection under it.
func (d *Driver) join(
	w http.ResponseWriter,
	r *http.Request,
	arrivals connectorapi.Arrivals,
	identity connectorapi.Identity,
	fingerprint string,
	profile connectorapi.Set,
) {
	// What a caller may ask for. The host resolved it against this driver's
	// published baseline, so it carries nothing that is not invokable.
	resolved := baseline().Intersect(profile)

	session := newSession(settings{
		identity:        identity,
		caps:            resolved,
		sends:           d.sendable(identity),
		storage:         d.storage,
		chunkCeiling:    d.chunkCeiling,
		queueDepth:      d.queueDepth,
		grace:           d.grace,
		live:            d.live,
		commandDeadline: d.commandDeadline,
		log:             d.log,
		onClose:         func() { d.forget(fingerprint) },
		onFirmware:      d.refilesFirmware(r, fingerprint),
	})

	if winner, taken := d.claim(fingerprint, session); taken {
		// Another connection for this printer won the race. Nothing has used
		// this session, so closing it costs the two goroutines it started.
		_ = session.Close()
		d.reattach(w, r, winner)

		return
	}

	conn, err := d.upgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = session.Close()

		return
	}

	if taken := arrivals.Arrived(r.Context(), session); taken != nil {
		_ = session.Close()
		_ = conn.Close()

		return
	}

	if !session.attach(r.Context(), conn) {
		_ = conn.Close()
	}
}

// reattach puts a new connection under the session a printer already has, which
// is what drains the commands queued while it was away.
func (d *Driver) reattach(w http.ResponseWriter, r *http.Request, session *Session) {
	conn, err := d.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	if !session.attach(r.Context(), conn) {
		_ = conn.Close()
	}
}

// identityOf is what the printer said it is: the two headers the handshake
// carries, plus the serial it gave when it registered.
//
// fingerprint is the spelling this table knows the printer by, which is not
// always the one the header carried: firmware puts fifty characters in a
// registration body and the leading sixteen in every header. When the two
// differ, the header spelling goes on the identity as well (secondSpellingOf),
// so the host learns that one machine has two spellings from the machine that
// got through the handshake rather than from a prefix anybody can copy off the
// wire and put in a registration.
func (d *Driver) identityOf(fingerprint string, r *http.Request) connectorapi.Identity {
	identity := connectorapi.Identity{
		Vendor:       Vendor,
		Model:        strings.TrimSpace(r.Header.Get(modelHeader)),
		Firmware:     connectorapi.NewFirmwareLabel(strings.TrimSpace(r.Header.Get(firmwareHeader))),
		UnitUID:      fingerprint,
		UnitUIDAlias: secondSpellingOf(r.Header.Get(fingerprintHeader), fingerprint),
	}

	if entry, ok := d.registrar.registrationFor(fingerprint); ok {
		identity.Serial = entry.Serial

		if identity.Model == "" {
			identity.Model = entry.PrinterType
		}

		if identity.Firmware.IsZero() {
			identity.Firmware = connectorapi.NewFirmwareLabel(entry.Firmware)
		}
	}

	return identity
}

// claim files a session under its fingerprint. winner is the session already
// there, and taken says the caller lost the race.
func (d *Driver) claim(fingerprint string, session *Session) (winner *Session, taken bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.sessions[fingerprint]; ok {
		return existing, true
	}

	d.sessions[fingerprint] = session

	return nil, false
}

func (d *Driver) lookup(fingerprint string) (*Session, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	session, ok := d.sessions[fingerprint]

	return session, ok
}

func (d *Driver) forget(fingerprint string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.sessions, fingerprint)
}

func (d *Driver) logError(ctx context.Context, message string, attrs ...slog.Attr) {
	if d.log == nil {
		return
	}

	d.log.LogAttrs(ctx, slog.LevelError, message, attrs...)
}

// logWarn reports something an operator has to know about and this process has
// survived, which is what a dropped or refused registration is.
func (d *Driver) logWarn(ctx context.Context, message string, attrs ...slog.Attr) {
	if d.log == nil {
		return
	}

	d.log.LogAttrs(ctx, slog.LevelWarn, message, attrs...)
}

// allowedMethods is what a route answers to, for the Allow header a 405 owes
// the caller.
func allowedMethods(path string) string {
	if path == registerPath {
		return http.MethodPost + ", " + http.MethodGet
	}

	return http.MethodGet
}

// parseTimeouts reads the liveness block. An absent key takes the shipping
// value, so a config naming none is the shipping connector's behaviour.
//
// Positive, and an interval below the deadline, are the shipping validator's
// rules (legacy/app/config.go:732-743). The ceiling on the interval is this
// port's: shortening it is safe and lengthening it is the one direction that can
// drop a printer that is really there, so the interval field firmware has been
// running against is the most this will take.
func parseTimeouts(cfg Config) (Liveness, error) {
	read, err := parsePositive("read_deadline", cfg.ReadDeadline, readDeadline)
	if err != nil {
		return Liveness{}, err
	}

	every, err := parsePositive("ping_interval", cfg.PingInterval, pingInterval)
	if err != nil {
		return Liveness{}, err
	}

	timeout, err := parsePositive("ping_timeout", cfg.PingTimeout, pingTimeout)
	if err != nil {
		return Liveness{}, err
	}

	if every >= read {
		return Liveness{}, fmt.Errorf("%w: ping_interval is %s and read_deadline is %s, so the deadline "+
			"expires before a pong can re-arm it and every printer is dropped on a timer; shorten "+
			"ping_interval below %s", ErrInvalidConfig, every, read, read)
	}

	if every > pingInterval {
		return Liveness{}, fmt.Errorf("%w: ping_interval is %s, above the %s firmware in the field has "+
			"been running against; a longer interval risks dropping printers that are there, and this "+
			"connector only shortens it", ErrInvalidConfig, every, pingInterval)
	}

	return Liveness{ReadDeadline: read, PingInterval: every, PingTimeout: timeout}, nil
}

// parsePositive reads one duration key. Empty takes the shipping value, and
// zero is refused rather than silently taken as one: an operator who wrote 0s
// asked for something this connector cannot do.
func parsePositive(key, value string, shipping time.Duration) (time.Duration, error) {
	if value == "" {
		return shipping, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", ErrInvalidConfig, key, err)
	}

	if parsed <= 0 {
		return 0, fmt.Errorf("%w: %s is %s, expected a positive duration", ErrInvalidConfig, key, parsed)
	}

	return parsed, nil
}

// parseGrace reads the offline grace, which is absent, zero or a positive
// duration and never a negative one.
func parseGrace(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: offline_grace: %w", ErrInvalidConfig, err)
	}

	if parsed < 0 {
		return 0, fmt.Errorf("%w: offline_grace must not be negative, and it is %s", ErrInvalidConfig, parsed)
	}

	return parsed, nil
}
