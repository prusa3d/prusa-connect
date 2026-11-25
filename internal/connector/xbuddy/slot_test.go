package xbuddy

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
	"testing/synctest"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func pathsOf(patches []connectorapi.StatePatch) map[string]string { return written(patches) }

// TestTheSlotBlockLandsOnTheTree reads the numbered telemetry slot block on a
// toolchanger, where each entry is a head, and in front of an MMU, where each
// entry is a slot of head 0.
func TestTheSlotBlockLandsOnTheTree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		block  string
		layout int32
		want   map[string]string
		active activity
	}{
		{
			name: "an XL",
			block: `{"active":2,"state":1,` +
				`"1":{"material":"PLA","temp":215.0,"fan_print":6000,"fan_hotend":8000},` +
				`"2":{"material":"PETG","temp":240},"3":{"temp":30},"4":{},"5":{"fan_print":0}}`,
			layout: layoutHeads,
			want: map[string]string{
				tool("0", connectorapi.MaterialAbbreviation): `"PLA"`,
				tool("0", connectorapi.NozzleTemp):           "215.0",
				tool("0", connectorapi.PrintFanRPM):          "6000",
				tool("0", connectorapi.HeatbreakFanRPM):      "8000",
				tool("1", connectorapi.MaterialAbbreviation): `"PETG"`,
				tool("1", connectorapi.NozzleTemp):           "240",
				tool("2", connectorapi.NozzleTemp):           "30",
				tool("4", connectorapi.PrintFanRPM):          "0",
			},
			active: activity{"0": false, "1": true, "2": false, "3": false, "4": false},
		},
		{
			// An INDX reads only the head on the carriage, and sends -1 for
			// each docked one. That is no reading, so it removes the head's
			// temperature instead of writing -1.
			name: "an INDX with its second head picked",
			block: `{"active":2,` +
				`"1":{"material":"ASA","temp":-1.0},"2":{"material":"ABS","temp":122.0},` +
				`"3":{"temp":-1.0},"4":{"temp":0.0},"5":{"temp":-0.5}}`,
			layout: layoutHeads,
			want: map[string]string{
				tool("0", connectorapi.MaterialAbbreviation): `"ASA"`,
				tool("0", connectorapi.NozzleTemp):           "null",
				tool("1", connectorapi.MaterialAbbreviation): `"ABS"`,
				tool("1", connectorapi.NozzleTemp):           "122.0",
				tool("2", connectorapi.NozzleTemp):           "null",
				tool("3", connectorapi.NozzleTemp):           "0.0",
				tool("4", connectorapi.NozzleTemp):           "-0.5",
			},
			active: activity{"0": false, "1": true, "2": false, "3": false, "4": false},
		},
		{
			name: "an MK4S with an MMU3",
			block: `{"active":3,"state":1,` +
				`"1":{"material":"PLA","temp":215,"fan_print":0,"fan_hotend":8000},` +
				`"2":{"material":"---","temp":215,"fan_print":0,"fan_hotend":8000},` +
				`"3":{"material":"PETG","temp":215,"fan_print":5000,"fan_hotend":8100}}`,
			layout: layoutFeeder,
			want: map[string]string{
				tool("0.0", connectorapi.MaterialAbbreviation): `"PLA"`,
				tool("0.1", connectorapi.MaterialAbbreviation): `""`,
				tool("0.2", connectorapi.MaterialAbbreviation): `"PETG"`,
				tool("0", connectorapi.PrintFanRPM):            "5000",
				tool("0", connectorapi.HeatbreakFanRPM):        "8100",
				tool("0", connectorapi.FeederMMUProgressCode):  "1",
			},
			active: activity{"0": true, "0.0": false, "0.1": false, "0.2": true},
		},
		{
			name:   "no tool selected",
			block:  `{"active":0,"1":{"material":""},"2":{"material":"---"}}`,
			layout: layoutHeads,
			want: map[string]string{
				tool("0", connectorapi.MaterialAbbreviation): `""`,
				tool("1", connectorapi.MaterialAbbreviation): `""`,
			},
			active: activity{"0": false, "1": false},
		},
		{
			name:   "an MMU loading a slot",
			block:  `{"active":1,"state":17,"command":"L","1":{"material":"PLA"}}`,
			layout: layoutFeeder,
			want: map[string]string{
				tool("0.0", connectorapi.MaterialAbbreviation):     `"PLA"`,
				tool("0", connectorapi.FeederMMUProgressCode):      "17",
				tool("0", connectorapi.FeederMMUCommandInProgress): `"L"`,
			},
			active: activity{"0": true, "0.0": true},
		},
		{
			// The firmware formats an idle unit's command 0 as a NUL.
			name:   "an idle MMU",
			block:  `{"active":1,"state":0,"command":"\u0000","1":{"material":"PLA"}}`,
			layout: layoutFeeder,
			want: map[string]string{
				tool("0.0", connectorapi.MaterialAbbreviation):     `"PLA"`,
				tool("0", connectorapi.FeederMMUProgressCode):      "0",
				tool("0", connectorapi.FeederMMUCommandInProgress): "null",
			},
			active: activity{"0": true, "0.0": true},
		},
		{
			// An XL has no MMU, and what its block says of one is ignored.
			name:   "a state on a toolchanger",
			block:  `{"active":1,"state":3,"command":"L","1":{"material":"PLA"}}`,
			layout: layoutHeads,
			want: map[string]string{
				tool("0", connectorapi.MaterialAbbreviation): `"PLA"`,
			},
			active: activity{"0": true},
		},
		{
			name:   "an MMU with no slot loaded",
			block:  `{"active":0,"1":{"material":"PLA"},"2":{"material":"PETG"}}`,
			layout: layoutFeeder,
			want: map[string]string{
				tool("0.0", connectorapi.MaterialAbbreviation): `"PLA"`,
				tool("0.1", connectorapi.MaterialAbbreviation): `"PETG"`,
			},
			active: activity{"0": true, "0.0": false, "0.1": false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			patches, active := numberedSlotPatches(json.RawMessage(tc.block), tc.layout)
			if got := pathsOf(patches); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("wrote %v, want %v", got, tc.want)
			}

			if !reflect.DeepEqual(active, tc.active) {
				t.Errorf("said %v is active, want %v", active, tc.active)
			}

			for i := 1; i < len(patches); i++ {
				if patches[i-1].Path > patches[i].Path {
					t.Errorf("patches are not sorted: %s before %s", patches[i-1].Path, patches[i].Path)
				}
			}
		})
	}
}

