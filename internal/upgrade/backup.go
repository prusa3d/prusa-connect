package upgrade

//revive:disable:file-length-limit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prusa3d/prusa-connect/internal/kernel/atomicfile"

	// Register the pure-Go SQLite driver used for snapshot integrity checks.
	_ "modernc.org/sqlite"
)

const (
	backupFormat           = 2
	backupPerm             = 0o600
	backupDir              = 0o700
	protectedKindFile      = "file"
	protectedKindDirectory = "directory"
	protectedKindMissing   = "missing"
	restoreNameRandomBytes = 6
	restoreNamePrefixLimit = 40
)

var (
	ErrBackupInvalid       = errors.New("upgrade: invalid backup")
	ErrBackupIntegrity     = errors.New("upgrade: backup integrity check failed")
	ErrPostgresUnconfirmed = errors.New("upgrade: PostgreSQL backup responsibility is not acknowledged")
)

type PostgresMode string

const (
	PostgresDump     PostgresMode = "pg_dump"
	PostgresExternal PostgresMode = "external"
	PostgresNone     PostgresMode = "none"
)

type SQLiteSource struct {
	Name string
	Path string
}

type FileSource struct {
	Name      string
	Path      string
	Directory bool
	// Exclude names files below a directory source that another source
	// snapshots, such as a SQLite database with its sidecar files. Restore
	// removes them with the other files the backup does not hold, and the
	// other source puts them back.
	Exclude []string
}

type PostgresSource struct {
	Name         string
	DSN          string
	RestoreDSN   string
	Mode         PostgresMode
	Acknowledged bool
}

type PostgresRestoreTarget struct {
	DSN      string
	AdminDSN string
}

type BackupPlan struct {
	ID          string
	FromVersion string
	ToVersion   string
	SQLite      []SQLiteSource
	Files       []FileSource
	Postgres    []PostgresSource
	Storage     []string
}

type BackupManager struct {
	Root          string
	Keep          int
	PGDumpPath    string
	PGRestorePath string
	PSQLPath      string
	Now           func() time.Time
}

type BackupManifest struct {
	Format       int               `json:"format"                  doc:"The backup manifest format version."`
	ID           string            `json:"id"                      doc:"The unique backup identifier."`
	CreatedAt    time.Time         `json:"created_at"              doc:"The instant the backup completed."`
	FromVersion  string            `json:"from_version,omitempty"  doc:"The server version that created the backup."`
	ToVersion    string            `json:"to_version,omitempty"    doc:"The intended update target, when known."`
	Directories  []BackupDirectory `json:"directories,omitempty"   doc:"Legacy exact directories retained for API compatibility."`
	Protected    []ProtectedPath   `json:"protected,omitempty"     doc:"The filesystem boundaries restored exactly from this backup."`
	Files        []BackupFile      `json:"files"                   doc:"The protected local files in this backup."`
	Postgres     []PostgresEntry   `json:"postgres,omitempty"      doc:"The PostgreSQL protection recorded for each database."`
	Storage      []string          `json:"storage,omitempty"       doc:"The storage roots excluded from the backup."`
	RestoredFrom string            `json:"restored_from,omitempty" doc:"The backup identifier from which this manifest was restored."`
}

type ProtectedPath struct {
	Name        string               `json:"name"                  doc:"The stable logical name of the protected path."`
	Source      string               `json:"source"                doc:"The original absolute path of the protected path."`
	Resolved    string               `json:"resolved"              doc:"The canonical protected path used to detect changed parent links."`
	Kind        string               `json:"kind"                  doc:"The original path kind: file, directory or missing."`
	Mode        uint32               `json:"mode,omitempty"        doc:"The original directory permission bits."`
	Directories []ProtectedDirectory `json:"directories,omitempty" doc:"The directories originally present below a protected directory."`
}

type BackupDirectory struct {
	Name    string `json:"name"    doc:"The stable logical name of the protected directory."`
	Source  string `json:"source"  doc:"The original path of the protected directory."`
	Mode    uint32 `json:"mode"    doc:"The original directory permission bits."`
	Existed bool   `json:"existed" doc:"Whether the directory existed when the snapshot was taken."`
}

type ProtectedDirectory struct {
	Path string `json:"path" doc:"The directory path relative to its protected root."`
	Mode uint32 `json:"mode" doc:"The original directory permission bits."`
}

type BackupFile struct {
	Name     string         `json:"name"               doc:"The stable logical name of the protected file."`
	Source   string         `json:"source"             doc:"The original path of the protected file."`
	Path     string         `json:"path"               doc:"The file path relative to the backup directory."`
	Kind     string         `json:"kind"               doc:"The protected file kind, such as file or sqlite."`
	Mode     uint32         `json:"mode"               doc:"The original file permission bits."`
	Size     int64          `json:"size"               doc:"The protected file size in bytes."`
	SHA256   string         `json:"sha256"             doc:"The lowercase SHA-256 digest of the protected file."`
	Database *DatabaseState `json:"database,omitempty" doc:"The schema state recorded for a protected database."`
}

type DatabaseState struct {
	Engine         string            `json:"engine"                    doc:"The database engine used by this store."`
	Version        int               `json:"version"                   doc:"The highest migration number recorded by the store, read from the migration names."`
	Floor          int               `json:"floor"                     doc:"The oldest schema version allowed to open this store."`
	FloorMigration string            `json:"floor_migration,omitempty" doc:"The migration a binary must know to open this store, when the store records one."`
	Ledger         []MigrationRecord `json:"ledger,omitempty"          doc:"The ordered migration ledger recorded by the store."`
}

type MigrationRecord struct {
	Name     string `json:"name"     doc:"The stable migration name."`
	Checksum string `json:"checksum" doc:"The recorded migration checksum."`
	Version  int    `json:"version"  doc:"The migration version number."`
}

type PostgresEntry struct {
	Name   string       `json:"name"             doc:"The stable logical database name."`
	Mode   PostgresMode `json:"mode"             doc:"The configured PostgreSQL backup strategy."`
	Path   string       `json:"path,omitempty"   doc:"The dump path relative to the backup directory."`
	Size   int64        `json:"size,omitempty"   doc:"The PostgreSQL dump size in bytes."`
	SHA256 string       `json:"sha256,omitempty" doc:"The lowercase SHA-256 digest of the dump."`
}

//nolint:gocognit // the branches are the ordered stages of one atomic backup transaction
func (m BackupManager) Create(ctx context.Context, plan BackupPlan) (BackupManifest, error) {
	if m.Root == "" {
		return BackupManifest{}, fmt.Errorf("%w: backup root is empty", ErrBackupInvalid)
	}

	now := time.Now
	if m.Now != nil {
		now = m.Now
	}

	created := now().UTC()
	id := plan.ID
	if id == "" {
		id = created.Format("20060102T150405Z") + "-" + safeName(plan.FromVersion)
		// A backup made by hand has no target version, so its name names none.
		if plan.ToVersion != "" {
			id += "-to-" + safeName(plan.ToVersion)
		}
	}

	if err := validBackupID(id); err != nil {
		return BackupManifest{}, err
	}

	if err := os.MkdirAll(m.Root, backupDir); err != nil {
		return BackupManifest{}, fmt.Errorf("create the backup root: %w", err)
	}

	stage, err := os.MkdirTemp(m.Root, ".creating-"+id+"-")
	if err != nil {
		return BackupManifest{}, fmt.Errorf("stage backup %s: %w", id, err)
	}

	if err = os.Chmod(stage, backupDir); err != nil {
		_ = os.RemoveAll(stage)

		return BackupManifest{}, fmt.Errorf("protect staged backup %s: %w", id, err)
	}

	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stage)
		}
	}()

	manifest := BackupManifest{
		Format: backupFormat, ID: id, CreatedAt: created,
		FromVersion: plan.FromVersion, ToVersion: plan.ToVersion,
		Storage: slices.Clone(plan.Storage),
	}

	for _, source := range plan.SQLite {
		entry, snapshotErr := snapshotSQLite(ctx, stage, source)
		if snapshotErr != nil {
			return BackupManifest{}, snapshotErr
		}

		manifest.Files = append(manifest.Files, entry)
	}

	for _, source := range plan.Files {
		protected, entries, copyErr := snapshotFiles(stage, source)
		if copyErr != nil {
			return BackupManifest{}, copyErr
		}

		manifest.Protected = append(manifest.Protected, protected)
		manifest.Files = append(manifest.Files, entries...)
	}

	for _, source := range plan.Postgres {
		entry, dumpErr := m.snapshotPostgres(ctx, stage, source)
		if dumpErr != nil {
			return BackupManifest{}, dumpErr
		}

		manifest.Postgres = append(manifest.Postgres, entry)
	}

	slices.SortFunc(manifest.Files, func(a, b BackupFile) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(manifest.Protected, func(a, b ProtectedPath) int {
		return strings.Compare(a.Source, b.Source)
	})
	if err = validateProtectedPaths(manifest); err != nil {
		return BackupManifest{}, err
	}

	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BackupManifest{}, fmt.Errorf("render backup manifest: %w", err)
	}

	if err = atomicfile.Write(filepath.Join(stage, "manifest.json"), append(content, '\n'), backupPerm); err != nil {
		return BackupManifest{}, fmt.Errorf("write backup manifest: %w", err)
	}

	destination := filepath.Join(m.Root, id)
	if _, err = os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return BackupManifest{}, fmt.Errorf("%w: backup %s already exists", ErrBackupInvalid, id)
		}

		return BackupManifest{}, fmt.Errorf("inspect backup destination: %w", err)
	}

	if err = os.Rename(stage, destination); err != nil {
		return BackupManifest{}, fmt.Errorf("publish backup %s: %w", id, err)
	}

	published = true

	if err = syncDirectory(m.Root); err != nil {
		return BackupManifest{}, err
	}

	if err = m.prune(); err != nil {
		return BackupManifest{}, err
	}

	return manifest, nil
}

func (m BackupManager) List() ([]BackupManifest, error) {
	entries, err := os.ReadDir(m.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	var out []BackupManifest

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		manifest, readErr := m.Read(entry.Name())
		if readErr != nil {
			return nil, readErr
		}

		out = append(out, manifest)
	}

	slices.SortFunc(out, func(a, b BackupManifest) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})

	return out, nil
}

func (m BackupManager) Read(id string) (BackupManifest, error) {
	if err := validBackupID(id); err != nil {
		return BackupManifest{}, err
	}

	content, err := os.ReadFile(filepath.Join(m.Root, id, "manifest.json"))
	if err != nil {
		return BackupManifest{}, fmt.Errorf("read backup %s: %w", id, err)
	}

	var manifest BackupManifest
	if err = json.Unmarshal(content, &manifest); err != nil {
		return BackupManifest{}, fmt.Errorf("%w: parse backup %s: %w", ErrBackupInvalid, id, err)
	}

	if manifest.Format != backupFormat || manifest.ID != id {
		return BackupManifest{}, fmt.Errorf("%w: backup %s has format %d and id %q",
			ErrBackupInvalid, id, manifest.Format, manifest.ID)
	}

	return manifest, nil
}

func (m BackupManager) Delete(id string) error {
	if _, err := m.Read(id); err != nil {
		return err
	}

	if err := os.RemoveAll(filepath.Join(m.Root, id)); err != nil {
		return fmt.Errorf("delete backup %s: %w", id, err)
	}

	return syncDirectory(m.Root)
}

func (m BackupManager) Verify(id string) (BackupManifest, error) {
	manifest, err := m.Read(id)
	if err != nil {
		return BackupManifest{}, err
	}

	if err = validateProtectedPaths(manifest); err != nil {
		return BackupManifest{}, err
	}

	root := filepath.Join(m.Root, id)
	for _, entry := range manifest.Files {
		if err = verifyBackupFile(root, entry); err != nil {
			return BackupManifest{}, err
		}
	}

	for _, entry := range manifest.Postgres {
		if entry.Mode == PostgresDump {
			if err = verifyDigest(filepath.Join(root, entry.Path), entry.Size, entry.SHA256); err != nil {
				return BackupManifest{}, err
			}
		}
	}

	return manifest, nil
}

// Restore puts every file, SQLite database and dumped PostgreSQL database of a
// backup back. See RestoreReporting for the databases it leaves alone.
func (m BackupManager) Restore(
	ctx context.Context, id string, postgres map[string]PostgresRestoreTarget,
) error {
	_, err := m.RestoreReporting(ctx, id, postgres)

	return err
}

// RestoreReporting restores a backup and returns the PostgreSQL databases it
// did not restore. A database in external or none mode has no dump in the
// backup: the operator acknowledged when the backup was taken that they
// restore it themselves, so it never blocks the local files and binary.
//
//nolint:gocognit // restore deliberately keeps verification, reconciliation and publication in order
func (m BackupManager) RestoreReporting(
	ctx context.Context, id string, postgres map[string]PostgresRestoreTarget,
) ([]PostgresEntry, error) {
	manifest, err := m.Verify(id)
	if err != nil {
		return nil, err
	}

	root := filepath.Join(m.Root, id)

	var external []PostgresEntry
	for _, entry := range manifest.Postgres {
		switch entry.Mode {
		case PostgresDump:
			if postgres[entry.Name].DSN == "" {
				return nil, fmt.Errorf("%w: no restore DSN for PostgreSQL database %s", ErrBackupInvalid, entry.Name)
			}
		case PostgresExternal, PostgresNone:
			external = append(external, entry)
		default:
			return nil, fmt.Errorf("%w: PostgreSQL database %s has mode %q", ErrBackupInvalid, entry.Name, entry.Mode)
		}
	}

	for _, protected := range manifest.Protected {
		if err = prepareProtectedPath(protected, manifest.Files); err != nil {
			return nil, err
		}
	}

	for _, entry := range manifest.Files {
		if err = restoreFile(root, entry); err != nil {
			return nil, err
		}
	}

	for _, protected := range manifest.Protected {
		if protected.Kind == protectedKindDirectory {
			if err = restoreDirectoryModes(protected); err != nil {
				return nil, err
			}
		}
	}

	for _, entry := range manifest.Postgres {
		if entry.Mode != PostgresDump {
			continue
		}
		if err = m.restorePostgres(ctx, filepath.Join(root, entry.Path), postgres[entry.Name]); err != nil {
			return nil, err
		}
	}

	return external, nil
}

func snapshotSQLite(ctx context.Context, root string, source SQLiteSource) (BackupFile, error) {
	if source.Name == "" || source.Path == "" {
		return BackupFile{}, fmt.Errorf("%w: SQLite source needs a name and path", ErrBackupInvalid)
	}

	// A database path in the configuration is relative to the directory the
	// server runs in. The backup records it absolute, as it records a file, so
	// a restore run from another directory puts the database back where the
	// server reads it, beside the files of the same backup.
	absolute, err := filepath.Abs(source.Path)
	if err != nil {
		return BackupFile{}, fmt.Errorf("resolve SQLite database %s: %w", source.Name, err)
	}

	source.Path = absolute

	if _, err = os.Stat(source.Path); err != nil {
		return BackupFile{}, fmt.Errorf("inspect SQLite database %s: %w", source.Name, err)
	}

	relative := filepath.Join("sqlite", safeName(source.Name)+".db")
	destination := filepath.Join(root, relative)
	if err = os.MkdirAll(filepath.Dir(destination), backupDir); err != nil {
		return BackupFile{}, fmt.Errorf("create SQLite backup directory: %w", err)
	}

	db, err := sql.Open("sqlite", source.Path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return BackupFile{}, fmt.Errorf("open SQLite database %s: %w", source.Name, err)
	}

	defer func() { _ = db.Close() }()

	// #nosec G202 -- SQLite has no parameter form for VACUUM INTO; quotes are escaped above.
	statement := `VACUUM INTO '` + strings.ReplaceAll(destination, `'`, `''`) + `'`
	if _, err = db.ExecContext(ctx, statement); err != nil {
		return BackupFile{}, fmt.Errorf("snapshot SQLite database %s: %w", source.Name, err)
	}

	if err = checkSQLite(ctx, destination); err != nil {
		return BackupFile{}, fmt.Errorf("%s: %w", source.Name, err)
	}

	entry, err := describeBackupFile(source.Name, source.Path, relative, "sqlite", destination)
	if err != nil {
		return BackupFile{}, err
	}

	entry.Database, err = inspectSQLite(ctx, destination)

	return entry, err
}

func checkSQLite(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("%w: open snapshot: %w", ErrBackupIntegrity, err)
	}

	defer func() { _ = db.Close() }()

	var result string
	if err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("%w: run SQLite integrity check: %w", ErrBackupIntegrity, err)
	}

	if result != "ok" {
		return fmt.Errorf("%w: SQLite reports %s", ErrBackupIntegrity, result)
	}

	return nil
}

func inspectSQLite(ctx context.Context, path string) (*DatabaseState, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("inspect SQLite snapshot: %w", err)
	}

	defer func() { _ = db.Close() }()

	state := &DatabaseState{Engine: "sqlite"}

	for _, ledger := range []struct {
		table      string
		floorTable string
	}{
		{"schema_migration", "schema_meta"},
		{"history_migration", "history_schema"},
		{"module_state_migration", "module_state_schema"},
	} {
		exists, existsErr := sqliteTableExists(ctx, db, ledger.table)
		if existsErr != nil {
			return nil, existsErr
		}
		if !exists {
			continue
		}

		records, recordsErr := sqliteLedger(ctx, db, ledger.table)
		if recordsErr != nil {
			return nil, recordsErr
		}

		state.Ledger = append(state.Ledger, records...)
		for _, record := range records {
			state.Version = max(state.Version, record.Version)
		}

		floor, floorName, floorErr := sqliteFloor(ctx, db, ledger.floorTable)
		if floorErr != nil {
			return nil, floorErr
		}
		state.Floor = max(state.Floor, floor)
		if floorName != "" {
			state.FloorMigration = floorName
		}
	}

	return state, nil
}

// sqliteFloor reads a store's compatibility floor, as a number and, where the
// store records one, as a migration name.
func sqliteFloor(ctx context.Context, db *sql.DB, table string) (int, string, error) {
	exists, err := sqliteTableExists(ctx, db, table)
	if err != nil || !exists {
		return 0, "", err
	}

	var floor int
	query := `SELECT min_schema_version FROM ` + table + ` WHERE id = 1` // #nosec G202 -- fixed table names
	if err = db.QueryRowContext(ctx, query).Scan(&floor); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("read SQLite compatibility floor: %w", err)
	}
	name, err := sqliteFloorMigration(ctx, db, table)

	return floor, name, err
}

// sqliteFloorMigration reads the floor a store records as a migration name.
// A store written before the column existed has none.
func sqliteFloorMigration(ctx context.Context, db *sql.DB, table string) (string, error) {
	var columns int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'min_migration'`, table).Scan(&columns); err != nil {
		return "", fmt.Errorf("inspect SQLite compatibility floor %s: %w", table, err)
	}
	if columns == 0 {
		return "", nil
	}

	var name string
	// #nosec G202 -- the table comes from the fixed ledger declarations.
	query := `SELECT min_migration FROM ` + table + ` WHERE id = 1`
	if err := db.QueryRowContext(ctx, query).Scan(&name); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read SQLite compatibility floor %s: %w", table, err)
	}

	return name, nil
}

// sqliteLedger reads one migration ledger. A record's number comes from its
// name: the version column a ledger may carry is not written by every
// migrator, so it is never read.
func sqliteLedger(ctx context.Context, db *sql.DB, table string) ([]MigrationRecord, error) {
	// #nosec G202 -- the table comes from the fixed ledger declarations.
	query := `SELECT name, checksum FROM ` + table + ` ORDER BY name`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read SQLite migration ledger %s: %w", table, err)
	}

	defer func() { _ = rows.Close() }()

	var records []MigrationRecord
	for rows.Next() {
		var record MigrationRecord
		if err = rows.Scan(&record.Name, &record.Checksum); err != nil {
			return nil, fmt.Errorf("read SQLite migration ledger %s: %w", table, err)
		}
		record.Version = migrationVersion(record.Name)
		records = append(records, record)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read SQLite migration ledger %s: %w", table, err)
	}

	return records, nil
}

func sqliteTableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect SQLite table %s: %w", table, err)
	}

	return count > 0, nil
}

func migrationVersion(name string) int {
	base := filepath.Base(name)
	prefix, _, found := strings.Cut(base, "_")
	if !found {
		return 0
	}

	version, _ := strconv.Atoi(prefix)

	return version
}

//nolint:gocognit // snapshot traversal validates every filesystem kind before copying it
func snapshotFiles(root string, source FileSource) (ProtectedPath, []BackupFile, error) {
	if source.Name == "" || source.Path == "" {
		return ProtectedPath{}, nil, fmt.Errorf("%w: file source needs a name and path", ErrBackupInvalid)
	}
	absolute, err := filepath.Abs(source.Path)
	if err != nil {
		return ProtectedPath{}, nil, fmt.Errorf("resolve backup source %s: %w", source.Name, err)
	}
	source.Path = filepath.Clean(absolute)

	info, err := os.Lstat(source.Path)
	if errors.Is(err, os.ErrNotExist) {
		resolved, resolveErr := canonicalDestination(source.Path)
		if resolveErr != nil {
			return ProtectedPath{}, nil, fmt.Errorf("resolve backup source %s: %w", source.Name, resolveErr)
		}

		return ProtectedPath{
			Name: source.Name, Source: source.Path, Resolved: resolved, Kind: protectedKindMissing,
		}, nil, nil
	}
	if err != nil {
		return ProtectedPath{}, nil, fmt.Errorf("inspect backup source %s: %w", source.Name, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return ProtectedPath{}, nil, fmt.Errorf("%w: backup source %s is a symbolic link",
			ErrBackupInvalid, source.Path)
	}
	resolved, err := canonicalDestination(source.Path)
	if err != nil {
		return ProtectedPath{}, nil, fmt.Errorf("resolve backup source %s: %w", source.Name, err)
	}

	if !info.IsDir() {
		if source.Directory {
			return ProtectedPath{}, nil, fmt.Errorf(
				"%w: backup source %s is not a directory", ErrBackupInvalid, source.Path,
			)
		}
		entry, copyErr := copyBackupFile(root, source, filepath.Base(source.Path), source.Path)
		if copyErr != nil {
			return ProtectedPath{}, nil, copyErr
		}

		return ProtectedPath{
				Name: source.Name, Source: source.Path, Resolved: resolved, Kind: protectedKindFile,
			},
			[]BackupFile{entry}, nil
	}
	if filepath.Dir(source.Path) == source.Path {
		return ProtectedPath{}, nil, fmt.Errorf("%w: filesystem root cannot be a protected directory",
			ErrBackupInvalid)
	}

	protected := ProtectedPath{
		Name: source.Name, Source: source.Path, Resolved: resolved,
		Kind: protectedKindDirectory, Mode: uint32(info.Mode().Perm()),
	}
	var entries []BackupFile
	excluded := make(map[string]bool, len(source.Exclude))
	for _, path := range source.Exclude {
		if absolutePath, absErr := filepath.Abs(path); absErr == nil {
			excluded[filepath.Clean(absolutePath)] = true
		}
	}

	err = filepath.WalkDir(source.Path, func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() {
			dirInfo, infoErr := item.Info()
			if infoErr != nil {
				return infoErr
			}
			relative, relativeErr := filepath.Rel(source.Path, path)
			if relativeErr != nil {
				return relativeErr
			}
			protected.Directories = append(protected.Directories, ProtectedDirectory{
				Path: filepath.ToSlash(relative), Mode: uint32(dirInfo.Mode().Perm()),
			})

			return nil
		}
		if excluded[path] {
			return nil
		}
		if item.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: backup source %s contains symbolic link %s",
				ErrBackupInvalid, source.Name, path)
		}

		relative, relativeErr := filepath.Rel(source.Path, path)
		if relativeErr != nil {
			return relativeErr
		}

		entry, copyErr := copyBackupFile(root, source, relative, path)
		if copyErr != nil {
			return copyErr
		}

		entries = append(entries, entry)

		return nil
	})
	if err != nil {
		return ProtectedPath{}, nil, fmt.Errorf("snapshot files from %s: %w", source.Name, err)
	}

	return protected, entries, nil
}

func copyBackupFile(root string, source FileSource, relative, path string) (BackupFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return BackupFile{}, err
	}

	backupPath := filepath.Join("files", safeName(source.Name), relative)
	destination := filepath.Join(root, backupPath)

	if copyErr := copyFile(path, destination, backupPerm); copyErr != nil {
		return BackupFile{}, fmt.Errorf("snapshot %s: %w", path, copyErr)
	}

	entry, err := describeBackupFile(source.Name, path, backupPath, protectedKindFile, destination)
	entry.Mode = uint32(info.Mode().Perm())

	return entry, err
}

func (m BackupManager) snapshotPostgres(
	ctx context.Context, root string, source PostgresSource,
) (PostgresEntry, error) {
	if source.Name == "" || source.DSN == "" {
		return PostgresEntry{}, fmt.Errorf("%w: PostgreSQL source needs a name and DSN", ErrBackupInvalid)
	}

	entry := PostgresEntry{Name: source.Name, Mode: source.Mode}

	switch source.Mode {
	case PostgresDump:
		if err := m.verifyPostgresRestore(ctx, source); err != nil {
			return PostgresEntry{}, err
		}
		dump := m.PGDumpPath
		if dump == "" {
			dump = "pg_dump"
		}

		entry.Path = filepath.Join("postgres", safeName(source.Name)+".dump")
		destination := filepath.Join(root, entry.Path)
		if err := os.MkdirAll(filepath.Dir(destination), backupDir); err != nil {
			return PostgresEntry{}, fmt.Errorf("create PostgreSQL backup directory: %w", err)
		}

		command := postgresCommand(ctx, dump, source.DSN, func(dsn string) []string {
			return []string{"--format=custom", "--create", "--file=" + destination, dsn}
		})
		if output, err := command.CombinedOutput(); err != nil {
			return PostgresEntry{}, fmt.Errorf("back up PostgreSQL database %s: %w: %s",
				source.Name, err, strings.TrimSpace(string(output)))
		}

		if err := os.Chmod(destination, backupPerm); err != nil {
			return PostgresEntry{}, fmt.Errorf("protect PostgreSQL backup %s: %w", source.Name, err)
		}

		info, digest, err := fileDigest(destination)
		if err != nil {
			return PostgresEntry{}, fmt.Errorf("hash PostgreSQL backup %s: %w", source.Name, err)
		}

		entry.Size = info.Size()
		entry.SHA256 = digest

		return entry, nil
	case PostgresExternal, PostgresNone:
		if !source.Acknowledged {
			return PostgresEntry{}, fmt.Errorf("%w: %s uses mode %s",
				ErrPostgresUnconfirmed, source.Name, source.Mode)
		}

		return entry, nil
	default:
		return PostgresEntry{}, fmt.Errorf("%w: PostgreSQL mode %q", ErrBackupInvalid, source.Mode)
	}
}

func (m BackupManager) restorePostgres(
	ctx context.Context, dump string, target PostgresRestoreTarget,
) error {
	restore := m.PGRestorePath
	if restore == "" {
		restore = "pg_restore"
	}
	psql := m.PSQLPath
	if psql == "" {
		psql = "psql"
	}

	database, owner, err := postgresIdentity(target.DSN)
	if err != nil {
		return err
	}
	adminDSN := target.AdminDSN
	if adminDSN == "" {
		adminDSN = target.DSN
	}
	admin, err := postgresAdminDSN(adminDSN)
	if err != nil {
		return err
	}
	temporary, previous, err := restoreDatabaseNames(database)
	if err != nil {
		return err
	}
	if err = runPSQL(ctx, psql, admin, map[string]string{
		"database": temporary,
		"owner":    owner,
	}, `CREATE DATABASE :"database" OWNER :"owner";`); err != nil {
		return err
	}

	temporaryDSN, err := postgresDatabaseDSN(target.DSN, temporary)
	if err != nil {
		return err
	}
	command := postgresCommand(ctx, restore, temporaryDSN, func(dsn string) []string {
		return []string{"--no-owner", "--dbname=" + dsn, dump}
	})
	if output, commandErr := command.CombinedOutput(); commandErr != nil {
		_ = dropRestoreDatabase(ctx, psql, admin, temporary)

		return fmt.Errorf("restore PostgreSQL backup: %w: %s",
			commandErr, strings.TrimSpace(string(output)))
	}
	if err = runPSQL(ctx, psql, admin, map[string]string{
		"target":    database,
		"temporary": temporary,
		"previous":  previous,
	}, `
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = :'target' AND pid <> pg_backend_pid();
BEGIN;
ALTER DATABASE :"target" RENAME TO :"previous";
ALTER DATABASE :"temporary" RENAME TO :"target";
COMMIT;
SELECT format(
  'COMMENT ON DATABASE %I IS %L',
  :'target',
  shobj_description(oid, 'pg_database')
)
FROM pg_database
WHERE datname = :'previous'
\gexec
REVOKE ALL PRIVILEGES ON DATABASE :"target" FROM PUBLIC;
SELECT format(
  'GRANT %s ON DATABASE %I TO %s%s',
  privilege_type,
  :'target',
  CASE WHEN grantee = 0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(grantee)) END,
  CASE WHEN is_grantable THEN ' WITH GRANT OPTION' ELSE '' END
)
FROM pg_database
CROSS JOIN LATERAL aclexplode(COALESCE(datacl, acldefault('d', datdba)))
WHERE datname = :'previous'
\gexec
SELECT format(
  'ALTER DATABASE %I SET %I TO %L',
  :'target',
  split_part(setting, '=', 1),
  substring(setting FROM position('=' IN setting) + 1)
)
FROM pg_db_role_setting
JOIN pg_database ON oid = setdatabase
CROSS JOIN LATERAL unnest(setconfig) AS setting
WHERE datname = :'previous' AND setrole = 0
\gexec
SELECT format(
  'ALTER ROLE %I IN DATABASE %I SET %I TO %L',
  rolname,
  :'target',
  split_part(setting, '=', 1),
  substring(setting FROM position('=' IN setting) + 1)
)
FROM pg_db_role_setting
JOIN pg_database ON oid = setdatabase
JOIN pg_roles ON pg_roles.oid = setrole
CROSS JOIN LATERAL unnest(setconfig) AS setting
WHERE datname = :'previous' AND setrole <> 0
\gexec
DROP DATABASE :"previous";
`); err != nil {
		return err
	}

	return nil
}

func (m BackupManager) verifyPostgresRestore(ctx context.Context, source PostgresSource) error {
	_, owner, err := postgresIdentity(source.DSN)
	if err != nil {
		return err
	}
	adminDSN := source.RestoreDSN
	if adminDSN == "" {
		adminDSN = source.DSN
	}
	admin, err := postgresAdminDSN(adminDSN)
	if err != nil {
		return err
	}
	psql := m.PSQLPath
	if psql == "" {
		psql = "psql"
	}

	err = runPSQL(ctx, psql, admin, map[string]string{"owner": owner}, `
