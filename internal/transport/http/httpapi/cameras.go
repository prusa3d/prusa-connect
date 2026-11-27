package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// The camera paths. A camera is addressed by the name it was added under,
// which is its device id and the name the log line and the metric label
// all use.
const (
	camerasPath        = "/api/v1/cameras"
	cameraTypesPath    = "/api/v1/camera-types"
	cameraSnapshotPath = camerasPath + "/{camera}/snapshot"
	cameraStreamPath   = camerasPath + "/{camera}/stream"
	cameraWatchPath    = camerasPath + "/{camera}/webrtc"
	cameraLivePath     = camerasPath + "/{camera}/live"
)

// cameraObject is the resource kind a camera refusal names, which is the kind
// a camera's settings are kept under.
const cameraObject = "camera"

// sdpMediaType is what an offer and an answer are carried as, registered as
// application/sdp by RFC 8866.
const sdpMediaType = "application/sdp"

// offerCeiling bounds the offer body. A browser's offer for one recvonly video
// transceiver is a few kilobytes, and the body is read into memory before
// anything parses it.
const offerCeiling = 64 << 10

// frameCeiling bounds one pushed frame, as camera.accept bounds a module's.
const frameCeiling = cameraapi.MaxAcceptBytes

// jpegMediaType is what a pushed frame is sent as, and the one type this path
// takes. Nothing here converts: a camera that cannot encode JPEG cannot push.
const jpegMediaType = "image/jpeg"

// jpegMagic is the start of every JPEG file: the start-of-image marker and the
// first byte of the marker after it. The body is checked against it because the
// bytes came from a device on somebody's network and are served back to a
// browser from this server's origin. A Content-Type header only says what the
// sender claims.
//
//nolint:gochecknoglobals // a constant byte string, never written to.
var jpegMagic = []byte{0xFF, 0xD8, 0xFF}

// IsJPEG reports whether frame starts as a JPEG does. Every door a pushed frame
// comes through checks it.
func IsJPEG(frame []byte) bool { return bytes.HasPrefix(frame, jpegMagic) }

// snapshotGrace is added to a camera's own cadence when the response says how
// long the frame may be held. A cache expiring exactly on the cadence refetches
// a frame the camera has not replaced yet.
const snapshotGrace = 2 * time.Second

// CameraFleet is the configured cameras as this surface reads them. It is
// declared here because this is where it is consumed, and the composition root
// in internal/app satisfies it.
//
// Every method takes the principal the middleware resolved. A camera is
// authorized by the grant on the printer it watches, so a caller who may not
// read that printer's telemetry is refused here exactly as they are on the
// printer itself, and with the same refusal: not found. A 403 would tell them
// the camera exists.
type CameraFleet interface {
	// Cameras returns every camera the caller may see, sorted by name.
	Cameras(ctx context.Context, principal string) ([]Camera, error)

	// Snapshot returns one camera's most recent still image. Fresh is false
	// when the camera is working and has produced no frame yet. That answers
	// 204 and is not a failure.
	Snapshot(ctx context.Context, principal string, name string) (CameraSnapshot, error)

	// Stream opens one camera's live view. The caller closes Body.
	Stream(ctx context.Context, principal string, name string) (CameraStream, error)

	// Watch answers one browser's SDP offer for a camera's H.264 live view.
	// The relay owns the camera stream from here and ends it when the viewer
	// goes, so there is nothing for this caller to close.
	Watch(ctx context.Context, principal string, name string, offer string) (CameraAnswer, error)

	// Live opens one camera's H.264 live view as access units, for a browser
	// that could not connect over WebRTC. The caller closes it.
	Live(ctx context.Context, principal string, name string) (CameraLive, error)

	// Push stores one frame a camera sent for itself, as that camera's most
	// recent one. It is a write, so it takes more than seeing the camera does.
	// The implementation refuses a caller who may watch a camera and may not
	// decide what it shows.
	Push(ctx context.Context, principal string, name string, frame CameraFrame) error

	// CameraTypes is the types a new camera may be declared as: the core's
	// own, and the ones installed modules offer (ADR-20261106-km2).
	CameraTypes(ctx context.Context, principal string) ([]CameraType, error)
}

