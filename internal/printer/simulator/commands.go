package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The endpoints and the websocket subprotocol a printer dials.
const (
	registerPath         = "/p/register"
	websocketPath        = "/p/ws"
	websocketSubprotocol = "prusa-connect"
)

// The printer states the firmware reports. The whole /state vocabulary is IDLE,
// PRINTING, PAUSED, FINISHED, STOPPED, READY, ERROR, BUSY, ATTENTION and
// UNKNOWN, as to_str(DeviceState) spells them in
// Prusa-Firmware-Buddy/src/state/printer_state.cpp:563.
//
// BUSY is the word a long motion command produces, such as an MK4 homing and
// levelling its bed. UNKNOWN comes off the same switch. No simulated fault
// raises it, and an unused constant is a lint finding, so it is named here and
// not declared. stateAttention and stateError are in dialog.go, because a
// dialog raises them.
const (
	stateIdle     = "IDLE"
	statePrinting = "PRINTING"
	statePaused   = "PAUSED"
	stateFinished = "FINISHED"
	stateReady    = "READY"
	stateStopped  = "STOPPED"
	stateBusy     = "BUSY"
)

// machineHoldsAPrint are the states a print is still on the bed in, the two the
// machine prints in and the one a dialog over a print is raised in. They are
// what JOB_INFO's state is rendered from while there is a print: see
// jobDataLocked.
//
//nolint:gochecknoglobals // a fixed set, read-only after init.
var machineHoldsAPrint = []string{statePrinting, statePaused, stateAttention}

// The events the printer sends. An inbound text frame with an event key is an
// event, which is how the server classifies it.
const (
	eventInfo         = "INFO"
	eventJobInfo      = "JOB_INFO"
	eventFileInfo     = "FILE_INFO"
	eventTransferInfo = "TRANSFER_INFO"
	// The three ways a transfer ends. FINISHED is the only one that means the
	// file is on the storage.
	eventTransferFinished = "TRANSFER_FINISHED"
	eventTransferAborted  = "TRANSFER_ABORTED"
	eventTransferStopped  = "TRANSFER_STOPPED"
	eventStateChanged     = "STATE_CHANGED"
	eventAccepted         = "ACCEPTED"
	eventFinished         = "FINISHED"
	eventRejected         = "REJECTED"
)

// The reasons a REJECTED event carries.
//
// These are the simulator's own vocabulary and not the firmware's. The
// simulator is stricter than the printer here. Every refusal is logged with
// reality_unknown, because no recording confirms the ground for it.
const (
	// ReasonUnsupportedCommand is a verb this model and firmware do not
	// declare.
	ReasonUnsupportedCommand = "UNSUPPORTED_COMMAND"
	// ReasonMissingArgument is a required argument the caller omitted.
	ReasonMissingArgument = "MISSING_ARGUMENT"
	// ReasonUnknownArgument is an argument the verb does not declare.
	ReasonUnknownArgument = "UNKNOWN_ARGUMENT"
	// ReasonInvalidArgument is an argument whose value is the wrong kind.
	ReasonInvalidArgument = "INVALID_ARGUMENT"
	// ReasonMalformedCommand is a J frame whose payload is not a command.
	ReasonMalformedCommand = "MALFORMED_COMMAND"
	// ReasonMediaNotInserted is a path on a storage device that is not there.
	ReasonMediaNotInserted = "MEDIA_NOT_INSERTED"
	// ReasonFileNotFound is a path on a storage device that holds no such
	// entry, or a delete asking for the wrong kind of one.
	ReasonFileNotFound = "FILE_NOT_FOUND"
	// ReasonFileExists is a name the storage already holds. The shipping
	// connector reads this reason back, so the spelling is the firmware's and
	// not this simulator's.
	ReasonFileExists = "FILE_EXISTS"
	// ReasonStorageFull is a write to a device with no space left.
	ReasonStorageFull = "STORAGE_FULL"
	// ReasonStorageReadOnly is a write to a device mounted read-only.
	ReasonStorageReadOnly = "STORAGE_READ_ONLY"
	// ReasonTransferTooLarge is a transfer at or above 4 GiB, which the uint32
	// on the wire cannot express.
	ReasonTransferTooLarge = "TRANSFER_TOO_LARGE"
	// ReasonInvalidState is a verb that needs a state the printer is not in: a
	// second print while one is running, a pause with nothing printing, a
	// resume with nothing paused, a stop with no job, or a move to ready or
	// idle while a job is on the bed. The firmware answers each of these with
	// REJECTED and a free text. The token is this simulator's.
	ReasonInvalidState = "INVALID_STATE"
	// ReasonNoDialog is a dialog answered on a display holding none. The
	// firmware's own word is "No buttons" (marlin_printer.cpp).
	ReasonNoDialog = "NO_DIALOG"
	// ReasonInvalidDialogID is an answer naming a dialog other than the one on
	// the display. A caller sends it when it answers a question the machine
	// has moved on from.
	ReasonInvalidDialogID = "INVALID_DIALOG_ID"
	// ReasonInvalidButton is a button the dialog on the display does not offer.
	ReasonInvalidButton = "INVALID_BUTTON"
	// ReasonTransferInProgress is a second transfer offered to a printer
	// already fetching one. A printer fetches one at a time.
	ReasonTransferInProgress = "TRANSFER_IN_PROGRESS"
)

