package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestRestoreLeavesAnUnchangedFileAlone is the reported rollback: the
// configuration file still holds the bytes the backup recorded, so the restore
// writes nothing. The modification time proves the file was not replaced.
func TestRestoreLeavesAnUnchangedFileAlone(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config", "config.yaml")
	content := []byte("core:\n  site: before\n")
	writeFile(t, configPath, content, 0o640)

	manager := BackupManager{
		Root: filepath.Join(root, "backups"),
		Keep: 5,
		Now:  func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
	}
	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID: "unchanged", Files: []FileSource{{Name: "config", Path: configPath}},
	})
	if err != nil {
		t.Fatal(err)
	}

	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err = os.Chtimes(configPath, past, past); err != nil {
		t.Fatal(err)
	}

	if err = manager.Restore(t.Context(), manifest.ID, nil); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Errorf("an unchanged file was rewritten: modification time %s, want %s", info.ModTime(), past)
	}
	if got, readErr := os.ReadFile(configPath); readErr != nil || string(got) != string(content) {
		t.Fatalf("content is %q, error %v; want %q", got, readErr, content)
	}
}

// TestRestoreRewritesAFileThatChanged proves the skip is conditional: a
// destination holding other bytes is restored.
func TestRestoreRewritesAFileThatChanged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config", "config.yaml")
	content := []byte("core:\n  site: before\n")
	writeFile(t, configPath, content, 0o640)

	manager := BackupManager{
		Root: filepath.Join(root, "backups"),
		Keep: 5,
		Now:  func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
	}
	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID: "changed", Files: []FileSource{{Name: "config", Path: configPath}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, configPath, []byte("core:\n  site: after\n"), 0o640)

	if err = manager.Restore(t.Context(), manifest.ID, nil); err != nil {
		t.Fatal(err)
	}

	if got, readErr := os.ReadFile(configPath); readErr != nil || string(got) != string(content) {
		t.Fatalf("content is %q, error %v; want the backed-up %q", got, readErr, content)
	}
}

// TestRestoreWritesNothingIntoAReadOnlyDirectory is the reported case: the
// configuration directory is mounted read-only, so a restore that skips an
// unchanged file succeeds, and one that has a file to write still fails.
func TestRestoreWritesNothingIntoAReadOnlyDirectory(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a directory mode does not stop this process here")
	}

	root := t.TempDir()
	configPath := filepath.Join(root, "config", "config.yaml")
	writeFile(t, configPath, []byte("core:\n  site: before\n"), 0o640)

	manager := BackupManager{
		Root: filepath.Join(root, "backups"),
		Keep: 5,
		Now:  func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) },
	}
	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID: "read-only", Files: []FileSource{{Name: "config", Path: configPath}},
	})
	if err != nil {
		t.Fatal(err)
	}

	directory := filepath.Dir(configPath)
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })

	if err = os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	if err = manager.Restore(t.Context(), manifest.ID, nil); err != nil {
		t.Fatalf("an unchanged file in a read-only directory failed to restore: %v", err)
	}

	if err = os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, configPath, []byte("core:\n  site: after\n"), 0o640)
	if err = os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}

	err = manager.Restore(t.Context(), manifest.ID, nil)
	if err == nil {
		t.Fatal("a changed file in a read-only directory was restored silently")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("restore error is %v; want a permission error", err)
	}
}

// TestRunnerRollsBackWhenTheConfigurationDirectoryIsReadOnly is the reported
// failure end to end: an interrupted trial has a backup, the configuration
// directory refuses writes, and the rollback must still finish so the next
// start runs.
func TestRunnerRollsBackWhenTheConfigurationDirectoryIsReadOnly(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a directory mode does not stop this process here")
	}

	fixture := newStagedRunnerFixture(t)
	advanceRecoveryFixture(t, fixture, StateTrial, true)

	directory := filepath.Dir(fixture.data)
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}

	runner := newRecoveryRunner(t, fixture, nil)
	journal, err := runner.Recover(t.Context())
	if journal.State != StateRolledBack {
		t.Fatalf("recovered state is %s, error %v; want %s", journal.State, err, StateRolledBack)
	}
	// A successful rollback still reports the interruption that caused it, so
	// the only acceptable error here is that cause and never a rollback error.
	if err == nil || err.Error() != "upgrade was interrupted" {
		t.Fatalf("recovery error is %v; want only the interruption cause", err)
	}
	if got := readRunnerFile(t, fixture.data); got != "before" {
		t.Fatalf("configuration file is %q; want %q", got, "before")
	}
	assertStoredState(t, fixture.store, StateRolledBack)

	started, err := runner.Recover(t.Context())
	if err != nil {
		t.Fatalf("the server start after the rollback failed: %v", err)
	}
	if started.State != StateRolledBack {
		t.Fatalf("the start after the rollback reads state %s; want %s", started.State, StateRolledBack)
	}
}
