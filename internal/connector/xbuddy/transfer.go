package xbuddy

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The transfer lifecycle events. The device volunteers these: they carry a
// transfer id and no command id, so they answer no command and reach observe
// rather than answer.
//
// TRANSFER_FINISHED is the only one that means the file is on the device. The
// other three are the ways it stops without arriving, and they are separate
// words on the wire rather than one because the device distinguishes an
// operator stopping a transfer from one that broke
// (legacy/connector/xbuddy/connection.go:537-538).
const (
	eventTransferInfo     = "TRANSFER_INFO"
	eventTransferFinished = "TRANSFER_FINISHED"
	eventTransferStopped  = "TRANSFER_STOPPED"
	eventTransferAborted  = "TRANSFER_ABORTED"
	eventTransferFailed   = "TRANSFER_FAILED"
)

const (
	// teamID is the tenant a transfer belongs to. This server has no team
	// concept, and the argument is required, so every transfer is team zero.
	// The shipping server does the same
	// (legacy/transport/httpapi/handlers.go:1703).
	teamID = 0

	// hashBytes is how many random bytes name a transfer. The device echoes the
	// value back on its first block request and never interprets it, so this is
	// a correlation token and not a checksum: the shipping server puts an MD5 of
	// a URL here, which no device could verify against the bytes it received.
	// Sixteen bytes is what the firmware's 29-character bound was written for.
	hashBytes = 16

	// maxChunkBytes bounds the buffer one block request may ask this host to
	// hold. The device asks for 4 KiB, and the bound is here because the number
	// arrives from firmware this build does not control.
	maxChunkBytes = 1 << 20

	// defaultChunkWriteCeiling bounds one chunk frame's payload, whatever chunk
	// size the device asked for, for a host that names no ceiling of its own.
	//
	// 1011 puts exactly 1024 bytes on the TLS connection per write, once the
	// nine-byte T header and the four-byte WebSocket header are counted, and Go
	// emits one TLS record per write. 1024 is the record buffer Buddy firmware
	// declares (MBEDTLS_SSL_IN_CONTENT_LEN), and it advertises the number
	// nowhere, so a host terminating the printers' TLS has to know it.
	//
	// Earlier research carries the measurements, what the enforced edge
	// turned out to be, and why the bench that raised this to 4096 never saw
	// the problem. transferceiling_test.go fails if the write grows past 1024.
	defaultChunkWriteCeiling = 1011

	// stopTransferBudget bounds telling the device to abandon a transfer nobody
	// is waiting for. It is generous because the command queues for a printer
	// that is away, which is the case worth waiting through, and it is finite
	// because nothing is waiting for the answer.
	stopTransferBudget = 5 * time.Minute

	// blockBacklog is how many block requests may wait for the goroutine
	// serving them. The device asks for the next range after the last one
	// arrived, so one is the ordinary depth and the rest absorbs a retry the
	// device sent while a block was still being written.
	blockBacklog = 4

	// progressStep is how many bytes a range is written between two reports.
	// Buddy firmware asks for the whole file in one range, so a report only at
	// its end left a host at 0 % for the length of the file.
	progressStep = 64 << 10
)

// transferMaximum is the first size the wire cannot express: the length and the
// id are uint32, so a file of 4 GiB has no orig_size.
const transferMaximum = 1 << 32

var _ connectorapi.Transfers = (*Session)(nil)

// transfer is one file this host has offered and the device has not finished
// fetching.
//
// A session holds at most one. The device runs one transfer at a time and
// starting a second abandons the first
// (legacy/connector/xbuddy/connection.go:1146-1157), so a second offer is
// refused here rather than silently destroying a transfer already in flight.
type transfer struct {
	hash    string
	path    string
	content connectorapi.Content

	// fileID is what the device addresses this attempt's chunks to, learned
	// from the block request that opened it. bound says it has been learned.
	//
	// It is not the transfer id. The device mints two numbers for one transfer
	// and they are different: measured on an MK4S at 6.5.7, where file_id was
	// 2116845183 and the transfer_id on the same transfer's events was
	// 1500973180.
	fileID uint32
	bound  bool

	// transferID is what the device calls this transfer in the events that end
	// it, learned from the TRANSFER_INFO that answers the start command.
	// known is false for a firmware that answers without one.
	transferID int64
	known      bool

	// blocks carries a request from the read goroutine to the one serving them.
	// It is bounded, and a request that will not fit is dropped: the device
	// retries a block request, so dropping one costs a retry.
	blocks chan queuedBlock

	// done carries exactly one outcome and is buffered, so whoever settles the
	// transfer never blocks on a caller that has already given up.
	done chan error
	once sync.Once

	// stop ends the goroutine serving blocks.
	stop chan struct{}
	// serving starts that goroutine once, however many times the device opens
	// the transfer.
	serving sync.Once

	// served is how many bytes this host has put on the wire for this transfer.
	// It is what separates a transfer the device never asked about from one it
	// abandoned part way.
	served atomic.Int64

	// covered is how many distinct bytes have been served, and what a report
	// says was sent. A Buddy board asks for the end of a G-code file first and
	// may ask for a range again, so neither the furthest byte nor the bytes
	// served measure progress. Only the goroutine serving blocks writes it.
	covered atomic.Int64
	spans   coverage

	// progress is the host's callback, nil for a caller that wants no reports,
	// and total is the length the device was told to expect.
	progress func(connectorapi.Progress)
	total    int64

	// reportMu serialises the reports and guards the two fields under it.
	//
	// It is held across the callback rather than released before it. The
	// contract promises a host that its callback is never entered twice at once,
	// and three goroutines report: the one that started the transfer, the one
	// serving blocks, and the one reading the socket. A lock dropped before the
	// call would let two of them overtake each other and deliver a stale figure
	// last. It is a lock of its own and nothing else takes it, so a host that
	// blocks inside its callback cannot deadlock against s.mu.
	reportMu      sync.Mutex
	received      int64
	receivedKnown bool

	// listingFloor is the stamp a remembered listing has to carry to be newer
	// than this transfer. The readback's fallback recalls nothing older, because
	// a listing taken before the file was written says nothing about it.
	listingFloor uint64

	// warning is what the driver could not establish about a transfer the
	// printer reported finished, and it rides out on the last report. It is
	// under reportMu because that is the lock the report is written under.
	warning string
}

// report tells the host how far the transfer has got, carrying whatever the
// device last said about its own side.
func (t *transfer) report(sent int64) {
	t.reportMu.Lock()
	defer t.reportMu.Unlock()

	t.emit(sent)
}

// recordReceived stores the device's own count and reports it. The count is
// kept because the device volunteers it rarely and every report after it should
// still carry the newest figure there is.
func (t *transfer) recordReceived(received int64) {
	t.reportMu.Lock()
	defer t.reportMu.Unlock()

	t.received, t.receivedKnown = received, true

	t.emit(t.covered.Load())
}

// emit calls the host's callback. reportMu is held.
func (t *transfer) emit(sent int64) {
	if t.progress == nil {
		return
	}

	t.progress(connectorapi.Progress{
		Total:    t.total,
		Sent:     sent,
		Received: t.received,
		Known:    t.receivedKnown,
		Ref:      t.hash,
		Warning:  t.warning,
	})
}

// warn records what this driver could not establish about the transfer, so the
// report that follows carries it to the host.
func (t *transfer) warn(note string) {
	t.reportMu.Lock()
	defer t.reportMu.Unlock()

	t.warning = note
}

// queuedBlock is one block request and the connection it arrived on.
//
// The connection is carried because a transfer outlives one and the file id does
// not: the device names the transfer again on every connection, with an id it has
// not used before. A request queued on a connection that has since gone names an
// id the device has retired, and answering it on the connection that replaced it
// puts a frame the firmware dislikes in front of a board that resets within
// twelve milliseconds of one, exactly while it is opening the transfer again.
type queuedBlock struct {
	request block
	arrived *link
}

// settle records the one outcome of a transfer.
func (t *transfer) settle(err error) {
	t.once.Do(func() {
		t.done <- err
		close(t.stop)
	})
}

// Send puts one file on the device and returns when it is there.
//
// The device pulls. This puts START_INLINE_DOWNLOAD on the wire naming a token
// and a length, and the bytes leave later, as T frames, when the device asks
// for the ranges it wants. So the command completing means the device accepted
// the transfer and not that the file arrived: what says that is
// TRANSFER_FINISHED, which is what this waits for.
func (s *Session) Send(ctx context.Context, up connectorapi.Upload) error {
	// Both halves are checked and only the generic one is named. The printer
	// has to declare that it can be sent a file and that it speaks the command
	// this dialect sends, and a caller can act on neither by learning that a
	// Prusa board calls it START_INLINE_DOWNLOAD.
	//
	// The second half is read from sends and not from caps. The transfer start
	// is deliberately absent from the published baseline, and the host resolves
	// caps against that baseline, so a session that looked for it there would
	// refuse every file on every printer. See Driver.sendable.
	if !s.caps.Has(connectorapi.VerbFileUpload) || !s.sends.Has(VerbStartInlineDownload) {
		return connectorapi.CapabilityNotSupported.
			New("the printer cannot be sent a file").
			With(fault.StringParam("verb", connectorapi.VerbFileUpload.String()))
	}

	if refusal := connectorapi.RefusePathEscape(up.Path); refusal != nil {
		return refusal
	}

	size := up.Content.Size()
	if size < 0 || size >= transferMaximum {
		return connectorapi.TransferTooLarge.
			New("the file is longer than a transfer on this wire can describe").
			With(
				fault.StringParam("size", strconv.FormatInt(size, 10)),
				fault.StringParam("limit_bytes", strconv.FormatInt(transferMaximum-1, 10)),
			)
	}

	pending, err := s.beginTransfer(up)
	if err != nil {
		return err
	}

	defer s.endTransfer(pending)

	s.logInfo(ctx, "offering the printer a file",
		slog.String("path", pending.path),
		slog.Int64("size", size),
		slog.String("transfer", pending.hash))

	if startErr := s.startTransfer(ctx, pending, size); startErr != nil {
		// The machine's own words for the refusal travel as params on the
		// fault, and the wrapped chain says only that it refused. Without them
		// a log line names no cause an operator can act on.
		s.logInfo(ctx, "the printer would not take the file",
			slog.String("path", pending.path),
			slog.String("machine_reason", paramOf(startErr, "machine_reason")),
			slog.String("reason", paramOf(startErr, "reason")),
			slog.String("error", startErr.Error()))

		// A start that reached the printer and went unanswered, or that the
		// caller stopped waiting for, may have opened the transfer there, and
		// nothing here serves it once Send returns. A Buddy board left like that
		// shows "Downloading 0 %" and resumes the transfer at every boot.
		if startMayHaveOpened(startErr) {
			s.stopTransfer(ctx, pending)
		}

		return startErr
	}

	s.logInfo(ctx, "the printer accepted the file and will fetch it",
		slog.String("path", pending.path), slog.String("transfer", pending.hash))

	pending.report(0)

	err = s.awaitTransfer(ctx, pending)

	// The printer saying the transfer finished is the printer's own word, and
	// BUG-014 is what it is worth on a FAT storage: two files whose long names
	// shorten to one 8.3 spelling were both reported finished and only the
	// first was on the stick. So the file is confirmed in the printer's own
	// listing before this returns.
	if err == nil {
		err = s.confirmLanding(ctx, pending)
	}

	// The last report, whichever way the transfer ended. A host that gave up or
	// was refused still learns how far the file got.
	pending.report(pending.covered.Load())

	s.logInfo(ctx, "the transfer ended",
		slog.String("path", pending.path),
		slog.String("transfer", pending.hash),
		slog.Bool("arrived", err == nil),
		slog.Int64("served", pending.served.Load()))

	return err
}

