package connectorapi_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func bed() connectorapi.Reading {
	return connectorapi.Reading{
		ID: "bed", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: "/temperature/bed", TargetPath: "/temperature/bed_target", Role: connectorapi.RoleBed,
	}
}

func TestReadingPathsSplitBackIntoTheirIDAndMember(t *testing.T) {
	t.Parallel()

	if got := connectorapi.ReadingPath("bed", "kind"); got != "/readings/bed/kind" {
		t.Errorf("the kind of bed is at %s", got)
	}

	cases := []struct {
		path, id, member string
		ok               bool
	}{
		{"/readings/bed/kind", "bed", "kind", true},
		{"/readings/heater_2/value_path", "heater_2", "value_path", true},
		{"/readings/" + strings.Repeat("a", 64) + "/unit", strings.Repeat("a", 64), "unit", true},
		{"/readings/" + strings.Repeat("a", 65) + "/unit", "", "", false},
		{"/readings/Bed/kind", "", "", false},
		{"/readings/2bed/kind", "", "", false},
		{"/readings/_bed/kind", "", "", false},
		{"/readings/be-d/kind", "", "", false},
		{"/readings//kind", "", "", false},
		{"/readings/bed/", "", "", false},
		{"/readings/bed", "", "", false},
		{"/readings/bed/kind/more", "", "", false},
		{"/reading/bed/kind", "", "", false},
		{"/temperature/bed", "", "", false},
	}

	for _, tc := range cases {
		id, member, ok := connectorapi.SplitReadingPath(tc.path)
		if ok != tc.ok || id != tc.id || member != tc.member {
			t.Errorf("%s: got %q %q %v", tc.path, id, member, ok)
		}
	}
}

func TestReadingProblems(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		change func(*connectorapi.Reading)
		bad    bool
	}{
		{"the bed as described", func(*connectorapi.Reading) {}, false},
		{"an id of 64 characters", func(r *connectorapi.Reading) { r.ID = strings.Repeat("b", 64) }, false},
		{"an id of 65 characters", func(r *connectorapi.Reading) { r.ID = strings.Repeat("b", 65) }, true},
		{"an id with digits and underscores", func(r *connectorapi.Reading) { r.ID = "heater_2" }, false},
		{"an empty id", func(r *connectorapi.Reading) { r.ID = "" }, true},
		{"an id with a capital", func(r *connectorapi.Reading) { r.ID = "Bed" }, true},
		{"an id starting with a digit", func(r *connectorapi.Reading) { r.ID = "1bed" }, true},
		{"an unknown kind", func(r *connectorapi.Reading) { r.Kind = "humidity" }, true},
		{"an empty kind", func(r *connectorapi.Reading) { r.Kind = "" }, true},
		{"a unit of another kind", func(r *connectorapi.Reading) { r.Unit = connectorapi.UnitPercent }, true},
		{"an empty unit", func(r *connectorapi.Reading) { r.Unit = "" }, true},
		{"no value path", func(r *connectorapi.Reading) { r.ValuePath = "" }, true},
		{"a value path with no slash first", func(r *connectorapi.Reading) { r.ValuePath = "temperature/bed" }, true},
		{"a value path with a slash last", func(r *connectorapi.Reading) { r.ValuePath = "/temperature/bed/" }, true},
		{
			"a value path with an empty segment",
			func(r *connectorapi.Reading) { r.ValuePath = "/temperature//bed" },
			true,
		},
		{"a value path of the root", func(r *connectorapi.Reading) { r.ValuePath = "/" }, true},
		{"a value path under the readings", func(r *connectorapi.Reading) { r.ValuePath = "/readings/bed/kind" }, true},
		{"a value path past the floor", func(r *connectorapi.Reading) {
			r.ValuePath = "/" + strings.Repeat("a", connectorapi.RetainedTextFloor)
		}, true},
		{"a value path at the floor", func(r *connectorapi.Reading) {
			r.ValuePath = "/" + strings.Repeat("a", connectorapi.RetainedTextFloor-1)
		}, false},
		{"no target path", func(r *connectorapi.Reading) { r.TargetPath = "" }, false},
		{"a target path with an empty segment", func(r *connectorapi.Reading) { r.TargetPath = "//bed_target" }, true},
		{
			"a target path under the readings",
			func(r *connectorapi.Reading) { r.TargetPath = "/readings/x/value_path" },
			true,
		},
		{"a target path that is the value path", func(r *connectorapi.Reading) { r.TargetPath = r.ValuePath }, true},
		{"a name of 64 bytes", func(r *connectorapi.Reading) { r.Name = strings.Repeat("n", 64) }, false},
		{"a name of 65 bytes", func(r *connectorapi.Reading) { r.Name = strings.Repeat("n", 65) }, true},
		{"no role", func(r *connectorapi.Reading) { r.Role = "" }, false},
		{"the chamber role", func(r *connectorapi.Reading) { r.Role = connectorapi.RoleChamber }, false},
		{"an unknown role", func(r *connectorapi.Reading) { r.Role = "nozzle" }, true},
	}

	for _, tc := range cases {
		reading := bed()
		tc.change(&reading)

		problems := reading.Problems()
		if got := len(problems) > 0; got != tc.bad {
			t.Errorf("%s: problems %v, want some: %v", tc.name, problems, tc.bad)
		}

		for _, problem := range problems {
			if !errors.Is(problem, connectorapi.ErrInvalidReading) {
				t.Errorf("%s: %v is not ErrInvalidReading", tc.name, problem)
			}
		}
	}
}

