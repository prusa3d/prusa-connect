// Command simple-queue keeps a queue of print files for each printer and
// prints them one after another.
//
// A person puts files on a printer's queue and orders them. When the printer
// is free and a person says its bed is clear, the next file is sent and
// started. A printer that finished a print waits until somebody takes the
// print off the bed and marks the printer as ready, either on the printer or
// here, so a print never starts on a bed nobody cleared.
//
// # The routes
//
//	GET    /queues/summary                             every queue the caller may see, in short
//	GET    /queues                                     every queue the caller may see, in full
//	GET    /printers/{device}                          one printer's queue
//	POST   /printers/{device}/entries                  put a file on the queue
//	DELETE /printers/{device}/entries/{entry}          take a file off the queue
//	POST   /printers/{device}/entries/{entry}/move     move a file in the queue
//	POST   /printers/{device}/start                    print the next file now
//	POST   /printers/{device}/ready                    mark the printer as ready, or not
//	GET    /printers/{device}/history                  the ended prints
//	DELETE /printers/{device}/history                  forget the ended prints
//
// Each path is relative to the address the server gives this module, so a
// caller reaches the first one at /api/v1/modules/<namespace>/queues/summary.
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"sync"
	"time"

	_ "github.com/lib/pq"  // the Postgres driver, which sdk/moduledb leaves to the module
	_ "modernc.org/sqlite" // the SQLite driver, likewise

	"github.com/prusa3d/prusa-connect/sdk/accessapi"
	"github.com/prusa3d/prusa-connect/sdk/busapi"
	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/moduledb"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/storageapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/simple-queue/internal/queue"
	"github.com/prusa3d/simple-queue/internal/routes"
	"github.com/prusa3d/simple-queue/internal/runner"
	"github.com/prusa3d/simple-queue/internal/settings"
)

// What this module reports when the host asks it what it is. The version is
// the newest entry of CHANGELOG.md and the version module.yaml declares, and
// changelog_test.go holds the three together.
const (
	name    = "simple-queue"
	release = extension.Version("0.1.1")
)

// openAPI is the fragment describing the routes, with every path relative to
// the mount.
//
//go:embed openapi.json
var openAPI []byte

// migrations is the schema of the queues, one directory per engine, because
// the operator chooses the engine.
//
//go:embed migrations/sqlite/* migrations/postgres/*
var migrations embed.FS

// schema is what sdk/moduledb applies.
//
//nolint:gochecknoglobals // a fixed description of the embedded files.
var schema = moduledb.Schema{Migrations: migrations}

// errNoDatabase refuses a start without a database grant: the module has
// nowhere to keep a queue, and would refuse every request.
var errNoDatabase = errors.New("the operator granted this module no database. Approve its needs.database request")

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

	held := newModule(host{
		fleet:  deviceapi.NewClient(conn),
		access: accessapi.NewClient(conn),
		files:  storageapi.NewClient(conn),
		feed:   busFeed{conn: conn},
	}, func(format string, args ...any) { fmt.Fprintf(os.Stderr, format, args...) })
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

// host is the parts of the host's session the module calls. main builds it
// over the session, and a test over a fake host.
type host struct {
	fleet  runner.Fleet
	access runner.Access
	files  routes.Files
	feed   runner.Feed
}

// module is everything this process holds.
type module struct {
	host     host
	store    *queue.Store
	runner   *runner.Runner
	settings *settings.Live

	mu     sync.Mutex
	cancel context.CancelFunc
	// db is the module's own database once the host granted it, and state is
	// what the module reports of its schema.
	db    *sql.DB
	state *wirerpc.DatabaseState
}

// newModule builds the queue over the host. Nothing runs until the host sends
// the settings. log is where a line for the operator goes.
func newModule(over host, log func(format string, args ...any)) *module {
	live := &settings.Live{}
	// The store is handed its database when the host grants it, in configure.
	store := &queue.Store{}

	return &module{
		host:     over,
		store:    store,
		settings: live,
		runner:   runner.New(store, over.fleet, over.access, live.Get, time.Now, log),
	}
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
		wirerpc.MethodConfigure: wirerpc.ConfigureReporting(m.configure),
		routesapi.MethodServe: routesapi.Serve(routes.New(routes.Service{
			Ready:    m.settings.Ready,
			Settings: m.settings.Get,
			Store:    m.store,
			Runner:   m.runner,
			Access:   m.host.access,
			Files:    m.host.files,
			Now:      time.Now,
		})),
	}
}

// configure opens and migrates the database the first time, applies the
// settings, and starts the queue. A setting out of range is refused, and so
// is a start with no database. The operator reads the reason on the instance.
//
// The database is opened here, so the module reports ready only on a schema
// it has applied.
func (m *module) configure(
	ctx context.Context, sent settings.Sent, grants wirerpc.Grants,
) (wirerpc.Configured, error) {
	values, err := settings.Read(sent)
	if err != nil {
		return wirerpc.Configured{}, err //nolint:wrapcheck // the reason the operator reads.
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		if grants.Database == nil {
			return wirerpc.Configured{}, errNoDatabase
		}

		db, report, openErr := moduledb.Open(ctx, *grants.Database, schema)
		if openErr != nil {
			// The cause first: the host shows this line where it may be cut
			// short. The grant's String leaves the password out.
			return wirerpc.Configured{}, fmt.Errorf("%w (the database is %s)", openErr, grants.Database)
		}

		m.db, m.state = db, report.State()
		m.store.Use(db, grants.Database.Engine)

		fmt.Fprintf(os.Stderr, "%s keeps its queues in %s at schema version %d\n",
			name, grants.Database, report.Version)
	}

	running := m.settings.Ready()
	m.settings.Set(values)

	if running {
		return wirerpc.Configured{Database: m.state}, nil
	}

	// The queue outlives this call and ends when the host stops the module,
	// so it runs under a context of its own.
	background, cancel := context.WithCancel(context.WithoutCancel(ctx))
	m.cancel = cancel

	// Run ends with the context's own error when the module stops, which is
	// nothing to tell anybody.
	go func() { _ = m.runner.Run(background, m.host.feed) }()

	return wirerpc.Configured{Database: m.state}, nil
}

// stop ends the queue. It runs once, whichever of shutdown and the end of run
// comes first.
func (m *module) stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// busFeed opens the host's event stream over the module's session.
type busFeed struct {
	conn *wirerpc.Conn
}

// Subscribe reads windows until the stream ends.
func (f busFeed) Subscribe(
	ctx context.Context, params busapi.SubscribeParams, onWindow func(busapi.Window) error,
) error {
	return busapi.Subscribe(ctx, f.conn, params, onWindow) //nolint:wrapcheck // busapi names the method.
}

// stdio is the pair the host handed over. Close shuts the reading half, which
// is what ends a session whose host has gone quiet.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }
