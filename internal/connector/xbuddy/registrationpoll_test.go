package xbuddy

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// lockedBuffer is a log sink a handler goroutine and the test both touch.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// The answer Buddy firmware acts on is the status and the Token header alone:
// it ignores the body and keeps at most 20 characters of the header
// (src/connect/registrator.cpp, ExtractToken). So a printer an operator
// approved has to get 200 and the whole token in that header on its next
// poll, or it shows its code for ever.
func TestAnOperatorsApprovalAnswersTheNextPollWithTheWholeToken(t *testing.T) {
	t.Parallel()

	logs := &lockedBuffer{}
	driver := NewDriver(ModeToken, 4, 0, "",
		slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	host := &takeOne{baseline: driver.Baseline(), sessions: make(chan *Session, 1)}

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
			`{"sn":"SN-POLL","fingerprint":"FINGERPRINT-POLL","printer_type":"3.1.0","firmware":"6.5.0"}`,
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

	poll := func() *http.Response {
		t.Helper()

		// The request the firmware sends: no body, the code in its own header.
		request, requestErr := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+registerPath, nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}

		request.Header.Set(codeHeader, code)
		request.Header.Set(modelHeader, "XL")
		request.Header.Set(firmwareHeader, "6.5.0")

		answered, requestErr := server.Client().Do(request)
		if requestErr != nil {
			t.Fatal(requestErr)
		}

		_ = answered.Body.Close()

		return answered
	}

	if waiting := poll(); waiting.StatusCode != http.StatusAccepted {
		t.Fatalf("a poll before the approval answered %d, want 202", waiting.StatusCode)
	}

	if _, err = driver.Approve(t.Context(), code, "user:operator"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	approved := poll()
	token := approved.Header.Get(tokenHeader)

	switch {
	case approved.StatusCode != http.StatusOK:
		t.Fatalf("the poll after the approval answered %d, want 200", approved.StatusCode)
	case token == "" || len(token) > 20:
		t.Fatalf("the Token header is %q; the firmware keeps 1 to 20 characters of it", token)
	}

	said := logs.String()
	for _, want := range []string{"answer=pending", "answer=token", "code=" + code} {
		if !strings.Contains(said, want) {
			t.Fatalf("the debug log of the polls has no %q:\n%s", want, said)
		}
	}

	if strings.Contains(said, token) {
		t.Fatal("the debug log carries the token")
	}
}
