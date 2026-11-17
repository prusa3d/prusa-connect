// Command buddy3d-camera is the server a Buddy3D camera talks to instead of
// Prusa's cloud. It takes the camera's pictures over HTTPS and hands each one
// to a push camera of the core with camera.accept, and it keeps the camera's
// Socket.IO session open to set how often it sends and to ask for a picture.
//
// The camera dials the port the operator granted under needs.listen
// (ADR-20261019-3ns), with the self-signed certificate this module makes in
// its data directory. A person pairs a camera on the settings page: the page
// declares the push camera and grants this module operate on it, and this
// module makes the token the camera's sdcard_cfg.txt carries.
//
// This file holds the handshake, the method set and the wiring. The camera's
// protocol is in internal/device, internal/sio and internal/camproto, the
// paired cameras in internal/store and the page's routes in internal/routes.
package main

import (
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/accessapi"
	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"

	"github.com/prusa3d/buddy3d-camera/internal/device"
	"github.com/prusa3d/buddy3d-camera/internal/routes"
	"github.com/prusa3d/buddy3d-camera/internal/store"
)

// What this module reports when the host asks it what it is. The version is
// the newest entry of CHANGELOG.md and the version module.yaml declares, and
// changelog_test.go holds the three together.
const (
	name    = "buddy3d-camera"
	release = extension.Version("0.8.1")
)

// The name module.yaml gives the camera's port.
const listenCamera = "camera"

// DefaultSnapshotInterval is the default module.yaml publishes, which
// manifest_test.go holds it to.
const DefaultSnapshotInterval = 30

//go:embed openapi.json
var openAPI []byte

