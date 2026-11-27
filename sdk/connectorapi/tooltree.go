package connectorapi

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ToolTree is the tools of one machine, built from its state with ReadTools.
// Read the tools through it, not through the paths.
//
// Its JSON form is that of the printer configuration spec: a tools object
// keyed by id. A disabled tool, a missing feeder, an empty tool and a heater
// that is off are null. A member nobody reported is left out.
type ToolTree struct {
	// Count is the reported tool count. CountKnown is false when no driver
	// reported one.
	Count      int
	CountKnown bool
	// Active is the id of the tool printing, from ActiveToolOf. Empty when
	// none is selected or unknown. It is not in the JSON form.
	Active ToolID
	// Tools is every tool that has at least one member in the state, by id.
	Tools map[ToolID]*Tool
	// GCodeToolMap is the tool each G-code tool number selects, as the driver
	// reported it at GCodeToolMapPath. See Resolve.
	GCodeToolMap map[int]ToolID
}

// Tool is one tool of a ToolTree.
type Tool struct {
	ID ToolID
	// Disabled is a tool the machine has room for and has switched off. Every
	// other field of such a tool is empty.
	Disabled bool
	// Type is ToolTypeHead, ToolTypeFeederSlot or empty when unknown.
	Type string
	// Active is the tool's ToolActive member, and nil when not reported.
	// IsActive reads it.
	Active *bool
	// Features is every reported feature by its name, such as
	// nozzle_diameter, with the value as the driver encoded it.
	Features map[string]json.RawMessage
	// Feeder is nil when it is unknown whether a feeder is fitted. A feeder
	// with SlotCount 0 and SlotCountKnown true means the tool has none.
	Feeder *Feeder
	// Material is nil when unknown, and points at NoMaterial for a tool with
	// nothing loaded.
	Material *string
	// Reported is the rest of the material package instance the driver
	// reported. Nil when it reported none of it.
	Reported *ReportedMaterial
	// The readings of a head, nil when not reported. A NozzleTargetTemp of 0
	// is a heater that is off. See HeaterOff.
	NozzleTemp, NozzleTargetTemp, PrintFanRPM, HeatbreakFanRPM, ConsumedWeight *float64
	// The odometers of the tool, nil when not reported. OdometerID is a whole
	// number that changes at each reset.
	OdometerID                *int64
	OdometerE, OdometerWeight *float64
}

// ReportedMaterial is what a driver reported of the material package instance
// in a tool, besides its abbreviation. A field nobody reported is empty or
// nil.
type ReportedMaterial struct {
	// InstanceUUID, PackageUUID and MaterialUUID are the uuids of the
	// instance, its package and its material.
	InstanceUUID, PackageUUID, MaterialUUID string
	// Type is the kind of material, and ColorRGBA its primary colour, empty
	// unless it is ValidColor.
	Type, ColorRGBA string
	// FullWeight is the weight of the full spool in grams, and
	// NominalFullWeight the weight of a full package of it as sold.
	FullWeight, NominalFullWeight *float64
	// Properties is the reported profile temperatures by name, each one of
	// ProfileTemperatures.
	Properties map[string]float64
}

// Feeder is the feeder on one tool.
type Feeder struct {
	SlotCount int
	// SlotCountKnown says a driver reported the slot count. Without it the
	// feeder is unknown, and SlotCount 0 does not mean no feeder.
	SlotCountKnown bool
	// Model, BaseModel and Type name the feeder, and are empty when not
	// reported.
	Model, BaseModel, Type string
	// MMUProgressCode is the step the multi-material unit is at, nil when
	// not reported. MMUCommandInProgress is the command it runs, empty when
	// it runs none or it is unknown.
	MMUProgressCode      *int64
	MMUCommandInProgress string
}

// ReadTools builds the tool tree from one device's state, keyed by path. It
// ignores a path that is not a tool path and a value of the wrong type.
func ReadTools(state map[string]json.RawMessage) ToolTree {
	tree := ToolTree{Tools: map[ToolID]*Tool{}}

	if n, ok := wholeNumber(state[ToolCountPath]); ok {
		tree.Count, tree.CountKnown = n, true
	}

	for path, value := range state {
		if n, ok := SplitGCodeToolMapPath(path); ok {
			tree.GCodeToolMap = withMapping(tree.GCodeToolMap, n, value)

			continue
		}

		id, member, ok := SplitToolPath(path)
		if !ok {
			continue
		}

		tool := tree.Tools[id]
		if tool == nil {
			tool = &Tool{ID: id}
			tree.Tools[id] = tool
		}

		tool.read(member, value)
	}

	for _, tool := range tree.Tools {
		if tool.Disabled {
			*tool = Tool{ID: tool.ID, Disabled: true}
		}
	}

	tree.Active = tree.derivedActive()

	return tree
}

// derivedActive is ActiveToolOf the tools that report a ToolActive member,
// and empty when none does or none qualifies.
func (tree ToolTree) derivedActive() ToolID {
	var active map[ToolID]bool

	for id, tool := range tree.Tools {
		if tool.Active == nil {
			continue
		}

		if active == nil {
			active = map[ToolID]bool{}
		}

		active[id] = *tool.Active
	}

	id, _ := ActiveToolOf(active)

	return id
}

// withMapping adds the tool G-code tool number n selects to a mapping.
func withMapping(mapping map[int]ToolID, n int, value json.RawMessage) map[int]ToolID {
	id, isText := text(value)
	if !isText || !ToolID(id).Valid() {
		return mapping
	}

	if mapping == nil {
		mapping = map[int]ToolID{}
	}

	mapping[n] = ToolID(id)

	return mapping
}

// read takes one member of a tool.
func (t *Tool) read(member string, value json.RawMessage) {
	if name, isFeature := strings.CutPrefix(member, featuresPrefix); isFeature {
		if name != "" && !strings.Contains(name, "/") && len(value) > 0 {
			if t.Features == nil {
				t.Features = map[string]json.RawMessage{}
			}

			t.Features[name] = value
		}

		return
	}

	if strings.HasPrefix(member, feederPrefix) {
		t.readFeeder(member, value)

		return
	}

	if name, isProperty := strings.CutPrefix(member, materialPropsPrefix); isProperty {
		t.readProperty(name, value)

		return
	}

	switch member {
	case ToolEnabled:
		var enabled bool
		if json.Unmarshal(value, &enabled) == nil {
			t.Disabled = !enabled
		}
	case ToolType:
		t.Type, _ = text(value)
	case ToolActive:
		var active bool
		if json.Unmarshal(value, &active) == nil {
			t.Active = &active
		}
	case MaterialAbbreviation:
		if material, ok := text(value); ok {
			t.Material = &material
		}
	case MaterialInstanceUUID, MaterialPackageUUID, MaterialUUID, MaterialType, MaterialColor, MaterialFullWeight,
		MaterialNominalFullWeight:
		t.readReported(member, value)
	case OdometerID:
		if n, ok := wholeNumber(value); ok {
			id := int64(n)
			t.OdometerID = &id
		}
	default:
		if reading := t.reading(member); reading != nil {
			*reading = number(value)
		}
	}
}

// readReported takes one member of the reported material package instance.
func (t *Tool) readReported(member string, value json.RawMessage) {
	if member == MaterialFullWeight || member == MaterialNominalFullWeight {
		weight := number(value)

		switch {
		case weight == nil:
		case member == MaterialFullWeight:
			t.reported().FullWeight = weight
		default:
			t.reported().NominalFullWeight = weight
		}

		return
	}

	name, ok := text(value)
	if !ok || member == MaterialColor && !ValidColor(name) {
		return
	}

	reported := t.reported()

	switch member {
	case MaterialInstanceUUID:
		reported.InstanceUUID = name
	case MaterialPackageUUID:
		reported.PackageUUID = name
	case MaterialUUID:
		reported.MaterialUUID = name
	case MaterialType:
		reported.Type = name
	case MaterialColor:
		reported.ColorRGBA = name
	}
}

