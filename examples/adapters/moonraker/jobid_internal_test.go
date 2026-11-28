package moonraker

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func jobIDOf(t *testing.T, patches []connectorapi.StatePatch) string {
	t.Helper()

	for _, patch := range patches {
		if patch.Path != jobIDPath {
			continue
		}

		var id string
		if err := json.Unmarshal(patch.Value, &id); err != nil {
			t.Fatalf("/job/id is not text: %v", err)
		}

		return id
	}

	return ""
}

func printing(t *testing.T, seconds string) map[string]json.RawMessage {
	t.Helper()

	return reading(
		`{"print_stats": {"state": "printing", "filename": "bracket.gcode", "total_duration": `+seconds+`}}`,
		t,
	)
}

// A driver that reconnects to a machine in the middle of a print works the id
// out again from a fresh reading. The two readings below are of one print that
// began at Klipper time 1000, and the driver's clock is skewed 0.3 s each way
// against total_duration, as it is when the two are not read in the same
// instant. An id taken from the driver's clock rounds the start to the minute,
// and a start near a minute boundary lands on either side of it.
func TestAReconnectDerivesTheSameJobID(t *testing.T) {
	t.Parallel()

	began := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

	first := jobIDOf(t, (&reader{}).patchesFrom(
		printing(t, "600.0"), 1600.0, began.Add(600*time.Second+300*time.Millisecond),
	))

	second := jobIDOf(t, (&reader{}).patchesFrom(
		printing(t, "1200.4"), 2200.4, began.Add(1200*time.Second),
	))

	if first == "" || second == "" {
		t.Fatalf("a print on the bed wrote no id: %q and %q", first, second)
	}

	if first != second {
		t.Errorf("one print got two ids across a reconnect: %s and %s", first, second)
	}
}

func TestTheSameFilePrintedLaterHasAnotherJobID(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)

	first := jobIDOf(t, (&reader{}).patchesFrom(printing(t, "600.0"), 1600.0, now))
	second := jobIDOf(t, (&reader{}).patchesFrom(printing(t, "600.0"), 9600.0, now))

	if first == second {
		t.Errorf("two prints of one file share the id %s", first)
	}
}

func TestAMissingEventTimeFallsBackToTheDriversClock(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 18, 12, 10, 0, 0, time.UTC)

	fallback := jobIDOf(t, (&reader{}).patchesFrom(printing(t, "600.0"), 0, now))
	klipper := jobIDOf(t, (&reader{}).patchesFrom(printing(t, "600.0"), 1600.0, now))

	if fallback == "" {
		t.Fatal("a print with no event time wrote no id")
	}

	if fallback == klipper {
		t.Errorf("the fallback and Klipper's clock share the id %s, so the two kinds can collide", fallback)
	}
}

// A status notification carries the event time as the second element of its
// params, and the id is taken from it. Two sessions that hear one print at
// different driver clocks, a minute apart, name it alike.
func TestANotificationCarriesTheEventTimeIntoTheJobID(t *testing.T) {
	t.Parallel()

	heard := func(params string) string {
		t.Helper()

		session := newSession(nil, time.Second, liveness{}, slog.New(slog.DiscardHandler))
		session.update(json.RawMessage(params))

		var patches []connectorapi.StatePatch

		for len(session.State()) > 0 {
			patches = append(patches, <-session.State())
		}

		return jobIDOf(t, patches)
	}

	const status = `{"print_stats": {"state": "printing", "filename": "bracket.gcode", "total_duration": 90.5}}`

	withTime := heard(`[` + status + `, 1090.5]`)
	again := heard(`[` + status + `, 1090.5]`)
	other := heard(`[` + status + `, 1190.5]`)
	without := heard(`[` + status + `]`)

	if withTime != again {
		t.Errorf("one notification gave two ids: %s and %s", withTime, again)
	}

	if withTime == other {
		t.Errorf("a print that began 100 s later has the same id %s", withTime)
	}

	if without == withTime {
		t.Errorf("a notification with no event time shares the id %s with one that has it", without)
	}
}

func TestAnUnusableDurationFallsBackToTheDriversClock(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 18, 12, 10, 0, 0, time.UTC)

	for _, duration := range []string{"null", "-50.0"} {
		withEventTime := jobIDOf(t, (&reader{}).patchesFrom(printing(t, duration), 1600.0, now))
		withNone := jobIDOf(t, (&reader{}).patchesFrom(printing(t, duration), 0, now))

		if withEventTime == "" || withEventTime != withNone {
			t.Errorf("total_duration %s gave %q with an event time and %q without",
				duration, withEventTime, withNone)
		}
	}
}
