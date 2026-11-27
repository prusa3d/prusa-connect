package printerpairing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// State is where one pairing stands.
type State string

// The states of a pairing.
const (
	// StateWaiting is a question on the printer's screen that nobody has
	// answered.
	StateWaiting State = "waiting"
	// StateApproved is a person who pressed yes at the printer. The printer
	// is now registering with this server.
	StateApproved State = "approved"
	// StateAdmitted is the printer's registration, approved because of this
	// pairing.
	StateAdmitted State = "admitted"
	// StateRefused is a person who pressed cancel at the printer.
	StateRefused State = "refused"
	// StateExpired is a pairing that ran out of time: nobody answered the
	// question, or the printer did not register after the yes.
	StateExpired State = "expired"
	// StateWithdrawn is a pairing the person who started it cancelled.
	StateWithdrawn State = "withdrawn"
)

// terminal reports a state nothing moves a pairing out of.
func (s State) terminal() bool {
	return s != StateWaiting && s != StateApproved
}

const (
	// QuestionWindow is how long a pairing waits for the person at the
	// printer. The firmware closes its question after 60 seconds, and the rest
	// is the poll that hears it closed.
	QuestionWindow = 75 * time.Second
	// RegistrationWindow is how long after a yes the printer's registration is
	// approved by the pairing. A printer starts registering the moment the
	// person presses yes, so this is time for its network and nothing else.
	RegistrationWindow = 2 * time.Minute
	// keepFinished is how long a pairing that ended can still be read, so a
	// page that was away for a moment learns how it ended.
	keepFinished = 10 * time.Minute
	// MaxOpen is how many pairings may wait at once on this server. A pairing
	// costs a poll a second, and a printer shows one question at a time.
	MaxOpen = 8
	// defaultPollInterval is how often the printer is asked about its question.
	// It is the interval the firmware's own documentation gives a client.
	defaultPollInterval = time.Second
)

// The errors the service reports besides the printer's own.
var (
	// ErrUnknownPairing is an id no pairing on this server answers to.
	ErrUnknownPairing = errors.New("printerpairing: no pairing has that id")
	// ErrTooMany is a server already waiting on MaxOpen pairings.
	ErrTooMany = errors.New("printerpairing: too many pairings are open on this server")
	// ErrOpen is a printer this server already has a pairing open with.
	ErrOpen = errors.New("printerpairing: a pairing with this printer is already open")
	// ErrConnect is an address of this server a printer cannot store or use.
	ErrConnect = errors.New("printerpairing: the printer cannot use this server address")
)

// Pairing is one pairing as a caller reads it.
type Pairing struct {
	ID string
	// Code is what the printer shows. The person compares it with the code
	// on this server's page before pressing yes.
	Code string
	// Printer is the address the pairing was sent to, as host:port.
	Printer string
	// Address is the IP address the request reached. The printer's
	// registration is expected from it.
	Address netip.Addr
	Connect Connect
	State   State
	// StartedBy is the principal who started the pairing, and who the
	// printer's registration is admitted by.
	StartedBy string
	StartedAt time.Time
	// Until is when the pairing stops waiting for the current step. It is
	// zero once the pairing ended.
	Until time.Time
	// Admitted is the registration the pairing approved, once it has.
	Admitted Admitted
}

// Arrival is a registration a printer just made over a connector's wire.
type Arrival struct {
	Connector string
	Code      string
	Serial    string
	Model     string
	// From is the address the registration came from: the remote end of the
	// connection, and never a header the caller wrote.
	From netip.Addr
}

// Admitted is the registration a pairing approved and what it became.
type Admitted struct {
	Connector string
	Code      string
	Serial    string
	Model     string
	DeviceID  string
}

// Admit approves one registration as the principal by, and reports the device
// id the approval bound, empty when it bound none.
type Admit func(ctx context.Context, code, by string) (deviceID string, err error)

// StartRequest is what a person asked for.
type StartRequest struct {
	Printer string
	Connect Connect
	Name    string
	By      string
}

// Options builds a Service.
type Options struct {
	// Client is what reaches the printers. Nil takes NewClient(nil).
	Client *Client
	// Admit approves a registration. Nil leaves every registration to a
	// person, and the pairing then only puts this server's address on the
	// printer.
	Admit  Admit
	Logger *slog.Logger
	// Now is the clock. Nil takes time.Now.
	Now func() time.Time
	// PollInterval is how often a printer is asked. Zero takes a second.
	PollInterval time.Duration
}

// Service holds the pairings this server has open, polls the printers they
// were sent to, and approves the registration a pairing leads to.
//
// It is held in memory. A pairing lasts about a minute and needs the printer
// to answer this process, so a restart ends every pairing and the printer's
// registration then waits for a person, as one made without a pairing does.
type Service struct {
	client   *Client
	admit    Admit
	log      *slog.Logger
	now      func() time.Time
	interval time.Duration

	mu    sync.Mutex
	byID  map[string]*pairing
	group sync.WaitGroup
	// closed is set by Close, after which nothing starts.
	closed bool
}

// pairing is one pairing and what the service needs to move it on.
type pairing struct {
	Pairing

	printer Printer
	// candidate is a registration from the pairing's address that came
	// while the question was still on the screen. It is approved when the
	// printer says yes, and dropped when it says anything else.
	candidate *Arrival
	// claimed is set once a registration was matched to the pairing, so a
	// pairing approves at most one.
	claimed bool
	endedAt time.Time
	stop    context.CancelFunc
}

// NewService returns a service with nothing open.
func NewService(options Options) *Service {
	client := options.Client
	if client == nil {
		client = NewClient(nil)
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	interval := options.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	log := options.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	return &Service{
		client:   client,
		admit:    options.Admit,
		log:      log,
		now:      now,
		interval: interval,
		byID:     make(map[string]*pairing),
	}
}

// Close stops every poll and waits for it to return. A pairing still open
// stays on the printer's screen until the printer times it out.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true

	for _, item := range s.byID {
		if item.stop != nil {
			item.stop()
		}
	}
	s.mu.Unlock()

	s.group.Wait()
}

// Start sends a pairing to a printer and starts polling it.
func (s *Service) Start(ctx context.Context, request StartRequest) (Pairing, error) {
	printer, err := ParsePrinter(request.Printer)
	if err != nil {
		return Pairing{}, err
	}

	if err = checkConnect(request.Connect); err != nil {
		return Pairing{}, err
	}

	if len(request.Name) > MaxNameBytes {
		return Pairing{}, fmt.Errorf("%w: the name is %d bytes and a printer shows at most %d",
			ErrConnect, len(request.Name), MaxNameBytes)
	}

	if err = s.reserve(printer); err != nil {
		return Pairing{}, err
	}

	code, err := NewCode()
	if err != nil {
		return Pairing{}, err
	}

	id, err := newID()
	if err != nil {
		return Pairing{}, err
	}

	reached, err := s.client.Start(ctx, printer, Request{Code: code, Name: request.Name, Connect: request.Connect})
	if err != nil {
		s.log.InfoContext(ctx, "a printer did not take a pairing request",
			"printer", printer.String(), "by", request.By, "error", err.Error())

		return Pairing{}, err
	}

	started := s.now()

	item := &pairing{
		Pairing: Pairing{
			ID:        id,
			Code:      code,
			Printer:   printer.String(),
			Address:   reached,
			Connect:   request.Connect,
			State:     StateWaiting,
			StartedBy: request.By,
			StartedAt: started,
			Until:     started.Add(QuestionWindow),
		},
		printer: printer,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return Pairing{}, fmt.Errorf("%w: the server is stopping", ErrUnreachable)
	}

	// The poll outlives the request that started it, and Close is what ends
	// it.
	pollCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	item.stop = stop
	s.byID[id] = item

	s.group.Go(func() { s.poll(pollCtx, item) })

	s.log.InfoContext(ctx, "a printer was asked to pair",
		"pairing", id, "printer", item.Printer, "address", reached.String(), "by", request.By)

	return item.Pairing, nil
}

// reserve refuses a pairing the server cannot open: one past MaxOpen, or a
// second one to a printer that already has one open.
func (s *Service) reserve(printer Printer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sweepLocked()

	open := 0

	for _, item := range s.byID {
		if item.State.terminal() {
			continue
		}

		open++

		if strings.EqualFold(item.printer.String(), printer.String()) {
			return ErrOpen
		}
	}

	if open >= MaxOpen {
		return ErrTooMany
	}

	return nil
}

// Get reads one pairing.
func (s *Service) Get(id string) (Pairing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sweepLocked()

	item, ok := s.byID[id]
	if !ok {
		return Pairing{}, fmt.Errorf("%w: %s", ErrUnknownPairing, id)
	}

	s.expireLocked(item)

	return item.Pairing, nil
}

// Cancel withdraws one pairing. A question still on the screen is taken off
// it, and a printer that already said yes is no longer approved by the
// pairing: its registration waits for a person. A pairing that already ended
// is returned as it ended.
func (s *Service) Cancel(ctx context.Context, id string) (Pairing, error) {
	s.mu.Lock()

	item, ok := s.byID[id]
	if !ok {
		s.mu.Unlock()

		return Pairing{}, fmt.Errorf("%w: %s", ErrUnknownPairing, id)
	}

	s.expireLocked(item)

	if item.State.terminal() {
		snapshot := item.Pairing
		s.mu.Unlock()

		return snapshot, nil
	}

	asked := item.State == StateWaiting

	s.endLocked(item, StateWithdrawn)
	snapshot, printer := item.Pairing, item.printer
	s.mu.Unlock()

	if asked {
		if err := s.client.Withdraw(ctx, printer, snapshot.Code); err != nil {
			// The question closes on its own when the printer times it out,
			// and the pairing is withdrawn here whatever the printer said.
			s.log.WarnContext(ctx, "a printer did not take a pairing question off its screen",
				"pairing", id, "printer", snapshot.Printer, "error", err.Error())
		}
	}

	return snapshot, nil
}

// Arrived is told about every registration a printer makes over a connector
// wire. It reports whether the registration belongs to an open pairing.
//
// A registration belongs to a pairing when it comes from the address the
// pairing reached, while the pairing waits, and before any other registration
// was matched to it. One that comes before the printer's yes is held, and is
// approved only when the printer says yes. See ADR-20261126-qca.
func (s *Service) Arrived(ctx context.Context, arrival Arrival) bool {
	if !arrival.From.IsValid() || arrival.Code == "" {
		return false
	}

	from := arrival.From.Unmap()

	s.mu.Lock()

	var match *pairing

	for _, item := range s.byID {
		s.expireLocked(item)

		if item.State.terminal() || item.claimed || item.Address != from {
			continue
		}

		match = item

		break
	}

	if match == nil {
		s.mu.Unlock()

		return false
	}

	match.claimed = true

	if match.State == StateWaiting {
		held := arrival
		match.candidate = &held
		s.mu.Unlock()

		s.log.InfoContext(ctx, "a registration is held until the printer answers its pairing question",
			"pairing", match.ID, "registration", arrival.Code, "connector", arrival.Connector)

		return true
	}

	s.mu.Unlock()

	s.approve(ctx, match.ID, arrival)

	return true
}

// approve admits a registration matched to an approved pairing.
func (s *Service) approve(ctx context.Context, id string, arrival Arrival) {
	s.mu.Lock()

	item, ok := s.byID[id]
	if !ok || item.State != StateApproved || s.admit == nil {
		s.mu.Unlock()

		return
	}

	by := item.StartedBy
	s.mu.Unlock()

	deviceID, err := s.admit(ctx, arrival.Code, by)
	if err != nil {
		// The registration stays where it is, waiting for a person, which is
		// what a registration with no pairing does.
		s.log.ErrorContext(ctx, "a paired printer's registration could not be approved",
			"pairing", id, "registration", arrival.Code, "connector", arrival.Connector, "error", err.Error())

		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if item.State != StateApproved {
		return
	}

	item.Admitted = Admitted{
		Connector: arrival.Connector,
		Code:      arrival.Code,
		Serial:    arrival.Serial,
		Model:     arrival.Model,
		DeviceID:  deviceID,
	}
	s.endLocked(item, StateAdmitted)

	s.log.InfoContext(ctx, "a paired printer's registration was approved",
		"pairing", id, "registration", arrival.Code, "connector", arrival.Connector,
		"device_id", deviceID, "by", by)
}

// poll asks the printer about the question until it is answered, withdrawn or
// out of time.
func (s *Service) poll(ctx context.Context, item *pairing) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if !s.Step(ctx, item.ID) {
			return
		}
	}
}

// Step asks the printer once about one pairing's question and moves the
// pairing on. It reports whether the question still waits, which is when the
// poll goes on. The poll loop calls it on a timer, and a test calls it
// directly.
func (s *Service) Step(ctx context.Context, id string) bool {
	s.mu.Lock()

	item, ok := s.byID[id]
	if !ok {
		s.mu.Unlock()

		return false
	}

	s.expireLocked(item)

	if item.State != StateWaiting {
		s.mu.Unlock()

		return false
	}

	printer, code := item.printer, item.Code
	s.mu.Unlock()

	answer, err := s.client.Poll(ctx, printer, code)
	if err != nil {
		// A printer that missed one poll is asked again. The question window
		// still bounds how long.
		s.log.DebugContext(ctx, "a printer did not answer a pairing poll", "pairing", id, "error", err.Error())

		return true
	}

	s.mu.Lock()

	if item.State != StateWaiting {
		s.mu.Unlock()

		return false
	}

	switch answer {
	case Waiting:
		s.mu.Unlock()

		return true
	case Approved:
		item.State = StateApproved
		item.Until = s.now().Add(RegistrationWindow)
		held := item.candidate
		item.candidate = nil
		s.mu.Unlock()

		s.log.InfoContext(ctx, "a person at the printer accepted a pairing", "pairing", id)

		if held != nil {
			s.approve(ctx, id, *held)
		}

		return false
	case Refused:
		s.endLocked(item, StateRefused)
	case Unknown:
		// The question timed out on the printer, or an approval was answered
		// to a poll whose answer never arrived. The printer says yes once, so
		// the two cannot be told apart, and the pairing approves nothing.
		s.endLocked(item, StateExpired)
	}

	s.mu.Unlock()

	return false
}

// expireLocked ends a pairing whose window has passed. The caller holds the
// lock.
func (s *Service) expireLocked(item *pairing) {
	if item.State.terminal() || s.now().Before(item.Until) {
		return
	}

	s.endLocked(item, StateExpired)
}

// endLocked moves a pairing into a state it does not leave. The caller holds
// the lock.
func (s *Service) endLocked(item *pairing, state State) {
	item.State = state
	item.Until = time.Time{}
	item.candidate = nil
	item.endedAt = s.now()

	if item.stop != nil {
		item.stop()
	}
}

// sweepLocked forgets the pairings that ended long enough ago. The caller
// holds the lock.
func (s *Service) sweepLocked() {
	cutoff := s.now().Add(-keepFinished)

	for id, item := range s.byID {
		s.expireLocked(item)

		if item.State.terminal() && item.endedAt.Before(cutoff) {
			delete(s.byID, id)
		}
	}
}

// checkConnect refuses an address of this server a printer cannot store or
// cannot use. The firmware refuses the same values with 422. Refusing them
// here says which one.
func checkConnect(connect Connect) error {
	host := strings.TrimSpace(connect.Host)

	switch {
	case host == "":
		return fmt.Errorf("%w: no host", ErrConnect)
	case len(host) > MaxConnectHostBytes:
		return fmt.Errorf("%w: the host is %d characters and a printer takes at most %d",
			ErrConnect, len(host), MaxConnectHostBytes)
	case strings.ContainsAny(host, "/:@ \t"):
		return fmt.Errorf("%w: %q is not a bare host name or IPv4 address", ErrConnect, host)
	case loopbackHost(host):
		return fmt.Errorf("%w: %q is the printer itself when the printer dials it", ErrConnect, host)
	case connect.Port < 1 || connect.Port > 65535:
		return fmt.Errorf("%w: port %d", ErrConnect, connect.Port)
	}

	return nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && (ip.IsLoopback() || ip.IsUnspecified())
}
