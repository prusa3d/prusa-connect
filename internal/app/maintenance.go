package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/hostschema"
	"github.com/prusa3d/prusa-connect/internal/app/moduledeps"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/app/resourcehost"
	"github.com/prusa3d/prusa-connect/internal/betaportal"
	"github.com/prusa3d/prusa-connect/internal/kernel/buildprofile"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/stream"
	"github.com/prusa3d/prusa-connect/internal/upgrade"
	"github.com/prusa3d/prusa-connect/sdk/event"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// ErrUpgradeApply asks the command launcher to swap a prepared executable
// after every listener, database and supervised resource has stopped.
var ErrUpgradeApply = errors.New("app: a prepared upgrade is ready to apply")
var ErrBackupRestore = errors.New("app: a backup is ready to restore")

type maintenance struct {
	service    *upgrade.Service
	store      upgrade.Store
	backups    upgrade.BackupManager
	releases   upgrade.ReleaseSource
	core       config.Core
	configPath string
	version    string
	fleet      *devices
	modules    runningModules
	// catalog is the module index a core upgrade's plan takes releases from,
	// and nil for a server that reads none.
	catalog        moduleCatalog
	requestApply   func()
	requestRestore func(string) error
	// requestRestart ends the run with ErrRestart.
	requestRestart func()
	logger         *slog.Logger
	external       bool
	reason         string
	hub            *stream.Hub
	origin         event.Origin
	// off is set when the holder was built with core.upgrade.enabled off. The
	// holder then has no service, store, backups or release source.
	off bool
}

const (
	candidateDiskCopies = 2
	protectedDiskCopies = 3
)

func newMaintenance(
	ctx context.Context,
	core config.Core,
	configPath string,
	currentVersion string,
	fleet *devices,
	modules runningModules,
	hub *stream.Hub,
	serverID string,
	entitlement *betaportal.Store,
	requestApply func(),
	requestRestore func(string) error,
) (*maintenance, error) {
	// The keys are checked with upgrades off too, so a key the file gets wrong
	// stops the start now and not on the day upgrades are switched on.
	keys := make([]upgrade.TrustedKey, 0, len(core.Upgrade.TrustedKeys))
	for _, key := range core.Upgrade.TrustedKeys {
		keys = append(keys, upgrade.TrustedKey{ID: key.ID, Key: key.Key})
	}

	verifier, err := upgrade.BuildVerifier(keys, core.Upgrade.AllowUnsigned)
	if err != nil {
		return nil, err
	}

	binary, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find the running binary: %w", err)
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return nil, fmt.Errorf("resolve the running binary: %w", err)
	}

	external, reason := externallyManaged(binary)
	holder := &maintenance{
		core: core, configPath: configPath, version: currentVersion, fleet: fleet,
		modules:      modules,
		requestApply: requestApply, external: external, reason: reason,
		requestRestore: requestRestore,
		hub:            hub,
		off:            !core.Upgrade.On(),
	}
	// With core.upgrade.enabled off the holder serves the version and the
	// restart and nothing else: no service, no release source, no backups and
	// no scheduler. What the start still does below the upgrade folder is in
	// reportUpgradesOff and RecoverUpgrade.
	if holder.off {
		return holder, nil
	}

	root := filepath.Dir(upgradeJournalPath(core))
	holder.origin, err = event.NewOrigin(event.Kind("server"), serverID)
	if err != nil {
		return nil, fmt.Errorf("build upgrade event origin: %w", err)
	}

	var releases upgrade.ReleaseSource = upgrade.MachineClient{
		BaseURL:   core.Upgrade.Server,
		Authorize: entitlement.Authorize,
		Rejected:  entitlement.Rejected,
		AssetName: upgrade.ReleaseBinaryName(buildprofile.Name, buildprofile.Variant),
	}
	// release_repository is the legacy, explicitly configured GitHub source.
	// An absent key uses the machine update API at core.upgrade.server.
	if core.Upgrade.ReleaseRepository != "" {
		releases = upgrade.ReleaseClient{
			Repository: core.Upgrade.ReleaseRepository,
			AssetName:  upgrade.ReleaseBinaryName(buildprofile.Name, buildprofile.Variant),
		}
	}

	// The catalogue is read once per pairing for a few minutes:
	// the update server limits how often one server may read it.
	releases = &upgrade.CachedSource{Source: releases, Scope: pairingScope(entitlement)}

	holder.store = upgrade.NewStore(upgradeJournalPath(core))
	holder.backups = backupManager(core, "")
	holder.releases = releases

	service, err := upgrade.NewService(upgrade.ServiceOptions{
		Root: root, CurrentVersion: currentVersion, CurrentBinary: binary,
		BuildProfile: buildprofile.Name, BuildVariant: buildprofile.Variant,
		Contract: wirerpc.Contract.String(), Managed: !external, ManagedReason: reason,
		Verifier: verifier,
		PlatformVerify: func(ctx context.Context, candidate string) error {
			if runtime.GOOS != "darwin" {
				return nil
			}
			output, signatureErr := exec.CommandContext(
				ctx, "codesign", "--verify", "--deep", "--strict", candidate,
			).CombinedOutput()
			if signatureErr != nil {
				return fmt.Errorf(
					"verify the macOS Developer ID signature: %w: %s",
					signatureErr, strings.TrimSpace(string(output)),
				)
			}

			return nil
		},
		Rehearse: func(ctx context.Context, candidate string) (upgrade.RehearsalReport, error) {
			// Signature verification fixes candidate before this execution.
			command := exec.CommandContext( //nolint:gosec // verified private candidate path
				ctx, candidate, upgradeCommandArgs(configPath)...,
			)
			output, commandErr := command.Output()
			if commandErr != nil {
				return upgrade.RehearsalReport{}, commandErr
			}

			return decodeRehearsal(output)
		},
	})
	if err != nil {
		return nil, err
	}

	holder.service = service
	go holder.runScheduler(ctx)

	return holder, nil
}

