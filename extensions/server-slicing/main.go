// Command server-slicing slices 3MF projects into G-code with the PrusaSlicer
// command line, and writes the G-code into one of the server's storages.
//
// A person or a program on the network asks for a job: a project in a storage,
// or one sent with the request, and where the G-code goes. A worker runs the
// slicer in a separate process with bounded time and memory, checks that it
// wrote a file, reads the print time, the filament and the objects out of that
// file, and writes it into the storage. An administrator chooses which
// PrusaSlicer versions are installed, from the public GitHub releases or from
// a manifest of builds they host. ADR-20261026-ydf holds the rules.
//
// # The routes
//
//	GET    /status                 whether the module can slice, and its bounds
//	GET    /slicers                the installed and the offered slicers
//	POST   /slicers/install        install one offered version (administrator)
//	DELETE /slicers/{version}      remove one installed version (administrator)
//	PUT    /slicers/default        choose the version a job gets (administrator)
//	GET    /jobs                   the caller's jobs, or every job for an administrator
//	POST   /jobs                   slice a project in a storage
//	POST   /jobs/upload            slice a project sent as the body
//	GET    /jobs/{id}              one job
//	POST   /jobs/{id}/cancel       stop one job
//
// Each path is relative to the address the server gives this module, so a
// caller reaches the first one at /api/v1/modules/<namespace>/status.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"runtime"
	"sync"

	"github.com/prusa3d/prusa-connect/sdk/accessapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/storageapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/server-slicing/internal/install"
	"github.com/prusa3d/server-slicing/internal/jobs"
	"github.com/prusa3d/server-slicing/internal/project"
	"github.com/prusa3d/server-slicing/internal/routes"
	"github.com/prusa3d/server-slicing/internal/runner"
	"github.com/prusa3d/server-slicing/internal/settings"
	"github.com/prusa3d/server-slicing/internal/slicers"
)

// What this module reports when the host asks it what it is. The version is
// the newest entry of CHANGELOG.md and the version module.yaml declares, and
// changelog_test.go holds the three together.
const (
	name    = "server-slicing"
	release = extension.Version("0.2.4")
)

// mebibyte is the unit the settings count memory in.
const mebibyte = 1 << 20

// openAPI is the fragment describing the routes, with every path relative to
// the mount.
//
//go:embed openapi.json
var openAPI []byte

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
}

// run writes the handshake line and then serves frames until the host closes
// the channel.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	line, err := wire.Handshake{
		Protocol: wire.ProtocolMajor,
		MinMinor: wire.MinMinor,
		MaxMinor: wire.MaxMinor,
		Address:  wire.StdioAddress,
	}.Line()
	if err != nil {
		return fmt.Errorf("render the handshake: %w", err)
	}

	if _, err = os.Stdout.WriteString(line); err != nil {
		return fmt.Errorf("write the handshake: %w", err)
	}

	serves := wirerpc.Methods{}

	conn, err := wirerpc.NewConn(stdio{}, wirerpc.Options{Role: wirerpc.RoleChild, Methods: serves})
	if err != nil {
		return fmt.Errorf("build the session: %w", err)
	}

	held := &module{
		files:  storageapi.NewClient(conn),
		access: accessapi.NewClient(conn),
		log:    slog.New(slog.NewTextHandler(os.Stderr, nil)).With("module", name),
	}
	defer held.stop()

	maps.Copy(serves, held.methods())

	// The pid is here because the host's log is the only place a person sees
	// this process. Nothing on the wire carries it.
	fmt.Fprintf(os.Stderr, "%s %s is serving on stdio as pid %d\n", name, release, os.Getpid())

	if err = conn.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// module is everything this process holds once the host sent its settings.
type module struct {
	files  jobs.Files
	access jobs.Access
	log    *slog.Logger

	mu     sync.Mutex
	state  routes.State
	cancel context.CancelFunc
}