func TestReadingPatchesComeInAFixedOrder(t *testing.T) {
	t.Parallel()

	reading := bed()
	reading.Name = "Heated bed"

	want := []connectorapi.StatePatch{
		{Path: "/readings/bed/kind", Value: json.RawMessage(`"temperature"`)},
		{Path: "/readings/bed/unit", Value: json.RawMessage(`"celsius"`)},
		{Path: "/readings/bed/value_path", Value: json.RawMessage(`"/temperature/bed"`)},
		{Path: "/readings/bed/target_path", Value: json.RawMessage(`"/temperature/bed_target"`)},
		{Path: "/readings/bed/name", Value: json.RawMessage(`"Heated bed"`)},
		{Path: "/readings/bed/role", Value: json.RawMessage(`"bed"`)},
	}

	if got := reading.Patches(); !reflect.DeepEqual(got, want) {
		t.Errorf("patches %v, want %v", got, want)
	}

	bare := connectorapi.Reading{
		ID: "air", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius, ValuePath: "/air",
	}
	if got := bare.Patches(); len(got) != 3 {
		t.Errorf("a reading with three members set wrote %d patches", len(got))
	}
}

func TestReadingRemovalNullsEveryMember(t *testing.T) {
	t.Parallel()

	removal := bed().Removal()
	members := []string{"kind", "unit", "value_path", "target_path", "name", "role"}

	if len(removal) != len(members) {
		t.Fatalf("removal has %d patches, want %d", len(removal), len(members))
	}

	tree := treeOf(bed().Patches())

	for i, patch := range removal {
		if want := connectorapi.ReadingPath("bed", members[i]); patch.Path != want || string(patch.Value) != "null" {
			t.Errorf("removal %d is %s = %s, want %s = null", i, patch.Path, patch.Value, want)
		}

		delete(tree, patch.Path)
	}

	if len(tree) != 0 || len(connectorapi.ReadReadings(tree)) != 0 {
		t.Errorf("after the removal the tree still holds %v", tree)
	}
}