// CameraType is one type a new camera may be declared as.
type CameraType struct {
	Type   string `json:"type"             doc:"What a camera of this type is added with, in the type of PUT /api/v1/cameras/{camera}."`
	Title  string `json:"title,omitempty"  doc:"The name a module gave the type. Absent for a type of the core, which a client names in its own language."`
	Module string `json:"module,omitempty" doc:"The namespace of the module that offers the type, which feeds a camera of it and may draw its live view. Absent for a type of the core."`
}

// CameraFrame is one still image a camera sent for itself.
//
// It carries no principal and no camera name. The sender is the identity the
// middleware resolved, and the camera is the one the path names. A device that
// could name either could choose whose camera it writes into.
type CameraFrame struct {
	// Bytes is the encoded image, already bounded and already checked to be a
	// JPEG.
	Bytes []byte
	// MediaType is what those bytes are, which this path fixes at image/jpeg.
	MediaType string
	// Taken is when the frame was produced. The transport stamps it on arrival
	// and does not trust the device's clock. With a camera clock a year ahead,
	// no conditional request would ever refresh the thumbnail.
	Taken time.Time
}

// Camera is one camera on the wire.
//
// It never carries a URL, a credential or a host. A camera's address often
// holds its credentials, so publishing the address would leak them. An
// operator reads the address back from the camera settings, under a resource
// grant, and a viewer never needs it.
type Camera struct {
	Name     string   `json:"name"      doc:"The name the camera was added under, which is its device id."`
	Type     string   `json:"type"      doc:"The type the camera is declared with. A type an installed module offers is <namespace>/<name>, and that module feeds the camera."`
	DeviceID string   `json:"device_id" doc:"The device this camera watches, or empty for a camera that watches none. A grant on that device reaches the camera, and a grant written on the camera itself reaches it too."`
	Formats  []string `json:"formats"   doc:"What this camera can emit, in the driver's order of preference. Nothing is converted: a format that is not here is unsupported."`
	Serving  bool     `json:"serving"   doc:"Whether the camera is ready right now. A camera that is not serving is listed and answers no frame."`

	// MinIntervalMS is the driver's own cadence, so a page refreshes a still
	// image at the rate the camera can answer it. A duration on the wire is
	// milliseconds under a key ending _ms.
	MinIntervalMS int64 `json:"min_interval_ms,omitempty" doc:"The shortest gap between two new frames, in milliseconds, as the driver declares it. Ask for a still image no faster than this: a request inside the gap is answered with the frame the camera already holds. Absent for a camera that declares no cadence, and a client then keeps its own wait."`

	// Snapshot, Stream and Watch say which of this camera's paths will answer,
	// derived from the declared formats, so a client builds a grid without
	// requesting a path that will refuse.
	Snapshot bool `json:"snapshot" doc:"Whether this camera answers a still image."`
	Stream   bool `json:"stream"   doc:"Whether this camera answers a multipart live view."`
	Watch    bool `json:"watch"    doc:"Whether this camera answers a WebRTC live view, which is what an H.264 camera serves."`
}

// CameraAnswer is the server's half of one WebRTC negotiation.
type CameraAnswer struct {
	// SDP is the answer, with every ICE candidate already in it. The exchange
	// is one request and one response, so there is no second channel a
	// trickled candidate could arrive on.
	SDP string
}

// CameraLive is one camera's H.264 stream read as whole pictures.
type CameraLive interface {
	// Next returns the next access unit, Annex-B framed, and whether it is a
	// keyframe carrying the parameter sets. The first one is a keyframe. It
	// reports io.EOF when the camera stream ends.
	Next() (unit []byte, key bool, err error)
	// Close ends the camera stream.
	Close() error
}

// CameraSnapshot is one still image and what a cache may do with it.
type CameraSnapshot struct {
	// Bytes is the encoded image. Empty means the camera has produced no frame
	// yet, which is answered with 204.
	Bytes []byte
	// MediaType is what those bytes are. The host has already refused anything
	// that is not a still image, so this is never a type a browser would run.
	MediaType string
	// Taken is when the device produced the frame, and the validator a
	// conditional GET compares against.
	Taken time.Time
	// MaxAge is how long this frame may be held before it is worth asking
	// again, which is the camera's own cadence.
	MaxAge time.Duration
}

// CameraStream is one viewer's live view.
type CameraStream struct {
	// Body is the framed bytes. The transport closes it when the viewer goes.
	Body io.ReadCloser
	// MediaType is what those bytes are framed as.
	MediaType string
}

// listCamerasInput narrows the listing to one printer's cameras.
//
// It is a filter and never a permission: what the caller may see is decided by
// the graph before this is read, so naming a printer they hold nothing on
// answers an empty list exactly as naming one that has no camera does.
//
// The filter is here and the device record carries no cameras. A printer's
// page asks for its cameras with one more request. The fleet listing then
// needs no camera lookup, so a page of four hundred printers does not cost
// four hundred of them.
type listCamerasInput struct {
	Device string `query:"device" doc:"Narrow the listing to the cameras watching one device. A device the caller cannot see, and one with no camera, both answer an empty list." maxLength:"63" example:"9f2a4c18-6d3b-4e07-b5a1-8c0f2d7e4b93"`
}

type listCamerasOutput struct {
	Body struct {
		Items []Camera `json:"items"`
	}
}

// CameraRef is the path of one camera. It is exported because Huma binds
// parameters by reflection and cannot write into an unexported embedded field.
type CameraRef struct {
	Camera string `path:"camera" doc:"The camera's name." maxLength:"63" example:"bench-top"`
}

type snapshotInput struct {
	CameraRef

	IfModifiedSince string `header:"If-Modified-Since" doc:"The Last-Modified of the frame this client already has. A frame no newer than it answers 304 with no body, which is what makes a grid of four hundred thumbnails cheap."`
}

// registerCameras publishes the media surface: what cameras the caller may see,
// one still image each, and one live view each.
//
// A camera that is declared and not serving is still listed. If it were
// hidden, a grid would lose a cell when a camera goes away, and an operator
// would have to read the camera settings to find out why.
func registerCameras(api huma.API, cfg Config, closing <-chan struct{}) {
	fleet := cfg.Cameras
	if fleet == nil {
		return
	}

	registerListCameras(api, fleet)
	registerCameraSnapshot(api, fleet)
	registerPutCameraSnapshot(api, fleet)
	registerCameraStream(api, fleet, cfg.Logger, closing)
	registerCameraWatch(api, fleet)
	registerCameraLive(api, fleet, cfg.Logger, closing)
	registerCameraTypes(api, fleet)
}

type listCameraTypesOutput struct {
	Body struct {
		Items []CameraType `json:"items" doc:"Every type a new camera may be declared as, the core's generic camera first."`
	}
}

func registerCameraTypes(api huma.API, fleet CameraFleet) {
	register(api, huma.Operation{
		OperationID: "list-camera-types",
		Method:      http.MethodGet,
		Path:        cameraTypesPath,
		Summary:     "The types a new camera may be declared as",
		Description: "The core's generic camera, and every camera type an installed module offers. A camera " +
			"of a module's type runs on the core's generic driver, and the module hands it its pictures. " +
			"Add one with PUT /api/v1/cameras/{camera}.",
		Tags:   []string{"cameras"},
		Errors: []int{http.StatusUnauthorized},
	}, func(ctx context.Context, _ *struct{}) (*listCameraTypesOutput, error) {
		listed, err := fleet.CameraTypes(ctx, principalFrom(ctx))
		if err != nil {
			return nil, err
		}

		out := &listCameraTypesOutput{}
		out.Body.Items = listed

		if out.Body.Items == nil {
			out.Body.Items = []CameraType{}
		}

		return out, nil
	})
}

// pushInput is one frame a camera sent. The body is taken raw because it is an
// image, and nothing here reads inside it beyond the first three bytes.
type pushInput struct {
	CameraRef

	RawBody []byte
}

type pushOutput struct {
	Status int
}

