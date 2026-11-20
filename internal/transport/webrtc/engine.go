// Package webrtc remuxes a camera's H.264 elementary stream into a WebRTC
// track a browser plays.
//
// It reads NAL boundaries and never a pixel. There is no pure-Go H.264 encoder
// and the Pi 5 has no hardware one, so code that re-encoded could not run on
// the target machine. What happens here is start-code framing in and RTP
// payloads out.
//
// The engine defaults to no ICE server. Host candidates are addresses bound on
// this machine (RFC 8445 section 3), so a browser on the same network completes
// a connection with no STUN and no TURN, and an air-gapped install keeps
// working. A public STUN server would be a hidden cloud dependency, so an
// operator must name one explicitly.
//
// A server in a container or behind NAT holds no address a browser can dial.
// Config.UDPPort serves every viewer on one port an operator can publish, and
// Config.Addresses offers the address the browser reaches it at, beside the
// ones bound here and never in their place.
package webrtc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/logging"
	pion "github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// The refusals a caller inspects with errors.Is.
var (
	// ErrNotAnOffer reports a body that is not an SDP offer this engine can
	// answer, which is a client error and never a camera's.
	ErrNotAnOffer = errors.New("webrtc: the body is not an SDP offer with a video track")
	// ErrOfferTooLarge reports an offer over the ceiling. An SDP a browser
	// produces is a few kilobytes.
	ErrOfferTooLarge = errors.New("webrtc: the offer is over the ceiling")
	// ErrAddressesWithoutPort refuses advertised addresses with no shared
	// port. pion would pair an advertised address with a socket of its own
	// choosing, the loopback one among them, and a reply from there never
	// reaches the browser.
	ErrAddressesWithoutPort = errors.New("webrtc: advertised addresses need a fixed UDP port")
)

const (
	// offerCeiling bounds one offer. Chrome's offer for a single recvonly video
	// transceiver is about 2 KiB, so 64 KiB is ample and still refuses a body
	// sent to make this server allocate.
	offerCeiling = 64 << 10

	// defaultConnectTimeout is how long a negotiated view may fail to connect
	// before it is torn down. A client that asks for an answer and never dials
	// otherwise holds a camera's bulkhead slot and a device socket for ever.
	defaultConnectTimeout = 30 * time.Second

	// h264ClockRate is the RTP clock H.264 is carried on, fixed by RFC 6184.
	h264ClockRate = 90000

	// openTimeout bounds how long a connected view waits for the camera to
	// open. The request that negotiated the view has already returned, so
	// nothing else ends a wait on a camera that never answers.
	openTimeout = 30 * time.Second

	// reportBuffer holds one receiver report. It is the usual path MTU, which
	// is larger than any RTCP compound this receives.
	reportBuffer = 1500

	// The bounds on one access unit's duration. A camera that stalls and then
	// resumes must not advance the RTP clock by the length of the stall, and
	// two units in the same millisecond must still advance it.
	minUnitDuration = time.Millisecond
	maxUnitDuration = time.Second
)

// Config is what the composition root supplies.
type Config struct {
	// Logger is where a view's failures are reported. It is required.
	Logger *slog.Logger
	// ICEServers is the STUN and TURN list, empty by default. An operator who
	// watches a camera across a NAT names one here and takes the dependency
	// deliberately.
	ICEServers []pion.ICEServer
	// ConnectTimeout bounds how long a negotiated view may stay unconnected.
	// Zero takes defaultConnectTimeout.
	ConnectTimeout time.Duration
	// LoopbackOnly gathers candidates on the loopback interface and nowhere
	// else, so no socket is bound on an address another machine can reach.
	//
	// The tests of this package set it, and so does a core started with
	// app.Options.LoopbackOnly. Gathering on every interface opens a UDP
	// socket on each one, and on Windows the firewall then asks the person at
	// the keyboard to allow the binary, on every run.
	LoopbackOnly bool
	// UDPPort serves every viewer on this one UDP port. Zero takes a random
	// port for each viewer, which a container cannot publish.
	UDPPort int
	// Addresses are IP addresses a browser reaches this machine at, offered as
	// host candidates beside the ones gathered here. They need UDPPort: only
	// the one shared socket answers from the address the kernel routes by.
	Addresses []string
}

// Engine negotiates viewers and relays frames to them. One engine serves every
// camera, because a peer connection is per viewer and nothing here is per
// device.
type Engine struct {
	api     *pion.API
	config  pion.Configuration
	timeout time.Duration
	log     *slog.Logger
	// mux is the socket on Config.UDPPort, or nil when each viewer takes a
	// port of its own.
	mux *ice.UDPMuxDefault
}

// New builds the engine. It fails on a media engine this build cannot
// construct, which is a programming error, and on a UDPPort it cannot bind,
// which is the operator's to fix.
func New(ctx context.Context, cfg Config) (*Engine, error) {
	if cfg.Logger == nil {
		return nil, errors.New("webrtc: a logger is required")
	}

	engine := &pion.MediaEngine{}
	if err := engine.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("register the media codecs: %w", err)
	}

	interceptors := &interceptor.Registry{}
	if err := pion.RegisterDefaultInterceptors(engine, interceptors); err != nil {
		return nil, fmt.Errorf("register the media interceptors: %w", err)
	}

	settings := pion.SettingEngine{}

	// A server publishes the address a browser dials. mDNS obfuscation is a
	// browser-side privacy feature and here it would hide the only candidate
	// that works on a LAN with no mDNS responder.
	settings.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

	// A host with no interface up but loopback is the air-gapped case and the
	// container case, and without this it gathers no candidate at all.
	settings.SetIncludeLoopbackCandidate(true)

	if cfg.LoopbackOnly {
		keepLoopbackOnly(&settings)
	}

	if len(cfg.Addresses) > 0 && cfg.UDPPort == 0 {
		return nil, ErrAddressesWithoutPort
	}

	if err := advertise(&settings, cfg.Addresses); err != nil {
		return nil, err
	}

	mux, err := listenOnePort(ctx, &settings, cfg.UDPPort, cfg.LoopbackOnly)
	if err != nil {
		return nil, err
	}

	timeout := cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = defaultConnectTimeout
	}

	return &Engine{
		mux: mux,
		api: pion.NewAPI(
			pion.WithMediaEngine(engine),
			pion.WithInterceptorRegistry(interceptors),
			pion.WithSettingEngine(settings),
		),
		config:  pion.Configuration{ICEServers: cfg.ICEServers},
		timeout: timeout,
		log:     cfg.Logger,
	}, nil
}

// advertise offers each address as one more host candidate. Append keeps the
// addresses gathered here, so a browser on the same network still dials this
// machine directly when the advertised address is one it cannot reach.
func advertise(settings *pion.SettingEngine, addresses []string) error {
	if len(addresses) == 0 {
		return nil
	}

	err := settings.SetICEAddressRewriteRules(pion.ICEAddressRewriteRule{
		External:        addresses,
		AsCandidateType: pion.ICECandidateTypeHost,
		Mode:            pion.ICEAddressRewriteAppend,
	})
	if err != nil {
		return fmt.Errorf("advertise the addresses %v: %w", addresses, err)
	}

	return nil
}

// listenOnePort binds one UDP socket on port and hands it to every viewer. It
// returns nil for port zero, which leaves each viewer to bind a random port of
// its own.
//
// The socket is bound to the unspecified address, so the kernel picks the
// source address of every reply by its route. A socket per interface would
// answer an advertised address from whichever interface pion listed first,
// and a reply from the loopback socket never leaves the machine.
func listenOnePort(
	ctx context.Context, settings *pion.SettingEngine, port int, loopbackOnly bool,
) (*ice.UDPMuxDefault, error) {
	if port == 0 {
		return nil, nil //nolint:nilnil // nil is the documented "no shared port", not a failure.
	}

	network, address := "udp", net.JoinHostPort("", strconv.Itoa(port))
	if loopbackOnly {
		network, address = "udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}

	var listener net.ListenConfig

	conn, err := listener.ListenPacket(ctx, network, address)
	if err != nil {
		return nil, fmt.Errorf("listen for camera viewers on UDP port %d: %w", port, err)
	}

	// pion warns about the unspecified address on every start. Binding it is
	// the choice above, so only its errors are kept.
	loggers := logging.NewDefaultLoggerFactory()
	loggers.DefaultLogLevel = logging.LogLevelError

	mux := ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: conn, Logger: loggers.NewLogger("ice")})
	settings.SetICEUDPMux(mux)

	return mux, nil
}

// Close releases the shared UDP port. A view still open loses its socket, so
// it is called once the HTTP surface that makes views has stopped.
func (e *Engine) Close() error {
	if e.mux == nil {
		return nil
	}

	if err := e.mux.Close(); err != nil {
		return fmt.Errorf("close the camera viewers' UDP port: %w", err)
	}

	return nil
}

