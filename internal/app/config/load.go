package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// The top-level blocks of the operator's file this build decodes. `core` is
// this package's, `resources` is internal/app/resource's, and `persistence` is
// the engine every part of the core shares. Load reads `persistence` from the
// top level because cmd/detect-build-tags reads the same key to decide whether
// the Postgres driver is linked in.
const (
	blockCore        = "core"
	blockPersistence = "persistence"
	blockResources   = "resources"
)

// ReadBlocks is that set in the order a refusal lists it.
//
//nolint:gochecknoglobals // the inventory of blocks this build reads, never written to.
var ReadBlocks = []string{blockCore, blockPersistence, blockResources}

// Load reads the core block out of the operator's file and fills in every key
// it does not set.
//
// A key this build does not decode is refused with the line of the file it is
// written on. A warning would not be enough: an operator who configures a
// subsystem this binary does not carry would learn at the next outage that it
// never ran. The resources block is the one exception, because its schema is
// the declared instance's and internal/app/resource refuses an unknown key in
// it.
//
// A missing file is a default configuration, because a first run has no file.
//
// The file the first-run wizard wrote, SetupFile(path), is read under the
// operator's file. Each file is checked on its own first, so a refusal names
// the file and the line the key is written on.
func Load(path string) (Core, error) {
	content, err := read(path)
	if err != nil {
		return Core{}, err
	}

	if _, err = parse(content); err != nil {
		return Core{}, fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}

	setupPath := ""
	if path != "" {
		setupPath = SetupFile(path)
	}

	setup, err := read(setupPath)
	if err != nil {
		return Core{}, err
	}

	if err = checkSetup(setup); err != nil {
		return Core{}, fmt.Errorf("%w: %s: %w", ErrInvalid, setupPath, err)
	}

	whole, err := merged(setup, content)
	if err != nil {
		return Core{}, fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}

	core, err := parse(whole)
	if err != nil {
		return Core{}, fmt.Errorf("%w: %s and %s: %w", ErrInvalid, path, setupPath, err)
	}

	return core.WithDefaults(path), nil
}

// LoadWith is Load with the wizard's file replaced by setup, for the first run
// to check what it is about to write against the same rules a start applies.
// The operator's file is read from path as Load reads it.
func LoadWith(path string, setup []byte) (Core, error) {
	content, err := read(path)
	if err != nil {
		return Core{}, err
	}

	if err = checkSetup(setup); err != nil {
		return Core{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	whole, err := merged(setup, content)
	if err != nil {
		return Core{}, fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}

	core, err := parse(whole)
	if err != nil {
		return Core{}, fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}

	return core.WithDefaults(path), nil
}

func read(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrInvalid, path, err)
	}

	return content, nil
}

// parse walks the top level of the document and decodes the two blocks this
// package reads, refusing everything else by name and by line.
//
// It walks the parsed nodes because a yaml.Node keeps the line each key was
// written on, and a refusal with a line number sends an operator straight to
// the block.
//
// A document whose top level is not a mapping names no key. It reads as the
// empty configuration, as a missing file does.
func parse(content []byte) (Core, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return Core{}, nil
	}

	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return Core{}, fmt.Errorf("parse the top level: %w", err)
	}

	root := rootMapping(document)
	if root == nil {
		return Core{}, nil
	}

	var (
		core        Core
		persistence Persistence
	)

	for index := 0; index+1 < len(root.Content); index += nodesPerEntry {
		name, block := root.Content[index], root.Content[index+1]

		var err error

		switch name.Value {
		case blockCore:
			err = decodeStrictly(block, blockCore, &core)
		case blockPersistence:
			err = decodeStrictly(block, blockPersistence, &persistence)
		case blockResources:
			// The declared instances. Their schema is each instance type's, and
			// internal/app/resource refuses an unknown key inside one.
		default:
			err = refuseBlock(name)
		}

		if err != nil {
			return Core{}, err
		}
	}

	core.Persistence = persistence

	return core, nil
}

// refuseBlock reports a top-level block this build does not read, with the line
// it is written on.
//
// A retired block is refused with the key that replaced it, as a retired key
// inside a block is. A process that ignored the block would run without the
// part the operator asked for and say nothing.
func refuseBlock(name *yaml.Node) error {
	if moved, retired := retiredKeys[name.Value]; retired {
		return fmt.Errorf("line %d: %s: this build no longer reads that block. %s",
			name.Line, name.Value, moved)
	}

	return fmt.Errorf("line %d: %s: this build does not read that block. It reads %s",
		name.Line, name.Value, strings.Join(ReadBlocks, ", "))
}

// withDefaults fills the database block's keys. The engine selector and the
// pool bounds are filled whichever engine was named, because database/sql
// reads a zero bound as no bound.
func (p Persistence) withDefaults(defaults Persistence) Persistence {
	if p.Type == "" {
		p.Type = defaults.Type
	}

	if p.MaxOpenConns == 0 {
		p.MaxOpenConns = defaults.MaxOpenConns
	}

	if p.MaxIdleConns == 0 {
		p.MaxIdleConns = defaults.MaxIdleConns
	}

	if p.ConnMaxLifetime == 0 {
		p.ConnMaxLifetime = defaults.ConnMaxLifetime
	}

	if p.Postgres.StatementTimeout == 0 {
		p.Postgres.StatementTimeout = defaults.Postgres.StatementTimeout
	}

	// The page cache is SQLite's alone, so Default() leaves it at zero and a
	// Postgres block keeps the zero. Validate can then tell a written value
	// from a default.
	if p.Type == EngineSQLite && p.SQLite.PageCacheBytes == 0 {
		p.SQLite.PageCacheBytes = DefaultSQLitePageCacheBytes
	}

	return p
}

