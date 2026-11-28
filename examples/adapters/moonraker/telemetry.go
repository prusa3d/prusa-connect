package moonraker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// Where a Klipper reading lands in the addressable state tree. The paths are
// the ones the xBuddy dialect already writes, so one client reads a Prusa and a
// Voron with the same addresses.
//
// jobNamePath is the one path here the xBuddy dialect has no twin for. Klipper
// has the file's name and no job number, so the name goes under its own path
// and the id a host needs is derived; see prints.
const (
	statePath           = "/state"
	jobNamePath         = "/job/name"
	jobIDPath           = connectorapi.JobIDPath
	jobOutcomePath      = connectorapi.JobOutcomePath
	jobFilamentPath     = connectorapi.JobFilamentUsedPath
	jobTimePrintingPath = connectorapi.JobTimePrintingPath
	jobProgressPath     = connectorapi.JobProgressPath
	bedPath             = "/temperature/bed"
	bedTargetPath       = "/temperature/bed_target"
	speedPath           = "/speed"
	flowPath            = "/flow"
)

// jobOutcomes maps Klipper's own terminal words onto the closed set the host
// publishes. A word this map does not carry writes no outcome, and the host
// then publishes the print as unknown.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var jobOutcomes = map[string]connectorapi.JobOutcome{
	"complete":  connectorapi.JobFinished,
	"cancelled": connectorapi.JobCancelled,
	"error":     connectorapi.JobFailed,
}

// klipperPrintingStates are Klipper's words for a print that is on the bed.
//
//nolint:gochecknoglobals // a fixed set, read-only after init.
var klipperPrintingStates = []string{"printing", "paused"}

// axisPaths is where the three axes of toolhead.position land, in the order
// Klipper reports them.
//
//nolint:gochecknoglobals // a fixed list, read-only after init.
var axisPaths = []string{"/axis/x", "/axis/y", "/axis/z"}

// stateWords maps Klipper's print_stats.state onto the words every other
// dialect here reports, so a client branches on one vocabulary.
//
// A word this map does not carry reaches the tree in upper case rather than
// being dropped. A Klipper fork that adds a state is then readable, which
// silence would not be.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var stateWords = map[string]string{
	"standby":   "IDLE",
	"printing":  "PRINTING",
	"paused":    "PAUSED",
	"complete":  "FINISHED",
	"cancelled": "STOPPED",
	"error":     "ERROR",
}

// percent is the scale a Klipper fraction is reported on. Klipper counts
// progress and the two factors from zero to one, and every path above is a
// percentage, so the number is converted rather than passed through.
const percent = 100

// roundingUnit is how far a converted number is rounded, as the reciprocal of
// the step. Multiplying 0.213 by a hundred in binary floating point gives
// 21.299999999999997, and that is not a reading anybody wants to look at.
const roundingUnit = 100

// prints is what this driver has to remember to give a Klipper print an id.
//
// Klipper counts no jobs. The driver contract asks every driver that reports a
// print to report one at /job/id, because a host names the row and the events
// of a print by it, so this derives one: the file's name and the moment the
// print began. The moment is worked out from the machine's own total_duration,
// which counts from the start of the print, and it is held for the life of the
// print so that two readings of one print produce one id.
//
// The moment is rounded to the minute. A driver that reconnects to a machine
// mid-print works it out again from a fresh reading, and total_duration and the
// clock are not read in the same instant: a finer rounding would give one print
// two ids across a reconnect, which is two prints in every record that reads
// them.
type prints struct {
	// id is the print on the bed now, empty when nothing is printing.
	id string
	// file is the name the id was derived from, so a machine that starts a
	// different file is a different print.
	file string
}

// jobStartRounding is how coarsely a derived start is rounded. See prints.
const jobStartRounding = time.Minute

// reader is what this driver remembers between two readings of one machine.
// Moonraker sends only what changed, so a reading alone does not say which
// print is on the bed or which head is printing.
type reader struct {
	prints  prints
	heads   heads
	machine machineState
}

