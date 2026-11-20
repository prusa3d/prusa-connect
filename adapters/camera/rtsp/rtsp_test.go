package rtsp_test

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/adapters/camera/rtsp"
	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
)

// The parameter sets the fake device declares, as a real camera's SDP carries
// them: base64 in sprop-parameter-sets.
var (
	//nolint:gochecknoglobals // fixture bytes; Go has no slice constant.
	deviceSPS = []byte{0x67, 0x42, 0xC0, 0x1E}
	//nolint:gochecknoglobals // fixture bytes.
	devicePPS = []byte{0x68, 0xCE, 0x3C, 0x80}
)

// A camera that challenges is answered, and the pictures it sends come back as
// an Annex-B stream whatever RTP shape it packed them in.
//
// The three shapes are the whole of RFC 6184 that a camera uses: several small
// units aggregated in one packet, one unit spread over several, and one unit on
// its own.
func TestTheStreamIsAnnexBWhateverShapeTheCameraPackedItIn(t *testing.T) {
	t.Parallel()

	device := serve(t, &fake{challenge: true, codec: "H264"})

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{
			"url":      device.url,
			"username": "operator",
			"password": "hunter2",
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, camera.Close()) })

	stream, err := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.NoError(t, err)

	t.Cleanup(func() { _ = stream.Body.Close() })

	require.Empty(t, stream.MediaType,
		"an elementary stream has no media type; nothing serves H.264 over HTTP directly")

	got := read(t, stream.Body, len(want()))
	require.Equal(t, want(), got)

	require.True(t, device.authorized.Load(),
		"the camera challenged and the driver never sent credentials")
}

// A camera that sends packets before it answers PLAY is played, and the packets
// that came first are dropped rather than read as the answer.
func TestPacketsBeforeThePlayAnswerAreSkipped(t *testing.T) {
	t.Parallel()

	device := serve(t, &fake{codec: "H264", early: true})

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": device.url},
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, camera.Close()) })

	stream, err := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.NoError(t, err)

	t.Cleanup(func() { _ = stream.Body.Close() })

	require.Equal(t, want(), read(t, stream.Body, len(want())))
}

// A device that describes H.265 is refused with a reason. Nothing here can
// convert it and Firefox does not play it, so a driver that accepted the
// session would produce a black player and no explanation.
func TestACameraThatDescribesH265IsRefusedWithAReason(t *testing.T) {
	t.Parallel()

	device := serve(t, &fake{codec: "H265"})

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": device.url},
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, camera.Close()) })

	_, err = camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.ErrorIs(t, err, rtsp.ErrNoH264)
	require.Contains(t, err.Error(), "H265", "the refusal does not say what the camera does emit")
}

// A camera that challenges and is given nothing is refused, rather than left
// retrying a handshake that cannot complete.
func TestACameraThatChallengesWithNoCredentialConfiguredIsRefused(t *testing.T) {
	t.Parallel()

	device := serve(t, &fake{challenge: true, codec: "H264"})

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": device.url},
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, camera.Close()) })

	_, err = camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.ErrorIs(t, err, rtsp.ErrUnauthorized)
}

// A viewer who stops watching gives the device connection back. One session per
// viewer is only bounded if the session ends when the viewer does.
func TestAViewerWhoStopsWatchingClosesTheDeviceConnection(t *testing.T) {
	t.Parallel()

	device := serve(t, &fake{codec: "H264"})

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": device.url},
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, camera.Close()) })

	stream, err := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.NoError(t, err)

	read(t, stream.Body, 8)

	require.NoError(t, stream.Body.Close())
	require.Eventually(t, func() bool { return device.closed.Load() > 0 }, 5*time.Second, 20*time.Millisecond,
		"the viewer went away and the device connection is still open")
}

// A camera that stops the instance ends every stream it opened, because the
// host stops an instance without knowing who is still reading from it.
func TestStoppingTheCameraEndsAStreamAViewerIsStillReading(t *testing.T) {
	t.Parallel()

	device := serve(t, &fake{codec: "H264", endless: true})

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": device.url},
	})
	require.NoError(t, err)

	stream, err := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.NoError(t, err)

	read(t, stream.Body, 8)

	require.NoError(t, camera.Close())

	done := make(chan error, 1)

	go func() {
		_, copyErr := io.Copy(io.Discard, stream.Body)
		done <- copyErr
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the camera stopped and a viewer is still reading from it")
	}
}

// A configuration this driver cannot dial is refused when the operator writes
// it, not when the first viewer arrives.
func TestAConfigurationThatCannotBeDialledIsRefusedAtBuild(t *testing.T) {
	t.Parallel()

	for name, config := range map[string]map[string]any{
		"noURL":        {},
		"httpURL":      {"url": "http://192.168.1.50/live"},
		"noHost":       {"url": "rtsp:///live"},
		"rtspSnapshot": {"url": "rtsp://192.168.1.50/live", "snapshot_url": "rtsp://192.168.1.50/snap"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := rtsp.New(cameraapi.ProviderSpec{ID: "watch", Type: rtsp.Type, Config: config})
			require.Error(t, err)
		})
	}
}

// The password never reaches a string. A driver rendered by %+v, which is what
// a supervisor error and a panic dump both do, must not print an operator's
// camera credential.
func TestTheCredentialNeverReachesAStringOrAnError(t *testing.T) {
	t.Parallel()

	const secret = "hunter2-do-not-print"

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{
			"url":      "rtsp://192.168.1.50/live",
			"username": "operator",
			"password": secret,
		},
	})
	require.NoError(t, err)

	require.NotContains(t, fmt.Sprintf("%+v", camera), secret)
	require.NotContains(t, fmt.Sprintf("%#v", camera), secret)

	// And the same for a credential written into the URL, which is the other
	// way an operator supplies one.
	fromURL, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": "rtsp://operator:" + secret + "@192.168.1.50/live"},
	})
	require.NoError(t, err)

	require.NotContains(t, fmt.Sprintf("%+v", fromURL), secret)

	// A refusal from an address nothing answers must not carry it either.
	_, err = fromURL.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.Error(t, err)
	require.NotContains(t, err.Error(), secret)
}

// want is the Annex-B the fake device's three packets decode to: the parameter
// sets, one keyframe, and one picture after it.
func want() []byte {
	var out []byte
	for _, unit := range [][]byte{
		deviceSPS, devicePPS, // replayed from the SDP, before the session's own
		deviceSPS, devicePPS, // the aggregated packet
		keyframe(), picture(),
	} {
		out = append(out, 0, 0, 0, 1)
		out = append(out, unit...)
	}

	return out
}

// keyframe is the unit the fake device fragments across three packets.
func keyframe() []byte {
	return append([]byte{0x65}, bytes.Repeat([]byte{0xA1}, 300)...)
}

// picture is the unit the fake device sends whole.
func picture() []byte {
	return append([]byte{0x41}, bytes.Repeat([]byte{0xB2}, 40)...)
}

// read takes exactly n bytes off the stream, failing rather than blocking when
// the device stops.
func read(t *testing.T, body io.Reader, n int) []byte {
	t.Helper()

	out := make([]byte, n)
	done := make(chan error, 1)

	go func() {
		_, err := io.ReadFull(body, out)
		done <- err
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the camera never delivered the stream")
	}

	return out
}

// fake is a camera that speaks enough RTSP to be dialled: the four methods this
// driver sends, an optional digest challenge, and three packets in the three
// payload shapes RFC 6184 defines.
type fake struct {
	challenge bool
	codec     string
	endless   bool
	// shortSession makes the device declare a one second session timeout, so
	// the driver's keepalive falls due inside a test rather than in thirty
	// seconds.
	shortSession bool
	// early makes the device send an RTCP report and an RTP packet before it
	// answers PLAY, as a Buddy3D's LIVE555 server does.
	early bool
	// stray makes the device send bytes that are neither a packet nor a
	// response between two packets, as a camera whose server names itself
	// rtsp_demo does every few seconds.
	stray bool
	// broken is how many malformed RTP packets the device sends each round:
	// the extension bit set and no extension after the header.
	broken int

	url        string
	authorized atomic.Bool
	closed     atomic.Int32
	keepalives atomic.Int32
}

// serve starts the fake device and returns it with the URL to dial.
func serve(t *testing.T, device *fake) *fake {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() { _ = listener.Close() })

	device.url = "rtsp://" + listener.Addr().String() + "/live"

	var running sync.WaitGroup

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}

			running.Go(func() {
				defer func() { device.closed.Add(1); _ = conn.Close() }()

				device.talk(conn)
			})
		}
	}()

	t.Cleanup(running.Wait)

	return device
}

// talk answers the handshake and then plays.
func (f *fake) talk(conn net.Conn) {
	reader := bufio.NewReader(conn)

	for {
		method, headers, err := readRequest(reader)
		if err != nil {
			return
		}

		seq := headers["cseq"]

		switch method {
		case "OPTIONS":
			write(conn, "200 OK", seq, map[string]string{
				"Public": "DESCRIBE, SETUP, PLAY, TEARDOWN",
			}, nil)
		case "DESCRIBE":
			if f.challenge && !f.checked(headers["authorization"]) {
				write(conn, "401 Unauthorized", seq, map[string]string{
					"WWW-Authenticate": `Digest realm="camera", nonce="abc123", qop="auth"`,
				}, nil)

				continue
			}

			write(conn, "200 OK", seq,
				map[string]string{"Content-Type": "application/sdp"}, []byte(f.sdp()))
		case "SETUP":
			write(conn, "200 OK", seq, map[string]string{
				"Session":   "12345678;timeout=" + f.sessionTimeout(),
				"Transport": "RTP/AVP/TCP;unicast;interleaved=0-1",
			}, nil)
		case "PLAY":
			if f.early {
				report := []byte{'$', 1, 0, 8, 0x80, 0xc8, 0, 6, 0, 0, 0, 1}
				_, _ = conn.Write(report)
				_ = interleave(conn, picture())
			}

			write(conn, "200 OK", seq, map[string]string{"Session": "12345678"}, nil)

			// The reader is handed over here and never read from again on
			// this goroutine. Two goroutines on one bufio.Reader is a race,
			// and it is the device's own bug rather than the driver's.
			go f.answerKeepalives(conn, reader)

			f.play(conn)

			return
		case "TEARDOWN":
			write(conn, "200 OK", seq, nil, nil)

			return
		default:
			write(conn, "501 Not Implemented", seq, nil, nil)
		}
	}
}

// sessionTimeout is what this device declares a session lives for.
func (f *fake) sessionTimeout() string {
	if f.shortSession {
		return "1"
	}

	return "60"
}

// checked answers the digest the driver sent, so a case that asserts the
// credential arrived is asserting a computed answer and not a header's presence.
func (f *fake) checked(authorization string) bool {
	if !strings.HasPrefix(authorization, "Digest ") {
		return false
	}

	fields := map[string]string{}

	for pair := range strings.SplitSeq(strings.TrimPrefix(authorization, "Digest "), ",") {
		key, value, found := strings.Cut(strings.TrimSpace(pair), "=")
		if found {
			fields[strings.ToLower(key)] = strings.Trim(value, `"`)
		}
	}

	ha1 := sum("operator:camera:hunter2")
	ha2 := sum("DESCRIBE:" + fields["uri"])
	want := sum(ha1 + ":abc123:" + fields["nc"] + ":" + fields["cnonce"] + ":auth:" + ha2)

	if fields["response"] != want {
		return false
	}

	f.authorized.Store(true)

	return true
}

// sdp is what this device describes.
func (f *fake) sdp() string {
	sets := base64.StdEncoding.EncodeToString(deviceSPS) + "," +
		base64.StdEncoding.EncodeToString(devicePPS)

	return "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=camera\r\nt=0 0\r\n" +
		"m=video 0 RTP/AVP 96\r\n" +
		"a=rtpmap:96 " + f.codec + "/90000\r\n" +
		"a=fmtp:96 packetization-mode=1;sprop-parameter-sets=" + sets + "\r\n" +
		"a=control:track1\r\n"
}

// play writes the three payload shapes and then either stops or repeats.
func (f *fake) play(conn net.Conn) {
	frame := keyframe()

	for {
		// One aggregated packet carrying both parameter sets.
		aggregate := []byte{0x18}
		for _, set := range [][]byte{deviceSPS, devicePPS} {
			aggregate = binary.BigEndian.AppendUint16(aggregate, uint16(len(set)))
			aggregate = append(aggregate, set...)
		}

		if !interleave(conn, aggregate) {
			return
		}

		for range f.broken {
			if !interleaveRaw(conn, []byte{0x90, 0x60, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1}) {
				return
			}
		}

		// The stray run holds a dollar sign on another channel, which is not a
		// packet of this session's either.
		if f.stray {
			if _, err := conn.Write([]byte{0x3f, 0xdf, '$', 0x07, 0x13, 0x00, 0xde}); err != nil {
				return
			}
		}

		// The keyframe over three fragments, which is what a camera does with
		// a unit larger than the path MTU.
		for at, chunk := 1, 100; at < len(frame); at += chunk {
			end := min(at+chunk, len(frame))

			header := byte(0)
			if at == 1 {
				header |= 0x80
			}

			if end == len(frame) {
				header |= 0x40
			}

			fragment := append([]byte{0x7C, header | (frame[0] & 0x1F)}, frame[at:end]...)
			if !interleave(conn, fragment) {
				return
			}
		}

		// One picture on its own, which is the common shape.
		if !interleave(conn, picture()) {
			return
		}

		if !f.endless {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// interleave writes one RTP packet inside the RTSP connection.
func interleave(conn net.Conn, payload []byte) bool {
	return interleaveRaw(conn, append([]byte{0x80, 0x60, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1}, payload...))
}

// interleaveRaw writes bytes as one packet, whatever they hold.
func interleaveRaw(conn net.Conn, packet []byte) bool {
	frame := []byte{'$', 0, 0, 0}
	binary.BigEndian.PutUint16(frame[2:], uint16(len(packet)))

	_, err := conn.Write(append(frame, packet...))

	return err == nil
}

// readRequest reads one RTSP message off the connection.
func readRequest(reader *bufio.Reader) (string, map[string]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", nil, err
	}

	method, _, _ := strings.Cut(strings.TrimSpace(line), " ")
	headers := map[string]string{}

	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			return "", nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return method, headers, nil
		}

		key, value, found := strings.Cut(line, ":")
		if found {
			headers[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
}

// write renders one RTSP response.
func write(conn net.Conn, status, seq string, headers map[string]string, body []byte) {
	var out strings.Builder

	out.WriteString("RTSP/1.0 " + status + "\r\nCSeq: " + seq + "\r\n")

	for key, value := range headers {
		out.WriteString(key + ": " + value + "\r\n")
	}

	if len(body) > 0 {
		out.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n")
	}

	out.WriteString("\r\n")

	_, _ = conn.Write(append([]byte(out.String()), body...))
}

func sum(value string) string {
	digest := md5.Sum([]byte(value))

	return hex.EncodeToString(digest[:])
}

// answerKeepalives replies to the requests that arrive while the device is
// playing. A camera answers on the same connection the media is interleaved on,
// which is what makes the reply land between two packets.
func (f *fake) answerKeepalives(conn net.Conn, reader *bufio.Reader) {
	for {
		_, headers, err := readRequest(reader)
		if err != nil {
			return
		}

		f.keepalives.Add(1)
		write(conn, "200 OK", headers["cseq"], map[string]string{"Session": "12345678"}, nil)
	}
}

// A camera that declares a session timeout closes a session nothing refreshes.
// The keepalive is what refreshes it, and it is written onto the connection the
// media is interleaved on, so a driver that read its answer inline would take
// the next packet and read it as an RTSP response.
func TestAStreamSurvivesTheKeepaliveOnTheMediaConnection(t *testing.T) {
	t.Parallel()

	device := serve(t, &fake{codec: "H264", endless: true, shortSession: true})

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": device.url},
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, camera.Close()) })

	stream, err := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.NoError(t, err)

	t.Cleanup(func() { _ = stream.Body.Close() })

	// The session declares one second, so the keepalive falls due at half of
	// that. Reading across three of them is reading across the failure.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		read(t, stream.Body, 64)
	}

	require.Positive(t, device.keepalives.Load(),
		"the session declared a timeout and the driver never refreshed it")
}

func TestAStreamSurvivesStrayBytesBetweenPackets(t *testing.T) {
	t.Parallel()

	device := serve(t, &fake{codec: "H264", endless: true, stray: true})

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": device.url},
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, camera.Close()) })

	stream, err := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.NoError(t, err)

	t.Cleanup(func() { _ = stream.Body.Close() })

	// Each round of the fake carries one stray run, so a few kilobytes is many
	// of them. The keyframe after each run still arrives whole.
	var seen []byte
	for len(seen) < 8<<10 {
		seen = append(seen, read(t, stream.Body, 512)...)
	}

	require.Greater(t, bytes.Count(seen, keyframe()), 2,
		"the keyframes after the stray bytes did not arrive whole")
}

func TestAStreamDropsAMalformedPacketAndGoesOn(t *testing.T) {
	t.Parallel()

	stream := openEndless(t, &fake{codec: "H264", endless: true, broken: 1})

	var seen []byte
	for len(seen) < 8<<10 {
		seen = append(seen, read(t, stream, 512)...)
	}

	require.Greater(t, bytes.Count(seen, keyframe()), 2,
		"the keyframes after the malformed packets did not arrive whole")
}

func TestAStreamOfOnlyMalformedPacketsEnds(t *testing.T) {
	t.Parallel()

	stream := openEndless(t, &fake{codec: "H264", endless: true, broken: 200})

	done := make(chan error, 1)

	go func() {
		_, err := io.Copy(io.Discard, stream)
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, rtsp.ErrHandshake)
	case <-time.After(10 * time.Second):
		t.Fatal("a camera that sends only malformed packets held the view open")
	}
}

// openEndless opens the stream of a fake device that keeps sending.
func openEndless(t *testing.T, device *fake) io.Reader {
	t.Helper()

	serve(t, device)

	camera, err := rtsp.New(cameraapi.ProviderSpec{
		ID: "watch", Type: rtsp.Type, Printer: "p1",
		Config: map[string]any{"url": device.url},
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, camera.Close()) })

	stream, err := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
	require.NoError(t, err)

	t.Cleanup(func() { _ = stream.Body.Close() })

	return stream.Body
}
