package app

import (
	"slices"
	"strings"

	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/app/resourcehost"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// modulePlane is the module plane as the HTTP surface reads it: a process
// module's routes from the host's own claim on the namespace, and an asset
// module's files read back by kind and mounted under the same namespace.
//
// The asset half lives here and not in the host because this is the reader
// the kind is held to (ref.Kind.Read): the bundle is verified by the plane
// and served by the transport, and the composition root is where the two are
// joined. A bundle is resolved per request, so an instance replaced between
// two requests is never reached through a pointer somebody kept.
type modulePlane struct {
	host *resourcehost.Host
	// store is the declared inventory, read for a root prefix the host holds
	// no claim on. A module that is declared and never started claims nothing,
	// and without this the address it declared would answer as though this
	// server had never heard of it.
	store *resource.Store
}

var _ httpapi.ModulePlane = modulePlane{}

// Module answers which surface claims one namespace. A process module wins
// where both could, and cannot in fact both exist: the host refuses a second
// instance on one namespace before either starts.
func (p modulePlane) Module(namespace string) (httpapi.ModuleSurface, bool) {
	if surface, ok := p.host.Module(namespace); ok {
		return surface, true
	}

	for instance, runner := range p.host.Supervising(ref.KindAsset) {
		held, ui, ok := resourcehost.UIOf(runner)
		if !ok || held != namespace {
			continue
		}

		surface := httpapi.ModuleSurface{Instance: instance, Handler: httpapi.ModuleUIHandler(ui)}
		if drawn, offers := p.host.UI(instance); offers {
			surface.Themes = httpapi.ThemeFiles(namespace, drawn)
		}

		return surface, true
	}

	return httpapi.ModuleSurface{}, false
}

// IsolatedTab reports whether an enabled, running module of the namespace
// declares this printer tab with cross_origin_isolated and nobody hides it.
// It reads no caller, because the entry document is the same for every
// viewer. It asks the host on every call, so a change to the module shows at
// the next request.
func (p modulePlane) IsolatedTab(namespace, tab string) bool {
	if p.host == nil || p.store == nil {
		return false
	}

	return isolatedAmong(p.store.Document().Instances, p.host.UI, namespace, tab)
}

// isolatedAmong is IsolatedTab over a list of declarations. Only the
// instance the host runs as the namespace's module answers.
func isolatedAmong(
	instances []resource.Declaration, uiOf func(ref.Ref) (httpapi.ResourceUI, bool), namespace, tab string,
) bool {
	for _, declared := range instances {
		if owner, isolated := tabIsolation(declared, uiOf, namespace, tab); owner {
			return isolated
		}
	}

	return false
}

// tabIsolation reads one declared instance. owner is whether it is the
// enabled module of the namespace that the host runs. isolated is whether its
// bundle declares the tab with cross_origin_isolated and nobody hides it. The
// devices selector is not read, so the tab is isolated on every printer.
func tabIsolation(
	declared resource.Declaration, uiOf func(ref.Ref) (httpapi.ResourceUI, bool), namespace, tab string,
) (owner, isolated bool) {
	held, isModule := declared.ModuleNamespace()
	if !isModule || held != namespace || !declared.Spec.Enabled {
		return false, false
	}

	surface, running := uiOf(declared.Ref)
	if !running {
		return false, false
	}

	hidden := manifest.SlotDeviceTab.Key() + "/" + tab
	if slices.Contains(declared.Spec.HiddenContributions, hidden) || slices.Contains(surface.Hidden, hidden) {
		return true, false
	}

	for _, entry := range surface.Contributes.DeviceTab {
		if entry.ID == tab {
			return true, entry.CrossOriginIsolated
		}
	}

	return true, false
}

// Root is the surface a root-mounted path belongs to. An asset instance
// declares no root mount, so no bundle is ever served at one.
//
// The host answers first, for every prefix a running instance claimed. What is
// left is a prefix that was declared and never claimed, because the instance is
// switched off or never started. That prefix answers 503 and not the router's
// not-found, so an integration can tell "switched off here" from "wrong
// server".
func (p modulePlane) Root(path string) (httpapi.ModuleSurface, bool) {
	if surface, claimed := p.host.Root(path); claimed {
		return surface, true
	}

	return p.declaredRoot(path)
}

// declaredRoot is the prefix a declaration names and nothing is running on.
func (p modulePlane) declaredRoot(path string) (httpapi.ModuleSurface, bool) {
	if p.store == nil {
		return httpapi.ModuleSurface{}, false
	}

	for _, declared := range p.store.Document().Instances {
		mount := declared.ModuleMount()
		if mount == "" || (path != mount && !strings.HasPrefix(path, mount+"/")) {
			continue
		}

		return httpapi.ModuleSurface{
			Instance:    declared.Ref,
			Mount:       mount,
			Unavailable: declaredReason(declared),
			ModulesOff:  p.store.Held(declared),
			SafeMode:    p.store.SafeMode(),
		}, true
	}

	return httpapi.ModuleSurface{}, false
}

// declaredReason says why a declared module is not answering on the prefix it
// named, in the words an operator can act on. It names nothing from disk: the
// refusal is reachable without a credential.
func declaredReason(declared resource.Declaration) string {
	if !declared.Spec.Enabled {
		return "the module that claims this address is declared and switched off. " +
			"Declare it enabled to start it"
	}

	return "the module that claims this address is switched on and is not running. " +
		"A module that was never installed stays there until `prusa-connect module add` " +
		"puts its files in place"
}
