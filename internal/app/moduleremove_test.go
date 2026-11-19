package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// What an uninstall has to do, and in what order: stop the module's process,
// delete its files, and only then take its declaration out of the overlay.
//
// The order is the whole of the fix. A declaration removed first and a delete
// that then fails leaves a deployment with files nobody will collect and a
// second attempt that reports the module was never installed.

// errHeld is the delete a running executable answers with. Windows reports
// Access is denied while anything holds the file.
var errHeld = errors.New("access is denied")

// anInstalledModule installs the module both halves of these tests remove, and
// answers the installer, its overlay paths and the declaration that was written.
func anInstalledModule(
	t *testing.T, plane runningModules,
) (*moduleInstaller, resource.Paths, resource.Declaration) {
	t.Helper()

	installer, configPath := theInstallerOver(t, true, plane)

	_, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: aRelease(t, bothHalvesManifest(), true),
		Approve: true,
	})
	require.NoError(t, err)

	paths := resource.Paths{File: configPath, Overlay: installer.core.Overlay}

	rows, err := resource.Installed(paths)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	return installer, paths, rows[0]
}

func TestADeleteThatFailsOnceIsTriedAgain(t *testing.T) {
	t.Parallel()

	installer, paths, found := anInstalledModule(t, nil)

	attempts := 0

	done, err := removeDeclared(t.Context(), moduleRemoval{
		core: installer.core, configPath: paths.File, found: found,
		pause: time.Millisecond,
		remove: func(path string) error {
			attempts++
			if attempts == 1 {
				return errHeld
			}

			return os.RemoveAll(path)
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts, "the first refusal was not tried again")
	require.NoDirExists(t, done.root)

	rows, err := resource.Installed(paths)
	require.NoError(t, err)
	require.Empty(t, rows, "the declaration outlived the files it points at")
}

func TestADeleteThatKeepsFailingKeepsTheModuleInstalled(t *testing.T) {
	t.Parallel()

	installer, paths, found := anInstalledModule(t, nil)

	_, err := removeDeclared(t.Context(), moduleRemoval{
		core: installer.core, configPath: paths.File, found: found,
		pause:  time.Microsecond,
		remove: func(string) error { return errHeld },
	})
	require.Error(t, err)

	var raised *fault.Fault

	require.ErrorAs(t, err, &raised)
	require.Equal(t, "core.resource/MODULE_FILES_HELD", raised.Code().String())

	root := filepath.Join(installer.core.Modules, found.Ref.ID())

	named, carried := raised.Params().Get("path")
	require.True(t, carried, "the refusal does not say which directory is still there")
	require.Equal(t, filepath.ToSlash(root), named.String())

	rows, err := resource.Installed(paths)
	require.NoError(t, err)
	require.Len(t, rows, 1, "the module was declared gone and its files are still here")
	require.DirExists(t, root)
}

// stoppedPlane is a supervision plane that records what it was asked to stop
// and what the module's directory looked like at that moment, which is how a
// test holds the stop against the delete that has to follow it.
type stoppedPlane struct {
	root    string
	stopped []ref.Ref
	// restarted is every instance rebuilt in place.
	restarted []ref.Ref
	held      bool
}

func (p *stoppedPlane) Database(ref.Ref) (wirerpc.DatabaseState, bool) {
	return wirerpc.DatabaseState{}, false
}

func (p *stoppedPlane) Stop(instance ref.Ref) bool {
	p.stopped = append(p.stopped, instance)

	if _, err := os.Stat(p.root); err == nil {
		p.held = true
	}

	return true
}

// Status reports a plane that is supervising the instance until it is stopped,
// which is what the install answer reads back.
func (p *stoppedPlane) Status(instance ref.Ref) (httpapi.ResourceStatus, bool) {
	return httpapi.ResourceStatus{Serving: true}, !slices.Contains(p.stopped, instance)
}

func TestRemovingOverHTTPStopsTheProcessBeforeItDeletesTheFiles(t *testing.T) {
	t.Parallel()

	plane := &stoppedPlane{}
	installer, paths, found := anInstalledModule(t, plane)
	plane.root = filepath.Join(installer.core.Modules, found.Ref.ID())

	require.NoError(t, installer.Remove(t.Context(), "user:operator", theNamespace, false))

	require.Equal(t, []ref.Ref{found.Ref}, plane.stopped, "the instance was not stopped")
	require.True(t, plane.held, "the files were already gone when the process was stopped")

	rows, err := resource.Installed(paths)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoDirExists(t, plane.root)
}

func (p *stoppedPlane) Restart(instance ref.Ref) bool {
	p.restarted = append(p.restarted, instance)

	return true
}
