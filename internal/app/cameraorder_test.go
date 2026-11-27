package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/builtin"
	"github.com/prusa3d/prusa-connect/internal/app/resourcehost"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/extension/registry"
	"github.com/prusa3d/prusa-connect/internal/extension/supervised"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/webrtc"
)

const (
	// declaredCamera and boundPrinter are the one instance manyCameras declares
	// and the printer it names.
	declaredCamera = "watch-00"
	boundPrinter   = "pfixedprinter0000000000"

	// vera is a principal the graph names nowhere, which is what a caller with
	// no grant on any printer looks like to the media plane.
	vera = "user:vera"

	// absentCamera is a well-formed instance name nothing supervises.
	absentCamera = "no-such-camera"
)

// browserOffer is the shape a browser sends to open a live view. The media
// plane hands it to the relay without reading it, so what it holds only has to
// be an offer.
const browserOffer = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n" +
	"m=video 9 UDP/TLS/RTP/SAVPF 102\r\nc=IN IP4 0.0.0.0\r\na=recvonly\r\na=mid:0\r\n"

// countingGraph is the authorization graph with a tally of what was asked of
// it. A refusal that hides which of three reasons it had is only hidden if
// every one of the three costs the same work, and the count is how a case reads
// that.
type countingGraph struct {
	inner *authz.Store
	asked atomic.Int64
}

func (g *countingGraph) Visible(ctx context.Context, subject authz.Subject) (authz.Visibility, error) {
	g.asked.Add(1)

	return g.inner.Visible(ctx, subject)
}

// Can forwards the point check. It is not counted: the order this file reads is
// the order of the two set reads, and the point check is asked by one path only.
func (g *countingGraph) Can(
	ctx context.Context, subject authz.Subject, action authz.Action, object authz.Object,
) (bool, error) {
	return g.inner.Can(ctx, subject, action, object)
}

// countingRelay is the WebRTC engine with a tally of what reached it. A refusal
// the graph took is only the graph's if nothing downstream was asked, and the
// count is how a case reads that.
type countingRelay struct {
	offered atomic.Int64
}

func (r *countingRelay) Offer(_ context.Context, _ string, _ webrtc.Opener) (string, error) {
	r.offered.Add(1)

	return "v=0\r\n", nil
}

// TestACameraRefusalCostsTheSameGraphReadWhetherOrNotTheNameIsSupervised holds
// the order the media plane asks its two planes in.
//
// Every refusal here is already the same bytes, so what is left to read is how
// long it took. Resolving the instance first makes that a signal: a name this
// server supervises reaches the graph and a name it does not never gets there,
// which separates the two for a caller who may see neither. Asking the graph
// first collapses them, and this case fails against the other order because the
// unsupervised name would then cost no query at all.
//
// The exploitability of the gap is a hypothesis rather than a measurement.
// ADR-20260827-wgm rule 6 records a noise floor of 29.9 microseconds against 27.3 on
// SQLite, which is wider than the gap is likely to be, and nothing here claims
// otherwise. The order is held because the discipline is cheap, not because the
// leak was demonstrated.
func TestACameraRefusalCostsTheSameGraphReadWhetherOrNotTheNameIsSupervised(t *testing.T) {
	t.Parallel()

	device := newCameraDevice(t)
	graph := newGraph(t)
	counted := &countingGraph{inner: graph}
	media, host := mediaPlaneOn(t, graph, counted, device)

	requireSupervised(t, host)

	// A name nothing supervises. The graph is still asked, which is the whole
	// property: the query is a cost of asking, not a cost of the name existing.
	counted.asked.Store(0)

	_, err := media.Snapshot(t.Context(), vera, absentCamera)
	require.EqualError(t, err, httpapi.CameraNotFound(absentCamera).Error())
	require.Equal(t, int64(1), counted.asked.Load(),
		"a camera name nothing supervises was refused without reading the graph, so how long the "+
			"refusal took says whether this server has that camera")

	// The same refusal for a camera that is there, bound to a printer this
	// caller holds nothing on. One read each, so the two are the same work.
	counted.asked.Store(0)

	_, err = media.Snapshot(t.Context(), vera, declaredCamera)
	require.EqualError(t, err, httpapi.CameraNotFound(declaredCamera).Error())
	require.Equal(t, int64(1), counted.asked.Load())

	// The live view is the same decision through the same helper, so it answers
	// the same way for both names.
	for _, name := range []string{absentCamera, declaredCamera} {
		_, streamErr := media.Stream(t.Context(), vera, name)
		require.EqualError(t, streamErr, httpapi.CameraNotFound(name).Error())
	}

	// And so is the WebRTC view. It reads the graph through the same helper, so
	// a caller who holds nothing is refused before an offer is looked at and
	// before a relay is asked for anything.
	counted.asked.Store(0)

	for _, name := range []string{absentCamera, declaredCamera} {
		_, watchErr := media.Watch(t.Context(), vera, name, browserOffer)
		require.EqualError(t, watchErr, httpapi.CameraNotFound(name).Error())
	}

	require.Equal(t, int64(2), counted.asked.Load(),
		"a WebRTC refusal skipped the graph, so how long it took says whether this server has "+
			"that camera")
	require.Zero(t, media.relay.(*countingRelay).offered.Load(),
		"a caller the graph grants nothing reached the relay, so an offer they sent was acted on")

	// The other direction. The grant is written and the same call is served, so
	// the refusals above are the graph's answer and not an absent instance.
	require.NoError(t, graph.UpsertDevice(t.Context(), boundPrinter, "Bound"))
	require.NoError(t, graph.Write(t.Context(),
		tuple(t, authz.TypeDevice, boundPrinter, authz.RelOperator, vera)))

	require.Eventually(t, func() bool {
		shot, shotErr := media.Snapshot(t.Context(), vera, declaredCamera)

		return shotErr == nil && len(shot.Bytes) > 0
	}, settle, 20*time.Millisecond,
		"the printer was granted and the camera watching it still will not serve a frame")

	// And the name nothing supervises is refused for the granted caller too, in
	// the bytes it gave the caller who held nothing.
	_, err = media.Snapshot(t.Context(), vera, absentCamera)
	require.EqualError(t, err, httpapi.CameraNotFound(absentCamera).Error())

	// This camera emits MJPEG and no H.264, so the granted caller is refused a
	// WebRTC view in the bytes a caller who may not see it gets. Telling them
	// which format it does emit tells them the camera is there, and the relay is
	// never asked to answer an offer nothing can feed.
	_, err = media.Watch(t.Context(), vera, declaredCamera, browserOffer)
	require.EqualError(t, err, httpapi.CameraNotFound(declaredCamera).Error())
	require.Zero(t, media.relay.(*countingRelay).offered.Load(),
		"a camera that emits no H.264 reached the relay, so an offer was answered with nothing to play")
}

// TestTheCameraListingReadsTheGraphOnADeploymentWithNoCameras is the same order
// at the listing. A caller learns whether this server supervises any camera at
// all from whether the answer cost a query, and an empty list is the answer
// either way.
func TestTheCameraListingReadsTheGraphOnADeploymentWithNoCameras(t *testing.T) {
	t.Parallel()

	graph := newGraph(t)
	counted := &countingGraph{inner: graph}

	types, err := registry.New()
	require.NoError(t, err)

	host, err := resourcehost.New(resourcehost.Config{Registry: types, HostKeys: builtin.HostKeys, Logger: discard()})
	require.NoError(t, err)

	listed, err := newCameras(counted, host, nil, discard()).Cameras(t.Context(), vera)
	require.NoError(t, err)
	require.Empty(t, listed)
	require.Equal(t, int64(1), counted.asked.Load(),
		"a deployment supervising no camera answered without reading the graph, so the listing's "+
			"cost says whether this server has one")
}

// mediaPlaneOn is the media plane over a real supervision plane running the one
// camera manyCameras declares, reading whatever graph the case hands it.
func mediaPlaneOn(
	t *testing.T, store *authz.Store, seen cameraGraph, device *cameraDevice,
) (*cameras, *resourcehost.Host) {
	t.Helper()

	types, err := builtin.Registry()
	require.NoError(t, err)

	host, err := resourcehost.New(resourcehost.Config{Registry: types, HostKeys: builtin.HostKeys, Logger: discard()})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	stopped := make(chan error, 1)

	go func() { stopped <- host.Run(ctx) }()

	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-stopped)
	})

	addCameras(t, store, host, manyCameras(device, 1)...)

	return newCameras(seen, host, &countingRelay{}, discard()), host
}

// requireSupervised waits until the declared camera is serving and reports the
// printer it watches. Until then the refusals below would be a camera that has
// not started rather than a decision the graph took.
func requireSupervised(t *testing.T, host *resourcehost.Host) {
	t.Helper()

	instance, err := ref.New(ref.KindCamera, declaredCamera)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		runner, running := host.Instance(instance)
		if !running {
			return false
		}

		camera, attachErr := supervised.AttachCamera(runner)

		return attachErr == nil && camera.Printer() == boundPrinter
	}, settle, 20*time.Millisecond,
		"the declared camera never reported the printer it watches, so a refusal for it proves nothing")
}

// BlockedDevices passes through without counting. The tally above is a tally of
// the visibility query, which is the one a refusal's cost is read from; a
// second read this graph makes on every path alike distinguishes nothing.
func (g *countingGraph) BlockedDevices(
	ctx context.Context, subject authz.Subject,
) (map[string]bool, error) {
	return g.inner.BlockedDevices(ctx, subject)
}
