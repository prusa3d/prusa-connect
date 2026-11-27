package xbuddy

// When Send stops a transfer whose start it gave up on, read off the frames a
// hostile printer receives.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// Send stops a transfer whose start may be on the printer, and only that one.
//
// STOP_TRANSFER names no transfer: it stops whatever the printer fetches, a
// PrusaLink upload included. A start that never left the queue opened nothing,
// so stopping after it can only end somebody else's transfer.
func TestSendStopsATransferOnlyWhenItsStartMayBeOnThePrinter(t *testing.T) {
	t.Parallel()

	t.Run("a start written and unanswered is stopped", func(t *testing.T) {
		t.Parallel()

		h := standHostile(t)
		printer := listen(t, h)

		ctx, giveUp := context.WithCancel(t.Context())
		sent := sendInBackground(ctx, h, "usb/unanswered.bgcode")

		if frame := printer.next(t); !strings.Contains(frame, "START_INLINE_DOWNLOAD") {
			t.Fatalf("the driver sent %q, want the start of the transfer", frame)
		}

		giveUp()
		awaitCancelled(t, sent)

		if frame := printer.next(t); !strings.Contains(frame, "STOP_TRANSFER") {
			t.Fatalf("the driver sent %q after the caller gave up, want STOP_TRANSFER", frame)
		}
	})

	t.Run("a start still queued behind another command is not", func(t *testing.T) {
		t.Parallel()

		h := standHostile(t)
		printer := listen(t, h)

		beep := h.start(t)
		ahead := printer.next(t)

		ctx, giveUp := context.WithCancel(t.Context())
		sent := sendInBackground(ctx, h, "usb/queued.bgcode")

		h.awaitQueued(t, 1)
		giveUp()
		awaitCancelled(t, sent)

		printer.answer([]byte(ahead))

		if _, err := h.await(t, beep); err != nil {
			t.Fatalf("the command ahead of the start failed: %v", err)
		}

		if frames := printer.drain(); frames.has("START_INLINE_DOWNLOAD") || frames.has("STOP_TRANSFER") {
			t.Fatalf("the driver sent %q for a start that never left the queue", frames)
		}
	})

	t.Run("a start offered while the printer is away is not", func(t *testing.T) {
		t.Parallel()

		h := standHostile(t)

		_ = h.client.Close()
		h.awaitDeparture(t)

		ctx, giveUp := context.WithCancel(t.Context())
		sent := sendInBackground(ctx, h, "usb/away.bgcode")

		time.Sleep(quietWindow)
		giveUp()
		awaitCancelled(t, sent)

		h.redial(t)

		if frames := listen(t, h).drain(); frames.has("START_INLINE_DOWNLOAD") || frames.has("STOP_TRANSFER") {
			t.Fatalf("the driver sent %q to a printer that was away while the start was offered", frames)
		}
	})
}

// sendInBackground offers one small file and hands back the channel Send's
// answer arrives on.
func sendInBackground(ctx context.Context, h *hostile, path string) chan error {
	sent := make(chan error, 1)

	go func() {
		sent <- h.session.Send(ctx, connectorapi.Upload{Path: path, Content: emptyFile{}})
	}()

	return sent
}

// awaitCancelled takes Send's answer and fails unless it is the caller's
// cancellation.
func awaitCancelled(t *testing.T, sent chan error) {
	t.Helper()

	select {
	case err := <-sent:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Send answered %v, want %v", err, context.Canceled)
		}
	case <-time.After(hostileBudget):
		t.Fatal("Send did not return after its caller gave up")
	}
}

// emptyFile is a file of no bytes. No case here gets as far as reading one.
type emptyFile struct{}

func (emptyFile) Size() int64 { return 0 }

func (emptyFile) Open(context.Context, int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

// Which failed starts may have opened the transfer on the printer.
func TestWhichFailedStartsMayHaveOpenedTheTransfer(t *testing.T) {
	t.Parallel()

	session := &Session{}

	for _, tc := range []struct {
		name   string
		err    error
		opened bool
	}{
		{name: "unanswered", err: session.unanswered(VerbStartInlineDownload), opened: true},
		{name: "unanswered inside the deadline", err: session.unansweredInTime(VerbStartInlineDownload), opened: true},
		{
			name:   "given up after it was written",
			err:    fmt.Errorf("xbuddy: waiting for START_INLINE_DOWNLOAD: %w", context.Canceled),
			opened: true,
		},
		{
			name: "given up before it was written",
			err:  fmt.Errorf("xbuddy: START_INLINE_DOWNLOAD %w: %w", errNotSent, context.Canceled),
		},
		{
			name: "refused",
			err: connectorapi.TransferRefused.New("the printer would not take the file").
				Wrap(connectorapi.ErrTransferRefused),
		},
		{name: "never sent on a closed session", err: session.neverSent(VerbStartInlineDownload)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := startMayHaveOpened(tc.err); got != tc.opened {
				t.Errorf("startMayHaveOpened(%v) = %v, want %v", tc.err, got, tc.opened)
			}
		})
	}
}
