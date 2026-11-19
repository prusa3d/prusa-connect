package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/prusa3d/prusa-connect/internal/kernel/condition"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/sdk/extension"
)

// MaxResourceTypeBytes is the longest type name this surface accepts. It is the
// qualified-name grammar's own bound, so every type a module install can write
// is one this API will take back.
const MaxResourceTypeBytes = extension.MaxQualifiedLen

// ResourceStore is the configured inventory: the instances the merged
// configuration declares, and the supervision plane's live view of each one. It
// is declared here because this is where it is consumed.
//
// What implements it reads two inputs and writes one. The hand-authored config
// file belongs to the operator and the API never rewrites it, because no Go
// YAML library writes such a file back without changing lines the operator
// did not touch. Writes go to a machine-owned JSON overlay that the loader
// merges over the file. Source says which of the two declared an instance, and
// an instance the operator declared by hand is refused with
// httpfault.ResourceNotWritable.
//
// The two halves are one interface on purpose. A read that did not carry the
// version a write must quote would leave a caller to guess it, and a write
// with a guessed version can overwrite an edit the operator just saved.
type ResourceStore interface {
	// List returns every instance the caller may see, narrowed to one kind when
	// kind is non-zero. The permission belongs in the query that produces the
	// rows, never in a filter applied to them afterwards.
	List(ctx context.Context, principal string, kind ref.Kind) (ResourceListing, error)

	// Get returns one instance.
	Get(ctx context.Context, principal string, instance ref.Ref) (ResourceInstance, error)

	// Apply writes desired into the overlay and reconciles, returning the
	// instance as it stands afterwards. It validates before it writes: a
	// rejected spec leaves both files untouched.
	//
	// Three refusals belong here and nowhere else, because only the store knows
	// the current state: a failed precondition is httpfault.VersionConflict, a
	// change to an immutable field is httpfault.FieldImmutable, and an instance
	// sourced from the operator's file is httpfault.ResourceNotWritable.
	Apply(
		ctx context.Context,
		principal string,
		instance ref.Ref,
		desired ResourceSpec,
		pre Precondition,
	) (ApplyResult, error)

	// Delete removes an instance from the overlay and reconciles.
	Delete(ctx context.Context, principal string, instance ref.Ref, pre Precondition) error

	// Restart rebuilds one running instance from the declaration in force. It
	// takes operate_resource, which is the level between reading an instance
	// and rewriting it, and it carries no precondition: it writes nothing, so
	// there is no version for a token to fence.
	Restart(ctx context.Context, principal string, instance ref.Ref) error

	// MemoryHistory is what a service module's process used since the given
	// instant, never reaching back further than an hour, with the stops inside
	// that window. It takes view_resource.
	MemoryHistory(
		ctx context.Context,
		principal string,
		instance ref.Ref,
		since time.Time,
	) (ResourceMemoryHistory, error)

	// Log is the newest standard error of a service module. It takes
	// configure_resource on the kind.
	Log(ctx context.Context, principal string, instance ref.Ref) (ResourceLog, error)

	// ModuleMemory is the memory pool and the sum of the enabled service
	// modules' budgets. It checks no permission: the route holds it.
	ModuleMemory() ModuleMemoryPool
}

// ResourceListing is the collection as one artefact. Version is the ETag of the
// listing, and it is the same token every instance under it carries: a write
// rewrites the overlay whole, so the file is the concurrency unit and a
// per-instance token would report success on a write that discarded a sibling.
type ResourceListing struct {
	Items   []ResourceInstance
	Version string
}

// ApplyResult is what a write produced. Created separates the PUT that brought
// an instance into existence, which answers 201 with a Location, from the one
// that replaced an existing declaration.
type ApplyResult struct {
	Instance ResourceInstance
	Created  bool
}

// Precondition is what makes a write safe against the other writer: a person
// with the file open in an editor. The transport never builds one that Fences
// reports false for, so a blind overwrite cannot be expressed.
//
// The fields are the two conditional headers RFC 9110 §13.1 defines, kept
// apart because the RFC compares them differently and evaluates If-Match first
// (§13.2.2). Holds is the comparison, so a store never reimplements it.
type Precondition struct {
	// IfMatch is the entity-tag set from If-Match, compared strongly. Empty
	// when the header was absent.
	IfMatch []string
	// IfMatchAny is If-Match: *, which asks only that the instance exists.
	IfMatchAny bool
	// IfNoneMatch is the entity-tag set from If-None-Match, compared weakly.
	IfNoneMatch []string
	// MustNotExist is If-None-Match: *, which asks that nothing is there yet.
	MustNotExist bool
	// DryRun asks for validation and the resulting instance with nothing
	// written and nothing reconciled.
	DryRun bool
}

// Holds reports whether the state the store has satisfies the
// precondition. version is the instance's version when exists is true and is
// ignored otherwise.
//
// It evaluates If-Match before If-None-Match, as RFC 9110 §13.2.2 requires.
// With both headers present, a client that quoted a stale If-Match must be
// told its token is stale, and not that the instance exists.
func (p Precondition) Holds(exists bool, version string) bool {
	switch {
	case p.IfMatchAny && !exists:
		return false
	case len(p.IfMatch) > 0 && (!exists || !strongMatch(tagsOf(p.IfMatch), version)):
		return false
	case p.MustNotExist && exists:
		return false
	case len(p.IfNoneMatch) > 0 && exists && weakMatch(tagsOf(p.IfNoneMatch), version):
		return false
	default:
		return true
	}
}

// Fences reports whether the precondition carries a fencing token: something
// the caller could only be holding because it read the resource, or something
// that refuses to touch anything already there. The transport refuses a write
// without one, so a store that sees one is being called by something else.
//
// Two of the four fields are preconditions in RFC 9110's sense and are not
// fencing tokens, so this cannot be a count of the headers that arrived:
//
//   - a fabricated If-None-Match tag list matches nothing, so it holds against
//     every state there is. "If-None-Match: i-made-this-up" would let a caller
//     that never read the resource replace it or delete it
//   - If-Match: * asks only that something is there. It says nothing about
//     what, so it overwrites whatever an operator saved a second earlier
//
// Both are still parsed, still published as accepted, and still evaluated by
// Holds. Neither may be the only thing a write carries, so a blind overwrite
// cannot be expressed.
func (p Precondition) Fences() bool {
	return len(p.IfMatch) > 0 || p.MustNotExist
}

// tagsOf lifts the opaque versions a Precondition carries back into the
// comparison functions. The weak flag is already resolved: a weak validator
// never reaches IfMatch, so every tag here compares as strong.
func tagsOf(versions []string) []entityTag {
	tags := make([]entityTag, 0, len(versions))
	for _, version := range versions {
		tags = append(tags, entityTag{value: version})
	}

	return tags
}

// ResourceSpec is what an operator declares: everything the configuration file
// holds for one instance, and nothing the supervision plane observes.
type ResourceSpec struct {
	// Type is the registered implementation this instance is built from, the
	// name an adapter registers itself under. It is fixed at creation: a
	// different implementation cannot inherit this instance's identity, and the
	// data rows keyed by its UID would follow the wrong type.
	//
	// The maxLength in the tag is MaxResourceTypeBytes, which a struct tag
	// cannot spell as a constant. It is the whole qualified name a module
	// install writes, a namespace and a slash and a name, and not the DNS label
	// limit of one half. A lower bound would refuse a type the install path
	// can declare, and its row could be read and never written back.
	// TestThePublishedTypeBoundIsTheQualifiedNameGrammar holds the two together.
	Type string `json:"type" minLength:"1" maxLength:"160" doc:"The registered implementation this instance is built from, either a bare name or a namespace and a name separated by one /. Fixed at creation: changing it is remove and add."`
	// Enabled is the operator's switch. A disabled instance stays in the file
	// and is not started.
	Enabled bool `json:"enabled" doc:"Whether the supervisor starts this instance."`
	// Required says the process must not serve without this instance. It
	// defaults to false: an optional resource that fails degrades one feature,
	// a required one that fails stops the server.
	Required bool `json:"required" doc:"Whether the process refuses to serve while this instance is not ready."`
	// Config is the implementation's own settings, validated by the
	// implementation and opaque to everything here.
	Config ResourceConfig `json:"config" doc:"Settings owned and validated by the implementation named by type."`
	// HiddenContributions are the module UI contributions the operator hides
	// for every viewer, each as <slot key>/<identity>: the slot key is the
	// contributes key (pages, settings, device_card, device_section,
	// device_tab, device_action, file_action, print_section) and the identity
	// is the page path, or the id for every other slot. Dashboard cards are
	// not hidden this way. Hiding is visibility and not access control. An
	// entry the manifest does not declare is kept and has no effect.
	HiddenContributions []string `json:"hidden_contributions,omitempty" maxItems:"64" nullable:"false" doc:"UI contributions of a module hidden for every viewer, each as slot key and identity joined by one /: pages/<path>, or settings, device_card, device_section, device_tab, device_action, file_action or print_section followed by /<id>. Dashboard cards cannot be hidden. At most 64, each at most 200 characters, no duplicates. This hides a menu entry or a block and grants or removes no access: a hidden page still answers a direct link. An entry the module does not declare is kept and has no effect. Omitted or empty hides nothing." example:"[\"device_card/fleet-stats\"]"`
	// MemoryMiB is the operator's memory budget for a service module's
	// process. Zero follows the module's recommendation, or the server
	// default when it makes none.
	MemoryMiB int `json:"memory_mib,omitempty" minimum:"0" maximum:"8192" doc:"The memory budget of a service module's process, in MiB, from 16 to 8192. Zero or omitted follows the module's recommendation, or the server default of 128 MiB when it makes none. Only a service module takes it. The write replaces the whole spec, so a client that leaves it out resets the budget."`
	// OnOverMemory is what the host does when a service module's process goes
	// over its budget. Empty is stop.
	OnOverMemory string `json:"on_over_memory,omitempty" enum:"stop,warn" doc:"What the host does when a service module's process goes over its memory budget: stop kills the process and counts towards switching the module off, warn keeps it running and marks the status over_budget. Omitted is stop. Only a service module takes it. The write replaces the whole spec, so a client that leaves it out resets it to stop."`
	// StagedRoutes is the limit of each staged route the operator approved
	// at install. A write changes the limits and never which routes are
	// staged (ADR-20261120-xys rule 5).
	StagedRoutes []ModuleStagedRoute `json:"staged_routes,omitempty" maxItems:"32" doc:"For a service module, each route whose request body the server writes into the module's data directory, with the largest body it takes, from 1 to 4096 MiB. The operator approves each route at install, and a write may change only the limit of a route listed here: a route it does not list keeps its limit, and a route the install did not approve is refused. Changing a limit restarts the module."`
	// SwitchOff is the operator's own switch-off for a service module. Nil
	// follows the server's core.modules_switch_off.
	SwitchOff *ResourceSwitchOffPolicy `json:"switch_off,omitempty" doc:"When the host switches a service module off because it keeps failing. A key left out follows the server's core.modules_switch_off. Only a service module takes it. The write replaces the whole spec, so a client that leaves it out returns the module to the server's setting."`
}

// ResourceSwitchOffPolicy is how many failed starts or stops inside how long a
// window switch a service module off.
type ResourceSwitchOffPolicy struct {
	Failures      *int `json:"failures,omitempty"       minimum:"0" maximum:"100"   doc:"How many failed starts or stops inside the window switch the module off, from 0 to 100. 0 means the host never switches it off for failing. An optional module whose start keeps failing still gives up after an hour and five attempts. Omitted follows the server."`
	WindowSeconds int  `json:"window_seconds,omitempty" minimum:"0" maximum:"86400" doc:"How far back failed starts and stops count, in seconds, from 60 to 86400. 0 or omitted follows the server."`
}

// ResourceInstance is one configured instance: what the operator declared, the
// version a write must quote, and what the supervision plane observes.
type ResourceInstance struct {
	ResourceSpec

	// Kind and Name identify the instance. They are the path, and they are in
	// the body too so a listing is usable without reparsing links.
	Kind string `json:"kind" doc:"The resource kind: what the instance plugs into."`
	Name string `json:"name" doc:"The operator-chosen name, unique within the kind."`

	// UID is the host-minted identity data rows reference. A rename changes
	// Name and never this, and nothing changes it after creation.
	UID string `json:"uid" doc:"Host-minted identity. Data rows reference it, and nothing changes it after creation."`

	// Runtime says how the type is reached, which is what separates a module
	// in its own process from a type compiled into this binary. A client reads
	// it here and does not infer it from a slash in the type.
	Runtime string `json:"runtime" enum:"builtin,service,script,asset" doc:"How the type is reached: compiled into this binary, launched as a child process, a script the host runs, or a directory of files the host verifies and serves."`

	// Namespace is the mount a module's routes are served under, derived from
	// its type, and absent for an instance that is not a module.
	Namespace string `json:"namespace,omitempty" doc:"For a module, the namespace its routes are served under at /api/v1/modules/{namespace}/, and for an asset module the namespace its files are served under at /api/v1/modules/{namespace}/ui/. Absent otherwise."`

	// Mount is the root prefix an operator named for the module's routes, and
	// absent for a module reached under its namespace and for everything that
	// is not a module. A client reads it here and does not guess where a
	// GraphQL or compatibility surface was mounted.
	Mount string `json:"mount,omitempty" doc:"For a module the operator mounted at a root prefix such as /graphql, that prefix: its routes are served there instead of under /api/v1/modules/{namespace}/. Absent otherwise."`

	// OpenAPIURL is where the module's own OpenAPI document is served, and
	// absent for an instance that is not a running module or ships none. It
	// is what a client generator reads to type the module's routes.
	OpenAPIURL string `json:"openapi_url,omitempty" doc:"For a running module that ships an OpenAPI document, the path it is served at. Absent otherwise."`

	// UI is the browser half of a module, and absent for every instance that
	// has none. It sits beside OpenAPIURL because it answers the same question
	// for the other surface a module can offer.
	UI *ResourceUI `json:"ui,omitempty" doc:"For an enabled and ready module whose manifest carries a ui block, what the frontend needs to load it. Absent otherwise."`

	// Module is what the module's own manifest said about itself when it was
	// installed. It is absent for an instance that is not a module, and for a
	// declaration whose install recorded none.
	Module *ResourceModule `json:"module,omitempty" doc:"For a module, what its manifest said about itself: the release, one line about it, and who answers for it. Absent otherwise."`

	// Source says which of the two inputs declared this instance. An instance
	// from the operator's file is not writable through the API, and a client
	// reads this before it tries a write that would be refused.
	Source string `json:"source" enum:"file,overlay" doc:"Which input declared this instance. Only overlay is writable through the API."`

	// Version is the fencing token for this instance's configuration. It is the
	// ETag of a GET and what a write quotes in If-Match. It covers the whole
	// overlay, not this instance alone, because a write rewrites that file whole.
	// A write to any instance moves every instance's token.
	Version string `json:"version" doc:"Fencing token, covering the whole overlay. Quote the ETag in If-Match on a write."`

	// Generation counts accepted changes to the declared configuration. Status
	// carries the generation it was observed at, which is how a caller tells a
	// settled instance from one still converging.
	Generation int64 `json:"generation" doc:"Counts accepted changes to the declaration."`

	Status ResourceStatus `json:"status" doc:"What the supervisor has observed of this instance, written by the server and ignored on a write."`
}

// ResourceModule is what a marketplace card shows about an installed module:
// the release it is running, its title, one line saying what it does, and who
// answers for it. They come from the module's own module.yaml and are recorded
// in the declaration by the install, together with what the install granted.
//
// The release is here and not beside version on the instance, because version
// there is the fencing token a write quotes in If-Match, and one word would
// then mean two things.
//
// Every member is optional. A manifest need not carry a title, a summary or a
// codeowner, and a declaration whose install recorded none carries them empty.
type ResourceModule struct {
	// Asked is what the install granted: what the release asked for when an
	// operator approved it. Absent when the install recorded none.
	Asked *ModuleRequests `json:"asked,omitempty" doc:"What the install granted: the permissions, events and records room the release asked for when an operator approved it. Absent for an install without approval, which leaves the module switched off; switching such a module on starts it with nothing granted."`
	// Title is the name a person reads, from the manifest's title key.
	Title string `json:"title,omitempty" doc:"The name a person reads, from the manifest's title key. Absent when the manifest names none; the instance name stands in." example:"Spool tracker"`
	// Version is the release the manifest declared, MAJOR.MINOR.PATCH.
	Version string `json:"version,omitempty" doc:"The release this module's manifest declared when it was installed." example:"1.4.0"`
	// Summary is the one line the manifest's summary key carries.
	Summary string `json:"summary,omitempty" doc:"One sentence saying what the module does, from its manifest." example:"Counts what each print used and warns before a spool runs out."`
	// Author is who answers for the module, from the manifest's codeowner key.
	Author string `json:"author,omitempty" doc:"Who answers for this module, from its manifest's codeowner key." example:"acme"`
	// Requires is what the module cannot run without. The server checks every
	// entry before it starts the instance, so an unmet one is also a condition
	// on the status beside this list.
	Requires []ResourceRequirement `json:"requires,omitempty" doc:"What the module declared it cannot run without. The server checks each entry before it starts the instance. An entry on another module carries the release this server has of it."`
	// Suggests is the modules this one works better with.
	Suggests []ResourceRequirement `json:"suggests,omitempty" doc:"The modules this module works better with, from its manifest's suggest, each with the reason. A suggestion installs nothing."`
	// Conflicts is the releases this module refuses to run beside.
	Conflicts []ResourceRequirement `json:"conflicts,omitempty" doc:"The releases of other modules this module refuses to run beside, from its manifest's conflict, each with the constraint and the release this server has."`
	// Pulled is true for a module an install added because another module
	// required it.
	Pulled bool `json:"pulled,omitempty" doc:"True when an install added this module because another module requires it, and false when the operator asked for it."`
	// RequiredBy is the installed modules that require this one.
	RequiredBy []ModuleDependant `json:"required_by,omitempty" doc:"The installed modules that require this one, by instance and range. While any is installed this module cannot be removed, and while any is switched on it cannot be switched off."`
	// Options is the settings the module reads, with the default of each. It
	// is the module's own default configuration: the server applies every
	// default, writes this instance's config block over it key by key, and
	// sends the result to the module.
	Options []ResourceOption `json:"options,omitempty" doc:"The settings this module reads, each with its type, its default and what it does. The config block of this instance overrides a key only where it writes that key."`
	// Listening is the ports the declaration grants, which may differ from
	// the ones the manifest asked for.
	Listening []ModuleListen `json:"listening,omitempty" doc:"The ports this instance may listen on now, by the name its manifest gave each, with the reason from the manifest. A port the operator moved carries the number it was moved to. Absent when no port is granted."`
	// AllowedOrigins is the browser origins the declaration grants.
	AllowedOrigins []string `json:"allowed_origins,omitempty" doc:"The browser origins this module adds to the ones the API answers. A page served from one may call the API from a browser while this instance is switched on. Absent when none is granted." maxItems:"8" nullable:"false"`
	// Database is the database of its own the declaration grants, with no
	// credential in it.
	Database *ResourceDatabase `json:"database,omitempty" doc:"The database of its own this instance was granted. Absent when it was granted none."`
}

// ResourceDatabase is where a module's own database is, without the
// credential the module logs in with.
type ResourceDatabase struct {
	Engine      string `json:"engine"                doc:"sqlite for a file in the module's data directory, postgres for a database on a Postgres server."                                  enum:"sqlite,postgres"`
	Provisioned bool   `json:"provisioned,omitempty" doc:"Whether this server made the database and its role on its own Postgres server."`
	Server      string `json:"server,omitempty"      doc:"For postgres, the server's host and port and the database name, as host:port/name. The role and its password are never answered."                        example:"db.lan:5432/module_notes"`
	// Schema is what the running module reported about its database after
	// it migrated it. The host never opens the module's database itself.
	Schema *ResourceDatabaseSchema `json:"schema,omitempty" doc:"The schema the database holds, as the running module reported it after it migrated the database at its last start. Absent while the module is not running, and for a module that reports none."`
}

// ResourceDatabaseSchema is the schema a module's own database holds.
type ResourceDatabaseSchema struct {
	Version int      `json:"version"           doc:"The highest schema version the database holds."                                                                                                 minimum:"0"`
	Compat  int      `json:"compat"            doc:"The compat floor the database records: a release whose highest schema version is below it refuses the database."                                minimum:"0"`
	Applied []string `json:"applied,omitempty" doc:"The migrations the module applied at its last start, in order, by the name its ledger records. Empty when the database was already up to date."`
}

// ResourceOption is one setting a module reads out of its config block, as its
// manifest declared it.
type ResourceOption struct {
	// Key is the key as an operator writes it, in lower_snake_case.
	Key string `json:"key" doc:"The key as an operator writes it under this instance's config block." example:"flush_interval"`
	// Type is the shape of the value.
	Type string `json:"type" doc:"The shape of the value: string, int, number, bool, duration, url, secret, list or object." example:"duration"`
	// Required says the module does not start until the operator writes it.
	Required bool `json:"required,omitempty" doc:"The module does not start until this key is written. A required key has no default."`
	// Default is what the key is worth when the config block leaves it out,
	// written the way an operator would write it. It is empty for a required
	// key and for a secret, whose default is never published.
	Default string `json:"default,omitempty" doc:"What this key is worth when the config block leaves it out. Empty for a required key, and for a secret, whose value is never published." example:"5s"`
	// Summary is one line saying what the key does.
	Summary string `json:"summary" doc:"One line saying what this key does." example:"How often the readings collected since the last report go up."`
}

// ResourceRequirement is one thing a module needs, as its manifest declared
// it. Exactly one of capability, core and module is set.
type ResourceRequirement struct {
	// Capability is a wire method the server must serve the instance.
	Capability string `json:"capability,omitempty" doc:"A wire method the server must serve this instance." example:"device.list"`
	// Core is the range of server versions the module runs on.
	Core string `json:"core,omitempty" doc:"The range of server versions this module runs on." example:">=1.2.0"`
	// Module is another module, by its namespace.
	Module string `json:"module,omitempty" doc:"Another module this one depends on, by its namespace." example:"com.github.acme.threemf"`
	// Version is the range of that module's releases.
	Version string `json:"version,omitempty" doc:"The constraint on that module's releases, in Composer's grammar." example:"^1.2"`
	// Header is a request header the module cannot work without.
	Header string `json:"header,omitempty" doc:"A request header the module marked required. The instance does not start while its service block does not grant it." example:"Print-After-Upload"`
	// Reason is why a module is suggested.
	Reason string `json:"reason,omitempty" doc:"For a suggestion, why the module is suggested, from the manifest."`
	// Met says whether the dependency is met.
	Met bool `json:"met,omitempty" doc:"For a module, whether this server meets the entry now: a required module is installed, switched on and in its constraint, and a conflicting one is absent or out of its constraint."`
	// Installed is the release this server has of that module.
	Installed string `json:"installed,omitempty" doc:"The release this server has of that module. Absent when it is not installed, or when the caller may not see it." example:"1.4.0"`
	// Instance is that module's instance, as kind:name.
	Instance string `json:"instance,omitempty" doc:"The instance of that module, as kind:name. Absent with installed." example:"sink:threemf"`
	// State is that instance's state.
	State string `json:"state,omitempty" doc:"The state of that instance, such as ready or failed. Absent with installed." example:"ready"`
	// Since is the contract version that introduced the requirement.
	Since string `json:"since,omitempty" doc:"The contract version that introduced the requirement, when the module said." example:"1.0.0"`
}

// ModuleDependant is one installed module that requires another.
type ModuleDependant struct {
	Instance string `json:"instance" doc:"The instance of the module that requires this one, as kind:name."                                                example:"sink:planner"`
	Module   string `json:"module"   doc:"Its namespace."                                                                                                  example:"com.github.acme.planner"`
	Version  string `json:"version"  doc:"The range of this module's releases it requires."                                                                example:"^1.2"`
	Enabled  bool   `json:"enabled"  doc:"Whether that module is switched on. While one that is requires this module, this module cannot be switched off."`
}

// ResourceUI is what the frontend needs to load one module's browser bundle:
// the entry module's address, the range of the library the bundle compiled
// against, and the pages and sections it may register.
//
// It appears for an enabled and ready instance only. A disabled instance has
// no verified bundle behind it, and an entry address published for one would
// be a 404 the frontend reaches at import time.
//
// There is no signature field. The install record carries a digest and no
// signing level (see install.Asset), so nothing here could say which level a
// bundle was verified at.
type ResourceUI struct {
	// EntryURL is where the bundle's entry module is served, an absolute path
	// under the module's own prefix. The frontend imports it as an ES module.
	EntryURL string `json:"entry_url" doc:"Where the bundle's entry module is served. Import it as an ES module."`
	// Library is the range of the component library the bundle compiled
	// against, as the manifest declared it. The frontend checks its own
	// version against it before the import.
	Library string `json:"library" doc:"The range of @prusa/connect-ui the bundle compiled against, as its manifest declared it."`
	// Contributes is the ceiling on what the bundle may register, as the
	// manifest declared it. A registration outside it is refused.
	Contributes ResourceContributes `json:"contributes" doc:"What the bundle may register. A registration outside it is refused."`
	// Hidden is what the running module hides of its own entries, in the form
	// hidden_contributions takes. The module states it over the wire and the
	// host keeps it in memory, so it is empty after every start until the
	// module states it again. It is separate from the operator's
	// hidden_contributions, and it cannot show an entry the operator hid.
	// Exports and Imports are the components the bundle hands other bundles
	// and draws from them.
	Exports []ResourceUIExport `json:"exports,omitempty" doc:"The components this bundle hands other modules' bundles."                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   maxItems:"16" nullable:"false"`
	Imports []ResourceUIImport `json:"imports,omitempty" doc:"The components of other modules this bundle draws."                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         maxItems:"16" nullable:"false"`
	Hidden  []string           `json:"hidden,omitempty"  doc:"The entries of its own interface the running module hides now, each as slot key and identity joined by one /, in the form hidden_contributions takes: pages/<path>, or settings, device_card, device_section, device_tab, device_action, file_action or print_section followed by /<id>. The module states it while it runs, such as when a feature of its own is switched off, and it is empty after each start until the module states it again. A client hides an entry that is here or in hidden_contributions. Like those, a hidden page still answers a direct link. Omitted or empty hides nothing." maxItems:"64" nullable:"false" example:"[\"pages/customers\"]"`
}

// ResourceUIExport is one component a bundle hands other bundles.
type ResourceUIExport struct {
	Name string `json:"name" doc:"The component's name, one DNS label." example:"spool-picker" maxLength:"63" minLength:"1"`
}

// ResourceUIImport is one component of another module a bundle draws.
type ResourceUIImport struct {
	Module string `json:"module" doc:"The namespace of the module that exports the component." example:"com.github.aux.acme.spools" minLength:"1"`
	Name   string `json:"name"   doc:"The component's name in that module."                    example:"spool-picker"               minLength:"1" maxLength:"63"`
}

// ResourceAction is one item a module adds to an action menu. The host draws
// it from these fields before the bundle loads, and the bundle registers only
// what runs.
type ResourceAction struct {
	ID          string `json:"id"             doc:"The action's name within its list, one DNS label."                 maxLength:"63" minLength:"1"`
	Title       string `json:"title"          doc:"What the menu item reads."                                                        minLength:"1"`
	Icon        string `json:"icon,omitempty" doc:"The icon the item draws, and absent for none."`
	Destructive bool   `json:"destructive"    doc:"Whether the item is drawn as one that deletes or stops something."`
	// FileTypes limits an entry of a slot handed a file.
	FileTypes []string `json:"file_types,omitempty" doc:"For a slot handed a file, the file types the entry is drawn for: a lowercase extension with its dot, such as .3mf. Absent for every file." maxItems:"16" nullable:"false"`
	// Primary draws the action as a button on its subject's page.
	Primary bool `json:"primary,omitempty" doc:"Whether the action is a button in the header of its subject's own page rather than an item in its menu. Only file-action entries carry it."`
	// Devices limits an entry of a slot handed a device.
	Devices *ResourceDeviceSelector `json:"devices,omitempty" doc:"On a device slot, the devices the entry is drawn for. Absent for every device."`
}

// ResourceDeviceSelector is the condition a device's record has to meet for
// an entry of a slot handed a device to be drawn on it. Every key it carries
// has to match.
type ResourceDeviceSelector struct {
	Resident bool `json:"resident,omitempty" doc:"True: the entry is drawn only on a device whose record has resident: true, which is a printer this server runs on. Absent when the selector names no such condition. A client draws no entry whose selector carries a key or a value it cannot read."`
}

// ResourcePage is one screen a module adds. The host builds the route from the
// module's namespace and this path, so the manifest names no address.
type ResourcePage struct {
	Path   string `json:"path"           doc:"The last segment of the route, one DNS label."                       maxLength:"63" minLength:"1"`
	Title  string `json:"title"          doc:"What the sidebar entry and the page heading read."                                  minLength:"1"`
	Icon   string `json:"icon,omitempty" doc:"The icon the sidebar entry draws, and absent for the default."`
	Hidden bool   `json:"hidden"         doc:"Whether the page is kept out of the sidebar. Its route still works."`
}

// ResourceSection is one device-card section or settings pane a module fills.
type ResourceSection struct {
	ID    string `json:"id"    doc:"The section's name within its list, one DNS label."  maxLength:"63" minLength:"1"`
	Title string `json:"title" doc:"What the section heading or the settings tab reads."                minLength:"1"`
	// FileTypes limits an entry of a slot handed a file.
	FileTypes []string `json:"file_types,omitempty" doc:"For a slot handed a file, the file types the entry is drawn for: a lowercase extension with its dot, such as .3mf. Absent for every file." maxItems:"16" nullable:"false"`
	// Devices limits an entry of a slot handed a device.
	Devices *ResourceDeviceSelector `json:"devices,omitempty" doc:"On a device slot, the devices the entry is drawn for. Absent for every device."`
	// CrossOriginIsolated is the manifest's cross_origin_isolated.
	CrossOriginIsolated bool `json:"cross_origin_isolated,omitempty" doc:"On a printer tab, true when the module's manifest asks for the tab's page to be cross-origin isolated. The server then sends that page with both isolation headers unless the tab is hidden, and a client opens and leaves the tab with a full page load. Absent otherwise."`
}

// ResourceCard is one card a module offers for a dashboard. The page draws the
// card picker and the settings form from it before the bundle loads.
type ResourceCard struct {
	ID          string                `json:"id"             doc:"The card's name within its list, one DNS label. Its type on a dashboard is the namespace, a slash and this id." maxLength:"63" minLength:"1"`
	Title       string                `json:"title"          doc:"What the card picker and the card's default heading read."                                                                     minLength:"1"`
	Description string                `json:"description"    doc:"One sentence the card picker shows under the title."                                                                           minLength:"1"`
	Icon        string                `json:"icon,omitempty" doc:"The icon the card picker draws, and absent for the default."`
	Size        ResourceCardSize      `json:"size"           doc:"The size a new card starts at, in cells of a 12-column grid."`
	Least       ResourceCardSize      `json:"least"          doc:"The smallest the card may be made, in cells of a 12-column grid."`
	Settings    []ResourceCardSetting `json:"settings"       doc:"The schema of the card's settings, in the order the form shows them."`
}

// ResourceCardSize is a size in cells of a 12-column grid.
type ResourceCardSize struct {
	W int `json:"w" doc:"How many columns the card covers." maximum:"12" minimum:"1"`
	H int `json:"h" doc:"How many rows the card covers."    maximum:"32" minimum:"1"`
}

// ResourceCardSetting is one field of a card's settings schema.
type ResourceCardSetting struct {
	Key      string                  `json:"key"                 doc:"The name the value is kept under in the card's settings."`
	Kind     string                  `json:"kind"                doc:"What the field holds, and which control the form draws."                                                                                   enum:"printer,printers,tag,camera,choice,choices,number,text,note,switch"`
	Label    string                  `json:"label"               doc:"What the form shows beside the control."`
	Default  any                     `json:"default"             doc:"The value a new card starts with, and the value a stored one that fails the check takes: text, a list of text, a number or true or false."`
	Options  []ResourceSettingOption `json:"options,omitempty"   doc:"The values of a choice or choices field, in the order the form shows them."`
	Min      *float64                `json:"min,omitempty"       doc:"The lowest value of a number field."`
	Max      *float64                `json:"max,omitempty"       doc:"The highest value of a number field."`
	Step     *float64                `json:"step,omitempty"      doc:"The step between two values of a number field."`
	MaxBytes int                     `json:"max_bytes,omitempty" doc:"The longest a text or note field may be, in UTF-8 bytes."`
}

// ResourceSettingOption is one value of a choice or choices field.
type ResourceSettingOption struct {
	Value string `json:"value" doc:"What the card's settings keep."`
	Label string `json:"label" doc:"What the form shows for it."`
}

// ResourceStatus is the supervision plane's view, projected onto the wire. The
// fields are supervisor.Status and no others: this transport may not import the
// extension group, so the mapping lives in the composition root and this is
// the shape it maps onto.
//
// There is no breaker field because there is no breaker axis: an open breaker
// is the BreakerClosed condition reading False, which is also how every other
// independent axis is reported.
type ResourceStatus struct {
	// State is derived from the conditions and the host's lifecycle position,
	// never assigned. It is the one field a badge reads.
	State string `json:"state" enum:"disabled,orphaned,pending,starting,ready,degraded,retrying,failed,stopping,stopped" doc:"The derived state. Read conditions for why."`
	// Serving reports whether calls are routed to this instance. Degraded
	// serves: available and degraded are independent axes.
	Serving bool `json:"serving" doc:"Whether calls are being routed to this instance."`
	// Result says why the instance last stopped or failed.
	Result string `json:"result,omitempty" doc:"Why the instance last stopped or failed."`
	// ObservedGeneration is the declaration generation this status was computed
	// from. Lower than generation means the change has not taken effect yet.
	ObservedGeneration int64 `json:"observed_generation" doc:"The declaration generation this status was computed from."`
	// Attempt is how many starts the current retry ladder has made.
	Attempt int `json:"attempt" doc:"Starts made by the current retry ladder."`
	// NextRetryAt is when the next attempt is due, absent when none is
	// scheduled. No attempt scheduled and none in flight means the failure is
	// permanent.
	NextRetryAt *time.Time `json:"next_retry_at,omitempty" doc:"When the next start attempt is due."`
	// Dropped is how many offered items this instance's queue has lost.
	Dropped int64 `json:"dropped" doc:"Items the supervisor's queue has dropped for this instance."`
	// Conditions are the independent axes, in first-write order.
	Conditions []ResourceCondition `json:"conditions" doc:"The independent axes of the instance's health, in the order they were first written."`
	// Memory is the service module's memory budget and what its process does
	// with it. It is absent for every other instance.
	Memory *ResourceMemory `json:"memory,omitempty" doc:"The memory budget of a service module and how its process uses it. Absent for every other instance."`
}

// ResourceMemory is a service module's memory budget and what the host saw
// its process do with it.
type ResourceMemory struct {
	BudgetBytes      int64              `json:"budget_bytes"           doc:"The memory the module's process may use, in bytes."`
	BudgetSource     string             `json:"budget_source"          doc:"Who set the budget: the operator with memory_mib, the module's author in its manifest, or the server default."        enum:"default,author,operator"`
	UsedBytes        *int64             `json:"used_bytes,omitempty"   doc:"The memory the process uses now, in bytes. Absent when the host cannot read it."`
	NearBudget       bool               `json:"near_budget"            doc:"Whether the process is close to its budget."`
	OnOverMemory     string             `json:"on_over_memory"         doc:"What the host does over the budget, as in force: stop or warn."                                                       enum:"stop,warn"`
	OverBudget       bool               `json:"over_budget"            doc:"Whether the last reading was over the budget. Only a module with on_over_memory warn keeps running in this state."`
	FailuresInWindow int                `json:"failures_in_window"     doc:"How many failed starts and stops count against the module inside the switch-off window of switch_off_window_seconds."`
	LastStop         *ResourceStop      `json:"last_stop,omitempty"    doc:"Why the process last stopped. Absent when it has not stopped."`
	SwitchedOff      *ResourceSwitchOff `json:"switched_off,omitempty" doc:"Present when the host switched the module off because it kept stopping."`
	HeldUntilRestart bool               `json:"held_until_restart"     doc:"Whether the module stays off until the server restarts."`
	// The switch-off in force, every value resolved.
	SwitchOffFailures      int    `json:"switch_off_failures"       doc:"How many failed starts or stops inside the window switch the module off, as in force. 0 never switches it off."`
	SwitchOffWindowSeconds int    `json:"switch_off_window_seconds" doc:"How far back failed starts and stops count, in seconds, as in force."`
	SwitchOffSource        string `json:"switch_off_source"         doc:"Who set the switch-off: the operator with switch_off on the spec, or the server default."                       enum:"default,operator"`
}

// ResourceStop is one stop of a module's process, mapped from supervisor.Stop.
type ResourceStop struct {
	Result      string    `json:"result"                 doc:"Why the process stopped."`
	At          time.Time `json:"at"                     doc:"When it stopped."`
	UsedBytes   *int64    `json:"used_bytes,omitempty"   doc:"The memory the process used when it was stopped, in bytes. Present for over-memory only."`
	BudgetBytes *int64    `json:"budget_bytes,omitempty" doc:"The budget it went over, in bytes. Present for over-memory only."`
	Code        *int      `json:"code,omitempty"         doc:"The exit code. Present for exited only."`
	Signal      string    `json:"signal,omitempty"       doc:"The signal that ended the process, such as SIGSEGV. Present for signal only."`
}

// ResourceSwitchOff is the host switching a module off, and the stops that
// made it do so.
type ResourceSwitchOff struct {
	At    time.Time      `json:"at"    doc:"When the host switched the module off."`
	Stops []ResourceStop `json:"stops" doc:"The stops that made the host switch it off, oldest first." nullable:"false"`
}

// MarshalJSON writes no stops as an empty list, because the schema does not
// allow null there.
func (s ResourceSwitchOff) MarshalJSON() ([]byte, error) {
	type plain ResourceSwitchOff

	if s.Stops == nil {
		s.Stops = []ResourceStop{}
	}

	return json.Marshal(plain(s))
}

// ResourceCondition is condition.Condition on the wire, field for field.
// TestTheConditionProjectionCoversTheKernelType fails when the kernel type
// grows a field this one does not carry.
type ResourceCondition struct {
	Type               string    `json:"type"                 doc:"The axis this condition reports."`
	Status             string    `json:"status"               doc:"Whether the axis holds: True, False, or Unknown when nothing has observed it yet." enum:"True,False,Unknown"`
	Reason             string    `json:"reason"               doc:"A CamelCase token a program branches on. Always present."`
	Message            string    `json:"message,omitempty"    doc:"The human sentence, when there is one."`
	LastTransitionTime time.Time `json:"last_transition_time" doc:"When the status last changed, not when it was last written."`
	ObservedGeneration int64     `json:"observed_generation"  doc:"The instance generation this condition was written from."`
}

// NewResourceCondition projects a kernel condition onto the wire shape.
func NewResourceCondition(c condition.Condition) ResourceCondition {
	return ResourceCondition{
		Type:               c.Type.String(),
		Status:             c.Status.String(),
		Reason:             c.Reason,
		Message:            c.Message,
		LastTransitionTime: c.LastTransitionTime,
		ObservedGeneration: c.ObservedGeneration,
	}
}

// ResourceConfig is an implementation's own settings block. The core never
// parses it: an adapter owns its schema, validates it, and is the only thing
// that can say whether it is right.
type ResourceConfig json.RawMessage

// MarshalJSON writes the settings through unchanged, and writes an empty object
// for an instance that declares none.
//
// It is an empty object and not null because the member is published as a
// required object and a write refuses null. A row read with null in it could
// not be sent back: a client that reads a row, changes enabled and puts it
// back would get a 422 on a member it never touched.
func (c ResourceConfig) MarshalJSON() ([]byte, error) {
	if len(c) == 0 {
		return []byte("{}"), nil
	}

	return c, nil
}

// UnmarshalJSON keeps the settings as they arrived, so a member this binary
// does not understand survives a read-modify-write by an older client.
func (c *ResourceConfig) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("httpapi: UnmarshalJSON on nil ResourceConfig")
	}

	*c = append((*c)[0:0], data...)

	return nil
}

// Schema describes the settings as an opaque object. Reflection would
// describe a raw message as a byte string.
func (ResourceConfig) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: huma.TypeObject,
		Description: "Settings owned by the implementation named by type. The core stores and returns them " +
			"unchanged; the implementation validates them and is the only thing that can reject them.",
		AdditionalProperties: true,
	}
}

// resourceKinds renders the kinds this surface accepts for the path
// parameter's enum, so the document and ref cannot disagree.
//
// It is SupervisedKinds and not ConfigurableKinds. A gcode preset is
// configuration this server holds and carries a resource-kind grant, and it
// names no implementation to build. It is written through its own operations
// and refused here, so it never reaches a supervision plane with a type
// nothing answers to.
func resourceKinds() string {
	names := make([]string, 0, len(ref.SupervisedKinds()))
	for _, kind := range ref.SupervisedKinds() {
		names = append(names, kind.String())
	}

	return strings.Join(names, ",")
}
