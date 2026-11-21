package resource_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
)

func TestTheStoredOriginsAreWrittenAndReadBack(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored, err := h.store.AllowedOrigins()
	require.NoError(t, err)
	require.Empty(t, stored)

	require.NoError(t, h.store.RecordSetupMode("required"))
	require.NoError(t, h.store.RecordAllowedOrigins([]string{"https://slicer.example"}))

	stored, err = h.store.AllowedOrigins()
	require.NoError(t, err)
	require.Equal(t, []string{"https://slicer.example"}, stored)

	mode, err := h.store.SetupMode()
	require.NoError(t, err)
	require.Equal(t, "required", mode, "the write changed another member of the file")

	var onDisk map[string]json.RawMessage

	content, err := os.ReadFile(h.paths.Overlay)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(content, &onDisk))
	require.JSONEq(t, `{"allowed_origins": ["https://slicer.example"]}`, string(onDisk["api"]))

	require.NoError(t, h.store.RecordAllowedOrigins(nil))

	stored, err = h.store.AllowedOrigins()
	require.NoError(t, err)
	require.Empty(t, stored)
}

// watchedOrigins records every list the store hands over.
type watchedOrigins struct {
	lists [][]resource.ModuleOrigin
}

func (w *watchedOrigins) SyncModuleOrigins(_ context.Context, origins []resource.ModuleOrigin) {
	w.lists = append(w.lists, origins)
}

func (w *watchedOrigins) last() []resource.ModuleOrigin { return w.lists[len(w.lists)-1] }

// slicerEntry is a module that only draws, as an approved install declares
// it, with one origin granted and one value no install writes.
func slicerEntry(dir string, enabled bool) install.Entry {
	return install.Entry{
		Type:    "com.github.acme.slicer/slicer",
		Enabled: &enabled,
		Runtime: install.RuntimeAsset.String(),
		Asset:   &install.AssetEntry{Path: dir, Checksum: "sha256:" + sixtyFourZeros},
		Module: &install.ModuleEntry{
			Title:          "Web slicer",
			AllowedOrigins: []string{"https://slicer.example", "http://slicer.example", "*"},
		},
	}
}

const sixtyFourZeros = "0000000000000000000000000000000000000000000000000000000000000000"

// A module's origins hold while it is switched on and leave with the write
// that switches it off or removes it. Only an exact https origin counts.
func TestAModuleOriginHoldsWhileTheModuleIsSwitchedOn(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	require.NoError(t, h.store.Reload(t.Context()))

	watched := &watchedOrigins{}
	h.store.WatchModuleOrigins(t.Context(), watched)
	require.Empty(t, watched.last())

	instance, err := ref.New(ref.KindAsset, "slicer")
	require.NoError(t, err)

	_, err = resource.InstallModule(h.paths.Overlay, instance, slicerEntry(h.dir, true), resource.LeftChange{})
	require.NoError(t, err)
	require.NoError(t, h.store.Reload(t.Context()))
	require.Equal(t, []resource.ModuleOrigin{
		{Origin: "https://slicer.example", Namespace: "com.github.acme.slicer", Title: "Web slicer"},
	}, watched.last())

	_, err = resource.InstallModule(h.paths.Overlay, instance, slicerEntry(h.dir, false), resource.LeftChange{})
	require.NoError(t, err)
	require.NoError(t, h.store.Reload(t.Context()))
	require.Empty(t, watched.last(), "a module that is switched off still adds its origin")
}

// While core.modules_enabled is false no installed module runs, and none adds
// an origin.
func TestAModuleHeldOffAddsNoOrigin(t *testing.T) {
	t.Parallel()

	h := newHarness(t, func(cfg *resource.Config) { cfg.ModulesOff = true })

	instance, err := ref.New(ref.KindAsset, "slicer")
	require.NoError(t, err)

	_, err = resource.InstallModule(h.paths.Overlay, instance, slicerEntry(h.dir, true), resource.LeftChange{})
	require.NoError(t, err)
	require.NoError(t, h.store.Reload(t.Context()))

	watched := &watchedOrigins{}
	h.store.WatchModuleOrigins(t.Context(), watched)
	require.Empty(t, watched.last())
}
