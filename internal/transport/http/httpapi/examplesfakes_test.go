package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/betaportal"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/devtools"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

// The server every sample is driven through. Each port is a fake holding
// exactly the value its sample was built from, read back out of the example
// table with sampled, so the sample and the fake cannot drift apart: the round
// trip then proves what the handler does between the port and the wire, which
// is the encoding, the status, the wrapping and every member it adds or drops.
//
// A fake whose answer depends on the request, the upload's digest and the
// gcode line the printer refuses, computes it the way the real port would.

// exampleDataServer is the server every sample is driven through.
func exampleDataServer(t *testing.T) *Server {
	t.Helper()

	cfg := exampleConfig(exampleFleetOf(t), exampleResourceStore{}, examplePresetStore{})

	health := sampled[map[string]any](t, "get-health", "")
	cfg.Version = health["version"].(string)        //nolint:forcetypeassert // the sample's own literal
	cfg.InstanceID = health["instance_id"].(string) //nolint:forcetypeassert // the sample's own literal
	cfg.Endpoints = exampleEndpoints{items: sampled[[]ConnectorEndpoint](t, "list-connector-endpoints", "items")}
	cfg.Links = &exampleLinks{items: sampled[[]LinkInstance](t, "list-links", "items")}
	cfg.Discovery = &fakeDiscovery{
		hidden: true, nearby: sampled[[]NearbyPrinter](t, "list-nearby-printers", "items"),
		described: hiddenWorkshopServer(),
	}
	cfg.TimeServer = &fakeTimeServer{enabled: true}
	cfg.Origins = &fakeOrigins{
		stored: []string{"https://dashboard.workshop.example"},
		fixed:  workshopOrigins()[1:],
	}
	cfg.Grants = &exampleGrants{
		fakeGrantStore: &fakeGrantStore{},
		held:           tuplesOf(t, sampled[[]Grant](t, "list-grants", "items")),
		org:            orgTuplesOf(t, sampled[[]Account](t, "list-accounts", "items")),
	}
	cfg.Verbs = exampleVerbs{
		held:     verbGrantsOf(t, sampled[[]VerbGrant](t, "list-device-verb-grants", "items")),
		accepted: acceptancesOf(sampled[[]AcceptedChange](t, "list-declaration-acceptances", "items")),
		devices:  rowsOf(sampled[[]SubjectHold](t, "list-subject-grants", "devices")),
		direct:   tuplesOf(t, sampled[[]Grant](t, "list-subject-direct-grants", "items")),
	}
	cfg.Accounts = exampleAccounts{
		info:    sampled[AuthInfo](t, "get-auth-info", ""),
		first:   sampled[Account](t, "create-first-account", ""),
		rows:    sampled[[]Account](t, "list-accounts", "items"),
		created: sampled[Account](t, "create-account", ""),
		caller:  callerOf(sampled[map[string]any](t, "get-principal", "")),
		expires: sampled[map[string]any](t, "password-login", "")["expires_at"].(time.Time), //nolint:forcetypeassert // the sample's own literal
	}
	cfg.Credentials = exampleCredentials{
		minted: sampled[createdCredentialBody](t, "create-credential", ""),
		rows:   sampled[[]APICredential](t, "list-credentials", "items"),
	}
	cfg.AppPairings = exampleAppPairings{
		pairing:   sampled[AppPairing](t, "get-app-pairing", ""),
		collected: sampled[collectedCredentialBody](t, "collect-app-pairing", ""),
		issued:    sampled[issuedAppPairingBody](t, "request-app-pairing", ""),
	}
	cfg.Pairings = examplePairings{
		paired: sampled[Pairing](t, "create-pairing", ""),
		rows:   sampled[[]Pairing](t, "list-pairings", "items"),
	}
	cfg.Stores = exampleStores{
		storages: sampled[[]Storage](t, "list-storages", "items"),
		files:    sampled[[]StorageEntry](t, "list-storage-files", "items"),
		entry:    sampled[StorageEntry](t, "stat-storage-entry", ""),
	}
	cfg.History = func() (HistoryStore, error) { return exampleHistory{}, nil }
	cfg.Registrations = &fakeRegistrations{
		waiting: sampled[[]Registration](t, "list-pending-registrations", "items"),
		// The id an approval binds. The real surface reads it off the fleet
		// after the binding, and a fake that answered none would publish a
		// sample of an approval that named no device.
		bound: sampled[Registration](t, "approve-registration", "").DeviceID,
	}
	cfg.Readiness = &fakeReadiness{report: sampled[ReadinessReport](t, "get-readiness", "")}
	cfg.BetaPortal = exampleBetaPortal{
		status:    sampled[betaPortalPairingStatus](t, "get-beta-portal-pairing-status", ""),
		started:   sampled[betaPortalPairingStatus](t, "start-beta-portal-pairing", ""),
		refreshed: sampled[betaPortalPairingStatus](t, "refresh-beta-portal-pairing", ""),
	}
	cfg.App = sampled[AppSettings](t, "get-ui-settings", "")
	cfg.Connectors = exampleConnectors{rollups: sampled[[]ConnectorRollup](t, "get-connector-health", "items")}
	cfg.State = func() (StateStore, error) {
		return exampleStateStore{items: sampled[[]StateNamespace](t, "list-state-namespaces", "items")}, nil
	}
	cfg.ModuleState = func() (ModuleStateStore, error) {
		return exampleModuleState{
			keys:   sampled[[]string](t, "list-module-state", "items"),
			record: sampled[ModuleStateRecord](t, "get-module-state", ""),
		}, nil
	}
	cfg.Modules = exampleModules(t)
	cfg.ModuleAccess = exampleModuleAccess{}
	cfg.ModuleInstall = exampleInstaller{
		installed: sampled[ModuleInstallation](t, "install-module", ""),
		offers:    sampled[[]ModuleOffer](t, "list-module-index", "items"),
		market:    sampled[ModuleMarketplace](t, "list-module-index", "marketplace"),
		left:      sampled[[]ModuleDatabaseLeft](t, "list-module-databases", "items"),
		purged:    sampled[[]ModuleDatabasePurged](t, "purge-module-database", "items"),
		plan:      sampled[ModulePlan](t, "plan-module-install", ""),
		removal:   sampled[ModuleRemoval](t, "get-module-removal", ""),
		why:       sampled[[]ModuleReason](t, "get-module-why", "items"),
		whyNot:    sampled[[]ModuleReason](t, "get-module-why-not", "items"),
		outdated:  sampled[[]ModuleOutdated](t, "list-module-outdated", "items"),
	}
	cfg.Events = exampleEvents{count: sampled[EventStreamStats](t, "get-event-stream-stats", "").Subscribers}
	cfg.Cameras = &stubCameras{
		listed: sampled[[]Camera](t, "list-cameras", "items"),
		types:  sampled[[]CameraType](t, "list-camera-types", "items"),
		answer: CameraAnswer{SDP: exampleAnswer},
	}
	cfg.CameraSettings = &stubCameraSettings{listed: sampled[[]CameraSetting](t, "list-camera-settings", "items")}
	cfg.Metrics = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", plainMedia)
		_, _ = io.WriteString(w, exampleExposition)
	})
	cfg.FleetTelemetry = &fleetGrant{allowed: true}
	cfg.Dashboards = exampleDashboardStore(t)
	cfg.Mounts = mountsOf(sampled[devtools.Inventory](t, "get-developer-connectors", "").Routes)

	server, err := New(cfg)
	require.NoError(t, err)

	return server
}

type exampleBetaPortal struct {
	status    betaPortalPairingStatus
	started   betaPortalPairingStatus
	refreshed betaPortalPairingStatus
}

func betaPortalExampleResult(status betaPortalPairingStatus) betaportal.Status {
	var expiresAt time.Time
	if status.ExpiresAt != nil {
		expiresAt = *status.ExpiresAt
	}

	result := betaportal.Status{
		State: status.State, Step: status.Step, UserCode: status.UserCode,
		VerificationURI: status.VerifyURL, VerificationURIComplete: status.VerifyURLComplete,
		ExpiresAt: expiresAt, IntervalSeconds: int(status.PollIntervalMS / millisecondsPerSecond),
		Identity: status.Identity,
	}
	if status.Product != nil {
		result.Product = &betaportal.Product{Slug: status.Product.Slug, Name: status.Product.Name}
	}
	if status.Device != nil {
		result.Device = &betaportal.Device{Identifier: status.Device.Identifier}
	}

	return result
}

func (e exampleBetaPortal) Start(context.Context) (betaportal.Status, error) {
	return betaPortalExampleResult(e.started), nil
}

func (e exampleBetaPortal) Status(context.Context) (betaportal.Status, error) {
	return betaPortalExampleResult(e.status), nil
}

func (e exampleBetaPortal) Refresh(context.Context) (betaportal.Status, error) {
	return betaPortalExampleResult(e.refreshed), nil
}

func (exampleBetaPortal) Disconnect(context.Context) error { return nil }

// sampled is the value one example publishes as its response, or one member of
// it, in the type the handler's port hands back. It is how a fake is seeded
// from the sample rather than from a second copy of the same literal.
func sampled[T any](t *testing.T, operation, member string) T {
	t.Helper()

	example, published := operationExamples()[operation]
	require.Truef(t, published, "no example is published for %s", operation)

	value := example.Response

	if member != "" {
		members, ok := value.(map[string]any)
		require.Truef(t, ok, "%s publishes a %T, which has no member %s", operation, value, member)

		value = members[member]
	}

	typed, ok := value.(T)
	require.Truef(t, ok, "%s publishes %s as a %T, not a %T", operation, member, value, typed)

	return typed
}

// exampleFleetOf is the fleet the device samples describe: the two printers,
// their storages, files, transfers and jobs, and the answer to a command.
func exampleFleetOf(t *testing.T) exampleFleet {
	t.Helper()

	return exampleFleet{
		fakeFleet: &fakeFleet{
			printer:   examplePrinter(),
			resolved:  exampleDeviceCapabilitySet(),
			extra:     []Device{exampleStoppedPrinter()},
			issues:    exampleIssueSummary(),
			total:     sampled[DevicePage](t, "list-devices", "").Total,
			storages:  sampled[DeviceStorages](t, "list-device-storages", "").Items,
			listing:   sampled[DeviceListing](t, "list-device-files", ""),
			transfers: sampled[[]DeviceTransfer](t, "list-device-transfers", "items"),
			running:   sampled[[]RunningTransfer](t, "list-running-transfers", "items"),
			jobs:      exampleDeviceJobs(),
			history:   exampleDeviceJobHistory(),
			fleetJobs: exampleJobPage(),
			result:    sampled[PrinterResult](t, "send-device-command", ""),
		},
		sent: sampled[DeviceTransfer](t, "send-device-file", "transfer"),
	}
}

// exampleFleet is the fake fleet with the two answers a plain fake cannot
// give: the transfer a send produces, and a printer that refuses one line.
type exampleFleet struct {
	*fakeFleet

	sent DeviceTransfer
}

func (f exampleFleet) Send(_ context.Context, _ string, id string, upload DeviceUpload) (DeviceTransfer, error) {
	if id != f.printer.ID {
		return DeviceTransfer{}, noSuchPrinter(id)
	}

	f.record(func() { f.fakeFleet.sent = append(f.fakeFleet.sent, upload) })

	return f.sent, nil
}

// Print composes this fake's own Send and Command, so the published sample
// carries the transfer the send-device-file sample carries.
func (f exampleFleet) Print(
	ctx context.Context, principal string, id string, request DevicePrint,
) (DevicePrintResult, error) {
	return printThrough(ctx, f, principal, id, request)
}

func (f exampleFleet) StartPrint(
	ctx context.Context, principal string, id string, request DevicePrint,
) (DevicePrintResult, error) {
	return f.Print(ctx, principal, id, request)
}

// Command refuses the homing line the preset sample stops on, because the
// example printer is busy, and answers every other command as the fake does.
//
// A line the printer took comes back with what it printed, the way the
// simulator answers one, so the published sample shows the answer field
// carrying something a person would really read rather than an empty string.
func (f exampleFleet) Command(
	ctx context.Context, principal string, id string, cmd PrinterCommand,
) (PrinterResult, error) {
	if id == f.printer.ID && cmd.Args[gcodeArgument] == "G28" {
		return PrinterResult{OK: false, MachineReason: "Device is busy"}, nil
	}

	result, err := f.fakeFleet.Command(ctx, principal, id, cmd)

	line, sent := cmd.Args[gcodeArgument]
	if err == nil && result.OK && sent {
		result.Answer = exampleGCodeAnswer(line)
	}

	return result, err
}

// exampleGCodeAnswer is what the example printer prints back to one line: a
// temperature reading for M105, and "ok" for a line it simply carried out.
func exampleGCodeAnswer(line string) string {
	if strings.HasPrefix(line, "M105") {
		return "ok T:215.0 /215.0 B:60.0 /60.0"
	}

	return "ok"
}

// exampleGrants holds the tuples the grant listing shows and permits every
// decision, so a sample is refused by nothing but its own handler.
type exampleGrants struct {
	*fakeGrantStore

	held []authz.Tuple
	org  []authz.Tuple
}

func (g *exampleGrants) Can(context.Context, authz.Subject, authz.Action, authz.Object) (bool, error) {
	return true, nil
}

func (g *exampleGrants) CanVerb(context.Context, authz.Subject, authz.Object, string) (bool, error) {
	return true, nil
}

// Grants answers the org's own rows for the org and the device rows for
// everything else. The account listing reads the org and the grant listing
// reads a device, and one answer to both would put a device's manager on a
// person's role column.
func (g *exampleGrants) Grants(_ context.Context, _ authz.Subject, object authz.Object) ([]authz.Tuple, error) {
	if object.Type() == authz.TypeOrg {
		return g.org, nil
	}

	return g.held, nil
}

func (g *exampleGrants) WriteGrant(context.Context, authz.Subject, authz.Tuple) (bool, error) {
	return true, nil
}

// tuplesOf parses the grant rows a sample shows back into the tuples the
// store hands the handler.
func tuplesOf(t *testing.T, grants []Grant) []authz.Tuple {
	t.Helper()

	tuples := make([]authz.Tuple, 0, len(grants))

	for _, g := range grants {
		object, err := authz.ParseObject(g.Object)
		require.NoError(t, err)

		subject, err := authz.ParseSubject(g.Subject)
		require.NoError(t, err)

		tuple, err := authz.NewTuple(object, authz.Relation(g.Relation), subject)
		require.NoError(t, err)

		tuples = append(tuples, tuple)
	}

	return tuples
}

// orgTuplesOf is the org rows the account listing reads its role column out of,
// built from the roles that listing publishes, so the sample and the graph
// behind it cannot say different things.
func orgTuplesOf(t *testing.T, accounts []Account) []authz.Tuple {
	t.Helper()

	org, err := authz.NewObject(authz.TypeOrg, bootstrapOrgName)
	require.NoError(t, err)

	var tuples []authz.Tuple

	for _, account := range accounts {
		subject, subjectErr := authz.NewSubject(authz.TypeUser, account.ID)
		require.NoError(t, subjectErr)

		for _, relation := range account.Grants {
			tuple, tupleErr := authz.NewTuple(org, authz.Relation(relation), subject)
			require.NoError(t, tupleErr)

			tuples = append(tuples, tuple)
		}
	}

	return tuples
}

// exampleInstaller is the installer the module samples describe: one release it
// installs and the index it lists.
type exampleInstaller struct {
	installed ModuleInstallation
	offers    []ModuleOffer
	market    ModuleMarketplace
	left      []ModuleDatabaseLeft
	purged    []ModuleDatabasePurged
	plan      ModulePlan
	removal   ModuleRemoval
	why       []ModuleReason
	whyNot    []ModuleReason
	outdated  []ModuleOutdated
}

func (i exampleInstaller) Install(
	context.Context, string, ModuleRelease,
) (ModuleInstallation, error) {
	return i.installed, nil
}

func (i exampleInstaller) Remove(context.Context, string, string, bool) error { return nil }

func (i exampleInstaller) Index(context.Context, string) (ModuleIndex, error) {
	return ModuleIndex{Items: i.offers, Marketplace: &i.market}, nil
}

func (i exampleInstaller) Marketplace(context.Context) (ModuleMarketplace, bool, error) {
	return i.market, true, nil
}

func (i exampleInstaller) LeftDatabases(context.Context, string) ([]ModuleDatabaseLeft, error) {
	return i.left, nil
}

func (i exampleInstaller) PurgeDatabases(context.Context, string, ref.Ref, bool) ([]ModuleDatabasePurged, error) {
	return i.purged, nil
}

func (i exampleInstaller) SetMarketplace(
	_ context.Context,
	enabled bool,
	community *bool,
) (ModuleMarketplace, bool, error) {
	market := i.market
	market.Enabled = enabled

	if community != nil {
		market.Community = *community
	}

	if !enabled {
		market.Reachable, market.ReadAt = false, ""
	}

	return market, true, nil
}

// exampleVerbs is the dangerous tier as the grant samples show it.
type exampleVerbs struct {
	held     []authz.VerbGrant
	accepted []authz.AcceptedChange
	devices  []authz.DeviceRow
	direct   []authz.Tuple
}

func (v exampleVerbs) VerbGrants(context.Context, authz.Subject, authz.Object) ([]authz.VerbGrant, error) {
	return v.held, nil
}

func (v exampleVerbs) WriteVerbGrant(context.Context, authz.Subject, authz.VerbGrant) (bool, error) {
	return true, nil
}

func (v exampleVerbs) WithdrawVerbGrant(context.Context, authz.Subject, authz.VerbGrant) error {
	return nil
}

func (v exampleVerbs) AcceptDeclarationChange(
	context.Context, authz.Subject, string, authz.DeviceKind, string, string,
) error {
	return nil
}

func (v exampleVerbs) Acceptances(context.Context, authz.Subject, authz.DeviceKind) ([]authz.AcceptedChange, error) {
	return v.accepted, nil
}

func (v exampleVerbs) VerbsOfSubject(context.Context, authz.Subject) (map[string][]string, error) {
	return map[string][]string{}, nil
}

func (v exampleVerbs) SubjectDevices(context.Context, authz.Subject, authz.Subject) ([]authz.DeviceRow, error) {
	return v.devices, nil
}

func (v exampleVerbs) SubjectGrants(context.Context, authz.Subject, authz.Subject) ([]authz.Tuple, error) {
	return v.direct, nil
}

func verbGrantsOf(t *testing.T, rows []VerbGrant) []authz.VerbGrant {
	t.Helper()

	grants := make([]authz.VerbGrant, 0, len(rows))

	for _, row := range rows {
		object, err := authz.ParseObject(row.Device)
		require.NoError(t, err)

		subject, err := authz.ParseSubject(row.Subject)
		require.NoError(t, err)

		grant, err := authz.NewVerbGrant(object, row.Verb, subject)
		require.NoError(t, err)

		grants = append(grants, grant)
	}

	return grants
}

func acceptancesOf(rows []AcceptedChange) []authz.AcceptedChange {
	changes := make([]authz.AcceptedChange, 0, len(rows))
	for _, row := range rows {
		changes = append(changes, authz.AcceptedChange{
			Module: row.Module, Kind: authz.DeviceKind(row.Kind), Verb: row.Verb,
			To: row.To, Class: row.Class, Markings: row.Markings,
		})
	}

	return changes
}

func rowsOf(holds []SubjectHold) []authz.DeviceRow {
	rows := make([]authz.DeviceRow, 0, len(holds))

	for _, hold := range holds {
		actions := make([]authz.Action, 0, len(hold.Actions))
		for _, action := range hold.Actions {
			actions = append(actions, authz.Action(action))
		}

		rows = append(rows, authz.DeviceRow{ID: hold.Device, Kind: ref.DeviceKind(hold.Kind), Actions: actions})
	}

	return rows
}

// exampleAccounts answers the local account surface with the account samples.
type exampleAccounts struct {
	info    AuthInfo
	first   Account
	rows    []Account
	created Account
	expires time.Time
	// caller is the account the get-principal sample describes. It answers a
	// Get but is not listed, so the list-accounts sample stays its own.
	caller Account
}

func (a exampleAccounts) Info(context.Context) (AuthInfo, error) { return a.info, nil }

func (a exampleAccounts) Setup(context.Context, NewAccount) (Account, error) { return a.first, nil }

func (a exampleAccounts) Login(context.Context, string, string) (Session, error) {
	return Session{Token: "pcl_sessiontoken", ExpiresAt: a.expires}, nil
}

func (a exampleAccounts) List(context.Context) ([]Account, error) { return a.rows, nil }

// Get answers the row the listing sample holds under this id, so the sample
// path a per-account write takes names a person the same document shows.
func (a exampleAccounts) Get(_ context.Context, id string) (Account, error) {
	for _, row := range append([]Account{a.caller}, a.rows...) {
		if row.ID == id {
			return row, nil
		}
	}

	return Account{}, httpfault.ResourceNotFound(httpfault.Reference("account", ""))
}

func (a exampleAccounts) Create(context.Context, NewAccount) (Account, error) { return a.created, nil }

func (a exampleAccounts) Delete(context.Context, string) error { return nil }

func (a exampleAccounts) ChangePassword(context.Context, string, string, string) (Session, error) {
	return Session{Token: "pcl_example", ExpiresAt: exampleTime.Add(time.Hour)}, nil
}

