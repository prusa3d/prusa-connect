package moonraker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// A reading of an idle machine must write no print path at all.
//
// Klipper reports print_stats on every frame, printing or not: an idle machine
// sends state "standby", an empty filename and a print_duration of zero. Those
// three are not a print, and the driver contract asks every driver that reports
// a print to report one at /job/id, because the host names the print's row and
// its events by that id. A reading that writes /job/name and /job/progress with
// no /job/id beside them gives the host a print it cannot name.
func TestAnIdleMachineReportsNoPrintPath(t *testing.T) {
	t.Parallel()

	held := &reader{}
	patches := held.patchesFrom(reading(`{
		"print_stats": {"state": "standby", "filename": "", "print_duration": 0, "total_duration": 0},
		"display_status": {"progress": 0}
	}`, t), 0, time.Unix(0, 0))

	for _, patch := range patches {
		if strings.HasPrefix(patch.Path, "/job/") {
			t.Errorf("an idle machine wrote %s, and no print is on the bed", patch.Path)
		}
	}
}

// A print on the bed reports its id and the rest of its paths together.
func TestAPrintOnTheBedReportsItsIDAndItsOtherPaths(t *testing.T) {
	t.Parallel()

	held := &reader{}
	patches := held.patchesFrom(reading(`{
		"print_stats": {
			"state": "printing", "filename": "bracket.gcode",
			"print_duration": 61.5, "filament_used": 120.25, "total_duration": 75
		},
		"display_status": {"progress": 0.42}
	}`, t), 0, time.Unix(0, 0))

	written := map[string]bool{}
	for _, patch := range patches {
		written[patch.Path] = true
	}

	for _, path := range []string{
		jobIDPath, jobNamePath, jobTimePrintingPath, jobFilamentPath, jobProgressPath,
	} {
		if !written[path] {
			t.Errorf("a print on the bed wrote no %s", path)
		}
	}
}

// The last reading of a print that ended still names the print it is about.
//
// The outcome and the file belong to the print that just came off the bed, so
// they are written together. Dropping the name here would leave the host's tree
// holding the previous print's name against this print's outcome.
func TestTheReadingThatEndsAPrintStillNamesIt(t *testing.T) {
	t.Parallel()

	held := &reader{}
	held.patchesFrom(reading(`{
		"print_stats": {
			"state": "printing", "filename": "bracket.gcode",
			"print_duration": 61.5, "total_duration": 75
		}
	}`, t), 0, time.Unix(0, 0))

	patches := held.patchesFrom(reading(`{
		"print_stats": {
			"state": "complete", "filename": "bracket.gcode",
			"print_duration": 120, "total_duration": 130
		}
	}`, t), 0, time.Unix(0, 0))

	written := map[string]bool{}
	for _, patch := range patches {
		written[patch.Path] = true
	}

	if !written[jobOutcomePath] {
		t.Error("the print ended and no outcome was written")
	}

	if !written[jobNamePath] {
		t.Error("the print ended and its name was not written with the outcome")
	}
}

// reading turns one JSON object into the shape a Moonraker status frame
// carries, so a case above reads as the frame the machine sends.
func reading(body string, t *testing.T) map[string]json.RawMessage {
	t.Helper()

	var status map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		t.Fatalf("the case is not a status object: %v", err)
	}

	return status
}

// An extruder object is a head counted from 0, with no upper bound, and an
// object that only looks like one is not.
func TestToolOfReadsKlippersExtruderNames(t *testing.T) {
	t.Parallel()

	for object, want := range map[string]int{
		"extruder": 0, "extruder1": 1, "extruder4": 4,
		"extruder0": -1, "extruder01": -1, "extruder_stepper": -1, "heater_bed": -1, "extruder99": 99, "extruder-1": -1,
	} {
		got, ok := toolOf(object)
		if want < 0 && ok {
			t.Errorf("%s is tool %d, want no tool", object, got)
		}

		if want >= 0 && (!ok || got != want) {
			t.Errorf("%s is tool %d, want %d", object, got, want)
		}
	}
}
