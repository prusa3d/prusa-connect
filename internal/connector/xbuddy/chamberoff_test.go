package xbuddy

import (
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// A chamber target of 0 goes as M141 S0 only while the printer is quiescent as
// it is sent, so the G-code never waits behind a print, a load, a selftest or a
// heating wait and holds Stop. Any other target goes as SET_VALUE. See
// chamberOffForm and Session.frameNow.
func TestAChamberTargetOfZeroGoesAsM141OnlyWhenThePrinterIsQuiescent(t *testing.T) {
	t.Parallel()

	const setValue0 = `{"command":"SET_VALUE","kwargs":{"chamber.target_temp":0}}`

	for _, tc := range []struct {
		state, target string
		heard         bool
		want          string
	}{
		{state: stateIdle, target: "0", heard: true, want: "M141 S0"},
		{state: stateIdle, target: "45", heard: true,
			want: `{"command":"SET_VALUE","kwargs":{"chamber.target_temp":45}}`},
		{state: statePrinting, target: "0", heard: true, want: setValue0},
		{state: statePaused, target: "0", heard: true, want: setValue0},
		{state: stateAttention, target: "0", heard: true, want: setValue0},
		{state: stateBusy, target: "0", heard: true, want: setValue0},
		{state: stateUnknown, target: "0", heard: true, want: setValue0},
		{state: stateError, target: "0", heard: true, want: setValue0},
		// A state from before the printer reconnected may be stale.
		{state: stateIdle, target: "0", heard: false, want: setValue0},
	} {
		name := tc.state + "/" + tc.target
		if !tc.heard {
			name += "/stale"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := sessionIn(stateIdle)

			pending := chamberCall(t, session, tc.target)

			// The state as the command is sent decides, not the one it was
			// queued in.
			state := tc.state
			session.machineState.Store(&state)
			session.stateHeard.Store(tc.heard)

			reported := &link{stateIn: make(chan struct{})}
			close(reported.stateIn)

			frame := session.frameNow(pending, reported)

			if got := string(frame[headerLength:]); got != tc.want {
				t.Errorf("renders %q, want %q", got, tc.want)
			}

			if pending.gcode != (tc.want == "M141 S0") {
				t.Errorf("holds the queue: %v, want %v", pending.gcode, !pending.gcode)
			}
		})
	}
}

// sessionIn is a session whose printer reported state on this connection.
func sessionIn(state string) *Session {
	session := &Session{}
	session.machineState.Store(&state)
	session.stateHeard.Store(true)

	return session
}

// chamberCall builds a chamber target command.
func chamberCall(t *testing.T, session *Session, target string) *call {
	t.Helper()

	pending, err := session.build(connectorapi.Command{
		Verb: VerbSetChamberTemp, Args: map[string]string{"chamber.target_temp": target},
	}, wireTable[VerbSetChamberTemp])
	if err != nil {
		t.Fatalf("build a target of %s: %v", target, err)
	}

	return pending
}
