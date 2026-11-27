//go:build diagnostics

package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/diagnostics"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/platform/observability"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// diagnosticsBuilt says this build carries the support bundle, so the first-run
// wizard offers its keys.
const diagnosticsBuilt = true

// diagnosticsLogLines is how many of the newest log records the core keeps in
// memory for a diagnostics bundle.
const diagnosticsLogLines = 2000

// diagnosticsLogBytes bounds the text those records hold. A module line is up
// to 8 KiB, so 2000 of them alone would hold about 16 MiB.
const diagnosticsLogBytes = 2 << 20

// diagnosticsPageSize is how many printers one read of the fleet asks for
// while a bundle collects all of them.
const diagnosticsPageSize = 100

// diagnosticsSaltBytes is the HMAC-SHA-256 key the bundle's hashes are salted
// with: the digest length, the least RFC 2104 recommends.
const diagnosticsSaltBytes = 32

// Served reports whether the bundle is offered. An install whose first run
// chose open access serves everyone as an admin, so there it takes an explicit
// core.diagnostics.enabled: true. The choice is read per call, because the
// first run records it while this process runs.
func (d diagnosticsBundles) Served() bool {
	if d.core.Auth.Enabled() || d.core.Auth.Local || d.mode.get() != resource.SetupOpen {
		return true
	}

	return d.core.Diagnostics.Explicit()
}

func (d diagnosticsBundles) Bundle(
	ctx context.Context, principal string, choice httpapi.DiagnosticsChoice, frontend []byte,
) ([]byte, error) {
	src := diagnostics.Sources{
		Version:   d.version,
		Started:   d.started,
		Config:    d.core,
		Logs:      d.ring.Entries(),
		Crash:     tailFile(d.crashes, crashTail),
		Readiness: d.probes.Ready(ctx, principal),
		Secrets: append(append(diagnostics.ConfigSecrets(d.core), d.setupCode),
			slices.Concat(d.store.SecretValues(), d.cameras.secretValues(ctx))...),
	}

	if commit := revision(); commit != "unknown" {
		src.Commit = commit
	}

	for offset := 0; ; {
		page, err := d.devices.List(ctx, principal,
			httpapi.DeviceQuery{Page: httpapi.Page{Limit: diagnosticsPageSize, Offset: offset}})
		if err != nil {
			return nil, fmt.Errorf("list devices: %w", err)
		}

		src.Devices = append(src.Devices, page.Items...)
		offset += len(page.Items)

		if len(page.Items) == 0 || offset >= page.Total {
			break
		}
	}

	listing, err := d.store.List(ctx, principal, "")
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}

	src.Resources = listing.Items

	if src.Connectors, err = d.devices.Connectors(ctx, principal); err != nil {
		return nil, fmt.Errorf("read connector health: %w", err)
	}

	// A count is all the bundle reports, and a store that cannot give one
	// leaves it at zero rather than costing the operator the whole bundle.
	if accounts, listErr := d.accounts.List(ctx); listErr == nil {
		src.Accounts = len(accounts)
	}

	if choice.Extensions {
		src.Extensions = d.extensions()
	}

	salt := make([]byte, diagnosticsSaltBytes)
	if _, err = rand.Read(salt); err != nil {
		return nil, fmt.Errorf("salt the bundle's hashes: %w", err)
	}

	bundle, err := diagnostics.Build(ctx, src, diagnostics.Options{
		Modules: choice.Modules, Extensions: choice.Extensions, Frontend: frontend, Now: time.Now(), Salt: salt,
	})
	if err != nil {
		return nil, fmt.Errorf("build the diagnostics bundle: %w", err)
	}

	return bundle, nil
}

// Extensions is the title of each running module that adds files to a
// bundle.
func (d diagnosticsBundles) Extensions() []string {
	if d.sources == nil {
		return nil
	}

	var titles []string
	for _, source := range d.sources() {
		titles = append(titles, source.Title)
	}

	return titles
}

// extensions is every running module that adds files. diagnostics.Build
// asks them one after another and writes each module's files before it asks
// the next, so a bundle never holds more than one module's files at a time.
func (d diagnosticsBundles) extensions() []diagnostics.ExtensionSource {
	if d.sources == nil {
		return nil
	}

	var out []diagnostics.ExtensionSource

	for _, source := range d.sources() {
		out = append(out, diagnostics.ExtensionSource{
			Instance: source.Instance, Title: source.Title, Collect: source.Collect,
		})
	}

	return out
}

// diagnosticsRing keeps the newest log records for a bundle, or nothing where
// core.diagnostics.enabled is off.
func diagnosticsRing(core config.Core) *observability.LogRing {
	if !core.Diagnostics.On() {
		return nil
	}

	return observability.NewLogRing(diagnosticsLogLines, diagnosticsLogBytes)
}

// ringSink copies what sink receives into ring, when there is one.
func ringSink(sink slog.Handler, ring *observability.LogRing) slog.Handler {
	if ring == nil {
		return sink
	}

	return ring.Handler(sink)
}

// keepCrashes sends the stack of a crash that ends the process to crash.log
// beside the overlay, and returns that path. It returns "" where
// core.diagnostics.enabled is off.
func keepCrashes(ctx context.Context, core config.Core, logger *slog.Logger) string {
	if !core.Diagnostics.On() {
		return ""
	}

	crashes := filepath.Join(filepath.Dir(core.Overlay), "crash.log")
	if err := recordCrashes(crashes); err != nil {
		logger.WarnContext(ctx, "a crash will not be kept for diagnostics", "error", err)
	}

	return crashes
}

// servedDiagnostics is the bundle the API serves, or nil where
// core.diagnostics.enabled is off, which the route answers with 404.
func servedDiagnostics(d diagnosticsBundles) httpapi.Diagnostics {
	if !d.core.Diagnostics.On() {
		return nil
	}

	return d
}

// crashTail is how much of the crash file a bundle carries: the newest crash,
// and whatever older ones fit before it.
const crashTail = 64 << 10

// recordCrashes sends the stack of a crash that ends the process to path, so
// the next start can still hand it to a diagnostics bundle. The file is
// appended to, and cut back at start to the tail a bundle carries, so a server
// that keeps crashing does not fill the disk.
func recordCrashes(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("make the crash file's directory: %w", err)
	}

	if info, err := os.Stat(path); err == nil && info.Size() > crashTail {
		if err = os.WriteFile(path, tailFile(path, crashTail), 0o600); err != nil {
			return fmt.Errorf("cut back the crash file: %w", err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open the crash file: %w", err)
	}
	// SetCrashOutput keeps a duplicate of the descriptor, so this one closes.
	defer f.Close()

	if err = debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		return fmt.Errorf("record crashes: %w", err)
	}

	return nil
}

// tailFile returns at most the last limit bytes of path, from the first whole
// line in them, or nothing when it is missing or unreadable. A line cut in
// half could start with the tail of a token the sweep does not recognise.
func tailFile(path string, limit int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil
	}

	if info.Size() > limit {
		if _, err = f.Seek(-limit, io.SeekEnd); err != nil {
			return nil
		}
	}

	content, _ := io.ReadAll(f) // a short read still reports what was there

	if info.Size() > limit {
		if i := bytes.IndexByte(content, '\n'); i >= 0 {
			return content[i+1:]
		}

		return nil
	}

	return content
}
