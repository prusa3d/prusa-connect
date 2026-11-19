// Package install reads the install mode of every resource instance declared
// in the operator's file: whether the instance runs at all, and how its type is
// reached.
//
// # Two states, not three
//
// One could expect disabled, runtime and build. Build is not a state. An
// artefact either carries a type or it does not, and no per-instance key can
// change that after the binary is linked. A small device cannot rebuild the
// binary. So the states are the two the enabled switch expresses, and build is
// derived by cmd/detect-build-tags from the same declarations.
//
// Runtime is a different question from enabled. Enabled says whether the
// instance runs; runtime says what is behind it: a type compiled into this
// binary, a type launched as a child process, an expression the operator wrote
// here for the host to compile, or a directory of files the host serves.
// Runtime sits beside type because it answers a question about this
// instance's type. It is not a third switch.
//
// # What this package does not own
//
// internal/app/resource owns the runtime decode of the resources block, the
// overlay, the merge and the identities. This package reads the same block for
// the install keys alone, because the build-tag detector has to answer the same
// question before a binary exists to run the other loader.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/extension/script"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/sdk/busapi"
	"github.com/prusa3d/prusa-connect/sdk/event"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
)

// ErrInvalid reports a declaration this package will not accept. Callers
// inspect it with errors.Is. The message names the file and the key, so an
// operator can find the line.
var ErrInvalid = errors.New("install: invalid declaration")

// Runtime is how an instance's type is reached.
//
// The vocabulary is internal/kernel/ref's, because it is published beside the
// resource kinds and generated from that package. This package holds the rules
// a declaration is refused against.
type Runtime = ref.Runtime

const (
	// RuntimeBuiltin is a type compiled into this binary, resolved through the
	// registry. It is the default, because every type in this tree is one.
	RuntimeBuiltin = ref.RuntimeBuiltin
	// RuntimeService is a type reached as a supervised child process over the
	// socket. The binary is not in this artefact.
	RuntimeService = ref.RuntimeService
	// RuntimeScript is an expression the operator wrote in this file, compiled
	// by the host and run over every event. The script is the implementation.
	RuntimeScript = ref.RuntimeScript
	// RuntimeAsset is a directory of files the host verifies and serves and
	// never runs. Only the asset kind offers it.
	RuntimeAsset = ref.RuntimeAsset
)

// ScriptType is the type name a script instance carries. A script runtime and
// a script type always come together, and a declaration with only one of the
// two is refused.
const ScriptType = "script"

// State is whether a declared instance runs. There are two, and build is not
// one of them.
type State string

const (
	// StateDisabled is an instance the operator declared and switched off. It
	// keeps its configuration, its identity and its API path.
	StateDisabled State = "disabled"
	// StateRuntime is an instance the operator declared and switched on.
	StateRuntime State = "runtime"
)

// String returns the state as it is reported.
func (s State) String() string { return string(s) }

// checksumPattern is the digest form a service artefact is recorded under. The
// digest is recorded at install and verified before every launch, and a
// mismatch is a refusal.
var checksumPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Service is where a child process is found and what proves it is the one that
// was installed. It is present only when Runtime is RuntimeService.
type Service struct {
	// Path is the directory the host scans for the binary, which is named
	// <name>_<GOOS>_<GOARCH>.
	Path string
	// Checksum is sha256:<64 hex> over the bytes, compared before every launch.
	Checksum string
	// ContractMajor is the wire major the child speaks, refused on mismatch at
	// the handshake. There is no default: a module must say which contract it
	// implements.
	ContractMajor int
	// Events is what this instance may publish on the host's own event stream:
	// the endpoint names a module selects by, against the event types the host
	// publishes for them.
	//
	// The operator writes the name after the namespace and the host writes the
	// namespace. The value here is the whole type EventTypeOf composed of the
	// two, and EventNameOf takes the operator's name back out of it. Because
	// the host writes the namespace, a module cannot publish into a namespace
	// it does not own.
	//
	// An absent or empty table is an instance that may publish nothing, which
	// is the default. The host then installs no emit method, so the module
	// reads CodeMethodNotFound.
	Events map[string]string
	// Permissions is what this instance may do as itself on the host's fleet
	// and event stream. The module declares the actions it needs for work with
	// no person behind it, and the operator writes them here at install. It is
	// a ceiling and never a grant. A call the module makes as itself is
	// refused unless its action is listed here, and then the authorization
	// graph decides it for the instance's own principal.
	//
	// An absent or empty list is an instance that may do nothing as itself,
	// which is the default. A call it makes for a person it is serving is that
	// person's and is not held to this list.
	Permissions []authz.Action
	// Checks is the actions the instance may ask access.check about, for a
	// person it serves, without taking them as itself. The operator writes it
	// at install. It grants no action, so the install writes no grant for it.
	// A module that asks whether a person may operate a printer needs this.
	Checks []authz.Action
	// CheckAbsentPeople lets the instance ask access.check about a person it
	// is not serving, which a queue does for the owner of an entry after a
	// restart. The operator writes it at install. It grants no action: the
	// question is still held to Permissions, and the host reads the person's
	// rights again for every question.
	CheckAbsentPeople bool
	// ListPeople lets the instance list the people this server holds, with
	// their names, which a module does to offer a list to assign work to. The
	// operator writes it at install. It grants no action.
	ListPeople bool
	// Calls is the namespaces of the other modules whose routes the instance
	// may call with routes.call. The operator writes it at install. It grants
	// no action: a call in a person's name is held to that person's grants,
	// and a call as the module runs as the instance.
	Calls []string
	// PublicRoutes is the routes the host sends to the module without
	// resolving a caller. The operator writes each one at install, from the
	// manifest's routes.public, and may take one away (ADR-20261120-xys rules 1
	// to 3). A route the list does not hold takes an authorized caller.
	PublicRoutes []manifest.Route
	// StagedRoutes is the routes whose request body the host writes into the
	// module's data directory, each with the largest body it takes. The
	// operator approves each one at install, from the manifest's
	// routes.staged, and may change its limit later (ADR-20261120-xys rules 4
	// to 6). A route the list does not hold gets its body on the wire.
	StagedRoutes []StagedRoute
	// RequestHeaders is the request headers the host passes to the module
	// beside the ones every module gets, each in its canonical form. The
	// operator approves each one at install, from the manifest's
	// routes.headers, and may take one away (ADR-20261118-nnm).
	RequestHeaders []string
	// RecordsMiB is the room the operator granted the module's record
	// namespace, in mebibytes. Zero is no grant, and the namespace then holds
	// stateapi.DefaultNamespaceBytes. The operator writes it at install when
	// the manifest asks for room, and may change it later.
	RecordsMiB int
	// MemoryMiB is the memory budget the operator set for the module's
	// process, in mebibytes. Zero follows the manifest's recommendation, or
	// the server default when there is none.
	MemoryMiB int
	// OnOverMemory is what the host does when the process goes over its
	// budget: OnOverMemoryStop, OnOverMemoryWarn, or empty for stop.
	OnOverMemory string
	// SwitchOff is the operator's switch-off for this module. What it does
	// not set follows the server's core.modules_switch_off.
	SwitchOff SwitchOff
	// Listen is each port the operator granted the module, by the name its
	// manifest gave it, and empty for a module granted none. The operator
	// approves ports apart from the other requests and may move one.
	Listen map[string]ListenPort
	// Data is whether the operator granted the module a writable directory.
	// The host chooses where it is.
	Data bool
	// Database is the database of its own the operator granted the module,
	// and nil for none.
	Database *Database
	// Mount is the root prefix an operator named for the module's routes, such
	// as /graphql, and empty for a module reached under its namespace at
	// /api/v1/modules/<namespace>/. The operator names it and the module never
	// does. A GraphQL or compatibility surface can then sit where its clients
	// expect it, and no module chooses its own address. The host holds it to
	// the paths the router can spare and refuses a second claim on one prefix.
	Mount string
	// Assets is the browser half of a module that ships both halves, and nil
	// for a module that draws nothing. The files are served under the same
	// namespace as the routes, at /api/v1/modules/<namespace>/ui/, so one
	// module is one instance with one enabled switch.
	//
	// It sits inside the service block because one instance carries one
	// runtime: the host launches the process, and the bundle belongs to that
	// process's declaration.
	Assets *Asset
}

// Asset is the install record of a module that runs no process: where its
// directory of files is and what proves it is the one that was installed. It
// is present only when Runtime is RuntimeAsset.
type Asset struct {
	// Path is the directory the files are served from.
	Path string
	// Checksum is sha256:<64 hex> over every file in the directory, in the
	// form wirehost.DigestDir computes, compared before every start.
	Checksum string
}

// EventTypeOf is the event type one declared endpoint publishes: the module's
// namespace, then the name the operator wrote.
//
// The module chooses neither half. The namespace is derived from the type an
// operator installed, which sdk/extension renders from a module path. The name
// is a value in the operator's own file. A module names the endpoint, which
// selects a row of this table, and the payload.
func EventTypeOf(namespace, declared string) string {
	return namespace + eventSeparator + declared
}

// EventNameOf is the name an operator wrote for an event type EventTypeOf
// composed under namespace: the type without the namespace.
func EventNameOf(namespace, eventType string) string {
	return strings.TrimPrefix(eventType, namespace+eventSeparator)
}

// Instance is one declaration's install mode.
type Instance struct {
	// Ref is the operator-chosen kind and name.
	Ref ref.Ref
	// Type is the registered implementation, bare or qualified as written.
	Type string
	// State is disabled or runtime.
	State State
	// Runtime is how the type is reached.
	Runtime Runtime
	// Service is set when Runtime is RuntimeService.
	Service Service
	// Script is the expression the host compiles, set when Runtime is
	// RuntimeScript.
	Script string
	// Asset is set when Runtime is RuntimeAsset.
	Asset Asset
}

// Failure is one instance that will not load, and the rest of the document
// without it. Rejection is per instance: one bad block must not stop the binary
// and must not stop the other blocks of the same kind.
type Failure struct {
	// Key is the path into the operator's file, such as resources.driver.bambu.
	Key string
	// Err is why, wrapping ErrInvalid.
	Err error
}

// Document is every instance the operator declared, and every one that would
// not load.
type Document struct {
	// Instances are the declarations that loaded, ordered by kind in the closed
	// set's order and then by name.
	Instances []Instance
	// Failures are the declarations that did not, in the same order.
	Failures []Failure
}

// Runtimes returns the runtimes a kind may offer, in declaration order. The
// table is internal/kernel/ref's, and the published vocabulary is generated
// from it.
func Runtimes(kind ref.Kind) []Runtime { return ref.KindRuntimes(kind) }

// Load reads the resources block out of the operator's file. A missing file is
// an empty document: a first run has no file, and a deployment configured
// entirely through the API has no resources block.
func Load(path string) (Document, error) {
	if path == "" {
		return Document{}, nil
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Document{}, nil
	}

	if err != nil {
		return Document{}, fmt.Errorf("%w: read %s: %w", ErrInvalid, path, err)
	}

	return Parse(path, content)
}

// Parse reads an operator file already in memory. Name is what a message calls
// the file, so a refusal points at something the operator can open.
//
// Only a resources block that is not a map of maps fails the whole document.
// Everything an instance can get wrong fails that instance alone.
func Parse(name string, content []byte) (Document, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return Document{}, nil
	}

	var file struct {
		Resources map[string]map[string]yaml.Node `yaml:"resources"`
	}

	if err := yaml.Unmarshal(content, &file); err != nil {
		return Document{}, fmt.Errorf("%w: %s: resources: %w", ErrInvalid, name, err)
	}

	var doc Document

	for kind, named := range file.Resources {
		// No file declares a camera (ADR-20261116-g5x).
		if kind == ref.KindCamera.String() {
			continue
		}

		for instance, node := range named {
			key := fmt.Sprintf("resources.%s.%s", kind, instance)

			declared, err := declaration(kind, instance, node)
			if err != nil {
				doc.Failures = append(doc.Failures, Failure{
					Key: key,
					Err: fmt.Errorf("%w: %s: %s: %w", ErrInvalid, name, key, err),
				})

				continue
			}

			doc.Instances = append(doc.Instances, declared)
		}
	}

	doc.Instances, doc.Failures = refuseSecondInstances(name, doc.Instances, doc.Failures)

	slices.SortFunc(doc.Instances, func(a, b Instance) int { return ref.Compare(a.Ref, b.Ref) })
	slices.SortFunc(doc.Failures, func(a, b Failure) int { return strings.Compare(a.Key, b.Key) })

	return doc, nil
}

// refuseSecondInstances holds a kind bounded to one instance to that bound.
// Every block of the kind fails, not the second alone. A node has one parent,
// and starting the block that sorted first could dial a parent the operator
// did not mean.
func refuseSecondInstances(name string, instances []Instance, failures []Failure) ([]Instance, []Failure) {
	byKind := map[ref.Kind][]Instance{}

	for _, instance := range instances {
		if instance.Ref.Kind().SoleInstance() {
			byKind[instance.Ref.Kind()] = append(byKind[instance.Ref.Kind()], instance)
		}
	}

	kept := instances[:0:0]

	for _, instance := range instances {
		declared := byKind[instance.Ref.Kind()]
		if len(declared) <= 1 {
			kept = append(kept, instance)

			continue
		}

		names := make([]string, 0, len(declared))
		for _, other := range declared {
			names = append(names, other.Ref.ID())
		}

		slices.Sort(names)

		key := fmt.Sprintf("resources.%s.%s", instance.Ref.Kind(), instance.Ref.ID())
		failures = append(failures, Failure{
			Key: key,
			Err: fmt.Errorf("%w: %s: %s: kind %s allows one instance and %d are declared: %s",
				ErrInvalid, name, key, instance.Ref.Kind(), len(declared), strings.Join(names, ", ")),
		})
	}

	return kept, failures
}

// Entry is the install keys of one instance block. The other keys belong to
// other readers, so the decode is permissive here and strict in
// internal/app/resource, which owns the block.
//
// It carries both tag sets because two sources hold this one shape: the
// operator writes YAML, and the machine-owned overlay is JSON. The install
// command writes the overlay.
//
// required is read here although internal/app/resource owns it, because whether
// an instance may hold it depends on the implementation behind the instance,
// and this package decides that.
type Entry struct {
	Type     string        `json:"type"               yaml:"type"`
	Enabled  *bool         `json:"enabled,omitempty"  yaml:"enabled"`
	Required bool          `json:"required,omitempty" yaml:"required"`
	Runtime  string        `json:"runtime,omitempty"  yaml:"runtime"`
	Service  *ServiceEntry `json:"service,omitempty"  yaml:"service"`
	Script   string        `json:"script,omitempty"   yaml:"script"`
	Asset    *AssetEntry   `json:"asset,omitempty"    yaml:"asset"`
	Module   *ModuleEntry  `json:"module,omitempty"   yaml:"module"`
	// Config is the settings of the implementation behind this instance. The
	// install command writes the operator's settings file here, so the module
	// is installed and configured in one machine-owned record.
	Config json.RawMessage `json:"config,omitempty" yaml:"-"`
}

// ModuleEntry is what the module's own module.yaml said about itself, copied
// into the declaration by the install so a listing can show it.
//
// It is recorded because a release directory is staged and thrown away, and a
// module that runs a process keeps no manifest on disk. Requires is here for
// the same reason: the host has to know what the module needs at every start.
//
// The first four keys decide nothing; a marketplace card prints them.
type ModuleEntry struct {
	Version string `json:"version,omitempty" yaml:"version"`
	Title   string `json:"title,omitempty"   yaml:"title"`
	Summary string `json:"summary,omitempty" yaml:"summary"`
	Author  string `json:"author,omitempty"  yaml:"author"`
	// Requires is what the module declared it cannot run without. The host
	// checks every entry before it starts the instance, and refuses the
	// instance when one is not met.
	Requires []Requirement `json:"requires,omitempty" yaml:"requires"`
	// Suggests is the modules this module works better with, from the
	// manifest's suggest. It installs nothing.
	Suggests []Suggestion `json:"suggests,omitempty" yaml:"suggests"`
	// Conflicts is the releases of other modules this module refuses to run
	// beside, from the manifest's conflict.
	Conflicts []Dependency `json:"conflicts,omitempty" yaml:"conflicts"`
	// Constraint is what the operator asked for when they installed this
	// module themselves, such as ^1.4, and empty for a module an install
	// added for another. The lock's content hash is taken over these.
	Constraint string `json:"constraint,omitempty" yaml:"constraint"`
	// Path is the module path the release was installed under, which is
	// what an index row names it by.
	Path string `json:"path,omitempty" yaml:"path"`
	// Source is where the release came from: the address of an archive or
	// the directory it was read from. Digest is the sha256 of the archive,
	// when it was stated. The lock file records both.
	Source string `json:"source,omitempty" yaml:"source"`
	Digest string `json:"digest,omitempty" yaml:"digest"`
	// Pulled is true for a module an install added because another module
	// required it, and false for one the operator asked for. A removal names
	// every pulled module that nothing requires after it.
	Pulled bool `json:"pulled,omitempty" yaml:"pulled"`
	// FileTypes is the file types the module reads for the core, from its
	// manifest's file_analysis block. The host asks the running module about
	// a file of one of them.
	FileTypes []string `json:"file_types,omitempty" yaml:"file_types"`
	// ViewTypes is the file types the module's file-view entries draw for,
	// and ["*"] when an entry names none and so draws for every file. Two
	// enabled modules never share one.
	ViewTypes []string `json:"view_types,omitempty" yaml:"view_types"`
	// CameraTypes is the camera types the module offers, from its manifest's
	// camera_types (ADR-20261106-km2). A camera declared as
	// <namespace>/<name> runs on the core's generic driver.
	CameraTypes []CameraType `json:"camera_types,omitempty" yaml:"camera_types"`
	// Options is the settings the module declared it reads, with the default
	// of each. It is the module's own default configuration.
	//
	// The host applies every default, writes the operator's config block over
	// it key by key, and sends the result to the process. A key the list does
	// not name is refused before the process starts. It is recorded here for
	// the reason Requires is: the staged release is thrown away and a module
	// that runs a process leaves no manifest on disk.
	Options []Option `json:"options,omitempty" yaml:"options"`
	// Asked is what the release's manifest asked for when an operator approved
	// it. The next approved install compares it with the declaration, so a
	// permission the operator took away stays away. Nil is a declaration
	// installed without approve, or one that records no ask.
	Asked *ModuleAsks `json:"asked,omitempty" yaml:"asked"`
	// AllowedOrigins is the browser origins the operator let this module add
	// to the ones the API answers, from its manifest's allowed_origins. The
	// server answers them while the module is switched on, and they leave
	// with the declaration. An approved install writes it.
	AllowedOrigins []string `json:"allowed_origins,omitempty" yaml:"allowed_origins"`
	// Channel is the marketplace channel the release came from, stable or
	// beta, and empty for a release that did not come from a marketplace. An
	// empty channel reads as stable.
	Channel string `json:"channel,omitempty" yaml:"channel"`
}

// ModuleAsks is what a module's manifest asks the operator for: the
// permissions it takes as itself, the events it publishes, whether it may ask
// about a person it is not serving, whether it may list the people, the
// modules it calls, the routes it answers with no caller, the routes whose
// body it reads from a file, the request headers it reads, the room for its
// records, the ports it listens on, whether it wants a data directory, whether
// it wants a database of its own, and the browser origins it wants the API to
// answer.
type ModuleAsks struct {
	Permissions       []string          `json:"permissions,omitempty"         yaml:"permissions"`
	Checks            []string          `json:"checks,omitempty"              yaml:"checks"`
	Events            map[string]string `json:"events,omitempty"              yaml:"events"`
	CheckAbsentPeople bool              `json:"check_absent_people,omitempty" yaml:"check_absent_people"`
	ListPeople        bool              `json:"list_people,omitempty"         yaml:"list_people"`
	Calls             []string          `json:"calls,omitempty"               yaml:"calls"`
	PublicRoutes      []string          `json:"public_routes,omitempty"       yaml:"public_routes"`
	StagedRoutes      []StagedRoute     `json:"staged_routes,omitempty"       yaml:"staged_routes"`
	RequestHeaders    []RequestHeader   `json:"request_headers,omitempty"     yaml:"request_headers"`
	RecordsMiB        int               `json:"records_mib,omitempty"         yaml:"records_mib"`
	MemoryMiB         int               `json:"memory_mib,omitempty"          yaml:"memory_mib"`
	Listen            []ListenAsk       `json:"listen,omitempty"              yaml:"listen"`
	Data              bool              `json:"data,omitempty"                yaml:"data"`
	Database          bool              `json:"database,omitempty"            yaml:"database"`
	AllowedOrigins    []string          `json:"allowed_origins,omitempty"     yaml:"allowed_origins"`
}

// ListenAsk is one port a manifest asks for, as the declaration records the
// ask and as a marketplace lists it.
type ListenAsk struct {
	Name    string           `json:"name"    yaml:"name"`
	Network manifest.Network `json:"network" yaml:"network"`
	Port    int              `json:"port"    yaml:"port"`
	Why     string           `json:"why"     yaml:"why"`
}

// Request is the line an operator reads for the port.
func (a ListenAsk) Request() string {
	return manifest.Listen{Name: a.Name, Network: a.Network, Port: a.Port, Why: a.Why}.Request()
}

// Option is one setting a module reads, as the declaration records it.
//
// It is sdk/manifest's shape kept in the file this server writes, so the host
// reads one record whether the module was installed by the command or over the
// API. It carries both tag sets for the reason Entry does: the operator writes
// YAML and the machine-owned overlay is JSON.
type Option struct {
	// Key is the key as an operator writes it, in lower_snake_case.
	Key string `json:"key" yaml:"key"`
	// Type is the shape of the value, from sdk/extension's closed set.
	Type extension.OptionType `json:"type" yaml:"type"`
	// Required says the module does not start until the operator writes it.
	// A required key has no default.
	Required bool `json:"required,omitempty" yaml:"required"`
	// Default is what the key is worth when the config block leaves it out,
	// written the way an operator would write it.
	Default string `json:"default,omitempty" yaml:"default"`
	// Summary is one line saying what the key does.
	Summary string `json:"summary" yaml:"summary"`
}

// CameraType is one camera type an installed module offers.
type CameraType struct {
	Name  string `json:"name"            yaml:"name"`
	Title string `json:"title,omitempty" yaml:"title"`
}

// AssetEntry is the asset block as an operator writes it: the two keys the
// install record holds and no more.
type AssetEntry struct {
	Path     string `json:"path"     yaml:"path"`
	Checksum string `json:"checksum" yaml:"checksum"`
}

// ServiceEntry is the service block as an operator writes it. ContractMajor is
// a pointer so that absent and zero are different answers: absent is a load
// error and zero is a value the host refuses.
type ServiceEntry struct {
	Path              string            `json:"path"                          yaml:"path"`
	Checksum          string            `json:"checksum"                      yaml:"checksum"`
	ContractMajor     *int              `json:"contract_major,omitempty"      yaml:"contract_major"`
	Events            map[string]string `json:"events,omitempty"              yaml:"events"`
	Permissions       []string          `json:"permissions,omitempty"         yaml:"permissions"`
	Checks            []string          `json:"checks,omitempty"              yaml:"checks"`
	CheckAbsentPeople bool              `json:"check_absent_people,omitempty" yaml:"check_absent_people"`
	ListPeople        bool              `json:"list_people,omitempty"         yaml:"list_people"`
	Calls             []string          `json:"calls,omitempty"               yaml:"calls"`
	PublicRoutes      []string          `json:"public_routes,omitempty"       yaml:"public_routes"`
	StagedRoutes      []StagedRoute     `json:"staged_routes,omitempty"       yaml:"staged_routes"`
	RequestHeaders    []string          `json:"request_headers,omitempty"     yaml:"request_headers"`
	RecordsMiB        int               `json:"records_mib,omitempty"         yaml:"records_mib"`
	MemoryMiB         int               `json:"memory_mib,omitempty"          yaml:"memory_mib"`
	OnOverMemory      string            `json:"on_over_memory,omitempty"      yaml:"on_over_memory"`
	SwitchOff         *SwitchOffEntry   `json:"switch_off,omitempty"          yaml:"switch_off"`
	Listen            map[string]string `json:"listen,omitempty"              yaml:"listen"`
	Data              bool              `json:"data,omitempty"                yaml:"data"`
	Database          *DatabaseEntry    `json:"database,omitempty"            yaml:"database"`
	Mount             string            `json:"mount,omitempty"               yaml:"mount"`
	Assets            *AssetEntry       `json:"assets,omitempty"              yaml:"assets"`
}

// declaration turns one instance block into its install mode, refusing
// everything that can be refused before anything is constructed.
func declaration(kind, name string, node yaml.Node) (Instance, error) {
	instance, err := ref.New(ref.Kind(kind), name)
	if err != nil {
		return Instance{}, err
	}

	var declared Entry
	if err = node.Decode(&declared); err != nil {
		return Instance{}, err
	}

	return Declared(instance, declared)
}

// Declared turns one already-decoded block into its install mode. The overlay
// calls it directly, because it holds the same keys in JSON.
func Declared(instance ref.Ref, declared Entry) (Instance, error) {
	if !instance.Kind().Supervised() {
		return Instance{}, fmt.Errorf("kind %q declares no instances here, expected one of %v",
			instance.Kind(), ref.SupervisedKinds())
	}

	if strings.TrimSpace(declared.Type) == "" {
		return Instance{}, errors.New("type is empty")
	}

	mode, err := runtimeOf(instance.Kind(), declared)
	if err != nil {
		return Instance{}, err
	}

	service, err := serviceOf(mode, declared.Type, declared.Service)
	if err != nil {
		return Instance{}, err
	}

	source, err := scriptOf(mode, declared)
	if err != nil {
		return Instance{}, err
	}

	asset, err := assetOf(mode, declared.Type, declared.Asset)
	if err != nil {
		return Instance{}, err
	}

	if err = requiredOf(mode, declared); err != nil {
		return Instance{}, err
	}

	state := StateRuntime
	if declared.Enabled != nil && !*declared.Enabled {
		state = StateDisabled
	}

	return Instance{
		Ref:     instance,
		Type:    strings.TrimSpace(declared.Type),
		State:   state,
		Runtime: mode,
		Service: service,
		Script:  source,
		Asset:   asset,
	}, nil
}

