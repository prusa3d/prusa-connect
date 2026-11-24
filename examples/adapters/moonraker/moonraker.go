// Package moonraker is the dialect of a Klipper machine behind Moonraker, the
// API server of Voron, Rat Rig and most converted Enders. It is outbound: the
// server dials the address in the endpoint list. One websocket has the
// identity exchange, the subscription and every command, as JSON-RPC 2.0.
//
// The driver ships no profiles. What the machine lists under
// printer.objects.list narrows the baseline: a machine with no pause_resume
// section is not offered pause. The heater bounds are the machine's own.
//
// Klipper reports no vendor, model or serial. The endpoint options give them:
//
//	vendor    who made the machine. Empty reports "Klipper"
//	model     the model name. Empty reports the Klipper host's hostname
//	serial    the serial number. Empty reports none
//	unit_uid  the stable identity. Empty derives one from the address
//
// Set unit_uid where an address can change. The printer id is derived from it.
package moonraker

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
)

// Type is the value of a connector's `type` in configuration that selects this
// driver.
const Type connectorapi.TypeName = "moonraker"

// defaultPort is where Moonraker listens by default.
const defaultPort = "7125"

// websocketPath is where Moonraker serves its JSON-RPC API. An address that
// names a path keeps it, for a machine behind a reverse proxy.
const websocketPath = "/websocket"

const (
	// defaultTimeout bounds one call to the machine. The subscription is
	// bounded by the read deadline.
	defaultTimeout = 10 * time.Second

	// patchBacklog bounds how many state patches may wait for the host.
	patchBacklog = 64

	// maxFrameBytes bounds one websocket frame. The configuration dump is the
	// largest thing Moonraker sends.
	maxFrameBytes = 1 << 21
)

// The errors this driver reports. Callers inspect them with errors.Is.
var (
	// ErrInvalidConfig reports a config block or an address this driver cannot
	// act on.
	ErrInvalidConfig = errors.New("moonraker: invalid configuration")
	// ErrUnreachable reports a machine that did not answer.
	ErrUnreachable = errors.New("moonraker: the machine did not answer")
	// ErrRefused reports a machine that answered a call with a JSON-RPC error
	// during the identity exchange, so no session was built.
	ErrRefused = errors.New("moonraker: the machine refused a call")
)

//nolint:gochecknoinits // registration has to happen before main; the host reaches it through a blank import.
func init() { connectorapi.Register(Type, doc(), New, dialect()) }

// dialect describes the machines of this driver to a page that adds one.
//
// The core names only Prusa models, so a driver for any other machine declares
// its own model rows: a name, a vendor and a drawing. A machine joins this
// row when its endpoint option model is "Klipper".
func dialect() connectorapi.Dialect {
	return connectorapi.Dialect{
		Brands: []string{"Voron", "Rat Rig", "Creality"},
		Needs:  connectorapi.NeedsAddress,
		Models: []connectorapi.DeviceModel{{
			Model:       "Klipper",
			DisplayName: "Klipper",
			Vendor:      defaultVendor,
			Artwork:     connectorapi.Artwork{MediaType: connectorapi.ArtworkSVG, Bytes: artwork},
		}},
		States: states(),
	}
}

// artwork is the drawing of a Klipper machine.
//
//go:embed artwork.svg
var artwork []byte

// doc is what the published extension reference says about this driver.
func doc() extension.Doc {
	return extension.Doc{
		Summary: "A Klipper machine reached through Moonraker, over one JSON-RPC websocket.",
		Description: "The host dials the address the endpoint list holds, asks Moonraker what the machine " +
			"is, and subscribes to the Klipper objects it reads telemetry from. What the machine reports " +
			"under printer.objects.list narrows what is offered, so a machine with no pause_resume " +
			"section is not offered pause. The endpoint options vendor, model, serial and unit_uid say " +
			"what Klipper itself does not report.",
		Version:   "0.1.0",
		Contract:  connectorapi.Contract,
		Stability: extension.StabilityExperimental,
		Options: []extension.Option{
			{
				Key:     "timeout",
				Type:    extension.OptionDuration,
				Default: "10s",
				Summary: "How long one call to the machine may take before it is abandoned.",
			},
		},
	}
}

// Config is what an operator writes under a connector's `config` block.
type Config struct {
	// Timeout bounds one call to the machine, as time.ParseDuration reads a
	// duration.
	Timeout string `json:"timeout,omitempty"`
}

// Driver is the connectorapi.Outbound this package registers. It has no
// per-machine state. That is on the Session built for each machine.
type Driver struct {
	timeout time.Duration
	live    liveness
	dialer  *websocket.Dialer
	log     *slog.Logger
}

var _ connectorapi.Outbound = (*Driver)(nil)

// New builds a Driver from its spec. It is the connectorapi.Factory for Type.
func New(spec connectorapi.Spec) (connectorapi.Driver, error) {
	var cfg Config
	if err := spec.DecodeConfig(&cfg); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	timeout := defaultTimeout

	if cfg.Timeout != "" {
		parsed, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("%w: timeout: %w", ErrInvalidConfig, err)
		}

		if parsed <= 0 {
			return nil, fmt.Errorf("%w: timeout must be positive, and it is %s", ErrInvalidConfig, parsed)
		}

		timeout = parsed
	}

	driver := newDriver(timeout, liveness{})
	if spec.Logger != nil {
		driver.log = spec.Logger
	}

	return driver, nil
}

// NewDriver returns a driver with the given call timeout. Zero or less means
// the default.
func NewDriver(timeout time.Duration) *Driver { return newDriver(timeout, liveness{}) }

// newDriver also takes the liveness numbers, so a test can shorten them.
func newDriver(timeout time.Duration, live liveness) *Driver {
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return &Driver{
		timeout: timeout,
		live:    live.orShipping(),
		dialer: &websocket.Dialer{
			HandshakeTimeout: timeout,
			ReadBufferSize:   readBufferBytes,
			WriteBufferSize:  writeBufferBytes,
		},
		log: slog.New(slog.DiscardHandler),
	}
}

// The websocket buffers of one connection, at the library's defaults. A
// larger frame is read in several passes.
const (
	readBufferBytes  = 4096
	writeBufferBytes = 4096
)

// Baseline is what this driver can do, for any Klipper machine. It returns a
// fresh set each time. The two temperature ceilings are the highest any
// Klipper configuration uses. Connect narrows them to the machine's own
// min_temp and max_temp.
func (d *Driver) Baseline() connectorapi.Set {
	return connectorapi.Set{
		VerbSendInfo:      {Flag: true},
		VerbSendFileInfo:  {Flag: true},
		VerbCreateFolder:  {Flag: true},
		VerbDeleteFile:    {Flag: true},
		VerbDeleteFolder:  {Flag: true},
		VerbFileUpload:    {Flag: true},
		VerbSendStateInfo: {Flag: true},
		VerbStartPrint:    {Flag: true},
		ArgStartPrintPath: {Flag: true, Kind: connectorapi.KindString, Required: true},
		VerbPausePrint:    {Flag: true},
		VerbResumePrint:   {Flag: true},
		VerbStopPrint:     {Flag: true},
		VerbNozzleTarget:  {Flag: true},
		ArgNozzleTarget:   temperatureArgument(nozzleCeiling),
		ArgNozzleTool:     toolArgumentValue(),
		VerbBedTarget:     {Flag: true},
		ArgBedTarget:      temperatureArgument(bedCeiling),
	}
}

// Connect dials the machine, runs the identity exchange and the live probe,
// and returns a session. profile is what the host offers. The driver and the
// machine can only narrow it, so each merge is an intersection.
func (d *Driver) Connect(
	ctx context.Context, endpoint connectorapi.Endpoint, profile connectorapi.Set,
) (connectorapi.Session, error) {
	target, err := websocketURL(endpoint.Address)
	if err != nil {
		return nil, err
	}

	conn, response, err := d.dialer.DialContext(ctx, target, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrUnreachable, target, err)
	}

	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}

	conn.SetReadLimit(maxFrameBytes)

	session := newSession(conn, httpBaseURL(target), d.timeout, d.live, d.log)

	resolved, err := session.handshake(ctx, d.Baseline().Intersect(profile))
	if err != nil {
		session.hangUp()

		return nil, err
	}

	session.begin(identityOf(endpoint, session.reported), resolved)

	return session, nil
}

// identityOf builds the identity from the endpoint options. A missing option
// falls back to what the machine or the address gives.
func identityOf(endpoint connectorapi.Endpoint, reported printerInfo) connectorapi.Identity {
	unit := endpoint.Options[optionUnitUID]
	if unit == "" {
		unit = unitPrefix + endpoint.Address
	}

	model := endpoint.Options[optionModel]
	if model == "" {
		model = reported.Hostname
	}

	vendor := endpoint.Options[optionVendor]
	if vendor == "" {
		vendor = defaultVendor
	}

	return connectorapi.Identity{
		Vendor:   vendor,
		Model:    model,
		Firmware: connectorapi.NewFirmwareLabel(reported.SoftwareVersion),
		Serial:   endpoint.Options[optionSerial],
		UnitUID:  unit,
	}
}

// The endpoint options this dialect reads, and what it reports when one is
// absent.
const (
	optionVendor  = "vendor"
	optionModel   = "model"
	optionSerial  = "serial"
	optionUnitUID = "unit_uid"

	// defaultVendor is the vendor of a machine that names none.
	defaultVendor = "Klipper"

	// unitPrefix keeps a derived identity from colliding with one an operator
	// wrote.
	unitPrefix = "moonraker:"
)

// websocketURL turns a bare host, a host and port, or a URL into the URL
// Moonraker serves. A URL keeps its path, for a machine behind a proxy.
func websocketURL(address string) (string, error) {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return "", fmt.Errorf("%w: the endpoint names no address", ErrInvalidConfig)
	}

	if !strings.Contains(trimmed, "://") {
		return "ws://" + withPort(trimmed) + websocketPath, nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %q is not an address: %w", ErrInvalidConfig, address, err)
	}

	switch parsed.Scheme {
	case "ws", "http":
		parsed.Scheme = "ws"
	case "wss", "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("%w: %q names the scheme %q, and this dialect speaks http, https, ws and wss",
			ErrInvalidConfig, address, parsed.Scheme)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("%w: %q names no host", ErrInvalidConfig, address)
	}

	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = websocketPath
	}

	return parsed.String(), nil
}

// httpBaseURL turns a validated Moonraker websocket URL into its HTTP base URL
// for file uploads and thumbnail downloads.
func httpBaseURL(wsTarget string) string {
	parsed, err := url.Parse(wsTarget)
	if err != nil {
		return ""
	}

	if parsed.Scheme == "wss" {
		parsed.Scheme = "https"
	} else {
		parsed.Scheme = "http"
	}

	parsed.Path = strings.TrimRight(strings.TrimSuffix(parsed.Path, websocketPath), "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}

// withPort adds Moonraker's default port to a host that names none.
func withPort(host string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}

	return net.JoinHostPort(host, defaultPort)
}

// decodeResult reads one JSON-RPC result. The error names the call.
func decodeResult(method string, result json.RawMessage, into any) error {
	if err := json.Unmarshal(result, into); err != nil {
		return fmt.Errorf("%w: the answer to %s is not readable: %w", ErrUnreachable, method, err)
	}

	return nil
}
