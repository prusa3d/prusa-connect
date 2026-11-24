package moonraker

// The states this dialect reports at /state, the moves between them, and the
// verbs each one accepts. The words are the values of stateWords in
// telemetry.go. A word Klipper adds that the table does not carry still reaches
// the tree in upper case, and a client then finds no entry for it here.
//
// The guards follow Klipper's own pause_resume section: PAUSE does nothing to a
// print that is already paused, and RESUME does nothing to one that is not.
// Neither is an error on the wire, so a guard is the only place a client can
// learn it. A heater target and a status read are accepted in every state.

import (
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The state words, as stateWords writes them.
const (
	stateIdle     connectorapi.StateName = "IDLE"
	statePrinting connectorapi.StateName = "PRINTING"
	statePaused   connectorapi.StateName = "PAUSED"
	stateFinished connectorapi.StateName = "FINISHED"
	stateStopped  connectorapi.StateName = "STOPPED"
	stateError    connectorapi.StateName = "ERROR"
)

// states is this dialect's declared state set. Each idle and terminal state
// declares two transitions to PRINTING: one with VerbStartPrint for prints
// started through Prusa Connect, and one without a verb for prints started
// externally from Mainsail, Fluidd, or the printer screen.
func states() connectorapi.StateSet {
	base := []connectorapi.Capability{VerbSendStateInfo, VerbNozzleTarget, VerbBedTarget}
	idle := append([]connectorapi.Capability{VerbStartPrint}, base...)

	return connectorapi.StateSet{
		States: []connectorapi.DeviceState{
			{Name: stateIdle, DisplayName: "Idle", Accepts: idle},
			{Name: statePrinting, DisplayName: "Printing", Accepts: append(
				[]connectorapi.Capability{VerbPausePrint, VerbStopPrint}, base...)},
			{Name: statePaused, DisplayName: "Paused", Accepts: append(
				[]connectorapi.Capability{VerbResumePrint, VerbStopPrint}, base...)},
			{Name: stateFinished, DisplayName: "Finished", Accepts: idle},
			{Name: stateStopped, DisplayName: "Stopped", Accepts: idle},
			{Name: stateError, DisplayName: "Error", Accepts: idle},
		},
		Transitions: []connectorapi.Transition{
			{From: stateIdle, To: statePrinting},
			{From: stateIdle, To: statePrinting, Verb: VerbStartPrint},
			{From: stateFinished, To: statePrinting},
			{From: stateFinished, To: statePrinting, Verb: VerbStartPrint},
			{From: stateStopped, To: statePrinting},
			{From: stateStopped, To: statePrinting, Verb: VerbStartPrint},
			{From: stateError, To: statePrinting},
			{From: stateError, To: statePrinting, Verb: VerbStartPrint},
			{From: statePrinting, To: statePaused, Verb: VerbPausePrint},
			{From: statePaused, To: statePrinting, Verb: VerbResumePrint},
			{From: statePrinting, To: stateStopped, Verb: VerbStopPrint},
			{From: statePaused, To: stateStopped, Verb: VerbStopPrint},
			{From: statePrinting, To: stateFinished},
			{From: statePrinting, To: stateError},
			{From: stateFinished, To: stateIdle},
			{From: stateStopped, To: stateIdle},
			{From: stateError, To: stateIdle},
		},
	}
}