// registerPutCameraSnapshot publishes the push path.
//
// This server reads from every other camera. A camera behind a router, on a
// mobile network, or on a segment this server has no route to cannot be read,
// so it sends its own frames here.
//
// The frame is stored the way a fetched one is: the driver holds it as its most
// recent, and GET on this path and the stream path then serve it. There is no
// second store and no queue, because only the newest still image is worth
// keeping.
func registerPutCameraSnapshot(api huma.API, fleet CameraFleet) {
	register(api, huma.Operation{
		OperationID: "put-camera-snapshot",
		Method:      http.MethodPut,
		Path:        cameraSnapshotPath,
		Summary:     "Send one frame from a camera that cannot be read",
		Description: "Takes one JPEG as image/jpeg, at most 8 MiB, and stores it as this camera's most " +
			"recent frame. It is for a camera this server cannot dial: the device sends its own " +
			"picture, and the snapshot and stream paths then serve it exactly as they serve a frame " +
			"the server fetched. Nothing is decoded or converted, so a body that is not a JPEG is " +
			"refused rather than re-encoded.\n\n" +
			"It takes operate on the camera, which a viewer does not hold: whoever may watch a camera " +
			"must not be able to decide what it shows. A camera the caller may not see, one that is " +
			"not declared, and one whose driver reads a device rather than taking frames all answer " +
			"404, which is the same refusal the rest of this surface gives.\n\n" +
			"The answer is 204 and carries no body. When the frame was taken is this server's own " +
			"clock and not the device's, because a camera whose clock is a year out would be a " +
			"thumbnail no conditional request ever refreshes.",
		Tags: []string{"cameras"},
		Errors: []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusNotFound,
			http.StatusRequestEntityTooLarge,
			http.StatusUnsupportedMediaType,
		},
		RequestBody:   frameBody(),
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *pushInput) (*pushOutput, error) {
		if len(in.RawBody) > frameCeiling {
			return nil, httpfault.BodyTooLarge(frameCeiling)
		}

		if !IsJPEG(in.RawBody) {
			return nil, httpfault.FrameNotAnImage(httpfault.Reference(cameraObject, in.Camera))
		}

		frame := CameraFrame{
			Bytes:     in.RawBody,
			MediaType: jpegMediaType,
			Taken:     time.Now().UTC(),
		}

		if err := fleet.Push(ctx, principalFrom(ctx), in.Camera, frame); err != nil {
			return nil, err
		}

		return &pushOutput{Status: http.StatusNoContent}, nil
	})
}

// frameBody documents what this operation takes. Huma derives a request body
// from the input struct, and a raw body has no schema to derive, so without this
// the document would publish an operation that takes nothing.
func frameBody() *huma.RequestBody {
	return &huma.RequestBody{
		Description: "One JPEG, as the camera encoded it.",
		Required:    true,
		Content: map[string]*huma.MediaType{
			jpegMediaType: {Schema: &huma.Schema{Type: "string", Format: "binary"}},
		},
	}
}

// watchInput is one browser's offer. The body is taken raw because an SDP is a
// text document with its own grammar and nothing here reads inside it.
type watchInput struct {
	CameraRef

	RawBody []byte
}

// registerCameraWatch publishes the WebRTC live view.
//
// One request carries the offer and the response carries the answer, with the
// candidates already gathered into it. The exchange has the shape of WHEP but
// is not WHEP: there is no session resource to delete afterwards. The view
// ends when the browser closes its peer connection, which the server sees as an
// ICE state change.
func registerCameraWatch(api huma.API, fleet CameraFleet) {
	register(api, huma.Operation{
		OperationID: "watch-camera",
		Method:      http.MethodPost,
		Path:        cameraWatchPath,
		Summary:     "One camera's live view over WebRTC",
		Description: "Takes an SDP offer as application/sdp and answers with an SDP answer that " +
			"already carries every ICE candidate, so one request completes the negotiation. It is " +
			"how an H.264 camera is watched in a browser: the elementary stream is repacketised into " +
			"RTP and nothing is decoded or re-encoded. The server offers no ICE server of its own, " +
			"because host candidates are enough on the network the camera is on. A camera that emits " +
			"no H.264 answers 404, the same refusal as one the caller may not see.",
		Tags: []string{"cameras"},
		Errors: []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusNotFound,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
		},
		RequestBody:   offerBody(),
		Responses:     watchResponses(),
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *watchInput) (*huma.StreamResponse, error) {
		if len(in.RawBody) > offerCeiling {
			return nil, httpfault.BodyTooLarge(offerCeiling)
		}

		answered, err := fleet.Watch(ctx, principalFrom(ctx), in.Camera, string(in.RawBody))
		if err != nil {
			return nil, err
		}

		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			hctx.SetHeader("Content-Type", sdpMediaType)
			hctx.SetHeader("Cache-Control", "no-store")
			hctx.SetHeader("Content-Length", strconv.Itoa(len(answered.SDP)))
			hctx.SetStatus(http.StatusCreated)

			_, _ = hctx.BodyWriter().Write([]byte(answered.SDP))
		}}, nil
	})
}

