package connectorapi_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func TestToolIDsAreCanonicalDottedNumbers(t *testing.T) {
	t.Parallel()

	for id, valid := range map[connectorapi.ToolID]bool{
		"0": true, "4": true, "12": true, "0.4": true, "3.1.0": true, "123456789012345678901234567890": true,
		"": false, "00": false, "01": false, "-1": false, "0.": false, ".0": false, "0..1": false,
		"0.01": false, "a": false, "0/1": false, "count": false,
	} {
		if got := id.Valid(); got != valid {
			t.Errorf("%q valid = %v, want %v", id, got, valid)
		}
	}
}

func TestToolIDsNestAndOrder(t *testing.T) {
	t.Parallel()

	slot := connectorapi.HeadID(3).Slot(1).Slot(0)
	if slot != "3.1.0" || slot.Head() != "3" || slot.IsHead() {
		t.Fatalf("slot %q, head %q", slot, slot.Head())
	}

	if parent, ok := slot.Parent(); !ok || parent != "3.1" {
		t.Errorf("parent of %q is %q %v", slot, parent, ok)
	}

	if _, ok := connectorapi.ToolID("3").Parent(); ok {
		t.Error("a head has a parent")
	}

	ids := []connectorapi.ToolID{"10", "2", "0.10", "0.2", "0", "1"}
	slices.SortFunc(ids, connectorapi.ToolID.Compare)

	if want := []connectorapi.ToolID{"0", "0.2", "0.10", "1", "2", "10"}; !slices.Equal(ids, want) {
		t.Errorf("sorted %v, want %v", ids, want)
	}
}

func TestToolPathsSplitBackIntoTheirIDAndMember(t *testing.T) {
	t.Parallel()

	if got := connectorapi.ToolPath("0.2", connectorapi.MaterialAbbreviation); got !=
		"/tools/0.2/material_package_instance/package/material/abbreviation" {
		t.Errorf("slot material is at %s", got)
	}

	cases := []struct {
		path   string
		id     connectorapi.ToolID
		member string
		ok     bool
	}{
		{"/tools/0/features/nozzle_diameter", "0", "features/nozzle_diameter", true},
		{"/tools/7/enabled", "7", "enabled", true},
		{"/tools/3.1.0/nozzle_temp", "3.1.0", "nozzle_temp", true},
		{"/tools/40/enabled", "40", "enabled", true},
		{"/tools/00/enabled", "", "", false},
		{"/tools/-1/enabled", "", "", false},
		{"/tools/0/", "", "", false},
		{"/tools/0", "", "", false},
		{"/tool/0/material", "", "", false},
		{"/tool_count", "", "", false},
	}

	for _, tc := range cases {
		id, member, ok := connectorapi.SplitToolPath(tc.path)
		if ok != tc.ok || id != tc.id || member != tc.member {
			t.Errorf("%s: got %q %q %v", tc.path, id, member, ok)
		}
	}

	if n, ok := connectorapi.SplitGCodeToolMapPath(connectorapi.GCodeToolMapPath(12)); !ok || n != 12 {
		t.Errorf("mapping path of 12 read back as %d %v", n, ok)
	}

	for _, bad := range []string{"/gcode_tool_map/01", "/gcode_tool_map/", "/gcode_tool_map/-1", "/tools/0"} {
		if _, ok := connectorapi.SplitGCodeToolMapPath(bad); ok {
			t.Errorf("%s read as a mapping", bad)
		}
	}
}

// state builds a state tree from path and JSON text pairs.
func state(pairs ...string) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = json.RawMessage(pairs[i+1])
	}

	return out
}

func at(id connectorapi.ToolID, member string) string { return connectorapi.ToolPath(id, member) }

// mk4sWithMMU is one head with a five-slot feeder, the second slot empty.
func mk4sWithMMU() map[string]json.RawMessage {
	return state(
		connectorapi.ToolCountPath, `1`,
		at("0", connectorapi.ToolActive), `true`,
		at("0.2", connectorapi.ToolActive), `true`,
		at("0", connectorapi.ToolEnabled), `true`,
		at("0", connectorapi.FeatureNozzleDiameter), `0.4`,
		at("0", connectorapi.FeatureNozzleHardened), `true`,
		at("0", connectorapi.FeederSlotCount), `5`,
		at("0", connectorapi.NozzleTemp), `215`,
		at("0.0", connectorapi.ToolEnabled), `true`,
		at("0.0", connectorapi.MaterialAbbreviation), `"PLA"`,
		at("0.1", connectorapi.ToolEnabled), `true`,
		at("0.1", connectorapi.MaterialAbbreviation), `""`,
		at("0.2", connectorapi.MaterialAbbreviation), `"PETG"`,
		at("0.3", connectorapi.MaterialAbbreviation), `"ASA"`,
		at("0.4", connectorapi.MaterialAbbreviation), `"PLA"`,
	)
}

func TestASingleHeadWithAFeederMapsEachTNumberOntoASlot(t *testing.T) {
	t.Parallel()

	tree := connectorapi.ReadTools(mk4sWithMMU())

	if got := tree.Heads(); !slices.Equal(got, []connectorapi.ToolID{"0"}) {
		t.Fatalf("heads %v", got)
	}

	if got := tree.Slots("0"); len(got) != 5 || got[4] != "0.4" {
		t.Fatalf("slots %v", got)
	}

	for n, want := range []connectorapi.ToolID{"0.0", "0.1", "0.2", "0.3", "0.4", "0.5"} {
		if got := tree.Resolve(n); got != want {
			t.Errorf("T%d resolved to %q, want %q", n, got, want)
		}
	}

	if tree.Live("0.5") {
		t.Error("a slot past the slot count is live")
	}

	if value, ok := tree.Nozzle("0.3", "nozzle_hardened"); !ok || string(value) != "true" {
		t.Errorf("a slot prints through its head's nozzle, read %s %v", value, ok)
	}

	if !tree.IsActive("0") || !tree.IsActive("0.2") || tree.IsActive("0.1") {
		t.Error("the active slot and its head are active, and nothing else")
	}

	if empty := tree.Tool("0.1").Material; empty == nil || *empty != connectorapi.NoMaterial {
		t.Error("an empty slot is not read as a known empty slot")
	}
}

// With the feeder switched off, the slots stay in the tree and are ignored,
// and the T numbers fall back onto heads.
func TestAFeederSwitchedOffLeavesItsSlotsIgnored(t *testing.T) {
	t.Parallel()

	raw := mk4sWithMMU()
	raw[at("0", connectorapi.FeederSlotCount)] = json.RawMessage(`0`)
	raw[at("0", connectorapi.MaterialAbbreviation)] = json.RawMessage(`"PLA"`)
	tree := connectorapi.ReadTools(raw)

	if tree.Tool("0").HasFeeder() || len(tree.Slots("0")) != 0 || tree.Live("0.0") {
		t.Fatal("a feeder with no slots still feeds")
	}

	if tree.Tool("0.0") == nil {
		t.Fatal("the slots were dropped from the tree")
	}

	if got := tree.Resolve(1); got != "1" {
		t.Errorf("T1 resolved to %q, want head 1", got)
	}

	image := imageOf(t, tree)
	if feeder, held := image["0"]["feeder"]; !held || feeder != nil {
		t.Errorf("the feeder reads %v, want null", feeder)
	}
}

// Two heads each with a feeder of their own: every T number is a head by
// default, and a mapping the driver reports is what reaches a slot.
func TestNestedFeedersNeedAReportedGCodeToolMap(t *testing.T) {
	t.Parallel()

	raw := state(
		connectorapi.ToolCountPath, `2`,
		at("0", connectorapi.FeederSlotCount), `4`,
		at("1", connectorapi.FeederSlotCount), `4`,
		at("1.3", connectorapi.MaterialAbbreviation), `"PETG"`,
		at("1.3", connectorapi.FeederSlotCount), `2`,
		at("1.3.1", connectorapi.MaterialAbbreviation), `"PLA"`,
	)

	tree := connectorapi.ReadTools(raw)
	if got := tree.Resolve(1); got != "1" {
		t.Errorf("default T1 is %q, want head 1", got)
	}

	if !tree.Live("1.3.1") || tree.Live("1.3.2") || tree.Live("1.4") {
		t.Error("nested slots are not bounded by each feeder's slot count")
	}

	if got := tree.Slots("1"); !slices.Equal(got, []connectorapi.ToolID{"1.3"}) {
		t.Errorf("slots of head 1 the tree holds: %v", got)
	}

	raw[connectorapi.GCodeToolMapPath(5)] = json.RawMessage(`"1.3.1"`)
	tree = connectorapi.ReadTools(raw)

	if got := tree.Resolve(5); got != "1.3.1" {
		t.Errorf("reported T5 is %q", got)
	}

	if got := tree.Resolve(0); got != "0" {
		t.Errorf("an unmapped T0 is %q, want the default", got)
	}
}

// A disabled tool reads as the architecture's null and carries nothing else,
// and a tool nobody spoke of is left out.
func TestADisabledToolIsNull(t *testing.T) {
	t.Parallel()

	tree := connectorapi.ReadTools(state(
		connectorapi.ToolCountPath, `5`,
		at("0", connectorapi.ToolActive), `true`,
		at("0", connectorapi.ToolEnabled), `true`,
		at("0", connectorapi.ToolType), `"FFF_PRINT_HEAD"`,
		at("0", connectorapi.FeatureNozzleDiameter), `0.6`,
		at("0", connectorapi.MaterialAbbreviation), `""`,
		at("1", connectorapi.ToolEnabled), `false`,
		at("1", connectorapi.FeatureNozzleDiameter), `0.4`,
	))

	if !tree.Tool("1").Disabled || tree.Tool("1").Features != nil || tree.Live("1") {
		t.Fatal("a disabled head kept its members")
	}

	if got := tree.Resolve(3); got != "3" {
		t.Errorf("T3 on a toolchanger is %q", got)
	}

	want := map[string]map[string]any{
		"0": {
			"active": true, "type": "FFF_PRINT_HEAD",
			"features":                  map[string]any{"nozzle_diameter": 0.6},
			"material_package_instance": nil,
		},
		"1": nil,
	}

	if image := imageOf(t, tree); !reflect.DeepEqual(image, want) {
		t.Errorf("image %v, want %v", image, want)
	}
}

func TestValuesOfTheWrongTypeAreUnknown(t *testing.T) {
	t.Parallel()

	tree := connectorapi.ReadTools(state(
		connectorapi.ToolCountPath, `-1`,
		at("0", connectorapi.ToolActive), `3`,
		at("0", connectorapi.FeederSlotCount), `2.5`,
		at("0", connectorapi.MaterialAbbreviation), `7`,
		at("0", connectorapi.NozzleTemp), `"hot"`,
		"/tools/00/enabled", `false`,
		connectorapi.GCodeToolMapPath(0), `0`,
	))

	tool := tree.Tool("0")
	unknown := tool.Active == nil && tool.Feeder == nil && tool.Material == nil && tool.NozzleTemp == nil
	if tree.CountKnown || tree.Active != "" || !unknown || tree.GCodeToolMap != nil || len(tree.Tools) != 1 {
		t.Errorf("a value of the wrong type was read: %+v %+v", tree, tool)
	}
}

// imageOf is the tools object of the tree's JSON form.
func imageOf(t *testing.T, tree connectorapi.ToolTree) map[string]map[string]any {
	t.Helper()

	encoded, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}

	var whole struct {
		Tools map[string]map[string]any `json:"tools"`
	}

	if decodeErr := json.Unmarshal(encoded, &whole); decodeErr != nil {
		t.Fatalf("%s: %v", encoded, decodeErr)
	}

	return whole.Tools
}

// The active tool is the deepest active tool whose tools above it are all
// active. A slot loaded into a head that is not picked is active and is not
// the tool printing now.
func TestActiveToolOf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		active map[connectorapi.ToolID]bool
		want   connectorapi.ToolID
	}{
		{"one head", map[connectorapi.ToolID]bool{"0": true}, "0"},
		{"a slot loaded", map[connectorapi.ToolID]bool{"0": true, "0.0": false, "0.1": true}, "0.1"},
		{"no slot loaded", map[connectorapi.ToolID]bool{"0": true, "0.0": false, "0.1": false}, "0"},
		{"a toolchanger", map[connectorapi.ToolID]bool{"0": false, "1": true, "2": false}, "1"},
		{
			"a slot loaded into a head that is not picked",
			map[connectorapi.ToolID]bool{"0": true, "0.1": true, "1": false, "1.3": true},
			"0.1",
		},
		{"a slot whose head is unknown", map[connectorapi.ToolID]bool{"1.3": true}, ""},
		{"nothing picked", map[connectorapi.ToolID]bool{"0": false, "1": false}, ""},
		{"two heads say they are picked", map[connectorapi.ToolID]bool{"2": true, "1": true}, "1"},
	}

	for _, tc := range cases {
		got, ok := connectorapi.ActiveToolOf(tc.active)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%s: got %q %v, want %q", tc.name, got, ok, tc.want)
		}
	}
}

// A tool's own active member is what IsActive and the JSON form read, and the
// active tool is derived from the members when the driver wrote no path.
func TestTheActiveMemberOfEachTool(t *testing.T) {
	t.Parallel()

	tree := connectorapi.ReadTools(state(
		connectorapi.ToolCountPath, `2`,
		at("0", connectorapi.ToolActive), `true`,
		at("0", connectorapi.FeederSlotCount), `2`,
		at("0.1", connectorapi.ToolActive), `true`,
		at("1", connectorapi.ToolActive), `false`,
		at("1", connectorapi.FeederSlotCount), `2`,
		at("1.0", connectorapi.ToolActive), `true`,
		at("1.1", connectorapi.ToolActive), `false`,
	))

	if tree.Active != "0.1" {
		t.Errorf("derived active tool %q, want 0.1", tree.Active)
	}

	for id, want := range map[connectorapi.ToolID]bool{"0": true, "0.1": true, "1": false, "1.0": true, "1.1": false} {
		if got := tree.IsActive(id); got != want {
			t.Errorf("IsActive(%s) = %v, want %v", id, got, want)
		}
	}

	if image := imageOf(t, tree); image["1.0"]["active"] != true || image["1"]["active"] != false {
		t.Errorf("image %v", image)
	}

	// The JSON form carries no active tool: each tool says it.
	encoded, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(encoded), "active_tool") {
		t.Errorf("the JSON form names an active tool: %s", encoded)
	}
}

// A target of 0 is a heater that is off, which the architecture writes as a
// null target. A target nobody reported is left out.
func TestAHeaterThatIsOffReadsNull(t *testing.T) {
	t.Parallel()

	tree := connectorapi.ReadTools(state(
		at("0", connectorapi.NozzleTargetTemp), `0`,
		at("1", connectorapi.NozzleTargetTemp), `215`,
		at("2", connectorapi.NozzleTemp), `24`,
	))

	if !tree.Tool("0").HeaterOff() || tree.Tool("1").HeaterOff() || tree.Tool("2").HeaterOff() {
		t.Error("HeaterOff does not follow the target")
	}

	image := imageOf(t, tree)

	if target, held := image["0"][connectorapi.NozzleTargetTemp]; !held || target != nil {
		t.Errorf("an off heater reads %v, want null", target)
	}

	if target := image["1"][connectorapi.NozzleTargetTemp]; target != 215.0 {
		t.Errorf("a heating head reads %v", target)
	}

	if _, held := image["2"][connectorapi.NozzleTargetTemp]; held {
		t.Error("an unreported target was written")
	}
}

// The feeder names itself and reports what its unit is doing, and each tool
// reports its odometers, under the architecture's names.
func TestTheFeederAndTheOdometers(t *testing.T) {
	t.Parallel()

	tree := connectorapi.ReadTools(state(
		at("0", connectorapi.FeederSlotCount), `5`,
		at("0", connectorapi.FeederModel), `"MMU3"`,
		at("0", connectorapi.FeederBaseModel), `"MMU3"`,
		at("0", connectorapi.FeederType), `"MMU"`,
		at("0", connectorapi.FeederMMUProgressCode), `17`,
		at("0", connectorapi.FeederMMUCommandInProgress), `"L"`,
		at("0.2", connectorapi.OdometerID), `4021`,
		at("0.2", connectorapi.OdometerE), `1520.5`,
		at("0.2", connectorapi.OdometerWeight), `4.6`,
		at("1", connectorapi.FeederModel), `"MMU3"`,
	))

	feeder := tree.Tool("0").Feeder
	if feeder.Type != "MMU" || feeder.BaseModel != "MMU3" || *feeder.MMUProgressCode != 17 ||
		feeder.MMUCommandInProgress != "L" {
		t.Errorf("feeder %+v", feeder)
	}

	image := imageOf(t, tree)

	want := map[string]any{
		"slot_count": 5.0, "model": "MMU3", "base_model": "MMU3", "type": "MMU",
		"mmu_progress_code": 17.0, "mmu_command_in_progress": "L",
	}
	if got := image["0"]["feeder"]; !reflect.DeepEqual(got, want) {
		t.Errorf("feeder image %v, want %v", got, want)
	}

	slot := image["0.2"]
	if slot[connectorapi.OdometerID] != 4021.0 || slot[connectorapi.OdometerE] != 1520.5 ||
		slot[connectorapi.OdometerWeight] != 4.6 {
		t.Errorf("odometers %v", slot)
	}

	// A feeder whose slot count nobody reported is unknown, not "no feeder".
	if _, held := image["1"]["feeder"]; held {
		t.Errorf("a feeder with no slot count was written: %v", image["1"]["feeder"])
	}
}

func reportedInstance() map[string]json.RawMessage {
	return state(
		at("0", connectorapi.MaterialInstanceUUID), `"i-1"`,
		at("0", connectorapi.MaterialPackageUUID), `"p-1"`,
		at("0", connectorapi.MaterialUUID), `"m-1"`,
		at("0", connectorapi.MaterialType), `"PLA"`,
		at("0", connectorapi.MaterialAbbreviation), `"PLA"`,
		at("0", connectorapi.MaterialColor), `"#ff8800ff"`,
		at("0", connectorapi.MaterialFullWeight), `1000`,
		at("0", connectorapi.MaterialNominalFullWeight), `750`,
		at("0", connectorapi.MaterialProperty("min_print_temperature")), `190`,
		at("0", connectorapi.MaterialProperty("foo")), `1`,
	)
}

func TestReadToolsKeepsTheReportedInstance(t *testing.T) {
	t.Parallel()

	got := connectorapi.ReadTools(reportedInstance()).Tool("0").Reported
	if got == nil {
		t.Fatal("no reported instance")
	}

	if got.InstanceUUID != "i-1" || got.PackageUUID != "p-1" || got.MaterialUUID != "m-1" ||
		got.Type != "PLA" || got.ColorRGBA != "#ff8800ff" {
		t.Errorf("text members read as %+v", got)
	}

	if got.FullWeight == nil || *got.FullWeight != 1000 {
		t.Errorf("full weight %v", got.FullWeight)
	}

	if got.NominalFullWeight == nil || *got.NominalFullWeight != 750 {
		t.Errorf("nominal full weight %v", got.NominalFullWeight)
	}

	if !reflect.DeepEqual(got.Properties, map[string]float64{"min_print_temperature": 190}) {
		t.Errorf("properties %v", got.Properties)
	}
}

func TestReadToolsDropsAColourThatIsNotHex(t *testing.T) {
	t.Parallel()

	for _, colour := range []string{"red; background-image:url(https://attacker.example/x)", "red", "#ff00", "#ff00zz"} {
		t.Run(colour, func(t *testing.T) {
			t.Parallel()

			encoded, _ := json.Marshal(colour) //nolint:errchkjson // a string always encodes.
			tool := connectorapi.ReadTools(state(
				at("0", connectorapi.MaterialType), `"PLA"`,
				at("0", connectorapi.MaterialColor), string(encoded),
			)).Tool("0")

			if tool.Reported == nil || tool.Reported.Type != "PLA" || tool.Reported.ColorRGBA != "" {
				t.Errorf("read %+v", tool.Reported)
			}
		})
	}
}

func TestValidColor(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]bool{
		"#ff8800": true, "#FF8800ff": true, "": false, "#": false, "ff8800": false,
		"#ff880": false, "#ff8800f": false, "#ff8800ff0": false, "#gg8800": false, "#ff88 0": false,
	} {
		if got := connectorapi.ValidColor(text); got != want {
			t.Errorf("ValidColor(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestReadToolsIgnoresAnUnknownProperty(t *testing.T) {
	t.Parallel()

	tool := connectorapi.ReadTools(state(at("0", connectorapi.MaterialProperty("foo")), `1`)).Tool("0")
	if tool.Reported != nil {
		t.Errorf("an unknown property made %+v", tool.Reported)
	}

	only := connectorapi.ReadTools(state(at("0", connectorapi.MaterialAbbreviation), `"PLA"`)).Tool("0")
	if only.Reported != nil {
		t.Error("an abbreviation alone made a reported instance")
	}
}

func TestToolTreeJSONCarriesTheInstance(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(connectorapi.ReadTools(reportedInstance()))
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		Tools map[string]struct {
			Instance struct {
				UUID    string  `json:"uuid"`
				Weight  float64 `json:"actual_netto_full_weight"`
				Package struct {
					UUID     string  `json:"uuid"`
					Nominal  float64 `json:"nominal_netto_full_weight"`
					Material struct {
						UUID         string `json:"uuid"`
						Type         string `json:"type"`
						Abbreviation string `json:"abbreviation"`
						Color        struct {
							RGBA string `json:"color_rgba"`
						} `json:"primary_color"`
						Properties map[string]float64 `json:"properties"`
					} `json:"material"`
				} `json:"package"`
			} `json:"material_package_instance"`
		} `json:"tools"`
	}

	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}

	got := out.Tools["0"].Instance
	m := got.Package.Material

	if got.UUID != "i-1" || got.Package.UUID != "p-1" || m.UUID != "m-1" || m.Type != "PLA" ||
		m.Abbreviation != "PLA" || m.Color.RGBA != "#ff8800ff" || got.Weight != 1000 || got.Package.Nominal != 750 ||
		m.Properties["min_print_temperature"] != 190 {
		t.Errorf("the instance reads %s", raw)
	}
}

func TestProfileTemperaturesCannotBeChangedByACaller(t *testing.T) {
	theirs := connectorapi.ProfileTemperatures()
	theirs[0] = "not_a_temperature"

	if slices.Contains(connectorapi.ProfileTemperatures(), "not_a_temperature") {
		t.Fatal("a caller changed the list every other caller reads")
	}
}