// setValueCommand is the frame the shipping connector rewrites several
// capability verbs into on the way to the wire. The simulator must answer it
// although no profile declares it. Model.verbForArguments resolves which verb
// it means from the profile data.
const setValueCommand = "SET_VALUE"

// writingCommands are the verbs that need space on the target device. A verb
// outside this set may still name a path, and a missing device refuses it, but
// a full one does not.
var writingCommands = map[string]bool{
	"CREATE_FOLDER":            true,
	"DELETE_FILE":              true,
	"DELETE_FOLDER":            true,
	"START_CONNECT_DOWNLOAD":   true,
	"START_ENCRYPTED_DOWNLOAD": true,
	"START_INLINE_DOWNLOAD":    true,
}

// serve answers one frame.
//
// The response delay is taken here and not inside one frame kind, because a
// slow printer is slow whatever it was asked. Many verbs leave as gcode, so a
// delay on the JSON path alone would leave part of the command surface unable
// to reach a host-side timeout.
func (p *Printer) serve(ctx context.Context, conn *websocket.Conn, f frame) error {
	switch f.kind {
	case frameJSON:
		p.waitResponseDelay(ctx)

		return p.serveJSON(ctx, conn, f)
	case frameGCode, frameForced:
		p.waitResponseDelay(ctx)

		return p.serveGCode(conn, f)
	case frameTransfer:
		// No response delay: a chunk is not a command. Delaying every chunk
		// would make the "answer slowly" fault throttle transfers too.
		return p.takeTransferChunk(conn, f)
	case frameDebug:
		// A debug frame is one-way.
		return nil
	default:
		return nil
	}
}

// serveJSON runs one high-level command through the lifecycle: validate against
// the model's projected vocabulary, then answer with exactly one terminal event
// carrying the command id it arrived with.
func (p *Printer) serveJSON(ctx context.Context, conn *websocket.Conn, f frame) error {
	command, err := decodeJSONCommand(f.payload)
	if err != nil {
		p.logBadFrame(ctx, err)

		return p.reject(conn, f.id, ReasonMalformedCommand)
	}

	toolKey := false
	if command.Command == setValueCommand {
		command.Kwargs, toolKey = toolSetterKwargs(command.Kwargs, p.cfg.ToolTree)
	}

	definition, ok := p.resolve(command)
	if !ok {
		p.logRefusal(ctx, command.Command, ReasonUnsupportedCommand)

		return p.reject(conn, f.id, ReasonUnsupportedCommand)
	}

	// The firmware reads a tool setter under its tools.<k> key and nothing
	// else, so the setter's names sent as flat kwargs are a frame missing it.
	if strings.HasPrefix(definition.Name, toolSetterPrefix) && !toolKey {
		p.logRefusal(ctx, command.Command, ReasonMissingArgument)

		return p.reject(conn, f.id, ReasonMissingArgument)
	}

	if reason, valid := checkArguments(definition, command.Kwargs); !valid {
		p.logRefusal(ctx, command.Command, reason)

		return p.reject(conn, f.id, reason)
	}

	if reason, allowed := p.checkStorage(command); !allowed {
		p.logRefusal(ctx, command.Command, reason)

		return p.reject(conn, f.id, reason)
	}

	return p.dispatch(ctx, conn, f.id, jsonCommand{Command: definition.Name, Kwargs: command.Kwargs})
}