// offerBody documents what this operation takes. Huma derives a request body
// from the input struct, and a raw body has no schema to derive, so without this
// the document would publish an operation that takes nothing.
func offerBody() *huma.RequestBody {
	return &huma.RequestBody{
		Description: "The browser's SDP offer.",
		Required:    true,
		Content: map[string]*huma.MediaType{
			sdpMediaType: {Schema: &huma.Schema{Type: "string"}},
		},
	}
}

// watchResponses documents the answer, for the same reason.
func watchResponses() map[string]*huma.Response {
	return map[string]*huma.Response{
		strconv.Itoa(http.StatusCreated): {
			Description: "The SDP answer, with the ICE candidates already gathered into it.",
			Content: map[string]*huma.MediaType{
				sdpMediaType: {Schema: &huma.Schema{Type: "string"}},
			},
		},
	}
}

func registerListCameras(api huma.API, fleet CameraFleet) {
	register(api, huma.Operation{
		OperationID: "list-cameras",
		Method:      http.MethodGet,
		Path:        camerasPath,
		Summary:     "Cameras this caller may see",
		Description: "Every declared camera whose printer the caller may read telemetry from. A camera " +
			"is under the same per-printer grant as the printer it watches, so a caller who cannot see " +
			"a printer does not learn that it has a camera. Pass device to read one printer's cameras, " +
			"which is what a printer's page renders. When what this answers for one device changes, " +
			"the event stream publishes com.prusa.connect.device.cameras.changed on that device, so a " +
			"client reads this again then rather than on a timer.",
		Tags:   []string{"cameras"},
		Errors: []int{http.StatusUnauthorized},
	}, func(ctx context.Context, in *listCamerasInput) (*listCamerasOutput, error) {
		listed, err := fleet.Cameras(ctx, principalFrom(ctx))
		if err != nil {
			return nil, err
		}

		out := &listCamerasOutput{}
		out.Body.Items = camerasOf(listed, in.Device)

		if out.Body.Items == nil {
			out.Body.Items = []Camera{}
		}

		return out, nil
	})
}

// camerasOf narrows a listing to one device, and returns it whole when no
// device was named. The filter runs over rows the graph already admitted, so it
// can never widen an answer.
func camerasOf(listed []Camera, device string) []Camera {
	if device == "" {
		return listed
	}

	out := make([]Camera, 0, len(listed))

	for _, camera := range listed {
		if camera.DeviceID == device {
			out = append(out, camera)
		}
	}

	return out
}

// registerCameraSnapshot publishes the still image. It is a raw response
// because the body is an image and not a document. It carries the
// conditional-GET headers, so a thumbnail grid does not transfer every image
// on every refresh.
func registerCameraSnapshot(api huma.API, fleet CameraFleet) {
	register(api, huma.Operation{
		OperationID: "get-camera-snapshot",
		Method:      http.MethodGet,
		Path:        cameraSnapshotPath,
		Summary:     "One camera's most recent still image",
		Description: "Answers image/jpeg, with Last-Modified and a private max-age of the camera's own " +
			"cadence. A client that sends If-Modified-Since is answered 304 with no body while its copy " +
			"is current, which is how a grid of four hundred printers costs four hundred empty " +
			"responses rather than four hundred images. A camera that is working and has produced no " +
			"frame yet answers 204. Cache-Control is private, so a shared proxy never holds one " +
			"caller's frame and serves it to another. A camera this server could not read answers " +
			"502, and a retry may work: the request is correct and the device did not answer.",
		Tags:      []string{"cameras"},
		Errors:    []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusBadGateway},
		Responses: snapshotResponses(),
	}, func(ctx context.Context, in *snapshotInput) (*huma.StreamResponse, error) {
		shot, err := fleet.Snapshot(ctx, principalFrom(ctx), in.Camera)
		if err != nil {
			return nil, err
		}

		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			writeSnapshot(hctx, shot, in.IfModifiedSince)
		}}, nil
	})
}