SELECT rolsuper OR (rolcreatedb AND pg_has_role(current_user, :'owner', 'MEMBER')) AS allowed
FROM pg_roles
WHERE rolname = current_user
\gset
\if :allowed
\else
SELECT 1 / 0;
\endif
`)
	if err != nil {
		return fmt.Errorf(
			"PostgreSQL restore role needs CREATEDB and membership in database owner %s: %w",
			owner, err,
		)
	}

	return nil
}

func runPSQL(
	ctx context.Context, path, dsn string, variables map[string]string, script string,
) error {
	command := postgresCommand(ctx, path, dsn, func(clean string) []string {
		args := []string{"--dbname=" + clean, "--set=ON_ERROR_STOP=1"}
		for _, name := range slices.Sorted(maps.Keys(variables)) {
			args = append(args, "--set="+name+"="+variables[name])
		}

		return args
	})
	command.Stdin = strings.NewReader(script)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("prepare PostgreSQL restore: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func dropRestoreDatabase(ctx context.Context, psql, admin, database string) error {
	return runPSQL(ctx, psql, admin, map[string]string{"database": database}, `
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = :'database' AND pid <> pg_backend_pid();
DROP DATABASE IF EXISTS :"database";
`)
}

func restoreDatabaseNames(database string) (string, string, error) {
	random := make([]byte, restoreNameRandomBytes)
	if _, err := rand.Read(random); err != nil {
		return "", "", fmt.Errorf("name PostgreSQL restore database: %w", err)
	}
	suffix := hex.EncodeToString(random)
	prefix := database
	if len(prefix) > restoreNamePrefixLimit {
		prefix = prefix[:restoreNamePrefixLimit]
	}

	return prefix + "_pcl_new_" + suffix, prefix + "_pcl_old_" + suffix, nil
}

func postgresIdentity(dsn string) (string, string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", "", fmt.Errorf("%w: PostgreSQL restore DSN is not a postgres URL", ErrBackupInvalid)
	}
	database := strings.TrimPrefix(parsed.Path, "/")
	if database == "" || parsed.User == nil || parsed.User.Username() == "" {
		return "", "", fmt.Errorf("%w: PostgreSQL restore DSN needs a database and user", ErrBackupInvalid)
	}

	return database, parsed.User.Username(), nil
}

func postgresDatabaseDSN(dsn, database string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("%w: PostgreSQL restore DSN is not a postgres URL", ErrBackupInvalid)
	}
	parsed.Path = "/" + database

	return parsed.String(), nil
}

func postgresAdminDSN(dsn string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("%w: PostgreSQL restore DSN is not a postgres URL", ErrBackupInvalid)
	}
	database := strings.TrimPrefix(parsed.Path, "/")
	if database == "" {
		return "", fmt.Errorf("%w: PostgreSQL restore DSN names no database", ErrBackupInvalid)
	}
	if database == "postgres" {
		parsed.Path = "/template1"
	} else {
		parsed.Path = "/postgres"
	}

	return parsed.String(), nil
}

//nolint:gocognit,cyclop // manifest validation rejects each unsafe combination before restore mutates disk
func validateProtectedPaths(manifest BackupManifest) error {
	seenNames := make(map[string]struct{}, len(manifest.Protected))
	seenBackupNames := make(map[string]struct{}, len(manifest.Protected))

	for index, protected := range manifest.Protected {
		if protected.Name == "" || protected.Source == "" ||
			!filepath.IsAbs(protected.Source) || filepath.Clean(protected.Source) != protected.Source ||
			!filepath.IsAbs(protected.Resolved) || filepath.Clean(protected.Resolved) != protected.Resolved {
			return fmt.Errorf("%w: protected path %d has an unusable name or source", ErrBackupInvalid, index)
		}
		if _, found := seenNames[protected.Name]; found {
			return fmt.Errorf("%w: protected path name %q is repeated", ErrBackupInvalid, protected.Name)
		}
		seenNames[protected.Name] = struct{}{}
		backupName := safeName(protected.Name)
		if backupName == "" {
			return fmt.Errorf("%w: protected path name %q has no usable characters",
				ErrBackupInvalid, protected.Name)
		}
		if _, found := seenBackupNames[backupName]; found {
			return fmt.Errorf("%w: protected path name %q collides in backup storage",
				ErrBackupInvalid, protected.Name)
		}
		seenBackupNames[backupName] = struct{}{}

		switch protected.Kind {
		case protectedKindFile, protectedKindMissing:
			if len(protected.Directories) != 0 {
				return fmt.Errorf("%w: protected %s %s records directories",
					ErrBackupInvalid, protected.Kind, protected.Source)
			}
		case protectedKindDirectory:
			if filepath.Dir(protected.Source) == protected.Source {
				return fmt.Errorf("%w: filesystem root cannot be protected", ErrBackupInvalid)
			}
			if err := validateBackupDirectories(protected); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: protected path %s has kind %q",
				ErrBackupInvalid, protected.Source, protected.Kind)
		}

		for previous := range index {
			other := manifest.Protected[previous]
			if within(other.Source, protected.Source) || within(protected.Source, other.Source) {
				return fmt.Errorf("%w: protected paths %s and %s overlap",
					ErrBackupInvalid, other.Source, protected.Source)
			}
		}
	}

	for _, entry := range manifest.Files {
		if entry.Kind == "sqlite" {
			continue
		}
		if entry.Kind != protectedKindFile {
			return fmt.Errorf("%w: protected file %s has kind %q",
				ErrBackupInvalid, entry.Source, entry.Kind)
		}
		protected, relative, err := protectedFile(manifest.Protected, entry)
		if err != nil {
			return err
		}
		wantPath := filepath.ToSlash(filepath.Join("files", safeName(protected.Name), relative))
		if entry.Path != wantPath {
			return fmt.Errorf("%w: protected file %s has backup path %q, want %q",
				ErrBackupInvalid, entry.Source, entry.Path, wantPath)
		}
	}

	for _, protected := range manifest.Protected {
		if protected.Kind != protectedKindFile {
			continue
		}
		found := false
		for _, entry := range manifest.Files {
			if entry.Kind == protectedKindFile &&
				entry.Name == protected.Name && entry.Source == protected.Source {
				if found {
					return fmt.Errorf("%w: protected file %s is repeated", ErrBackupInvalid, protected.Source)
				}
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: protected file %s has no backup content",
				ErrBackupInvalid, protected.Source)
		}
	}

	return nil
}

func validateBackupDirectories(protected ProtectedPath) error {
	seen := make(map[string]struct{}, len(protected.Directories))
	for _, directory := range protected.Directories {
		path := filepath.FromSlash(directory.Path)
		if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path ||
			!within(".", path) {
			return fmt.Errorf("%w: protected directory %s has invalid child %q",
				ErrBackupInvalid, protected.Source, directory.Path)
		}
		if _, found := seen[directory.Path]; found {
			return fmt.Errorf("%w: protected directory %s repeats child %q",
				ErrBackupInvalid, protected.Source, directory.Path)
		}
		seen[directory.Path] = struct{}{}
	}
	if _, found := seen["."]; !found {
		return fmt.Errorf("%w: protected directory %s does not record its root",
			ErrBackupInvalid, protected.Source)
	}

	return nil
}

func protectedFile(paths []ProtectedPath, entry BackupFile) (ProtectedPath, string, error) {
	for _, protected := range paths {
		switch protected.Kind {
		case protectedKindFile:
			if entry.Name == protected.Name && entry.Source == protected.Source {
				return protected, filepath.Base(protected.Source), nil
			}
		case protectedKindDirectory:
			if entry.Name != protected.Name || !within(protected.Source, entry.Source) ||
				entry.Source == protected.Source {
				continue
			}
			relative, err := filepath.Rel(protected.Source, entry.Source)
			if err != nil {
				break
			}

			return protected, relative, nil
		}
	}

	return ProtectedPath{}, "", fmt.Errorf("%w: file %s is outside its protected path",
		ErrBackupInvalid, entry.Source)
}

func prepareProtectedPath(protected ProtectedPath, files []BackupFile) error {
	if err := validateResolvedPath(protected); err != nil {
		return err
	}

	switch protected.Kind {
	case protectedKindMissing:
		if err := os.RemoveAll(protected.Source); err != nil {
			return fmt.Errorf("remove path absent from backup %s: %w", protected.Source, err)
		}

		return syncExistingDirectory(filepath.Dir(protected.Source))
	case protectedKindFile:
		info, err := os.Lstat(protected.Source)
		if err == nil && !info.Mode().IsRegular() {
			if err = os.RemoveAll(protected.Source); err != nil {
				return fmt.Errorf("replace protected file %s: %w", protected.Source, err)
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect protected file %s: %w", protected.Source, err)
		}

		return nil
	case protectedKindDirectory:
		return prepareProtectedDirectory(protected, files)
	default:
		return fmt.Errorf("%w: protected path %s has kind %q",
			ErrBackupInvalid, protected.Source, protected.Kind)
	}
}

//nolint:gocognit // exact reconciliation handles type conflicts and unexpected entries in one pass
func prepareProtectedDirectory(protected ProtectedPath, files []BackupFile) error {
	info, err := os.Lstat(protected.Source)
	if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		if err = os.RemoveAll(protected.Source); err != nil {
			return fmt.Errorf("replace protected directory %s: %w", protected.Source, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect protected directory %s: %w", protected.Source, err)
	}
	if err = os.MkdirAll(protected.Source, backupDir); err != nil {
		return fmt.Errorf("create protected directory %s: %w", protected.Source, err)
	}

	expectedDirectories := make(map[string]struct{}, len(protected.Directories))
	for _, directory := range protected.Directories {
		expectedDirectories[filepath.FromSlash(directory.Path)] = struct{}{}
	}
	expectedFiles := make(map[string]struct{})
	for _, entry := range files {
		if entry.Kind != protectedKindFile || entry.Name != protected.Name {
			continue
		}
		relative, relativeErr := filepath.Rel(protected.Source, entry.Source)
		if relativeErr != nil {
			return relativeErr
		}
		expectedFiles[relative] = struct{}{}
	}

	directoryRoot, err := os.OpenRoot(protected.Source)
	if err != nil {
		return fmt.Errorf("open protected directory %s: %w", protected.Source, err)
	}
	defer func() { _ = directoryRoot.Close() }()

	changedDirectories := map[string]struct{}{}
	err = fs.WalkDir(directoryRoot.FS(), ".", func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		relative := filepath.FromSlash(path)
		_, wantDirectory := expectedDirectories[relative]
		_, wantFile := expectedFiles[relative]
		if wantDirectory && item.IsDir() && item.Type()&os.ModeSymlink == 0 ||
			wantFile && item.Type().IsRegular() {
			return nil
		}
		if removeErr := directoryRoot.RemoveAll(relative); removeErr != nil {
			return removeErr
		}
		changedDirectories[filepath.Join(protected.Source, filepath.Dir(relative))] = struct{}{}
		if item.IsDir() {
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("clean protected directory %s: %w", protected.Source, err)
	}

	for _, directory := range protected.Directories {
		path := filepath.Join(protected.Source, filepath.FromSlash(directory.Path))
		if err = os.MkdirAll(path, fs.FileMode(directory.Mode).Perm()); err != nil {
			return fmt.Errorf("recreate protected directory %s: %w", path, err)
		}
	}
	for directory := range changedDirectories {
		if err = syncDirectory(directory); err != nil {
			return err
		}
	}

	return syncDirectory(protected.Source)
}

func restoreDirectoryModes(protected ProtectedPath) error {
	for _, directory := range slices.Backward(protected.Directories) {
		path := filepath.Join(protected.Source, filepath.FromSlash(directory.Path))
		if err := os.Chmod(path, fs.FileMode(directory.Mode).Perm()); err != nil {
			return fmt.Errorf("restore directory mode %s: %w", path, err)
		}
	}

	return syncDirectory(protected.Source)
}

func validateResolvedPath(protected ProtectedPath) error {
	resolved, err := canonicalDestination(protected.Source)
	if err != nil {
		return fmt.Errorf("resolve protected path %s: %w", protected.Source, err)
	}
	if resolved != protected.Resolved {
		return fmt.Errorf("%w: protected path %s now resolves to %s, originally %s",
			ErrBackupInvalid, protected.Source, resolved, protected.Resolved)
	}

	return nil
}

func canonicalDestination(path string) (string, error) {
	existing := path
	var suffix []string
	for {
		_, err := os.Lstat(existing)
		if err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(existing)
			if resolveErr != nil {
				return "", resolveErr
			}
			for _, component := range slices.Backward(suffix) {
				resolved = filepath.Join(resolved, component)
			}

			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", err
		}
		suffix = append(suffix, filepath.Base(existing))
		existing = parent
	}
}

func syncExistingDirectory(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	return syncDirectory(path)
}

func restoreFile(root string, entry BackupFile) error {
	source := filepath.Join(root, filepath.Clean(entry.Path))
	if !within(root, source) {
		return fmt.Errorf("%w: backup file leaves its root: %s", ErrBackupInvalid, entry.Path)
	}

	mode := fs.FileMode(entry.Mode)
	if mode == 0 {
		mode = backupPerm
	}

	if err := copyFile(source, entry.Source, mode); err != nil {
		return fmt.Errorf("restore %s: %w", entry.Name, err)
	}

	if entry.Kind == "sqlite" {
		for _, suffix := range []string{"-wal", "-shm"} {
			if err := os.Remove(entry.Source + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove stale SQLite sidecar %s: %w", entry.Source+suffix, err)
			}
		}
	}

	return nil
}

func verifyBackupFile(root string, entry BackupFile) error {
	path := filepath.Join(root, filepath.Clean(entry.Path))
	if !within(root, path) {
		return fmt.Errorf("%w: backup file leaves its root: %s", ErrBackupInvalid, entry.Path)
	}

	return verifyDigest(path, entry.Size, entry.SHA256)
}

func verifyDigest(path string, wantSize int64, wantDigest string) error {
	content, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: open %s: %w", ErrBackupIntegrity, path, err)
	}

	defer func() { _ = content.Close() }()

	hash := sha256.New()
	size, err := io.Copy(hash, content)
	if err != nil {
		return fmt.Errorf("%w: hash %s: %w", ErrBackupIntegrity, path, err)
	}

	digest := hex.EncodeToString(hash.Sum(nil))
	if digest != wantDigest || size != wantSize {
		return fmt.Errorf("%w: %s has size %d and SHA256 %s", ErrBackupIntegrity, path, size, digest)
	}

	return nil
}

func describeBackupFile(name, source, relative, kind, path string) (BackupFile, error) {
	info, digest, err := fileDigest(path)
	if err != nil {
		return BackupFile{}, err
	}

	return BackupFile{
		Name: name, Source: source, Path: filepath.ToSlash(relative), Kind: kind,
		Mode: uint32(info.Mode().Perm()), Size: info.Size(), SHA256: digest,
	}, nil
}

func fileDigest(path string) (os.FileInfo, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}

	content, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}

	defer func() { _ = content.Close() }()

	hash := sha256.New()
	if _, err = io.Copy(hash, content); err != nil {
		return nil, "", err
	}

	return info, hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}

	defer func() { _ = input.Close() }()

	if err = os.MkdirAll(filepath.Dir(destination), backupDir); err != nil {
		return err
	}

	staged, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".restore-*")
	if err != nil {
		return err
	}

	stagedName := staged.Name()
	published := false

	defer func() {
		_ = staged.Close()
		if !published {
			_ = os.Remove(stagedName)
		}
	}()

	if err = staged.Chmod(mode.Perm()); err != nil {
		return err
	}

	if _, err = io.Copy(staged, input); err != nil {
		return err
	}

	if err = staged.Sync(); err != nil {
		return err
	}

	if err = staged.Close(); err != nil {
		return err
	}

	if err = os.Rename(stagedName, destination); err != nil {
		return err
	}

	published = true

	return syncDirectory(filepath.Dir(destination))
}

func (m BackupManager) prune() error {
	if m.Keep <= 0 {
		return nil
	}

	backups, err := m.List()
	if err != nil {
		return err
	}

	if len(backups) <= m.Keep {
		return nil
	}

	for _, backup := range backups[m.Keep:] {
		if err = os.RemoveAll(filepath.Join(m.Root, backup.ID)); err != nil {
			return fmt.Errorf("prune backup %s: %w", backup.ID, err)
		}
	}

	return nil
}

func validBackupID(id string) error {
	if id == "" || id != filepath.Base(id) || id == "." || strings.HasPrefix(id, ".") {
		return fmt.Errorf("%w: unusable backup id %q", ErrBackupInvalid, id)
	}

	return nil
}

func safeName(value string) string {
	if value == "" {
		return "unknown"
	}

	var out strings.Builder
	for _, one := range value {
		if one >= 'a' && one <= 'z' || one >= 'A' && one <= 'Z' || one >= '0' && one <= '9' ||
			one == '.' || one == '-' || one == '_' {
			out.WriteRune(one)
		} else {
			out.WriteByte('-')
		}
	}

	return strings.Trim(out.String(), "-")
}

func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)

	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
