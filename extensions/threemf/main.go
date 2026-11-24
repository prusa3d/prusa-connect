// Command threemf reads 3MF projects for this server.
//
// A 3MF file is a zip archive with the models, how they stand on the beds and
// the slicer's settings. The reader is the Go package threemf/ beside this
// file, which other modules import as a normal Go dependency. This program
// puts the reader to work in two places.
//
// The core asks it about a .3mf file: the picture in a listing, the title,
// the facts and the objects on the file's page, and what a printer needs
// (file_analysis in module.yaml, ADR-20261027-wvj). The host opens the file
// through a read handle for that one call.
//
// The browser bundle under ui/ draws a project on the file's page, and hands
// the same view to other modules as the component project-view. It reads the
// routes below.
//
// # The routes
//
//	GET /project?storage=&path=            what one project holds
//	GET /project/thumbnail?storage=&path=  one picture of it, by index
//	GET /project/mesh?storage=&path=       one object as triangles
//
// Each path is relative to the address the server gives this module, so a
// caller reaches the first one at /api/v1/modules/<namespace>/project. The
// server takes view_resource on this module's instance and then calls it. The
// module reads the file as the person the server named, so the storage takes
// read_storage for that person.
//
// This file holds the handshake, the method set and the wiring. The reading
// and the caches are in internal/projects, the answer to the core in
// internal/analysis and the handlers in internal/routes.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"os"
	"os/signal"

	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/fileapi"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/storageapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/threemf/internal/analysis"
	"github.com/prusa3d/threemf/internal/projects"
	"github.com/prusa3d/threemf/internal/routes"
)

// What this module reports when the host asks it what it is. The version is
// the newest entry of CHANGELOG.md and the version module.yaml declares, and
// changelog_test.go holds the three together.
const (
	name    = "threemf"
	release = extension.Version("0.1.13")
)

// openAPI is the fragment describing the routes, with every path relative to
// the mount. The host holds it to routesapi.ParseFragment at launch and serves
// it rebased under the namespace.
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

	// The method set is filled after the session exists, because the module
	// reads files through the host's storage door, and a client needs the
	// session to call on. Nothing answers a frame until Serve is running.
	serves := wirerpc.Methods{}

	conn, err := wirerpc.NewConn(stdio{}, wirerpc.Options{
		Role:    wirerpc.RoleChild,
		Methods: serves,
	})
	if err != nil {
		return fmt.Errorf("build the session: %w", err)
	}

	files := storageapi.NewClient(conn)
	maps.Copy(serves, methods(files, projects.New(projects.Wire(files))))

	// The pid is here because the host's log is the only place a person sees
	// this process. Nothing on the wire carries it.
	fmt.Fprintf(os.Stderr, "%s %s is serving on stdio as pid %d\n", name, release, os.Getpid())

	if err = conn.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// methods is the lifecycle set every module answers, the call that carries
// this module's HTTP surface, and the core's question about a file.
func methods(files *storageapi.Client, reader *projects.Reader) wirerpc.Methods {
	return wirerpc.Methods{
		wirerpc.MethodInfo: func(context.Context, wirerpc.Call) (any, error) {
			// The fragment is attached here and not served as a route,
			// because the host publishes it whether or not the module is
			// serving yet, and a fragment the host would refuse is refused
			// on this call.
			return routesapi.Describe(
				wirerpc.Info{Name: name, Version: release, Contract: wirerpc.Contract}, openAPI)
		},
		wirerpc.MethodShutdown: func(context.Context, wirerpc.Call) (any, error) {
			fmt.Fprintf(os.Stderr, "%s was asked to stop\n", name)

			return nil, nil
		},
		routesapi.MethodServe: routesapi.Serve(routes.New(reader)),
		// The file type this module reads for the core. The host calls it with
		// a read handle for one .3mf file.
		fileapi.MethodAnalyze: fileapi.Serve(files, analysis.New(reader)),
	}
}

// stdio is the pair the host handed over. Close shuts the reading half, which
// is what ends a session whose host has gone quiet.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }
