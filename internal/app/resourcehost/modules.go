package resourcehost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/app/resource"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/extension/supervisor"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
	"github.com/prusa3d/prusa-connect/sdk/accessapi"
	"github.com/prusa3d/prusa-connect/sdk/busapi"
	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
	"github.com/prusa3d/prusa-connect/sdk/deviceapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/fault"
	"github.com/prusa3d/prusa-connect/sdk/linkapi"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
	"github.com/prusa3d/prusa-connect/sdk/storageapi"
	"github.com/prusa3d/prusa-connect/sdk/uiapi"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

// ErrNamespaceClaimed reports a second instance declaring a module under a
// namespace another instance already holds.
//
// One namespace is one mount, so a second claim is refused. With two modules
// under one prefix, the module a caller reaches would depend on which instance
// started first, and an operator who installed a module could find another
// module's routes at its address.
//
// Only this instance is refused. It fails and reports why, and every other
// instance in the deployment keeps running.
var ErrNamespaceClaimed = errors.New("resourcehost: the module namespace is already claimed")

// moduleNamespaceOf is the namespace one declaration's module surface is served
// under, and false for a declaration that is not a module. The declaration
// derives it, so the listing and this mount agree on it.
func moduleNamespaceOf(declared resource.Declaration) (string, bool) {
	return declared.ModuleNamespace()
}

// ErrNameClaimed reports an instance declaring a name another kind is running
// under, where both read the authorization graph as a principal built from
// that name.
//
// The principal of an instance is built from the instance name alone, so
// `sink: {audit: ...}` and `link: {audit: ...}` would be one principal with
// one set of grants: every grant an operator wrote for the sink would also be
// a grant of the link. The configuration keeps instance names per kind, so
// the configuration check does not refuse the pair.
//
// Only this instance is refused, as with ErrNamespaceClaimed. It fails and
// reports why, and every other instance in the deployment keeps running.
//
// It limits what runs and not what is granted. A grant is written for a name
// and outlives the instance, so when a sink that was granted a name is
// switched off, the next instance started under that name holds those grants.
// Only a check where the pair is declared, in internal/app/resource, would
// prevent that.
//
// A plan starts before it stops. Moving a name from one kind to another in
// one write therefore refuses the start and then stops the holder, and the
// operator writes the configuration again to get the new instance running.
// The refusal says so.
var ErrNameClaimed = errors.New("resourcehost: another kind is running under this instance name")

// holdsAGraphPrincipal reports whether an instance reads the authorization
// graph as a principal built from its own name. A sink does, because the
// events a sink may receive are granted to that principal. A module running in
// a process of its own does too, because a call it makes as itself is resolved
// against that principal.
//
// An instance whose runtime is asset does not. It is a bundle of files the
// host serves. servesTo is never called for it, so it has no methods for the
// fleet or the stream and no principal is built for it.
func holdsAGraphPrincipal(declared resource.Declaration) bool {
	if declared.Ref.Kind() == ref.KindSink {
		return true
	}

	_, isModule := moduleNamespaceOf(declared)

	return isModule && declared.Spec.Runtime == install.RuntimeService
}

// claimName refuses an instance whose name another kind is already running
// under, where both would read the graph as one principal. The caller holds
// h.mu.
func (h *Host) claimName(declared resource.Declaration) error {
	if !holdsAGraphPrincipal(declared) {
		return nil
	}

	for instance, running := range h.running {
		if instance == declared.Ref || instance.ID() != declared.Ref.ID() {
			continue
		}

		if holdsAGraphPrincipal(running.declaration) {
			return fmt.Errorf(
				"%w: %s and %s are one principal in the authorization graph. Give one of them "+
					"another name, or write the configuration again once %s has stopped",
				ErrNameClaimed, instance, declared.Ref, instance)
		}
	}

	return nil
}

// claimNamespace refuses a start that would take a name another running
// instance holds: the instance name it reads the authorization graph as, and
// the namespace its module surface answers under. The caller holds h.mu.
func (h *Host) claimNamespace(declared resource.Declaration) error {
	if err := h.claimName(declared); err != nil {
		return err
	}

	namespace, isModule := moduleNamespaceOf(declared)
	if !isModule {
		return nil
	}

	for instance, running := range h.running {
		if instance == declared.Ref {
			continue
		}

		held, ok := moduleNamespaceOf(running.declaration)
		if ok && held == namespace {
			return fmt.Errorf("%w: %s is served by %s, and %s declares it too",
				ErrNamespaceClaimed, namespace, instance, declared.Ref)
		}
	}

	return nil
}

// servesTo is the method set this host answers on the session it opens to one
// module. Each method is included only when the host is configured for it and
// the declaration allows it.
//
// It is built at every start, so a module restarted after a configuration
// change is bound to the namespace and the endpoints of the declaration in
// force at that start.
//
// A declaration that is not a module gets an empty set and not nil. Both
// answer CodeMethodNotFound to a call. An empty set shows that the host made
// the decision.
func (h *Host) servesTo(declared resource.Declaration) (wirerpc.Methods, error) {
	namespace, isModule := moduleNamespaceOf(declared)
	if !isModule {
		return wirerpc.Methods{}, nil
	}

	serves := wirerpc.Methods{}

	// An instance of kind db gets no state methods. The state methods find
	// the serving record backend on each call, and a backend given them
	// would be given itself while it is still starting.
	//
	// A host that declares no record backend installs no state method at all,
	// so a module reads CodeMethodNotFound and knows there is no record store.
	// When a backend is declared and is not serving yet, the methods are
	// installed and the calls fail until it serves.
	if h.cfg.State != nil && declared.Ref.Kind() != ref.KindDB && h.recordBackendDeclared() {
		// The gate is the outermost wrapper. A call refused because the module
		// is not ready or has too many calls open never opens a transaction,
		// and is not logged as a store failure. Inside it the store is
		// resolved per call, because the backend is a supervised instance of
		// this host and this method set is installed once.
		records := &lateRecords{namespace: namespace, limit: recordLimitOf(declared), open: h.cfg.State}

		maps.Copy(serves, stateapi.Handlers(newGate(h, declared.Ref, h.policyFor(declared), &loggedRecords{
			inner:    records,
			instance: declared.Ref,
			log:      h.cfg.Logger,
		})))
	}

	emits, err := h.emitsFor(declared)
	if err != nil {
		return nil, err
	}

	maps.Copy(serves, emits)

	// The fleet methods. They are bound to the instance and not to the
	// namespace, because an operator grants access to an instance and not to
	// the type it was installed from: two instances of one module can hold
	// different grants and different permissions. The vouch wrapper accepts a
	// principal the module names only when this host handed it that principal.
	module := moduleOf(declared)
	module.Data = h.dataGranted(declared)

	if h.cfg.Fleet != nil {
		maps.Copy(serves, deviceapi.Handlers(h.vouchFleet(declared.Ref, h.cfg.Fleet(module))))
	}

	// The instance is starting, so it has served nobody. If another module
	// held this name before, the people that module served are forgotten.
	h.flights.forget(declared.Ref)

	if h.cfg.Access != nil {
		maps.Copy(serves, accessapi.Handlers(h.AccessFor(module)))
	}

	// The list of people, only for an instance whose operator approved
	// list_people.
	maps.Copy(serves, accessapi.PeopleHandlers(h.DirectoryFor(module)))

	// The routes of other modules, for an instance whose operator approved
	// calls or whose manifest requires the method. It reaches only the
	// modules the operator approved.
	maps.Copy(serves, routesapi.CallHandlers(h.callsFor(module, requiresCalls(serviceOf(declared)))))

	// Reading this deployment's storages. Bound to the instance and wrapped
	// in a vouch for the same reason as the fleet methods.
	if h.cfg.Storage != nil {
		maps.Copy(serves, storageapi.Handlers(h.vouchStorage(declared.Ref, h.cfg.Storage(module))))
	}

	// The push cameras. Bound to the instance and wrapped in a vouch for the
	// same reason as the fleet methods.
	if h.cfg.Cameras != nil {
		maps.Copy(serves, cameraapi.Handlers(h.vouchCameras(declared.Ref, h.cfg.Cameras(module))))
	}

	// What the module hides of its own interface while it runs. It is bound
	// to the instance, so a module sets only its own list.
	maps.Copy(serves, uiapi.Handlers(h.hiderFor(declared.Ref)))

	// Subscribing to the bus. Bound to the instance and wrapped in a vouch
	// for the same reason as the fleet methods.
	if h.cfg.Feed != nil {
		maps.Copy(serves, busapi.SubscribeHandler(h.vouchFeed(declared.Ref, h.cfg.Feed(module))))
	}

	// The calls a driver module makes on the host: its sessions' state and
	// the bytes of an upload (ADR-20261123-sat rule 7).
	if declared.Ref.Kind() == ref.KindDriver {
		maps.Copy(serves, h.DriverClient(declared.Ref).Handlers())
	}

	// The one call a link module makes on the host. It is installed only for
	// a link instance, because the event it produces is this site's link
	// state and no other kind reports one.
	if declared.Ref.Kind() == ref.KindLink {
		// The instance is starting, so it has reported nothing yet. The
		// servers its last run reported are forgotten, and it answers for no
		// server until its first report arrives.
		h.flights.nodes.forget(declared.Ref)

		maps.Copy(serves, linkapi.ChangedHandlers(
			newLinkChanges(h, declared.Ref, h.emitterFor(declared.Ref))))
	}

	return serves, nil
}

// moduleOf is the declaration as the host's inbound methods see it. A
// declaration with no service block carries no permissions, so that module
// may do nothing as itself.
func moduleOf(declared resource.Declaration) Module {
	module := Module{Instance: declared.Ref}

	if spec := serviceOf(declared); spec != nil {
		module.Permissions = spec.service.Permissions
		module.Checks = spec.service.Checks
		module.CheckAbsentPeople = spec.service.CheckAbsentPeople
		module.ListPeople = spec.service.ListPeople
		module.Calls = spec.service.Calls
	}

	if entry := declared.Spec.Module; entry != nil {
		for _, required := range entry.Requires {
			if required.Module != "" {
				module.Dependencies = append(module.Dependencies, required.Module)
			}
		}

		for _, suggested := range entry.Suggests {
			module.Dependencies = append(module.Dependencies, suggested.Module)
		}
	}

	return module
}

// dataGranted is the data directory of an instance the operator granted one,
// the same path grantsOf sends the module, and empty for any other instance.
func (h *Host) dataGranted(declared resource.Declaration) string {
	spec := serviceOf(declared)
	if spec == nil || !spec.service.Data || h.cfg.ModuleData == "" {
		return ""
	}

	dir, err := dataPath(h.cfg.ModuleData, declared.Ref)
	if err != nil {
		return ""
	}

	return dir
}

// emitsFor is the bus.emit method of one module's session, and an empty set
// for a declaration that names no endpoint.
//
// The empty set is the default. An installed module whose declaration names
// no events reads CodeMethodNotFound from bus.emit, so a module publishes
// nothing until an operator writes what it may publish and under which name.
//
// The endpoint table is built in the host, from the namespace the operator's
// type produced and the names the operator wrote. A module passes no namespace
// and no event type, so no module can publish under another module's name or
// under a name of the core.
func (h *Host) emitsFor(declared resource.Declaration) (wirerpc.Methods, error) {
	spec := serviceOf(declared)
	if spec == nil || len(spec.service.Events) == 0 {
		return wirerpc.Methods{}, nil
	}

	publisher, err := newBus(h, declared.Ref, h.policyFor(declared),
		spec.service.Events, h.emitterFor(declared.Ref))
	if err != nil {
		return nil, err
	}

	return busapi.Handlers(publisher), nil
}

// loggedRecords logs a store failure for the operator.
//
// stateapi tells a module that the host could not answer and sends none of the
// detail, because the detail names the engine, the statement and a path on this
// host's disk. An operator needs that detail, so it is logged here with the
// instance that asked.
//
// A refusal the contract has a code for is not logged. A namespace at its
// limit and a version conflict are ordinary answers a module acts on, and
// logging them would fill the log.
type loggedRecords struct {
	inner    stateapi.Records
	instance ref.Ref
	log      *slog.Logger
}

var _ stateapi.Records = (*loggedRecords)(nil)

// note logs a failure of the host and returns it unchanged.
func (l *loggedRecords) note(ctx context.Context, call string, key stateapi.Key, err error) error {
	if err == nil || refusedByContract(err) {
		return err
	}

	l.log.ErrorContext(ctx, "a module's record store failed",
		"resource", l.instance.String(), "call", call, "key", string(key), "error", err.Error())

	return err
}

// refusedByContract reports an error stateapi publishes a code for. Such an
// error is an answer to the module and not a failure of this host.
func refusedByContract(err error) bool {
	for _, refusal := range []error{
		stateapi.ErrNoRecord,
		stateapi.ErrVersion,
		stateapi.ErrKey,
		stateapi.ErrValueTooLarge,
		stateapi.ErrNamespaceFull,
	} {
		if errors.Is(err, refusal) {
			return true
		}
	}

	return false
}

func (l *loggedRecords) Get(ctx context.Context, key stateapi.Key) (stateapi.Value, error) {
	held, err := l.inner.Get(ctx, key)

	return held, l.note(ctx, "get", key, err)
}

func (l *loggedRecords) Put(ctx context.Context, key stateapi.Key, value []byte) (uint64, error) {
	version, err := l.inner.Put(ctx, key, value)

	return version, l.note(ctx, "put", key, err)
}

func (l *loggedRecords) CompareAndPut(
	ctx context.Context, key stateapi.Key, expected uint64, value []byte,
) (uint64, error) {
	version, err := l.inner.CompareAndPut(ctx, key, expected, value)

	return version, l.note(ctx, "compare_and_put", key, err)
}

func (l *loggedRecords) Delete(ctx context.Context, key stateapi.Key) error {
	return l.note(ctx, "delete", key, l.inner.Delete(ctx, key))
}

func (l *loggedRecords) List(ctx context.Context, prefix stateapi.Key) ([]stateapi.Key, error) {
	keys, err := l.inner.List(ctx, prefix)

	return keys, l.note(ctx, "list", prefix, err)
}

func (l *loggedRecords) Version(ctx context.Context, key stateapi.Key) (uint64, error) {
	version, err := l.inner.Version(ctx, key)

	return version, l.note(ctx, "version", key, err)
}

func (l *loggedRecords) Usage(ctx context.Context) (stateapi.Usage, error) {
	usage, err := l.inner.Usage(ctx)

	return usage, l.note(ctx, "usage", "", err)
}

// Module returns the module that claims one namespace. The HTTP surface mounts
// /api/v1/modules/<namespace>/ through it.
//
// It is resolved per request and not at start-up, because an operator installs
// and removes modules while the process runs. The router is then never
// rebuilt.
func (h *Host) Module(namespace string) (httpapi.ModuleSurface, bool) {
	if extension.Namespace(namespace).Validate() != nil {
		return httpapi.ModuleSurface{}, false
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for instance, running := range h.running {
		held, ok := moduleNamespaceOf(running.declaration)
		if !ok || held != namespace {
			continue
		}

		return h.surfaceOf(instance, running)
	}

	return httpapi.ModuleSurface{}, false
}

// surfaceOf is one running module as the mount sees it, under its namespace
// or under the root prefix the operator named. The caller holds h.mu.
//
// A module that declared no HTTP surface claims its namespace and mounts
// nothing, so its path is unrouted like an unclaimed one.
func (h *Host) surfaceOf(instance ref.Ref, running *instance) (httpapi.ModuleSurface, bool) {
	if !moduleServesRoutes(running.runner) {
		return httpapi.ModuleSurface{}, false
	}

	surface := httpapi.ModuleSurface{
		Instance: instance,
		Handler:  h.handlerFor(instance, running),
		OpenAPI:  moduleDocumentOf(running.runner),
		Mount:    moduleMountOf(running.declaration),
	}

	if spec := serviceOf(running.declaration); spec != nil {
		surface.Public = spec.service.PublicRoutes
	}

	_, surface.Staged = h.stagedOf(running.declaration)

	if module := running.declaration.Spec.Module; module != nil {
		surface.Title, surface.Summary = module.Title, module.Summary
	}

	if drawn, ok := uiOf(running.runner); ok {
		if namespace, named := moduleNamespaceOf(running.declaration); named {
			surface.Themes = httpapi.ThemeFiles(namespace, drawn)
		}
	}

	return surface, true
}

// handlerFor is the handler under one module's namespace: its own routes, and
// its files under the /ui/ subtree when its declaration carries a bundle.
//
// The host serves the files and the child does not. The host verified the
// digest and holds the directory open, so the bundle a page already imported
// is still served while the module's process is restarting.
func (h *Host) handlerFor(instance ref.Ref, running *instance) http.Handler {
	routes := &moduleHandler{host: h, instance: instance}

	bundle, ok := supervisor.Peek(running.runner, bundleTarget)
	if !ok || bundle == nil {
		return routes
	}

	files := httpapi.ModuleUIHandler(bundle.ui)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if httpapi.UnderModuleUI(r.URL.Path) {
			files.ServeHTTP(w, r)

			return
		}

		routes.ServeHTTP(w, r)
	})
}

var _ httpapi.ModulePlane = (*Host)(nil)

// Documented reports whether the instance is a running module that shipped an
// OpenAPI document, for the listing to publish the document's address beside
// it.
func (h *Host) Documented(instance ref.Ref) bool {
	h.mu.Lock()
	running, ok := h.running[instance]
	h.mu.Unlock()

	if !ok {
		return false
	}

	return moduleDocumentOf(running.runner) != nil
}

// UI is the browser part of a running module, for the listing to publish
// beside the instance. It is false for an instance that is not running, has no
// bundle, or ships a bundle whose manifest declares no ui block.
//
// A bundle becomes the instance's target only after its digest was verified
// and its manifest was read, so this reports false for an instance that failed
// either step.
func (h *Host) UI(instance ref.Ref) (httpapi.ResourceUI, bool) {
	h.mu.Lock()
	running, ok := h.running[instance]
	h.mu.Unlock()

	if !ok {
		return httpapi.ResourceUI{}, false
	}

	surface, drawn := uiOf(running.runner)
	if drawn {
		surface.Hidden = h.hidden.of(instance)
	}

	return surface, drawn
}

// Database is the schema a running module's own database holds, as the
// module reported it when it started, for the listing to publish beside the
// grant. It is false for an instance that is not running or reported none.
func (h *Host) Database(instance ref.Ref) (wirerpc.DatabaseState, bool) {
	h.mu.Lock()
	running, ok := h.running[instance]
	h.mu.Unlock()

	if !ok {
		return wirerpc.DatabaseState{}, false
	}

	state, ok := supervisor.Peek(running.runner, func(target any) *wirerpc.DatabaseState {
		if child, isChild := target.(*serviceChild); isChild {
			return child.database
		}

		return nil
	})
	if !ok || state == nil {
		return wirerpc.DatabaseState{}, false
	}

	return *state, true
}

// moduleServesRoutes reports whether the running child declared an HTTP
// surface. It is false for a child that named no routes, and for an asset
// bundle, which has files and no routes: those are mounted through UIOf. A
// module between restarts stays mounted, so a caller reads the supervisor's
// refusal during the restart and not an unrouted path.
func moduleServesRoutes(runner *supervisor.Instance[any]) bool {
	serves, ok := supervisor.Peek(runner, func(target any) bool {
		switch held := target.(type) {
		case *serviceChild:
			// A module that named no route still claims its namespace when it
			// ships a bundle: the host serves the files under it.
			return held.child.ServesRoutes() || held.bundle != nil
		case *assetBundle:
			return false
		default:
			return true
		}
	})

	return !ok || serves
}

// moduleDocumentOf is the rebased document the running child holds, and nil
// for an instance that is not running, is not a child process, or shipped
// none.
//
// It is read from the target with Peek and not through a call, because the
// launch set the value and the module is not asked for it again. A module that
// is restarting has no document until it serves again, and the address
// answers 404 until then.
func moduleDocumentOf(runner *supervisor.Instance[any]) []byte {
	document, ok := supervisor.Peek(runner, func(target any) []byte {
		if child, isChild := target.(*serviceChild); isChild {
			return child.document
		}

		return nil
	})
	if !ok {
		return nil
	}

	return document
}

// headersFor is the request headers the operator granted the running
// instance beside the ones every module gets, and nil for none or for an
// instance that is not running (ADR-20261118-nnm).
func (h *Host) headersFor(instance ref.Ref) []string {
	// A reconcile replaces the declaration under mu, so it is read under mu.
	h.mu.Lock()
	defer h.mu.Unlock()

	running, ok := h.running[instance]
	if !ok {
		return nil
	}

	declared := running.declaration.Spec
	if declared.Runtime != install.RuntimeService || declared.Service == nil {
		return nil
	}

	return slices.Clone(declared.Service.RequestHeaders)
}

// moduleHandler forwards one authorized request into the module's own process
// and writes back what it answered.
//
// It holds the ref and not the instance, and resolves the instance per request.
// A module that was replaced or removed after the mount was resolved must not
// be reached through an old pointer.
type moduleHandler struct {
	host     *Host
	instance ref.Ref
}

func (m *moduleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	supervised, ok := m.host.Instance(m.instance)
	if !ok {
		httpapi.WriteProblem(w, r,
			httpfault.ResourceNotRunning(m.instance.String()))

		return
	}

	request, staged, err := m.requestOf(w, r)
	if err != nil {
		if errors.Is(err, errStagingBusy) {
			w.Header().Set("Retry-After", strconv.Itoa(int(stagedRetryAfter.Seconds())))
		}

		httpapi.WriteProblem(w, r, m.bodyFault(r, err))

		return
	}

	if staged != nil {
		defer staged.remove(m.host.cfg.Logger)
	}

	// The request is recorded for as long as the call is open. A fleet call
	// or a subscription the module makes for this person during that time is
	// accepted, and one it makes after the call ended is not.
	leave := m.host.flights.board(m.instance, request.Principal, authz.ScopesFrom(r.Context()))
	defer leave()

	// supervisor.Call applies the state gate, the breaker, the bulkhead, the
	// call deadline and the panic recovery. A module that has not reported
	// ready is refused here and the call never leaves the host. A module that
	// dies during a request fails only its own instance. A staged request
	// takes the longer budget of its route, so a module reading a large file
	// is not timed out and counted against its breaker.
	var budget time.Duration
	if staged != nil {
		budget = staged.budget
	}

	answered, err := supervisor.CallWithin(r.Context(), supervised, string(routesapi.MethodServe), budget,
		func(callCtx context.Context, target any) (routesapi.Response, error) {
			return serveThroughChild(callCtx, target, request)
		})
	if err != nil {
		m.host.cfg.Logger.WarnContext(r.Context(), "a module refused a request",
			"resource", m.instance.String(), "path", r.URL.Path, "error", err.Error())

		httpapi.WriteProblem(w, r, moduleCallFault(err))

		return
	}

	if err = answered.Write(w); err != nil {
		m.host.cfg.Logger.WarnContext(r.Context(), "writing a module's answer",
			"resource", m.instance.String(), "error", err.Error())
	}
}

// requestOf is the request as the wire carries it. On a staged route the body
// is written to a file first, and the caller removes it once the module
// answered.
func (m *moduleHandler) requestOf(w http.ResponseWriter, r *http.Request) (routesapi.Request, *stagedBody, error) {
	granted := m.host.headersFor(m.instance)

	route, isStaged := m.host.stagedFor(m.instance, r.Method, r.URL.Path)
	if !isStaged {
		request, err := moduleRequestOf(r, granted)

		return request, nil, err
	}

	// The server's deadlines are sized for a document, and a staged body may
	// be a gigabyte. The write deadline runs from the same start as the read
	// one, so it moves too, past the module's own budget, or the answer could
	// not be written after a long upload. A writer that cannot move them is a
	// test recorder.
	control := http.NewResponseController(w)
	read := time.Now().Add(route.readBudget())
	_ = control.SetReadDeadline(read)
	_ = control.SetWriteDeadline(read.Add(route.readBudget() + stagedAnswer))

	slot, free := m.host.staging.take(m.instance)
	if !free {
		return routesapi.Request{}, nil, errStagingBusy
	}

	reservation, err := m.host.room(r, route)
	if err != nil {
		slot()

		return routesapi.Request{}, nil, err
	}

	// Once the body is on disk, the disk's free space shows it, so the room
	// it was let in for is given back then and not counted twice.
	body, err := stage(r, route)
	reservation()

	if err != nil {
		slot()

		return routesapi.Request{}, nil, err
	}

	body.release = slot
	body.budget = route.readBudget()

	request := wireRequestOf(r, nil, granted)
	request.BodyFile = body.file

	return request, &body, nil
}

// bodyFault is the refusal of a body the host would not take. A body over a
// staged route's limit names that limit, and any other the wire's.
//
// The server's body limit wraps every request in http.MaxBytesReader at the
// same limit, so a chunked body over it fails there, as a
// *http.MaxBytesError, before the host's own count sees the extra byte. That
// is the same refusal.
func (m *moduleHandler) bodyFault(r *http.Request, err error) *fault.Fault {
	var overServerLimit *http.MaxBytesError

	switch {
	case errors.Is(err, errBodyTooLarge), errors.As(err, &overServerLimit):
		limit := int64(routesapi.MaxModuleBody)
		if route, isStaged := m.host.stagedFor(m.instance, r.Method, r.URL.Path); isStaged {
			limit = route.limit
		}

		return httpfault.BodyTooLarge(limit)
	case errors.Is(err, errStagingBusy):
		return httpfault.ModuleUploadsBusy(stagedSlots)
	case errors.Is(err, errStagingFull):
		m.host.cfg.Logger.WarnContext(r.Context(), "a staged request body was refused for want of disk space",
			"resource", m.instance.String(), "error", err.Error())

		return httpfault.ModuleStorageFull()
	case errors.Is(err, errStageWrite):
		m.host.cfg.Logger.ErrorContext(r.Context(), "staging a request body",
			"resource", m.instance.String(), "error", err.Error())

		return httpfault.Unexpected("the request body could not be stored")
	default:
		return httpfault.RequestInvalid("body", "unreadable", "the request body could not be read")
	}
}

// moduleCaller is the part of a supervised target this surface needs: one
// control call over the session the host opened. Only serviceChild satisfies
// it, so only a module in a process of its own has an HTTP surface.
type moduleCaller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
}

// serveThroughChild makes the one call, and refuses an answer this host will
// not relay.
//
// The type assertion is made inside the supervised call and not at mount
// time, because the target is rebuilt on every start. An instance whose
// runtime builds another kind of target then fails as a counted call and does
// not panic on the caller's goroutine.
func serveThroughChild(
	ctx context.Context, target any, request routesapi.Request,
) (routesapi.Response, error) {
	child, ok := target.(moduleCaller)
	if !ok {
		return routesapi.Response{}, fmt.Errorf("%w: %T serves no HTTP surface", errNoRoutes, target)
	}

	raw, err := child.Call(ctx, string(routesapi.MethodServe), request)
	if err != nil {
		return routesapi.Response{}, fmt.Errorf("call the module: %w", err)
	}

	var answered routesapi.Response
	if err = json.Unmarshal(raw, &answered); err != nil {
		return routesapi.Response{}, fmt.Errorf("read the module's answer: %w", err)
	}

	if err = answered.Validate(); err != nil {
		return routesapi.Response{}, err
	}

	return answered, nil
}

// errNoRoutes reports an instance whose target answers no HTTP surface.
var errNoRoutes = errors.New("resourcehost: the instance serves no routes")

// moduleCallFault renders what the call came back with.
//
// A refusal the supervision plane raised already carries its own code and
// status, such as the one for a module that is not ready. Any other error
// comes from the module. Its text is in the log and not in the answer, because
// a module writes its own error text and this server does not publish that
// text to a client.
func moduleCallFault(err error) *fault.Fault {
	if f, ok := errors.AsType[*fault.Fault](err); ok {
		return f
	}

	return httpfault.Unexpected("the module did not answer")
}

// readBounded reads a request body up to limit, and reports a body over it.
// It does not truncate: a module could not tell that a truncated body was
// cut.
func readBounded(r *http.Request, limit int) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read the request body: %w", err)
	}

	if len(body) > limit {
		return nil, fmt.Errorf("%w: the body is over %d bytes", errBodyTooLarge, limit)
	}

	return body, nil
}

// errBodyTooLarge reports a request body this method cannot carry.
var errBodyTooLarge = errors.New("resourcehost: the request body is over the module bound")

// moduleRequestOf is the request as the wire carries it. The host reads the
// body under the bound, so a caller cannot make this host buffer more than the
// method may carry before the request is refused.
//
// granted is the request headers the operator granted the module beside the
// ones every module gets (ADR-20261118-nnm).
func moduleRequestOf(r *http.Request, granted []string) (routesapi.Request, error) {
	body, err := readBounded(r, routesapi.MaxModuleBody)
	if err != nil {
		return routesapi.Request{}, err
	}

	return wireRequestOf(r, body, granted), nil
}

// wireRequestOf is the request with the body the caller read.
func wireRequestOf(r *http.Request, body []byte, granted []string) routesapi.Request {
	return routesapi.Request{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		// Only the headers every module gets and the ones the operator
		// granted this one. The person's Cookie, Authorization and X-Api-Key
		// stay here, so the module cannot call the core as them.
		Headers:   routesapi.GrantedRequestHeaders(r.Header, granted),
		Body:      body,
		Principal: httpapi.ModulePrincipalOf(r.Context()),
	}
}
