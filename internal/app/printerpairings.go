package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/prusa3d/prusa-connect/internal/connector/printerpairing"
	"github.com/prusa3d/prusa-connect/internal/kernel/admission"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// hearsRegistrations is the half of a driver that tells the host about every
// registration with the address it came from. The xBuddy driver implements
// it, and a printer paired at its screen is approved through it
// (ADR-20261126-qca). It is declared here because this is where it is
// consumed.
type hearsRegistrations interface {
	HearRegistrations(hear func(ctx context.Context, registration admission.Registration, from netip.Addr))
}

// printerPairings is the printer pairing surface as the HTTP layer consumes
// it. The protocol and the binding are the connector's; this turns them into
// the transport's types.
type printerPairings struct {
	service *printerpairing.Service
}

var _ httpapi.PrinterPairings = printerPairings{}

// newPrinterPairings is the pairing service this process runs, approving
// through the same surface an operator's approval goes through. So a paired
// printer is bound to a device, owned by the person who paired it, and
// announced on the event stream exactly as one a person approved by its code.
// Every table that reports its registrations is handed to it, and a
// registration from the address a pairing reached is approved
// (ADR-20261126-qca). The caller closes it before the fleet.
func newPrinterPairings(
	admissions registrations, tables []admissionTable, logger *slog.Logger, permit func(netip.Addr) bool,
) *printerpairing.Service {
	service := printerpairing.NewService(printerpairing.Options{
		Client: printerpairing.NewClient(permit),
		Admit: func(ctx context.Context, code, by string) (string, error) {
			row, err := admissions.admit(ctx, code, by, by)

			return row.DeviceID, err
		},
		Logger: logger,
	})

	hearPairings(tables, service)

	return service
}

// hearPairings hands every driver that reports its registrations to the
// pairing service.
func hearPairings(tables []admissionTable, service *printerpairing.Service) {
	for _, table := range tables {
		hears, ok := table.driver.(hearsRegistrations)
		if !ok {
			continue
		}

		connector := table.ref.String()

		hears.HearRegistrations(func(ctx context.Context, registration admission.Registration, from netip.Addr) {
			service.Arrived(ctx, printerpairing.Arrival{
				Connector: connector,
				Code:      registration.Code,
				Serial:    registration.Serial,
				Model:     registration.Model,
				From:      from,
			})
		})
	}
}

func (p printerPairings) Start(
	ctx context.Context, request httpapi.PrinterPairingRequest,
) (httpapi.PrinterPairing, error) {
	started, err := p.service.Start(ctx, printerpairing.StartRequest{
		Printer: request.Printer,
		Connect: printerpairing.Connect{
			Host: request.Connect.Host, Port: request.Connect.Port, TLS: request.Connect.TLS,
		},
		Name: request.Name,
		By:   httpapi.PrincipalFrom(ctx),
	})
	if err != nil {
		return httpapi.PrinterPairing{}, pairingError(err)
	}

	return pairingRow(started), nil
}

func (p printerPairings) Get(_ context.Context, id string) (httpapi.PrinterPairing, error) {
	read, err := p.service.Get(id)
	if err != nil {
		return httpapi.PrinterPairing{}, pairingError(err)
	}

	return pairingRow(read), nil
}

func (p printerPairings) Cancel(ctx context.Context, id string) (httpapi.PrinterPairing, error) {
	cancelled, err := p.service.Cancel(ctx, id)
	if err != nil {
		return httpapi.PrinterPairing{}, pairingError(err)
	}

	return pairingRow(cancelled), nil
}

func pairingRow(item printerpairing.Pairing) httpapi.PrinterPairing {
	row := httpapi.PrinterPairing{
		ID:      item.ID,
		Code:    item.Code,
		Printer: item.Printer,
		Connect: httpapi.PrinterPairingConnect{
			Host: item.Connect.Host, Port: item.Connect.Port, TLS: item.Connect.TLS,
		},
		State:     string(item.State),
		StartedAt: item.StartedAt.UTC(),
	}

	if !item.Until.IsZero() {
		until := item.Until.UTC()
		row.Until = &until
	}

	if item.State == printerpairing.StateAdmitted {
		row.Registration = &httpapi.PairedRegistration{
			Code:      item.Admitted.Code,
			Connector: item.Admitted.Connector,
			Serial:    item.Admitted.Serial,
			Model:     item.Admitted.Model,
			DeviceID:  item.Admitted.DeviceID,
		}
	}

	return row
}

// pairingError is the transport's sentinel for what the service reported. The
// printer's own words travel with a refusal, because they name the field.
func pairingError(err error) error {
	if locked, ok := errors.AsType[*printerpairing.LockedOutError](err); ok {
		return &httpapi.PrinterPairingLockedError{RetryAfter: locked.RetryAfter}
	}

	if invalid, ok := errors.AsType[*printerpairing.InvalidError](err); ok {
		return &httpapi.PrinterPairingDetailError{Err: httpapi.ErrPrinterPairingRefused, Detail: invalid.Said}
	}

	// Only the service's own sentences reach the caller, which is what
	// detailed marks. A failed dial carries the resolver's and the socket's
	// words, which are for the log.
	for _, pair := range []struct {
		from, to error
		detailed bool
	}{
		{printerpairing.ErrUnknownPairing, httpapi.ErrPrinterPairingUnknown, false},
		{printerpairing.ErrBusy, httpapi.ErrPrinterPairingBusy, false},
		{printerpairing.ErrPending, httpapi.ErrPrinterPairingPending, false},
		{printerpairing.ErrOpen, httpapi.ErrPrinterPairingOpen, false},
		{printerpairing.ErrTooMany, httpapi.ErrPrinterPairingLimited, false},
		{printerpairing.ErrDisabled, httpapi.ErrPrinterPairingDisabled, false},
		{printerpairing.ErrUnsupported, httpapi.ErrPrinterPairingUnsupported, false},
		{printerpairing.ErrUnreachable, httpapi.ErrPrinterPairingUnreachable, false},
		{printerpairing.ErrAddressRefused, httpapi.ErrPrinterPairingAddress, true},
		{printerpairing.ErrConnect, httpapi.ErrPrinterPairingConnect, true},
	} {
		if !errors.Is(err, pair.from) {
			continue
		}

		if pair.detailed {
			return &httpapi.PrinterPairingDetailError{Err: pair.to, Detail: detailOf(err, pair.from)}
		}

		return fmt.Errorf("%w: %w", pair.to, err)
	}

	return err
}

// detailOf is the part of an error's text after its sentinel's, which is what
// the service added about this one request: the address, or the limit.
func detailOf(err, sentinel error) string {
	text, prefix := err.Error(), sentinel.Error()
	if len(text) > len(prefix)+2 && text[:len(prefix)] == prefix {
		return text[len(prefix)+2:]
	}

	return ""
}
