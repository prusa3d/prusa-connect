package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/connector/printerpairing/printerpairingtest"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// pairingRig is the shipping connector in the mode that asks a person, served
// on loopback, with the pairing service heard by it.
type pairingRig struct {
	wire       *httptest.Server
	admissions registrations
	pairings   printerPairings
}

func newPairingRig(t *testing.T) pairingRig {
	t.Helper()

	plane := newPlane(t)

	built, err := mountConnectors(t.Context(), config.Default(), "", discard(), plane, nil, nil, resource.AdmissionAsk)
	require.NoError(t, err)

	var wire http.Handler

	for _, mount := range built.mounts {
		if mount.Device {
			wire = mount.Handler
		}
	}

	require.NotNil(t, wire, "the shipping connector mounted no device wire")

	server := httptest.NewServer(wire)
	t.Cleanup(server.Close)

	admissions := newRegistrations(built.admissions, plane)
	service := newPrinterPairings(admissions, built.admissions, discard(), func(netip.Addr) bool { return true })
	t.Cleanup(service.Close)

	return pairingRig{wire: server, admissions: admissions, pairings: printerPairings{service: service}}
}

// register is the printer's own registration, the first step of the xBuddy
// handshake. It returns the code the printer was given.
func (r pairingRig) register(t *testing.T, fingerprint string) string {
	t.Helper()

	request, err := http.NewRequestWithContext(context.WithoutCancel(t.Context()), http.MethodPost,
		r.wire.URL+"/p/register", strings.NewReader(
			`{"sn":"SN-PAIRED","fingerprint":"`+fingerprint+`","printer_type":"1.4.0","firmware":"6.5.0"}`))
	require.NoError(t, err)

	response, err := r.wire.Client().Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)

	return response.Header.Get("Code")
}

// token is the printer's poll for its credential, empty while it waits.
func (r pairingRig) token(t *testing.T, code string) string {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, r.wire.URL+"/p/register", nil)
	require.NoError(t, err)
	request.Header.Set("Code", code)

	response, err := r.wire.Client().Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	return response.Header.Get("Token")
}

// TestAPrinterPairedAtItsScreenJoinsWithNoSecondStep is the whole journey
// against a fake printer: the server asks it to pair, the person presses yes,
// the printer registers over the xBuddy wire, and the registration is
// approved by the pairing in a mode where every other one waits for a person.
func TestAPrinterPairedAtItsScreenJoinsWithNoSecondStep(t *testing.T) {
	t.Parallel()

	rig := newPairingRig(t)
	printer := printerpairingtest.New(t)

	registered := make(chan string, 1)

	printer.OnApprove(func(printerpairingtest.Request) {
		registered <- rig.register(t, "FINGERPRINT-OF-A-PAIRED-PRINTER-0123456789ABCDEF")
	})

	started, err := rig.pairings.Start(t.Context(), httpapi.PrinterPairingRequest{
		Printer: printer.Address(),
		Connect: httpapi.PrinterPairingConnect{Host: "192.168.1.50", Port: 8080},
	})
	require.NoError(t, err)
	require.Equal(t, "waiting", started.State)
	require.Len(t, printer.Requests(), 1)
	require.Equal(t, started.Code, printer.Requests()[0].Code)

	printer.Accept()

	code := <-registered

	require.Eventually(t, func() bool {
		read, readErr := rig.pairings.Get(t.Context(), started.ID)

		return readErr == nil && read.State == "admitted"
	}, 10*time.Second, 20*time.Millisecond)

	read, err := rig.pairings.Get(t.Context(), started.ID)
	require.NoError(t, err)
	require.NotNil(t, read.Registration)
	require.Equal(t, code, read.Registration.Code)
	require.NotEmpty(t, read.Registration.DeviceID, "the approval bound no device the page could open")
	require.Equal(t, "SN-PAIRED", read.Registration.Serial)

	require.NotEmpty(t, rig.token(t, code), "the printer was not given its credential")

	waiting, err := rig.admissions.Pending(t.Context())
	require.NoError(t, err)
	require.Empty(t, waiting, "the paired printer still waits for a person")
}

// TestARegistrationNoPairingLedToWaitsForAPerson holds the refusal half: in
// the mode that asks a person, a printer that registers with no pairing open
// for its address is never approved by the pairing service.
func TestARegistrationNoPairingLedToWaitsForAPerson(t *testing.T) {
	t.Parallel()

	rig := newPairingRig(t)
	printer := printerpairingtest.New(t)

	started, err := rig.pairings.Start(t.Context(), httpapi.PrinterPairingRequest{
		Printer: printer.Address(),
		Connect: httpapi.PrinterPairingConnect{Host: "192.168.1.50", Port: 8080},
	})
	require.NoError(t, err)

	// The person cancels, and the printer registers anyway, as one already
	// pointed at this server does when somebody chooses Add Printer to
	// Connect in its menu.
	printer.Cancel()

	require.Eventually(t, func() bool {
		read, readErr := rig.pairings.Get(t.Context(), started.ID)

		return readErr == nil && read.State == "refused"
	}, 10*time.Second, 20*time.Millisecond)

	code := rig.register(t, "FINGERPRINT-OF-A-PRINTER-NOBODY-PAIRED-0123456789")

	require.Empty(t, rig.token(t, code), "a registration no pairing led to was approved")

	waiting, err := rig.admissions.Pending(t.Context())
	require.NoError(t, err)
	require.Len(t, waiting, 1)
	require.Equal(t, code, waiting[0].Code)
}
