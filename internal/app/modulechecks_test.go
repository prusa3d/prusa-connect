package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// checkingManifest is the two-halves module asking to ask whether a person
// may operate a printer, without operating one itself (ADR-20261106-jnb).
func checkingManifest() string {
	return strings.Replace(bothHalvesManifest(), "permissions:\n", "checks:\n  - operate\npermissions:\n", 1)
}

// An approved install records the check in the declaration and the ask, and
// the command says so. It plans no grant for it, so nothing is withheld.
func TestAnApprovedInstallGivesTheChecksAndWithholdsNothing(t *testing.T) {
	t.Parallel()

	configPath, core := installConfig(t)
	said := addModuleFrom(t, configPath, theModulePath+"@1.0.0", aRelease(t, checkingManifest(), true), "-approve")

	doc, err := resource.Load(resource.Paths{File: configPath, Overlay: core.Overlay})
	require.NoError(t, err)
	require.Empty(t, doc.Failures)

	instance, err := ref.New(ref.KindSink, "spools")
	require.NoError(t, err)

	declared, found := doc.Find(instance)
	require.True(t, found)

	require.Equal(t, []authz.Action{authz.ActionOperate}, declared.Spec.Service.Checks)
	require.Equal(t, []authz.Action{authz.ActionObserve}, declared.Spec.Service.Permissions)
	require.Equal(t, []string{"operate"}, declared.Spec.Module.Asked.Checks)
	require.Contains(t, said, "may check   whether a person may operate (checks)")
	require.NotContains(t, said, "not granted")
}

// An install over HTTP says it gave the check with approve and withholds
// nothing, and lists it among the requests without approve.
func TestAnInstallOverHTTPSaysItGaveTheChecks(t *testing.T) {
	t.Parallel()

	for _, approve := range []bool{true, false} {
		configPath, core := installConfig(t)

		host, closeHost, err := openGraph(t.Context(), core, "", quiet())
		require.NoError(t, err)

		installer := newModuleInstaller(core, configPath, host.graph, nil, nil, host.graph, discard())

		held, err := installer.Install(t.Context(), "user:"+core.Principal, httpapi.ModuleRelease{
			Module: theModulePath, Version: "1.0.0", From: aRelease(t, checkingManifest(), true), Approve: approve,
		})
		closeHost()
		require.NoError(t, err)
		require.Empty(t, held.Withheld)

		if approve {
			require.ElementsMatch(t, []string{"permission observe", "check operate"}, held.Approved)

			continue
		}

		require.Empty(t, held.Approved)
		require.Contains(t, held.Requests, "check operate")
	}
}

// A release that checks an action the operator was not shown is refused, and
// the ask goes through the API's spelling and back unchanged.
func TestChecksAreHeldToWhatTheOperatorWasShown(t *testing.T) {
	t.Parallel()

	asked := install.ModuleAsks{Checks: []string{"operate"}}

	require.Equal(t, []string{"check operate"}, beyondAsks(asked, install.ModuleAsks{}, theNamespace))
	require.Empty(t, beyondAsks(asked, asked, theNamespace))

	requests := resource.RequestsOf(asked)
	require.Equal(t, []string{"operate"}, requests.Checks)
	require.Equal(t, []string{"operate"}, resource.AsksOf(requests).Checks)
}

// An approved update keeps a check the operator left, drops one they took
// away, and names a check the release before did not ask.
func TestAnUpdateKeepsTheChecksTheOperatorLeft(t *testing.T) {
	t.Parallel()

	kept, added := keptLines([]string{"operate", "configure"}, []string{"operate", "configure"},
		[]string{"operate"}, nil, checkRequest)
	require.Equal(t, []string{"operate"}, kept)
	require.Empty(t, added)

	kept, added = keptLines([]string{"operate"}, nil, nil, nil, checkRequest)
	require.Equal(t, []string{"operate"}, kept)
	require.Equal(t, []string{"check operate"}, added)
}
