package xbuddy

import (
	"context"
	"errors"
	"log/slog"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// This file is when the driver tells the device to stop a transfer: one whose
// start Send gave up on, and one the device fetches that no Send holds.

// startMayHaveOpened reports whether a start that failed with err may have
// opened the transfer on the printer: it was written and no answer came. A
// refusal opened nothing, and neither did a start never written, so stopping
// after either could only end another transfer the printer runs.
func startMayHaveOpened(err error) bool {
	if fault.HasCode(err, connectorapi.CommandUnanswered.Code()) {
		return true
	}

	gaveUp := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)

	return gaveUp && !errors.Is(err, errNotSent)
}

// stopTransfer abandons whatever the device is fetching, for a caller that
// stopped waiting or a start the device did not answer.
//
// It leaves on a goroutine of its own, under its own budget, and the caller does
// not wait for it. A command waits for a socket to attach, and the printer being
// away is the usual reason a caller cancels: stopping the transfer on the
// caller's way out held Send until the printer came back, and while Send held
// the transfer slot every later Send was refused as busy. The budget is what
// bounds the other shape, a firmware that never answers STOP_TRANSFER while its
// telemetry keeps the connection alive.
func (s *Session) stopTransfer(ctx context.Context, pending *transfer) {
	s.sendStopTransfer(ctx, nil, slog.String("path", pending.path))
}

// stopStrayTransfer tells the device to stop a transfer this host is not
// holding. A request naming the file id of the last one it stopped on this
// connection is not stopped again.
//
// A device resumes an unfinished transfer at every boot and on every
// connection, and refuses every other file while it runs, so one this host
// forgot keeps the printer busy for good: a restart in the middle of a
// transfer, or a stop that never reached the device. It is sent only while this
// host is offering nothing. While it offers a file, a request naming another
// may be a late one for that file from a connection that has ended.
//
// It is sent during a print too. Nothing here will serve that transfer, so a
// print of its file cannot finish either way: Buddy pauses it with a file
// warning the next time the print reads ahead after a stop, or at the end of
// the fetched part without one (marlin_server.cpp, media_print_loop).
//
// The stop is for the connection the request came on, current, and is not sent
// on a later one: the device asks again on every connection, and a stop that
// waited there may meet a file sent meanwhile.
func (s *Session) stopStrayTransfer(ctx context.Context, current *link, request block) {
	s.mu.Lock()
	stale := s.transfer != nil || (s.strayTransferKnown && s.strayTransfer == request.fileID)

	if !stale {
		s.strayTransfer = request.fileID
		s.strayTransferKnown = true
	}

	s.mu.Unlock()

	if stale {
		return
	}

	s.sendStopTransfer(ctx, current, slog.Uint64("file_id", uint64(request.fileID)))
}

// sendStopTransfer puts STOP_TRANSFER on the wire under its own budget, without
// waiting for it. what names the transfer in the log line of a stop that failed.
//
// A stop for one connection, on, is dropped if that connection ends before the
// stop is written. A nil on sends it on whichever connection comes.
func (s *Session) sendStopTransfer(ctx context.Context, on *link, what slog.Attr) {
	if !s.caps.Has(VerbStopTransfer) {
		return
	}

	// The context the caller ended is not the one this travels under.
	detached := context.WithoutCancel(ctx)

	go func() {
		bounded, done := context.WithTimeout(detached, stopTransferBudget)
		defer done()

		if on != nil {
			go func() {
				select {
				case <-on.gone:
					done()
				case <-bounded.Done():
				}
			}()
		}

		if _, err := s.Do(bounded, connectorapi.Command{Verb: VerbStopTransfer}); err != nil {
			s.logDebug(detached, "the printer was not told to stop a transfer nobody is waiting for",
				what, slog.String("error", err.Error()))
		}
	}()
}