// startTransfer puts the command on the wire that tells the device to come and
// fetch the file.
func (s *Session) startTransfer(ctx context.Context, pending *transfer, size int64) error {
	result, err := s.Do(ctx, connectorapi.Command{
		Verb: VerbStartInlineDownload,
		Args: map[string]string{
			"path":      pending.path,
			"hash":      pending.hash,
			"team_id":   strconv.Itoa(teamID),
			"orig_size": strconv.FormatInt(size, 10),
		},
	})
	if err != nil {
		return err
	}

	if id, carried := transferIDOf(result.Raw); carried {
		s.rememberTransferID(pending, id)
	}

	if !result.OK {
		// Both words, because this firmware does not always send the machine one
		// (ADR-20260826-ezr rule 5, and see Session.manage, where a delete aimed at the
		// wrong kind of entry comes back with a human sentence and nothing else).
		// The machine reason stays the field to branch on and the human one is
		// there to show.
		return connectorapi.TransferRefused.
			New("the printer would not take the file").
			With(
				fault.StringParam("path", pending.path),
				fault.StringParam("machine_reason", result.MachineReason),
				fault.StringParam("reason", humanReason(result.Raw)),
			).
			Wrap(connectorapi.ErrTransferRefused)
	}

	return nil
}

// paramOf reads one string param off a fault, empty where the error carries
// none.
func paramOf(err error, key string) string {
	var carried *fault.Fault
	if !errors.As(err, &carried) {
		return ""
	}

	for _, param := range carried.Params() {
		if param.Key == key {
			return param.Value.String()
		}
	}

	return ""
}

// rememberTransferID records what the device calls this transfer in its events.
func (s *Session) rememberTransferID(pending *transfer, id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending.transferID = id
	pending.known = true
}

// transferIDOf reads the transfer id out of the event that answered the start
// command. It is the only place the device says which number its TRANSFER_
// events will carry, and it is not the file id the chunks are addressed to.
func transferIDOf(raw json.RawMessage) (int64, bool) {
	var event printerEvent
	if err := json.Unmarshal(raw, &event); err != nil || event.TransferID == nil {
		return 0, false
	}

	return *event.TransferID, true
}

// awaitTransfer waits for the device to say the file arrived, for it to say it
// did not, or for the caller to give up.
//
// A caller that gives up leaves the device fetching a file nobody is waiting
// for, so the transfer is stopped on the way out. It is best effort: the device
// may have finished already, and a stop it refuses changes nothing here.
func (s *Session) awaitTransfer(ctx context.Context, pending *transfer) error {
	select {
	case err := <-pending.done:
		return err
	case <-ctx.Done():
		s.stopTransfer(ctx, pending)

		return fmt.Errorf("xbuddy: sending %s: %w", pending.path, ctx.Err())
	case <-s.done:
		return fmt.Errorf("xbuddy: sending %s: %w", pending.path, ErrSessionClosed)
	}
}

