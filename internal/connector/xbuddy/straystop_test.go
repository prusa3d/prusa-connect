package xbuddy

// When the stop for a transfer this host does not hold goes out, read off the
// frames a hostile printer receives.

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// strayBlockRequest is a block request for a transfer no Send is holding.
const strayBlockRequest = `{"transfer":"inline","file_id":4242,"chunk":4096,"start":0,"end":4095}`

// A transfer this host does not hold is stopped at once, whatever the machine
// is doing.
//
// Nothing here will ever serve that transfer, so a print of its file cannot
// finish either way. Buddy pauses such a print with a file warning: the next
// time the print reads ahead after a stop, or at the end of the fetched part
// without one.
func TestAStrayTransferIsStoppedInEveryMachineState(t *testing.T) {
	t.Parallel()

	for _, state := range []string{stateIdle, statePrinting, statePaused, stateAttention, stateBusy} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()

			printer := answer(t, standHostile(t))

			printer.report(t, state)
			printer.drain()

			printer.text(t, strayBlockRequest)

			if !printer.drain().has("STOP_TRANSFER") {
				t.Fatalf("a printer reporting %s was not told to stop a stray transfer", state)
			}
		})
	}
}

// A stray stop belongs to the connection the request came on.
//
// Buddy asks for a block once on each connection, with a file id it has not
// used before, so a new connection asks again for whatever it still fetches.
// A stop queued on the one before would land on a printer that may now fetch a
// file somebody sent it.
func TestAStrayStopEndsWithItsConnection(t *testing.T) {
	t.Parallel()

	t.Run("a new connection's request is stopped though it names the last one's id", func(t *testing.T) {
		t.Parallel()

		h := standHostile(t)
		printer := answer(t, h)

		printer.report(t, stateIdle)
		printer.drain()
		printer.text(t, strayBlockRequest)

		if !printer.drain().has("STOP_TRANSFER") {
			t.Fatal("a stray request was not stopped")
		}

		_ = h.client.Close()
		h.awaitDeparture(t)
		h.redial(t)
		printer = answer(t, h)

		printer.report(t, stateIdle)
		printer.drain()
		printer.text(t, strayBlockRequest)

		if !printer.drain().has("STOP_TRANSFER") {
			t.Fatal("a stray request on a new connection was not stopped, because the last one named the same id")
		}
	})

	t.Run("a stop queued on a connection that ended is not sent on the next", func(t *testing.T) {
		t.Parallel()

		h := standHostile(t)
		printer := listen(t, h)

		printer.text(t, `{"state":"IDLE"}`)

		// A command the printer never answers holds the queue, so the stop
		// waits behind it.
		beep := h.start(t)

		if frame := printer.next(t); !strings.Contains(frame, "BEEP") && !strings.Contains(frame, "M300") {
			t.Fatalf("the driver sent %q, want the command that holds the queue", frame)
		}

		printer.text(t, strayBlockRequest)
		h.awaitQueued(t, 1)

		_ = h.client.Close()
		h.awaitDeparture(t)

		if _, err := h.await(t, beep); err == nil {
			t.Fatal("the command whose connection ended was reported answered")
		}

		h.redial(t)

		if frames := answer(t, h).drain(); frames.has("STOP_TRANSFER") {
			t.Fatalf("the driver sent %q: a stop for the last connection went out on this one", frames)
		}
	})
}

// sentFrames is what the driver wrote to the printer.
type sentFrames []string

func (f sentFrames) has(command string) bool {
	for _, frame := range f {
		if strings.Contains(frame, command) {
			return true
		}
	}

	return false
}

// answering is a hostile printer that answers every command with FINISHED in
// the machine state the case last reported, so an answer does not move the
// state the case set. One made by listen answers only what the case replies to.
//
// One goroutine reads, because a websocket read that timed out leaves the
// connection unusable, and writes share a lock with it.
type answering struct {
	client *websocket.Conn
	auto   bool
	mu     sync.Mutex
	state  string
	frames chan string
}

func answer(t *testing.T, h *hostile) *answering {
	t.Helper()

	return startAnswering(h, true)
}

func listen(t *testing.T, h *hostile) *answering {
	t.Helper()

	return startAnswering(h, false)
}

func startAnswering(h *hostile, auto bool) *answering {
	a := &answering{client: h.client, auto: auto, frames: make(chan string, 64)}
	client := a.client

	go func() {
		for {
			_, data, err := client.ReadMessage()
			if err != nil {
				return
			}

			a.frames <- string(data)

			if a.auto {
				a.answer(data)
			}
		}
	}()

	return a
}

// answer sends FINISHED for the command frame carries.
func (a *answering) answer(frame []byte) {
	if len(frame) < hostileHeader {
		return
	}

	id, err := strconv.ParseUint(string(frame[1:hostileHeader]), 16, 32)
	if err != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	reply := fmt.Sprintf(`{"event":"FINISHED","command_id":%d,"state":%q}`, id, a.state)
	// A write that fails is a connection the case has closed.
	_ = a.client.WriteMessage(websocket.TextMessage, []byte(reply))
}

// next takes the next frame the driver sends.
func (a *answering) next(t *testing.T) string {
	t.Helper()

	select {
	case frame := <-a.frames:
		return frame
	case <-time.After(hostileBudget):
		t.Fatal("the driver sent the printer nothing")

		return ""
	}
}

// report writes a telemetry frame carrying state.
func (a *answering) report(t *testing.T, state string) {
	t.Helper()

	a.mu.Lock()
	a.state = state
	a.mu.Unlock()

	a.text(t, fmt.Sprintf(`{"state":%q}`, state))
}

func (a *answering) text(t *testing.T, frame string) {
	t.Helper()

	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.client.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
		t.Fatalf("write %q as the printer: %v", frame, err)
	}
}

// drain takes every frame the driver sends until it goes quiet.
func (a *answering) drain() sentFrames {
	var sent sentFrames

	for {
		select {
		case frame := <-a.frames:
			sent = append(sent, frame)
		case <-time.After(quietWindow):
			return sent
		}
	}
}
