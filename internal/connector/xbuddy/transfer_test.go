package xbuddy_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math/rand/v2"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prusa3d/prusa-connect/internal/connector/xbuddy"
	"github.com/prusa3d/prusa-connect/internal/printer/capability"
	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

func uploadsOf(t *testing.T, session connectorapi.Session) connectorapi.Transfers {
	t.Helper()

	transfers, ok := connectorapi.Uploads(session)
	if !ok {
		t.Fatal("an xBuddy session cannot be sent a file")
	}

	return transfers
}

// bytesContent is a file held in memory, for a case whose point is the bytes
// that arrive. The streaming property is proved by generatedContent below,
// which holds nothing.
type bytesContent struct{ data []byte }

func (c bytesContent) Size() int64 { return int64(len(c.data)) }

func (c bytesContent) Open(_ context.Context, offset int64) (io.ReadCloser, error) {
	if offset >= int64(len(c.data)) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	return io.NopCloser(bytes.NewReader(c.data[offset:])), nil
}

// sampleFile is a deterministic file of the given length. It is pseudorandom so
// that a transfer which reordered or repeated a block produces a different
// digest, which a file of one repeated byte would not.
func sampleFile(size int) []byte {
	source := rand.NewChaCha8([32]byte{7})
	data := make([]byte, size)
	_, _ = source.Read(data)

	return data
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// The whole path: a file this host is holding reaches the printer's storage,
// byte for byte, and shows up in the printer's own file tree afterwards.
//
// The file is larger than one block request and not a multiple of the chunk, so
// the case covers a transfer spanning several block requests and one ending on
// a partial chunk. Both are shapes an off-by-one in the inclusive end produces
// a wrong file for, and the digest is what catches it.
func TestAFileReachesThePrintersStorage(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		data := sampleFile(700_000)
		path := "usb/part.bgcode"

		if err := uploadsOf(t, rig.spy.inner).Send(rig.ctx, connectorapi.Upload{
			Path: path, Content: bytesContent{data: data},
		}); err != nil {
			t.Fatalf("send: %v", err)
		}

		landed, ok := rig.printer.LastTransfer()
		if !ok {
			t.Fatal("Send returned and the printer fetched nothing")
		}

		t.Logf("observed: the printer fetched %d bytes to %q, finished=%v",
			landed.Received, landed.Path, landed.Finished)

		if !landed.Finished {
			t.Error("Send returned before the printer reported the transfer finished")
		}

		if landed.Received != int64(len(data)) {
			t.Errorf("the printer received %d bytes, and the file is %d", landed.Received, len(data))
		}

		if landed.Digest != digestOf(data) {
			t.Errorf("the printer received a different file: digest %s, want %s",
				landed.Digest, digestOf(data))
		}

		if landed.Path != "/"+path {
			t.Errorf("the file landed at %q, want %q", landed.Path, "/"+path)
		}

		listing, err := filesOf(t, rig.spy.inner).List(rig.ctx, "usb")
		if err != nil {
			t.Fatalf("list usb: %v", err)
		}

		if !listingCarries(listing, path) {
			t.Errorf("the file is not in the printer's own listing of usb: %+v", listing.Entries)
		}
	})
}

// hasMachineReason reports whether a refusal carries the machine's own word.
func hasMachineReason(err error, want string) bool {
	carried, ok := errors.AsType[*fault.Fault](err)
	if !ok {
		return false
	}

	for _, param := range carried.Params() {
		if param.Key == "machine_reason" && param.Value.String() == want {
			return true
		}
	}

	return false
}

// A name the stick already holds is refused, and the file there stays. The
// firmware stats the destination before it begins and answers "File already
// exists" (transfer.cpp Transfer::begin, planner.cpp handle_transfer_result),
// so the refusal carries FILE_EXISTS for a caller to branch on.
func TestAFileSentToANameTheStickHoldsIsRefused(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		uploads := uploadsOf(t, rig.spy.inner)
		first := sampleFile(10_000)

		if err := uploads.Send(rig.ctx, connectorapi.Upload{
			Path: "usb/part.bgcode", Content: bytesContent{data: first},
		}); err != nil {
			t.Fatalf("the first send: %v", err)
		}

		err := uploads.Send(rig.ctx, connectorapi.Upload{
			Path: "usb/part.bgcode", Content: bytesContent{data: sampleFile(20_000)},
		})
		if !errors.Is(err, connectorapi.ErrTransferRefused) {
			t.Fatalf("the second send answered %v, want the printer's refusal", err)
		}

		if !hasMachineReason(err, "FILE_EXISTS") {
			t.Errorf("the refusal %v does not carry FILE_EXISTS", err)
		}

		listing, err := filesOf(t, rig.spy.inner).List(rig.ctx, "usb")
		if err != nil {
			t.Fatalf("list usb: %v", err)
		}

		for _, entry := range listing.Entries {
			if entry.Path == "usb/part.bgcode" && entry.Size != int64(len(first)) {
				t.Errorf("the file there is %d bytes, want the first file's %d", entry.Size, len(first))
			}
		}
	})
}

func listingCarries(listing connectorapi.Listing, path string) bool {
	for _, entry := range listing.Entries {
		if entry.Path == path {
			return true
		}
	}

	return false
}

// A path that leaves the storage it names is refused here and never reaches the
// printer, which is the rule every path-bearing verb takes.
func TestATransferPathThatEscapesIsRefused(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		err := uploadsOf(t, rig.spy.inner).Send(rig.ctx, connectorapi.Upload{
			Path: "usb/../../etc/passwd", Content: bytesContent{data: []byte("x")},
		})
		if err == nil {
			t.Fatal("a path leaving its storage was accepted")
		}

		t.Logf("observed: %v", err)

		if _, fetched := rig.printer.LastTransfer(); fetched {
			t.Error("the printer was asked to fetch a file for a path that should never have left")
		}
	})
}

// hugeContent declares a length and is never read.
type hugeContent struct{ size int64 }

func (c hugeContent) Size() int64 { return c.size }

func (c hugeContent) Open(context.Context, int64) (io.ReadCloser, error) {
	return nil, errors.New("a file this long is never opened")
}

// A file at or above 4 GiB has no orig_size the wire can carry, so it is
// refused before the command is composed rather than sent as a length that
// wrapped.
func TestAFileAtTheUint32CeilingIsRefused(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		err := uploadsOf(t, rig.spy.inner).Send(rig.ctx, connectorapi.Upload{
			Path: "usb/huge.bgcode", Content: hugeContent{size: 1 << 32},
		})
		if err == nil {
			t.Fatal("a file at the uint32 ceiling was accepted")
		}

		t.Logf("observed: %v", err)

		if _, fetched := rig.printer.LastTransfer(); fetched {
			t.Error("the printer was asked to fetch a file whose length cannot be described")
		}
	})
}

// heldContent does not answer a read until the test lets it, so a transfer can
// be observed while it is still running.
type heldContent struct {
	data    []byte
	release chan struct{}
}

func (c *heldContent) Size() int64 { return int64(len(c.data)) }

func (c *heldContent) Open(ctx context.Context, offset int64) (io.ReadCloser, error) {
	select {
	case <-c.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return io.NopCloser(bytes.NewReader(c.data[min(offset, int64(len(c.data))):])), nil
}

// A second transfer offered while one is running is refused rather than
// silently destroying the transfer in flight, which is what the device does
// with it.
func TestASecondTransferIsRefusedWhileOneIsRunning(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)
		transfers := uploadsOf(t, rig.spy.inner)

		held := &heldContent{data: sampleFile(64_000), release: make(chan struct{})}
		first := make(chan error, 1)

		go func() {
			first <- transfers.Send(rig.ctx, connectorapi.Upload{
				Path: "usb/first.bgcode", Content: held,
			})
		}()

		synctest.Wait()

		err := transfers.Send(rig.ctx, connectorapi.Upload{
			Path: "usb/second.bgcode", Content: bytesContent{data: sampleFile(16)},
		})
		if !errors.Is(err, connectorapi.ErrTransferBusy) {
			t.Fatalf("the second transfer answered %v, want %v", err, connectorapi.ErrTransferBusy)
		}

		t.Logf("observed: %v", err)

		close(held.release)

		if firstErr := <-first; firstErr != nil {
			t.Fatalf("the first transfer did not survive the second being refused: %v", firstErr)
		}

		landed, _ := rig.printer.LastTransfer()
		if landed.Path != "/usb/first.bgcode" || !landed.Finished {
			t.Errorf("the transfer in flight did not finish: %+v", landed)
		}
	})
}

