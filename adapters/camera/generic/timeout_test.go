package generic_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prusa3d/prusa-connect/adapters/camera/generic"
	"github.com/prusa3d/prusa-connect/adapters/camera/rtsp"
	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
)

// readFailure is what a handshake that waited past its timeout reports.
const readFailure = "read from the camera: read"

// pastTheDefault is longer than the 5s default. A source behind go2rtc that
// starts ffmpeg before it answers DESCRIBE takes about this long.
const pastTheDefault = 5500 * time.Millisecond

// A generic camera reads its RTSP stream with the timeout it is given. A source
// that answers DESCRIBE later than the 5s default is waited for when the
// timeout is longer, and is abandoned at the timeout when it is shorter.
//
// The cases that wait past the default take about five seconds each. They run
// in parallel, so the test takes about as long as the slowest one.
func TestAGenericCameraWaitsForASlowRTSPSourceAsLongAsItIsTold(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		delay   time.Duration
		timeout string
		// opens is whether the session is set up. A case that does not open
		// fails within [least, most) of the start.
		opens       bool
		least, most time.Duration
	}{
		"a timeout longer than the answer waits for it": {
			delay: 400 * time.Millisecond, timeout: "3s", opens: true,
			least: 400 * time.Millisecond,
		},
		"a timeout longer than a go2rtc answer waits for it": {
			delay: pastTheDefault, timeout: "8s", opens: true,
			least: pastTheDefault,
		},
		"no timeout gives up at the 5s default, as before": {
			delay: pastTheDefault, opens: false,
			least: 5 * time.Second, most: pastTheDefault,
		},
		"a timeout shorter than the answer gives up at the timeout": {
			delay: 2 * time.Second, timeout: "200ms", opens: false,
			least: 200 * time.Millisecond, most: time.Second,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			device := slowSource(t, tc.delay)

			config := map[string]any{"stream_url": device}
			if tc.timeout != "" {
				config["timeout"] = tc.timeout
			}

			camera := build(t, config)

			began := time.Now()
			stream, err := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
			took := time.Since(began)

			if tc.opens {
				if err != nil {
					t.Fatalf("after %s: %v", took, err)
				}

				_ = stream.Body.Close()

				if took < tc.least {
					t.Fatalf("opened after %s, before the source answered at %s", took, tc.least)
				}

				return
			}

			if err == nil {
				_ = stream.Body.Close()
				t.Fatalf("opened after %s, past a timeout that should have ended it", took)
			}

			if !strings.Contains(err.Error(), readFailure) {
				t.Fatalf("got %q, want the error a handshake past its timeout gives: %q", err, readFailure)
			}

			if took < tc.least || took >= tc.most {
				t.Fatalf("gave up after %s, want between %s and %s", took, tc.least, tc.most)
			}
		})
	}
}

// A timeout the rtsp driver does not take leaves its 5s default in place, and
// the generic camera gives it the same value. Each value is tried on both
// drivers against a source that answers in 5.5s, so both must give up at about
// 5s with the same error.
func TestAGenericCameraReadsATimeoutAsTheRTSPDriverDoes(t *testing.T) {
	t.Parallel()

	for _, written := range []any{"soon", "0s", "-3s", "", 7} {
		t.Run(strconv.Quote(fmt.Sprint(written)), func(t *testing.T) {
			t.Parallel()

			var (
				wait   sync.WaitGroup
				errs   [2]error
				tooks  [2]time.Duration
				builds = [2]func(string) (cameraapi.Camera, error){
					func(address string) (cameraapi.Camera, error) {
						return generic.New(cameraapi.ProviderSpec{ID: "front", Type: generic.Type,
							Config: map[string]any{"stream_url": address, "timeout": written}})
					},
					func(address string) (cameraapi.Camera, error) {
						return rtsp.New(cameraapi.ProviderSpec{ID: "front", Type: rtsp.Type,
							Config: map[string]any{"url": address, "timeout": written}})
					},
				}
			)

			for i, newCamera := range builds {
				camera, err := newCamera(slowSource(t, pastTheDefault))
				if err != nil {
					t.Fatal(err)
				}

				t.Cleanup(func() { _ = camera.Close() })

				wait.Go(func() {
					began := time.Now()

					stream, openErr := camera.Open(t.Context(), cameraapi.FormatH264AnnexB)
					if openErr == nil {
						_ = stream.Body.Close()
					}

					errs[i], tooks[i] = openErr, time.Since(began)
				})
			}

			wait.Wait()

			for i, driver := range []string{"generic", "rtsp"} {
				if errs[i] == nil || !strings.Contains(errs[i].Error(), readFailure) {
					t.Errorf("%s: got %v after %s, want %q", driver, errs[i], tooks[i], readFailure)
				}

				if tooks[i] < 5*time.Second || tooks[i] >= pastTheDefault {
					t.Errorf("%s: gave up after %s, want the 5s default", driver, tooks[i])
				}
			}
		})
	}
}

// The timeout also bounds one fetch from snapshot_url, as it does on the mjpeg
// driver the generic camera fetches through. A value the rtsp driver ignores
// keeps the default here too, where the mjpeg driver on its own would end every
// fetch at once.
func TestAGenericCameraFetchesItsSnapshotWithTheSameTimeout(t *testing.T) {
	t.Parallel()

	device := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(300 * time.Millisecond):
		case <-r.Context().Done():
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpeg)
	}))
	t.Cleanup(device.Close)

	for timeout, fetched := range map[string]bool{"100ms": false, "2s": true, "0s": true, "": true} {
		t.Run(strconv.Quote(timeout), func(t *testing.T) {
			t.Parallel()

			config := map[string]any{"snapshot_url": device.URL + "/snap.jpg"}
			if timeout != "" {
				config["timeout"] = timeout
			}

			_, err := build(t, config).Snapshot(t.Context())
			if fetched && err != nil {
				t.Fatalf("a fetch the timeout allows: %v", err)
			}

			if !fetched && err == nil {
				t.Fatal("a fetch longer than the timeout was waited for")
			}
		})
	}
}

// slowSource is an RTSP device that answers OPTIONS at once and DESCRIBE after
// delay, as go2rtc does while ffmpeg starts. It returns the address to dial.
func slowSource(t *testing.T, delay time.Duration) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	var running sync.WaitGroup

	t.Cleanup(func() {
		close(done)
		_ = listener.Close()
		running.Wait()
	})

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}

			running.Go(func() {
				defer func() { _ = conn.Close() }()

				answerSlowly(conn, delay, done)
			})
		}
	}()

	return "rtsp://" + listener.Addr().String() + "/live"
}

// answerSlowly answers the handshake on one connection, DESCRIBE after delay.
func answerSlowly(conn net.Conn, delay time.Duration, done <-chan struct{}) {
	go func() {
		<-done
		_ = conn.Close()
	}()

	reader := bufio.NewReader(conn)

	for {
		method, seq, err := request(reader)
		if err != nil {
			return
		}

		switch method {
		case "OPTIONS":
			respond(conn, seq, map[string]string{"Public": "DESCRIBE, SETUP, PLAY, TEARDOWN"}, "")
		case "DESCRIBE":
			select {
			case <-time.After(delay):
			case <-done:
				return
			}

			respond(conn, seq, map[string]string{"Content-Type": "application/sdp"},
				"v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=camera\r\nt=0 0\r\n"+
					"m=video 0 RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\n"+
					"a=fmtp:96 packetization-mode=1\r\na=control:track1\r\n")
		case "SETUP":
			respond(conn, seq, map[string]string{
				"Session":   "12345678;timeout=60",
				"Transport": "RTP/AVP/TCP;unicast;interleaved=0-1",
			}, "")
		default:
			respond(conn, seq, map[string]string{"Session": "12345678"}, "")
		}
	}
}

// request reads one RTSP request and returns its method and sequence number.
func request(reader *bufio.Reader) (method, seq string, err error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", "", err
	}

	method, _, _ = strings.Cut(strings.TrimSpace(line), " ")

	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			return "", "", err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			return method, seq, nil
		}

		if key, value, found := strings.Cut(line, ":"); found && strings.EqualFold(key, "CSeq") {
			seq = strings.TrimSpace(value)
		}
	}
}

// respond writes one 200 answer.
func respond(conn net.Conn, seq string, headers map[string]string, body string) {
	var out strings.Builder

	out.WriteString("RTSP/1.0 200 OK\r\nCSeq: " + seq + "\r\n")

	for key, value := range headers {
		out.WriteString(key + ": " + value + "\r\n")
	}

	if body != "" {
		out.WriteString("Content-Length: " + strconv.Itoa(len(body)) + "\r\n")
	}

	out.WriteString("\r\n" + body)

	_, _ = conn.Write([]byte(out.String()))
}