// registerCameraStream publishes the live view. The bytes are relayed as the
// driver framed them and nothing here reads inside a frame.
func registerCameraStream(api huma.API, fleet CameraFleet, log *slog.Logger, closing <-chan struct{}) {
	register(api, huma.Operation{
		OperationID: "stream-camera",
		Method:      http.MethodGet,
		Path:        cameraStreamPath,
		Summary:     "One camera's live view",
		Description: "Relays the camera's own frames as multipart/x-mixed-replace, which an img tag " +
			"plays directly. Nothing is decoded or re-encoded. The response carries " +
			"X-Accel-Buffering: no, because a reverse proxy that buffers by default holds an endless " +
			"body and delivers nothing. A camera that emits no streamable format answers 404, the same " +
			"refusal as one the caller may not see. A camera this server could not open answers " +
			"502, and a retry may work: the request is correct and the device did not answer.",
		Tags: []string{"cameras"},
		Errors: []int{
			http.StatusUnauthorized,
			http.StatusNotFound,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
		},
		Responses: streamResponses(),
		Metadata:  streams(),
	}, func(ctx context.Context, in *snapshotInput) (*huma.StreamResponse, error) {
		view, err := fleet.Stream(ctx, principalFrom(ctx), in.Camera)
		if err != nil {
			return nil, err
		}

		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			relayFrames(hctx, view, log, closing)
		}}, nil
	})
}

// writeSnapshot renders one still image, or the reason there is nothing to
// render.
//
// nosniff is required. The bytes came from a device on somebody's network. A
// browser that guessed the type could run what a compromised camera answered
// with, on this server's origin. The host has already refused a media type
// that is not a still image, and this header covers the browser side.
func writeSnapshot(hctx huma.Context, shot CameraSnapshot, ifModifiedSince string) {
	hctx.SetHeader("X-Content-Type-Options", "nosniff")
	// A browser may show a kept frame for the max-age, the camera's own
	// cadence, without asking the server. That is accepted. A frame is the
	// same image for everyone who may see this camera, and private keeps a
	// shared proxy from holding it.
	hctx.SetHeader("Cache-Control", "private, max-age="+strconv.Itoa(int(shot.MaxAge.Seconds())))

	if len(shot.Bytes) == 0 {
		hctx.SetStatus(http.StatusNoContent)

		return
	}

	stamp := shot.Taken.UTC().Truncate(time.Second)

	hctx.SetHeader("Last-Modified", stamp.Format(http.TimeFormat))
	hctx.SetHeader("Expires", stamp.Add(shot.MaxAge+snapshotGrace).Format(http.TimeFormat))

	if frameIsCurrent(stamp, ifModifiedSince) {
		hctx.SetStatus(http.StatusNotModified)

		return
	}

	hctx.SetHeader("Content-Type", shot.MediaType)
	hctx.SetHeader("Content-Length", strconv.Itoa(len(shot.Bytes)))
	hctx.SetStatus(http.StatusOK)

	_, _ = hctx.BodyWriter().Write(shot.Bytes)
}

// relayFrames copies the camera's frames to the viewer and closes the stream
// when the viewer stops reading.
//
// The copy is unbuffered. The driver hands over one whole frame at a time, so
// a buffer here would add a frame of latency and a second place for frames to
// queue behind a slow viewer. A viewer that stops reading stalls this copy.
// The supervised stream's own timeout then frees the bulkhead slot.
//
// The server's whole-response write timeout is lifted and a deadline is set
// per frame, the way the event stream does it. A live view is a response that
// never ends, so one deadline over the whole of it would cut off every viewer.
// A deadline per frame ends only the viewer that has stopped reading.
func relayFrames(hctx huma.Context, view CameraStream, log *slog.Logger, closing <-chan struct{}) {
	defer func() { _ = view.Body.Close() }()

	_, raw := humachi.Unwrap(hctx)
	control := http.NewResponseController(raw)

	if err := control.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		log.Warn("a camera live view keeps the server's write timeout", "error", err.Error())
	}

	hctx.SetHeader("Content-Type", view.MediaType)
	hctx.SetHeader("Cache-Control", "no-store")
	hctx.SetHeader("X-Content-Type-Options", "nosniff")

	// nginx defaults to proxy_buffering on, which holds an endless body and
	// delivers nothing. With this header the operator does not have to edit
	// their proxy.
	hctx.SetHeader("X-Accel-Buffering", "no")
	hctx.SetStatus(http.StatusOK)

	writer := hctx.BodyWriter()

	flusher, flushable := writer.(http.Flusher)
	if flushable {
		flusher.Flush()
	}

	buf := make([]byte, relayChunk)

	for {
		n, err := view.Body.Read(buf)
		if n > 0 {
			// The deadline is set before the write and never before the read.
			// The wait between two frames is the camera's, and it must not
			// count against the viewer.
			if deadline := control.SetWriteDeadline(time.Now().Add(writeTimeout)); deadline != nil &&
				!errors.Is(deadline, http.ErrNotSupported) {
				return
			}

			if _, writeErr := writer.Write(buf[:n]); writeErr != nil {
				return
			}

			if flushable {
				flusher.Flush()
			}
		}

		if err != nil {
			return
		}

		select {
		case <-hctx.Context().Done():
			return
		// A draining listener waits on every response in flight, and a live
		// view never ends on its own. Without this case a shutdown would wait
		// out its whole budget.
		case <-closing:
			return
		default:
		}
	}
}

