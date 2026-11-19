// Package manifest reads module.yaml, the one file a module ships at the root
// of its repository and its release.
//
// A module has one manifest. A module that runs a process declares the wire
// contract, the events and the permissions it asks for. A module that draws
// in the browser declares the ui block. A module that does both writes both
// in this file.
//
// The standard library reads no YAML, so Parse takes the document and a
// decoder: gopkg.in/yaml.v3's Unmarshal, or encoding/json's for a JSON
// document. An unknown key is refused by name.
package manifest

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// ErrInvalid reports a manifest this package will not accept. Test for it
// with errors.Is. The message names the key.
var ErrInvalid = errors.New("manifest: invalid module.yaml")

// Decode reads a document into a value. gopkg.in/yaml.v3's Unmarshal and
// encoding/json's both have this shape.
type Decode func(document []byte, into any) error

// Manifest is one module.yaml. Every field is the key of the same name, except
// ContractMajor (contract_major) and IssueTracker (issue_tracker). It has no
// namespace: the host derives one from the module path the operator installed.
type Manifest struct {
	// Name is the module's short name. The namespace is derived from the
	// module path and never from this.
	Name string
	// Kind is the resource kind an instance of this module is declared under.
	Kind string
	// Version is the module's own release, MAJOR.MINOR.PATCH.
	Version string
	// License names the licence the module is released under, as free text
	// of at most 200 characters. An SPDX identifier is the usual form.
	License string
	// ContractMajor is the plugin wire major the process half speaks. It is
	// zero for a module that ships no process.
	ContractMajor int
	// Core is the constraint the module's require puts on the core, and empty
	// when it puts none.
	Core string
	// Requires is the require map: the core, the capabilities the core must
	// serve, and the other modules with a constraint each.
	Requires []Requirement
	// Suggests is the suggest map: the modules this module works better with,
	// each with the reason. It installs nothing.
	Suggests []Suggestion
	// Conflicts is the conflict map: the releases of other modules this
	// module refuses to run beside.
	Conflicts []Dependency
	// Permissions is the ceiling the module asks for, as action names. It is a
	// request: the operator approves it before it is written into the
	// declaration.
	Permissions []string
	// Checks is the actions the module asks access.check about without taking
	// them itself. It is a request like Permissions and grants no action.
	Checks []string
	// CheckAbsentPeople asks to put access.check questions about a person the
	// module is not serving. It is a request like Permissions and grants no
	// action.
	CheckAbsentPeople bool
	// ListPeople asks to list the people of this server, with their names. It
	// is a request like Permissions and grants no action.
	ListPeople bool
	// RecordsMiB is the room the module asks for its records, in mebibytes.
	// Zero takes the default. It is a request like Permissions.
	RecordsMiB int
	// Events is the endpoints the module publishes on, each an endpoint name
	// against the tail of the event type. The host writes the namespace.
	Events map[string]string
	// Calls is the namespaces of the modules whose routes this module calls
	// with routes.call. Each is also under Requires or Suggests. It is a
	// request like Permissions.
	Calls []string
	// AllowedOrigins is the browser origins the module asks the server to
	// answer, each an exact https origin. A page of another site that works
	// with the module calls the API from one. It is a request like
	// Permissions, and it holds only while the module is switched on.
	AllowedOrigins []string
	// Needs is the ports the module listens on and whether it asks for a
	// data directory. It is a request like Permissions, and a port takes an
	// approval of its own.
	Needs Needs
	// Routes is the routes the host serves without a caller and the routes
	// whose body it writes to a file. A public or staged route is a request
	// like Permissions, and the operator approves each one.
	Routes Routes
	// Options is the settings this module reads, each with its type, its
	// sentence and its default. The host applies every default and writes the
	// operator's config block over it. A key outside the list is refused.
	Options []Option
	// Egress is every connection the module makes outside Prusa Connect, from
	// its process or its page. Empty means it connects nowhere outside.
	Egress []Egress
	// OpenAPI is the path of the module's OpenAPI fragment inside the
	// repository, and empty for a module that serves no routes.
	OpenAPI string
	// Platforms is the GOOS_GOARCH pairs the release ships a binary for.
	Platforms []string
	// Build is the command that produces the binary, one per platform, keyed
	// by the same GOOS_GOARCH pair.
	Build map[string]string
	// Title is the name a person reads, for a catalogue to show beside the
	// short name. Optional.
	Title string
	// Summary is one sentence saying what the module does, for a catalogue to
	// show. Optional.
	Summary string
	// Codeowner is who answers for this module, for a catalogue to show.
	Codeowner string
	// IssueTracker is where a defect against this module is filed.
	IssueTracker string
	// Lifecycle is how far along the module is, for a catalogue to show.
	Lifecycle string
	// UI is the browser half, and nil for a module that draws nothing.
	UI *UI
	// FileAnalysis is the file types the module reads for the core, and nil
	// for a module that reads none.
	FileAnalysis *FileAnalysis
	// CameraTypes is the camera types the module offers, and nil for a module
	// that offers none (ADR-20261106-km2).
	CameraTypes []CameraType
}

// Requirement is one entry of the require map. One of Capability, Core and
// Module is set: a capability the core serves, a constraint on the core, or
// another module and a constraint on its releases.
type Requirement struct {
	// Capability is a wire method, written core-<method> in require.
	Capability string
	// Core is a constraint on the core, written under core in require.
	Core string
	// Module is the namespace of another module.
	Module string
	// Version is the constraint on Module's releases.
	Version string
}

// Dependency is the module half of the requirement.
func (r Requirement) Dependency() Dependency {
	return Dependency{Module: r.Module, Version: r.Version}
}

// MaxRecordsMiB is the most room records_mib may ask for, in mebibytes. It
// equals stateapi.MaxGrantedNamespaceBytes.
const MaxRecordsMiB = 1024

// manifestKeys is every key a manifest may have. Any other key is refused, so
// a misspelt key does not pass as an absent one.
//
//nolint:gochecknoglobals // a closed table read at parse, never written.
var manifestKeys = []string{
	"name", "kind", "version", "license", "contract_major", "require", "suggest", "conflict",
	"permissions", "checks", "check_absent_people", "list_people", "records_mib", "events", "calls", "needs",
	"allowed_origins",
	"routes", "options", "egress",
	"openapi", "platforms", "build",
	"title", "summary", "codeowner", "issue_tracker", "lifecycle", "ui", "file_analysis", "camera_types",
}

// Parse reads one module.yaml with the decoder the caller brings. name is what
// a refusal calls the file.
func Parse(name string, document []byte, decode Decode) (Manifest, error) {
	if decode == nil {
		return Manifest{}, fmt.Errorf("%w: %s: no decoder was passed", ErrInvalid, name)
	}

	var tree map[string]any

	if err := decode(document, &tree); err != nil {
		return Manifest{}, fmt.Errorf("%w: %s: %w", ErrInvalid, name, err)
	}

	held, err := FromTree(tree)
	if err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", name, err)
	}

	return held, nil
}

// FromTree reads a manifest from a tree another reader already decoded.
func FromTree(tree map[string]any) (Manifest, error) {
	if tree == nil {
		return Manifest{}, fmt.Errorf("%w: the document is empty", ErrInvalid)
	}

	if err := refuseUnknown(tree, "", manifestKeys); err != nil {
		return Manifest{}, err
	}

	held, err := readTop(tree)
	if err != nil {
		return Manifest{}, err
	}

	held.UI, err = readUI(tree)
	if err != nil {
		return Manifest{}, err
	}

	if held.FileAnalysis, err = readFileAnalysis(tree); err != nil {
		return Manifest{}, err
	}

	if held.CameraTypes, err = readCameraTypes(tree); err != nil {
		return Manifest{}, err
	}

	// A camera of the module's type is fed over the wire, so a module with no
	// process has nothing to feed it with.
	if len(held.CameraTypes) > 0 && held.ContractMajor == 0 {
		return Manifest{}, fmt.Errorf("%w: camera_types is set and contract_major is not, and a camera "+
			"of a module's type is fed by the module's process", ErrInvalid)
	}

	// A camera takes a frame only from a caller holding operate on it.
	if len(held.CameraTypes) > 0 && !slices.Contains(held.Permissions, "operate") {
		return Manifest{}, fmt.Errorf("%w: camera_types is set and permissions does not name operate, "+
			"which camera.accept takes", ErrInvalid)
	}

	// The core asks the module over the wire, so a module with no process
	// has nothing to answer with.
	if held.FileAnalysis != nil && held.ContractMajor == 0 {
		return Manifest{}, fmt.Errorf("%w: file_analysis is set and contract_major is not, and the core "+
			"asks a module's process to read a file", ErrInvalid)
	}

	// A route is answered by the module's process.
	if (len(held.Routes.Public) > 0 || len(held.Routes.Staged) > 0 || len(held.Routes.Headers) > 0) &&
		held.ContractMajor == 0 {
		return Manifest{}, fmt.Errorf("%w: routes is set and contract_major is not, and only a module's "+
			"process answers a route", ErrInvalid)
	}

	if err = holdDependencies(held); err != nil {
		return Manifest{}, err
	}

	return held, nil
}

// readTop reads every key but ui, which is long enough to read on its own.
func readTop(tree map[string]any) (Manifest, error) {
	var (
		held Manifest
		err  error
	)

	for _, read := range []func(map[string]any, *Manifest) error{
		readIdentity, readContract, readRequests, readRelease, readCatalogue,
	} {
		if err = read(tree, &held); err != nil {
			return Manifest{}, err
		}
	}

	return held, nil
}

// readIdentity reads what names the module: the short name, the kind an
// instance is declared under, and the release this manifest describes.
func readIdentity(tree map[string]any, held *Manifest) error {
	var err error

	if held.Name, err = requiredText(tree, "", "name"); err != nil {
		return err
	}

	if held.Kind, err = requiredText(tree, "", "kind"); err != nil {
		return err
	}

	if held.Version, err = text(tree, "", "version"); err != nil {
		return err
	}

	if err = exactVersion("version", held.Version); err != nil {
		return err
	}

	return readLicense(tree, held)
}

