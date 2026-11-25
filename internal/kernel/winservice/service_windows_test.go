//go:build windows

package winservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/svc"
)

func TestServiceReportsWhatTheServerReturned(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		code     int
		specific bool
	}{
		{name: "a clean exit is success", code: 0, specific: false},
		{name: "a restart is the server's own code", code: 3, specific: true},
		{name: "a failure is the server's own code", code: 1, specific: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := &handler{body: func(context.Context) int { return tc.code }}
			requests := make(chan svc.ChangeRequest)
			status := make(chan svc.Status, 4)

			specific, code := service.Execute(nil, requests, status)

			require.Equal(t, tc.specific, specific)
			require.Equal(t, uint32(tc.code), code)
			require.Equal(t, svc.StartPending, (<-status).State)
			require.Equal(t, svc.Running, (<-status).State)
		})
	}
}

func TestStopCancelsTheServerAndReportsSuccess(t *testing.T) {
	t.Parallel()

	for _, command := range []svc.Cmd{svc.Stop, svc.Shutdown} {
		cancelled := make(chan struct{})
		service := &handler{body: func(ctx context.Context) int {
			<-ctx.Done()
			close(cancelled)

			return 1
		}}
		requests := make(chan svc.ChangeRequest, 1)
		status := make(chan svc.Status, 4)
		requests <- svc.ChangeRequest{Cmd: command}

		specific, code := service.Execute(nil, requests, status)

		<-cancelled
		require.False(t, specific, "a requested stop never asks for a restart")
		require.Zero(t, code)
		require.Equal(t, 1, service.code, "the server's own code is kept for the caller")
		require.Equal(t, svc.StartPending, (<-status).State)
		require.Equal(t, svc.Running, (<-status).State)
		stopping := <-status
		require.Equal(t, svc.StopPending, stopping.State)
		require.Equal(t, uint32(StopWaitHint), stopping.WaitHint)
	}
}

func TestInterrogateAnswersTheCurrentStatus(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	service := &handler{body: func(context.Context) int {
		<-release

		return 0
	}}
	requests := make(chan svc.ChangeRequest, 1)
	status := make(chan svc.Status, 4)
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	requests <- svc.ChangeRequest{Cmd: svc.Interrogate, CurrentStatus: current}

	result := make(chan uint32, 1)
	go func() {
		_, code := service.Execute(nil, requests, status)
		result <- code
	}()

	require.Equal(t, svc.StartPending, (<-status).State)
	require.Equal(t, svc.Running, (<-status).State)
	require.Equal(t, current, <-status)

	close(release)
	require.Zero(t, <-result)
}
