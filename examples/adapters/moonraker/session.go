package moonraker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The shipping liveness numbers. Moonraker sends a process reading every second
// of its own accord, so a minute of silence is a connection that has gone away
// rather than a machine with nothing to say.
const (
	readDeadline = 60 * time.Second
	pingInterval = 20 * time.Second
	pingTimeout  = 10 * time.Second
)

// ErrSessionEnded reports a call made on a session whose connection has gone.
var ErrSessionEnded = errors.New("moonraker: the session has ended")

// liveness is how long a connection may say nothing, how often the host asks it
// to say something, and how long that asking may take.
//
// A link that blackholes rather than resets leaves a socket that still accepts
// writes, so the ping proves nothing on its own and the read deadline is what
// turns the silence into a disconnection. The zero value takes the shipping
// numbers.
type liveness struct {
	readDeadline time.Duration
	pingInterval time.Duration
	pingTimeout  time.Duration
}

func (l liveness) orShipping() liveness {
	if l.readDeadline <= 0 {
		l.readDeadline = readDeadline
	}

	if l.pingInterval <= 0 {
		l.pingInterval = pingInterval
	}

	if l.pingTimeout <= 0 {
		l.pingTimeout = pingTimeout
	}

	return l
}

// Session is one connected Klipper machine.
//
// It has two lives. During the identity exchange the goroutine inside Connect
// reads the connection itself, because there is nothing to hand a reply to yet.
// After begin, one reader goroutine owns every read, and it is the only writer
// to the patch channel and therefore the only thing that closes it.
type Session struct {
	conn        *websocket.Conn
	httpBaseURL string
	httpClient  *http.Client
	timeout     time.Duration
	live        liveness
	log         *slog.Logger

	// reported is what printer.info answered, kept so Connect can name the
	// machine after the exchange it performed.
	reported printerInfo
	// asked is the objects the subscription covers, which is also what a status
	// query asks for.
	asked []string

	identity connectorapi.Identity
	caps     connectorapi.Set
	// probe is what this machine answered about itself during the handshake:
	// the verbs the objects it reports can carry out, with the bounds its own
	// configuration declares. The host reads it through
	// connectorapi.ReportsProbe and narrows the set it commands against to it.
	probe connectorapi.Set

	storagesMu     sync.Mutex
	storages       []connectorapi.Storage
	gcodesWritable bool

	transferMu sync.Mutex

	// reader gives each Klipper print an id, which the machine does not, and
	// remembers which head prints. See telemetry.go.
	reader reader

	// first is the reading the subscription answered with. The reader emits it
	// before its loop, so that the patch channel has exactly one writer.
	first []connectorapi.StatePatch

	patches chan connectorapi.StatePatch
	// done closes when the reader has gone, so a call waiting on a reply that
	// can no longer arrive is released.
	done chan struct{}

	nextID atomic.Uint64

	writing sync.Mutex

	pending sync.Map

	closing sync.Once

	dropped atomic.Uint64
	refused atomic.Uint64
}

var _ connectorapi.Session = (*Session)(nil)

// The host reads what this connection has cost through connectorapi.CountsLink.
// A session that answers neither method is reported as counting nothing.
var _ connectorapi.CountsLink = (*Session)(nil)

// This machine says what it has, so the host holds it to that and not to what
// this driver's code can do for a Klipper machine in general.
var _ connectorapi.ReportsProbe = (*Session)(nil)

// newSession wraps a connection that is open and has said nothing yet.
func newSession(
	conn *websocket.Conn, httpBaseURL string, timeout time.Duration, live liveness, log *slog.Logger,
) *Session {
	return &Session{
		conn:        conn,
		httpBaseURL: httpBaseURL,
		httpClient:  &http.Client{},
		timeout:     timeout,
		live:        live,
		log:         log,
		patches:     make(chan connectorapi.StatePatch, patchBacklog),
		done:        make(chan struct{}),
	}
}

