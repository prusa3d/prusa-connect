package moonraker_test

import (
	"testing"
	"time"

	"example.com/prusa/moonraker/moonrakersim"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func TestAKlipperThatShutsDownIsReportedAsAnErrorThroughASession(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)

	collect(t, session, map[string]string{"/state": `"IDLE"`})

	machine.Push(map[string]map[string]any{"webhooks": {"state": "shutdown", "state_message": "Shutdown"}})
	collect(t, session, map[string]string{"/state": `"ERROR"`})

	machine.Push(map[string]map[string]any{"webhooks": {"state": "ready", "state_message": "Printer is ready"}})
	collect(t, session, map[string]string{"/state": `"IDLE"`})
}

// Moonraker drops a client's subscriptions when Klippy disconnects, so only a
// fresh session clears the ERROR a shutdown left.
func TestAKlipperThatShutsDownIsRecoveredAfterKlippyReconnects(t *testing.T) {
	t.Parallel()

	machine := startMachine(t, moonrakersim.Options{})
	session := connect(t, machine)
	collect(t, session, map[string]string{"/state": `"IDLE"`})

	machine.Push(map[string]map[string]any{"webhooks": {"state": "shutdown"}})
	collect(t, session, map[string]string{"/state": `"ERROR"`})

	machine.Notify("notify_klippy_disconnected")
	awaitClosed(t, session)

	machine.Push(map[string]map[string]any{"webhooks": {"state": "ready"}})
	collect(t, connect(t, machine), map[string]string{"/state": `"IDLE"`})
}

// awaitClosed waits for the state channel to close, which is how a session
// tells the host to redial.
func awaitClosed(t *testing.T, session connectorapi.Session) {
	t.Helper()

	timer := time.NewTimer(settle)
	defer timer.Stop()

	for {
		select {
		case _, open := <-session.State():
			if !open {
				return
			}
		case <-timer.C:
			t.Fatal("session remained open after Klippy disconnected")
		}
	}
}
