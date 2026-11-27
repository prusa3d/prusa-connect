package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakePrinterPairings answers whatever a case set, and records what it was
// asked.
type fakePrinterPairings struct {
	mu        sync.Mutex
	fail      error
	started   []PrinterPairingRequest
	cancelled []string
}

var _ PrinterPairings = (*fakePrinterPairings)(nil)

func (f *fakePrinterPairings) Start(_ context.Context, request PrinterPairingRequest) (PrinterPairing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.fail != nil {
		return PrinterPairing{}, f.fail
	}

	f.started = append(f.started, request)

	return PrinterPairing{
		ID: "p1", Code: "K7QX-29FM", Printer: request.Printer + ":80", Connect: request.Connect,
		State: "waiting", StartedAt: time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakePrinterPairings) Get(_ context.Context, id string) (PrinterPairing, error) {
	if id != "p1" {
		return PrinterPairing{}, ErrPrinterPairingUnknown
	}

	return PrinterPairing{
		ID: "p1", Code: "K7QX-29FM", State: "admitted",
		Registration: &PairedRegistration{Code: "K7PQ2XR4MN", Connector: "xbuddy", DeviceID: "dev-1"},
	}, nil
}

func (f *fakePrinterPairings) Cancel(_ context.Context, id string) (PrinterPairing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if id != "p1" {
		return PrinterPairing{}, ErrPrinterPairingUnknown
	}

	f.cancelled = append(f.cancelled, id)

	return PrinterPairing{ID: "p1", Code: "K7QX-29FM", State: "withdrawn"}, nil
}

func printerPairingServer(t *testing.T, pairings PrinterPairings, grants GrantStore) http.Handler {
	t.Helper()

	server, err := New(Config{
		Version:         "test",
		Principals:      fixedPrincipal("user:operator"),
		Resources:       seededStore(),
		Presets:         seededPresets(),
		Endpoints:       seededEndpoints(),
		Grants:          grants,
		Sessions:        newFakeSessions(),
		Accounts:        &fakeAccounts{},
		Readiness:       &fakeReadiness{report: readyReport()},
		Devices:         noPrinters{},
		Stores:          noStorages{},
		Events:          noEvents{},
		PrinterPairings: pairings,
	})
	require.NoError(t, err)

	return server.Handler()
}

const printerPairingBody = `{"printer":"192.168.1.20","connect":{"host":"192.168.1.50","port":8080,"tls":false}}`

func TestAnOperatorStartsReadsAndWithdrawsAPrinterPairing(t *testing.T) {
	t.Parallel()

	pairings := &fakePrinterPairings{}
	graph := &grantingStore{allow: true}
	handler := printerPairingServer(t, pairings, graph)

	rec := call(t, handler, http.MethodPost, printerPairingsPath, printerPairingBody)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, "resource_kind:driver", graph.asked[0].String())

	var started PrinterPairing

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &started))
	require.Equal(t, "K7QX-29FM", started.Code)
	require.Equal(t, "waiting", started.State)
	require.Equal(t, []PrinterPairingRequest{{
		Printer: "192.168.1.20",
		Connect: PrinterPairingConnect{Host: "192.168.1.50", Port: 8080},
	}}, pairings.started)

	rec = call(t, handler, http.MethodGet, "/api/v1/printer-pairings/p1", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var read PrinterPairing

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &read))
	require.Equal(t, "admitted", read.State)
	require.Equal(t, "dev-1", read.Registration.DeviceID)

	rec = call(t, handler, http.MethodDelete, "/api/v1/printer-pairings/p1", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []string{"p1"}, pairings.cancelled)

	rec = call(t, handler, http.MethodGet, "/api/v1/printer-pairings/nothing", "")
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// Pairing a printer admits it, so a caller who may not admit a device reaches
// none of the operations and the printer is never asked.
func TestNoPrinterPairingReachesACallerWhoMayNotAdmitADevice(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		method, path, body string
	}{
		"starting":    {http.MethodPost, printerPairingsPath, printerPairingBody},
		"reading":     {http.MethodGet, "/api/v1/printer-pairings/p1", ""},
		"withdrawing": {http.MethodDelete, "/api/v1/printer-pairings/p1", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pairings := &fakePrinterPairings{}
			handler := printerPairingServer(t, pairings, &grantingStore{allow: false})

			rec := call(t, handler, tc.method, tc.path, tc.body)
			require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
			require.Empty(t, pairings.started, "a refused caller still asked a printer to pair")
			require.Empty(t, pairings.cancelled)
		})
	}
}

func TestEveryAnswerAPrinterGivesIsItsOwnFault(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		fail   error
		status int
		code   string
	}{
		"busy":         {ErrPrinterPairingBusy, http.StatusConflict, "core.printer/PAIRING_BUSY"},
		"pending":      {ErrPrinterPairingPending, http.StatusConflict, "core.printer/PAIRING_PENDING"},
		"already open": {ErrPrinterPairingOpen, http.StatusConflict, "core.printer/PAIRING_LIMITED"},
		"disabled":     {ErrPrinterPairingDisabled, http.StatusTooManyRequests, "core.printer/PAIRING_DISABLED"},
		"old firmware": {ErrPrinterPairingUnsupported, http.StatusUnprocessableEntity, "core.printer/PAIRING_UNSUPPORTED"},
		"refused": {
			&PrinterPairingDetailError{Err: ErrPrinterPairingRefused, Detail: "Invalid Connect host"},
			http.StatusUnprocessableEntity, "core.printer/PAIRING_REFUSED",
		},
		"unreachable":    {ErrPrinterPairingUnreachable, http.StatusBadGateway, "core.printer/PAIRING_UNREACHABLE"},
		"public address": {ErrPrinterPairingAddress, http.StatusUnprocessableEntity, "core.printer/PAIRING_ADDRESS_REFUSED"},
		"server address": {ErrPrinterPairingConnect, http.StatusUnprocessableEntity, "core.printer/PAIRING_SERVER_ADDRESS"},
		"locked": {
			&PrinterPairingLockedError{RetryAfter: 239500 * time.Millisecond},
			http.StatusTooManyRequests, "core.printer/PAIRING_LOCKED",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler := printerPairingServer(t, &fakePrinterPairings{fail: tc.fail}, &grantingStore{allow: true})

			rec := call(t, handler, http.MethodPost, printerPairingsPath, printerPairingBody)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())

			var problem struct {
				Code   string         `json:"code"`
				Detail string         `json:"detail"`
				Params map[string]any `json:"params"`
			}

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			require.Equal(t, tc.code, problem.Code)

			switch name {
			case "locked":
				require.Equal(t, "240", rec.Header().Get("Retry-After"), "a wait is rounded up")
				require.InDelta(t, 240, problem.Params[RetryAfterParam], 0)
			case "refused":
				require.Contains(t, problem.Detail, "Invalid Connect host", "the printer's words name the field")
			}
		})
	}
}

func TestAServerThatPairsNoPrinterSaysSo(t *testing.T) {
	t.Parallel()

	handler := printerPairingServer(t, nil, &grantingStore{allow: true})

	rec := call(t, handler, http.MethodPost, printerPairingsPath, printerPairingBody)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())

	rec = call(t, handler, http.MethodGet, "/api/v1/printer-pairings/p1", "")
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}
