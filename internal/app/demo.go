package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/prusa3d/prusa-connect/internal/app/builtin"
	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/kernel/procgroup"
)

// DemoOptions is one demonstration run: a core, a fleet of simulated printers
// that dial it, and a line saying where to look. Flag parsing belongs to cmd,
// and this is the shape cmd hands over.
type DemoOptions struct {
	// Reaper is the process's subreaper, handed on to the core. See
	// Options.Reaper.
	Reaper *procgroup.Reaper
	// APIAddr is what the core binds and what the fleet dials. It carries a
	// real port, because a printer cannot be given a listener on port zero.
	APIAddr string
	// Dir holds the configuration, the graph and the records. Empty is a
	// temporary directory, removed when the run ends.
	Dir string
	// ConfigPath is the operator's file the demo lays its own settings over,
	// so a module declared there starts and an origin allowed there is
	// honoured. Empty, or a path with no file at it, runs on the defaults.
	ConfigPath string
	// Examples is where the built example module releases are. The demo names
	// each release folder it finds there in core.modules_local, so a module can
	// be installed with no network and no release on anybody's forge. A
	// directory with nothing in it offers nothing, which is what a checkout
	// nobody has built looks like.
	Examples string
	// Marketplace is the marketplace the demo's module index reads, such as a
	// marketplace running on this machine. Empty leaves whatever the operator's
	// file says, which is usually none.
	Marketplace string
	// Password is the demo account's. Empty is a new one on every run.
	Password string
	// Printers is how many printers dial in.
	Printers int
	// Models are the printer models the fleet is drawn from, in order, cycled
	// when there are more printers than models.
	Models []string
	// Flap is how often one printer goes away and comes back. Zero keeps the
	// whole fleet connected.
	Flap time.Duration
	// PrintGap overrides the idle wait between a printer's prints and
	// shortens every print to seconds, a print a module starts on an idle
	// fleet too. Zero is the demo's own pace. A gap shorter than two record
	// flushes is raised to that, so every end is seen.
	PrintGap time.Duration
	// TransferRate is how many bytes a second each printer fetches a file at.
	// Zero fetches as fast as the server serves it.
	TransferRate int64
	// ListingDelay is how long each printer takes to list a folder, holding
	// its one command slot meanwhile. Zero lists at once.
	ListingDelay time.Duration
	// FailShare is the share of prints the demo ends in ERROR. Zero is the
	// default share, a negative value none.
	FailShare float64
	// Idle leaves the printers to whoever prints on them: the demo starts no
	// print of its own and raises no dialog, so a module that plans the
	// prints, such as a planner, has the whole fleet to itself. A print anybody
	// starts still takes the demo's own few minutes, and the printers report
	// no material, so the module keeps what a person said is loaded.
	Idle bool
	// Cameras starts one simulated camera per printer and declares it against
	// that printer, so the demonstration shows a live view as well as a fleet.
	// It is on by default: a camera is one of the things this server does, and
	// a demonstration that leaves it out is one nobody sees it in.
	Cameras bool
	// LogLevel is the core's own level. It is error by default, so the address
	// this run prints is the last thing on the screen. The line a warn level
	// adds is a readiness warning the next second withdraws, and it reads as a
	// fault to somebody seeing this server for the first time.
	LogLevel string
	// Version is the build this process reports.
	Version string
	// Out is where the run says what it did. Nil is os.Stdout.
	Out io.Writer
}

// The demo's defaults. The fleet is three because three different models show
// that a capability set is per printer, and the loopback address is deliberate:
// a demo that binds every interface asks a Windows firewall for permission
// before it shows anybody anything.
const (
	demoAddr     = "127.0.0.1:8080"
	demoPrinters = 3
	demoFlap     = 60 * time.Second
	demoAway     = 15 * time.Second
	demoLogLevel = "error"
	demoSite     = "demo"
	demoSerial   = "DEMO-%02d"
)

// The machine in the demo fleet that is not a Prusa, so a person sees that this
// server is not a Prusa-only product.
//
// It speaks the echo reference dialect, not Moonraker. The Moonraker driver a
// Klipper machine really speaks is an out-of-tree module under
// examples/adapters/moonraker, and `make demo` does not compose it, so the demo
// stands a machine of another dialect in its place. The connector instance is
// named for what it is, because that name is what the device page shows as the
// driver.
//
// The model is the spelling the moonraker example declares a row for. The demo
// composes no driver that declares it, so the device-model vocabulary has no
// row for it and a page draws it as a machine it has no drawing of.
const (
	demoEchoConnector = "demo-echo"
	demoEchoVendor    = "Klipper"
	demoEchoModel     = "Klipper"
	demoEchoSerial    = "DEMO-06"
	demoEchoFirmware  = "0.12.0"
)

// demoEnclosureModel is the model whose demo printer has its enclosure fitted.
const demoEnclosureModel = "XL"

// demoModels are the models the fleet is drawn from. Three families with three
// different capability sets, so the demo shows what a fleet of one model cannot.
// The XL is the one with five toolheads, so a page's per-tool rows have a
// printer to draw.
//
//nolint:gochecknoglobals // a fixed default list, read-only.
var demoModels = []string{"CoreOne", "MK4S", "XL"}

// errNoPrinters is a demo asked for a fleet of none. The whole point of the
// command is the fleet, so it is refused rather than served empty.
var errNoPrinters = errors.New("a demo needs at least one printer")

// Demo runs the core and a fake fleet in one process, until the context ends.
//
// One process rather than two: a person starting this has one terminal and one
// Ctrl+C, and a make recipe that backgrounds a server has to reproduce job
// control on Git Bash, on macOS and on Linux. The printers are
// internal/printer/simulator, which is the fleet the matrix, the end-to-end
// suite and cmd/fleetload already drive.
//
// The core is started the way the shipping binary starts it, on a file: the
// operator's own, with the demo's settings laid over it. That is what lets a
// module declared in config/config.yaml start under the demo, and it is why
// the demo has no startup of its own.
func Demo(ctx context.Context, opts DemoOptions) error {
	opts = opts.withDefaults()

	if opts.Printers < 1 {
		return errNoPrinters
	}

	if opts.Marketplace != "" {
		if err := config.ValidRemoteURL(opts.Marketplace); err != nil {
			return fmt.Errorf("the marketplace %q: %w", opts.Marketplace, err)
		}
	}

	operator, found, err := readOperatorFile(opts.ConfigPath)
	if err != nil {
		return err
	}

	dir, cleanup, err := demoDir(opts)
	if err != nil {
		return err
	}

	defer cleanup()

	local := demoLocalModules(opts.Examples, "extensions")

	document, err := demoDocument(operator, demoConfig(opts, dir, local, !declaresRecordBackend(operator)))
	if err != nil {
		return err
	}

	path := filepath.Join(dir, "config.yaml")
	if err = os.WriteFile(path, document, 0o600); err != nil {
		return fmt.Errorf("write the demo configuration: %w", err)
	}

	password, err := demoAccount(ctx, path, opts)
	if err != nil {
		return err
	}

	if err = demoModuleGrants(ctx, opts.Out, path); err != nil {
		return err
	}

	fleet, err := newDemoFleet(opts)
	if err != nil {
		return err
	}

	fleet.login = demoLogin{username: demoUsername, password: password}
	fleet.offered = len(local)

	if found {
		fleet.operatorFile = opts.ConfigPath
	}

	group, runCtx := errgroup.WithContext(ctx)

	// A restart asked for in the web app, such as one into safe mode, starts
	// the server again in this process: there is no command around it here.
	group.Go(func() error {
		for {
			runErr := Run(runCtx, Options{
				ConfigPath:   path,
				APIAddr:      opts.APIAddr,
				LogLevel:     opts.LogLevel,
				Version:      opts.Version,
				LoopbackOnly: opts.onLoopback(),
				Reaper:       opts.Reaper,
			})
			if !errors.Is(runErr, ErrRestart) || runCtx.Err() != nil {
				return runErr
			}
		}
	})

	group.Go(func() error { return fleet.run(runCtx, opts, dir) })

	// A demo ends when the person stops it, and that is not a failure.
	if err = group.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

// withDefaults fills what the caller left empty.
func (o DemoOptions) withDefaults() DemoOptions {
	if o.APIAddr == "" {
		o.APIAddr = demoAddr
	}

	if o.Printers == 0 {
		o.Printers = demoPrinters
	}

	if len(o.Models) == 0 {
		o.Models = demoModels
	}

	if o.LogLevel == "" {
		o.LogLevel = demoLogLevel
	}

	if o.Examples == "" {
		o.Examples = demoExamplesDir
	}

	if o.Out == nil {
		o.Out = os.Stdout
	}

	return o
}

// onLoopback reports whether the demo's address is a loopback one. Only this
// machine reaches such a server, so the demo then announces nothing on the
// local network and keeps its camera views on loopback too. The default
// address is loopback, so a plain `make demo` opens nothing another machine
// can reach, and nothing the Windows firewall asks about.
func (o DemoOptions) onLoopback() bool {
	host, _, err := net.SplitHostPort(o.APIAddr)
	if err != nil {
		return false
	}

	if host == "localhost" {
		return true
	}

	address, err := netip.ParseAddr(host)

	return err == nil && address.IsLoopback()
}

// baseURL is the server as the fleet and the reader reach it.
func (o DemoOptions) baseURL() string { return "http://" + o.APIAddr }

// demoDir is where the configuration, the graph and the records go. A directory
// the caller named is kept, because they asked for it; one this run made is
// removed, because a demo that leaves a database behind is one the next run
// starts from somebody else's fleet.
func demoDir(opts DemoOptions) (string, func(), error) {
	if opts.Dir != "" {
		if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
			return "", nil, fmt.Errorf("make the demo directory: %w", err)
		}

		return opts.Dir, func() {}, nil
	}

	dir, err := os.MkdirTemp("", "prusa-connect-demo-")
	if err != nil {
		return "", nil, fmt.Errorf("make the demo directory: %w", err)
	}

	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// demoMarketplace is the core.marketplace block the demo writes, and nothing
// for a run that names none, so the operator's own key still holds.
func demoMarketplace(url string) string {
	if url == "" {
		return ""
	}

	return "  marketplace:\n    url: \"" + url + "\""
}

// demoConfig is the demo's own side of the deployment it runs, laid over the
// operator's file by demoDocument: the shipping defaults everywhere it has no
// reason to differ, one storage so readiness has something to report on, a
// history store so the past is readable, and the local identity provider, so
// a person can sign in and mint a token the way they would on their own
// server. The account behind that login is written by demoAccount.
//
// records adds the demo's own record backend. It is false when the operator's
// file declares one, because a deployment with two answers every module state
// call with a refusal.
func demoConfig(opts DemoOptions, dir string, local map[string]string, records bool) string {
	shipping := string(builtin.ShippingConnector())

	return strings.Join([]string{
		"core:",
		"  api:",
		`    addr: "` + opts.APIAddr + `"`,
		"  log:",
		"    level: " + opts.LogLevel,
		"    format: text",
		"  site: " + demoSite,
		"  principal: operator",
		"  overlay: " + demoPath(filepath.Join(dir, "resources.json")),
		// The repository's own file names a modules directory read against the
		// working directory, so without this two demos started from one
		// checkout install into, and start from, the same directory.
		"  modules: " + demoPath(config.DefaultModules(filepath.Join(dir, "config.yaml"))),
		demoMarketplace(opts.Marketplace),
		demoModulesLocal(local),
		"  auth:",
		"    local: true",
		"  connectors:",
		// The shipping dialect and the reference one, both named by the
		// composition root: this file knows no dialect by name.
		"    " + shipping + ":",
		"      type: " + shipping,
		"      config:",
		"        max_registrations: " + strconv.Itoa(opts.Printers*demoRegistrationHeadroom),
		// The simulated fleet is let in on arrival. The demo runs no first run
		// and has nobody at a keyboard to approve a code, and a server that has
		// recorded no choice asks first (withAdmission).
		"        mode: none",
		// The second dialect, so one machine in the fleet is not a Prusa. It is
		// the reference connector and the instance name says so: see the
		// demoEcho constants for why the demo does not run Moonraker.
		"    " + demoEchoConnector + ":",
		"      type: " + string(builtin.ReferenceConnector()),
		// The engine is the demo's, whatever the operator's file names: a demo
		// that wrote its fleet into somebody's Postgres would leave it there.
		"persistence:",
		"  type: sqlite",
		"resources:",
		// Declared, because the demo is what somebody runs to see this server
		// work and the history paths answer 501 without a serving provider. The
		// store is its own SQLite file under the demo directory and goes with
		// it, and it keeps everything for as long as the demo runs.
		"  history:",
		"    past:",
		"      type: sqlite",
		"      config:",
		"        path: " + demoPath(filepath.Join(dir, "history.db")),
		"        retention:",
		`          raw: "0"`,
		demoRecords(dir, records),
		"  storage:",
		"    local:",
		"      type: filesystem",
		"      enabled: true",
		"      required: true",
		"      config:",
		"        root: " + demoPath(filepath.Join(dir, "storage")),
		// A second storage, so the demo shows that a deployment carries more
		// than one and that the file browser has something to switch between.
		// It is the staging type rather than the keeping type: a scratch store
		// is one whose contents nothing outside this server should expect to
		// survive, and `temporary` is the type that says so. Its directory is
		// under the demo directory and goes when the demo directory goes, which
		// is what makes it scratch.
		//
		// It is not `type: memory`. That store is not in this binary: it is
		// examples/adapters/storage-memory and is admitted by naming it in a
		// build configuration, and the demo builds no adapter of its own.
		"    scratch:",
		"      type: temporary",
		"      enabled: true",
		"      config:",
		"        path: " + demoPath(filepath.Join(dir, "scratch")),
		"",
	}, "\n")
}

// demoRecords is the demo's own record backend, or nothing when records is
// false. It is declared because a module's state calls answer 501 without a
// record backend, and an extension the demo offers, such as parts, keeps its
// records there. It is SQLite under the demo directory, so it goes with the
// demo.
func demoRecords(dir string, records bool) string {
	if !records {
		return ""
	}

	return strings.Join([]string{
		"  db:",
		"    records:",
		"      type: sqlite",
		"      enabled: true",
		"      config:",
		"        path: " + demoPath(filepath.Join(dir, "records.db")),
	}, "\n")
}

// demoRegistrationHeadroom is what the fleet size is multiplied by to size the
// connector's registration table. A printer that reconnects takes a second row
// before the first ages out, so a table the size of the fleet is a table the
// fleet can fill by flapping.
const demoRegistrationHeadroom = 4

// demoPath renders a path for the configuration file. A Windows separator is
// turned into a forward slash, which the Go file API accepts on every platform,
// and the value is single-quoted so YAML reads no escape in what is left.
func demoPath(path string) string {
	return "'" + strings.ReplaceAll(filepath.ToSlash(path), "'", "''") + "'"
}
