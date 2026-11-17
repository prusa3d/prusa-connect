package xbuddy_test

import (
	"testing"
	"testing/synctest"

	"github.com/prusa3d/prusa-connect/internal/connector/xbuddy"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// A CORE One reports its chamber, and a target set through the verb comes
// back in the next frame, as it does from the firmware.
func TestACoreOneReportsItsChamberAndTakesATarget(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "CoreOne", withAutoRegistration)
		defer rig.stop(t)

		session := rig.connect(t)

		seen := map[string]string{}
		await := func(path, want string) {
			for seen[path] != want {
				select {
				case patch, open := <-session.State():
					if !open {
						t.Fatal("the state channel closed")
					}

					seen[patch.Path] = string(patch.Value)
				case <-rig.ctx.Done():
					t.Fatalf("%s never read %s; it reads %q", path, want, seen[path])
				}
			}
		}

		await("/temperature/chamber", "25")
		await("/temperature/chamber_target", "0")

		result, err := session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbSetChamberTemp, Args: map[string]string{"chamber.target_temp": "45"},
		})
		if err != nil || !result.OK {
			t.Fatalf("the CORE One refused a chamber target: %v %s", err, result.Raw)
		}

		await("/temperature/chamber_target", "45")
		await("/temperature/chamber", "45")

		t.Logf("observed: chamber %s, target %s", seen["/temperature/chamber"], seen["/temperature/chamber_target"])
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
				case "/temperature/chamber", "/temperature/chamber_target":
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