// beginTransfer files the offer so that the device's first block request can
// find it. It is what makes one transfer at a time true by construction.
func (s *Session) beginTransfer(up connectorapi.Upload) (*transfer, error) {
	hash, err := mintTransferHash()
	if err != nil {
		return nil, err
	}

	pending := &transfer{
		hash:     hash,
		path:     up.Path,
		content:  up.Content,
		progress: up.Progress,
		total:    up.Content.Size(),
		blocks:   make(chan queuedBlock, blockBacklog),
		done:     make(chan error, 1),
		stop:     make(chan struct{}),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.transfer != nil {
		return nil, connectorapi.TransferBusy.
			New("the printer is already fetching a file").
			With(fault.StringParam("path", up.Path)).
			Wrap(connectorapi.ErrTransferBusy)
	}

	s.transfer = pending
	pending.listingFloor = s.listings + 1

	s.transferMovedNow()

	return pending, nil
}

// endTransfer drops the offer, so a block request that arrives afterwards finds
// nothing rather than a caller that has gone.
func (s *Session) endTransfer(pending *transfer) {
	s.mu.Lock()

	if s.transfer == pending {
		s.transfer = nil
	}

	// The id this attempt was known by outlives the attempt. A caller that gave
	// up leaves the device stopping a transfer, and its TRANSFER_STOPPED can
	// arrive after the next file has been offered.
	if pending.known {
		s.retiredTransfer, s.retiredKnown = pending.transferID, true
	}

	s.mu.Unlock()

	s.transferMovedNow()

	pending.settle(nil)
}

// mintTransferHash returns the token a transfer is named by on the wire.
func mintTransferHash() (string, error) {
	raw := make([]byte, hashBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("xbuddy: naming a transfer: %w", err)
	}

	return base64.StdEncoding.EncodeToString(raw), nil
}

// serveBlockRequest answers the device asking for a range of the file it is
// fetching.
//
// It runs on the read goroutine, so it does no I/O of its own: it finds the
// transfer the request belongs to and hands the request to the goroutine that
// writes the bytes. A read goroutine that streamed a file would be a printer
// whose telemetry and command results stopped for the length of a transfer.
func (s *Session) serveBlockRequest(ctx context.Context, current *link, message inboundMessage) {
	request, err := readBlockRequest(message.fields)
	if err != nil {
		// Counted, because it is a frame this dialect could not read: the frame
		// classified as a block request and the numbers in it have no reading
		// here, so a firmware that spells the range differently is what makes
		// this counter move. A healthy retry is a different frame and is not
		// counted anywhere; see Session.Refused.
		s.refuse(ctx, current, "the printer asked for a file block naming a range this dialect cannot read",
			slog.String("error", err.Error()))

		return
	}

	pending, found := s.transferFor(ctx, request)
	if !found {
		// The device is fetching something this host is not holding: a transfer
		// whose caller gave up, or one this session never started.
		//
		// No chunk is sent back. An empty chunk is the wire's failure signal and
		// it is answered here with silence instead, because a Buddy board that
		// receives one for a transfer it is starting closes the connection
		// within ten milliseconds: measured on an MK4S at 6.5.7, where every
		// reconnection was answered with an empty block and ended the same way.
		// Silence costs the device a retry, which is what it already does. What
		// ends the transfer is STOP_TRANSFER, a command and not a chunk: see
		// stopStrayTransfer.
		s.logDebug(ctx, "the printer asked for a block of a file this server is not holding",
			slog.Uint64("file_id", uint64(request.fileID)),
			slog.Bool("opens_a_transfer", request.initial),
			slog.String("transfer", request.hash),
			slog.Int64("team_id", request.teamID),
			slog.String("holding", s.transferSummary()))

		s.stopStrayTransfer(ctx, current, request)

		return
	}

	s.logDebug(ctx, "the printer asked for a file block",
		slog.Uint64("file_id", uint64(request.fileID)),
		slog.Bool("opens_a_transfer", request.initial),
		slog.Int64("start", request.start),
		slog.Int64("end", request.end),
		slog.Int("chunk", request.chunk))

	select {
	case pending.blocks <- queuedBlock{request: request, arrived: current}:
	default:
		// The device retries a block request, so dropping one costs a retry. It
		// is not a refusal: this dialect read the frame and has the bytes, and a
		// counter that rose here would rise on a healthy transfer.
		s.logDebug(ctx, "a file block request was dropped because the one before it is still being served",
			slog.Uint64("file_id", uint64(request.fileID)))
	}
}

// transferSummary is what this session is holding, for the log line that
// reports a block request nothing answers. It names the transfer and not the
// file, because the token is what the two sides match on.
func (s *Session) transferSummary() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.transfer == nil {
		return "nothing"
	}

	if !s.transfer.bound {
		return s.transfer.hash + " (unbound)"
	}

	return s.transfer.hash
}

// transferFor finds the transfer a block request belongs to, and binds a
// transfer to the id the device gave it on the first request.
//
// The first request of a transfer carries the token this host minted, and every
// request after it carries only the id the device chose. So the first is what
// names the transfer and the rest are matched against that name.
func (s *Session) transferFor(ctx context.Context, request block) (*transfer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending := s.transfer
	if pending == nil {
		return nil, false
	}

	if !request.initial {
		if !pending.bound || pending.fileID != request.fileID {
			return nil, false
		}

		return pending, true
	}

	if request.hash != pending.hash || request.teamID != teamID {
		return nil, false
	}

	// The device names a transfer again every time it opens one, and the name
	// it chooses is not the one it chose before: an MK4S at 6.5.7 resets its
	// connection within a hundred milliseconds of accepting a transfer and asks
	// again with a fresh file id, three times over. So the token is the
	// transfer's identity and the file id is only what this attempt's chunks
	// are addressed to. Binding the id once and refusing the rest is a transfer
	// that can never survive the reconnection the device itself begins with.
	pending.fileID = request.fileID
	pending.bound = true

	pending.serving.Do(func() { go s.serveBlocks(ctx, pending) })

	return pending, true
}

// serveBlocks writes the ranges one transfer is asked for, one at a time.
//
// One goroutine per transfer is what keeps the device's own ordering: a range
// is written whole before the next is started, and two goroutines interleaving
// chunks of different ranges would be a file assembled out of order.
//
// The context is detached from the connection the first request arrived on,
// because a transfer outlives it: this firmware resets its connection mid
// transfer and asks again on the next one. It is cancelled when the transfer
// settles or the session closes, so a Content that reads from somewhere slow is
// not left reading for a caller that has gone.
func (s *Session) serveBlocks(parent context.Context, pending *transfer) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	defer cancel()

	// The loop below cannot cancel while it is blocked inside Content.Open, and
	// that is the case the cancellation is for. This ends with ctx, which the
	// deferred cancel closes.
	go func() {
		select {
		case <-pending.stop:
		case <-s.done:
		case <-ctx.Done():
		}

		cancel()
	}()

	buffer := make([]byte, 0)

	for {
		select {
		case <-pending.stop:
			return
		case <-s.done:
			return
		case queued := <-pending.blocks:
			buffer = s.serveOneBlock(ctx, pending, queued, buffer)
		}
	}
}

// serveOneBlock writes one requested range and returns the buffer it used, so
// the next range reuses it. Nothing here allocates in proportion to the file's
// length: the buffer is one chunk, whatever the file is.
func (s *Session) serveOneBlock(
	ctx context.Context, pending *transfer, queued queuedBlock, buffer []byte,
) []byte {
	request := queued.request

	want := headerLength + request.chunk
	if cap(buffer) < want {
		buffer = make([]byte, want)
	}

	buffer = buffer[:want]

	current, live := s.linkFor(pending, queued)
	if !live {
		s.logDebug(ctx, "a file block request was dropped because the printer has retired it",
			slog.Uint64("file_id", uint64(request.fileID)))

		return buffer
	}

	before := pending.served.Load()

	if err := s.streamBlock(ctx, current, pending, request, buffer); err != nil {
		s.logDebug(ctx, "a file block could not be served",
			slog.Uint64("file_id", uint64(request.fileID)), slog.String("error", err.Error()))

		// Nothing is sent for a transfer that is already over, and ctx ending is
		// what says it is. An empty chunk is the wire's failure signal, and a
		// Buddy board that receives one closes the connection within ten
		// milliseconds: telling a device that a block is not coming is worth a
		// reconnection, and telling it about a transfer it has already been told
		// to stop is not.
		if ctx.Err() == nil {
			s.writeEmptyBlock(ctx, current, request.fileID)
		}

		return buffer
	}

	s.logDebug(ctx, "a file block was served",
		slog.Uint64("file_id", uint64(request.fileID)),
		slog.Int64("start", request.start),
		slog.Int64("end", request.end),
		slog.Int64("wrote", pending.served.Load()-before))

	// The bytes past the last step of the range.
	pending.report(pending.covered.Load())

	return buffer
}

// streamBlock reads the requested range and writes it as chunks.
//
// End is inclusive, which is what the shipping streamer reads it as. A device
// that asked for more than the file holds is answered with what there is: the
// short read ends the range, and the device asks for nothing past the length it
// was told.
func (s *Session) streamBlock(
	ctx context.Context, current *link, pending *transfer, request block, buffer []byte,
) error {
	reader, err := pending.content.Open(ctx, request.start)
	if err != nil {
		return fmt.Errorf("open %s at %d: %w", pending.path, request.start, err)
	}

	// A read handle's Close reports nothing a caller can act on: the bytes it
	// held have either reached the wire already or the write below failed and
	// said so.
	defer func() { _ = reader.Close() }()

	header := writeTransferHeader(buffer, request.fileID)
	remaining := request.end - request.start + 1

	// Once a step, not once a chunk. A chunk is a kilobyte and a host that
	// redraws on every one of them is redrawing thousands of times a file.
	unreported := int64(0)

	for remaining > 0 {
		select {
		case <-pending.stop:
			return nil
		case <-s.done:
			return nil
		default:
		}

		room := min(int64(len(buffer)-header), remaining, int64(s.chunkCeiling))

		read, readErr := reader.Read(buffer[header : header+int(room)])
		if read > 0 {
			if writeErr := current.writeBinary(buffer[:header+read]); writeErr != nil {
				return fmt.Errorf("write a chunk of %s: %w", pending.path, writeErr)
			}

			pending.served.Add(int64(read))

			from := request.end + 1 - remaining
			remaining -= int64(read)

			pending.covered.Store(pending.spans.add(from, from+int64(read)))

			unreported += int64(read)
			if unreported >= progressStep {
				pending.report(pending.covered.Load())

				unreported = 0
			}
		}

		if errors.Is(readErr, io.EOF) {
			return nil
		}

		if readErr != nil {
			return fmt.Errorf("read %s: %w", pending.path, readErr)
		}
	}

	return nil
}

// writeEmptyBlock is how this host tells a device a block cannot be served. It
// is a chunk with no bytes in it, which is what the shipping streamer sends on
// an error (legacy/connector/xbuddy/transfer/transfer.go:114-118).
func (s *Session) writeEmptyBlock(ctx context.Context, current *link, fileID uint32) {
	frame := make([]byte, headerLength)
	frame = frame[:writeTransferHeader(frame, fileID)]

	if err := current.writeBinary(frame); err != nil {
		s.logDebug(ctx, "the printer was not told that a block could not be served",
			slog.String("error", err.Error()))
	}
}

// linkFor is the connection a queued block request may still be answered on,
// and false when it may not be answered at all.
//
// A transfer does not wait for a printer to come back: the device asks again
// when it reconnects, with an id it has not used before. So a request is
// answered only on the connection it arrived on and only while the transfer is
// still addressed to the id it names. Anything else is a range the device
// retired, and writing it would send the old id to the connection that replaced
// the old one.
func (s *Session) linkFor(pending *transfer, queued queuedBlock) (*link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current == nil || s.current != queued.arrived {
		return nil, false
	}

	if !pending.bound || pending.fileID != queued.request.fileID {
		return nil, false
	}

	return s.current, true
}

// observeTransfer reads what a lifecycle event says about the transfer this
// session is running.
//
// The device volunteers these with a transfer id and no command id, so nothing
// is waiting on them as a command result and this is where a transfer ends.
func (s *Session) observeTransfer(event printerEvent) {
	switch event.Event {
	case eventTransferFinished, eventTransferStopped, eventTransferAborted, eventTransferFailed:
		s.finishTransfer(event)
	case eventTransferInfo:
		s.observeTransferInfo(event)
	}
}

// observeTransferInfo records what the device says it has taken of the file it
// is fetching.
//
// It runs on the read goroutine, and the host's callback runs with it, which is
// the reason the contract says a host that blocks inside its callback blocks the
// transfer.
func (s *Session) observeTransferInfo(event printerEvent) {
	received, carried := receivedOf(event.Data)
	if !carried {
		return
	}

	s.mu.Lock()
	pending := s.transfer
	s.mu.Unlock()

	if pending == nil {
		return
	}

	// An event naming a transfer this session is not running is about the one
	// before it: the device keeps counting for a transfer whose caller has gone,
	// and its figure would be read as the new transfer's.
	if event.TransferID != nil && pending.known && *event.TransferID != pending.transferID {
		return
	}

	pending.recordReceived(received)
}

// receivedOf reads how many bytes the device says it has taken out of a
// TRANSFER_INFO payload.
//
// The member is "transferred": the board counts it under that name
// (internal/printer/simulator/transfer.go:509) and the shipping server reads the
// same member (legacy/printer/state/printer_state.go:1212). It is read as a
// float as well as an integer because the wire is JSON and a firmware is free to
// send 1024.0. A payload with no readable number leaves the host's last figure
// alone: the device volunteering nothing is not a transfer that went wrong.
func receivedOf(data json.RawMessage) (int64, bool) {
	if len(data) == 0 {
		return 0, false
	}

	var carried struct {
		Transferred *json.Number `json:"transferred"`
	}

	if err := json.Unmarshal(data, &carried); err != nil || carried.Transferred == nil {
		return 0, false
	}

	received, err := carried.Transferred.Int64()
	if err != nil {
		asFloat, floatErr := carried.Transferred.Float64()
		if floatErr != nil {
			return 0, false
		}

		received = int64(asFloat)
	}

	if received < 0 {
		return 0, false
	}

	return received, true
}

// transferOutcome is what a terminal event says happened, as the caller of Send
// receives it.
//
// The event name is carried because the device distinguishes an operator
// stopping a transfer from one that broke, and both of the device's own words for
// why ride with it: the machine one is what a caller branches on and the human
// one is what it shows, and neither is invented here (ADR-20260826-ezr rule 5).
func transferOutcome(event printerEvent, pending *transfer) error {
	if event.Event == eventTransferFinished {
		return nil
	}

	return connectorapi.TransferFailed.
		New("the transfer did not finish").
		With(
			fault.StringParam("path", pending.path),
			fault.StringParam("event", event.Event),
			fault.StringParam("machine_reason", event.MachineReason),
			fault.StringParam("reason", event.Reason),
		).
		Wrap(connectorapi.ErrTransferFailed)
}

// finishTransfer settles the transfer an event names, and nothing when the
// event belongs to another one.
func (s *Session) finishTransfer(event printerEvent) {
	s.mu.Lock()
	pending := s.transfer
	s.mu.Unlock()

	if pending == nil {
		return
	}

	if !s.endsTheTransfer(event, pending) {
		return
	}

	pending.settle(transferOutcome(event, pending))
}

// endsTheTransfer reports whether a terminal event is about the transfer this
// session is running.
//
// An event carrying no transfer id is: the firmware this port was written
// against carries one, and a firmware that does not has still said the transfer
// it is running ended, and a session runs one. An id is matched against what the
// device gave when it accepted the command, never against the file id, because
// they are two different numbers for one transfer.
//
// Before that id is bound there is nothing to match on, and the answer is then
// what the wire has already shown. An id this session has retired is an attempt
// that ended, and the running transfer is only the subject of an id-carrying
// event once the device has opened it, which happens on the goroutine that
// dispatches this event and is therefore ordered before it. Settling on
// anything else fails the next transfer with the word for the one before it.
func (s *Session) endsTheTransfer(event printerEvent, pending *transfer) bool {
	if event.TransferID == nil {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if pending.known {
		return *event.TransferID == pending.transferID
	}

	if s.retiredKnown && *event.TransferID == s.retiredTransfer {
		return false
	}

	return pending.bound
}

// coverage is the parts of a file served at least once, as sorted, disjoint
// half-open spans. A device asks for few ranges, so a merge per chunk is cheap.
type coverage struct {
	spans [][2]int64
}

// add records the bytes from start up to end and answers how many distinct
// bytes are covered now.
func (c *coverage) add(start, end int64) int64 {
	if end > start {
		c.spans = append(c.spans, [2]int64{start, end})
		slices.SortFunc(c.spans, func(a, b [2]int64) int { return cmp.Compare(a[0], b[0]) })

		merged := c.spans[:1]
		for _, span := range c.spans[1:] {
			last := &merged[len(merged)-1]
			if span[0] <= last[1] {
				last[1] = max(last[1], span[1])

				continue
			}

			merged = append(merged, span)
		}

		c.spans = merged
	}

	total := int64(0)
	for _, span := range c.spans {
		total += span[1] - span[0]
	}

	return total
}
