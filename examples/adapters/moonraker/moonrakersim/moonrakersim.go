// Package moonrakersim is a Moonraker host with no Klipper behind it: it
// answers the JSON-RPC calls the real one answers, over the same websocket, so
// a dialect can be driven end to end without a machine on the bench.
//
// It is a fake written from Moonraker's published API and not a recording of a
// real host. So it proves this project's client and says nothing about whether
// a field Moonraker sends is spelled the way this package spells it. What it is
// for is the half a fake can prove: the framing, the subscription, the
// narrowing a machine's own object list performs, and a command reaching the
// machine and being answered.
//
// The default machine is a complete one: it has pause_resume, an extruder and a
// heated bed, so every verb the moonraker dialect declares is offered. Options
// narrows that, which is how a case watches a verb being withheld.
package moonrakersim

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"net/http"
	pathpkg "path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// websocketPath is where Moonraker serves its JSON-RPC API, and therefore where
// this host serves it too.
const websocketPath = "/websocket"

// readHeaderTimeout bounds the request line and headers of one upgrade, because
// the zero value is no timeout at all.
const readHeaderTimeout = 5 * time.Second

// jsonrpcVersion is the version every frame carries.
const jsonrpcVersion = "2.0"

// The methods this host answers. A method outside this list is answered with
// the error a real Moonraker gives, which is what a dialect calling something
// that is not there has to cope with.
const (
	methodPrinterInfo          = "printer.info"
	methodObjectsList          = "printer.objects.list"
	methodObjectsQuery         = "printer.objects.query"
	methodObjectsSubscribe     = "printer.objects.subscribe"
	methodPrintStart           = "printer.print.start"
	methodPrintPause           = "printer.print.pause"
	methodPrintResume          = "printer.print.resume"
	methodPrintCancel          = "printer.print.cancel"
	methodGcodeScript          = "printer.gcode.script"
	methodFilesRoots           = "server.files.roots"
	methodFilesGetDirectory    = "server.files.get_directory"
	methodFilesMetadata        = "server.files.metadata"
	methodFilesPostDirectory   = "server.files.post_directory"
	methodFilesDeleteFile      = "server.files.delete_file"
	methodFilesDeleteDirectory = "server.files.delete_directory"
)

// notifyStatusUpdate is the notification a subscription arrives as.
const notifyStatusUpdate = "notify_status_update"

// methodNotFound is the JSON-RPC code for a method the server does not have.
const methodNotFound = -32601

// requestRefused is the code Moonraker answers a call its printer cannot carry
// out with.
const requestRefused = 400

// The readings the default machine reports. They are a machine that is switched
// on, cold and idle, with the two heater ranges a stock configuration carries.
const (
	roomNozzle      = 24.5
	roomBed         = 23.9
	nozzleMax       = 300.0
	nozzle          = 0.4
	bedMax          = 120.0
	coldest         = 0.0
	klipperTime     = 578243.57824499
	axisXYMax       = 300.0
	axisYMin        = -5.0
	axisZMax        = 280.0
	simModifiedTime = 1700000000.0
	simDirSize      = 4096
	simThumbSize    = 300
	simDiskTotal    = int64(10_000_000_000)
	simDiskUsed     = int64(2_000_000_000)
	simDiskFree     = int64(8_000_000_000)
	maxUploadBytes  = 32 << 20
)

