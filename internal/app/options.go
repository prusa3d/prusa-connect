package app

import (
	"io"
	"net/netip"
	"os"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/kernel/procgroup"
)

// defaultVersion is what the process reports when the build did not stamp one.
const defaultVersion = "0.0.0-dev"

// Options are what the command line supplies. Flag parsing lives in cmd, and
// this is the value it hands over. A non-empty field overrides the same key in
// the configuration file.
type Options struct {
	// ConfigPath is the operator's file. It is also the file the resource store
	// reads its declarations from.
	ConfigPath string
	// Overlay is the machine-owned file the resource API writes.
	Overlay string
	// APIAddr is the address the HTTP surface binds.
	APIAddr string
	// PrinterAddr is the address the mounted connector's own listener binds. It
	// is written into that connector's settings block, because the listener is
	// the dialect's (see withListenerAddr). Empty defers to the file, where a
	// connector that names no address asks for no socket and its printers dial
	// the API listener.
	PrinterAddr string
	// LogLevel is debug, info, warn or error.
	LogLevel string
	// LogFormat is text or json.
	LogFormat string
	// SafeMode starts this run with every module off. A restart the server
	// performs drops the flag that sets it.
	SafeMode bool
	// safe is how this run entered safe mode. Run sets it from SafeMode and
	// the marker.
	safe safeMode
	// Version is the build this process reports. Empty is an unstamped build.
	Version string
	// restart stops the process so it starts again on the file the first-run
	// wizard wrote. Run sets it; nothing on the command line does.
	restart func()
	// pairingPermit is the addresses a printer pairing may dial. Nil is the
	// local network. A test sets it to reach a fake printer on loopback.
	pairingPermit func(netip.Addr) bool
	// upgradeApply stops the fully composed process before the outer launcher
	// swaps its executable.
	upgradeApply func()
	// backupRestore records a restore request and stops the composed process so
	// no open database handle can race restoration.
	backupRestore func(string) error
	// appMount builds the mount that serves the core's app. Nil is
	// httpapi.AppMount. Tests set it to serve a fixture bundle.
	appMount appMounter
	// Stderr is where the setup code is printed for a person to read. Nil is
	// the process's own standard error.
	Stderr io.Writer
	// LoopbackOnly keeps this process off the local network on its own
	// account. The mDNS announcement and listener stay off, whatever
	// core.discovery.enabled says, and the camera relay gathers candidates on
	// the loopback interface alone. The addresses the operator names, such as
	// core.api.addr, are still bound as written.
	//
	// The test harnesses and the demo set it. On Windows every new binary that
	// joins a multicast group or binds an address another machine can reach
	// makes the firewall ask the person at the keyboard to allow it.
	LoopbackOnly bool
	// Reaper is the process's subreaper, which every module process is
	// started through. The process entry builds it once and runs it: two in
	// one process collect each other's children. Nil runs none, which is what
	// a test that starts several servers in one process needs.
	Reaper *procgroup.Reaper
}

// stderr is where a message for the person at the terminal goes.
func (o Options) stderr() io.Writer {
	if o.Stderr == nil {
		return os.Stderr
	}

	return o.Stderr
}

// ReportedVersion is the version the process reports in its startup log, at
// GET /api/v1/health and for `--version`, so the three always agree.
func (o Options) ReportedVersion() string {
	return o.version()
}

// version is what the process reports about itself.
func (o Options) version() string {
	if o.Version == "" {
		return defaultVersion
	}

	return o.Version
}

// applyTo lays the flags over the file and fills the defaults again. The
// result is validated afterwards, so a bad flag is refused the same way a bad
// file key is.
//
// PrinterAddr is not here. It names a key inside a connector's own settings
// block, which this package carries and never reads, so it is laid over that
// block where the connector is built.
func (o Options) applyTo(core config.Core) config.Core {
	if o.APIAddr != "" {
		core.API.Addr = o.APIAddr
	}

	if o.LogLevel != "" {
		core.Log.Level = o.LogLevel
	}

	if o.LogFormat != "" {
		core.Log.Format = o.LogFormat
	}

	if o.Overlay != "" {
		core.Overlay = o.Overlay
	}

	if o.LoopbackOnly {
		off := false
		core.Discovery.Enabled = &off
	}

	return core.WithDefaults(o.ConfigPath)
}
