// Package connect answers the requests PrusaSlicer 3 makes to Prusa Connect,
// with the bodies Prusa Connect answers with.
package connect

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// legacyModel is the model and the base model the cloud reports for a precise
// model name.
type legacyModel struct {
	model     string
	baseModel string
	toolCount int
}

// legacyModels maps each precise model name to what the cloud reports. The
// slicer matches its printer profiles by model and base_model, so the values
// must be the cloud's.
//
//nolint:gochecknoglobals // a fixed table, read-only.
var legacyModels = map[string]legacyModel{
	"XLIS": {"XL", "XL", 1}, "XL": {"XL", "XL", 1}, "XL2IS": {"XL", "XL", 2}, "XL2": {"XL", "XL", 2},
	"XL5": {"XL", "XL", 5}, "XL5IS": {"XL", "XL", 5},
	"XLP1T": {"XLP", "XL", 1}, "XLP2T": {"XLP", "XL", 2}, "XLP5T": {"XLP", "XL", 5},
	"COREONE": {"COREONE", "COREONE", 1},
	"MK4":     {"MK4", "MK4", 1}, "MK4IS": {"MK4", "MK4", 1}, "MK4S": {"MK4S", "MK4", 1},
	"MK3.9": {"MK3.9", "MK4", 1}, "MK3.9S": {"MK3.9S", "MK4", 1},
	"MK3.5": {"MK3.5", "MK3.5", 1}, "MK3S": {"MK3S", "MK3", 1}, "MK3": {"MK3", "MK3", 1},
	"MINI": {"MINI", "MINI", 1}, "MINIIS": {"MINI", "MINI", 1},
	"SL1S": {"SL1S", "SL1", 1}, "SL1": {"SL1", "SL1", 1},
	"COREONEL":        {"COREONEL", "COREONE", 1},
	"COREONEL_INDX8T": {"COREONEL_INDX", "COREONE_INDX", 8}, "COREONEL_INDX4T": {"COREONEL_INDX", "COREONE_INDX", 4},
	"COREONE_INDX8T": {"COREONE_INDX", "COREONE_INDX", 8}, "COREONE_INDX4T": {"COREONE_INDX", "COREONE_INDX", 4},
	"MK4ISMMU3": {"MK4", "MK4", 1}, "MK4SMMU3": {"MK4S", "MK4", 1},
	"MK3.9MMU3": {"MK3.9", "MK4", 1}, "MK3.9SMMU3": {"MK3.9S", "MK4", 1},
	"MK3.5MMU3": {"MK3.5", "MK3.5", 1}, "MK3SMMU3": {"MK3S", "MK3", 1},
	"COREONEMMU3": {"COREONE", "COREONE", 1}, "COREONELMMU3": {"COREONEL", "COREONE", 1},
	"COREONEOAK": {"COREONEOAK", "COREONE", 1},
}

// localModels maps the model a printer reports to this server, in the
// firmware's spelling, to the precise model name the cloud keys its table by.
// A model whose precise name depends on the tool count or on an MMU is
// finished by preciseModel.
//
//nolint:gochecknoglobals // a fixed table, read-only.
var localModels = map[string]string{
	"CoreOne":       "COREONE",
	"CoreOneL":      "COREONEL",
	"CoreOne_INDX":  "COREONE_INDX",
	"CoreOneL_INDX": "COREONEL_INDX",
	"MINI":          "MINIIS",
	"MK3.5":         "MK3.5",
	"MK3.5S":        "MK3.5",
	"MK3.9":         "MK3.9",
	"MK3.9S":        "MK3.9S",
	"MK4":           "MK4IS",
	"MK4S":          "MK4S",
	"XL":            "XL",
	"MK3":           "MK3",
	"MK3S":          "MK3S",
	"SL1":           "SL1",
	"SL1S":          "SL1S",
}

// foldedModels is localModels, and the precise names the INDX rows are
// finished from, keyed by the folded spelling the core compares models by. A
// firmware reports the INDX by the id of its own model table, COREONEINDX or
// COREONEL-INDX, which no exact key of localModels matches.
//
//nolint:gochecknoglobals // derived once from the fixed tables above.
var foldedModels = func() map[string]string {
	indx := []string{"COREONE_INDX", "COREONEL_INDX"}

	out := make(map[string]string, len(localModels)+len(indx))
	for spelling, precise := range localModels {
		out[connectorapi.FoldModel(spelling)] = precise
	}

	for _, precise := range indx {
		out[connectorapi.FoldModel(precise)] = precise
	}

	return out
}()

// connectStates are the values of connect_state PrusaSlicer knows. The cloud also answers UNKNOWN for a printer it has
// not heard from.
//
//nolint:gochecknoglobals // a fixed set, read-only.
var connectStates = map[string]bool{
	"OFFLINE": true, "PRINTING": true, "PAUSED": true, "STOPPED": true, "IDLE": true,
	"FINISHED": true, "READY": true, "ATTENTION": true, "BUSY": true, "ERROR": true,
}

// Printer is what the printer list needs to know of one device.
type Printer struct {
	ID        string
	Name      string
	Model     string
	Connected bool
	State     map[string]json.RawMessage
	// Capabilities is the verb set the host resolved for the printer.
	Capabilities []string
}

// The capabilities a printer needs to take a file and print it. The cloud
// refuses an upload to a printer that takes no file with PRINTER_NO_TRASFER,
// and this module does the same.
const (
	capabilityUpload = "core.feature.file_upload"
	capabilityPrint  = "core.command.start_print"
)

// TakesFiles reports whether the printer takes a file and starts it.
func (p Printer) TakesFiles() bool {
	upload, start := false, false

	for _, capability := range p.Capabilities {
		upload = upload || capability == capabilityUpload
		start = start || capability == capabilityPrint
	}

	return upload && start
}

// PrinterJSON renders a printer as the cloud's slicer printer list renders
// one: uuid, name, connect_state, the model fields of the legacy table,
// legacy_printer_model, and tools. Empty values are left out, as the cloud
// leaves them out, and tools is always there.
func PrinterJSON(p Printer) map[string]any {
	out := map[string]any{"uuid": p.ID}

	if p.Name != "" {
		out["name"] = p.Name
	}

	out["connect_state"] = connectState(p)

	tools := toolsOf(p.State)
	precise := preciseModel(p.Model, headCount(p.Model, p.State, tools), hasMMU(tools))

	if precise != "" {
		out["legacy_printer_model"] = precise
	}

	if row, ok := legacyModels[precise]; ok {
		out["model"] = row.model
		out["base_model"] = row.baseModel
		out["tool_count"] = row.toolCount
	}

	out["tools"] = tools

	return out
}

// connectState is the state the cloud reports: OFFLINE for a printer that is
// away, the state the printer reported when PrusaSlicer knows it, and UNKNOWN
// otherwise.
func connectState(p Printer) string {
	if !p.Connected {
		return "OFFLINE"
	}

	var state string
	if err := json.Unmarshal(p.State["/state"], &state); err != nil {
		return "UNKNOWN"
	}

	state = strings.ToUpper(state)
	if connectStates[state] {
		return state
	}

	return "UNKNOWN"
}

// The tool counts that tell the precise models of one family apart: an INDX
// with more than four tools is the eight-tool one, and the XL comes with one,
// two or five tools.
const (
	indxFourTools = 4
	xlTwoTools    = 2
	xlFiveTools   = 5
)

// preciseModel is the precise model name the cloud would report.
func preciseModel(model string, tools int, mmu bool) string {
	precise, ok := localModels[model]
	if !ok {
		precise, ok = foldedModels[connectorapi.FoldModel(model)]
	}

	if !ok {
		precise = strings.ToUpper(model)
	}

	switch precise {
	case "COREONE_INDX", "COREONEL_INDX":
		if tools > indxFourTools {
			return precise + "8T"
		}

		return precise + "4T"
	case "XL":
		switch {
		case tools >= xlFiveTools:
			return "XL5IS"
		case tools == xlTwoTools:
			return "XL2IS"
		default:
			return "XLIS"
		}
	}

	if mmu {
		if _, known := legacyModels[precise+"MMU3"]; known {
			return precise + "MMU3"
		}
	}

	return precise
}

// headCount is the number of heads that tells the precise model apart. It is
// toolCount, except on an INDX: its firmware reports a tool_count of eight for
// the four-head build too, and disables the heads that are not fitted. There
// the highest enabled head decides, and toolCount only when no head says.
func headCount(model string, state map[string]json.RawMessage, tools map[string]any) int {
	count := toolCount(state, tools)

	switch preciseModel(model, 0, false) {
	case "COREONE_INDX4T", "COREONEL_INDX4T":
		if fitted := highestEnabledHead(state); fitted > 0 {
			return fitted
		}
	}

	return count
}

// highestEnabledHead is one more than the highest head id whose
// /tools/<id>/enabled is true, and 0 when no head reports enabled. A slot of a
// feeder, such as 0.1, is not a head.
func highestEnabledHead(state map[string]json.RawMessage) int {
	highest := 0

	for path, raw := range state {
		rest, ok := strings.CutPrefix(path, "/tools/")
		if !ok {
			continue
		}

		id, member, ok := strings.Cut(rest, "/")
		if !ok || member != "enabled" {
			continue
		}

		n, err := strconv.Atoi(id)
		if err != nil || n < 0 {
			continue
		}

		var enabled bool
		if json.Unmarshal(raw, &enabled) == nil && enabled {
			highest = max(highest, n+1)
		}
	}

	return highest
}

// toolCount is /tool_count, and the number of tools when the printer did not
// report it.
func toolCount(state map[string]json.RawMessage, tools map[string]any) int {
	var count int
	if err := json.Unmarshal(state["/tool_count"], &count); err == nil && count > 0 {
		return count
	}

	return len(tools)
}

// hasMMU reports a tool with an MMU feeder.
func hasMMU(tools map[string]any) bool {
	for _, tool := range tools {
		fields, _ := tool.(map[string]any)
		feeder, _ := fields["feeder"].(map[string]any)

		if kind, _ := feeder["type"].(string); strings.EqualFold(kind, "MMU") {
			return true
		}
	}

	return false
}

