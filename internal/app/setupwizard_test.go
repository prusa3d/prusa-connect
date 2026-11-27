package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/platform/auth/oidc"
	"github.com/prusa3d/prusa-connect/internal/platform/observability/prom"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// wizardInstall is a fresh install whose first run knows its operator file and
// counts the restarts it asks for.
func wizardInstall(t *testing.T, dir string) (firstRunSetup, *int) {
	t.Helper()

	install := startInstall(t, dir, filepath.Join(dir, "host.db"), nil)
	restarts := 0
	install.first.configPath = filepath.Join(dir, "config.yaml")
	install.first.restart = func() { restarts++ }
	// The server under test already listens where the defaults put it.
	install.first.listening = config.DefaultAPIAddr

	return install, &restarts
}

// wizardDecision is what the page sends on Confirm.
func wizardDecision(dir string) httpapi.SetupDecision {
	return httpapi.SetupDecision{
		Code: testSetupCode,
		Mode: resource.SetupRequired,
		Account: &httpapi.NewAccount{
			Username: "Owner@Example.com",
			Password: "correct horse battery",
		},
		Admission: resource.AdmissionOpen,
		Configuration: map[string]any{
			"core.site":                             "workshop",
			"core.frontend.theme":                   "dark",
			"core.api.request_timeout":              "45s",
			"core.discovery.enabled":                false,
			"core.events.max_streams_per_principal": float64(32),
		},
		StorageRoot: filepath.Join(dir, "uploads"),
		History:     &httpapi.SetupHistory{Retention: "168h"},
		MQTT: &httpapi.SetupMQTT{
			URL:      "tcp://broker.local:1883",
			Username: "prusa",
			Password: "broker secret",
		},
	}
}

// TestTheContractListsWhatTheWizardMaySet holds rules 10 to 14 of ADR-20261015-ejz:
// every key it lists is one the reference lists, the keys that decide where
// the server writes are absent, and a key the operator wrote is locked with
// its value.
func TestTheContractListsWhatTheWizardMaySet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("core:\n  site: shed\n"), 0o600))

	contract, err := setupContract(path)
	require.NoError(t, err)
	require.True(t, contract.ConfigFile.Exists)
	require.False(t, contract.SetupFile.Exists)
	require.False(t, contract.Storage.Declared)
	wantRoot, err := filepath.Abs("storage")
	require.NoError(t, err)
	require.Equal(t, wantRoot, contract.Storage.Root)
	require.NotEqual(t, filepath.Join(filepath.Dir(dir), "storage"), contract.Storage.Root,
		"storage is in the start folder, not beside the configuration folder")

	byPath := map[string]httpapi.SetupKey{}
	for _, key := range contract.Keys {
		_, documented := config.Keys[key.Path]
		require.True(t, documented, "%s is not a key the reference lists", key.Path)
		require.NotContains(t, key.Path, config.Placeholder)

		byPath[key.Path] = key
	}

	for hidden := range wizardHidden {
		require.NotContains(t, byPath, hidden)
	}

	site := byPath["core.site"]
	require.True(t, site.Locked)
	require.Equal(t, "shed", site.Value)
	require.Equal(t, groupServer, site.Group)

	addr := byPath["core.api.addr"]
	require.False(t, addr.Locked)
	require.Equal(t, config.LevelBasic, addr.Level)
	require.Equal(t, "api", addr.Group)
	require.Equal(t, ":8080", addr.Default)
	require.Equal(t, "config/network/#key-core-api-addr", addr.Reference,
		"the wizard links a key to its row on the page of its part")
	require.Equal(t, "certificates/", byPath["core.api.tls.certificate"].Guide)

	require.Equal(t, "6h", byPath["core.api.stream_timeout"].Default)
	require.Equal(t, []string{"system", "light", "dark"}, byPath["core.frontend.theme"].Choices)
	require.Equal(t, groupDatabase, byPath["persistence.max_open_conns"].Group)
}

