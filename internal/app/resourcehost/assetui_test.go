package resourcehost_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/extension/wirehost"
	"github.com/prusa3d/prusa-connect/internal/kernel/condition"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// assetNamespace is the namespace assetType derives, which is where the
// bundle's files and its entry address sit.
const assetNamespace = "com.github.aux.acme.spools"

// theUIBlock is the manifest ADR-20260911-4p6 rule 13 prints, as a module ships it at
// the root of its bundle.
const theUIBlock = `
name: spools
kind: asset
license: MIT
version: 1.4.2
ui:
  entry: index.js
  library: ">=0.3.0 <1.0.0"
  contributes:
    pages:
      - path: spools
        title: Spools
        icon: layers
        hidden: false
    device_card:
      - id: spool-status
        title: Loaded spool
    settings:
      - id: spools
        title: Spools
`

// aBundleWith writes a bundle carrying one manifest and records the digest
// over the whole directory, the way an install does. An empty manifest writes
// no file at all, which is the bundle the core's own app ships.
func aBundleWith(t *testing.T, document string) (string, string) {
	t.Helper()

	dir, _ := aBundle(t)

	if document != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "module.yaml"), []byte(document), 0o600))
	}

	digest, err := wirehost.DigestDir(dir)
	require.NoError(t, err)

	return dir, string(digest)
}

// TestAReadyAssetInstanceCarriesTheUIBlockOfItsManifest is ADR-20260911-4p6 rule 32 at
// the plane. The block the author wrote reaches the listing whole, and the
// entry address is built from the namespace the host derived rather than from
// anything the manifest said.
func TestAReadyAssetInstanceCarriesTheUIBlockOfItsManifest(t *testing.T) {
	t.Parallel()

	w := newWired(t)
	instance := theAsset(t)
	dir, checksum := aBundleWith(t, theUIBlock)

	w.declareAsset(dir, checksum)
	w.waitReady(instance)

	surface, drawn := w.host.UI(instance)
	require.True(t, drawn, "a ready module with a ui block draws nothing")

	require.Equal(t, "/api/v1/modules/"+assetNamespace+"/ui/index.js", surface.EntryURL)
	require.Equal(t, ">=0.3.0 <1.0.0", surface.Library)
	require.Equal(t, []httpapi.ResourcePage{{Path: "spools", Title: "Spools", Icon: "layers"}},
		surface.Contributes.Pages)
	require.Equal(t, []httpapi.ResourceSection{{ID: "spool-status", Title: "Loaded spool"}},
		surface.Contributes.DeviceCard)
	require.Equal(t, []httpapi.ResourceSection{{ID: "spools", Title: "Spools"}},
		surface.Contributes.Settings)
}

// devicesBlock is a manifest with a device tab and a device action that
// show only on the printer the server runs on, beside a tab that shows on every
// printer.
const devicesBlock = `
name: spools
kind: asset
license: MIT
version: 1.4.2
ui:
  entry: index.js
  library: ">=0.3.0 <1.0.0"
  contributes:
    device_tab:
      - id: display
        title: Display
        devices:
          resident: true
      - id: log
        title: Log
    device_action:
      - id: wake
        title: Wake
        devices:
          resident: true
`

// TestAReadyAssetInstanceCarriesTheDevicesSelector is ADR-20261110-5np rule 7.
// The selector reaches the listing on the entry that set it, and an entry that
// left it out has no devices key in the JSON.
func TestAReadyAssetInstanceCarriesTheDevicesSelector(t *testing.T) {
	t.Parallel()

	w := newWired(t)
	instance := theAsset(t)
	dir, checksum := aBundleWith(t, devicesBlock)

	w.declareAsset(dir, checksum)
	w.waitReady(instance)

	surface, drawn := w.host.UI(instance)
	require.True(t, drawn, "a ready module with a ui block draws nothing")

	require.Equal(t, []httpapi.ResourceSection{
		{ID: "display", Title: "Display", Devices: &httpapi.ResourceDeviceSelector{Resident: true}},
		{ID: "log", Title: "Log"},
	}, surface.Contributes.DeviceTab)
	require.Equal(t, []httpapi.ResourceAction{
		{ID: "wake", Title: "Wake", Devices: &httpapi.ResourceDeviceSelector{Resident: true}},
	}, surface.Contributes.DeviceAction)

	wire, err := json.Marshal(surface.Contributes.DeviceTab)
	require.NoError(t, err)
	require.JSONEq(t,
		`[{"id":"display","title":"Display","devices":{"resident":true}},{"id":"log","title":"Log"}]`,
		string(wire))
}

// TestABundleThatDeclaresNoUIBlockDrawsNothing covers the two bundles that
// fill no slot: the core's own app, which ships no manifest at all, and a
// module whose manifest carries every other key and no ui block. Both serve
// their files and neither appears in the listing as drawing anything.
func TestABundleThatDeclaresNoUIBlockDrawsNothing(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"no manifest at all":     "",
		"a manifest with no ui":  "name: spools\nkind: asset\nlicense: MIT\nversion: 1.4.2\n",
		"a manifest with no key": "name: spools\nkind: asset\nlicense: MIT\n",
	}

	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := newWired(t)
			instance := theAsset(t)
			dir, checksum := aBundleWith(t, document)

			w.declareAsset(dir, checksum)
			w.waitReady(instance)

			_, drawn := w.host.UI(instance)
			require.False(t, drawn, "a bundle that declares no ui block was published as drawing one")

			_, _, served := bundleOf(w, instance)
			require.True(t, served, "the bundle's files are not served")
		})
	}
}

// TestABundleWhoseManifestIsRefusedIsNeverServed holds the manifest the way
// the digest is held. A bundle whose module.yaml this host will not read is a
// bundle nobody can say what it contributes, so nothing of it is served and
// waiting does not fix it.
func TestABundleWhoseManifestIsRefusedIsNeverServed(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"a key this version does not read": "name: spools\nkind: asset\nlicense: MIT\nnamespace: com.acme\n",
		"an entry that climbs out":         uiWith(`entry: ../index.js`),
		"an entry the app cannot import":   uiWith(`entry: index.html`),
		"a library range no version holds": uiWith("entry: index.js\n  library: \">=1.0.0 <0.3.0\""),
		"a page path with a slash": uiWith("entry: index.js\n  library: \">=0.3.0 <1.0.0\"\n" +
			"  contributes:\n    pages:\n      - path: spools/loaded\n        title: Spools"),
		"a document that is not a mapping": "- name: spools\n",
	}

	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := newWired(t)
			instance := theAsset(t)
			dir, checksum := aBundleWith(t, document)

			w.declareAsset(dir, checksum)

			status := w.waitState(instance, condition.StateFailed())
			require.True(t, says(status, "RESOURCE_MANIFEST_INVALID"), conditionsOf(&status))
			require.False(t, status.Serving)

			_, _, served := bundleOf(w, instance)
			require.False(t, served, "a bundle whose manifest was refused reached the mount")

			_, drawn := w.host.UI(instance)
			require.False(t, drawn)
		})
	}
}

// TestAnInstanceThePlaneDoesNotHoldDrawsNothing is the other side of the
// readiness rule: a name nothing is supervising has no bundle behind it, so
// the listing publishes no entry address for it.
func TestAnInstanceThePlaneDoesNotHoldDrawsNothing(t *testing.T) {
	t.Parallel()

	w := newWired(t)

	_, drawn := w.host.UI(theAsset(t))
	require.False(t, drawn, "an instance the plane is not supervising draws a ui block")
}

// uiWith writes the smallest manifest carrying one ui block, so a case says
// what it is testing and nothing else.
func uiWith(block string) string {
	return "name: spools\nkind: asset\nlicense: MIT\nui:\n  " + block + "\n"
}

// themesBlock is a manifest that offers one stylesheet as a look for the app.
const themesBlock = `
name: ocean-theme
kind: asset
license: MIT
version: 0.1.0
ui:
  entry: index.js
  library: ">=0.3.0 <1.0.0"
  themes:
    - id: ocean
      title: Ocean
      stylesheet: ui/ocean.css
`

// TestAThemeReachesTheListingWithItsStylesheetAddress holds the stylesheet
// address to the namespace the host derived, as the entry address is.
func TestAThemeReachesTheListingWithItsStylesheetAddress(t *testing.T) {
	t.Parallel()

	w := newWired(t)
	instance := theAsset(t)
	dir, checksum := aBundleWith(t, themesBlock)

	w.declareAsset(dir, checksum)
	w.waitReady(instance)

	surface, drawn := w.host.UI(instance)
	require.True(t, drawn, "a ready module with a ui block draws nothing")
	require.Equal(t, []httpapi.ResourceUITheme{{
		ID:            "ocean",
		Title:         "Ocean",
		StylesheetURL: "/api/v1/modules/" + assetNamespace + "/ui/ui/ocean.css",
	}}, surface.Themes)
}
