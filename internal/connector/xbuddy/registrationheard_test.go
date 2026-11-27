package xbuddy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/prusa3d/prusa-connect/internal/kernel/admission"
)

// A registration is reported with the address it came from, before the printer
// is given its code. An approval made there answers the printer's first poll,
// which is what lets a printer paired at its own screen join with nothing
// more from the person (ADR-20261126-qca).
func TestARegistrationIsHeardWithItsAddressBeforeTheCodeIsAnswered(t *testing.T) {
	t.Parallel()

	driver := NewDriver(ModeToken, 4, 0, "", nil)
	host := &takeOne{baseline: driver.Baseline(), sessions: make(chan *Session, 1)}

	var (
		mu    sync.Mutex
		heard []admission.Registration
		from  []netip.Addr
	)

	driver.HearRegistrations(func(ctx context.Context, registration admission.Registration, at netip.Addr) {
		mu.Lock()
		heard = append(heard, registration)
		from = append(from, at)
		mu.Unlock()

		if _, err := driver.Approve(ctx, registration.Code, "user:alice"); err != nil {
			t.Errorf("approve the heard registration: %v", err)
		}
	})

	handler, err := driver.Handler(host)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	begin, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL+registerPath,
		strings.NewReader(
			`{"sn":"SN-HEARD","fingerprint":"FINGERPRINT-HEARD","printer_type":"1.4.0","firmware":"6.4.0"}`,
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	begun, err := server.Client().Do(begin)
	if err != nil {
		t.Fatal(err)
	}

	_ = begun.Body.Close()

	code := begun.Header.Get(codeHeader)

	mu.Lock()
	if len(heard) != 1 {
		mu.Unlock()
		t.Fatalf("heard %d registrations, want 1", len(heard))
	}

	got, at := heard[0], from[0]
	mu.Unlock()

	if got.Code != code || got.Serial != "SN-HEARD" || got.Fingerprint != "FINGERPRINT-HEARD" || got.Model != "1.4.0" {
		t.Fatalf("heard %+v for code %q", got, code)
	}

	if at != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("heard it from %s, want the connection's own remote address", at)
	}

	poll, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+registerPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	poll.Header.Set(codeHeader, code)

	polled, err := server.Client().Do(poll)
	if err != nil {
		t.Fatal(err)
	}

	_ = polled.Body.Close()

	if polled.StatusCode != http.StatusOK || polled.Header.Get(tokenHeader) == "" {
		t.Fatalf("the first poll answered %d with no token, so the approval came too late", polled.StatusCode)
	}
}

// A forwarded header names whatever the caller wrote, so it never stands in
// for the address a registration came from.
func TestAForwardedHeaderIsNotTheAddressARegistrationCameFrom(t *testing.T) {
	t.Parallel()

	driver := NewDriver(ModeToken, 4, 0, "", nil)

	var at netip.Addr

	driver.HearRegistrations(func(_ context.Context, _ admission.Registration, from netip.Addr) { at = from })

	request := httptest.NewRequest(http.MethodPost, registerPath,
		strings.NewReader(`{"sn":"SN","fingerprint":"FINGERPRINT-FORWARDED"}`))
	request.RemoteAddr = "192.168.1.77:40000"
	request.Header.Set("X-Forwarded-For", "192.168.1.20")
	request.Header.Set("X-Real-IP", "192.168.1.20")

	driver.handleBeginRegistration(httptest.NewRecorder(), request)

	if at != netip.MustParseAddr("192.168.1.77") {
		t.Fatalf("heard the registration from %s, want the connection's 192.168.1.77", at)
	}
}