// assetOf validates the asset block against the runtime that decides whether
// it belongs at all.
//
// The type has to be qualified, because the namespace the files are served
// under is derived from it and a bare name has none. Everything else a module
// declares, a contract major, events, permissions or a mount, is refused by
// the reader of its own block: none of it means anything for files.
func assetOf(mode Runtime, typeName string, declared *AssetEntry) (Asset, error) {
	if mode != RuntimeAsset {
		if declared != nil {
			return Asset{}, fmt.Errorf("asset is set and runtime is %q, so nothing reads it", mode)
		}

		return Asset{}, nil
	}

	if declared == nil {
		return Asset{}, fmt.Errorf("runtime is %q and asset is absent", RuntimeAsset)
	}

	if !extension.IsQualified(strings.TrimSpace(typeName)) {
		return Asset{}, fmt.Errorf("type %q is a bare name, and an asset is served under the namespace "+
			"its type carries", strings.TrimSpace(typeName))
	}

	return bundleOf("asset", declared)
}

// bundleOf validates a directory of files and the digest that proves they are
// the ones installed. Two blocks carry that pair: the asset block of an asset
// instance, and the assets block of a module that also runs a process. key is
// what a refusal calls the block.
func bundleOf(key string, declared *AssetEntry) (Asset, error) {
	if strings.TrimSpace(declared.Path) == "" {
		return Asset{}, fmt.Errorf("%s.path is empty", key)
	}

	if !checksumPattern.MatchString(declared.Checksum) {
		return Asset{}, fmt.Errorf("%s.checksum is %q, expected sha256: and 64 hex digits",
			key, declared.Checksum)
	}

	return Asset{Path: strings.TrimSpace(declared.Path), Checksum: declared.Checksum}, nil
}

// scriptOf validates the script key against the runtime that decides whether it
// belongs at all, and holds the type and the runtime to one answer.
//
// The length is refused here because the file and the key are known here. The
// parser is mutually recursive with no depth limit, so a long enough source
// ends in a stack overflow that no recover catches.
func scriptOf(mode Runtime, declared Entry) (string, error) {
	source := declared.Script
	typeName := strings.TrimSpace(declared.Type)

	if mode != RuntimeScript {
		if strings.TrimSpace(source) != "" {
			return "", fmt.Errorf("script is set and runtime is %q, so nothing runs it", mode)
		}

		if typeName == ScriptType {
			return "", fmt.Errorf("type is %q and runtime is %q, expected runtime %q",
				ScriptType, mode, RuntimeScript)
		}

		return "", nil
	}

	if typeName != ScriptType {
		return "", fmt.Errorf("runtime is %q and type is %q, expected type %q",
			RuntimeScript, typeName, ScriptType)
	}

	if strings.TrimSpace(source) == "" {
		return "", fmt.Errorf("runtime is %q and script is empty", RuntimeScript)
	}

	if len(source) > script.MaxSourceBytes {
		return "", fmt.Errorf("script is %d bytes, the limit is %d", len(source), script.MaxSourceBytes)
	}

	return source, nil
}

// requiredOf holds the readiness switch to implementations the core ships.
//
// required is the operator saying the deployment does not work without this
// instance, and the core acts on it twice. GET /api/v1/readiness answers 503
// while the instance is not serving, which takes the pod out of a Kubernetes
// Service so no printer reaches the server. And the supervisor never stops
// restarting the instance. Both are the core's own availability, so a module
// a customer installed must not hold the flag.
//
// The whole instance is refused, not the flag alone. Dropping the flag without
// a message would leave the operator believing a dependency is enforced.
func requiredOf(mode Runtime, declared Entry) error {
	typeName := strings.TrimSpace(declared.Type)

	if !declared.Required || IsCore(mode, typeName) {
		return nil
	}

	return fmt.Errorf("required is true and type %q with runtime %q is not part of the core. "+
		"An instance the core did not write may not hold the whole server out of readiness. "+
		"Remove required, and watch this instance's own condition instead", typeName, mode)
}

// IsCore reports whether an instance's implementation is one the core wrote and
// ships in this artefact. Both halves have to hold: the type is compiled in,
// and its name belongs to the core namespace.
//
// The runtime is asked first because a name cannot answer alone. A service
// instance may be declared with a bare type, which coreType reads as core
// because a bare name resolves inside this build, while the binary that serves
// it is a separate file somebody else compiled. A script carries the type
// "script" and a body the operator wrote, so it is not the core's either.
func IsCore(mode Runtime, typeName string) bool {
	return mode == RuntimeBuiltin && coreType(typeName)
}

// runtimeOf reads the runtime key and holds it to what the kind can offer. A
// mode nothing can serve is refused at load, so the operator does not meet a
// resource that never becomes ready.
func runtimeOf(kind ref.Kind, declared Entry) (Runtime, error) {
	offered := ref.KindRuntimes(kind)

	mode := Runtime(strings.TrimSpace(declared.Runtime))
	if mode == "" {
		mode = defaultRuntime(offered)
	}

	if !slices.Contains(offered, mode) {
		return "", fmt.Errorf("runtime is %q, and %s offers %s", mode, kind, list(offered))
	}

	if err := RefuseModuleAsBuiltin(mode, declared.Type); err != nil {
		return "", err
	}

	return mode, nil
}

// defaultRuntime is what an absent runtime key reads as: builtin, or the one
// runtime a kind offers when it offers exactly one. An operator then writes no
// runtime for an asset.
func defaultRuntime(offered []Runtime) Runtime {
	if len(offered) == 1 {
		return offered[0]
	}

	return RuntimeBuiltin
}