// withDefaults fills the app's keys. Enabled is filled too, so a reader of
// the filled value never meets a nil.
func (f Frontend) withDefaults(defaults Frontend) Frontend {
	f.Enabled = orDefault(f.Enabled, defaults.Enabled)

	if f.Title == "" {
		f.Title = defaults.Title
	}

	if f.DefaultLanguage == "" {
		f.DefaultLanguage = defaults.DefaultLanguage
	}

	if f.Theme == "" {
		f.Theme = defaults.Theme
	}

	return f
}

// withCacheDefaults fills the bounds of the caches that differ between the
// normal and the printer build.
func (c Core) withCacheDefaults(defaults Core) Core {
	if c.PreviewCacheBytes == 0 {
		c.PreviewCacheBytes = defaults.PreviewCacheBytes
	}

	if c.DeviceListingCacheBytes == 0 {
		c.DeviceListingCacheBytes = defaults.DeviceListingCacheBytes
	}

	if c.DeviceThumbnailDiskBytes == 0 {
		c.DeviceThumbnailDiskBytes = defaults.DeviceThumbnailDiskBytes
	}

	c.DeviceThumbnailDisk = orDefault(c.DeviceThumbnailDisk, defaults.DeviceThumbnailDisk)
	c.DeviceThumbnailPrefetch = orDefault(c.DeviceThumbnailPrefetch, defaults.DeviceThumbnailPrefetch)

	return c
}

// orDefault keeps what the operator wrote and fills what they left out. A
// switch that defaults to on cannot be a bare bool: false and "not written"
// are the same zero value, and only one of them means off.
func orDefault(written, fallback *bool) *bool {
	if written == nil {
		return fallback
	}

	return written
}

// rootMapping is the document's top-level mapping, or nil when it has none.
func rootMapping(document yaml.Node) *yaml.Node {
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return nil
	}

	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}

	return root
}

// WithDefaults fills every key the file left unset. It is separate from parse
// so that a caller can tell an operator's explicit value from a default. The
// flag overrides need that.
//
// It is exported and idempotent because a flag can turn on a block that the
// file left off. Some defaults apply only to a block in use, so the caller
// runs it again after the flags are applied.
func (c Core) WithDefaults(path string) Core {
	defaults := Default()

	if c.API.Addr == "" {
		c.API.Addr = defaults.API.Addr
	}

	if c.API.RequestTimeout == 0 {
		c.API.RequestTimeout = defaults.API.RequestTimeout
	}

	if c.API.StreamTimeout == 0 {
		c.API.StreamTimeout = defaults.API.StreamTimeout
	}

	if c.Log.Level == "" {
		c.Log.Level = defaults.Log.Level
	}

	if c.Log.Format == "" {
		c.Log.Format = defaults.Log.Format
	}

	c.Frontend = c.Frontend.withDefaults(defaults.Frontend)
	c.Discovery = c.Discovery.withDefaults(defaults.Discovery)
	c.Marketplace = c.Marketplace.withDefaults(defaults.Marketplace)

	if c.Principal == "" {
		c.Principal = defaults.Principal
	}

	if c.Site == "" {
		c.Site = defaults.Site
	}

	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = defaults.ShutdownTimeout
	}

	c.ModulesSwitchOff = c.ModulesSwitchOff.Over(defaults.ModulesSwitchOff)

	if c.Events.Heartbeat == 0 {
		c.Events.Heartbeat = defaults.Events.Heartbeat
	}

	if c.Events.Retry == 0 {
		c.Events.Retry = defaults.Events.Retry
	}

	if c.Events.MaxStreamsPerPrincipal == 0 {
		c.Events.MaxStreamsPerPrincipal = defaults.Events.MaxStreamsPerPrincipal
	}

	if c.DeviceStateBudget == 0 {
		c.DeviceStateBudget = defaults.DeviceStateBudget
	}

	if c.ModuleSettingsBytes == 0 {
		c.ModuleSettingsBytes = defaults.ModuleSettingsBytes
	}

	c = c.withCacheDefaults(defaults)

	c.Auth = c.Auth.withDefaults(defaults.Auth)

	if c.Overlay == "" {
		c.Overlay = DefaultOverlay(path)
	}

	if c.Modules == "" {
		c.Modules = DefaultModules(path)
	}

	if c.Authz.Database == "" {
		c.Authz.Database = DefaultGraph(path)
	}

	c.Persistence = c.Persistence.withDefaults(defaults.Persistence)
	c.Upgrade = c.Upgrade.withDefaults(defaults.Upgrade)

	// Only with a local account store. Validate refuses the key on a
	// deployment that has none.
	if c.Auth.Local && c.Auth.Accounts == "" {
		c.Auth.Accounts = DefaultAccounts(path)
	}

	return c
}

func (u Upgrade) withDefaults(defaults Upgrade) Upgrade {
	u.Enabled = orDefault(u.Enabled, defaults.Enabled)

	if u.Server == "" {
		u.Server = defaults.Server
	}
	if u.BackupRetention == 0 {
		u.BackupRetention = defaults.BackupRetention
	}
	if u.PostgresBackup == "" {
		u.PostgresBackup = defaults.PostgresBackup
	}
	if u.TrialTimeout == 0 {
		u.TrialTimeout = defaults.TrialTimeout
	}

	return u
}

// withDefaults fills the password keys a file left out.
func (a Auth) withDefaults(defaults Auth) Auth {
	if a.PasswordMemoryBytes == 0 {
		a.PasswordMemoryBytes = defaults.PasswordMemoryBytes
	}

	if a.PasswordPasses == 0 {
		a.PasswordPasses = defaults.PasswordPasses
	}

	if a.PasswordHashesAtOnce == 0 {
		a.PasswordHashesAtOnce = defaults.PasswordHashesAtOnce
	}

	return a
}
