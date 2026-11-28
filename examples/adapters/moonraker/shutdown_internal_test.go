package moonraker

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func stateOf(t *testing.T, patches []connectorapi.StatePatch) string {
	t.Helper()

	for _, patch := range patches {
		if patch.Path != statePath {
			continue
		}

		var word string
		if err := json.Unmarshal(patch.Value, &word); err != nil {
			t.Fatalf("/state is not text: %v", err)
		}

		return word
	}

	return ""
}

// print_stats changes only when a print starts, pauses, ends or fails. M112 on
// an idle machine, or an MCU that cannot connect, leaves it at "standby", so
// the machine's own state is read from webhooks.state.
func TestAShutDownKlipperIsReportedAsAnError(t *testing.T) {
	t.Parallel()

	for _, hook := range []string{"shutdown", "error"} {
		t.Run(hook, func(t *testing.T) {
			t.Parallel()

			patches := (&reader{}).patchesFrom(reading(`{
				"print_stats": {"state": "standby", "filename": ""},
				"webhooks": {"state": "`+hook+`", "state_message": "Printer is shutdown"}
			}`, t), time.Unix(0, 0))

			if got := stateOf(t, patches); got != "ERROR" {
				t.Errorf("webhooks.state %q beside an idle print_stats reported %q, want ERROR", hook, got)
			}
		})
	}
}

// Moonraker sends only what changed, so the state is worked out from the last
// print_stats and the last webhooks the machine sent, not from one frame.
func TestTheMachineStateSurvivesFramesThatCarryOnlyOneObject(t *testing.T) {
	t.Parallel()

	held := &reader{}
	now := time.Unix(0, 0)

	steps := []struct {
		name, frame, want string
	}{
		{"idle", `{"print_stats": {"state": "standby"}, "webhooks": {"state": "ready"}}`, "IDLE"},
		{"M112 sends webhooks alone", `{"webhooks": {"state": "shutdown"}}`, "ERROR"},
		{"print_stats alone does not clear it", `{"print_stats": {"state": "standby"}}`, "ERROR"},
		{"a restart sends webhooks alone", `{"webhooks": {"state": "ready"}}`, "IDLE"},
		{"starting up is not an error", `{"webhooks": {"state": "startup"}}`, "IDLE"},
		{
			"a print",
			`{"webhooks": {"state": "ready"}, "print_stats": {"state": "printing", "filename": "a.gcode"}}`,
			"PRINTING",
		},
		{"a shutdown during the print", `{"webhooks": {"state": "shutdown"}}`, "ERROR"},
	}

	for _, step := range steps {
		got := stateOf(t, held.patchesFrom(reading(step.frame, t), now))
		if got != step.want {
			t.Errorf("%s: reported %q, want %q", step.name, got, step.want)
		}
	}
}

// A machine whose first reading is a shutdown has no print state to fall back
// on when it recovers. It is not printing, so it is IDLE.
func TestARecoveryWithNoPrintStateIsIdle(t *testing.T) {
	t.Parallel()

	held := &reader{}
	now := time.Unix(0, 0)

	if got := stateOf(t, held.patchesFrom(reading(`{"webhooks": {"state": "shutdown"}}`, t), now)); got != "ERROR" {
		t.Fatalf("a shutdown reported %q, want ERROR", got)
	}

	if got := stateOf(t, held.patchesFrom(reading(`{"webhooks": {"state": "ready"}}`, t), now)); got != "IDLE" {
		t.Errorf("a recovery with no print state reported %q, want IDLE", got)
	}

	if got := stateOf(t, held.patchesFrom(reading(`{"webhooks": {"state": "shutdown"}}`, t), now)); got != "ERROR" {
		t.Errorf("a second shutdown reported %q, want ERROR", got)
	}
}

// A ready machine that has reported no print state and no error says nothing
// about whether it is printing, so nothing is written.
func TestAReadyMachineWithNoPrintStateWritesNoState(t *testing.T) {
	t.Parallel()

	patches := (&reader{}).patchesFrom(reading(`{"webhooks": {"state": "ready"}}`, t), time.Unix(0, 0))

	if got := stateOf(t, patches); got != "" {
		t.Errorf("a ready machine with no print state reported %q, want nothing", got)
	}
}