// MaxLicense is the longest licence a manifest may name, in characters.
const MaxLicense = 200

// readLicense reads the licence the module is released under. It is free
// text, so a community author may name any licence, but it has to be there.
func readLicense(tree map[string]any, held *Manifest) error {
	var err error

	if held.License, err = requiredText(tree, "", "license"); err != nil {
		return err
	}

	if err = plainText("", "license", held.License); err != nil {
		return err
	}

	if utf8.RuneCountInString(held.License) > MaxLicense {
		return fmt.Errorf("%w: license is %d characters, and it may have at most %d",
			ErrInvalid, utf8.RuneCountInString(held.License), MaxLicense)
	}

	return nil
}

// readContract reads what the module needs from the host: the wire major it
// speaks, the core version it runs on, and what must be there before it does.
func readContract(tree map[string]any, held *Manifest) error {
	var err error

	if held.ContractMajor, err = whole(tree, "", "contract_major"); err != nil {
		return err
	}

	if held.ContractMajor < 0 {
		return fmt.Errorf("%w: contract_major is %d, expected 1 or more", ErrInvalid, held.ContractMajor)
	}

	if held.Requires, held.Core, err = readRequire(tree); err != nil {
		return err
	}

	if held.Suggests, err = readSuggest(tree); err != nil {
		return err
	}

	held.Conflicts, err = readConflict(tree)

	return err
}

// readRequests reads the keys the operator must approve: permissions, checks,
// records, events, calls, allowed origins and needs.
func readRequests(tree map[string]any, held *Manifest) error {
	var err error

	if held.Permissions, err = textList(tree, "", "permissions"); err != nil {
		return err
	}

	if err = refuseRepeat("permissions", held.Permissions); err != nil {
		return err
	}

	if held.Checks, err = textList(tree, "", "checks"); err != nil {
		return err
	}

	if err = refuseRepeat("checks", held.Checks); err != nil {
		return err
	}

	for _, action := range held.Checks {
		if slices.Contains(held.Permissions, action) {
			return fmt.Errorf("%w: checks names %q, which permissions names too, and a permission "+
				"may be asked about already", ErrInvalid, action)
		}
	}

	if held.CheckAbsentPeople, err = truth(tree, "", "check_absent_people"); err != nil {
		return err
	}

	if held.ListPeople, err = truth(tree, "", "list_people"); err != nil {
		return err
	}

	if held.RecordsMiB, err = whole(tree, "", "records_mib"); err != nil {
		return err
	}

	if held.RecordsMiB < 0 || held.RecordsMiB > MaxRecordsMiB {
		return fmt.Errorf("%w: records_mib is %d, expected 1 to %d, or no key for the default",
			ErrInvalid, held.RecordsMiB, MaxRecordsMiB)
	}

	if held.Events, err = textTable(tree, "", "events"); err != nil {
		return err
	}

	if held.Calls, err = readCalls(tree); err != nil {
		return err
	}

	if held.AllowedOrigins, err = readAllowedOrigins(tree); err != nil {
		return err
	}

	if held.Needs, err = readNeeds(tree); err != nil {
		return err
	}

	if held.Routes, err = readRoutes(tree, held.Needs); err != nil {
		return err
	}

	if held.Options, err = readOptions(tree); err != nil {
		return err
	}

	held.Egress, err = readEgress(tree, held.Options)

	return err
}

// readRelease reads the OpenAPI fragment path, the platforms and the build
// command of each.
func readRelease(tree map[string]any, held *Manifest) error {
	var err error

	if held.OpenAPI, err = text(tree, "", "openapi"); err != nil {
		return err
	}

	if held.OpenAPI != "" {
		if err = insideBundle("openapi", held.OpenAPI); err != nil {
			return err
		}
	}

	if held.Platforms, err = textList(tree, "", "platforms"); err != nil {
		return err
	}

	if err = refuseRepeat("platforms", held.Platforms); err != nil {
		return err
	}

	held.Build, err = textTable(tree, "", "build")

	return err
}

// readCatalogue reads the keys a catalogue listing shows.
func readCatalogue(tree map[string]any, held *Manifest) error {
	var err error

	if held.Title, err = text(tree, "", "title"); err != nil {
		return err
	}

	if held.Summary, err = text(tree, "", "summary"); err != nil {
		return err
	}

	if held.Codeowner, err = text(tree, "", "codeowner"); err != nil {
		return err
	}

	if held.IssueTracker, err = text(tree, "", "issue_tracker"); err != nil {
		return err
	}

	held.Lifecycle, err = text(tree, "", "lifecycle")

	return err
}

// exactVersion refuses a range where one version is expected. An empty value
// is absent.
func exactVersion(key, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	if _, err := ParseVersion(value); err != nil {
		return fmt.Errorf("%s is %q: %w", key, value, err)
	}

	return nil
}

// refuseRepeat refuses a list that names a value twice.
func refuseRepeat(key string, values []string) error {
	seen := make(map[string]bool, len(values))

	for _, value := range values {
		if seen[value] {
			return fmt.Errorf("%w: %s names %q twice", ErrInvalid, key, value)
		}

		seen[value] = true
	}

	return nil
}
