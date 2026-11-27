package xbuddy

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// A transition is a printer changing what it has between two frames: an MMU
// switched off and on, a head going null and coming back, the error state
// where every head is null, a slot going null, and a member the printer stops
// reporting. Each step goes through the session's own INFO and telemetry
// paths, and the test holds the whole set of tool patches the step writes:
// what the new frame says, and the removal of everything the previous frames
// said that the new one leaves out. ADR-20261023-gna rule 8 gives the values of the
// three nulls.

// transitionStep is one frame and every tool patch it must write, as path to
// encoded value, with null for a removal.
type transitionStep struct {
	name  string
	frame inboundMessage
	want  map[string]string
}

// fixtureFrame reads one fixture under testdata/tools. edit, when not nil,
// changes the tools object before the frame is built.
func fixtureFrame(t *testing.T, fixture string, edit func(tools map[string]json.RawMessage)) []byte {
	t.Helper()

	data, err := os.ReadFile("testdata/tools/" + fixture)
	if err != nil {
		t.Fatalf("read %s: %v", fixture, err)
	}

	if edit == nil {
		return data
	}

	var frame map[string]json.RawMessage
	if decodeErr := json.Unmarshal(data, &frame); decodeErr != nil {
		t.Fatalf("%s: %v", fixture, decodeErr)
	}

	var tools map[string]json.RawMessage
	if decodeErr := json.Unmarshal(frame["tools"], &tools); decodeErr != nil {
		t.Fatalf("%s tools: %v", fixture, decodeErr)
	}

	edit(tools)

	frame["tools"] = mustJSON(t, tools)

	return mustJSON(t, frame)
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	return encoded
}

// info is an INFO event carrying data.
func info(t *testing.T, data []byte) inboundMessage {
	t.Helper()

	message, err := classify([]byte(`{"event":"INFO","data":` + string(data) + `}`))
	if err != nil {
		t.Fatalf("classify INFO: %v", err)
	}

	return message
}

// telemetry is a telemetry frame.
func telemetry(t *testing.T, data []byte) inboundMessage {
	t.Helper()

	message, err := classify(data)
	if err != nil || message.kind != inboundTelemetry {
		t.Fatalf("classify telemetry: %v (kind %v)", err, message.kind)
	}

	return message
}

// feed hands one frame to the path the session reads it on.
func feed(s *Session, message inboundMessage) {
	if message.kind == inboundEvent {
		s.observe(message)

		return
	}

	s.dispatchTelemetry(message)
}

// toolPatches takes every patch the session wrote and keeps the tool paths.
// It runs in a synctest bubble: once every goroutine of the session is
// blocked, nothing more is on its way. A path written twice keeps its last
// value, which is what the host keeps.
func toolPatches(s *Session) map[string]string {
	out := map[string]string{}

	for {
		synctest.Wait()

		select {
		case patch := <-s.State():
			if _, _, ok := connectorapi.SplitToolPath(patch.Path); ok ||
				patch.Path == connectorapi.ToolCountPath {
				out[patch.Path] = string(patch.Value)
			}
		default:
			return out
		}
	}
}

// members writes the same value for several members of one tool.
func members(into map[string]string, id connectorapi.ToolID, value string, names ...string) {
	for _, name := range names {
		into[tool(id, name)] = value
	}
}

// removedTool is every member of a tool removed, as when the tool leaves the
// tree.
func removedTool(into map[string]string, id connectorapi.ToolID) {
	members(into, id, "null", connectorapi.ToolMembers()...)
}

// disabledTool is the architecture's null tool: enabled false, and every
// other member removed.
func disabledTool(into map[string]string, id connectorapi.ToolID) {
	removedTool(into, id)
	into[tool(id, connectorapi.ToolEnabled)] = "false"
}

// merge is the union of patch sets, the later winning.
func merge(sets ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, set := range sets {
		maps.Copy(out, set)
	}

	return out
}