// upgradesOff reports a holder built with core.upgrade.enabled off, which
// serves the version and the restart and nothing else. The upgrade and
// backup operations and the restart read this one field.
func (m *maintenance) upgradesOff() bool {
	return m.off
}

// pairingScope names the Beta Portal pairing the catalogue is read with, so a
// new or cleared pairing reads the catalogue again.
func pairingScope(entitlement *betaportal.Store) func() string {
	return func() string {
		if entitlement == nil {
			return ""
		}
		status := entitlement.Status()
		identity, _ := json.Marshal(status.Identity) //nolint:errchkjson // a plain struct always encodes

		return string(status.State) + "\x00" + string(identity)
	}
}

func (m *maintenance) ServerVersion(context.Context) (httpapi.ServerVersion, error) {
	schema := hostschema.Schema()
	schemaVersion, err := schema.Version(1)
	if err != nil {
		return httpapi.ServerVersion{}, err
	}

	// A configuration that cannot be read asks for the acknowledgement: apply
	// reads it again and names the error there.
	unprotected, err := m.postgresOutsideBackups()
	if err != nil {
		unprotected = true
	}

	return httpapi.ServerVersion{
		Version: m.version, BuildProfile: buildprofile.Name, BuildVariant: buildprofile.Variant,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Contract: wirerpc.Contract.String(), Capabilities: resourcehost.Capabilities(),
		HostSchemaVersion: schemaVersion, HostSchemaCompat: schema.Compat,
		ManagedExternally: m.external, ManagedReason: m.reason,
		PostgresAckRequired: unprotected,
	}, nil
}

func (m *maintenance) UpgradeStatus(context.Context) (upgrade.Journal, error) {
	return m.service.Status()
}

