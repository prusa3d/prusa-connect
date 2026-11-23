package resource_test

import (
	"context"
	"encoding/json"
	"fmt"
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
// it, with one origin granted.
func slicerEntry(dir string, enabled bool) install.Entry {
	return install.Entry{
		Type:    "com.github.acme.slicer/slicer",
		Enabled: &enabled,
		Runtime: install.RuntimeAsset.String(),
		Asset:   &install.AssetEntry{Path: dir, Checksum: "sha256:" + sixtyFourZeros},
		Module: &install.ModuleEntry{
			Title:          "Web slicer",
			AllowedOrigins: []string{"https://slicer.example"},
		},
	}
}

const sixtyFourZeros = "0000000000000000000000000000000000000000000000000000000000000000"

// A module's origins hold while it is switched on and leave with the write
// that switches it off or removes it.
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

// A module the operator declares in their own file is held off in memory when
// the host stops retrying it, and its origin leaves with that hold.
func TestAModuleTheHostHoldsOffAddsNoOrigin(t *testing.T) {
	t.Parallel()

	farm := mustRef(t, ref.KindSink, "farm")
	h := newHarness(t)
	h.writeFile(operatorsFarm + "      module:\n        allowed_origins: [https://slicer.example]\n")
	require.NoError(t, h.store.Reload(t.Context()))

	watched := &watchedOrigins{}
	h.store.WatchModuleOrigins(t.Context(), watched)
	require.Len(t, watched.last(), 1)

	require.NoError(t, h.store.SwitchOffByHost(t.Context(), farm, threeStops()))
	require.Empty(t, watched.last(), "a module the host holds off still adds its origin")
}

// An origin an install never writes is a hand edit of a declaration, and the
// instance is refused with the reason in either file.
func TestADeclaredOriginNoInstallWritesRefusesItsInstance(t *testing.T) {
	t.Parallel()

	for name, origins := range map[string]string{
		"http":     "[http://slicer.example]",
		"wildcard": "['https://*.slicer.example']",
		"repeat":   "[https://slicer.example, https://slicer.example]",
		"too many": "[https://a.example, https://b.example, https://c.example, https://d.example, " +
			"https://e.example, https://f.example, https://g.example, https://h.example, https://i.example]",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.writeFile(operatorsFarm + "      module:\n        allowed_origins: " + origins + "\n")

			doc, err := resource.Load(h.paths)
			require.NoError(t, err)
			require.Empty(t, doc.Instances)
			require.Len(t, doc.Failures, 1)
			require.ErrorContains(t, doc.Failures[0].Err, "module.allowed_origins")
		})
	}
}

// An install and a settings write that overlap each keep the other's change
// to the machine-owned file.
func TestAnInstallAndAStoredListKeepEachOther(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	done := make(chan error, 1)

	const rounds = 40

	go func() {
		for i := range rounds {
			instance, err := ref.New(ref.KindAsset, fmt.Sprintf("slicer%d", i))
			if err != nil {
				done <- err

				return
			}

			if _, err = resource.InstallModule(h.paths.Overlay, instance, slicerEntry(h.dir, true),
				resource.LeftChange{}); err != nil {
				done <- err

				return
			}
		}

		done <- nil
	}()

	for i := range rounds {
		require.NoError(t, h.store.RecordAllowedOrigins([]string{fmt.Sprintf("https://s%d.example", i)}))
	}

	require.NoError(t, <-done)

	stored, err := h.store.AllowedOrigins()
	require.NoError(t, err)
	require.Equal(t, []string{fmt.Sprintf("https://s%d.example", rounds-1)}, stored)

	content, err := os.ReadFile(h.paths.Overlay)
	require.NoError(t, err)

	for i := range rounds {
		require.Contains(t, string(content), fmt.Sprintf(`"slicer%d"`, i), "an install was written over")
	}
}
