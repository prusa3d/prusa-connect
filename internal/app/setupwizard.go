package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/app/builtin"
	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/kernel/atomicfile"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/platform/auth"
	"github.com/prusa3d/prusa-connect/internal/platform/auth/local"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/fault"
)

// ErrRestart is what Run returns when the first-run wizard was confirmed: the
// server stopped so that it can start again on the file the wizard wrote. The
// command that started it starts it again.
var ErrRestart = errors.New("app: the first run asks for a restart")

// The two files the confirmed wizard leaves beside its configuration. The
// first is the decision the next start records. The second is the start that
// failed on it, which the page shows beside the answers that caused it.
const (
	firstRunFileName  = "first-run.json"
	firstRunFailed    = "first-run-failure.json"
	refusedSetupName  = ".refused"
	firstRunVersion   = 1
	privateFileMode   = 0o600
	setupFileMode     = 0o644
	storageFolderMode = 0o750
	storageInstance   = "local"
)

// pendingFirstRun is the decision the wizard confirmed, as the next start reads
// it. It holds a hash of the administrator's password, made at the cost of the
// new configuration, and never the password.
type pendingFirstRun struct {
	Version       int                   `json:"version"`
	Mode          string                `json:"mode"`
	Admission     string                `json:"admission"`
	Modules       []pendingModule       `json:"modules,omitempty"`
	Administrator *pendingAdministrator `json:"administrator,omitempty"`
	StorageRoot   string                `json:"storage_root,omitempty"`
	MQTT          *pendingMQTT          `json:"mqtt,omitempty"`
	History       *pendingHistory       `json:"history,omitempty"`
}

// pendingHistory is the history store the next start declares.
type pendingHistory struct {
	Path      string `json:"path"`
	Retention string `json:"retention"`
}

// pendingMQTT is the broker a sink is declared for. The password is written
// here because the sink needs it in clear. The file is the server's own, at
// 0600, and is deleted once the server serves on the new configuration. From
// then on the overlay holds the password as it holds every other credential
// option.
type pendingMQTT struct {
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type pendingModule struct {
	Instance string `json:"instance"`
	Enabled  bool   `json:"enabled"`
}

type pendingAdministrator struct {
	Username string `json:"username"`
	Hash     string `json:"password_hash"`
}

// firstRunFailure is a start that failed on the wizard's configuration.
type firstRunFailure struct {
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// firstRunPath and failurePath are where the two files go for an operator file.
func firstRunPath(configPath string) string {
	return filepath.Join(filepath.Dir(config.SetupFile(configPath)), firstRunFileName)
}

func failurePath(configPath string) string {
	return filepath.Join(filepath.Dir(config.SetupFile(configPath)), firstRunFailed)
}

// Configuration is the contract the wizard draws its fields from, behind the
// setup code.
func (f *firstRun) Configuration(
	ctx context.Context, client httpapi.SetupClient, code string,
) (httpapi.SetupConfiguration, error) {
	if !f.Pending() {
		return httpapi.SetupConfiguration{}, fault.AuthSetupClosed.New(
			"this server has already recorded what it does about identity")
	}

	if err := f.code.admit(ctx, client, code); err != nil {
		return httpapi.SetupConfiguration{}, err
	}

	contract, err := f.contract()
	if err != nil {
		return httpapi.SetupConfiguration{}, fmt.Errorf("describe the configuration: %w", err)
	}

	return contract, nil
}

// recordForRestart writes the confirmed wizard down and asks for a restart.
// Nothing is recorded in this process: the account belongs in the database the
// new configuration names, so the new process records the whole decision. It
// runs with the setup code accepted and the recording lock held.
func (f *firstRun) recordForRestart(
	ctx context.Context, decision httpapi.SetupDecision, admission string,
) (httpapi.SetupResult, error) {
	contract, err := f.contract()
	if err != nil {
		return httpapi.SetupResult{}, fmt.Errorf("describe the configuration: %w", err)
	}

	document, err := setupDocument(decision.Configuration, contract, decision.Printers)
	if err != nil {
		return httpapi.SetupResult{}, err
	}

	core, err := config.LoadWith(f.configPath, document)
	if err == nil {
		err = core.Validate()
	}

	if err == nil {
		err = requireBuiltParts(core)
	}

	if err != nil {
		return httpapi.SetupResult{}, httpfault.RequestInvalid("body.configuration",
			"a configuration this server starts on", err.Error())
	}

	if err = f.checkPortsFree(core, decision.Printers); err != nil {
		return httpapi.SetupResult{}, err
	}

	if err = checkPrincipal(core); err != nil {
		return httpapi.SetupResult{}, err
	}

	if err = f.checkIssuer(ctx, core); err != nil {
		return httpapi.SetupResult{}, err
	}

	pending := pendingFirstRun{Version: firstRunVersion, Mode: decision.Mode, Admission: admission}

	for _, module := range decision.Modules {
		if _, err = ref.Parse(module.Instance); err != nil {
			return httpapi.SetupResult{}, httpfault.RequestInvalid("body.modules.instance",
				"an instance in kind:name spelling", module.Instance)
		}

		pending.Modules = append(pending.Modules, pendingModule(module))
	}

	if pending.StorageRoot, err = storageRootOf(decision.StorageRoot); err != nil {
		return httpapi.SetupResult{}, err
	}

	if pending.MQTT, err = mqttOf(decision.MQTT); err != nil {
		return httpapi.SetupResult{}, err
	}

	if pending.History, err = historyOf(decision.History, f.configPath); err != nil {
		return httpapi.SetupResult{}, err
	}

	if pending.Administrator, err = administratorOf(core, decision); err != nil {
		return httpapi.SetupResult{}, err
	}

	if err = f.writeForRestart(document, setupModeFor(decision.Configuration, contract), pending); err != nil {
		return httpapi.SetupResult{}, httpfault.Unexpected("write the configuration the wizard confirmed").Wrap(err)
	}

	f.code.consume()
	f.logger.InfoContext(ctx, "the first-run wizard was confirmed and the server restarts on it",
		"setup", config.SetupFile(f.configPath), "mode", decision.Mode, "admission", admission)

	if f.restart != nil {
		f.restart()
	}

	return httpapi.SetupResult{Mode: decision.Mode, Admission: admission, Restarting: true}, nil
}

// writeForRestart writes the decision first and the configuration second, and
// clears a failure an earlier attempt left. A process that dies between the two
// writes starts on the old configuration with the decision, so what the person
// chose is still recorded, on the defaults.
func (f *firstRun) writeForRestart(document []byte, mode os.FileMode, pending pendingFirstRun) error {
	encoded, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the first-run decision: %w", err)
	}

	setup := config.SetupFile(f.configPath)
	if err = os.MkdirAll(filepath.Dir(setup), storageFolderMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(setup), err)
	}

	if err = atomicfile.Write(firstRunPath(f.configPath), encoded, privateFileMode); err != nil {
		return err
	}

	if err = atomicfile.Write(setup, document, mode); err != nil {
		return err
	}

	if err = os.Remove(failurePath(f.configPath)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("clear the last failed start: %w", err)
	}

	return nil
}

// administratorOf hashes the first administrator's password at the cost of the
// configuration the server restarts on, and checks the name now, so a name the
// account store refuses is refused while the person is still on the page.
func administratorOf(core config.Core, decision httpapi.SetupDecision) (*pendingAdministrator, error) {
	if decision.Account == nil {
		return nil, nil //nolint:nilnil // an open deployment names no administrator, which is not an error.
	}

	if core.Auth.Enabled() {
		return nil, httpfault.RequestInvalid("body.account",
			"nothing", "core.auth.issuer is set, so people sign in at the issuer and this server keeps no password")
	}

	username := local.FoldUsername(decision.Account.Username)
	if err := local.ValidateUsername(username); err != nil {
		return nil, httpfault.RequestInvalid("body.account.username", "a name the account store takes", err.Error())
	}

	hasher, err := auth.NewHasher(core.Auth.PasswordCost())
	if err != nil {
		return nil, httpfault.RequestInvalid("body.configuration",
			"a password cost this server can hash at", err.Error())
	}

	hash, err := hasher.Hash([]byte(decision.Account.Password))
	if err != nil {
		return nil, httpfault.RequestInvalid("body.account.password", "8 to 72 bytes", err.Error())
	}

	return &pendingAdministrator{Username: username, Hash: hash}, nil
}

