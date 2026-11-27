package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/gorilla/websocket"
)

// The live socket's framing. Every message is binary: one flag byte, then one
// access unit, Annex-B framed. The browser decodes it with WebCodecs, which
// takes that framing as it is, so nothing here builds a container.
const (
	liveDelta byte = 0
	liveKey   byte = 1
)

const (
	// liveQueue is how many pictures may wait for a slow viewer, about a
	// second at the frame rates cameras run at. A viewer further behind
	// skips to the next keyframe rather than watching the past.
	liveQueue = 32

	// liveReadCeiling bounds what a viewer may send. A viewer sends nothing
	// but the close of the socket, so a message is refused well before it
	// could cost anything.
	liveReadCeiling = 512

	// liveWriteBuffer is the socket's write buffer. A message larger than it
	// is written in several frames, so it bounds memory and not pictures.
	liveWriteBuffer = 32 << 10

	// liveCloseWait bounds the close message the server sends on the way out.
	liveCloseWait = time.Second
)

// liveUpgrader turns a request into the live socket.
//
// Its CheckOrigin is the library's default, which refuses an Origin whose host
// is not the request's. The socket is authorized by the session cookie, and a
// page on another site must not open one with the viewer's cookie and read the
// camera.
var liveUpgrader = websocket.Upgrader{
	ReadBufferSize:  liveReadCeiling,
	WriteBufferSize: liveWriteBuffer,
}

// registerCameraLive publishes the live view a browser falls back to when it
// cannot reach this server over WebRTC.
//
// The camera is opened before the socket is upgraded. A camera the caller may
// not see, one that emits no H.264 and one that does not answer are then the
// same HTTP refusals as on the WebRTC path, and a browser reads them before
// any socket exists.
func registerCameraLive(api huma.API, fleet CameraFleet, log *slog.Logger, closing <-chan struct{}) {
	register(api, huma.Operation{
		OperationID: "live-camera",
		Method:      http.MethodGet,
		Path:        cameraLivePath,
		Summary:     "One camera's live view over a websocket",
		Description: "Upgrades to a websocket that carries the camera's H.264 stream, for a browser " +
			"that cannot reach this server over WebRTC: behind a tunnel that carries no UDP, or a " +
			"container with no published UDP port. Each binary message is one picture: a flag byte, " +
			"1 for a keyframe and 0 otherwise, then the access unit in Annex-B framing. The first " +
			"message is a keyframe, and every keyframe carries the parameter sets. A viewer that " +
			"falls about a second behind skips to the next keyframe. Nothing is decoded or " +
			"re-encoded. A camera that emits no H.264 answers 404, the same refusal as one the " +
			"caller may not see.",
		Tags: []string{"cameras"},
		Errors: []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusNotFound,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
		},
		Responses:     liveResponses(),
		DefaultStatus: http.StatusSwitchingProtocols,
		Metadata:      streams(),
	}, func(ctx context.Context, in *liveInput) (*huma.StreamResponse, error) {
		live, err := fleet.Live(ctx, principalFrom(ctx), in.Camera)
		if err != nil {
			return nil, err
		}

		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			relayUnits(hctx, live, log, closing)
		}}, nil
	})
}

// liveInput is the camera the socket plays. The upgrade's own headers are read
// by the upgrader, not bound here.
type liveInput struct {
	CameraRef
}

// liveResponses documents the upgrade, which carries no body a schema could
// describe.
func liveResponses() map[string]*huma.Response {
	return map[string]*huma.Response{
		strconv.Itoa(http.StatusSwitchingProtocols): {
			Description: "The websocket, carrying one picture per binary message.",
		},
	}
}

// relayUnits upgrades the request and writes the camera's pictures to it until
// the viewer goes, the camera stops or the server shuts down.
//
// The camera is read on a goroutine of its own and handed over through a short
// queue. A viewer that stops reading would otherwise stall the camera stream,
// and the frames it got after it caught up would be seconds old. When the
// queue is full, pictures are dropped until the next keyframe, because a
// decoder handed a picture whose reference was dropped draws garbage.
func relayUnits(hctx huma.Context, live CameraLive, log *slog.Logger, closing <-chan struct{}) {
	defer func() { _ = live.Close() }()

	request, raw := humachi.Unwrap(hctx)

	conn, err := liveUpgrader.Upgrade(raw, request, nil)
	if err != nil {
		// The upgrader has already answered the request with the reason.
		return
	}

	defer func() { _ = conn.Close() }()

	gone := watchViewer(conn)
	pictures := readPictures(live, log)

	for {
		select {
		case picture, open := <-pictures:
			if !open {
				sayGoodbye(conn, websocket.CloseNormalClosure, "the camera stream ended")

				return
			}

			if conn.SetWriteDeadline(time.Now().Add(writeTimeout)) != nil {
				return
			}

			if conn.WriteMessage(websocket.BinaryMessage, picture) != nil {
				return
			}
		case <-gone:
			return
		// A draining listener waits on every response in flight, and a live
		// view never ends on its own.
		case <-closing:
			sayGoodbye(conn, websocket.CloseGoingAway, "the server is shutting down")

			return
		}
	}
}

// watchViewer reads the socket so its close is seen, and reports when the
// viewer has gone. The library answers pings and closes only while somebody
// reads.
func watchViewer(conn *websocket.Conn) <-chan struct{} {
	conn.SetReadLimit(liveReadCeiling)

	gone := make(chan struct{})

	go func() {
		defer close(gone)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	return gone
}

// readPictures reads the camera into a queue the socket drains, and closes the
// queue when the camera stream ends. It never blocks on the queue: closing the
// camera stream, which relayUnits does on the way out, is what ends it.
func readPictures(live CameraLive, log *slog.Logger) <-chan []byte {
	pictures := make(chan []byte, liveQueue)

	go func() {
		defer close(pictures)

		skipping := false

		for {
			unit, key, err := live.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
					log.Debug("a camera live socket ended on a stream failure", "error", err.Error())
				}

				return
			}

			if skipping && !key {
				continue
			}

			skipping = false

			flag := liveDelta
			if key {
				flag = liveKey
			}

			message := make([]byte, 0, len(unit)+1)
			message = append(message, flag)
			message = append(message, unit...)

			select {
			case pictures <- message:
			default:
				skipping = true
			}
		}
	}()

	return pictures
}

// sayGoodbye tells the viewer why the socket ends, so a browser can tell a
// camera that stopped from a network that dropped.
func sayGoodbye(conn *websocket.Conn, code int, reason string) {
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason), time.Now().Add(liveCloseWait))
}
