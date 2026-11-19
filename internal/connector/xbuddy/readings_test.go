package xbuddy

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The readings the reader has seen described, read the way a client reads them.
func (h *hostile) readings() []connectorapi.Reading {
	tree := map[string]json.RawMessage{}

	for path, value := range h.dialog {
		if value != "null" {
			tree[path] = json.RawMessage(value)
		}
	}

	return connectorapi.ReadReadings(tree)
}

func TestTheReadingsAreDescribedWithoutProblems(t *testing.T) {
	t.Parallel()

	for _, reading := range []connectorapi.Reading{bedReading(), chamberReading(true), chamberReading(false)} {
		if problems := reading.Problems(); len(problems) > 0 {
			t.Errorf("%s: %v", reading.ID, problems)
		}
	}
}

func TestTheBedIsDescribedOnceAtThePathsTelemetryWrites(t *testing.T) {
	t.Parallel()

	if telemetryPaths["temp_bed"] != bedTempPath || telemetryPaths["target_bed"] != bedTargetPath {
		t.Fatalf("telemetry writes the bed at %s and %s", telemetryPaths["temp_bed"], telemetryPaths["target_bed"])
	}

	printer := standHostile(t)
	kind := connectorapi.ReadingPath("bed", "kind")
	value := connectorapi.ReadingPath("bed", "value_path")

	printer.text(t, `{"state":"IDLE","temp_bed":25.0,"target_bed":0}`)
	printer.awaitDialogTree(t, map[string]string{
		bedTempPath: "25.0", kind: `"temperature"`, value: `"/temperature/bed"`,
	})
	printer.text(t, `{"state":"IDLE","temp_bed":26.0}`)
	printer.awaitDialogTree(t, map[string]string{bedTempPath: "26.0"})

	want := []connectorapi.Reading{{
		ID: "bed", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: bedTempPath, TargetPath: bedTargetPath, Role: connectorapi.RoleBed,
	}}
	if got := printer.readings(); !reflect.DeepEqual(got, want) {
		t.Errorf("readings %+v, want %+v", got, want)
	}

	if got := printer.dialogPatchCount(kind); got != 1 {
		t.Errorf("the bed was described %d times in two frames", got)
	}
}

func TestACoreOneChamberIsDescribedWithItsTarget(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)
	role := connectorapi.ReadingPath("chamber", "role")
	value := connectorapi.ReadingPath("chamber", "value_path")

	printer.text(t, `{"state":"IDLE","chamber":{"temp":24.0,"target_temp":40}}`)
	printer.awaitDialogTree(t, map[string]string{
		chamberTempPath: "24.0", role: `"chamber"`, value: `"/temperature/chamber"`,
	})

	got := printer.readings()
	if len(got) != 1 || got[0] != chamberReading(true) {
		t.Fatalf("readings %+v", got)
	}

	if got[0].ValuePath != chamberTempPath || got[0].TargetPath != chamberTargetPath {
		t.Errorf("the chamber is described at %s and %s", got[0].ValuePath, got[0].TargetPath)
	}

	printer.text(t, `{"state":"IDLE","chamber":{"temp":25.0,"target_temp":40}}`)
	printer.awaitDialogTree(t, map[string]string{chamberTempPath: "25.0"})

	if count := printer.dialogPatchCount(role); count != 1 {
		t.Errorf("the chamber was described %d times in two frames", count)
	}
}

func TestAnXLEnclosureIsDescribedWithNoTargetAndRemovedWhenItGoes(t *testing.T) {
	t.Parallel()

	printer := standHostile(t)
	value := connectorapi.ReadingPath("chamber", "value_path")
	bedValue := connectorapi.ReadingPath("bed", "value_path")

	printer.text(t, `{"state":"IDLE","temp_bed":25.0,"enclosure":{"temp":31,"fan_rpm":1500,"time_in_use":3600}}`)
	printer.awaitDialogTree(t, map[string]string{
		chamberTempPath: "31", value: `"/temperature/chamber"`, bedValue: `"/temperature/bed"`,
	})

	got := printer.readings()
	if len(got) != 2 || got[1] != chamberReading(false) || got[1].TargetPath != "" {
		t.Fatalf("readings %+v, want the bed and a chamber with no target", got)
	}

	printer.text(t, `{"state":"IDLE","temp_bed":24.0}`)
	printer.awaitDialogTree(t, map[string]string{chamberTempPath: "null", value: "null"})

	if after := printer.readings(); len(after) != 1 || after[0].ID != "bed" {
		t.Errorf("after the enclosure went the readings are %+v", after)
	}
}
