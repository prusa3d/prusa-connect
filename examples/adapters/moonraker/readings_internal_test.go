package moonraker

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func TestEachSensorSectionIsAReading(t *testing.T) {
	t.Parallel()

	objects := []string{
		"heater_bed", "heater_generic chamber", "temperature_sensor Raspberry Pi",
		"temperature_sensor 2nd", "temperature_fan exhaust", "heater_generic bed", "temperature_sensor chamber",
	}

	got := sensorsOf(objects)
	want := []sensor{
		{"heater_generic chamber", connectorapi.Reading{
			ID: "chamber", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
			ValuePath: "/temperature/chamber", TargetPath: "/temperature/chamber_target",
			Name: "chamber", Role: connectorapi.RoleChamber,
		}},
		{"temperature_sensor Raspberry Pi", connectorapi.Reading{
			ID: "raspberry_pi", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
			ValuePath: "/temperature/raspberry_pi", Name: "Raspberry Pi",
		}},
		{"temperature_sensor 2nd", connectorapi.Reading{
			ID: "s_2nd", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
			ValuePath: "/temperature/s_2nd", Name: "2nd",
		}},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sensors %+v, want %+v", got, want)
	}

	for _, reading := range append([]connectorapi.Reading{bedReading()}, got[0].reading, got[1].reading, got[2].reading) {
		if problems := reading.Problems(); len(problems) > 0 {
			t.Errorf("%s: %v", reading.ID, problems)
		}
	}
}

func TestTheDescribedPathsAreThePathsTelemetryWrites(t *testing.T) {
	t.Parallel()

	sensors := sensorsOf([]string{"heater_bed", "heater_generic chamber", "temperature_sensor enclosure"})
	held := &reader{sensors: sensors}

	patches := held.patchesFrom(reading(`{
		"heater_bed": {"temperature": 60.1, "target": 60},
		"heater_generic chamber": {"temperature": 35.5, "target": 40},
		"temperature_sensor enclosure": {"temperature": 30.2}
	}`, t), time.Unix(0, 0))

	written := map[string]json.RawMessage{}
	for _, patch := range patches {
		written[patch.Path] = patch.Value
	}

	tree := map[string]json.RawMessage{}
	for _, patch := range readingPatches([]string{"heater_bed"}, sensors) {
		tree[patch.Path] = patch.Value
	}

	readings := connectorapi.ReadReadings(tree)
	if len(readings) != 3 {
		t.Fatalf("readings %+v", readings)
	}

	if readings[0].ValuePath != bedPath || readings[0].TargetPath != bedTargetPath {
		t.Errorf("the bed is described at %s and %s", readings[0].ValuePath, readings[0].TargetPath)
	}

	for _, r := range readings {
		if _, ok := written[r.ValuePath]; !ok {
			t.Errorf("%s is described at %s, and telemetry wrote nothing there", r.ID, r.ValuePath)
		}

		if _, ok := written[r.TargetPath]; r.TargetPath != "" && !ok {
			t.Errorf("%s has a target at %s, and telemetry wrote nothing there", r.ID, r.TargetPath)
		}
	}

	if readings[2].ID != "enclosure" || readings[2].TargetPath != "" || readings[2].Role != "" {
		t.Errorf("a temperature sensor is described as %+v", readings[2])
	}
}

func TestAMachineWithNoBedDescribesNoBed(t *testing.T) {
	t.Parallel()

	if patches := readingPatches([]string{"extruder", "toolhead"}, nil); len(patches) != 0 {
		t.Errorf("a machine with no heater_bed described %v", patches)
	}
}
