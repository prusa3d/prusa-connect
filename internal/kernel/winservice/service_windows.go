//go:build windows

package winservice

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// Detect reports whether the service control manager started this process.
func Detect() (bool, error) {
	running, err := svc.IsWindowsService()
	if err != nil {
		return false, fmt.Errorf("ask Windows whether this is a service: %w", err)
	}

	return running, nil
}

// Run hands this process to the service control manager and runs body until
// it returns or the service is stopped. A stop cancels the context body gets.
// It returns the exit code body returned.
//
// The arguments the service control manager passes to Execute are the ones
// typed into `sc start`, not the command line the service was installed
// with. body reads os.Args, which holds the installed command line.
func Run(body func(context.Context) int) (int, error) {
	handler := &handler{body: body}
	if err := svc.Run(Name, handler); err != nil {
		return 0, fmt.Errorf("run as the %s service: %w", Name, err)
	}

	return handler.code, nil
}

// RedirectStandardError points standard error, for this process and every
// process it starts, at a file. The Go runtime writes a panic there. Without
// it the panic of a service is written nowhere.
func RedirectStandardError(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s for standard error: %w", path, err)
	}

	if err = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(file.Fd())); err != nil {
		_ = file.Close() // the error that matters is the one from SetStdHandle.

		return nil, fmt.Errorf("point standard error at %s: %w", path, err)
	}

	// Code that writes to os.Stderr holds the handle it had at start, which a
	// service does not have, so the variable follows the handle.
	os.Stderr = file //nolint:reassign // the process entry owns standard error, and sets it once before the core starts.

	return file, nil
}

type handler struct {
	body func(context.Context) int
	code int
}

// Execute is called by svc.Run on its own goroutine, and the service ends when
// it returns.
func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan int, 1)
	go func() { done <- h.body(ctx) }()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case h.code = <-done:
			return exitCode(h.code)
		case request := <-requests:
			if request.Cmd == svc.Interrogate {
				status <- request.CurrentStatus

				continue
			}

			// The service accepts Stop and Shutdown alone, and Windows
			// sends no request it did not accept.
			if request.Cmd != svc.Stop && request.Cmd != svc.Shutdown {
				continue
			}

			status <- svc.Status{State: svc.StopPending, WaitHint: StopWaitHint}
			cancel()
			h.code = <-done

			// A stop somebody asked for reports success whatever the server
			// returned. A failure code would make the service control
			// manager start the service again, against the person who
			// stopped it. The reason is in the log.
			return false, 0
		}
	}
}

// exitCode reports a code other than 0 as the service's own, so `sc query`
// shows the number the console exit would carry, and a recovery action
// configured for failures starts the service again.
func exitCode(code int) (bool, uint32) {
	if code == 0 {
		return false, 0
	}

	return true, uint32(code) //nolint:gosec // the server's exit codes are small and not negative.
}
