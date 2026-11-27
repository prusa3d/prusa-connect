// Package printerpairingtest is a fake printer that serves the pairing
// endpoints of Buddy firmware's local API, for tests on both sides of a
// pairing. It follows lib/WUI/link_content/pairing_api.cpp and pairing.cpp of
// the firmware: one question at a time, a refusal or a timeout locks pairing,
// two refusals in a row turn it off, and an approval is answered once.
package printerpairingtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// The firmware's alphabet and shape: Crockford's base32 in two halves.
var codeShape = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}$`)

// The firmware's limits: the name it shows, the host it takes, and the host
// it stores once a name that does not end in prusa3d.com is not shortened.
const (
	maxName   = 20
	maxHost   = 35
	maxStored = 20
)

// Request is one pairing request the printer received, as it decoded it.
type Request struct {
	Code    string `json:"code"`
	Client  string `json:"client"`
	Name    string `json:"name"`
	Connect struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		TLS  bool   `json:"tls"`
	} `json:"connect"`
	// APIKey is the X-Api-Key header the request carried.
	APIKey string `json:"-"`
}

type phase int

const (
	none phase = iota
	pending
	approved
	refused
)

// Printer is the fake. Its methods are safe to call from the test while a
// client talks to it.
type Printer struct {
	Server *httptest.Server

	mu        sync.Mutex
	phase     phase
	request   Request
	requests  []Request
	busy      bool
	lockedFor int
	disabled  bool
	// legacy is firmware without pairing, which checks the key first.
	legacy bool
	// polls counts the GETs, so a test can wait for the client to have asked.
	polls     int
	withdrawn []string
	// onApprove runs when the person accepts, with the request accepted.
	onApprove func(Request)
}

// New starts a fake printer on loopback and stops it when the test ends.
func New(t *testing.T) *Printer {
	t.Helper()

	printer := &Printer{}
	printer.Server = httptest.NewServer(http.HandlerFunc(printer.serve))
	t.Cleanup(printer.Server.Close)

	return printer
}

// Address is the host:port the fake listens on.
func (p *Printer) Address() string {
	return strings.TrimPrefix(p.Server.URL, "http://")
}

// Busy makes the printer answer as one that prints.
func (p *Printer) Busy() { p.mu.Lock(); p.busy = true; p.mu.Unlock() }

// LockFor makes the printer refuse requests for seconds.
func (p *Printer) LockFor(seconds int) { p.mu.Lock(); p.lockedFor = seconds; p.mu.Unlock() }

// Disable makes the printer answer as one where pairing is off until reboot.
func (p *Printer) Disable() { p.mu.Lock(); p.disabled = true; p.mu.Unlock() }

// Legacy makes the printer answer as firmware with no pairing endpoints.
func (p *Printer) Legacy() { p.mu.Lock(); p.legacy = true; p.mu.Unlock() }

// OnApprove runs do when the person accepts, before the printer answers the
// next poll. A test makes the printer register with a server from here, as
// the firmware does.
func (p *Printer) OnApprove(do func(Request)) { p.mu.Lock(); p.onApprove = do; p.mu.Unlock() }

// Accept is the person pressing yes.
func (p *Printer) Accept() {
	p.mu.Lock()

	if p.phase != pending {
		p.mu.Unlock()

		return
	}

	p.phase = approved
	do, request := p.onApprove, p.request
	p.mu.Unlock()

	if do != nil {
		do(request)
	}
}

// Cancel is the person pressing cancel.
func (p *Printer) Cancel() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.phase == pending {
		p.phase = refused
	}
}

// TimeOut is the question closing because nobody answered it.
func (p *Printer) TimeOut() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.phase == pending {
		p.phase = none
	}
}

// Requests are the pairing requests the printer took, in order.
func (p *Printer) Requests() []Request {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]Request(nil), p.requests...)
}

// Showing reports whether a question is on the screen.
func (p *Printer) Showing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.phase == pending
}

// Polls is how many polls the printer answered.
func (p *Printer) Polls() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.polls
}

// Withdrawn are the codes a client withdrew.
func (p *Printer) Withdrawn() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]string(nil), p.withdrawn...)
}

func (p *Printer) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.legacy {
		if r.Header.Get("X-Api-Key") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="Printer API"`)
		}

		w.WriteHeader(http.StatusUnauthorized)

		return
	}

	switch {
	case r.URL.Path == "/api/v1/pairing" && r.Method == http.MethodPost:
		p.start(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/pairing/") && r.Method == http.MethodGet:
		p.poll(w, strings.TrimPrefix(r.URL.Path, "/api/v1/pairing/"))
	case strings.HasPrefix(r.URL.Path, "/api/v1/pairing/") && r.Method == http.MethodDelete:
		p.withdraw(w, strings.TrimPrefix(r.URL.Path, "/api/v1/pairing/"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (p *Printer) start(w http.ResponseWriter, r *http.Request) {
	var request Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Couldn't parse JSON", http.StatusBadRequest)

		return
	}

	request.APIKey = r.Header.Get("X-Api-Key")

	switch {
	case !codeShape.MatchString(request.Code):
		http.Error(w, "Invalid code", http.StatusBadRequest)

		return
	case request.Client != "Prusa Connect" && request.Client != "PrusaLink",
		len(request.Name) > maxName, len(request.Connect.Host) > maxHost:
		http.Error(w, "Invalid pairing request", http.StatusUnprocessableEntity)

		return
	case request.Connect.Host != "" && len(request.Connect.Host) > maxStored &&
		!strings.HasSuffix(request.Connect.Host, "prusa3d.com"):
		http.Error(w, "Invalid Connect host", http.StatusUnprocessableEntity)

		return
	case p.busy:
		reason(w, http.StatusConflict, "busy")

		return
	case p.disabled:
		reason(w, http.StatusTooManyRequests, "disabled")

		return
	case p.lockedFor > 0:
		w.Header().Set("Retry-After", strconv.Itoa(p.lockedFor))
		reason(w, http.StatusTooManyRequests, "locked_out")

		return
	case p.phase != none:
		reason(w, http.StatusConflict, "pending")

		return
	}

	p.phase, p.request = pending, request
	p.requests = append(p.requests, request)

	w.WriteHeader(http.StatusAccepted)
}

func (p *Printer) poll(w http.ResponseWriter, code string) {
	p.polls++

	if p.phase == none || code != p.request.Code {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	switch p.phase {
	case pending:
		w.WriteHeader(http.StatusAccepted)
	case approved:
		p.phase = none
		// The key goes only to PrusaLink, as firmware commit 7bb7738fe sends it.
		w.Header().Set("Content-Type", "application/json")

		if p.request.Client == "PrusaLink" {
			_, _ = w.Write([]byte(`{"api_key":"secretprinterkey","user":"maker"}`))

			return
		}

		_, _ = w.Write([]byte(`{"paired":true}`))
	case refused:
		p.phase = none
		w.WriteHeader(http.StatusForbidden)
	case none:
	}
}

func (p *Printer) withdraw(w http.ResponseWriter, code string) {
	if p.phase != pending || code != p.request.Code {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	p.phase = none
	p.withdrawn = append(p.withdrawn, code)

	w.WriteHeader(http.StatusNoContent)
}

func reason(w http.ResponseWriter, status int, why string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"reason":"` + why + `"}`))
}
