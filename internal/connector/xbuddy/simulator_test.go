package xbuddy_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prusa3d/prusa-connect/internal/connector/xbuddy"
	"github.com/prusa3d/prusa-connect/internal/printer/capability"
	"github.com/prusa3d/prusa-connect/internal/printer/profile"
	"github.com/prusa3d/prusa-connect/internal/printer/simulator"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The port is exercised against internal/printer/simulator, which is projected
// from the same profile data the shipping firmware matrix was extracted into
// and speaks the frozen wire. A driver checked against a mock written beside it
// proves that the two agree; a driver checked against the simulator proves it
// speaks the wire.
//
// Everything runs over net.Pipe inside a testing/synctest bubble, so a printer
// that is away for an hour costs microseconds.

// Scope item 1: a printer registers, waits for an operator, is admitted, and
// becomes a session the host holds.
func TestAPrinterRegistersWaitsForApprovalAndBecomesASession(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withApproval)
		defer rig.stop(t)

		registered := make(chan error, 1)
		go func() { registered <- rig.printer.Register(rig.ctx) }()

		synctest.Wait()

		waiting := rig.driver.Pending()
		if len(waiting) != 1 {
			t.Fatalf("%d registrations are waiting for an operator, want 1", len(waiting))
		}

		if got := waiting[0].Fingerprint; got != rig.printer.Fingerprint() {
			t.Fatalf("the registration waiting is for %q, want %q", got, rig.printer.Fingerprint())
		}

		t.Logf("observed: the printer is polling and holds code %q for serial %q, model %q, firmware %q",
			waiting[0].Code, waiting[0].Serial, waiting[0].Model, waiting[0].Firmware)

		if _, err := rig.driver.Approve(t.Context(), waiting[0].Code, "user:tester"); err != nil {
			t.Fatalf("approve: %v", err)
		}

		if err := <-registered; err != nil {
			t.Fatalf("register: %v", err)
		}

		if rig.printer.Token() == "" {
			t.Fatal("the printer was approved and holds no token")
		}

		t.Logf("observed: the printer holds token %q", rig.printer.Token())

		rig.run()

		session := rig.await(t)
		identity := session.Identity()

		t.Logf("observed identity: vendor %q, model %q, firmware %q, serial %q, unit %q",
			identity.Vendor, identity.Model, identity.Firmware, identity.Serial, identity.UnitUID)

		if identity.Model != "MK4" {
			t.Errorf("the host was told the model is %q, want %q", identity.Model, "MK4")
		}

		if identity.Serial != rig.printer.Serial() {
			t.Errorf("the host was told the serial is %q, want %q", identity.Serial, rig.printer.Serial())
		}

		caps, epoch := session.Capabilities()
		t.Logf("observed: %d capabilities resolved at epoch %d, digest %s", len(caps), epoch, caps.Digest())

		if !caps.Has(xbuddy.VerbStartPrint) {
			t.Error("an MK4 session cannot start a print")
		}
	})
}

// A printer the host will not take is hung up on rather than served, so there
// is no window in which a printer nothing supervises is on the wire.
func TestAPrinterTheHostRefusesNeverBecomesASession(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// A long redial delay is what makes the assertion possible at all:
		// virtual time advances the moment everything is blocked, so a printer
		// retrying every 100ms is a bubble that is never idle and never
		// observed.
		rig := start(t, "MK4", withRefusal, withPrinter(func(cfg *simulator.Config) {
			cfg.ReconnectDelay = time.Hour
		}))
		defer rig.stop(t)

		// The registration is approved: what refuses this printer is the host,
		// at Admit, and a printer nobody let through would prove nothing about
		// that.
		registered := make(chan error, 1)
		go func() { registered <- rig.printer.Register(rig.ctx) }()

		rig.approve(t)

		if err := <-registered; err != nil {
			t.Fatalf("register: %v", err)
		}

		rig.run()
		synctest.Wait()

		select {
		case session := <-rig.host.sessions:
			t.Fatalf("a refused printer became a session: %v", session.Identity())
		default:
		}

		t.Logf("observed: %d admissions asked, none taken", len(rig.host.admissions()))
	})
}

// Scope item 3: a command reaches the printer and its answer comes back under
// the id the frame carried.
func TestACommandReachesThePrinterAndItsResultComesBack(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		session := rig.connect(t)

		result, err := session.Do(rig.ctx, connectorapi.Command{Verb: xbuddy.VerbSendInfo})
		if err != nil {
			t.Fatalf("SEND_INFO: %v", err)
		}

		t.Logf("observed result: ok=%v raw=%s", result.OK, result.Raw)

		if !result.OK {
			t.Error("the printer refused SEND_INFO")
		}

		if !bytes.Contains(result.Raw, []byte(`"event":"INFO"`)) {
			t.Errorf("the answer is not an INFO event: %s", result.Raw)
		}

		// A gcode verb: HOME never travels as JSON, so this is the half a
		// driver generated from profiles.json alone would get wrong.
		homed, err := session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbHome, Args: map[string]string{"axis": "XYZ"},
		})
		if err != nil {
			t.Fatalf("HOME: %v", err)
		}

		t.Logf("observed result for the gcode verb HOME: ok=%v raw=%s", homed.OK, homed.Raw)

		if !homed.OK {
			t.Error("the printer refused HOME")
		}

		// A SET_VALUE verb, on a model that has one. The path is one the
		// simulated printer's default tree holds, because a printer refuses a
		// print of a file it does not have.
		started, err := session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbStartPrint, Args: map[string]string{"path": "tangle.bgcode"},
		})
		if err != nil {
			t.Fatalf("START_PRINT: %v", err)
		}

		if !started.OK {
			t.Errorf("the printer refused START_PRINT: %s", started.Raw)
		}

		if got, want := rig.printer.State(), "PRINTING"; got != want {
			t.Errorf("the printer is %q after START_PRINT, want %q", got, want)
		}

		t.Logf("observed: the simulated printer is %q", rig.printer.State())

		// LOAD_FILAMENT is M701 with the name in S"...", so the printer receives
		// the filament the caller chose and the result reports nothing ignored.
		loaded, err := session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbLoadFilament, Args: map[string]string{"filament": "PETG"},
		})
		if err != nil {
			t.Fatalf("LOAD_FILAMENT: %v", err)
		}

		t.Logf("observed result for LOAD_FILAMENT: ok=%v ignored=%v", loaded.OK, loaded.Ignored)

		if len(loaded.Ignored) != 0 {
			t.Errorf("the printer receives the filament name and the result reports %v as ignored", loaded.Ignored)
		}

		if name, asked := rig.printer.FilamentToLoad(); !asked || name != "PETG" {
			t.Errorf("the printer was asked to load %q (asked: %v), want PETG", name, asked)
		}

		// The firmware keeps seven bytes of a name. The host refuses an
		// eight-byte name before the driver sees it, so the printer is never
		// sent a name it would read as no name.
		_, err = session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbLoadFilament, Args: map[string]string{"filament": "ABCDEFGH"},
		})
		if !fault.HasCode(err, connectorapi.ArgumentOutOfRange.Code()) {
			t.Errorf("an eight-byte filament name reported %v, want %s", err, connectorapi.ArgumentOutOfRange.Code())
		}

		if name, _ := rig.printer.FilamentToLoad(); name != "PETG" {
			t.Errorf("the refused name reached the printer, which was asked to load %q", name)
		}
	})
}

// Scope item 3, the half earlier research says is most likely to be lost:
// a command sent while the printer is away is delivered when it comes back.
func TestACommandSentWhileOfflineIsDeliveredOnReconnect(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		session := rig.connect(t)

		rig.printer.GoOffline()
		synctest.Wait()

		answered := make(chan connectorapi.Result, 1)
		failed := make(chan error, 1)

		go func() {
			result, err := session.Do(rig.ctx, connectorapi.Command{Verb: xbuddy.VerbSetPrinterReady})
			if err != nil {
				failed <- err

				return
			}

			answered <- result
		}()

		// An hour of virtual time with the printer switched off. Nothing is
		// answered and nothing is failed: the command waits.
		time.Sleep(time.Hour)
		synctest.Wait()

		select {
		case result := <-answered:
			t.Fatalf("an offline printer answered: %v", result)
		case err := <-failed:
			t.Fatalf("a command queued for an offline printer failed: %v", err)
		default:
		}

		t.Log("observed: after an hour offline the command is still queued")

		rig.printer.GoOnline()

		select {
		case result := <-answered:
			t.Logf("observed: the queued command was answered on reconnect, ok=%v raw=%s", result.OK, result.Raw)

			if !result.OK {
				t.Errorf("the drained command was refused: %s", result.Raw)
			}
		case err := <-failed:
			t.Fatalf("the drained command failed: %v", err)
		case <-rig.ctx.Done():
			t.Fatal("the queued command was never delivered")
		}

		if got, want := rig.printer.State(), "READY"; got != want {
			t.Errorf("the printer is %q after the drained command, want %q", got, want)
		}

		if got, want := rig.printer.Sessions(), uint64(2); got != want {
			t.Errorf("the printer established %d websocket sessions, want %d", got, want)
		}

		// The host was asked again on the reconnect and was handed no second
		// session: one printer is one session, however many sockets it takes.
		if got := len(rig.host.admissions()); got != 2 {
			t.Errorf("the host was asked to admit the printer %d times, want 2", got)
		}

		synctest.Wait()

		select {
		case extra := <-rig.host.sessions:
			t.Fatalf("the reconnect announced a second session: %v", extra.Identity())
		default:
		}

		t.Logf("observed: the printer is %q after %d websocket sessions, %d admissions and one host session",
			rig.printer.State(), rig.printer.Sessions(), len(rig.host.admissions()))
	})
}

// Scope item 4: telemetry becomes patches to the addressable state tree.
func TestTelemetryBecomesStatePatches(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		session := rig.connect(t)

		seen := map[string]string{}
		wanted := []string{"/state", "/tools/0/nozzle_temp", "/temperature/bed", "/axis/z"}

		for !holdsAll(seen, wanted) {
			select {
			case patch, open := <-session.State():
				if !open {
					t.Fatal("the state channel closed before the printer reported anything")
				}

				seen[patch.Path] = string(patch.Value)
			case <-rig.ctx.Done():
				t.Fatalf("only %d paths arrived: %v", len(seen), seen)
			}
		}

		t.Logf("observed patches: %v", seen)

		if got, want := seen["/state"], `"IDLE"`; got != want {
			t.Errorf("/state is %s, want %s", got, want)
		}

		if _, reported := seen["/tools/0/nozzle_temp"]; !reported {
			t.Error("no nozzle temperature reached the state tree")
		}
	})
}

// Scope item 3, an earlier design decision: the host refuses a verb the model does not declare
// before the driver is reached, so the frame is never built and the printer
// never has to say no.
func TestAVerbTheModelDoesNotDeclareIsRefusedBeforeTheDriver(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MINI", withAutoRegistration)
		defer rig.stop(t)

		session := rig.connect(t)

		before := rig.spy.calls.Load()

		_, err := session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbSetChamberTemp, Args: map[string]string{"chamber.target_temp": "40"},
		})
		if err == nil {
			t.Fatal("a MINI accepted a chamber temperature")
		}

		t.Logf("observed refusal: %v", err)

		var refusal *fault.Fault
		if !errors.As(err, &refusal) {
			t.Fatalf("the refusal is not a fault: %v", err)
		}

		if got, want := refusal.Code(), connectorapi.CapabilityNotSupported.Code(); got != want {
			t.Errorf("the refusal carries %s, want %s", got, want)
		}

		if after := rig.spy.calls.Load(); after != before {
			t.Errorf("the driver was reached %d times for a verb the model does not declare", after-before)
		}

		t.Logf("observed: the driver's Do was called %d times, so nothing was put on the wire", rig.spy.calls.Load())

		// The session still works, which is what says the refusal cost nothing.
		if _, stillWorks := session.Do(rig.ctx, connectorapi.Command{Verb: xbuddy.VerbSendInfo}); stillWorks != nil {
			t.Errorf("the session is unusable after a refusal: %v", stillWorks)
		}
	})
}

// A session outlives an absence so its queue does, and an operator who wants a
// printer reaped after a while says how long. Nothing else ends a session: the
// contract has no "the device went away but may come back", so an absence is a
// session with no socket under it.
func TestASessionEndsAfterTheOfflineGrace(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration, withGrace(time.Minute))
		defer rig.stop(t)

		session := rig.connect(t)

		rig.printer.GoOffline()
		synctest.Wait()

		select {
		case _, open := <-session.State():
			if !open {
				t.Fatal("the session ended the moment the printer went away, before the grace")
			}
		default:
		}

		time.Sleep(2 * time.Minute)
		synctest.Wait()

		for {
			patch, open := <-session.State()
			if !open {
				break
			}

			_ = patch
		}

		t.Log("observed: the state channel closed once the printer had been away past its grace")
	})
}

// Every model the profile data covers has to survive the intersection with this
// driver's baseline. A verb both sides declare that the merge drops is a
// descriptor disagreement, and it would show up in the field as a capability
// nobody can reach.
func TestEveryProfileIntersectsWithTheBaseline(t *testing.T) {
	t.Parallel()

	profiles, err := profile.Builtin()
	if err != nil {
		t.Fatalf("load the profiles: %v", err)
	}

	base := xbuddy.NewDriver(xbuddy.ModeNone, 1, 0, "", nil).Baseline()

	for _, p := range profiles.Profiles {
		resolved := base.Intersect(p.Capabilities)

		for _, key := range base.Keys() {
			if _, isArgument := key.Argument(); isArgument {
				continue
			}

			if !base.Has(key) || !p.Capabilities.Has(key) || resolved.Has(key) {
				continue
			}

			t.Errorf("%s: %s is in the baseline and in the profile and does not survive the merge", p.Model, key)
		}
	}
}

// The rig, and the host the driver is wired into.

type admissionMode uint8

const (
	withAutoRegistration admissionMode = iota
	withApproval
	withRefusal
)

type rig struct {
	driver   *xbuddy.Driver
	host     *arrivals
	printer  *simulator.Printer
	ctx      context.Context //nolint:containedctx // the rig is one test's scope and dies with it.
	cancel   context.CancelFunc
	server   *http.Server
	listener *pipeListener
	sessions []*capability.Session
	spy      *driverSpy
	running  chan error
}

// rigOptions are the two halves a case can vary: the driver's settings and the
// simulated printer's.
type rigOptions struct {
	grace    time.Duration
	deadline time.Duration
	printer  func(*simulator.Config)
}

type rigOption func(*rigOptions)

// withGrace closes a session whose printer has been away that long.
func withGrace(grace time.Duration) rigOption {
	return func(o *rigOptions) { o.grace = grace }
}

// withCommandDeadline bounds how long one command waits for the printer to
// commandDeadline. Without it the driver's own default applies, which is longer
// than any case here waits.
func withCommandDeadline(o *rigOptions) { o.deadline = commandDeadline }

// withPrinter varies the simulated printer.
func withPrinter(tweak func(*simulator.Config)) rigOption {
	return func(o *rigOptions) { o.printer = tweak }
}

func start(t *testing.T, model string, mode admissionMode, opts ...rigOption) *rig {
	t.Helper()

	options := rigOptions{}
	for _, apply := range opts {
		apply(&options)
	}

	profiles, err := profile.Builtin()
	if err != nil {
		t.Fatalf("load the profiles: %v", err)
	}

	quiet := slog.New(slog.DiscardHandler)

	host := &arrivals{
		profiles: profiles,
		resolver: capability.NewResolver(profiles, quiet),
		refuse:   mode == withRefusal,
		sessions: make(chan *capability.Session, 4),
		spies:    make(chan *driverSpy, 4),
	}

	// ModeToken is the strictest mode a simulated printer can satisfy: it
	// presents the token registration issued and nothing else. Every
	// registration in it waits for an operator, so rig.connect approves the one
	// the printer makes; withApproval is the case that does that half itself.
	// A refused printer still registers: the refusal under test is the host's,
	// at Admit, and mixing the two would prove nothing about either.
	settings := []xbuddy.Option{}
	if options.deadline > 0 {
		settings = append(settings, xbuddy.WithCommandDeadline(options.deadline))
	}

	driver := xbuddy.NewDriver(xbuddy.ModeToken, 8, options.grace, "", quiet, settings...)
	host.baseline = driver.Baseline()

	handler, err := driver.Handler(host)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	listener := newPipeListener()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	go func() { _ = server.Serve(listener) }()

	cfg := simulator.Config{Model: model, BaseURL: "http://xbuddy-host", Dial: listener.dial}
	if options.printer != nil {
		options.printer(&cfg)
	}

	printer, err := simulator.New(cfg)
	if err != nil {
		t.Fatalf("new printer: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())

	return &rig{
		driver: driver, host: host, printer: printer,
		ctx: ctx, cancel: cancel, server: server, listener: listener,
	}
}

// run starts the printer's own loop.
func (r *rig) run() {
	r.running = make(chan error, 1)

	go func() { r.running <- r.printer.Run(r.ctx) }()
}

// connect registers, runs and takes the session, which is what every case but
// the registration one wants.
func (r *rig) connect(t *testing.T) *capability.Session {
	t.Helper()

	// The printer registers and polls until somebody decides, so the operator's
	// half runs on this goroutine while the printer's runs on its own.
	registered := make(chan error, 1)
	go func() { registered <- r.printer.Register(r.ctx) }()

	r.approve(t)

	if err := <-registered; err != nil {
		t.Fatalf("register: %v", err)
	}

	r.run()

	return r.await(t)
}

// approve lets through the registration the printer is polling on, which is
// what an operator does at /api/v1/registrations.
//
// It polls rather than synchronising, so it works inside a synctest bubble and
// outside one: in a bubble the sleep is instant once every goroutine is idle.
func (r *rig) approve(t *testing.T) {
	t.Helper()

	const attempts = 500

	for range attempts {
		if waiting := r.driver.Pending(); len(waiting) > 0 {
			if _, err := r.driver.Approve(r.ctx, waiting[0].Code, "user:tester"); err != nil {
				t.Fatalf("approve: %v", err)
			}

			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("no registration was waiting for an operator, so the printer is polling on nothing")
}

// await takes the session the host was handed, and the printer's half of it.
func (r *rig) await(t *testing.T) *capability.Session {
	t.Helper()

	select {
	case session := <-r.host.sessions:
		r.sessions = append(r.sessions, session)
		r.spy = <-r.host.spies

		r.awaitAdoption(t, uint64(len(r.sessions)))

		return session
	case <-r.ctx.Done():
		t.Fatal("no printer was announced")

		return nil
	}
}

// awaitAdoption waits until the printer has taken on as many connections as the
// host has been handed sessions.
//
// The host is handed one the moment the server accepts the upgrade, and the
// printer's own dial returns after that, so the two are not ordered. A case
// that drove the printer straight after connect found it holding no connection
// and failed with "the printer is not connected". That is
// TestABlockRequestForNothingIsAnsweredWithSilence on a two-core runner. The
// wait is on the printer rather than on the scheduler.
func (r *rig) awaitAdoption(t *testing.T, want uint64) {
	t.Helper()

	for r.printer.Sessions() < want {
		select {
		case <-r.printer.Adopted():
		case <-r.ctx.Done():
			t.Fatal("the printer never took on the connection the host accepted")

			return
		}
	}
}

// stop ends everything the rig started, in the order that lets each goroutine
// finish: the printer first, then the sessions holding websockets, then the
// server.
func (r *rig) stop(t *testing.T) {
	t.Helper()

	r.cancel()

	if r.running != nil {
		<-r.running
	}

	for _, session := range r.sessions {
		if err := session.Close(); err != nil {
			t.Errorf("close a session: %v", err)
		}
	}

	_ = r.server.Close()
	_ = r.listener.Close()
}

// arrivals is the host's half of an inbound dialect, wired the way the real one
// is: capability.Resolve on Admit and capability.Wrap on Arrived.
type arrivals struct {
	profiles profile.Registry
	resolver *capability.Resolver
	baseline connectorapi.Set
	refuse   bool

	sessions chan *capability.Session
	spies    chan *driverSpy

	mu       sync.Mutex
	admitted []connectorapi.Identity
}

func (a *arrivals) Admit(_ context.Context, identity connectorapi.Identity) (connectorapi.Set, error) {
	a.mu.Lock()
	a.admitted = append(a.admitted, identity)
	a.mu.Unlock()

	if a.refuse {
		return nil, fmt.Errorf("%w: this host takes no printers", connectorapi.ErrDeviceRefused)
	}

	matched, ok := a.profiles.Match(identity.Model, identity.Firmware)
	if !ok {
		return nil, fmt.Errorf("%w: no profile covers %s", connectorapi.ErrDeviceRefused, identity.Model)
	}

	return matched.Capabilities, nil
}

func (a *arrivals) Arrived(ctx context.Context, session connectorapi.Session) error {
	probe, _ := session.Capabilities()
	watched := &driverSpy{inner: session}
	resolution := a.resolver.Resolve(ctx, a.baseline, session.Identity(), probe, 1)

	a.spies <- watched
	a.sessions <- capability.Wrap(watched, resolution)

	return nil
}

func (a *arrivals) admissions() []connectorapi.Identity {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]connectorapi.Identity(nil), a.admitted...)
}

// driverSpy counts what reaches the driver, so "the host refused it first" is an
// observation rather than an inference.
type driverSpy struct {
	inner connectorapi.Session
	calls atomic.Int64
}

func (s *driverSpy) Identity() connectorapi.Identity { return s.inner.Identity() }

func (s *driverSpy) Capabilities() (connectorapi.Set, uint64) { return s.inner.Capabilities() }

func (s *driverSpy) State() <-chan connectorapi.StatePatch { return s.inner.State() }

func (s *driverSpy) Close() error { return s.inner.Close() }

func (s *driverSpy) Do(ctx context.Context, cmd connectorapi.Command) (connectorapi.Result, error) {
	s.calls.Add(1)

	return s.inner.Do(ctx, cmd)
}

// pipeListener hands out net.Pipe connections. net.Pipe blocks on channels,
// which testing/synctest counts as durably blocked, so a whole session runs in
// virtual time. A localhost socket does not.
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, io.EOF
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })

	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()

	select {
	case l.conns <- server:
		return client, nil
	case <-l.done:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// holdsAll reports whether every wanted path has arrived.
func holdsAll(seen map[string]string, wanted []string) bool {
	for _, path := range wanted {
		if _, ok := seen[path]; !ok {
			return false
		}
	}

	return true
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "xbuddy-host" }

// BUG-004: the print the state tree describes follows the machine even though
// the machine volunteers nothing about it.
//
// Buddy firmware sends a JOB_INFO when a print starts and never again. It sends
// none after PAUSE_PRINT, RESUME_PRINT or STOP_PRINT and none when a print
// reaches its own end: it moves its own state and waits to be asked. Measured
// on an MK4S running 6.5.7+12836, that left /job/state saying PRINTING for the
// whole of a print that was paused, resumed and then aborted, and the history
// kept PRINTING as how the print ended.
//
// So the machine's own /state is what every move is read off here, and
// /job/state is asserted only where the firmware really moves it: PRINTING at
// the start, and the terminal word this driver writes when the print ends. The
// simulated printer keeps the firmware's cadence, so the file reaching the tree
// and the SEND_JOB_INFO count are what say the driver put the question.
func TestThePrintIsRefreshedWhenTheMachineVolunteersNothing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// A print long enough that nothing here is racing the end of it.
		rig := start(t, "MK4", withAutoRegistration, withPrinter(func(cfg *simulator.Config) {
			cfg.JobFrames = 10000
		}))
		defer rig.stop(t)

		session := rig.connect(t)

		// The tree as this host holds it, filled from the patch channel. It is
		// read on this goroutine rather than in a collector, because every wait
		// below is a wait for a patch and nothing else runs between them.
		tree := map[string]string{}

		// The bound is virtual time and costs nothing to wait out. It is here
		// rather than left to the context because a host that never asks the
		// printer anything still receives telemetry for ever, so a case with no
		// bound would spin rather than fail.
		await := func(path, want string) {
			t.Helper()

			giveUp := time.After(time.Minute)

			for tree[path] != want {
				select {
				case patch, open := <-session.State():
					if !open {
						t.Fatalf("the state channel closed while waiting for %s to be %s", path, want)
					}

					tree[patch.Path] = string(patch.Value)
				case <-giveUp:
					t.Fatalf("%s never became %s; the tree holds %v", path, want, tree)
				case <-rig.ctx.Done():
					t.Fatalf("%s never became %s; the tree holds %v", path, want, tree)
				}
			}

			t.Logf("observed: %s is %s", path, want)
		}

		move := func(verb connectorapi.Capability, args map[string]string) {
			t.Helper()

			result, err := session.Do(rig.ctx, connectorapi.Command{Verb: verb, Args: args})
			if err != nil {
				t.Fatalf("%s: %v", verb, err)
			}

			if !result.OK {
				t.Fatalf("the printer refused %s: %s", verb, result.Raw)
			}
		}

		move(xbuddy.VerbStartPrint, map[string]string{"path": "tangle.bgcode"})

		// The file is the one fact about a print that telemetry never carries,
		// so this is JOB_INFO arriving and nothing else.
		await("/job/path", `"/usb/tangle.bgcode"`)
		await("/job/state", `"PRINTING"`)
		await("/state", `"PRINTING"`)

		// The pause and the resume are read off the machine, which is where the
		// firmware reports them. /job/state stays PRINTING across both: Buddy's
		// vocabulary for a print is PRINTING, FINISHED and IDLE, and a paused
		// print is a print.
		move(xbuddy.VerbPausePrint, nil)
		await("/state", `"PAUSED"`)

		move(xbuddy.VerbResumePrint, nil)
		await("/state", `"PRINTING"`)

		move(xbuddy.VerbStopPrint, nil)
		await("/state", `"STOPPED"`)

		// The terminal word, written by the driver as the machine reports it.
		// It is the answer a record keeps for a print that was aborted, and the
		// JOB_INFO the refresh brings back says IDLE because the machine has no
		// job left; that answer must not erase this one.
		await("/job/state", `"STOPPED"`)

		// The driver asked. Without this the case would pass on a host that
		// reads /state and never puts the question, which is the host that
		// tracked a real print for ever as PRINTING. The ask is deferred by
		// jobRefreshDelay and the moves above land off the event envelopes in
		// less time than that, so the question is waited for rather than read.
		asked := rig.printer.Asked("SEND_JOB_INFO")
		for deadline := time.Now().Add(10 * time.Second); asked == 0 && time.Now().Before(deadline); {
			time.Sleep(20 * time.Millisecond)
			asked = rig.printer.Asked("SEND_JOB_INFO")
		}

		if asked == 0 {
			t.Error("the driver never sent SEND_JOB_INFO, so nothing on this side of the wire " +
				"asked the printer what it is printing")
		} else {
			t.Logf("observed: the driver sent SEND_JOB_INFO %d times over one print", asked)
		}

		// The file survives the end of the print, because the record's history
		// is about a file and a machine that has stopped reports no job number
		// on telemetry at all.
		if got, want := tree["/job/path"], `"/usb/tangle.bgcode"`; got != want {
			t.Errorf("the print that stopped is recorded against %s, want %s", got, want)
		}
	})
}