// patchesFrom turns one reading of the Klipper objects into patches, in a fixed
// order so that two runs of one frame produce the same sequence.
//
// A field the machine did not report produces no patch. Moonraker sends only
// what changed, so a frame carrying one temperature must not blank the rest of
// the tree.
func (r *reader) patchesFrom(status map[string]json.RawMessage, now time.Time) []connectorapi.StatePatch {
	p := &r.prints
	objects := objectFields(status)

	var out []connectorapi.StatePatch

	stats := objects[objectPrintStats]
	out = r.machine.appendState(out, stats["state"], objects[objectWebhooks]["state"])

	// Klipper reports print_stats whether a print is on the bed or not: an idle
	// machine sends state "standby", an empty filename and a print_duration of
	// zero. Those are not a print. The other print paths are written only for a
	// reading that names one, because the host names a print's row and its
	// events by /job/id, and a /job/name with no id beside it is a print the
	// host cannot name.
	//
	// The id before the call is read as well as the one after it, so the
	// reading that ends a print writes the name and the duration beside the
	// outcome. That reading is the last one about a print appendJob has just
	// taken off the bed.
	before := p.id
	out = p.appendJob(out, stats, now)

	if before != "" || p.id != "" {
		out = appendVerbatim(out, stats["filename"], jobNamePath)
		out = appendVerbatim(out, stats["print_duration"], jobTimePrintingPath)
		out = appendVerbatim(out, stats["filament_used"], jobFilamentPath)
		out = appendScaled(out, objects[objectDisplayStatus]["progress"], jobProgressPath)
	}

	out = r.heads.appendReadings(out, objects)

	bed := objects[objectHeaterBed]
	out = appendVerbatim(out, bed["temperature"], bedPath)
	out = appendVerbatim(out, bed["target"], bedTargetPath)

	out = appendPosition(out, objects[objectToolhead]["position"])

	move := objects[objectGcodeMove]
	out = appendScaled(out, move["speed_factor"], speedPath)
	out = appendScaled(out, move["extrude_factor"], flowPath)

	return out
}

// appendJob gives the print an id and, when it ends, an outcome.
//
// The id is minted once, on the first reading that says a file is printing, and
// held until the machine prints something else. A reading that carries neither
// a state nor a file name says nothing about which print it means, so it leaves
// what is held alone: Moonraker sends only what changed.
func (p *prints) appendJob(
	out []connectorapi.StatePatch, stats map[string]json.RawMessage, now time.Time,
) []connectorapi.StatePatch {
	state, file := klipperText(stats["state"]), klipperText(stats["filename"])

	if state == "" && file == "" {
		return out
	}

	if file == "" {
		file = p.file
	}

	if !slices.Contains(klipperPrintingStates, state) {
		if p.id == "" {
			return out
		}

		if outcome, ended := jobOutcomes[state]; ended {
			out = appendText(out, jobOutcomePath, outcome)
		}

		// The print is off the bed, so the next one is a different print even
		// when it is the same file. The id stays in the host's tree, which is
		// what keeps the row named while the machine tidies up.
		p.id, p.file = "", ""

		return out
	}

	if p.id == "" || p.file != file {
		p.file, p.id = file, derivedJobID(file, startOf(stats, now))
	}

	return appendText(out, jobIDPath, p.id)
}

// startOf is when this print began, from the machine's own count of how long it
// has been running. A machine that reports no count began it now.
func startOf(stats map[string]json.RawMessage, now time.Time) time.Time {
	var running float64
	if json.Unmarshal(stats["total_duration"], &running) != nil {
		return now.Truncate(jobStartRounding)
	}

	return now.Add(-time.Duration(running * float64(time.Second))).Truncate(jobStartRounding)
}

// derivedJobID is the id this driver gives a print the machine did not number:
// the file and the minute the print began, as the hex of their digest.
//
// A digest rather than the two values joined, because a file name is whatever
// somebody called the file and an id travels in a URL, in a cursor and in an
// event.
func derivedJobID(file string, started time.Time) string {
	sum := sha256.Sum256([]byte(file + "\x00" + started.UTC().Format(time.RFC3339)))

	return hex.EncodeToString(sum[:jobIDBytes])
}

// jobIDBytes is how much of the digest the id carries. Sixteen bytes is what a
// UUID is, and the space it names is far past the prints one machine makes.
const jobIDBytes = 16