func (m *maintenance) UpgradeHistory(context.Context) ([]upgrade.Journal, error) {
	return m.store.History()
}

func (m *maintenance) Releases(ctx context.Context) ([]upgrade.Release, error) {
	return m.releases.List(ctx, m.version)
}

func (m *maintenance) Stage(
	ctx context.Context,
	input httpapi.MaintenanceStage,
) (upgrade.Journal, error) {
	if input.Version == "" {
		return upgrade.Journal{}, fmt.Errorf("%w: a release version is required", upgrade.ErrBundle)
	}

	if len(input.Binary) == 0 {
		releases, err := m.releases.List(ctx, m.version)
		if err != nil {
			return upgrade.Journal{}, err
		}

		index := slices.IndexFunc(releases, func(release upgrade.Release) bool {
			return release.Version == input.Version
		})
		if index < 0 {
			return upgrade.Journal{}, fmt.Errorf("release %s is not available for this platform", input.Version)
		}

		return m.stageDownload(ctx, releases[index], input.Principal)
	}

	return m.stageAndVerify(ctx, upgrade.StageRequest{
		Name: input.Name, Version: input.Version, Binary: bytes.NewReader(input.Binary),
		Sums: input.Sums, Signature: input.Signature, RequestedBy: input.Principal,
	})
}

// StageBundle stages a signed offline bundle as the request body arrives. The
// bundle is verified while it is read, so nothing holds the whole file.
func (m *maintenance) StageBundle(ctx context.Context, body io.Reader, principal string) (upgrade.Journal, error) {
	staged, err := m.service.StageBundle(ctx, upgrade.BundleStageRequest{Bundle: body, RequestedBy: principal})
	m.publish(staged)

	return staged, err
}

// stageDownload streams a catalogue release to a private folder beside the
// journal and stages it from there, so the binary is never held in memory.
func (m *maintenance) stageDownload(
	ctx context.Context, release upgrade.Release, principal string,
) (upgrade.Journal, error) {
	parent := filepath.Join(filepath.Dir(upgradeJournalPath(m.core)), "downloads")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return upgrade.Journal{}, fmt.Errorf("create the download folder: %w", err)
	}
	dir, err := os.MkdirTemp(parent, "release-")
	if err != nil {
		return upgrade.Journal{}, fmt.Errorf("create the download folder: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	downloaded, err := upgrade.DownloadRelease(ctx, m.releases, release, dir, 0)
	if err != nil {
		return upgrade.Journal{}, err
	}
	binary, err := os.Open(downloaded.Binary)
	if err != nil {
		return upgrade.Journal{}, fmt.Errorf("open the downloaded release: %w", err)
	}
	defer func() { _ = binary.Close() }()

	return m.stageAndVerify(ctx, upgrade.StageRequest{
		Name: release.Binary.Name, Version: release.Version, Binary: binary,
		Sums: downloaded.Sums, Signature: downloaded.Signature, RequestedBy: principal, Online: true,
	})
}

func (m *maintenance) stageAndVerify(ctx context.Context, request upgrade.StageRequest) (upgrade.Journal, error) {
	staged, err := m.service.Stage(ctx, request)
	if err != nil {
		return upgrade.Journal{}, err
	}
	m.publish(staged)

	verified, err := m.service.Verify(ctx)
	m.publish(verified)

	return verified, err
}

func (m *maintenance) Prepare(ctx context.Context) (upgrade.Journal, error) {
	status, err := m.service.Status()
	if err != nil {
		return upgrade.Journal{}, err
	}

	// A verify or a rehearsal stopped with its request, or by a restart,
	// leaves the journal where that step began. Prepare runs it from there.
	if status.State == upgrade.StateStaged {
		status, err = m.service.Verify(ctx)
		m.publish(status)
		if err != nil {
			return status, err
		}
	}
	if status.State == upgrade.StatePreflighted {
		status, err = m.service.Rehearse(ctx)
		m.publish(status)

		return status, err
	}

	planned, err := m.planModules(ctx, status)
	if err != nil {
		return upgrade.Journal{}, fmt.Errorf("%w: plan the modules of the upgrade: %w", upgrade.ErrPreflight, err)
	}

	// A bundle whose releases do not fit together, or with what this server
	// keeps, is refused whole: installing part of a signed set proves nothing
	// about the set (ADR-20261027-e6k rule 3a).
	if planned.refused != nil && len(status.Extensions) > 0 {
		return upgrade.Journal{}, fmt.Errorf("%w: the bundled extensions: %w", upgrade.ErrPreflight, planned.refused)
	}

	doc, err := resource.Load(resource.Paths{File: m.configPath, Overlay: m.core.Overlay})
	if err != nil {
		return upgrade.Journal{}, err
	}

	installed := moduledeps.InstalledOf(doc.Instances)

	extensions, err := m.stagePlan(ctx, status, planned, installed)
	if err != nil {
		return upgrade.Journal{}, fmt.Errorf("%w: %w", upgrade.ErrPreflight, err)
	}

	withPlan := status
	withPlan.Extensions = extensions

	reviews, err := reviewBundledExtensions(m.core, m.configPath, withPlan)
	if err != nil {
		return upgrade.Journal{}, fmt.Errorf("%w: %w", upgrade.ErrPreflight, err)
	}
	if len(extensions) > 0 {
		if err = m.moduleDatabasesDumped(); err != nil {
			return upgrade.Journal{}, err
		}

		// Apply writes the lock after the releases are in, and a lock it
		// cannot write fails the upgrade into a rollback. The same check an
		// install makes refuses it here, before anything is staged to run.
		if err = lockWritable(m.core); err != nil {
			return upgrade.Journal{}, fmt.Errorf("%w: %w", upgrade.ErrPreflight, err)
		}
	}

	active := m.activeDevices()
	impacts := append(m.moduleImpacts(status.Metadata, planned), stagedImpacts(extensions, installed)...)
	required, available, err := m.diskBudget(withPlan)
	if err != nil {
		return upgrade.Journal{}, err
	}
	preflighted, err := m.service.Preflight(ctx, upgrade.PreflightRequest{
		Printing: active, PreparationOnly: true, ModuleImpacts: impacts,
		TrialRequired: m.trialExtensions(withPlan, impacts), ExtensionReviews: reviews,
		DiskRequired: required, DiskAvailable: available, Extensions: extensions,
		ConfigPath: m.configPath,
	})
	if err != nil {
		return upgrade.Journal{}, err
	}
	m.publish(preflighted)

	rehearsed, err := m.service.Rehearse(ctx)
	m.publish(rehearsed)

	return rehearsed, err
}

func (m *maintenance) Apply(
	ctx context.Context,
	input httpapi.MaintenanceApply,
) (upgrade.Journal, error) {
	if m.external {
		return upgrade.Journal{}, fmt.Errorf("%w: the executable is managed externally: %s",
			upgrade.ErrPreflight, m.reason)
	}

	active := m.activeDevices()
	if len(active) > 0 && !input.ForcePrinting {
		return upgrade.Journal{}, fmt.Errorf("%w: printers are active: %s",
			upgrade.ErrPreflight, strings.Join(active, ", "))
	}
	if input.ForcePrinting && input.Schedule != "when-idle" {
		if transfers := m.activeTransfers(); len(transfers) > 0 {
			return upgrade.Journal{}, fmt.Errorf("%w: file transfers are active: %s",
				upgrade.ErrPreflight, strings.Join(transfers, ", "))
		}
	}
	if err := m.requirePostgresAck(input.PostgresAck); err != nil {
		return upgrade.Journal{}, err
	}

	journal, err := m.service.AuthorizeApply(input.ForcePrinting, input.ApproveExtensions)
	if err != nil {
		return upgrade.Journal{}, err
	}
	if input.ForcePrinting && (input.Schedule == "" || input.Schedule == "now") {
		if err = m.pausePrinting(ctx, input.Principal); err != nil {
			return upgrade.Journal{}, err
		}
	}
	m.publish(journal)

	if input.Schedule != "" && input.Schedule != "now" {
		scheduled, scheduleErr := m.service.Schedule(input.Schedule, input.ScheduledFor)
		m.publish(scheduled)

		return scheduled, scheduleErr
	}

	if m.requestApply != nil {
		m.publish(journal)
		m.requestApply()
	}

	return journal, nil
}

func (m *maintenance) requirePostgresAck(acknowledged bool) error {
	if acknowledged {
		return nil
	}
	unprotected, err := m.postgresOutsideBackups()
	if err != nil {
		return err
	}
	if unprotected {
		return upgrade.ErrPostgresUnconfirmed
	}

	return nil
}

// postgresOutsideBackups reports whether a rollback would leave a PostgreSQL
// database, of the host or of a module, for the operator to restore.
func (m *maintenance) postgresOutsideBackups() (bool, error) {
	if m.core.Upgrade.PostgresBackup == string(upgrade.PostgresDump) {
		return false, nil
	}
	if m.core.Persistence.Type == config.EnginePostgres {
		return true, nil
	}
	_, postgres, err := moduleDatabaseSources(
		m.configPath, m.core, upgrade.PostgresMode(m.core.Upgrade.PostgresBackup), true,
	)
	if err != nil {
		return false, err
	}

	return len(postgres) > 0, nil
}

func (m *maintenance) publish(journal upgrade.Journal) {
	if m.hub == nil || journal.ID == "" {
		return
	}

	data, err := json.Marshal(journal)
	if err != nil {
		return
	}
	_, _ = m.hub.Publish(stream.Publication{
		Origin: m.origin, Type: event.TypeServerUpgrade, Class: event.ClassState, Data: data,
	})
}

func (m *maintenance) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.applyScheduled(ctx) {
				return
			}
		}
	}
}

func (m *maintenance) applyScheduled(ctx context.Context) bool {
	journal, err := m.service.Status()
	if err != nil || journal.State != upgrade.StateScheduled || m.requestApply == nil {
		return false
	}

	active := len(m.activeDevices()) > 0
	dueWhenIdle := journal.Schedule == "when-idle" && !active
	dueAt := journal.Schedule == "at" && journal.ScheduledFor != nil &&
		!journal.ScheduledFor.After(time.Now()) &&
		(!active || journal.ForcePrinting && len(m.activeTransfers()) == 0)
	if !dueWhenIdle && !dueAt {
		return false
	}
	if journal.ForcePrinting {
		if err = m.pausePrinting(ctx, journal.RequestedBy); err != nil {
			return false
		}
	}
	m.requestApply()

	return true
}

// Cancel ends the open upgrade. One whose staged release is left on disk is
// still cancelled, so the release left is logged and the cancel stands.
func (m *maintenance) Cancel(ctx context.Context) error {
	cancelled, err := m.service.Cancel()
	if cancelled && err != nil {
		m.logger.WarnContext(ctx, "the staged release of the cancelled upgrade is left on disk", "error", err.Error())

		return nil
	}

	return err
}

func (m *maintenance) Backups(context.Context) ([]upgrade.BackupManifest, error) {
	return m.backups.List()
}

func (m *maintenance) CreateBackup(
	ctx context.Context,
	toVersion string,
	postgresAck bool,
) (upgrade.BackupManifest, error) {
	if m.core.Persistence.Type == config.EnginePostgres &&
		m.core.Upgrade.PostgresBackup != string(upgrade.PostgresDump) && !postgresAck {
		return upgrade.BackupManifest{}, upgrade.ErrPostgresUnconfirmed
	}
	plan, err := backupPlan(
		m.configPath, m.core, toVersion,
		upgrade.PostgresMode(m.core.Upgrade.PostgresBackup), postgresAck,
	)
	if err != nil {
		return upgrade.BackupManifest{}, err
	}

	return m.backups.Create(ctx, plan)
}

