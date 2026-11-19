package app

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

// ADR-20261005-64j rules 5 to 8: what a download may reach, how far it is redirected,
// and what an install over an installed module does to the running process.

func TestOnlyHttpsOrALoopbackHostIsFetched(t *testing.T) {
	t.Parallel()

	cases := []struct {
		address string
		allowed bool
	}{
		{"https://plugins.example/a.tar.gz", true},
		{"https://storage.example/a.tar.gz?X-Goog-Signature=abc", true},
		{"http://127.0.0.1:4545/a.tar.gz", true},
		{"http://localhost:4545/a.tar.gz", true},
		{"http://plugins.example/a.tar.gz", false},
		{"http://192.168.1.20/a.tar.gz", false},
		{"https://user:secret@plugins.example/a.tar.gz", false},
		{"ftp://plugins.example/a.tar.gz", false},
	}

	for _, row := range cases {
		parsed, err := url.Parse(row.address)
		require.NoError(t, err)
		require.Equal(t, row.allowed, remoteAllowed(parsed), row.address)
	}
}

func TestPlainHttpToAnotherHostIsRefusedBeforeAnythingIsFetched(t *testing.T) {
	t.Parallel()

	installer, _ := theInstaller(t, true)

	_, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: "http://plugins.example/spools-1.0.0.tar.gz",
	})
	requireCode(t, err, httpfault.CodeRequestInvalid())
}

// A redirect is held to the same rule, and the refusal is the one every
// failed download answers, so it says nothing about where the address led.
func TestARedirectIsHeldToTheSameRule(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/elsewhere.tar.gz":
			http.Redirect(w, r, "http://plugins.example/spools.tar.gz", http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/loop"):
			http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
		}
	}))
	t.Cleanup(server.Close)

	installer, _ := marketInstaller(t, server.URL)

	for _, path := range []string{"/elsewhere.tar.gz", "/loop.tar.gz"} {
		_, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
			Module: theModulePath, Version: "1.0.0", From: server.URL + path, Sha256: strings.Repeat("ab", 32),
		})
		requireCode(t, err, httpfault.CodeModuleDownloadFailed())
		require.NotContains(t, err.Error(), "redirect", path)
	}
}

func TestAReleaseMayCarryTheAuthorsSignature(t *testing.T) {
	t.Parallel()

	installer, _ := theInstaller(t, true)
	release := aRelease(t, bothHalvesManifest(), true)

	// checksums.txt over every file, and its signature beside it, unlisted.
	writeChecksums(t, release)
	require.NoError(t, os.WriteFile(filepath.Join(release, checksumsName+".sig"),
		[]byte("c2lnbmF0dXJl\n"), 0o600))

	held, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: anArchiveOf(t, release), Approve: true,
	})
	require.NoError(t, err)
	require.Equal(t, 3, held.Verified)
}

// A checksum file vouches for a whole release. A file beside it that it does
// not list would be installed with nothing having checked it, so the install
// refuses the release before anything is written, as the marketplace does
// before it publishes one.
func TestAReleaseCarryingAFileItsChecksumsDoNotListIsRefused(t *testing.T) {
	t.Parallel()

	installer, configPath := theInstaller(t, true)
	release := aRelease(t, bothHalvesManifest(), true)

	writeChecksums(t, release)
	require.NoError(t, os.WriteFile(filepath.Join(release, "ui", "extra.js"),
		[]byte("alert(1)\n"), 0o600))

	_, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: release, Approve: true,
	})
	require.ErrorContains(t, err, "ui/extra.js")

	rows, err := resource.Installed(resource.Paths{File: configPath, Overlay: installer.core.Overlay})
	require.NoError(t, err)
	_, declared := declarationOfNamespace(rows, theNamespace)
	require.False(t, declared, "a release with an unlisted file was declared")
}

// writeChecksums writes the checksums.txt a release of bothHalvesManifest
// carries, listing every file aRelease wrote.
func writeChecksums(t *testing.T, release string) {
	t.Helper()

	var lines []string

	for _, name := range []string{manifestName, "ui/index.js", thisPlatformBinary()} {
		sum, err := digestOfFile(filepath.Join(release, filepath.FromSlash(name)))
		require.NoError(t, err)

		lines = append(lines, sum+"  "+name)
	}

	require.NoError(t, os.WriteFile(filepath.Join(release, checksumsName),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o600))
}

// macOS tar writes a "._" file beside every file with extended attributes.
// Those entries are no part of the release: the install neither refuses them
// as unlisted nor writes them anywhere, and a "._module.yaml" is not read as
// the manifest.
func TestAnArchiveCarryingAppleDoubleFilesInstallsWithoutThem(t *testing.T) {
	t.Parallel()

	installer, _ := theInstaller(t, true)
	release := aRelease(t, bothHalvesManifest(), true)
	writeChecksums(t, release)

	for _, name := range []string{"._" + checksumsName, "._" + manifestName, "ui/._index.js"} {
		require.NoError(t, os.WriteFile(filepath.Join(release, filepath.FromSlash(name)),
			[]byte("name: not-this-module\n"), 0o600))
	}

	held, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: anArchiveOf(t, release), Approve: true,
	})
	require.NoError(t, err)
	require.Equal(t, 3, held.Verified)

	require.NoError(t, filepath.WalkDir(installer.core.Modules, func(where string, _ fs.DirEntry, err error) error {
		require.False(t, strings.HasPrefix(filepath.Base(where), "._"), "%s was installed", where)

		return err
	}))
}

// A release with no checksum file is still installed, and says it checked
// nothing: that is an author's own build directory a minute after a build.
func TestAReleaseWithNoChecksumFileIsInstalledUnchecked(t *testing.T) {
	t.Parallel()

	installer, _ := theInstaller(t, true)

	held, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
		Module: theModulePath, Version: "1.0.0", From: aRelease(t, bothHalvesManifest(), true), Approve: true,
	})
	require.NoError(t, err)
	require.Zero(t, held.Verified)
}

// versionedManifest is the both-halves module at another version.
func versionedManifest(version string) string {
	return strings.Replace(bothHalvesManifest(), "version: 1.0.0", "version: "+version, 1)
}

func TestAnUpdateStopsTheRunningProcessBeforeItReplacesTheFiles(t *testing.T) {
	t.Parallel()

	plane := &stoppedPlane{}
	installer, configPath := theInstallerOver(t, true, plane)
	plane.root = filepath.Join(installer.core.Modules, "spools")

	install := func(version string) {
		t.Helper()

		_, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
			Module: theModulePath, Version: version,
			From: aRelease(t, versionedManifest(version), true), Approve: true,
		})
		require.NoError(t, err)
	}

	install("0.1.0")
	require.Empty(t, plane.stopped, "a first install stopped something")

	instance, err := ref.New(ref.KindSink, "spools")
	require.NoError(t, err)
	require.Empty(t, plane.restarted, "a first install restarted something")

	// The same version again writes the same declaration, which the store
	// plans as nothing, so the running process is left alone. Its files were
	// replaced, so it is rebuilt to open them.
	install("0.1.0")
	require.Empty(t, plane.stopped, "reinstalling the same release stopped the process")
	require.Equal(t, []ref.Ref{instance}, plane.restarted)

	install("0.2.0")

	require.Equal(t, []ref.Ref{instance}, plane.stopped)
	require.True(t, plane.held, "the process was stopped after its files were already gone")

	rows, err := resource.Installed(resource.Paths{File: configPath, Overlay: installer.core.Overlay})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Spec.Module)
	require.Equal(t, "0.2.0", rows[0].Spec.Module.Version)
}

// countingReload is the configured inventory as an install sees it, counting
// how often it was asked to converge.
type countingReload struct{ calls int }

func (c *countingReload) Reload(context.Context) error {
	c.calls++

	return nil
}

// TestAnUpdateIsReconciledAfterTheStop holds every install, the update among
// them, to a reconcile of the store. The host starts an instance it is not
// supervising when the plan restarts it, which is what brings the stopped
// process up on the new release.
func TestAnUpdateIsReconciledAfterTheStop(t *testing.T) {
	t.Parallel()

	plane := &stoppedPlane{}
	installer, _ := theInstallerOver(t, true, plane)
	reloads := &countingReload{}
	installer.configured = reloads

	for _, version := range []string{"0.1.0", "0.2.0"} {
		held, err := installer.Install(t.Context(), "user:operator", httpapi.ModuleRelease{
			Module: theModulePath, Version: version,
			From: aRelease(t, versionedManifest(version), true), Approve: true,
		})
		require.NoError(t, err)
		require.Equal(t, version, held.Version)
		require.False(t, held.Restart, "an update asked for a restart of the server")
	}

	require.Equal(t, 2, reloads.calls, "an update was not reconciled after the stop")
}
