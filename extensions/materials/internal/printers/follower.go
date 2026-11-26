// Package printers follows what the printers report of their tools, and
// applies it to the assignments through the merge rules.
package printers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/busapi"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/event"

	"github.com/prusa3d/materials/internal/assign"
	"github.com/prusa3d/materials/internal/stock"
)

// statePath is where a device keeps the value of its state machine.
const statePath = "/state"

// The wait before the follower subscribes again after the stream failed. It
// doubles after each failure up to the ceiling.
const (
	firstRetry = time.Second
	lastRetry  = 30 * time.Second
)

// Devices is the part of the host's device door the follower reads.
type Devices interface {
	List(ctx context.Context, params deviceapi.ListParams) (deviceapi.Page, error)
	Get(ctx context.Context, params deviceapi.GetParams) (deviceapi.Device, error)
}

// Subscribe opens the host's event stream, as busapi.Subscribe does on the
// session.
type Subscribe func(ctx context.Context, params busapi.SubscribeParams, onWindow func(busapi.Window) error) error

// Printer is one printer as the follower last read it.
type Printer struct {
	// Model is the model the printer names itself, such as MK4S, empty when unknown.
	ID, Name, Model string
	Connected       bool
	// State is the value of the printer's state machine, such as IDLE.
	State string
	Tools connectorapi.ToolTree
}

// Follower reads every printer's state tree from the host, keeps it current
// from the event stream, and applies each tool's report to its assignment.
type Follower struct {
	devices   Devices
	subscribe Subscribe
	store     *stock.Store
	log       *slog.Logger

	mu       sync.Mutex
	printers map[string]deviceapi.Device
	// listed is true once a listing of the printers was read.
	listed bool

	// cursor is where the stream resumes. Only Run touches it.
	cursor string
}

// New returns a follower that has read nothing yet.
func New(devices Devices, subscribe Subscribe, store *stock.Store, log *slog.Logger) *Follower {
	return &Follower{
		devices: devices, subscribe: subscribe, store: store, log: log,
		printers: map[string]deviceapi.Device{},
	}
}

// Printer answers the printer with that id as last read, and ok false for
// one the follower does not hold.
func (f *Follower) Printer(id string) (Printer, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	d, ok := f.printers[id]
	if !ok {
		return Printer{}, false
	}

	return printerOf(d), true
}

// Listed is true once the follower has listed the printers. A printer it
// does not hold then has left the host.
func (f *Follower) Listed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.listed
}

// Printers answers every printer the follower holds, by id, without its
// tools: reading every printer's tree for a list is what Printer is for, one
// printer at a time.
func (f *Follower) Printers() []Printer {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]Printer, 0, len(f.printers))
	for _, id := range slices.Sorted(maps.Keys(f.printers)) {
		out = append(out, headerOf(f.printers[id]))
	}

	return out
}

// headerOf reads a printer out of a device, without its tools.
func headerOf(d deviceapi.Device) Printer {
	var state string
	_ = json.Unmarshal(d.State[statePath], &state)

	return Printer{ID: d.ID, Name: d.Name, Model: d.Model, Connected: d.Connected, State: state}
}

// printerOf reads a printer out of a device. ReadTools builds a new tree, so
// the caller holds nothing the follower changes.
func printerOf(d deviceapi.Device) Printer {
	p := headerOf(d)
	p.Tools = connectorapi.ReadTools(d.State)

	return p
}

// Run lists the printers, then applies the event stream until ctx ends. A
// listing that cannot be read or a stream that fails is retried after a
// wait, so the follower ends only with ctx.
//
// The host sends no snapshot when a subscription opens, and never replays
// telemetry. A change between the listing and the first window would be
// lost, so the first window of each subscription lists the printers again.
func (f *Follower) Run(ctx context.Context) error {
	params := busapi.SubscribeParams{Types: []string{
		event.TypeDeviceTelemetrySampled, event.TypeDevicePresenceChanged, event.TypeDeviceRecordChanged,
	}}

	for wait := firstRetry; ; {
		read := false

		err := f.reload(ctx)
		if err == nil {
			params.Resume = f.cursor
			err = f.subscribe(ctx, params, func(w busapi.Window) error {
				w.Resync = w.Resync || !read
				read = true

				return f.window(ctx, w)
			})
		}

		if read {
			wait = firstRetry
		}

		select {
		case <-ctx.Done():
			return nil
		default:
			f.log.WarnContext(ctx, "the printers could not be followed, trying again", "error", err, "wait", wait)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}

		wait = min(wait+wait, lastRetry)
	}
}

