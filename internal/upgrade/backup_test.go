package upgrade

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupSnapshotsVerifiesAndRestoresSQLiteAndFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	database := filepath.Join(root, "live", "authz.db")
	config := filepath.Join(root, "config", "config.yaml")
	modules := filepath.Join(root, "config.d", "modules")

	writeSQLite(t, database, "before")
	writeFile(t, config, []byte("core:\n  site: before\n"), 0o640)
	writeFile(t, filepath.Join(modules, "planner", "module.yaml"), []byte("version: 1\n"), 0o600)

	manager := BackupManager{
		Root: filepath.Join(root, "backups"),
		Keep: 5,
		Now:  func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) },
	}

	manifest, err := manager.Create(t.Context(), BackupPlan{
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		SQLite:      []SQLiteSource{{Name: "host", Path: database}},
		Files: []FileSource{
			{Name: "config", Path: config},
			{Name: "modules", Path: modules, Directory: true},
		},
		Storage: []string{"/srv/prints"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if manifest.ID != "20261026T120000Z-1.0.0-to-1.1.0" {
		t.Fatalf("backup id is %q", manifest.ID)
	}
	if _, err = manager.Verify(manifest.ID); err != nil {
		t.Fatal(err)
	}

	writeSQLite(t, database, "after")
	writeFile(t, config, []byte("changed\n"), 0o600)
	if err = os.RemoveAll(modules); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(modules, "planner", "extra"), []byte("extra"), 0o600)

	if err = manager.Restore(t.Context(), manifest.ID, nil); err != nil {
		t.Fatal(err)
	}

	if got := readSQLite(t, database); got != "before" {
		t.Fatalf("restored SQLite value is %q", got)
	}

	content, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "core:\n  site: before\n" {
		t.Fatalf("restored config is %q", content)
	}

	content, err = os.ReadFile(filepath.Join(modules, "planner", "module.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "version: 1\n" {
		t.Fatalf("restored module is %q", content)
	}
	if _, err = os.Stat(filepath.Join(modules, "planner", "extra")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore kept an extra module file: %v", err)
	}
}

// A backup made by hand has no target version, so its name carries only the
// version that made it. A backup taken before an update also names the target.
func TestBackupNameNamesTheTargetOnlyWhenThereIsOne(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		to   string
		want string
	}{
		{name: "manual", to: "", want: "20261123T104715Z-0.1.0-beta.1"},
		{name: "before an update", to: "0.2.0", want: "20261123T104715Z-0.1.0-beta.1-to-0.2.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			config := filepath.Join(root, "config.yaml")
			writeFile(t, config, []byte("core: {}\n"), 0o600)

			manager := BackupManager{
				Root: filepath.Join(root, "backups"),
				Keep: 5,
				Now:  func() time.Time { return time.Date(2026, 11, 23, 10, 47, 15, 0, time.UTC) },
			}

			manifest, err := manager.Create(t.Context(), BackupPlan{
				FromVersion: "0.1.0-beta.1",
				ToVersion:   test.to,
				Files:       []FileSource{{Name: "config", Path: config}},
			})
			if err != nil {
				t.Fatal(err)
			}

			if manifest.ID != test.want {
				t.Errorf("backup id is %q, want %q", manifest.ID, test.want)
			}
		})
	}
}