// RefuseModuleAsBuiltin refuses a module declared as builtin. A module runs as
// runtime: service.
//
// A type outside the core namespace is code the core did not write. Builtin
// would put it inside this address space, where it shares the heap, the
// goroutine scheduler and every secret the process holds. Nothing can contain
// code there, and a module must not be able to break the core.
//
// The declaration also could never run. Nothing outside the core namespace is
// in this artefact's type registry, so without the refusal the instance would
// report a missing type at its first start, with no key to change.
//
// Nil is the answer for every other declaration, including a bare type name: a
// bare name resolves inside this build, and only the main module holds the core
// namespace, so neither form can be claimed by a plugin (see coreType).
func RefuseModuleAsBuiltin(mode Runtime, typeName string) error {
	name := strings.TrimSpace(typeName)

	if !ModuleAsBuiltin(mode, name) {
		return nil
	}

	return fmt.Errorf("runtime is %q and type %q is not part of the core, so this declares a module "+
		"this binary would run inside itself. A module runs as %q, in a process of its own, because "+
		"nothing that shares this address space can be stopped without stopping the server with it. "+
		"Write runtime: %s with the service block naming the artefact the module ships, or drop the "+
		"namespace if this is a type the core wrote", mode, name, RuntimeService, RuntimeService)
}

// ModuleAsBuiltin is RefuseModuleAsBuiltin's decision without the message, for
// the overlay and the resource API. The overlay is machine-owned and the
// resource API writes only builtin instances, so each writes its own message.
// One predicate keeps the three callers agreed on which declarations are
// modules.
func ModuleAsBuiltin(mode Runtime, typeName string) bool {
	return mode == RuntimeBuiltin && !coreType(strings.TrimSpace(typeName))
}

// serviceOf validates the service block against the runtime that decides
// whether it belongs at all.
func serviceOf(mode Runtime, typeName string, declared *ServiceEntry) (Service, error) {
	if mode != RuntimeService {
		if declared != nil {
			return Service{}, fmt.Errorf("service is set and runtime is %q, so nothing reads it", mode)
		}

		return Service{}, nil
	}

	if declared == nil {
		return Service{}, fmt.Errorf("runtime is %q and service is absent", RuntimeService)
	}

	if strings.TrimSpace(declared.Path) == "" {
		return Service{}, errors.New("service.path is empty")
	}

	if !checksumPattern.MatchString(declared.Checksum) {
		return Service{}, fmt.Errorf("service.checksum is %q, expected sha256: and 64 hex digits",
			declared.Checksum)
	}

	if declared.ContractMajor == nil {
		return Service{}, errors.New("service.contract_major is absent, and it has no default")
	}

	if *declared.ContractMajor < 1 {
		return Service{}, fmt.Errorf("service.contract_major is %d, expected 1 or more", *declared.ContractMajor)
	}

	events, err := eventsOf(typeName, declared.Events)
	if err != nil {
		return Service{}, err
	}

	permissions, checks, err := actionsOf(declared)
	if err != nil {
		return Service{}, err
	}

	mount, err := mountOf(typeName, declared.Mount)
	if err != nil {
		return Service{}, err
	}

	switchOff, err := limitsOf(declared)
	if err != nil {
		return Service{}, err
	}

	public, staged, err := routeGrantsOf(declared)
	if err != nil {
		return Service{}, err
	}

	headers, err := headerGrantsOf(declared.RequestHeaders)
	if err != nil {
		return Service{}, err
	}

	listen, err := listenOf(declared.Listen)
	if err != nil {
		return Service{}, err
	}

	database, err := databaseOf(declared.Database)
	if err != nil {
		return Service{}, err
	}

	assets, err := assetsOf(typeName, declared.Assets)
	if err != nil {
		return Service{}, err
	}

	if mount != "" && assets != nil {
		return Service{}, fmt.Errorf("service.mount is %q and service.assets is set. A module at a root "+
			"prefix serves its routes there, and its files are served under its namespace at "+
			"/api/v1/modules/<namespace>/ui/, which that module no longer answers on. Drop one of the two",
			mount)
	}

	return Service{
		Path:              strings.TrimSpace(declared.Path),
		Checksum:          declared.Checksum,
		ContractMajor:     *declared.ContractMajor,
		Events:            events,
		Permissions:       permissions,
		Checks:            checks,
		CheckAbsentPeople: declared.CheckAbsentPeople,
		ListPeople:        declared.ListPeople,
		Calls:             slices.Clone(declared.Calls),
		PublicRoutes:      public,
		StagedRoutes:      staged,
		RequestHeaders:    headers,
		RecordsMiB:        declared.RecordsMiB,
		MemoryMiB:         declared.MemoryMiB,
		OnOverMemory:      declared.OnOverMemory,
		SwitchOff:         switchOff,
		Listen:            listen,
		Data:              declared.Data,
		Database:          database,
		Mount:             mount,
		Assets:            assets,
	}, nil
}

// MaxRecordsMiB is the most room an operator can grant one module's records,
// in mebibytes: stateapi.MaxGrantedNamespaceBytes.
const MaxRecordsMiB = stateapi.MaxGrantedNamespaceBytes / stateapi.NamespaceMiB

// checkRecordsMiB refuses a grant the record store would not hold. Zero is no
// grant and passes.
func checkRecordsMiB(key string, mib int) error {
	if mib < 0 || mib > MaxRecordsMiB {
		return fmt.Errorf("%s is %d, expected 0 for the default of %d MiB, or 1 to %d",
			key, mib, stateapi.DefaultNamespaceBytes/stateapi.NamespaceMiB, MaxRecordsMiB)
	}

	return nil
}

// RecordLimit is the byte limit the module's record namespace is held to: what
// the operator granted, or stateapi.DefaultNamespaceBytes without a grant.
func (s *Service) RecordLimit() int64 {
	if s == nil || s.RecordsMiB == 0 {
		return stateapi.DefaultNamespaceBytes
	}

	return int64(s.RecordsMiB) * stateapi.NamespaceMiB
}

// assetsOf reads the browser half of a module that also runs a process, and nil
// for a module that ships none.
//
// The type has to be qualified for the reason an asset instance's does: the
// files are served under the namespace the type carries, and a bare name has
// none. serviceOf refuses a mount and a bundle together, because the files are
// served under the namespace group and a module moved to a root prefix answers
// nothing there.
func assetsOf(typeName string, declared *AssetEntry) (*Asset, error) {
	if declared == nil {
		return nil, nil //nolint:nilnil // a module that ships no browser half declares no bundle.
	}

	name := strings.TrimSpace(typeName)
	if !extension.IsQualified(name) {
		return nil, fmt.Errorf("service.assets is set and type %q is a bare name, so this instance is not "+
			"a module and has no namespace to serve files under", name)
	}

	bundle, err := bundleOf("service.assets", declared)
	if err != nil {
		return nil, err
	}

	return &bundle, nil
}

// mountOf reads the root prefix an instance's routes are served under, and
// empty for an instance reached under its namespace.
//
// Only what the file alone can answer is refused here: a key that is present
// and empty, and a mount on an instance that is not a module. The host decides
// what the prefix may look like and which paths the router keeps, because this
// package runs before a router exists.
func mountOf(typeName, declared string) (string, error) {
	if declared == "" {
		return "", nil
	}

	mount := strings.TrimSpace(declared)
	if mount == "" {
		return "", errors.New("service.mount is set and empty; write a root prefix such as /graphql, or drop the key")
	}

	if !extension.IsQualified(strings.TrimSpace(typeName)) {
		return "", fmt.Errorf("service.mount is set and type %q is a bare name, so this instance is not a "+
			"module and has no routes to mount", strings.TrimSpace(typeName))
	}

	return mount, nil
}

// eventSeparator joins a namespace to the name an operator wrote. It is the
// event type's own separator, because the result is one event type.
const eventSeparator = "."

// eventsOf resolves the endpoint table one module instance may publish on.
//
// It makes three refusals, and the operator reads each at load:
//
//  1. an instance whose type is a bare name claims no namespace. There is no
//     prefix to compose, and a type this build compiled in is not a module.
//  2. an endpoint name the wire will not carry. The name is quoted back in
//     every refusal and written into this host's log.
//  3. a composed type the event vocabulary refuses. The namespace is derived
//     and the name is the operator's, so the only way to land inside
//     com.prusa.connect. is a module path that renders there, and that
//     namespace is closed whoever reaches it.
func eventsOf(typeName string, declared map[string]string) (map[string]string, error) {
	out := map[string]string{}
	if len(declared) == 0 {
		return out, nil
	}

	name := strings.TrimSpace(typeName)
	if !extension.IsQualified(name) {
		return nil, fmt.Errorf(
			"service.events is set and type %q is a bare name, so this instance claims no namespace. "+
				"An event type is the module's namespace and the name written here, and a bare type "+
				"has no namespace to publish under", name)
	}

	namespace, _, _ := strings.Cut(name, extension.QualifiedSeparator)
	if err := extension.Namespace(namespace).Validate(); err != nil {
		return nil, fmt.Errorf("service.events is set and type %q carries no usable namespace: %w", name, err)
	}

	for endpoint, tail := range declared {
		if err := validEndpoint(endpoint); err != nil {
			return nil, err
		}

		eventType := EventTypeOf(namespace, strings.TrimSpace(tail))

		if err := event.ValidateType(eventType); err != nil {
			return nil, fmt.Errorf("service.events[%q] publishes %q: %w", endpoint, eventType, err)
		}

		out[endpoint] = eventType
	}

	return out, nil
}

// validEndpoint holds an endpoint name to what the wire carries and what a
// refusal may quote.
func validEndpoint(endpoint string) error {
	switch {
	case strings.TrimSpace(endpoint) == "":
		return errors.New("service.events carries an endpoint with no name")
	case len(endpoint) > busapi.MaxEndpointBytes:
		return fmt.Errorf("service.events endpoint %q is %d bytes and the limit is %d",
			endpoint, len(endpoint), busapi.MaxEndpointBytes)
	}

	for i := range len(endpoint) {
		if endpoint[i] <= ' ' || endpoint[i] == 0x7f {
			return fmt.Errorf("service.events endpoint %q carries a character the wire will not take",
				endpoint)
		}
	}

	return nil
}

// coreType reports whether a type name belongs to the main module. A bare name
// resolves inside this build, and only the main module holds the core
// namespace, so neither form can be claimed by a plugin.
//
// It answers about the name alone. IsCore is the whole question.
func coreType(name string) bool {
	if !extension.IsQualified(name) {
		return true
	}

	namespace, _, _ := strings.Cut(name, "/")

	return extension.Namespace(namespace).IsCore()
}

// list renders the runtimes a kind offers, for a refusal to name them.
func list(offered []Runtime) string {
	if len(offered) == 0 {
		return "none"
	}

	names := make([]string, 0, len(offered))
	for _, mode := range offered {
		names = append(names, string(mode))
	}

	return strings.Join(names, " or ")
}