// reload reads every printer again and applies the tools of each.
func (f *Follower) reload(ctx context.Context) error {
	listed := map[string]deviceapi.Device{}

	for offset := 0; ; {
		page, err := f.devices.List(ctx, deviceapi.ListParams{Limit: deviceapi.MaxPageLimit, Offset: offset})
		if err != nil {
			return fmt.Errorf("list the printers: %w", err)
		}

		for _, d := range page.Items {
			listed[d.ID] = d
		}

		offset += len(page.Items)
		if len(page.Items) == 0 || offset >= page.Total {
			break
		}
	}

	f.mu.Lock()
	f.printers, f.listed = listed, true
	f.mu.Unlock()

	for _, id := range slices.Sorted(maps.Keys(listed)) {
		if err := f.apply(ctx, id, nil); err != nil {
			return err
		}
	}

	return f.forgetRetired(ctx, listed)
}

// forgetRetired takes the assignments off the printers retired while the
// stream was not read. List leaves a retired printer out, so each printer
// that holds an assignment and is not listed is read on its own. A printer
// the host does not answer for keeps its assignments: the module may only
// have lost sight of it.
func (f *Follower) forgetRetired(ctx context.Context, listed map[string]deviceapi.Device) error {
	held, err := f.store.AssignedDevices(ctx)
	if err != nil {
		return err
	}

	for _, id := range held {
		if _, ok := listed[id]; ok {
			continue
		}

		d, getErr := f.devices.Get(ctx, deviceapi.GetParams{ID: id})
		if errors.Is(getErr, deviceapi.ErrNoDevice) {
			continue
		}

		if getErr != nil {
			return fmt.Errorf("read printer %s: %w", id, getErr)
		}

		if d.Retired {
			if err = f.forget(ctx, id); err != nil {
				return err
			}
		}
	}

	return nil
}

// forget takes the assignments off a retired printer.
func (f *Follower) forget(ctx context.Context, id string) error {
	if err := f.store.Forget(ctx, id); err != nil {
		return fmt.Errorf("clear the tools of retired printer %s: %w", id, err)
	}

	f.log.InfoContext(ctx, "a retired printer's tools were cleared", "printer", id)

	return nil
}

// window applies one window of the stream. Telemetry changes one path of a
// tree. Any other event reads its printer again, which covers its presence
// and its name.
func (f *Follower) window(ctx context.Context, w busapi.Window) error {
	if w.Resync {
		if err := f.reload(ctx); err != nil {
			return err
		}
	}

	changed, gone := f.samples(w.Telemetry)

	for _, raw := range w.Events {
		id, ok := deviceOf(raw)
		if !ok {
			continue
		}

		d, err := f.devices.Get(ctx, deviceapi.GetParams{ID: id})
		if errors.Is(err, deviceapi.ErrNoDevice) {
			f.mu.Lock()
			delete(f.printers, id)
			f.mu.Unlock()

			continue
		}

		if err != nil {
			return fmt.Errorf("read printer %s: %w", id, err)
		}

		if d.Retired {
			f.mu.Lock()
			delete(f.printers, id)
			f.mu.Unlock()

			if err = f.forget(ctx, id); err != nil {
				return err
			}

			continue
		}

		f.mu.Lock()
		f.printers[id] = d
		f.mu.Unlock()

		changed[id] = true
	}

	for _, id := range slices.Sorted(maps.Keys(changed)) {
		if err := f.apply(ctx, id, gone[id]); err != nil {
			return err
		}
	}

	f.cursor = w.Cursor

	return nil
}

