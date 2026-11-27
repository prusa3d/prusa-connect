package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/moduledeps"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/betaportal"
	"github.com/prusa3d/prusa-connect/internal/kernel/buildprofile"
	"github.com/prusa3d/prusa-connect/internal/kernel/version"
	"github.com/prusa3d/prusa-connect/internal/upgrade"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

const BackupCommand = "backup"

func Backup(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: %s create|list|restore", ErrUsage, BackupCommand)
	}

	switch args[0] {
	case "create":
		return createBackup(ctx, out, args[1:])
	case "list":
		return listBackups(out, args[1:])
	case "restore":
		return restoreBackup(ctx, out, args[1:])
	default:
		return fmt.Errorf("%w: %s %s is not a command. It is create, list or restore",
			ErrUsage, BackupCommand, args[0])
	}
}

func createBackup(ctx context.Context, out io.Writer, args []string) error {
	flags := flag.NewFlagSet(BackupCommand+" create", flag.ContinueOnError)
	flags.SetOutput(out)

	configPath := flags.String("config", defaultConfigPath, "the operator's configuration file")
	destination := flags.String("to", "", "backup root; defaults beside the configuration overlay")
	toVersion := flags.String("upgrade-to", "", "the target version this backup precedes")
	postgresMode := flags.String("postgres", string(upgrade.PostgresExternal),
		"PostgreSQL backup mode: pg_dump, external or none")
	acknowledge := flags.Bool("acknowledge-external-postgres", false,
		"record that PostgreSQL is backed up externally or intentionally omitted")
	jsonOutput := flags.Bool("json", false, "write the manifest as JSON")

	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%w: %s create takes flags only", ErrUsage, BackupCommand)
	}

	core, err := loadCore(*configPath)
	if err != nil {
		return err
	}

	manager := backupManager(core, *destination)
	plan, err := backupPlan(*configPath, core, *toVersion, upgrade.PostgresMode(*postgresMode), *acknowledge)
	if err != nil {
		return err
	}

	manifest, err := manager.Create(ctx, plan)
	if err != nil {
		return err
	}

	if *jsonOutput {
		return writeJSON(out, manifest)
	}

	fmt.Fprintf(out, "created backup %s with %d files\n", manifest.ID, len(manifest.Files))

	return nil
}

func listBackups(out io.Writer, args []string) error {
	flags := flag.NewFlagSet(BackupCommand+" list", flag.ContinueOnError)
	flags.SetOutput(out)

	configPath := flags.String("config", defaultConfigPath, "the operator's configuration file")
	destination := flags.String("from", "", "backup root; defaults beside the configuration overlay")
	jsonOutput := flags.Bool("json", false, "write the list as JSON")

	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%w: %s list takes flags only", ErrUsage, BackupCommand)
	}

	core, err := loadCore(*configPath)
	if err != nil {
		return err
	}

	backups, err := backupManager(core, *destination).List()
	if err != nil {
		return err
	}

	if *jsonOutput {
		return writeJSON(out, backups)
	}

	for _, backup := range backups {
		versions := backup.FromVersion
		if backup.ToVersion != "" {
			versions += " -> " + backup.ToVersion
		}

		fmt.Fprintf(out, "%s\t%s\t%s\t%d files\n",
			backup.ID, backup.CreatedAt.Format("2006-01-02 15:04:05Z"),
			versions, len(backup.Files))
	}

	return nil
}

func restoreBackup(ctx context.Context, out io.Writer, args []string) error {
	flags := flag.NewFlagSet(BackupCommand+" restore", flag.ContinueOnError)
	flags.SetOutput(out)

	configPath := flags.String("config", defaultConfigPath, "the operator's configuration file")
	source := flags.String("from", "", "backup root; defaults beside the configuration overlay")

	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("%w: %s restore <backup id> [flags]", ErrUsage, BackupCommand)
	}

	core, err := loadCore(*configPath)
	if err != nil {
		return err
	}

	plan, err := backupPlan(
		*configPath, core, "", upgrade.PostgresMode(core.Upgrade.PostgresBackup), true,
	)
	if err != nil {
		return err
	}

	external, err := backupManager(core, *source).RestoreReporting(
		ctx, flags.Arg(0), postgresRestoreTargets(plan),
	)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "restored backup %s\n", flags.Arg(0))
	for _, entry := range external {
		fmt.Fprintf(out, "PostgreSQL database %s was not restored (mode %s): restore it from your own backup\n",
			entry.Name, entry.Mode)
	}

	return nil
}

