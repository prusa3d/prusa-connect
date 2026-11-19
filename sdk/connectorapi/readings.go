package connectorapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// A reading is one measured value of a whole machine, such as the bed
// temperature. A driver describes each reading under /readings/<id>/ in the
// state of its device, and a reader finds them with ReadReadings. The value
// itself stays at its own path. A value of one tool belongs in the tool tree.

// ReadingKind is what a reading measures.
type ReadingKind string

// ReadingTemperature is a temperature.
const ReadingTemperature ReadingKind = "temperature"

// ReadingRole is the part of the machine a reading belongs to. A client
// translates the label of a reading with a role. One device has at most one
// reading per role.
type ReadingRole string

// The roles a reading may have.
const (
	// RoleBed is the heated bed.
	RoleBed ReadingRole = "bed"
	// RoleChamber is the air inside the printer or its enclosure.
	RoleChamber ReadingRole = "chamber"
)

// MaxReadingNameLen bounds the name of one reading, in bytes.
const MaxReadingNameLen = 64

// maxReadingIDLen bounds the id of one reading.
const maxReadingIDLen = 64

// ErrInvalidReading reports a reading description a reader would skip or read
// in part. Reading.Problems wraps it.
var ErrInvalidReading = errors.New("connectorapi: invalid reading")

// readingsRoot is the prefix of every reading path.
const readingsRoot = "/readings/"

// The members of one reading, in the order Reading.Patches writes them.
const (
	readingKind       = "kind"
	readingUnit       = "unit"
	readingValuePath  = "value_path"
	readingTargetPath = "target_path"
	readingName       = "name"
	readingRole       = "role"
)

//nolint:gochecknoglobals // a read-only lookup table.
var readingMembers = []string{
	readingKind, readingUnit, readingValuePath, readingTargetPath, readingName, readingRole,
}

//nolint:gochecknoglobals // a read-only lookup table.
var readingUnits = map[ReadingKind][]Unit{ReadingTemperature: {UnitCelsius}}

// Reading is one reading a driver describes. Build it in a driver and write it
// with Patches. Read it from a state with ReadReadings.
type Reading struct {
	// ID is 1 to 64 characters from a-z, 0-9 and _, starting with a letter. It
	// stays the same for one device.
	ID   string      `json:"id"`
	Kind ReadingKind `json:"kind"`
	// Unit is the unit of the value. It belongs to the kind: UnitCelsius for a
	// temperature.
	Unit Unit `json:"unit"`
	// ValuePath is the state path of the value. TargetPath is the state path
	// of the value the machine holds it to, empty when it has none.
	ValuePath  string `json:"value_path"`
	TargetPath string `json:"target_path,omitempty"`
	// Name is what the machine or its owner calls the reading. A client shows
	// it for a reading with no role.
	Name string      `json:"name,omitempty"`
	Role ReadingRole `json:"role,omitempty"`
}

// ReadingPath is the state path of one member of one reading, such as
// /readings/bed/value_path.
func ReadingPath(id, member string) string {
	return readingsRoot + id + "/" + member
}

// SplitReadingPath returns the id and the member of a path under /readings/.
// It reports false for any other path and for an invalid id.
func SplitReadingPath(path string) (id, member string, ok bool) {
	rest, found := strings.CutPrefix(path, readingsRoot)
	if !found {
		return "", "", false
	}

	id, member, found = strings.Cut(rest, "/")
	if !found || member == "" || strings.Contains(member, "/") || !validReadingID(id) {
		return "", "", false
	}

	return id, member, true
}

// ReadReadings builds the readings described in one device's state, keyed by
// path, sorted by id. It skips a reading whose kind, unit or value path is
// missing or invalid, and drops an optional member that is invalid. Of two
// readings with one role, the first by id keeps the role.
func ReadReadings(state map[string]json.RawMessage) []Reading {
	members := map[string]map[string]string{}

	for path, value := range state {
		id, member, ok := SplitReadingPath(path)
		if !ok {
			continue
		}

		value, isText := text(value)
		if !isText {
			continue
		}

		if members[id] == nil {
			members[id] = map[string]string{}
		}

		members[id][member] = value
	}

	var readings []Reading

	for id, set := range members {
		if reading, ok := readReading(id, set); ok {
			readings = append(readings, reading)
		}
	}

	slices.SortFunc(readings, func(a, b Reading) int { return strings.Compare(a.ID, b.ID) })

	taken := map[ReadingRole]bool{}

	for i := range readings {
		role := readings[i].Role
		if role == "" {
			continue
		}

		if taken[role] {
			readings[i].Role = ""
		}

		taken[role] = true
	}

	return readings
}

