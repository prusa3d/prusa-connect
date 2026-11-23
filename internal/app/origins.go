package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

// originStore is the part of the resource store that keeps the browser
// origins a person added, in the machine-owned file.
type originStore interface {
	AllowedOrigins() ([]string, error)
	RecordAllowedOrigins(origins []string) error
}

// browserOrigins is every browser origin the API answers, from the three
// places one comes from: core.api.allowed_origins in the operator's file, the
// list a person keeps in Settings, and the modules that are switched on.
//
// The CORS answer reads it for every request, so the set it matches against
// is a snapshot behind an atomic pointer, rebuilt on each change.
type browserOrigins struct {
	file  []string
	store originStore

	// writing makes the write to the file and the change to settings one
	// step, so two people storing at once leave the two agreeing. It is not
	// mu: the store calls SyncModuleOrigins with its own lock held, and a
	// write that held mu while it waited for that lock would never end.
	writing sync.Mutex

	mu       sync.Mutex
	settings []string
	modules  []resource.ModuleOrigin

	allowed atomic.Pointer[map[string]struct{}]
}

var (
	_ httpapi.BrowserOrigins = (*browserOrigins)(nil)
	_ resource.OriginWatcher = (*browserOrigins)(nil)
)

// newBrowserOrigins reads the stored list. A file that cannot be read does
// not stop the start: the operator's list still holds, and the log says so.
func newBrowserOrigins(ctx context.Context, core config.Core, store originStore, logger *slog.Logger) *browserOrigins {
	held := &browserOrigins{file: slices.Clone(core.API.AllowedOrigins), store: store}

	stored, err := store.AllowedOrigins()
	if err != nil {
		logger.WarnContext(ctx, "the stored browser origins could not be read, so only the configuration's hold",
			"error", err)
	}

	// A value written into the machine-owned file by hand is held to the rule
	// a request is held to.
	for _, origin := range stored {
		if config.CheckOrigin(origin) == nil {
			held.settings = append(held.settings, origin)
		}
	}

	held.publish()

	return held
}

// publish rebuilds the snapshot. The caller holds mu, or is the constructor.
func (b *browserOrigins) publish() {
	allowed := make(map[string]struct{}, len(b.file)+len(b.settings)+len(b.modules))

	for _, origin := range b.file {
		allowed[origin] = struct{}{}
	}

	for _, origin := range b.settings {
		allowed[origin] = struct{}{}
	}

	for _, origin := range b.modules {
		allowed[origin.Origin] = struct{}{}
	}

	b.allowed.Store(&allowed)
}

// Any is httpapi.BrowserOrigins.
func (b *browserOrigins) Any() bool { return len(*b.allowed.Load()) > 0 }

// Allows is httpapi.BrowserOrigins.
func (b *browserOrigins) Allows(origin string) bool {
	_, ok := (*b.allowed.Load())[origin]

	return ok
}

// SyncModuleOrigins is resource.OriginWatcher.
func (b *browserOrigins) SyncModuleOrigins(_ context.Context, origins []resource.ModuleOrigin) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.modules = slices.Clone(origins)
	b.publish()
}

// AllowedOrigins is httpapi.BrowserOrigins.
func (b *browserOrigins) AllowedOrigins(context.Context) (httpapi.AllowedOriginList, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.listed(), nil
}

// SetAllowedOrigins stores the list first and then applies it, so a write the
// file refused leaves the answer as it was. A write quoting a version the
// stored list no longer has is refused, so it does not undo a change it never
// read.
func (b *browserOrigins) SetAllowedOrigins(
	_ context.Context, origins []string, pre httpapi.Precondition,
) (httpapi.AllowedOriginList, error) {
	next, err := settingsOrigins(origins)
	if err != nil {
		return httpapi.AllowedOriginList{}, err
	}

	b.writing.Lock()
	defer b.writing.Unlock()

	b.mu.Lock()
	current := settingsVersion(b.settings)
	b.mu.Unlock()

	if !pre.Holds(true, current) {
		return httpapi.AllowedOriginList{}, httpfault.VersionConflict(httpapi.AllowedOriginsReference, current)
	}

	if err = b.store.RecordAllowedOrigins(next); err != nil {
		return httpapi.AllowedOriginList{}, httpfault.Unexpected("store the allowed browser origins").Wrap(err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.settings = next
	b.publish()

	return b.listed(), nil
}

// settingsVersion is the version of a settings list: a digest of the sorted
// list, so two servers that hold the same list answer the same version, and
// a write that changed nothing leaves it as it was.
func settingsVersion(settings []string) string {
	sum := sha256.Sum256([]byte(strings.Join(settings, "\n")))

	return hex.EncodeToString(sum[:8])
}

// settingsOrigins holds a request's list to what core.api.allowed_origins is
// held to, without spaces around an entry and without a repeat.
func settingsOrigins(origins []string) ([]string, error) {
	if len(origins) > httpapi.MaxSettingsOrigins {
		return nil, httpfault.RequestInvalid("body.origins",
			fmt.Sprintf("at most %d origins", httpapi.MaxSettingsOrigins),
			fmt.Sprintf("the list holds %d origins", len(origins)))
	}

	next := make([]string, 0, len(origins))

	for i, written := range origins {
		origin := strings.TrimSpace(written)
		location := "body.origins[" + strconv.Itoa(i) + "]"

		if err := config.CheckOrigin(origin); err != nil {
			return nil, httpfault.RequestInvalid(location, "an exact origin such as https://slicer.example",
				fmt.Sprintf("%q is not an origin a browser sends: %s", origin, err.Error()))
		}

		if slices.Contains(next, origin) {
			return nil, httpfault.RequestInvalid(location, "each origin once",
				fmt.Sprintf("%q is listed twice", origin))
		}

		next = append(next, origin)
	}

	slices.Sort(next)

	return next, nil
}

// listed is every allowed origin with its source: the file's, then the
// stored ones, then the modules'. The caller holds mu.
func (b *browserOrigins) listed() httpapi.AllowedOriginList {
	out := make([]httpapi.AllowedOrigin, 0, len(b.file)+len(b.settings)+len(b.modules))

	for _, origin := range slices.Sorted(slices.Values(b.file)) {
		out = append(out, httpapi.AllowedOrigin{Origin: origin, Source: httpapi.OriginFromFile})
	}

	for _, origin := range b.settings {
		out = append(out, httpapi.AllowedOrigin{Origin: origin, Source: httpapi.OriginFromSettings})
	}

	for _, origin := range b.modules {
		out = append(out, httpapi.AllowedOrigin{
			Origin: origin.Origin, Source: httpapi.OriginFromExtension,
			Extension: origin.Namespace, Title: origin.Title,
		})
	}

	return httpapi.AllowedOriginList{Items: out, Version: settingsVersion(b.settings)}
}
