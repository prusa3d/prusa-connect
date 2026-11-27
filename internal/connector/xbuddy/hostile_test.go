package xbuddy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The other direction of TestTheFramingRefusesEveryMalformedHeader
// (internal/printer/simulator/wire_internal_test.go).
//
// That test proves the simulated printer refuses a bad frame from the host.
// This one drives a bad frame into the driver, which is the direction nothing
// on our side constrains: a printer on a bench runs firmware this build does
// not control, and what it puts on the socket is the one input to this system
// that no amount of care here bounds.
//
// The rule this file holds the driver to, in one sentence: an inbound frame
// this dialect cannot read is dropped, the connection is kept, the drop is
// counted, and the frames after it are served as though it never arrived. The
// argument for dropping the frame rather than the connection is on
// Session.refuse; the one case that cannot obey it is the frame above the read
// limit, which the websocket layer fails before the driver sees a byte, and
// TestAFrameAboveTheReadLimitEndsTheConnectionAndKeepsTheSession is what says
// the session still survives that.

// The header the printer never writes and the driver always does. It is decoded
// here rather than through the driver's own codec, for the reason
// internal/printer/simulator/wire.go states: a shared codec makes an encoder bug
// agree with itself on both sides and the test passes anyway.
const hostileHeader = 9

// hostileBudget bounds every wait in this file. It is real time on a real
// socket, and its only job is to turn a wedge into a failure instead of a hang.
const hostileBudget = 10 * time.Second

// hostileSilence is how long a connection in this file may say nothing before
// liveness ends it, and it is far above hostileBudget on purpose.
//
// The oversized case asserts that a connection went. A read deadline anywhere
// near the budget would end it too, and the assertion would then pass against a
// session with no read limit at all: watched, with the limit removed and a
// deadline of hostileBudget, the test still passed after 12.6s. Nothing here
// waits on silence, so the deadline is set where it cannot be the cause.
const hostileSilence = 5 * time.Minute

// departureBudget is how long the oversized case gives the session to notice
// the connection went. The websocket layer fails that read as it happens, so
// anything above a second is slack, and it stays well below hostileSilence so
// that a departure inside it can only be the read limit.
const departureBudget = 2 * time.Second

// badFrame is one thing a printer can put on the wire that this dialect has no
// reading of.
//
// The inventory is the simulator's, carried across the direction change. The
// two directions do not share a framing - server to printer is a nine-byte
// header and printer to server is a bare JSON object - so each of the
// simulator's header cases is listed here against the inbound shape that fails
// the same way, and the cases with no header analogue are the ones the
// transport itself contributes.
type badFrame struct {
	// name is the subtest name.
	name string
	// mirrors is the case in TestTheFramingRefusesEveryMalformedHeader this one
	// is the inbound half of, or what it covers where that test has no analogue.
	mirrors string
	// binary sends the frame as a websocket binary message.
	binary bool
	// frame is what the printer writes.
	frame []byte
}

//nolint:gochecknoglobals // the inventory is data, read-only after init.
var badFrames = []badFrame{
	{
		name:    "an empty frame",
		mirrors: `the simulator's "empty"`,
		frame:   []byte(``),
	},
	{
		name:    "a frame that stops in the middle",
		mirrors: `the simulator's "header one byte short"`,
		frame:   []byte(`{"event":"FINISHED","command_id":`),
	},
	{
		name:    "a JSON array where an object belongs",
		mirrors: `the simulator's "an undefined type letter": a shape the dialect does not define`,
		frame:   []byte(`[{"event":"FINISHED"}]`),
	},
	{
		name:    "a JSON string where an object belongs",
		mirrors: `the simulator's "an undefined type letter"`,
		frame:   []byte(`"FINISHED"`),
	},
	{
		name:    "a bare number where an object belongs",
		mirrors: `the simulator's "an undefined type letter"`,
		frame:   []byte(`42`),
	},
	{
		name:    "an event key that is not a string",
		mirrors: `the simulator's "a lower-case type letter": the right key, the wrong spelling`,
		frame:   []byte(`{"event":123}`),
	},
	{
		name:    "a command id that is not a number",
		mirrors: `the simulator's "an id that is not hex"`,
		frame:   []byte(`{"event":"FINISHED","command_id":"1"}`),
	},
	{
		name:    "a command id with a fraction",
		mirrors: `the simulator's "an id in decimal padding"`,
		frame:   []byte(`{"event":"FINISHED","command_id":1.5}`),
	},
	{
		name:    "a frame carrying a NUL byte",
		mirrors: "no header analogue: text this dialect never writes",
		frame:   []byte{'{', '"', 'a', 0x00, '"', ':', '1', '}'},
	},
	{
		name:    "a frame that opens with bytes that are not UTF-8",
		mirrors: "no header analogue: a payload no JSON decoder accepts at all",
		frame:   []byte{0xff, 0xfe, '{', '}'},
	},
	{
		name:    "a well-formed event sent as binary",
		mirrors: "no header analogue: the frame type is the whole of what is wrong",
		binary:  true,
		frame:   []byte(`{"event":"FINISHED","command_id":1}`),
	},
}

// TestAMalformedFrameIsDroppedAndTheSessionServesTheNextOne is the whole rule.
//
// Every case runs against a session with a command already on the wire, because
// that is what separates the two possible behaviours: a driver that hung up on
// the bad frame fails the command in flight with ErrPrinterWentAway, and a
// driver that dropped the frame answers it. Asserting only that nothing panicked
// would pass against a driver that swallowed the frame and wedged.
func TestAMalformedFrameIsDroppedAndTheSessionServesTheNextOne(t *testing.T) {
	t.Parallel()

	for _, testCase := range badFrames {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			printer := standHostile(t)

			answered := printer.start(t)
			id := printer.takeFrame(t)

			before := printer.session.Refused()
			printer.send(t, testCase)

			// The reader serves one frame at a time in one goroutine, so an
			// answer that has arrived is proof the frame before it was dealt
			// with. Nothing here polls.
			printer.answer(t, id)

			result, err := printer.await(t, answered)
			if err != nil {
				t.Fatalf("the command in flight when the printer sent %s failed: %v", testCase.mirrors, err)
			}

			if !result.OK {
				t.Errorf("the command in flight came back refused: %s", result.Raw)
			}

			if got := printer.session.Refused(); got != before+1 {
				t.Errorf("the session refused %d frames and %d were refused before this one, so the "+
					"frame was swallowed rather than dropped", got, before)
			}

			if !printer.session.Connected() {
				t.Error("the printer's connection went, and this dialect drops the frame and keeps the socket")
			}

			printer.expectTelemetry(t)

			t.Logf("observed: %s was refused, the command in flight completed, and telemetry after it "+
				"reached the state tree (mirrors %s)", testCase.name, testCase.mirrors)
		})
	}
}

// TestAMalformedFrameNeverAnswersACommand is the half the round trip cannot
// see: a bad frame that carries a plausible command id must not settle the
// command waiting under it.
//
// A driver that matched an event to a command before reading whether the id is
// usable would report a command finished on the strength of a frame it could
// not read, which is worse than dropping it: the caller is told the printer
// carried out something nobody knows it received.
func TestAMalformedFrameNeverAnswersACommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// spell renders the frame from the id of the command in flight, so
		// every case names the one command that is actually waiting.
		spell func(id uint32) string
	}{
		{
			name:  "an id below zero",
			spell: func(id uint32) string { return fmt.Sprintf(`{"event":"FINISHED","command_id":-%d}`, id) },
		},
		{
			name:  "an id that is not a number",
			spell: func(id uint32) string { return fmt.Sprintf(`{"event":"FINISHED","command_id":"%d"}`, id) },
		},
		{
			name: "an id above what the framing carries",
			spell: func(id uint32) string {
				return fmt.Sprintf(`{"event":"FINISHED","command_id":%d}`, uint64(id)+1<<32)
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			printer := standHostile(t)

			answered := printer.start(t)
			id := printer.takeFrame(t)

			frame := testCase.spell(id)
			printer.text(t, frame)

			// A second frame the reader must get to. Its arrival is what proves
			// the first was processed, so the check below is not a race with a
			// frame still in the pipe.
			printer.text(t, `{"temp_bed":60}`)

			select {
			case settled := <-answered:
				t.Fatalf("a frame carrying an unusable id settled the command: %+v %v",
					settled.result, settled.err)
			case <-time.After(250 * time.Millisecond):
			}

			printer.answer(t, id)

			result, err := printer.await(t, answered)
			if err != nil {
				t.Fatalf("the command never completed after the real answer: %v", err)
			}

			if !result.OK {
				t.Errorf("the command came back refused: %s", result.Raw)
			}

			// The drop is counted whichever route it took. An id spelled as a
			// string fails classify and an id out of range fails the range
			// check, and an operator reading the counter cannot be expected to
			// know which of the two a firmware chose.
			if got := printer.session.Refused(); got != 1 {
				t.Errorf("%q was dropped and the session reports %d refused frames, so the number an "+
					"operator reads is not the number of frames refused", frame, got)
			}

			t.Logf("observed: %q left the command in flight untouched, was counted refused, and the "+
				"real answer settled it", frame)
		})
	}
}

// TestAFrameTheDriverCannotMatchLeavesNoTrace is the other half of
// TestAMalformedFrameNeverAnswersACommand.
//
// That test proves a frame carrying an unusable id does not settle the command
// waiting under it. This one proves it does not settle the *next* one either:
// an event is read for what it says about the printer before it is matched to a
// command, so a frame the driver refuses could rewrite the storages and plant a
// directory listing on its way to being refused. A printer is a device on a
// network this build does not control, so the planted entry is a file a user
// then sees in a listing no answer to their request ever carried.
func TestAFrameTheDriverCannotMatchLeavesNoTrace(t *testing.T) {
	t.Parallel()

	t.Run("an INFO carrying an unusable id does not rewrite the storages", func(t *testing.T) {
		t.Parallel()

		printer := standHostile(t)

		answered := printer.start(t)
		id := printer.takeFrame(t)

		printer.text(t, `{"event":"INFO","data":{"storages":[`+
			`{"mountpoint":"/local","type":"LOCAL","free_space":512}]}}`)
		printer.answer(t, id)

		if _, err := printer.await(t, answered); err != nil {
			t.Fatalf("the command in flight failed: %v", err)
		}

		before := printer.storages(t)
		if len(before) != 1 || before[0].Mountpoint != "/local" {
			t.Fatalf("the printer's own INFO did not reach the session: %+v", before)
		}

		printer.text(t, `{"event":"INFO","state":"IDLE","command_id":-1,"data":{"storages":[`+
			`{"mountpoint":"/hijacked","type":"LOCAL","free_space":1},`+
			`{"mountpoint":"/hijacked2","type":"USB","free_space":2}]}}`)

		// Telemetry after it is what says the frame was processed, so the read
		// below is not a race with a frame still in the pipe.
		printer.expectTelemetry(t)

		after := printer.storages(t)
		if !slices.Equal(after, before) {
			t.Errorf("the storages went from %+v to %+v on a frame the driver refused to match",
				before, after)
		}

		if got := printer.session.Refused(); got != 1 {
			t.Errorf("the session refused %d frames, and a command id nothing can be matched to is one", got)
		}

		t.Logf("observed: the storages read %+v after the frame, and %d frames were refused",
			after, printer.session.Refused())
	})

	t.Run("a FILE_INFO carrying an unusable id does not answer a later listing", func(t *testing.T) {
		t.Parallel()

		printer := standHostile(t)

		printer.text(t, `{"event":"FILE_INFO","command_id":-1,"data":{"path":"/usb/deep",`+
			`"name":"deep","type":"FOLDER","children":[`+
			`{"path":"/usb/deep/PLANTED.gco","name":"PLANTED.gco","display_name":"PLANTED.gco"}]}}`)
		printer.expectTelemetry(t)

		listed := printer.list(t, "/usb/deep")

		id := printer.takeFrame(t)

		// A bare terminal event: the real answer to the caller's request carries
		// no listing at all, which is the case the recall fallback exists for.
		printer.answer(t, id)

		result := printer.awaitListing(t, listed)
		if result.err == nil {
			t.Fatalf("List(/usb/deep) returned directory %q with %d entries, and no answer to the "+
				"caller's request carried one: %+v",
				result.listing.Directory.Path, len(result.listing.Entries), result.listing.Entries)
		}

		if !errors.Is(result.err, ErrNoListing) {
			t.Errorf("List failed with %v, and a request nothing answered is %v", result.err, ErrNoListing)
		}

		if got := printer.session.Refused(); got != 1 {
			t.Errorf("the session refused %d frames, and a command id nothing can be matched to is one", got)
		}

		t.Logf("observed: the planted listing never reached a caller (%v)", result.err)
	})
}

// TestAFrameThisDialectCarriesNothingOfIsNotRefused keeps the refusal counter
// meaning what it says.
//
// A firmware this build does not control sends keys this build has never seen,
// and that is the ordinary case rather than the malformed one: the frame parses,
// nothing in it maps to the state tree, and it is carried and not refused. A
// counter that also counted these would rise on every healthy printer and tell
// an operator nothing.
//
// The second case is the one that established the boundary by execution: a
// string carrying bytes that are not UTF-8 is a websocket protocol violation
// that gorilla does not enforce and that encoding/json accepts into a
// json.RawMessage, so the frame arrives readable. It is recorded here because a
// reader would otherwise expect it in the refused inventory above.
func TestAFrameThisDialectCarriesNothingOfIsNotRefused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		frame []byte
	}{
		{name: "a telemetry key no version of this driver maps", frame: []byte(`{"temp_heatbreak":41}`)},
		{
			name:  "a string carrying bytes that are not UTF-8",
			frame: []byte{'{', '"', 'a', '"', ':', '"', 0xff, 0xfe, '"', '}'},
		},
		{name: "an empty object", frame: []byte(`{}`)},
		{name: "a JSON null", frame: []byte(`null`)},
		{
			// A block request this host has nothing for. The printer retries it,
			// so a counter that rose on it would rise through a healthy transfer
			// and say nothing.
			name: "a file block request for a transfer this session is not running",
			frame: []byte(`{"transfer":"inline","file_id":4242,"chunk":4096,"start":0,"end":4095,` +
				`"hash":"not-a-transfer-this-host-minted","team_id":0}`),
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			printer := standHostile(t)

			answered := printer.start(t)
			id := printer.takeFrame(t)

			printer.send(t, badFrame{frame: testCase.frame})
			printer.answer(t, id)

			if _, err := printer.await(t, answered); err != nil {
				t.Fatalf("the command in flight failed: %v", err)
			}

			if got := printer.session.Refused(); got != 0 {
				t.Errorf("%q was counted as refused %d times, and it is a frame this dialect read and "+
					"had nothing to do with", testCase.frame, got)
			}

			printer.expectTelemetry(t)

			t.Logf("observed: %q was carried, not refused, and the state tree kept being fed",
				testCase.frame)
		})
	}
}

// A block request whose numbers this wire cannot express is counted.
//
// It is the other side of the case above: the frame classified as a block
// request, and then nothing in it could be read as a range. That is a firmware
// spelling the range differently, which is exactly what the counter is for, and
// it cannot rise on a healthy transfer because a printer that retries a range
// this dialect reads is a different frame.
func TestABlockRequestNamingAnUnreadableRangeIsRefused(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		frame string
	}{
		{name: "no numbers at all", frame: `{"transfer":"inline","offset":0,"size":4096}`},
		{
			name:  "a chunk this host will not buffer",
			frame: `{"transfer":"inline","file_id":1,"chunk":33554432,"start":0,"end":4095}`,
		},
		{
			name:  "a file id past the header's uint32",
			frame: `{"transfer":"inline","file_id":4294967296,"chunk":4096,"start":0,"end":4095}`,
		},
		{
			name:  "an end past the uint32 the wire is shaped by",
			frame: `{"transfer":"inline","file_id":1,"chunk":4096,"start":0,"end":9223372036854775807}`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			printer := standHostile(t)

			answered := printer.start(t)
			id := printer.takeFrame(t)

			printer.text(t, testCase.frame)
			printer.answer(t, id)

			if _, err := printer.await(t, answered); err != nil {
				t.Fatalf("the command in flight failed: %v", err)
			}

			printer.expectTelemetry(t)

			if got := printer.session.Refused(); got != 1 {
				t.Errorf("%q was dropped and the session reports %d refused frames, and a range this "+
					"dialect cannot read is one", testCase.frame, got)
			}

			t.Logf("observed: %q was counted refused and the session kept working", testCase.frame)
		})
	}
}

// refusalStorm is how many malformed frames the rate case sends. It is far
// above any bound a reasonable policy would put on the log and small enough to
// run on every commit: one developer workstation refuses 20,000 of these in
// 141 ms, and a log with no rate rule records one line for each.
const refusalStorm = 2000

// recordedRefusals is how many log records a connection may write about frames
// it refused: the first one, naming what arrived, and a total when the
// connection ends.
const recordedRefusals = 2

// TestARefusedFrameDoesNotWriteOneRecordPerFrame holds the cost of keeping the
// connection.
//
// Dropping the frame instead of the connection is the right trade and it moved
// the bound: hanging up capped the log at one record per printer, and dropping
// caps it at nothing. A printer runs firmware this build does not control, so a
// dialect this build cannot read is a printer writing to an operator's disk at
// line rate for as long as it stays connected. The policy is Session.refuse's:
// the first refusal on a connection is logged and the rest are counted, with
// the total written once when the connection ends.
func TestARefusedFrameDoesNotWriteOneRecordPerFrame(t *testing.T) {
	t.Parallel()

	records := &countingHandler{}
	printer := standHostileLogging(t, slog.New(records))

	for range refusalStorm {
		printer.text(t, `x`)
	}

	// A well-formed frame after the storm, whose arrival says every frame
	// before it was dealt with.
	printer.expectTelemetry(t)

	if got := printer.session.Refused(); got != refusalStorm {
		t.Fatalf("the session refused %d frames and %d were sent, so this is not measuring the storm",
			got, refusalStorm)
	}

	written := records.count()
	if written > recordedRefusals-1 {
		t.Errorf("%d malformed frames wrote %d log records while the connection was up, and the policy "+
			"is one: a printer whose firmware this build cannot read would otherwise fill a disk",
			refusalStorm, written)
	}

	// Ending the connection is what publishes the total, and it is the second
	// and last record.
	if err := printer.client.Close(); err != nil {
		t.Fatalf("close the printer's connection: %v", err)
	}

	printer.awaitDeparture(t)

	if written = records.count(); written > recordedRefusals {
		t.Errorf("%d malformed frames wrote %d log records across the whole connection, and the policy "+
			"is %d", refusalStorm, written, recordedRefusals)
	}

	if total := records.lastRefusalTotal(); total != refusalStorm {
		t.Errorf("the connection reported %d refused frames as it ended, and %d were refused",
			total, refusalStorm)
	}

	t.Logf("observed: %d refused frames wrote %d log records, the last one carrying the total",
		refusalStorm, written)
}

// A CORE One reconnected every four seconds during a transfer, and a log at
// INFO named each admission and never why the connection before it ended
// (CL-296). The reason is what tells a printer that hung up from a server
// that closed the connection.
func TestAnEndedConnectionIsLoggedWithItsReason(t *testing.T) {
	t.Parallel()

	out := &lines{}
	printer := standHostileLogging(t, slog.New(slog.NewJSONHandler(out, nil)))

	if err := printer.client.Close(); err != nil {
		t.Fatalf("close the printer's connection: %v", err)
	}

	printer.awaitDeparture(t)

	for _, record := range out.records(t) {
		if record["msg"] != "the printer's connection ended" {
			continue
		}

		if reason, _ := record["error"].(string); reason == "" {
			t.Errorf("the record names no reason: %v", record)
		}

		return
	}

	t.Error("a log at INFO does not say the printer's connection ended")
}

// countingHandler counts the records a session writes and keeps the refusal
// total the last of them carried.
//
// It takes WARN and above, which is where an operator's log is set: a debug
// record is one somebody asked for.
type countingHandler struct {
	mu      sync.Mutex
	records int
	total   uint64
}

func (h *countingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (h *countingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.records++

	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "refused_on_this_connection" {
			h.total = attr.Value.Uint64()
		}

		return true
	})

	return nil
}

func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }

func (h *countingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.records
}

func (h *countingHandler) lastRefusalTotal() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.total
}

// TestAFrameAboveTheReadLimitEndsTheConnectionAndKeepsTheSession is the one
// case the rule cannot hold, stated rather than hidden.
//
// maxFrameBytes is handed to the websocket layer as a read limit, so a frame
// above it fails the connection before a byte of it reaches this driver. There
// is no drop-the-frame behaviour available. What must still be true is the
// property the whole session shape exists for: the session outlives the
// connection, the command offered while the printer is away queues rather than
// failing, and it drains when the printer dials again.
func TestAFrameAboveTheReadLimitEndsTheConnectionAndKeepsTheSession(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	printer.send(t, badFrame{frame: oversizedFrame()})

	printer.awaitDeparture(t)

	if printer.session.Connected() {
		t.Fatal("a frame above the read limit left the connection attached, and gorilla fails it")
	}

	answered := printer.start(t)

	select {
	case settled := <-answered:
		t.Fatalf("the command failed instead of queueing for the printer's return: %+v %v",
			settled.result, settled.err)
	case <-time.After(250 * time.Millisecond):
	}

	printer.redial(t)

	id := printer.takeFrame(t)
	printer.answer(t, id)

	result, err := printer.await(t, answered)
	if err != nil {
		t.Fatalf("the queued command never drained: %v", err)
	}

	if !result.OK {
		t.Errorf("the drained command came back refused: %s", result.Raw)
	}

	printer.expectTelemetry(t)

	t.Logf("observed: the oversized frame ended the connection, the session held the command, and the "+
		"printer's next connection drained it under id %d", id)
}

// The harness.

// hostile is one session behind a real websocket the test writes whatever it
// likes to.
//
// A real socket rather than the simulator, for the reason the simulator cannot
// help with: everything it sends is well formed by construction, so nothing it
// can do reaches this path.
type hostile struct {
	session *Session
	client  *websocket.Conn
	// dialed carries every connection the server takes, so a case that ends one
	// can open the next.
	dialed chan struct{}
	url    string
	// dialog is the dialog subtree as this reader has seen it, kept because a
	// case that asserts a path was left alone has to read a value no later patch
	// carried. See hostile.awaitDialogTree.
	dialog map[string]string
	// dialogPatches is how many patches arrived for each of those paths, which
	// is what separates a field reported once from one reported per frame.
	dialogPatches map[string]int
}

func standHostile(t *testing.T) *hostile {
	t.Helper()

	return standHostileLogging(t, nil)
}

// standHostileLogging is standHostile with somewhere for the session's log to
// go, which is what the refusal-rate case measures.
func standHostileLogging(t *testing.T, log *slog.Logger) *hostile {
	t.Helper()

	return standHostileWith(t, log, 0)
}

// standHostileWith is standHostileLogging with a command deadline of its own.
// Zero takes the default.
func standHostileWith(t *testing.T, log *slog.Logger, deadline time.Duration) *hostile {
	t.Helper()

	// SEND_INFO is withheld so the wire belongs entirely to the test. The
	// session asks for it on every attach, and a request nobody answers would
	// hold the one command slot this suite is measuring.
	caps := baseline().Clone()
	delete(caps, connectorapi.VerbSendInfo)

	session := newSession(settings{
		identity:        connectorapi.Identity{Vendor: Vendor, Model: "MK4", Serial: "SN-HOSTILE"},
		caps:            caps,
		sends:           connectorapi.Set{VerbStartInlineDownload: {Flag: true}},
		storage:         defaultStorage,
		queueDepth:      4,
		log:             log,
		commandDeadline: deadline,
		live: Liveness{
			ReadDeadline: hostileSilence,
			PingInterval: hostileSilence,
			PingTimeout:  hostileBudget,
		},
	})
	t.Cleanup(func() { _ = session.Close() })

	printer := &hostile{session: session, dialed: make(chan struct{}, 4)}

	var upgrader websocket.Upgrader

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		if !session.attach(r.Context(), conn) {
			_ = conn.Close()

			return
		}

		printer.dialed <- struct{}{}
	}))
	t.Cleanup(server.Close)

	printer.url = "ws://" + strings.TrimPrefix(server.URL, "http://")
	printer.redial(t)

	return printer
}

// redial opens a connection as the printer and waits for the session to adopt
// it, so nothing after it races the attach.
func (h *hostile) redial(t *testing.T) {
	t.Helper()

	client, response, err := websocket.DefaultDialer.Dial(h.url, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}

	if err != nil {
		t.Fatalf("dial as the printer: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	h.client = client

	select {
	case <-h.dialed:
	case <-time.After(hostileBudget):
		t.Fatal("the connection never attached, so this test is not watching a session")
	}

	// Attaching is a presence change and leaves a wake-up waiting. Taking it
	// here is what lets awaitDeparture read the departure rather than this.
	select {
	case <-h.session.PresenceChanged():
	case <-time.After(hostileBudget):
		t.Fatal("attaching woke nobody, so presence is not being reported at all")
	}
}

// send writes one frame as the printer.
func (h *hostile) send(t *testing.T, frame badFrame) {
	t.Helper()

	kind := websocket.TextMessage
	if frame.binary {
		kind = websocket.BinaryMessage
	}

	if err := h.client.WriteMessage(kind, frame.frame); err != nil {
		t.Fatalf("write %q as the printer: %v", frame.frame, err)
	}
}

// text writes one well-formed-looking text frame as the printer.
func (h *hostile) text(t *testing.T, frame string) {
	t.Helper()

	h.send(t, badFrame{frame: []byte(frame)})
}

// awaitDropped waits until the session has evicted a patch no host took, and
// reports how many it had evicted by then.
//
// A write returns once the frame is on the socket, and the session reads it in
// a goroutine of its own, so the counter answers whatever the scheduler has
// reached rather than what the case has sent. A machine with one processor, or
// one running the whole suite, runs that reader only when this goroutine parks.
// Reading the counter once therefore measures the machine; waiting for it
// measures the queue.
func (h *hostile) awaitDropped(t *testing.T) uint64 {
	t.Helper()

	deadline := time.After(hostileBudget)

	for {
		if dropped := h.session.Dropped(); dropped > 0 {
			return dropped
		}

		select {
		case <-deadline:
			t.Fatalf("the session evicted no patch inside %s with nothing reading the state channel, "+
				"so the queue never filled and this case measures nothing", hostileBudget)

			return 0
		case <-time.After(time.Millisecond):
		}
	}
}

// bulk writes one large text frame as the printer, reporting its length rather
// than its bytes. A failure here is about the size, and printing 161 KB of JSON
// buries the sentence that says so.
func (h *hostile) bulk(t *testing.T, frame string) {
	t.Helper()

	if err := h.client.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
		t.Fatalf("a %d-byte frame did not reach the driver: %v. The read limit is %d bytes and a "+
			"directory listing this size is one a printer really answers with",
			len(frame), err, maxFrameBytes)
	}
}

// start offers one command and hands back the channel its outcome arrives on.
//
// One verb throughout: BEEP carries no arguments and every profile declares it,
// so what is being measured is the round trip and not the verb.
func (h *hostile) start(t *testing.T) chan outcome {
	t.Helper()

	answered := make(chan outcome, 1)

	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), hostileBudget)
		defer cancel()

		result, err := h.session.Do(ctx, connectorapi.Command{Verb: VerbBeep})
		answered <- outcome{result: result, err: err}
	}()

	return answered
}

// storages is what the session currently believes the printer's storages are.
func (h *hostile) storages(t *testing.T) []connectorapi.Storage {
	t.Helper()

	devices, err := h.session.Storages(t.Context())
	if err != nil {
		t.Fatalf("read the storages: %v", err)
	}

	return devices
}

// listed is one List call's outcome.
type listed struct {
	listing connectorapi.Listing
	err     error
}

// list asks for one directory and hands back the channel its outcome arrives
// on, so the test can answer the command the call puts on the wire.
func (h *hostile) list(t *testing.T, path string) chan listed {
	t.Helper()

	out := make(chan listed, 1)

	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), hostileBudget)
		defer cancel()

		listing, err := h.session.List(ctx, path)
		out <- listed{listing: listing, err: err}
	}()

	return out
}

func (h *hostile) awaitListing(t *testing.T, out chan listed) listed {
	t.Helper()

	select {
	case result := <-out:
		return result
	case <-time.After(hostileBudget):
		t.Fatal("the listing never came back, so the session is wedged")

		return listed{}
	}
}

// await takes the outcome of a command, failing rather than hanging.
func (h *hostile) await(t *testing.T, answered chan outcome) (connectorapi.Result, error) {
	t.Helper()

	select {
	case settled := <-answered:
		return settled.result, settled.err
	case <-time.After(hostileBudget):
		t.Fatal("the command never settled, so the session is wedged")

		return connectorapi.Result{}, errors.New("unreachable")
	}
}

// takeFrame reads one server-to-printer frame and returns the id its header
// carries.
func (h *hostile) takeFrame(t *testing.T) uint32 {
	t.Helper()

	if err := h.client.SetReadDeadline(time.Now().Add(hostileBudget)); err != nil {
		t.Fatalf("arm the printer's read deadline: %v", err)
	}

	_, data, err := h.client.ReadMessage()
	if err != nil {
		t.Fatalf("read a frame as the printer: %v", err)
	}

	if len(data) < hostileHeader {
		t.Fatalf("the driver wrote %q, which is shorter than the nine-byte header", data)
	}

	id, err := strconv.ParseUint(string(data[1:hostileHeader]), 16, 32)
	if err != nil {
		t.Fatalf("the driver wrote a header whose id is not eight hex digits: %q", data[:hostileHeader])
	}

	return uint32(id)
}

// answer sends the terminal event for one command id. FINISHED is the shape
// every completion takes; the two that fail a command have their own test.
func (h *hostile) answer(t *testing.T, id uint32) {
	t.Helper()

	frame := fmt.Sprintf(`{"event":"FINISHED","command_id":%d,"state":"IDLE"}`, id)
	if err := h.client.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
		t.Fatalf("answer command %d: %v", id, err)
	}
}

// expectTelemetry sends one well-formed telemetry frame and waits for the two
// state paths it carries, which is what says the state tree is still being fed.
func (h *hostile) expectTelemetry(t *testing.T) {
	t.Helper()

	if err := h.client.WriteMessage(websocket.TextMessage,
		[]byte(`{"temp_nozzle":222,"state":"IDLE"}`)); err != nil {
		t.Fatalf("send telemetry as the printer: %v", err)
	}

	want := map[string]string{"/state": `"IDLE"`, "/tools/0/nozzle_temp": "222"}
	seen := make(map[string]string, len(want))

	deadline := time.After(hostileBudget)

	// An INFO before the telemetry puts patches of its own on the channel, so
	// the wait is for the paths named and not for a number of patches.
	for !seenAll(seen, want) {
		select {
		case patch, open := <-h.session.State():
			if !open {
				t.Fatal("the state channel closed, so the session ended rather than dropping the frame")
			}

			if _, named := want[patch.Path]; named {
				seen[patch.Path] = string(patch.Value)
			}
		case <-deadline:
			t.Fatalf("telemetry after the bad frame reached %v, want %v", seen, want)
		}
	}

	for path, value := range want {
		if seen[path] != value {
			t.Errorf("%s arrived as %s, want %s", path, seen[path], value)
		}
	}
}

// seenAll reports whether every path named has arrived.
func seenAll(seen, want map[string]string) bool {
	for path := range want {
		if _, arrived := seen[path]; !arrived {
			return false
		}
	}

	return true
}

// awaitDeparture waits for the session to notice the connection has gone.
func (h *hostile) awaitDeparture(t *testing.T) {
	t.Helper()

	select {
	case <-h.session.PresenceChanged():
	case <-time.After(departureBudget):
		t.Fatalf("the connection was still attached %s after the oversized frame, and the read limit "+
			"fails that read as it happens", departureBudget)
	}
}

// crowdedDirectory is how many files the read-limit floor is measured against.
// A USB stick with a thousand gcode files on it is an ordinary thing, and the
// firmware answers SEND_FILE_INFO with the whole directory in one frame.
const crowdedDirectory = 1000

// TestTheReadLimitCarriesADirectoryListingAPrinterReallySends pins
// maxFrameBytes from below.
//
// TestAFrameAboveTheReadLimitEndsTheConnectionAndKeepsTheSession measures a
// frame built from the constant, so it passes at any value of it: watched, with
// maxFrameBytes cut from 1 << 20 to 64, that test stays green. A limit is two
// bounds and only one of them was held. This is the other one, and it is stated
// as a frame a printer really sends rather than as a number, because a number
// on both sides of the same constant proves nothing.
func TestTheReadLimitCarriesADirectoryListingAPrinterReallySends(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)

	listed := printer.list(t, "/usb/deep")

	id := printer.takeFrame(t)

	frame := crowdedListing(id, "/usb/deep", crowdedDirectory)
	printer.bulk(t, frame)

	result := printer.awaitListing(t, listed)
	if result.err != nil {
		t.Fatalf("a %d-byte FILE_INFO listing %d files did not reach the driver: %v. The read limit is "+
			"%d bytes and a directory this size is one a printer really answers with",
			len(frame), crowdedDirectory, result.err, maxFrameBytes)
	}

	if got := len(result.listing.Entries); got != crowdedDirectory {
		t.Errorf("the listing carried %d entries and the frame named %d", got, crowdedDirectory)
	}

	t.Logf("observed: a %d-byte FILE_INFO carrying %d files crossed the %d-byte read limit intact",
		len(frame), crowdedDirectory, maxFrameBytes)
}

// crowdedListing renders one FILE_INFO answering command id with a directory of
// files, spelled the way shipping firmware spells them: a firmware path, a
// short name and a long one.
func crowdedListing(id uint32, path string, files int) string {
	var frame strings.Builder

	fmt.Fprintf(&frame, `{"event":"FILE_INFO","command_id":%d,"data":{"path":%q,"name":"deep",`+
		`"type":"FOLDER","children":[`, id, path)

	for i := range files {
		if i > 0 {
			frame.WriteByte(',')
		}

		long := fmt.Sprintf("PRINT_PART_%04d.bgcode", i)
		fmt.Fprintf(&frame, `{"path":"%s/%s","name":"PRINT~%03d.BGC","display_name":%q,"type":"FILE",`+
			`"size":1234567,"m_timestamp":1700000000}`, path, long, i, long)
	}

	frame.WriteString(`]}}`)

	return frame.String()
}

// oversizedFrame is one byte-run above maxFrameBytes, which is the read limit
// the session hands the websocket layer.
func oversizedFrame() []byte {
	body := strings.Repeat("x", maxFrameBytes)

	return []byte(`{"note":"` + body + `"}`)
}
