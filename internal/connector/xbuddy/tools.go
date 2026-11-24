package xbuddy

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// What the firmware says about the tools, normalized onto the tool tree in
// sdk/connectorapi/tools.go (ADR-20261023-gna). The firmware speaks one of two shapes,
// and this file reads both.
//
// # The numbered shape
//
// The shipping firmware writes it in render_msg (src/connect/render.cpp, the
// EventType::Info arm):
//
//	"tools": {"1": {"nozzle_diameter": 0.40, "high_flow": false,
//	                "hardened": false, "material": "PLA"}, ...},
//	"mmu": {"enabled": true, "version": "3.0.3"},
//	"slots": 5
//
// It numbers from 1 every filament path the firmware can select. Which kind of
// path that is depends on the machine. On an XL or an INDX each entry is a
// head; on a machine with an MMU each entry is a slot of the MMU in front of
// the one head. A disabled path has no entry. The telemetry slot block uses
// the same numbers. This file turns them into the tree:
//
//   - with a fitted and enabled MMU, entry n is slot "0.<n-1>" of head "0",
//     and the nozzle of the first entry is the head's
//   - otherwise entry n is head "<n-1>", and every head the model has room for
//     that INFO does not list is disabled
//   - "---" is connectorapi.NoMaterial
//
// # The tree shape
//
// Firmware that speaks the public architecture (the printer configuration at
// https://specs.prusa3d.tech/architecture/#/printer_configuration), as
// firmware 8.0 does, sends tool_count and tools keyed "0", "0.0" with null
// for a disabled tool, and telemetry with a tools object in place of the slot
// block. Its ids and names are the tree's own, so it is read as it is.
// treeShaped decides which shape a frame has, by its shape alone: the two
// shapes cannot be mistaken for one another, and a firmware version would
// say nothing a frame does not.

// noMaterial is the firmware's word for a tool with no filament loaded.
const noMaterial = "---"

// The ways the numbered shape sits on the tree.
const (
	layoutUnknown int32 = iota
	layoutHeads
	layoutFeeder
)

// headCounts is how many heads a model has room for, where it is more than
// one: XL is five (EXTRUDERS in include/marlin/Configuration_XL.h) and an INDX
// is eight (the HAS_INDX() branch of Configuration_COREONE.h). Every other
// model this driver knows has one.
//
//nolint:gochecknoglobals,mnd // a fixed table, transcribed and read-only.
var headCounts = map[string]int{"XL": 5, "CoreOne_INDX": 8, "CoreOneL_INDX": 8}

// headsOf is the number of heads of a model, matched by sameModel, and false
// for a model this driver has no figures for.
func headsOf(model string) (int, bool) {
	for name, heads := range headCounts {
		if sameModel(name, model) {
			return heads, true
		}
	}

	if _, known := buildVolumeOf(model); known {
		return 1, true
	}

	return 0, false
}

// numberedTool is one entry of the numbered INFO tools block. The values stay
// encoded, because a driver that re-encodes a number changes it.
type numberedTool struct {
	NozzleDiameter json.RawMessage `json:"nozzle_diameter"`
	HighFlow       json.RawMessage `json:"high_flow"`
	Hardened       json.RawMessage `json:"hardened"`
	Material       json.RawMessage `json:"material"`
}

// numberedInfo is the part of the numbered INFO event about the tools.
type numberedInfo struct {
	Tools          map[string]numberedTool `json:"tools"`
	NozzleDiameter json.RawMessage         `json:"nozzle_diameter"`
	MMU            *struct {
		Enabled json.RawMessage `json:"enabled"`
	} `json:"mmu"`
}

// toolReport is what one INFO says about the tools.
type toolReport struct {
	patches []connectorapi.StatePatch
	// heads is every head the report wrote, enabled or disabled.
	heads []connectorapi.ToolID
	// layout is how the numbered telemetry slot block sits on the tree.
	layout int32
	// sole is the one enabled head when there is exactly one, which is where
	// the material at the top of a telemetry frame belongs. It is empty
	// otherwise.
	sole connectorapi.ToolID
	// base is where the firmware starts counting tools in a setter key, and
	// toolBaseUnknown for a payload that is not an INFO object.
	base toolBase
}

