package upgrade

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestServiceStagesVerifiesPreflightsAndRehearses(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	binary := []byte("candidate")
	sum := sha256.Sum256(binary)
	sums := []byte(hex.EncodeToString(sum[:]) + "  core\n")
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

	verifier, err := NewVerifier(nil, true)
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewService(ServiceOptions{
		Root:           root,
		CurrentVersion: "1.0.0",
		CurrentBinary:  filepath.Join(root, "current"),
		BuildProfile:   "full",
		Contract:       "1.0.0",
		Verifier:       verifier,
		Now:            func() time.Time { return now },
		Metadata: func(context.Context, string) (BinaryMetadata, error) {
			return BinaryMetadata{
				Version: "1.1.0", BuildProfile: "full", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
				Contract: "1.0.0", HostSchemaVersion: 10, HostSchemaCompat: 9,
			}, nil
		},
		Rehearse: func(context.Context, string) (RehearsalReport, error) {
			return RehearsalReport{Warnings: []string{"external PostgreSQL backup required"}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	journal, err := service.Stage(t.Context(), StageRequest{
		Name: "core", Version: "1.1.0", Binary: bytes.NewReader(binary), Sums: sums, RequestedBy: "user:ada",
	})
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != StateStaged {
		t.Fatalf("staged state is %s", journal.State)
	}

	journal, err = service.Verify(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != StateVerified || journal.Verified == nil || journal.Metadata == nil {
		t.Fatalf("verified journal is %#v", journal)
	}

	_, err = service.Preflight(t.Context(), PreflightRequest{Printing: []string{"MK4"}})
	if !errors.Is(err, ErrPreflight) {
		t.Fatalf("printing preflight error is %v", err)
	}

	journal, err = service.Preflight(t.Context(), PreflightRequest{
		Printing: []string{"MK4"}, ForcePrinting: true, DiskRequired: 100, DiskAvailable: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != StatePreflighted || !journal.ForcePrinting {
		t.Fatalf("preflight journal is %#v", journal)
	}

	now = now.Add(time.Second)
	journal, err = service.Rehearse(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != StateRehearsed || journal.Rehearsal == nil {
		t.Fatalf("rehearsal journal is %#v", journal)
	}

	journal, err = service.Schedule("when-idle", nil)
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != StateScheduled || journal.Schedule != "when-idle" {
		t.Fatalf("scheduled journal is %#v", journal)
	}
}

func TestServiceStagesVerifiedOfflineBundleWithExtensions(t *testing.T) {
	t.Parallel()

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	core := filepath.Join(root, "core")
	if err = os.WriteFile(core, []byte("candidate"), 0o700); err != nil {
		t.Fatal(err)
	}
	extension := moduleArchive(t, moduleYAML("1.0.0", ">=1.1.0", 1))
	var bundle bytes.Buffer
	if _, err = BuildBundle(&bundle, BundleBuild{
		CoreVersion: "1.1.0", BuildProfile: "normal", BuildVariant: "full",
		CorePath: core, PrivateKey: private,
		Extensions: []BundleBuildExtension{{Module: "example.test/spools", Path: extension}},
	}); err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier([]ed25519.PublicKey{public}, false)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(ServiceOptions{
		Root: root, CurrentVersion: "1.0.0",
		BuildProfile: "normal", BuildVariant: "full", Contract: "1.0.0", Verifier: verifier,
		Metadata: func(context.Context, string) (BinaryMetadata, error) {
			return BinaryMetadata{
				Version: "1.1.0", BuildProfile: "normal", BuildVariant: "full",
				GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Contract: "1.0.0",
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	journal, err := service.StageBundle(t.Context(), BundleStageRequest{
		Bundle: bytes.NewReader(bundle.Bytes()), RequestedBy: "user:ada",
	})
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != StateVerified || journal.BundleKeyID != KeyID(public) ||
		len(journal.Extensions) != 1 {
		t.Fatalf("journal is %#v", journal)
	}
	if _, err = os.Stat(journal.Extensions[0].StagedPath); err != nil {
		t.Fatal(err)
	}

	other, err := NewService(ServiceOptions{
		Root: t.TempDir(), CurrentVersion: "1.0.0",
		BuildProfile: "normal", BuildVariant: "full", Contract: "1.0.0", Verifier: verifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = other.StageBundle(t.Context(), BundleStageRequest{
		Bundle: bytes.NewReader(bundle.Bytes()), Version: "1.2.0",
	})
	if !errors.Is(err, ErrBundleCompatibility) {
		t.Fatalf("mismatched requested version error is %v", err)
	}

	journal.State = StateRehearsed
	journal.Revision++
	if err = service.store.Save(journal); err != nil {
		t.Fatal(err)
	}
	if _, err = service.AuthorizeApply(false, false); err != nil {
		t.Fatalf("extension apply authorization error is %v", err)
	}
	journal, err = service.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	journal.ExtensionReviews = []ExtensionReview{{
		Module: "example.test/spools", NewAsks: []string{"permission control"},
	}}
	if err = service.store.Save(journal); err != nil {
		t.Fatal(err)
	}
	if _, err = service.AuthorizeApply(false, false); !errors.Is(err, ErrPreflight) {
		t.Fatalf("a new extension ask was applied without approval: %v", err)
	}
	approved, err := service.AuthorizeApply(false, true)
	if err != nil || !approved.ExtensionsApproved {
		t.Fatalf("approved apply is %v, error %v", approved.ExtensionsApproved, err)
	}

	stagedRoot := journal.StagedRoot
	if _, err = service.Cancel(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(stagedRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancel left bundle staging behind: %v", err)
	}
}

func TestServiceRefusesWrongCandidateMetadata(t *testing.T) {
	t.Parallel()

	binary := []byte("candidate")
	sum := sha256.Sum256(binary)
	verifier, err := NewVerifier(nil, true)
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewService(ServiceOptions{
		Root: t.TempDir(), CurrentVersion: "1.0.0", Verifier: verifier,
		Metadata: func(context.Context, string) (BinaryMetadata, error) {
			return BinaryMetadata{Version: "1.1.0", GOOS: "wrong", GOARCH: runtime.GOARCH}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Stage(t.Context(), StageRequest{
		Name: "core", Version: "1.1.0", Binary: bytes.NewReader(binary),
		Sums: []byte(hex.EncodeToString(sum[:]) + "  core\n"),
	})
	if err != nil {
		t.Fatal(err)
	}

	journal, err := service.Verify(t.Context())
	if !errors.Is(err, ErrPlatformMismatch) {
		t.Fatalf("verify error is %v", err)
	}
	if journal.State != StateFailedBeforeSwap || journal.Failure != err.Error() {
		t.Fatalf("refused journal is %#v", journal)
	}

	_, err = service.Stage(t.Context(), StageRequest{
		Name: "core", Version: "1.1.0", Binary: bytes.NewReader(binary),
		Sums: []byte(hex.EncodeToString(sum[:]) + "  core\n"),
	})
	if err != nil {
		t.Fatalf("stage after a refused candidate: %v", err)
	}
}

func TestServiceEndsATransactionWhoseRehearsalFails(t *testing.T) {
	t.Parallel()

	binary := []byte("candidate")
	sum := sha256.Sum256(binary)
	verifier, err := NewVerifier(nil, true)
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewService(ServiceOptions{
		Root: t.TempDir(), CurrentVersion: "1.0.0", Verifier: verifier,
		Metadata: func(context.Context, string) (BinaryMetadata, error) {
			return BinaryMetadata{Version: "1.1.0", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}, nil
		},
		Rehearse: func(context.Context, string) (RehearsalReport, error) {
			return RehearsalReport{}, errors.New("migration 12 failed")
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	stage := func() error {
		_, staged := service.Stage(t.Context(), StageRequest{
			Name: "core", Version: "1.1.0", Binary: bytes.NewReader(binary),
			Sums: []byte(hex.EncodeToString(sum[:]) + "  core\n"),
		})

		return staged
	}
	if err = stage(); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Preflight(t.Context(), PreflightRequest{}); err != nil {
		t.Fatal(err)
	}

	journal, err := service.Rehearse(t.Context())
	if err == nil {
		t.Fatal("a failed rehearsal was accepted")
	}
	if journal.State != StateFailedBeforeSwap || journal.Failure != err.Error() {
		t.Fatalf("journal after a failed rehearsal is %#v", journal)
	}
	if err = stage(); err != nil {
		t.Fatalf("stage after a failed rehearsal: %v", err)
	}
}

// The wire contract is judged module by module in the upgrade impact, so a
// candidate whose contract moved still verifies.
func TestServiceVerifiesACandidateWhateverItsContract(t *testing.T) {
	t.Parallel()

	for _, candidate := range []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0"} {
		t.Run(candidate, func(t *testing.T) {
			t.Parallel()

			binary := []byte("candidate")
			sum := sha256.Sum256(binary)
			verifier, err := NewVerifier(nil, true)
			if err != nil {
				t.Fatal(err)
			}

			service, err := NewService(ServiceOptions{
				Root: t.TempDir(), CurrentVersion: "1.0.0", Contract: "1.1.0", Verifier: verifier,
				Metadata: func(context.Context, string) (BinaryMetadata, error) {
					return BinaryMetadata{
						Version: "1.1.0", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Contract: candidate,
					}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = service.Stage(t.Context(), StageRequest{
				Name: "core", Version: "1.1.0", Binary: bytes.NewReader(binary),
				Sums: []byte(hex.EncodeToString(sum[:]) + "  core\n"),
			})
			if err != nil {
				t.Fatal(err)
			}

			if _, err = service.Verify(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServiceReadsAVersionReportWithFieldsItDoesNotKnow(t *testing.T) {
	t.Parallel()

	metadata, err := decodeBinaryMetadata([]byte(`{"version":"1.2.0","contract":"1.1.0","added_later":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Version != "1.2.0" || metadata.Contract != "1.1.0" {
		t.Fatalf("metadata is %#v", metadata)
	}
}

// verifiedServiceForPreflight stages and verifies a candidate, which is the
// state Preflight needs. Every test gets its own root.
func verifiedServiceForPreflight(t *testing.T) *Service {
	t.Helper()

	binary := []byte("candidate")
	sum := sha256.Sum256(binary)
	verifier, err := NewVerifier(nil, true)
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewService(ServiceOptions{
		Root: t.TempDir(), CurrentVersion: "1.0.0", Verifier: verifier,
		Metadata: func(context.Context, string) (BinaryMetadata, error) {
			return BinaryMetadata{Version: "1.1.0", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = service.Stage(t.Context(), StageRequest{
		Name: "core", Version: "1.1.0", Binary: bytes.NewReader(binary),
		Sums: []byte(hex.EncodeToString(sum[:]) + "  core\n"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}

	return service
}

// preflightForConfigPath runs a preflight that knows where the configuration
// file lives and returns the report the preflight stored.
func preflightForConfigPath(t *testing.T, configPath string) PreflightReport {
	t.Helper()

	service := verifiedServiceForPreflight(t)
	journal, err := service.Preflight(t.Context(), PreflightRequest{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("the preflight refused %s: %v", configPath, err)
	}
	if journal.State != StatePreflighted || journal.Preflight == nil {
		t.Fatalf("preflight journal is %#v", journal)
	}

	return *journal.Preflight
}

// configDirectoryWarning is the sentence the preflight stores for a
// configuration file whose directory refuses a write.
func configDirectoryWarning(configPath string) string {
	return "the configuration directory " + filepath.Dir(configPath) +
		" is not writable, so a rollback cannot rewrite " + configPath +
		", and the server will not start when the upgrade changed it"
}

// assertOnlyWarning fails unless the report holds exactly this one warning.
func assertOnlyWarning(t *testing.T, report PreflightReport, want string) {
	t.Helper()

	if len(report.Warnings) != 1 || report.Warnings[0] != want {
		t.Fatalf("warnings are %q; want %q", report.Warnings, want)
	}
}

// TestPreflightReportsAConfigurationDirectoryARollbackCannotWrite is the
// shipped unit: /etc/prusa-connect is read-only, so a rollback leaves an
// unchanged file alone and cannot rewrite one that changed. The warning names
// the directory and never blocks.
func TestPreflightReportsAConfigurationDirectoryARollbackCannotWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// A directory that does not exist stands for one the preflight could not
	// create; the subtest covers one that exists and refuses a write.
	absent := filepath.Join(root, "missing", "config.d", "config.yaml")
	assertOnlyWarning(t, preflightForConfigPath(t, absent), configDirectoryWarning(absent))

	// A directory that takes a write produces no such warning.
	for _, warning := range preflightForConfigPath(t, filepath.Join(root, "config.yaml")).Warnings {
		if strings.Contains(warning, "the configuration directory") {
			t.Fatalf("a writable configuration directory produced %q", warning)
		}
	}

	t.Run("a read-only directory", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("a directory mode does not stop this process here")
		}

		directory := filepath.Join(t.TempDir(), "config.d")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
		if err := os.Chmod(directory, 0o500); err != nil {
			t.Fatal(err)
		}

		configPath := filepath.Join(directory, "config.yaml")
		assertOnlyWarning(t, preflightForConfigPath(t, configPath), configDirectoryWarning(configPath))
	})
}