func loadCore(path string) (config.Core, error) {
	core, err := config.Load(path)
	if err != nil {
		return config.Core{}, err
	}
	if err = core.Validate(); err != nil {
		return config.Core{}, err
	}

	return core, nil
}

func backupManager(core config.Core, root string) upgrade.BackupManager {
	if root == "" {
		root = filepath.Join(filepath.Dir(core.Overlay), "upgrade", "backups")
	}
	keep := core.Upgrade.BackupRetention
	if buildprofile.Name == "printer" {
		keep = 1
	}

	return upgrade.BackupManager{Root: root, Keep: keep}
}

func backupPlan(
	configPath string,
	core config.Core,
	toVersion string,
	postgresMode upgrade.PostgresMode,
	acknowledge bool,
) (upgrade.BackupPlan, error) {
	plan := upgrade.BackupPlan{
		FromVersion: version.Version(),
		ToVersion:   toVersion,
		Files: []upgrade.FileSource{
			{Name: "config", Path: configPath},
			{Name: "setup", Path: config.SetupFile(configPath)},
			{Name: "beta-portal-pairing", Path: betaportal.Locate(configPath, core.Overlay)},
			{Name: "overlay", Path: core.Overlay},
			// The lock names the module releases the overlay declares, so an
			// upgrade that rolls the overlay back rolls the lock back with it.
			{Name: "modules-lock", Path: moduledeps.LockPath(core.Overlay)},
			{Name: "modules", Path: core.Modules, Directory: true},
		},
	}

	switch core.Persistence.Type {
	case config.EngineSQLite:
		plan.SQLite = append(plan.SQLite, upgrade.SQLiteSource{Name: "host", Path: core.Authz.Database})
	case config.EnginePostgres:
		plan.Postgres = append(plan.Postgres, upgrade.PostgresSource{
			Name: "host", DSN: core.Persistence.Postgres.DSN, RestoreDSN: core.Persistence.Postgres.DSN,
			Mode: postgresMode, Acknowledged: acknowledge,
		})
	}

	paths, err := resourceSQLitePaths(configPath, core.Overlay)
	if err != nil {
		return upgrade.BackupPlan{}, err
	}

	for _, path := range paths {
		if path == core.Authz.Database {
			continue
		}

		plan.SQLite = append(plan.SQLite, upgrade.SQLiteSource{
			Name: "resource-" + strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
			Path: path,
		})
	}

	moduleSQLite, modulePostgres, err := moduleDatabaseSources(configPath, core, postgresMode, acknowledge)
	if err != nil {
		return upgrade.BackupPlan{}, err
	}

	plan.SQLite = append(plan.SQLite, moduleSQLite...)
	plan.Postgres = append(plan.Postgres, modulePostgres...)

	// A module keeps files of its own below the data root, and a failed trial
	// may migrate them. The SQLite databases there are snapshotted above.
	if dataRoot := moduleDataRoot(core); dataRoot != "" {
		var databases []string
		for _, source := range moduleSQLite {
			for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
				databases = append(databases, source.Path+suffix)
			}
		}
		plan.Files = append(plan.Files, upgrade.FileSource{
			Name: "module-data", Path: dataRoot, Directory: true, Exclude: databases,
		})
	}

	return plan, nil
}