// samples writes the telemetry of one window into the printers' trees. A
// null value removes its path. It answers the printers a tool path of
// changed, and the tools each of them lost: tools in the tree before the
// window and out of it after. Only such an explicit removal takes a tool
// away. A tree read whole may still be filling after a reconnect, and a tool
// missing from it has not left.
func (f *Follower) samples(telemetry []json.RawMessage) (map[string]bool, map[string][]connectorapi.ToolID) {
	changed := map[string]bool{}
	before := map[string]connectorapi.ToolTree{}

	f.mu.Lock()
	defer f.mu.Unlock()

	for _, raw := range telemetry {
		id, path, value, ok := sampleOf(raw)
		d, held := f.printers[id]

		// A heater or fan reading changes nothing apply reads, and arrives
		// every few seconds per tool: it is kept without a write.
		if _, member, tool := connectorapi.SplitToolPath(path); !ok || !held || !tool || unread[member] {
			if ok && held {
				write(&d, path, value)
				f.printers[id] = d
			}

			continue
		}

		if !changed[id] {
			changed[id] = true
			before[id] = connectorapi.ReadTools(d.State)
		}

		write(&d, path, value)
		f.printers[id] = d
	}

	gone := map[string][]connectorapi.ToolID{}

	for id, tree := range before {
		after := connectorapi.ReadTools(f.printers[id].State)
		for tool := range tree.Tools {
			if after.Tools[tool] == nil {
				gone[id] = append(gone[id], tool)
			}
		}
	}

	return changed, gone
}

// unread are the tool members apply never reads: the heater and fan
// readings, which a printer sends every few seconds.
//
//nolint:gochecknoglobals // a fixed set, read-only.
var unread = map[string]bool{
	connectorapi.NozzleTemp: true, connectorapi.NozzleTargetTemp: true,
	connectorapi.PrintFanRPM: true, connectorapi.HeatbreakFanRPM: true,
}

// sampleOf reads one telemetry envelope: its printer, the path and the value.
func sampleOf(raw json.RawMessage) (id, path string, value json.RawMessage, ok bool) {
	var (
		envelope event.Envelope
		reading  struct {
			Path  string          `json:"path"`
			Value json.RawMessage `json:"value"`
		}
	)

	if json.Unmarshal(raw, &envelope) != nil || envelope.Type != event.TypeDeviceTelemetrySampled ||
		json.Unmarshal(envelope.Data, &reading) != nil || reading.Path == "" {
		return "", "", nil, false
	}

	id, ok = deviceOf(raw)

	return id, reading.Path, reading.Value, ok
}

// write sets one path of a printer's tree, and removes it for a null value.
func write(d *deviceapi.Device, path string, value json.RawMessage) {
	if d.State == nil {
		d.State = map[string]json.RawMessage{}
	}

	if len(value) == 0 || string(value) == "null" {
		delete(d.State, path)
	} else {
		d.State[path] = value
	}
}

// deviceOf reads the device id out of an envelope's subject, device:<id>.
func deviceOf(raw json.RawMessage) (string, bool) {
	var envelope event.Envelope
	if json.Unmarshal(raw, &envelope) != nil {
		return "", false
	}

	origin, err := event.ParseOrigin(envelope.Subject)
	if err != nil || origin.Kind() != event.KindDevice {
		return "", false
	}

	return origin.ID(), true
}

// apply puts what one printer reports of its tools onto their assignments.
// The assignment of each tool in gone goes, since the stream removed it.
func (f *Follower) apply(ctx context.Context, deviceID string, gone []connectorapi.ToolID) error {
	f.mu.Lock()
	d, held := f.printers[deviceID]
	tree := connectorapi.ReadTools(d.State)
	f.mu.Unlock()

	if !held {
		return nil
	}

	stored, err := f.store.Assignments(ctx, deviceID)
	if err != nil {
		return err
	}

	on := map[connectorapi.ToolID]*stock.Assignment{}
	for i := range stored {
		on[connectorapi.ToolID(stored[i].ToolID)] = &stored[i]
	}

	for _, id := range gone {
		if a := on[id]; a != nil && tree.Tools[id] == nil {
			if err = f.store.Drop(ctx, *a); err != nil {
				return err
			}
		}
	}

	// What a tool fed is charged before its report is merged, so a window
	// that unloads one spool and reads the next charges the one unloaded.
	for _, id := range slices.Sorted(maps.Keys(tree.Tools)) {
		if err = f.consume(ctx, deviceID, tree.Tools[id]); err != nil {
			return err
		}
	}

	for _, id := range slices.Sorted(maps.Keys(tree.Tools)) {
		if err = f.merge(ctx, deviceID, *tree.Tools[id], on[id]); err != nil {
			return err
		}
	}

	return nil
}

// consume takes what a tool fed off the spool on it. A head with a feeder
// holds no material, its slots do, and its odometer is never put on a slot:
// Buddy reports one only from the slots.
func (f *Follower) consume(ctx context.Context, deviceID string, tool *connectorapi.Tool) error {
	if tool.HasFeeder() || (tool.OdometerE == nil && tool.ConsumedWeight == nil) {
		return nil
	}

	r := stock.Reading{E: tool.OdometerE, ID: tool.OdometerID, TagConsumedG: tool.ConsumedWeight}
	if tool.Reported != nil {
		r.TagUUID = tool.Reported.InstanceUUID
	}

	return f.store.Consume(ctx, deviceID, string(tool.ID), r)
}

// merge applies the report of one tool to the assignment stored on it, nil
// for none, through assign.Merge.
func (f *Follower) merge(ctx context.Context, deviceID string, tool connectorapi.Tool, stored *stock.Assignment) error {
	if tool.Material == nil {
		return nil
	}

	reported := connectorapi.ReportedMaterial{}
	if tool.Reported != nil {
		reported = *tool.Reported
	}

	report, err := stock.EncodeReport(reported)
	if err != nil {
		return err
	}

	var (
		spoolID int64
		known   bool
	)

	if reported.InstanceUUID != "" {
		if spoolID, known, err = f.store.SpoolByUUID(ctx, reported.InstanceUUID); err != nil {
			return err
		}
	}

	var tagged *int64
	if known {
		tagged = &spoolID
	}

	// A spool added at 0 g before its printer reported a weight takes the
	// weight the report now carries. The report changes when it does.
	if known && (stored == nil || !bytes.Equal(report, stored.Reported)) {
		if err = f.store.WeighTagged(ctx, spoolID, stock.ReportedSpool{
			MaterialUUID: reported.MaterialUUID, PackageUUID: reported.PackageUUID,
			FullWeightG: stock.FullWeight(reported), ConsumedG: tool.ConsumedWeight,
		}); err != nil {
			return err
		}
	}

	// Every write below holds the tool to what was read here. A person's
	// assign that lands in between stands, and the next report is merged
	// over it.
	outcome := assign.Merge(stored, tool, tagged)

	switch {
	case outcome.Drop:
		if stored == nil {
			return nil
		}

		return f.store.Drop(ctx, *stored)
	case outcome.Replace:
		return f.replace(ctx, deviceID, tool, reported, report, spoolID, known, stored)
	}

	kept := *stored
	fill(&kept.Abbreviation, *tool.Material)
	fill(&kept.Type, reported.Type)
	fill(&kept.MaterialUUID, reported.MaterialUUID)
	fill(&kept.ColorRGBA, reported.ColorRGBA)
	kept.Reported = report

	if kept.Abbreviation == stored.Abbreviation && kept.Type == stored.Type &&
		kept.MaterialUUID == stored.MaterialUUID && kept.ColorRGBA == stored.ColorRGBA &&
		bytes.Equal(kept.Reported, stored.Reported) {
		return nil
	}

	return f.store.Refresh(ctx, kept)
}

// replace puts the printer's material on the tool. A spool the report names
// by its uuid goes with it, and leaves the tool it was on: an NFC report
// moves a spool. The material is then the spool's record, which
// describes the same spool, and not the report. A tag no spool carries is
// shown and never added.
func (f *Follower) replace(ctx context.Context, deviceID string, tool connectorapi.Tool,
	reported connectorapi.ReportedMaterial, report json.RawMessage, spoolID int64, known bool,
	stored *stock.Assignment,
) error {
	a := stock.Assignment{
		DeviceID: deviceID, ToolID: string(tool.ID),
		Abbreviation: *tool.Material, Type: reported.Type, MaterialUUID: reported.MaterialUUID,
		ColorRGBA: reported.ColorRGBA, Reported: report,
	}

	if known {
		a.SpoolID = &spoolID
	}

	var read *time.Time
	if stored != nil {
		read = &stored.UpdatedAt
	}

	_, err := f.store.Report(ctx, a, read)

	return err
}

// fill sets a value the assignment lacks from the report.
func fill(field *string, reported string) {
	if *field == "" {
		*field = reported
	}
}