// keepLoopbackOnly holds candidate gathering to the loopback interface.
//
// Both filters are set, because the two answer different questions. The
// interface filter is asked once per interface and the address filter once per
// address, and an interface named lo can still carry an address that is not a
// loopback one.
func keepLoopbackOnly(settings *pion.SettingEngine) {
	settings.SetInterfaceFilter(func(name string) bool {
		found, err := net.InterfaceByName(name)
		if err != nil {
			return false
		}

		return found.Flags&net.FlagLoopback != 0
	})

	settings.SetIPFilter(func(address net.IP) bool { return address.IsLoopback() })
}

// Opener opens the camera stream a view relays. The view calls it once, when
// the browser has connected, and owns what it returns.
type Opener func(ctx context.Context) (io.ReadCloser, error)

// Offer answers one browser's SDP offer and relays the camera to it once the
// browser connects.
//
// The camera is opened only then. A browser that never reaches this server,
// because no candidate in the answer is one it can dial, would otherwise hold
// one of the camera's stream slots until the connect deadline. The view the
// browser falls back to would then wait behind its own failed attempt.
//
// The view closes the stream when it ends, whichever way it ends: the browser
// going away, the camera stopping, or the connection failing. Closing the
// stream is what gives the camera's bulkhead slot and its device socket back.
func (e *Engine) Offer(ctx context.Context, offer string, open Opener) (string, error) {
	if open == nil {
		return "", errors.New("webrtc: no camera to relay")
	}

	if err := checkOffer(offer); err != nil {
		return "", err
	}

	peer, err := e.api.NewPeerConnection(e.config)
	if err != nil {
		return "", fmt.Errorf("open a peer connection: %w", err)
	}

	view := &view{peer: peer, open: open, log: e.log, connected: make(chan struct{})}

	// Watched before the offer is read, because a connection that arrives
	// during the negotiation would otherwise change state with nothing
	// listening, and the deadline below would then close a working view.
	view.watch()

	answer, err := e.negotiate(ctx, view, offer)
	if err != nil {
		view.close()

		return "", err
	}

	// The deadline starts here and not above, because it bounds a view that was
	// answered and never dialled. A clock started before the answer exists
	// closes the connection the negotiation is still building, and the caller
	// is then told the offer failed on a machine that was only slow.
	view.deadline(e.timeout)

	return answer, nil
}

// negotiate adds the track, answers the offer and waits for the candidates.
//
// The answer is returned with every candidate already in it. Trickle needs a
// second channel from the server back to the client, and this surface is one
// request and one response, so the candidates are gathered to completion and
// one POST is enough.
func (e *Engine) negotiate(ctx context.Context, v *view, offer string) (string, error) {
	track, err := pion.NewTrackLocalStaticSample(
		pion.RTPCodecCapability{MimeType: pion.MimeTypeH264, ClockRate: h264ClockRate},
		"video", "camera")
	if err != nil {
		return "", fmt.Errorf("open a video track: %w", err)
	}

	v.track = track

	sender, err := v.peer.AddTrack(track)
	if err != nil {
		return "", fmt.Errorf("attach the video track: %w", err)
	}

	go drainReports(sender)

	if err = v.peer.SetRemoteDescription(
		pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: offer}); err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotAnOffer, err)
	}

	answer, err := v.peer.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNotAnOffer, err)
	}

	gathered := pion.GatheringCompletePromise(v.peer)

	if err = v.peer.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("set the local description: %w", err)
	}

	select {
	case <-gathered:
	case <-ctx.Done():
		return "", fmt.Errorf("gather ice candidates: %w", ctx.Err())
	}

	local := v.peer.LocalDescription()
	if local == nil {
		return "", errors.New("webrtc: the peer connection produced no answer")
	}

	return local.SDP, nil
}

// view is one viewer: their peer connection, the camera stream feeding it, and
// the one close that ends both.
type view struct {
	peer  *pion.PeerConnection
	track *pion.TrackLocalStaticSample
	open  Opener
	log   *slog.Logger

	connected chan struct{}
	once      sync.Once
	ready     sync.Once
	dialled   sync.Once

	// mu guards frames and ended. The camera opens on a goroutine of its own
	// while the browser may already be closing the view.
	mu     sync.Mutex
	frames io.ReadCloser
	ended  bool
}

// watch ends the view when the connection fails or goes away.
func (v *view) watch() {
	v.peer.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		switch state {
		case pion.PeerConnectionStateConnected:
			v.ready.Do(func() { close(v.connected) })
			v.dialled.Do(func() { go v.relay() })
		case pion.PeerConnectionStateFailed,
			pion.PeerConnectionStateClosed,
			pion.PeerConnectionStateDisconnected:
			v.close()
		case pion.PeerConnectionStateNew,
			pion.PeerConnectionStateConnecting,
			pion.PeerConnectionStateUnknown:
		}
	})
}

// deadline ends the view when the answer is never dialled. It is started once
// the answer exists, so the clock measures the wait for the browser and not the
// negotiation this server did first.
func (v *view) deadline(timeout time.Duration) {
	timer := time.AfterFunc(timeout, func() {
		select {
		case <-v.connected:
		default:
			v.log.Warn("a camera view was answered and never dialled",
				"timeout", timeout.String())
			v.close()
		}
	})

	// The timer holds a reference to the view until it fires, so it is stopped
	// as soon as the connection arrives rather than left to run.
	go func() {
		<-v.connected
		timer.Stop()
	}()
}

// relay reads access units from the camera and writes them as samples.
//
// Each sample is one access unit, so the payloader marks the last packet of a
// picture and not the last packet of a NAL unit. The mark tells a decoder that
// a frame is complete.
func (v *view) relay() {
	defer v.close()

	frames, ok := v.start()
	if !ok {
		return
	}

	pictures := newPictures(frames).withLog(v.log)

	var last time.Time

	for {
		unit, err := pictures.next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				v.log.Warn("a camera view ended on a stream failure", "error", err.Error())
			}

			return
		}

		now := time.Now()

		duration := minUnitDuration
		if !last.IsZero() {
			duration = clamp(now.Sub(last))
		}

		last = now

		if err = v.track.WriteSample(media.Sample{Data: unit, Duration: duration}); err != nil {
			if !errors.Is(err, io.ErrClosedPipe) {
				v.log.Warn("a camera view ended on a write failure", "error", err.Error())
			}

			return
		}
	}
}

// start opens the camera and hands the stream to the view. It reports false
// when there is nothing to relay: the camera did not open, or the view ended
// while it was opening, in which case the stream is closed here.
func (v *view) start() (io.ReadCloser, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()

	frames, err := v.open(ctx)
	if err != nil {
		v.log.Warn("a connected camera view could not open the camera", "error", err.Error())

		return nil, false
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.ended {
		_ = frames.Close()

		return nil, false
	}

	v.frames = frames

	return frames, true
}

// close ends the view once. Closing the camera stream is what releases the
// device connection and the instance's bulkhead slot, so it happens on every
// path out.
func (v *view) close() {
	v.once.Do(func() {
		v.ready.Do(func() { close(v.connected) })

		v.mu.Lock()
		frames := v.frames
		v.ended = true
		v.mu.Unlock()

		if frames != nil {
			if err := frames.Close(); err != nil {
				v.log.Warn("a camera stream did not close cleanly", "error", err.Error())
			}
		}

		if err := v.peer.Close(); err != nil {
			v.log.Warn("a peer connection did not close cleanly", "error", err.Error())
		}
	})
}

// drainReports reads the receiver reports the browser sends back. They are not
// acted on, and they have to be read: the interceptor chain buffers them, and a
// buffer nobody drains stops the chain.
func drainReports(sender *pion.RTPSender) {
	report := make([]byte, reportBuffer)

	for {
		if _, _, err := sender.Read(report); err != nil {
			return
		}
	}
}

// checkOffer refuses a body that is not an offer this engine can answer, before
// a peer connection is built for it.
func checkOffer(offer string) error {
	if len(offer) > offerCeiling {
		return fmt.Errorf("%w: %d bytes", ErrOfferTooLarge, len(offer))
	}

	parsed, err := (&pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: offer}).Unmarshal()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotAnOffer, err)
	}

	for _, medium := range parsed.MediaDescriptions {
		if medium.MediaName.Media == "video" {
			return nil
		}
	}

	return fmt.Errorf("%w: it offers no video", ErrNotAnOffer)
}

// clamp bounds one access unit's duration. The elementary stream carries no
// timing, so the pace of arrival is the pace of the stream, and a camera that
// stalls for a minute must not advance the RTP clock by a minute.
func clamp(d time.Duration) time.Duration {
	switch {
	case d < minUnitDuration:
		return minUnitDuration
	case d > maxUnitDuration:
		return maxUnitDuration
	default:
		return d
	}
}
