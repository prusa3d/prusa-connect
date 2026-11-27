// Package rtsp reads a camera that speaks RTSP and emits H.264, such as the
// Prusa Buddy3D camera
// ([Buddy3D Camera](https://help.prusa3d.com/article/buddy3d-camera_821264)).
//
// No browser plays RTSP. This driver gives the host the H.264 elementary
// stream, and the host repacketises it into WebRTC. Nothing is decoded: the
// driver reads RTP payloads and writes the NAL units out with start codes.
//
// A session that describes anything but H.264 is refused with ErrNoH264.
package rtsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
)

// Type is the `type` of a camera instance that selects this driver.
const Type = "rtsp"

const (
	// defaultTimeout bounds one handshake message.
	defaultTimeout = 5 * time.Second

	// defaultMaxUnitBytes bounds one NAL unit, so a device cannot fill the
	// host's memory with one fragmented unit.
	defaultMaxUnitBytes = 4 << 20

	// emittableFormats is how many formats this driver can declare: an H.264
	// stream, and a still image when a snapshot URL is configured.
	emittableFormats = 2

	// frameBuffer is the starting size of the buffer a stream reuses for
	// every packet.
	frameBuffer = 64 << 10

	// malformedCeiling is how many malformed packets in a row a stream drops
	// before it ends. Two seconds of video at a low bit rate is about that
	// many packets.
	malformedCeiling = 64
)

var (
	// ErrNoURL rejects a spec that names no stream.
	ErrNoURL = errors.New("rtsp: config.url is required")
	// ErrNotRTSP rejects a scheme this driver cannot dial.
	ErrNotRTSP = errors.New("rtsp: config.url is rtsp or rtsps")
)

//nolint:gochecknoinits // registration has to happen before main; the host reaches it through a blank import.
func init() { cameraapi.Register(Type, doc(), New) }

// doc is this driver's entry in the published extension reference.
func doc() extension.Doc {
	return extension.Doc{
		Summary: "Reads an H.264 camera over RTSP, which is what the Prusa Buddy3D camera speaks.",
		Description: "Opens one RTSP session per viewer, interleaved over the one TCP connection, and " +
			"hands the host the H.264 elementary stream. The host repacketises that into WebRTC, so a " +
			"browser plays a camera whose own answer is otherwise VLC. Nothing is decoded or " +
			"re-encoded. A camera that describes H.265 is refused with a reason, because no browser " +
			"matrix covers it and nothing here can convert it. The session is opened when a viewer " +
			"arrives and closed when they leave, so an idle camera holds no device socket.",
		Version:   "1.0.0",
		Contract:  cameraapi.Contract,
		Stability: extension.StabilityExperimental,
		Options: []extension.Option{
			{
				Key:      "url",
				Type:     extension.OptionURL,
				Required: true,
				Summary:  "The RTSP stream, such as rtsp://192.168.1.50/live. rtsps carries the same session over TLS.",
			},
			{
				Key:     "snapshot_url",
				Type:    extension.OptionURL,
				Summary: "An HTTP endpoint answering one JPEG, for the thumbnail grid. Without it this camera declares no still image.",
			},
			{
				Key:     "username",
				Type:    extension.OptionString,
				Summary: "The user the camera challenges for. It is never logged, reported or echoed back.",
			},
			{
				Key:     "password",
				Type:    extension.OptionSecret,
				Summary: "The password the camera challenges for. Digest is answered where the camera offers it. It is never logged, reported or echoed back.",
			},
			{
				Key:     "timeout",
				Type:    extension.OptionDuration,
				Default: "5s",
				Summary: "How long one handshake message waits before the session is abandoned.",
			},
			{
				Key:     "max_unit_bytes",
				Type:    extension.OptionInt,
				Default: "4194304",
				Summary: "The ceiling on one H.264 NAL unit. A device that fragments without ever ending a unit is cut off here.",
			},
			{
				Key:     "verify_certificate",
				Type:    extension.OptionBool,
				Default: "false",
				Summary: "Require an rtsps camera's certificate to verify. A camera on a LAN rarely has one that does, so it is off unless the operator says so.",
			},
		},
	}
}

// Config is what the operator writes under a camera instance's `config`.
//
// It has no credential field on purpose. A %v of the struct would print one
// in a log or an error. The credential lives in a closure.
type Config struct {
	URL          string
	SnapshotURL  string
	Timeout      time.Duration
	MaxUnitBytes int
	Verify       bool
}

// Camera implements cameraapi.Camera.
type Camera struct {
	cfg       Config
	authorize credential
	formats   []cameraapi.Format
	still     *stills

	closeOnce sync.Once
	closed    chan struct{}
}

var _ cameraapi.Camera = (*Camera)(nil)

// New builds a Camera from its spec.
func New(spec cameraapi.ProviderSpec) (cameraapi.Camera, error) {
	cfg, authorize, err := readConfig(spec.Config)
	if err != nil {
		return nil, err
	}

	formats := make([]cameraapi.Format, 0, emittableFormats)
	formats = append(formats, cameraapi.FormatH264AnnexB)

	camera := &Camera{
		cfg:       cfg,
		authorize: authorize,
		formats:   formats,
		closed:    make(chan struct{}),
	}

	if cfg.SnapshotURL != "" {
		camera.formats = append(camera.formats, cameraapi.FormatJPEG)
		camera.still = newStills(cfg, authorize)
	}

	return camera, nil
}

// Formats is what this camera can emit: an H.264 stream always, and a still
// image when a snapshot URL is configured.
func (c *Camera) Formats() []cameraapi.Format {
	// A copy, because the host may sort it.
	return append([]cameraapi.Format(nil), c.formats...)
}

// Snapshot returns the still image from the configured HTTP endpoint. It does
// not take a frame from the stream, which would need an open RTSP session.
func (c *Camera) Snapshot(ctx context.Context) (cameraapi.Snapshot, error) {
	if c.still == nil {
		return cameraapi.Snapshot{}, fmt.Errorf("%w: this camera declares no still image",
			cameraapi.ErrFormatNotSupported)
	}

	select {
	case <-c.closed:
		return cameraapi.Snapshot{}, cameraapi.ErrNoFrame
	default:
	}

	return c.still.get(ctx)
}

// Open starts one RTSP session and returns its H.264 elementary stream. Each
// viewer gets its own session, released when the stream is closed.
func (c *Camera) Open(ctx context.Context, format cameraapi.Format) (cameraapi.Stream, error) {
	if format != cameraapi.FormatH264AnnexB {
		return cameraapi.Stream{}, fmt.Errorf("%w: this camera emits %v",
			cameraapi.ErrFormatNotSupported, c.formats)
	}

	select {
	case <-c.closed:
		return cameraapi.Stream{}, fmt.Errorf("%w: the camera is stopping", cameraapi.ErrNoFrame)
	default:
	}

	live, err := dial(ctx, c.cfg.URL, c.authorize, c.cfg.Timeout, c.cfg.Verify)
	if err != nil {
		return cameraapi.Stream{}, err
	}

	stream := newStream(live, c.cfg.MaxUnitBytes, c.closed)

	// The media type of an elementary stream is the empty string. The host
	// repacketises it.
	return cameraapi.Stream{Body: stream, MediaType: cameraapi.FormatH264AnnexB.MediaType()}, nil
}

// Close releases the device and ends every session this camera opened. It is
// safe to call more than once.
func (c *Camera) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)

		if c.still != nil {
			c.still.close()
		}
	})

	return nil
}

// stream is one viewer's elementary stream. A goroutine reads the RTSP session
// and writes Annex-B into a pipe the host reads. The pipe is unbuffered on
// purpose, so a slow viewer applies backpressure.
type stream struct {
	live    *session
	reader  *io.PipeReader
	writer  *io.PipeWriter
	limit   int
	stopped chan struct{}

	closeOnce sync.Once
	closeErr  error
}

func newStream(live *session, limit int, camera <-chan struct{}) *stream {
	reader, writer := io.Pipe()

	s := &stream{
		live:    live,
		reader:  reader,
		writer:  writer,
		limit:   limit,
		stopped: make(chan struct{}),
	}

	go s.pump()
	go s.follow(camera)
	go s.refresh()

	return s
}

// Read returns the next bytes of the elementary stream.
func (s *stream) Read(p []byte) (int, error) { return s.reader.Read(p) }

// Close ends the session and releases the connection to the device. Closing
// the pipe makes the pump return.
func (s *stream) Close() error {
	s.closeOnce.Do(func() {
		close(s.stopped)
		_ = s.reader.CloseWithError(io.EOF)
		s.closeErr = s.live.close()
	})

	return s.closeErr
}

// follow ends this stream when the camera instance stops.
func (s *stream) follow(camera <-chan struct{}) {
	select {
	case <-camera:
		_ = s.Close()
	case <-s.stopped:
	}
}

// refresh sends keepalives on a ticker. A slow camera can send no packet for
// longer than its session timeout, so keepalives cannot wait for packets.
func (s *stream) refresh() {
	tick := time.NewTicker(s.live.interval())
	defer tick.Stop()

	for {
		select {
		case <-s.stopped:
			return
		case <-tick.C:
			if err := s.live.keepalive(); err != nil {
				return
			}
		}
	}
}

// pump reads RTP packets and writes their NAL units. The parameter sets from
// the SDP go first, so a viewer has an SPS and a PPS before the first picture.
func (s *stream) pump() {
	defer func() { _ = s.writer.CloseWithError(io.EOF) }()

	units := newDepacketizer(s.limit)

	frame := make([]byte, 0, frameBuffer)
	for _, set := range s.live.sprop() {
		frame = appendUnit(frame, set)
	}

	if len(frame) > 0 {
		if _, err := s.writer.Write(frame); err != nil {
			return
		}
	}

	malformed := 0

	for {
		select {
		case <-s.stopped:
			return
		default:
		}

		packet, err := s.live.next()
		if err != nil {
			return
		}

		frame, err = units.append(frame[:0], packet)
		if err != nil {
			// A malformed packet loses the unit it belongs to, and a decoder
			// conceals one lost unit. A run of them is a camera that sends no
			// video, and that ends the view.
			units.drop()

			malformed++
			if malformed > malformedCeiling {
				_ = s.writer.CloseWithError(err)

				return
			}

			continue
		}

		malformed = 0

		if len(frame) == 0 {
			continue
		}

		if _, err = s.writer.Write(frame); err != nil {
			return
		}

		// The marker bit ends a picture. Nothing acts on it yet.
		_ = marked(packet)
	}
}

// readConfig reads the spec's map into typed settings. The credential is
// returned separately, so a Config never contains a password.
func readConfig(raw map[string]any) (Config, credential, error) {
	cfg := Config{
		Timeout:      defaultTimeout,
		MaxUnitBytes: defaultMaxUnitBytes,
	}

	cfg.URL = stringAt(raw, "url")
	cfg.SnapshotURL = stringAt(raw, "snapshot_url")
	cfg.Verify = boolAt(raw, "verify_certificate")

	if value, ok := durationAt(raw, "timeout"); ok && value > 0 {
		cfg.Timeout = value
	}

	if value, ok := intAt(raw, "max_unit_bytes"); ok && value > 0 {
		cfg.MaxUnitBytes = value
	}

	if cfg.URL == "" {
		return Config{}, nil, ErrNoURL
	}

	// Take the credential out of every URL. A %v of the struct would print
	// it. Any URL added later must go through withoutCredential too.
	dialable, username, password, err := checkURL(cfg.URL)
	if err != nil {
		return Config{}, nil, err
	}

	cfg.URL = dialable

	fetchable, stillUser, stillPassword, err := checkSnapshotURL(cfg.SnapshotURL)
	if err != nil {
		return Config{}, nil, err
	}

	cfg.SnapshotURL = fetchable

	// Both endpoints use one credential. The snapshot URL's applies when the
	// stream URL has none.
	if username == "" && password == "" {
		username, password = stillUser, stillPassword
	}

	// The username and password options win over a credential in a URL.
	if configured := stringAt(raw, "username"); configured != "" {
		username = configured
		password = stringAt(raw, "password")
	}

	if username == "" && password == "" {
		return cfg, nil, nil
	}

	return cfg, digestOrBasic(username, password), nil
}

func checkURL(endpoint string) (dialable, username, password string, err error) {
	return withoutCredential(endpoint, "config.url", "rtsp", "rtsps")
}

func checkSnapshotURL(endpoint string) (fetchable, username, password string, err error) {
	if endpoint == "" {
		return "", "", "", nil
	}

	return withoutCredential(endpoint, "config.snapshot_url", "http", "https")
}

// withoutCredential rejects a scheme not in schemes and returns the endpoint
// without its userinfo. No error quotes the URL, which may contain a password.
func withoutCredential(
	endpoint, field string, schemes ...string,
) (bare, username, password string, err error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %s could not be parsed", ErrNotRTSP, field)
	}

	if !slices.Contains(schemes, strings.ToLower(parsed.Scheme)) {
		return "", "", "", fmt.Errorf("%w: %s is %q and this driver speaks %s",
			ErrNotRTSP, field, strings.ToLower(parsed.Scheme), strings.Join(schemes, " or "))
	}

	if parsed.Host == "" {
		return "", "", "", fmt.Errorf("%w: %s names no host", ErrNotRTSP, field)
	}

	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}

	parsed.User = nil

	return parsed.String(), username, password, nil
}

func stringAt(raw map[string]any, key string) string {
	value, _ := raw[key].(string)

	return value
}

func boolAt(raw map[string]any, key string) bool {
	value, _ := raw[key].(bool)

	return value
}

func durationAt(raw map[string]any, key string) (time.Duration, bool) {
	switch value := raw[key].(type) {
	case string:
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return 0, false
		}

		return parsed, true
	case time.Duration:
		return value, true
	}

	return 0, false
}

func intAt(raw map[string]any, key string) (int, bool) {
	switch value := raw[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	}

	return 0, false
}
