package xbuddy

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// written is the patches as path to encoded value, null for a removal.
func written(patches []connectorapi.StatePatch) map[string]string {
	out := make(map[string]string, len(patches))
	for _, patch := range patches {
		out[patch.Path] = string(patch.Value)
	}

	return out
}

// tool is the path of one member of one tool.
func tool(id connectorapi.ToolID, member string) string { return connectorapi.ToolPath(id, member) }

// numberedEntry is one entry of the numbered INFO tools block.
func numberedEntry(nozzle string, highFlow, hardened bool, material string) string {
	return `{"nozzle_diameter":` + nozzle + `,"high_flow":` + boolText(highFlow) + `,"hardened":` +
		boolText(hardened) + `,"material":"` + material + `"}`
}

func boolText(b bool) string {
	if b {
		return "true"
	}

	return "false"
}

// TestTheNumberedInfoBecomesTheTree maps the INFO tools block, in the shape
// render.cpp writes it with tools keyed from 1, onto the tree.
func TestTheNumberedInfoBecomesTheTree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		model  string
		info   string
		want   map[string]string
		absent []string
		layout int32
		heads  int
		sole   connectorapi.ToolID
	}{
		{
			name:  "an XL with five heads",
			model: "XL",
			info: `{"sn":"X","tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
				`,"2":` + numberedEntry("0.60", true, true, "PETG") +
				`,"3":` + numberedEntry("0.40", false, false, "ASA") +
				`,"4":` + numberedEntry("0.40", false, false, "---") +
				`,"5":` + numberedEntry("0.40", false, false, "PVA") + `},"slots":5}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                    "5",
				tool("0", connectorapi.ToolEnabled):           "true",
				tool("0", connectorapi.ToolType):              `"FFF_PRINT_HEAD"`,
				tool("0", connectorapi.FeatureNozzleDiameter): "0.40",
				tool("1", connectorapi.FeatureNozzleDiameter): "0.60",
				tool("1", connectorapi.FeatureNozzleHighFlow): "true",
				tool("1", connectorapi.FeatureNozzleHardened): "true",
				tool("1", connectorapi.MaterialAbbreviation):  `"PETG"`,
				tool("3", connectorapi.MaterialAbbreviation):  `""`,
				tool("4", connectorapi.MaterialAbbreviation):  `"PVA"`,
				tool("4", connectorapi.FeederSlotCount):       "0",
			},
			absent: []string{tool("5", connectorapi.ToolEnabled), "/mmu/enabled"},
			layout: layoutHeads, heads: 5,
		},
		{
			name:  "an XL with heads 1 and 3 of 5 fitted",
			model: "XL",
			info: `{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
				`,"3":` + numberedEntry("0.40", false, false, "PETG") + `},"slots":2}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                    "5",
				tool("0", connectorapi.ToolEnabled):           "true",
				tool("1", connectorapi.ToolEnabled):           "false",
				tool("1", connectorapi.FeatureNozzleDiameter): "null",
				tool("1", connectorapi.NozzleTemp):            "null",
				tool("2", connectorapi.MaterialAbbreviation):  `"PETG"`,
				tool("3", connectorapi.ToolEnabled):           "false",
				tool("4", connectorapi.ToolEnabled):           "false",
			},
			layout: layoutHeads, heads: 5,
		},
		{
			name:  "an INDX with eight heads",
			model: "CoreOne_INDX",
			info: `{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
				`,"8":` + numberedEntry("0.40", false, true, "PETG") + `},"slots":2}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                    "8",
				tool("7", connectorapi.FeatureNozzleHardened): "true",
				tool("7", connectorapi.MaterialAbbreviation):  `"PETG"`,
				tool("6", connectorapi.ToolEnabled):           "false",
			},
			layout: layoutHeads, heads: 8,
		},
		{
			name:  "an INDX in the firmware's spelling with four heads stays four",
			model: "COREONEINDX",
			info: `{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
				`,"4":` + numberedEntry("0.40", false, true, "PETG") + `},"slots":2}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                   "4",
				tool("3", connectorapi.MaterialAbbreviation): `"PETG"`,
				tool("1", connectorapi.ToolEnabled):          "false",
			},
			absent: []string{tool("4", connectorapi.ToolEnabled)},
			layout: layoutHeads, heads: 4,
		},
		{
			name:  "an INDX L in the firmware's spelling with eight heads",
			model: "COREONEL-INDX",
			info: `{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
				`,"8":` + numberedEntry("0.40", false, true, "PETG") + `},"slots":2}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                   "8",
				tool("7", connectorapi.MaterialAbbreviation): `"PETG"`,
				tool("6", connectorapi.ToolEnabled):          "false",
			},
			layout: layoutHeads, heads: 8,
		},
		{
			name:  "an INDX with three heads listed is a four-head build",
			model: "CoreOne_INDX",
			info: `{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
				`,"3":` + numberedEntry("0.40", false, false, "PETG") + `},"slots":2}`,
			want: map[string]string{
				connectorapi.ToolCountPath:          "4",
				tool("3", connectorapi.ToolEnabled): "false",
			},
			layout: layoutHeads, heads: 4,
		},
		{
			name:  "an MK4S with an MMU3 is one head with a feeder of five slots",
			model: "MK4S",
			info: `{"tools":{"1":` + numberedEntry("0.40", true, false, "PLA") +
				`,"2":` + numberedEntry("0.40", true, false, "PETG") +
				`,"3":` + numberedEntry("0.40", true, false, "---") +
				`,"4":` + numberedEntry("0.40", true, false, "ASA") +
				`,"5":` + numberedEntry("0.40", true, false, "---") + `},` +
				`"mmu":{"enabled":true,"version":"3.0.3"},"slots":5}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                     "1",
				tool("0", connectorapi.ToolType):               `"FFF_PRINT_HEAD"`,
				tool("0", connectorapi.FeatureNozzleDiameter):  "0.40",
				tool("0", connectorapi.FeatureNozzleHighFlow):  "true",
				tool("0", connectorapi.FeederSlotCount):        "5",
				tool("0", connectorapi.MaterialAbbreviation):   "null",
				tool("0.0", connectorapi.ToolType):             `"FFF_FEEDER_SLOT"`,
				tool("0.0", connectorapi.MaterialAbbreviation): `"PLA"`,
				tool("0.1", connectorapi.MaterialAbbreviation): `"PETG"`,
				tool("0.2", connectorapi.MaterialAbbreviation): `""`,
				tool("0.4", connectorapi.MaterialAbbreviation): `""`,
				tool("0.4", connectorapi.ToolEnabled):          "true",
			},
			absent: []string{tool("1", connectorapi.ToolEnabled), tool("0.0", connectorapi.FeatureNozzleDiameter)},
			layout: layoutFeeder, heads: 1, sole: "0",
		},
		{
			name:  "an MK4S with its MMU switched off keeps the slots and drops the feeder",
			model: "MK4S",
			info: `{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") + `},` +
				`"mmu":{"enabled":false,"version":"0.0.0"},"slots":1}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                   "1",
				tool("0", connectorapi.FeederSlotCount):      "0",
				tool("0", connectorapi.MaterialAbbreviation): `"PLA"`,
			},
			absent: []string{tool("0.0", connectorapi.ToolEnabled), tool("0.0", connectorapi.MaterialAbbreviation)},
			layout: layoutHeads, heads: 1, sole: "0",
		},
		{
			name:   "firmware without a tools block names the first nozzle only",
			model:  "MK4",
			info:   `{"sn":"X","nozzle_diameter":0.40,"storages":[]}`,
			want:   map[string]string{tool("0", connectorapi.FeatureNozzleDiameter): "0.40"},
			absent: []string{connectorapi.ToolCountPath, tool("0", connectorapi.ToolEnabled)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.False(t, treeShaped(json.RawMessage(tc.info)), "the numbered shape read as the tree")

			report := infoTools(json.RawMessage(tc.info), tc.model)
			got := written(report.patches)

			for path, value := range tc.want {
				require.Equal(t, value, got[path], path)
			}

			for _, path := range tc.absent {
				require.NotContains(t, got, path)
			}

			require.Equal(t, tc.layout, report.layout)
			require.Len(t, report.heads, tc.heads)
			require.Equal(t, tc.sole, report.sole)

			for path := range got {
				require.False(t, strings.HasPrefix(path, "/tool/") || strings.HasPrefix(path, "/slot/"), path)
			}

			for i := 1; i < len(report.patches); i++ {
				require.LessOrEqual(t, report.patches[i-1].Path, report.patches[i].Path)
			}
		})
	}
}