// A transfer survives the printer's connection going and coming back, and the
// printer names it with a file id it has not used before.
//
// This is the shape of the real exchange rather than an unlikely edge: an MK4S
// at 6.5.7 resets its connection within a hundred milliseconds of accepting a
// transfer, every time, and asks again on the next connection with a fresh file
// id. A driver that bound the first id and refused the rest served that printer
// the whole file seven times over and finished none of them.
//
// The reconnection count is asserted because the simulator ends the connection
// on a chunk it did not ask for: a driver that answered with a frame this
// printer dislikes reconnects its way to the same digest, and the count is what
// tells the two apart. Which frames may still be written after a reconnection is
// pinned on its own, by TestABlockRequestIsOnlyAnsweredOnTheConnectionItArrivedOn.
func TestATransferSurvivesThePrinterReconnecting(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		data := sampleFile(40_000)
		held := &heldContent{data: data, release: make(chan struct{})}
		sent := make(chan error, 1)

		go func() {
			sent <- uploadsOf(t, rig.spy.inner).Send(rig.ctx, connectorapi.Upload{
				Path: "usb/resumed.bgcode", Content: held,
			})
		}()

		synctest.Wait()

		// The cable comes out while the printer is fetching, and goes back in.
		rig.printer.GoOffline()
		synctest.Wait()
		rig.printer.GoOnline()

		close(held.release)

		if err := <-sent; err != nil {
			t.Fatalf("the transfer did not survive the reconnection: %v", err)
		}

		if sessions := rig.printer.Sessions(); sessions != 2 {
			t.Errorf("the printer established %d connections, and one cable pulled out is two", sessions)
		}

		landed, ok := rig.printer.LastTransfer()
		if !ok || !landed.Finished {
			t.Fatalf("the printer did not finish the transfer: %+v", landed)
		}

		if landed.Digest != digestOf(data) {
			t.Errorf("the file that survived the reconnection is not the one sent: %s, want %s",
				landed.Digest, digestOf(data))
		}

		t.Logf("observed: %d bytes arrived intact across a reconnection", landed.Received)
	})
}

// A caller that gives up while the printer is away gets its answer.
//
// The printer being away is the usual reason a caller cancels, and telling the
// device to stop is a command that waits for a socket to attach. Waiting for it
// held Send for as long as the printer stayed off, and Send holds the one
// transfer slot, so every later Send was refused as busy for as long again.
func TestSendReturnsWhenTheCallerGivesUpOnAnAbsentPrinter(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		ctx, giveUp := context.WithCancel(rig.ctx)
		held := &heldContent{data: sampleFile(40_000), release: make(chan struct{})}
		sent := make(chan error, 1)

		go func() {
			sent <- uploadsOf(t, rig.spy.inner).Send(ctx, connectorapi.Upload{
				Path: "usb/abandoned.bgcode", Content: held,
			})
		}()

		synctest.Wait()

		rig.printer.GoOffline()
		synctest.Wait()

		giveUp()
		synctest.Wait()

		select {
		case err := <-sent:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Send answered %v, and a cancelled caller is %v", err, context.Canceled)
			}

			t.Logf("observed: %v", err)
		default:
			t.Fatal("Send is still waiting for a printer that is away and a caller that has gone")
		}
	})
}

// A file can be sent after a transfer the caller gave up on while the printer
// was away.
//
// Cancelling the first Send has to release the one transfer slot even though
// the stop command cannot leave yet, and the printer's word for the first
// transfer ending has to not be read as the second transfer's.
func TestAFileCanBeSentAfterACancelledTransfer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)
		transfers := uploadsOf(t, rig.spy.inner)

		ctx, giveUp := context.WithCancel(rig.ctx)
		held := &heldContent{data: sampleFile(40_000), release: make(chan struct{})}
		abandoned := make(chan error, 1)

		go func() {
			abandoned <- transfers.Send(ctx, connectorapi.Upload{
				Path: "usb/abandoned.bgcode", Content: held,
			})
		}()

		synctest.Wait()

		rig.printer.GoOffline()
		synctest.Wait()

		giveUp()
		synctest.Wait()

		if err := <-abandoned; !errors.Is(err, context.Canceled) {
			t.Fatalf("the abandoned transfer answered %v, and a cancelled caller is %v", err, context.Canceled)
		}

		rig.printer.GoOnline()

		data := sampleFile(30_000)
		if err := transfers.Send(rig.ctx, connectorapi.Upload{
			Path: "usb/second.bgcode", Content: bytesContent{data: data},
		}); err != nil {
			t.Fatalf("the second file was not sent after the first was abandoned: %v", err)
		}

		landed, ok := rig.printer.LastTransfer()
		if !ok || !landed.Finished || landed.Path != "/usb/second.bgcode" {
			t.Fatalf("the second file did not land: %+v", landed)
		}

		if landed.Digest != digestOf(data) {
			t.Errorf("the second file arrived as %s, want %s", landed.Digest, digestOf(data))
		}

		t.Logf("observed: %d bytes arrived after a transfer the caller gave up on", landed.Received)
	})
}

// A printer that comes back fetching a transfer this server no longer holds is
// told to stop fetching it.
//
// A printer that stays away past its grace comes back to a new session, and the
// stop the old one queued for a cancelled transfer went with it. A Buddy board
// keeps that transfer on its USB drive, resumes it at every boot and refuses
// every later file while it is open, so silence alone leaves the printer stuck
// for good. A server restarted in the middle of a transfer is the same case.
func TestATransferThePrinterResumesAfterTheServerDroppedItIsStopped(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration, withGrace(time.Minute))
		defer rig.stop(t)

		rig.connect(t)
		transfers := uploadsOf(t, rig.spy.inner)

		ctx, giveUp := context.WithCancel(rig.ctx)
		held := &heldContent{data: sampleFile(40_000), release: make(chan struct{})}
		abandoned := make(chan error, 1)

		go func() {
			abandoned <- transfers.Send(ctx, connectorapi.Upload{
				Path: "usb/orphan.bgcode", Content: held,
			})
		}()

		synctest.Wait()

		rig.printer.GoOffline()
		synctest.Wait()

		giveUp()
		synctest.Wait()

		if err := <-abandoned; !errors.Is(err, context.Canceled) {
			t.Fatalf("the abandoned transfer answered %v, and a cancelled caller is %v", err, context.Canceled)
		}

		// Past the grace, so the session and the stop it queued are gone.
		time.Sleep(2 * time.Minute)
		synctest.Wait()

		rig.printer.GoOnline()
		rig.await(t)

		time.Sleep(time.Minute)
		synctest.Wait()

		data := sampleFile(30_000)
		if err := uploadsOf(t, rig.spy.inner).Send(rig.ctx, connectorapi.Upload{
			Path: "usb/second.bgcode", Content: bytesContent{data: data},
		}); err != nil {
			t.Fatalf("the printer is still fetching the transfer the server dropped: %v", err)
		}

		landed, ok := rig.printer.LastTransfer()
		if !ok || !landed.Finished || landed.Digest != digestOf(data) {
			t.Fatalf("the second file did not land intact: %+v", landed)
		}
	})
}

// A printer that comes back printing and fetching a transfer the server no
// longer holds is told to stop it at once, and the print is not stopped.
//
// Buddy itself pauses a print of that file on a file warning; the simulator
// does not, so the check is only that this host sent the print no stop.
func TestAResumedStrayTransferOnAPrintingPrinterIsStopped(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration, withGrace(time.Minute))
		defer rig.stop(t)

		session := rig.connect(t)

		do := func(session *capability.Session, verb connectorapi.Capability, args map[string]string) {
			t.Helper()

			answer, err := session.Do(rig.ctx, connectorapi.Command{Verb: verb, Args: args})
			if err != nil || !answer.OK {
				t.Fatalf("%s: %v %+v", verb, err, answer)
			}

			// The session reads the machine's state off its telemetry.
			time.Sleep(5 * time.Second)
			synctest.Wait()
		}

		do(session, xbuddy.VerbStartPrint, map[string]string{"path": "tangle.bgcode"})

		ctx, giveUp := context.WithCancel(rig.ctx)
		held := &heldContent{data: sampleFile(40_000), release: make(chan struct{})}
		abandoned := make(chan error, 1)

		go func() {
			abandoned <- uploadsOf(t, rig.spy.inner).Send(ctx, connectorapi.Upload{
				Path: "usb/orphan.bgcode", Content: held,
			})
		}()

		synctest.Wait()

		rig.printer.GoOffline()
		synctest.Wait()

		giveUp()
		synctest.Wait()

		if err := <-abandoned; !errors.Is(err, context.Canceled) {
			t.Fatalf("the abandoned transfer answered %v, and a cancelled caller is %v", err, context.Canceled)
		}

		// Past the grace, so the session and the stop it queued are gone.
		time.Sleep(2 * time.Minute)
		synctest.Wait()

		rig.printer.GoOnline()
		rig.await(t)

		time.Sleep(time.Minute)
		synctest.Wait()

		if !rig.printer.Printing() {
			t.Fatal("the print ended, so this case no longer watches a printing printer")
		}

		if got := rig.printer.Asked("STOP_TRANSFER"); got != 1 {
			t.Fatalf("a printing printer was told to stop the transfer it resumed %d times, want 1", got)
		}

		if !rig.printer.Printing() {
			t.Fatal("stopping the stray transfer also stopped the print")
		}
	})
}