func TestTheSlotBlockIgnoresWhatIsNotASlot(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"state":1,"0":{"temp":1},"-1":{"temp":1},"01":{"temp":1},"x":{"temp":1},` +
		`"1":5,"2":"PLA","3":null,"4":[1],"active":-1}`)

	if got, active := numberedSlotPatches(raw, layoutHeads); len(got) != 0 || active != nil {
		t.Errorf("junk in the slot block wrote %v and %v, want nothing", pathsOf(got), active)
	}

	for _, bad := range []string{``, `null`, `3`, `"x"`, `[1]`} {
		if got, active := numberedSlotPatches(json.RawMessage(bad), layoutFeeder); len(got) != 0 || active != nil {
			t.Errorf("slot value %q wrote %v and %v, want nothing", bad, pathsOf(got), active)
		}
	}
}

// TestTheTreeShapedTelemetryIsReadAsItIs reads the architecture's printer
// telemetry: each tool's active member is what it says, a null target is a
// heater that is off, and any other null removes.
func TestTheTreeShapedTelemetryIsReadAsItIs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		tools  string
		want   map[string]string
		nulls  []connectorapi.ToolID
		active activity
	}{
		{
			name: "a head with a feeder and slot 2 loaded",
			tools: `{"0":{"active":true,"nozzle_temp":215.5,"nozzle_target_temp":null,"print_fan_rpm":0,` +
				`"heatbreak_fan_rpm":7000,"feeder":{"mmu_progress_code":0}},` +
				`"0.1":{"active":false,"consumed_weight":12.5},"0.2":{"active":true,"consumed_weight":null},"0.3":null}`,
			want: map[string]string{
				tool("0", connectorapi.NozzleTemp):                 "215.5",
				tool("0", connectorapi.NozzleTargetTemp):           "0",
				tool("0", connectorapi.PrintFanRPM):                "0",
				tool("0", connectorapi.HeatbreakFanRPM):            "7000",
				tool("0", connectorapi.FeederMMUProgressCode):      "0",
				tool("0", connectorapi.FeederMMUCommandInProgress): "null",
				tool("0.1", connectorapi.ConsumedWeight):           "12.5",
				tool("0.2", connectorapi.ConsumedWeight):           "null",
			},
			nulls:  []connectorapi.ToolID{"0.3"},
			active: activity{"0": true, "0.1": false, "0.2": true},
		},
		{
			name: "a feeder loading, and odometers",
			tools: `{"0":{"active":true,"feeder":{"mmu_progress_code":17,"mmu_command_in_progress":"L"}},` +
				`"0.1":{"active":true,"odometer_id":7,"odometer_e":12.5,"odometer_weight":null}}`,
			want: map[string]string{
				tool("0", connectorapi.FeederMMUProgressCode):      "17",
				tool("0", connectorapi.FeederMMUCommandInProgress): `"L"`,
				tool("0.1", connectorapi.OdometerID):               "7",
				tool("0.1", connectorapi.OdometerE):                "12.5",
				tool("0.1", connectorapi.OdometerWeight):           "null",
			},
			active: activity{"0": true, "0.1": true},
		},
		{
			name:  "a toolchanger with no head picked",
			tools: `{"0":{"active":false,"nozzle_temp":30},"1":{"active":false},"x":{"nozzle_temp":1}}`,
			want: map[string]string{
				tool("0", connectorapi.NozzleTemp): "30",
			},
			active: activity{"0": false, "1": false},
		},
		{
			// An INDX reads only the head on the carriage, and sends -1 for
			// each docked one. That is no reading, so it removes the head's
			// temperature instead of writing -1.
			name: "an INDX with its second head picked",
			tools: `{"0":{"active":false,"nozzle_temp":-1.0},"1":{"active":true,"nozzle_temp":122.0},` +
				`"2":{"active":false,"nozzle_temp":0},"3":{"active":false,"nozzle_temp":-0.5}}`,
			want: map[string]string{
				tool("0", connectorapi.NozzleTemp): "null",
				tool("1", connectorapi.NozzleTemp): "122.0",
				tool("2", connectorapi.NozzleTemp): "0",
				tool("3", connectorapi.NozzleTemp): "-0.5",
			},
			active: activity{"0": false, "1": true, "2": false, "3": false},
		},
		{
			name:  "nested feeders",
			tools: `{"1":{"active":true},"1.2":{"active":true},"1.2.1":{"active":true,"nozzle_temp":1}}`,
			want: map[string]string{
				tool("1.2.1", connectorapi.NozzleTemp): "1",
			},
			active: activity{"1": true, "1.2": true, "1.2.1": true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			patches, nulls, active := treeTelemetryPatches(json.RawMessage(tc.tools))
			if got := pathsOf(patches); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("wrote %v, want %v", got, tc.want)
			}

			if !reflect.DeepEqual(nulls, tc.nulls) {
				t.Errorf("reported %v as null, want %v", nulls, tc.nulls)
			}

			if !reflect.DeepEqual(active, tc.active) {
				t.Errorf("said %v is active, want %v", active, tc.active)
			}
		})
	}
}

// TestATargetLeavesAHeadThatIsNoLongerPicked holds the nozzle target at the top
// of a numbered XL frame to the head picked when the frame was sent. The slot
// block carries no head's target, so a head that is put down has no target the
// printer says, and the one it had while picked must not stay on the tree.
func TestATargetLeavesAHeadThatIsNoLongerPicked(t *testing.T) {
	t.Parallel()

	frame := func(active int, target string) string {
		return `{"state":"PRINTING","temp_nozzle":215,"target_nozzle":` + target +
			`,"slot":{"active":` + strconv.Itoa(active) + `,"state":1,` +
			`"1":{"material":"PLA","temp":215},"2":{"material":"PETG","temp":180}}}`
	}

	steps := []struct {
		name  string
		frame string
		want  map[string]string
	}{
		{
			name: "head 0 picked", frame: frame(1, "215"),
			want: map[string]string{tool("0", connectorapi.NozzleTargetTemp): "215"},
		},
		{
			name: "head 1 picked", frame: frame(2, "240"),
			want: map[string]string{
				tool("0", connectorapi.NozzleTargetTemp): "null",
				tool("1", connectorapi.NozzleTargetTemp): "240",
			},
		},
		{
			name: "head 1 still picked", frame: frame(2, "245"),
			want: map[string]string{tool("1", connectorapi.NozzleTargetTemp): "245"},
		},
		{
			name: "no head picked", frame: frame(0, "0"),
			want: map[string]string{tool("1", connectorapi.NozzleTargetTemp): "null"},
		},
	}

	synctest.Test(t, func(t *testing.T) {
		session := newSession(settings{
			identity: connectorapi.Identity{Vendor: Vendor, Model: "XL", Serial: "SN-XL-TARGET"},
			storage:  defaultStorage,
		})
		defer func() { _ = session.Close() }()

		for _, step := range steps {
			feed(session, telemetry(t, []byte(step.frame)))

			got := map[string]string{}

			for path, value := range toolPatches(session) {
				if _, member, ok := connectorapi.SplitToolPath(path); ok && member == connectorapi.NozzleTargetTemp {
					got[path] = value
				}
			}

			if !reflect.DeepEqual(got, step.want) {
				t.Errorf("%s: wrote targets %v, want %v", step.name, got, step.want)
			}
		}
	})
}