// head is what a tree-shaped INFO writes for one enabled head with the three
// nozzle features.
func head(id connectorapi.ToolID, nozzle string) map[string]string {
	return map[string]string{
		tool(id, connectorapi.ToolEnabled):           "true",
		tool(id, connectorapi.ToolType):              `"FFF_PRINT_HEAD"`,
		tool(id, connectorapi.FeatureNozzleDiameter): nozzle,
		tool(id, connectorapi.FeatureNozzleHardened): "false",
		tool(id, connectorapi.FeatureNozzleHighFlow): "false",
	}
}

// noInstance is every member of the material package instance but the
// abbreviation removed, as INFO writes it for a spool that reports nothing
// else.
func noInstance(id connectorapi.ToolID) map[string]string {
	out := map[string]string{}

	for _, member := range connectorapi.ToolMembers() {
		if strings.HasPrefix(member, "material_package_instance/") && member != connectorapi.MaterialAbbreviation {
			out[tool(id, member)] = "null"
		}
	}

	return out
}

// fullInstance is what INFO writes for the spool of the fixtures that carry
// a whole instance: a PLA with a uuid, a colour and a full weight.
func fullInstance(id connectorapi.ToolID) map[string]string {
	return merge(noInstance(id), map[string]string{
		tool(id, connectorapi.MaterialInstanceUUID): `"9c1d0e2f-1a2b-4c3d-8e5f-0000000000a0"`,
		tool(id, connectorapi.MaterialPackageUUID):  `"5f02a1b3-c4d5-46e7-88f9-0a1b2c3d4e5f"`,
		tool(id, connectorapi.MaterialUUID):         `"264f8a1b-2c3d-4e5f-9a0b-1c2d3e4f5a6b"`,
		tool(id, connectorapi.MaterialType):         `"PLA"`,
		tool(id, connectorapi.MaterialColor):        `"#1a1a1aff"`,
		tool(id, connectorapi.MaterialFullWeight):   "1000",
	})
}

// slot is what an INFO writes for one enabled feeder slot.
func slot(id connectorapi.ToolID, material string) map[string]string {
	return map[string]string{
		tool(id, connectorapi.ToolEnabled):          "true",
		tool(id, connectorapi.ToolType):             `"FFF_FEEDER_SLOT"`,
		tool(id, connectorapi.MaterialAbbreviation): material,
	}
}

// treeSlot is a slot of the tree-shaped INFO that reports its abbreviation
// alone: the rest of the instance is removed.
func treeSlot(id connectorapi.ToolID, material string) map[string]string {
	return merge(slot(id, material), noInstance(id))
}

// noFeederMembers is every member of a feeder past its slot count removed,
// as INFO writes it for a tool known to have no feeder.
func noFeederMembers(id connectorapi.ToolID) map[string]string {
	out := map[string]string{}
	members(out, id, "null", feederMembers...)

	return out
}

func mmuOnInfo() map[string]string {
	return merge(
		map[string]string{connectorapi.ToolCountPath: "1"},
		head("0", "0.4"),
		map[string]string{
			tool("0", connectorapi.FeederSlotCount): "5",
			tool("0", connectorapi.FeederModel):     `"MMU3"`,
			tool("0", connectorapi.FeederBaseModel): `"MMU3"`,
			tool("0", connectorapi.FeederType):      `"MMU"`,
		},
		treeSlot("0.0", `"PLA"`), fullInstance("0.0"), treeSlot("0.1", `"PETG"`), treeSlot("0.2", `"PLA"`),
		treeSlot("0.3", `""`), treeSlot("0.4", `"ASA"`),
	)
}

func mmuOffInfo() map[string]string {
	return merge(
		map[string]string{connectorapi.ToolCountPath: "1"},
		head("0", "0.4"),
		noFeederMembers("0"), fullInstance("0"),
		map[string]string{
			tool("0", connectorapi.FeederSlotCount):      "0",
			tool("0", connectorapi.MaterialAbbreviation): `"PLA"`,
		},
	)
}

// xlInfo is info-xl.json: heads 0 and 1 enabled, 2 to 4 null.
func xlInfo() map[string]string {
	out := merge(
		map[string]string{connectorapi.ToolCountPath: "5"},
		head("0", "0.4"), head("1", "0.6"), fullInstance("0"), noInstance("1"),
		map[string]string{
			tool("1", connectorapi.MaterialColor):        `"#ff8000"`,
			tool("0", connectorapi.MaterialAbbreviation): `"PLA"`,
			tool("1", connectorapi.MaterialAbbreviation): `"PLA"`,
		},
	)

	for _, id := range []connectorapi.ToolID{"2", "3", "4"} {
		disabledTool(out, id)
	}

	return out
}

// xlTelemetry is the readings of telemetry-xl.json: head 0 printing, head 1
// parked and cold, with its heater off.
func xlTelemetry() map[string]string {
	return map[string]string{
		tool("0", connectorapi.NozzleTemp):       "215.3",
		tool("0", connectorapi.NozzleTargetTemp): "215",
		tool("0", connectorapi.HeatbreakFanRPM):  "6500",
		tool("0", connectorapi.PrintFanRPM):      "3200",
		tool("0", connectorapi.ConsumedWeight):   "12",
		tool("0", connectorapi.OdometerID):       "3",
		tool("0", connectorapi.OdometerE):        "4120",
		tool("1", connectorapi.NozzleTemp):       "30.1",
		tool("1", connectorapi.NozzleTargetTemp): "0",
		tool("1", connectorapi.HeatbreakFanRPM):  "0",
		tool("1", connectorapi.PrintFanRPM):      "0",
		tool("1", connectorapi.ConsumedWeight):   "null",
		tool("1", connectorapi.OdometerID):       "0",
		tool("1", connectorapi.OdometerE):        "0",
	}
}

// xlActivity is what telemetry-xl.json says the first time: head 0 is
// picked and head 1 is not. The session reports a change only, so a frame
// that says the same again writes nothing of it.
func xlActivity() map[string]string {
	return map[string]string{
		tool("0", connectorapi.ToolActive): "true",
		tool("1", connectorapi.ToolActive): "false",
	}
}

// head0Telemetry is the readings of head 0 in the telemetry samples, which
// every one of them shares.
func head0Telemetry() map[string]string {
	return map[string]string{
		tool("0", connectorapi.NozzleTemp):       "215.3",
		tool("0", connectorapi.NozzleTargetTemp): "215",
		tool("0", connectorapi.HeatbreakFanRPM):  "6500",
		tool("0", connectorapi.PrintFanRPM):      "3200",
		tool("0", connectorapi.ConsumedWeight):   "12",
		tool("0", connectorapi.OdometerID):       "3",
		tool("0", connectorapi.OdometerE):        "4120",
	}
}

func readingsRemoved(id connectorapi.ToolID) map[string]string {
	out := map[string]string{}
	members(out, id, "null", treeTelemetryMembers...)

	return out
}

//nolint:maintidx // one table of scenarios, each a list of frames.
func TestToolTransitions(t *testing.T) {
	t.Parallel()

	nullTool := func(id string) func(map[string]json.RawMessage) {
		return func(tools map[string]json.RawMessage) { tools[id] = json.RawMessage(dialogAbsent) }
	}

	cases := []struct {
		name  string
		model string
		steps func(t *testing.T) []transitionStep
	}{
		{
			name:  "an MK4S switches its MMU off and on",
			model: "MK4S",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				mmuOnTelemetry := map[string]string{
					tool("0", connectorapi.ToolActive):       "true",
					tool("0.1", connectorapi.ToolActive):     "true",
					tool("0", connectorapi.NozzleTemp):       "215.3",
					tool("0", connectorapi.NozzleTargetTemp): "215",
					tool("0", connectorapi.HeatbreakFanRPM):  "6500",
					tool("0", connectorapi.PrintFanRPM):      "3200",
					tool("0.1", connectorapi.ConsumedWeight): "7",
					tool("0.1", connectorapi.OdometerID):     "5",
					tool("0.1", connectorapi.OdometerE):      "900",

					tool("0", connectorapi.FeederMMUProgressCode):      "4",
					tool("0", connectorapi.FeederMMUCommandInProgress): `"T"`,
				}
				for _, id := range []connectorapi.ToolID{"0.0", "0.2", "0.3", "0.4"} {
					mmuOnTelemetry[tool(id, connectorapi.ConsumedWeight)] = "null"
					mmuOnTelemetry[tool(id, connectorapi.ToolActive)] = "false"
					mmuOnTelemetry[tool(id, connectorapi.OdometerID)] = "0"
					mmuOnTelemetry[tool(id, connectorapi.OdometerE)] = "0"
				}

				// The slots leave the tree, and the head takes the spool. With
				// the loaded slot gone, the head is the tool printing now.
				off := merge(mmuOffInfo())
				for _, id := range []connectorapi.ToolID{"0.0", "0.1", "0.2", "0.3", "0.4"} {
					removedTool(off, id)
				}

				// The slots come back, and the head no longer holds a material
				// of its own.
				on := merge(
					mmuOnInfo(), noInstance("0"),
					map[string]string{tool("0", connectorapi.MaterialAbbreviation): "null"},
				)
				// The head never reported a profile temperature or a nominal
				// weight, so there is none to remove.
				for _, name := range connectorapi.ProfileTemperatures() {
					delete(on, tool("0", connectorapi.MaterialProperty(name)))
				}

				delete(on, tool("0", connectorapi.MaterialNominalFullWeight))

				return []transitionStep{
					{name: "MMU on", frame: info(t, fixtureFrame(t, "info-mmu-on.json", nil)), want: mmuOnInfo()},
					{
						name:  "MMU on telemetry",
						frame: telemetry(t, fixtureFrame(t, "telemetry-mmu-on.json", nil)), want: mmuOnTelemetry,
					},
					{name: "MMU off", frame: info(t, fixtureFrame(t, "info-mmu-off.json", nil)), want: off},
					{
						name:  "MMU off telemetry",
						frame: telemetry(t, fixtureFrame(t, "telemetry-mmu-off.json", nil)), want: head0Telemetry(),
					},
					{name: "MMU on again", frame: info(t, fixtureFrame(t, "info-mmu-on.json", nil)), want: on},
				}
			},
		},
		{
			name:  "an MK4S on firmware 7 switches its MMU off and on",
			model: "MK4S",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				on := []byte(`{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
					`,"2":` + numberedEntry("0.40", false, false, "PETG") +
					`,"3":` + numberedEntry("0.40", false, false, "---") +
					`},"mmu":{"enabled":true,"version":"3.0.3"},"slots":3}`)
				off := []byte(`{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
					`},"mmu":{"enabled":false,"version":"0.0.0"},"slots":1}`)

				// The one head in front of an MMU is always active. The first
				// INFO says so, and the later ones say the same again.
				onPatches := merge(
					map[string]string{connectorapi.ToolCountPath: "1"},
					head("0", "0.40"),
					map[string]string{
						tool("0", connectorapi.FeederSlotCount):      "3",
						tool("0", connectorapi.FeederType):           `"MMU"`,
						tool("0", connectorapi.MaterialAbbreviation): "null",
					},
					slot("0.0", `"PLA"`), slot("0.1", `"PETG"`), slot("0.2", `""`),
				)

				offPatches := merge(
					map[string]string{connectorapi.ToolCountPath: "1"},
					head("0", "0.40"),
					noFeederMembers("0"),
					map[string]string{
						tool("0", connectorapi.FeederSlotCount):      "0",
						tool("0", connectorapi.MaterialAbbreviation): `"PLA"`,
					},
				)
				for _, id := range []connectorapi.ToolID{"0.0", "0.1", "0.2"} {
					removedTool(offPatches, id)
				}

				firstOn := merge(onPatches, map[string]string{
					tool("0", connectorapi.ToolActive): "true",
				})

				return []transitionStep{
					{name: "MMU on", frame: info(t, on), want: firstOn},
					{name: "MMU off", frame: info(t, off), want: offPatches},
					{name: "MMU on again", frame: info(t, on), want: onPatches},
				}
			},
		},
		{
			name:  "an XL head goes null in INFO and in telemetry and comes back",
			model: "XL",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				headOff := merge(xlInfo())
				disabledTool(headOff, "1")

				return []transitionStep{
					{name: "two heads", frame: info(t, fixtureFrame(t, "info-xl.json", nil)), want: xlInfo()},
					{
						name:  "telemetry",
						frame: telemetry(t, fixtureFrame(t, "telemetry-xl.json", nil)),
						want:  merge(xlTelemetry(), xlActivity()),
					},
					{
						name:  "head 1 null",
						frame: info(t, fixtureFrame(t, "info-xl-head1-off.json", nil)),
						want:  headOff,
					},
					{
						// INFO has removed the readings already. Removing them
						// again changes nothing the host holds.
						name:  "head 1 null in telemetry",
						frame: telemetry(t, fixtureFrame(t, "telemetry-xl-head1-off.json", nil)),
						want:  merge(head0Telemetry(), readingsRemoved("1")),
					},
					{name: "head 1 back", frame: info(t, fixtureFrame(t, "info-xl.json", nil)), want: xlInfo()},
					{
						// INFO removed the active member of head 1, so the
						// session says it again.
						name:  "head 1 back in telemetry",
						frame: telemetry(t, fixtureFrame(t, "telemetry-xl.json", nil)),
						want:  merge(xlTelemetry(), map[string]string{tool("1", connectorapi.ToolActive): "false"}),
					},
				}
			},
		},
		{
			name:  "an XL head goes null in telemetry alone",
			model: "XL",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				return []transitionStep{
					{name: "two heads", frame: info(t, fixtureFrame(t, "info-xl.json", nil)), want: xlInfo()},
					{
						name:  "telemetry",
						frame: telemetry(t, fixtureFrame(t, "telemetry-xl.json", nil)),
						want:  merge(xlTelemetry(), xlActivity()),
					},
					{
						name:  "head 1 null in telemetry",
						frame: telemetry(t, fixtureFrame(t, "telemetry-xl-head1-off.json", nil)),
						want:  merge(head0Telemetry(), readingsRemoved("1")),
					},
					{
						name:  "head 1 still null",
						frame: telemetry(t, fixtureFrame(t, "telemetry-xl-head1-off.json", nil)),
						want:  head0Telemetry(),
					},
					{
						name:  "head 1 back in telemetry",
						frame: telemetry(t, fixtureFrame(t, "telemetry-xl.json", nil)),
						want:  xlTelemetry(),
					},
				}
			},
		},
		{
			name:  "two heads with a feeder each, and a slot loaded into the head not picked",
			model: "XL",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				frame := func(picked int) []byte {
					return []byte(`{"state":"PRINTING","tools":{` +
						`"0":{"active":` + strconv.FormatBool(picked == 0) + `},"0.0":{"active":false},"0.1":{"active":true},` +
						`"1":{"active":` + strconv.FormatBool(picked == 1) + `},"1.0":{"active":true},"1.1":{"active":false}}}`)
				}

				first := map[string]string{
					tool("0", connectorapi.ToolActive):   "true",
					tool("0.0", connectorapi.ToolActive): "false",
					tool("0.1", connectorapi.ToolActive): "true",
					tool("1", connectorapi.ToolActive):   "false",
					tool("1.0", connectorapi.ToolActive): "true",
					tool("1.1", connectorapi.ToolActive): "false",
				}

				switched := map[string]string{
					tool("0", connectorapi.ToolActive): "false",
					tool("1", connectorapi.ToolActive): "true",
				}

				return []transitionStep{
					{name: "head 0 picked", frame: telemetry(t, frame(0)), want: first},
					{name: "the same again", frame: telemetry(t, frame(0)), want: map[string]string{}},
					{name: "head 1 picked", frame: telemetry(t, frame(1)), want: switched},
				}
			},
		},
		{
			name:  "an XL on the numbered shape picks one head after another",
			model: "XL",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				infoFrame := []byte(`{"tools":{"1":` + numberedEntry("0.40", false, false, "PLA") +
					`,"2":` + numberedEntry("0.40", false, false, "PETG") + `}}`)
				slotBlock := func(active int) []byte {
					return []byte(`{"state":"PRINTING","slot":{"active":` + strconv.Itoa(active) +
						`,"1":{"material":"PLA"},"2":{"material":"PETG"}}}`)
				}

				infoPatches := merge(
					map[string]string{connectorapi.ToolCountPath: "5"},
					head("0", "0.40"), head("1", "0.40"), noFeederMembers("0"), noFeederMembers("1"),
					map[string]string{
						tool("0", connectorapi.FeederSlotCount):      "0",
						tool("1", connectorapi.FeederSlotCount):      "0",
						tool("0", connectorapi.MaterialAbbreviation): `"PLA"`,
						tool("1", connectorapi.MaterialAbbreviation): `"PETG"`,
					},
				)
				for _, id := range []connectorapi.ToolID{"2", "3", "4"} {
					disabledTool(infoPatches, id)
				}

				materials := map[string]string{
					tool("0", connectorapi.MaterialAbbreviation): `"PLA"`,
					tool("1", connectorapi.MaterialAbbreviation): `"PETG"`,
				}

				return []transitionStep{
					{name: "INFO", frame: info(t, infoFrame), want: infoPatches},
					{name: "head 1 picked", frame: telemetry(t, slotBlock(2)), want: merge(materials, map[string]string{
						tool("0", connectorapi.ToolActive): "false",
						tool("1", connectorapi.ToolActive): "true",
					})},
					{name: "the same again", frame: telemetry(t, slotBlock(2)), want: materials},
					{name: "none picked", frame: telemetry(t, slotBlock(0)), want: merge(materials, map[string]string{
						tool("1", connectorapi.ToolActive): "false",
					})},
				}
			},
		},
		{
			name:  "an XL goes into the error state with every head null and comes out",
			model: "XL",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				errored := map[string]string{connectorapi.ToolCountPath: "5"}
				for _, id := range []connectorapi.ToolID{"0", "1", "2", "3", "4"} {
					disabledTool(errored, id)
				}

				return []transitionStep{
					{name: "two heads", frame: info(t, fixtureFrame(t, "info-xl.json", nil)), want: xlInfo()},
					{name: "error", frame: info(t, fixtureFrame(t, "info-error.json", nil)), want: errored},
					{name: "out of the error", frame: info(t, fixtureFrame(t, "info-xl.json", nil)), want: xlInfo()},
				}
			},
		},
		{
			name:  "a slot of the MMU goes null and comes back",
			model: "MK4S",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				slotOff := mmuOnInfo()
				disabledTool(slotOff, "0.2")

				return []transitionStep{
					{name: "MMU on", frame: info(t, fixtureFrame(t, "info-mmu-on.json", nil)), want: mmuOnInfo()},
					{
						name:  "slot 0.2 null",
						frame: info(t, fixtureFrame(t, "info-mmu-on.json", nullTool("0.2"))), want: slotOff,
					},
					{
						name:  "slot 0.2 back",
						frame: info(t, fixtureFrame(t, "info-mmu-on.json", nil)),
						want:  mmuOnInfo(),
					},
				}
			},
		},
		{
			name:  "an XL head stops reporting a feature, its spool and a feature of its own",
			model: "XL",
			steps: func(t *testing.T) []transitionStep {
				t.Helper()

				extra := func(tools map[string]json.RawMessage) {
					var head0 map[string]any
					if err := json.Unmarshal(tools["0"], &head0); err != nil {
						t.Fatalf("head 0: %v", err)
					}

					features, _ := head0["features"].(map[string]any)
					features["nozzle_high_temp"] = true
					features["nozzle_kind"] = "brass"
					tools["0"] = mustJSON(t, head0)
				}

				withExtra := merge(xlInfo(), map[string]string{
					tool("0", connectorapi.FeatureNozzleHighTemp): "true",
					tool("0", "features/nozzle_kind"):             `"brass"`,
				})

				fewer := merge(xlInfo(), map[string]string{
					tool("0", connectorapi.FeatureNozzleHighFlow): "null",
					tool("0", connectorapi.FeatureNozzleHighTemp): "null",
					tool("0", "features/nozzle_kind"):             "null",
				})

				noSpool := merge(
					xlInfo(), noInstance("0"),
					map[string]string{tool("0", connectorapi.MaterialAbbreviation): `""`},
				)

				// The head stops naming its material at all: unknown, so the
				// path goes.
				noMaterialKey := func(tools map[string]json.RawMessage) {
					var head0 map[string]any
					if err := json.Unmarshal(tools["0"], &head0); err != nil {
						t.Fatalf("head 0: %v", err)
					}

					delete(head0, "material_package_instance")
					tools["0"] = mustJSON(t, head0)
				}

				// The spool was said gone by the step before, so only the
				// abbreviation changes.
				unknownMaterial := merge(
					xlInfo(),
					map[string]string{tool("0", connectorapi.MaterialAbbreviation): "null"},
				)
				for path := range noInstance("0") {
					delete(unknownMaterial, path)
				}

				for path := range fullInstance("0") {
					delete(unknownMaterial, path)
				}

				return []transitionStep{
					{name: "extra features", frame: info(t, fixtureFrame(t, "info-xl.json", extra)), want: withExtra},
					{
						name:  "fewer features",
						frame: info(t, fixtureFrame(t, "info-xl-head0-fewer-features.json", nil)), want: fewer,
					},
					{
						name:  "no spool",
						frame: info(t, fixtureFrame(t, "info-xl-head0-no-spool.json", nil)),
						want:  noSpool,
					},
					{
						name:  "material unsaid",
						frame: info(t, fixtureFrame(t, "info-xl.json", noMaterialKey)),
						want:  unknownMaterial,
					},
					{name: "everything back", frame: info(t, fixtureFrame(t, "info-xl.json", nil)), want: xlInfo()},
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				session := newSession(settings{
					identity: connectorapi.Identity{Vendor: Vendor, Model: tc.model, Serial: "SN-TRANSITION"},
					storage:  defaultStorage,
				})
				defer func() { _ = session.Close() }()

				for _, step := range tc.steps(t) {
					feed(session, step.frame)

					if got := toolPatches(session); !maps.Equal(got, step.want) {
						t.Errorf("%s:\n%s", step.name, patchDiff(got, step.want))
					}
				}
			})
		})
	}
}

// patchDiff names every path whose value differs, sorted.
func patchDiff(got, want map[string]string) string {
	var lines []string

	for path := range merge(got, want) {
		g, inGot := got[path]
		w, inWant := want[path]

		switch {
		case !inGot:
			lines = append(lines, "  missing "+path+" = "+w)
		case !inWant:
			lines = append(lines, "  extra   "+path+" = "+g)
		case g != w:
			lines = append(lines, "  wrong   "+path+" = "+g+", want "+w)
		}
	}

	slices.Sort(lines)

	return strings.Join(lines, "\n")
}
