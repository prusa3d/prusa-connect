package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/builtin"
	"github.com/prusa3d/prusa-connect/internal/app/resourcehost"
	"github.com/prusa3d/prusa-connect/internal/extension/supervisor"
	"github.com/prusa3d/prusa-connect/internal/kernel/condition"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/platform/devicestore"
)

// TestAGenericCameraWithATimeoutStarts is the timeout setting through the path
// a camera added over the API takes: the camera row, the real type table and
// the host's check of the block against the keys the type declares. A generic
// camera that writes timeout comes up, and one that misspells it fails with a
// refusal that names the key it wrote. The condition shortens the list of keys
// the type reads, so resourcehost's own case checks that timeout is in it.
//
// The stream is never dialled, because a generic camera opens it only for a
// viewer, so the address needs nothing on the other end.
func TestAGenericCameraWithATimeoutStarts(t *testing.T) {
	t.Parallel()

	latest := newStateLog()

	types, err := builtin.Registry()
	require.NoError(t, err)

	host, err := resourcehost.New(resourcehost.Config{
		HostKeys: builtin.HostKeys,
		Registry: types,
		Logger:   discard(),
		OnStatus: latest.observe,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	stopped := make(chan error, 1)

	go func() { stopped <- host.Run(ctx) }()

	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-stopped)
	})

	addCameras(t, newGraph(t), host,
		devicestore.Camera{ID: "slow", Type: "generic", Config: json.RawMessage(
			`{"printer":"p1","stream_url":"rtsp://192.0.2.1/live","timeout":"15s"}`)},
		devicestore.Camera{ID: "typo", Type: "generic", Config: json.RawMessage(
			`{"printer":"p1","stream_url":"rtsp://192.0.2.1/live","timeut":"15s"}`)},
	)

	slow, typo := cameraRef("slow"), cameraRef("typo")

	require.Eventually(t, func() bool {
		return latest.state(slow) == condition.StateReady()
	}, settle, 20*time.Millisecond, "a generic camera with a timeout is %v: %s",
		latest.state(slow), latest.said(slow))

	require.Eventually(t, func() bool {
		return latest.state(typo) == condition.StateFailed()
	}, settle, 20*time.Millisecond, "a generic camera with a misspelt timeout is %v", latest.state(typo))

	require.Contains(t, latest.said(typo), `"timeut"`, "the refusal names the key the operator wrote")
}

// stateLog keeps the last status each instance reported.
type stateLog struct {
	mu   sync.Mutex
	last map[ref.Ref]supervisor.Status
}

func newStateLog() *stateLog { return &stateLog{last: map[ref.Ref]supervisor.Status{}} }

func (l *stateLog) observe(status supervisor.Status) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.last[status.Ref] = status
}

func (l *stateLog) state(instance ref.Ref) condition.State {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.last[instance].State
}

// said is every message on the instance's last status.
func (l *stateLog) said(instance ref.Ref) string {
	l.mu.Lock()
	defer l.mu.Unlock()

	status := l.last[instance]
	messages := make([]string, 0, len(status.Conditions))

	for _, held := range status.Conditions {
		messages = append(messages, held.Message)
	}

	return strings.Join(messages, "\n")
}