func (a exampleAccounts) ResetPassword(context.Context, string, string) error { return nil }

// exampleCredentials mints the credential the sample shows and lists the
// caller's own.
type exampleCredentials struct {
	minted createdCredentialBody
	rows   []APICredential
}

func (c exampleCredentials) Issue(context.Context, NewCredential) (IssuedCredential, error) {
	return IssuedCredential{
		APICredential: APICredential{
			ID: c.minted.ID, Name: c.minted.Name, Subject: c.minted.Subject,
			Scopes: c.minted.Scopes, Origin: c.minted.Origin,
			CreatedAt: c.minted.CreatedAt, ExpiresAt: c.minted.ExpiresAt,
		},
		Token: c.minted.Token,
	}, nil
}

// SignedIn says yes, because the sample mint is made from a session.
func (exampleCredentials) SignedIn(*http.Request) bool { return true }

// Fence answers zero, because the sample mint races nothing.
func (exampleCredentials) Fence(context.Context, string, string, string) (CredentialFence, error) {
	return CredentialFence{}, nil
}

// Root answers nothing, so the sample caller is its own root.
func (exampleCredentials) Root(*http.Request) string { return "" }

func (c exampleCredentials) List(context.Context, string) ([]APICredential, error) {
	return c.rows, nil
}

func (c exampleCredentials) Get(context.Context, string) (APICredential, error) {
	return APICredential{}, nil
}

func (c exampleCredentials) Revoke(context.Context, string) error { return nil }

// Presented answers no credential: the samples are sent with a session.
func (exampleCredentials) Presented(*http.Request) string { return "" }

// examplePairings answers the pairing operations with the pairing samples.
type examplePairings struct {
	paired Pairing
	rows   []Pairing
}

func (p examplePairings) Pair(context.Context, NewPairing) (Pairing, error) { return p.paired, nil }

// Fence answers zero, because the sample pairing races nothing.
func (examplePairings) Fence(context.Context, string) (int64, error) { return 0, nil }

func (p examplePairings) List(context.Context, string) ([]Pairing, error) { return p.rows, nil }

func (p examplePairings) Unpair(context.Context, string, string) error { return nil }

// exampleEndpoints answers the endpoint operations with the endpoint sample.
type exampleEndpoints struct {
	items []ConnectorEndpoint
}

func (e exampleEndpoints) ListEndpoints(context.Context, string) (EndpointListing, error) {
	return EndpointListing{Items: e.items, Version: "v7"}, nil
}

func (e exampleEndpoints) ApplyEndpoint(
	context.Context, string, string, EndpointSpec, Precondition,
) (EndpointResult, error) {
	return EndpointResult{Endpoint: exampleConnectorEndpoint()}, nil
}

func (e exampleEndpoints) DeleteEndpoint(context.Context, string, string, Precondition) error {
	return nil
}

// Apply answers a resource write with the instance the samples publish.
func (exampleResourceStore) Apply(
	context.Context, string, ref.Ref, ResourceSpec, Precondition,
) (ApplyResult, error) {
	return ApplyResult{Instance: exampleResource()}, nil
}

// LookupPreset answers a run with the preset the samples publish.
func (examplePresetStore) LookupPreset(context.Context, string) (GCodePreset, error) {
	return exampleGCodePreset(), nil
}

// ApplyPreset answers a preset write with the preset the samples publish.
func (examplePresetStore) ApplyPreset(
	context.Context, string, string, GCodePresetSpec, Precondition,
) (GCodePresetResult, error) {
	return GCodePresetResult{Preset: exampleGCodePreset()}, nil
}

// exampleStores is the server's own storage as the storage samples browse it.
// Save hashes what it was sent, as a real storage does, so the upload sample's
// digest is computed rather than declared.
type exampleStores struct {
	storages []Storage
	files    []StorageEntry
	entry    StorageEntry
}

func (s exampleStores) List(context.Context, string) ([]Storage, error) { return s.storages, nil }

func (s exampleStores) Files(context.Context, string, string, string) ([]StorageEntry, error) {
	return s.files, nil
}

func (s exampleStores) Stat(context.Context, string, string, string) (StorageEntry, error) {
	return s.entry, nil
}

func (s exampleStores) Open(context.Context, string, string, string) (Object, error) {
	return Object{}, noSuchStorageHere()
}

func (s exampleStores) Thumbnail(
	context.Context, string, string, string,
) (StorageThumbnail, error) {
	return StorageThumbnail{}, noSuchStorageHere()
}

func (s exampleStores) FileVersion(
	context.Context, string, string, string,
) (FileVersion, error) {
	return FileVersion{}, noSuchStorageHere()
}

func (s exampleStores) FileExtent(
	context.Context, string, string, string,
) (FileExtent, error) {
	return FileExtent{}, noSuchStorageHere()
}

func (s exampleStores) Save(
	_ context.Context, _ string, _ string, path string, body io.Reader,
) (SaveResult, error) {
	sent, err := io.ReadAll(body)
	if err != nil {
		return SaveResult{}, err //nolint:wrapcheck // the reader's own failure is what the route maps
	}

	return SaveResult{Path: path, Size: int64(len(sent)), SHA256: digestOf(string(sent))}, nil
}

// SaveTemporary answers as the server does: the copy lands in a folder of its
// own under the temporary area.
func (s exampleStores) SaveTemporary(
	ctx context.Context, principal string, storage string, name string, body io.Reader,
) (SaveResult, error) {
	saved, err := s.Save(ctx, principal, storage, ".pcl-incoming/1761300000-example/"+name, body)
	saved.Temporary = err == nil

	return saved, err
}

func (s exampleStores) Delete(context.Context, string, string, string) error { return nil }

func (s exampleStores) DeleteMany(context.Context, string, string, []string) error { return nil }

func (s exampleStores) DownloadURL(context.Context, string, string, string) (string, bool, error) {
	return "", false, nil
}

func (s exampleStores) CreateFolder(context.Context, string, string, string) error { return nil }

func (s exampleStores) Move(context.Context, string, string, string, string) error { return nil }

// exampleHistory is the stored past the history samples read, and the
// command record beside it.
type exampleHistory struct{}

func (exampleHistory) Facts(context.Context, string, HistoryFactQuery) (HistoryFactPage, error) {
	return exampleFactPage(), nil
}

func (exampleHistory) Samples(context.Context, string, HistorySampleQuery) (HistorySamplePage, error) {
	return exampleSamplePage(), nil
}

func (exampleHistory) Commands(context.Context, string, HistoryCommandQuery) (HistoryCommandPage, error) {
	return exampleCommandPage(), nil
}

// exampleConnectors is the connector rollup the health sample shows.
type exampleConnectors struct {
	rollups []ConnectorRollup
}

func (c exampleConnectors) Connectors(context.Context, string) ([]ConnectorRollup, error) {
	return c.rollups, nil
}

// exampleStateStore holds the namespaces the state sample counts.
type exampleStateStore struct {
	items []StateNamespace
}

func (s exampleStateStore) Namespaces(context.Context) ([]StateNamespace, error) {
	return s.items, nil
}

// exampleModuleState is one person's settings under one module, as the four
// settings samples read and write them. A write records nothing: the handler
// answers with what it was sent, which is what the round trip compares.
type exampleModuleState struct {
	keys   []string
	record ModuleStateRecord
}

func (s exampleModuleState) Keys(context.Context, string, string) ([]string, error) {
	return s.keys, nil
}

func (s exampleModuleState) Value(_ context.Context, _, _, key string) ([]byte, error) {
	if key != s.record.Key {
		return nil, ErrNoStateRecord
	}

	return s.record.Value, nil
}

func (s exampleModuleState) Write(context.Context, string, string, string, []byte) error {
	return nil
}

func (s exampleModuleState) Remove(context.Context, string, string, string) error { return nil }

// exampleModules is the module plane the state and OpenAPI samples describe:
// each namespace in the state sample claimed by the instance the row names,
// and the example namespace carrying the module document sample.
func exampleModules(t *testing.T) moduleTable {
	t.Helper()

	document, err := json.Marshal(sampled[map[string]any](t, "get-module-openapi", ""))
	require.NoError(t, err)

	plane := moduleTable{}

	for _, row := range sampled[[]StateNamespace](t, "list-state-namespaces", "items") {
		instance, parseErr := ref.Parse(row.Instance)
		require.NoError(t, parseErr)

		surface := ModuleSurface{Instance: instance, Handler: http.NotFoundHandler()}
		if row.Namespace == exampleNamespace {
			surface.OpenAPI = document
		}

		plane[row.Namespace] = surface
	}

	return plane
}

// moduleTable is a module plane keyed by namespace, mounted at no root.
type moduleTable map[string]ModuleSurface

func (m moduleTable) Module(namespace string) (ModuleSurface, bool) {
	surface, claimed := m[namespace]

	return surface, claimed
}

func (moduleTable) Root(string) (ModuleSurface, bool) { return ModuleSurface{}, false }

// exampleModuleAccess permits every module read, for the same reason
// exampleGrants permits every decision.
type exampleModuleAccess struct{}

func (exampleModuleAccess) RequireResource(context.Context, authz.Subject, authz.Action, ref.Ref) error {
	return nil
}

// exampleEvents is a hub serving as many readers as the stats sample counts.
type exampleEvents struct {
	noEvents

	count int
}

func (e exampleEvents) Subscribers() int { return e.count }

// mountsOf is the origin's non-operation paths as the developer inventory
// sample lists them.
func mountsOf(routes []devtools.Route) []Mount {
	mounts := make([]Mount, 0, len(routes))
	for _, route := range routes {
		mounts = append(mounts, Mount{Pattern: route.Pattern, Handler: http.NotFoundHandler(), Reason: route.Reason})
	}

	return mounts
}

// callerOf reads the account a get-principal sample describes.
func callerOf(sample map[string]any) Account {
	subject, _ := sample["principal"].(string)
	username, _ := sample["username"].(string)
	name, _ := sample["display_name"].(string)

	return Account{ID: strings.TrimPrefix(subject, "user:"), Username: username, DisplayName: name}
}

// exampleAppPairings answers the app pairing operations with their samples.
type exampleAppPairings struct {
	pairing   AppPairing
	collected collectedCredentialBody
	issued    issuedAppPairingBody
}

func (p exampleAppPairings) Request(context.Context, AppPairingRequest) (IssuedAppPairing, error) {
	return IssuedAppPairing{
		AppPairing: AppPairing{UserCode: p.issued.UserCode},
		DeviceCode: p.issued.DeviceCode,
		Interval:   time.Duration(p.issued.Interval) * time.Second,
		Lifetime:   time.Duration(p.issued.ExpiresIn) * time.Second,
	}, nil
}

func (p exampleAppPairings) Lookup(context.Context, string) (AppPairing, error) {
	return p.pairing, nil
}

func (exampleAppPairings) Decide(context.Context, AppPairingDecision) error { return nil }

func (p exampleAppPairings) Collect(context.Context, string) (IssuedCredential, error) {
	return IssuedCredential{
		APICredential: APICredential{
			ID: p.collected.ID, Name: p.collected.Name, Subject: p.collected.Subject,
			CreatedAt: p.collected.CreatedAt, ExpiresAt: p.collected.ExpiresAt,
			Scopes: p.collected.Scopes, Origin: p.collected.Origin,
		},
		Token:       p.collected.Token,
		AccessToken: p.collected.AccessToken,
	}, nil
}

func (exampleAppPairings) OpenPageSession(context.Context, string, func(string) bool) (PageSession, error) {
	return PageSession{Token: exampleCredentialToken, ExpiresAt: exampleTime.Add(time.Hour)}, nil
}

func (i exampleInstaller) Plan(context.Context, string, ModuleReleaseRef) (ModulePlan, error) {
	return i.plan, nil
}

func (i exampleInstaller) Removal(context.Context, string, string) (ModuleRemoval, error) {
	return i.removal, nil
}

func (i exampleInstaller) Why(context.Context, string, string) ([]ModuleReason, error) {
	return i.why, nil
}

func (i exampleInstaller) WhyNot(context.Context, string, string, string) ([]ModuleReason, error) {
	return i.whyNot, nil
}

func (i exampleInstaller) Outdated(context.Context, string) ([]ModuleOutdated, error) {
	return i.outdated, nil
}

func (exampleResourceStore) MemoryHistory(
	context.Context, string, ref.Ref, time.Time,
) (ResourceMemoryHistory, error) {
	return operationExamples()["get-resource-memory"].Response.(ResourceMemoryHistory), nil
}

func (exampleResourceStore) Log(context.Context, string, ref.Ref) (ResourceLog, error) {
	return operationExamples()["get-resource-log"].Response.(ResourceLog), nil
}

func (exampleResourceStore) ModuleMemory() ModuleMemoryPool {
	return ModuleMemoryPool{PoolBytes: new(int64(1 << 30)), CommittedBytes: 384 << 20, Source: "detected"}
}
