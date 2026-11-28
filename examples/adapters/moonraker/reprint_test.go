package moonraker_test

import (
	"testing"
	"time"

	"example.com/prusa/moonraker/moonrakersim"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func nextJobID(t *testing.T, session connectorapi.Session) string {
	t.Helper()

	deadline := time.NewTimer(settle)
	defer deadline.Stop()

	for {
		select {
		case patch, open := <-session.State():
			if !open {
				t.Fatal("the state channel closed before a job id arrived")
			}

			if patch.Path == "/job/id" {
				return string(patch.Value)
			}
		case <-deadline.C:
			t.Fatal("no job id arrived")
		}
	}
}

// Moonraker sends no filename for a reprint of the file that just finished, so
// the session that watched the first print has to remember the name.
func TestAReprintOfTheSameFileHasOneIDAcrossAReconnect(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	machine.Push(map[string]map[string]any{
		"print_stats": {"state": "printing", "filename": "bracket.gcode", "total_duration": 1.0},
	})
	first := nextJobID(t, session)

	machine.Push(map[string]map[string]any{"print_stats": {"state": "complete", "total_duration": 8.0}})
	machine.Push(map[string]map[string]any{"print_stats": {"state": "printing", "total_duration": 2.0}})
	reprint := nextJobID(t, session)

	reconnected := nextJobID(t, connect(t, machine))

	if reprint != reconnected {
		t.Errorf("one print has the id %s here and %s after a reconnect", reprint, reconnected)
	}

	if reprint == first {
		t.Errorf("the reprint has the id %s of the print before it", first)
	}
}
