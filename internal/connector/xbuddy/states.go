package xbuddy

// The states a Buddy board reports, the moves between them, and the verbs each
// one accepts. The host serves them in the device-state vocabulary, so a client
// learns them from the server and not from this file.
//
// Every rule below is the firmware's own, read from the public
// Prusa-Firmware-Buddy source, except the flash row, which says so where it
// stands. The words are
// to_str(DeviceState) in src/state/printer_state.cpp, which this driver passes
// through unchanged. The guards are where the firmware rejects a command:
//
//   - start_print and set_printer_ready need remote_print_ready, which is true
//     in IDLE, READY, STOPPED and FINISHED (src/state/printer_state.cpp)
//   - pause_print needs PRINTING, resume_print needs PAUSED, and stop_print
//     needs PRINTING, PAUSED or ATTENTION (MarlinPrinter::job_control in
//     src/connect/marlin_printer.cpp)
//   - set_idle needs FINISHED or STOPPED (MarlinPrinter::set_idle)
//   - in ERROR the planner rejects every command but four
//     (command_is_error_whitelisted in src/connect/planner.cpp)
//
// Every other verb is accepted in every state but ERROR. The firmware may still
// refuse one for a reason no state names, such as a transfer already running,
// and its answer to the command is what decides.

import (
	"slices"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The words this file adds to the ones the job and dialog code already names.
const (
	stateIdle    = "IDLE"
	stateBusy    = "BUSY"
	stateReady   = "READY"
	stateUnknown = "UNKNOWN"
)

// stateGuards is the states each guarded verb is accepted in. A verb with no
// row is accepted in every state but ERROR.
//
//nolint:gochecknoglobals // descriptor data, read-only after init.
var stateGuards = map[connectorapi.Capability][]connectorapi.StateName{
	VerbStartPrint:      {stateIdle, stateReady, stateStopped, stateFinished},
	VerbSetPrinterReady: {stateIdle, stateReady, stateStopped, stateFinished},
	VerbPausePrint:      {statePrinting},
	VerbResumePrint:     {statePaused},
	VerbStopPrint:       {statePrinting, statePaused, stateAttention},
	VerbSetIdle:         {stateFinished, stateStopped},
	VerbSendInfo:        everyState(),
	VerbSendStateInfo:   everyState(),
	VerbSetToken:        everyState(),
	VerbResetPrinter:    everyState(),
	// This driver's rule and not the firmware's: M997 runs in every state and
	// resets the board, so a flash takes START_PRINT's states, as it does in
	// Prusa Connect. Session.refuseInState holds the two to it.
	VerbFlash:   {stateIdle, stateReady, stateStopped, stateFinished},
	VerbUpgrade: {stateIdle, stateReady, stateStopped, stateFinished},
}

// everyState is the guard of a verb the firmware accepts even in ERROR.
func everyState() []connectorapi.StateName {
	return []connectorapi.StateName{
		stateIdle, stateBusy, stateReady, statePrinting, statePaused, stateFinished,
		stateStopped, stateAttention, stateError, stateUnknown,
	}
}

// stateNames is every state, with the words a client shows for each, in the
// order a person reads them.
//
//nolint:gochecknoglobals // descriptor data, read-only after init.
var stateNames = []connectorapi.DeviceState{
	{Name: stateIdle, DisplayName: "Idle"},
	{Name: stateBusy, DisplayName: "Busy"},
	{Name: stateReady, DisplayName: "Ready"},
	{Name: statePrinting, DisplayName: "Printing"},
	{Name: statePaused, DisplayName: "Paused"},
	{Name: stateFinished, DisplayName: "Finished"},
	{Name: stateStopped, DisplayName: "Stopped"},
	{Name: stateAttention, DisplayName: "Needs attention"},
	{Name: stateError, DisplayName: "Error"},
	{Name: stateUnknown, DisplayName: "Unknown"},
}

// stateTransitions is every move a Buddy board makes. A move a caller causes
// names its verb, and the rest the machine makes on its own.
//
//nolint:gochecknoglobals // descriptor data, read-only after init.
var stateTransitions = []connectorapi.Transition{
	{From: stateIdle, To: statePrinting, Verb: VerbStartPrint},
	{From: stateReady, To: statePrinting, Verb: VerbStartPrint},
	{From: stateFinished, To: statePrinting, Verb: VerbStartPrint},
	{From: stateStopped, To: statePrinting, Verb: VerbStartPrint},
	{From: stateIdle, To: stateReady, Verb: VerbSetPrinterReady},
	{From: stateFinished, To: stateReady, Verb: VerbSetPrinterReady},
	{From: stateStopped, To: stateReady, Verb: VerbSetPrinterReady},
	{From: stateReady, To: stateIdle, Verb: VerbCancelPrinterReady},
	{From: statePrinting, To: statePaused, Verb: VerbPausePrint},
	{From: statePaused, To: statePrinting, Verb: VerbResumePrint},
	{From: statePrinting, To: stateStopped, Verb: VerbStopPrint},
	{From: statePaused, To: stateStopped, Verb: VerbStopPrint},
	{From: stateAttention, To: stateStopped, Verb: VerbStopPrint},
	{From: stateFinished, To: stateIdle, Verb: VerbSetIdle},
	{From: stateStopped, To: stateIdle, Verb: VerbSetIdle},
	{From: stateFinished, To: stateIdle},
	{From: stateStopped, To: stateIdle},
	{From: stateIdle, To: stateBusy},
	{From: stateBusy, To: stateIdle},
	{From: statePrinting, To: stateFinished},
	{From: statePrinting, To: stateIdle},
	{From: statePrinting, To: stateAttention},
	{From: stateAttention, To: statePrinting},
	{From: stateIdle, To: stateAttention},
	{From: stateAttention, To: stateIdle},
	{From: statePrinting, To: stateError},
	{From: stateIdle, To: stateError},
	{From: stateError, To: stateIdle, Verb: VerbResetPrinter},
	{From: stateUnknown, To: stateIdle},
}

// states is this dialect's declared state set. The accepted verbs are worked
// out from the published baseline, so a verb added to the wire table is offered
// in every state but ERROR until a row above says otherwise.
func states() connectorapi.StateSet {
	verbs := published().Verbs()

	declared := make([]connectorapi.DeviceState, 0, len(stateNames))

	for _, state := range stateNames {
		state.Accepts = make([]connectorapi.Capability, 0, len(verbs))

		for _, verb := range verbs {
			if acceptedIn(verb, state.Name) {
				state.Accepts = append(state.Accepts, verb)
			}
		}

		declared = append(declared, state)
	}

	return connectorapi.StateSet{States: declared, Transitions: slices.Clone(stateTransitions)}
}

// acceptedIn reports whether the firmware accepts verb in state.
func acceptedIn(verb connectorapi.Capability, state connectorapi.StateName) bool {
	if guard, guarded := stateGuards[verb]; guarded {
		return slices.Contains(guard, state)
	}

	return state != stateError
}

// frameNow is the frame pending goes out as on this attempt on current. A
// chamber target of 0 takes its M141 S0 frame only if the printer is quiescent
// now, not when the command was queued: a printer that was away may be
// printing. Right after a reconnection it waits for the printer's state first,
// so an idle printer is not taken for UNKNOWN.
func (s *Session) frameNow(pending *call, current *link) []byte {
	if pending.chamberOff == nil {
		return pending.frame
	}

	s.awaitState(current)

	pending.gcode = s.quiescent()
	if pending.gcode {
		return pending.chamberOff
	}

	return pending.frame
}

// heardState is the state the printer reported on its current connection, and
// UNKNOWN before it has reported one there.
func (s *Session) heardState() connectorapi.StateName {
	if now := s.machineState.Load(); now != nil && s.stateHeard.Load() {
		return connectorapi.StateName(*now)
	}

	return stateUnknown
}

// quiescent reports whether the printer is in a state Start print is accepted
// in: IDLE, READY, STOPPED or FINISHED. No print, load, selftest or recovery
// runs then, so a G-code line does not wait behind one.
func (s *Session) quiescent() bool {
	return acceptedIn(VerbStartPrint, s.heardState())
}

// awaitState waits until the printer has reported its state on current, for
// at most the command deadline. A printer sends one in its first telemetry
// frame, within seconds of connecting.
func (s *Session) awaitState(current *link) {
	overdue := time.NewTimer(s.commandDeadline)
	defer overdue.Stop()

	select {
	case <-current.stateIn:
	case <-current.gone:
	case <-s.done:
	case <-overdue.C:
	}
}
