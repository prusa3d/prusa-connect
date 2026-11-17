package simulator

import "testing"

// chamberAfter builds a printer, applies the setters in order and reads the
// chamber block of a frame taken while it prints.
func chamberAfter(t *testing.T, model string, sets ...map[string]any) map[string]any {
	t.Helper()

	printer, err := New(Config{Model: model, Serial: "SIM-1", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}

	printer.mu.Lock()
	defer printer.mu.Unlock()

	for _, set := range sets {
		for key := range set {
			command := map[string]string{
				"chamber.target_temp":    chamberSetter,
				"chamber.fan_pwm_target": "SET_CHAMBER_FAN_TARGET",
				"chamber.led_intensity":  "SET_CHAMBER_LED_INTENSITY",
			}[key]
			printer.setChamber(command, set)
		}
	}

	return printer.chamberFields(true)
}

// The chamber has fans and no heater. A target under the air of a print cools
// it there while the fans run on their own, and a fan set by hand stops that.
func TestAChamberTargetCoolsAndCannotHeat(t *testing.T) {
	t.Parallel()

	if got := chamberAfter(t, "CoreOne", map[string]any{"chamber.target_temp": 28.0})["temp"]; got != 28.0 {
		t.Errorf("a target of 28 under a print reads %v, want 28", got)
	}

	if got := chamberAfter(t, "CoreOne", map[string]any{"chamber.target_temp": 45.0})["temp"]; got != printingChamberTemp {
		t.Errorf("a target of 45 above the air reads %v, want the print's %v", got, printingChamberTemp)
	}

	manual := chamberAfter(t, "CoreOne",
		map[string]any{"chamber.target_temp": 28.0}, map[string]any{"chamber.fan_pwm_target": 30.0})
	if manual["temp"] != printingChamberTemp {
		t.Errorf("a fan set by hand still held the air at %v", manual["temp"])
	}
}

// The firmware cuts a target over the model's ceiling down to it, drops one
// while the fans are set by hand, takes a negative fan target as automatic, and rounds the light down on its 0 to 255
// scale.
func TestTheChamberSettersActAsTheFirmwareDoes(t *testing.T) {
	t.Parallel()

	for model, ceiling := range map[string]int{"CoreOne": 55, "CoreOneL": 60} {
		if got := chamberAfter(t, model, map[string]any{"chamber.target_temp": 70.0})["target_temp"]; got != ceiling {
			t.Errorf("%s: a target of 70 reads %v, want %d", model, got, ceiling)
		}
	}

	// A target sent while the fans are set by hand is dropped, and the one
	// before it stands.
	dropped := chamberAfter(t, "CoreOne", map[string]any{"chamber.target_temp": 45.0},
		map[string]any{"chamber.fan_pwm_target": 40.0}, map[string]any{"chamber.target_temp": 50.0})
	if got := dropped["target_temp"]; got != 45 {
		t.Errorf("a target sent with the fans set by hand reads %v, want the 45 before it", got)
	}

	if got := chamberAfter(t, "CoreOne", map[string]any{"chamber.fan_pwm_target": -7.0})["fan_pwm_target"]; got != -1 {
		t.Errorf("a negative fan target reads %v, want -1", got)
	}

	if got := chamberAfter(t, "CoreOne", map[string]any{"chamber.led_intensity": 50.0})["led_intensity"]; got != 49 {
		t.Errorf("a light of 50 reads %v, want 49", got)
	}
}

// The chamber block rides only the frames the firmware sends it in: the first
// one of a connection and the next one after the block changed.
func TestTheChamberBlockIsSentWhenItChanges(t *testing.T) {
	t.Parallel()

	printer, err := New(Config{Model: "CoreOne", Serial: "SIM-1", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}

	printer.mu.Lock()
	defer printer.mu.Unlock()

	sent := func() bool {
		_, present := printer.telemetryFields()["chamber"]

		return present
	}

	if !sent() {
		t.Error("the first frame carries no chamber block")
	}

	if sent() {
		t.Error("a frame after an unchanged chamber carries the block again")
	}

	printer.setChamber("SET_CHAMBER_LED_INTENSITY", map[string]any{"chamber.led_intensity": 20.0})

	if !sent() {
		t.Error("the frame after the light changed carries no chamber block")
	}

	printer.sessions++

	if !sent() {
		t.Error("the first frame of a new connection carries no chamber block")
	}
}
