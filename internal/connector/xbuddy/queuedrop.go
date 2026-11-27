package xbuddy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// This file is what a command is told when the session carrying it ends, or
// when its caller stops waiting for it.
//
// It is separate from session.go because the answer has to tell three endings
// apart, and a command told the wrong one is worse than a command told nothing:
// it invites a caller to send a print a second time.

// CloseReplaced releases the session because the printer came back saying it is
// something this session was not built for.
//
// The capability set is fixed for a session's life, so a machine that comes
// back on other firmware gets a session of its own and this one ends. What was
// queued on it is dropped, and recording why here is what lets each dropped
// command tell its caller that rather than "the session closed".
//
// The identity is stored before the session closes, so a command that settles
// on the close reads it. It is safe to call more than once and the first reason
// stands.
func (s *Session) CloseReplaced(now connectorapi.Identity) error {
	s.replacedBy.CompareAndSwap(nil, &now)

	return s.Close()
}

// neverSent is what a command settles with when the session ended before the
// command reached the wire. It reports which of the two endings happened,
// because they are different answers: a printer that is not there is waited
// for, and a printer that came back as something else is read again.
//
// It is only correct for a command that was never written. A command already on
// the wire settles with unanswered, which promises nothing about whether the
// machine ran it.
func (s *Session) neverSent(verb connectorapi.Capability) error {
	if now := s.replacedBy.Load(); now != nil {
		return connectorapi.PrinterChanged.
			New("the printer came back reporting something this session was not built for").
			Wrap(ErrSessionClosed).
			With(
				fault.StringParam("verb", verb.String()),
				fault.StringParam("previous_firmware", firmwareText(s.identity.Firmware)),
				fault.StringParam("firmware", firmwareText(now.Firmware)),
			)
	}

	return fault.PrinterSessionEnded.
		New("the printer's session ended before " + verb.String() + " was sent").
		Wrap(ErrSessionClosed)
}

// firmwareText renders a reported firmware version for a refusal a person
// reads.
//
// It goes through MarshalText rather than String for the reason
// internal/app/devicerender.go does: the forbidigo rule on String keeps a
// version comparison out of every package but the two resolvers, and this is a
// rendering. Nothing here orders it.
func firmwareText(label connectorapi.FirmwareLabel) string {
	text, err := label.MarshalText()
	if err != nil {
		return ""
	}

	return string(text)
}

// connectionEnded is the outcome of a command whose connection ended before the
// printer answered it.
func (s *Session) connectionEnded(pending *call) outcome {
	if wireTable[pending.verb].restarts {
		return outcome{result: connectorapi.Result{OK: true, Ignored: pending.dropped}}
	}

	return outcome{err: s.unanswered(pending.verb)}
}

// unanswered is what a command settles with once it has been written and the
// connection or the session ended before an event answered it. It is a
// different code from neverSent and deliberately so: this one cannot say the
// machine did not run the command.
func (s *Session) unanswered(verb connectorapi.Capability) error {
	return connectorapi.CommandUnanswered.
		New("the connection went before the printer answered " + verb.String()).
		Wrap(ErrPrinterWentAway).
		With(fault.StringParam("verb", verb.String()))
}

// unansweredInTime is what a command settles with when it was written, the
// connection is still there, and the printer said nothing about it for the
// whole of the session's command deadline.
//
// It carries the same code as unanswered and for the same reason: the frame is
// on the wire, so this side cannot say whether the machine ran the command. The
// cause is what separates the two for a host, because ErrCommandDeadline
// carries context.DeadlineExceeded and the outcome is then recorded as a
// timeout (internal/app/devicecommand.go, settleStatus).
func (s *Session) unansweredInTime(verb connectorapi.Capability) error {
	return connectorapi.CommandUnanswered.
		New("the printer had not answered " + verb.String() + " when the command deadline ran out").
		Wrap(ErrCommandDeadline).
		With(fault.StringParam("verb", verb.String()))
}

// dropQueued settles every command still waiting for the wire, because the
// session that was going to carry them has ended and nothing else will.
//
// It runs after done is closed, so the pump is on its way out and cannot put a
// dropped command back. A command the pump has already taken is settled by the
// pump, and a command settles once whichever of the two reaches it first, so
// the overlap costs nothing.
//
// This is the whole of "a queued command is never silently dropped". Without it
// a caller waits for its own context to expire and learns nothing about why.
func (s *Session) dropQueued() {
	dropped := s.queue.drain()

	for _, pending := range dropped {
		pending.settle(outcome{err: s.neverSent(pending.verb)})
	}

	if len(dropped) > 0 {
		s.logWarn(context.Background(),
			"the printer's session ended with commands still waiting for it, and each caller was told",
			slog.Int("commands", len(dropped)),
			slog.Bool("printer_changed", s.replacedBy.Load() != nil))
	}
}

// errNotSent marks a command whose caller gave up before any part of it was
// written, so the printer never saw it.
var errNotSent = errors.New("was not sent")

// The values of call.wire.
const (
	callQueued int32 = iota
	callWritten
	callAbandoned
)

// claimWire reports whether the pump may write the call: false once its caller
// gave up before any write.
func (c *call) claimWire() bool {
	return c.wire.CompareAndSwap(callQueued, callWritten) || c.wire.Load() == callWritten
}

// giveUp settles a call whose caller stopped waiting with cause. The error
// carries errNotSent when nothing of the call was written.
func (c *call) giveUp(cause error) error {
	err := fmt.Errorf("xbuddy: waiting for %s: %w", c.verb, cause)
	if c.wire.CompareAndSwap(callQueued, callAbandoned) || c.wire.Load() == callAbandoned {
		err = fmt.Errorf("xbuddy: %s %w: %w", c.verb, errNotSent, cause)
	}

	c.settle(outcome{err: err})

	return err
}
