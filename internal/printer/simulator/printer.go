package simulator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
)

// The defaults a zero Config takes. The telemetry cadences are the firmware's:
// 4 Hz idle, 1 Hz printing.
const (
	DefaultFirmware = "6.4.0"

	defaultIdlePeriod       = 250 * time.Millisecond
	defaultPrintingPeriod   = time.Second
	defaultRegisterPoll     = 100 * time.Millisecond
	defaultReconnectDelay   = 100 * time.Millisecond
	defaultHandshakeTimeout = 5 * time.Second
	defaultBusyDuration     = 500 * time.Millisecond
	fingerprintLength       = 15
	defaultFreeSpace        = 1 << 30
)

// ErrNotRegistered is a connection attempted before registration produced a
// token.
var ErrNotRegistered = errors.New("the printer has no token, so it has not registered")

// Storage is one storage device the printer reports in INFO and prints from.
type Storage struct {
	// Mountpoint is the firmware-side path, with its leading slash: /usb.
	Mountpoint string
	// Type is the device class the firmware reports, such as USB.
	Type string
	// ReadOnly refuses writes to this device.
	ReadOnly bool
	// FreeSpace is the bytes left. Zero is a full disk, a fault the caller
	// sets.
	FreeSpace int64
}

// Config is one simulated printer.
type Config struct {
	// Model is the printer model, matched against the capability profiles
	// without regard to case. Required.
	Model string
	// Serial is the serial number registration carries as sn. Derived from the
	// model when empty.
	Serial string
	// Fingerprint identifies the printer across reconnections. Derived from the
	// model and serial when empty, so it is stable across runs.
	Fingerprint string
	// MAC is the Ethernet address the printer reports in INFO and names
	// itself by in its syslog, lower case. Derived from the fingerprint when
	// empty.
	MAC string
	// Firmware is the version label the printer reports. It selects the profile
	// the host resolves capabilities against.
	Firmware string
	// BaseURL is the server, such as http://localhost:8080.
	BaseURL string
	// Dial opens the transport. A test passes a net.Pipe dialler, which keeps
	// the whole session inside a testing/synctest bubble. Nil means a real TCP
	// socket, which does not.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
	// Storages are the devices the printer reports. A nil value means one
	// writable USB stick.
	Storages []Storage
	// Files is the tree the printer reports and prints from, in the firmware's
	// own absolute spelling. A nil value means the default tree, which is not
	// empty: a listing checked against an empty storage passes whatever the
	// code under it does.
	Files []File
	// IdlePeriod and PrintingPeriod are the telemetry cadences.
	IdlePeriod     time.Duration
	PrintingPeriod time.Duration
	// RegisterPoll is the wait between GET /p/register attempts.
	RegisterPoll time.Duration
	// ReconnectDelay is the wait before redialling after a lost session. It is
	// a fixed delay and not a jittered ladder, because the ladder belongs to the
	// implementation under test and randomness here would hide it.
	ReconnectDelay time.Duration
	// MaxChunkBytes is the largest transfer chunk this printer takes in one
	// frame before it hangs up. Zero means maxFramePayload, the chunk the
	// firmware asks for. A larger value is a board that takes more, for a test
	// of a server that writes larger chunks. A smaller value is a board that
	// hangs up on a frame the default would take.
	MaxChunkBytes int
	// BusyDuration is how long a long move holds the machine at BUSY: G28 and
	// G29 in busyGCodes. Zero means defaultBusyDuration. A real MK4 takes 80
	// to 100 seconds, which is too long for a test, so this is a setting.
	BusyDuration time.Duration
	// MMU fits a multi-material unit with five filament slots. It applies
	// only to a model whose firmware is built with MMU support, such as the
	// MK4S, and is ignored on the others.
	MMU bool
	// ToolTree makes the printer describe its tools in the tree shape of the
	// public Prusa architecture, as firmware that speaks it would: INFO with
	// tool_count and tools keyed "0", "0.0", and telemetry with a tools object
	// in place of the slot block. Zero sends the numbered shape the shipping
	// firmware sends.
	ToolTree bool
	// FeederHeads, with ToolTree, gives the printer that many heads, each
	// with a feeder of FeederSlots slots. No model has such a machine; it is
	// what proves a reader handles a feeder on every head.
	FeederHeads int
	FeederSlots int
	// DisabledHeads switches off these heads, counted from 0, as a person
	// does on an XL. The numbered shape leaves them out, and the tree shape
	// writes them as null.
	DisabledHeads []int
	// JobFrames is how many telemetry frames a print takes before it finishes.
	// Zero means a print runs until it is stopped.
	JobFrames int
	// Logger receives the printer's own log. Discarded when nil.
	Logger *slog.Logger
	// NoMaterial leaves the loaded material out of telemetry, as a printer
	// with no filament sensor reports it, so the host keeps what a person said
	// is loaded. Zero reports PLA.
	NoMaterial bool
	// Enclosure fits an XL with its enclosure, switched on. Such a machine
	// sends an enclosure block in its telemetry, and one without sends none.
	Enclosure bool
	// TransferRate is how many bytes a second this printer fetches a file at.
	// It asks for the next block once the last one would have taken that long
	// to arrive. Zero fetches as fast as the server answers. A real board fetches
	// at some hundreds of kilobytes a second, and a demo that shows a file on
	// its way needs a transfer that lasts long enough to be seen.
	TransferRate int64
	// ListingDelay is how long this printer takes to answer SEND_FILE_INFO on a
	// folder. It waits in the goroutine that reads commands, so every command
	// sent meanwhile waits behind the listing, as on Buddy firmware, which runs
	// one command at a time. An MK4S lists the root of a USB stick of about 300
	// files in 12 to 15 s. Zero answers at once.
	ListingDelay time.Duration
	// RestartsTransfers makes the printer ask for a file from its first byte
	// on each connection after the one that accepted it, and drop what it had
	// taken. False resumes where the last connection stopped. A real board can
	// start over like this.
	RestartsTransfers bool
}

// Printer is one simulated printer. It is safe for concurrent use: the fault
// setters may be called from a test goroutine while the session runs.
type Printer struct {
	cfg   Config
	model Model
	log   *slog.Logger

	client *http.Client
	dialer *websocket.Dialer

	mu      sync.Mutex
	token   string
	conn    *websocket.Conn
	offline bool
	online  chan struct{}
	delay   time.Duration
	// acceptFirst answers every command with ACCEPTED before its own answer,
	// as the Prusa Connect SDK does. See SetAcceptFirst.
	acceptFirst bool
	storages    []Storage
	files       []File
	state       string
	job         job
	// nextFrames is the length SetJobFrames asked of the next print. Zero
	// means the configured JobFrames.
	nextFrames int
	// nextJobID is the number SetJobID asked the next print to carry. Zero
	// means the id of the command that started it.
	nextJobID uint32
	lastJob   job
	// jobState is what JOB_INFO says the print is doing. It is not the machine's
	// state: Buddy reports one of PRINTING, FINISHED and IDLE for a print and
	// ten words for itself (Prusa-Firmware-Buddy/src/state/printer_state.cpp:563),
	// and it moves this one at the start of a print and at its end and nowhere
	// else. See jobDataLocked.
	jobState string
	// busyUntil is when the long move the machine is BUSY with ends, and
	// busyResume is the state it returns to. See beginBusy.
	busyUntil  time.Time
	busyResume string
	// toolTargets is the nozzle target an M104 set on each XL head, zero for
	// none. See noteToolTarget.
	toolTargets [maxHeads]float64
	// nozzles is the nozzle a tool setter put on each head, by head id from
	// 0. A head with none keeps the nozzle it was built with. See setNozzle.
	nozzles  map[int]nozzle
	dialog   *Dialog
	sessions uint64
	// asked counts the commands the host has sent, by the firmware's own name
	// for each. A verb the host should send and never does is invisible on the
	// state tree. See Asked.
	asked map[string]int
	// chamber is the enclosure of a machine with one. See chamber.go.
	chamber chamber
	// blocksSent is the chamber and enclosure blocks as last sent, and
	// blocksSession the connection they were sent on. See sendChanged.
	blocksSent    map[string]string
	blocksSession uint64
	// filamentToLoad is the name the last M701 carried, and filamentAsked is
	// whether one arrived at all. See FilamentToLoad.
	filamentToLoad string
	filamentAsked  bool
	// flashed is the path the last M997 named, and flashAsked is whether one
	// arrived at all. See Flashed.
	flashed    string
	flashAsked bool
	// adopted wakes whoever is waiting for the printer's half of a connection.
	// See Adopted.
	adopted chan struct{}
	// transfer is the file being fetched, nil when there is none. A printer
	// runs one at a time: see transfer.go.
	transfer     *transfer
	lastTransfer Transfer
	// transferHeld stops the transfer at the end of the block it is fetching,
	// and parked is the block request it then owes. See HoldTransfer.
	transferHeld bool
	parked       *parkedBlock
	// metrics is the Metrics & Log setting M334 stored: whether it is on and
	// where the printer sends. See metrics.go.
	metrics      metricsTarget
	metricsSeq   uint32
	syslogSeq    uint32
	nextDialogID int64
	// dialogAnswered runs when a person answers the dialog on the display,
	// with the button they pressed. Nil for a dialog nothing waits on.
	dialogAnswered func(button string)
	// beforeDialog is the state the printer was in when the dialog on the
	// display was raised, and what answering it returns to.
	beforeDialog string
	booted       time.Time
	// emitter holds one token while an emitter runs, so a Run after an
	// earlier one ended starts a new emitter and two Runs never start two.
	emitter chan struct{}
	// locked is the paths this printer will not let go of. See Printer.plant.
	locked         map[string]struct{}
	nextFileID     uint32
	nextTransferID int64

	writeMu sync.Mutex
}

// job is the print in progress, or the zero value when there is none.
type job struct {
	id      uint32
	path    string
	frames  int
	printed int
}

// lastJob is the print the machine most recently held, kept after the job is
// off the bed so that JOB_INFO can still say what stopped.
//
// A host that sends SEND_JOB_INFO after a print ended must still learn which
// file it was, because telemetry never carries the file.

// New returns a printer for a model the capability profiles cover.
func New(cfg Config) (*Printer, error) {
	model, err := ModelByName(cfg.Model)
	if err != nil {
		return nil, err
	}

	cfg = withDefaults(cfg, model)

	online := make(chan struct{})
	close(online)

	p := &Printer{
		cfg:          cfg,
		model:        model,
		log:          cfg.Logger,
		online:       online,
		adopted:      make(chan struct{}, 1),
		storages:     append([]Storage(nil), cfg.Storages...),
		files:        append([]File(nil), cfg.Files...),
		state:        stateIdle,
		jobState:     stateIdle,
		asked:        map[string]int{},
		chamber:      newChamber(),
		booted:       time.Now(),
		nextDialogID: firstDialogID,
		metrics:      metricsTarget{metricsPort: defaultMetricsPort, syslogPort: defaultSyslogPort},
		emitter:      make(chan struct{}, 1),
	}

	transport := &http.Transport{DialContext: cfg.Dial}
	p.client = &http.Client{Transport: transport}
	p.dialer = &websocket.Dialer{
		NetDialContext:   cfg.Dial,
		Subprotocols:     []string{websocketSubprotocol},
		HandshakeTimeout: defaultHandshakeTimeout,
	}

	return p, nil
}

// Fingerprint is the identity the printer presents on every connection.
func (p *Printer) Fingerprint() string { return p.cfg.Fingerprint }

// Serial is the serial number registration carried.
func (p *Printer) Serial() string { return p.cfg.Serial }

// Model returns the wire vocabulary this printer answers with.
func (p *Printer) Model() Model { return p.model }

// Token returns the token registration produced, empty before it has.
func (p *Printer) Token() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.token
}

// Asked is how many times the host has sent one command, under the firmware's
// name for it, such as SEND_JOB_INFO.
//
// A test cannot read this anywhere else. A real machine volunteers nothing
// after a print changes state, so a host must ask, and only this count shows
// whether it did.
func (p *Printer) Asked(command string) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.asked[command]
}

// SetToken supplies a token out of band, for a printer that is already admitted
// and does not repeat the handshake.
func (p *Printer) SetToken(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.token = token
}

// Sessions is how many websocket sessions this printer has established. It is
// the count a reconnection test waits on.
func (p *Printer) Sessions() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.sessions
}

// Adopted wakes a caller each time the printer takes on a connection it
// dialled, and Sessions says how many it has taken on.
//
// Nothing else orders the two halves of one connection. A server hands its own
// side over as soon as it has accepted the upgrade, and the dial here returns
// after that. A caller that drives the printer as soon as the server has seen
// it can find no connection to write on. Such a caller waits for a wake-up
// here and reads Sessions again.
//
// It carries one wake-up at a time and never blocks a session. A caller that
// missed a wake-up reads Sessions again.
func (p *Printer) Adopted() <-chan struct{} { return p.adopted }

// Register performs the registration handshake: POST /p/register for a Code,
// then GET /p/register with that code until the operator has decided. The
// Token header is the answer.
func (p *Printer) Register(ctx context.Context) error {
	code, err := p.beginRegistration(ctx)
	if err != nil {
		return err
	}

	for {
		token, pending, pollErr := p.pollRegistration(ctx, code)

		switch {
		case pollErr != nil:
			return pollErr
		case !pending:
			p.SetToken(token)

			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.cfg.RegisterPoll):
		}
	}
}

// Run connects and serves until the context is cancelled, redialling after a
// lost session and waiting while the printer has been taken offline.
//
// A cancelled context is a stop and not a failure, so it returns nil.
func (p *Printer) Run(ctx context.Context) error {
	if p.Token() == "" {
		return ErrNotRegistered
	}

	go p.emit(ctx)

	for {
		if !p.waitOnline(ctx) {
			return nil
		}

		if err := p.session(ctx); err != nil && ctx.Err() == nil {
			p.logSessionEnd(ctx, err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(p.cfg.ReconnectDelay):
		}
	}
}

// RegisterAndRun is Register followed by Run, which is what a fleet wants.
func (p *Printer) RegisterAndRun(ctx context.Context) error {
	if p.Token() == "" {
		if err := p.Register(ctx); err != nil {
			return err
		}
	}

	return p.Run(ctx)
}

// session is one websocket connection, from the upgrade to the read that fails.
func (p *Printer) session(ctx context.Context) error {
	conn, err := p.connect(ctx)
	if err != nil {
		return err
	}

	if !p.beginSession(conn) {
		// The printer was taken offline while this dial was in flight. Serving
		// the connection would break what GoOffline promises.
		_ = conn.Close()

		return nil
	}

	defer func() {
		p.endSession()
		_ = conn.Close()
	}()

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		<-groupCtx.Done()

		// Unblocks the reader: a websocket read has no context, so the
		// connection is what carries the cancellation.
		_ = conn.Close()

		return nil
	})
	group.Go(func() error { return p.readLoop(groupCtx, conn) })
	group.Go(func() error { return p.telemetryLoop(groupCtx) })
	group.Go(func() error { return p.resumeTransfer(conn) })

	return group.Wait()
}

// connect dials GET /p/ws with the Fingerprint and Token headers, and the two
// User-Agent headers the server reads the model and firmware from.
func (p *Printer) connect(ctx context.Context) (*websocket.Conn, error) {
	token := p.Token()
	if token == "" {
		return nil, ErrNotRegistered
	}

	endpoint, err := p.websocketURL()
	if err != nil {
		return nil, err
	}

	header := http.Header{}
	header.Set("Fingerprint", p.cfg.Fingerprint)
	header.Set("Token", token)
	header.Set("User-Agent-Printer", p.model.Name)
	header.Set("User-Agent-Version", p.cfg.Firmware)

	conn, resp, err := p.dialer.DialContext(ctx, endpoint, header)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", endpoint, err)
	}

	if conn.Subprotocol() != websocketSubprotocol {
		_ = conn.Close()

		return nil, fmt.Errorf("server negotiated subprotocol %q, not %q",
			conn.Subprotocol(), websocketSubprotocol)
	}

	return conn, nil
}

// readLoop serves one frame at a time. Serving them in one goroutine makes
// "one command in flight per printer" true without a lock.
func (p *Printer) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		if kind != websocket.TextMessage && kind != websocket.BinaryMessage {
			continue
		}

		decoded, decodeErr := decodeFrame(data)
		if decodeErr != nil {
			p.logBadFrame(ctx, decodeErr)

			continue
		}

		if serveErr := p.serve(ctx, conn, decoded); serveErr != nil {
			return serveErr
		}
	}
}

// writeText sends one printer-to-server message. Printer to server is always
// text JSON with no header, classified by the server on content.
func (p *Printer) writeText(conn *websocket.Conn, payload []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// sendJSON writes one printer-to-server object that is not an event. The block
// request is the only one: the server classifies it by its transfer key and
// not by an event key.
func (p *Printer) sendJSON(conn *websocket.Conn, body map[string]any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode a block request: %w", err)
	}

	return p.writeText(conn, payload)
}

func (p *Printer) beginRegistration(ctx context.Context) (string, error) {
	body := fmt.Sprintf(`{"sn":%q,"fingerprint":%q,"printer_type":%q,"firmware":%q}`,
		p.cfg.Serial, p.cfg.Fingerprint, p.model.DeviceTypeID, p.cfg.Firmware)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+registerPath, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build the registration request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("post %s: %w", registerPath, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("post %s answered %d", registerPath, resp.StatusCode)
	}

	code := resp.Header.Get("Code")
	if code == "" {
		return "", fmt.Errorf("post %s answered 200 with no Code header", registerPath)
	}

	return code, nil
}

// pollRegistration is one GET /p/register. 202 and 204 both mean the operator
// has not decided; 200 carries the Token.
func (p *Printer) pollRegistration(ctx context.Context, code string) (token string, pending bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+registerPath, nil)
	if err != nil {
		return "", false, fmt.Errorf("build the poll request: %w", err)
	}

	req.Header.Set("Code", code)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("get %s: %w", registerPath, err)
	}

	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusAccepted, http.StatusNoContent:
		return "", true, nil
	case http.StatusOK:
		token = resp.Header.Get("Token")
		if token == "" {
			return "", false, fmt.Errorf("get %s answered 200 with no Token header", registerPath)
		}

		return token, false, nil
	default:
		return "", false, fmt.Errorf("get %s answered %d", registerPath, resp.StatusCode)
	}
}

func (p *Printer) websocketURL() (string, error) {
	parsed, err := url.Parse(p.cfg.BaseURL)
	if err != nil {
		return "", fmt.Errorf("parse the base URL %q: %w", p.cfg.BaseURL, err)
	}

	switch parsed.Scheme {
	case "http", "ws":
		parsed.Scheme = "ws"
	case "https", "wss":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("base URL %q has no usable scheme", p.cfg.BaseURL)
	}

	parsed.Path = websocketPath

	return parsed.String(), nil
}

// beginSession adopts a freshly dialled connection. ok is false when the
// printer went offline while the dial was in flight.
func (p *Printer) beginSession(conn *websocket.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.offline {
		return false
	}

	p.conn = conn
	p.sessions++

	select {
	case p.adopted <- struct{}{}:
	default:
	}

	return true
}

func (p *Printer) endSession() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.conn = nil
}

func (p *Printer) logSessionEnd(ctx context.Context, err error) {
	if p.log == nil {
		return
	}

	p.log.DebugContext(ctx, "simulated printer session ended",
		slog.String("fingerprint", p.cfg.Fingerprint),
		slog.String("model", p.model.Name),
		slog.String("error", err.Error()))
}

func (p *Printer) logBadFrame(ctx context.Context, err error) {
	if p.log == nil {
		return
	}

	p.log.WarnContext(ctx, "simulated printer refused a frame",
		slog.String("fingerprint", p.cfg.Fingerprint),
		slog.String("error", err.Error()))
}

// withDefaults fills the zero Config. Every default is a fixed value, so two
// runs of the same test see the same printer.
func withDefaults(cfg Config, model Model) Config {
	cfg.Model = model.Name

	if cfg.Firmware == "" {
		cfg.Firmware = model.Firmware.From
	}

	if cfg.Firmware == "" {
		cfg.Firmware = DefaultFirmware
	}

	if cfg.Serial == "" {
		cfg.Serial = "SIM-" + strings.ToUpper(model.Name)
	}

	if cfg.Fingerprint == "" {
		sum := sha256.Sum256([]byte(model.Name + "/" + cfg.Serial))
		cfg.Fingerprint = strings.ToUpper(hex.EncodeToString(sum[:]))[:fingerprintLength]
	}

	if cfg.MAC == "" {
		cfg.MAC = macFor(cfg.Fingerprint)
	}

	cfg.MAC = strings.ToLower(cfg.MAC)

	if cfg.Storages == nil {
		cfg.Storages = []Storage{{Mountpoint: "/usb", Type: "USB", FreeSpace: defaultFreeSpace}}
	}

	if cfg.Files == nil {
		cfg.Files = defaultTree(cfg.Storages)
	}

	cfg.IdlePeriod = orDuration(cfg.IdlePeriod, defaultIdlePeriod)
	cfg.PrintingPeriod = orDuration(cfg.PrintingPeriod, defaultPrintingPeriod)
	cfg.RegisterPoll = orDuration(cfg.RegisterPoll, defaultRegisterPoll)
	cfg.ReconnectDelay = orDuration(cfg.ReconnectDelay, defaultReconnectDelay)

	return cfg
}

func orDuration(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}

	return value
}
