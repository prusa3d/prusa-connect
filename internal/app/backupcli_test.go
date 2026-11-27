package app

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/moduledeps"
	"github.com/prusa3d/prusa-connect/internal/betaportal"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/upgrade"
)

func TestBackupCLIListsAndRestoresAHostDatabase(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	database := filepath.Join(root, "config", "config.d", "authz.db")
	writeCLIState(t, database, "before")

	backups := filepath.Join(root, "backups")
	var out bytes.Buffer
	if err := Backup(t.Context(), &out, []string{
		"create", "-config", configPath, "-to", backups, "-upgrade-to", "2.0.0",
	}); err != nil {
		t.Fatal(err)
	}

	list, err := backupIDs(backups)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("backups are %#v", list)
	}

	writeCLIState(t, database, "after")

	out.Reset()
	if err = Backup(t.Context(), &out, []string{
		"restore", "-config", configPath, "-from", backups, list[0],
	}); err != nil {
		t.Fatal(err)
	}

	if got := readCLIState(t, database); got != "before" {
		t.Fatalf("restored value is %q", got)
	}
}

func TestResourceSQLitePathsReadOperatorAndOverlayDeclarations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	operator := filepath.Join(root, "config.yaml")
	overlay := filepath.Join(root, "resources.json")

	if err := os.WriteFile(operator, []byte(`
resources:
  history:
    past:
      type: sqlite
      config:
        path: history.db
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, []byte(`{
  "instances": [
    {"spec": {"type": "sqlite", "config": {"path": "records.db"}}}
  ]
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	paths, err := resourceSQLitePaths(operator, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "history.db" || paths[1] != "records.db" {
		t.Fatalf("SQLite paths are %#v", paths)
	}
}

// TestResourceSQLitePathsReadTheHistoryTheWizardDeclares checks that a backup
// of a server the first-run wizard configured includes the print history.
func TestResourceSQLitePathsReadTheHistoryTheWizardDeclares(t *testing.T) {
	t.Parallel()

	install := freshInstall(t)
	operator := filepath.Join(install.dir, "config.yaml")

	history, err := historyOf(&httpapi.SetupHistory{Retention: "168h"}, operator)
	if err != nil {
		t.Fatal(err)
	}
	if err = install.first.declareHistory(t.Context(), history); err != nil {
		t.Fatal(err)
	}

	paths, err := resourceSQLitePaths(operator, filepath.Join(install.dir, "config.d", "resources.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != history.Path {
		t.Fatalf("SQLite paths are %#v, want the history file %q", paths, history.Path)
	}
}

func TestUpgradeBackupNeverRestoresItsTransactionJournal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	core, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := backupPlan(configPath, core, "2.0.0", upgrade.PostgresNone, true)
	if err != nil {
		t.Fatal(err)
	}

	journal := upgradeJournalPath(core)
	pairing := betaportal.Path(core.Overlay)
	pairingProtected := false
	for _, source := range plan.Files {
		if source.Path == journal {
			t.Fatalf("the protected data includes its transaction coordinator %s", journal)
		}
		if source.Path == pairing {
			pairingProtected = true
		}
	}
	if !pairingProtected {
		t.Fatalf("the protected data omits the Beta Portal pairing %s", pairing)
	}
}

// TestABackupProtectsThePairingTheServerUses is the systemd service, whose
// overlay sits in another folder than its configuration file. A new pairing
// sits beside the overlay, and one release 0.0.1-alpha.7 kept in config.d
// stays in use there, so a restore puts back the file the server reads.
func TestABackupProtectsThePairingTheServerUses(t *testing.T) {
	t.Parallel()

	for name, earlierHeld := range map[string]bool{"a new pairing": false, "an earlier pairing": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			configPath := filepath.Join(root, "etc", "config.yaml")
			overlay := filepath.Join(root, "state", "resources.json")
			write(t, configPath, lines("core:", "  overlay: "+slashed(overlay)))
			used, unused := betaportal.Path(overlay), betaportal.EarlierPath(configPath)
			if earlierHeld {
				if _, err := betaportal.Open(unused); err != nil {
					t.Fatal(err)
				}
				used, unused = unused, used
			}

			core, err := config.Load(configPath)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := backupPlan(configPath, core, "2.0.0", upgrade.PostgresNone, true)
			if err != nil {
				t.Fatal(err)
			}

			protected := false
			for _, source := range plan.Files {
				if source.Path == unused {
					t.Fatalf("the backup protects %s, which the server does not read", unused)
				}
				protected = protected || source.Path == used
			}
			if !protected {
				t.Fatalf("the protected data omits the Beta Portal pairing %s", used)
			}
		})
	}
}

func backupIDs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name()[0] != '.' {
			ids = append(ids, entry.Name())
		}
	}

	return ids, nil
}

func writeCLIState(t *testing.T, path, value string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err = db.ExecContext(t.Context(),
		"CREATE TABLE IF NOT EXISTS state (value TEXT NOT NULL); DELETE FROM state; INSERT INTO state VALUES (?)",
		value); err != nil {
		t.Fatal(err)
	}
}

func readCLIState(t *testing.T, path string) string {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var value string
	if err = db.QueryRowContext(t.Context(), "SELECT value FROM state").Scan(&value); err != nil {
		t.Fatal(err)
	}

	return value
}

func TestUpgradeBackupProtectsTheModuleDataRoot(t *testing.T) {
	t.Parallel()

	configPath, core := installConfig(t)
	plan, err := backupPlan(configPath, core, "2.0.0", upgrade.PostgresNone, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range plan.Files {
		if source.Name == "module-data" && source.Path == moduleDataRoot(core) && source.Directory {
			return
		}
	}
	t.Fatalf("the backup plan omits the module data root %s: %+v", moduleDataRoot(core), plan.Files)
}

// TestUpgradeBackupProtectsTheModulesLock keeps the lock and the overlay one
// pair: a rollback that restored the declarations and left a lock naming the
// candidate's releases would install them again on the next `module install`.
func TestUpgradeBackupProtectsTheModulesLock(t *testing.T) {
	t.Parallel()

	configPath, core := installConfig(t)
	plan, err := backupPlan(configPath, core, "2.0.0", upgrade.PostgresNone, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range plan.Files {
		if source.Name == "modules-lock" && source.Path == moduledeps.LockPath(core.Overlay) && !source.Directory {
			return
		}
	}
	t.Fatalf("the backup plan omits the modules lock %s: %+v", moduledeps.LockPath(core.Overlay), plan.Files)
}