// klipperText reads a reported value as text, and answers empty for anything
// else.
func klipperText(raw json.RawMessage) string {
	var text string
	if len(raw) == 0 || json.Unmarshal(raw, &text) != nil {
		return ""
	}

	return text
}

// appendText writes one string value at a path.
func appendText(out []connectorapi.StatePatch, path, value string) []connectorapi.StatePatch {
	encoded, err := json.Marshal(value)
	if err != nil {
		return out
	}

	return append(out, connectorapi.StatePatch{Path: path, Value: encoded})
}

// objectFields reads each object's fields without reading their values, so a
// value this dialect passes through reaches the host as the machine encoded it.
// An object whose body is not an object at all is skipped.
func objectFields(status map[string]json.RawMessage) map[string]map[string]json.RawMessage {
	out := make(map[string]map[string]json.RawMessage, len(status))

	for object, raw := range status {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			continue
		}

		out[object] = fields
	}

	return out
}

// hookErrorStates are the webhooks.state values that mean Klipper is not
// running. It shut down, for example after M112, or it stopped with a
// configuration or MCU error. The other values are ready and startup.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var hookErrorStates = []string{"shutdown", "error"}

// Moonraker sends only what changed, so one frame may carry either object
// alone. print_stats changes only when a print starts, pauses, ends or fails,
// so it stays "standby" after M112 on an idle machine and when an MCU cannot
// connect. The machine's own state is webhooks.state, and /state needs both.
type machineState struct {
	print, hook string
	// errored is set once ERROR has been written, so a recovery that has no
	// print state to report is still told as IDLE.
	errored bool
}

func (m *machineState) appendState(
	out []connectorapi.StatePatch, printState, hookState json.RawMessage,
) []connectorapi.StatePatch {
	heard := false

	if value := klipperText(printState); value != "" {
		m.print, heard = value, true
	}

	if value := klipperText(hookState); value != "" {
		m.hook, heard = value, true
	}

	if !heard {
		return out
	}

	var word string

	switch {
	case slices.Contains(hookErrorStates, m.hook):
		word, m.errored = stateWords["error"], true
	case m.print == "" && m.errored:
		word, m.errored = stateWords["standby"], false
	case m.print == "":
		return out
	default:
		var known bool

		word, known = stateWords[m.print]
		if !known {
			word = strings.ToUpper(m.print)
		}

		m.errored = false
	}

	encoded, err := json.Marshal(word)
	if err != nil {
		return out
	}

	return append(out, connectorapi.StatePatch{Path: statePath, Value: encoded})
}

// appendVerbatim passes a value on exactly as the machine encoded it. A driver
// that re-encodes a number changes it.
func appendVerbatim(
	out []connectorapi.StatePatch, raw json.RawMessage, path string,
) []connectorapi.StatePatch {
	if len(raw) == 0 {
		return out
	}

	return append(out, connectorapi.StatePatch{Path: path, Value: raw})
}

// appendScaled converts a Klipper fraction to the percentage its path carries.
func appendScaled(
	out []connectorapi.StatePatch, raw json.RawMessage, path string,
) []connectorapi.StatePatch {
	if len(raw) == 0 {
		return out
	}

	var fraction float64
	if json.Unmarshal(raw, &fraction) != nil {
		return out
	}

	rounded := math.Round(fraction*percent*roundingUnit) / roundingUnit
	encoded := strconv.FormatFloat(rounded, 'f', -1, 64)

	return append(out, connectorapi.StatePatch{Path: path, Value: json.RawMessage(encoded)})
}

// appendPosition writes the three axes Klipper reports as one array. The fourth
// element is the extruder's own position, which is not an axis a client shows.
func appendPosition(
	out []connectorapi.StatePatch, raw json.RawMessage,
) []connectorapi.StatePatch {
	if len(raw) == 0 {
		return out
	}

	var position []json.RawMessage
	if json.Unmarshal(raw, &position) != nil {
		return out
	}

	for index, path := range axisPaths {
		if index >= len(position) {
			break
		}

		out = append(out, connectorapi.StatePatch{Path: path, Value: position[index]})
	}

	return out
}
