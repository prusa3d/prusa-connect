package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resourcehost"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/extension/supervised"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/internal/transport/stream"
	"github.com/prusa3d/prusa-connect/internal/transport/webrtc"
	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
	"github.com/prusa3d/prusa-connect/sdk/event"
)

// snapshotMaxAge is how long a still image may be held before it is worth
// asking the camera again. It is the fastest cadence any camera this product
// meets runs at, so it never tells a client to hold a frame past the point the
// device has replaced it.
const snapshotMaxAge = 10 * time.Second

// cameraGraph is the authorization graph as the media plane reads it: one query
// for every device this principal may watch, cameras included.
//
// It is an interface so that a test can observe when the graph is asked. The
// media plane must ask the graph before it touches the supervision plane, and
// from outside only the moment of the graph call shows that order.
type cameraGraph interface {
	Visible(ctx context.Context, subject authz.Subject) (authz.Visibility, error)

	// Can reports whether this subject holds one action on one device. It is
	// the point check every other surface asks, and the media plane asks it for
	// one thing only: a frame arriving on the push path is a write, and reading
	// a camera must not be permission to decide what it shows.
	Can(ctx context.Context, subject authz.Subject, action authz.Action, object authz.Object) (bool, error)

	// BlockedDevices is every device this subject is blocked on. It is read
	// beside the visible set because a block on a printer has to reach the
	// cameras bound to it: a camera is placed in a tenant and carries grants
	// copied from its printer, so without this a person kept away from a
	// machine would go on watching that machine's camera through a path that
	// does not pass through it.
	BlockedDevices(ctx context.Context, subject authz.Subject) (map[string]bool, error)
}

// cameraRelay is the WebRTC engine as the media plane uses it. It is declared
// here because this is where it is consumed, and internal/transport/webrtc
// satisfies it.
//
// The relay is handed a way to open the camera and not an open stream. It
// opens the camera once the browser connects and closes it when the viewer
// goes, whichever way the viewer goes. One close releases the bulkhead slot
// and the device socket, and only the relay knows when the viewer has stopped
// watching.
type cameraRelay interface {
	Offer(ctx context.Context, offer string, open webrtc.Opener) (string, error)
}

// cameras is the media plane: the supervised camera instances, read through the
// authorization graph.
//
// It holds no state of its own. The supervision plane already knows which
// cameras are running, and the graph already knows who may see which printer,
// so a set kept here would be a third copy that goes stale between them. No
// authorization answer is cached, so no cache key can collide. Every request
// asks the graph. The only thing held per camera is that camera's own last
// frame, which belongs to no principal.
type cameras struct {
	graph cameraGraph
	host  *resourcehost.Host
	relay cameraRelay
	log   *slog.Logger
	// hub is where a stored frame is announced. Nil announces nothing.
	hub *stream.Hub
}

var _ httpapi.CameraFleet = (*cameras)(nil)

func newCameras(
	graph cameraGraph, host *resourcehost.Host, relay cameraRelay, logger *slog.Logger,
) *cameras {
	return &cameras{graph: graph, host: host, relay: relay, log: logger}
}

// Cameras returns every camera this principal may read telemetry from, sorted
// by name.
//
// The permitted device set is read once, in one query, and the cameras are
// supervised instances in memory and not rows. There is no page to truncate
// and no total to leak: the count this answers with is the count of what the
// caller may see.
//
// A camera is a device with a row of its own, so the question asked here is
// the question asked about a printer. A camera bound to a printer is in the
// set because the graph inherits the printer's grants down the parent edge. A
// camera granted directly is in it because somebody wrote a tuple on the
// camera.
//
// The graph is read before the supervision plane is asked what it runs, so a
// deployment supervising no camera costs a caller the same query as one
// supervising many. reach holds the same order for one name.
func (c *cameras) Cameras(ctx context.Context, principal string) ([]httpapi.Camera, error) {
	permitted, err := c.permittedDevices(ctx, principal)
	if err != nil {
		return nil, err
	}

	blocked, err := c.blockedDevices(ctx, principal)
	if err != nil {
		return nil, err
	}

	running := c.host.Supervising(ref.KindCamera)

	out := make([]httpapi.Camera, 0, len(running))

	for instance := range running {
		if !permitted[instance.ID()] {
			continue
		}

		camera, attachErr := supervised.AttachCamera(running[instance])
		if attachErr != nil {
			continue
		}

		if blocked[instance.ID()] || (camera.Printer() != "" && blocked[camera.Printer()]) {
			continue
		}

		out = append(out, describe(instance, camera.Printer(), declaredType(c.host, instance), camera))
	}

	slices.SortFunc(out, func(a, b httpapi.Camera) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		}

		return 0
	})

	return out, nil
}

// Snapshot returns one camera's most recent still image.
//
// A camera that is working and has produced no frame yet is not a failure: the
// answer is an empty snapshot, which the transport renders as 204. A caller who
// may not see this camera gets the one refusal every media path gives, so they
// learn nothing about it. A caller who may see it and whose camera did not
// answer is told that, because they already know the camera is there.
func (c *cameras) Snapshot(
	ctx context.Context, principal string, name string,
) (httpapi.CameraSnapshot, error) {
	camera, err := c.reach(ctx, principal, name)
	if err != nil {
		return httpapi.CameraSnapshot{}, err
	}

	if !slices.Contains(camera.Formats(), cameraapi.FormatJPEG) {
		return httpapi.CameraSnapshot{}, httpapi.CameraNotFound(name)
	}

	shot, err := camera.Snapshot(ctx)

	switch {
	case errors.Is(err, cameraapi.ErrNoFrame):
		return httpapi.CameraSnapshot{MaxAge: snapshotMaxAge}, nil
	case err != nil:
		c.log.WarnContext(ctx, "a camera could not be read",
			"resource", camera.Ref().String(), "error", err.Error())

		// The device on the operator's network did not answer. The transport
		// cannot type the driver's own error, so returning it would report
		// every unread camera as a 500 of this server and not as a fault of
		// the camera.
		return httpapi.CameraSnapshot{}, httpapi.CameraUnreachable(name)
	}

	return httpapi.CameraSnapshot{
		Bytes:     shot.Bytes,
		MediaType: shot.MediaType,
		Taken:     shot.Taken,
		MaxAge:    snapshotMaxAge,
	}, nil
}

// Push stores one frame a camera sent for itself.
//
// It is the only write path of the media plane. A camera nothing on this
// network can dial sends its own picture here, the driver holds it as its most
// recent frame, and the snapshot and stream paths serve it exactly as they
// serve a frame the host fetched.
//
// It takes operate on the camera's own device row, and not observe. Whoever may
// watch a camera must not be able to decide what it shows: a viewer who could
// push could put any picture in front of everybody else. The action is operate
// because sending a frame is the camera's normal duty and changes nothing
// about what the machine is.
//
// report_telemetry would fit better, but only the agent relation reaches that
// action and only a device credential holds that relation. Nothing mints a
// device credential for a camera, because a camera is a declared resource and
// not a machine that registers.
//
// A camera the caller may not see, one nothing supervises, and one whose driver
// reads a device and takes no frames are the same refusal, for the reason
// every other path on this plane gives: a different answer for each would let a
// caller enumerate the cameras of printers they cannot see.
func (c *cameras) Push(
	ctx context.Context, principal string, name string, frame httpapi.CameraFrame,
) error {
	camera, err := c.admit(ctx, principal, name)
	if err != nil {
		return err
	}

	err = camera.Accept(ctx, cameraapi.Snapshot{
		Bytes:     frame.Bytes,
		MediaType: frame.MediaType,
		Taken:     frame.Taken,
	})

	switch {
	case errors.Is(err, cameraapi.ErrPushNotSupported):
		return httpapi.CameraNotFound(name)
	case errors.Is(err, cameraapi.ErrInvalidSnapshot):
		return httpapi.CameraFrameRefused(name)
	case err != nil:
		c.log.WarnContext(ctx, "a pushed camera frame was not stored",
			"resource", camera.Ref().String(), "error", err.Error())

		return err
	}

	c.announceFrame(ctx, name)

	return nil
}

// MayPush answers whether principal may hand the camera a frame, without one.
// A door that reads the frame after it asks spends no memory on a refusal.
func (c *cameras) MayPush(ctx context.Context, principal string, name string) error {
	_, err := c.admit(ctx, principal, name)

	return err
}

// admit is the camera principal may push to, or the refusal Push answers.
func (c *cameras) admit(ctx context.Context, principal string, name string) (*supervised.Camera, error) {
	if c.ownedBy(principal, name) {
		return c.running(name)
	}

	camera, err := c.reach(ctx, principal, name)
	if err != nil {
		return nil, err
	}

	if err = c.mayPush(ctx, principal, name); err != nil {
		return nil, err
	}

	return camera, nil
}

// ownedBy reports whether principal is the module that offers the camera's
// declared type (ADR-20261106-km2). It reads the type at each call, so a camera
// whose type changes leaves its module at once.
func (c *cameras) ownedBy(principal string, name string) bool {
	instance, err := ref.New(ref.KindCamera, name)
	if err != nil {
		return false
	}

	declared, ok := c.host.Declared(instance)
	if !ok {
		return false
	}

	owner, ok := c.host.CameraTypes()[declared.Spec.Type]

	return ok && sinkPrincipal(owner.Module) == principal
}

// running is the camera the host runs under name.
func (c *cameras) running(name string) (*supervised.Camera, error) {
	instance, err := ref.New(ref.KindCamera, name)
	if err != nil {
		return nil, httpapi.CameraNotFound(name)
	}

	runner, ok := c.host.Instance(instance)
	if !ok {
		return nil, httpapi.CameraNotFound(name)
	}

	camera, err := supervised.AttachCamera(runner)
	if err != nil {
		return nil, httpapi.CameraNotFound(name)
	}

	return camera, nil
}

// announceFrame publishes event.TypeCameraFrameReceived on the camera's own
// origin, so a page loads the still when there is a new one rather than on a
// clock. The frame stays on the camera's own path.
func (c *cameras) announceFrame(ctx context.Context, name string) {
	if c.hub == nil {
		return
	}

	origin, err := event.NewOrigin(event.KindDevice, name)
	if err != nil {
		return
	}

	if _, err = c.hub.Publish(stream.Publication{
		Origin: origin,
		Type:   event.TypeCameraFrameReceived,
		Class:  event.ClassTelemetry,
		Data:   []byte("{}"),
	}); err != nil {
		c.log.DebugContext(ctx, "a camera frame reached no hub", "camera", name, "error", err.Error())
	}
}

// mayPush refuses a caller who may watch this camera and may not decide what it
// shows.
//
// It is 403 and not the 404 the rest of this surface answers with, for the
// reason raw gcode answers 403: the camera is already in the caller's listing,
// because reaching it is what put it there, so hiding it here would withhold
// nothing and would tell somebody with a real grant that their camera had gone
// away.
func (c *cameras) mayPush(ctx context.Context, principal string, name string) error {
	who, err := authz.ParseSubject(principal)
	if err != nil {
		return fmt.Errorf("read the caller's principal: %w", err)
	}

	object, err := authz.NewObject(authz.TypeDevice, name)
	if err != nil {
		return httpapi.CameraNotFound(name)
	}

	allowed, err := c.graph.Can(ctx, who, authz.ActionOperate, object)
	if err != nil {
		return err
	}

	if !allowed {
		return httpapi.CameraPushDenied(name)
	}

	return nil
}

// Stream opens one camera's live view.
//
// A camera that emits no streamable format is the same refusal as one the
// caller may not see. It is not a 415: telling a caller which format a camera
// they cannot see does not emit is telling them the camera is there. A device
// that did not answer is separate, because reaching it is what put it in this
// caller's listing.
func (c *cameras) Stream(
	ctx context.Context, principal string, name string,
) (httpapi.CameraStream, error) {
	camera, err := c.reach(ctx, principal, name)
	if err != nil {
		return httpapi.CameraStream{}, err
	}

	if !slices.Contains(camera.Formats(), cameraapi.FormatMJPEG) {
		return httpapi.CameraStream{}, httpapi.CameraNotFound(name)
	}

	view, err := camera.Open(ctx, cameraapi.FormatMJPEG)
	if err != nil {
		if errors.Is(err, cameraapi.ErrFormatNotSupported) {
			return httpapi.CameraStream{}, httpapi.CameraNotFound(name)
		}

		c.log.WarnContext(ctx, "a camera could not be opened",
			"resource", camera.Ref().String(), "error", err.Error())

		return httpapi.CameraStream{}, httpapi.CameraUnreachable(name)
	}

	return httpapi.CameraStream{Body: view.Body, MediaType: view.MediaType}, nil
}

// Watch answers one browser's offer for a camera's H.264 live view.
//
// The refusals are the refusals of every other media path: a camera the caller
// may not see, one nothing supervises, and one that emits no H.264 are the same
// bytes. A malformed offer is the exception and it is the caller's own request,
// so it answers 400 and says which part of the body was wrong. Nothing about
// the camera reaches that answer.
//
// The camera is opened by the relay once the browser connects, not here. A
// browser that cannot reach this server's candidates falls back to Live, and
// an open here would hold the camera's stream slot against it.
func (c *cameras) Watch(
	ctx context.Context, principal string, name string, offer string,
) (httpapi.CameraAnswer, error) {
	camera, err := c.reach(ctx, principal, name)
	if err != nil {
		return httpapi.CameraAnswer{}, err
	}

	if c.relay == nil || !slices.Contains(camera.Formats(), cameraapi.FormatH264AnnexB) {
		return httpapi.CameraAnswer{}, httpapi.CameraNotFound(name)
	}

	answer, err := c.relay.Offer(ctx, offer, func(openCtx context.Context) (io.ReadCloser, error) {
		view, openErr := camera.Open(openCtx, cameraapi.FormatH264AnnexB)
		if openErr != nil {
			c.log.WarnContext(openCtx, "a camera could not be opened",
				"resource", camera.Ref().String(), "error", openErr.Error())

			return nil, openErr
		}

		return view.Body, nil
	})
	if err != nil {
		return httpapi.CameraAnswer{}, offerRefusal(err)
	}

	return httpapi.CameraAnswer{SDP: answer}, nil
}

// Live opens one camera's H.264 live view as access units, for a browser that
// could not connect over WebRTC.
//
// It is refused exactly as Watch is, and the camera is opened here, before the
// socket is upgraded, so a camera that does not answer is a 502 the browser can
// read and not a socket that closes with no reason.
func (c *cameras) Live(
	ctx context.Context, principal string, name string,
) (httpapi.CameraLive, error) {
	camera, err := c.reach(ctx, principal, name)
	if err != nil {
		return nil, err
	}

	if !slices.Contains(camera.Formats(), cameraapi.FormatH264AnnexB) {
		return nil, httpapi.CameraNotFound(name)
	}

	view, err := camera.Open(ctx, cameraapi.FormatH264AnnexB)
	if err != nil {
		if errors.Is(err, cameraapi.ErrFormatNotSupported) {
			return nil, httpapi.CameraNotFound(name)
		}

		c.log.WarnContext(ctx, "a camera could not be opened",
			"resource", camera.Ref().String(), "error", err.Error())

		return nil, httpapi.CameraUnreachable(name)
	}

	return webrtc.NewAccessUnits(view.Body, c.log), nil
}

// offerRefusal renders a negotiation failure. An offer this server cannot
// answer is the caller's own body, so it is reported as an invalid request.
// Behind the camera's refusal a client would retry a request that can never
// work.
func offerRefusal(err error) error {
	if errors.Is(err, webrtc.ErrNotAnOffer) || errors.Is(err, webrtc.ErrOfferTooLarge) {
		return httpfault.RequestInvalid("body", "the body is not an SDP offer this server can answer", "")
	}

	return err
}

// reach resolves one camera and decides whether this principal may see it.
//
// The decision is taken on the camera's own device row. A camera bound to a
// printer is reached by whoever reaches that printer, because the graph
// inherits down the parent edge. A camera granted directly is reached by
// whoever holds that grant, and a camera bound to nothing is reached by
// whoever holds a grant on it.
// A camera the host is not supervising, one the graph does not permit, and a
// name nothing answers to all give the same refusal, because a different answer
// for each would let a caller enumerate the cameras of printers they cannot see.
//
// The graph is read before the supervision plane is touched, and it is read for
// every name a caller asks about. If the code decided what exists first, a
// name this server supervises would reach the query and a name it does not
// would not. The time the refusal took would then tell the two apart, and the
// identical bytes would hide nothing. deviceaccess.go holds the same order for
// printers.
//
// It reads the caller's whole permitted set and does not ask about one
// device. The set is one query either way, and this keeps the answer for a
// name in the set and a name outside it one code path.
func (c *cameras) reach(
	ctx context.Context, principal string, name string,
) (*supervised.Camera, error) {
	instance, err := ref.New(ref.KindCamera, name)
	if err != nil {
		return nil, httpapi.CameraNotFound(name)
	}

	permitted, err := c.permittedDevices(ctx, principal)
	if err != nil {
		return nil, err
	}

	// Both graph reads happen before the supervision plane is touched, for the
	// reason the comment above gives: a name this server supervises and a name
	// it does not must cost the same work, or the time the refusal took would
	// tell them apart.
	blocked, err := c.blockedDevices(ctx, principal)
	if err != nil {
		return nil, err
	}

	runner, running := c.host.Instance(instance)
	if !running {
		return nil, httpapi.CameraNotFound(name)
	}

	camera, err := supervised.AttachCamera(runner)
	if err != nil {
		return nil, httpapi.CameraNotFound(name)
	}

	if !permitted[instance.ID()] {
		return nil, httpapi.CameraNotFound(name)
	}

	// A camera the caller may reach and whose printer blocks them is refused
	// with the same bytes as one they may not reach at all, because a block is
	// a decision about a person and not something to tell them about.
	if blocked[instance.ID()] || (camera.Printer() != "" && blocked[camera.Printer()]) {
		return nil, httpapi.CameraNotFound(name)
	}

	return camera, nil
}

