package xbuddy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The websocket link under a session: the connection itself, the lifecycle that
// attaches and gives one up, and the writers every frame leaves through.
//
// A session outlives its connections. It is the identity, the capability set
// and the command queue, and it survives a printer that drops and dials again;
// a link is one TCP connection and is replaced. Keeping the two apart is what
// makes that difference readable, because the methods here are the only ones
// whose receiver dies at a reconnect.

// link is one websocket connection under a session.
type link struct {
	conn *websocket.Conn
	// gone is closed once this connection is finished, so a command waiting for
	// an answer on it stops waiting.
	gone chan struct{}
	once sync.Once
	// stateIn is closed once the printer has reported its state on this
	// connection. See Session.awaitState.
	stateIn   chan struct{}
	stateOnce sync.Once
	// writeMu serialises writes. gorilla permits one concurrent writer, and the
	// pump is not the only one: a pong answers a ping from the read goroutine.
	writeMu sync.Mutex
	// refused is how many frames this connection sent that the dialect could
	// not read. It is per connection rather than per session because it is what
	// bounds the log: see Session.refuse.
	refused atomic.Uint64
}

// waitLink blocks until a connection is live. ok is false when the session
// closed first, which is what ends the pump goroutine.
func (s *Session) waitLink() (*link, bool) {
	for {
		s.mu.Lock()
		current, ready := s.current, s.ready
		s.mu.Unlock()

		if current != nil {
			return current, true
		}

		select {
		case <-ready:
		case <-s.done:
			return nil, false
		}
	}
}

// attach adopts a freshly upgraded connection and starts reading it. It reports
// false when the session has already closed, and the caller hangs up.
func (s *Session) attach(ctx context.Context, conn *websocket.Conn) bool {
	current := &link{conn: conn, gone: make(chan struct{}), stateIn: make(chan struct{})}

	// Nothing here allocates in proportion to what a printer sends. The largest
	// legitimate frame is an INFO event listing storage devices, which is
	// kilobytes.
	conn.SetReadLimit(maxFrameBytes)

	// Every frame the printer sends says it is still there, and a pong is the
	// frame a printer with nothing to report sends. A ping counts as well: the
	// firmware pings after 15s of sending nothing (src/connect/connect.cpp).
	// Its pong alone is not enough, because the firmware reads its socket only
	// when its planner lets it. A pong later than ReadDeadline less PingInterval
	// would otherwise drop a printer that is sending telemetry the whole time.
	conn.SetPingHandler(func(payload string) error {
		if err := s.rearm(current); err != nil {
			return err
		}

		return current.pong(payload)
	})
	conn.SetPongHandler(func(string) error { return s.rearm(current) })

	if err := s.rearm(current); err != nil {
		s.logWarn(ctx, "the printer's connection would not take a read deadline, so a printer that "+
			"goes away silently will not be noticed on it", slog.String("error", err.Error()))
	}

	s.mu.Lock()

	if s.closed() {
		s.mu.Unlock()

		return false
	}

	previous := s.current
	s.current = current
	// The printer on this connection may run other firmware than the last one,
	// after an update, so its tool numbering is not known until its INFO.
	s.toolBase.Store(int32(toolBaseUnknown))
	// What the last connection reported is kept, and is not taken as this
	// one's answer: a printer can come back with a stick pulled.
	s.storagesReported = false
	s.stateHeard.Store(false)

	// A printer that dials again before its first socket failed reaches here
	// with ready already closed, and closing a closed channel panics.
	select {
	case <-s.ready:
	default:
		close(s.ready)
	}

	if s.grace != nil {
		s.grace.Stop()
	}

	s.mu.Unlock()

	s.noteChange()

	// A printer that dials twice without the first socket failing has two
	// connections open, and the older one is the one nothing will answer on.
	if previous != nil {
		previous.close()
	}

	go s.read(ctx, current)
	go s.keepalive(ctx, current)
	go s.requestInfo(ctx, current)

	return true
}

// requestInfo asks the printer what it is and what storages it has.
//
// Buddy firmware says nothing about its storages until it is asked, so without
// this a session knows no mountpoint but the configured default and can report
// no file tree at all. The shipping connector does the same, on the same delay
// (legacy/connector/xbuddy/connection.go:42 and :165-181).
//
// It ends when the printer answers, when the connection goes, when the session
// closes, or at infoRequestTimeout.
func (s *Session) requestInfo(ctx context.Context, current *link) {
	if !s.caps.Has(connectorapi.VerbSendInfo) {
		return
	}

	timer := time.NewTimer(infoDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-current.gone:
		return
	case <-s.done:
		return
	}

	// The request context ends when the handshake response is written, and this
	// outlives it: what bounds the call is the session, the connection and this
	// deadline.
	if err := s.askStorages(context.WithoutCancel(ctx), infoRequestTimeout); err != nil {
		s.logDebug(ctx, "the printer was not asked what storages it has",
			slog.String("error", err.Error()))
	}
}

// AskStorages sends SEND_INFO and waits for the answer, for at most
// askStoragesTimeout. The INFO that answers it is observed before the command
// settles (Session.dispatchEvent), so the storages it carries are held by the
// time this returns.
//
// A session whose resolved set does not carry the verb, and one with no
// connection to ask over, are not asked and answer nil: what the printer last
// said is all there is to read. A command offered to an absent printer waits
// in the queue for it, and an ask nobody is waiting for any more would sit
// there taking a slot a person's own command needs.
func (s *Session) AskStorages(ctx context.Context) error {
	return s.askStorages(ctx, askStoragesTimeout)
}

// storagesAsk is one SEND_INFO in flight. Every ask that arrives while it runs
// waits on it, because the printer runs one command at a time and a second
// SEND_INFO would only queue behind the first to hear the same answer.
type storagesAsk struct {
	done chan struct{}
	err  error
}

// askStorages joins the ask in flight or starts one bounded by bound. The
// flight outlives a caller that stops waiting, so the others still hear it.
func (s *Session) askStorages(ctx context.Context, bound time.Duration) error {
	if !s.caps.Has(connectorapi.VerbSendInfo) {
		return nil
	}

	s.mu.Lock()

	if s.current == nil {
		s.mu.Unlock()

		return nil
	}

	ask := s.asking
	if ask == nil {
		ask = &storagesAsk{done: make(chan struct{})}
		s.asking = ask

		go s.flyAsk(context.WithoutCancel(ctx), ask, bound)
	}

	s.mu.Unlock()

	select {
	case <-ask.done:
		return ask.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) flyAsk(ctx context.Context, ask *storagesAsk, bound time.Duration) {
	asked, cancel := context.WithTimeout(ctx, bound)
	defer cancel()

	_, ask.err = s.Do(asked, connectorapi.Command{Verb: connectorapi.VerbSendInfo})

	s.mu.Lock()
	s.asking = nil
	s.mu.Unlock()

	close(ask.done)
}

// rearm pushes the read deadline out to the full silence this dialect tolerates.
// It runs once on attach and again on every frame the printer sends: a data
// frame, a ping or a pong.
func (s *Session) rearm(current *link) error {
	if err := current.conn.SetReadDeadline(time.Now().Add(s.live.ReadDeadline)); err != nil {
		return fmt.Errorf("arm the read deadline: %w", err)
	}

	return nil
}

// readMessage reads one whole message and re-arms the read deadline whenever a
// part of it arrives.
//
// The firmware sends a long answer as one message in many fragments, and it
// renders each fragment as it reads the storage: a FILE_INFO for a folder of 300
// files on a USB stick takes half a minute or more (src/connect/connect.cpp,
// send_command). Nothing else reaches the host while it does, because websocket
// cannot put a frame inside another message. Re-arming only once a whole message
// is read ends the connection while the printer is still sending the answer.
func (s *Session) readMessage(current *link) (int, []byte, error) {
	kind, reader, err := current.conn.NextReader()
	if err != nil {
		return kind, nil, fmt.Errorf("read the next message: %w", err)
	}

	data, err := io.ReadAll(&rearmingReader{reader: reader, rearm: func() error { return s.rearm(current) }})
	if err != nil {
		return kind, nil, fmt.Errorf("read a message: %w", err)
	}

	return kind, data, nil
}

// rearmingReader re-arms a read deadline on every read that brings bytes.
type rearmingReader struct {
	reader io.Reader
	rearm  func() error
}

func (r *rearmingReader) Read(buffer []byte) (int, error) {
	read, err := r.reader.Read(buffer)
	if read > 0 {
		if rearmErr := r.rearm(); rearmErr != nil {
			return read, rearmErr
		}
	}

	return read, err //nolint:wrapcheck // io.EOF has to reach io.ReadAll as it is.
}

// keepalive asks the printer to prove the connection is still there, until the
// connection ends or the session does.
//
// Without it a printer that lost power holds a session that looks connected:
// waitLink hands the pump a dead link, the write into it does not fail for as
// long as the operating system keeps retrying, and the grace timer that ends an
// absence never starts. Commands are then lost rather than queued for the
// printer's return, which is the property earlier research records as the
// one most likely to go missing in a rewrite.
func (s *Session) keepalive(ctx context.Context, current *link) {
	ticker := time.NewTicker(s.live.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-current.gone:
			return
		case <-s.done:
			return
		case <-ticker.C:
			if err := current.ping(s.live.PingTimeout); err != nil {
				s.logDebug(ctx, "the printer would not take a ping, so its connection is gone",
					slog.String("error", err.Error()))

				// read is what detaches: closing here ends its ReadMessage, and
				// two paths calling detach for one connection is how a session
				// loses a socket that has already been replaced.
				current.close()

				return
			}
		}
	}
}

// detach drops a connection, leaving the session open so the queue survives the
// absence.
func (s *Session) detach(current *link) {
	s.mu.Lock()

	went := s.current == current
	if went {
		s.current = nil
		s.ready = make(chan struct{})

		if s.grace != nil {
			s.grace.Reset(s.graceDelay)
		}
	}

	s.mu.Unlock()

	// Told the moment the socket goes and not when the grace runs out. The grace
	// ends the session, which is a different fact and already has an event: an
	// absence is what somebody at a bench watching a cable come out is waiting
	// for, and the default grace of zero never ends a session at all.
	if went {
		s.noteChange()
	}

	current.close()
}

// write sends one server-to-printer frame.
func (l *link) write(frame []byte) error {
	if err := l.send(websocket.TextMessage, frame); err != nil {
		return fmt.Errorf("write a frame: %w", err)
	}

	return nil
}

// writeBinary sends one frame that is not text. The transfer chunk is the only
// one this dialect writes, and it is binary because the payload is a slice of a
// file rather than anything that survives being read as UTF-8.
func (l *link) writeBinary(frame []byte) error {
	if err := l.send(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("write a binary frame: %w", err)
	}

	return nil
}

// send puts one frame on the wire under a deadline, through the lock every write
// takes.
//
// The deadline is what a socket that stopped draining without resetting costs. A
// transfer writes thousands of frames through here, and a write with no bound
// parks inside gorilla holding writeMu until the read deadline notices sixty
// seconds later, with the ping that would have noticed queued behind it. Every
// write sets its own, because a deadline left armed from an earlier frame is one
// the next frame fails on.
func (l *link) send(kind int, frame []byte) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	if err := l.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return fmt.Errorf("arm the write deadline: %w", err)
	}

	if err := l.conn.WriteMessage(kind, frame); err != nil {
		return fmt.Errorf("write %d bytes: %w", len(frame), err)
	}

	return nil
}

// ping asks the printer to say something. The pong that comes back re-arms the
// read deadline, as every other frame from the printer does.
func (l *link) ping(timeout time.Duration) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	if err := l.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("ping the printer: %w", err)
	}

	return nil
}

// pong answers a ping through the same lock every other write takes.
func (l *link) pong(payload string) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	if err := l.conn.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(pongDeadline)); err != nil {
		return fmt.Errorf("answer a ping: %w", err)
	}

	return nil
}

func (l *link) close() {
	l.once.Do(func() {
		close(l.gone)
		_ = l.conn.Close()
	})
}
