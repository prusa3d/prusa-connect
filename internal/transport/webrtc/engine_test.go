package webrtc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	pion "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

// settle is how long a case waits for a peer connection to reach a state. ICE
// over loopback completes in tens of milliseconds; this is the ceiling before
// the case is a failure rather than a slow machine.
const settle = 20 * time.Second

// TestABrowserReceivesTheCamerasPicturesAsRTP is the whole feature in one case.
//
// A viewer offers, this engine answers, and the pictures the camera produced
// arrive as H.264 RTP on the viewer's track. The viewer here is a second peer
// connection rather than a browser, so what it proves is the negotiation and the
// packetisation. Whether a decoder is happy with the bytes is a different claim
// and it is not made here.
func TestABrowserReceivesTheCamerasPicturesAsRTP(t *testing.T) {
	t.Parallel()

	engine := newEngine(t)
	frames := newCamera(t, 40*time.Millisecond)

	viewer, offer := newViewer(t)

	packets := make(chan *pion.TrackRemote, 1)
	viewer.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		select {
		case packets <- track:
		default:
		}
	})

	answer, err := engine.Offer(t.Context(), offer, frames.opener())
	require.NoError(t, err)
	require.Contains(t, answer, "m=video", "the answer carries no video, so there is nothing to play")

	require.NoError(t, viewer.SetRemoteDescription(
		pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}))

	var track *pion.TrackRemote

	select {
	case track = <-packets:
	case <-time.After(settle):
		t.Fatal("no track ever arrived on the viewer, so the negotiation produced nothing to play")
	}

	require.Equal(t, pion.MimeTypeH264, track.Codec().MimeType)

	// Enough packets that the first one is not a fluke, and few enough that a
	// stalled relay fails the case rather than hanging it.
	var payload int

	for range 8 {
		require.NoError(t, track.SetReadDeadline(time.Now().Add(settle)))

		packet, _, readErr := track.ReadRTP()
		require.NoError(t, readErr, "the track stopped delivering after %d packets", payload)

		payload += len(packet.Payload)
	}

	require.Positive(t, payload, "eight packets arrived carrying no payload at all")
}

// A viewer who stops watching gives the camera back. The stream is what holds
// the instance's bulkhead slot and the device socket, so a view that ended
// without closing it is a slot nothing ever returns.
func TestAViewerGoingAwayClosesTheCameraStream(t *testing.T) {
	t.Parallel()

	frames := newCamera(t, 20*time.Millisecond)
	viewer, track := watching(t, newEngine(t), frames)

	// One packet read, so the view is playing rather than still negotiating.
	// A viewer that closes before it ever connects is the deadline's case and
	// it has its own.
	require.NoError(t, track.SetReadDeadline(time.Now().Add(settle)))

	_, _, err := track.ReadRTP()
	require.NoError(t, err)

	require.NoError(t, viewer.Close())

	require.Eventually(t, frames.isClosed, settle, 100*time.Millisecond,
		"the viewer went away and the camera stream is still open, so its bulkhead slot never comes back")
}

// watching negotiates one view and returns it once the pictures are arriving.
func watching(t *testing.T, engine *Engine, frames *camera) (*pion.PeerConnection, *pion.TrackRemote) {
	t.Helper()

	viewer, offer := newViewer(t)

	tracks := make(chan *pion.TrackRemote, 1)
	viewer.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
		select {
		case tracks <- track:
		default:
		}
	})

	answer, err := engine.Offer(t.Context(), offer, frames.opener())
	require.NoError(t, err)
	require.NoError(t, viewer.SetRemoteDescription(
		pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}))

	select {
	case track := <-tracks:
		return viewer, track
	case <-time.After(settle):
		t.Fatal("no track ever arrived on the viewer, so the negotiation produced nothing to play")
	}

	return nil, nil
}

// A client that asks for an answer and never dials never opens the camera. A
// browser that cannot reach any candidate falls back to the live socket, and
// a stream opened for the attempt would hold the camera's stream slot against
// it until the deadline.
func TestAViewerThatNeverDialsNeverOpensTheCamera(t *testing.T) {
	t.Parallel()

	engine := newEngineWithTimeout(t, 200*time.Millisecond)
	frames := newCamera(t, time.Hour)

	_, offer := newViewer(t)

	_, err := engine.Offer(t.Context(), offer, frames.opener())
	require.NoError(t, err)

	require.Never(t, frames.wasOpened, time.Second, 20*time.Millisecond,
		"the answer was never dialled and the camera was opened for it anyway")
}

// A camera that does not open once the browser has connected ends the view,
// so the browser sees the connection close rather than a picture that never
// comes.
func TestACameraThatDoesNotOpenEndsTheConnectedView(t *testing.T) {
	t.Parallel()

	engine := newEngine(t)
	viewer, offer := newViewer(t)

	ended := make(chan struct{})
	viewer.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		if state == pion.PeerConnectionStateClosed || state == pion.PeerConnectionStateFailed ||
			state == pion.PeerConnectionStateDisconnected {
			select {
			case <-ended:
			default:
				close(ended)
			}
		}
	})

	refused := func(context.Context) (io.ReadCloser, error) { return nil, errors.New("no answer") }

	answer, err := engine.Offer(t.Context(), offer, refused)
	require.NoError(t, err)
	require.NoError(t, viewer.SetRemoteDescription(
		pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}))

	select {
	case <-ended:
	case <-time.After(settle):
		t.Fatal("the camera did not open and the browser's connection stayed up")
	}
}

// The deadline bounds the wait for the browser and never the negotiation.
//
// A deadline of one millisecond is shorter than any negotiation, which is what
// a loaded runner makes of a longer one. A clock started before the answer
// existed closed the peer connection the negotiation was still building, and
// the caller was told its offer was not an offer.
func TestADeadlineShorterThanTheNegotiationStillAnswers(t *testing.T) {
	t.Parallel()

	engine := newEngineWithTimeout(t, time.Millisecond)
	frames := newCamera(t, time.Hour)

	_, offer := newViewer(t)

	answer, err := engine.Offer(t.Context(), offer, frames.opener())
	require.NoError(t, err,
		"the connect deadline closed the connection it was still negotiating")
	require.Contains(t, answer, "m=video")
	require.False(t, frames.wasOpened(), "the answer was never dialled and the camera was opened for it")
}

// Every address either half puts in its SDP is a loopback one.
//
// A candidate names an address a socket is bound on, so one naming a real
// interface is a socket another machine can reach. On Windows the firewall
// then asks the person at the keyboard to allow the test binary, on every run.
func TestEveryCandidateTheseTestsGatherIsOnTheLoopback(t *testing.T) {
	t.Parallel()

	engine := newEngine(t)
	frames := newCamera(t, time.Hour)

	viewer, offer := newViewer(t)

	answer, err := engine.Offer(t.Context(), offer, frames.opener())
	require.NoError(t, err)
	require.NoError(t, viewer.SetRemoteDescription(
		pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}))

	for name, sdp := range map[string]string{
		"the viewer's offer":   offer,
		"this engine's answer": answer,
	} {
		found := 0

		for line := range strings.SplitSeq(sdp, "\n") {
			if !strings.Contains(line, "candidate:") {
				continue
			}

			parts := strings.Fields(strings.TrimSpace(line))
			require.GreaterOrEqual(t, len(parts), 5, "%s carries a candidate this test cannot read: %s", name, line)

			address := net.ParseIP(parts[4])
			require.NotNil(t, address, "%s names something that is not an address: %s", name, line)
			require.True(t, address.IsLoopback(),
				"%s gathered %s, so a socket was bound off the loopback", name, address)

			found++
		}

		require.Positive(t, found, "%s carries no candidate at all", name)
	}
}

// A body that is not an offer is the caller's own request and it is refused
// before a peer connection is built for it, and before the camera is opened.
func TestABodyThatIsNotAnOfferIsRefusedAndTheCameraIsNeverOpened(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]struct {
		offer string
		want  error
	}{
		"empty":     {offer: "", want: ErrNotAnOffer},
		"prose":     {offer: "this is not an SDP", want: ErrNotAnOffer},
		"audioOnly": {offer: audioOnlyOffer, want: ErrNotAnOffer},
		// Refused for its size, and refused before a peer connection is built
		// for it. A body that reaches the SDP parser has already been read into
		// memory and parsed, which is the work the ceiling exists to refuse.
		"tooLarge": {offer: strings.Repeat("a", offerCeiling+1), want: ErrOfferTooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			frames := newCamera(t, time.Hour)

			_, err := newEngine(t).Offer(t.Context(), body.offer, frames.opener())
			require.ErrorIs(t, err, body.want,
				"a body this server cannot answer was reported as something else")
			require.False(t, frames.wasOpened(), "the offer was refused and the camera was opened for it")
		})
	}
}

// A driver that declared h264/annexb and framed something else ends the view
// rather than putting the bytes on a browser's decoder.
func TestAStreamThatIsNotH264EndsTheViewRatherThanReachingTheBrowser(t *testing.T) {
	t.Parallel()

	engine := newEngine(t)
	frames := &camera{body: io.NopCloser(strings.NewReader(
		"--prusaframe\r\nContent-Type: image/jpeg\r\n\r\n\xff\xd8\xff"))}

	viewer, offer := newViewer(t)

	answer, err := engine.Offer(t.Context(), offer, frames.opener())
	require.NoError(t, err, "the negotiation is answered; it is the relay that then finds the bytes wrong")
	require.NoError(t, viewer.SetRemoteDescription(
		pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}))

	require.Eventually(t, frames.isClosed, settle, 20*time.Millisecond,
		"the driver framed something that is not H.264 and the view stayed open")
}

// audioOnlyOffer is a well-formed offer with nothing this engine can fill. It
// is the shape a client sends by asking for the wrong kind of transceiver.
const audioOnlyOffer = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n" +
	"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\nc=IN IP4 0.0.0.0\r\na=recvonly\r\na=mid:0\r\n"

func newEngine(t *testing.T) *Engine { return newEngineWithTimeout(t, 0) }

func newEngineWithTimeout(t *testing.T, timeout time.Duration) *Engine {
	t.Helper()

	// Loopback only, here and on the viewer below. Gathering on every
	// interface binds a UDP socket on each one, and on Windows the firewall
	// then asks the person at the keyboard to allow the test binary every run.
	engine, err := New(t.Context(), Config{
		Logger:         discard(),
		ConnectTimeout: timeout,
		LoopbackOnly:   true,
	})
	require.NoError(t, err)

	return engine
}

// newViewer is the browser's half: one recvonly video transceiver, offered with
// its candidates already gathered.
func newViewer(t *testing.T) (*pion.PeerConnection, string) {
	t.Helper()

	settings := pion.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	keepLoopbackOnly(&settings)
	// pion asks for .local candidates over mDNS unless told not to, and that
	// opens a socket on port 5353 of every address, whatever the filters say.
	settings.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

	peer, err := pion.NewAPI(pion.WithSettingEngine(settings)).NewPeerConnection(pion.Configuration{})
	require.NoError(t, err)

	t.Cleanup(func() { _ = peer.Close() })

	_, err = peer.AddTransceiverFromKind(pion.RTPCodecTypeVideo,
		pion.RTPTransceiverInit{Direction: pion.RTPTransceiverDirectionRecvonly})
	require.NoError(t, err)

	offer, err := peer.CreateOffer(nil)
	require.NoError(t, err)

	gathered := pion.GatheringCompletePromise(peer)
	require.NoError(t, peer.SetLocalDescription(offer))

	select {
	case <-gathered:
	case <-time.After(settle):
		t.Fatal("the viewer never gathered a candidate")
	}

	return peer, peer.LocalDescription().SDP
}

// camera is a device producing one keyframe and then pictures for ever, at the
// cadence the case asks for. It reports whether the host closed it, which is
// what every leak case here reads.
type camera struct {
	body   io.ReadCloser
	closed atomic.Bool
	opened atomic.Bool
}

// newCamera is an endless H.264 stream. The pause between pictures is what
// makes it a live source rather than a file: a relay tested against a file
// reaches the end and stops for the wrong reason.
func newCamera(t *testing.T, cadence time.Duration) *camera {
	t.Helper()

	reader, writer := io.Pipe()
	stop := make(chan struct{})

	t.Cleanup(func() {
		close(stop)
		_ = writer.CloseWithError(io.EOF)
	})

	go func() {
		opening := annexB(unit(headerSPS, 6), unit(headerPPS, 4), unit(headerIDR, 900))
		if _, err := writer.Write(opening); err != nil {
			return
		}

		for {
			select {
			case <-stop:
				return
			case <-time.After(cadence):
			}

			if _, err := writer.Write(annexB(unit(headerNonIDR, 400))); err != nil {
				return
			}
		}
	}()

	return &camera{body: reader}
}

func (c *camera) Read(p []byte) (int, error) { return c.body.Read(p) }

func (c *camera) Close() error {
	c.closed.Store(true)

	return c.body.Close()
}

func (c *camera) isClosed() bool { return c.closed.Load() }

// opener is how the engine opens this camera. It records that it was asked.
func (c *camera) opener() Opener {
	return func(context.Context) (io.ReadCloser, error) {
		c.opened.Store(true)

		return c, nil
	}
}

func (c *camera) wasOpened() bool { return c.opened.Load() }

// discard is a logger nothing reads, so a case's output is its assertions.
func discard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