// TestTheContractSaysWhetherTheOperatorEncryptedThePrinters is CL-205: an
// operator's file that declares the printers' listener with a certificate
// locks the wizard's switch, and the switch has to show it on.
func TestTheContractSaysWhetherTheOperatorEncryptedThePrinters(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		file      string
		printers  bool
		encrypted bool
	}{
		"nothing declared": {file: "core:\n  site: shed\n", printers: true},
		"a plain listener": {
			file: "core:\n  connectors:\n    xbuddy:\n      type: xbuddy\n      config:\n" +
				"        listener:\n          addr: \":8443\"\n",
		},
		"a listener with a certificate": {
			file: "core:\n  connectors:\n    xbuddy:\n      type: xbuddy\n      config:\n" +
				"        listener:\n          addr: \":8443\"\n          tls:\n" +
				"            certificate: /etc/pcl/printer.crt\n            key: /etc/pcl/printer.key\n",
			encrypted: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.file), 0o600))

			contract, err := setupContract(path)
			require.NoError(t, err)
			require.Equal(t, tc.printers, contract.Printers)
			require.Equal(t, tc.encrypted, contract.PrintersTLS)
		})
	}
}

// TestAConfirmedWizardIsWrittenDownAndNotRecorded holds rules 17 to 20: the
// two files are written, the password is not in either, nothing is recorded
// in this process, and the restart is asked for.
func TestAConfirmedWizardIsWrittenDownAndNotRecorded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	install, restarts := wizardInstall(t, dir)

	result, err := install.first.Record(t.Context(), wizardDecision(dir))
	require.NoError(t, err)
	require.True(t, result.Restarting)
	require.False(t, result.SignedIn)
	require.Equal(t, 1, *restarts)
	require.True(t, install.first.Pending(), "the decision was recorded before the restart")

	setup, err := os.ReadFile(config.SetupFile(install.first.configPath))
	require.NoError(t, err)
	require.Contains(t, string(setup), "first-run wizard")
	require.Contains(t, string(setup), "site: workshop")

	pending, err := os.ReadFile(firstRunPath(install.first.configPath))
	require.NoError(t, err)
	require.NotContains(t, string(pending), "correct horse battery", "the password was written in clear")
	require.Contains(t, string(pending), "$argon2id$")

	info, err := os.Stat(firstRunPath(install.first.configPath))
	require.NoError(t, err)

	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(privateFileMode), info.Mode().Perm(), "the decision is readable by others")
	}

	core, err := config.Load(install.first.configPath)
	require.NoError(t, err)
	require.Equal(t, "workshop", core.Site)
	require.Equal(t, "dark", core.Frontend.Theme)
	require.Equal(t, 32, core.Events.MaxStreamsPerPrincipal)
	require.False(t, core.Discovery.On())

	_, err = install.first.Record(t.Context(), wizardDecision(dir))
	require.Error(t, err, "the code was used again after the wizard was confirmed")
}

// TestAnAnswerTheServerWouldRefuseWritesNothing holds rule 17: the answers are
// validated as a start validates them, and a refusal names the key.
func TestAnAnswerTheServerWouldRefuseWritesNothing(t *testing.T) {
	t.Parallel()

	for name, answers := range map[string]map[string]any{
		"a value outside the closed set": {"core.log.level": "loud"},
		"a key the wizard does not set":  {"core.overlay": "/tmp/elsewhere.json"},
		"a value of the wrong kind":      {"core.discovery.enabled": "yes"},
		"a duration that is not one":     {"core.api.request_timeout": "soon"},
		"a value validation refuses":     {"core.api.allowed_origins": []any{"*"}},
		"a key this build does not read": {"core.mqtt.enabled": true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			install, restarts := wizardInstall(t, dir)

			decision := wizardDecision(dir)
			decision.Configuration = answers

			_, err := install.first.Record(t.Context(), decision)
			require.Error(t, err)
			require.Zero(t, *restarts)

			_, statErr := os.Stat(config.SetupFile(install.first.configPath))
			require.ErrorIs(t, statErr, os.ErrNotExist, "a refused answer wrote the wizard's file")

			_, statErr = os.Stat(firstRunPath(install.first.configPath))
			require.ErrorIs(t, statErr, os.ErrNotExist, "a refused answer wrote the decision")
		})
	}
}

// oidcDecision is the wizard confirmed for people who sign in at an issuer:
// no account, and the administrator named by their subject there.
func oidcDecision(dir, issuer string) httpapi.SetupDecision {
	decision := wizardDecision(dir)
	decision.Account = nil
	decision.Configuration = map[string]any{
		"core.auth.issuer":        issuer,
		"core.auth.client_id":     "prusa-connect",
		"core.auth.client_secret": "GOCSPX-secret",
		"core.auth.redirect_uri":  "http://localhost:8080/api/v1/auth/callback",
		"core.principal":          "4f1c2a9e-7d3b-4c55-9a10-2b8e6f0d1c3a",
	}

	return decision
}