// Options is what kind of machine this host pretends to have. Every zero field
// takes the full machine below.
type Options struct {
	// Hostname is what printer.info reports as the Klipper host's name.
	Hostname string
	// SoftwareVersion is the Klipper version printer.info reports.
	SoftwareVersion string
	// Objects is what printer.objects.list answers. Nil takes DefaultObjects.
	Objects []string
	// Status is one reading of every object, keyed by object then field. Nil
	// takes DefaultStatus.
	Status map[string]map[string]any
	// Refused names the methods this host answers with an error, against the
	// message it answers with. It is how a case reads what a dialect does with a
	// machine that will not do as it is told.
	Refused map[string]string
	// Addr is the host:port to bind. Empty binds an ephemeral loopback port,
	// which is what a test wants: two cases running at once must not have to
	// agree about a number.
	//
	// It exists for cmd/moonraker-sim, which a process outside this module runs
	// and then dials, and which is therefore the one caller that may need to say
	// where. Bind an address that is not loopback and this fake answers whatever
	// reaches it, so do not: it authenticates nobody and it is not a machine.
	Addr string
	// WithoutGcodes omits the gcodes root from server.files.roots.
	WithoutGcodes bool
	// GcodesReadOnly reports gcodes with "r" permissions instead of "rw".
	GcodesReadOnly bool
	// CorruptUploadSize writes a mismatched file size on upload to test
	// landing verification.
	CorruptUploadSize bool
}

// DefaultObjects is a complete Klipper machine: it can pause, it has a hotend
// and it has a heated bed.
func DefaultObjects() []string {
	return []string{
		"configfile", "display_status", "extruder", "gcode", "gcode_move",
		"heater_bed", "heaters", "pause_resume", "print_stats", "toolhead",
		"virtual_sdcard", "webhooks",
	}
}

// DefaultStatus is one reading of a machine that is switched on and idle, with
// the two heater sections a real configuration dump carries.
func DefaultStatus() map[string]map[string]any {
	return map[string]map[string]any{
		"print_stats": {
			"state": "standby", "filename": "", "print_duration": coldest, "total_duration": coldest,
		},
		"display_status": {"progress": coldest, "message": nil},
		"extruder":       {"temperature": roomNozzle, "target": coldest, "power": coldest},
		"heater_bed":     {"temperature": roomBed, "target": coldest, "power": coldest},
		"toolhead": {
			"position":   []any{coldest, coldest, coldest, coldest},
			"homed_axes": "",
			"extruder":   "extruder",
		},
		"gcode_move": {"speed_factor": 1.0, "extrude_factor": 1.0},
		"configfile": {
			"settings": map[string]any{
				"extruder":   map[string]any{"min_temp": coldest, "max_temp": nozzleMax, "nozzle_diameter": nozzle},
				"heater_bed": map[string]any{"min_temp": coldest, "max_temp": bedMax},
				"printer":    map[string]any{"kinematics": "corexy"},
				"stepper_x":  map[string]any{"position_min": coldest, "position_max": axisXYMax},
				"stepper_y":  map[string]any{"position_min": axisYMin, "position_max": axisXYMax},
				"stepper_z":  map[string]any{"position_max": axisZMax},
			},
		},
	}
}

// Host is a running fake. Close it when the case is over.
type Host struct {
	listener net.Listener
	server   *http.Server
	upgrader websocket.Upgrader

	hostname          string
	version           string
	objects           []string
	withoutGcodes     bool
	gcodesReadOnly    bool
	corruptUploadSize bool

	mu           sync.Mutex
	status       map[string]map[string]any
	refused      map[string]string
	conns        []*link
	calls        []string
	scripts      []string
	startedFiles []string
	dirs         map[string]bool
	files        map[string][]byte
}

// link is one open connection and the lock that keeps two writers off it. A
// push and an answer are written by different goroutines, and gorilla permits
// one writer at a time.
type link struct {
	conn *websocket.Conn

	mu sync.Mutex
}

func (l *link) write(frame any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.conn.WriteJSON(frame); err != nil {
		return fmt.Errorf("moonrakersim: write a frame: %w", err)
	}

	return nil
}

// Start binds a loopback port and serves until Close. ctx bounds the bind and
// nothing after it: the host serves for as long as it is open.
func Start(ctx context.Context, opts Options) (*Host, error) {
	var binding net.ListenConfig

	listener, err := binding.Listen(ctx, "tcp", orElse(opts.Addr, "127.0.0.1:0"))
	if err != nil {
		return nil, fmt.Errorf("moonrakersim: bind a port: %w", err)
	}

	host := &Host{
		listener:          listener,
		hostname:          orElse(opts.Hostname, "voron-bench"),
		version:           orElse(opts.SoftwareVersion, "v0.12.0-284-gfb1b4e39"),
		objects:           opts.Objects,
		withoutGcodes:     opts.WithoutGcodes,
		gcodesReadOnly:    opts.GcodesReadOnly,
		corruptUploadSize: opts.CorruptUploadSize,
		status:            opts.Status,
		refused:           maps.Clone(opts.Refused),
		dirs:              map[string]bool{"": true},
		files:             map[string][]byte{},
	}

	if host.objects == nil {
		host.objects = DefaultObjects()
	}

	if host.status == nil {
		host.status = DefaultStatus()
	}

	mux := http.NewServeMux()
	mux.HandleFunc(websocketPath, host.serve)
	mux.HandleFunc("/server/files/upload", host.serveUpload)
	mux.HandleFunc("/server/files/gcodes/", host.serveGcodesFile)

	host.server = &http.Server{Handler: mux, ReadHeaderTimeout: readHeaderTimeout}

	go func() { _ = host.server.Serve(listener) }()

	return host, nil
}

// Address is the host and port an endpoint names to reach this machine.
func (h *Host) Address() string { return h.listener.Addr().String() }

// Close stops serving and drops every open connection.
func (h *Host) Close() error {
	h.HangUp()

	if err := h.server.Close(); err != nil {
		return fmt.Errorf("moonrakersim: stop serving: %w", err)
	}

	return nil
}

// HangUp drops every open connection, which is the machine losing power rather
// than the fake stopping.
func (h *Host) HangUp() {
	h.mu.Lock()
	dropped := h.conns
	h.conns = nil
	h.mu.Unlock()

	for _, held := range dropped {
		_ = held.conn.Close()
	}
}

// Calls is every method this host was asked for, in the order it was asked.
func (h *Host) Calls() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return slices.Clone(h.calls)
}

// Called reports whether the method was asked for at least once.
func (h *Host) Called(method string) bool {
	return slices.Contains(h.Calls(), method)
}

// Scripts is every gcode line this host was sent, in order. It is what a case
// reads to see the line a verb became.
func (h *Host) Scripts() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return slices.Clone(h.scripts)
}

// StartedFiles returns every filename passed to printer.print.start, in order.
func (h *Host) StartedFiles() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	return slices.Clone(h.startedFiles)
}

// FileBytes returns the bytes held at a path relative to gcodes.
func (h *Host) FileBytes(rel string) ([]byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, ok := h.files[strings.Trim(rel, "/")]
	if !ok {
		return nil, false
	}

	return slices.Clone(data), true
}

// PutFile seeds a file at a path relative to gcodes, creating parent dirs.
func (h *Host) PutFile(rel string, data []byte) {
	clean := strings.Trim(rel, "/")

	h.mu.Lock()
	defer h.mu.Unlock()

	for dir := pathpkg.Dir(clean); dir != "." && dir != ""; dir = pathpkg.Dir(dir) {
		h.dirs[dir] = true
	}

	h.files[clean] = slices.Clone(data)
}

// Push sends one status update to every open connection and folds it into what
// a later query answers, which is what a real machine reporting a change does.
func (h *Host) Push(status map[string]map[string]any) {
	h.mu.Lock()

	for object, fields := range status {
		if h.status[object] == nil {
			h.status[object] = map[string]any{}
		}

		maps.Copy(h.status[object], fields)
	}

	open := slices.Clone(h.conns)
	h.mu.Unlock()

	frame := map[string]any{
		"jsonrpc": jsonrpcVersion,
		"method":  notifyStatusUpdate,
		"params":  []any{status, klipperTime},
	}

	for _, held := range open {
		_ = held.write(frame)
	}
}

// serve upgrades one connection and answers calls on it until it ends.
func (h *Host) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	open := &link{conn: conn}

	h.mu.Lock()
	h.conns = append(h.conns, open)
	h.mu.Unlock()

	defer func() { _ = conn.Close() }()

	for {
		var request struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			ID     *uint64         `json:"id"`
		}

		if err = conn.ReadJSON(&request); err != nil {
			return
		}

		if request.ID == nil {
			continue
		}

		h.record(request.Method)

		if err = open.write(h.answer(*request.ID, request.Method, request.Params)); err != nil {
			return
		}
	}
}

func (h *Host) record(method string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.calls = append(h.calls, method)
}

// answer is what this host makes of one call.
func (h *Host) answer(id uint64, method string, params json.RawMessage) map[string]any {
	if message, refused := h.refusal(method); refused {
		return errorFrame(id, requestRefused, message)
	}

	switch method {
	case methodPrinterInfo:
		return resultFrame(id, map[string]any{
			"state": "ready", "state_message": "Printer is ready",
			"hostname": h.hostname, "software_version": h.version,
		})
	case methodObjectsList:
		return resultFrame(id, map[string]any{"objects": h.objects})
	case methodObjectsQuery, methodObjectsSubscribe:
		return resultFrame(id, map[string]any{
			"eventtime": klipperTime, "status": h.reading(params),
		})
	case methodGcodeScript:
		h.recordScript(params)

		return resultFrame(id, "ok")
	case methodPrintStart:
		return h.startPrint(id, params)
	case methodPrintPause, methodPrintResume, methodPrintCancel:
		return resultFrame(id, "ok")
	case methodFilesRoots:
		return h.filesRoots(id)
	case methodFilesGetDirectory:
		return h.filesGetDirectory(id, params)
	case methodFilesMetadata:
		return h.filesMetadata(id, params)
	case methodFilesPostDirectory:
		return h.filesPostDirectory(id, params)
	case methodFilesDeleteFile:
		return h.filesDeleteFile(id, params)
	case methodFilesDeleteDirectory:
		return h.filesDeleteDirectory(id, params)
	default:
		return errorFrame(id, methodNotFound, "Method "+method+" is not available")
	}
}

// recordScript keeps the gcode line one call carried, so a case can read the
// line rather than only the fact that a call was made.
func (h *Host) recordScript(params json.RawMessage) {
	var sent struct {
		Script string `json:"script"`
	}

	if len(params) == 0 || json.Unmarshal(params, &sent) != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.scripts = append(h.scripts, sent.Script)
}

func (h *Host) refusal(method string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	message, refused := h.refused[method]

	return message, refused
}

// reading is the status of every object the call asked for, and of nothing
// else. An object this machine has not got is left out, which is what Moonraker
// does with an object Klipper does not have.
func (h *Host) reading(params json.RawMessage) map[string]map[string]any {
	asked := askedFor(params)

	h.mu.Lock()
	defer h.mu.Unlock()

	out := make(map[string]map[string]any, len(asked))

	for _, object := range asked {
		fields, held := h.status[object]
		if !held || !slices.Contains(h.objects, object) {
			continue
		}

		out[object] = maps.Clone(fields)
	}

	return out
}

// askedFor is the object names one query or subscription named.
func askedFor(params json.RawMessage) []string {
	var asked struct {
		Objects map[string]json.RawMessage `json:"objects"`
	}

	if len(params) == 0 || json.Unmarshal(params, &asked) != nil {
		return nil
	}

	return slices.Sorted(maps.Keys(asked.Objects))
}

func resultFrame(id uint64, result any) map[string]any {
	return map[string]any{"jsonrpc": jsonrpcVersion, "id": id, "result": result}
}

func errorFrame(id uint64, code int, message string) map[string]any {
	return map[string]any{
		"jsonrpc": jsonrpcVersion, "id": id,
		"error": map[string]any{"code": code, "message": message},
	}
}

func orElse(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