// A block request naming another transfer does not stop the one this host is
// sending.
//
// While a file is on its way, a request under another file id may be a late
// one for that same file from a connection that has ended, and a stop would end
// the transfer the caller is waiting for.
func TestAStrayBlockRequestLeavesTheTransferInFlightRunning(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		data := sampleFile(40_000)
		held := &heldContent{data: data, release: make(chan struct{})}
		sent := make(chan error, 1)

		go func() {
			sent <- uploadsOf(t, rig.spy.inner).Send(rig.ctx, connectorapi.Upload{
				Path: "usb/in-flight.bgcode", Content: held,
			})
		}()

		synctest.Wait()

		if err := rig.printer.RequestStrayBlock(4242); err != nil {
			t.Fatalf("send a stray block request: %v", err)
		}

		synctest.Wait()
		close(held.release)

		if err := <-sent; err != nil {
			t.Fatalf("the transfer in flight did not finish after a stray block request: %v", err)
		}

		landed, ok := rig.printer.LastTransfer()
		if !ok || !landed.Finished || landed.Digest != digestOf(data) {
			t.Fatalf("the file in flight did not land intact: %+v", landed)
		}
	})
}

// A printer holding a print is told to stop a transfer the server does not hold
// at once, and only once for that request. The check on the print is only that
// this host sent it no stop.
func TestAStrayTransferIsStoppedDuringAPrint(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		session := rig.connect(t)

		answer, err := session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbStartPrint, Args: map[string]string{"path": "tangle.bgcode"},
		})
		if err != nil || !answer.OK {
			t.Fatalf("start print: %v %+v", err, answer)
		}

		// The session reads the machine's state off its telemetry.
		time.Sleep(5 * time.Second)
		synctest.Wait()

		stray := func() {
			t.Helper()

			if sendErr := rig.printer.RequestStrayBlock(4242); sendErr != nil {
				t.Fatalf("send a stray block request: %v", sendErr)
			}

			time.Sleep(time.Second)
			synctest.Wait()
		}

		stray()

		if got := rig.printer.Asked("STOP_TRANSFER"); got != 1 {
			t.Fatalf("a printing printer was told to stop a stray transfer %d times, want 1", got)
		}

		if !rig.printer.Printing() {
			t.Fatal("stopping the stray transfer also stopped the print")
		}

		stray()

		if got := rig.printer.Asked("STOP_TRANSFER"); got != 1 {
			t.Fatalf("a stray transfer already stopped was stopped again: %d stops, want 1", got)
		}
	})
}

// A printer that takes the start of a transfer and answers it too late is told
// to stop fetching.
//
// COMMAND_UNANSWERED cannot say whether the printer opened the transfer. A Buddy
// board that did keeps asking for a file this server no longer holds: it shows
// "Downloading 0 %", resumes the transfer at every boot, and refuses every
// later file as a transfer in progress.
func TestAnUnansweredTransferStartIsStopped(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration, withCommandDeadline)
		defer rig.stop(t)

		rig.connect(t)
		transfers := uploadsOf(t, rig.spy.inner)

		// The storage request a new connection makes is on the wire at this
		// point, and the pump carries one command at a time.
		time.Sleep(2 * time.Second)
		synctest.Wait()

		rig.printer.SetResponseDelay(2 * commandDeadline)

		err := transfers.Send(rig.ctx, connectorapi.Upload{
			Path: "usb/unanswered.bgcode", Content: bytesContent{data: sampleFile(30_000)},
		})
		if !fault.HasCode(err, connectorapi.CommandUnanswered.Code()) {
			t.Fatalf("Send answered %v, want %s", err, connectorapi.CommandUnanswered.Code())
		}

		// The printer runs the late start, and whatever the server sent after it.
		rig.printer.SetResponseDelay(0)
		time.Sleep(2 * commandDeadline)
		synctest.Wait()

		data := sampleFile(30_000)
		if err = transfers.Send(rig.ctx, connectorapi.Upload{
			Path: "usb/second.bgcode", Content: bytesContent{data: data},
		}); err != nil {
			t.Fatalf("the printer is still fetching the file whose start went unanswered: %v", err)
		}

		landed, ok := rig.printer.LastTransfer()
		if !ok || !landed.Finished || landed.Digest != digestOf(data) {
			t.Fatalf("the second file did not land intact: %+v", landed)
		}
	})
}