// readProperty takes one profile temperature, and ignores a name that is not
// one of ProfileTemperatures.
func (t *Tool) readProperty(name string, value json.RawMessage) {
	degrees := number(value)
	if degrees == nil || !slices.Contains(profileTemperatures, name) {
		return
	}

	reported := t.reported()
	if reported.Properties == nil {
		reported.Properties = map[string]float64{}
	}

	reported.Properties[name] = *degrees
}

func (t *Tool) reported() *ReportedMaterial {
	if t.Reported == nil {
		t.Reported = &ReportedMaterial{}
	}

	return t.Reported
}

// reading returns the field of one numeric reading, and nil for a member that
// is not one.
func (t *Tool) reading(member string) **float64 {
	switch member {
	case NozzleTemp:
		return &t.NozzleTemp
	case NozzleTargetTemp:
		return &t.NozzleTargetTemp
	case PrintFanRPM:
		return &t.PrintFanRPM
	case HeatbreakFanRPM:
		return &t.HeatbreakFanRPM
	case ConsumedWeight:
		return &t.ConsumedWeight
	case OdometerE:
		return &t.OdometerE
	case OdometerWeight:
		return &t.OdometerWeight
	}

	return nil
}

// readFeeder takes one member of the feeder on a tool.
func (t *Tool) readFeeder(member string, value json.RawMessage) {
	switch member {
	case FeederSlotCount:
		if n, ok := wholeNumber(value); ok {
			t.feeder().SlotCount, t.feeder().SlotCountKnown = n, true
		}
	case FeederModel, FeederBaseModel, FeederType, FeederMMUCommandInProgress:
		if name, ok := text(value); ok {
			t.feeder().name(member, name)
		}
	case FeederMMUProgressCode:
		if n, ok := wholeNumber(value); ok {
			code := int64(n)
			t.feeder().MMUProgressCode = &code
		}
	}
}

// name takes one text member of a feeder.
func (f *Feeder) name(member, value string) {
	switch member {
	case FeederModel:
		f.Model = value
	case FeederBaseModel:
		f.BaseModel = value
	case FeederType:
		f.Type = value
	case FeederMMUCommandInProgress:
		f.MMUCommandInProgress = value
	}
}

func (t *Tool) feeder() *Feeder {
	if t.Feeder == nil {
		t.Feeder = &Feeder{}
	}

	return t.Feeder
}

// HeaterOff reports whether the head says its heater is off: a
// NozzleTargetTemp of 0.
func (t *Tool) HeaterOff() bool {
	return t != nil && t.NozzleTargetTemp != nil && *t.NozzleTargetTemp == 0
}

// HasFeeder reports whether a feeder with at least one slot is fitted.
func (t *Tool) HasFeeder() bool { return t != nil && t.Feeder != nil && t.Feeder.SlotCount > 0 }

// Feature returns one feature of this tool as the driver encoded it. It
// reports false when the feature is not reported.
func (t *Tool) Feature(name string) (json.RawMessage, bool) {
	if t == nil {
		return nil, false
	}

	value, ok := t.Features[name]

	return value, ok
}

// Tool returns the tool with this id, and nil when the tree does not have it.
func (tree ToolTree) Tool(id ToolID) *Tool { return tree.Tools[id] }

// Heads returns the id of every head in the tree, in order. A head that only
// the tool count implies is not listed.
func (tree ToolTree) Heads() []ToolID {
	seen := map[ToolID]bool{}

	for id := range tree.Tools {
		if id.IsHead() {
			seen[id] = true
		}
	}

	return sortedIDs(seen)
}

