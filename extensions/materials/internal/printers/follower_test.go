package printers

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/prusa3d/prusa-connect/sdk/busapi"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/event"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/materials/internal/dbtest"
	"github.com/prusa3d/materials/internal/stock"
)

// fleet is the host's device door, held in memory.
type fleet struct {
	mu      sync.Mutex
	devices map[string]deviceapi.Device
}

func (f *fleet) set(d deviceapi.Device) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.devices[d.ID] = d
}

func (f *fleet) List(_ context.Context, p deviceapi.ListParams) (deviceapi.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	page := deviceapi.Page{Limit: p.Limit, Offset: p.Offset}
	for _, d := range f.devices {
		// The host lists no retired device, and Get still answers one.
		if d.Retired {
			continue
		}

		d.State = maps.Clone(d.State)
		page.Items = append(page.Items, d)
	}

	page.Total = len(page.Items)

	return page, nil
}

func (f *fleet) Get(_ context.Context, p deviceapi.GetParams) (deviceapi.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	d, ok := f.devices[p.ID]
	if !ok {
		return deviceapi.Device{}, deviceapi.ErrNoDevice
	}

	d.State = maps.Clone(d.State)

	return d, nil
}

// bus hands the follower one window at a time. send returns once the
// follower has applied it.
type bus struct {
	windows chan busapi.Window
	applied chan error
	// opened runs when a subscription opens, before its first window.
	opened func()
}

func (b *bus) subscribe(ctx context.Context, _ busapi.SubscribeParams, onWindow func(busapi.Window) error) error {
	if b.opened != nil {
		b.opened()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case w := <-b.windows:
			err := onWindow(w)
			b.applied <- err

			if err != nil {
				return err
			}
		}
	}
}

func (b *bus) send(t *testing.T, w busapi.Window) {
	t.Helper()

	// A follower that cannot apply what it read retries and never reads the
	// window, so the test fails rather than waits for ever.
	select {
	case b.windows <- w:
	case <-time.After(10 * time.Second):
		t.Fatal("the follower did not read the window")
	}

	if err := <-b.applied; err != nil {
		t.Fatal(err)
	}
}

type world struct {
	fleet    *fleet
	bus      *bus
	store    *stock.Store
	follower *Follower
	ctx      context.Context
}

// follow starts a follower over the devices. It returns once the follower
// has read them all and is reading the bus.
func follow(t *testing.T, devices ...deviceapi.Device) *world {
	t.Helper()

	db, _ := dbtest.SQLite(t)
	w := &world{
		fleet: &fleet{devices: map[string]deviceapi.Device{}},
		bus:   &bus{windows: make(chan busapi.Window), applied: make(chan error)},
		store: stock.New(db, wirerpc.DatabaseSQLite),
		ctx:   context.Background(),
	}

	for _, d := range devices {
		w.fleet.set(d)
	}

	w.follower = New(w.fleet, w.bus.subscribe, w.store, slog.New(slog.DiscardHandler))

	return w
}

// start runs the follower until the test ends.
func (w *world) start(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithCancel(w.ctx)
	done := make(chan error, 1)

	go func() { done <- w.follower.Run(ctx) }()

	t.Cleanup(func() {
		cancel()

		if err := <-done; err != nil {
			t.Error(err)
		}
	})

	// The first window is read once the listing is applied.
	w.bus.send(t, busapi.Window{})
}

func (w *world) spool(t *testing.T) stock.Spool {
	t.Helper()

	m, err := w.store.CreateCustom(w.ctx, "alice", stock.MaterialFields{
		Name: "Galaxy", Class: "FFF", Type: "PLA", Abbreviation: "PLA",
	})
	if err != nil {
		t.Fatal(err)
	}

	// A spool a printer read off its tag, as only such a spool's uuid is one
	// a printer reports.
	full := 500.0

	sp, err := w.store.AddReported(w.ctx, "alice", stock.ReportedSpool{
		UUID: uuid.NewString(), MaterialUUID: m.UUID, Abbreviation: "PLA", FullWeightG: &full,
	}, 1000)
	if err != nil {
		t.Fatal(err)
	}

	return sp
}

func (w *world) assignments(t *testing.T, device string) map[string]stock.Assignment {
	t.Helper()

	list, err := w.store.Assignments(w.ctx, device)
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]stock.Assignment{}
	for _, a := range list {
		out[a.ToolID] = a
	}

	return out
}

// tool is what a printer reports of one tool: its abbreviation, and the uuid
// of the instance when it read a tag. A nil material is a tool that reports
// none.
type tool struct {
	material *string
	instance string
}

func pla(instance string) tool {
	m := "PLA"

	return tool{material: &m, instance: instance}
}

func printer(id string, tools map[connectorapi.ToolID]tool) deviceapi.Device {
	state := map[string]json.RawMessage{"/state": json.RawMessage(`"IDLE"`)}

	for id, t := range tools {
		state[connectorapi.ToolPath(id, connectorapi.ToolEnabled)] = json.RawMessage(`true`)
		if t.material != nil {
			state[connectorapi.ToolPath(id, connectorapi.MaterialAbbreviation)] = encode(*t.material)
		}

		if t.instance != "" {
			state[connectorapi.ToolPath(id, connectorapi.MaterialInstanceUUID)] = encode(t.instance)
		}
	}

	return deviceapi.Device{Kind: "printer", ID: id, Name: id, Connected: true, State: state}
}

func encode(v any) json.RawMessage {
	out, _ := json.Marshal(v)

	return out
}

// sample is one telemetry envelope: the value at one path of p1's tree.
func sample(path string, value any) json.RawMessage {
	data := encode(map[string]any{"path": path, "value": value})

	return encode(event.Envelope{
		Type: event.TypeDeviceTelemetrySampled, Subject: "device:p1", Class: event.ClassTelemetry, Data: data,
	})
}

func TestFollowerAppliesAReportedNFCSpool(t *testing.T) {
	w := follow(t)
	read, assigned := w.spool(t), w.spool(t)

	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: "p2", ToolID: "0", SpoolID: &assigned.ID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	w.fleet.set(printer("p1", map[connectorapi.ToolID]tool{"0": pla(read.UUID)}))
	w.fleet.set(printer("p2", map[connectorapi.ToolID]tool{"0": pla(assigned.UUID)}))
	w.start(t)

	got := w.assignments(t, "p1")["0"]
	if got.By != stock.ByPrinter || got.SpoolID == nil || *got.SpoolID != read.ID || got.Abbreviation != "PLA" {
		t.Fatalf("the reported spool was not applied: %+v", got)
	}

	// The tag of a spool a person put on the tool is that spool: the
	// assignment stays theirs and keeps the report.
	kept := w.assignments(t, "p2")["0"]
	if kept.By != stock.ByPerson || kept.SpoolID == nil || *kept.SpoolID != assigned.ID ||
		!json.Valid(kept.Reported) || !containsUUID(kept.Reported, assigned.UUID) {
		t.Fatalf("the person's spool was not kept: %+v", kept)
	}

	p, ok := w.follower.Printer("p1")
	if !ok || p.State != "IDLE" || !p.Connected || p.Tools.Tool("0") == nil || len(w.follower.Printers()) != 2 {
		t.Fatalf("%+v %v", p, ok)
	}
}

func containsUUID(reported json.RawMessage, uuid string) bool {
	r, err := stock.DecodeReport(reported)

	return err == nil && r.InstanceUUID == uuid
}

func TestFollowerReloadsOnResync(t *testing.T) {
	w := follow(t, printer("p1", map[connectorapi.ToolID]tool{"0": pla("")}))
	w.start(t)

	petg := "PETG"
	w.fleet.set(printer("p1", map[connectorapi.ToolID]tool{"0": {material: &petg}}))

	if got := w.assignments(t, "p1")["0"]; got.Abbreviation != "PLA" {
		t.Fatalf("%+v", got)
	}

	w.bus.send(t, busapi.Window{Resync: true})

	if got := w.assignments(t, "p1")["0"]; got.Abbreviation != "PETG" || got.By != stock.ByPrinter {
		t.Fatalf("the resync was not read: %+v", got)
	}
}

