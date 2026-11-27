package connectorapi

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// The state paths where a driver reports the tools of its machine. The tools
// form a tree. Most names are those of the public printer configuration spec
// (https://specs.prusa3d.tech/architecture/#/printer_configuration):
//
//   - a head is a top-level tool with the id "0", "1" and so on. A machine
//     with one nozzle has head "0"
//   - a feeder sits on a tool. Slot k of the feeder on tool "0" has the id
//     "0.k". A slot may have a feeder of its own, so "3.1.0" is a legal id
//
// The driver maps what its machine sends onto these paths. A reader never
// branches on a vendor, a model or a firmware. A path left out is unknown. A
// reader never takes it as zero, false or "nothing loaded".

// The paths that describe the tools as a whole.
const (
	// ToolCountPath is how many heads the machine has room for, enabled or
	// not, as a number. The heads are "0" to that number minus one.
	ToolCountPath = "/tool_count"
)

// toolsRoot is the prefix of every tool path.
const toolsRoot = "/tools/"

// gcodeToolMapRoot is the prefix of every GCodeToolMapPath.
const gcodeToolMapRoot = "/gcode_tool_map/"

// The kinds of tool, with the values of the spec.
const (
	// ToolTypeHead is a physical print head, with a hotend and an extruder.
	ToolTypeHead = "FFF_PRINT_HEAD"
	// ToolTypeFeederSlot is one input slot of a feeder.
	ToolTypeFeederSlot = "FFF_FEEDER_SLOT"
)

// The members of one tool. The path of a member is ToolPath of the tool's id
// and the member.
const (
	// ToolEnabled says whether the tool is enabled, as a boolean. Write true
	// for every tool, and false for a tool the machine has switched off.
	// Remove the other members of a disabled tool.
	ToolEnabled = "enabled"
	// ToolType is the kind of tool, ToolTypeHead or ToolTypeFeederSlot, as
	// text.
	ToolType = "type"
	// ToolActive says whether the tool is selected, as a boolean. The one
	// head of a machine with no toolchanger is always active. A slot is active
	// while it is loaded into its head, so several tools can be active at once.
	ToolActive = "active"

	// FeatureNozzleDiameter is the nozzle diameter in millimetres, as a
	// number.
	FeatureNozzleDiameter = featuresPrefix + "nozzle_diameter"
	// FeatureNozzleHighFlow says whether the nozzle is a high-flow one, as a
	// boolean.
	FeatureNozzleHighFlow = featuresPrefix + "nozzle_high_flow"
	// FeatureNozzleHardened says whether the nozzle is hardened against
	// abrasive filament, as a boolean.
	FeatureNozzleHardened = featuresPrefix + "nozzle_hardened"
	// FeatureNozzleHighTemp says whether the nozzle is made for high
	// temperatures, as a boolean.
	FeatureNozzleHighTemp = featuresPrefix + "nozzle_high_temp"

	// FeederSlotCount is how many slots the feeder on this tool has, enabled
	// or not, as a number. Its slots are the child ids 0 to FeederSlotCount-1.
	// Zero means the tool has no feeder.
	FeederSlotCount = feederPrefix + "slot_count"
	// FeederModel is the feeder's model, as text.
	FeederModel = feederPrefix + "model"
	// FeederBaseModel is the family of the feeder's model, as text.
	FeederBaseModel = feederPrefix + "base_model"
	// FeederType is the kind of feeder, as text. The spec allows only MMU.
	FeederType = feederPrefix + "type"
	// FeederMMUProgressCode is the progress code the multi-material unit
	// reports, as a whole number: the step of its work, 0 when it is done. It
	// is on the tool that has the feeder.
	FeederMMUProgressCode = feederPrefix + "mmu_progress_code"
	// FeederMMUCommandInProgress is the command the multi-material unit is
	// running, as text of one character, such as L for a load. Remove it
	// while the unit runs none.
	FeederMMUCommandInProgress = feederPrefix + "mmu_command_in_progress"

	// MaterialAbbreviation is the short name of the material loaded in the
	// tool, such as PLA, as text. NoMaterial means nothing is loaded. A tool
	// with a feeder has no material. Its slots do.
	MaterialAbbreviation = "material_package_instance/package/material/abbreviation"
	// MaterialInstanceUUID is the uuid of the material package instance in
	// the tool, as text: the OpenPrintTag instance_uuid of a spool with an
	// NFC tag. A machine that reads no tag does not report it.
	MaterialInstanceUUID = "material_package_instance/uuid"
	// MaterialPackageUUID is the uuid of the material package of that
	// instance, as text.
	MaterialPackageUUID = "material_package_instance/package/uuid"
	// MaterialUUID is the uuid of the material of that package, as text.
	MaterialUUID = "material_package_instance/package/material/uuid"
	// MaterialType is the kind of material, such as PLA, as text.
	MaterialType = "material_package_instance/package/material/type"
	// MaterialColor is the primary colour of the material, as text: the
	// OpenPrintTag color_rgba, #rrggbbaa, or #rrggbb with no alpha. A driver
	// must publish no other text. See ValidColor.
	MaterialColor = "material_package_instance/package/material/primary_color/color_rgba"
	// MaterialFullWeight is the weight of the material when the spool was
	// full, in grams, as a number: the OpenPrintTag actual_netto_full_weight.
	MaterialFullWeight = "material_package_instance/actual_netto_full_weight"
	// MaterialNominalFullWeight is the weight of the material in a full
	// package as sold, in grams, as a number: the OpenPrintTag
	// nominal_netto_full_weight. A tag that carries no actual weight
	// often carries this one.
	MaterialNominalFullWeight = "material_package_instance/package/nominal_netto_full_weight"

	// NozzleTemp is the nozzle temperature of a head in degrees Celsius, as a
	// number. A machine with one nozzle reports it under head "0".
	NozzleTemp = "nozzle_temp"
	// NozzleTargetTemp is the temperature a head is heating to, in degrees
	// Celsius, as a number. Write 0 while the heater is off. Remove it only
	// when the target is unknown.
	NozzleTargetTemp = "nozzle_target_temp"
	// PrintFanRPM and HeatbreakFanRPM are the speeds of a head's print fan
	// and heatbreak fan, in revolutions per minute, as numbers.
	PrintFanRPM     = "print_fan_rpm"
	HeatbreakFanRPM = "heatbreak_fan_rpm"
	// ConsumedWeight is how much of the material loaded in the tool is used,
	// in grams, as a number: the OpenPrintTag consumed_weight.
	ConsumedWeight = "consumed_weight"
	// OdometerID is a whole number that changes each time the tool's
	// odometers are reset. It does not count up. A reset can happen at any
	// time, also during a print. A reader that sees it change starts again.
	OdometerID = "odometer_id"
	// OdometerE is how much filament the tool has extruded since its
	// odometers were last reset, in millimetres, as a number.
	OdometerE = "odometer_e"
	// OdometerWeight is the weight of that filament in grams, as a number. A
	// machine that cannot turn length into weight leaves it out.
	OdometerWeight = "odometer_weight"
)

const (
	featuresPrefix = "features/"
	feederPrefix   = "feeder/"

	materialPropsPrefix = "material_package_instance/package/material/properties/"
)

// MaterialProperty is the member of one profile temperature of the material
// loaded in the tool, such as min_print_temperature, as a number in degrees
// Celsius. ProfileTemperatures lists the names a driver can report.
func MaterialProperty(name string) string { return materialPropsPrefix + name }

// ProfileTemperatures returns the names of the temperatures a printer reports
// from its material profile. Each call returns a new slice.
func ProfileTemperatures() []string { return slices.Clone(profileTemperatures) }

var profileTemperatures = []string{
	"chamber_temperature",
	"heatbreak_temperature",
	"max_bed_temperature",
	"max_chamber_temperature",
	"max_print_temperature",
	"min_bed_temperature",
	"min_chamber_temperature",
	"min_print_temperature",
	"preheat_temperature",
}

// NoMaterial is the value of MaterialAbbreviation for a tool with no filament
// loaded. A path that was never written means unknown. A driver maps its
// machine's own word for an empty tool, such as "---", to it.
const NoMaterial = ""

// ToolMembers returns every member of a tool this contract names. A driver
// that removes a tool from the tree removes each of them.
func ToolMembers() []string {
	members := []string{
		ToolEnabled, ToolType, ToolActive,
		FeatureNozzleDiameter, FeatureNozzleHighFlow, FeatureNozzleHardened, FeatureNozzleHighTemp,
		FeederSlotCount, FeederModel, FeederBaseModel, FeederType,
		FeederMMUProgressCode, FeederMMUCommandInProgress,
		MaterialAbbreviation, MaterialInstanceUUID, MaterialPackageUUID, MaterialUUID,
		MaterialType, MaterialColor, MaterialFullWeight, MaterialNominalFullWeight,
		NozzleTemp, NozzleTargetTemp, PrintFanRPM, HeatbreakFanRPM, ConsumedWeight,
		OdometerID, OdometerE, OdometerWeight,
	}

	for _, name := range profileTemperatures {
		members = append(members, MaterialProperty(name))
	}

	return members
}

// ToolID is the id of one tool: whole numbers counted from 0, joined by dots.
// "0" is the first head, "0.4" the fifth slot of the feeder on it.
type ToolID string

// HeadID is the id of the head counted n from 0.
func HeadID(n int) ToolID { return ToolID(strconv.Itoa(n)) }

// Slot is the id of slot n of the feeder on this tool.
func (id ToolID) Slot(n int) ToolID { return id + "." + ToolID(strconv.Itoa(n)) }

// Valid reports whether the id is one or more whole numbers with no sign and
// no leading zero, joined by single dots. It limits neither the numbers nor
// the depth.
func (id ToolID) Valid() bool {
	if id == "" {
		return false
	}

	for part := range strings.SplitSeq(string(id), ".") {
		if !canonicalNumber(part) {
			return false
		}
	}

	return true
}

// Parent is the tool this one is a slot of, and false for a head.
func (id ToolID) Parent() (ToolID, bool) {
	at := strings.LastIndexByte(string(id), '.')
	if at < 0 {
		return "", false
	}

	return id[:at], true
}

// Head is the head this tool belongs to: the first part of its id.
func (id ToolID) Head() ToolID {
	head, _, _ := strings.Cut(string(id), ".")

	return ToolID(head)
}

// IsHead reports whether the id is a top-level tool.
func (id ToolID) IsHead() bool { return !strings.Contains(string(id), ".") }

// Compare orders two ids part by part, as numbers: "2" before "10", and a
// head before its slots.
func (id ToolID) Compare(other ToolID) int {
	left, right := strings.Split(string(id), "."), strings.Split(string(other), ".")

	for i := range min(len(left), len(right)) {
		if c := compareNumbers(left[i], right[i]); c != 0 {
			return c
		}
	}

	return len(left) - len(right)
}

// ToolPath is the state path of one member of one tool, such as
// /tools/0/features/nozzle_diameter for the first head.
func ToolPath(id ToolID, member string) string {
	return toolsRoot + string(id) + "/" + member
}

// SplitToolPath returns the tool id and the member of a path under /tools/.
// It reports false for any other path and for an invalid id:
// /tools/00/enabled is not head 0.
func SplitToolPath(path string) (ToolID, string, bool) {
	rest, found := strings.CutPrefix(path, toolsRoot)
	if !found {
		return "", "", false
	}

	id, member, found := strings.Cut(rest, "/")
	if !found || member == "" || !ToolID(id).Valid() {
		return "", "", false
	}

	return ToolID(id), member, true
}

// GCodeToolMapPath is the state path where a driver reports the tool that
// G-code tool number n selects, as a tool id in text. n is the number of a
// T<n> line. Without it a reader uses the default rule of ToolTree.Resolve.
func GCodeToolMapPath(n int) string { return gcodeToolMapRoot + strconv.Itoa(n) }

// SplitGCodeToolMapPath returns the G-code tool number of a
// GCodeToolMapPath.
func SplitGCodeToolMapPath(path string) (int, bool) {
	rest, found := strings.CutPrefix(path, gcodeToolMapRoot)
	if !found || !canonicalNumber(rest) {
		return 0, false
	}

	n, err := strconv.Atoi(rest)

	return n, err == nil
}

// ActiveToolOf returns the tool the machine prints with, from the ToolActive
// member of each tool: the deepest active tool whose parents are all active.
// Between two of the same depth the lower id wins. It reports false when no
// tool qualifies.
func ActiveToolOf(active map[ToolID]bool) (ToolID, bool) {
	var best ToolID

	for id, on := range active {
		if !on || !id.Valid() || !activeAbove(active, id) {
			continue
		}

		if best == "" || deeperTool(id, best) {
			best = id
		}
	}

	return best, best != ""
}

// activeAbove reports whether every tool above id is active.
func activeAbove(active map[ToolID]bool, id ToolID) bool {
	for parent, ok := id.Parent(); ok; parent, ok = parent.Parent() {
		if !active[parent] {
			return false
		}
	}

	return true
}

// deeperTool reports whether a wins over b: the deeper id, and the lower one
// between two of the same depth.
func deeperTool(a, b ToolID) bool {
	da, db := strings.Count(string(a), "."), strings.Count(string(b), ".")
	if da != db {
		return da > db
	}

	return a.Compare(b) < 0
}

// canonicalNumber reports whether text is a whole number with no sign and no
// leading zero.
func canonicalNumber(text string) bool {
	if text == "" || (len(text) > 1 && text[0] == '0') {
		return false
	}

	for i := range len(text) {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}

	return true
}

// compareNumbers orders two canonical numbers of any length.
func compareNumbers(a, b string) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}

	return strings.Compare(a, b)
}

// ValidColor reports whether text is a MaterialColor: #rrggbb or #rrggbbaa in
// hex digits of either case. A page puts the colour into its CSS, so a driver
// must publish no other text.
func ValidColor(text string) bool {
	if len(text) != len("#rrggbb") && len(text) != len("#rrggbbaa") || text[0] != '#' {
		return false
	}

	return !strings.ContainsFunc(text[1:], func(r rune) bool {
		return !unicode.Is(unicode.ASCII_Hex_Digit, r)
	})
}
