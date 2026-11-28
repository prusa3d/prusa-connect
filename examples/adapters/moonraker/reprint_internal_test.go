package moonraker

import (
	"testing"
	"time"
)

// Moonraker sends only the fields that changed, and a reprint of the file that
// just finished changes none of its name. The frames below are what a printer
// sent for the first print and for the reprint of the same file.
func TestAReprintOfTheSameFileHasTheIDAFreshSessionDerives(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	held := &reader{}

	first := jobIDOf(t, held.patchesFrom(reading(
		`{"print_stats": {"state": "printing", "filename": "bracket.gcode", "total_duration": 1.5}}`, t), 1001.5, now))
	held.patchesFrom(reading(`{"print_stats": {"state": "complete", "total_duration": 8}}`, t), 1008, now)

	reprint := jobIDOf(t, held.patchesFrom(reading(
		`{"print_stats": {"state": "printing", "total_duration": 2.5}}`, t), 1012.5, now))

	fresh := jobIDOf(t, (&reader{}).patchesFrom(reading(
		`{"print_stats": {"state": "printing", "filename": "bracket.gcode", "total_duration": 2.5}}`, t), 1012.5, now))

	if reprint == "" || fresh == "" {
		t.Fatalf("a print on the bed wrote no id: %q and %q", reprint, fresh)
	}

	if reprint != fresh {
		t.Errorf("the reprint got id %s, and a fresh session derives %s", reprint, fresh)
	}

	if reprint == first {
		t.Errorf("the reprint has the id %s of the print before it", first)
	}
}

func TestAPrintingReadingThatNamesNoFileWritesNoID(t *testing.T) {
	t.Parallel()

	patches := (&reader{}).patchesFrom(reading(
		`{"print_stats": {"state": "printing", "total_duration": 2.5}}`, t), 1012.5, time.Unix(0, 0))

	if id := jobIDOf(t, patches); id != "" {
		t.Errorf("a print with no file name got the id %s", id)
	}
}