// writer collects patches in the order they are written.
type writer struct{ patches []connectorapi.StatePatch }

func (w *writer) set(path string, value json.RawMessage) {
	if carriesValue(value) {
		w.patches = append(w.patches, connectorapi.StatePatch{Path: path, Value: value})
	}
}

func (w *writer) member(id connectorapi.ToolID, member string, value json.RawMessage) {
	w.set(connectorapi.ToolPath(id, member), value)
}

func (w *writer) remove(path string) {
	w.patches = append(w.patches, connectorapi.StatePatch{Path: path, Value: json.RawMessage(dialogAbsent)})
}

// enable says a tool is there and what kind it is.
func (w *writer) enable(id connectorapi.ToolID, kind string) {
	w.member(id, connectorapi.ToolEnabled, json.RawMessage(`true`))
	w.member(id, connectorapi.ToolType, encodeText(kind))
}

// disable writes the architecture's null tool: enabled false, and nothing
// else.
func (w *writer) disable(id connectorapi.ToolID) {
	w.member(id, connectorapi.ToolEnabled, json.RawMessage(`false`))

	for _, member := range connectorapi.ToolMembers() {
		if member != connectorapi.ToolEnabled {
			w.remove(connectorapi.ToolPath(id, member))
		}
	}
}

// infoTools reads the tools out of one INFO event of either shape. A payload
// that is not an object reads as nothing.
func infoTools(data json.RawMessage, model string) toolReport {
	if len(data) == 0 {
		return toolReport{}
	}

	if treeShaped(data) {
		report := treeInfoTools(data)
		report.base = toolBaseZero

		return report
	}

	var info numberedInfo
	if json.Unmarshal(data, &info) != nil {
		return toolReport{}
	}

	report := numberedInfoTools(info, model)
	report.base = toolBaseOne

	return report
}

// treeShaped reports whether an INFO payload carries its tools in the tree
// shape. The numbered shape has no tool_count, keys its tools from "1", and
// never writes a null tool or a tool with features or a type. Anything else
// in the tools block is the tree shape.
func treeShaped(data json.RawMessage) bool {
	var probe struct {
		ToolCount json.RawMessage            `json:"tool_count"`
		Tools     map[string]json.RawMessage `json:"tools"`
	}

	if json.Unmarshal(data, &probe) != nil {
		return false
	}

	if len(probe.ToolCount) > 0 {
		return true
	}

	for key, raw := range probe.Tools {
		if _, numbered := wireNumber(key); !numbered || string(raw) == dialogAbsent {
			return true
		}

		var entry map[string]json.RawMessage
		if json.Unmarshal(raw, &entry) != nil {
			continue
		}

		if _, has := entry["features"]; has {
			return true
		}

		if _, has := entry["type"]; has {
			return true
		}
	}

	return false
}

// numberedInfoTools maps the numbered shape onto the tree.
func numberedInfoTools(info numberedInfo, model string) toolReport {
	var w writer

	// Firmware older than the tools block (6.4.0 sends it) names the nozzle of
	// the first tool and nothing else.
	if info.Tools == nil {
		if !carriesValue(info.NozzleDiameter) {
			return toolReport{}
		}

		w.member("0", connectorapi.FeatureNozzleDiameter, info.NozzleDiameter)

		return toolReport{patches: sortedPatches(w.patches), layout: layoutUnknown}
	}

	listed := map[int]numberedTool{}
	highest := 0

	for key, tool := range info.Tools {
		if n, ok := wireNumber(key); ok {
			listed[n] = tool
			highest = max(highest, n)
		}
	}

	report := toolReport{layout: toolLayoutFor(info, model, len(listed))}

	if report.layout == layoutFeeder {
		numberedFeeder(&w, listed, highest)
		report.heads = []connectorapi.ToolID{"0"}
		report.sole = "0"
	} else {
		heads, _ := headsOf(model)
		count := max(heads, highest)
		w.set(connectorapi.ToolCountPath, json.RawMessage(strconv.Itoa(count)))

		for n := 1; n <= count; n++ {
			id := connectorapi.HeadID(n - 1)
			report.heads = append(report.heads, id)

			tool, isListed := listed[n]
			if !isListed {
				w.disable(id)

				continue
			}

			w.enable(id, connectorapi.ToolTypeHead)
			nozzle(&w, id, tool)
			// The numbered shape says everything there is about a feeder:
			// a head has none unless the MMU block says it is enabled.
			noFeeder(&w, id)
			material(&w, id, tool.Material)
		}
	}

	if report.layout == layoutHeads && len(listed) == 1 {
		report.sole = connectorapi.HeadID(highest - 1)
	}

	report.patches = sortedPatches(w.patches)

	return report
}

// numberedFeeder writes one head with an MMU in front of it. Every numbered
// entry is a slot of the MMU, and all of them print through the one nozzle,
// so the head takes the nozzle of the first entry. A head with a feeder holds
// no material of its own: its slots do. The slot count is the highest entry,
// and a slot below it with no entry is disabled.
func numberedFeeder(w *writer, listed map[int]numberedTool, highest int) {
	w.set(connectorapi.ToolCountPath, json.RawMessage(`1`))
	w.enable("0", connectorapi.ToolTypeHead)
	nozzle(w, "0", listed[slices.Min(mapKeys(listed))])
	w.member("0", connectorapi.FeederSlotCount, json.RawMessage(strconv.Itoa(highest)))
	// The block is the firmware's MMU block, and the architecture's only kind
	// of feeder is an MMU. The block names no model.
	w.member("0", connectorapi.FeederType, encodeText(mmuFeederType))
	w.remove(connectorapi.ToolPath("0", connectorapi.MaterialAbbreviation))

	for n := 1; n <= highest; n++ {
		slot := connectorapi.ToolID("0").Slot(n - 1)

		tool, isListed := listed[n]
		if !isListed {
			w.disable(slot)

			continue
		}

		w.enable(slot, connectorapi.ToolTypeFeederSlot)
		material(w, slot, tool.Material)
	}
}

// toolLayoutFor decides what the numbered entries are. The MMU block decides
// when the firmware sends one. Without it, several entries on a model with one
// head can only be slots, and anything else is heads.
func toolLayoutFor(info numberedInfo, model string, listed int) int32 {
	if info.MMU != nil {
		var enabled bool
		if json.Unmarshal(info.MMU.Enabled, &enabled) == nil && enabled && listed > 0 {
			return layoutFeeder
		}

		return layoutHeads
	}

	if heads, known := headsOf(model); known && heads == 1 && listed > 1 {
		return layoutFeeder
	}

	return layoutHeads
}

// nozzle writes the features of one numbered entry under the architecture's
// names. The firmware says high_flow and hardened, which are nozzle_high_flow
// and nozzle_hardened.
func nozzle(w *writer, id connectorapi.ToolID, tool numberedTool) {
	w.member(id, connectorapi.FeatureNozzleDiameter, tool.NozzleDiameter)
	w.member(id, connectorapi.FeatureNozzleHighFlow, tool.HighFlow)
	w.member(id, connectorapi.FeatureNozzleHardened, tool.Hardened)
}

// material writes the material of one tool when the frame carries one.
func material(w *writer, id connectorapi.ToolID, raw json.RawMessage) {
	if carriesValue(raw) {
		w.member(id, connectorapi.MaterialAbbreviation, normalizeMaterial(raw))
	}
}

// treeTool is one tool of the tree-shaped INFO. The members are kept raw so
// that a key the frame leaves out is told apart from a null.
type treeTool struct {
	Type     json.RawMessage            `json:"type"`
	Features map[string]json.RawMessage `json:"features"`
	Feeder   json.RawMessage            `json:"feeder"`
	Material json.RawMessage            `json:"material_package_instance"`
}