// Slots returns the id of every slot of the feeder on this tool, in order,
// and nothing for a tool with no feeder. It ignores a slot past the feeder's
// slot count.
func (tree ToolTree) Slots(id ToolID) []ToolID {
	seen := map[ToolID]bool{}

	for slot := range tree.Tools {
		if tree.withinFeeder(slot, id) {
			seen[slot] = true
		}
	}

	return sortedIDs(seen)
}

// withinFeeder reports whether slot is a slot of the feeder on parent that
// the feeder's slot count covers.
func (tree ToolTree) withinFeeder(slot, parent ToolID) bool {
	above, isSlot := slot.Parent()
	if !isSlot || above != parent || !tree.Tools[parent].HasFeeder() {
		return false
	}

	n, err := strconv.Atoi(string(slot[len(parent)+1:]))

	return err == nil && n < tree.Tools[parent].Feeder.SlotCount
}

// Live reports whether the machine can print through the tool: the tree has
// it, it is not disabled, and each tool above it is live and has a feeder
// whose slot count covers it.
func (tree ToolTree) Live(id ToolID) bool {
	tool := tree.Tools[id]
	if tool == nil || tool.Disabled {
		return false
	}

	parent, isSlot := id.Parent()
	if !isSlot {
		return true
	}

	return tree.withinFeeder(id, parent) && tree.Live(parent)
}

// Head returns the head a tool belongs to, and nil when the tree does not
// have it.
func (tree ToolTree) Head(id ToolID) *Tool { return tree.Tools[id.Head()] }

// Nozzle returns one feature of the nozzle a tool prints through: the value
// from the tool itself or from the nearest tool above it that reports the
// feature.
func (tree ToolTree) Nozzle(id ToolID, name string) (json.RawMessage, bool) {
	for {
		if value, ok := tree.Tools[id].Feature(name); ok {
			return value, true
		}

		parent, ok := id.Parent()
		if !ok {
			return nil, false
		}

		id = parent
	}
}

// Resolve returns the tool that G-code tool number n selects. An entry in
// GCodeToolMap wins. Without one, Tn is slot n of the head on a machine with
// one head that has a feeder, and head n on any other machine. The result may
// name a tool the machine does not have. Check it with Live.
func (tree ToolTree) Resolve(n int) ToolID {
	if id, reported := tree.GCodeToolMap[n]; reported {
		return id
	}

	heads := tree.Heads()
	single := len(heads) == 1 && (!tree.CountKnown || tree.Count <= 1)

	if single && tree.Tools[heads[0]].HasFeeder() {
		return heads[0].Slot(n)
	}

	return HeadID(n)
}

// IsActive reports whether the tool is selected. It is the tool's own
// ToolActive member when the driver wrote one. Otherwise a tool is active
// when it is the active tool or the active tool is one of its slots.
func (tree ToolTree) IsActive(id ToolID) bool {
	if tool := tree.Tools[id]; tool != nil && tool.Active != nil {
		return *tool.Active
	}

	if tree.Active == "" {
		return false
	}

	return tree.Active == id || strings.HasPrefix(string(tree.Active), string(id)+".")
}

// MarshalJSON writes the tree in the form of the printer configuration spec.
func (tree ToolTree) MarshalJSON() ([]byte, error) {
	tools := make(map[string]any, len(tree.Tools))

	for id, tool := range tree.Tools {
		if tool.Disabled {
			tools[string(id)] = nil

			continue
		}

		tools[string(id)] = tree.image(tool)
	}

	out := map[string]any{"tools": tools}

	if tree.CountKnown {
		out["tool_count"] = tree.Count
	}

	if len(tree.GCodeToolMap) > 0 {
		mapping := make(map[string]ToolID, len(tree.GCodeToolMap))
		for n, id := range tree.GCodeToolMap {
			mapping[strconv.Itoa(n)] = id
		}

		out["gcode_tool_map"] = mapping
	}

	return json.Marshal(out)
}