// toolsOf builds the tools object from the state paths under /tools/. This
// server keeps the tree the printer reported as flat paths, such as
// /tools/0/features/nozzle_diameter, and the cloud answers the same tree
// nested. The tool type this server holds is the v3 type, such as
// FFF_PRINT_HEAD, and the v2 list answers the class apart: FFF and PRINT_HEAD.
func toolsOf(state map[string]json.RawMessage) map[string]any {
	tools := map[string]any{}

	for path, raw := range state {
		rest, ok := strings.CutPrefix(path, "/tools/")
		if !ok {
			continue
		}

		segments := strings.Split(rest, "/")
		if len(segments) < 2 || segments[0] == "" {
			continue
		}

		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}

		node, _ := tools[segments[0]].(map[string]any)
		if node == nil {
			node = map[string]any{}
			tools[segments[0]] = node
		}

		insert(node, segments[1:], value)
	}

	for key, tool := range tools {
		fields, _ := tool.(map[string]any)

		// The architecture's null tool: a head that is not fitted, or a slot
		// switched off. The cloud's list names only the tools that are there.
		if enabled, says := fields["enabled"].(bool); says && !enabled {
			delete(tools, key)

			continue
		}

		splitToolType(fields)
		materialType(fields)
		tools[key] = cloudTool(fields)
	}

	return tools
}

// cloudToolKeys are the keys a tool carries in the cloud's v2 list. This server keeps more
// about a tool, such as its temperatures, and the list leaves those out.
//
//nolint:gochecknoglobals // a fixed set, read-only.
var cloudToolKeys = []string{"class", "type", "features", "feeder", "material_package_instance", "telemetry"}

// The MMU this server reports by its slot count. The cloud names the model of
// the feeder, and PrusaSlicer refuses a feeder without one. Every Prusa MMU a printer reports today is an
// MMU3.
const (
	feederMMU      = "MMU"
	feederMMUModel = "MMU3"
)

// cloudTool keeps the keys the cloud answers and writes the feeder and the
// material the way the cloud writes them.
func cloudTool(fields map[string]any) map[string]any {
	out := map[string]any{}

	for _, key := range cloudToolKeys {
		if value, ok := fields[key]; ok {
			out[key] = value
		}
	}

	if feeder, ok := cloudFeeder(fields["feeder"]); ok {
		out["feeder"] = feeder
	} else {
		delete(out, "feeder")
	}

	if instance, has := out["material_package_instance"]; has && !hasMaterial(instance) {
		out["material_package_instance"] = nil
	}

	return out
}

// cloudFeeder is the feeder of a tool as the cloud reports it, and false for a
// tool with no feeder. This server reports a feeder with zero slots on every
// print head; the cloud reports none there.
func cloudFeeder(raw any) (map[string]any, bool) {
	feeder, _ := raw.(map[string]any)
	if feeder == nil {
		return nil, false
	}

	kind, _ := feeder["type"].(string)
	slots, _ := feeder["slot_count"].(float64)

	if !strings.EqualFold(kind, feederMMU) || slots <= 0 {
		return nil, false
	}

	return map[string]any{
		"type":       feederMMU,
		"model":      feederMMUModel,
		"base_model": feederMMUModel,
		"num_slots":  int(slots),
	}, true
}

// hasMaterial reports a material package that names a material.
func hasMaterial(instance any) bool {
	fields, _ := instance.(map[string]any)
	pack, _ := fields["package"].(map[string]any)
	material, _ := pack["material"].(map[string]any)
	kind, _ := material["type"].(string)

	return kind != ""
}

// insert writes value at the path inside node, making the objects on the way.
func insert(node map[string]any, path []string, value any) {
	for _, segment := range path[:len(path)-1] {
		next, _ := node[segment].(map[string]any)
		if next == nil {
			next = map[string]any{}
			node[segment] = next
		}

		node = next
	}

	node[path[len(path)-1]] = value
}

// materialType fills material.type from material.abbreviation when the
// printer reported only the abbreviation. The cloud answers both, and
// PrusaSlicer reads type.
func materialType(fields map[string]any) {
	instance, _ := fields["material_package_instance"].(map[string]any)
	pack, _ := instance["package"].(map[string]any)
	material, _ := pack["material"].(map[string]any)

	if material == nil {
		return
	}

	if _, has := material["type"]; !has {
		if abbreviation, ok := material["abbreviation"].(string); ok && abbreviation != "" {
			material["type"] = abbreviation
		}
	}
}

// splitToolType turns a v3 tool type into the v2 class and type.
func splitToolType(fields map[string]any) {
	kind, _ := fields["type"].(string)

	for _, class := range []string{"FFF", "SLA"} {
		if rest, ok := strings.CutPrefix(kind, class+"_"); ok {
			fields["class"] = class
			fields["type"] = rest

			return
		}
	}
}

// teamID reads the team in a path. Every team id is accepted: this server has
// one team, and the slicer sends the id the page gave it.
func teamID(text string) (int64, bool) {
	id, err := strconv.ParseInt(text, 10, 64)

	return id, err == nil && id >= 0
}
