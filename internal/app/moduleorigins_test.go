package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

const slicerPath = "github.com/acme/nozzle-watch"

// originManifest is a module that only draws and asks the API to answer the
// pages of one other site.
func originManifest(origins ...string) string {
	return bundleOnlyManifest() + "allowed_origins:\n  - " + strings.Join(origins, "\n  - ") + "\n"
}

// originProcessManifest is a module with a process that asks for an origin
// and for nothing else an approval records.
func originProcessManifest() string {
	return noPermissionManifest() + "allowed_origins:\n  - " + slicerOrigin + "\n"
}

func installSlicer(
	t *testing.T, core config.Core, configPath string, approve bool, document string,
) (httpapi.ModuleInstallation, error) {
	t.Helper()

	host, closeHost, err := openGraph(t.Context(), core, "", quiet())
	require.NoError(t, err)

	defer closeHost()

	installer := newModuleInstaller(core, configPath, host.graph, nil, nil, host.graph, discard())

	return installer.Install(t.Context(), "user:"+core.Principal, httpapi.ModuleRelease{
		Module: slicerPath, Version: "2.1.0", From: aRelease(t, document, false), Approve: approve,
	})
}

func declaredSlicer(t *testing.T, core config.Core, configPath string) resource.Declaration {
	t.Helper()

	doc, err := resource.Load(resource.Paths{File: configPath, Overlay: core.Overlay})
	require.NoError(t, err)

	instance, err := ref.New(ref.KindAsset, "nozzle-watch")
	require.NoError(t, err)

	declared, found := doc.Find(instance)
	require.True(t, found)

	return declared
}

// An origin is a request. An install nobody approved lists it and writes
// none, and an approved one writes it into the declaration of a module that
// runs no process.
func TestAnOriginIsGrantedOnlyByAnApprovedInstall(t *testing.T) {
	t.Parallel()

	configPath, core := installConfig(t)

	held, err := installSlicer(t, core, configPath, false, originManifest(slicerOrigin))
	require.NoError(t, err)
	require.False(t, held.Enabled)
	require.Contains(t, held.Requests, "origin "+slicerOrigin)
	require.Empty(t, declaredSlicer(t, core, configPath).Spec.Module.AllowedOrigins)

	held, err = installSlicer(t, core, configPath, true, originManifest(slicerOrigin))
	require.NoError(t, err)
	require.True(t, held.Enabled)
	require.Contains(t, held.Approved, "origin "+slicerOrigin)

	declared := declaredSlicer(t, core, configPath)
	require.Equal(t, []string{slicerOrigin}, declared.Spec.Module.AllowedOrigins)
	require.Equal(t, []string{slicerOrigin}, declared.Spec.Module.Asked.AllowedOrigins)
}

// Approving an origin lets another site act for whoever opens it, so it takes
// the standing every other approval takes: a caller who may only configure
// the kind is refused before anything is written.
func TestApprovingAnOriginNeedsTheOrganization(t *testing.T) {
	t.Parallel()

	configPath, core := installConfig(t)

	_, err := installManifestOverHTTP(t, core, configPath, aKindAdmin(t, core), true, originProcessManifest())
	requireInstallDenied(t, err)

	rows, err := resource.Installed(resource.Paths{File: configPath, Overlay: core.Overlay})
	require.NoError(t, err)
	require.Empty(t, rows, "the refused install left a declaration behind")
}

// A release that asks for an origin the operator was not shown is refused,
// as one that asks for a permission they were not shown is.
func TestAnOriginThePageDidNotShowIsRefused(t *testing.T) {
	t.Parallel()

	configPath, core := installConfig(t)

	host, closeHost, err := openGraph(t.Context(), core, "", quiet())
	require.NoError(t, err)

	defer closeHost()

	installer := newModuleInstaller(core, configPath, host.graph, nil, nil, host.graph, discard())

	_, err = installer.Install(t.Context(), "user:"+core.Principal, httpapi.ModuleRelease{
		Module: slicerPath, Version: "2.1.0", Approve: true,
		From:   aRelease(t, originManifest(slicerOrigin, "https://other.example"), false),
		Expect: &httpapi.ModuleRequests{AllowedOrigins: []string{slicerOrigin}},
	})
	require.ErrorContains(t, err, "origin https://other.example")
}

// An update keeps what the operator decided: an origin they took away from
// the declaration stays away, and one the release adds is granted and named.
func TestAnUpdateKeepsAnOriginTheOperatorTookAway(t *testing.T) {
	t.Parallel()

	kept, added := keptOrigins(
		[]string{"https://a.example", "https://b.example", "https://c.example"},
		[]string{"https://a.example", "https://b.example"},
		[]string{"https://a.example"},
		nil,
	)

	require.Equal(t, []string{"https://a.example", "https://c.example"}, kept)
	require.Equal(t, []string{"origin https://c.example"}, added)
}