func TestAssignmentsOfVanishedToolsGo(t *testing.T) {
	mmu := printer("p1", map[connectorapi.ToolID]tool{"0": {}, "0.0": pla(""), "0.1": pla("")})
	// A printer that reports no tools, as an MK3, keeps what a person set.
	mk3 := printer("p2", nil)
	w := follow(t, mmu, mk3)

	if _, err := w.store.Assign(w.ctx, "", stock.Assignment{
		DeviceID: "p2", ToolID: "0", Abbreviation: "PLA", By: stock.ByPrinter,
	}); err != nil {
		t.Fatal(err)
	}

	w.start(t)

	if got := w.assignments(t, "p1"); len(got) != 2 {
		t.Fatalf("%+v", got)
	}

	// The MMU loses its second slot.
	w.bus.send(t, busapi.Window{Telemetry: []json.RawMessage{
		sample(connectorapi.ToolPath("0.1", connectorapi.ToolEnabled), nil),
		sample(connectorapi.ToolPath("0.1", connectorapi.MaterialAbbreviation), nil),
	}})

	if got := w.assignments(t, "p1"); len(got) != 1 || got["0.0"].Abbreviation != "PLA" {
		t.Fatalf("the vanished slot kept its assignment: %+v", got)
	}

	// A printer that goes offline has not lost its tools.
	offline := printer("p1", map[connectorapi.ToolID]tool{"0": {}, "0.0": pla("")})
	offline.Connected = false
	w.fleet.set(offline)
	w.bus.send(t, busapi.Window{Events: []json.RawMessage{encode(event.Envelope{
		Type: event.TypeDevicePresenceChanged, Subject: "device:p1", Class: event.ClassState,
	})}})

	if got := w.assignments(t, "p1"); len(got) != 1 {
		t.Fatalf("an offline printer lost its assignments: %+v", got)
	}

	if p, _ := w.follower.Printer("p1"); p.Connected {
		t.Fatal("the presence change was not read")
	}

	if got := w.assignments(t, "p2"); len(got) != 1 {
		t.Fatalf("a printer reporting no tools lost its assignment: %+v", got)
	}
}

func TestAnUnknownNFCSpoolIsShownNotAdded(t *testing.T) {
	w := follow(t, printer("p1", map[connectorapi.ToolID]tool{"0": pla("tag-nobody-added")}))
	w.start(t)

	got := w.assignments(t, "p1")["0"]
	if got.By != stock.ByPrinter || got.SpoolID != nil || !containsUUID(got.Reported, "tag-nobody-added") {
		t.Fatalf("%+v", got)
	}

	if _, ok, err := w.store.SpoolByUUID(w.ctx, "tag-nobody-added"); err != nil || ok {
		t.Fatalf("a spool was added: %v %v", ok, err)
	}
}

func TestAnNFCReportMovesTheSpool(t *testing.T) {
	w := follow(t)
	s := w.spool(t)

	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: "p1", ToolID: "0", SpoolID: &s.ID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	w.fleet.set(printer("p1", map[connectorapi.ToolID]tool{"0": {}}))
	w.fleet.set(printer("p2", map[connectorapi.ToolID]tool{"0": pla(s.UUID)}))
	w.start(t)

	if got := w.assignments(t, "p1"); len(got) != 0 {
		t.Fatalf("the spool stayed on p1: %+v", got)
	}

	if got := w.assignments(t, "p2")["0"]; got.SpoolID == nil || *got.SpoolID != s.ID || got.By != stock.ByPrinter {
		t.Fatalf("%+v", got)
	}
}

func TestTheFirstWindowListsThePrintersAgain(t *testing.T) {
	w := follow(t, printer("p1", map[connectorapi.ToolID]tool{"0": pla("")}))

	// The tool changes after the listing and before the stream reaches the
	// hub. No window carries the change.
	petg := "PETG"
	w.bus.opened = func() { w.fleet.set(printer("p1", map[connectorapi.ToolID]tool{"0": {material: &petg}})) }
	w.start(t)

	if got := w.assignments(t, "p1")["0"]; got.Abbreviation != "PETG" {
		t.Fatalf("the change before the first window was lost: %+v", got)
	}
}

func TestAReconnectingPrinterKeepsItsAssignments(t *testing.T) {
	tools := map[connectorapi.ToolID]tool{"0": pla(""), "1": pla(""), "2": pla("")}
	w := follow(t, printer("p1", tools))

	m, err := w.store.CreateCustom(w.ctx, "alice", stock.MaterialFields{
		Name: "Galaxy", Class: "FFF", Type: "PLA", Abbreviation: "PLA",
	})
	if err != nil {
		t.Fatal(err)
	}

	for id := range tools {
		if _, err = w.store.Assign(w.ctx, "alice", stock.Assignment{
			DeviceID: "p1", ToolID: string(id), MaterialUUID: m.UUID, By: stock.ByPerson,
		}); err != nil {
			t.Fatal(err)
		}
	}

	w.start(t)

	kept := func(when string) {
		t.Helper()

		got := w.assignments(t, "p1")
		if len(got) != 3 {
			t.Fatalf("%s: %+v", when, got)
		}

		for id, a := range got {
			if a.By != stock.ByPerson {
				t.Fatalf("%s: tool %s: %+v", when, id, a)
			}
		}
	}

	kept("at the start")

	// The printer comes back and its tree refills one tool at a time.
	w.fleet.set(printer("p1", map[connectorapi.ToolID]tool{"0": pla("")}))
	w.bus.send(t, busapi.Window{Events: []json.RawMessage{encode(event.Envelope{
		Type: event.TypeDevicePresenceChanged, Subject: "device:p1", Class: event.ClassState,
	})}})
	kept("after the presence change")

	w.bus.send(t, busapi.Window{Resync: true})
	kept("after a resync")

	for _, id := range []connectorapi.ToolID{"1", "2"} {
		w.bus.send(t, busapi.Window{Telemetry: []json.RawMessage{
			sample(connectorapi.ToolPath(id, connectorapi.ToolEnabled), true),
			sample(connectorapi.ToolPath(id, connectorapi.MaterialAbbreviation), "PLA"),
		}})
		kept("as tool " + string(id) + " comes back")
	}
}

// A known tag links its spool to a tool that holds no spool yet: the row a
// printer made from a profile, and a person's bare material alike. The
// spool's record then names the material.
func TestAKnownTagLinksItsSpoolToARowWithNone(t *testing.T) {
	w := follow(t, printer("p1", map[connectorapi.ToolID]tool{"0": pla("")}),
		printer("p2", map[connectorapi.ToolID]tool{"0": pla("")}))
	profile, bare := w.spool(t), w.spool(t)

	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: "p2", ToolID: "0", MaterialUUID: profile.MaterialUUID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	w.start(t)

	if got := w.assignments(t, "p1")["0"]; got.By != stock.ByPrinter || got.SpoolID != nil {
		t.Fatalf("the profile report made %+v", got)
	}

	w.bus.send(t, busapi.Window{Telemetry: []json.RawMessage{
		sample(connectorapi.ToolPath("0", connectorapi.MaterialInstanceUUID), profile.UUID),
	}})

	if got := w.assignments(t, "p1")["0"]; got.SpoolID == nil || *got.SpoolID != profile.ID ||
		got.MaterialName != profile.MaterialName {
		t.Errorf("the tag read after the profile did not link its spool: %+v", got)
	}

	w.fleet.set(printer("p2", map[connectorapi.ToolID]tool{"0": pla(bare.UUID)}))
	w.bus.send(t, busapi.Window{Resync: true})

	if got := w.assignments(t, "p2")["0"]; got.SpoolID == nil || *got.SpoolID != bare.ID {
		t.Errorf("the tag over a person's bare material did not link its spool: %+v", got)
	}
}

// A printer missing from the listing keeps its tools: Core lists every
// printer it stores, so one missing was deleted, and what it held stays until
// a person unassigns it.
func TestAPrinterGoneFromTheListingKeepsItsTools(t *testing.T) {
	w := follow(t, printer("p1", map[connectorapi.ToolID]tool{"0": {}}))
	s := w.spool(t)

	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: "p9", ToolID: "0", MaterialUUID: s.MaterialUUID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	if w.follower.Listed() {
		t.Error("a follower that read nothing says it listed the printers")
	}

	w.start(t)

	if !w.follower.Listed() {
		t.Error("the follower does not say it listed the printers")
	}

	if got := w.assignments(t, "p9"); len(got) != 1 {
		t.Errorf("the printer gone from the listing lost %+v", got)
	}
}

// assignSpool puts a spool on tool 0 of the printer.
func (w *world) assignSpool(t *testing.T, device string) {
	t.Helper()

	s := w.spool(t)
	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: device, ToolID: "0", SpoolID: &s.ID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestARetiredPrinterLetsGoOfItsSpools(t *testing.T) {
	w := follow(t, printer("p1", map[connectorapi.ToolID]tool{"0": {}}))
	w.assignSpool(t, "p1")
	w.start(t)

	if got := w.assignments(t, "p1"); len(got) != 1 {
		t.Fatalf("the listed printer lost %+v", got)
	}

	retired := printer("p1", map[connectorapi.ToolID]tool{"0": {}})
	retired.Retired = true
	w.fleet.set(retired)
	w.bus.send(t, busapi.Window{Events: []json.RawMessage{encode(event.Envelope{
		Type: event.TypeDeviceRecordChanged, Subject: "device:p1", Class: event.ClassState,
	})}})

	if got := w.assignments(t, "p1"); len(got) != 0 {
		t.Errorf("the retired printer kept %+v", got)
	}

	if _, held := w.follower.Printer("p1"); held {
		t.Error("the follower still holds the retired printer")
	}
}

func TestAPrinterRetiredWhileNotFollowedLetsGoAtTheStart(t *testing.T) {
	retired := printer("p9", map[connectorapi.ToolID]tool{"0": {}})
	retired.Retired = true
	w := follow(t, printer("p1", map[connectorapi.ToolID]tool{"0": {}}), retired)
	w.assignSpool(t, "p1")
	w.assignSpool(t, "p9")
	w.start(t)

	if got := w.assignments(t, "p9"); len(got) != 0 {
		t.Errorf("the printer retired before the start kept %+v", got)
	}

	if got := w.assignments(t, "p1"); len(got) != 1 {
		t.Errorf("the listed printer lost %+v", got)
	}
}

// left answers what remains of a spool.
func (w *world) left(t *testing.T, sp stock.Spool) float64 {
	t.Helper()

	page, err := w.store.Spools(w.ctx, sp.MaterialUUID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}

	for _, got := range page.Spools {
		if got.ID == sp.ID {
			return got.RemainingWeightG
		}
	}

	t.Fatalf("spool %d is gone", sp.ID)

	return 0
}

// odometer is the odometer of p1's tool 0 at e, under odometer id 7.
func odometer(e float64) []json.RawMessage {
	return []json.RawMessage{
		sample(connectorapi.ToolPath("0", connectorapi.OdometerID), 7),
		sample(connectorapi.ToolPath("0", connectorapi.OdometerE), e),
	}
}

// metre is what 1000 mm of 1.75 mm PLA weighs at 1.24 g/cm³.
const metre = 2.98255

func TestAPrintingToolTakesFilamentOffItsSpool(t *testing.T) {
	w := follow(t, printer("p1", map[connectorapi.ToolID]tool{"0": {}}))
	sp := w.spool(t)

	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: "p1", ToolID: "0", SpoolID: &sp.ID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	w.start(t)
	w.bus.send(t, busapi.Window{Telemetry: odometer(100)})
	w.bus.send(t, busapi.Window{Telemetry: odometer(1100)})

	if got := w.left(t, sp); math.Abs(got-(500-metre)) > 1e-4 {
		t.Errorf("a metre fed leaves %v g", got)
	}
}

func TestTheReadingOutlivesARestart(t *testing.T) {
	at := func(e float64) deviceapi.Device {
		d := printer("p1", map[connectorapi.ToolID]tool{"0": {}})
		d.State[connectorapi.ToolPath("0", connectorapi.OdometerID)] = encode(7)
		d.State[connectorapi.ToolPath("0", connectorapi.OdometerE)] = encode(e)

		return d
	}

	w := follow(t)
	sp := w.spool(t)

	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: "p1", ToolID: "0", SpoolID: &sp.ID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	w.fleet.set(at(100))
	w.start(t)

	// The module starts again, on the same database, and finds the printer
	// a metre further on.
	again := &world{
		fleet: &fleet{devices: map[string]deviceapi.Device{}},
		bus:   &bus{windows: make(chan busapi.Window), applied: make(chan error)},
		store: w.store,
		ctx:   w.ctx,
	}
	again.fleet.set(at(1100))
	again.follower = New(again.fleet, again.bus.subscribe, again.store, slog.New(slog.DiscardHandler))
	again.start(t)

	if got := w.left(t, sp); math.Abs(got-(500-metre)) > 1e-4 {
		t.Errorf("the metre fed while the module was down leaves %v g", got)
	}
}

func TestAHeadWithAFeederCreditsNothing(t *testing.T) {
	mmu := printer("p1", map[connectorapi.ToolID]tool{"0": {}, "0.0": pla(""), "0.1": pla("")})
	mmu.State[connectorapi.ToolPath("0", connectorapi.FeederSlotCount)] = encode(2)
	w := follow(t, mmu)
	sp := w.spool(t)

	w.start(t)

	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: "p1", ToolID: "0.0", SpoolID: &sp.ID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	w.bus.send(t, busapi.Window{Telemetry: odometer(100)})
	w.bus.send(t, busapi.Window{Telemetry: odometer(1100)})

	if got := w.left(t, sp); got != 500 {
		t.Errorf("the head's odometer went onto a slot: %v g left", got)
	}
}

func TestTheWindowThatSwapsTheSpoolChargesTheOldOne(t *testing.T) {
	p := printer("p1", map[connectorapi.ToolID]tool{"0": pla("")})
	p.State[connectorapi.ToolPath("0", connectorapi.OdometerID)] = encode(7)
	p.State[connectorapi.ToolPath("0", connectorapi.OdometerE)] = encode(100)
	w := follow(t, p)
	old, read := w.spool(t), w.spool(t)

	if _, err := w.store.Assign(w.ctx, "alice", stock.Assignment{
		DeviceID: "p1", ToolID: "0", SpoolID: &old.ID, By: stock.ByPerson,
	}); err != nil {
		t.Fatal(err)
	}

	w.start(t)

	// The old spool is unloaded and the printer reads the next one's tag, in
	// one window.
	w.bus.send(t, busapi.Window{Telemetry: append(odometer(1100),
		sample(connectorapi.ToolPath("0", connectorapi.MaterialInstanceUUID), read.UUID))})

	if got := w.assignments(t, "p1")["0"].SpoolID; got == nil || *got != read.ID {
		t.Fatalf("the tag's spool is not on the tool: %v", got)
	}

	if got := w.left(t, old); math.Abs(got-(500-metre)) > 1e-4 {
		t.Errorf("the unloaded spool kept %v g", got)
	}

	if got := w.left(t, read); got != 500 {
		t.Errorf("the spool read after the unload was charged: %v g left", got)
	}
}

func TestAHeaterOrFanSampleChangesNothingApplyReads(t *testing.T) {
	f := &Follower{printers: map[string]deviceapi.Device{"p1": {ID: "p1"}}}

	for _, member := range []string{
		connectorapi.NozzleTemp, connectorapi.NozzleTargetTemp,
		connectorapi.PrintFanRPM, connectorapi.HeatbreakFanRPM,
	} {
		changed, _ := f.samples([]json.RawMessage{sample(connectorapi.ToolPath("0", member), 215)})
		if len(changed) != 0 {
			t.Errorf("a %s sample marked the printer changed", member)
		}
	}

	if _, held := f.printers["p1"].State[connectorapi.ToolPath("0", connectorapi.NozzleTemp)]; !held {
		t.Error("the nozzle temperature was not kept")
	}

	changed, _ := f.samples([]json.RawMessage{sample(connectorapi.ToolPath("0", connectorapi.OdometerE), 1000)})
	if !changed["p1"] {
		t.Error("an odometer sample did not mark the printer changed")
	}
}

// A spool added at 0 g before its printer reported a weight takes the weight
// once the printer reports it, less what the tag says was used.
func TestATaggedSpoolWithNoWeightIsWeighedByTheNextReport(t *testing.T) {
	w := follow(t)

	m, err := w.store.CreateCustom(w.ctx, "alice", stock.MaterialFields{
		Name: "Galaxy", Class: "FFF", Type: "PLA", Abbreviation: "PLA",
	})
	if err != nil {
		t.Fatal(err)
	}

	tag := uuid.NewString()

	sp, err := w.store.AddReported(w.ctx, "alice",
		stock.ReportedSpool{UUID: tag, MaterialUUID: m.UUID, Abbreviation: "PLA"}, 1000)
	if err != nil || sp.FullWeightG != nil || sp.RemainingWeightG != 0 {
		t.Fatalf("added with no weight: %+v %v", sp, err)
	}

	d := printer("p1", map[connectorapi.ToolID]tool{"0": pla(tag)})
	d.State[connectorapi.ToolPath("0", connectorapi.MaterialNominalFullWeight)] = encode(1000)
	d.State[connectorapi.ToolPath("0", connectorapi.ConsumedWeight)] = encode(595)
	w.fleet.set(d)
	w.start(t)

	got, _, err := w.store.Spool(w.ctx, sp.ID)
	if err != nil || got.FullWeightG == nil || *got.FullWeightG != 1000 || got.RemainingWeightG != 405 {
		t.Fatalf("the spool after the report: %+v %v", got, err)
	}
}
