package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/internal/kernel/atomicfile"
)

const (
	DefaultMaxBinaryBytes int64 = 512 << 20
	executablePerm              = 0o700
)

var (
	ErrUpgradeBusy      = errors.New("upgrade: another transaction is active")
	ErrPreflight        = errors.New("upgrade: preflight failed")
	ErrPlatformMismatch = errors.New("upgrade: binary platform does not match this server")
)

type BinaryMetadata struct {
	Version           string   `json:"version"                doc:"The semantic version reported by the candidate."`
	BuildProfile      string   `json:"build_profile"          doc:"The deployment profile linked into the candidate."`
	BuildVariant      string   `json:"build_variant"          doc:"The feature variant linked into the candidate."`
	GOOS              string   `json:"goos"                   doc:"The operating system targeted by the candidate."`
	GOARCH            string   `json:"goarch"                 doc:"The processor architecture targeted by the candidate."`
	Contract          string   `json:"contract"               doc:"The module wire contract served by the candidate."`
	Capabilities      []string `json:"capabilities,omitempty" doc:"The host capabilities served to extensions by the candidate."`
	HostSchemaVersion int      `json:"host_schema_version"    doc:"The newest host schema understood by the candidate."`
	HostSchemaCompat  int      `json:"host_schema_compat"     doc:"The oldest host schema compatible with the candidate."`
}

type PreflightReport struct {
	CheckedAt     time.Time      `json:"checked_at"               doc:"The instant the preflight completed."`
	Printing      []string       `json:"printing,omitempty"       doc:"The identifiers of devices with active work."`
	ModuleImpacts []ModuleImpact `json:"module_impacts,omitempty" doc:"The predicted compatibility effect on installed modules."`
	Warnings      []string       `json:"warnings,omitempty"       doc:"Non-blocking conditions found by preflight."`
	DiskRequired  int64          `json:"disk_required"            doc:"The estimated free bytes required for update and rollback."`
	DiskAvailable int64          `json:"disk_available"           doc:"The free bytes available in the update filesystem."`
	Writable      bool           `json:"writable"                 doc:"Whether the executable directory passed its write probe."`
	Managed       bool           `json:"managed"                  doc:"Whether this process may replace its executable."`
	ManagedReason string         `json:"managed_reason,omitempty" doc:"Why another platform owns executable replacement."`
}

// The outcomes a ModuleImpact predicts.
const (
	// ImpactOK is a module that keeps its release and runs.
	ImpactOK = "ok"
	// ImpactUpdated is a module the upgrade's module plan moves to another
	// release, which runs on the candidate core.
	ImpactUpdated = "updated"
	// ImpactStaged is a release the upgrade installs that the server does not
	// have yet.
	ImpactStaged = "staged"
	// ImpactDisabled is a module that does not run on the candidate core.
	ImpactDisabled = "disabled"
	// ImpactBlocked is a module the server needs to start that would not run,
	// which refuses the upgrade.
	ImpactBlocked = "blocked"
)

type ModuleImpact struct {
	Module  string `json:"module"  doc:"The installed module identifier."`
	Outcome string `json:"outcome" doc:"The predicted module outcome after the update."`
	// NeededToStart is the resource's own required flag: the server does not
	// serve while this instance is not serving. The modules that one module
	// requires are a separate thing.
	NeededToStart bool   `json:"needed_to_start"  doc:"Whether the server needs this module serving to start."`
	Reason        string `json:"reason,omitempty" doc:"The compatibility reason for this outcome."`
}

type RehearsalReport struct {
	CheckedAt  time.Time `json:"checked_at"         doc:"The instant the migration rehearsal completed."`
	DurationMS int64     `json:"duration_ms"        doc:"The rehearsal wall time in milliseconds."`
	Warnings   []string  `json:"warnings,omitempty" doc:"Non-blocking conditions found during rehearsal."`
}

type StageRequest struct {
	Name        string
	Version     string
	Binary      io.Reader
	Sums        []byte
	Signature   []byte
	RequestedBy string
	// Online marks a candidate the server downloaded itself. allow_unsigned
	// never applies to it.
	Online bool
}

type BundleStageRequest struct {
	Bundle      io.Reader
	Version     string
	RequestedBy string
}

type PreflightRequest struct {
	Printing         []string
	ForcePrinting    bool
	PreparationOnly  bool
	ModuleImpacts    []ModuleImpact
	TrialRequired    []string
	ExtensionReviews []ExtensionReview
	DiskRequired     int64
	DiskAvailable    int64
	ConfigCheck      func(context.Context, string) error
	ConfigPath       string
	// Extensions is every module release apply installs, in the order it
	// installs them: the signed bundle's, and the ones the upgrade's module
	// plan takes from the index. Nil keeps the bundle's own. It must hold
	// every extension the bundle staged, unchanged.
	Extensions []StagedExtension
}

type ServiceOptions struct {
	Root           string
	CurrentVersion string
	CurrentBinary  string
	BuildProfile   string
	BuildVariant   string
	Contract       string
	Managed        bool
	ManagedReason  string
	MaxBinaryBytes int64
	Verifier       Verifier
	Now            func() time.Time
	Metadata       func(context.Context, string) (BinaryMetadata, error)
	PlatformVerify func(context.Context, string) error
	Rehearse       func(context.Context, string) (RehearsalReport, error)
}

type Service struct {
	mu      sync.Mutex
	options ServiceOptions
	store   Store
}

func NewService(options ServiceOptions) (*Service, error) {
	if options.Root == "" || options.CurrentVersion == "" {
		return nil, errors.New("upgrade: service needs a root and current version")
	}
	if options.MaxBinaryBytes == 0 {
		options.MaxBinaryBytes = DefaultMaxBinaryBytes
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Metadata == nil {
		options.Metadata = readBinaryMetadata
	}

	return &Service{
		options: options,
		store:   NewStore(filepath.Join(options.Root, "journal.json")),
	}, nil
}

func (s *Service) Status() (Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store.Load()
}

func (s *Service) Stage(ctx context.Context, request StageRequest) (Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if request.Binary == nil || request.Name == "" || request.Version == "" {
		return Journal{}, errors.New("upgrade: stage needs a binary name, version and content")
	}

	finished, err := s.finishedStage()
	if err != nil {
		return Journal{}, err
	}

	if err = PermitVersion(s.options.CurrentVersion, request.Version); err != nil {
		return Journal{}, err
	}

	id := s.options.Now().UTC().Format("20060102T150405.000000000Z") + "-" + safeName(request.Version)
	stageDir := filepath.Join(s.options.Root, "staged", id)
	if err = os.MkdirAll(stageDir, privateDir); err != nil {
		return Journal{}, fmt.Errorf("upgrade: create stage: %w", err)
	}

	clean := true
	defer func() {
		if clean {
			_ = os.RemoveAll(stageDir)
		}
	}()

	binaryPath := filepath.Join(stageDir, filepath.Base(request.Name))
	if err = streamFile(binaryPath, request.Binary, s.options.MaxBinaryBytes, executablePerm); err != nil {
		return Journal{}, err
	}
	if err = atomicfile.Write(filepath.Join(stageDir, "SHA256SUMS"), request.Sums, privateFile); err != nil {
		return Journal{}, fmt.Errorf("upgrade: stage SHA256SUMS: %w", err)
	}
	if err = atomicfile.Write(filepath.Join(stageDir, "SHA256SUMS.sig"), request.Signature, privateFile); err != nil {
		return Journal{}, fmt.Errorf("upgrade: stage SHA256SUMS.sig: %w", err)
	}

	journal, err := NewJournal(id, s.options.CurrentVersion, request.Version, binaryPath, s.options.Now())
	if err != nil {
		return Journal{}, err
	}
	journal.RequestedBy = request.RequestedBy
	journal.Online = request.Online
	journal.CurrentBinary = s.options.CurrentBinary

	if err = s.store.Save(journal); err != nil {
		return Journal{}, err
	}

	clean = false
	s.removeFinished(finished)

	_ = ctx

	return journal, nil
}

// StageBundle verifies and stages a signed offline bundle as one immutable
// candidate. Extensions remain staged until the transaction engine can install
// them under the same rollback boundary as the core.
func (s *Service) StageBundle(ctx context.Context, request BundleStageRequest) (Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if request.Bundle == nil {
		return Journal{}, errors.New("upgrade: stage bundle needs content")
	}
	finished, err := s.finishedStage()
	if err != nil {
		return Journal{}, err
	}

	id := s.options.Now().UTC().Format("20060102T150405.000000000Z") + "-offline"
	stageDir := filepath.Join(s.options.Root, "staged", id)
	contractMajor, _, _ := strings.Cut(s.options.Contract, ".")
	major, _ := strconv.Atoi(contractMajor)
	staged, err := s.options.Verifier.VerifyAndStageBundle(request.Bundle, stageDir, BundleVerifyOptions{
		BuildProfile: s.options.BuildProfile, BuildVariant: s.options.BuildVariant, ContractMajor: major,
	})
	if err != nil {
		return Journal{}, err
	}
	clean := true
	defer func() {
		if clean {
			_ = os.RemoveAll(stageDir)
		}
	}()

	if request.Version != "" && staged.Manifest.CoreVersion != request.Version {
		return Journal{}, fmt.Errorf("%w: bundle version is %s, request expected %s",
			ErrBundleCompatibility, staged.Manifest.CoreVersion, request.Version)
	}
	if err = PermitVersion(s.options.CurrentVersion, staged.Manifest.CoreVersion); err != nil {
		return Journal{}, err
	}
	if s.options.PlatformVerify != nil {
		if err = s.options.PlatformVerify(ctx, staged.CorePath); err != nil {
			return Journal{}, err
		}
	}
	metadata, err := s.options.Metadata(ctx, staged.CorePath)
	if err != nil {
		return Journal{}, err
	}
	journal, err := NewJournal(
		id, s.options.CurrentVersion, staged.Manifest.CoreVersion, staged.CorePath, s.options.Now(),
	)
	if err != nil {
		return Journal{}, err
	}
	if err = s.validateMetadata(journal, metadata); err != nil {
		return Journal{}, err
	}
	journal.State = StateVerified
	journal.Revision++
	journal.RequestedBy = request.RequestedBy
	journal.CurrentBinary = s.options.CurrentBinary
	journal.StagedRoot = stageDir
	journal.BundleKeyID = staged.KeyID
	journal.Extensions = staged.Extensions
	journal.Metadata = &metadata
	journal.Verified = &Verified{
		Name: staged.Manifest.Core.Path, SHA256: staged.Manifest.Core.SHA256,
		KeyID: staged.KeyID, Size: staged.Manifest.Core.Size,
	}
	if err = s.store.Save(journal); err != nil {
		return Journal{}, err
	}
	clean = false
	s.removeFinished(finished)

	return journal, nil
}

func (s *Service) Verify(ctx context.Context) (Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	journal, err := s.store.Load()
	if err != nil {
		return Journal{}, err
	}
	if journal.State != StateStaged {
		return Journal{}, fmt.Errorf("%w: verify needs staged, found %s", ErrJournal, journal.State)
	}

	verified, metadata, err := s.verifyStaged(ctx, journal)
	if err != nil {
		return s.failBeforeSwap(ctx, journal, err)
	}

	return s.store.Transition(journal, StateVerified, s.options.Now(), func(next *Journal) {
		next.Verified = &verified
		next.Metadata = &metadata
	})
}

func (s *Service) verifyStaged(ctx context.Context, journal Journal) (Verified, BinaryMetadata, error) {
	stageDir := filepath.Dir(journal.StagedBinary)
	sums, err := os.ReadFile(filepath.Join(stageDir, "SHA256SUMS"))
	if err != nil {
		return Verified{}, BinaryMetadata{}, fmt.Errorf("upgrade: read staged SHA256SUMS: %w", err)
	}
	signature, err := os.ReadFile(filepath.Join(stageDir, "SHA256SUMS.sig"))
	if err != nil {
		return Verified{}, BinaryMetadata{}, fmt.Errorf("upgrade: read staged SHA256SUMS.sig: %w", err)
	}

	verifier := s.options.Verifier
	if journal.Online {
		verifier.allowUnsigned = false
	}
	verified, err := verifier.Verify(
		journal.StagedBinary,
		filepath.Base(journal.StagedBinary),
		sums,
		signature,
	)
	if err != nil {
		return Verified{}, BinaryMetadata{}, err
	}
	if s.options.PlatformVerify != nil {
		if err = s.options.PlatformVerify(ctx, journal.StagedBinary); err != nil {
			return Verified{}, BinaryMetadata{}, err
		}
	}

	metadata, err := s.options.Metadata(ctx, journal.StagedBinary)
	if err != nil {
		return Verified{}, BinaryMetadata{}, err
	}
	if err = s.validateMetadata(journal, metadata); err != nil {
		return Verified{}, BinaryMetadata{}, err
	}

	return verified, metadata, nil
}

// failBeforeSwap ends a transaction whose verify or rehearsal failed and
// records why. Neither step can run again on that journal. The Updates page
// shows the reason, and a new stage is accepted. A step stopped because its
// request ended has not failed. Its journal stays as it was, and the next
// prepare runs the step again.
func (s *Service) failBeforeSwap(ctx context.Context, journal Journal, cause error) (Journal, error) {
	if ctx.Err() != nil {
		return journal, cause
	}

	failed, err := s.store.Transition(journal, StateFailedBeforeSwap, s.options.Now(), func(next *Journal) {
		next.Failure = cause.Error()
	})

	return failed, errors.Join(cause, err)
}

// finishedStage refuses a new stage while a transaction is open and returns
// the finished one, or nil. Its staged release stays until the new candidate
// has verified, so a bad upload leaves the last state as it was.
func (s *Service) finishedStage() (*Journal, error) {
	current, err := s.store.Load()
	if errors.Is(err, ErrNoJournal) {
		return nil, nil //nolint:nilnil // no transaction has finished
	}
	if err != nil {
		return nil, err
	}
	if !terminalState(current.State) {
		return nil, fmt.Errorf("%w: transaction %s is %s", ErrUpgradeBusy, current.ID, current.State)
	}

	return &current, nil
}

// removeFinished removes the staged release of the finished transaction the
// new one replaced, as a cancel does. A release it cannot remove stays on
// disk, and the new candidate stands all the same.
func (s *Service) removeFinished(finished *Journal) {
	if finished != nil {
		_ = removeStaged(s.options.Root, *finished)
	}
}

func (s *Service) Preflight(ctx context.Context, request PreflightRequest) (Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	journal, err := s.store.Load()
	if err != nil {
		return Journal{}, err
	}
	if journal.State != StateVerified {
		return Journal{}, fmt.Errorf("%w: preflight needs verified, found %s", ErrJournal, journal.State)
	}

	report := PreflightReport{
		CheckedAt: s.options.Now().UTC(), Printing: request.Printing,
		ModuleImpacts: request.ModuleImpacts, DiskRequired: request.DiskRequired,
		DiskAvailable: request.DiskAvailable, Managed: s.options.Managed,
		ManagedReason: s.options.ManagedReason,
	}

	if journal.CurrentBinary != "" {
		report.Writable = writableBeside(journal.CurrentBinary)
	}

	// A rollback restores the configuration file into the directory it lives
	// in. The shipped unit mounts that directory read-only, so an unwritable
	// one is a warning and not a blocker: the restore now writes only a file
	// that changed, and blocking would refuse every upgrade on that unit.
	if request.ConfigPath != "" && !writableBeside(request.ConfigPath) {
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"the configuration directory %s is not writable, so a rollback cannot rewrite %s, and the server will not start when the upgrade changed it",
			filepath.Dir(request.ConfigPath),
			request.ConfigPath,
		))
	}

	blockers := s.preflightBlockers(ctx, request, report)
	if len(blockers) > 0 {
		return Journal{}, fmt.Errorf("%w: %s", ErrPreflight, strings.Join(blockers, "; "))
	}

	if request.Extensions != nil {
		if err = keepsSignedExtensions(journal.Extensions, request.Extensions); err != nil {
			return Journal{}, err
		}
	}

	return s.store.Transition(journal, StatePreflighted, s.options.Now(), func(next *Journal) {
		if request.Extensions != nil {
			next.Extensions = slices.Clone(request.Extensions)
		}

		next.ForcePrinting = request.ForcePrinting
		next.TrialRequired = slices.Clone(request.TrialRequired)
		next.ExtensionReviews = slices.Clone(request.ExtensionReviews)
		next.Preflight = &report
	})
}

