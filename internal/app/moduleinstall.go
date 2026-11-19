package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/betaportal"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/fault"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// Installing a module over HTTP. It is the `module add` and
// `module remove` commands through the same functions, so the two paths cannot
// check different things, plus the index a marketplace page lists from.

// moduleIndexName is the file the index is read from, inside the modules
// directory. An operator writes it or copies it there. A deployment with no
// such file offers nothing.
const moduleIndexName = "index.json"

// The bound one index is read under. An index is a list of rows, and anything
// claiming otherwise is refused rather than held in memory.
const maxIndexBytes = 4 << 20

// moduleGraph is the authorization graph as the installer asks it. It is the
// one question this type asks: may this caller configure that resource kind.
type moduleGraph interface {
	Can(ctx context.Context, who authz.Subject, action authz.Action, object authz.Object) (bool, error)
}

// runningModules is the supervision plane as the installer asks it: stop one
// instance and wait for its process to exit, and say whether it is supervising
// one under a given name.
//
// The stop is what an uninstall needs, because a module's process has to be
// gone before its files can be deleted. The second answer is what an install
// reports back, because a declaration that was written and an instance that is
// running are two different facts and the caller asked about the second.
type runningModules interface {
	Stop(instance ref.Ref) bool
	// Restart rebuilds a running instance from its declaration.
	Restart(instance ref.Ref) bool
	Status(instance ref.Ref) (httpapi.ResourceStatus, bool)
	// Database is what a running module reported about its own database
	// when it started: the schema version and the migrations it applied.
	Database(instance ref.Ref) (wirerpc.DatabaseState, bool)
}

// declaredResources is the configured inventory as the installer asks it: read
// both files again and converge the supervision plane on what they say.
//
// An install writes the overlay directly, the way `module add` does, so the
// store has not seen the declaration it produced. This call makes the module
// run at once instead of at the next start. The startup path makes the same
// call over the same document.
//
// Nil is a caller with no store in this process, which is a test. The
// declaration is then on disk and the next start reads it, which is what the
// answer's restart flag says.
type declaredResources interface {
	Reload(ctx context.Context) error
}

// moduleInstaller installs a module for the API. It holds the same two files
// the commands hold, the operator's and the machine-owned overlay, the graph
// that decides who may write the second one, the plane that is running what it
// installs and removes, and the store that reconciles the change.
type moduleInstaller struct {
	core       config.Core
	configPath string
	graph      moduleGraph
	plane      runningModules
	configured declaredResources
	log        *slog.Logger
	client     *http.Client
	// writer is the authorization graph an approved install writes the
	// module's grants into. Nil is a caller with no graph in this process,
	// which is a test, and the install then writes no grant.
	//
	// An install writes as the caller. The caller may hold configure_resource
	// on one resource kind and nothing on the organization, and a grant written
	// for them as the configuration's principal would hand their module more
	// than they hold themselves.
	writer grantWriter
	// market is the marketplace the index reads and an install with no
	// address resolves against. Nil is a deployment that reads none.
	market *marketplaceCatalogue
	// portal scopes the shared entitlement credential to the configured Beta
	// Portal origin for marketplace catalog and artifact requests.
	portal portalAuthorization
	// writing makes one install or removal at a time touch the modules
	// directory, the overlay and the plane. Two at once would race on the same
	// files: one copying a release into a folder the other is deleting. A
	// download happens before it is taken, so a slow one blocks nobody.
	writing sync.Mutex
	// toggle keeps whether a person switched the marketplace off. Nil is a
	// caller with no machine-owned file, which is a test, and then a
	// configured marketplace is always read.
	toggle marketplaceSwitch
	// grants takes a removed module's own grants out of the authorization
	// graph. Nil is a caller with no graph in this process, which is a test.
	//
	// It writes as the principal the configuration names and not as the caller,
	// and that is right for a withdrawal where it would not be for a write.
	// Taking a grant away confers nothing on anybody, so it widens nothing the
	// caller reaches, and the grant it takes away is one this server wrote
	// itself when the module was installed.
	grants *moduleAuthority
	// settle is how long an install waits for the module it switched on to
	// finish starting before it answers. Zero is startSettle.
	settle time.Duration
}

// newModuleInstaller builds the installer the composition root wires. The
// client is this type's own: a release fetch is a download of somebody else's
// bytes, and it must not share a client with anything the core dials.
//
// A nil plane is a caller that supervises nothing, which is a test. An
// uninstall then deletes the files of a module no process is holding. A nil
// writer is a caller with no authorization graph, and neither an install nor a
// removal then touches a grant.
func newModuleInstaller(
	core config.Core, configPath string, graph moduleGraph,
	plane runningModules, configured declaredResources, writer grantWriter, log *slog.Logger,
	entitlements ...*betaportal.Store,
) *moduleInstaller {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}

	var grants *moduleAuthority
	if writer != nil {
		grants = removalAuthority(core, writer, log)
	}

	portal := newPortalAuthorization(core.Upgrade.Server, firstEntitlement(entitlements))

	installer := &moduleInstaller{
		core:       core,
		configPath: configPath,
		graph:      graph,
		plane:      plane,
		configured: configured,
		writer:     writer,
		grants:     grants,
		log:        log,
		client:     downloadClient(core.Marketplace),
		market:     newMarketplaceCatalogue(core.Marketplace, log, portal),
		portal:     portal,
	}
	installer.client.CheckRedirect = portal.redirects

	// The store the composition root hands in also keeps the machine-owned
	// file, and that is where the marketplace switch is stored.
	if toggle, ok := configured.(marketplaceSwitch); ok {
		installer.toggle = toggle
	}

	return installer
}

// Install is httpapi.ModuleInstaller. It runs exactly what `module add` runs,
// with one check added: the graph decides whether this caller may declare an
// instance of the kind the manifest names, before anything is written.
//
// The grants an approved install writes are the ones the command writes, through
// the same plan and the same writes. Without them a module switched on here
// would reach no printer.
func (m *moduleInstaller) Install(
	ctx context.Context, principal string, release httpapi.ModuleRelease,
) (httpapi.ModuleInstallation, error) {
	if err := modulesRefused(m.core); err != nil {
		return httpapi.ModuleInstallation{}, err
	}

	who, err := authz.ParseSubject(principal)
	if err != nil {
		return httpapi.ModuleInstallation{}, err
	}

	source, err := m.addressOf(ctx, release)
	if err != nil {
		return httpapi.ModuleInstallation{}, err
	}

	from, done, err := m.reachable(ctx, source)
	if err != nil {
		return httpapi.ModuleInstallation{}, err
	}

	defer done()

	m.writing.Lock()
	defer m.writing.Unlock()

	if err = lockWritable(m.core); err != nil {
		return httpapi.ModuleInstallation{}, httpfault.ModuleLockNotWritten(
			strings.TrimSpace(release.Module),
			false,
			err,
		)
	}

	operation := moduleInstall{
		request: moduleRequest{
			source:        strings.TrimSpace(release.Module) + versionSeparator + strings.TrimSpace(release.Version),
			from:          from,
			approve:       release.Approve,
			given:         json.RawMessage(release.Settings),
			recordsMiB:    release.RecordsMiB,
			memoryMiB:     release.MemoryMiB,
			approveListen: release.ApproveListen,
			listen:        listenMoves(release.Listen),
			database:      databaseChoiceOf(release.Database),
			shown: map[string]*install.ModuleAsks{
				shownByMarketplace: source.listed,
				shownByRequest:     expectedAsks(release.Expect),
			},
			channel: source.channel,
			address: source.address,
			digest:  source.digest,
			// A page installs one release, which composer require records
			// as the caret of it.
			constraint: caretOf(release.Version),
		},
		mayDeclare: func(ctx context.Context, kind ref.Kind) error {
			return m.mayConfigure(ctx, who, kind)
		},
		grants:       m.grantsOf(who),
		replacing:    func(instance ref.Ref) { m.stopInstance(instance) },
		keepPrevious: true,
	}
	operation.dependencies = m.dependenciesFor(release.Dependencies, true, operation)

	held, err := installDeclared(ctx, m.core, m.configPath, operation)
	if err != nil {
		return httpapi.ModuleInstallation{}, installFault(strings.TrimSpace(release.Module), err)
	}

	if err = recordLock(m.core, m.configPath); err != nil {
		return httpapi.ModuleInstallation{}, httpfault.ModuleLockNotWritten(
			strings.TrimSpace(release.Module),
			true,
			err,
		)
	}

	// After the declaration is written and before the answer is rendered, so
	// the facts the answer carries about this server are read from the plane
	// that has just acted on it.
	restart := m.converge(ctx, "install")
	m.reopenSameRelease(held)
	started := m.startOf(ctx, held.instance, held.enabled, restart, m.settleFor(held))

	if err = m.settleUpdate(ctx, who, strings.TrimSpace(release.Module), held, started); err != nil {
		return httpapi.ModuleInstallation{}, err
	}

	schema, migrations := m.databaseOf(held.instance, started)

	return httpapi.ModuleInstallation{
		Instance:  held.instance.String(),
		Type:      held.typeName,
		Namespace: held.namespace,
		Kind:      held.instance.Kind().String(),
		Version:   held.version,
		Runtime:   held.runtime,
		Root:      filepath.ToSlash(held.root),
		Enabled:   held.enabled,
		Verified:  held.checked,
		Approved:  held.approved,
		Withheld:  held.withheld,
		Running:   started.running,
		State:     started.state,
		Reason:    started.reason,
		Restart:   restart,
		Requests:  held.requests,
		Added:     held.added,

		Dependencies: held.dependencies,

		SchemaVersion: schema,
		Migrations:    migrations,
		DatabaseLeft:  held.databaseLeft,
	}, nil
}

// Remove is httpapi.ModuleInstaller. It resolves the namespace to the one
// declaration that carries it, stops the instance this core is running for it,
// and takes its files and then its declaration away.
//
// The stop is the half the command cannot do: the supervision plane is in this
// process, the module's process is its child, and a running child holds its own
// executable open.
func (m *moduleInstaller) Remove(ctx context.Context, principal string, namespace string, withUnused bool) error {
	who, err := authz.ParseSubject(principal)
	if err != nil {
		return err
	}

	m.writing.Lock()
	defer m.writing.Unlock()

	found, err := m.declarationOf(namespace)
	if err != nil {
		return err
	}

	if err = m.mayConfigure(ctx, who, found.Ref.Kind()); err != nil {
		return err
	}

	// The same grants the command takes back, taken back here too. This is the
	// route a marketplace page removes a module through, and a module removed
	// here whose principal kept the fleet would be a grant nobody can see,
	// written against a name nothing declares any more.
	plan := declaredGrants(found)

	if err = lockWritable(m.core); err != nil {
		return httpfault.ModuleLockNotWritten(namespace, false, err)
	}

	if _, err = removeWithUnused(ctx, moduleRemoval{
		core: m.core, configPath: m.configPath, found: found, stop: m.stopInstance,
		grants: m.grants, plan: plan, withUnused: withUnused,
	}); err != nil {
		return installFault(namespace, err)
	}

	locked := recordLock(m.core, m.configPath)

	// The process is already stopped. This is what takes the instance out of
	// the document every reader joins against, so the resources listing and the
	// readiness report drop it in this request rather than at the next start.
	// It runs whether or not the lock was written, because the removal stands.
	m.converge(ctx, "uninstall")

	if locked != nil {
		return httpfault.ModuleLockNotWritten(namespace, true, locked)
	}

	return nil
}

// declarationOf is the one declaration served under a namespace, and a 404 when
// nothing is.
func (m *moduleInstaller) declarationOf(namespace string) (resource.Declaration, error) {
	rows, err := resource.Installed(resource.Paths{File: m.configPath, Overlay: m.core.Overlay})
	if err != nil {
		return resource.Declaration{}, installFault(namespace, err)
	}

	found, declared := declarationOfNamespace(rows, namespace)
	if !declared {
		return resource.Declaration{}, httpfault.RouteNotFound(http.MethodDelete, "/api/v1/modules/"+namespace)
	}

	return found, nil
}

// stopInstance ends the running instance and waits for its process to exit,
// and reports false when this build supervises nothing at all or is running
// nothing under that name.
func (m *moduleInstaller) stopInstance(instance ref.Ref) bool {
	if m.plane == nil {
		return false
	}

	return m.plane.Stop(instance)
}

// reopenSameRelease rebuilds the instance an install of its own version
// replaced. The declaration is unchanged, so the store plans nothing, and the
// running instance would keep serving from the directory this install deleted:
// its browser bundle answered 404 until the next start.
func (m *moduleInstaller) reopenSameRelease(held moduleReport) {
	declared := held.replaced
	if m.plane == nil || declared.Ref.IsZero() || declared.Spec.Module == nil ||
		declared.Spec.Module.Version != held.version {
		return
	}

	m.plane.Restart(held.instance)
}

// converge makes the overlay this call has just written take effect, and
// reports whether a restart of this server is still needed to pick it up.
//
// A failure is logged here and nowhere else. The declaration is on disk and the
// install or the removal happened, so there is nothing to undo, and a returned
// error would say the write failed when it did not. The flag tells the caller
// that the change waits on a restart.
func (m *moduleInstaller) converge(ctx context.Context, op string) bool {
	if m.configured == nil {
		return true
	}

	if err := m.configured.Reload(ctx); err != nil {
		m.log.ErrorContext(ctx, "a module was written to the overlay and this server did not converge on it",
			"operation", op, "error", err.Error())

		return true
	}

	return false
}

// grantsOf is how an install opens the graph it writes the module's grants
// into, as the caller. It is nil when this installer holds no graph.
func (m *moduleInstaller) grantsOf(who authz.Subject) func() (*moduleAuthority, func(), error) {
	if m.writer == nil {
		return nil
	}

	return func() (*moduleAuthority, func(), error) {
		return &moduleAuthority{graph: m.writer, who: who}, func() {}, nil
	}
}

// updateSettle bounds how long an update waits for the new release to start.
// It is longer than startSettle, because the new release may migrate its own
// database before it reports ready, and the update is kept or taken back by
// what the wait sees.
const updateSettle = time.Minute

// settleFor is how long an install waits for the module to start. A test sets
// settle, and then every install waits that long.
func (m *moduleInstaller) settleFor(held moduleReport) time.Duration {
	switch {
	case m.settle > 0:
		return m.settle
	case held.previous != "":
		return updateSettle
	}

	return startSettle
}

// startSettle bounds how long an install waits for the module to finish
// starting. A module that runs a process launches it, hands it its settings
// and reads its routes in that time. One that takes longer is answered as
// starting, and list-resources carries the rest.
const startSettle = 5 * time.Second

// startPoll is how often the wait reads the plane again.
const startPoll = 25 * time.Millisecond

// The states an install answers. They are the plane's own words, so a page
// reads them the way it reads the resources listing.
const (
	moduleStateDisabled = "disabled"
	moduleStatePending  = "pending"
	moduleStateStarting = "starting"
	moduleStateFailed   = "failed"
	moduleStateRetrying = "retrying"
)

// moduleStart is what the plane says about an instance an install has just
// converged on.
type moduleStart struct {
	state   string
	reason  string
	running bool
}

// startOf waits for the instance the install switched on to leave pending and
// starting, and answers the state it reached.
//
// The start runs on the plane's own goroutine, so right after the converge the
// instance is still on its way up. An answer read then says nothing about
// whether the module works: a module that refuses its settings reaches failed
// a moment later.
func (m *moduleInstaller) startOf(
	ctx context.Context, instance ref.Ref, enabled, restart bool, settle time.Duration,
) moduleStart {
	switch {
	case !enabled:
		return moduleStart{state: moduleStateDisabled}
	case restart || m.plane == nil:
		return moduleStart{state: moduleStatePending}
	}

	deadline := time.NewTimer(settle)
	defer deadline.Stop()

	ticker := time.NewTicker(startPoll)
	defer ticker.Stop()

	for {
		status, found := m.plane.Status(instance)
		if found && !stillStarting(status.State) {
			return startFrom(status)
		}

		select {
		case <-ctx.Done():
			return stillOnItsWay(status, found)
		case <-deadline.C:
			return stillOnItsWay(status, found)
		case <-ticker.C:
		}
	}
}

// stillStarting reports whether the plane has not finished starting the
// instance yet.
func stillStarting(state string) bool {
	return state == moduleStatePending || state == moduleStateStarting || state == ""
}

// stillOnItsWay is the answer for an instance the wait stopped reading before
// the plane finished starting it.
func stillOnItsWay(status httpapi.ResourceStatus, found bool) moduleStart {
	if !found || status.State == "" {
		return moduleStart{state: moduleStatePending}
	}

	return moduleStart{state: status.State}
}

// startFrom is the answer for an instance the plane has finished starting,
// with the reason when it did not come up.
func startFrom(status httpapi.ResourceStatus) moduleStart {
	out := moduleStart{state: status.State, running: status.Serving}

	if status.State != moduleStateFailed && status.State != moduleStateRetrying {
		return out
	}

	out.reason = status.Result

	for _, held := range status.Conditions {
		if held.Status == "False" && held.Message != "" {
			out.reason = held.Message

			break
		}
	}

	return out
}

// Index is httpapi.ModuleInstaller. It reads the index file the modules
// directory holds, then the marketplace's catalogue, and marks the rows this
// deployment already declares with the version it declares.
func (m *moduleInstaller) Index(ctx context.Context, _ string) (httpapi.ModuleIndex, error) {
	if err := modulesRefused(m.core); err != nil {
		return httpapi.ModuleIndex{}, err
	}

	offers, err := readModuleIndex(filepath.Join(m.core.Modules, moduleIndexName))
	if err != nil {
		return httpapi.ModuleIndex{}, err
	}

	offers = m.localModuleOffers(ctx, offers)

	on, err := m.marketplaceOn()
	if err != nil {
		return httpapi.ModuleIndex{}, err
	}

	// A marketplace a person switched off is not read at all, so it costs no
	// request and its rows are not listed.
	var remote []marketplaceModule
	if on {
		remote = m.market.modules(ctx)
	}

	offers = m.market.mergeOffers(offers, remote)

	market, err := m.indexMarketplace(on)
	if err != nil {
		return httpapi.ModuleIndex{}, err
	}

	rows, err := resource.Installed(resource.Paths{File: m.configPath, Overlay: m.core.Overlay})
	if err != nil {
		return httpapi.ModuleIndex{}, installFault("", err)
	}

	listed := make([]httpapi.ModuleOffer, 0, len(offers))

	for _, offer := range offers {
		namespace, derived := extension.NamespaceOf(offer.Module)
		if derived == nil {
			offer.Namespace = string(namespace)

			declared, found := declarationOfNamespace(rows, string(namespace))
			if found && betaOverStable(offer, declared) {
				continue
			}

			if found {
				offer.Installed = true
				offer.Instance = declared.Ref.String()

				if declared.Spec.Module != nil {
					offer.InstalledVersion = declared.Spec.Module.Version
				}
			}
		}

		listed = append(listed, offer)
	}

	return httpapi.ModuleIndex{Items: listed, Marketplace: market}, nil
}

// mayConfigure asks the graph whether this caller administers one resource
// kind's configuration, which is what declaring an instance of it is.
func (m *moduleInstaller) mayConfigure(ctx context.Context, who authz.Subject, kind ref.Kind) error {
	object, err := authz.NewObject(authz.TypeResourceKind, kind.String())
	if err != nil {
		return httpfault.Unexpected("name the " + kind.String() + " resource kind")
	}

	allowed, err := m.graph.Can(ctx, who, authz.ActionConfigureResource, object)
	if err != nil {
		return httpfault.Unexpected("ask the authorization graph who may install a module")
	}

	if !allowed {
		return fault.New(authz.CodeAccessDenied(),
			"this caller may not declare an instance of the "+kind.String()+" resource kind").With(
			fault.StringParam("object", object.String()),
			fault.StringParam("action", authz.ActionConfigureResource.String()))
	}

	return nil
}

// declarationOfNamespace is the one declaration served under a namespace, and
// false when nothing is.
func declarationOfNamespace(
	rows []resource.Declaration, namespace string,
) (resource.Declaration, bool) {
	for _, row := range rows {
		if held, module := row.ModuleNamespace(); module && held == namespace {
			return row, true
		}
	}

	return resource.Declaration{}, false
}

// moduleIndexFile is the document an index is written as: one row per module,
// under a key that says what the list is.
type moduleIndexFile struct {
	Modules []moduleIndexRow `json:"modules"`
}

// moduleIndexRow is one offer as the file spells it. The request keys are
// pointers because a row that states none is a row whose requests are unknown,
// and one that states an empty list asks for nothing.
type moduleIndexRow struct {
	// The file types the release claims.
	fileTypeClaims

	Module            string               `json:"module"`
	Name              string               `json:"name"`
	Description       string               `json:"description"`
	Version           string               `json:"version"`
	Source            string               `json:"source"`
	Kind              string               `json:"kind"`
	Author            string               `json:"author"`
	Permissions       *[]string            `json:"permissions,omitempty"`
	Checks            *[]string            `json:"checks,omitempty"`
	Events            *map[string]string   `json:"events,omitempty"`
	CheckAbsentPeople *bool                `json:"check_absent_people,omitempty"`
	ListPeople        *bool                `json:"list_people,omitempty"`
	Calls             *[]string            `json:"calls,omitempty"`
	RecordsMiB        *int                 `json:"records_mib,omitempty"`
	MemoryMiB         *int                 `json:"memory_mib,omitempty"`
	Listen            *[]install.ListenAsk `json:"listen,omitempty"`
	Data              *bool                `json:"data,omitempty"`
	Database          *bool                `json:"database,omitempty"`

	// The release's require, suggest and conflict, in Composer's shape.
	Require  map[string]string `json:"require,omitempty"`
	Suggest  map[string]string `json:"suggest,omitempty"`
	Conflict map[string]string `json:"conflict,omitempty"`
}

// manifestIn reads the manifest of a release directory.
func manifestIn(dir string) (manifest.Manifest, error) {
	document, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return manifest.Manifest{}, err
	}

	if len(document) > maxIndexBytes {
		return manifest.Manifest{}, fmt.Errorf("%s is larger than this server reads", manifestName)
	}

	return manifest.Parse(manifestName, document, yaml.Unmarshal)
}

// localModuleOffers adds one offer per release directory the operator named
// in core.modules_local, in module path order. A module an earlier row offers
// keeps that row, and a directory that cannot be read is skipped, so one bad
// entry never empties the listing.
func (m *moduleInstaller) localModuleOffers(
	ctx context.Context, listed []httpapi.ModuleOffer,
) []httpapi.ModuleOffer {
	named := make(map[string]bool, len(listed))
	for _, offer := range listed {
		named[offer.Module] = true
	}

	for _, module := range slices.Sorted(maps.Keys(m.core.ModulesLocal)) {
		if named[module] {
			continue
		}

		from := strings.TrimSpace(m.core.ModulesLocal[module])

		declared, err := manifestIn(from)
		if err != nil {
			m.log.WarnContext(ctx, "a local module release cannot be read",
				"module", module, "source", from, "reason", err.Error())

			continue
		}

		offer := httpapi.ModuleOffer{
			Module:      module,
			Name:        declared.Name,
			Title:       declared.Title,
			Description: declared.Summary,
			Version:     declared.Version,
			Source:      from,
			Kind:        declared.Kind,
			Author:      declared.Codeowner,
			Process:     declared.ContractMajor > 0,
		}
		withAsks(&offer, asksOf(declared))
		withManifest(&offer, declared)
		listed = append(listed, offer)
	}

	return listed
}

// localManifest is the manifest of a release directory on this server, and
// false for a source that is not one.
func localManifest(source string) (manifest.Manifest, bool) {
	source = strings.TrimSpace(source)
	if source == "" || strings.Contains(source, schemeSeparator) {
		return manifest.Manifest{}, false
	}

	declared, err := manifestIn(source)
	if err != nil {
		return manifest.Manifest{}, false
	}

	return declared, true
}

// readModuleIndex reads the index file, and answers an empty list for a
// deployment that has none.
//
// A file that is not there is not a failure. A server with nothing to offer is
// the ordinary case, and a marketplace page that had to handle an error to show
// an empty screen would show the error instead.
func readModuleIndex(path string) ([]httpapi.ModuleOffer, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []httpapi.ModuleOffer{}, nil
	}

	if err != nil {
		return nil, httpfault.Unexpected("read the module index")
	}

	if len(body) > maxIndexBytes {
		return nil, httpfault.Unexpected("read the module index, which is larger than this server reads")
	}

	var document moduleIndexFile

	if err = json.Unmarshal(body, &document); err != nil {
		return nil, httpfault.Unexpected("read the module index, which is not the document this server reads")
	}

	offers := make([]httpapi.ModuleOffer, 0, len(document.Modules))

	for _, row := range document.Modules {
		if strings.TrimSpace(row.Module) == "" {
			continue
		}

		offer := httpapi.ModuleOffer{
			Module:      row.Module,
			Name:        row.Name,
			Description: row.Description,
			Version:     row.Version,
			Source:      row.Source,
			Kind:        row.Kind,
			Author:      row.Author,
		}

		requestsOf(&offer, row)
		dependenciesOf(&offer, row)

		offers = append(offers, offer)
	}

	return offers, nil
}

// installFault renders what the install machinery reported. A fault it raised
// is kept as it stands; a usage error is the caller's request and is answered
// as one; everything else is the caller naming something this server could not
// read, which is still their request and not this server failing.
func installFault(module string, err error) error {
	if raised, ok := errors.AsType[*fault.Fault](err); ok {
		return raised
	}

	if changed, ok := errors.AsType[requestsChangedError](err); ok {
		return httpfault.ModuleRequestsChanged(module, changed.shown, changed.beyond)
	}

	if taken, ok := errors.AsType[nameTakenError](err); ok {
		return httpfault.ModuleNameTaken(taken.instance.String(), taken.held, taken.wanted)
	}

	if errors.Is(err, ErrUsage) {
		return httpfault.RequestInvalid("body", "the install request", err.Error())
	}

	if notMade, ok := errors.AsType[databaseNotMadeError](err); ok {
		return httpfault.ModuleDatabaseNotMade(module, notMade.name, notMade.reason.Error())
	}

	if refused, ok := errors.AsType[settingsError](err); ok {
		return httpfault.RequestInvalid("body.settings", "the module's settings", refused.Error())
	}

	if written, ok := errors.AsType[installWriteError](err); ok {
		return httpfault.ModuleInstallFailed(written.dir)
	}

	if declared, ok := errors.AsType[handDeclaredError](err); ok {
		return httpfault.ResourceNotWritable(
			declared.instance.String(), string(resource.SourceFile), declared.file)
	}

	return httpfault.RequestInvalid("body.from", "the release", err.Error())
}

// caretOf is the constraint an install of one release records, ^ on it, and
// the constraint itself when it is one already.
func caretOf(version string) string {
	version = strings.TrimSpace(version)

	if _, err := manifest.ParseRelease(version); err == nil {
		return "^" + strings.TrimPrefix(version, "v")
	}

	return version
}