// relayChunk is the copy buffer. One buffer for the life of the connection,
// because a fresh slice per read is an allocation per frame per viewer.
const relayChunk = 32 << 10

// frameIsCurrent reports whether the client copy is current. A malformed header
// is treated as no header, as RFC 9110 section 13.1.3 requires: the server
// ignores a date it cannot parse and does not refuse the request.
func frameIsCurrent(taken time.Time, ifModifiedSince string) bool {
	if ifModifiedSince == "" {
		return false
	}

	since, err := http.ParseTime(ifModifiedSince)
	if err != nil {
		return false
	}

	return !taken.After(since.UTC())
}

// snapshotResponses documents what this operation actually answers. Huma
// derives responses from the returned struct, and a raw body has none to
// derive, so the document would otherwise publish an operation that returns
// nothing.
func snapshotResponses() map[string]*huma.Response {
	return map[string]*huma.Response{
		strconv.Itoa(http.StatusOK): {
			Description: "The most recent frame.",
			Content: map[string]*huma.MediaType{
				"image/jpeg": {Schema: &huma.Schema{Type: "string", Format: "binary"}},
			},
			Headers: map[string]*huma.Param{
				"Last-Modified": {
					Description: "When the device produced this frame. Send it back in If-Modified-Since.",
				},
				"Cache-Control": {Description: "private, and the camera's own cadence as max-age."},
				"Expires":       {Description: "When this frame stops being worth holding."},
			},
		},
		strconv.Itoa(http.StatusNoContent): {
			Description: "The camera is working and has produced no frame yet.",
		},
		strconv.Itoa(http.StatusNotModified): {
			Description: "The client's copy is still current. No body is sent.",
		},
	}
}

// streamResponses documents the live view, for the same reason.
func streamResponses() map[string]*huma.Response {
	return map[string]*huma.Response{
		strconv.Itoa(http.StatusOK): {
			Description: "An endless multipart body, one JPEG per part.",
			Content: map[string]*huma.MediaType{
				"multipart/x-mixed-replace": {Schema: &huma.Schema{Type: "string", Format: "binary"}},
			},
		},
	}
}

// CameraNotFound is the one refusal this surface has for a camera it will not
// serve. A camera the caller may not see, one that is not declared, and one
// that emits no format the caller asked for are the same answer, because
// telling a caller which of the three it was tells them a camera exists.
func CameraNotFound(name string) error {
	return httpfault.ResourceNotFound(httpfault.Reference(cameraObject, name))
}

// CameraUnreachable is a camera the caller may see and this server could not
// read. It is not the 404 the refusals above give. The caller already knows
// this camera is there, and without the reason they would read a 500 and
// blame this server instead of the device.
func CameraUnreachable(name string) error {
	return httpfault.CameraUnreachable(httpfault.Reference(cameraObject, name))
}

// CameraFrameRefused is a frame this server will not store: bytes no camera
// could serve back. It is exported because the media plane raises it, and the
// driver is the last thing that can tell.
func CameraFrameRefused(name string) error {
	return httpfault.FrameNotAnImage(httpfault.Reference(cameraObject, name))
}

// CameraPushDenied is the refusal for a caller who may watch a camera and may
// not decide what it shows. It is 403 and not the 404 the rest of this
// surface gives, because the camera is already in that caller's listing.
func CameraPushDenied(name string) error {
	return fault.New(authz.CodeAccessDenied(),
		"this caller may watch the camera and may not send it a frame. Deciding what a camera "+
			"shows takes operate on it, which a viewer does not hold").With(
		fault.StringParam("object", "device:"+name),
		fault.StringParam("action", authz.ActionOperate.String()))
}