// waitListing takes ListingDelay when the path names a folder. A file's own
// details are one read of its header and answer at once.
func (p *Printer) waitListing(ctx context.Context, path string) {
	if node, found := p.fileNode(path); found && node.Folder {
		wait(ctx, p.cfg.ListingDelay)
	}
}

// resolve names the verb a J frame is asking for. Everything but SET_VALUE is
// the name it carries.
func (p *Printer) resolve(command jsonCommand) (Command, bool) {
	// The profiles name a flash as FLASH and UPGRADE, and the firmware has no
	// JSON command for either (src/connect/command.cpp): it flashes on the G-code
	// M997, which gcodeAnswer records.
	if command.Command == "FLASH" || command.Command == "UPGRADE" {
		return Command{}, false
	}

	if command.Command == setValueCommand {
		return p.model.verbForArguments(command.Kwargs)
	}

	return p.model.Command(command.Command)
}

// serveGCode answers a G or F frame. Gcode is not in the capability vocabulary,
// so the framing decides it and the model does not.
//
// A long move is the exception: the machine reports BUSY while it runs, so the
// payload's first word is read. See beginBusy.
//
// Both events carry the line's reply under AnswerKey, which real firmware does
// not send. AnswerKey says why it is here and what a host may assume about it.
//
// The reply is on both because a host settles a command on the first event
// that carries its id, and that is ACCEPTED. A host that waits for the
// completion reads the reply on FINISHED.
func (p *Printer) serveGCode(conn *websocket.Conn, f frame) error {
	// M997 resets a real board within milliseconds of reading it, before any
	// event can leave, so it is recorded and answered with nothing.
	if isFlash(f.payload) {
		_ = p.gcodeAnswer(f.payload)

		return nil
	}

	p.beginBusy(f.payload)
	p.noteToolTarget(f.payload)
	p.noteChamberTarget(f.payload)
	p.noteMetricsConfig(f.payload)

	answer := map[string]any{AnswerKey: p.gcodeAnswer(f.payload)}

	if err := p.send(conn, event{
		Event: eventAccepted, CommandID: &f.id, State: p.State(), Data: answer,
	}); err != nil {
		return err
	}

	return p.send(conn, event{
		Event: eventFinished, CommandID: &f.id, State: p.State(), Data: answer,
	})
}

// dispatch runs a validated command. Every verb the model declares reaches a
// terminal event, so a command that arrives is a command that completes.
func (p *Printer) dispatch(ctx context.Context, conn *websocket.Conn, id uint32, command jsonCommand) error {
	p.noteAsked(command.Command)

	p.mu.Lock()
	acceptFirst := p.acceptFirst
	p.mu.Unlock()

	if acceptFirst {
		if err := p.send(conn, event{Event: eventAccepted, CommandID: &id, State: p.State()}); err != nil {
			return err
		}
	}

	if reason, refused := p.runStateVerb(id, command); refused {
		return p.reject(conn, id, reason)
	}

	switch command.Command {
	case "SEND_INFO":
		return p.send(conn, event{Event: eventInfo, CommandID: &id, State: p.State(), Data: p.infoData()})
	case "SEND_JOB_INFO":
		return p.send(conn, event{Event: eventJobInfo, CommandID: &id, State: p.State(), Data: p.jobData()})
	case "SEND_FILE_INFO":
		p.waitListing(ctx, pathArgument(command.Kwargs))

		data, found := p.fileInfoData(pathArgument(command.Kwargs))
		if !found {
			return p.reject(conn, id, ReasonFileNotFound)
		}

		return p.send(conn, event{Event: eventFileInfo, CommandID: &id, State: p.State(), Data: data})
	case "CREATE_FOLDER":
		if !p.createFolder(pathArgument(command.Kwargs)) {
			return p.reject(conn, id, ReasonFileExists)
		}
	case "DELETE_FILE":
		if !p.removeEntry(pathArgument(command.Kwargs), false) {
			return p.reject(conn, id, ReasonFileNotFound)
		}
	case "DELETE_FOLDER":
		if !p.removeEntry(pathArgument(command.Kwargs), true) {
			return p.reject(conn, id, ReasonFileNotFound)
		}
	case "SEND_TRANSFER_INFO":
		return p.send(conn, event{
			Event: eventTransferInfo, CommandID: &id, State: p.State(), Data: p.transferInfoData(),
		})
	case "START_INLINE_DOWNLOAD", "START_CONNECT_DOWNLOAD":
		return p.startTransfer(conn, id, command)
	case "STOP_TRANSFER":
		if err := p.stopTransfer(conn); err != nil {
			return err
		}
	case "SEND_STATE_INFO":
		return p.send(conn, event{Event: eventStateChanged, CommandID: &id, State: p.State()})
	case chamberSetter, "SET_CHAMBER_FAN_TARGET", "SET_CHAMBER_LED_INTENSITY":
		p.mu.Lock()
		p.setChamber(command.Command, command.Kwargs)
		p.mu.Unlock()
	}

	if err := p.send(conn, event{Event: eventFinished, CommandID: &id, State: p.State()}); err != nil {
		return err
	}

	if strings.HasPrefix(command.Command, toolSetterPrefix) {
		return p.announceTools(conn)
	}

	return p.announceJob(conn, command.Command)
}

// announceTools volunteers an INFO after a tool setter, with no command id,
// so the host learns the new nozzle without asking. The numbered shape
// carries the nozzles in INFO alone, and the host does not ask again after a
// setter it sent.
func (p *Printer) announceTools(conn *websocket.Conn) error {
	return p.send(conn, event{Event: eventInfo, State: p.State(), Data: p.infoData()})
}

// announceJob volunteers the print after START_PRINT, and after nothing else.
//
// That is the firmware's cadence. A JOB_INFO arrives unasked when a print
// starts. None arrives after PAUSE_PRINT, RESUME_PRINT or STOP_PRINT, or when
// the print reaches its own end. Those are state changes, and the machine
// reports them at /state on every telemetry frame. A host that wants the
// print's own word for them sends SEND_JOB_INFO.
//
// It carries no command id. The command has already been answered with its
// terminal event, and the machine sends this whether or not anybody asked.
func (p *Printer) announceJob(conn *websocket.Conn, command string) error {
	if command != "START_PRINT" {
		return nil
	}

	return p.send(conn, event{Event: eventJobInfo, State: p.State(), Data: p.jobData()})
}

// noteAsked counts one command the host sent. See Printer.Asked.
func (p *Printer) noteAsked(command string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.asked[command]++
}

