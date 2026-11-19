package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// An install answers the state the module reached, read after its start has
// had a moment to finish. Read at once, a module that is about to fail looks
// the same as one on its way up.

// spoolsModule is examples/spools, a module whose process reads one setting.
var spoolsModule = &artefact{pkg: "./examples/spools", name: "spools"}

// refusedSettingsManifest is examples/spools declaring its one setting as text.
// The host sends the text on plugin.configure, and the module, which reads a
// number, refuses it.
func refusedSettingsManifest() string {
	return strings.Join([]string{
		"name: spools",
		"kind: sink",
		"license: MIT",
		"version: 1.0.0",
		"contract_major: 1",
		"platforms:",
		"  - " + runtime.GOOS + "_" + runtime.GOARCH,
		"options:",
		"  - key: shelf_size",
		"    type: string",
		`    default: "lots"`,
		"    summary: How many spools this module keeps, as text the module cannot read.",
		"",
	}, "\n")
}

// aReleaseOfSpools is a release carrying the real examples/spools binary.
func aReleaseOfSpools(t *testing.T, document string) string {
	t.Helper()

	built, _ := spoolsModule.install(t)
	dir := aRelease(t, document, false)

	body, err := os.ReadFile(filepath.Join(built, thisPlatformBinary()))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, thisPlatformBinary()), body, 0o700))

	return dir
}

// A module whose plugin.configure call fails is installed and does not run.
// The answer says so, with the reason, where it said running.
func TestAModuleThatRefusesItsSettingsIsAnsweredAsFailed(t *testing.T) {
	t.Parallel()

	deployment := aLiveDeployment(t)
	deployment.installer.settle = settle

	installed, err := deployment.installer.Install(t.Context(), deployment.principal, httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0",
		From: aReleaseOfSpools(t, refusedSettingsManifest()), Approve: true,
	})
	require.NoError(t, err, "a module that fails to start is still installed")

	require.True(t, installed.Enabled)
	require.False(t, installed.Running, "a module that refused its settings was answered as running")
	require.Equal(t, "failed", installed.State)
	require.Contains(t, installed.Reason, "refused the settings")
	require.False(t, installed.Restart)
}

// missingSettingManifest is a module with one setting nobody gave a value,
// which is a module whose address the operator has to write before it starts.
func missingSettingManifest() string {
	return strings.Join([]string{
		"name: spools",
		"kind: sink",
		"license: MIT",
		"version: 1.0.0",
		"contract_major: 1",
		"platforms:",
		"  - " + runtime.GOOS + "_" + runtime.GOARCH,
		"options:",
		"  - key: url",
		"    type: string",
		"    required: true",
		"    summary: Where the module sends what it reads.",
		"",
	}, "\n")
}

// A required setting nobody wrote stops the start before any process runs, and
// the answer names it.
func TestAModuleMissingARequiredSettingIsAnsweredAsFailed(t *testing.T) {
	t.Parallel()

	deployment := aLiveDeployment(t)
	deployment.installer.settle = settle

	installed, err := deployment.installer.Install(t.Context(), deployment.principal, httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0",
		From: aRelease(t, missingSettingManifest(), true), Approve: true,
	})
	require.NoError(t, err)

	require.False(t, installed.Running)
	require.Equal(t, "failed", installed.State, installed.Reason)
	require.Contains(t, installed.Reason, "url")
}

// A start still going when the wait ends is answered as it stands, and never as
// running.
func TestAStartTheWaitOutlastsIsAnsweredAsStillStarting(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status httpapi.ResourceStatus
		found  bool
		want   string
	}{
		{name: "not supervised yet", want: "pending"},
		{name: "starting", status: httpapi.ResourceStatus{State: "starting"}, found: true, want: "starting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			instance := sinkRef(t, "spools")

			synctest.Test(t, func(t *testing.T) {
				installer := &moduleInstaller{plane: fixedPlane{status: tc.status, found: tc.found}}

				started := installer.startOf(t.Context(), instance, true, false, startSettle)
				require.Equal(t, tc.want, started.state)
				require.False(t, started.running)
				require.Empty(t, started.reason)
			})
		})
	}
}

// fixedPlane answers the same status for every instance and stops nothing.
type fixedPlane struct {
	status httpapi.ResourceStatus
	found  bool
}

func (p fixedPlane) Database(ref.Ref) (wirerpc.DatabaseState, bool) {
	return wirerpc.DatabaseState{}, false
}

func (p fixedPlane) Stop(ref.Ref) bool { return false }

func (p fixedPlane) Restart(ref.Ref) bool { return false }

func (p fixedPlane) Status(ref.Ref) (httpapi.ResourceStatus, bool) { return p.status, p.found }