func (m *maintenance) RestoreBackup(_ context.Context, id string) error {
	if m.core.Persistence.Type == config.EnginePostgres &&
		m.core.Upgrade.PostgresBackup != string(upgrade.PostgresDump) {
		return fmt.Errorf("%w: restore the external PostgreSQL backup before restoring local files",
			upgrade.ErrPostgresUnconfirmed)
	}

	backups, err := m.backups.List()
	if err != nil {
		return err
	}
	index := slices.IndexFunc(backups, func(backup upgrade.BackupManifest) bool { return backup.ID == id })
	if index < 0 {
		return os.ErrNotExist
	}
	backup := backups[index]
	if backup.FromVersion != "" && backup.FromVersion != m.version {
		journal, loadErr := m.store.Load()
		if loadErr != nil || journal.BackupID != id || journal.PreviousBinary == "" {
			return fmt.Errorf("%w: "+
				"backup %s belongs to version %s and its executable is unavailable",
				upgrade.ErrPreflight, id, backup.FromVersion,
			)
		}
	}
	if m.requestRestore == nil {
		return errors.New("backup restore is not available")
	}

	return m.requestRestore(id)
}

func (m *maintenance) DeleteBackup(_ context.Context, id string) error {
	status, err := m.service.Status()
	if err == nil && status.BackupID == id {
		switch status.State {
		case upgrade.StateCommitted, upgrade.StateRolledBack, upgrade.StateFailedBeforeSwap:
		case upgrade.StateStaged, upgrade.StateVerified, upgrade.StatePreflighted,
			upgrade.StateRehearsed, upgrade.StateScheduled, upgrade.StateDraining,
			upgrade.StateBackedUp, upgrade.StateSwapped, upgrade.StateTrial,
			upgrade.StateRollingBack:
			return fmt.Errorf("backup %s belongs to the current upgrade", id)
		}
	}

	return m.backups.Delete(id)
}

func (m *maintenance) activeDevices() []string {
	if m.fleet == nil {
		return nil
	}

	m.fleet.mu.Lock()
	entries := make(map[string]*device, len(m.fleet.entries))
	maps.Copy(entries, m.fleet.entries)
	m.fleet.mu.Unlock()

	var active []string
	for id, entry := range entries {
		var state string
		_ = json.Unmarshal(entry.snapshot()["/state"], &state)
		if state == statePrinting || state == stateBusy || state == statePaused || state == stateAttention {
			active = append(active, id)
		}
	}
	for device := range m.fleet.transfers.byDevice() {
		if !slices.Contains(active, device) {
			active = append(active, device)
		}
	}
	slices.Sort(active)

	return active
}

func (m *maintenance) activeTransfers() []string {
	if m.fleet == nil {
		return nil
	}

	var active []string
	for device := range m.fleet.transfers.byDevice() {
		active = append(active, device)
	}
	slices.Sort(active)

	return active
}

func (m *maintenance) pausePrinting(ctx context.Context, principal string) error {
	if m.fleet == nil {
		return nil
	}

	m.fleet.mu.Lock()
	entries := make(map[string]*device, len(m.fleet.entries))
	maps.Copy(entries, m.fleet.entries)
	m.fleet.mu.Unlock()

	for id, entry := range entries {
		var state string
		_ = json.Unmarshal(entry.snapshot()["/state"], &state)
		if state != statePrinting {
			continue
		}
		if _, err := m.fleet.Command(ctx, principal, id, httpapi.PrinterCommand{
			Verb: "core.command.pause_print",
		}); err != nil {
			return fmt.Errorf("pause printer %s before upgrade: %w", id, err)
		}
	}

	return nil
}