// A transfer the caller stops while its start is still unanswered is stopped on
// the printer too.
//
// The start may already be on the printer. A Buddy board that opens it keeps a
// full-size partial file, refuses to delete it, and resumes the transfer at every
// boot, so a stop that only ends the wait leaves the printer unusable.
func TestATransferStoppedBeforeItsStartIsAnsweredIsStopped(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration, withCommandDeadline)
		defer rig.stop(t)

		rig.connect(t)
		transfers := uploadsOf(t, rig.spy.inner)

		// The storage request a new connection makes is on the wire at this
		// point, and the pump carries one command at a time.
		time.Sleep(2 * time.Second)
		synctest.Wait()

		// Answered within the deadline, but only after the caller has stopped.
		rig.printer.SetResponseDelay(commandDeadline / 2)

		ctx, stop := context.WithCancel(rig.ctx)
		defer stop()

		time.AfterFunc(time.Second, stop)

		err := transfers.Send(ctx, connectorapi.Upload{
			Path: "usb/stopped.bgcode", Content: bytesContent{data: sampleFile(30_000)},
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Send answered %v, want %v", err, context.Canceled)
		}

		// The printer runs the late start, and whatever the server sent after it.
		rig.printer.SetResponseDelay(0)
		time.Sleep(2 * commandDeadline)
		synctest.Wait()

		data := sampleFile(30_000)
		if err = transfers.Send(rig.ctx, connectorapi.Upload{
			Path: "usb/second.bgcode", Content: bytesContent{data: data},
		}); err != nil {
			t.Fatalf("the printer is still fetching the file whose transfer was stopped: %v", err)
		}

		landed, ok := rig.printer.LastTransfer()
		if !ok || !landed.Finished || landed.Digest != digestOf(data) {
			t.Fatalf("the second file did not land intact: %+v", landed)
		}
	})
}

// A transfer the caller stops while its start is unanswered is stopped on a
// printing printer too, and the print is not stopped.
//
// The printer may also ask for a block of that transfer after Send has let it
// go, and that request is stopped as a stray one: a printer that no longer
// runs the transfer refuses the second stop.
func TestATransferStoppedBeforeItsStartIsAnsweredIsStoppedDuringAPrint(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration, withCommandDeadline)
		defer rig.stop(t)

		session := rig.connect(t)
		transfers := uploadsOf(t, rig.spy.inner)

		time.Sleep(2 * time.Second)
		synctest.Wait()

		answer, err := session.Do(rig.ctx, connectorapi.Command{
			Verb: xbuddy.VerbStartPrint, Args: map[string]string{"path": "tangle.bgcode"},
		})
		if err != nil || !answer.OK {
			t.Fatalf("start a print: %v %+v", err, answer)
		}

		// The session reads the machine's state off its telemetry.
		time.Sleep(5 * time.Second)
		synctest.Wait()

		rig.printer.SetResponseDelay(commandDeadline / 2)

		ctx, stop := context.WithCancel(rig.ctx)
		defer stop()

		time.AfterFunc(time.Second, stop)

		err = transfers.Send(ctx, connectorapi.Upload{
			Path: "usb/stopped.bgcode", Content: bytesContent{data: sampleFile(30_000)},
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Send answered %v, want %v", err, context.Canceled)
		}

		rig.printer.SetResponseDelay(0)
		time.Sleep(2 * commandDeadline)
		synctest.Wait()

		if !rig.printer.Printing() {
			t.Fatal("the print ended, so this case no longer watches a printing printer")
		}

		// One stop from Send, and at most one for the printer's later request on
		// this connection.
		if got := rig.printer.Asked("STOP_TRANSFER"); got < 1 || got > 2 {
			t.Fatalf("the printer was told to stop the transfer %d times, want 1 or 2", got)
		}
	})
}

// A block request naming a transfer this host is not holding is answered with
// silence, and is not counted as a refusal.
//
// Silence costs the printer a retry, which is what it already does. The
// alternative is the empty chunk, which is the wire's one failure signal and
// which a Buddy board answers by closing the connection within ten
// milliseconds: the simulator ends the connection on a chunk it did not ask
// for, so a version that answered here fails this rather than a printer.
func TestABlockRequestForNothingIsAnsweredWithSilence(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		counted, ok := rig.spy.inner.(interface{ Refused() uint64 })
		if !ok {
			t.Fatal("the session does not report what it refused")
		}

		before := counted.Refused()
		sessions := rig.printer.Sessions()

		if err := rig.printer.RequestStrayBlock(4242); err != nil {
			t.Fatalf("send a stray block request: %v", err)
		}

		synctest.Wait()

		if after := counted.Refused(); after != before {
			t.Errorf("a block request this host had nothing for was counted as refused: %d then %d",
				before, after)
		}

		// The printer is still on the connection it asked from, which is what says
		// nothing was written back to it.
		if now := rig.printer.Sessions(); now != sessions {
			t.Errorf("the printer reconnected %d times after a stray block request, so it was answered "+
				"with a chunk rather than with silence", now-sessions)
		}

		if _, err := filesOf(t, rig.spy.inner).List(rig.ctx, "usb"); err != nil {
			t.Fatalf("the session stopped working after a stray block request: %v", err)
		}

		t.Logf("observed: the refused counter stayed at %d and the printer kept its connection", before)
	})
}

// Nothing this port sends scales with the file's length.
//
// It is the bar the old backlog sets, at a size that runs in a test rather than at the
// heap budget, and the property is proved by what holds the file rather than by
// the size: generatedContent stores nothing, the driver's buffer is one chunk,
// and the simulator keeps a digest and a count.
func TestATransferHoldsNoneOfTheFile(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rig := start(t, "MK4", withAutoRegistration)
		defer rig.stop(t)

		rig.connect(t)

		const size = 8 << 20

		if err := uploadsOf(t, rig.spy.inner).Send(rig.ctx, connectorapi.Upload{
			Path: "usb/big.bgcode", Content: generatedContent{size: size},
		}); err != nil {
			t.Fatalf("send: %v", err)
		}

		landed, _ := rig.printer.LastTransfer()

		if landed.Received != size {
			t.Fatalf("the printer received %d bytes, want %d", landed.Received, size)
		}

		if landed.Digest != generatedDigest(size) {
			t.Errorf("the printer received a different file: digest %s, want %s",
				landed.Digest, generatedDigest(size))
		}

		t.Logf("observed: %d bytes arrived intact and neither side held the file", landed.Received)
	})
}

// generatedContent is a file that exists only while it is being read: every
// byte is computed from its offset. A transfer that read the wrong offsets, or
// served a range twice, produces a different digest.
type generatedContent struct{ size int64 }

func (c generatedContent) Size() int64 { return c.size }

func (c generatedContent) Open(_ context.Context, offset int64) (io.ReadCloser, error) {
	if offset >= c.size {
		return io.NopCloser(strings.NewReader("")), nil
	}

	return io.NopCloser(&generatedReader{at: offset, end: c.size}), nil
}

type generatedReader struct{ at, end int64 }

func (r *generatedReader) Read(out []byte) (int, error) {
	if r.at >= r.end {
		return 0, io.EOF
	}

	count := min(int64(len(out)), r.end-r.at)
	for i := range count {
		out[i] = generatedByte(r.at + i)
	}

	r.at += count

	return int(count), nil
}

// generatedByte is the file's content: a byte that depends on where it is, so
// no two blocks are the same bytes.
func generatedByte(offset int64) byte { return byte(offset*31 + offset>>8) }

func generatedDigest(size int64) string {
	sum := sha256.New()
	buffer := make([]byte, 4096)

	for at := int64(0); at < size; {
		count := min(int64(len(buffer)), size-at)
		for i := range count {
			buffer[i] = generatedByte(at + i)
		}

		sum.Write(buffer[:count])

		at += count
	}

	return hex.EncodeToString(sum.Sum(nil))
}
