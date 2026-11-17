package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// The values telemetry carries. They are fixed and not sampled. A simulator
// that imitated a real thermistor would force every assertion to use a
// tolerance.
const (
	printingNozzleTemp = 215.0
	printingBedTemp    = 60.0
	ambientTemp        = 25.0
	layerHeight        = 0.2
	fullProgress       = 100
)

// State returns the state the printer is reporting.
func (p *Printer) State() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.endBusyLocked()

	return p.state
}

// busyGCodes are the long moves that hold a machine at BUSY while they run: G28
// homes the axes and G29 levels the bed. Both leave the driver as gcode and
// not as a JSON command (VerbHome and VerbMeshBedLeveling in
// internal/connector/xbuddy/verbs.go), so the frame's payload names them. The
// firmware reports BUSY for the whole move.
//
//nolint:gochecknoglobals // a fixed set, read-only after init.
var busyGCodes = []string{"G28", "G29"}

// beginBusy holds the machine at BUSY for BusyDuration when a line of the
// payload is one of busyGCodes. A running or paused print takes no such move,
// and neither does a machine stopped on a dialog, so neither state is
// overwritten here.
func (p *Printer) beginBusy(payload []byte) {
	if !slices.ContainsFunc(strings.Split(string(payload), "\n"), isBusyLine) {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == statePrinting || p.state == statePaused || p.dialog != nil {
		return
	}

	p.busyResume = p.state
	p.busyUntil = time.Now().Add(p.busyDuration())
	p.state = stateBusy
}

// isBusyLine says whether one G-code line is a long move.
func isBusyLine(line string) bool {
	word, _, _ := strings.Cut(strings.TrimSpace(line), " ")

	return slices.Contains(busyGCodes, strings.ToUpper(word))
}

// endBusyLocked returns the machine to the state the move started from once the
// simulated move is over. The caller holds the lock.
//
// The expiry is read off the clock where the state is read and is not fired by
// a timer. A test inside a synctest bubble sees the move end on the first
// telemetry frame past the deadline and never on a goroutine it did not start.
func (p *Printer) endBusyLocked() {
	if p.state != stateBusy || time.Now().Before(p.busyUntil) {
		return
	}

	p.state = p.busyResume
	p.busyResume = ""
	p.busyUntil = time.Time{}
}

// busyDuration is how long a long move takes. It is configurable because the
// firmware's own 80 to 100 seconds is too long for a test.
func (p *Printer) busyDuration() time.Duration {
	if p.cfg.BusyDuration > 0 {
		return p.cfg.BusyDuration
	}

	return defaultBusyDuration
}

// Progress is how far the print has gone, in whole percent, and zero for a
// machine with nothing on the bed. It is the same figure telemetry carries, so
// a caller watching the machine from outside the wire reads what the host does.
func (p *Printer) Progress() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.progress()
}

// Printing reports whether a job is running or paused.
func (p *Printer) Printing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.state == statePrinting || p.state == statePaused
}

// telemetryLoop emits one frame per tick for as long as the session lasts. The
// cadence follows the state: 4 Hz idle and 1 Hz printing, as the firmware
// sends.
func (p *Printer) telemetryLoop(ctx context.Context) error {
	timer := time.NewTimer(p.period())
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}

		conn := p.currentConn()
		if conn == nil {
			return nil
		}

		payload, finished := p.nextTelemetry()

		if err := p.writeText(conn, payload); err != nil {
			return fmt.Errorf("telemetry: %w", err)
		}

		if finished {
			if err := p.announceFinished(conn); err != nil {
				return err
			}
		}

		timer.Reset(p.period())
	}
}

// nextTelemetry advances the print by one frame and encodes the result.
// finished is true on the frame that completed the job.
func (p *Printer) nextTelemetry() (payload []byte, finished bool) {
	p.mu.Lock()

	p.endBusyLocked()

	if p.state == statePrinting {
		p.job.printed++

		if p.job.frames > 0 && p.job.printed >= p.job.frames {
			p.state = stateFinished
			// The print's own word for how it ended, which is the second and
			// last time /job/state moves. The host reads it by asking, because
			// no JOB_INFO is volunteered here: see announceJob.
			p.jobState = stateFinished
			finished = true
		}
	}

	frame := p.telemetryFields()

	p.mu.Unlock()

	encoded, err := json.Marshal(frame)
	if err != nil {
		// Every value in the map is a string, a number or a bool, so this
		// cannot fail. An empty object keeps the session alive if it does.
		encoded = []byte("{}")
	}

	return encoded, finished
}

// telemetryFields builds one frame. The caller holds the lock.
func (p *Printer) telemetryFields() map[string]any {
	printing := p.state == statePrinting || p.state == statePaused

	fields := map[string]any{
		"state":    p.state,
		"speed":    fullProgress,
		"flow":     fullProgress,
		"material": "PLA",
		"axis_z":   layerHeight * float64(p.job.printed),
	}

	if p.cfg.NoMaterial {
		delete(fields, "material")
	}

	// The machine repeats the dialog's id for as long as it is on the display.
	// A host that missed the raising event learns of the dialog only from that
	// repeat, so the id is in every frame.
	if p.dialog != nil {
		fields["dialog_id"] = p.dialog.ID
	}

	if printing {
		fields["temp_nozzle"] = printingNozzleTemp
		fields["target_nozzle"] = printingNozzleTemp
		fields["temp_bed"] = printingBedTemp
		fields["target_bed"] = printingBedTemp
		fields["job_id"] = p.job.id
		fields["progress"] = p.progress()
		fields["time_printing"] = p.job.printed
		fields["time_remaining"] = p.remaining()
	} else {
		fields["temp_nozzle"] = ambientTemp
		fields["target_nozzle"] = 0.0
		fields["temp_bed"] = ambientTemp
		fields["target_bed"] = 0.0
		fields["axis_x"] = 0.0
		fields["axis_y"] = 0.0
	}

	if p.hasChamber() {
		p.sendChanged(fields, "chamber", p.chamberFields(printing))
	}

	if p.cfg.Enclosure {
		p.sendChanged(fields, "enclosure", p.enclosureFields(printing))
	}

	switch {
	case p.cfg.ToolTree:
		fields["tools"] = p.treeTelemetry(printing)
	case p.multiHead():
		fields["slot"] = p.slotFields(printing)
	case p.hasMMU():
		nozzle, _ := fields["temp_nozzle"].(float64)
		fields["slot"] = p.mmuSlotFields(printing, nozzle)
	}

	return fields
}

// modelXL is the multi-toolhead model that reports the numbered slot block,
// one entry per toolhead keyed from 1 (Prusa-Firmware-Buddy,
// src/connect/render.cpp).
const modelXL = "XL"

// xlTools is the XL's toolheads in slot order: the material loaded in each and
// the temperature it prints at. The temperatures differ so a consumer cannot
// confuse one slot for another.
//
//nolint:gochecknoglobals // a fixed table, read-only.
var xlTools = []headSpec{
	{"PLA", 215}, {"PETG", 240}, {"ASA", 260}, {"PLA", 205}, {"PVA", 190},
}

// indxTools is the eight heads of an INDX machine, as xlTools is the XL's.
// The temperatures differ for the same reason.
//
//nolint:gochecknoglobals // a fixed table, read-only.
var indxTools = []headSpec{
	{"PLA", 215}, {"PETG", 240}, {"ASA", 260}, {"PLA", 210},
	{"PVA", 190}, {"PETG", 235}, {"PLA", 205}, {"ASA", 255},
}

// headSpec is one head of a machine with several: the material loaded in it
// and the temperature it prints at.
type headSpec struct {
	material string
	temp     float64
}

// maxHeads is the most heads a simulated machine has, an INDX's eight.
const maxHeads = 8

// headTable is the heads of a machine with several, and nothing for any other
// machine.
func (p *Printer) headTable() []headSpec {
	switch {
	case p.model.Name == modelXL:
		return xlTools
	case strings.HasSuffix(strings.ToUpper(p.model.Name), "_INDX"):
		return indxTools
	}

	return nil
}

// multiHead reports whether the machine has several heads, which its
// numbered telemetry reports in a slot block.
func (p *Printer) multiHead() bool { return len(p.headTable()) > 0 }

const (
	// xlStandbyDrop is how far below its printing temperature a parked tool
	// idles while another one prints.
	xlStandbyDrop = 30.0
	xlFanPrint    = 6000
	xlFanHotend   = 8000
	slotReady     = 1
)

// slotFields builds the slot block of a machine with several heads. The active
// tool walks from the first to the last as the print progresses, an equal
// share of the job each. The caller holds the lock.
func (p *Printer) slotFields(printing bool) map[string]any {
	active := 1
	if printing {
		active = min(p.progress()*len(p.headTable())/fullProgress, len(p.headTable())-1) + 1
	}

	slot := map[string]any{"active": active, "state": slotReady}

	for i, tool := range p.headTable() {
		if p.headDisabled(i) {
			continue
		}

		number := i + 1
		temp := ambientTemp + float64(i)
		fanPrint := 0

		if printing {
			temp = tool.temp - xlStandbyDrop
			if number == active {
				temp = tool.temp
				fanPrint = xlFanPrint
			}
		}

		// The head reaches its target at once: nothing here models heating.
		if target := p.toolTargets[i]; target > 0 {
			temp = target
		}

		entry := map[string]any{
			"material":   tool.material,
			"temp":       temp,
			"fan_print":  fanPrint,
			"fan_hotend": xlFanHotend,
		}

		if p.cfg.NoMaterial {
			delete(entry, "material")
		}

		slot[strconv.Itoa(number)] = entry
	}

	return slot
}

// noteToolTarget holds the target an M104 naming a tool sets on one XL head,
// so that head's slot temperature reads it. The firmware counts tools from 0.
// A line naming no tool sets the active head, which the slot block does not
// model, so it changes nothing here.
func (p *Printer) noteToolTarget(line []byte) {
	if !p.multiHead() {
		return
	}

	words := strings.Fields(strings.ToUpper(string(line)))
	if len(words) == 0 || words[0] != "M104" {
		return
	}

	tool, target := -1, -1.0

	for _, word := range words[1:] {
		switch word[0] {
		case 'T':
			if n, err := strconv.Atoi(word[1:]); err == nil {
				tool = n
			}
		case 'S':
			if v, err := strconv.ParseFloat(word[1:], 64); err == nil {
				target = v
			}
		}
	}

	if tool < 0 || tool >= len(p.headTable()) || target < 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.toolTargets[tool] = target
}

// remaining is the seconds the machine says are left. One frame is one second
// of print at the printing cadence, so the frames still to run are the
// estimate. The caller holds the lock.
func (p *Printer) remaining() int {
	return max(p.job.frames-p.job.printed, 0)
}

// progress is whole percent, as the firmware reports it. The caller holds the
// lock.
func (p *Printer) progress() int {
	if p.job.frames <= 0 {
		return 0
	}

	if p.job.printed >= p.job.frames {
		return fullProgress
	}

	return p.job.printed * fullProgress / p.job.frames
}

// announceFinished reports the print that just completed, under the id of the
// command that started it.
//
// It volunteers no JOB_INFO. The print reaching its end is a state change like
// pausing and stopping: the next telemetry frame says FINISHED, and a host
// that wants to know which file that was asks with SEND_JOB_INFO. See
// announceJob.
func (p *Printer) announceFinished(conn *websocket.Conn) error {
	p.mu.Lock()
	id := p.job.id
	p.mu.Unlock()

	return p.send(conn, event{Event: eventFinished, CommandID: &id, State: p.State()})
}

func (p *Printer) period() time.Duration {
	if p.Printing() {
		return p.cfg.PrintingPeriod
	}

	return p.cfg.IdlePeriod
}

func (p *Printer) currentConn() *websocket.Conn {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.conn
}

// setPaused moves a job into or out of the paused state and reports whether
// there was a job the move applies to. Pausing needs a print that is running
// and resuming needs one that is paused. Anything else is refused.
func (p *Printer) setPaused(paused bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch {
	case paused && p.state == statePrinting:
		p.state = statePaused
	case !paused && p.state == statePaused:
		p.state = statePrinting
	default:
		return false
	}

	return true
}

// SetJobFrames makes the next print take that many frames instead of
// JobFrames. It is an input like the faults: whoever drives the printer
// decides the length, the printer never picks one.
func (p *Printer) SetJobFrames(frames int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.nextFrames = frames
}

// SetJobID makes the next print carry that job number instead of the id of
// the command that started it. A printer whose counter was reset numbers its
// prints from the start again, so a number the host has already seen comes
// back for a print it has not.
func (p *Printer) SetJobID(id uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.nextJobID = id
}

// startJob begins a print and reports whether the printer was free to take one.
// A machine already printing has one job, so a second START_PRINT is refused
// and does not replace what is on the bed.
func (p *Printer) startJob(id uint32, path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == statePrinting || p.state == statePaused {
		return false
	}

	frames := p.cfg.JobFrames
	if p.nextFrames > 0 {
		frames = p.nextFrames
		p.nextFrames = 0
	}

	if p.nextJobID > 0 {
		id = p.nextJobID
		p.nextJobID = 0
	}

	p.job = job{id: id, path: path, frames: frames}
	p.lastJob = p.job
	p.state = statePrinting
	p.jobState = statePrinting

	return true
}

// stopJob ends the print in progress and reports whether there was one. A
// printer with nothing running refuses the verb.
func (p *Printer) stopJob(state string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != statePrinting && p.state != statePaused {
		return false
	}

	p.job = job{}
	p.state = state
	// A machine with no job on the bed has no job to describe. Buddy answers
	// SEND_JOB_INFO with IDLE there, and the word for how the print ended is
	// the machine's own STOPPED at /state.
	p.jobState = stateIdle

	return true
}

// setIdleState answers SET_PRINTER_READY, CANCEL_PRINTER_READY and SET_IDLE. It
// reports whether the printer was free to take the state. None of these verbs
// interrupts a print in progress.
func (p *Printer) setIdleState(state string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == statePrinting || p.state == statePaused {
		return false
	}

	p.state = state

	return true
}

// infoData is the INFO event's payload: the identity and the storage devices,
// which the host merges into its file tree.
func (p *Printer) infoData() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()

	storages := make([]map[string]any, 0, len(p.storages))

	for _, device := range p.storages {
		storages = append(storages, map[string]any{
			"mountpoint": device.Mountpoint,
			"type":       device.Type,
			"read_only":  device.ReadOnly,
			"free_space": device.FreeSpace,
		})
	}

	info := map[string]any{
		"firmware":     p.cfg.Firmware,
		"printer_type": p.model.DeviceTypeID,
		"sn":           p.cfg.Serial,
		"fingerprint":  p.cfg.Fingerprint,
		"appendix":     false,
		"storages":     storages,
		// INFO writes the MAC in upper case (render.cpp JSON_MAC). The
		// address is the loopback a simulated printer sends from.
		"network_info": map[string]any{
			"lan_mac":  strings.ToUpper(p.cfg.MAC),
			"lan_ipv4": "127.0.0.1",
			"hostname": "prusa-" + strings.ToLower(p.model.Name),
		},
	}

	if p.cfg.ToolTree {
		p.treeInfo(info)
	} else {
		p.infoTools(info)
	}

	return info
}

// jobData is the JOB_INFO event's payload, empty on a machine that has never
// printed.
func (p *Printer) jobData() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.jobDataLocked()
}

// jobDataLocked is jobData with the lock already held.
//
// It falls back to the print that has left the bed, so a SEND_JOB_INFO sent
// after a print ended still names the file. Telemetry never carries the file.
//
// The state it reports is the print's and not the machine's. For a print that
// has left the bed it is FINISHED or IDLE: a print that ran to its end says
// so, and a stopped one is IDLE because there is no job left to describe.
//
// For a print that is still on the bed the firmware renders the word from its
// own device state, and renders it coarsely: PRINTING while the device state
// is exactly Printing, and PAUSED for every other one, paused, a dialog on the
// screen, or busy (Prusa-Firmware-Buddy, src/connect/render.cpp, the
// EventType::JobInfo arm). So a SEND_JOB_INFO answered while a print-preview
// dialog stands says PAUSED about a print nobody paused.
func (p *Printer) jobDataLocked() map[string]any {
	reported := p.job
	if reported.path == "" {
		reported = p.lastJob
	}

	if reported.path == "" {
		return nil
	}

	state := p.jobState
	if p.job.path != "" && slices.Contains(machineHoldsAPrint, p.state) {
		state = statePaused
		if p.state == statePrinting {
			state = statePrinting
		}
	}

	// No "display". Buddy firmware names one only for a file whose name it had
	// to shorten, so a plainly named print reaches a host with a path and no
	// display name. A simulator that filled the field in would hide the case
	// the host has to handle.
	return map[string]any{
		"state":  state,
		"path":   reported.path,
		"job_id": reported.id,
	}
}
