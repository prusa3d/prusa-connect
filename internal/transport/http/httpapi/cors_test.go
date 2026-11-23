package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	farmOrigin  = "https://printers.example"
	otherOrigin = "https://elsewhere.example"
)

// corsServer is the production server with an allow-list and one device wire,
// so both listeners can be asked the same question.
func corsServer(t *testing.T, origins ...string) *Server {
	t.Helper()

	cfg := mountConfig([]Mount{
		{Pattern: "/p/ws", Handler: mountedText("connector"), Reason: "a frozen device wire", Device: true},
	})
	fixed := &fakeOrigins{}
	for _, origin := range origins {
		fixed.fixed = append(fixed.fixed, AllowedOrigin{Origin: origin, Source: OriginFromFile})
	}

	cfg.Origins = fixed

	server, err := New(cfg)
	require.NoError(t, err)

	return server
}

// callFrom is call with an Origin header and any others the browser would send.
func callFrom(handler http.Handler, method, path, origin string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Origin", origin)

	for name, value := range headers {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

// requireNoCORS holds a response to what every response carried before the
// allow-list existed: not one Access-Control header.
func requireNoCORS(t *testing.T, header http.Header) {
	t.Helper()

	for name := range header {
		require.False(t, strings.HasPrefix(name, "Access-Control-"), "response carries %s", name)
	}
}

// TestAnAllowedOriginIsEchoedWithCredentials is the read side: a page on the
// list may read the response, sends its credential, and sees the two headers
// a client reads after a write. The origin comes back as itself and never as
// *, because a browser refuses a credentialed answer that says *.
func TestAnAllowedOriginIsEchoedWithCredentials(t *testing.T) {
	t.Parallel()

	rec := callFrom(corsServer(t, farmOrigin).Handler(), http.MethodGet, readinessPath, farmOrigin, nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	header := rec.Header()
	require.Equal(t, farmOrigin, header.Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", header.Get("Access-Control-Allow-Credentials"))
	require.Equal(t, "ETag, Location", header.Get("Access-Control-Expose-Headers"))
	require.Contains(t, header.Values("Vary"), "Origin")
}

// TestAnOriginOffTheListGetsNoCORSHeader is the refusal half of the rule
// above. The response is otherwise the same one, so a page that is not allowed
// is told nothing about what the list holds.
func TestAnOriginOffTheListGetsNoCORSHeader(t *testing.T) {
	t.Parallel()

	rec := callFrom(corsServer(t, farmOrigin).Handler(), http.MethodGet, readinessPath, otherOrigin, nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	requireNoCORS(t, rec.Header())
}

// TestAnEmptyListSendsNothing is today's behaviour kept: a deployment that
// set no key answers a cross-origin request exactly as it did before the key
// existed, down to the absence of Vary.
func TestAnEmptyListSendsNothing(t *testing.T) {
	t.Parallel()

	rec := callFrom(corsServer(t).Handler(), http.MethodGet, readinessPath, farmOrigin, nil)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	requireNoCORS(t, rec.Header())
	require.Empty(t, rec.Header().Values("Vary"))
}

// TestAPreflightFromAnAllowedOriginIsAnsweredBeforeTheRouter is what a browser
// sends before a PUT with an If-Match, and what it has to read back before it
// sends the PUT at all. It is answered 204 with no body, on a path the router
// would answer 405 to, because OPTIONS is not an operation.
func TestAPreflightFromAnAllowedOriginIsAnsweredBeforeTheRouter(t *testing.T) {
	t.Parallel()

	rec := callFrom(corsServer(t, farmOrigin).Handler(), http.MethodOptions, readinessPath, farmOrigin,
		map[string]string{
			"Access-Control-Request-Method":  http.MethodPut,
			"Access-Control-Request-Headers": "if-match, content-type, authorization",
		})
	require.Equal(t, http.StatusNoContent, rec.Code, "body: %s", rec.Body.String())
	require.Empty(t, rec.Body.String())

	header := rec.Header()
	require.Equal(t, farmOrigin, header.Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", header.Get("Access-Control-Allow-Credentials"))
	require.Equal(t, corsAllowMethods, header.Get("Access-Control-Allow-Methods"))
	require.Equal(t, corsMaxAge, header.Get("Access-Control-Max-Age"))
	require.Contains(t, header.Values("Vary"), "Origin")

	// Every header an operation binds, so a browser that asks for any of them
	// is told yes. The list is the contract; a name missing from it is a
	// request the browser never sends.
	for _, name := range []string{
		"Authorization", "Content-Type", "If-Match", "If-None-Match",
		"If-Modified-Since", "Last-Event-ID", "Traceparent",
	} {
		require.Contains(t, header.Get("Access-Control-Allow-Headers"), name)
	}
}

// TestAPreflightFromAnOriginOffTheListAnswersAsBefore is the refusal the
// browser then enforces: no allow header, and the router's own answer to an
// OPTIONS nothing registered, which is the 405 it gave before the list existed.
func TestAPreflightFromAnOriginOffTheListAnswersAsBefore(t *testing.T) {
	t.Parallel()

	rec := callFrom(corsServer(t, farmOrigin).Handler(), http.MethodOptions, readinessPath, otherOrigin,
		map[string]string{"Access-Control-Request-Method": http.MethodPut})
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code, "body: %s", rec.Body.String())
	requireNoCORS(t, rec.Header())
}

// TestThePrinterListenerNeverSetsACORSHeader is the listener the allow-list
// must not reach. The device router serves wires a printer dials, and a page
// on an allowed origin that could read a printer's wire is exactly the
// exposure the second listener exists to prevent.
func TestThePrinterListenerNeverSetsACORSHeader(t *testing.T) {
	t.Parallel()

	devices := corsServer(t, farmOrigin).DeviceHandler()

	t.Run("a request from an allowed origin", func(t *testing.T) {
		t.Parallel()

		rec := callFrom(devices, http.MethodGet, "/p/ws", farmOrigin, nil)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		requireNoCORS(t, rec.Header())
		require.Empty(t, rec.Header().Values("Vary"))
	})

	t.Run("a preflight from an allowed origin", func(t *testing.T) {
		t.Parallel()

		rec := callFrom(devices, http.MethodOptions, "/p/ws", farmOrigin,
			map[string]string{"Access-Control-Request-Method": http.MethodGet})
		require.NotEqual(t, http.StatusNoContent, rec.Code)
		requireNoCORS(t, rec.Header())
	})
}
