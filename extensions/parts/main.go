// Command parts keeps a private catalogue of parts for this server.
//
// A person makes folders, puts parts in them, and gives each part a name, a
// description, tags and comments. The files behind a part are its versions: a
// version is the set of files the part was at, with a note on what changed.
// A print file of a version is printed from the part's page.
//
// The module ships two programs. This one serves the routes below. The browser
// bundle under ui/ draws the Parts page on them.
//
// # The routes
//
//	GET    /overview                          the folder tree, the tags, the room left
//	POST   /folders                           make a folder
//	PATCH  /folders/{id}                      rename, move or describe a folder
//	DELETE /folders/{id}                      delete an empty folder
//	GET    /parts                             one page of the parts that match
//	POST   /parts                             add a part
//	GET    /parts/{id}                        one part, with its versions and comments
//	PATCH  /parts/{id}                        change a part
//	DELETE /parts/{id}                        delete a part
//	POST   /parts/{id}/versions               add a version
//	PATCH  /parts/{id}/versions/{number}      rewrite a version's note
//	DELETE /parts/{id}/versions/{number}      delete a version
//	POST   /parts/{id}/comments               write a comment
//	PATCH  /parts/{id}/comments/{comment}     rewrite your own comment
//	DELETE /parts/{id}/comments/{comment}     delete your own comment
//
// Each path is relative to the address the server gives this module, so a
// caller reaches the first one at /api/v1/modules/<namespace>/overview. The
// server decides who may call a route and then calls it. It takes
// view_resource for a read and operate_resource for a write.
//
// # Where things are kept
//
// The catalogue is in a database of the module's own (needs.database in
// module.yaml). The operator puts it in a SQLite file in the module's data
// directory, which is the default, or on a Postgres server. The module opens
// it and applies migrations/ in plugin.configure, before it reports ready.
// internal/catalog keeps a copy of the rows in memory while the program runs,
// read from the database at the first request.
//
// Releases up to 0.2.3 kept the catalogue in the server's record store. The
// first request of a newer release moves those records into the database,
// once.
//
// The files are in one of the server's own storages, which the settings name.
// The page uploads them through the core's storage routes, which take
// write_storage for the person, and this module records where they went after
// asking the storage that they are there. It never writes or deletes a file.
//
// This file holds the handshake, the method set and the wiring. The rules are
// in internal/catalog and the handlers in internal/routes.
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/signal"
	"sync"

	// The two drivers the catalogue's database may need. sdk/moduledb
	// imports none.
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/moduledb"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
	"github.com/prusa3d/prusa-connect/sdk/storageapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/parts/internal/catalog"
	"github.com/prusa3d/parts/internal/routes"
	"github.com/prusa3d/parts/internal/settings"
)

// What this module reports when the host asks it what it is. The version is
// the newest entry of CHANGELOG.md and the version module.yaml declares, and
// changelog_test.go holds the three together.
const (
	name    = "parts"
	release = extension.Version("0.3.8")
)

// openAPI is the fragment describing the routes, with every path relative to
// the mount. The host holds it to routesapi.ParseFragment at launch and serves
// it rebased under the namespace, which is what cmd/generate-client-sdk reads
// to type this module's surface.
//
//go:embed openapi.json
var openAPI []byte

// migrations is the catalogue's schema, one directory per engine, because the
// operator chooses the engine.
//
//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrations embed.FS

// schema is what sdk/moduledb applies. Compat stays at zero while every
// migration only adds: an older release of 0.3 still reads what a newer one
// wrote.
//
//nolint:gochecknoglobals // a fixed description of the embedded files.
var schema = moduledb.Schema{Migrations: migrations}

// errNoDatabase refuses a start without a database grant. The catalogue has
// nowhere to be kept, and a module that ran without it would refuse every
// request.
var errNoDatabase = errors.New("the operator granted this module no database. Approve its needs.database request")

