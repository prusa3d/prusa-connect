package xbuddy

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// What a telemetry frame says about the tools, normalized onto the tool tree.
// See tools.go for the two shapes the firmware speaks.
//
// The numbered shape carries a slot block on a machine with more than one
// filament path enabled (render_msg in src/connect/render.cpp):
//
//	"slot": {"active": 1, "state": 1, "1": {"material": "PLA", "temp": 215,
//	         "fan_print": 0, "fan_hotend": 8000}}
//
// Its entries are numbered from 1 like the INFO tools, and active is the
// selected entry plus 1, with 0 for none. In front of an MMU the block also
// carries state, the unit's progress code, and command, the command it runs
// as one character. The nozzle temperature and target at the top of the frame
// are the selected path's, and land on the head that path prints through.
//
// The tree shape carries a tools object keyed by id, with the members of the
// architecture's printer telemetry.
//
// Both shapes say which tools are active. The tree shape marks each tool, and
// the numbered shape names one entry. Either becomes an activity, which
// Session.reportActivity turns into the ToolActive member of each tool. The
// session keeps the tool those members derive (connectorapi.ActiveToolOf),
// which is where the top of a numbered frame lands, and writes no path for it.

// activity is what a frame says about which tools are active, by id.
type activity map[connectorapi.ToolID]bool

// reportedActivity is what a session last reported of the active tools: the
// member of each tool, and the active tool derived from them.
type reportedActivity struct {
	members map[connectorapi.ToolID]bool
	tool    connectorapi.ToolID
}

// numberedSlotMembers maps the members of one slot block entry to the head
// members they are on the tree.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var numberedSlotMembers = map[string]string{
	"temp":       connectorapi.NozzleTemp,
	"fan_print":  connectorapi.PrintFanRPM,
	"fan_hotend": connectorapi.HeatbreakFanRPM,
}

// treeTelemetryMembers are the members of one tool in a tree-shaped
// telemetry frame this driver passes through. A null target is a heater that
// is off, which the tree holds as 0. Any other null removes the path: the
// architecture writes a null consumed weight for a printer that cannot count
// it.
//
//nolint:gochecknoglobals // a fixed list, read-only after init.
var treeTelemetryMembers = []string{
	connectorapi.NozzleTemp, connectorapi.NozzleTargetTemp,
	connectorapi.PrintFanRPM, connectorapi.HeatbreakFanRPM, connectorapi.ConsumedWeight,
	connectorapi.OdometerID, connectorapi.OdometerE, connectorapi.OdometerWeight,
}

// currentLayout is how the numbered entries sit on the tree: what the last
// INFO said, and before any INFO, slots on a model with one head and heads on
// any other.
func (s *Session) currentLayout() int32 {
	if layout := s.layout.Load(); layout != layoutUnknown {
		return layout
	}

	if heads, known := headsOf(s.identity.Model); known && heads == 1 {
		return layoutFeeder
	}

	return layoutHeads
}

// currentSoleHead is the head the material at the top of a frame belongs to
// when no slot block says more, and false when it is not known.
func (s *Session) currentSoleHead() (connectorapi.ToolID, bool) {
	s.toolsMu.Lock()
	sole, reported := s.soleHead, s.heldTools != nil
	s.toolsMu.Unlock()

	if reported {
		return sole, sole != ""
	}

	if heads, known := headsOf(s.identity.Model); known && heads == 1 {
		return "0", true
	}

	return "", false
}

// reportToolTelemetry writes what one telemetry frame says about the tools.
func (s *Session) reportToolTelemetry(fields map[string]json.RawMessage) {
	if tools, tree := fields["tools"]; tree {
		patches, nulls, said := treeTelemetryPatches(tools)
		for _, patch := range patches {
			s.report(patch)
		}

		for _, patch := range s.staleReadings(patches, nulls) {
			s.report(patch)
		}

		s.reportActivity(said)

		return
	}

	slot, multi := fields["slot"]

	if multi {
		layout := s.currentLayout()

		patches, said := numberedSlotPatches(slot, layout)
		for _, patch := range patches {
			s.report(patch)
		}

		s.reportActivity(said)
		s.reportTopReadings(fields, layout)

		return
	}

	// A machine with one path is one head, and the top of the frame is all
	// it says of it. It sends no material while nothing is loaded, so the
	// empty tool is read from INFO. Before an INFO names the head, a model
	// this driver has no figures for is taken as head 0.
	head, known := s.currentSoleHead()
	if !known {
		head = "0"
	}

	s.reportHead(fields, head, true)
}

// reportTopReadings writes the nozzle temperature and target at the top of a
// numbered frame with a slot block. In front of an MMU they are the one
// head's. On a toolchanger the slot block carries each head's temperature and
// no head's target. The target at the top is the picked head's, so it lands
// there. A head that is not picked has no target this shape says, so the
// target written while it was picked is removed rather than left standing. With
// no head picked, the target says nothing of a head.
func (s *Session) reportTopReadings(fields map[string]json.RawMessage, layout int32) {
	if layout == layoutFeeder {
		s.reportHead(fields, "0", false)

		return
	}

	target, present := fields["target_nozzle"]
	if !present {
		return
	}

	s.toolsMu.Lock()
	head := s.activity.tool.Head()
	stale := s.targetHead
	s.targetHead = head
	s.toolsMu.Unlock()

	if stale != "" && stale != head {
		s.report(connectorapi.StatePatch{
			Path: connectorapi.ToolPath(stale, connectorapi.NozzleTargetTemp), Value: json.RawMessage(dialogAbsent),
		})
	}

	if head != "" {
		s.report(connectorapi.StatePatch{
			Path: connectorapi.ToolPath(head, connectorapi.NozzleTargetTemp), Value: target,
		})
	}
}

// reportHead writes what the top of a frame says of one head: its nozzle
// temperature and target, and its material when withMaterial is true.
func (s *Session) reportHead(fields map[string]json.RawMessage, head connectorapi.ToolID, withMaterial bool) {
	for key, member := range map[string]string{
		"temp_nozzle": connectorapi.NozzleTemp, "target_nozzle": connectorapi.NozzleTargetTemp,
	} {
		if value, present := fields[key]; present {
			s.report(connectorapi.StatePatch{Path: connectorapi.ToolPath(head, member), Value: value})
		}
	}

	if material, present := fields["material"]; present && withMaterial {
		s.report(connectorapi.StatePatch{
			Path:  connectorapi.ToolPath(head, connectorapi.MaterialAbbreviation),
			Value: normalizeMaterial(material),
		})
	}
}

// noNozzleTemp is the nozzle temperature the firmware sends for a head it has
// no reading of. An INDX reads only the head on the carriage, and each docked
// head carries -1, in the slot block's temp and in the tree's nozzle_temp alike
// (render_msg copies hotend.temp_nozzle with no check). It is no temperature,
// so the head's reading is removed rather than shown as -1 °C.
const noNozzleTemp = -1

// noNozzleReading reports whether a head's nozzle temperature is the
// firmware's -1 for a head with no reading. Only that value is, so a real reading below zero
// still passes through.
func noNozzleReading(raw json.RawMessage) bool {
	var value float64

	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value == noNozzleTemp
}

// numberedSlotPatches reads the numbered slot block onto the tree.
//
// On a toolchanger, entry n is head n-1 and carries that head's readings and
// material. In front of an MMU, entry n is slot "0.<n-1>" and carries its
// material; the readings are the one head's, and the selected entry's are
// taken for it.
//
// Only members the frame carries are written, values pass through as encoded
// apart from the material, and the result is sorted by path. Anything else in
// the block is ignored, as is a block that is not an object. The activity is
// what the block's active entry says, and nil when it carries none.
func numberedSlotPatches(raw json.RawMessage, layout int32) ([]connectorapi.StatePatch, activity) {
	// Most models send no slot block, and this runs for every telemetry frame:
	// an Unmarshal of nothing would allocate an error each time.
	if len(raw) == 0 {
		return nil, nil
	}

	var block map[string]json.RawMessage
	if json.Unmarshal(raw, &block) != nil {
		return nil, nil
	}

	feeder := layout == layoutFeeder
	pathOf := func(n int) connectorapi.ToolID {
		if feeder {
			return connectorapi.ToolID("0").Slot(n - 1)
		}

		return connectorapi.HeadID(n - 1)
	}

	var w writer

	entries := numberedEntries(&w, block, feeder, pathOf)

	active, activeKnown := wholeNumber(block["active"])
	said := numberedActivity(entries, active, activeKnown, feeder, pathOf)

	if feeder {
		numberedUnit(&w, block)
	}

	if feeder && len(entries) > 0 {
		// Every entry reads the one nozzle. The fans are the head's, taken
		// from the selected entry, or the first when none is selected.
		pick := slices.Min(mapKeys(entries))
		if _, has := entries[active]; activeKnown && has {
			pick = active
		}

		for _, from := range []string{"fan_print", "fan_hotend"} {
			w.member("0", numberedSlotMembers[from], entries[pick][from])
		}
	}

	return sortedPatches(w.patches), said
}

// numberedUnit writes what the MMU is doing, from the slot block of a head
// with an MMU: state is its progress code, and command is the command it runs
// as one character. The firmware formats a command of 0, which an idle unit
// reports, as an empty or a NUL character, and that is no command.
func numberedUnit(w *writer, block map[string]json.RawMessage) {
	if _, ok := wholeNumber(block["state"]); ok {
		w.member("0", connectorapi.FeederMMUProgressCode, block["state"])
	}

	raw, present := block["command"]
	if !present {
		return
	}

	command, isText := textOf(raw)

	switch {
	case isText && command != "\x00":
		w.member("0", connectorapi.FeederMMUCommandInProgress, encodeText(command))
	case isText || string(raw) == `""`:
		w.remove(connectorapi.ToolPath("0", connectorapi.FeederMMUCommandInProgress))
	}
}

// numberedEntries writes the material of every entry, and on a toolchanger
// its readings, and returns the entries by number.
func numberedEntries(
	w *writer, block map[string]json.RawMessage, feeder bool, pathOf func(int) connectorapi.ToolID,
) map[int]map[string]json.RawMessage {
	entries := map[int]map[string]json.RawMessage{}

	for key, value := range block {
		n, ok := wireNumber(key)
		if !ok {
			continue
		}

		var members map[string]json.RawMessage
		if json.Unmarshal(value, &members) != nil {
			continue
		}

		entries[n] = members
		id := pathOf(n)

		if material, has := members["material"]; has && carriesValue(material) {
			w.member(id, connectorapi.MaterialAbbreviation, normalizeMaterial(material))
		}

		if !feeder {
			for from, to := range numberedSlotMembers {
				if from == "temp" && noNozzleReading(members[from]) {
					w.remove(connectorapi.ToolPath(id, to))

					continue
				}

				w.member(id, to, members[from])
			}
		}
	}

	return entries
}

// numberedActivity is what the firmware's active entry says, counted from 1
// with 0 for none. On a toolchanger the entry is the head picked, and every
// other listed head is not. In front of an MMU the entry is the slot loaded,
// every other listed slot is not, and the one head is always active. It is
// nil when the block carries no active entry.
func numberedActivity(
	entries map[int]map[string]json.RawMessage, active int, known, feeder bool, pathOf func(int) connectorapi.ToolID,
) activity {
	if !known {
		return nil
	}

	said := activity{}

	if feeder {
		said["0"] = true
	}

	for n := range entries {
		said[pathOf(n)] = n == active
	}

	if active > 0 {
		said[pathOf(active)] = true
	}

	return said
}

// staleReadings records which tools a tree-shaped telemetry frame wrote
// readings for, and returns the removal of the readings of each tool the frame
// reports as null that an earlier frame wrote readings for. A null tool is one
// the machine has switched off, and its last temperature is not its
// temperature now. Whether it is enabled is the INFO's to say.
func (s *Session) staleReadings(
	patches []connectorapi.StatePatch, nulls []connectorapi.ToolID,
) []connectorapi.StatePatch {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()

	for _, patch := range patches {
		id, member, ok := connectorapi.SplitToolPath(patch.Path)
		if !ok || string(patch.Value) == dialogAbsent || !slices.Contains(treeTelemetryMembers, member) {
			continue
		}

		if s.readings == nil {
			s.readings = map[connectorapi.ToolID]bool{}
		}

		s.readings[id] = true
	}

	var out []connectorapi.StatePatch

	for _, id := range nulls {
		if !s.readings[id] {
			continue
		}

		delete(s.readings, id)

		for _, member := range treeTelemetryMembers {
			out = append(out, connectorapi.StatePatch{
				Path: connectorapi.ToolPath(id, member), Value: json.RawMessage(dialogAbsent),
			})
		}
	}

	return out
}

// treeTelemetryPatches reads a tree-shaped tools object. said is the active
// member of each tool that carries one: the architecture marks a head while it
// is picked and a slot while it is loaded. nulls is every tool the object
// reports as null, sorted.
func treeTelemetryPatches(
	raw json.RawMessage,
) (patches []connectorapi.StatePatch, nulls []connectorapi.ToolID, said activity) {
	var tools map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &tools) != nil {
		return nil, nil, nil
	}

	var w writer

	for key, value := range tools {
		id := connectorapi.ToolID(key)
		if !id.Valid() {
			continue
		}

		if string(value) == dialogAbsent {
			nulls = append(nulls, id)

			continue
		}

		var members map[string]json.RawMessage
		if json.Unmarshal(value, &members) != nil {
			continue
		}

		treeReadings(&w, id, members)
		treeUnit(&w, id, members["feeder"])

		var active bool
		if json.Unmarshal(members[connectorapi.ToolActive], &active) != nil {
			continue
		}

		if said == nil {
			said = activity{}
		}

		said[id] = active
	}

	slices.SortFunc(nulls, connectorapi.ToolID.Compare)

	return sortedPatches(w.patches), nulls, said
}

// treeReadings writes the readings one tree-shaped tool carries. A null
// target is a heater that is off and is written as 0. Any other null removes
// the path, and a member the frame leaves out says nothing.
func treeReadings(w *writer, id connectorapi.ToolID, members map[string]json.RawMessage) {
	for _, member := range treeTelemetryMembers {
		reading, present := members[member]

		switch {
		case !present:
		case string(reading) == dialogAbsent && member == connectorapi.NozzleTargetTemp:
			w.member(id, member, json.RawMessage(`0`))
		case string(reading) == dialogAbsent:
			w.remove(connectorapi.ToolPath(id, member))
		case member == connectorapi.NozzleTemp && noNozzleReading(reading):
			w.remove(connectorapi.ToolPath(id, member))
		default:
			w.member(id, member, reading)
		}
	}
}

// treeUnit writes the feeder telemetry of one tree-shaped tool: the MMU's
// progress code and the command it runs. The firmware leaves the command out
// while the unit runs none, so a feeder object without one removes it. A tool
// whose telemetry carries no feeder says nothing of it.
func treeUnit(w *writer, id connectorapi.ToolID, raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == dialogAbsent {
		return
	}

	var unit struct {
		ProgressCode json.RawMessage `json:"mmu_progress_code"`
		Command      json.RawMessage `json:"mmu_command_in_progress"`
	}

	if json.Unmarshal(raw, &unit) != nil {
		return
	}

	if _, ok := wholeNumber(unit.ProgressCode); ok {
		w.member(id, connectorapi.FeederMMUProgressCode, unit.ProgressCode)
	}

	if command, ok := textOf(unit.Command); ok {
		w.member(id, connectorapi.FeederMMUCommandInProgress, encodeText(command))
	} else {
		w.remove(connectorapi.ToolPath(id, connectorapi.FeederMMUCommandInProgress))
	}
}

// reportActivity reports what a frame says about which tools are active: the
// ToolActive member of each tool it names. Only a change is reported. The
// paths are reserved (activityPath), so a value is never dropped from a full
// queue and the last one always reaches the host. It also keeps the tool the
// members derive, which reportTopReadings reads.
func (s *Session) reportActivity(said activity) {
	s.toolsMu.Lock()

	ids := make([]connectorapi.ToolID, 0, len(said))
	for id := range said {
		ids = append(ids, id)
	}

	slices.SortFunc(ids, connectorapi.ToolID.Compare)

	var out []connectorapi.StatePatch

	for _, id := range ids {
		on := said[id]
		if was, held := s.activity.members[id]; held && was == on {
			continue
		}

		if s.activity.members == nil {
			s.activity.members = map[connectorapi.ToolID]bool{}
		}

		s.activity.members[id] = on
		out = append(out, connectorapi.StatePatch{
			Path: connectorapi.ToolPath(id, connectorapi.ToolActive), Value: json.RawMessage(strconv.FormatBool(on)),
		})
	}

	s.activity.tool, _ = connectorapi.ActiveToolOf(s.activity.members)
	s.toolsMu.Unlock()

	for _, patch := range out {
		s.report(patch)
	}
}

// forgetActivity drops what this session holds of the active member of each
// tool an INFO took off the tree or disabled, since the INFO removed that
// member. The caller holds toolsMu.
func (s *Session) forgetActivity(tools map[connectorapi.ToolID]bool, patches []connectorapi.StatePatch) {
	for id := range s.activity.members {
		if !tools[id] {
			delete(s.activity.members, id)
		}
	}

	for _, patch := range patches {
		if id, member, ok := connectorapi.SplitToolPath(patch.Path); ok &&
			member == connectorapi.ToolActive && string(patch.Value) == dialogAbsent {
			delete(s.activity.members, id)
		}
	}
}

// activityPath reports whether a path is one reportActivity writes.
func activityPath(path string) bool {
	_, member, ok := connectorapi.SplitToolPath(path)

	return ok && member == connectorapi.ToolActive
}