// readReading builds one reading from its text members.
func readReading(id string, set map[string]string) (Reading, bool) {
	reading := Reading{
		ID: id, Kind: ReadingKind(set[readingKind]), Unit: Unit(set[readingUnit]),
		ValuePath: set[readingValuePath],
	}

	if !slices.Contains(readingUnits[reading.Kind], reading.Unit) || readingPathProblem(reading.ValuePath) != nil {
		return Reading{}, false
	}

	if target := set[readingTargetPath]; readingPathProblem(target) == nil && target != reading.ValuePath {
		reading.TargetPath = target
	}

	if name := set[readingName]; len(name) <= MaxReadingNameLen {
		reading.Name = name
	}

	if role := ReadingRole(set[readingRole]); role.valid() {
		reading.Role = role
	}

	return reading, true
}

// Problems reports every part of the reading a reader would skip or drop. A
// driver checks its readings with it in a test.
func (r Reading) Problems() []error {
	var problems []error

	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("%w %q: "+format, append([]any{ErrInvalidReading, r.ID}, args...)...))
	}

	if !validReadingID(r.ID) {
		add("the id is not 1 to %d characters from a-z, 0-9 and _ starting with a letter", maxReadingIDLen)
	}

	units, known := readingUnits[r.Kind]

	switch {
	case !known:
		add("the kind %q is not known", r.Kind)
	case !slices.Contains(units, r.Unit):
		add("the unit %q is not a unit of %s", r.Unit, r.Kind)
	}

	if err := readingPathProblem(r.ValuePath); err != nil {
		add("the value path: %w", err)
	}

	if r.TargetPath != "" {
		if err := readingPathProblem(r.TargetPath); err != nil {
			add("the target path: %w", err)
		}

		if r.TargetPath == r.ValuePath {
			add("the target path is the value path")
		}
	}

	if len(r.Name) > MaxReadingNameLen {
		add("the name is %d bytes, more than %d", len(r.Name), MaxReadingNameLen)
	}

	if r.Role != "" && !r.Role.valid() {
		add("the role %q is not known", r.Role)
	}

	return problems
}

// Patches writes the reading: one patch per member that is set, always in the
// same order.
func (r Reading) Patches() []StatePatch {
	values := map[string]string{
		readingKind: string(r.Kind), readingUnit: string(r.Unit), readingValuePath: r.ValuePath,
		readingTargetPath: r.TargetPath, readingName: r.Name, readingRole: string(r.Role),
	}

	patches := make([]StatePatch, 0, len(readingMembers))

	for _, member := range readingMembers {
		if values[member] == "" {
			continue
		}

		value, _ := json.Marshal(values[member]) //nolint:errchkjson // a string always encodes.
		patches = append(patches, StatePatch{Path: ReadingPath(r.ID, member), Value: value})
	}

	return patches
}

// Removal removes the reading from the state: a null for each member, set or
// not.
func (r Reading) Removal() []StatePatch {
	patches := make([]StatePatch, 0, len(readingMembers))

	for _, member := range readingMembers {
		patches = append(patches, StatePatch{Path: ReadingPath(r.ID, member), Value: json.RawMessage("null")})
	}

	return patches
}

func (role ReadingRole) valid() bool { return role == RoleBed || role == RoleChamber }

func validReadingID(id string) bool {
	if id == "" || len(id) > maxReadingIDLen || id[0] < 'a' || id[0] > 'z' {
		return false
	}

	for _, c := range []byte(id) {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}

	return true
}

// readingPathProblem checks one path a reading points at.
func readingPathProblem(path string) error {
	switch {
	case path == "":
		return errors.New("it is empty")
	case len(path) > RetainedTextFloor:
		return fmt.Errorf("it is %d bytes, more than %d", len(path), RetainedTextFloor)
	case !strings.HasPrefix(path, "/"):
		return errors.New("it does not start with /")
	case strings.HasSuffix(path, "/"):
		return errors.New("it ends with /")
	case strings.Contains(path, "//"):
		return errors.New("it has an empty segment")
	case strings.HasPrefix(path, readingsRoot):
		return errors.New("it is under /readings/")
	}

	return nil
}