// treeInfoTools reads the tree shape. The ids and names are the tree's own.
func treeInfoTools(data json.RawMessage) toolReport {
	var info struct {
		ToolCount json.RawMessage            `json:"tool_count"`
		Tools     map[string]json.RawMessage `json:"tools"`
	}

	if json.Unmarshal(data, &info) != nil {
		return toolReport{}
	}

	var w writer

	report := toolReport{layout: layoutUnknown}

	if _, ok := wholeNumber(info.ToolCount); ok {
		w.set(connectorapi.ToolCountPath, info.ToolCount)
	}

	for key, raw := range info.Tools {
		id := connectorapi.ToolID(key)
		if !id.Valid() {
			continue
		}

		if id.IsHead() {
			report.heads = append(report.heads, id)
		}

		if string(raw) == dialogAbsent {
			w.disable(id)

			continue
		}

		var tool treeTool
		if json.Unmarshal(raw, &tool) != nil {
			continue
		}

		w.member(id, connectorapi.ToolEnabled, json.RawMessage(`true`))

		if kind, ok := textOf(tool.Type); ok {
			w.member(id, connectorapi.ToolType, encodeText(kind))
		}

		for name, value := range tool.Features {
			if featureName(name) && scalar(value) {
				w.member(id, "features/"+name, value)
			}
		}

		treeFeeder(&w, id, tool.Feeder)
		treeMaterial(&w, id, tool.Material)
	}

	report.patches = sortedPatches(w.patches)

	return report
}

// treeFeeder writes the feeder of one tree-shaped tool: null is a tool known
// to have none, and a key the frame leaves out says nothing.
func treeFeeder(w *writer, id connectorapi.ToolID, raw json.RawMessage) {
	switch {
	case len(raw) == 0:
		return
	case string(raw) == dialogAbsent:
		noFeeder(w, id)

		return
	}

	var feeder struct {
		SlotCount json.RawMessage `json:"slot_count"`
		Model     json.RawMessage `json:"model"`
		BaseModel json.RawMessage `json:"base_model"`
		Type      json.RawMessage `json:"type"`
	}

	if json.Unmarshal(raw, &feeder) != nil {
		return
	}

	if _, ok := wholeNumber(feeder.SlotCount); ok {
		w.member(id, connectorapi.FeederSlotCount, feeder.SlotCount)
	}

	for member, value := range map[string]json.RawMessage{
		connectorapi.FeederModel: feeder.Model, connectorapi.FeederBaseModel: feeder.BaseModel,
		connectorapi.FeederType: feeder.Type,
	} {
		if name, ok := textOf(value); ok {
			w.member(id, member, encodeText(name))
		}
	}
}

// mmuFeederType is the architecture's word for a multi-material unit, the
// only kind of feeder it allows now.
const mmuFeederType = "MMU"

// feederMembers is every member of a feeder past its slot count: what names
// it and what its unit reports.
//
//nolint:gochecknoglobals // a fixed list, read-only after init.
var feederMembers = []string{
	connectorapi.FeederModel, connectorapi.FeederBaseModel, connectorapi.FeederType,
	connectorapi.FeederMMUProgressCode, connectorapi.FeederMMUCommandInProgress,
}

// noFeeder writes a tool known to have no feeder: a slot count of 0, the
// architecture's null feeder, and nothing else of a feeder. The unit's
// telemetry is not an INFO path, so INFO does not remove it as stale.
func noFeeder(w *writer, id connectorapi.ToolID) {
	w.member(id, connectorapi.FeederSlotCount, json.RawMessage(`0`))

	for _, member := range feederMembers {
		w.remove(connectorapi.ToolPath(id, member))
	}
}

// treeMaterial writes the material of one tree-shaped tool: null is nothing
// loaded, and a package names its material by abbreviation, or by type when
// the abbreviation is left out (the architecture lets the abbreviation be
// inherited from the type). The uuids, type, colour, full weight and profile
// temperatures are written when the instance carries them and removed when it
// does not, so a spool taken out leaves nothing of itself behind.
func treeMaterial(w *writer, id connectorapi.ToolID, raw json.RawMessage) {
	switch {
	case len(raw) == 0:
		return
	case string(raw) == dialogAbsent:
		w.member(id, connectorapi.MaterialAbbreviation, encodeText(connectorapi.NoMaterial))
		dropMaterial(w, id)

		return
	}

	var instance struct {
		UUID    json.RawMessage `json:"uuid"`
		Package struct {
			UUID     json.RawMessage `json:"uuid"`
			Material struct {
				UUID         json.RawMessage `json:"uuid"`
				Abbreviation json.RawMessage `json:"abbreviation"`
				Type         json.RawMessage `json:"type"`
				// Raw, so a colour or properties of another shape lose only
				// themselves and not the abbreviation with them.
				PrimaryColor json.RawMessage `json:"primary_color"`
				Properties   json.RawMessage `json:"properties"`
			} `json:"material"`
		} `json:"package"`
		FullWeight json.RawMessage `json:"actual_netto_full_weight"`
	}

	if json.Unmarshal(raw, &instance) != nil {
		return
	}

	material := instance.Package.Material

	var (
		color struct {
			ColorRGBA json.RawMessage `json:"color_rgba"`
		}
		properties map[string]json.RawMessage
	)

	_ = json.Unmarshal(material.PrimaryColor, &color)
	_ = json.Unmarshal(material.Properties, &properties)

	for _, name := range []json.RawMessage{material.Abbreviation, material.Type} {
		if text, ok := textOf(name); ok {
			w.member(id, connectorapi.MaterialAbbreviation, encodeText(text))

			break
		}
	}

	for member, value := range map[string]json.RawMessage{
		connectorapi.MaterialInstanceUUID: instance.UUID,
		connectorapi.MaterialPackageUUID:  instance.Package.UUID,
		connectorapi.MaterialUUID:         material.UUID,
		connectorapi.MaterialType:         material.Type,
		connectorapi.MaterialColor:        color.ColorRGBA,
	} {
		if text, ok := textOf(value); ok && (member != connectorapi.MaterialColor || connectorapi.ValidColor(text)) {
			w.member(id, member, encodeText(text))
		} else {
			w.remove(connectorapi.ToolPath(id, member))
		}
	}

	reportNumber(w, id, connectorapi.MaterialFullWeight, instance.FullWeight)

	for _, name := range connectorapi.ProfileTemperatures() {
		reportNumber(w, id, connectorapi.MaterialProperty(name), properties[name])
	}
}

// reportNumber writes a member that is a number and removes one that is not.
func reportNumber(w *writer, id connectorapi.ToolID, member string, raw json.RawMessage) {
	var number float64
	if len(raw) == 0 || json.Unmarshal(raw, &number) != nil {
		w.remove(connectorapi.ToolPath(id, member))

		return
	}

	w.member(id, member, raw)
}

// dropMaterial removes every member of the material package instance but the
// abbreviation.
func dropMaterial(w *writer, id connectorapi.ToolID) {
	for _, member := range connectorapi.ToolMembers() {
		if member != connectorapi.MaterialAbbreviation && strings.HasPrefix(member, "material_package_instance/") {
			w.remove(connectorapi.ToolPath(id, member))
		}
	}
}

// featureName reports whether a feature name can stand as one path segment.
func featureName(name string) bool {
	if name == "" {
		return false
	}

	for i := range len(name) {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}

	return true
}

// scalar reports whether a value is a number, a boolean or text.
func scalar(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}

	switch value.(type) {
	case float64, bool, string:
		return true
	}

	return false
}

// normalizeMaterial maps the firmware's material onto the tool model: its
// "---" and an empty name are connectorapi.NoMaterial, and any other text
// passes through as encoded. A value that is not text passes through too.
func normalizeMaterial(raw json.RawMessage) json.RawMessage {
	var material string
	if json.Unmarshal(raw, &material) != nil {
		return raw
	}

	if trimmed := strings.TrimSpace(material); trimmed == "" || trimmed == noMaterial {
		return encodeText(connectorapi.NoMaterial)
	}

	return raw
}

func sortedPatches(patches []connectorapi.StatePatch) []connectorapi.StatePatch {
	slices.SortStableFunc(patches, func(a, b connectorapi.StatePatch) int { return strings.Compare(a.Path, b.Path) })

	return patches
}

// hardwarePath reports whether a path is one INFO writes about the tools.
//
// These paths reach the host whatever the backlog is doing, as the dialog does
// (see Session.report). INFO is sent once per connection and when a person
// lists the printer's storages, so a nozzle diameter dropped from a full queue
// would stay unknown until then.
func hardwarePath(path string) bool {
	if path == connectorapi.ToolCountPath {
		return true
	}

	_, member, ok := connectorapi.SplitToolPath(path)
	if !ok {
		return false
	}

	switch member {
	case connectorapi.ToolEnabled, connectorapi.ToolType, connectorapi.MaterialAbbreviation:
		return true
	}

	return strings.HasPrefix(member, "features/") || strings.HasPrefix(member, "feeder/") ||
		strings.HasPrefix(member, "material_package_instance/")
}

// reportTools puts what one INFO says about the tools on the tree. The first
// INFO of a session also removes the 1-based slot paths, which a state tree
// stored by an earlier release of this driver holds; see legacySlotPaths.
//
// A removal takes a reserved slot whatever its path. There are up to a few
// hundred of them, most for paths the host does not hold, and sent through the
// queue they would push out the telemetry behind them. A slot keeps one value
// per path, so they cost the host a bounded number of patches that it keeps
// nothing of.
func (s *Session) reportTools(data json.RawMessage) {
	if !s.slotsCleared.Swap(true) {
		for _, path := range legacySlotPaths() {
			s.reserve(connectorapi.StatePatch{Path: path, Value: json.RawMessage(dialogAbsent)})
		}

		// The volume is the model's and does not move within a session, so
		// it is said once, beside the first account of the tools.
		for _, patch := range buildVolumePatches(s.identity.Model) {
			s.reserve(patch)
		}
	}

	report := infoTools(data, s.identity.Model)
	if report.layout != layoutUnknown {
		s.layout.Store(report.layout)
	}

	if report.base != toolBaseUnknown {
		s.toolBase.Store(int32(report.base))
	}

	for _, patch := range report.patches {
		s.reserve(patch)
	}

	if report.heads == nil {
		return
	}

	// An INFO describes every tool the printer has, so what the previous one
	// said and this one does not is gone: a tool it no longer names, such as
	// the slots of an MMU switched off or a head of a model whose tool count
	// shrinks, and a member it no longer reports, such as a feature or the
	// material of a head that now carries a feeder.
	tools, paths := heldBy(report.patches)

	s.toolsMu.Lock()
	s.soleHead = report.sole
	previousTools, previousPaths := s.heldTools, s.heldPaths
	s.heldTools, s.heldPaths = tools, paths
	s.forgetActivity(tools, report.patches)
	s.toolsMu.Unlock()

	for _, path := range staleToolPaths(previousTools, previousPaths, tools, paths) {
		s.reserve(connectorapi.StatePatch{Path: path, Value: json.RawMessage(dialogAbsent)})
	}

	s.reportActivity(report.activity())
}

// activity is what an INFO of the numbered shape says about which tools are
// active. The one head of a machine that is not a toolchanger is always
// active: the head in front of an MMU, and the one enabled head of a model
// with several. Telemetry says the rest. The tree shape says it all in its
// telemetry.
func (r toolReport) activity() activity {
	switch {
	case r.layout == layoutFeeder:
		return activity{"0": true}
	case r.layout == layoutHeads && r.sole != "":
		return activity{r.sole: true}
	}

	return nil
}

// heldBy reads what one INFO's patches leave on the tree: every tool they
// name, enabled or not, and every path they give a value.
func heldBy(patches []connectorapi.StatePatch) (tools map[connectorapi.ToolID]bool, paths map[string]bool) {
	tools = map[connectorapi.ToolID]bool{}
	paths = make(map[string]bool, len(patches))

	for _, patch := range patches {
		if string(patch.Value) != dialogAbsent {
			paths[patch.Path] = true
		}

		if id, _, ok := connectorapi.SplitToolPath(patch.Path); ok {
			tools[id] = true
		}
	}

	return tools, paths
}

// staleToolPaths is every path the previous INFO left on the tree that the
// current one does not, sorted: each member of a tool it no longer names, and
// each path it gave a value that the current one gives none.
func staleToolPaths(
	previousTools map[connectorapi.ToolID]bool, previousPaths map[string]bool,
	tools map[connectorapi.ToolID]bool, paths map[string]bool,
) []string {
	stale := map[string]bool{}

	for id := range previousTools {
		if tools[id] {
			continue
		}

		for _, member := range connectorapi.ToolMembers() {
			stale[connectorapi.ToolPath(id, member)] = true
		}
	}

	for path := range previousPaths {
		if !paths[path] {
			stale[path] = true
		}
	}

	out := make([]string, 0, len(stale))
	for path := range stale {
		out = append(out, path)
	}

	slices.Sort(out)

	return out
}

// legacySlotMembers are the members this driver wrote under /slot/<n>, where
// n counted from 1, in a release before the tool tree.
//
//nolint:gochecknoglobals // a fixed list, read-only after init.
var legacySlotMembers = []string{"material", "temperature", "fan_print", "fan_hotend"}

// legacySlotCount is how many /slot/<n> entries that release could write.
const legacySlotCount = 16

// legacyFlatPaths are the paths an earlier release of this driver wrote beside
// the tree for the tool printing now. The tree is the only place a tool lives
// now (ADR-20261025-pze), so a stored tree loses them.
//
//nolint:gochecknoglobals // a fixed list, read-only after init.
var legacyFlatPaths = []string{"/active_tool", "/material", "/temperature/nozzle", "/temperature/nozzle_target"}

// legacySlotPaths is every 1-based slot path this driver wrote, and the flat
// paths of the tool printing now. A host keeps nothing of a removal of a path
// it does not hold, so removing all of them costs nothing on a tree that holds
// none.
func legacySlotPaths() []string {
	out := append([]string{"/slot/active"}, legacyFlatPaths...)

	for n := 1; n <= legacySlotCount; n++ {
		for _, member := range legacySlotMembers {
			out = append(out, "/slot/"+strconv.Itoa(n)+"/"+member)
		}
	}

	return out
}

// wireNumber reads one key the numbered shape counts from 1. Only the
// canonical spelling of a whole number from 1 is one.
func wireNumber(key string) (int, bool) {
	n, err := strconv.Atoi(key)
	if err != nil || n < 1 || strconv.Itoa(n) != key {
		return 0, false
	}

	return n, true
}

func wholeNumber(raw json.RawMessage) (int, bool) {
	var value float64
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value < 0 || value != float64(int(value)) {
		return 0, false
	}

	return int(value), true
}

// textOf is a value as text, and false for one that is not text or is empty:
// an empty type, model or material name says nothing.
func textOf(raw json.RawMessage) (string, bool) {
	var out string
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil || out == "" {
		return "", false
	}

	return out, true
}

func encodeText(text string) json.RawMessage {
	encoded, _ := json.Marshal(text) //nolint:errchkjson // a string always encodes.

	return encoded
}

func mapKeys[V any](m map[int]V) []int {
	out := make([]int, 0, len(m))
	for key := range m {
		out = append(out, key)
	}

	return out
}
