// Package generic is one camera whose stills are pushed or polled and whose
// optional stream is RTSP or MJPEG. It only routes each call to the push, mjpeg
// or rtsp driver.
package generic

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/prusa3d/prusa-connect/adapters/camera/mjpeg"
	"github.com/prusa3d/prusa-connect/adapters/camera/push"
	"github.com/prusa3d/prusa-connect/adapters/camera/rtsp"
	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
)

// Type is the value of a camera instance's `type` in configuration that selects
// this driver.
const Type = "generic"

// ErrStreamScheme rejects a stream address this driver cannot read.
var ErrStreamScheme = errors.New("generic: config.stream_url is rtsp, rtsps, http or https")

//nolint:gochecknoinits // registration has to happen before main; see ADR-20260824-nyw.
func init() { cameraapi.Register(Type, doc(), New) }

// doc is what the published extension reference says about this driver.
func doc() extension.Doc {
	return extension.Doc{
		Summary: "One camera that takes still images, pushed or polled, and may also stream over RTSP or MJPEG.",
		Description: "The still images are the frames the device sends to PUT /api/v1/cameras/{camera}/snapshot, " +
			"or, when snapshot_url is set, the image this server fetches from it. The stream is optional: " +
			"an rtsp:// address is read as H.264 and served to a browser through the WebRTC relay, and an " +
			"http:// address is relayed as MJPEG. It is read only while somebody watches. A user name and " +
			"password go in the address, and are kept out of every log.",
		Version:   "1.0.0",
		Contract:  cameraapi.Contract,
		Stability: extension.StabilityStable,
		Options: []extension.Option{
			{
				Key:     "snapshot_url",
				Type:    extension.OptionURL,
				Summary: "An address this server fetches a still image from. Empty takes the images the camera sends.",
			},
			{
				Key:     "stream_url",
				Type:    extension.OptionURL,
				Summary: "An rtsp:// address of an H.264 stream, or an http:// address of an MJPEG stream. Empty is no stream.",
			},
			{
				Key:     "stale",
				Type:    extension.OptionDuration,
				Default: "5m",
				Summary: "How long a sent image is shown after it arrived. Past it this camera answers as one that has produced nothing.",
			},
			{
				Key:     "timeout",
				Type:    extension.OptionDuration,
				Default: "5s",
				Summary: "How long one RTSP handshake message, or one fetch from snapshot_url, waits before it is abandoned. A source that is slow to answer, such as one that starts a transcoder first, needs longer.",
			},
		},
	}
}

// Camera is the sdk/cameraapi.Camera this package registers.
type Camera struct {
	// stills answers a snapshot: the push driver, or the MJPEG driver over
	// snapshot_url.
	stills cameraapi.Camera
	// pushed is stills when the camera sends its own images, and nil when
	// this server polls them.
	pushed cameraapi.Receiver
	// stream answers Open, and is nil for a camera with no stream.
	stream       cameraapi.Camera
	streamFormat cameraapi.Format
}

var (
	_ cameraapi.Camera   = (*Camera)(nil)
	_ cameraapi.Paced    = (*Camera)(nil)
	_ cameraapi.Receiver = (*Camera)(nil)
)

// New builds a Camera from its spec. It is the cameraapi.ProviderFactory for
// Type.
func New(spec cameraapi.ProviderSpec) (cameraapi.Camera, error) {
	camera := &Camera{}

	if err := camera.buildStills(spec); err != nil {
		return nil, err
	}

	if err := camera.buildStream(spec); err != nil {
		_ = camera.stills.Close()

		return nil, err
	}

	return camera, nil
}

func (c *Camera) buildStills(spec cameraapi.ProviderSpec) error {
	snapshot, _ := spec.Config["snapshot_url"].(string)
	if snapshot == "" {
		held, err := push.New(subSpec(spec, map[string]any{"stale": spec.Config["stale"]}))
		if err != nil {
			return fmt.Errorf("generic: %w", err)
		}

		c.stills = held
		c.pushed, _ = held.(cameraapi.Receiver)

		return nil
	}

	fetched, err := mjpeg.New(subSpec(spec, map[string]any{
		"snapshot_url": snapshot,
		"timeout":      timeoutOf(spec.Config),
	}))
	if err != nil {
		return fmt.Errorf("generic: %w", err)
	}

	c.stills = fetched

	return nil
}

func (c *Camera) buildStream(spec cameraapi.ProviderSpec) error {
	address, _ := spec.Config["stream_url"].(string)
	if address == "" {
		return nil
	}

	parsed, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("%w: it does not parse", ErrStreamScheme)
	}

	switch parsed.Scheme {
	case "rtsp", "rtsps":
		c.stream, err = rtsp.New(subSpec(spec, map[string]any{"url": address, "timeout": timeoutOf(spec.Config)}))
		c.streamFormat = cameraapi.FormatH264AnnexB
	case "http", "https":
		c.stream, err = mjpeg.New(subSpec(spec, map[string]any{
			"stream_url": address,
			"timeout":    timeoutOf(spec.Config),
		}))
		c.streamFormat = cameraapi.FormatMJPEG
	default:
		return fmt.Errorf("%w: it is %q", ErrStreamScheme, parsed.Scheme)
	}

	if err != nil {
		return fmt.Errorf("generic: %w", err)
	}

	return nil
}

// timeoutOf is the timeout setting as the rtsp driver reads it: a positive
// duration, or nil when the inner driver's own 5s default applies. The mjpeg
// driver takes zero or a negative value as written and would then fail every
// fetch, so a value the rtsp driver ignores is passed to neither of them.
func timeoutOf(config map[string]any) any {
	switch value := config["timeout"].(type) {
	case string:
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			return value
		}
	case time.Duration:
		if value > 0 {
			return value
		}
	}

	return nil
}

// subSpec is the spec one inner driver is built from: the same camera, with
// only the keys that driver reads.
func subSpec(spec cameraapi.ProviderSpec, config map[string]any) cameraapi.ProviderSpec {
	inner := spec
	inner.Config = map[string]any{}

	for key, value := range config {
		if value != nil {
			inner.Config[key] = value
		}
	}

	return inner
}

// Formats is a still image always, and the stream's format when there is one.
// A camera that sends its own images declares no MJPEG made of them: at one
// image every half minute it would be a stream that shows nothing new.
func (c *Camera) Formats() []cameraapi.Format {
	formats := []cameraapi.Format{cameraapi.FormatJPEG}
	if c.stream != nil {
		formats = append(formats, c.streamFormat)
	}

	return formats
}

// MinInterval is how often the still images can change. A camera that is sent
// its images declares none, because the device decides, and a page waits for
// the frame event instead of asking at the push driver's poll rate.
func (c *Camera) MinInterval() time.Duration {
	if c.pushed != nil {
		return 0
	}

	if paced, ok := c.stills.(cameraapi.Paced); ok {
		return paced.MinInterval()
	}

	return 0
}

// Snapshot is the last image the camera sent, or the one fetched from
// snapshot_url.
func (c *Camera) Snapshot(ctx context.Context) (cameraapi.Snapshot, error) {
	return c.stills.Snapshot(ctx)
}

// Accept keeps one image the camera sent. A camera whose images this server
// fetches takes none, because the next fetch would replace it.
func (c *Camera) Accept(ctx context.Context, shot cameraapi.Snapshot) error {
	if c.pushed == nil {
		return fmt.Errorf("%w: this camera's images are fetched from snapshot_url",
			cameraapi.ErrPushNotSupported)
	}

	return c.pushed.Accept(ctx, shot)
}

// Open starts the stream, the one format besides the still image.
func (c *Camera) Open(ctx context.Context, format cameraapi.Format) (cameraapi.Stream, error) {
	if c.stream == nil || format != c.streamFormat {
		return cameraapi.Stream{}, fmt.Errorf("%w: this camera emits %v",
			cameraapi.ErrFormatNotSupported, c.Formats())
	}

	return c.stream.Open(ctx, format)
}

// Close releases both halves.
func (c *Camera) Close() error {
	err := c.stills.Close()

	if c.stream != nil {
		err = errors.Join(err, c.stream.Close())
	}

	return err
}