func TestPatchesReadBackAsTheSameReadings(t *testing.T) {
	t.Parallel()

	chamber := connectorapi.Reading{
		ID: "chamber", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: "/temperature/chamber", Name: "Chamber", Role: connectorapi.RoleChamber,
	}
	probe := connectorapi.Reading{
		ID: "a_probe", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: "/temperature/a_probe",
	}

	patches := append(append(chamber.Patches(), bed().Patches()...), probe.Patches()...)
	tree := treeOf(patches)
	tree["/temperature/bed"] = json.RawMessage(`60`)

	want := []connectorapi.Reading{probe, bed(), chamber}
	if got := connectorapi.ReadReadings(tree); !reflect.DeepEqual(got, want) {
		t.Errorf("read %+v, want %+v", got, want)
	}
}

func TestReadReadingsSkipsWhatItCannotUse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		tree  map[string]json.RawMessage
		wants []connectorapi.Reading
	}{
		{"no readings", state("/temperature/bed", `60`), nil},
		{"an invalid id", state(
			"/readings/Bed/kind", `"temperature"`, "/readings/Bed/unit", `"celsius"`,
			"/readings/Bed/value_path", `"/temperature/bed"`), nil},
		{"no value path", state("/readings/bed/kind", `"temperature"`, "/readings/bed/unit", `"celsius"`), nil},
		{"a kind that is not text", state(
			"/readings/bed/kind", `1`, "/readings/bed/unit", `"celsius"`,
			"/readings/bed/value_path", `"/temperature/bed"`), nil},
		{"an unknown kind", state(
			"/readings/bed/kind", `"humidity"`, "/readings/bed/unit", `"percent"`,
			"/readings/bed/value_path", `"/humidity"`), nil},
		{"an unknown unit", state(
			"/readings/bed/kind", `"temperature"`, "/readings/bed/unit", `"kelvin"`,
			"/readings/bed/value_path", `"/temperature/bed"`), nil},
		{"a value path under the readings", state(
			"/readings/bed/kind", `"temperature"`, "/readings/bed/unit", `"celsius"`,
			"/readings/bed/value_path", `"/readings/bed/kind"`), nil},
		{"a null member", state(
			"/readings/bed/kind", `null`, "/readings/bed/unit", `"celsius"`,
			"/readings/bed/value_path", `"/temperature/bed"`), nil},
		{"an unknown role and a bad target", state(
			"/readings/bed/kind", `"temperature"`, "/readings/bed/unit", `"celsius"`,
			"/readings/bed/value_path", `"/temperature/bed"`, "/readings/bed/target_path", `7`,
			"/readings/bed/role", `"plate"`, "/readings/bed/name", `"Bed"`, "/readings/bed/colour", `"red"`),
			[]connectorapi.Reading{{
				ID: "bed", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
				ValuePath: "/temperature/bed", Name: "Bed",
			}}},
	}

	for _, tc := range cases {
		if got := connectorapi.ReadReadings(tc.tree); !reflect.DeepEqual(got, tc.wants) {
			t.Errorf("%s: read %+v, want %+v", tc.name, got, tc.wants)
		}
	}
}

func TestOneRoleBelongsToTheFirstReadingByID(t *testing.T) {
	t.Parallel()

	first := connectorapi.Reading{
		ID: "chamber", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: "/temperature/chamber", Role: connectorapi.RoleChamber,
	}
	second := first
	second.ID, second.ValuePath = "enclosure", "/temperature/enclosure"

	got := connectorapi.ReadReadings(treeOf(append(first.Patches(), second.Patches()...)))
	if len(got) != 2 || got[0].ID != "chamber" || got[1].ID != "enclosure" {
		t.Fatalf("read %+v", got)
	}

	if got[0].Role != connectorapi.RoleChamber || got[1].Role != "" {
		t.Errorf("roles %q and %q, want the chamber role on chamber only", got[0].Role, got[1].Role)
	}
}

// treeOf applies patches to an empty tree, a null removing the node.
func treeOf(patches []connectorapi.StatePatch) map[string]json.RawMessage {
	tree := map[string]json.RawMessage{}

	for _, patch := range patches {
		if string(patch.Value) == "null" {
			delete(tree, patch.Path)

			continue
		}

		tree[patch.Path] = patch.Value
	}

	return tree
}