// runStateVerb answers the verbs that move the printer's own state, and reports
// the refusal when the state does not allow the move. A verb outside this set
// is neither run nor refused here.
//
// Each of these needs a state to act on: a print needs a free bed and a file
// the storage holds, a pause needs a running job, a resume needs a paused one,
// ready and idle need nothing on the bed, and a dialog reply needs a question
// on the display. A machine answers every one of them with REJECTED when the
// condition is not met.
func (p *Printer) runStateVerb(id uint32, command jsonCommand) (reason string, refused bool) {
	switch command.Command {
	case "START_PRINT":
		// The job keeps the long-name path whichever spelling started it.
		path, found := p.printable(pathArgument(command.Kwargs))
		if !found {
			return ReasonFileNotFound, true
		}

		if !p.startJob(id, path) {
			return ReasonInvalidState, true
		}
	case "PAUSE_PRINT":
		if !p.setPaused(true) {
			return ReasonInvalidState, true
		}
	case "RESUME_PRINT":
		if !p.setPaused(false) {
			return ReasonInvalidState, true
		}
	case "STOP_PRINT":
		if !p.stopJob(stateStopped) {
			return ReasonInvalidState, true
		}
	case "SET_PRINTER_READY":
		if !p.setIdleState(stateReady) {
			return ReasonInvalidState, true
		}
	case "CANCEL_PRINTER_READY", "SET_IDLE":
		if !p.setIdleState(stateIdle) {
			return ReasonInvalidState, true
		}
	case "DIALOG_ACTION":
		if answerReason, answered := p.answerDialog(command.Kwargs); !answered {
			return answerReason, true
		}
	case "SET_TOOL_NOZZLE_DIAMETER", "SET_TOOL_HIGH_FLOW", "SET_TOOL_HARDENED":
		return p.setNozzle(command)
	}

	return "", false
}

// reject answers with the one terminal event that is not a success.
func (p *Printer) reject(conn *websocket.Conn, id uint32, reason string) error {
	return p.send(conn, event{Event: eventRejected, CommandID: &id, State: p.State(), Reason: reason})
}

func (p *Printer) send(conn *websocket.Conn, message event) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode %s: %w", message.Event, err)
	}

	return p.writeText(conn, payload)
}

// waitResponseDelay is the "answer slowly" fault. It is taken in the read
// goroutine, so a slow printer also has one command in flight. That makes a
// host-side timeout and a breaker reachable.
func (p *Printer) waitResponseDelay(ctx context.Context) {
	wait(ctx, p.ResponseDelay())
}

// wait blocks for the delay or until the context ends.
func wait(ctx context.Context, delay time.Duration) {
	if delay <= 0 {
		return
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (p *Printer) logRefusal(ctx context.Context, command, reason string) {
	if p.log == nil {
		return
	}

	p.log.InfoContext(ctx, "simulated printer rejected a command",
		slog.String("fingerprint", p.cfg.Fingerprint),
		slog.String("model", p.model.Name),
		slog.String("firmware", p.cfg.Firmware),
		slog.String("command", command),
		slog.String("capability", CapabilityKeyFor(command).String()),
		slog.String("reason", reason),
		slog.Bool("reality_unknown", true))
}

// checkArguments holds a command to exactly the arguments its model declares.
//
// Presence is the declaration: a verb accepts the arguments for which a
// capability key exists and no others, so an unknown argument is refused.
func checkArguments(definition Command, kwargs map[string]any) (reason string, valid bool) {
	for name, argument := range definition.Arguments {
		if !argument.Required {
			continue
		}

		if _, present := kwargs[name]; !present {
			return ReasonMissingArgument, false
		}
	}

	for name, value := range kwargs {
		argument, ok := definition.Arguments[name]
		if !ok {
			return ReasonUnknownArgument, false
		}

		if !matchesKind(argument.Kind, value) {
			return ReasonInvalidArgument, false
		}
	}

	return "", true
}

// matchesKind reads an argument the way the profile says it travels. JSON gives
// every number as a float64, so an integer is a float64 with nothing after the
// point.
//
// KindFlag is a key that carries no value, so a J frame that supplies a value
// for it is refused, like an argument of the wrong kind.
func matchesKind(kind connectorapi.Kind, value any) bool {
	switch kind {
	case connectorapi.KindString:
		_, ok := value.(string)

		return ok
	case connectorapi.KindBoolean:
		_, ok := value.(bool)

		return ok
	case connectorapi.KindNumber:
		_, ok := value.(float64)

		return ok
	case connectorapi.KindInteger:
		number, ok := value.(float64)

		return ok && number == math.Trunc(number)
	case connectorapi.KindOpaque:
		return true
	case connectorapi.KindFlag:
		return false
	default:
		return false
	}
}

// pathArgument reads the one argument every path-bearing verb carries.
func pathArgument(kwargs map[string]any) string {
	value, _ := kwargs["path"].(string)

	return value
}
