package xbuddy_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prusa3d/prusa-connect/internal/connector/xbuddy"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// commandDeadline is what these cases configure. It is well under the 60s read
// deadline, so a command that ends at this number ended on its own deadline and
// not because the connection was dropped.
const commandDeadline = 10 * time.Second

// settleAllowance is how much past the deadline a settled command may be. The
// bubble's clock moves in one step, so the real figure is the deadline itself.
const settleAllowance = time.Second

// A printer that takes the frame, keeps its connection healthy and answers
// nothing is the case nothing else here catches.
//
// The read deadline notices silence, and this printer is not silent: it reports
// telemetry four times a second and re-arms the deadline with every frame. So
// the command's own deadline is the only thing that ends the wait, and without
// it the pump holds that one command until whoever sent it gives up, with every
// command behind it waiting too.
func TestACommandThePrinterNeverAnswersEndsAtItsDeadline(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration, withCommandDeadline)
		defer rig.stop(t)

		session := rig.connect(t)

		// The storage request a new connection makes is on the wire at this
		// point, and the pump carries one command at a time.
		time.Sleep(2 * time.Second)
		synctest.Wait()

		rig.printer.SetResponseDelay(time.Hour)

		started := time.Now()

		_, err := session.Do(rig.ctx, connectorapi.Command{Verb: xbuddy.VerbSetPrinterReady})
		if err == nil {
			t.Fatal("a printer that answered nothing for an hour reported the command carried out")
		}

		waited := time.Since(started)

		t.Logf("observed: the command ended after %v with %v", waited, err)

		if waited < commandDeadline || waited > commandDeadline+settleAllowance {
			t.Errorf("the command waited %v, want about %v", waited, commandDeadline)
		}

		// The host reads this to record the outcome as timed out rather than as
		// an unexplained failure. See settleStatus in internal/app.
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("the failure is %v, and a host reads a deadline off it", err)
		}

		if !errors.Is(err, xbuddy.ErrCommandDeadline) {
			t.Errorf("the failure is %v, want the command deadline", err)
		}

		var refusal *fault.Fault
		if !errors.As(err, &refusal) {
			t.Fatalf("the failure is %v, and a caller reads a code", err)
		}

		// Unanswered and never failed: the frame is on the wire, so this side
		// cannot say whether the machine ran the command.
		if got, want := refusal.Code().String(), connectorapi.CommandUnanswered.Code().String(); got != want {
			t.Errorf("the failure carries %s, want %s", got, want)
		}

		// The connection is still the one the printer dialled. A case that ended
		// because the socket went would prove nothing about the deadline.
		if got := rig.printer.Sessions(); got != 1 {
			t.Errorf("the printer opened %d websocket sessions, so the connection went and the "+
				"deadline is not what ended the command", got)
		}
	})
}

// The control. A printer that is slow and still answers inside the deadline is
// not touched by it, which is what says the bound is a deadline and not a
// throttle.
func TestACommandAnsweredInsideItsDeadlineIsUntouched(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration, withCommandDeadline)
		defer rig.stop(t)

		session := rig.connect(t)

		time.Sleep(2 * time.Second)
		synctest.Wait()

		rig.printer.SetResponseDelay(commandDeadline / 2)

		started := time.Now()

		result, err := session.Do(rig.ctx, connectorapi.Command{Verb: xbuddy.VerbSetPrinterReady})
		if err != nil {
			t.Fatalf("a printer that answered inside the deadline failed the command: %v", err)
		}

		waited := time.Since(started)

		t.Logf("observed: the command was answered after %v, ok=%v", waited, result.OK)

		if !result.OK {
			t.Errorf("the printer refused the command: %s", result.Raw)
		}

		if waited >= commandDeadline {
			t.Errorf("the command waited %v, which is its whole %v deadline", waited, commandDeadline)
		}

		if got, want := rig.printer.State(), "READY"; got != want {
			t.Errorf("the printer is %q after the command, want %q", got, want)
		}
	})
}

// A command deadline the connector cannot run on is refused at startup, where
// an operator is reading the error, rather than at the first printer.
func TestAnUnusableCommandDeadlineIsRefusedAtStartup(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		config string
	}{
		{"a deadline nothing can parse", `{"command_deadline":"in a bit"}`},
		{"a deadline of zero, which fails every command before it is sent", `{"command_deadline":"0s"}`},
		{"a negative deadline", `{"command_deadline":"-5s"}`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := xbuddy.New(connectorapi.Spec{
				Ref: "printer-1", Type: xbuddy.Type, Config: json.RawMessage(testCase.config),
			})
			if err == nil {
				t.Fatalf("%s was accepted, and every command on that connector would run on it",
					testCase.config)
			}

			if !strings.Contains(err.Error(), "command_deadline") {
				t.Errorf("the refusal is %q and names no key an operator can correct", err)
			}

			t.Logf("observed refusal: %v", err)
		})
	}
}

// A block that names the key builds, and one that names none builds too. A
// deployment that never heard of this key keeps the driver's own default.
func TestACommandDeadlineAnOperatorCanWriteIsAccepted(t *testing.T) {
	t.Parallel()

	for _, block := range []string{`{}`, `{"command_deadline":"25s"}`, `{"command_deadline":"5m"}`} {
		t.Run(block, func(t *testing.T) {
			t.Parallel()

			if _, err := xbuddy.New(connectorapi.Spec{
				Ref: "printer-1", Type: xbuddy.Type, Config: json.RawMessage(block),
			}); err != nil {
				t.Errorf("the config block %s was refused: %v", block, err)
			}
		})
	}
}