// Settings is what an operator writes under this instance's config block.
type Settings struct {
	// Address is the host a camera dials, which the certificate names. Empty
	// is the first IPv4 address of this machine.
	Address string `json:"address"`
	// SnapshotInterval is how often a camera sends a picture, in seconds.
	SnapshotInterval int `json:"snapshot_interval"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
}

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

	// The method set is filled after the session exists, because the camera
	// client needs the session to call on.
	serves := wirerpc.Methods{}

	conn, err := wirerpc.NewConn(stdio{}, wirerpc.Options{Role: wirerpc.RoleChild, Methods: serves})
	if err != nil {
		return fmt.Errorf("build the session: %w", err)
	}

	held := &module{
		frames: frames{cameraapi.NewClient(conn)},
		access: accessapi.NewClient(conn),
		log:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	defer held.stop()

	maps.Copy(serves, held.methods())

	fmt.Fprintf(os.Stderr, "%s %s is serving on stdio as pid %d\n", name, release, os.Getpid())

	if err = conn.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// frames hands a picture to the core over camera.accept.
type frames struct{ client *cameraapi.Client }

func (f frames) Accept(ctx context.Context, principal, camera string, frame []byte) error {
	return f.client.Accept(ctx, principal, camera, frame)
}

// module is everything this process holds once the host sent its settings.
type module struct {
	frames device.Frames
	access *accessapi.Client
	log    *slog.Logger

	mu      sync.Mutex
	state   routes.State
	server  *http.Server
	stopped chan struct{}
}

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
			m.stop()

			return nil, nil
		},
		wirerpc.MethodConfigure: wirerpc.ConfigureGranted(m.configure),
		routesapi.MethodServe:   routesapi.Serve(routes.New(m.current)),
	}
}

// configure checks the settings, opens the store and the camera's port. A
// port that cannot be opened does not refuse the start: the page still lists
// the cameras, and the status says why nothing listens.
func (m *module) configure(ctx context.Context, sent Settings, grants wirerpc.Grants) error {
	interval := sent.SnapshotInterval
	if interval == 0 {
		interval = DefaultSnapshotInterval
	}

	if interval < store.MinSnapshotInterval || interval > store.MaxSnapshotInterval {
		return fmt.Errorf("%s: snapshot_interval is %d, and it is %d to %d seconds",
			name, interval, store.MinSnapshotInterval, store.MaxSnapshotInterval)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Ready {
		return errors.New(name + ": the settings were sent twice")
	}

	state := routes.State{
		Ready:            true,
		Address:          sent.Address,
		May:              m.may,
		SnapshotInterval: interval,
	}

	if grants.Data == "" {
		state.Problem = "the operator granted no data directory, so no camera can be paired. Install it again with approve."
		m.state = state

		return nil
	}

	cameras, err := store.Open(grants.Data, time.Now)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	state.Store = cameras

	if state.Address == "" {
		state.Address = firstAddress()
	}

	if state.Address == "" {
		state.Problem = "this machine has no IPv4 address to give a camera. Set address in the module's settings."
		m.state = state

		return nil
	}

	m.stopped = make(chan struct{})

	cert, err := device.LoadOrMake(grants.Data, state.Address)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	state.Cert = cert.PEM

	service := device.New(cameras, m.frames, uint32(interval), m.log)
	state.Device = service

	grant, granted := grants.Listen[listenCamera]
	if !granted {
		state.Problem = "the operator granted no port for cameras. Install the module again with approve_listen."
		m.state = state

		return nil
	}

	// The port the camera is told is the granted one, even before it opens.
	state.Port = grant.Port

	m.server = &http.Server{
		Handler:           service.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert.TLS},
			MinVersion:   tls.VersionTLS12,
		},
		ErrorLog: slog.NewLogLogger(m.log.Handler(), slog.LevelDebug),
	}

	m.state = state

	// The port outlives the request that configured it.
	go m.listen(context.WithoutCancel(ctx), m.server, grant, m.stopped)

	return nil
}

// readHeaderTimeout bounds a camera's request headers, and shutdownWait how
// long a stop waits for requests in flight.
const (
	readHeaderTimeout = 10 * time.Second
	shutdownWait      = 5 * time.Second
)

// listenRetry is how long the module waits before it tries a port that was
// taken again. A module started again by its host often finds the port still
// held by the process it replaces, for a moment.
var listenRetry = 5 * time.Second

// listenHost is the host the ports are opened on: every interface, since the
// cameras dial from the network. A test keeps them on loopback.
var listenHost = ""

// listen opens the camera's port and serves it, trying again while the port
// is taken, until the module stops. The status says why it is not listening.
func (m *module) listen(ctx context.Context, server *http.Server, grant wirerpc.ListenGrant, stopped <-chan struct{}) {
	for {
		var config net.ListenConfig

		listener, err := config.Listen(ctx, grant.Network, net.JoinHostPort(listenHost, strconv.Itoa(grant.Port)))
		if err == nil {
			m.mu.Lock()
			if addr, ok := listener.Addr().(*net.TCPAddr); ok {
				m.state.Port = addr.Port
			}

			m.state.Listening = true
			m.state.Problem = ""
			m.mu.Unlock()

			if err = server.ServeTLS(listener, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				m.log.ErrorContext(ctx, "the camera port closed", "error", err.Error())
			}

			return
		}

		m.mu.Lock()
		m.state.Problem = "the camera port cannot be opened, and the module tries again: " + err.Error()
		m.mu.Unlock()

		select {
		case <-stopped:
			return
		case <-time.After(listenRetry):
		}
	}
}

// may asks the core whether a person may take an action on a camera's device
// row, as the core's own camera routes ask: observe to see it, operate to
// change it.
func (m *module) may(ctx context.Context, principal, action, camera string) (bool, error) {
	decision, err := m.access.Check(ctx, accessapi.CheckParams{
		Subject: principal, Action: action, ObjectType: "device", ObjectID: camera,
	})
	if err != nil {
		return false, err
	}

	return decision.Allowed, nil
}

func (m *module) current() routes.State {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.state
}

func (m *module) stop() {
	m.mu.Lock()
	server, stopped := m.server, m.stopped
	m.server, m.stopped = nil, nil
	m.mu.Unlock()

	if stopped != nil {
		close(stopped)
	}

	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownWait)
		defer cancel()

		_ = server.Shutdown(ctx)
	}
}

// firstAddress is the first IPv4 address of this machine that is not a
// loopback one, which is where a camera on the same network finds it.
func firstAddress() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}

	return ""
}

// stdio is the pair the host handed over.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }
