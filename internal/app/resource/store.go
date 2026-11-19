package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/atomicfile"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpfault"
)

// File modes. The overlay is 0600 because a settings block can hold a
// connection string with a password, and the directory is 0700 for the same
// reason.
const (
	overlayPerm = 0o600
	dirPerm     = 0o700

	backupSuffix = ".bak"
	jsonIndent   = "  "
)

// Config is what a Store is built from.
type Config struct {
	// Paths names the two inputs.
	Paths Paths
	// Plane carries out reconcile plans and reports what it observes. A nil
	// Plane accepts writes and reports every enabled instance as pending. A
	// configuration-only deployment and a test use nil.
	Plane Plane
	// Graph decides what each caller may see and change. It is required, so
	// no store can answer an access question without the authorization graph.
	Graph Graph
	// Validator is the type's own check of a declaration, called before
	// anything is written. Nil validates structure only.
	Validator Validator
	// Declared is the settings each registered type accepts. It says whether
	// a key is a credential, and whether this build has an implementation for
	// a type name. A block declared for an unknown type name is answered with
	// every value replaced, because the store cannot tell which of its keys is
	// a credential.
	//
	// Nil is a store with no type table. It redacts a module's declared
	// secrets, because a module carries its own option list in the
	// declaration, and it treats a compiled-in type as one that declares no
	// settings. Every deployment passes one. A configuration-only test may
	// leave it nil.
	Declared Declared
	// Logger is where a rejected merge and a failed reconcile are reported.
	Logger *slog.Logger
	// Minter creates the identity a new instance keeps for its whole life. Nil
	// takes one of the Store's own.
	Minter *ref.Minter
	// Now is the clock the minter reads. Nil is time.Now.
	Now func() time.Time
	// ModulesOff is the operator's core.modules_enabled: false. Every module
	// the server installed is then reconciled as switched off, and the listing
	// says why. The zero value keeps modules on.
	ModulesOff bool
	// SafeMode is the server running in safe mode. With ModulesOff it keeps
	// every module off, including the ones the operator declared in the
	// configuration file, and the listing and the refusals say safe mode. It
	// has no effect without ModulesOff.
	SafeMode bool
	// Pool is the memory the budgets of all enabled service modules are meant
	// to fit in. The store reports it and refuses no write because of it.
	Pool Pool
	// SwitchOff is the server's core.modules_switch_off, which the status of
	// a module reports when its operator set none. The zero value reports
	// the install defaults.
	SwitchOff install.SwitchOff
}

// Store is the configured inventory over two files: the operator's, which it
// only reads, and the overlay, which it owns.
//
// One mutex covers every method. A write rewrites the whole overlay file and
// not one instance, so writes are serialised on the file and one lock is
// enough.
type Store struct {
	mu        sync.Mutex
	current   Document
	paths     Paths
	plane     Plane
	graph     Graph
	validator Validator
	declared  Declared
	log       *slog.Logger
	minter    *ref.Minter
	now       func() time.Time
	// modules is the operator's key for the modules the server installed.
	modules modulesKey
	pool    Pool
	// switchOff is Config.SwitchOff.
	switchOff install.SwitchOff
	// hostHeld are the instances of the operator's file the host switched off
	// because they failed too often. The store does not write that file, so
	// they are kept off here until the server restarts.
	hostHeld map[ref.Ref]SwitchOff
	// endpoints receives the list of dialled addresses whenever it changes.
	// It is nil until WatchEndpoints sets it. That happens after the startup
	// reconcile, so WatchEndpoints passes the first list itself.
	endpoints EndpointWatcher
	// origins receives the browser origins of the modules that are switched
	// on whenever the document changes. It is nil until WatchModuleOrigins
	// sets it.
	origins OriginWatcher
}

var _ httpapi.ResourceStore = (*Store)(nil)

// New returns a store over the two paths. Nothing is read and nothing is
// reconciled until Reload.
func New(cfg Config) (*Store, error) {
	if cfg.Paths.Overlay == "" {
		return nil, fmt.Errorf("%w: no overlay path", ErrConfigInvalid)
	}

	if cfg.Graph == nil {
		return nil, fmt.Errorf("%w: no authorization graph", ErrConfigInvalid)
	}

	store := &Store{
		paths:     cfg.Paths,
		plane:     cfg.Plane,
		graph:     cfg.Graph,
		validator: cfg.Validator,
		declared:  cfg.Declared,
		log:       cfg.Logger,
		minter:    cfg.Minter,
		now:       cfg.Now,
		modules:   modulesKey{off: cfg.ModulesOff, safe: cfg.SafeMode},
		pool:      cfg.Pool,
		switchOff: cfg.SwitchOff,
		hostHeld:  map[ref.Ref]SwitchOff{},
	}

	if store.log == nil {
		store.log = slog.New(slog.DiscardHandler)
	}

	if store.minter == nil {
		store.minter = &ref.Minter{}
	}

	if store.now == nil {
		store.now = time.Now
	}

	store.modules.since = store.now()

	return store, nil
}

// Reload reads both inputs and reconciles against them. It is the startup
// path. An accepted write ends with the same reconcile, so a configuration
// behaves the same when it is applied and when the process next starts.
//
// It writes only when an instance has no identity yet or its digest changed.
// Reloading an unchanged configuration writes nothing and restarts nothing.
func (s *Store) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	read, err := readInputs(s.paths)
	if err != nil {
		return err
	}

	doc, err := document(read)
	if err != nil {
		return err
	}

	s.reportFailures(ctx, doc)

	if doc, err = s.recordIdentities(doc, read); err != nil {
		return err
	}

	return s.reconcile(ctx, doc)
}

// List returns every instance the caller may see, narrowed to one kind when
// kind is non-zero. An instance they hold no grant on is not in the answer and
// is not counted in it.
func (s *Store) List(ctx context.Context, principal string, kind ref.Kind) (httpapi.ResourceListing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	who, err := s.subject(principal)
	if err != nil {
		return httpapi.ResourceListing{}, err
	}

	permitted, err := s.permitted(ctx, who)
	if err != nil {
		return httpapi.ResourceListing{}, err
	}

	doc, err := s.reread(ctx, principal, "list")
	if err != nil {
		return httpapi.ResourceListing{}, err
	}

	listing := httpapi.ResourceListing{
		Items: make([]httpapi.ResourceInstance, 0, len(permitted)),
	}

	for _, declared := range doc.Instances {
		if kind != "" && declared.Ref.Kind() != kind {
			continue
		}

		if _, allowed := permitted[declared.Ref]; !allowed {
			continue
		}

		listing.Items = append(listing.Items, s.project(declared, doc.Version, seenIn(permitted)))
	}

	// An empty answer carries no token. The version covers the whole overlay,
	// so it changes every time anybody writes an instance. A caller who may
	// see no instance must not learn from it that others are writing. Such a
	// caller also needs no token: a write needs a grant on the instance, and
	// a create quotes If-None-Match.
	if len(listing.Items) > 0 {
		listing.Version = doc.Version
	}

	return listing, nil
}

// Get returns one instance. A caller who holds no grant on it gets the same
// answer, byte for byte, as a caller asking for an instance that does not
// exist. The graph is asked before the configuration is read, so the answer
// cannot depend on whether the instance exists.
func (s *Store) Get(ctx context.Context, principal string, instance ref.Ref) (httpapi.ResourceInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	who, err := s.subject(principal)
	if err != nil {
		return httpapi.ResourceInstance{}, err
	}

	seen, err := s.visible(ctx, who, instance)
	if err != nil {
		return httpapi.ResourceInstance{}, err
	}

	if !seen {
		return httpapi.ResourceInstance{}, httpfault.ResourceNotFound(instance.String())
	}

	doc, err := s.reread(ctx, principal, "get")
	if err != nil {
		return httpapi.ResourceInstance{}, err
	}

	declared, ok := doc.Find(instance)
	if !ok {
		return httpapi.ResourceInstance{}, httpfault.ResourceNotFound(instance.String())
	}

	return s.project(declared, doc.Version, s.seeing(ctx, who)), nil
}

// Restart rebuilds one running instance from the declaration in force. It
// needs operate_resource: the instance changes state and its declaration does
// not. A caller who may restart a failed sink may therefore not rewrite the
// connection string inside it.
//
// The refusals are in the same order as in Get, for the same reason. A caller
// who holds no grant gets the answer for an instance that does not exist. A
// caller who can see the instance and may not operate it is told which grant
// is missing, because that caller can already read the instance.
//
// An instance that is not supervised is a conflict and not a missing
// instance. A disabled instance is declared and readable, and it has nothing
// to restart until somebody enables it, which is a write.
func (s *Store) Restart(ctx context.Context, principal string, instance ref.Ref) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	who, err := s.subject(principal)
	if err != nil {
		return err
	}

	seen, err := s.visible(ctx, who, instance)
	if err != nil {
		return err
	}

	if !seen {
		return httpfault.ResourceNotFound(instance.String())
	}

	operates, err := s.operable(ctx, who, instance.Kind())
	if err != nil {
		return err
	}

	if !operates {
		return authz.Denied(authz.ActionOperateResource, instance)
	}

	doc, err := s.reread(ctx, principal, "restart")
	if err != nil {
		return err
	}

	declared, found := doc.Find(instance)
	if !found {
		return httpfault.ResourceNotFound(instance.String())
	}

	if err = s.restart(declared); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "a resource instance was restarted",
		"resource", instance.String(), "principal", principal)

	return nil
}

// Apply writes desired into the overlay and reconciles. Every refusal happens
// before anything is written, so a rejected declaration leaves both files
// unchanged.
func (s *Store) Apply(
	ctx context.Context,
	principal string,
	instance ref.Ref,
	desired httpapi.ResourceSpec,
	pre httpapi.Precondition,
) (httpapi.ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	write, err := s.begin(ctx, principal, instance, pre, opApply)
	if err != nil {
		return httpapi.ApplyResult{}, err
	}

	spec, err := s.validate(ctx, instance, desired, write.current, write.exists)
	if err != nil {
		return httpapi.ApplyResult{}, err
	}

	if err = refuseSecondInstance(instance, write.doc, write.exists); err != nil {
		return httpapi.ApplyResult{}, err
	}

	uid := write.current.UID
	if uid.IsZero() {
		if uid, err = s.minter.Mint(s.now()); err != nil {
			return httpapi.ApplyResult{}, httpfault.Unexpected("mint an instance identity: " + err.Error())
		}
	}

	next := withInstance(write.overlay, instance, uid, spec)
	applied := Declaration{
		Ref:        instance,
		UID:        uid,
		Spec:       spec,
		Source:     SourceOverlay,
		Generation: next.Identities[instance.Path()].Generation,
	}

	if err = refuseUnresolved(write.doc, applied, false); err != nil {
		return httpapi.ApplyResult{}, err
	}

	if pre.DryRun {
		return httpapi.ApplyResult{
			Instance: s.project(
				applied,
				write.doc.Version,
				s.seeingPrincipal(ctx, principal),
			),
			Created: !write.exists,
		}, nil
	}

	version, err := s.commit(ctx, next, write.read)
	if err != nil {
		return httpapi.ApplyResult{}, err
	}

	return httpapi.ApplyResult{
		Instance: s.project(applied, version, s.seeingPrincipal(ctx, principal)), Created: !write.exists,
	}, nil
}

// Delete removes an instance from the overlay and reconciles. It removes the
// identity too. The same name declared again is a new instance with a new
// UID, so the data rows of the old instance do not attach to it.
func (s *Store) Delete(
	ctx context.Context, principal string, instance ref.Ref, pre httpapi.Precondition,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	write, err := s.begin(ctx, principal, instance, pre, opDelete)
	if err != nil {
		return err
	}

	if err = refuseUnresolved(write.doc, Declaration{Ref: instance}, true); err != nil {
		return err
	}

	if pre.DryRun {
		return nil
	}

	_, err = s.commit(ctx, withoutInstance(write.overlay, instance), write.read)

	return err
}

// Version returns the token a write must quote, as of the last reconcile. It
// covers the whole configuration, because a write rewrites the whole overlay
// file.
func (s *Store) Version() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.current.Version
}

// Document returns the merged configuration as it was last reconciled.
func (s *Store) Document() Document {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.current
}

// writeOp is which write is being made. The authorization check reads only
// mayCreate: an apply may name an instance that does not exist yet, and a
// delete may not.
type writeOp struct {
	name      string
	mayCreate bool
}

var (
	opApply  = writeOp{name: "apply", mayCreate: true}
	opDelete = writeOp{name: "delete"}
)

// pending is one write that passed its refusals and is not committed yet.
type pending struct {
	read    inputs
	doc     Document
	overlay overlayFile
	current Declaration
	exists  bool
}

// begin authorizes the write, reads the inputs and applies the three refusals
// of refuse. It reads from disk and not from the last reconciled document,
// because an operator may have edited the file by hand, and that edit did not
// pass through the store.
//
// The graph is asked first and the disk is read second. A refusal made before
// the configuration is read cannot depend on it, so the answer for an instance
// the caller may not see is the same as for one that does not exist.
func (s *Store) begin(
	ctx context.Context, principal string, instance ref.Ref, pre httpapi.Precondition, op writeOp,
) (pending, error) {
	s.log.DebugContext(ctx, "resource write",
		"op", op.name, "principal", principal, "resource", instance.String())

	who, err := s.subject(principal)
	if err != nil {
		return pending{}, err
	}

	if err = s.authorizeWrite(ctx, who, instance, op.mayCreate); err != nil {
		return pending{}, err
	}

	read, err := readInputs(s.paths)
	if err != nil {
		return pending{}, s.internal("read the configuration", err)
	}

	doc, err := document(read)
	if err != nil {
		return pending{}, s.internal("parse the configuration", err)
	}

	overlay, err := parseOverlay(read.overlay)
	if err != nil {
		return pending{}, s.internal("parse the overlay", err)
	}

	current, exists := doc.Find(instance)

	if err = s.refuse(instance, doc, current, exists, pre); err != nil {
		return pending{}, err
	}

	return pending{read: read, doc: doc, overlay: overlay, current: current, exists: exists}, nil
}

// refuse makes the refusals in the required order. An instance the operator
// declared by hand is refused first. No token makes it writable, and a 412
// would send a client to read a new version that does not help.
func (s *Store) refuse(
	instance ref.Ref, doc Document, current Declaration, exists bool, pre httpapi.Precondition,
) error {
	switch {
	case exists && current.Source == SourceFile:
		return httpfault.ResourceNotWritable(instance.String(), current.Source.String(), s.paths.File)
	case !exists && !pre.MustNotExist && len(pre.IfNoneMatch) == 0:
		return httpfault.ResourceNotFound(instance.String())
	case !pre.Holds(exists, doc.Version):
		return httpfault.VersionConflict(instance.String(), doc.Version)
	}

	return nil
}

// commit stages the new overlay, checks that neither input changed while it
// was staged, keeps a copy of the bytes it replaces, and then renames. The
// second read is the compare step of a compare-and-swap. If a person saved the
// file while this call was running, the rename does not happen and their edit
// is kept.
func (s *Store) commit(ctx context.Context, next overlayFile, read inputs) (string, error) {
	encoded, err := renderOverlay(next)
	if err != nil {
		return "", s.internal("render the overlay", err)
	}

	if err = os.MkdirAll(filepath.Dir(s.paths.Overlay), dirPerm); err != nil {
		return "", s.internal("create the overlay directory", err)
	}

	staged, err := atomicfile.Stage(s.paths.Overlay, encoded, overlayPerm)
	if err != nil {
		return "", s.internal("stage the overlay", err)
	}

	if err = s.verifyUnmoved(read); err != nil {
		_ = staged.Discard()

		return "", err
	}

	s.keepPrevious(ctx, read.overlay)

	if err = staged.Commit(); err != nil {
		return "", s.internal("publish the overlay", err)
	}

	doc, err := Load(s.paths)
	if err != nil {
		return "", s.internal("reload the configuration", err)
	}

	// From here the caller cannot cancel the work. The rename above put the
	// declaration on disk. Reconcile then writes the device row in the graph
	// and tells the supervision plane to start the instance. A client that
	// stopped waiting, or a request that ran past its route budget, ends the
	// context before that is done.
	//
	// No later call repeats a cancelled reconcile. Reconcile adopts the
	// document it applied, so every later write and reload diffs against a
	// document that already holds the instance and produces no step for it.
	// The instance would stay declared, readable and not supervised until the
	// process restarted.
	applied := context.WithoutCancel(ctx)

	if err = s.reconcile(applied, doc); err != nil {
		// The declaration was accepted and is on disk, so the write happened.
		// An error returned here would say it did not. A supervision refusal
		// does not reach here: reconcile reports it per instance (see
		// reportRefused). The error here means the graph did not record the
		// change.
		s.log.ErrorContext(applied, "reconcile after a write failed", "error", err.Error())
	}

	return doc.Version, nil
}

// verifyUnmoved reads both inputs again and refuses when either changed since
// the token was computed. An edit between this read and the rename is not
// caught, because a filesystem offers no compare-and-swap.
func (s *Store) verifyUnmoved(read inputs) error {
	now, err := readInputs(s.paths)
	if err != nil {
		return s.internal("re-read the configuration", err)
	}

	if now.version == read.version {
		return nil
	}

	s.log.Warn("the configuration changed while a write was being staged; the write was refused",
		"read_version", read.version, "current_version", now.version)

	return httpfault.VersionConflict("", now.version)
}

// keepPrevious writes the bytes the commit is about to replace to a file
// beside the overlay, with the suffix .bak. An operator restores from that
// copy when a write was wrong. OPNsense keeps a backup of its configuration in
// the same way.
//
// A failure here does not fail the write. An accepted declaration is worth
// more than its backup.
func (s *Store) keepPrevious(ctx context.Context, previous []byte) {
	if len(previous) == 0 {
		return
	}

	if err := atomicfile.Write(s.paths.Overlay+backupSuffix, previous, overlayPerm); err != nil {
		s.log.WarnContext(ctx, "could not keep the previous overlay", "error", err.Error())
	}
}

// reconcile diffs against the last reconciled document and hands the plan to
// the supervision plane. The document is adopted even when a later step
// fails. It is the document on disk, and the next write must be decided
// against it.
//
// A plane that reports a failure has already recorded it against the instance.
// An error from writing the directory to the graph is returned to the caller,
// so the startup path can refuse to start. The next reconcile writes the whole
// declared set again, which repairs the directory.
func (s *Store) reconcile(ctx context.Context, next Document) error {
	for _, shadowed := range next.Shadowed {
		s.log.WarnContext(ctx, "an overlay instance is shadowed by the operator's file and is not in effect",
			"resource", shadowed.String(), "file", s.paths.File, "overlay", s.paths.Overlay)
	}

	// The directory is written before the document is adopted. Every access
	// check on an instance reads the directory. If the document were adopted
	// first, a new instance would be invisible to its creator, and a removed
	// one would still be readable by everybody who could read it before.
	//
	// A failure here does not stop the rest. The declaration is already on
	// disk. If the store did not adopt it, the next write would be decided
	// against a document that is not on disk, and the plane would never be
	// told to run what the file declares. The error is returned at the end.
	directory := s.syncDirectory(ctx, next)
	if directory != nil {
		directory = fmt.Errorf("record the declared instances in the authorization graph: %w", directory)

		s.log.ErrorContext(ctx, "the declared instances did not reach the authorization graph",
			"error", directory.Error())
	}

	previous := s.effective(s.current)
	s.releaseHeld(next)

	plan := Diff(previous, s.effective(next))
	moved := !slices.EqualFunc(s.current.Endpoints, next.Endpoints, sameEndpoint)
	s.current = next

	// The watcher is called after the document is adopted, so a watcher that
	// reads this store gets the same list. An added address then takes effect
	// with the write and needs no restart.
	if moved && s.endpoints != nil {
		s.endpoints.SyncEndpoints(ctx, slices.Clone(next.Endpoints))
	}

	// With the document, so an origin holds from the write that switched its
	// module on and leaves with the one that switched it off or removed it.
	s.syncModuleOrigins(ctx)

	if plan.Empty() || s.plane == nil {
		return directory
	}

	if err := s.plane.Reconcile(ctx, plan); err != nil {
		s.reportRefused(ctx, err)
	}

	return directory
}

// reread loads both inputs for a read. A read serves what is on disk and not
// what was last reconciled. A client whose write was refused for an old token
// can then read the current token and try again.
func (s *Store) reread(ctx context.Context, principal, op string) (Document, error) {
	s.log.DebugContext(ctx, "resource read", "op", op, "principal", principal)

	doc, err := Load(s.paths)
	if err != nil {
		return Document{}, s.internal("read the configuration", err)
	}

	return doc, nil
}

// recordIdentities gives an identity to every instance that has none, and
// raises the generation of a declaration whose digest changed since the
// overlay last recorded it. It is the only write the startup path makes, and
// it writes nothing when nothing changed.
//
// A hand edit to the operator's file is counted here, because the edit did not
// pass through the API.
func (s *Store) recordIdentities(doc Document, read inputs) (Document, error) {
	overlay, err := parseOverlay(read.overlay)
	if err != nil {
		return Document{}, err
	}

	next := pruneIdentities(overlay, doc)
	changed := len(next.Identities) != len(overlay.Identities)

	for _, declared := range doc.Instances {
		identity, ok := next.Identities[declared.Ref.Path()]
		if ok && identity.Digest == declared.Spec.Digest() && identity.UID != "" {
			continue
		}

		if identity.UID == "" {
			uid, mintErr := s.minter.Mint(s.now())
			if mintErr != nil {
				return Document{}, fmt.Errorf("mint an identity for %s: %w", declared.Ref, mintErr)
			}

			identity.UID = uid.String()
		}

		identity.Generation++
		identity.Digest = declared.Spec.Digest()
		next.Identities[declared.Ref.Path()] = identity
		changed = true
	}

	if !changed {
		return doc, nil
	}

	if err = s.writeOverlay(next); err != nil {
		return Document{}, err
	}

	return Load(s.paths)
}

// writeOverlay writes an overlay without the compare-and-swap, for the
// identities a load records. It checks no token: it changes only fields the
// store owns, and it runs while the store's lock is held.
func (s *Store) writeOverlay(overlay overlayFile) error {
	encoded, err := renderOverlay(overlay)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(s.paths.Overlay), dirPerm); err != nil {
		return fmt.Errorf("create the overlay directory: %w", err)
	}

	return atomicfile.Write(s.paths.Overlay, encoded, overlayPerm)
}

// internal logs a failure the caller cannot act on and returns the redacted
// fault. The path, the parse position and the errno stay in the log line.
func (s *Store) internal(what string, err error) error {
	s.log.Error("resource store failure", "what", what, "error", err.Error())

	if errors.Is(err, ErrConfigInvalid) {
		return httpfault.Unexpected(what + ": the configuration on disk does not parse")
	}

	return httpfault.Unexpected(what)
}

// withInstance returns the overlay with one instance declared, replacing any
// entry already there and recording its identity and generation.
func withInstance(overlay overlayFile, instance ref.Ref, uid ref.UID, spec Spec) overlayFile {
	next := overlay
	next.Version = overlayFormat
	next.Instances = make([]overlayEntry, 0, len(overlay.Instances)+1)
	next.Identities = copyIdentities(overlay.Identities)

	entry := overlayEntry{
		Kind:     instance.Kind().String(),
		Name:     instance.ID(),
		Type:     spec.Type,
		Enabled:  spec.Enabled,
		Required: spec.Required,
		Config:   spec.Config,

		HiddenContributions: spec.HiddenContributions,
	}

	// A module's entry carries its runtime and its artefact. The resource API
	// writes only types compiled into this binary, so it never writes such an
	// entry. The install command does, through this function, so both write
	// entries of the same form.
	if spec.Runtime != "" && spec.Runtime != install.RuntimeBuiltin {
		entry.Runtime = spec.Runtime.String()
		entry.Service = serviceEntryOf(spec.Service, spec.Type)
		entry.Asset = assetEntryOf(spec.Asset)
		entry.Module = spec.Module
		entry.SwitchedOff = spec.SwitchedOff
	}

	replaced := false

	for _, existing := range overlay.Instances {
		if existing.Kind == entry.Kind && existing.Name == entry.Name {
			next.Instances = append(next.Instances, entry)
			replaced = true

			continue
		}

		next.Instances = append(next.Instances, existing)
	}

	if !replaced {
		next.Instances = append(next.Instances, entry)
	}

	identity := next.Identities[instance.Path()]

	// The install command passes a zero UID. It writes a declaration, and the
	// next load creates the UID, as for a block an operator wrote in their own
	// file.
	if !uid.IsZero() {
		identity.UID = uid.String()
	}

	if identity.Digest != spec.Digest() {
		identity.Generation++
		identity.Digest = spec.Digest()
	}

	next.Identities[instance.Path()] = identity

	return next
}

// withoutInstance returns the overlay without one instance and without its
// identity. A name declared again after a delete is a new instance with a new
// UID, so the rows that referenced the old UID do not attach to it.
func withoutInstance(overlay overlayFile, instance ref.Ref) overlayFile {
	next := overlay
	next.Version = overlayFormat
	next.Instances = make([]overlayEntry, 0, len(overlay.Instances))
	next.Identities = copyIdentities(overlay.Identities)

	for _, existing := range overlay.Instances {
		if existing.Kind == instance.Kind().String() && existing.Name == instance.ID() {
			continue
		}

		next.Instances = append(next.Instances, existing)
	}

	delete(next.Identities, instance.Path())

	return next
}

// pruneIdentities drops the identities of instances that neither input
// declares, so the overlay does not keep a row for every name an operator ever
// used.
func pruneIdentities(overlay overlayFile, doc Document) overlayFile {
	next := overlay
	next.Identities = make(map[string]overlayIdentity, len(overlay.Identities))

	for _, declared := range doc.Instances {
		if identity, ok := overlay.Identities[declared.Ref.Path()]; ok {
			next.Identities[declared.Ref.Path()] = identity
		}
	}

	return next
}

func copyIdentities(identities map[string]overlayIdentity) map[string]overlayIdentity {
	next := make(map[string]overlayIdentity, len(identities)+1)
	maps.Copy(next, identities)

	return next
}

// renderOverlay encodes the overlay in one fixed form: indented, ending in a
// newline, with the instances, presets and endpoints sorted. Two writes of the
// same configuration then produce the same bytes, and a diff of the file is
// readable.
func renderOverlay(overlay overlayFile) ([]byte, error) {
	overlay.Version = overlayFormat
	sortEntries(overlay.Instances)
	sortPresets(overlay.Presets)
	sortEndpoints(overlay.Endpoints)

	encoded, err := json.MarshalIndent(overlay, "", jsonIndent)
	if err != nil {
		return nil, err
	}

	return append(encoded, '\n'), nil
}