// A configuration names its databases relative to the directory the server
// runs in. A restore run from another directory, such as by an operator on a
// service host, puts each database back where the backup took it from.
func TestBackupRestoresARelativeDatabaseFromAnotherDirectory(t *testing.T) {
	root := t.TempDir()
	server := filepath.Join(root, "server")
	elsewhere := filepath.Join(root, "elsewhere")
	writeSQLite(t, filepath.Join(server, "config.d", "authz.db"), "before")
	writeSQLite(t, filepath.Join(elsewhere, "config.d", "authz.db"), "untouched")

	manager := BackupManager{
		Root: filepath.Join(root, "backups"),
		Keep: 5,
		Now:  func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) },
	}

	t.Chdir(server)

	manifest, err := manager.Create(t.Context(), BackupPlan{
		FromVersion: "1.0.0",
		SQLite:      []SQLiteSource{{Name: "host", Path: filepath.Join("config.d", "authz.db")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeSQLite(t, filepath.Join(server, "config.d", "authz.db"), "after")

	t.Chdir(elsewhere)

	if err = manager.Restore(t.Context(), manifest.ID, nil); err != nil {
		t.Fatal(err)
	}

	if got := readSQLite(t, filepath.Join(server, "config.d", "authz.db")); got != "before" {
		t.Errorf("the server's database reads %q after the restore", got)
	}

	if got := readSQLite(t, filepath.Join(elsewhere, "config.d", "authz.db")); got != "untouched" {
		t.Errorf("the restore wrote the database into the directory it ran in: %q", got)
	}
}

func TestBackupRestoresAnAbsentDirectoryAsAbsent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modules := filepath.Join(root, "modules")
	manager := BackupManager{Root: filepath.Join(root, "backups")}
	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID:    "absent-directory",
		Files: []FileSource{{Name: "modules", Path: modules, Directory: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(modules, "new", "file"), []byte("new"), 0o600)

	if err = manager.Restore(t.Context(), manifest.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(modules); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore kept a directory absent from the snapshot: %v", err)
	}
}

func TestPostgresRestoreConnectsOutsideTheDatabaseItRecreates(t *testing.T) {
	t.Parallel()

	got, err := postgresAdminDSN("postgres://user:secret@db.example:5432/module?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://user:secret@db.example:5432/postgres?sslmode=require" {
		t.Fatalf("admin DSN is %q", got)
	}

	got, err = postgresAdminDSN("postgresql://user@db.example/postgres")
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgresql://user@db.example/template1" {
		t.Fatalf("postgres database fallback is %q", got)
	}
}

func TestBackupVerificationRefusesChangedContentBeforeRestore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	config := filepath.Join(root, "config.yaml")
	writeFile(t, config, []byte("safe\n"), 0o600)

	manager := BackupManager{Root: filepath.Join(root, "backups")}
	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID: "one", Files: []FileSource{{Name: "config", Path: config}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(manager.Root, manifest.ID, manifest.Files[0].Path), []byte("tampered\n"), 0o600)
	writeFile(t, config, []byte("live\n"), 0o600)
	extra := filepath.Join(filepath.Dir(config), "unprotected.txt")
	writeFile(t, extra, []byte("leave me\n"), 0o600)

	err = manager.Restore(t.Context(), manifest.ID, nil)
	if !errors.Is(err, ErrBackupIntegrity) {
		t.Fatalf("restore error is %v", err)
	}

	content, readErr := os.ReadFile(config)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "live\n" {
		t.Fatalf("restore changed the live file before verification: %q", content)
	}
	if content, readErr = os.ReadFile(extra); readErr != nil || string(content) != "leave me\n" {
		t.Fatalf("failed restore changed unrelated data: content %q, error %v", content, readErr)
	}
}

func TestBackupRestoreMakesProtectedDirectoryExact(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modules := filepath.Join(root, "config.d", "modules")
	kept := filepath.Join(modules, "planner", "module.yaml")
	empty := filepath.Join(modules, "empty")
	unrelated := filepath.Join(root, "prints", "job.gcode")
	outside := filepath.Join(root, "outside.txt")

	writeFile(t, kept, []byte("version: 1\n"), 0o640)
	if err := os.MkdirAll(empty, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, unrelated, []byte("G1 X10\n"), 0o600)
	writeFile(t, outside, []byte("outside\n"), 0o600)

	manager := BackupManager{Root: filepath.Join(root, "backups")}
	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID: "exact-directory", Files: []FileSource{{Name: "modules", Path: modules}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, kept, []byte("candidate\n"), 0o600)
	writeFile(t, filepath.Join(modules, "candidate", "new.yaml"), []byte("new\n"), 0o600)
	if err = os.RemoveAll(empty); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, filepath.Join(modules, "candidate-link")); err != nil {
		t.Fatal(err)
	}

	if err = manager.Restore(t.Context(), manifest.ID, nil); err != nil {
		t.Fatal(err)
	}

	if content, readErr := os.ReadFile(kept); readErr != nil || string(content) != "version: 1\n" {
		t.Fatalf("restored module content is %q, error %v", content, readErr)
	}
	for _, path := range []string{
		filepath.Join(modules, "candidate"),
		filepath.Join(modules, "candidate-link"),
	} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("candidate path %s survived rollback: %v", path, statErr)
		}
	}
	if info, statErr := os.Stat(empty); statErr != nil || !info.IsDir() || info.Mode().Perm() != 0o750 {
		t.Fatalf("empty directory was not restored exactly: info %#v, error %v", info, statErr)
	}
	if content, readErr := os.ReadFile(unrelated); readErr != nil || string(content) != "G1 X10\n" {
		t.Fatalf("unprotected data changed: content %q, error %v", content, readErr)
	}
	if content, readErr := os.ReadFile(outside); readErr != nil || string(content) != "outside\n" {
		t.Fatalf("symlink target changed: content %q, error %v", content, readErr)
	}
}

func TestBackupRestoreRemovesPathMissingAtSnapshot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	generated := filepath.Join(root, "config.d", "candidate")
	manager := BackupManager{Root: filepath.Join(root, "backups")}
	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID: "missing-path", Files: []FileSource{{Name: "candidate", Path: generated}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(generated, "module.yaml"), []byte("candidate\n"), 0o600)
	if err = manager.Restore(t.Context(), manifest.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(generated); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path absent from the snapshot survived rollback: %v", err)
	}
}

func TestBackupRestoreRejectsTamperedDeletionBoundaryBeforeChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modules := filepath.Join(root, "config.d", "modules")
	kept := filepath.Join(modules, "module.yaml")
	writeFile(t, kept, []byte("before\n"), 0o600)

	manager := BackupManager{Root: filepath.Join(root, "backups")}
	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID: "boundary", Files: []FileSource{{Name: "modules", Path: modules}},
	})
	if err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(modules, "candidate.yaml")
	writeFile(t, extra, []byte("candidate\n"), 0o600)

	manifestPath := filepath.Join(manager.Root, manifest.ID, "manifest.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Protected[0].Source = filepath.Join(root, "other")
	content, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifestPath, append(content, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	err = manager.Restore(t.Context(), manifest.ID, nil)
	if !errors.Is(err, ErrBackupInvalid) {
		t.Fatalf("restore error is %v", err)
	}
	if extraContent, readErr := os.ReadFile(extra); readErr != nil || string(extraContent) != "candidate\n" {
		t.Fatalf("invalid manifest changed live data: content %q, error %v", extraContent, readErr)
	}
}

func TestBackupRetentionKeepsNewestSuccessfulBackups(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "config.yaml")
	writeFile(t, source, []byte("config\n"), 0o600)

	manager := BackupManager{Root: filepath.Join(root, "backups"), Keep: 2}
	for index, id := range []string{"one", "two", "three"} {
		manager.Now = func() time.Time {
			return time.Date(2026, 10, 26, 12, 0, index, 0, time.UTC)
		}

		if _, err := manager.Create(t.Context(), BackupPlan{
			ID: id, Files: []FileSource{{Name: "config", Path: source}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	backups, err := manager.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 2 || backups[0].ID != "three" || backups[1].ID != "two" {
		t.Fatalf("retained backups are %#v", backups)
	}
}

func TestExternalPostgresBackupRequiresAcknowledgement(t *testing.T) {
	t.Parallel()

	manager := BackupManager{Root: t.TempDir()}
	_, err := manager.Create(t.Context(), BackupPlan{
		ID: "unconfirmed",
		Postgres: []PostgresSource{{
			Name: "host", DSN: "postgres://unused", Mode: PostgresExternal,
		}},
	})
	if !errors.Is(err, ErrPostgresUnconfirmed) {
		t.Fatalf("create error is %v", err)
	}

	manifest, err := manager.Create(t.Context(), BackupPlan{
		ID: "confirmed",
		Postgres: []PostgresSource{{
			Name: "host", DSN: "postgres://unused", Mode: PostgresExternal, Acknowledged: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Postgres) != 1 || manifest.Postgres[0].Mode != PostgresExternal {
		t.Fatalf("PostgreSQL manifest is %#v", manifest.Postgres)
	}
}

func writeSQLite(t *testing.T, path, value string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err = db.ExecContext(context.Background(),
		"CREATE TABLE IF NOT EXISTS state (value TEXT NOT NULL); DELETE FROM state; INSERT INTO state VALUES (?)",
		value); err != nil {
		t.Fatal(err)
	}
}

func readSQLite(t *testing.T, path string) string {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var value string
	if err = db.QueryRowContext(context.Background(), "SELECT value FROM state").Scan(&value); err != nil {
		t.Fatal(err)
	}

	return value
}

func writeFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
}
