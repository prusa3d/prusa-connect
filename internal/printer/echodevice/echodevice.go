// Package echodevice is a fake printer that speaks the echo connector's
// dialect: it dials the server, says what it is, answers the command poll and
// posts what it reads.
//
// It is the far end of adapters/connector/echo. internal/printer/simulator is
// the far end of the xBuddy dialect and simulates a whole printer; this one
// prints nothing. A demonstration needs a machine on the wire that is not a
// Prusa, and this is the smallest honest one.
//
// The Moonraker dialect a Klipper machine really speaks is an out-of-tree
// module under examples/adapters/moonraker, and the demo build does not compose
// it, so the demo stands a machine of this dialect in its place. Nothing here
// is Moonraker and nothing here says it is.
package echodevice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The budgets this device keeps on its own requests.
const (
	// pollBudget is longer than the connector's own poll window, so a poll that
	// answers nothing is ended by the server and not by this client's clock.
	// The shipped window is 25 seconds.
	pollBudget = 60 * time.Second
	// callBudget bounds every other request, all of which answer at once.
	callBudget = 10 * time.Second
	// retryWait is how long the device waits after a request it could not
	// complete. A real machine retries rather than giving up on its server.
	retryWait = 2 * time.Second
	// reportEvery is how often one reading goes up when the caller names no
	// cadence.
	reportEvery = 3 * time.Second
)

// What this machine reports while it sits idle. The paths are the addressable
// state tree's, the same ones the xBuddy dialect writes its telemetry to, so a
// page draws this machine with the code it already has. Its one nozzle is head
// 0 of the tool tree, and it is always the active head.
const (
	statePath         = "/state"
	bedPath           = "/temperature/bed"
	bedTargetPath     = "/temperature/bed_target"
	idleState         = `"IDLE"`
	targetOff         = "0"
	nozzleIdleCelsius = 26.0
	bedIdleCelsius    = 23.0
	// wobbleCelsius is how far a reading moves between ticks. A number that
	// never changes reads as a frozen page rather than as a live machine.
	wobbleCelsius = 0.4
	// readingDigits is how many decimals a temperature carries.
	readingDigits = 1
)

// The errors New reports for a device nobody can build. Callers inspect them
// with errors.Is.
var (
	// ErrNoServer reports a device that was told no server to dial.
	ErrNoServer = errors.New("echodevice: the device was given no server to dial")
	// ErrNoConnector reports a device that was told no connector instance, so
	// there is no path on the server for it to join.
	ErrNoConnector = errors.New("echodevice: the device was given no connector instance to dial")
	// ErrNoSerial reports a device with no serial number. A person matches a
	// machine by its serial, and two machines with none are one row twice.
	ErrNoSerial = errors.New("echodevice: the device was given no serial number")
	// ErrRefused reports a handshake the server did not complete.
	ErrRefused = errors.New("echodevice: the server did not admit the device")
)

// Config is one fake machine.
type Config struct {
	// BaseURL is the server as this device reaches it, such as
	// http://127.0.0.1:8080.
	BaseURL string
	// Connector is the instance name the host mounted the echo driver under.
	// The routes sit below /connectors/ plus this name.
	Connector string
	// Vendor, Model, Serial and Firmware are what the device says it is in the
	// handshake. The host renders the model and the serial as the name a person
	// reads, and joins the model onto the model catalogue.
	Vendor   string
	Model    string
	Serial   string
	Firmware string
	// Report is how often one reading goes up. Zero is three seconds.
	Report time.Duration
	// BuildVolume is the space a print may fill, in millimetres. The zero
	// value is a machine whose figures nobody knows, and the device then
	// reports none.
	BuildVolume BuildVolume
}

// BuildVolume is the width, depth and height a print may fill, in millimetres.
type BuildVolume struct {
	X, Y, Z float64
}

// The one head of this machine, as the tool tree names it. Every driver with
// one head writes the same shape (ADR-20261025-pze): one head of the print-head type,
// enabled and active.
//
//nolint:gochecknoglobals // fixed paths, built once from the SDK's names.
var (
	headID           = connectorapi.HeadID(0)
	nozzlePath       = connectorapi.ToolPath(headID, connectorapi.NozzleTemp)
	nozzleTargetPath = connectorapi.ToolPath(headID, connectorapi.NozzleTargetTemp)
)

// Device is a fake printer on the echo wire.
type Device struct {
	cfg    Config
	root   string
	client *http.Client

	// token names the connection the server gave this device. It is written by
	// the handshake, before anything else reads it.
	token string

	// ticks counts the readings sent. It is confined to the reporting
	// goroutine, which is the only thing that touches it.
	ticks int
	// described is set once the server has taken every fact of the machine
	// that does not change. It is confined to the reporting goroutine.
	described bool
}

// New builds a device without dialing anything, so a configuration nobody can
// act on is refused before a demonstration takes a port.
func New(cfg Config) (*Device, error) {
	switch {
	case cfg.BaseURL == "":
		return nil, ErrNoServer
	case cfg.Connector == "":
		return nil, ErrNoConnector
	case cfg.Serial == "":
		return nil, ErrNoSerial
	}

	if cfg.Report <= 0 {
		cfg.Report = reportEvery
	}

	return &Device{
		cfg:  cfg,
		root: strings.TrimSuffix(cfg.BaseURL, "/") + "/connectors/" + cfg.Connector,
		// The timeout is the longest request this device makes, and every call
		// carries a context of its own below it.
		client: &http.Client{Timeout: pollBudget},
	}, nil
}

// Serial is what a person matches this machine by.
func (d *Device) Serial() string { return d.cfg.Serial }

// Run dials the server and holds the connection until the context ends.
//
// A context that ends is how a demonstration stops, so it is not a failure. The
// device says goodbye on the way out, which is what makes the listing report it
// gone rather than waiting for the session to time out.
func (d *Device) Run(ctx context.Context) error {
	if err := d.connect(ctx); err != nil {
		return err
	}

	defer d.disconnect(ctx)

	group, runCtx := errgroup.WithContext(ctx)

	group.Go(func() error { return d.answerCommands(runCtx) })
	group.Go(func() error { return d.reportReadings(runCtx) })

	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

// connectRequest is what this device says about itself. The field names are the
// echo dialect's, and a device that spelled one of them differently would be
// admitted as a machine that reported nothing.
type connectRequest struct {
	Vendor   string `json:"vendor"`
	Model    string `json:"model"`
	Firmware string `json:"firmware"`
	Serial   string `json:"serial"`
	UnitUID  string `json:"unit_uid"`
}

// connectResponse is the connection the server gave this device.
type connectResponse struct {
	Device string `json:"device"`
}

// wireCommand is one command the server sent.
type wireCommand struct {
	ID uint64 `json:"id"`
}

// wireResult is what this device made of one command.
type wireResult struct {
	ID uint64 `json:"id"`
	OK bool   `json:"ok"`
}

// connect is the handshake. Nothing else runs until it has answered, so the
// token the rest of this device uses is written once and never changes.
func (d *Device) connect(ctx context.Context) error {
	// The unit id is the serial. A real machine has an identifier the factory
	// burned in and a serial printed on a label; a fake one has a serial, and
	// two fields carrying it say the same true thing twice.
	body, err := json.Marshal(connectRequest{
		Vendor:   d.cfg.Vendor,
		Model:    d.cfg.Model,
		Firmware: d.cfg.Firmware,
		Serial:   d.cfg.Serial,
		UnitUID:  d.cfg.Serial,
	})
	if err != nil {
		return fmt.Errorf("echodevice: write the identity of %s: %w", d.cfg.Serial, err)
	}

	var answer connectResponse

	status, err := d.call(ctx, http.MethodPost, d.root+"/connect", body, &answer, callBudget)
	if err != nil {
		return err
	}

	if status != http.StatusOK || answer.Device == "" {
		return fmt.Errorf("%w: %s answered %s at %s",
			ErrRefused, d.cfg.Serial, http.StatusText(status), d.root+"/connect")
	}

	d.token = answer.Device

	return nil
}

// answerCommands is the long poll. It answers every command the same way,
// because this machine moves nothing: a demonstration reads the command path
// working, and a fake outcome for a motion that did not happen would be a
// reading nobody can tell from a real one.
func (d *Device) answerCommands(ctx context.Context) error {
	for ctx.Err() == nil {
		var command wireCommand

		status, err := d.call(ctx, http.MethodGet, d.deviceURL("/commands"), nil, &command, pollBudget)

		// A poll that did not complete is the demonstration stopping or the
		// server restarting under a machine that is still on the wire. A
		// printer waits and polls again, so this one does too, and the loop
		// condition is what ends it.
		if err != nil || (status != http.StatusOK && status != http.StatusNoContent) {
			wait(ctx, retryWait)

			continue
		}

		if status == http.StatusOK {
			d.answer(ctx, command.ID)
		}
	}

	return nil
}

// answer reports one command done. A result the server is no longer waiting for
// is dropped by the driver, so nothing here retries it.
func (d *Device) answer(ctx context.Context, id uint64) {
	body, err := json.Marshal(wireResult{ID: id, OK: true})
	if err != nil {
		return
	}

	// The outcome is the whole of what this call carries, and there is nothing
	// to do with a failure to deliver it: the caller's own deadline is what
	// ends its wait, and this device has no state riding on the answer.
	_, _ = d.call(ctx, http.MethodPost, d.deviceURL("/results"), body, nil, callBudget)
}

// reportReadings posts what this machine reads, over and over, until the
// context ends. The first reading goes up at once, so the device page has
// something on it the moment the machine appears in the listing.
func (d *Device) reportReadings(ctx context.Context) error {
	ticker := time.NewTicker(d.cfg.Report)
	defer ticker.Stop()

	for {
		d.report(ctx)

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// report sends one reading, as one patch per node. The facts that never
// change go first, and again with each reading until the server has taken
// all of them.
func (d *Device) report(ctx context.Context) {
	if !d.described {
		d.described = d.post(ctx, d.facts())
	}

	drift := math.Sin(float64(d.ticks)) * wobbleCelsius
	d.ticks++

	// A reading the server did not take is replaced by the next one a few
	// seconds later, so there is nothing here to recover.
	d.post(ctx, []connectorapi.StatePatch{
		{Path: statePath, Value: json.RawMessage(idleState)},
		{Path: nozzlePath, Value: json.RawMessage(celsius(nozzleIdleCelsius + drift))},
		{Path: nozzleTargetPath, Value: json.RawMessage(targetOff)},
		{Path: bedPath, Value: json.RawMessage(celsius(bedIdleCelsius + drift))},
		{Path: bedTargetPath, Value: json.RawMessage(targetOff)},
	})
}

// facts is what this machine is: one head, enabled and active, its bed, and
// the build volume when the configuration names one.
func (d *Device) facts() []connectorapi.StatePatch {
	out := []connectorapi.StatePatch{
		{Path: connectorapi.ToolCountPath, Value: json.RawMessage("1")},
		{Path: connectorapi.ToolPath(headID, connectorapi.ToolEnabled), Value: json.RawMessage("true")},
		{
			Path:  connectorapi.ToolPath(headID, connectorapi.ToolType),
			Value: json.RawMessage(strconv.Quote(connectorapi.ToolTypeHead)),
		},
		{Path: connectorapi.ToolPath(headID, connectorapi.ToolActive), Value: json.RawMessage("true")},
	}

	out = append(out, bedReading().Patches()...)

	volume := d.cfg.BuildVolume
	if volume.X <= 0 || volume.Y <= 0 || volume.Z <= 0 {
		return out
	}

	return append(out,
		connectorapi.StatePatch{Path: connectorapi.BuildVolumeXPath, Value: json.RawMessage(millimetres(volume.X))},
		connectorapi.StatePatch{Path: connectorapi.BuildVolumeYPath, Value: json.RawMessage(millimetres(volume.Y))},
		connectorapi.StatePatch{Path: connectorapi.BuildVolumeZPath, Value: json.RawMessage(millimetres(volume.Z))},
	)
}

// bedReading describes the bed this machine reports.
func bedReading() connectorapi.Reading {
	return connectorapi.Reading{
		ID: "bed", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: bedPath, TargetPath: bedTargetPath, Role: connectorapi.RoleBed,
	}
}

// post sends each patch and reports whether the server took all of them. The
// echo dialect answers a patch it took with 204 No Content.
func (d *Device) post(ctx context.Context, patches []connectorapi.StatePatch) bool {
	taken := true

	for _, patch := range patches {
		body, err := json.Marshal(patch)
		if err != nil {
			taken = false

			continue
		}

		status, err := d.call(ctx, http.MethodPost, d.deviceURL("/state"), body, nil, callBudget)
		if err != nil || status != http.StatusNoContent {
			taken = false
		}
	}

	return taken
}

// disconnect tells the server the machine is going away.
//
// It runs after the context that ran the device has ended, so it carries that
// context with the cancellation taken off. A goodbye sent on a cancelled
// context is a request that never leaves, and the session then waits for a
// machine that is already gone.
func (d *Device) disconnect(ctx context.Context) {
	stopping, cancel := context.WithTimeout(context.WithoutCancel(ctx), callBudget)
	defer cancel()

	_, _ = d.call(stopping, http.MethodPost, d.deviceURL("/disconnect"), nil, nil, callBudget)
}

// deviceURL is one of this device's routes, with the connection it was given.
func (d *Device) deviceURL(route string) string {
	return d.root + route + "?device=" + url.QueryEscape(d.token)
}

// call makes one request and decodes the answer when the caller wants one.
func (d *Device) call(
	ctx context.Context, method, target string, body []byte, into any, budget time.Duration,
) (int, error) {
	callCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	request, err := http.NewRequestWithContext(callCtx, method, target, reader)
	if err != nil {
		return 0, fmt.Errorf("echodevice: build %s %s: %w", method, target, err)
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := d.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("echodevice: %s %s: %w", method, target, err)
	}

	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, fmt.Errorf("echodevice: read the answer to %s %s: %w", method, target, err)
	}

	if into == nil || response.StatusCode != http.StatusOK {
		return response.StatusCode, nil
	}

	if err = json.Unmarshal(payload, into); err != nil {
		return response.StatusCode, fmt.Errorf("echodevice: read the answer to %s %s: %w", method, target, err)
	}

	return response.StatusCode, nil
}

// celsius renders one temperature for the state tree.
func celsius(reading float64) string {
	return strconv.FormatFloat(reading, 'f', readingDigits, 64)
}

// millimetres is a length as the state tree carries it, with no more digits
// than it needs.
func millimetres(length float64) string {
	return strconv.FormatFloat(length, 'f', -1, 64)
}

// wait holds for the given stretch, and returns at once when the context ends
// instead. The caller decides what to do next, which is what keeps a stopping
// demonstration from waiting out one last retry.
func wait(ctx context.Context, stretch time.Duration) {
	timer := time.NewTimer(stretch)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
