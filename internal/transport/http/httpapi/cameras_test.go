package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

// stubCameras is the media plane as this transport sees it. Every answer comes
// from a field, so the operations are driven without a device, a driver or a
// supervisor.
type stubCameras struct {
	listed []Camera
	shot   CameraSnapshot
	view   CameraStream
	answer CameraAnswer
	live   CameraLive
	types  []CameraType
	err    error

	// offers records the SDP body each negotiation was handed, so a handler
	// that reads the body wrongly is visible as the wrong bytes rather than as
	// a status code that happens to match.
	offers []string

	// principals records who each call was made on behalf of. A camera is
	// authorized by the grant on the printer it watches, so a handler that drops
	// the resolved principal hands the fleet a question about nobody.
	principals []string

	// pushed records the frames the push path handed over, so a case reads what
	// reached the plane rather than only the status the caller saw.
	pushed []CameraFrame

	// mu guards the records: the published-examples test drives one stub from
	// parallel subtests.
	mu sync.Mutex
}

// record notes who a call was made for, under the lock.
func (c *stubCameras) record(principal string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.principals = append(c.principals, principal)
}

func (c *stubCameras) Cameras(_ context.Context, principal string) ([]Camera, error) {
	c.record(principal)

	return c.listed, c.err
}

func (c *stubCameras) Snapshot(_ context.Context, principal, _ string) (CameraSnapshot, error) {
	c.record(principal)

	return c.shot, c.err
}

func (c *stubCameras) Stream(_ context.Context, principal, _ string) (CameraStream, error) {
	c.record(principal)

	return c.view, c.err
}

func (c *stubCameras) Watch(_ context.Context, principal, _, offer string) (CameraAnswer, error) {
	c.record(principal)

	c.mu.Lock()
	c.offers = append(c.offers, offer)
	c.mu.Unlock()

	return c.answer, c.err
}

func (c *stubCameras) Live(_ context.Context, principal, _ string) (CameraLive, error) {
	c.record(principal)

	if c.err != nil {
		return nil, c.err
	}

	return c.live, nil
}

func (c *stubCameras) Push(_ context.Context, principal, _ string, frame CameraFrame) error {
	c.record(principal)

	c.mu.Lock()
	c.pushed = append(c.pushed, frame)
	c.mu.Unlock()

	return c.err
}

func (c *stubCameras) CameraTypes(_ context.Context, principal string) ([]CameraType, error) {
	c.record(principal)

	return c.types, c.err
}

// serverWithCameras is the production server over a media plane the caller
// controls.
func serverWithCameras(t *testing.T, fleet CameraFleet) *Server {
	t.Helper()

	server, err := New(Config{
		Version:    "test",
		Principals: fixedPrincipal("user:operator"),
		Resources:  seededStore(),
		Presets:    seededPresets(),
		Endpoints:  seededEndpoints(),
		Grants:     &fakeGrantStore{},
		Sessions:   newFakeSessions(),
		Accounts:   &fakeAccounts{},
		Readiness:  &fakeReadiness{report: readyReport()},
		Devices:    noPrinters{},
		Cameras:    fleet,
		Stores:     noStorages{},
		Events:     noEvents{},
	})
	require.NoError(t, err)

	return server
}

// A camera that is declared and not serving is still listed. A grid that hid it
// would lose a cell without saying why, and the operator would have to read the
// resource API to find out.
func TestTheCameraListingCarriesADeclaredCameraThatIsNotServing(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{listed: []Camera{
		{Name: "front", DeviceID: "mk4-01", Formats: []string{"jpeg"}, Serving: true, Snapshot: true},
		{Name: "back", DeviceID: "mk4-02", Formats: []string{"jpeg"}, Serving: false},
	}}

	rec := call(t, serverWithCameras(t, fleet).Handler(), http.MethodGet, camerasPath, "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body struct {
		Items []Camera `json:"items"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Items, 2)
	require.Equal(t, "back", body.Items[1].Name)
	require.False(t, body.Items[1].Serving)
	require.Equal(t, []string{"user:operator"}, fleet.principals,
		"the fleet was asked about nobody, so the grant on the printer decides nothing")
}

// A camera says how often it has a new frame, so a page refreshes a still image
// at the rate the camera can answer it rather than at a rate somebody picked.
// A camera that declares no cadence carries no member at all, which is what
// lets a client tell "ask every 200 ms" apart from "wait as long as you like".
func TestTheCameraListingCarriesTheDeclaredInterval(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{listed: []Camera{
		{Name: "front", DeviceID: "mk4-01", Serving: true, Snapshot: true, MinIntervalMS: 200},
		{Name: "quiet", DeviceID: "mk4-02", Serving: true, Snapshot: true},
	}}

	rec := call(t, serverWithCameras(t, fleet).Handler(), http.MethodGet, camerasPath, "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body struct {
		Items []map[string]any `json:"items"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Items, 2)
	require.InDelta(t, 200.0, body.Items[0]["min_interval_ms"], 0,
		"the camera declares 200 ms and the listing has to publish it")
	require.NotContains(t, body.Items[1], "min_interval_ms",
		"a camera that declares no cadence carries no member, not a zero")
}

// A caller who sees no camera is answered an empty array. A null teaches every
// client to guard a field that is a list on every other answer.
// The camera types are what the fleet answers, asked as the caller, and a
// module's type carries its title and namespace (ADR-20261106-km2).
func TestTheCameraTypesAreListed(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{types: []CameraType{
		{Type: "generic"},
		{Type: "com.github.aux.acme.cam/buddy3d", Title: "Buddy3D camera", Module: "com.github.aux.acme.cam"},
	}}

	rec := call(t, serverWithCameras(t, fleet).Handler(), http.MethodGet, cameraTypesPath, "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body struct {
		Items []CameraType `json:"items"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, fleet.types, body.Items)
	require.Equal(t, []string{"user:operator"}, fleet.principals)
}

func TestACallerWhoSeesNoCameraIsAnsweredAnEmptyArray(t *testing.T) {
	t.Parallel()

	rec := call(t, serverWithCameras(t, &stubCameras{}).Handler(), http.MethodGet, camerasPath, "")

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"items":[]}`, rec.Body.String())
}

// The still image carries what makes a grid of four hundred thumbnails cheap:
// a validator to send back, a private max-age, and a type the browser will not
// sniff past.
func TestASnapshotCarriesItsValidatorAndRefusesToBeSniffed(t *testing.T) {
	t.Parallel()

	taken := time.Date(2026, time.August, 26, 10, 30, 0, 0, time.UTC)
	fleet := &stubCameras{shot: CameraSnapshot{
		Bytes:     []byte("\xff\xd8\xff\xe0jpeg"),
		MediaType: "image/jpeg",
		Taken:     taken,
		MaxAge:    5 * time.Second,
	}}

	rec := call(t, serverWithCameras(t, fleet).Handler(), http.MethodGet, camerasPath+"/front/snapshot", "")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "private, max-age=5", rec.Header().Get("Cache-Control"))
	require.Equal(t, taken.Format(http.TimeFormat), rec.Header().Get("Last-Modified"))
	require.Equal(t, taken.Add(5*time.Second+snapshotGrace).Format(http.TimeFormat),
		rec.Header().Get("Expires"), "a cache expiring on the cadence refetches a frame the camera still holds")
	require.Equal(t, "\xff\xd8\xff\xe0jpeg", rec.Body.String())
}

// A client holding the current frame is answered 304 with no body, which is the
// whole reason If-Modified-Since is bound on this operation.
func TestAClientHoldingTheCurrentFrameIsAnsweredNotModified(t *testing.T) {
	t.Parallel()

	taken := time.Date(2026, time.August, 26, 10, 30, 0, 0, time.UTC)
	fleet := &stubCameras{shot: CameraSnapshot{
		Bytes: []byte("jpeg"), MediaType: "image/jpeg", Taken: taken, MaxAge: time.Second,
	}}

	rec := write(t, serverWithCameras(t, fleet).Handler(), http.MethodGet,
		camerasPath+"/front/snapshot", http.Header{"If-Modified-Since": {taken.Format(http.TimeFormat)}}, "")

	require.Equal(t, http.StatusNotModified, rec.Code)
	require.Empty(t, rec.Body.String())
}

// RFC 9110 section 13.1.3 requires a date the server cannot parse to be ignored
// rather than refused, so the frame is sent.
func TestAnUnparseableIfModifiedSinceIsIgnoredRatherThanRefused(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{shot: CameraSnapshot{
		Bytes: []byte("jpeg"), MediaType: "image/jpeg", Taken: time.Now(), MaxAge: time.Second,
	}}

	rec := write(t, serverWithCameras(t, fleet).Handler(), http.MethodGet,
		camerasPath+"/front/snapshot", http.Header{"If-Modified-Since": {"yesterday"}}, "")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "jpeg", rec.Body.String())
}

// A working camera that has produced no frame yet is not a failure. It is a
// camera with nothing to show.
func TestACameraWithNoFrameYetAnswersNoContent(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{shot: CameraSnapshot{MediaType: "image/jpeg", MaxAge: 3 * time.Second}}

	rec := call(t, serverWithCameras(t, fleet).Handler(), http.MethodGet, camerasPath+"/front/snapshot", "")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())
	require.Equal(t, "private, max-age=3", rec.Header().Get("Cache-Control"))
}

// The live view is relayed as the driver framed it, and it carries the header
// that stops a buffering reverse proxy holding an endless body and delivering
// nothing.
func TestTheLiveViewIsRelayedUnbufferedAndTheBodyIsClosed(t *testing.T) {
	t.Parallel()

	body := &countingCloser{Reader: strings.NewReader("--frame\r\n\r\njpeg\r\n")}
	fleet := &stubCameras{view: CameraStream{Body: body, MediaType: "multipart/x-mixed-replace; boundary=frame"}}

	rec := call(t, serverWithCameras(t, fleet).Handler(), http.MethodGet, camerasPath+"/front/stream", "")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "multipart/x-mixed-replace; boundary=frame", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "--frame\r\n\r\njpeg\r\n", rec.Body.String())
	require.Equal(t, 1, body.closes, "a viewer that goes away leaves the driver's stream open")
}

// A camera the caller may not see, one that is not declared, and one that emits
// no streamable format are the same answer. Any other refusal tells a caller
// that a camera exists on a printer they hold no grant for.
func TestACameraTheCallerMayNotSeeIsTheAbsentAnswer(t *testing.T) {
	t.Parallel()

	handler := serverWithCameras(t, &stubCameras{err: CameraNotFound("front")}).Handler()

	for _, path := range []string{camerasPath + "/front/snapshot", camerasPath + "/front/stream"} {
		rec := call(t, handler, http.MethodGet, path, "")
		require.Equal(t, http.StatusNotFound, rec.Code, "%s body: %s", path, rec.Body.String())
	}
}

// The old backlog. A live view is a response that never ends, and the server arms
// its whole-response write timeout when the request arrives, so a viewer still
// watching at sixty seconds is cut off. The relay lifts that deadline and arms
// one of its own per frame, the way the event stream does.
//
// The bound here is a tenth of a second rather than the production sixty, so
// the case runs in well under a second.
func TestACameraRelayKeepsServingPastTheServersWriteTimeout(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()
	fleet := &stubCameras{view: CameraStream{
		Body:      reader,
		MediaType: "multipart/x-mixed-replace; boundary=frame",
	}}

	listener := httptest.NewUnstartedServer(serverWithCameras(t, fleet).Handler())
	listener.Config.WriteTimeout = 100 * time.Millisecond
	listener.Start()

	t.Cleanup(listener.Close)

	go func() {
		defer func() { _ = writer.Close() }()

		for frame := range relayTestFrames {
			time.Sleep(60 * time.Millisecond)

			if _, err := fmt.Fprintf(writer, "--frame\r\n\r\njpeg-%d\r\n", frame); err != nil {
				return
			}
		}
	}()

	answer, err := listener.Client().Get(listener.URL + camerasPath + "/front/stream")
	require.NoError(t, err)

	defer func() { _ = answer.Body.Close() }()

	require.Equal(t, http.StatusOK, answer.StatusCode)

	served, err := io.ReadAll(answer.Body)
	require.NoError(t, err, "the relay was cut off at the server's write timeout")
	require.Contains(t, string(served), "jpeg-5", "the last frame never reached the viewer")
}

// relayTestFrames is how many frames the case above sends. Six at sixty
// milliseconds is three and a half times the deadline it runs under.
const relayTestFrames = 6

// The deadline is lifted once and armed again for each frame, so a viewer that
// stops draining is still bounded while one that keeps up is left alone.
func TestTheCameraRelayArmsAWriteDeadlinePerFrame(t *testing.T) {
	t.Parallel()

	frames := &frameReader{frames: []string{
		"--frame\r\n\r\none\r\n", "--frame\r\n\r\ntwo\r\n", "--frame\r\n\r\nthree\r\n",
	}}
	fleet := &stubCameras{view: CameraStream{
		Body:      frames,
		MediaType: "multipart/x-mixed-replace; boundary=frame",
	}}

	req := httptest.NewRequest(http.MethodGet, camerasPath+"/front/stream", nil)
	writer, rec := serveRecordingDeadlines(serverWithCameras(t, fleet).Handler(), req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, cleared(writer.writes), "the server's whole-response write timeout was never lifted")
	require.Equal(t, len(frames.frames), countArmed(writer.writes),
		"the write deadline was not armed once per frame")
}

// countArmed is how many real deadlines were set, the zero time left out.
func countArmed(set []time.Time) int {
	armed := 0

	for _, at := range set {
		if !at.IsZero() {
			armed++
		}
	}

	return armed
}

// frameReader hands over one frame per Read, which is how a driver delivers a
// live view: a whole picture at a time, never a slice of the next one.
type frameReader struct {
	frames []string
	at     int
}

func (r *frameReader) Read(p []byte) (int, error) {
	if r.at >= len(r.frames) {
		return 0, io.EOF
	}

	n := copy(p, r.frames[r.at])
	r.at++

	return n, nil
}

func (r *frameReader) Close() error { return nil }

// countingCloser reports how often the transport closed the driver's stream.
type countingCloser struct {
	io.Reader

	closes int
}

func (c *countingCloser) Close() error {
	c.closes++

	return nil
}

// pushFrame is one PUT of a frame, with the media type a camera sends and
// nothing the JSON helper would add. The camera is the one every case here
// declares, because what is under test is the body and the answer.
func pushFrame(t *testing.T, handler http.Handler, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, camerasPath+"/front/snapshot", bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/jpeg")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

// oneJPEG is the smallest thing that is a JPEG as far as this path reads one:
// the start-of-image marker and the first byte of the marker after it, with
// some bytes behind them.
func oneJPEG() []byte {
	return append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("a frame from a camera")...)
}

// TestACameraPushesItsOwnFrame is task 19 of the old backlog. The camera cannot be read,
// so it sends its own picture, and what reaches the media plane is the bytes it
// sent under the identity the middleware resolved.
func TestACameraPushesItsOwnFrame(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{}
	frame := oneJPEG()

	rec := pushFrame(t, serverWithCameras(t, fleet).Handler(), frame)
	require.Equal(t, http.StatusNoContent, rec.Code, "body: %s", rec.Body.String())
	require.Empty(t, rec.Body.String(), "a stored frame answers nothing")

	require.Len(t, fleet.pushed, 1)
	require.Equal(t, frame, fleet.pushed[0].Bytes)
	require.Equal(t, "image/jpeg", fleet.pushed[0].MediaType)
	require.False(t, fleet.pushed[0].Taken.IsZero(),
		"the frame carries no time, so every conditional request would refetch it")
	require.Equal(t, []string{"user:operator"}, fleet.principals,
		"the plane was handed a frame from nobody, so no grant decided it")
}

// TestAPushedBodyThatIsNotAnImageIsRefused is the check that is on the bytes
// and not on the header. What this server serves back to a browser from its own
// origin has to be something it looked at.
func TestAPushedBodyThatIsNotAnImageIsRefused(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{}

	rec := pushFrame(t, serverWithCameras(t, fleet).Handler(),
		[]byte("<html><script>this is not a frame</script></html>"))

	body := requireProblem(t, rec, http.StatusUnsupportedMediaType)
	require.Equal(t, httpfault.FrameNotAnImage(httpfault.Reference("camera", "front")).Code(), body.Code)
	require.Empty(t, fleet.pushed, "a body nothing checked reached the media plane")
}

// TestAPushedFrameOverTheCeilingIsRefused holds the bound the operation
// publishes. The body is read into memory before anything looks at it, so the
// ceiling is what stops one camera deciding what a push costs this server.
func TestAPushedFrameOverTheCeilingIsRefused(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{}
	oversized := append(oneJPEG(), make([]byte, frameCeiling)...)

	rec := pushFrame(t, serverWithCameras(t, fleet).Handler(), oversized)

	body := requireProblem(t, rec, http.StatusRequestEntityTooLarge)
	require.Equal(t, httpfault.BodyTooLarge(frameCeiling).Code(), body.Code)
	require.Empty(t, fleet.pushed)
}

// TestAPushToACameraTheCallerCannotSeeIsNotFound keeps the push path's refusals
// the refusals of the rest of this surface: a camera somebody may not see and
// one that is not there are the same bytes.
func TestAPushToACameraTheCallerCannotSeeIsNotFound(t *testing.T) {
	t.Parallel()

	fleet := &stubCameras{err: CameraNotFound("front")}

	rec := pushFrame(t, serverWithCameras(t, fleet).Handler(), oneJPEG())

	body := requireProblem(t, rec, http.StatusNotFound)
	require.Equal(t, httpfault.ResourceNotFound(httpfault.Reference("camera", "front")).Code(), body.Code)
}