// methods is the lifecycle set every module answers, plus the routes.
func (m *module) methods() wirerpc.Methods {
	return wirerpc.Methods{
		wirerpc.MethodInfo: func(context.Context, wirerpc.Call) (any, error) {
			return routesapi.Describe(wirerpc.Info{
				Name:     name,
				Version:  release,
				Contract: wirerpc.Contract,
				Serves:   []wirerpc.Method{wirerpc.MethodConfigure},
			}, openAPI)
		},
		wirerpc.MethodShutdown: func(context.Context, wirerpc.Call) (any, error) {
			fmt.Fprintf(os.Stderr, "%s was asked to stop\n", name)
			m.stop()

			return nil, nil
		},
		wirerpc.MethodConfigure: wirerpc.ConfigureGranted(m.configure),
		routesapi.MethodServe:   routesapi.Serve(routes.New(routes.Service{State: m.current, Access: m.access})),
	}
}

// configure applies the settings and starts the workers. A setting out of
// range refuses the start, and the operator reads the reason on the
// instance.
func (m *module) configure(ctx context.Context, sent settings.Settings, grants wirerpc.Grants) error {
	values, err := sent.Checked()
	if err != nil {
		return err
	}

	budget, held := wire.MemoryBudget()
	runMemory := settings.RunMemoryBytes(budget, held, values.MaxJobs)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Ready {
		return errors.New("server-slicing: the settings were sent twice")
	}

	limits := routes.Limits{
		MaxJobs:        values.MaxJobs,
		MaxQueued:      values.MaxQueued,
		TimeoutMinutes: int(values.Timeout.Minutes()),
		MemoryMiB:      runMemory / mebibyte,
		Threads:        values.Threads,
		MaxProjectMiB:  values.MaxInputBytes / mebibyte,
		MaxUploadBytes: routes.MaxUploadBytes,
	}

	if grants.Data == "" {
		m.state = routes.State{
			Ready: true, Limits: limits,
			Problem: "the operator granted no data directory, so this module cannot install a slicer or slice. " +
				"Install it again with approve.",
		}

		return nil
	}

	// The workers and the installs outlive this call and end when the host
	// stops the module, so they run under a context of their own.
	background, cancel := context.WithCancel(context.WithoutCancel(ctx))

	service, err := slicers.Open(slicers.Config{
		Dir: grants.Data, OS: runtime.GOOS, Arch: runtime.GOARCH, GitHub: values.GitHubReleases,
		GitHubToken: values.GitHubToken,
		ManifestURL: values.ManifestURL, LocalBinaries: values.LocalBinaries,
		Client: install.NewHTTPClient(), Log: m.log,
	})
	if err != nil {
		cancel()

		return fmt.Errorf("server-slicing: %w", err)
	}

	go service.Load(background)
	go service.Keep(background)

	manager, err := jobs.New(grants.Data, jobs.Deps{
		Files: m.files, Access: m.access, Slicers: service, Preparer: project.ThreeMF{},
		Run: runner.Run, Log: m.log,
	}, jobs.Limits{
		Workers: values.MaxJobs, MaxQueued: values.MaxQueued, Keep: values.KeepJobs,
		Timeout: values.Timeout, MemoryBytes: runMemory, Threads: values.Threads,
		MaxInputBytes: values.MaxInputBytes,
	})
	if err != nil {
		cancel()

		return fmt.Errorf("server-slicing: %w", err)
	}

	manager.Start(background)

	m.cancel = func() {
		cancel()
		manager.Stop()
	}

	m.state = routes.State{
		Ready: true, Jobs: manager, Slicers: service, Limits: limits, Background: background,
		Selections: true,
	}

	return nil
}

// current is the state the routes read.
func (m *module) current() routes.State {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.state
}

// stop ends the workers and the installs. It runs once, whichever of shutdown
// and the end of run comes first.
func (m *module) stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// stdio is the pair the host handed over. Close shuts the reading half, which
// is what ends a session whose host has gone quiet.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }
