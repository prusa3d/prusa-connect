package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"

	"github.com/prusa3d/materials/internal/assign"
	"github.com/prusa3d/materials/internal/colour"
	"github.com/prusa3d/materials/internal/printers"
	"github.com/prusa3d/materials/internal/stock"
)

// The printer routes. The host lets a person call them with access to this
// module. The module reads the printers as itself, so it asks the host
// through access.check what the person may do on each printer (access.go).
// A printer the person may not observe is not there for them, as in the
// host's own device list. A change to a tool also takes operate on the
// printer.

// Fleet is the printers as the follower last read them.
type Fleet interface {
	Printer(id string) (printers.Printer, bool)
	// Printers lists every printer without its tools.
	Printers() []printers.Printer
	// Listed is true once the follower has listed the printers, so a printer
	// it does not hold has left the host.
	Listed() bool
}

// reportedView is what the printer reports of the material on a tool.
type reportedView struct {
	// Abbreviation is empty for a tool reported empty.
	Abbreviation string   `json:"abbreviation"`
	InstanceUUID string   `json:"instance_uuid"`
	PackageUUID  string   `json:"package_uuid"`
	MaterialUUID string   `json:"material_uuid"`
	Type         string   `json:"type"`
	ColorRGBA    string   `json:"color_rgba"`
	FullWeightG  *float64 `json:"full_weight_g,omitempty"`
	// ConsumedWeightG is what the printer counts used of the spool.
	ConsumedWeightG *float64 `json:"consumed_weight_g,omitempty"`
}

// toolView is one tool with what is on it and what the caller may do.
type toolView struct {
	ID         string            `json:"id"`
	Assignment *stock.Assignment `json:"assignment"`
	Reported   *reportedView     `json:"reported"`
	FromNFC    bool              `json:"from_nfc"`
	Assign     assign.Reason     `json:"assign"`
	Unassign   assign.Reason     `json:"unassign"`
	// Kind is head for a print head and slot for a slot of a feeder.
	Kind string `json:"kind"`
	// Active is true for the tool the printer prints with, as the printer
	// reports it.
	Active bool `json:"active"`
	// Nozzle is the nozzle the tool prints through: a slot's is its head's.
	Nozzle nozzleView `json:"nozzle"`
	// Feeder names the feeder a slot sits in, such as MMU3S, empty for a head
	// or an unknown model.
	Feeder string `json:"feeder,omitempty"`
}

// nozzleView is what the printer reports of a nozzle. A feature it does not
// report is left out.
type nozzleView struct {
	DiameterMM *float64 `json:"diameter_mm,omitempty"`
	HighFlow   *bool    `json:"high_flow,omitempty"`
	Hardened   *bool    `json:"hardened,omitempty"`
	HighTemp   *bool    `json:"high_temp,omitempty"`
}

// nozzleOf reads the nozzle of one tool off the tree. A value of the wrong
// type reads as not reported.
func nozzleOf(tree connectorapi.ToolTree, id connectorapi.ToolID) nozzleView {
	var n nozzleView

	for name, into := range map[string]any{
		"nozzle_diameter": &n.DiameterMM, "nozzle_high_flow": &n.HighFlow,
		"nozzle_hardened": &n.Hardened, "nozzle_high_temp": &n.HighTemp,
	} {
		if raw, ok := tree.Nozzle(id, name); ok {
			_ = json.Unmarshal(raw, into)
		}
	}

	return n
}

// shapeOf is a tool's kind, nozzle and feeder, as the Tools tab draws them.
func shapeOf(tree connectorapi.ToolTree, id connectorapi.ToolID, view *toolView) {
	view.Kind, view.Active, view.Nozzle = "head", tree.IsActive(id), nozzleOf(tree, id)
	if id.IsHead() {
		return
	}

	view.Kind = "slot"
	if head := tree.Head(id); head != nil && head.Feeder != nil {
		view.Feeder = head.Feeder.Model
	}
}

// toolsView is one printer and its tools.
type toolsView struct {
	printerView

	Tools []toolView `json:"tools"`
}

// tools answers the tools of one printer.
func (s *server) tools(w http.ResponseWriter, r *http.Request, by string) {
	device := r.PathValue("device")

	p, ok := s.fleet.Printer(device)
	if !ok {
		noPrinter(w, device)

		return
	}

	observe, operate, err := s.rights(r.Context(), by, device)
	if err != nil {
		undecided(w, r, err)

		return
	}

	if !observe {
		noPrinter(w, device)

		return
	}

	view := viewOf(p, operate)

	stored, err := s.store.Assignments(r.Context(), p.ID)
	if err != nil {
		refused(w, r, err, InvalidQuery)

		return
	}

	on := map[string]*stock.Assignment{}
	for i := range stored {
		on[stored[i].ToolID] = &stored[i]
	}

	state := assign.StateOf(p.Connected, p.State)
	out := []toolView{}

	for _, id := range slices.Sorted(maps.Keys(p.Tools.Tools)) {
		tool := p.Tools.Tools[id]
		if !holdsMaterial(tool) {
			continue
		}

		rules := assign.ToolOf(*tool)
		// The tool's own reason. A material the report does not match is
		// refused when it is picked.
		candidate := assign.Candidate{}
		if tool.Material != nil {
			candidate.Abbreviation = *tool.Material
		}

		view := toolView{
			ID: string(id), Assignment: on[string(id)], Reported: reportedOf(*tool), FromNFC: rules.FromNFC,
			Assign: assign.MayAssign(state, rules, candidate), Unassign: assign.MayUnassign(state, rules),
		}
		shapeOf(p.Tools, id, &view)
		out = append(out, view)
	}

	answer(w, http.StatusOK, toolsView{printerView: view, Tools: out})
}

// toolRow is one printer tool as another module reads it: what is on it,
// and the spool with what is left of it.
type toolRow struct {
	DeviceID         string   `json:"device_id"`
	ToolID           string   `json:"tool_id"`
	MaterialUUID     string   `json:"material_uuid,omitempty"`
	MaterialName     string   `json:"material_name,omitempty"`
	BrandName        string   `json:"brand_name,omitempty"`
	Type             string   `json:"type,omitempty"`
	Abbreviation     string   `json:"abbreviation,omitempty"`
	ColorRGBA        string   `json:"color_rgba,omitempty"`
	ColorFamily      string   `json:"color_family,omitempty"`
	By               string   `json:"by,omitempty"`
	FromNFC          bool     `json:"from_nfc"`
	SpoolID          *int64   `json:"spool_id,omitempty"`
	SpoolUUID        string   `json:"spool_uuid,omitempty"`
	RemainingWeightG *float64 `json:"remaining_weight_g,omitempty"`
	FullWeightG      *float64 `json:"full_weight_g,omitempty"`
}

// allTools answers the tools of one page of the printers the caller may
// observe, by printer id, one row per tool that holds a material.
func (s *server) allTools(w http.ResponseWriter, r *http.Request, by string) {
	query, err := queryOf(r)
	if err != nil {
		refuse(w, InvalidQuery, err.Error(), nil)

		return
	}

	visible, err := s.visible(r.Context(), by)
	if err != nil {
		undecided(w, r, err)

		return
	}

	seen := slices.DeleteFunc(s.fleet.Printers(), func(p printers.Printer) bool { return !visible[p.ID] })
	slices.SortFunc(seen, func(a, b printers.Printer) int { return strings.Compare(a.ID, b.ID) })

	limit := query.Limit
	if limit <= 0 || limit > stock.MaxPage {
		limit = stock.MaxPage
	}

	from := min(max(query.Offset, 0), len(seen))
	page := seen[from:min(from+limit, len(seen))]

	ids := make([]string, len(page))
	for i, p := range page {
		ids[i] = p.ID
	}

	stored, err := s.store.AssignedSpools(r.Context(), ids)
	if err != nil {
		refused(w, r, err, InvalidQuery)

		return
	}

	on := map[[2]string]stock.AssignedSpool{}
	for _, a := range stored {
		on[[2]string{a.DeviceID, a.ToolID}] = a
	}

	rows := []toolRow{}

	for _, listed := range page {
		// A printer that left since the list has no tools to show.
		if p, ok := s.fleet.Printer(listed.ID); ok {
			rows = rowsOf(p, on, rows)
		}
	}

	out := map[string]any{"tools": rows, "total": len(seen)}
	if next := from + len(page); next < len(seen) {
		out["next"] = next
	}

	answer(w, http.StatusOK, out)
}