// handshake performs the identity exchange and the live probe, and returns the
// capability set the machine's own answer narrowed the offer to.
//
// Every read here is made by the caller's goroutine. The reader goroutine has
// not started, so a frame that is not the reply being waited for is skipped
// rather than dispatched.
func (s *Session) handshake(ctx context.Context, offered connectorapi.Set) (connectorapi.Set, error) {
	if err := s.callSync(ctx, methodPrinterInfo, nil, &s.reported); err != nil {
		return nil, err
	}

	if s.reported.Hostname == "" {
		return nil, fmt.Errorf(
			"%w: printer.info named no hostname, so this machine has nothing to be called", ErrUnreachable)
	}

	var listed objectList
	if err := s.callSync(ctx, methodObjectsList, nil, &listed); err != nil {
		return nil, err
	}

	extruders := extrudersOf(listed.Objects)
	s.asked = append(present(subscribed, listed.Objects), furtherExtruders(extruders)...)

	dumped, err := s.readConfig(ctx, listed.Objects)
	if err != nil {
		return nil, err
	}

	heaters := heaterBounds(dumped)

	var opening statusReply
	if err = s.callSync(ctx, methodObjectsSubscribe, objectsParam(s.asked), &opening); err != nil {
		return nil, err
	}

	hasGcodes, gcodesWritable, err := s.readInitialStorages(ctx)
	if err != nil {
		return nil, err
	}

	s.reader.heads.count = len(extruders)
	s.first = append(toolPatches(dumped, extruders), buildVolumePatches(dumped)...)
	s.first = append(s.first, s.reader.patchesFrom(opening.Status, time.Now())...)

	s.probe = probe(listed.Objects, heaters, len(extruders), hasGcodes, gcodesWritable, offered)

	return offered.Intersect(s.probe), nil
}

// readConfig asks the machine for its own configuration: the bounds of its
// heaters, the nozzle of each extruder and the travel of each axis. A machine that reports no
// configfile object answers an empty configuration, and the ceiling this
// driver declares stands.
func (s *Session) readConfig(ctx context.Context, available []string) (configDump, error) {
	if !slices.Contains(available, objectConfigFile) {
		return configDump{}, nil
	}

	var reply statusReply

	params := map[string]any{"objects": map[string]any{objectConfigFile: []string{"settings"}}}
	if err := s.callSync(ctx, methodObjectsQuery, params, &reply); err != nil {
		return configDump{}, err
	}

	var dumped configDump
	if err := decodeResult(methodObjectsQuery, reply.Status[objectConfigFile], &dumped); err != nil {
		return configDump{}, err
	}

	return dumped, nil
}

// furtherExtruders is every extruder but the first, which subscribed already
// names.
func furtherExtruders(extruders []string) []string {
	return slices.DeleteFunc(slices.Clone(extruders), func(object string) bool { return object == objectExtruder })
}

// present is the objects this dialect wants that the machine reports, in the
// order this dialect names them.
func present(wanted, available []string) []string {
	out := make([]string, 0, len(wanted))

	for _, object := range wanted {
		if slices.Contains(available, object) {
			out = append(out, object)
		}
	}

	return out
}

// begin fixes what the session reports and starts the goroutines that own the
// connection from here on.
func (s *Session) begin(identity connectorapi.Identity, caps connectorapi.Set) {
	s.identity = identity
	s.caps = caps

	go s.read()
	go s.keepalive()
}

// Identity is what this machine is called.
func (s *Session) Identity() connectorapi.Identity { return s.identity }

// Capabilities is the resolved set and its epoch.
//
// The set is copied out, because the contract promises it does not move within
// an epoch and a caller handed the live map can move it.
//
// The epoch is 1 and never changes. This dialect does not renegotiate: a
// machine whose configuration changed is reached again as a new session.
func (s *Session) Capabilities() (connectorapi.Set, uint64) { return s.caps.Clone(), 1 }

// Probe is what this machine answered about itself, which is the third tier of
// the capability model. It is nil until the handshake has run.
//
// The keys it carries are the verbs the objects this machine reports can carry
// out. A verb it leaves out is one no object behind it exists for, so the
// machine would answer the call with an error.
func (s *Session) Probe() connectorapi.Set { return s.probe.Clone() }

// State is the addressable state tree's update channel. It closes when the
// connection ends, whichever end ended it.
func (s *Session) State() <-chan connectorapi.StatePatch { return s.patches }

// Dropped is how many state patches this session discarded because the host was
// not reading them.
func (s *Session) Dropped() uint64 { return s.dropped.Load() }

// Refused is how many frames the machine sent that this dialect could not read.
func (s *Session) Refused() uint64 { return s.refused.Load() }

// Close releases the session. It is safe to call more than once, which is what
// a host shutting down after an error needs.
//
// Closing the connection is what ends the reader, and the reader is what closes
// the patch channel. So there is one path out and no way for a second Close to
// close the channel twice.
func (s *Session) Close() error {
	s.hangUp()

	return nil
}

// hangUp drops the connection. It is separate from Close so that a handshake
// that failed lets go of the socket without promising anything about a patch
// channel no reader ever owned.
func (s *Session) hangUp() {
	s.closing.Do(func() { _ = s.conn.Close() })
}

// Do sends one command to the machine and reports what it made of it.
//
// The arguments were validated against Capabilities and the declared defaults
// filled in before this was reached, so nothing here re-checks a value this
// driver declared.
func (s *Session) Do(ctx context.Context, cmd connectorapi.Command) (connectorapi.Result, error) {
	// The host refuses a verb this machine does not have before it reaches
	// here: it reads Probe when it takes the session and holds the machine to
	// it. This is the last line rather than the first, and it is here because a
	// driver that answers for a verb nothing declared reports an outcome that
	// did not happen, and no caller can tell that apart from one that did.
	if !s.caps.Has(cmd.Verb) {
		return connectorapi.Result{}, connectorapi.CapabilityNotSupported.
			New("the printer does not have that capability").
			With(fault.StringParam("verb", cmd.Verb.String()))
	}

	method, params, err := s.callFor(cmd)
	if err != nil {
		return connectorapi.Result{}, err
	}

	result, refusal, err := s.call(ctx, method, params)
	if err != nil {
		return connectorapi.Result{}, err
	}

	if refusal != nil {
		return connectorapi.Result{OK: false, MachineReason: refusal.Message}, nil
	}

	return connectorapi.Result{OK: true, Raw: result}, nil
}

// callFor is the call one command becomes: a Moonraker method of its own, a
// status query, or a gcode line.
func (s *Session) callFor(cmd connectorapi.Command) (method string, params any, err error) {
	if cmd.Verb == VerbSendStateInfo {
		return methodObjectsQuery, objectsParam(s.asked), nil
	}

	if cmd.Verb == VerbStartPrint {
		rawPath := cmd.Args[pathArgument]
		if refusal := connectorapi.RefusePathEscape(rawPath); refusal != nil {
			return "", nil, refusal
		}

		rel, ok := relativeToGcodes(rawPath)
		if !ok {
			return "", nil, connectorapi.PathNotAllowed.
				New("start_print path must be on gcodes storage").
				With(fault.StringParam("path", rawPath))
		}

		return methodPrintStart, map[string]any{"filename": rel}, nil
	}

	if named, ok := methodFor(cmd.Verb); ok {
		return named, nil, nil
	}

	script, err := gcodeFor(cmd)
	if err != nil {
		return "", nil, err
	}

	return methodGcodeScript, map[string]any{"script": script}, nil
}

// call sends one request and waits for the machine's answer. refusal is the
// machine's own error, which is an answer and not a failure of this connection.
func (s *Session) call(
	ctx context.Context, method string, params any,
) (result json.RawMessage, refusal *rpcError, err error) {
	// Before the write and not only around the wait. A caller who has already
	// given up must not move the machine, and a pause the caller cancelled is
	// still a pause once it is on the wire.
	if err = ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("moonraker: %s: %w", method, err)
	}

	id := s.nextID.Add(1)
	replies := make(chan rpcFrame, 1)

	s.pending.Store(id, replies)
	defer s.pending.Delete(id)

	if err = s.write(rpcRequest{JSONRPC: jsonrpcVersion, Method: method, Params: params, ID: id}); err != nil {
		return nil, nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	select {
	case frame := <-replies:
		return frame.Result, frame.Error, nil
	case <-s.done:
		return nil, nil, fmt.Errorf("%w: %s was never answered", ErrSessionEnded, method)
	case <-callCtx.Done():
		return nil, nil, fmt.Errorf("moonraker: %s: %w", method, callCtx.Err())
	}
}

// callSync sends one request and reads the connection itself until the answer
// arrives. It is the handshake's call, and it is used before any goroutine owns
// the reads.
func (s *Session) callSync(ctx context.Context, method string, params any, into any) error {
	id := s.nextID.Add(1)

	if err := s.write(rpcRequest{JSONRPC: jsonrpcVersion, Method: method, Params: params, ID: id}); err != nil {
		return err
	}

	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > s.timeout {
		deadline = time.Now().Add(s.timeout)
	}

	if err := s.conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("%w: arm the read deadline: %w", ErrUnreachable, err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("moonraker: %s: %w", method, err)
		}

		var frame rpcFrame
		if err := s.conn.ReadJSON(&frame); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrUnreachable, method, err)
		}

		if frame.ID == nil || *frame.ID != id {
			continue
		}

		if frame.Error != nil {
			return fmt.Errorf("%w: %s: %s", ErrRefused, method, frame.Error.Message)
		}

		if into == nil {
			return nil
		}

		return decodeResult(method, frame.Result, into)
	}
}

// write puts one request on the wire. The mutex is what makes a command sent
// from one goroutine and a ping sent from another two whole frames.
func (s *Session) write(request rpcRequest) error {
	s.writing.Lock()
	defer s.writing.Unlock()

	if err := s.conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
		return fmt.Errorf("%w: arm the write deadline: %w", ErrUnreachable, err)
	}

	if err := s.conn.WriteJSON(request); err != nil {
		return fmt.Errorf("%w: send %s: %w", ErrUnreachable, request.Method, err)
	}

	return nil
}

// read owns every read once the session has begun. It is the only writer to the
// patch channel, which is what lets it be the only closer.
func (s *Session) read() {
	defer close(s.patches)
	defer close(s.done)
	defer s.hangUp()

	s.conn.SetPongHandler(func(string) error { return s.rearm() })

	if err := s.rearm(); err != nil {
		return
	}

	for _, patch := range s.first {
		s.report(patch)
	}

	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			s.log.Debug("the machine's connection ended", "error", err.Error())

			return
		}

		if err = s.rearm(); err != nil {
			return
		}

		s.dispatch(data)
	}
}

// rearm pushes the read deadline out to the full silence this dialect tolerates.
func (s *Session) rearm() error {
	if err := s.conn.SetReadDeadline(time.Now().Add(s.live.readDeadline)); err != nil {
		return fmt.Errorf("%w: arm the read deadline: %w", ErrUnreachable, err)
	}

	return nil
}

// dispatch acts on one frame: an answer to a call, a subscription update, or
// something this dialect has no reading of.
func (s *Session) dispatch(data []byte) {
	var frame rpcFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		s.refused.Add(1)

		return
	}

	if frame.ID != nil {
		s.answer(*frame.ID, frame)

		return
	}

	if frame.Method != notifyStatusUpdate {
		return
	}

	s.update(frame.Params)
}

// answer hands one reply to the call that is waiting for it. A reply nobody is
// waiting for is dropped: the caller gave up, and the channel it left behind is
// gone with it.
func (s *Session) answer(id uint64, frame rpcFrame) {
	waiting, found := s.pending.Load(id)
	if !found {
		return
	}

	replies, ok := waiting.(chan rpcFrame)
	if !ok {
		return
	}

	select {
	case replies <- frame:
	default:
	}
}

// update turns one subscription notification into patches. Its params are an
// array whose first element is the changed status.
func (s *Session) update(params json.RawMessage) {
	var carried []json.RawMessage
	if json.Unmarshal(params, &carried) != nil || len(carried) == 0 {
		s.refused.Add(1)

		return
	}

	var status map[string]json.RawMessage
	if json.Unmarshal(carried[0], &status) != nil {
		s.refused.Add(1)

		return
	}

	for _, patch := range s.reader.patchesFrom(status, time.Now()) {
		s.report(patch)
	}
}

// report queues one patch for the host. It never waits, because the goroutine
// that calls it is the one reading the connection: a send that waited would
// hold up the reply to a command as well, and every command behind it.
//
// A machine reporting faster than the host reads costs a fixed amount of
// memory, so the backlog gives way and not the process. The oldest patch is
// what goes, which is connectorapi.Session.State's rule: telemetry is a
// repeating snapshot, and a driver that dropped the newest would hold the
// host's tree at a stale reading for as long as the host stayed behind.
func (s *Session) report(patch connectorapi.StatePatch) {
	select {
	case s.patches <- patch:
		return
	case <-s.done:
		return
	default:
	}

	select {
	case <-s.patches:
		s.dropped.Add(1)
	default:
	}

	select {
	case s.patches <- patch:
	case <-s.done:
	default:
		s.dropped.Add(1)
	}
}

// keepalive asks the machine to prove the connection is still there, until the
// connection ends.
//
// Without it a machine that lost power holds a session that looks connected:
// the write into a blackholed socket does not fail for as long as the operating
// system keeps retrying, so nothing ends the session and nothing redials.
func (s *Session) keepalive() {
	ticker := time.NewTicker(s.live.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if err := s.ping(); err != nil {
				s.log.Debug("the machine would not take a ping, so its connection is gone",
					"error", err.Error())

				// read is what closes the channels, so ending the connection
				// here leaves exactly one path out.
				s.hangUp()

				return
			}
		}
	}
}

// ping writes one control frame. It takes the write lock because a control
// frame and a request must not interleave on one connection.
func (s *Session) ping() error {
	s.writing.Lock()
	defer s.writing.Unlock()

	if err := s.conn.WriteControl(
		websocket.PingMessage, nil, time.Now().Add(s.live.pingTimeout),
	); err != nil {
		return fmt.Errorf("%w: send a ping: %w", ErrUnreachable, err)
	}

	return nil
}