func moduleDatabaseSources(
	configPath string,
	core config.Core,
	postgresMode upgrade.PostgresMode,
	acknowledge bool,
) ([]upgrade.SQLiteSource, []upgrade.PostgresSource, error) {
	document, err := resource.Load(resource.Paths{File: configPath, Overlay: core.Overlay})
	if err != nil {
		return nil, nil, err
	}

	var (
		sqliteSources   []upgrade.SQLiteSource
		postgresSources []upgrade.PostgresSource
	)

	dataRoot := moduleDataRoot(core)

	for _, declared := range document.Instances {
		service := declared.Spec.Service
		if service == nil || service.Database == nil {
			continue
		}

		name := "module-" + declared.Ref.ID()

		switch service.Database.Engine {
		case wirerpc.DatabaseSQLite:
			sqliteSources = append(sqliteSources, upgrade.SQLiteSource{
				Name: name, Path: filepath.Join(dataRoot, declared.Ref.ID(), sqliteDatabaseName),
			})
		case wirerpc.DatabasePostgres:
			restoreDSN := service.Database.DSN
			if service.Database.Provisioned {
				restoreDSN = moduleRestoreDSN(core)
			}
			postgresSources = append(postgresSources, upgrade.PostgresSource{
				Name: name, DSN: service.Database.DSN, RestoreDSN: restoreDSN,
				Mode: postgresMode, Acknowledged: acknowledge,
			})
		}
	}

	left, err := resource.LeftDatabases(core.Overlay)
	if err != nil {
		return nil, nil, err
	}

	for index, database := range left {
		name := fmt.Sprintf("module-left-%d", index+1)

		switch wirerpc.DatabaseEngine(database.Database.Engine) {
		case wirerpc.DatabaseSQLite:
			instance, parseErr := database.Ref()
			if parseErr != nil {
				return nil, nil, parseErr
			}

			sqliteSources = append(sqliteSources, upgrade.SQLiteSource{
				Name: name, Path: filepath.Join(dataRoot, instance.ID(), sqliteDatabaseName),
			})
		case wirerpc.DatabasePostgres:
			restoreDSN := database.Database.DSN
			if database.Database.Provisioned {
				restoreDSN = moduleRestoreDSN(core)
			}
			postgresSources = append(postgresSources, upgrade.PostgresSource{
				Name: name, DSN: database.Database.DSN, RestoreDSN: restoreDSN,
				Mode: postgresMode, Acknowledged: acknowledge,
			})
		}
	}

	return sqliteSources, postgresSources, nil
}

func moduleRestoreDSN(core config.Core) string {
	if core.Persistence.Postgres.ModuleAdminDSN != "" {
		return core.Persistence.Postgres.ModuleAdminDSN
	}

	return core.Persistence.Postgres.DSN
}

func postgresRestoreTargets(plan upgrade.BackupPlan) map[string]upgrade.PostgresRestoreTarget {
	targets := make(map[string]upgrade.PostgresRestoreTarget, len(plan.Postgres))
	for _, source := range plan.Postgres {
		targets[source.Name] = upgrade.PostgresRestoreTarget{
			DSN: source.DSN, AdminDSN: source.RestoreDSN,
		}
	}

	return targets
}

func resourceSQLitePaths(paths ...string) ([]string, error) {
	unique := map[string]bool{}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read database declarations from %s: %w", path, err)
		}

		var document any
		if err = yaml.Unmarshal(content, &document); err != nil {
			return nil, fmt.Errorf("read database declarations from %s: %w", path, err)
		}

		collectSQLitePaths(document, unique)
	}

	out := slices.Collect(func(yield func(string) bool) {
		for path := range unique {
			if !yield(path) {
				return
			}
		}
	})
	slices.Sort(out)

	return out, nil
}

func collectSQLitePaths(value any, out map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		if kind, hasKind := typed["type"].(string); hasKind && kind == "sqlite" {
			if settings, hasConfig := typed["config"].(map[string]any); hasConfig {
				if path, hasPath := settings["path"].(string); hasPath && path != "" {
					out[path] = true
				}
			}
		}

		for _, child := range typed {
			collectSQLitePaths(child, out)
		}
	case []any:
		for _, child := range typed {
			collectSQLitePaths(child, out)
		}
	}
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}

	return nil
}
