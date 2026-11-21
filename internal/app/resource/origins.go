package resource

import (
	"context"
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// overlayAPI is what a person changed about the API in the web app.
type overlayAPI struct {
	// AllowedOrigins is the browser origins a person added in Settings. They
	// are added to core.api.allowed_origins in the operator's file, which
	// this server never writes.
	AllowedOrigins []string `json:"allowed_origins"`
}

// AllowedOrigins returns the browser origins the overlay records, and none
// while nobody has stored a list.
func (s *Store) AllowedOrigins() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return nil, err
	}

	if overlay.API == nil {
		return nil, nil
	}

	return slices.Clone(overlay.API.AllowedOrigins), nil
}

// RecordAllowedOrigins writes the browser origins into the overlay and
// changes no other member of the file. It replaces the recorded list.
func (s *Store) RecordAllowedOrigins(origins []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	overlay, err := s.readOverlay()
	if err != nil {
		return err
	}

	overlay.API = &overlayAPI{AllowedOrigins: slices.Clone(origins)}

	return s.writeOverlay(overlay)
}

// ModuleOrigin is one browser origin a module adds to the ones the API
// answers, and the module it comes from.
type ModuleOrigin struct {
	// Origin is the origin as a browser writes it.
	Origin string
	// Namespace is the module's namespace.
	Namespace string
	// Title is the name a person reads for the module.
	Title string
}

// OriginWatcher receives the origins of the modules that are switched on
// every time the document changes. It is called with the store's lock held,
// so it must not call the store.
type OriginWatcher interface {
	SyncModuleOrigins(ctx context.Context, origins []ModuleOrigin)
}

// WatchModuleOrigins sets the watcher and passes it the current list.
func (s *Store) WatchModuleOrigins(ctx context.Context, watcher OriginWatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.origins = watcher
	s.syncModuleOrigins(ctx)
}

// syncModuleOrigins hands the watcher the origins of the document the plane
// converges on. A module that is switched off, by its operator, by
// core.modules_enabled or by safe mode, adds none.
func (s *Store) syncModuleOrigins(ctx context.Context) {
	if s.origins == nil {
		return
	}

	s.origins.SyncModuleOrigins(ctx, moduleOrigins(s.effective(s.current)))
}

// moduleOrigins is every origin the enabled modules of a document were
// granted, sorted by origin and then by namespace. An entry that is not an
// exact https origin is left out: an install never writes one, and the
// operator names any other origin in core.api.allowed_origins.
func moduleOrigins(doc Document) []ModuleOrigin {
	var out []ModuleOrigin

	for _, declared := range doc.Instances {
		namespace, module := declared.ModuleNamespace()
		if !module || !declared.Spec.Enabled || declared.Spec.Module == nil {
			continue
		}

		title := declared.Spec.Module.Title
		if title == "" {
			title = declared.Ref.ID()
		}

		for _, origin := range declared.Spec.Module.AllowedOrigins {
			if manifest.IsAllowedOrigin(origin) {
				out = append(out, ModuleOrigin{Origin: origin, Namespace: namespace, Title: title})
			}
		}
	}

	slices.SortFunc(out, func(a, b ModuleOrigin) int {
		if byOrigin := strings.Compare(a.Origin, b.Origin); byOrigin != 0 {
			return byOrigin
		}

		return strings.Compare(a.Namespace, b.Namespace)
	})

	return out
}
