package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// liveSettle bounds how long a case waits on the socket.
const liveSettle = 5 * time.Second

// fedLive is a camera whose pictures the case hands over one at a time. Next
// returns each one when it is sent, and io.EOF once the feed is closed or the
// view is.
type fedLive struct {
	feed   chan livePicture
	shut   chan struct{}
	closed atomic.Bool
}

type livePicture struct {
	unit []byte
	key  bool
}

func newFedLive() *fedLive {
	return &fedLive{feed: make(chan livePicture), shut: make(chan struct{})}
}

func (f *fedLive) Next() ([]byte, bool, error) {
	select {
	case picture, open := <-f.feed:
		if !open {
			return nil, false, io.EOF
		}

		return picture.unit, picture.key, nil
	case <-f.shut:
		return nil, false, io.EOF
	}
}

func (f *fedLive) Close() error {
	if f.closed.CompareAndSwap(false, true) {
		close(f.shut)
	}

	return nil
}

func (f *fedLive) send(t *testing.T, unit string, key bool) {
	t.Helper()

	select {
	case f.feed <- livePicture{unit: []byte(unit), key: key}:
	case <-time.After(liveSettle):
		t.Fatalf("the relay never asked the camera for %q", unit)
	}
}

func liveServer(t *testing.T, fleet *stubCameras) string {
	t.Helper()

	listener := httptest.NewServer(serverWithCameras(t, fleet).Handler())
	t.Cleanup(listener.Close)

	return "ws" + strings.TrimPrefix(listener.URL, "http") + camerasPath + "/front/live"
}

func dialLive(t *testing.T, url string, header http.Header) *websocket.Conn {
	t.Helper()

	conn, answer, err := websocket.DefaultDialer.Dial(url, header)
	if answer != nil {
		_ = answer.Body.Close()
	}

	require.NoError(t, err, "the live socket did not open")

	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func readPicture(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(liveSettle)))

	kind, message, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, kind, "a picture arrived as text")

	return message
}

// The camera's pictures reach the browser one per message, each behind the
// flag that says whether it is a keyframe, and the camera is given back when
// the browser goes.
func TestTheLiveSocketCarriesOnePicturePerMessage(t *testing.T) {
	t.Parallel()

	camera := newFedLive()
	conn := dialLive(t, liveServer(t, &stubCameras{live: camera}), nil)

	camera.send(t, "key-picture", true)
	camera.send(t, "next-picture", false)

	require.Equal(t, append([]byte{liveKey}, "key-picture"...), readPicture(t, conn))
	require.Equal(t, append([]byte{liveDelta}, "next-picture"...), readPicture(t, conn))

	require.NoError(t, conn.Close())

	require.Eventually(t, camera.closed.Load, liveSettle, 10*time.Millisecond,
		"the browser went away and the camera stream is still open")
}

// A camera stream that ends closes the socket normally, so the browser can
// tell a camera that stopped from a network that dropped.
func TestTheLiveSocketClosesNormallyWhenTheCameraStops(t *testing.T) {
	t.Parallel()

	camera := newFedLive()
	conn := dialLive(t, liveServer(t, &stubCameras{live: camera}), nil)

	camera.send(t, "key-picture", true)
	close(camera.feed)

	readPicture(t, conn)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(liveSettle)))

	_, _, err := conn.ReadMessage()

	var closed *websocket.CloseError
	require.ErrorAs(t, err, &closed)
	require.Equal(t, websocket.CloseNormalClosure, closed.Code)
}

// A refusal is an HTTP answer, given before any socket exists, so a camera the
// caller may not see is the same 404 here as on every other media path.
func TestALiveSocketToACameraTheCallerMayNotSeeIsAPlain404(t *testing.T) {
	t.Parallel()

	url := liveServer(t, &stubCameras{err: CameraNotFound("front")})

	_, answer, err := websocket.DefaultDialer.Dial(url, nil)
	require.ErrorIs(t, err, websocket.ErrBadHandshake)
	require.NotNil(t, answer)

	defer func() { _ = answer.Body.Close() }()

	require.Equal(t, http.StatusNotFound, answer.StatusCode)
}

// A page on another site must not open the socket with the viewer's cookie and
// read the camera. The upgrade is refused and the camera given back.
func TestALiveSocketFromAnotherSiteIsRefused(t *testing.T) {
	t.Parallel()

	camera := newFedLive()
	url := liveServer(t, &stubCameras{live: camera})

	_, answer, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {"https://elsewhere.example"}})
	require.ErrorIs(t, err, websocket.ErrBadHandshake)
	require.NotNil(t, answer)

	defer func() { _ = answer.Body.Close() }()

	require.Equal(t, http.StatusForbidden, answer.StatusCode)
	require.Eventually(t, camera.closed.Load, liveSettle, 10*time.Millisecond,
		"the upgrade was refused and the camera stream is still open")
}

// A viewer that falls behind skips to the next keyframe. The pictures between
// would be the past, and a decoder handed one whose reference was dropped
// draws garbage.
func TestASlowViewerSkipsToTheNextKeyframe(t *testing.T) {
	t.Parallel()

	camera := newFedLive()
	pictures := readPictures(camera, slog.New(slog.DiscardHandler))

	camera.send(t, "key-0", true)

	// The queue fills, and the picture after it is dropped.
	for range liveQueue {
		camera.send(t, "delta", false)
	}

	// Skipped whatever the queue holds, because the picture it follows was
	// dropped.
	camera.send(t, "delta-after-the-drop", false)

	for range liveQueue {
		<-pictures
	}

	camera.send(t, "key-1", true)
	camera.send(t, "delta-1", false)
	close(camera.feed)

	var rest []string
	for picture := range pictures {
		rest = append(rest, string(picture))
	}

	require.Equal(t, []string{"\x01key-1", "\x00delta-1"}, rest,
		"the viewer was handed a picture whose reference it never got")
}

// TestTheLiveSocketRefusalsAreTheFleets holds that nothing here invents a
// refusal of its own for a camera that did not open.
func TestTheLiveSocketRefusalsAreTheFleets(t *testing.T) {
	t.Parallel()

	url := liveServer(t, &stubCameras{err: CameraUnreachable("front")})

	_, answer, err := websocket.DefaultDialer.Dial(url, nil)
	require.ErrorIs(t, err, websocket.ErrBadHandshake)
	require.NotNil(t, answer)

	defer func() { _ = answer.Body.Close() }()

	require.Equal(t, http.StatusBadGateway, answer.StatusCode)
}
