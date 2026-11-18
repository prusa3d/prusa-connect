// Command buddy-debug-logs receives the log lines and the metrics a Prusa printer
// sends when its Metrics & Log setting is on, keeps them for a day, and adds
// them to the diagnostics bundle.
//
// A printer sends both over UDP to one host, the log lines to one port and
// the metrics to another. This module listens on the two ports
// the operator granted it (ADR-20261019-3ns), matches each datagram to a printer by
// its MAC address, and keeps it in files in its data directory. The page
// under ui/ reads them through the routes below, and the diagnostics bundle
// asks for the newest of them through diagnostics.collect (ADR-20261019-nrq).
//
// # The routes
//
//	GET  /status                     the ports, what arrived, the room taken
//	GET  /sources                    every MAC that sent something, and its printer
//	PUT  /sources/{mac}              match a MAC to a printer by hand
//	GET  /logs                       one page of log lines, newest first
//	GET  /metrics/names              the metrics a printer sent
//	GET  /metrics/series             one metric as points for a chart
//	GET  /export                     one page of a download as text
//	POST /devices/{device_id}/setup  ask a printer to send, with M334
//	POST /data/purge                 remove the kept records, all or the older ones
//
// Each path is relative to the address the server gives this module, so a
// caller reaches the first one at /api/v1/modules/<namespace>/status.
//
// This file holds the handshake, the method set and the wiring. The parsing
// is in internal/datagram, the files in internal/store, the matching in
// internal/sources and the handlers in internal/routes.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/diagnosticsapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/buddy-debug-logs/internal/collect"
	"github.com/prusa3d/buddy-debug-logs/internal/query"
	"github.com/prusa3d/buddy-debug-logs/internal/receiver"
	"github.com/prusa3d/buddy-debug-logs/internal/routes"
	"github.com/prusa3d/buddy-debug-logs/internal/settings"
	"github.com/prusa3d/buddy-debug-logs/internal/sources"
	"github.com/prusa3d/buddy-debug-logs/internal/store"
)

// What this module reports when the host asks it what it is. The version is
// the newest entry of CHANGELOG.md and the version module.yaml declares, and
// changelog_test.go holds the three together.
const (
	name    = "buddy-debug-logs"
	release = extension.Version("0.1.10")
)

// The names module.yaml gives the two ports under needs.listen.
const (
	listenSyslog  = "syslog"
	listenMetrics = "metrics"
)

// How often the open segments reach the disk, and how often the retention
// runs. A printer sends many small records, so a sync per record would wear
// a Raspberry Pi's card.
const (
	syncEvery    = 5 * time.Second
	enforceEvery = time.Minute
)

// openAPI is the fragment describing the routes, with every path relative to
// the mount.
//
//go:embed openapi.json
var openAPI []byte

// Settings is what an operator writes under this instance's config block.
type Settings struct {
	RetentionHours int `json:"retention_hours"`
	MaxMiB         int `json:"max_mib"`
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

	// The method set is filled after the session exists, because this module
	// calls the host's device door and a client needs the session to call on.
	serves := wirerpc.Methods{}

	conn, err := wirerpc.NewConn(stdio{}, wirerpc.Options{Role: wirerpc.RoleChild, Methods: serves})
	if err != nil {
		return fmt.Errorf("build the session: %w", err)
	}

	held := &module{devices: deviceapi.NewClient(conn), now: time.Now}
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

// fleet is the part of the host's device door this module calls.
type fleet interface {
	routes.Devices
}

// module is everything this process holds once the host sent its settings.
type module struct {
	devices fleet
	now     func() time.Time

	mu     sync.Mutex
	state  routes.State
	cancel context.CancelFunc
	done   []<-chan struct{}
	store  *store.Store
	work   sync.WaitGroup
}

// methods is the lifecycle set every module answers, plus the routes and the
// diagnostics this module serves.
func (m *module) methods() wirerpc.Methods {
	return wirerpc.Methods{
		wirerpc.MethodInfo: func(context.Context, wirerpc.Call) (any, error) {
			return routesapi.Describe(wirerpc.Info{
				Name:     name,
				Version:  release,
				Contract: wirerpc.Contract,
				Serves:   []wirerpc.Method{wirerpc.MethodConfigure, diagnosticsapi.MethodCollect},
			}, openAPI)
		},
		wirerpc.MethodShutdown: func(context.Context, wirerpc.Call) (any, error) {
			fmt.Fprintf(os.Stderr, "%s was asked to stop\n", name)
			m.stop()

			return nil, nil
		},
		// The settings and the grants. A setting out of range refuses the
		// start, and the operator reads the reason on the instance. A port
		// that cannot be opened does not: the module still serves what it
		// kept, and the status says why the port is closed.
		wirerpc.MethodConfigure: wirerpc.ConfigureGranted(m.configure),
		routesapi.MethodServe:   routesapi.Serve(routes.New(routes.Service{State: m.current, Devices: m.devices})),
		diagnosticsapi.MethodCollect: diagnosticsapi.Serve(
			func(_ context.Context, cursor string) (diagnosticsapi.Page, error) {
				return m.collect(cursor)
			},
		),
	}
}

// configure applies the settings and opens what the grants allow.
func (m *module) configure(ctx context.Context, sent Settings, grants wirerpc.Grants) error {
	values, err := settings.Values{RetentionHours: sent.RetentionHours, MaxMiB: sent.MaxMiB}.Checked()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// The host configures a child once, after it starts it. A second call
	// would open the ports twice.
	if m.state.Ready {
		return errors.New("buddy-debug-logs: the settings were sent twice")
	}

	state := routes.State{Ready: true, Settings: values, Ports: grantedPorts(grants)}

	if grants.Data == "" {
		state.Problem = "the operator granted no data directory, so this module keeps nothing. " +
			"Install it again with approve."
		m.state = state

		return nil
	}

	held, err := store.Open(grants.Data, m.now, values.Limits())
	if err != nil {
		return fmt.Errorf("buddy-debug-logs: open the data directory: %w", err)
	}

	table, err := sources.Open(grants.Data, m.devices, m.now)
	if err != nil {
		_ = held.Close() // the start fails on the first error, which is the one returned

		return fmt.Errorf("buddy-debug-logs: %w", err)
	}

	m.store = held
	state.Book = query.Book{Store: held, Sources: table, Now: m.now}
	state.Receiver = receiver.New(held, table, m.now)

	// The background work outlives this call and ends when the host stops the
	// module, so it runs under a context of its own.
	background, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.cancel = cancel

	m.listen(background, state, grants)
	m.maintain(background, held, table)

	// The port a socket holds is the one a printer is told. It is the
	// granted one, except where a grant of 0 let the system pick, as a test
	// does.
	for stream, status := range state.Receiver.Status() {
		if status.Listener.Listening {
			state.Ports[stream] = status.Listener.Port
		}
	}

	m.state = state

	return nil
}

// listen opens each granted port. A stream granted none is reported in the
// status and nothing listens for it.
func (m *module) listen(ctx context.Context, state routes.State, grants wirerpc.Grants) {
	for stream, port := range map[store.Stream]string{store.Syslog: listenSyslog, store.Metrics: listenMetrics} {
		grant, granted := grants.Listen[port]
		if !granted {
			state.Receiver.Refuse(stream, "the operator granted no port for "+string(stream)+
				". Install the module again with approve_listen.")

			continue
		}

		done, err := state.Receiver.Listen(ctx, stream, grant.Network, grant.Port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s cannot listen for %s: %v\n", name, stream, err)

			continue
		}

		m.done = append(m.done, done)
	}
}

// maintain reads the fleet, syncs the segments and applies the retention,
// until ctx ends.
func (m *module) maintain(ctx context.Context, held *store.Store, table *sources.Table) {
	m.work.Go(func() {
		last := ""

		table.Run(ctx, func(err error) {
			// Once per new reason, so a host that is away does not fill the log.
			if err.Error() != last {
				last = err.Error()
				fmt.Fprintf(os.Stderr, "%s cannot read the printers: %v\n", name, err)
			}
		})
	})

	m.work.Go(func() {
		syncing := time.NewTicker(syncEvery)
		enforcing := time.NewTicker(enforceEvery)

		defer syncing.Stop()
		defer enforcing.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-syncing.C:
				// A failed sync is a full or failing disk, which the next
				// append reports as a dropped record in the status.
				_ = held.Sync()
			case <-enforcing.C:
				held.Enforce()
			}
		}
	})
}

// current is the state the routes read.
func (m *module) current() routes.State {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.state
}

// collect answers one page of the diagnostics bundle.
func (m *module) collect(cursor string) (diagnosticsapi.Page, error) {
	state := m.current()
	status := func() any { return routes.StatusOf(state) }

	if state.Book.Store == nil {
		if cursor != "" {
			return diagnosticsapi.Page{}, nil
		}

		return collect.StatusOnly(status)
	}

	// The bundle is built by an admin and carries every printer's records.
	book := state.Book
	book.Viewer = query.Everyone()

	return collect.Collector{Book: book, Status: status}.Page(cursor)
}

// stop closes the ports, waits for the background work and closes the
// segments. It runs once whichever of shutdown and the end of run comes
// first.
func (m *module) stop() {
	m.mu.Lock()
	cancel, done, held := m.cancel, m.done, m.store
	m.cancel, m.done, m.store = nil, nil, nil
	m.mu.Unlock()

	if cancel == nil {
		return
	}

	cancel()

	for _, closed := range done {
		<-closed
	}

	m.work.Wait()

	if err := held.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "%s could not close its files: %v\n", name, err)
	}
}

// grantedPorts is the port each stream was granted.
func grantedPorts(grants wirerpc.Grants) map[store.Stream]int {
	out := map[store.Stream]int{}

	if grant, ok := grants.Listen[listenSyslog]; ok {
		out[store.Syslog] = grant.Port
	}

	if grant, ok := grants.Listen[listenMetrics]; ok {
		out[store.Metrics] = grant.Port
	}

	return out
}

// stdio is the pair the host handed over. Close shuts the reading half, which
// is what ends a session whose host has gone quiet.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }
