// Command prusaslicer3-connector lets PrusaSlicer 3 use this server the way it uses
// Prusa Account and Prusa Connect.
//
// PrusaSlicer has an offline mode in which it sends the requests it sends to
// the cloud to this module instead. The module answers them with the cloud's
// bodies, and does the work through the host: the host lists the printers and
// prints the file (ADR-20261112-2eb). The slicer gets its credential from the
// core's app pairing (ADR-20261112-nay). The module never calls Prusa Account
// or Prusa Connect.
//
// The routes are in internal/routes, the Prusa Account me reply in
// internal/account and the Prusa Connect replies in internal/connect.
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
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/prusaslicer3-connector/internal/account"
	"github.com/prusa3d/prusaslicer3-connector/internal/connect"
	"github.com/prusa3d/prusaslicer3-connector/internal/routes"
)

// What this module reports when the host asks it what it is. The version is
// the newest entry of CHANGELOG.md and the version module.yaml declares.
const (
	name    = "prusaslicer3-connector"
	release = extension.Version("0.1.2")
)

// openAPI is the fragment describing the routes, with every path relative to
// the mount.
//
//go:embed openapi.json
var openAPI []byte

// Settings is what an operator writes under this instance's config block.
// The module has no setting.
type Settings struct{}

// The staged route of the G-code upload, as module.yaml writes it, and the
// unit its limit is granted in.
const (
	uploadMethod = "PUT"
	uploadPath   = "/connect/app/teams/{team}/files/raw"
	bytesPerMiB  = 1 << 20
)

// maxUploadBytes bounds one G-code file: the limit the operator approved for
// the upload route. The host refuses a larger body before the module sees it,
// and the module refuses a larger stated size at registration, as the cloud
// does. A route the operator did not approve is not staged, and its body
// comes over the wire, held to the wire's bound.
func maxUploadBytes(grants wirerpc.Grants) int64 {
	if mib, approved := grants.StagedMiB(uploadMethod, uploadPath); approved {
		return int64(mib) * bytesPerMiB
	}

	return routesapi.MaxModuleBody
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
		people:      people{client: accessapi.NewClient(conn)},
		fleet:       fleet{devices: deviceapi.NewClient(conn)},
		printAccess: access{client: accessapi.NewClient(conn)},
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
	people      account.People
	fleet       connect.Fleet
	printAccess connect.Access

	mu      sync.Mutex
	connect *connect.Service
	// stopSweeping stops the timer that sweeps the print folder of the
	// connect service configured last.
	stopSweeping func()
}

// methods is the lifecycle set every module answers, plus the routes.
func (m *module) methods() wirerpc.Methods {
	handler := routes.New(routes.Host{
		Principal: routesapi.PrincipalOf,
		BodyFile:  routesapi.BodyFileOf,
		People:    m.people,
	}, routes.Services{Connect: m.currentConnect})

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

// configure builds the connect service from the settings and the data folder.
func (m *module) configure(ctx context.Context, _ Settings, grants wirerpc.Grants) error {
	if grants.Data == "" {
		return fmt.Errorf("%s needs a data folder for the files it prints", name)
	}

	if err := os.MkdirAll(filepath.Join(grants.Data, "print"), 0o750); err != nil {
		return fmt.Errorf("make the print folder: %w", err)
	}

	_, staged := grants.StagedMiB(uploadMethod, uploadPath)
	connectService := connect.New(connect.Options{
		Fleet: m.fleet, Access: m.printAccess, Data: grants.Data,
		MaxSize: maxUploadBytes(grants), Staged: staged, Now: time.Now,
	})
	connectService.Sweep()

	// The sweeper outlives this call and ends at the next configure.
	sweeping, stopSweeping := context.WithCancel(context.WithoutCancel(ctx))
	go connectService.Sweeper(sweeping)

	m.mu.Lock()
	m.connect = connectService
	stopPrevious := m.stopSweeping
	m.stopSweeping = stopSweeping
	m.mu.Unlock()

	if stopPrevious != nil {
		stopPrevious()
	}

	return nil
}

func (m *module) currentConnect() *connect.Service {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.connect
}

// stdio is the pipe the host started this process with.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }   //nolint:wrapcheck // the pipe itself.
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) } //nolint:wrapcheck // the pipe itself.
func (stdio) Close() error                { return os.Stdin.Close() }   //nolint:wrapcheck // the pipe itself.