// rowsOf appends a row for each tool of p that holds a material, with what
// on holds for it by device and tool id.
func rowsOf(p printers.Printer, on map[[2]string]stock.AssignedSpool, rows []toolRow) []toolRow {
	for _, id := range slices.Sorted(maps.Keys(p.Tools.Tools)) {
		tool := p.Tools.Tools[id]
		if !holdsMaterial(tool) {
			continue
		}

		row := toolRow{DeviceID: p.ID, ToolID: string(id), FromNFC: assign.ToolOf(*tool).FromNFC}

		if a, ok := on[[2]string{p.ID, string(id)}]; ok {
			row.MaterialUUID, row.MaterialName, row.BrandName, row.Type = a.MaterialUUID, a.MaterialName, a.BrandName, a.Type
			row.Abbreviation, row.ColorRGBA, row.By, row.SpoolID = a.Abbreviation, a.ColorRGBA, a.By, a.SpoolID
			row.SpoolUUID, row.RemainingWeightG, row.FullWeightG = a.SpoolUUID, a.RemainingWeightG, a.FullWeightG
			// A module asking for "any black PLA" matches on the family, which the stored colour
			// already decides, so it is worked out here and kept nowhere.
			row.ColorFamily = colour.Family(a.ColorRGBA)
		}

		rows = append(rows, row)
	}

	return rows
}

func reportedOf(t connectorapi.Tool) *reportedView {
	if t.Material == nil {
		return nil
	}

	view := &reportedView{Abbreviation: *t.Material}
	if r := t.Reported; r != nil {
		view.InstanceUUID, view.PackageUUID, view.MaterialUUID = r.InstanceUUID, r.PackageUUID, r.MaterialUUID
		view.Type, view.ColorRGBA, view.FullWeightG = r.Type, r.ColorRGBA, stock.FullWeight(*r)
		view.ConsumedWeightG = t.ConsumedWeight
	}

	return view
}

// holdsMaterial is true for a tool that takes a material: one the printer
// has switched on, and not a head with a feeder. Each slot of a feeder holds
// its own material, and the head holds none.
func holdsMaterial(t *connectorapi.Tool) bool {
	return t != nil && !t.Disabled && !t.HasFeeder()
}

// toolHandler is a route on one tool the caller may operate.
type toolHandler func(w http.ResponseWriter, r *http.Request, by string, p printers.Printer, tool connectorapi.Tool)

// noPrinter answers a printer the follower does not hold, or one the caller
// may not observe, in the same words.
func noPrinter(w http.ResponseWriter, device string) {
	refuse(w, NotFound, "there is no printer "+device, map[string]any{"device_id": device})
}

// operating refuses a printer the follower does not hold or the caller may
// not observe as one that is not there, then a caller who may not operate it,
// then a tool the printer does not have or that holds no material. The host
// is asked only about a printer the follower holds.
//
// gone, when set, takes a tool the printer no longer has in place of the
// refusal.
func (s *server) operating(next toolHandler, gone handler) handler {
	return func(w http.ResponseWriter, r *http.Request, by string) {
		device, id := r.PathValue("device"), r.PathValue("tool")

		p, ok := s.fleet.Printer(device)
		if !ok {
			noPrinter(w, device)

			return
		}

		observe, operate, err := s.rights(r.Context(), by, device)

		switch {
		case err != nil:
			undecided(w, r, err)

			return
		case !observe:
			noPrinter(w, device)

			return
		case !operate:
			refuse(w, NotOperator, "changing the material on a tool takes operate on the printer",
				map[string]any{"device_id": device})

			return
		}

		if tool := p.Tools.Tools[connectorapi.ToolID(id)]; holdsMaterial(tool) {
			next(w, r, by, p, *tool)

			return
		}

		if gone != nil {
			gone(w, r, by)

			return
		}

		refuse(w, NotFound, fmt.Sprintf("there is no tool %s on printer %s", id, device),
			map[string]any{"device_id": device, "tool_id": id})
	}
}

// refusalError is a change the rules refuse, with their reason.
type refusalError assign.Reason

func (t refusalError) Error() string { return "the rules refuse the change: " + string(t) }

// refuseTool answers a change the rules refuse. The state lets a page say
// which state refused a change for busy, which covers states the rules do not trust.
func refuseTool(w http.ResponseWriter, p printers.Printer, reason assign.Reason) {
	refuse(w, ToolRefused, "the tool cannot be changed now: "+string(reason),
		map[string]any{"reason": string(reason), "state": p.State})
}

// assignRequest names a material or a spool, one of the two.
type assignRequest struct {
	MaterialUUID string `json:"material_uuid,omitempty"`
	SpoolID      *int64 `json:"spool_id,omitempty"`
}

// assign puts a material or a spool on a tool, once the rules allow it.
func (s *server) assign(w http.ResponseWriter, r *http.Request, by string, p printers.Printer, tool connectorapi.Tool) {
	var body assignRequest
	if !decoded(w, r, &body, InvalidQuery) {
		return
	}

	if (strings.TrimSpace(body.MaterialUUID) == "") == (body.SpoolID == nil) {
		refuse(w, InvalidQuery, "the body names a material_uuid or a spool_id, and only one of the two", nil)

		return
	}

	state, rules := assign.StateOf(p.Connected, p.State), assign.ToolOf(tool)

	a, err := s.store.AssignIf(r.Context(), by, stock.Assignment{
		DeviceID: p.ID, ToolID: string(tool.ID), MaterialUUID: body.MaterialUUID, SpoolID: body.SpoolID,
		By: stock.ByPerson,
	}, func(a stock.Assignment) error {
		if reason := assign.MayAssign(
			state,
			rules,
			assign.Candidate{Abbreviation: a.Abbreviation, Type: a.Type},
		); reason != assign.Allowed {
			return refusalError(reason)
		}

		return nil
	})
	if reason, ok := errors.AsType[refusalError](err); ok {
		refuseTool(w, p, assign.Reason(reason))

		return
	}

	if err != nil {
		refused(w, r, err, InvalidQuery)

		return
	}

	answer(w, http.StatusOK, a)
}

// unassign takes the person's assignment off a tool.
func (s *server) unassign(
	w http.ResponseWriter,
	r *http.Request,
	_ string,
	p printers.Printer,
	tool connectorapi.Tool,
) {
	if reason := assign.MayUnassign(
		assign.StateOf(p.Connected, p.State),
		assign.ToolOf(tool),
	); reason != assign.Allowed {
		refuseTool(w, p, reason)

		return
	}

	if err := s.clear(r.Context(), p.ID, tool); err != nil {
		refused(w, r, err, InvalidQuery)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unassignGone clears a tool the printer no longer has, such as a toolhead
// taken off an XL. Nothing reports on such a tool, so no rule applies, and
// operating checked that the person may operate the printer. A printer that
// left the host is not there: the follower clears the tools of a removed one.
func (s *server) unassignGone(w http.ResponseWriter, r *http.Request, _ string) {
	device, id := r.PathValue("device"), r.PathValue("tool")

	stored, err := s.store.Assignments(r.Context(), device)
	if err != nil {
		refused(w, r, err, InvalidQuery)

		return
	}

	if !slices.ContainsFunc(stored, func(a stock.Assignment) bool { return a.ToolID == id }) {
		refuse(w, NotFound, fmt.Sprintf("there is no tool %s on printer %s", id, device),
			map[string]any{"device_id": device, "tool_id": id})

		return
	}

	if err = s.store.Unassign(r.Context(), device, id); err != nil {
		refused(w, r, err, InvalidQuery)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// placed marks the tool of each spool that the printer no longer has. A
// person may clear such a tool with an unassign. A printer by may not
// observe is not named, as the host's device list leaves it out: the spool
// is on a printer, and the device id is blank. A printer that left the host
// and a question the host could not answer leave it blank too.
func (s *server) placed(ctx context.Context, by string, spools []stock.Spool) {
	for i := range spools {
		on := spools[i].Tool
		if on == nil {
			continue
		}

		p, ok := s.fleet.Printer(on.DeviceID)

		// Before the printers are listed, a printer the follower holds no
		// entry for may still be one the caller must not see named.
		if !ok && !s.fleet.Listed() {
			if observe, _, err := s.rights(ctx, by, on.DeviceID); err != nil || !observe {
				on.DeviceID = ""
			}

			continue
		}

		if !ok {
			on.DeviceID = ""

			continue
		}

		if observe, _, err := s.rights(ctx, by, on.DeviceID); err != nil || !observe {
			on.DeviceID = ""

			continue
		}

		on.Gone = !holdsMaterial(p.Tools.Tools[connectorapi.ToolID(on.ToolID)])
	}
}

// addReported adds the spool the printer read off its NFC tag to stock, and
// puts it on the tool as the next report would. A tag a spool carries
// already answers that spool, so a second click adds nothing.
func (s *server) addReported(w http.ResponseWriter, r *http.Request, by string, p printers.Printer,
	tool connectorapi.Tool,
) {
	reported := tool.Reported
	if tool.Material == nil || reported == nil || reported.InstanceUUID == "" {
		refuse(w, NotReported, fmt.Sprintf("printer %s reports no tagged spool on tool %s", p.ID, tool.ID),
			map[string]any{"device_id": p.ID, "tool_id": string(tool.ID)})

		return
	}

	spool, err := s.store.AddReported(r.Context(), by, stock.ReportedSpool{
		UUID: reported.InstanceUUID, MaterialUUID: reported.MaterialUUID, PackageUUID: reported.PackageUUID,
		Abbreviation: *tool.Material,
		Type:         reported.Type, ColorRGBA: reported.ColorRGBA, FullWeightG: stock.FullWeight(*reported),
		ConsumedG: tool.ConsumedWeight,
	}, s.live.Get().MaxSpools)

	written := false
	if err == nil {
		written, err = s.putReported(r.Context(), p.ID, tool, &spool.ID)
	}

	if err != nil {
		refused(w, r, err, SpoolInvalid)

		return
	}

	// A tool that changed since it was read keeps what it holds, and the next report is merged over it.
	if written {
		spool.Tool = &stock.ToolRef{DeviceID: p.ID, ToolID: string(tool.ID)}
	}

	answer(w, http.StatusOK, spool)
}

// clear takes the person's assignment off a tool. A tool that reports a
// material keeps what the printer reports, with the spool its tag names.
// Only a tool that reports nothing is left
// empty.
func (s *server) clear(ctx context.Context, deviceID string, tool connectorapi.Tool) error {
	if tool.Material == nil || *tool.Material == connectorapi.NoMaterial {
		return s.store.Unassign(ctx, deviceID, string(tool.ID))
	}

	var spoolID *int64

	if tool.Reported != nil && tool.Reported.InstanceUUID != "" {
		id, known, err := s.store.SpoolByUUID(ctx, tool.Reported.InstanceUUID)
		if err != nil {
			return err
		}

		if known {
			spoolID = &id
		}
	}

	_, err := s.putReported(ctx, deviceID, tool, spoolID)

	return err
}

// putReported puts what the printer reports on the tool, as the follower
// does, with the spool the tag names or none. It holds the tool to what it
// read, so a change in between stands.
func (s *server) putReported(ctx context.Context, deviceID string, tool connectorapi.Tool, spoolID *int64,
) (bool, error) {
	reported := connectorapi.ReportedMaterial{}
	if tool.Reported != nil {
		reported = *tool.Reported
	}

	report, err := stock.EncodeReport(reported)
	if err != nil {
		return false, err
	}

	stored, err := s.store.Assignments(ctx, deviceID)
	if err != nil {
		return false, err
	}

	var read *time.Time

	for i := range stored {
		if stored[i].ToolID == string(tool.ID) {
			read = &stored[i].UpdatedAt
		}
	}

	return s.store.Report(ctx, stock.Assignment{
		DeviceID: deviceID, ToolID: string(tool.ID), SpoolID: spoolID, Abbreviation: *tool.Material,
		Type: reported.Type, MaterialUUID: reported.MaterialUUID, ColorRGBA: reported.ColorRGBA,
		Reported: report,
	}, read)
}
