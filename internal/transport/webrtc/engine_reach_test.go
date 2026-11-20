package webrtc

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

// Two viewers on one fixed port both get pictures. The port is the one an
// operator publishes from a container, so every candidate in every answer has
// to name it, and the shared socket has to tell the two viewers apart.
func TestEveryViewerIsServedOnTheOneConfiguredPort(t *testing.T) {
	t.Parallel()

	port := freeUDPPort(t)
	engine := newReachableEngine(t, port, nil)

	for viewerNumber := range 2 {
		viewer, offer := newViewer(t)

		tracks := make(chan *pion.TrackRemote, 1)
		viewer.OnTrack(func(track *pion.TrackRemote, _ *pion.RTPReceiver) {
			select {
			case tracks <- track:
			default:
			}
		})

		answer, err := engine.Offer(t.Context(), offer, newCamera(t, 40*time.Millisecond).opener())
		require.NoError(t, err)

		gathered := candidatesOf(t, answer)
		require.NotEmpty(t, gathered, "viewer %d was answered with no candidate", viewerNumber)

		for _, candidate := range gathered {
			require.Equal(t, port, int(candidate.Port()),
				"viewer %d was offered %s, off the configured port", viewerNumber, candidate)
		}

		require.NoError(t, viewer.SetRemoteDescription(
			pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: answer}))

		select {
		case track := <-tracks:
			require.NoError(t, track.SetReadDeadline(time.Now().Add(settle)))

			_, _, err = track.ReadRTP()
			require.NoError(t, err, "viewer %d connected and received no picture", viewerNumber)
		case <-time.After(settle):
			t.Fatalf("viewer %d never connected through the shared port", viewerNumber)
		}
	}
}

// An advertised address is offered beside the addresses bound here, never in
// their place. A server in a container is reached at the address the operator
// names, and a browser on the same host still dials the bound one.
func TestAnAdvertisedAddressIsOfferedBesideTheBoundOnes(t *testing.T) {
	t.Parallel()

	const advertised = "192.0.2.10"

	port := freeUDPPort(t)
	engine := newReachableEngine(t, port, []string{advertised})

	_, offer := newViewer(t)

	answer, err := engine.Offer(t.Context(), offer, newCamera(t, time.Hour).opener())
	require.NoError(t, err)

	var sawAdvertised, sawBound bool

	for _, candidate := range candidatesOf(t, answer) {
		switch {
		case candidate.Addr().String() == advertised:
			sawAdvertised = true

			require.Equal(t, port, int(candidate.Port()),
				"the advertised address names a port the operator did not publish")
		case candidate.Addr().IsLoopback():
			sawBound = true
		}
	}

	require.True(t, sawAdvertised, "the answer never offers %s:\n%s", advertised, answer)
	require.True(t, sawBound, "the advertised address replaced the bound one:\n%s", answer)
}

// Close gives the port back, so a restart in the same process binds it again.
func TestClosingTheEngineReleasesTheConfiguredPort(t *testing.T) {
	t.Parallel()

	port := freeUDPPort(t)

	engine, err := New(t.Context(), Config{Logger: discard(), LoopbackOnly: true, UDPPort: port})
	require.NoError(t, err)
	require.NoError(t, engine.Close())

	again, err := net.ListenPacket("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	require.NoError(t, err, "the port is still held after Close")
	require.NoError(t, again.Close())
}

// A port another program holds is refused when the engine is built, so the
// server stops at startup and does not answer viewers it can never serve.
func TestAPortAlreadyInUseIsRefusedAtStartup(t *testing.T) {
	t.Parallel()

	held, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)

	t.Cleanup(func() { _ = held.Close() })

	port := held.LocalAddr().(*net.UDPAddr).Port //nolint:forcetypeassert // a udp4 listener has a UDP address.

	_, err = New(t.Context(), Config{Logger: discard(), LoopbackOnly: true, UDPPort: port})
	require.ErrorContains(t, err, strconv.Itoa(port))
}

func newReachableEngine(t *testing.T, port int, addresses []string) *Engine {
	t.Helper()

	engine, err := New(t.Context(), Config{
		Logger:       discard(),
		LoopbackOnly: true,
		UDPPort:      port,
		Addresses:    addresses,
	})
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, engine.Close()) })

	return engine
}

// freeUDPPort is a loopback port nothing held a moment ago. Another process
// could take it before the engine binds it, and the case then fails on
// startup, loudly, rather than passing on the wrong port.
func freeUDPPort(t *testing.T) int {
	t.Helper()

	probe, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)

	port := probe.LocalAddr().(*net.UDPAddr).Port //nolint:forcetypeassert // a udp4 listener has a UDP address.
	require.NoError(t, probe.Close())

	return port
}

// candidatesOf reads the address and port of every candidate line in an SDP.
func candidatesOf(t *testing.T, sdp string) []netip.AddrPort {
	t.Helper()

	var found []netip.AddrPort

	for line := range strings.SplitSeq(sdp, "\n") {
		if !strings.Contains(line, "candidate:") {
			continue
		}

		parts := strings.Fields(strings.TrimSpace(line))
		require.GreaterOrEqual(t, len(parts), 6, "a candidate this test cannot read: %s", line)

		address, err := netip.ParseAddr(parts[4])
		require.NoError(t, err, "a candidate names something that is not an address: %s", line)

		port, err := strconv.ParseUint(parts[5], 10, 16)
		require.NoError(t, err, "a candidate names something that is not a port: %s", line)

		found = append(found, netip.AddrPortFrom(address, uint16(port)))
	}

	return found
}

// An advertised address with no fixed port is refused. pion would then pair
// it with a socket of its own choosing, the loopback one among them, and a
// reply sent from there never reaches the browser.
func TestAnAdvertisedAddressWithNoFixedPortIsRefused(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), Config{Logger: discard(), LoopbackOnly: true, Addresses: []string{"192.0.2.10"}})
	require.ErrorIs(t, err, ErrAddressesWithoutPort)
}
