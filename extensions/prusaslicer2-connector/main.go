// Command prusaslicer2-connector lets PrusaSlicer 2 send a print to each printer
// of this server as it sends one to a printer with PrusaLink.
//
// A person adds a physical printer in PrusaSlicer with the host type
// PrusaLink, the address of one printer under this module's mount and an API
// credential of this server as the API key. The slicer then sends the PrusaLink
// requests to the module. The module answers them and does the work through
// the host: the host reads the printer and lands the file on it, started or
// not, and saves it in a storage first when the printer's Save to storage is
// on. The routes are in internal/routes and the answers in internal/link.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/accessapi"
	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/storageapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/prusaslicer2-connector/internal/link"
	"github.com/prusa3d/prusaslicer2-connector/internal/routes"
)

// What this module reports when the host asks it what it is. The version is
// the newest entry of CHANGELOG.md and the version module.yaml declares.
const (
	name    = "prusaslicer2-connector"
	release = extension.Version("0.1.2")
)

// openAPI is the fragment describing the routes, with every path relative to
// the mount.
//
//go:embed openapi.json
var openAPI []byte

// Settings is what an operator writes under this instance's config block.
// The module reads none.
type Settings struct {
	// Storage was where 0.1.0 kept an Upload. It is read and ignored, so a
	// config block written for 0.1.0 still starts the module: the host
	// refuses a key the settings do not name.
	Storage string `json:"storage,omitempty"`
}

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
		fleet:  fleet{devices: deviceapi.NewClient(conn)},
		access: access{client: accessapi.NewClient(conn)},
		files:  files{storages: storageapi.NewClient(conn)},
	}

	maps.Copy(serves, held.methods())

	fmt.Fprintf(os.Stderr, "%s %s is serving on stdio as pid %d\n", name, release, os.Getpid())

	if err = conn.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// module is everything this process holds once the host sent its settings.
type module struct {
	fleet  link.Fleet
	access link.Access
	files  link.Files

	mu   sync.Mutex
	link *link.Service
	// stopSweeping stops the timer that sweeps the print folder of the
	// service configured last.
	stopSweeping func()
}

// methods is the lifecycle set every module answers, plus the routes.
func (m *module) methods() wirerpc.Methods {
	handler := routes.New(routes.Host{
		Principal: routesapi.PrincipalOf,
		BodyFile:  routesapi.BodyFileOf,
	}, m.current)

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

			return nil, nil
		},
		wirerpc.MethodConfigure: wirerpc.ConfigureGranted(m.configure),
		routesapi.MethodServe:   routesapi.Serve(handler),
	}
}

// configure builds the service from the settings and the data folder.
func (m *module) configure(ctx context.Context, _ Settings, grants wirerpc.Grants) error {
	if grants.Data == "" {
		return fmt.Errorf("%s needs a data folder for the files it prints", name)
	}

	if err := os.MkdirAll(filepath.Join(grants.Data, "print"), 0o750); err != nil {
		return fmt.Errorf("make the print folder: %w", err)
	}

	service := link.New(link.Options{
		Fleet: m.fleet, Access: m.access, Files: m.files,
		Data: grants.Data, Version: string(release), Now: time.Now,
	})
	service.Sweep()

	// The sweeper outlives this call and ends at the next configure.
	sweeping, stopSweeping := context.WithCancel(context.WithoutCancel(ctx))
	go sweep(sweeping, service)

	m.mu.Lock()
	m.link = service
	stopPrevious := m.stopSweeping
	m.stopSweeping = stopSweeping
	m.mu.Unlock()

	if stopPrevious != nil {
		stopPrevious()
	}

	return nil
}

// sweep sweeps the print folder every link.SweepEvery until ctx ends.
func sweep(ctx context.Context, service *link.Service) {
	ticker := time.NewTicker(link.SweepEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.Sweep()
		}
	}
}

func (m *module) current() *link.Service {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.link
}

// stdio is the pipe the host started this process with.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }   //nolint:wrapcheck // the pipe itself.
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) } //nolint:wrapcheck // the pipe itself.
func (stdio) Close() error                { return os.Stdin.Close() }   //nolint:wrapcheck // the pipe itself.