// AuthorizeApply records the operator's decision about active printers after
// preparation and before either an immediate or scheduled apply.
func (s *Service) AuthorizeApply(forcePrinting, approveExtensions bool) (Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	journal, err := s.store.Load()
	if err != nil {
		return Journal{}, err
	}
	if journal.State != StateRehearsed {
		return Journal{}, fmt.Errorf("%w: apply authorization needs rehearsed, found %s", ErrJournal, journal.State)
	}
	for _, review := range journal.ExtensionReviews {
		if review.NeedsApproval() && !approveExtensions {
			return Journal{}, fmt.Errorf("%w: bundled extension %s %s; approve the bundled extensions to apply",
				ErrPreflight, review.Module, review.approvalReason())
		}
	}
	journal.ForcePrinting = forcePrinting
	journal.ExtensionsApproved = approveExtensions
	journal.Revision++
	journal.UpdatedAt = s.options.Now().UTC()
	if err = s.store.Save(journal); err != nil {
		return Journal{}, err
	}

	return journal, nil
}

func (s *Service) Rehearse(ctx context.Context) (Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	journal, err := s.store.Load()
	if err != nil {
		return Journal{}, err
	}
	if journal.State != StatePreflighted {
		return Journal{}, fmt.Errorf("%w: rehearsal needs preflighted, found %s", ErrJournal, journal.State)
	}
	if s.options.Rehearse == nil {
		return Journal{}, errors.New("upgrade: no migration rehearsal is configured")
	}

	started := s.options.Now()
	report, err := s.options.Rehearse(ctx, journal.StagedBinary)
	if err != nil {
		return s.failBeforeSwap(ctx, journal, fmt.Errorf("upgrade: migration rehearsal: %w", err))
	}
	if report.CheckedAt.IsZero() {
		report.CheckedAt = s.options.Now().UTC()
	}
	if report.DurationMS == 0 {
		report.DurationMS = max(1, s.options.Now().Sub(started).Milliseconds())
	}

	return s.store.Transition(journal, StateRehearsed, s.options.Now(), func(next *Journal) {
		next.Rehearsal = &report
	})
}

func (s *Service) Schedule(schedule string, at *time.Time) (Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	journal, err := s.store.Load()
	if err != nil {
		return Journal{}, err
	}
	if journal.State != StateRehearsed {
		return Journal{}, fmt.Errorf("%w: schedule needs rehearsed, found %s", ErrJournal, journal.State)
	}
	if schedule != "when-idle" && schedule != "at" {
		return Journal{}, fmt.Errorf("upgrade: schedule is %q, expected when-idle or at", schedule)
	}
	if schedule == "at" && at == nil {
		return Journal{}, errors.New("upgrade: an at schedule needs a time")
	}

	return s.store.Transition(journal, StateScheduled, s.options.Now(), func(next *Journal) {
		next.Schedule = schedule
		next.ScheduledFor = at
	})
}

// Cancel removes the open transaction and its staged release, and reports
// whether the transaction is gone. A transaction it removed with its release
// left on disk comes with an error that wraps ErrStagedLeft.
func (s *Service) Cancel() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	journal, err := s.store.Load()
	if err != nil {
		return false, err
	}
	if journal.Interrupted() {
		return false, fmt.Errorf("%w: transaction %s is already %s", ErrUpgradeBusy, journal.ID, journal.State)
	}

	return cancelTransaction(s.options.Root, s.store, journal)
}

func (s *Service) validateMetadata(journal Journal, metadata BinaryMetadata) error {
	if metadata.Version != journal.ToVersion {
		return fmt.Errorf("%w: staged metadata says version %s, expected %s",
			ErrPlatformMismatch, metadata.Version, journal.ToVersion)
	}
	if metadata.GOOS != runtime.GOOS || metadata.GOARCH != runtime.GOARCH {
		return fmt.Errorf("%w: staged binary is %s/%s, server is %s/%s",
			ErrPlatformMismatch, metadata.GOOS, metadata.GOARCH, runtime.GOOS, runtime.GOARCH)
	}
	if s.options.BuildProfile != "" && metadata.BuildProfile != s.options.BuildProfile {
		return fmt.Errorf("%w: staged profile is %s, server profile is %s",
			ErrPlatformMismatch, metadata.BuildProfile, s.options.BuildProfile)
	}
	if s.options.BuildVariant != "" && metadata.BuildVariant != s.options.BuildVariant {
		return fmt.Errorf("%w: staged build variant is %s, expected %s",
			ErrPlatformMismatch, metadata.BuildVariant, s.options.BuildVariant)
	}

	return nil
}

func readBinaryMetadata(ctx context.Context, path string) (BinaryMetadata, error) {
	command := exec.CommandContext(ctx, path, "--version", "--json")
	output, err := command.Output()
	if err != nil {
		return BinaryMetadata{}, fmt.Errorf("upgrade: read staged binary metadata: %w", err)
	}

	return decodeBinaryMetadata(output)
}

// decodeBinaryMetadata ignores a field it does not know. A later release adds
// fields to its version report, and this server must still read it.
func decodeBinaryMetadata(output []byte) (BinaryMetadata, error) {
	var metadata BinaryMetadata
	if err := json.Unmarshal(output, &metadata); err != nil {
		return BinaryMetadata{}, fmt.Errorf("upgrade: decode staged binary metadata: %w", err)
	}

	return metadata, nil
}

func streamFile(path string, source io.Reader, limit int64, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("upgrade: create staged binary: %w", err)
	}

	defer func() { _ = file.Close() }()

	written, err := io.Copy(file, io.LimitReader(source, limit+1))
	if err != nil {
		return fmt.Errorf("upgrade: write staged binary: %w", err)
	}
	if written > limit {
		return fmt.Errorf("upgrade: staged binary exceeds %d bytes", limit)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("upgrade: sync staged binary: %w", err)
	}

	return nil
}

func writableBeside(path string) bool {
	directory := filepath.Dir(path)
	probe, err := os.CreateTemp(directory, ".upgrade-write-probe-*")
	if err != nil {
		return false
	}

	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)

	return true
}

func terminalState(state State) bool {
	return state == StateCommitted || state == StateRolledBack || state == StateFailedBeforeSwap
}

func stagingRoot(journal Journal) string {
	if journal.StagedRoot != "" {
		return journal.StagedRoot
	}

	return filepath.Dir(journal.StagedBinary)
}

// keepsSignedExtensions refuses a module plan that drops or changes an
// extension the signed bundle staged. The plan only orders them and adds
// releases beside them.
func keepsSignedExtensions(signed, planned []StagedExtension) error {
	for _, one := range signed {
		if !slices.ContainsFunc(planned, func(other StagedExtension) bool {
			return other.StagedPath == one.StagedPath && other.SHA256 == one.SHA256 && other.Size == one.Size &&
				other.Module == one.Module && other.Version == one.Version
		}) {
			return fmt.Errorf("%w: the module plan leaves out the signed extension %s@%s",
				ErrPreflight, one.Module, one.Version)
		}
	}

	return nil
}

// impactServes reports whether a module still serves after the upgrade: it
// runs as it is, or the upgrade's module plan updates it, or installs it.
func impactServes(outcome string) bool {
	return outcome == ImpactOK || outcome == ImpactUpdated || outcome == ImpactStaged
}

// preflightBlockers is every reason the upgrade may not go ahead.
func (s *Service) preflightBlockers(ctx context.Context, request PreflightRequest, report PreflightReport) []string {
	var blockers []string
	if len(request.Printing) > 0 && !request.ForcePrinting && !request.PreparationOnly {
		blockers = append(blockers, "printers are active: "+strings.Join(request.Printing, ", "))
	}
	if request.DiskRequired > 0 && request.DiskAvailable < request.DiskRequired {
		blockers = append(blockers, fmt.Sprintf("disk has %d bytes available and %d are required",
			request.DiskAvailable, request.DiskRequired))
	}
	if s.options.Managed && !report.Writable {
		blockers = append(blockers, "the current binary directory is not writable")
	}
	for _, impact := range request.ModuleImpacts {
		if impact.NeededToStart && !impactServes(impact.Outcome) {
			blockers = append(blockers, "module "+impact.Module+", which the server needs to start: "+impact.Reason)
		}
	}
	if request.ConfigCheck != nil {
		if err := request.ConfigCheck(ctx, request.ConfigPath); err != nil {
			blockers = append(blockers, "candidate configuration: "+err.Error())
		}
	}

	return blockers
}