// TestTheTreeShapedInfoIsReadAsItIs reads a firmware that speaks the
// architecture: tool_count, ids from "0", null for a disabled tool.
func TestTheTreeShapedInfoIsReadAsItIs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		info   string
		want   map[string]string
		absent []string
		heads  int
	}{
		{
			name: "an MK4S with an MMU3, one slot disabled",
			info: `{"tool_count":1,"tools":{` +
				`"0":{"type":"FFF_PRINT_HEAD","features":{"nozzle_diameter":0.4,"nozzle_hardened":true,` +
				`"nozzle_high_flow":true},"feeder":{"type":"MMU","model":"MMU3","base_model":"MMU3","slot_count":5}},` +
				`"0.0":{"type":"FFF_FEEDER_SLOT","material_package_instance":{"package":{"material":{"abbreviation":"PLA"}}}},` +
				`"0.1":null,` +
				`"0.2":{"type":"FFF_FEEDER_SLOT","material_package_instance":{"package":{"material":{"type":"PETG"}}}},` +
				`"0.3":{"type":"FFF_FEEDER_SLOT","material_package_instance":null},` +
				`"0.4":{"type":"FFF_FEEDER_SLOT"}}}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                     "1",
				tool("0", connectorapi.FeatureNozzleHardened):  "true",
				tool("0", connectorapi.FeatureNozzleHighFlow):  "true",
				tool("0", connectorapi.FeederSlotCount):        "5",
				tool("0", connectorapi.FeederModel):            `"MMU3"`,
				tool("0.0", connectorapi.MaterialAbbreviation): `"PLA"`,
				tool("0.1", connectorapi.ToolEnabled):          "false",
				tool("0.1", connectorapi.MaterialAbbreviation): "null",
				tool("0.2", connectorapi.MaterialAbbreviation): `"PETG"`,
				tool("0.3", connectorapi.MaterialAbbreviation): `""`,
				tool("0.4", connectorapi.ToolEnabled):          "true",
			},
			absent: []string{tool("0.4", connectorapi.MaterialAbbreviation)},
			heads:  1,
		},
		{
			name: "two heads each with a feeder of its own",
			info: `{"tool_count":2,"tools":{` +
				`"0":{"type":"FFF_PRINT_HEAD","features":{"nozzle_diameter":0.4},"feeder":{"slot_count":4}},` +
				`"0.3":{"type":"FFF_FEEDER_SLOT","material_package_instance":{"package":{"material":{"abbreviation":"ASA"}}}},` +
				`"1":{"type":"FFF_PRINT_HEAD","features":{"nozzle_diameter":0.6,"nozzle_high_temp":true},"feeder":{"slot_count":4}},` +
				`"1.2":{"type":"FFF_FEEDER_SLOT","feeder":{"slot_count":2}},` +
				`"1.2.1":{"type":"FFF_FEEDER_SLOT","material_package_instance":{"package":{"material":{"abbreviation":"PC"}}}}}}`,
			want: map[string]string{
				connectorapi.ToolCountPath:                       "2",
				tool("1", connectorapi.FeatureNozzleHighTemp):    "true",
				tool("1", connectorapi.FeederSlotCount):          "4",
				tool("1.2", connectorapi.FeederSlotCount):        "2",
				tool("1.2.1", connectorapi.MaterialAbbreviation): `"PC"`,
				tool("0.3", connectorapi.MaterialAbbreviation):   `"ASA"`,
			},
			heads: 2,
		},
		{
			name: "a disabled head and a head with no feeder",
			info: `{"tool_count":2,"tools":{"0":{"type":"FFF_PRINT_HEAD","feeder":null,` +
				`"material_package_instance":null,"features":{"nozzle_diameter":0.4,"bad name":1,"nested":{"a":1}}},"1":null}}`,
			want: map[string]string{
				tool("0", connectorapi.FeederSlotCount):      "0",
				tool("0", connectorapi.MaterialAbbreviation): `""`,
				tool("1", connectorapi.ToolEnabled):          "false",
				tool("1", connectorapi.FeederSlotCount):      "null",
			},
			absent: []string{"/tools/0/features/bad name", tool("0", "features/nested")},
			heads:  2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.True(t, treeShaped(json.RawMessage(tc.info)))

			report := infoTools(json.RawMessage(tc.info), "MK4S")
			got := written(report.patches)

			for path, value := range tc.want {
				require.Equal(t, value, got[path], path)
			}

			for _, path := range tc.absent {
				require.NotContains(t, got, path)
			}

			require.Len(t, report.heads, tc.heads)
			require.Equal(t, layoutUnknown, report.layout)
		})
	}
}

// TestTheShapeIsDetectedFromTheFrame names what makes a frame tree-shaped.
func TestTheShapeIsDetectedFromTheFrame(t *testing.T) {
	t.Parallel()

	for info, tree := range map[string]bool{
		`{"tools":{"1":{"nozzle_diameter":0.4}},"slots":1}`:  false,
		`{"tools":{"1":{"material":"PLA"},"2":{}},"mmu":{}}`: false,
		`{"nozzle_diameter":0.4}`:                            false,
		`{"tools":{}}`:                                       false,
		`[1]`:                                                false,
		`{"tool_count":1}`:                                   true,
		`{"tools":{"0":{"nozzle_diameter":0.4}}}`:            true,
		`{"tools":{"1.0":{}}}`:                               true,
		`{"tools":{"1":null}}`:                               true,
		`{"tools":{"1":{"features":{}}}}`:                    true,
		`{"tools":{"1":{"type":"FFF_PRINT_HEAD"}}}`:          true,
	} {
		require.Equal(t, tree, treeShaped(json.RawMessage(info)), info)
	}

	require.Empty(t, infoTools(json.RawMessage(`[1,2]`), "XL").patches)
	require.Empty(t, infoTools(nil, "XL").patches)
}

// TestReservedPathsNameTheToolsINFOWrites names the paths a full queue
// must not drop, and a few that look like them and are not. The active
// members and the active tool are reported only when they change, so they
// are reserved too.
func TestReservedPathsNameTheToolsINFOWrites(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]bool{
		connectorapi.ToolCountPath:                     true,
		tool("0", connectorapi.FeatureNozzleDiameter):  true,
		tool("12", connectorapi.ToolEnabled):           true,
		tool("0.3", connectorapi.MaterialAbbreviation): true,
		tool("0", connectorapi.FeederSlotCount):        true,
		tool("0", connectorapi.NozzleTemp):             false,
		"/active_tool":                                 false,
		tool("0.2", connectorapi.ToolActive):           true,
		"/tools/00/enabled":                            false,
		"/slot/active":                                 false,
		"/material":                                    false,
	} {
		require.Equal(t, want, reservedPath(path), path)
	}

	require.Len(t, legacySlotPaths(), 1+len(legacyFlatPaths)+legacySlotCount*len(legacySlotMembers))
}

const reportedInstance = `{"uuid":"i-1","package":{"uuid":"p-1","nominal_netto_full_weight":750,"material":{"uuid":"m-1","type":"PLA",` +
	`"abbreviation":"PLA","primary_color":{"color_rgba":"#ff0000ff"},` +
	`"properties":{"min_print_temperature":190,"max_print_temperature":230}}` +
	`},"actual_netto_full_weight":1000}`

func treeInfo(instance string) string {
	return `{"tool_count":1,"tools":{"0":{"type":"FFF_PRINT_HEAD","material_package_instance":` + instance + `}}}`
}

func TestTreeMaterialPublishesTheInstance(t *testing.T) {
	t.Parallel()

	got := written(infoTools(json.RawMessage(treeInfo(reportedInstance)), "XL").patches)

	for member, want := range map[string]string{
		connectorapi.MaterialInstanceUUID:                      `"i-1"`,
		connectorapi.MaterialPackageUUID:                       `"p-1"`,
		connectorapi.MaterialUUID:                              `"m-1"`,
		connectorapi.MaterialType:                              `"PLA"`,
		connectorapi.MaterialAbbreviation:                      `"PLA"`,
		connectorapi.MaterialColor:                             `"#ff0000ff"`,
		connectorapi.MaterialFullWeight:                        "1000",
		connectorapi.MaterialNominalFullWeight:                 "750",
		connectorapi.MaterialProperty("min_print_temperature"): "190",
		connectorapi.MaterialProperty("max_print_temperature"): "230",
	} {
		require.Equal(t, want, got[tool("0", member)], member)
	}

	require.Equal(t, "null", got[tool("0", connectorapi.MaterialProperty("preheat_temperature"))])
}

func TestTreeMaterialDropsTheInstanceItNoLongerReports(t *testing.T) {
	t.Parallel()

	gone := []string{
		connectorapi.MaterialInstanceUUID, connectorapi.MaterialPackageUUID, connectorapi.MaterialUUID,
		connectorapi.MaterialType, connectorapi.MaterialColor, connectorapi.MaterialFullWeight,
		connectorapi.MaterialNominalFullWeight,
		connectorapi.MaterialProperty("min_print_temperature"),
		connectorapi.MaterialProperty("max_print_temperature"),
	}

	abbreviationOnly := treeInfo(`{"package":{"material":{"abbreviation":"PLA"}}}`)
	got := written(infoTools(json.RawMessage(abbreviationOnly), "XL").patches)
	require.Equal(t, `"PLA"`, got[tool("0", connectorapi.MaterialAbbreviation)])

	for _, member := range gone {
		require.Equal(t, "null", got[tool("0", member)], member)
	}

	got = written(infoTools(json.RawMessage(treeInfo(`null`)), "XL").patches)
	require.Equal(t, `""`, got[tool("0", connectorapi.MaterialAbbreviation)])

	for _, member := range gone {
		require.Equal(t, "null", got[tool("0", member)], member)
	}
}

func TestTreeMaterialKeepsTheAbbreviationPastAnOddColourOrProperties(t *testing.T) {
	t.Parallel()

	odd := treeInfo(`{"uuid":"i-1","package":{"material":{"abbreviation":"PETG",` +
		`"primary_color":"#ff0000ff","properties":[190,230]}}}`)
	got := written(infoTools(json.RawMessage(odd), "XL").patches)

	require.Equal(t, `"PETG"`, got[tool("0", connectorapi.MaterialAbbreviation)])
	require.Equal(t, `"i-1"`, got[tool("0", connectorapi.MaterialInstanceUUID)])
	require.Equal(t, "null", got[tool("0", connectorapi.MaterialColor)])
	require.Equal(t, "null", got[tool("0", connectorapi.MaterialProperty("min_print_temperature"))])
}

func TestTreeMaterialDropsAColourThatIsNotHex(t *testing.T) {
	t.Parallel()

	for _, colour := range []string{
		"red; background-image:url(https://attacker.example/x)",
		"red",
		"#ff00",
		"#ff00zz",
		"#ff0000ff00",
	} {
		t.Run(colour, func(t *testing.T) {
			t.Parallel()

			odd := treeInfo(`{"uuid":"i-1","package":{"material":{"abbreviation":"PLA",` +
				`"primary_color":{"color_rgba":` + strconv.Quote(colour) + `}}}}`)
			got := written(infoTools(json.RawMessage(odd), "XL").patches)

			require.Equal(t, `"PLA"`, got[tool("0", connectorapi.MaterialAbbreviation)])
			require.Equal(t, "null", got[tool("0", connectorapi.MaterialColor)])
		})
	}
}

func TestFlatMaterialStaysAbbreviationOnly(t *testing.T) {
	t.Parallel()

	flat := `{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") + `}}`
	got := written(infoTools(json.RawMessage(flat), "XL").patches)

	require.Equal(t, `"PLA"`, got[tool("0", connectorapi.MaterialAbbreviation)])
	require.NotContains(t, got, tool("0", connectorapi.MaterialInstanceUUID))
	require.NotContains(t, got, tool("0", connectorapi.MaterialColor))
}
