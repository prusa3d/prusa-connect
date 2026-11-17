package xbuddy_test

import (
	"testing"
	"testing/synctest"

	"github.com/prusa3d/prusa-connect/internal/connector/xbuddy"
	"github.com/prusa3d/prusa-connect/internal/printer/simulator"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// stateWatch reads a session's patches and remembers the last value of each
// path.
type stateWatch struct {
	rig     *rig
	session interface {
		State() <-chan connectorapi.StatePatch
	}
	seen map[string]string
}

// await reads patches until path reads want.
func (w *stateWatch) await(t *testing.T, path, want string) {
	t.Helper()

	for w.seen[path] != want {
		select {
		case patch, open := <-w.session.State():
			if !open {
				t.Fatal("the state channel closed")
			}

			w.seen[patch.Path] = string(patch.Value)
		case <-w.rig.ctx.Done():
			t.Fatalf("%s never read %s; it reads %q", path, want, w.seen[path])
		}
	}
}

// A CORE One reports its chamber, its fans and its light, and each of the three
// setters comes back in the next frame, as it does from firmware 7.0.0 on.
func TestACoreOneReportsItsChamberAndTakesItsSetters(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "CoreOne", withAutoRegistration)
		defer rig.stop(t)

		session := rig.connect(t)
		watch := &stateWatch{rig: rig, session: session, seen: map[string]string{}}

		watch.await(t, "/temperature/chamber", "25")
		watch.await(t, "/temperature/chamber_target", "0")
		watch.await(t, "/chamber/fan_pwm_target", "-1")
		watch.await(t, "/chamber/led_intensity", "100")

		for _, set := range []struct {
			verb       connectorapi.Capability
			arg, value string
			path, want string
		}{
			{xbuddy.VerbSetChamberTemp, "chamber.target_temp", "45", "/temperature/chamber_target", "45"},
			{xbuddy.VerbSetChamberFan, "chamber.fan_pwm_target", "40", "/chamber/fan_pwm_target", "40"},
			// The firmware keeps the light on a 0 to 255 scale, so 50 comes
			// back as 49.
			{xbuddy.VerbSetChamberLED, "chamber.led_intensity", "50", "/chamber/led_intensity", "49"},
		} {
			result, err := session.Do(rig.ctx, connectorapi.Command{
				Verb: set.verb, Args: map[string]string{set.arg: set.value},
			})
			if err != nil || !result.OK {
				t.Fatalf("the CORE One refused %s=%s: %v %s", set.arg, set.value, err, result.Raw)
			}

			watch.await(t, set.path, set.want)
		}

		// With the fans set by hand the firmware drops a new target and
		// still answers that it finished, so the 45 stands.
		result, err := session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbSetChamberTemp, Args: map[string]string{"chamber.target_temp": "50"},
		})
		if err != nil || !result.OK {
			t.Fatalf("the CORE One refused a target with its fans set by hand: %v %s", err, result.Raw)
		}

		// The light moves the block, so the frame after it carries the target
		// the printer holds.
		if _, err = session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbSetChamberLED, Args: map[string]string{"chamber.led_intensity": "100"},
		}); err != nil {
			t.Fatal(err)
		}

		watch.await(t, "/chamber/led_intensity", "100")

		if target := watch.seen["/temperature/chamber_target"]; target != "45" {
			t.Errorf("the chamber target reads %s after a target sent with the fans set by hand, want 45", target)
		}

		// The chamber has no heater: a target above the air leaves the air
		// where it is.
		watch.await(t, "/temperature/chamber", "25")
	})
}

// An XL with its enclosure on reports the enclosure's air at the chamber
// reading, and nothing at the chamber target.
func TestAnXLWithItsEnclosureReportsTheChamberReading(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "XL", withAutoRegistration, withPrinter(func(cfg *simulator.Config) {
			cfg.Enclosure = true
		}))
		defer rig.stop(t)

		session := rig.connect(t)
		watch := &stateWatch{rig: rig, session: session, seen: map[string]string{}}

		watch.await(t, "/temperature/chamber", "25")

		if target, written := watch.seen["/temperature/chamber_target"]; written {
			t.Errorf("an XL wrote a chamber target of %s, and it has none", target)
		}
	})
}

// A machine with no chamber writes nothing at the chamber's paths.
func TestAnMK4ReportsNoChamber(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		session := rig.connect(t)

		frames := 0
		for frames < 3 {
			select {
			case patch := <-session.State():
				switch patch.Path {
				case "/temperature/chamber", "/temperature/chamber_target", "/chamber/fan_pwm_target":
					t.Fatalf("an MK4 wrote %s = %s", patch.Path, patch.Value)
				case "/temperature/bed":
					frames++
				}
			case <-rig.ctx.Done():
				t.Fatalf("only %d frames arrived", frames)
			}
		}
	})
}