// Settings is what an operator writes under this instance's config block. The
// host merges it over the defaults module.yaml declares and sends the result
// on plugin.configure, before any request reaches this module.
type Settings struct {
	// Storage is the storage the catalogue's files go into.
	Storage string `json:"storage"`
	// Root is the folder inside that storage the files go under.
	Root string `json:"root"`
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
	// calls the host's record store and storage door, and a client needs the
	// session to call on. The map is the one the session holds, and nothing
	// answers a frame until Serve is running.
	serves := wirerpc.Methods{}

	conn, err := wirerpc.NewConn(stdio{}, wirerpc.Options{
		Role:    wirerpc.RoleChild,
		Methods: serves,
	})
	if err != nil {
		return fmt.Errorf("build the session: %w", err)
	}

	// The host's record store, where an earlier release kept the catalogue,
	// and its storage door, which answers as the person each request names.
	live := &settings.Live{}
	held := catalog.New(stateapi.NewClient(conn), storageapi.NewClient(conn), live.Get().Place(), nil).
		WithLog(slog.New(slog.NewTextHandler(os.Stderr, nil)).With(slog.String("module", name)))

	opened := &database{}
	defer opened.close()

	maps.Copy(serves, methods(held, live, opened))

	// The pid is here because the host's log is the only place a person sees
	// this process. Nothing on the wire carries it.
	fmt.Fprintf(os.Stderr, "%s %s is serving on stdio as pid %d\n", name, release, os.Getpid())

	if err = conn.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// methods is the lifecycle set every module answers, plus the one call that
// carries this module's own HTTP surface.
func methods(held *catalog.Catalog, live *settings.Live, opened *database) wirerpc.Methods {
	return wirerpc.Methods{
		wirerpc.MethodInfo: func(context.Context, wirerpc.Call) (any, error) {
			// The fragment is attached here rather than served as a route,
			// because the host publishes it at a stable address whether or not
			// the module is serving yet, and a fragment the host would refuse
			// is refused on this call rather than found by a client.
			return routesapi.Describe(
				wirerpc.Info{
					Name:     name,
					Version:  release,
					Contract: wirerpc.Contract,
					// This module reads settings, so the host sends them.
					Serves: []wirerpc.Method{wirerpc.MethodConfigure},
				}, openAPI)
		},
		wirerpc.MethodShutdown: func(context.Context, wirerpc.Call) (any, error) {
			fmt.Fprintf(os.Stderr, "%s was asked to stop\n", name)

			opened.close()

			return nil, nil
		},
		// The settings the host merged from module.yaml's defaults and this
		// instance's config block, and the database the operator granted. A
		// key this module does not read refuses the start, and so does a
		// value of the wrong shape. Files kept under a default nobody chose
		// would be found nowhere.
		//
		// The database is opened and migrated here, so the module reports
		// ready only on a schema it has applied. A migration that fails
		// refuses the start, and the operator reads the reason on the
		// instance.
		//
		// Nothing reads the record store here. The host answers every call
		// made while it is still starting this module with
		// stateapi.ErrNotReady, so the move out of the record store waits
		// for the first request.
		wirerpc.MethodConfigure: wirerpc.ConfigureReporting(
			func(ctx context.Context, sent Settings, grants wirerpc.Grants) (wirerpc.Configured, error) {
				if err := live.Set(settings.Values{Storage: sent.Storage, Root: sent.Root}); err != nil {
					return wirerpc.Configured{}, err
				}

				if grants.Database == nil {
					return wirerpc.Configured{}, errNoDatabase
				}

				db, report, err := moduledb.Open(ctx, *grants.Database, schema)
				if err != nil {
					// The cause first: the host shows this line where it may
					// be cut short. The grant's String leaves the password out.
					return wirerpc.Configured{}, fmt.Errorf("%w (the database is %s)", err, grants.Database)
				}

				held.Attach(db, grants.Database.Engine)
				opened.swap(db)
				held.Move(live.Get().Place())

				fmt.Fprintf(os.Stderr, "%s keeps its catalogue in %s at schema version %d\n",
					name, grants.Database, report.Version)

				return wirerpc.Configured{Database: report.State()}, nil
			}),
		routesapi.MethodServe: routesapi.Serve(routes.New(held)),
	}
}

// database is the handle plugin.configure opened. A second configure replaces
// it, and the old one is closed once the catalogue no longer reads it.
type database struct {
	mu sync.Mutex
	db *sql.DB
}

func (d *database) swap(next *sql.DB) {
	d.mu.Lock()
	old := d.db
	d.db = next
	d.mu.Unlock()

	if old != nil {
		_ = old.Close() // the catalogue holds the new handle, and nothing reads the old one
	}
}

func (d *database) close() { d.swap(nil) }

// stdio is the pair the host handed over. Close shuts the reading half, which
// is what ends a session whose host has gone quiet.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }
