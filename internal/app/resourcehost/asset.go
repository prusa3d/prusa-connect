package resourcehost

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/extension/supervisor"
	"github.com/prusa3d/prusa-connect/internal/extension/wirehost"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// errNoAsset reports an instance whose runtime is asset and which carries no
// asset block. The loader refuses that declaration, so reaching it here is a
// defect in this host and not a mistake in the operator's file.
var errNoAsset = errors.New("resourcehost: the instance carries no asset block")

// assetSpec is the declaration of an instance whose runtime is asset: the
// namespace its files are served under, and where they are. It is separate
// from spec for the same reason as serviceSpec: no asset type is registered.
type assetSpec struct {
	namespace string
	asset     install.Asset
}

// assetOf is the asset half of a declaration, and nil for every instance
// reached any other way.
func assetOf(declared resource.Declaration) *assetSpec {
	if declared.Spec.Runtime != install.RuntimeAsset || declared.Spec.Asset == nil {
		return nil
	}

	namespace, isModule := declared.ModuleNamespace()
	if !isModule {
		return nil
	}

	return &assetSpec{namespace: namespace, asset: *declared.Spec.Asset}
}

// buildAsset verifies one bundle and opens it for serving. No process is
// launched. The target is the directory, opened as an os.Root so no file name
// can lead outside it.
//
// The two refusals carry the codes a service launch carries for the same two
// conditions, because the operator fixes them the same way: install the
// bundle where the declaration says, or reinstall it so the recorded digest
// matches the files. A missing directory is retried, because an install may
// still put it there. A digest that does not match is not retried.
func buildAsset(
	startCtx context.Context, handle supervisor.Handle, instance ref.Ref, declared *assetSpec,
) (any, error) {
	if declared == nil {
		return nil, fmt.Errorf("%w: %s", errNoAsset, instance)
	}

	bundle, err := openBundle(startCtx, handle, instance, declared.namespace, declared.asset)
	if err != nil {
		return nil, err
	}

	return bundle, nil
}

// openBundle verifies one directory of files and opens it for serving. Two
// declarations reach it: an asset instance, which is a module that runs no
// process, and the assets block of a module that runs one. In both cases the
// files are served under the namespace.
func openBundle(
	startCtx context.Context, handle supervisor.Handle,
	instance ref.Ref, namespace string, declared install.Asset,
) (*assetBundle, error) {
	dir := declared.Path

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("%s is not a directory", dir)
		}

		return nil, pluginMissing.New("no bundle to serve").
			With(fault.StringParam("resource", instance.String()), fault.StringParam("path", dir)).
			Wrap(err)
	}

	if err = wirehost.Digest(declared.Checksum).VerifyDir(dir); err != nil {
		return nil, pluginChecksum.New("the bundle does not match its recorded checksum").
			With(fault.StringParam("resource", instance.String()), fault.StringParam("path", dir)).
			Wrap(err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open the bundle %s: %w", dir, err)
	}

	surface, err := bundleUI(root, namespace)
	if err != nil {
		_ = root.Close()

		return nil, manifestInvalid.New("the module's manifest was not read").
			With(fault.StringParam("resource", instance.String()),
				fault.StringParam("path", dir),
				fault.StringParam("reason", err.Error())).
			Wrap(err)
	}

	handle.Logger.InfoContext(startCtx, "the module's files are serving",
		"resource", instance.String(),
		"path", dir,
		"mount", httpapi.ModuleUIPath(namespace),
		"draws", surface != nil)

	return &assetBundle{
		root:      root,
		namespace: namespace,
		ui:        httpapi.ModuleUI{Files: root.FS(), Digest: declared.Checksum},
		surface:   surface,
	}, nil
}

// manifestFile is where a module keeps its manifest inside its bundle. The
// digest covers every file in the directory, so the manifest the host reads
// here is the one the install recorded.
const manifestFile = "module.yaml"

// bundleUI reads the ui block out of the bundle's own manifest and renders it
// as the listing publishes it.
//
// A bundle with no manifest draws nothing and is not an error. The core's own
// app is an asset module and it fills no slot: it is the app the slots are
// in.
func bundleUI(root *os.Root, namespace string) (*httpapi.ResourceUI, error) {
	content, err := fs.ReadFile(root.FS(), manifestFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // a bundle with no manifest draws nothing, and that is not a failure.
	}

	if err != nil {
		return nil, fmt.Errorf("read the bundle's %s: %w", manifestFile, err)
	}

	declared, err := manifest.Parse(manifestFile, content, yaml.Unmarshal)
	if err != nil {
		return nil, err //nolint:wrapcheck // the refusal already names the file and the key.
	}

	if declared.UI == nil {
		return nil, nil //nolint:nilnil // a module that declares no ui block draws nothing.
	}

	entry := ""
	if declared.UI.Entry != "" {
		entry = httpapi.ModuleUIPath(namespace) + declared.UI.Entry
	}

	return &httpapi.ResourceUI{
		EntryURL:    entry,
		Library:     declared.UI.Library,
		Contributes: contributesOf(declared.UI.Contributes),
		Exports:     exportsOf(declared.UI.Exports),
		Imports:     importsOf(declared.UI.Imports),
		Themes:      themesOf(namespace, declared.UI.Themes),
	}, nil
}

// themesOf renders the themes a bundle offers, each with the address its
// stylesheet is served at.
func themesOf(namespace string, declared []manifest.Theme) []httpapi.ResourceUITheme {
	out := make([]httpapi.ResourceUITheme, 0, len(declared))
	for _, theme := range declared {
		out = append(out, httpapi.ResourceUITheme{
			ID:            theme.ID,
			Title:         theme.Title,
			StylesheetURL: httpapi.ModuleUIPath(namespace) + theme.Stylesheet,
		})
	}

	return out
}

// exportsOf renders the components a bundle exports.
func exportsOf(declared []manifest.UIExport) []httpapi.ResourceUIExport {
	out := make([]httpapi.ResourceUIExport, 0, len(declared))
	for _, export := range declared {
		out = append(out, httpapi.ResourceUIExport{Name: export.Name})
	}

	return out
}

// importsOf renders the components a bundle imports.
func importsOf(declared []manifest.UIImport) []httpapi.ResourceUIImport {
	out := make([]httpapi.ResourceUIImport, 0, len(declared))
	for _, imported := range declared {
		out = append(out, httpapi.ResourceUIImport{Module: imported.Module, Name: imported.Name})
	}

	return out
}

// pagesOf renders the pages list on the wire.
func pagesOf(declared []manifest.Page) []httpapi.ResourcePage {
	if len(declared) == 0 {
		return nil
	}

	out := make([]httpapi.ResourcePage, 0, len(declared))
	for _, page := range declared {
		out = append(out, httpapi.ResourcePage{
			Path:   page.Path,
			Title:  page.Title,
			Icon:   string(page.Icon),
			Hidden: page.Hidden,
		})
	}

	return out
}

// sectionsOf renders one id-and-title list on the wire.
func sectionsOf(declared []manifest.Section) []httpapi.ResourceSection {
	if len(declared) == 0 {
		return nil
	}

	out := make([]httpapi.ResourceSection, 0, len(declared))
	for _, section := range declared {
		out = append(out, httpapi.ResourceSection{
			ID:                  section.ID,
			Title:               section.Title,
			FileTypes:           section.FileTypes,
			Devices:             devicesOf(section.Devices),
			CrossOriginIsolated: section.CrossOriginIsolated,
		})
	}

	return out
}

// devicesOf is an entry's devices selector on the wire, and nil for an entry
// shown on every device, which leaves the key out.
func devicesOf(selector manifest.DeviceSelector) *httpapi.ResourceDeviceSelector {
	if selector.IsZero() {
		return nil
	}

	return &httpapi.ResourceDeviceSelector{Resident: selector.Resident}
}

// actionsOf renders one run-button list on the wire.
func actionsOf(declared []manifest.Action) []httpapi.ResourceAction {
	if len(declared) == 0 {
		return nil
	}

	out := make([]httpapi.ResourceAction, 0, len(declared))
	for _, action := range declared {
		out = append(out, httpapi.ResourceAction{
			ID:          action.ID,
			Title:       action.Title,
			Icon:        string(action.Icon),
			Destructive: action.Destructive,
			FileTypes:   action.FileTypes,
			Primary:     action.Primary,
			Devices:     devicesOf(action.Devices),
		})
	}

	return out
}

// cardsOf renders the card list on the wire.
func cardsOf(declared []manifest.Card) []httpapi.ResourceCard {
	if len(declared) == 0 {
		return nil
	}

	out := make([]httpapi.ResourceCard, 0, len(declared))
	for _, card := range declared {
		settings := make([]httpapi.ResourceCardSetting, 0, len(card.Settings))
		for _, setting := range card.Settings {
			var options []httpapi.ResourceSettingOption
			for _, option := range setting.Options {
				options = append(options, httpapi.ResourceSettingOption{Value: option.Value, Label: option.Label})
			}

			settings = append(settings, httpapi.ResourceCardSetting{
				Key:      setting.Key,
				Kind:     string(setting.Kind),
				Label:    setting.Label,
				Default:  setting.Default,
				Options:  options,
				Min:      setting.Min,
				Max:      setting.Max,
				Step:     setting.Step,
				MaxBytes: setting.MaxBytes,
			})
		}

		out = append(out, httpapi.ResourceCard{
			ID:          card.ID,
			Title:       card.Title,
			Description: card.Description,
			Icon:        string(card.Icon),
			Size:        httpapi.ResourceCardSize{W: card.Size.W, H: card.Size.H},
			Least:       httpapi.ResourceCardSize{W: card.Least.W, H: card.Least.H},
			Settings:    settings,
		})
	}

	return out
}

// assetBundle is one verified bundle the host holds open: the root every file
// is opened through, and what the mount and the listing serve from it.
type assetBundle struct {
	root      *os.Root
	namespace string
	ui        httpapi.ModuleUI
	// surface is the ui block of the bundle's own manifest, rendered for the
	// listing, and nil for a bundle that declares none.
	surface *httpapi.ResourceUI
}

// Close releases the directory. A file opened through the root before this
// stays readable until it is closed, so an open request can finish while a
// reinstall replaces the instance.
func (b *assetBundle) Close() error {
	if err := b.root.Close(); err != nil {
		return fmt.Errorf("close the bundle: %w", err)
	}

	return nil
}

// UIOf is the verified bundle a running instance serves, and false for an
// instance that has none, is between starts, or failed its digest.
//
// It is read from the target with Peek and not through a call, because a
// bundle is set at start and no call is made on it. A directory that failed
// verification has no target, so none of its files is served. The caller
// reads every asset instance through it to mount the files under the module's
// namespace.
func UIOf(runner *supervisor.Instance[any]) (string, httpapi.ModuleUI, bool) {
	bundle, ok := supervisor.Peek(runner, bundleTarget)
	if !ok || bundle == nil {
		return "", httpapi.ModuleUI{}, false
	}

	return bundle.namespace, bundle.ui, true
}

// uiOf is the ui block a running instance declared, for the listing, and
// false for an instance that declared none. Only the host's UI method calls
// it.
//
// It answers for both kinds of bundle: an asset instance, and the assets
// block of a module that runs a process.
func uiOf(runner *supervisor.Instance[any]) (httpapi.ResourceUI, bool) {
	surface, ok := supervisor.Peek(runner, func(target any) *httpapi.ResourceUI {
		if held := bundleTarget(target); held != nil {
			return held.surface
		}

		return nil
	})
	if !ok || surface == nil {
		return httpapi.ResourceUI{}, false
	}

	return *surface, true
}

// bundleTarget is the verified bundle a supervised target holds, and nil for a
// target that holds none.
func bundleTarget(target any) *assetBundle {
	switch held := target.(type) {
	case *assetBundle:
		return held
	case *serviceChild:
		return held.bundle
	default:
		return nil
	}
}
