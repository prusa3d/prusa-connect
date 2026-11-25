// Package compose is how a binary other than the shipped one runs this server.
//
// It exists for build-time adapter composition (ADR-20260901-7t4). A composed build is
// its own Go module: it requires the core and each adapter module, blank-imports
// the adapters so their init functions run, and calls one function here. The
// composition root the core ships, internal/app, is unreachable from any other
// module, because Go refuses an internal/ import across a module boundary. So
// this package is the whole of what the core has to publish for that to work.
//
// One exported function, deliberately. ADR-20260901-7t4 rule 6 makes this the only new
// public API the record mints, because a compatibility obligation is paid
// forever and nothing under internal/ has to move to keep this one small. An
// adapter needs sdk/ and nothing else; a composition module needs this.
//
// What a composition module's main.go looks like:
//
//	package main
//
//	import (
//		"os"
//
//		"github.com/prusa3d/prusa-connect/compose"
//		_ "github.com/acme/prusa-adapter-shelf"
//	)
//
//	func main() { os.Exit(compose.Main()) }
//
// cmd/compose-build writes exactly that file, and cmd/prusa-connect is
// the same call with no adapters.
package compose

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/prusa3d/prusa-connect/internal/app"
	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/hostschema"
	"github.com/prusa3d/prusa-connect/internal/app/resourcehost"
	"github.com/prusa3d/prusa-connect/internal/kernel/buildprofile"
	"github.com/prusa3d/prusa-connect/internal/kernel/procgroup"
	"github.com/prusa3d/prusa-connect/internal/kernel/sqlstore"
	"github.com/prusa3d/prusa-connect/internal/kernel/winservice"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// Exit codes. A usage error is separated from a failure to run so a supervisor
// can tell a typo from an outage. exitRestart asks the supervisor to start the
// process again, for a supervisor that cannot follow a process which starts
// its own successor.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
	exitRestart = 3
)

// The files a Windows service writes its log to, under the folder it runs in.
// A service has no terminal, so these are where an operator reads the log and
// the setup code of the first run.
const (
	serviceLogPath    = "logs/prusa-connect.log"
	serviceStderrPath = "logs/stderr.log"
	serviceLogLimit   = 10 << 20
	serviceLogsKept   = 5
)

// errRestartBySupervisor ends a run that asked for a restart under a
// supervisor that starts the process again itself.
var errRestartBySupervisor = errors.New("the server stops, and the service control manager starts it again")

// supervisor is what started this process, and so what starts it again.
type supervisor struct {
	// service is true when the Windows service control manager started the
	// process. It loses track of a process that starts its own successor, so
	// a restart ends this process with exitRestart, and the recovery action of
	// the service starts it again (ADR-20261125-jqp).
	service bool
}

func (s supervisor) restart() error {
	if s.service {
		return errRestartBySupervisor
	}

	return restart()
}

// restartAt starts binary under a terminal. A service is installed with the
// path of its executable, and an upgrade replaces the file at that path, so
// the service control manager starts the new binary.
func (s supervisor) restartAt(binary string) error {
	if s.service {
		return errRestartBySupervisor
	}

	return restartAt(binary)
}

// coreCommand names the core. The core is what the binary runs anyway, so the
// word is accepted rather than required: it is in the image's CMD, in the
// Kubernetes manifests and in the end-to-end suite, and refusing it would break
// those three to save one branch.
const coreCommand = "core"

// probeCommand asks a core already running on this host whether it is ready. It
// is a word after the core's flags, so `core healthcheck` reads as the sentence
// it is and the container image needs no curl.
const probeCommand = "healthcheck"

// Main runs the server: it parses the flags this binary takes, installs signal
// handling, and returns the exit code the process should carry. It reads
// os.Args itself, so a composition module's main is one line.
//
// It is the entry point of every build of this server, the shipped one
// included. There is one server and no word here selects another, so a first
// argument this function does not know is a usage error.
func Main() int {
	arguments := os.Args[1:]

	// A service runs only the core. Its command line is the one it was
	// installed with, and os.Args holds it.
	service, err := winservice.Detect()
	if err != nil {
		fmt.Fprintln(os.Stderr, "prusa-connect: "+err.Error()+"; running as a program in this terminal")
	}
	if service {
		return runService(arguments)
	}

	// The operator commands, before the core's own flags are parsed: they take
	// flags of their own, they run against a server that is not running, and
	// they are here rather than in cmd/prusa-connect so that a composed
	// build has them too. A composition is somebody's deployment, and a
	// deployment that cannot create its first administrator without a browser
	// is finding CL-05 again. A deployment that cannot install a module without
	// hand-editing a file is the same shape of problem.
	if len(arguments) > 0 {
		switch arguments[0] {
		case app.AdminCommand:
			return runOperatorCommand(app.Admin, arguments[1:])
		case app.RegistrationsCommand:
			return runOperatorCommand(app.Registrations, arguments[1:])
		case app.ModuleCommand:
			return runOperatorCommand(app.Module, arguments[1:])
		case app.BackupCommand:
			return runOperatorCommand(app.Backup, arguments[1:])
		case app.UpgradeCommand:
			return runOperatorCommand(app.Upgrade, arguments[1:])
		}
	}

	if len(arguments) > 0 && arguments[0] == coreCommand {
		arguments = arguments[1:]
	}

	return runCore(context.Background(), arguments, supervisor{})
}

// runService runs the core under the Windows service control manager. A stop
// of the service cancels the core's context, as Ctrl+C does in a terminal.
func runService(arguments []string) int {
	if len(arguments) > 0 && arguments[0] == coreCommand {
		arguments = arguments[1:]
	}

	code, err := winservice.Run(func(ctx context.Context) int {
		return runCore(ctx, arguments, supervisor{service: true})
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "prusa-connect: "+err.Error())

		return exitFailure
	}

	return code
}

// runOperatorCommand runs one of the commands that work on a server which is
// not running, and separates a usage error from a failure so a script can tell
// a typo from an outage.
//
// It installs no signal handler. These commands are one database transaction
// and a line of output; there is nothing to drain, and a Ctrl-C that arrives
// mid-write is what the database's own atomicity is for.
func runOperatorCommand(run func(context.Context, io.Writer, []string) error, args []string) int {
	err := run(context.Background(), os.Stdout, args)
	if err == nil {
		return exitOK
	}

	fmt.Fprintln(os.Stderr, "prusa-connect: "+err.Error())

	if errors.Is(err, app.ErrUsage) {
		return exitUsage
	}

	return exitFailure
}

// runCore parses the core's flags, installs the signal handler and runs it. A
// cancelled context is how a shutdown starts, and every flag left empty defers
// to the configuration file. The context is cancelled by a stop of the
// service, or never in a terminal, where a signal stops the core.
//
// One word may follow the flags, and it is `healthcheck`: the probe reads the
// same configuration the server reads, so it dials whatever address that file
// names.
func runCore(parent context.Context, args []string, host supervisor) int {
	options, rest, err := parseFlags(args)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}

	if err != nil {
		return exitUsage
	}

	// Before anything reads a path: relative paths in the configuration, and
	// -config itself, resolve from this folder.
	if options.workdir != "" {
		if err = os.Chdir(options.workdir); err != nil {
			return report(fmt.Errorf("change to the folder -workdir names: %w", err))
		}
	}

	if options.showVersion {
		if options.versionJSON {
			return reportVersionJSON(options.Options)
		}

		fmt.Fprintln(os.Stdout, "prusa-connect "+options.ReportedVersion())

		return exitOK
	}

	if len(rest) > 0 {
		if len(rest) > 1 || rest[0] != probeCommand {
			fmt.Fprintln(os.Stderr, usage)

			return exitUsage
		}

		return report(app.Probe(parent, options.Options))
	}

	if host.service {
		closeLogs, logErr := openServiceLogs(&options.Options)
		if logErr != nil {
			return report(logErr)
		}
		defer closeLogs()
	}

	if _, err = app.RecoverBackupRestore(parent, options.ConfigPath); err != nil {
		return report(recoveryFailed("recover interrupted backup restore", err))
	}

	restartNeeded, err := app.RecoverUpgrade(parent, options.ConfigPath)
	if err != nil {
		return report(recoveryFailed("recover interrupted upgrade", err))
	}
	if restartNeeded {
		return report(host.restart())
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// One reaper for the whole process, before any module starts. No module
	// inherits the right to open a port below 1024.
	options.Reaper = procgroup.NewReaper()
	procgroup.KeepPortRightFromChildren()

	go options.Reaper.Run(ctx)

	err = app.Run(ctx, options.Options)
	stop()

	return afterRun(err, options.Options, host)
}

// afterRun carries out what the run asked for as it ended: an upgrade, a
// restore, or a restart. Any other outcome is reported as the run's own.
func afterRun(err error, options app.Options, host supervisor) int {
	if errors.Is(err, app.ErrUpgradeApply) {
		return applyUpgrade(options, host)
	}
	if errors.Is(err, app.ErrBackupRestore) {
		if err = app.ApplyPreparedRestore(context.Background(), options.ConfigPath); err != nil {
			return report(err)
		}

		return report(host.restart())
	}

	return report(restartAfter(options.ConfigPath, err, host))
}

// openServiceLogs sends what the core writes to files under the folder the
// service runs in: the log and the setup code to a file that is moved aside
// as it grows, and standard error, where Go writes a panic, to a second file.
// It returns the function that closes the log.
func openServiceLogs(options *app.Options) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(filepath.FromSlash(serviceLogPath)), 0o700); err != nil {
		return nil, fmt.Errorf("create the folder for the service log: %w", err)
	}

	if _, err := winservice.RedirectStandardError(filepath.FromSlash(serviceStderrPath)); err != nil {
		return nil, err
	}

	log, err := winservice.OpenLog(filepath.FromSlash(serviceLogPath), serviceLogLimit, serviceLogsKept)
	if err != nil {
		return nil, err
	}

	options.Stderr = log
	options.LogOutput = log

	return func() {
		if closeErr := log.Close(); closeErr != nil {
			fmt.Fprintln(os.Stderr, "prusa-connect core: close the service log: "+closeErr.Error())
		}
	}, nil
}

// restartAfter starts the server again when the first-run wizard asked for it,
// and when a start on the wizard's configuration failed and was rolled back
// (ADR-20261015-ejz rules 21 and 24). Any other outcome is the run's own.
func restartAfter(configPath string, err error, host supervisor) error {
	if err == nil {
		return nil
	}

	if !errors.Is(err, app.ErrRestart) {
		rolledBack, rollbackErr := app.RollbackFirstRun(configPath, err)
		if rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}

		if !rolledBack {
			return err
		}

		fmt.Fprintln(os.Stderr, "prusa-connect core: the configuration the first-run wizard wrote "+
			"did not start, so the server starts again on the one it had: "+err.Error())
	}

	if restartErr := host.restart(); restartErr != nil {
		return restartErr
	}

	return nil
}

// report turns what the core returned into an exit code, naming the failure on
// standard error so a supervisor's log carries the reason.
// applyUpgrade swaps in the prepared upgrade and starts the executable it
// names. A failed upgrade whose restored executable is in place starts that
// one, so the server comes back instead of staying down.
func applyUpgrade(options app.Options, host supervisor) int {
	binary, err := app.ApplyPreparedUpgrade(context.Background(), options)
	if err != nil {
		if binary == "" {
			return report(err)
		}

		report(err)
	}

	return report(host.restartAt(binary))
}

func report(err error) int {
	if err == nil {
		return exitOK
	}

	fmt.Fprintln(os.Stderr, "prusa-connect core: "+err.Error())

	if errors.Is(err, errRestartBySupervisor) {
		return exitRestart
	}

	return exitFailure
}

// usage names every command this binary takes. It is printed for -h and for a
// word the core does not know, so an operator who types something wrong sees
// the commands that exist.
const usage = "usage: prusa-connect [" + coreCommand + "] [flags] [" + probeCommand + "]\n" +
	"       prusa-connect -version [-json]\n" +
	"       prusa-connect " + app.AdminCommand + " create|reset-password|list [flags]\n" +
	"       prusa-connect " + app.RegistrationsCommand + " list|approve <code>|reject <code> [flags]\n" +
	"       prusa-connect " + app.BackupCommand + " create|list|restore [flags]\n" +
	"       prusa-connect " + app.UpgradeCommand + " check [flags]\n" +
	"       prusa-connect " + app.ModuleCommand +
	" add <module path>@<version> -from <release>|list|remove <module or kind:name> [flags]"

// coreFlags is what the core's command line asks for: the options the core
// runs with, and whether it asked for the version and nothing else.
type coreFlags struct {
	app.Options

	workdir     string
	showVersion bool
	versionJSON bool
}

// parseFlags reads the core's flags and returns whatever followed them.
func parseFlags(args []string) (coreFlags, []string, error) {
	flags := flag.NewFlagSet(coreCommand, flag.ContinueOnError)
	options := coreFlags{Options: app.Options{Version: install.CoreVersion()}}

	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), usage+"\n\nflags of the core:")
		flags.PrintDefaults()
	}

	flags.StringVar(&options.ConfigPath, "config", "config/config.yaml",
		"the operator's configuration file, read for the core and resources blocks")
	flags.StringVar(&options.Overlay, "overlay", "",
		"the machine-owned file the resource API writes, defaulting to config.d/resources.json beside the config")
	flags.StringVar(&options.APIAddr, "addr", "", "address the HTTP API binds, overriding core.api.addr")
	flags.StringVar(&options.PrinterAddr, "printer-addr", "",
		"address the device-facing listener binds, setting listener.addr on the mounted connector")
	flags.StringVar(&options.LogLevel, "log-level", "", "debug, info, warn or error, overriding core.log.level")
	flags.StringVar(&options.LogFormat, "log-format", "", "text or json, overriding core.log.format")
	flags.StringVar(&options.workdir, "workdir", "",
		"the folder the server runs in, from which -config and relative paths in the configuration resolve")
	flags.BoolVar(&options.SafeMode, "safe", false,
		"start once with every module off; a restart the server performs drops this flag")
	flags.BoolVar(&options.showVersion, "version", false, "print the version of this build and stop")
	flags.BoolVar(&options.versionJSON, "json", false, "write machine-readable output with -version")

	if err := flags.Parse(args); err != nil {
		return coreFlags{}, nil, err
	}

	if options.versionJSON && !options.showVersion {
		return coreFlags{}, nil, fmt.Errorf("%w: -json requires -version", app.ErrUsage)
	}

	return options, flags.Args(), nil
}

type versionReport struct {
	Version           string   `json:"version"`
	BuildProfile      string   `json:"build_profile"`
	BuildVariant      string   `json:"build_variant"`
	GOOS              string   `json:"goos"`
	GOARCH            string   `json:"goarch"`
	Contract          string   `json:"contract"`
	Capabilities      []string `json:"capabilities"`
	HostSchemaVersion int      `json:"host_schema_version"`
	HostSchemaCompat  int      `json:"host_schema_compat"`
}

func reportVersionJSON(options app.Options) int {
	schema := hostschema.Schema()
	schemaVersion, err := schema.Version(sqlstore.EngineSQLite)
	if err != nil {
		return report(fmt.Errorf("read the host schema version: %w", err))
	}

	body := versionReport{
		Version:           options.ReportedVersion(),
		BuildProfile:      buildprofile.Name,
		BuildVariant:      buildprofile.Variant,
		GOOS:              runtime.GOOS,
		GOARCH:            runtime.GOARCH,
		Contract:          wirerpc.Contract.String(),
		Capabilities:      resourcehost.Capabilities(),
		HostSchemaVersion: schemaVersion,
		HostSchemaCompat:  schema.Compat,
	}

	if err = json.NewEncoder(os.Stdout).Encode(body); err != nil {
		return report(fmt.Errorf("write version metadata: %w", err))
	}

	return exitOK
}
