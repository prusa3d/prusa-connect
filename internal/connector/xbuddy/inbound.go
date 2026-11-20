package xbuddy

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// pongDeadline bounds how long answering a ping may take before the connection
// is treated as stuck. gorilla's control writes need a deadline and the zero
// value is none.
const pongDeadline = 5 * time.Second

// The shipping connector's liveness numbers, transcribed from
// legacy/connector/xbuddy/connection.go rather than chosen here.
//
// Liveness on this dialect rests on the server's ping: the firmware pings only
// after 15s of sending nothing, and a printer that lost power or fell out of a
// NAT table leaves a socket that reads as live for as long as the operating
// system keeps retrying. Silence is the only symptom, and the read deadline is
// what turns it into a disconnection the session can queue against. The
// interval sits one ping's grace below the deadline.
const (
	readDeadline = 60 * time.Second
	pingInterval = 54 * time.Second
	pingTimeout  = 10 * time.Second

	// writeDeadline bounds one frame leaving. It is the ping's bound, because a
	// socket that will not take a chunk within it will not take a ping either,
	// and it sits well below readDeadline so that a link which stopped draining
	// is a write that fails rather than a writer holding the lock until the read
	// deadline ends the connection.
	writeDeadline = pingTimeout

	// infoDelay is how long the shipping connector waits after a socket
	// attaches before asking the printer what it is, and it is transcribed
	// rather than chosen (legacy/connector/xbuddy/connection.go:42).
	infoDelay = 100 * time.Millisecond

	// infoRequestTimeout bounds the storage request one attached connection
	// makes. This driver holds nothing waiting for the answer, and the next
	// connection asks again, so a printer that has not answered by then is
	// asked afresh rather than waited for.
	infoRequestTimeout = 30 * time.Second

	// askStoragesTimeout bounds the SEND_INFO a root listing or a refreshed
	// storages read sends. A person is waiting on the page that asked, and a
	// printer that has not answered by then is listed from what it last said.
	// An idle printer answers in well under a second.
	askStoragesTimeout = 5 * time.Second
)

// dialogIDPath is where the dialog the machine is waiting on is addressed.
//
// core.command.dialog_action names a dialog_id and the printer is the only
// thing that knows one, so a state tree that does not carry it leaves the verb
// with nothing to name.
const dialogIDPath = "/dialog/id"

// Where the rest of the dialog is addressed: what the firmware put on the
// screen, so that a caller answering the dialog knows what it is agreeing to.
//
// The names are the firmware's own keys, and every value is passed through as
// the printer encoded it. Nothing here looks a code up in an error catalogue:
// this is a driver, and a sentence written by something else is not the thing
// the machine displayed.
const (
	dialogCodePath    = "/dialog/code"
	dialogTitlePath   = "/dialog/title"
	dialogTextPath    = "/dialog/text"
	dialogButtonsPath = "/dialog/buttons"
)

// dialogPaths is every path a dialog is addressed at, in the order the host is
// told about them. The id leads, so a reader that acts on the id already holds
// the words that go with it.
//
//nolint:gochecknoglobals // a fixed list, read-only after init.
var dialogPaths = []string{
	dialogIDPath, dialogCodePath, dialogTitlePath, dialogTextPath, dialogButtonsPath,
}

// reservedPath reports whether a path reaches the host whatever the backlog is
// doing: the dialog, what INFO says about the tools, and which tools are
// active. See Session.report.
func reservedPath(path string) bool {
	return slices.Contains(dialogPaths, path) || hardwarePath(path) || activityPath(path)
}

// The printer states in which a dialog is up, which are the two names the
// shipping server uses (legacy/printer/state/printer_state.go:1055). What any
// other state means for the dialog is dialogClosedBy's rule.
const (
	stateAttention = "ATTENTION"
	stateError     = "ERROR"
)

// eventStateChanged is the event that carries a dialog. It is also the event
// this dialect uses for every other state transition, which is why a dialog is
// read out of it only when it names a code.
const eventStateChanged = "STATE_CHANGED"

// eventJobInfo is the frame that says what the machine is printing. It answers
// SEND_JOB_INFO and a machine volunteers one whenever a print starts, pauses,
// resumes or ends.
const eventJobInfo = "JOB_INFO"

// Where the print is addressed, beyond the four keys telemetry already carries.
//
// The path is the one that has to come from here. Telemetry has a job number, a
// progress and two times, and no file: a host reading only telemetry knows a
// machine is printing and cannot say what. So JOB_INFO is where the file name
// enters the state tree, and it enters it as ordinary state rather than through
// a channel of this driver's own.
//
// The job's state is addressed separately from /state on purpose. /state is the
// machine, and it moves on to IDLE or READY after a print ends; /job/state is
// the print, and the last thing it says is how the print ended. A host that
// stores a job wants the second one and a host that shows a machine wants the
// first.
const (
	jobStatePath   = "/job/state"
	jobPathPath    = "/job/path"
	jobDisplayPath = "/job/display"
)

// telemetryPaths is where each telemetry key lands in the addressable state
// tree.
//
// It covers the keys every model reports. The tools, in a slot block or a
// tools object, and the nozzle and material at the top of the frame, which
// belong to a head, are read by reportToolTelemetry. The chamber and enclosure
// blocks are read by reportChamber. A key this map does not carry is not
// emitted.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var telemetryPaths = map[string]string{
	"state":          "/state",
	"temp_bed":       "/temperature/bed",
	"target_bed":     "/temperature/bed_target",
	"axis_x":         "/axis/x",
	"axis_y":         "/axis/y",
	"axis_z":         "/axis/z",
	"speed":          "/speed",
	"flow":           "/flow",
	"job_id":         "/job/id",
	"progress":       "/job/progress",
	"time_printing":  "/job/time_printing",
	"time_remaining": "/job/time_remaining",
}

// read serves one connection until it fails, and detaches it when it does. The
// session outlives it: the queue is what a printer coming back drains.
//
// Serving in one goroutine is what keeps the printer's own ordering: an event
// is matched to the command in flight before the next frame is read.
func (s *Session) read(ctx context.Context, current *link) {
	defer s.detach(current)
	defer s.summariseRefusals(ctx, current)

	for {
		kind, data, err := s.readMessage(current)
		if err != nil {
			// INFO, beside the admission that follows it. It is the only record
			// that tells a printer that hung up from a server that closed it.
			s.logInfo(ctx, "the printer's connection ended", slog.String("error", err.Error()))

			return
		}

		// A frame is proof the printer is there, whatever it holds. A connection
		// that will not take the deadline is one the next read fails on.
		if rearmErr := s.rearm(current); rearmErr != nil {
			s.logDebug(ctx, "the printer's connection would not take a read deadline",
				slog.String("error", rearmErr.Error()))

			return
		}

		if kind != websocket.TextMessage {
			// Printer to server is always text. A binary frame is something
			// this dialect has no reading of, and guessing at one is worse than
			// dropping it.
			s.refuse(ctx, current, "the printer sent a frame that is not text, and this dialect reads only text",
				slog.Int("websocket_type", kind))

			continue
		}

		message, classifyErr := classify(data)
		if classifyErr != nil {
			s.refuse(ctx, current, "the printer sent a frame this dialect cannot read",
				slog.String("error", classifyErr.Error()))

			continue
		}

		s.dispatch(ctx, current, message)
	}
}

// dispatch acts on one classified frame.
func (s *Session) dispatch(ctx context.Context, current *link, message inboundMessage) {
	switch message.kind {
	case inboundEvent:
		s.dispatchEvent(ctx, current, message)
	case inboundTelemetry:
		s.dispatchTelemetry(message)
	case inboundBlockRequest:
		s.serveBlockRequest(ctx, current, message)
	}

	if s.stateHeard.Load() {
		current.stateOnce.Do(func() { close(current.stateIn) })
	}
}

// dispatchEvent matches an event to the command in flight.
//
// The id is read first and refused before anything else in the frame is
// believed. A frame whose id this framing cannot express is a frame from a
// dialect this build does not have, and reading the printer's storages or a
// directory listing out of one is trusting the same bytes that were just found
// unreadable: the state it writes outlives the frame and answers the next
// caller. An id the event simply does not carry is a different thing and is the
// ordinary case, because INFO and FILE_INFO are the shape a printer volunteers.
//
// Telemetry carries the state at 4 Hz, so nothing here has to read that out of
// an event as well.
func (s *Session) dispatchEvent(ctx context.Context, current *link, message inboundMessage) {
	id, carried, err := message.event.commandID()
	if err != nil {
		s.refuse(ctx, current, "the printer answered with a command id nothing can be matched to",
			slog.String("event", message.event.Event), slog.String("error", err.Error()))

		return
	}

	s.logEvent(ctx, message.event, carried, id)

	s.observe(message)
	s.noteJob(message.event)
	s.noteCancelableObjects(message)
	// Every event carries the machine's state on the envelope, so an event is
	// the other place a state change can first be seen. It goes into the tree
	// as well as into the refresh logic: a JOB_INFO saying PRINTING arrives
	// before the telemetry frame that would say the same, and a record flush
	// falling in that gap read a job that is running against a machine the tree
	// still called IDLE, stamped the print ended, and held that end for ever
	// (found by the e2e print gate, 2026-09-03). It costs nothing when it is the
	// state telemetry already reported: an unchanged value is no patch.
	s.reportText(telemetryPaths["state"], message.event.State)
	s.noteMachineState(message.event.State)
	s.noteDialog(message.event.DialogID, message.event.State, dialogFrom(message.event))

	if !carried {
		return
	}

	s.answer(id, message.event, message.raw)
}

// dispatchTelemetry turns one telemetry frame into patches to the state tree.
//
// The keys are emitted in a fixed order so that two runs of one frame produce
// the same sequence, and each value is passed through as the printer encoded
// it: a driver that re-encodes a number changes it.
func (s *Session) dispatchTelemetry(message inboundMessage) {
	keys := make([]string, 0, len(message.fields))

	for key := range message.fields {
		if _, mapped := telemetryPaths[key]; mapped {
			keys = append(keys, key)
		}
	}

	slices.Sort(keys)

	for _, key := range keys {
		value := message.fields[key]

		// Buddy reads the flow off the tool it has selected, and with none
		// selected, as on an MMU with no filament loaded, it sends the 0 it
		// starts from (src/connect/marlin_printer.cpp). That is no reading
		// rather than a flow of nothing, so the node goes.
		if key == "flow" && string(value) == "0" {
			value = nullValue
		}

		s.report(connectorapi.StatePatch{Path: telemetryPaths[key], Value: value})
	}

	s.reportToolTelemetry(message.fields)
	s.reportChamber(message.fields)

	// The job number is remembered as well as reported. SEND_JOB_INFO names the
	// print it is asking about and telemetry is where the number comes from, so
	// a driver that only forwarded it would have nothing to ask with.
	if raw, present := message.fields["job_id"]; present {
		s.noteReportedJobID(raw)
	}

	// The machine's own state is the second thing that says the print moved,
	// and the one that catches a print that ended or was paused at the machine
	// rather than through a verb of ours. See jobrefresh.go.
	s.noteMachineState(stateOf(message.fields))

	// The shipping connector reads the dialog off telemetry as well as off an
	// event (legacy/connector/xbuddy/connection.go:415), because a dialog raised
	// while nothing was in flight is carried by nothing else.
	//
	// Telemetry carries the id and nothing else about the dialog, so what a
	// dialog says arrives with the event that raised it or not at all.
	s.noteDialog(dialogIDOf(message.fields), stateOf(message.fields), nil)
}

// jobFrame is what a JOB_INFO event carries about the print. Every member is
// optional: a machine that has never printed answers the command with no data
// at all, and a firmware that leaves one of them out has simply not said it.
//
// A machine whose storage keeps FAT short names reports the path in the short
// form, such as `/usb/1A_PET~7.GCO`, and the long name as `display_name` (the
// Connect API's JOB_INFO schema requires it there). `display` is the key some
// firmware sends instead.
type jobFrame struct {
	State       string `json:"state"`
	Path        string `json:"path"`
	JobID       *int64 `json:"job_id"`
	DisplayName string `json:"display_name"`
	Display     string `json:"display"`
}

// noteJob turns a JOB_INFO event into patches to the state tree.
//
// It reports the job number again, which telemetry also carries, and that
// repetition is the point rather than a duplicate: telemetry carries the number
// only while the machine is printing, so the tree would otherwise hold the
// number of the print that is over and no way to tell that it is. The frame
// that announces the end restates it beside a state that says the print ended.
//
// A value the frame does not carry is not written. This dialect's frames are
// sparse, and writing an empty string over a path the machine reported a minute
// ago would lose the file name every time a firmware left the key out.
func (s *Session) noteJob(reported printerEvent) {
	if reported.Event != eventJobInfo {
		return
	}

	var frame jobFrame

	if len(reported.Data) > 0 {
		if err := json.Unmarshal(reported.Data, &frame); err != nil {
			return
		}
	}

	// The event's own state is the fallback for the job's, because a firmware
	// that puts the state on the envelope and not in the payload has still said
	// what the print is doing.
	state := frame.State
	if state == "" {
		state = reported.State
	}

	// And the machine's own word is what decides, wherever it has one. The
	// firmware renders this frame's state as PRINTING only while its device
	// state is exactly Printing and as PAUSED for every other one, ATTENTION
	// and Busy included (src/connect/render.cpp), so its PAUSED is not evidence
	// that the print is paused: an answer that arrived while a print-preview
	// dialog stood said PAUSED about a print nobody had paused and the tree
	// kept it (BUG-013).
	//
	// A machine holding a print decides /job/state from its own state, and
	// ATTENTION resolves to no word at all, because a dialog over a print says
	// nothing about the print and reportJobState writes nothing for it. A
	// machine that says nothing about the print leaves the word already there,
	// which is the same rule read from the other side. See
	// noteThePrintIsRunning and machineSaysNothingAboutThePrint.
	switch machine := s.machineWord(reported.State); {
	case slices.Contains(machineIsHoldingAPrint, machine):
		state = ""
		if machine != stateAttention {
			state = machine
		}
	case s.machineSaysNothingAboutThePrint(machine):
		state = ""
	}

	if frame.JobID != nil {
		s.noteJobID(*frame.JobID)
		s.report(connectorapi.StatePatch{
			Path:  telemetryPaths["job_id"],
			Value: json.RawMessage(strconv.FormatInt(*frame.JobID, 10)),
		})
	}

	// A JOB_INFO answered after the print ended carries IDLE, because the
	// machine has no job any more, and the print's own last word is how it
	// ended. See noteThePrintEnded.
	if !s.jobStateHoldsAnEnd() || slices.Contains(machineIsHoldingAPrint, state) {
		s.reportJobState(state)
	}

	s.reportText(jobPathPath, frame.Path)
	s.reportText(jobDisplayPath, cmp.Or(frame.DisplayName, frame.Display))
}

// reportText patches one path with a string the machine reported, and reports
// nothing for a value it did not.
func (s *Session) reportText(path, value string) {
	if value == "" {
		return
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}

	s.report(connectorapi.StatePatch{Path: path, Value: encoded})
}

// loggedRefusalsPerConnection is how many of a connection's refused frames get a
// log record of their own. The rest are counted and reported once at the end;
// see Session.refuse.
const loggedRefusalsPerConnection = 1

// refuse drops one frame the dialect has no reading of and records that it did.
//
// The frame goes and the connection stays. A printer runs firmware this build
// does not control, so an unreadable frame is a key or a shape nobody here has
// seen rather than an attack, and hanging up on one costs the printer every
// command in flight and every command queued behind it. Dropping it costs the
// frame. The counter and the log together are what separate a deliberate drop
// from a silent one, on the printer that is connected and whose frames are all
// arriving unreadable.
//
// Only the first refusal on a connection is logged, and the rest are counted.
// The condition repeats at whatever rate the printer sends: a firmware speaking
// something else refuses every frame, and 20,000 of them were refused here in
// 141 ms. One record each is an operator's disk spent saying the same sentence.
// summariseRefusals is where the total arrives.
func (s *Session) refuse(ctx context.Context, current *link, message string, attrs ...slog.Attr) {
	s.refused.Add(1)

	if current.refused.Add(1) > loggedRefusalsPerConnection {
		return
	}

	s.logWarn(ctx, message, attrs...)
}

// summariseRefusals reports what one connection refused, once, as it ends.
//
// Without it a connection that refused twenty thousand frames and one that
// refused two read identically in a log, because only the first of each was
// written.
func (s *Session) summariseRefusals(ctx context.Context, current *link) {
	total := current.refused.Load()
	if total <= loggedRefusalsPerConnection {
		return
	}

	s.logWarn(ctx, "the printer sent more frames this dialect cannot read, and only the first was logged",
		slog.Uint64("refused_on_this_connection", total))
}

func (s *Session) logWarn(ctx context.Context, message string, attrs ...slog.Attr) {
	s.logAt(ctx, slog.LevelWarn, message, attrs...)
}

func (s *Session) logDebug(ctx context.Context, message string, attrs ...slog.Attr) {
	s.logAt(ctx, slog.LevelDebug, message, attrs...)
}

func (s *Session) logAt(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	if s.log == nil {
		return
	}

	s.log.LogAttrs(ctx, level, message,
		append([]slog.Attr{
			slog.String("serial", s.identity.Serial),
			slog.String("model", s.identity.Model),
		}, attrs...)...)
}

// logEvent records what the printer volunteered. An event is how a printer says
// everything that is not telemetry, and a dialect that acts on a few of them
// gives an operator no way to see the rest.
func (s *Session) logEvent(ctx context.Context, event printerEvent, carried bool, id uint32) {
	attrs := []slog.Attr{slog.String("event", event.Event)}

	if carried {
		attrs = append(attrs, slog.Uint64("command_id", uint64(id)))
	}

	if event.TransferID != nil {
		attrs = append(attrs, slog.Int64("transfer_id", *event.TransferID))
	}

	if event.Reason != "" {
		attrs = append(attrs, slog.String("reason", event.Reason))
	}

	s.logDebug(ctx, "the printer sent an event", attrs...)
}

func (s *Session) logInfo(ctx context.Context, message string, attrs ...slog.Attr) {
	s.logAt(ctx, slog.LevelInfo, message, attrs...)
}

// noteDialog records the dialog the machine is waiting on, from whichever frame
// carried it.
//
// This dialect's telemetry is sparse: a key one frame leaves out says nothing
// about it, so a dialog closes on an explicit signal and on nothing else. There
// are two signals. One is a state that is neither ATTENTION nor ERROR. The other
// is a dialog_id of zero or below, because the id is a uint32 on the machine.
// The firmware sends no "dialog closed" of its own, so there is no third.
//
// Reading a missing dialog_id as a closed dialog is the defect the shipping
// connector was fixed for against a real machine: the id flapped on alternate
// frames and every flap queued another SEND_STATE_INFO
// (legacy/connector/xbuddy/connection.go:835-846 and :415).
//
// A frame naming a dialog in a state no dialog is up in leaves nothing behind,
// which is the shipping server's rule (legacy/printer/state/printer_state.go:758).
func (s *Session) noteDialog(id *int64, state string, said *dialogPayload) {
	id = answerableID(id)

	if dialogClosedBy(id, state) {
		s.clearDialog()

		return
	}

	moved := id != nil && s.raiseDialog(*id)

	if moved && said == nil {
		// A different dialog, and this frame says nothing about it. Telemetry
		// carries the id alone, so the words on the tree belong to the dialog
		// that has gone and describing this one with them is worse than saying
		// nothing about it.
		said = &dialogPayload{}
	}

	s.describeDialog(said)
}

// answerableID drops an id no caller could name. The dialog reply bounds
// dialog_id to a uint32 (verbs.go), so an id above that is reported as no id at
// all rather than as a dialog every answer to it is refused.
func answerableID(id *int64) *int64 {
	if id != nil && *id > maxUint32 {
		return nil
	}

	return id
}

// dialogClosedBy reports whether one frame says no dialog is on the screen. Only
// the two explicit signals count, and a frame carrying neither of them says
// nothing about the dialog. See Session.noteDialog for where the rule comes
// from.
func dialogClosedBy(id *int64, state string) bool {
	if id != nil && *id <= 0 {
		return true
	}

	return state != "" && state != stateAttention && state != stateError
}

// raiseDialog puts the dialog's id on the state tree, and only when it moved,
// reporting whether it did. Telemetry repeats the id at 4 Hz for as long as the
// dialog is up, and a patch per frame would be the same fact written four times
// a second.
//
// The patch is reported under the lock, which is the rule the whole dialog
// subtree keeps: two connections can be read at once (Session.attach), and a
// raise and a close that interleave between deciding and reporting leave the
// tree holding an id the session believes is gone. report never blocks, so the
// lock costs nothing.
func (s *Session) raiseDialog(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.inDialog && s.dialog == id {
		return false
	}

	s.dialog, s.inDialog = id, true

	s.report(connectorapi.StatePatch{
		Path:  dialogIDPath,
		Value: json.RawMessage(strconv.FormatInt(id, 10)),
	})

	return true
}

// describeDialog puts what the firmware displayed on the state tree: the code,
// the title, the text and the button labels, as the printer encoded them.
//
// It is what makes a dialog answerable by somebody who is not at the machine. A
// caller holding only the id can press a button without knowing what it is
// agreeing to, and core.command.dialog_action is exactly that button.
func (s *Session) describeDialog(said *dialogPayload) {
	if said == nil {
		return
	}

	s.reportDialogField(dialogCodePath, said.Code)
	s.reportDialogField(dialogTitlePath, said.Title)
	s.reportDialogField(dialogTextPath, said.Text)
	s.reportDialogField(dialogButtonsPath, said.Buttons)
}

// reportDialogField reports one field of the dialog, and only when it moved.
//
// A field this dialog does not carry is removed rather than left behind, so the
// title of the dialog before it is never read as this one's. A field that was
// never there is not removed either, because a null on a path the tree does not
// hold is a patch that says nothing.
func (s *Session) reportDialogField(path string, value json.RawMessage) {
	next := dialogAbsent
	if carriesValue(value) {
		next = string(value)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous, reported := s.dialogFields[path]

	switch {
	case next == dialogAbsent:
		delete(s.dialogFields, path)
	case s.dialogFields == nil:
		s.dialogFields = map[string]string{path: next}
	default:
		s.dialogFields[path] = next
	}

	if next == previous || (!reported && next == dialogAbsent) {
		return
	}

	s.report(connectorapi.StatePatch{Path: path, Value: json.RawMessage(next)})
}

// clearDialog removes the dialog once the machine reports a state none is up
// in. A null value removes the node, which is how "there is no dialog" is told
// apart from "the dialog is numbered zero".
//
// The whole dialog goes, not only its id. A title and a set of buttons that
// outlive the dialog they belong to describe a machine that has moved on, and a
// caller reading them answers a question nobody is asking any more.
func (s *Session) clearDialog() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.inDialog && len(s.dialogFields) == 0 {
		return
	}

	had := s.inDialog
	s.inDialog = false

	// Sorted, so that two runs of one frame produce the same sequence of
	// patches, which is the rule dispatchTelemetry already keeps.
	described := slices.Sorted(maps.Keys(s.dialogFields))
	clear(s.dialogFields)

	if had {
		s.report(connectorapi.StatePatch{Path: dialogIDPath, Value: json.RawMessage(dialogAbsent)})
	}

	for _, path := range described {
		s.report(connectorapi.StatePatch{Path: path, Value: json.RawMessage(dialogAbsent)})
	}
}

// dialogPayload is what a STATE_CHANGED event's data object says about the
// dialog on the screen. Every value is left encoded, because a driver that
// re-encodes what the printer sent changes it.
type dialogPayload struct {
	Code    json.RawMessage `json:"code"`
	Title   json.RawMessage `json:"title"`
	Text    json.RawMessage `json:"text"`
	Buttons json.RawMessage `json:"buttons"`
}

// dialogAbsent is the value that removes a node from the state tree.
const dialogAbsent = "null"

// dialogFrom reads what a dialog says out of one event.
//
// Only STATE_CHANGED carries one, which is the shipping server's rule
// (legacy/printer/state/printer_state.go:810), and only when it says something
// about a dialog. Both are needed: STATE_CHANGED is this dialect's event for
// every state transition, so reading each of them as a dialog would blank the
// dialog on the screen every time the machine reported the state it is already
// in.
//
// A code is not required: some printers send a dialog without one, such as the HT90.
func dialogFrom(event printerEvent) *dialogPayload {
	if event.Event != eventStateChanged || len(event.Data) == 0 {
		return nil
	}

	var said dialogPayload
	if err := json.Unmarshal(event.Data, &said); err != nil {
		return nil
	}

	if !carriesValue(said.Code) && !carriesValue(said.Title) &&
		!carriesValue(said.Text) && !carriesValue(said.Buttons) {
		return nil
	}

	return &said
}

// carriesValue reports whether the firmware sent something for a key. A key it
// left out and a key it sent as null are the same absence.
func carriesValue(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != dialogAbsent
}

// dialogIDOf reads the dialog id off a telemetry frame. A frame that leaves the
// key out carries no id at all, which says nothing about the dialog.
func dialogIDOf(fields map[string]json.RawMessage) *int64 {
	raw, present := fields["dialog_id"]
	if !present {
		return nil
	}

	// Read as a float and truncated, which is what the shipping connector does
	// with the same value (legacy/connector/xbuddy/connection.go:822-830): the
	// wire is JSON, so 42.5 is the machine's 42 rather than a frame to act on.
	var reported float64
	if err := json.Unmarshal(raw, &reported); err != nil {
		// A value that is no number names nothing, and closing a dialog the
		// machine is still waiting on because one frame was unreadable is the
		// defect this rule exists to prevent, the other way round.
		return nil
	}

	id := int64(reported)

	return &id
}

// stateOf reads the printer state off a telemetry frame, empty when the frame
// carries none or carries something that is not a string.
func stateOf(fields map[string]json.RawMessage) string {
	raw, present := fields["state"]
	if !present {
		return ""
	}

	var state string
	if err := json.Unmarshal(raw, &state); err != nil {
		return ""
	}

	return state
}