// TestAWizardForAnIssuerNamesTheAdministratorBySubject: the issuer is read
// before the restart, the file names it, and the first grant goes to the
// subject the person typed, because nobody has a password here.
func TestAWizardForAnIssuerNamesTheAdministratorBySubject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	install, restarts := wizardInstall(t, dir)

	var asked string

	install.first.discover = func(_ context.Context, issuer string) error {
		asked = issuer

		return nil
	}

	result, err := install.first.Record(t.Context(), oidcDecision(dir, "https://sso.example.com/realms/home"))
	require.NoError(t, err)
	require.True(t, result.Restarting)
	require.Equal(t, 1, *restarts)
	require.Equal(t, "https://sso.example.com/realms/home", asked)

	pending, err := os.ReadFile(firstRunPath(install.first.configPath))
	require.NoError(t, err)
	require.NotContains(t, string(pending), "administrator", "an issuer deployment wrote a password account")

	core, err := config.Load(install.first.configPath)
	require.NoError(t, err)
	require.True(t, core.Auth.Enabled())
	require.Equal(t, "prusa-connect", core.Auth.ClientID)
	require.Equal(t, "4f1c2a9e-7d3b-4c55-9a10-2b8e6f0d1c3a", core.Principal)

	// The secret reaches the client the next start builds, which is what
	// authenticates the code exchange.
	require.Equal(t, "GOCSPX-secret", oidcConfig(core.Auth).ClientSecret)

	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(config.SetupFile(install.first.configPath))
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(privateFileMode), info.Mode().Perm(),
			"the wizard's file holds a client secret and others can read it")
	}
}

// TestAWizardTakesASubjectTheGraphCannotNameAsItIs: authentik's default
// subject, Auth0's and Dex's do not fit the id grammar. The file keeps the
// subject the person typed, and the first grant goes to the id it maps to
// (ADR-20261023-wgv), which is the id that person signs in as.
func TestAWizardTakesASubjectTheGraphCannotNameAsItIs(t *testing.T) {
	t.Parallel()

	for _, subject := range []string{
		strings.Repeat("0123456789abcdef", 4),
		"auth0|5f7c8ec7c33c6c004bbafe82",
		"CiQwOGE4Njg0Yi1kYjg4LTRiNzMtOTBhOS0zY2QxNjYxZjU0NjYSBWxvY2Fs",
	} {
		t.Run(subject, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			install, restarts := wizardInstall(t, dir)
			install.first.discover = func(context.Context, string) error { return nil }

			decision := oidcDecision(dir, "https://sso.example.com/")
			decision.Configuration["core.principal"] = subject

			_, err := install.first.Record(t.Context(), decision)
			require.NoError(t, err)
			require.Equal(t, 1, *restarts)

			core, err := config.Load(install.first.configPath)
			require.NoError(t, err)
			require.Equal(t, subject, core.Principal)
			require.Equal(t, oidc.PrincipalID("https://sso.example.com", subject), principalID(core))
			require.True(t, strings.HasPrefix(principalID(core), oidc.DerivedPrefix))
		})
	}
}

// TestTheContractMarksTheClientSecretSecret: the page never shows a secret
// back, and the wizard's file goes private when one is answered.
func TestTheContractMarksTheClientSecretSecret(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	install, _ := wizardInstall(t, dir)

	contract, err := install.first.contract()
	require.NoError(t, err)

	index := slices.IndexFunc(contract.Keys, func(key httpapi.SetupKey) bool {
		return key.Path == "core.auth.client_secret"
	})
	require.NotEqual(t, -1, index, "the wizard cannot set the client secret")
	require.True(t, contract.Keys[index].Secret)

	require.Equal(t, os.FileMode(setupFileMode), setupModeFor(map[string]any{"core.site": "x"}, contract))
	require.Equal(t, os.FileMode(privateFileMode),
		setupModeFor(map[string]any{"core.auth.client_secret": "s"}, contract))
}

// TestAnIssuerOrSubjectTheNextStartRefusesWritesNothing: an issuer that cannot
// be read, or a subject no provider hands out, would leave a server that does
// not come back, so the confirm refuses it with the key.
func TestAnIssuerOrSubjectTheNextStartRefusesWritesNothing(t *testing.T) {
	t.Parallel()

	for name, change := range map[string]func(*httpapi.SetupDecision){
		"an issuer that cannot be read": func(*httpapi.SetupDecision) {},
		"a subject no provider can name": func(decision *httpapi.SetupDecision) {
			decision.Configuration["core.principal"] = "two words"
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			install, restarts := wizardInstall(t, dir)
			install.first.discover = func(context.Context, string) error {
				if name == "an issuer that cannot be read" {
					return errors.New("fetch https://sso.example.com: connection refused")
				}

				return nil
			}

			decision := oidcDecision(dir, "https://sso.example.com")
			change(&decision)

			_, err := install.first.Record(t.Context(), decision)
			require.Error(t, err)
			require.Zero(t, *restarts)

			_, statErr := os.Stat(config.SetupFile(install.first.configPath))
			require.ErrorIs(t, statErr, os.ErrNotExist, "a refused answer wrote the wizard's file")
		})
	}
}

// TestTheStartAfterTheWizardRecordsTheDecision holds rules 22 and 23: the new
// process creates the administrator from the hash, declares the storage,
// records the mode and removes the file.
func TestTheStartAfterTheWizardRecordsTheDecision(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	install, _ := wizardInstall(t, dir)

	_, err := install.first.Record(t.Context(), wizardDecision(dir))
	require.NoError(t, err)

	restarted, _ := wizardInstall(t, dir)

	pending, err := readPendingFirstRun(restarted.first.configPath)
	require.NoError(t, err)
	require.NotNil(t, pending)

	require.NoError(t, recordPendingAdmission(restarted.store, pending))
	require.NoError(t, restarted.first.applyPending(t.Context(), pending))
	require.False(t, restarted.first.Pending())

	_, statErr := os.Stat(firstRunPath(restarted.first.configPath))
	require.NoError(
		t,
		statErr,
		"the decision went before the ports were open, so a port that fails could not roll back",
	)

	restarted.first.letPendingGo(t.Context())()

	admission, err := restarted.store.AdmissionMode()
	require.NoError(t, err)
	require.Equal(t, resource.AdmissionOpen, admission)

	session, err := restarted.who.accounts.Login(t.Context(), "owner@example.com", "correct horse battery")
	require.NoError(t, err, "the administrator the wizard named cannot sign in")
	require.NotEmpty(t, session.Token)

	_, statErr = os.Stat(firstRunPath(restarted.first.configPath))
	require.ErrorIs(t, statErr, os.ErrNotExist, "the decision outlived its recording")

	declared := restarted.store.Document().Instances
	found := false

	for _, instance := range declared {
		if instance.Ref.String() == "storage:local" {
			found = true
		}
	}

	require.True(t, found, "the storage the wizard named was not declared")

	sink := false

	for _, instance := range declared {
		if instance.Ref.String() == "sink:mqtt" {
			sink = true
		}
	}

	require.True(t, sink, "the MQTT sink the wizard named was not declared")

	history := false

	for _, instance := range declared {
		if instance.Ref.String() == "history:past" {
			history = true
		}
	}

	require.True(t, history, "the history store the wizard named was not declared")
}

// TestAStartThatFailsOnTheWizardsFileGoesBack holds rule 24: the wizard's file
// is moved aside, the failure is kept for the page, and the first run is
// pending again.
func TestAStartThatFailsOnTheWizardsFileGoesBack(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	install, _ := wizardInstall(t, dir)

	_, err := install.first.Record(t.Context(), wizardDecision(dir))
	require.NoError(t, err)

	rolledBack, err := RollbackFirstRun(install.first.configPath, errors.New("listen tcp :80: permission denied"))
	require.NoError(t, err)
	require.True(t, rolledBack)

	setup := config.SetupFile(install.first.configPath)
	_, statErr := os.Stat(setup)
	require.ErrorIs(t, statErr, os.ErrNotExist)

	_, statErr = os.Stat(setup + refusedSetupName)
	require.NoError(t, statErr, "the refused file was not kept for the operator to read")

	again, _ := wizardInstall(t, dir)
	require.True(t, again.first.Pending())

	state, err := again.first.State(t.Context())
	require.NoError(t, err)
	require.NotNil(t, state.LastError)
	require.Contains(t, state.LastError.Message, "permission denied")

	rolledBack, err = RollbackFirstRun(install.first.configPath, errors.New("again"))
	require.NoError(t, err)
	require.False(t, rolledBack, "a second failure rolled back again, which would loop")
}

// TestAConfirmedWizardGivesPrintersTheirOwnListener writes the connector
// block the driver reads, under the name the server declares by itself.
func TestAConfirmedWizardGivesPrintersTheirOwnListener(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	install, _ := wizardInstall(t, dir)

	made, err := writeCertificates(certsPath(install.first.configPath), []string{"printserver"}, time.Now())
	require.NoError(t, err)

	decision := wizardDecision(dir)
	decision.Printers = &httpapi.SetupPrinters{
		Addr: ":8443", Certificate: made.PrinterCertificate, Key: made.PrinterKey,
	}

	_, err = install.first.Record(t.Context(), decision)
	require.NoError(t, err)

	core, err := config.Load(install.first.configPath)
	require.NoError(t, err)
	require.Contains(t, core.Connectors, printerConnector())
	require.Equal(t, printerConnector(), core.Connectors[printerConnector()].Type)
}

// TestAPortAnotherProgramHoldsIsRefusedBeforeTheRestart catches what would
// otherwise stop the start after the restart, where nothing can roll back.
func TestAPortAnotherProgramHoldsIsRefusedBeforeTheRestart(t *testing.T) {
	t.Parallel()

	held, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Close() })

	dir := t.TempDir()
	install, restarts := wizardInstall(t, dir)
	install.first.listening = "127.0.0.1:1"

	decision := wizardDecision(dir)
	decision.Configuration = map[string]any{"core.api.addr": held.Addr().String()}

	_, err = install.first.Record(t.Context(), decision)
	require.Error(t, err)
	require.Contains(t, err.Error(), held.Addr().String())
	require.Zero(t, *restarts)
}

// TestThePortIsLockedWhenTheServerWasStartedWithAddr: the flag wins at every
// start, so the wizard shows its value and takes no answer for the key.
func TestThePortIsLockedWhenTheServerWasStartedWithAddr(t *testing.T) {
	t.Parallel()

	install, _ := wizardInstall(t, t.TempDir())
	install.first.apiFlag = ":8090"

	contract, err := install.first.contract()
	require.NoError(t, err)

	for _, key := range contract.Keys {
		if key.Path == apiAddrKey {
			require.True(t, key.Locked)
			require.Equal(t, ":8090", key.Value)
		}
	}
}

// TestTheWizardOffersOnlyWhatThisBinaryRuns holds ADR-20261015-ejz rule 13 for
// every part a build tag adds or takes out. A key whose part this binary was
// built without is not in the contract, and an answer for it is refused with
// the build tag named and nothing written. A start on that file would refuse
// it or ignore it, and the person would not learn why. A key whose part is
// built in is offered. The case runs in every build and checks the side that
// build is on.
func TestTheWizardOffersOnlyWhatThisBinaryRuns(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		key    string
		answer any
		tag    string
		built  bool
	}{
		"metrics":     {key: "core.observability.enabled", answer: true, tag: "observability", built: prom.Compiled},
		"diagnostics": {key: "core.diagnostics.enabled", answer: true, tag: "diagnostics", built: diagnosticsBuilt},
		"dashboards":  {key: "core.dashboards.enabled", answer: true, tag: "nodashboards", built: dashboardsBuilt},
		"postgres": {
			key: "persistence.postgres.dsn", answer: "postgres://db.local/connect", tag: "postgres",
			built: authz.PostgresLinked,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			install, restarts := wizardInstall(t, dir)

			contract, err := install.first.contract()
			require.NoError(t, err)

			offered := slices.ContainsFunc(contract.Keys, func(key httpapi.SetupKey) bool {
				return key.Path == tc.key
			})
			require.Equal(t, tc.built, offered,
				"%s is offered by a binary that cannot run it, or hidden by one that can", tc.key)

			if tc.built {
				return
			}

			decision := wizardDecision(dir)
			decision.Configuration = map[string]any{tc.key: tc.answer}

			_, err = install.first.Record(t.Context(), decision)
			require.Error(t, err)

			raised, ok := errors.AsType[*fault.Fault](err)
			require.True(t, ok, "the refusal is not a fault: %v", err)
			require.Equal(t, http.StatusUnprocessableEntity, raised.Status())
			require.Contains(t, err.Error(), "the "+tc.tag+" tag", "the refusal does not name the build tag")
			require.Zero(t, *restarts)

			_, statErr := os.Stat(config.SetupFile(install.first.configPath))
			require.ErrorIs(t, statErr, os.ErrNotExist, "a refused answer wrote the wizard's file")
		})
	}
}