// blockedDevices is the set of devices this principal is blocked on, as one
// query. A camera bound to a printer in it is refused whatever grant the camera
// itself carries.
func (c *cameras) blockedDevices(ctx context.Context, principal string) (map[string]bool, error) {
	who, err := authz.ParseSubject(principal)
	if err != nil {
		return nil, fmt.Errorf("read the caller's principal: %w", err)
	}

	return c.graph.BlockedDevices(ctx, who)
}

// permittedDevices is the set of devices this principal may read telemetry
// from, as one query. A camera is one of them.
func (c *cameras) permittedDevices(ctx context.Context, principal string) (map[string]bool, error) {
	who, err := authz.ParseSubject(principal)
	if err != nil {
		return nil, fmt.Errorf("read the caller's principal: %w", err)
	}

	visible, err := c.graph.Visible(ctx, who)
	if err != nil {
		return nil, err
	}

	permitted := make(map[string]bool, len(visible.Devices))
	for _, device := range visible.Devices {
		permitted[device.ID] = true
	}

	return permitted, nil
}

// CameraTypes is the core's generic camera and the camera types the installed
// modules offer. The list names what a person may pick when adding a camera,
// so it carries no camera and takes no grant beyond being signed in, which the
// API's middleware already holds every caller to.
func (c *cameras) CameraTypes(context.Context, string) ([]httpapi.CameraType, error) {
	out := []httpapi.CameraType{{Type: "generic"}}

	offered := c.host.CameraTypes()
	names := slices.Sorted(maps.Keys(offered))

	for _, name := range names {
		one := offered[name]
		out = append(out, httpapi.CameraType{Type: one.Type, Title: one.Title, Module: one.Namespace})
	}

	return out, nil
}

// declaredType is the type a camera is declared with, which is a module's
// type for a camera that runs on the generic driver for it (ADR-20261106-km2).
func declaredType(host *resourcehost.Host, instance ref.Ref) string {
	declared, ok := host.Declared(instance)
	if !ok {
		return ""
	}

	return declared.Spec.Type
}

// describe renders one camera for the listing. The declared format set decides
// which paths will answer, so a client builds a grid without asking for a path
// that is going to refuse.
func describe(instance ref.Ref, printer, typeName string, camera *supervised.Camera) httpapi.Camera {
	declared := camera.Formats()

	names := make([]string, 0, len(declared))
	for _, format := range declared {
		names = append(names, format.String())
	}

	return httpapi.Camera{
		Name:          instance.ID(),
		Type:          typeName,
		DeviceID:      printer,
		Formats:       names,
		Serving:       camera.Instance().Status().Serving(),
		Snapshot:      slices.Contains(declared, cameraapi.FormatJPEG),
		Stream:        slices.Contains(declared, cameraapi.FormatMJPEG),
		Watch:         slices.Contains(declared, cameraapi.FormatH264AnnexB),
		MinIntervalMS: intervalMS(camera.MinInterval()),
	}
}

// intervalMS renders a declared cadence for the wire, as an integer of
// milliseconds.
//
// A driver that declares nothing, and one whose cadence is under a
// millisecond, both answer zero, and the listing leaves the member out. A
// rounded-down cadence would be a number telling a client to ask faster than
// the camera can answer.
func intervalMS(declared time.Duration) int64 {
	if declared < time.Millisecond {
		return 0
	}

	return declared.Milliseconds()
}

// closeRelay releases the camera relay's UDP port once the HTTP surface that
// opens views has stopped.
func closeRelay(ctx context.Context, relay *webrtc.Engine, logger *slog.Logger) {
	if err := relay.Close(); err != nil {
		logger.WarnContext(ctx, "the media relay did not close", "error", err.Error())
	}
}

// newRelay builds the camera relay on the port and addresses the
// configuration gives browsers to reach it.
func newRelay(
	ctx context.Context, logger *slog.Logger, reach config.WebRTC, loopbackOnly bool,
) (*webrtc.Engine, error) {
	return webrtc.New(ctx, webrtc.Config{
		Logger:       logger,
		LoopbackOnly: loopbackOnly,
		UDPPort:      reach.UDPPort,
		Addresses:    reach.Addresses,
	})
}