// setupDocument turns the wizard's answers into the file it writes. Each path
// must be one the contract offered and the operator's file does not set, and
// each value must be of the key's kind.
func setupDocument(
	answers map[string]any, contract httpapi.SetupConfiguration, printers *httpapi.SetupPrinters,
) ([]byte, error) {
	keys := make(map[string]httpapi.SetupKey, len(contract.Keys))
	for _, key := range contract.Keys {
		keys[key.Path] = key
	}

	root := map[string]any{}

	for path, answer := range answers {
		key, known := keys[path]
		if part, missing := notBuilt(path); !known && missing {
			return nil, httpfault.RequestInvalid("body.configuration."+path,
				"a path get-setup-configuration lists", part.without)
		}

		if !known {
			return nil, httpfault.RequestInvalid("body.configuration."+path,
				"a path get-setup-configuration lists", "this key is not one the wizard sets")
		}

		if key.Locked {
			return nil, httpfault.RequestInvalid("body.configuration."+path,
				"a key the operator's file does not set", "the operator's file sets this key, and its value wins")
		}

		value, refused := answerOf(key, answer)
		if refused != nil {
			return nil, httpfault.RequestInvalid("body.configuration."+path, "a "+key.Kind, refused.Error())
		}

		place(root, strings.Split(path, "."), value)
	}

	if printers != nil {
		block, blockErr := printersOf(printers, contract, keys, answers)
		if blockErr != nil {
			return nil, blockErr
		}

		place(root, []string{"core", "connectors"}, block)
	}

	body, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("encode the wizard's configuration: %w", err)
	}

	if len(root) == 0 {
		body = nil
	}

	header := "# Written by the first-run wizard on " + time.Now().UTC().Format(time.DateOnly) + ".\n" +
		"# A key in config.yaml overrides the same key here. An operator may edit or delete this file.\n"

	return append([]byte(header), body...), nil
}

// printersOf is the connector block the wizard writes for the printers'
// listener, against the API address the server will listen on.
func printersOf(
	printers *httpapi.SetupPrinters, contract httpapi.SetupConfiguration,
	keys map[string]httpapi.SetupKey, answers map[string]any,
) (map[string]any, error) {
	if !contract.Printers {
		return nil, httpfault.RequestInvalid("body.printers",
			"nothing", "the operator's file declares the connectors, so the printers' listener is theirs")
	}

	api := config.DefaultAPIAddr
	if answered, isText := answers["core.api.addr"].(string); isText && answered != "" {
		api = answered
	} else if held, isHeld := keys["core.api.addr"].Value.(string); isHeld && held != "" {
		api = held
	}

	return printersBlock(printers, api)
}

// place writes value at the dotted path under root.
func place(root map[string]any, path []string, value any) {
	for _, part := range path[:len(path)-1] {
		next, ok := root[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			root[part] = next
		}

		root = next
	}

	root[path[len(path)-1]] = value
}

// errKind reports an answer that is not of its key's kind.
var errKind = errors.New("the value is not of the key's kind")

// answerOf checks one answer against its key and returns it as the file writes
// it.
func answerOf(key httpapi.SetupKey, answer any) (any, error) {
	switch config.Kind(key.Kind) {
	case config.KindSwitch:
		if value, ok := answer.(bool); ok {
			return value, nil
		}
	case config.KindNumber:
		if value, ok := answer.(float64); ok && value == math.Trunc(value) && math.Abs(value) < 1<<53 {
			return int64(value), nil
		}
	case config.KindDuration:
		if value, ok := answer.(string); ok {
			if _, err := time.ParseDuration(value); err != nil {
				return nil, fmt.Errorf("%w: %w", errKind, err)
			}

			return value, nil
		}
	case config.KindTextList:
		return textList(answer)
	case config.KindText, config.KindMapping:
		if value, ok := answer.(string); ok {
			return checkedChoice(key, value)
		}
	}

	return nil, errKind
}

// textList is a list of text, each item a string.
func textList(answer any) (any, error) {
	items, ok := answer.([]any)
	if !ok {
		return nil, errKind
	}

	out := make([]string, 0, len(items))

	for _, item := range items {
		text, isText := item.(string)
		if !isText {
			return nil, errKind
		}

		out = append(out, text)
	}

	return out, nil
}

// checkedChoice refuses a value outside a key's closed set.
func checkedChoice(key httpapi.SetupKey, value string) (any, error) {
	if len(key.Choices) == 0 {
		return value, nil
	}

	if slices.Contains(key.Choices, value) {
		return value, nil
	}

	return nil, fmt.Errorf("%w: %q is not one of %s", errKind, value, strings.Join(key.Choices, ", "))
}

// readPendingFirstRun reads the decision the wizard left for this start, and
// reports none when there is no file.
func readPendingFirstRun(configPath string) (*pendingFirstRun, error) {
	raw, err := os.ReadFile(firstRunPath(configPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // no file is a start no wizard was confirmed before.
	}

	if err != nil {
		return nil, fmt.Errorf("read the first-run decision: %w", err)
	}

	var pending pendingFirstRun
	if err = json.Unmarshal(raw, &pending); err != nil {
		return nil, fmt.Errorf("read the first-run decision in %s: %w", firstRunPath(configPath), err)
	}

	if pending.Version != firstRunVersion {
		return nil, fmt.Errorf("%s says version %d and this build reads %d",
			firstRunPath(configPath), pending.Version, firstRunVersion)
	}

	return &pending, nil
}

// admitPending reads the decision a confirmed wizard left and writes its
// admission choice, which has to be there before the connectors are built.
func admitPending(configPath string, store *resource.Store) (*pendingFirstRun, error) {
	pending, err := readPendingFirstRun(configPath)
	if err != nil {
		return nil, err
	}

	if err = recordPendingAdmission(store, pending); err != nil {
		return nil, err
	}

	return pending, nil
}

// settleWith records the decision a confirmed wizard left, and then settles
// an install that has no recorded first run, in the order a start takes them.
func (f *firstRun) settleWith(ctx context.Context, pending *pendingFirstRun) error {
	if err := f.applyPending(ctx, pending); err != nil {
		return err
	}

	return f.settle(ctx)
}

// recordPendingAdmission writes the admission choice the wizard confirmed
// before the connectors are built, because a connector's mode is fixed when it
// is built. Written later, the choice would wait for one more start.
func recordPendingAdmission(store *resource.Store, pending *pendingFirstRun) error {
	if pending == nil {
		return nil
	}

	if err := store.RecordAdmission(pending.Admission); err != nil {
		return fmt.Errorf("record how a new printer joins this server: %w", err)
	}

	return nil
}

// applyPending records the decision the wizard confirmed, in the order Record
// takes it: the storage, the sink, the history store and the modules, then the
// administrator, and the mode last. It does not delete the file. letPendingGo
// deletes it once the server serves, so a start that fails before then finds
// the file again and the rollback can run.
func (f *firstRun) applyPending(ctx context.Context, pending *pendingFirstRun) error {
	if pending == nil || !f.Pending() {
		return nil
	}

	if err := f.declareStorage(ctx, pending.StorageRoot); err != nil {
		return err
	}

	if err := f.declareMQTT(ctx, pending.MQTT); err != nil {
		return err
	}

	if err := f.declareHistory(ctx, pending.History); err != nil {
		return err
	}

	choices := make([]httpapi.SetupModuleChoice, 0, len(pending.Modules))
	for _, module := range pending.Modules {
		choices = append(choices, httpapi.SetupModuleChoice(module))
	}

	if err := f.switchModules(ctx, choices); err != nil {
		return err
	}

	if pending.Administrator != nil {
		if f.importFirst == nil {
			return errors.New("the first-run decision names an administrator and this server keeps no accounts")
		}

		if err := f.importFirst(ctx, pending.Administrator.Username, pending.Administrator.Hash); err != nil {
			return fmt.Errorf("create the first administrator the wizard named: %w", err)
		}
	}

	if err := f.store.RecordSetupMode(pending.Mode); err != nil {
		return fmt.Errorf("record what this server does about identity: %w", err)
	}

	f.mode.set(pending.Mode)
	f.code.consume()
	f.applied = true

	f.logger.InfoContext(ctx, "the first run the wizard confirmed is recorded",
		"mode", pending.Mode, "admission", pending.Admission, "storage", pending.StorageRoot)

	return nil
}

// declareStorage declares the storage the wizard asked for, through the same
// store an administrator's own call goes through.
func (f *firstRun) declareStorage(ctx context.Context, root string) error {
	if root == "" {
		return nil
	}

	instance, err := ref.New(ref.KindStorage, storageInstance)
	if err != nil {
		return fmt.Errorf("name the storage the wizard declares: %w", err)
	}

	// A start that declared the storage and then failed later in the same
	// apply finds it declared on the next try.
	if _, err = f.store.Get(ctx, f.principal, instance); err == nil {
		return nil
	}

	settings, err := json.Marshal(map[string]string{"root": root})
	if err != nil {
		return fmt.Errorf("encode the storage settings: %w", err)
	}

	desired := httpapi.ResourceSpec{
		Type:    builtin.TypeFilesystem,
		Enabled: true,
		Config:  httpapi.ResourceConfig(settings),
	}

	if _, err = f.store.Apply(ctx, f.principal, instance, desired,
		httpapi.Precondition{MustNotExist: true}); err != nil {
		return fmt.Errorf("declare the storage the wizard named: %w", err)
	}

	return nil
}

// RollbackFirstRun undoes a wizard configuration the server could not start
// on. It moves the wizard's file aside, writes the failure where the next
// first run reads it, and removes the decision, so the next start runs on the
// configuration the server had and the wizard asks again.
//
// It reports false when there was nothing to undo, which is every failed start
// that did not follow a confirmed wizard. The caller then reports the failure
// to the operator and stops.
func RollbackFirstRun(configPath string, cause error) (bool, error) {
	if _, err := os.Stat(firstRunPath(configPath)); err != nil {
		return false, nil //nolint:nilerr // no decision means nothing to undo, which is not an error.
	}

	setup := config.SetupFile(configPath)
	if err := os.Rename(setup, setup+refusedSetupName); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("move the wizard's configuration aside: %w", err)
	}

	encoded, err := json.MarshalIndent(firstRunFailure{Message: cause.Error(), At: time.Now().UTC()}, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode the failed start: %w", err)
	}

	if err = atomicfile.Write(failurePath(configPath), encoded, privateFileMode); err != nil {
		return false, err
	}

	if err = os.Remove(firstRunPath(configPath)); err != nil {
		return false, fmt.Errorf("remove the first-run decision: %w", err)
	}

	return true, nil
}

// lastFailure is the start that failed after the wizard was last confirmed,
// or nil.
func lastFailure(configPath string) *httpapi.SetupFailure {
	raw, err := os.ReadFile(failurePath(configPath))
	if err != nil {
		return nil
	}

	var failure firstRunFailure
	if json.Unmarshal(raw, &failure) != nil {
		return nil
	}

	return &httpapi.SetupFailure{Message: failure.Message, At: failure.At.Format(time.RFC3339)}
}

// mqttSinkName is the instance a sink the wizard declares is named.
const mqttSinkName = "mqtt"

// mqttSinkType is the sink type that sends to a broker.
const mqttSinkType = "mqtt"

// mqttSchemes are the broker addresses the sink connects to.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var mqttSchemes = map[string]bool{"tcp": true, "mqtt": true, "ssl": true, "tls": true, "mqtts": true}

// mqttOf checks the broker the wizard names, so an address the sink would
// refuse is refused while the person can still correct it.
func mqttOf(broker *httpapi.SetupMQTT) (*pendingMQTT, error) {
	if broker == nil {
		return nil, nil //nolint:nilnil // no broker is a wizard that declared no sink.
	}

	address := strings.TrimSpace(broker.URL)
	scheme, rest, found := strings.Cut(address, "://")

	if address == "" || (found && !mqttSchemes[strings.ToLower(scheme)]) || (found && rest == "") {
		return nil, httpfault.RequestInvalid("body.mqtt.url",
			"tcp://host:1883, or ssl://host:8883 for TLS", address)
	}

	return &pendingMQTT{URL: address, Username: broker.Username, Password: broker.Password}, nil
}

// declareMQTT declares the sink that sends the printers' state to the broker
// the wizard named, through the same store an administrator's call goes
// through. It is optional: a broker that is down never stops the server.
func (f *firstRun) declareMQTT(ctx context.Context, broker *pendingMQTT) error {
	if broker == nil {
		return nil
	}

	instance, err := ref.New(ref.KindSink, mqttSinkName)
	if err != nil {
		return fmt.Errorf("name the sink the wizard declares: %w", err)
	}

	if _, err = f.store.Get(ctx, f.principal, instance); err == nil {
		return nil
	}

	settings := map[string]string{"url": broker.URL}
	if broker.Username != "" {
		settings["username"] = broker.Username
	}

	if broker.Password != "" {
		settings["password"] = broker.Password
	}

	encoded, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode the sink settings: %w", err)
	}

	desired := httpapi.ResourceSpec{Type: mqttSinkType, Enabled: true, Config: httpapi.ResourceConfig(encoded)}

	if _, err = f.store.Apply(ctx, f.principal, instance, desired,
		httpapi.Precondition{MustNotExist: true}); err != nil {
		return fmt.Errorf("declare the MQTT sink the wizard named: %w", err)
	}

	return nil
}

// The history store the wizard declares: one SQLite file beside the wizard's
// configuration file, apart from the host's own database.
const (
	historyInstance   = "past"
	historyType       = "core/sqlite"
	wizardHistoryFile = "history.db"
)

// historyOf checks how long the wizard keeps history and names the file, so a
// retention the provider would refuse is refused while the person is here.
func historyOf(history *httpapi.SetupHistory, configPath string) (*pendingHistory, error) {
	if history == nil {
		return nil, nil //nolint:nilnil // no history is a wizard that keeps none.
	}

	retention := strings.TrimSpace(history.Retention)
	if retention != "0" {
		if keep, err := time.ParseDuration(retention); err != nil || keep <= 0 {
			return nil, httpfault.RequestInvalid("body.history.retention", "a duration such as 48h, or 0", retention)
		}
	}

	path, err := filepath.Abs(filepath.Join(filepath.Dir(config.SetupFile(configPath)), wizardHistoryFile))
	if err != nil {
		return nil, fmt.Errorf("name the history file: %w", err)
	}

	return &pendingHistory{Path: path, Retention: retention}, nil
}

// declareHistory declares the history store the wizard asked for. It is
// optional: a store that fails to open retries and never stops the server.
func (f *firstRun) declareHistory(ctx context.Context, history *pendingHistory) error {
	if history == nil {
		return nil
	}

	instance, err := ref.New(ref.KindHistory, historyInstance)
	if err != nil {
		return fmt.Errorf("name the history store the wizard declares: %w", err)
	}

	if _, err = f.store.Get(ctx, f.principal, instance); err == nil {
		return nil
	}

	encoded, err := json.Marshal(map[string]any{
		"path":      history.Path,
		"retention": map[string]string{"raw": history.Retention},
	})
	if err != nil {
		return fmt.Errorf("encode the history settings: %w", err)
	}

	desired := httpapi.ResourceSpec{Type: historyType, Enabled: true, Config: httpapi.ResourceConfig(encoded)}

	if _, err = f.store.Apply(ctx, f.principal, instance, desired,
		httpapi.Precondition{MustNotExist: true}); err != nil {
		return fmt.Errorf("declare the history store the wizard named: %w", err)
	}

	return nil
}

// letPendingGo is what the start runs once every port is open: the decision
// the wizard left is recorded and the server will serve, so the file is
// deleted. A start that fails before this point still finds the file, and the
// command rolls the wizard's answers back.
func (f *firstRun) letPendingGo(ctx context.Context) func() {
	return func() {
		if !f.applied {
			return
		}

		if err := os.Remove(firstRunPath(f.configPath)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			f.logger.WarnContext(ctx, "the first-run decision could not be removed once the server served",
				"file", firstRunPath(f.configPath), "error", err.Error())
		}
	}
}

// apiAddrKey is the key the -addr flag overrides.
const apiAddrKey = "core.api.addr"

// contract is the configuration the wizard reads, with the port locked when
// the process was started with -addr: the flag wins over the file at every
// start, so an answer the wizard wrote there would never be used.
func (f *firstRun) contract() (httpapi.SetupConfiguration, error) {
	contract, err := setupContract(f.configPath)
	if err != nil {
		return httpapi.SetupConfiguration{}, err
	}

	if f.apiFlag == "" {
		return contract, nil
	}

	for index := range contract.Keys {
		if contract.Keys[index].Path == apiAddrKey {
			contract.Keys[index].Locked = true
			contract.Keys[index].Value = f.apiFlag
		}
	}

	return contract, nil
}

// checkPortsFree refuses an answer that names a port another program holds,
// while the person can still pick another. A port this server listens on
// already is its own, and is free once it restarts.
func (f *firstRun) checkPortsFree(core config.Core, printers *httpapi.SetupPrinters) error {
	if f.apiFlag == "" && !config.SameSocket(core.API.Addr, f.listening) {
		if err := portFree(core.API.Addr); err != nil {
			return httpfault.RequestInvalid(
				"body.configuration."+apiAddrKey,
				"a port no other program uses",
				err.Error(),
			)
		}
	}

	if printers != nil && !config.SameSocket(printers.Addr, f.listening) {
		if err := portFree(printers.Addr); err != nil {
			return httpfault.RequestInvalid("body.printers.addr", "a port no other program uses", err.Error())
		}
	}

	return nil
}

// portFree opens addr and closes it again.
func portFree(addr string) error {
	listener, err := net.Listen("tcp", addr) //nolint:noctx // a probe that is closed at once.
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	return listener.Close()
}