// image is one enabled tool in the JSON form.
func (tree ToolTree) image(tool *Tool) map[string]any {
	entry := map[string]any{"active": tree.IsActive(tool.ID)}

	if tool.Type != "" {
		entry["type"] = tool.Type
	}

	if len(tool.Features) > 0 {
		entry["features"] = maps.Clone(tool.Features)
	}

	switch {
	case tool.Feeder == nil || !tool.Feeder.SlotCountKnown:
	case tool.Feeder.SlotCount == 0:
		entry["feeder"] = nil
	default:
		entry["feeder"] = feederImage(tool.Feeder)
	}

	switch {
	case tool.HasFeeder() || (tool.Material == nil && tool.Reported == nil):
	case tool.Material != nil && *tool.Material == NoMaterial:
		entry["material_package_instance"] = nil
	default:
		entry["material_package_instance"] = instanceImage(tool)
	}

	for name, value := range map[string]*float64{
		NozzleTemp: tool.NozzleTemp, NozzleTargetTemp: tool.NozzleTargetTemp,
		PrintFanRPM: tool.PrintFanRPM, HeatbreakFanRPM: tool.HeatbreakFanRPM,
		ConsumedWeight: tool.ConsumedWeight,
	} {
		if value != nil {
			entry[name] = *value
		}
	}

	if tool.HeaterOff() {
		entry[NozzleTargetTemp] = nil
	}

	for name, value := range map[string]*float64{OdometerE: tool.OdometerE, OdometerWeight: tool.OdometerWeight} {
		if value != nil {
			entry[name] = *value
		}
	}

	if tool.OdometerID != nil {
		entry[OdometerID] = *tool.OdometerID
	}

	return entry
}

// instanceImage is the material package instance of a tool in the JSON form:
// the abbreviation and what the driver reported.
func instanceImage(tool *Tool) map[string]any {
	material := map[string]any{}
	pack := map[string]any{"material": material}
	instance := map[string]any{"package": pack}

	if tool.Material != nil {
		material["abbreviation"] = *tool.Material
	}

	if r := tool.Reported; r != nil {
		putText(instance, "uuid", r.InstanceUUID)
		putText(pack, "uuid", r.PackageUUID)
		putText(material, "uuid", r.MaterialUUID)
		putText(material, "type", r.Type)

		if r.ColorRGBA != "" {
			material["primary_color"] = map[string]any{"color_rgba": r.ColorRGBA}
		}

		if len(r.Properties) > 0 {
			material["properties"] = maps.Clone(r.Properties)
		}

		if r.FullWeight != nil {
			instance["actual_netto_full_weight"] = *r.FullWeight
		}

		if r.NominalFullWeight != nil {
			pack["nominal_netto_full_weight"] = *r.NominalFullWeight
		}
	}

	return instance
}

// putText sets a member of an image when the text is not empty.
func putText(image map[string]any, name, value string) {
	if value != "" {
		image[name] = value
	}
}

// feederImage is a fitted feeder in the JSON form: its configuration and its
// telemetry in one object.
func feederImage(feeder *Feeder) map[string]any {
	out := map[string]any{"slot_count": feeder.SlotCount}

	for name, value := range map[string]string{
		"model": feeder.Model, "base_model": feeder.BaseModel, "type": feeder.Type,
		"mmu_command_in_progress": feeder.MMUCommandInProgress,
	} {
		if value != "" {
			out[name] = value
		}
	}

	if feeder.MMUProgressCode != nil {
		out["mmu_progress_code"] = *feeder.MMUProgressCode
	}

	return out
}

func sortedIDs(set map[ToolID]bool) []ToolID {
	out := slices.Collect(maps.Keys(set))
	slices.SortFunc(out, ToolID.Compare)

	return out
}

func text(raw json.RawMessage) (string, bool) {
	var out string
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return "", false
	}

	return out, true
}

func number(raw json.RawMessage) *float64 {
	var out float64
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return nil
	}

	return &out
}

func wholeNumber(raw json.RawMessage) (int, bool) {
	value := number(raw)
	if value == nil || *value < 0 || *value != float64(int(*value)) {
		return 0, false
	}

	return int(*value), true
}
