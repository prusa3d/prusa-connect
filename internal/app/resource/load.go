package resource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
)

// Errors reported while the inputs are read. Callers inspect them with
// errors.Is.
var (
	// ErrConfigInvalid reports an input that does not parse or does not
	// describe a resource instance.
	ErrConfigInvalid = errors.New("resource: invalid configuration")
	// errTrailingJSON reports a settings block with a second value after the
	// first.
	errTrailingJSON = fmt.Errorf("%w: trailing content after the settings object", ErrConfigInvalid)
)

// unknownOverlayKey is how the JSON decoder begins the error for a key the
// overlay's type does not have. The rest of the message is the key, quoted.
const unknownOverlayKey = "json: unknown field "

// overlayFormat is the overlay's schema version. A file with a version this
// binary does not know is refused. Reading it as this format would silently
// drop what the newer format added.
const overlayFormat = 1

// Paths names the two inputs. Overlay is required. An empty File is a
// deployment with no hand-written resource block, configured only through the
// API.
type Paths struct {
	// File is the operator's hand-written YAML. The API never writes it.
	File string
	// Overlay is the machine-owned JSON the API writes.
	Overlay string
}

// inputs are the two files as bytes. A write keeps them to check that neither
// file changed between the read that produced the token and the rename.
type inputs struct {
	// name is the path of the operator's file as they wrote it, so a refusal
	// names a file they can open.
	name    string
	file    []byte
	overlay []byte
	version string
}

// overlayFile is the machine-owned document. Identities covers the instances
// of both inputs. An instance an operator moves from the file to the API
// therefore keeps the identity its data rows reference, and a declaration's
// generation survives a restart.
type overlayFile struct {
	Version    int                        `json:"version"`
	Identities map[string]overlayIdentity `json:"identities,omitempty"`
	Instances  []overlayEntry             `json:"instances"`
	// Presets are the stored gcode sequences. They are in the same file as
	// the instances, so they get the same atomic write and version token, and
	// a preset write and an instance write cannot overwrite each other.
	Presets []overlayPreset `json:"presets,omitempty"`
	// Endpoints are the addresses this server dials. They are in this file
	// for the same reason as the presets. They are not under core.connectors,
	// because a customer adds a printer through the interface and the API and
	// never edits a file.
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	// Auth is the first-run choice: whether a caller needs a credential on
	// this deployment. It is absent until a choice is recorded, as on a fresh
	// install. It is in this file and not in the operator's, because the
	// server writes it.
	Auth *overlayAuth `json:"auth,omitempty"`
	// Admission is the second first-run choice: how a new printer joins. It
	// is in this file for the same reason as Auth. It is not under
	// core.connectors, because that block is in the operator's file and this
	// server never writes that file.
	Admission *overlayAdmission `json:"admission,omitempty"`
	// Discovery is the hidden setting a person chose in the web app. It is
	// absent until somebody uses the switch, and while it is absent
	// core.discovery.hidden in the operator's file decides.
	Discovery *overlayDiscovery `json:"discovery,omitempty"`
	// TimeServer is the time server switch a person chose in the web app. It
	// is absent until somebody uses the switch, and while it is absent
	// core.time_server.enabled decides.
	TimeServer *overlayTimeServer `json:"time_server,omitempty"`
	// Marketplace is the marketplace switch a person chose in the web app. It
	// is absent until somebody uses the switch, and while it is absent a
	// configured core.marketplace.url is read.
	Marketplace *overlayMarketplace `json:"marketplace,omitempty"`
	// API is the browser origins a person added in the web app. It is absent
	// until somebody stores a list.
	API *overlayAPI `json:"api,omitempty"`
	// Databases are the modules' own databases that no declaration grants. A
	// removal leaves one, and so does an update that moved the module's data
	// to another place. They stay until the operator purges them.
	Databases []LeftDatabase `json:"databases_left,omitempty"`
}

// overlayAuth is the recorded first-run choice about credentials.
type overlayAuth struct {
	// Mode is SetupOpen or SetupRequired.
	Mode string `json:"mode"`
}

// overlayAdmission is the recorded choice about how a new printer joins.
type overlayAdmission struct {
	// Mode is AdmissionAsk or AdmissionOpen.
	Mode string `json:"mode"`
}

// overlayTimeServer is the recorded time server switch.
type overlayTimeServer struct {
	Enabled bool `json:"enabled"`
}

// overlayDiscovery is the recorded hidden setting.
type overlayDiscovery struct {
	// Hidden says the server announces nothing on the local network.
	Hidden bool `json:"hidden"`
}

// overlayMarketplace is the marketplace switches, as the web app stored them.
type overlayMarketplace struct {
	// Enabled says the server reads the marketplace core.marketplace.url
	// names. It is absent until somebody uses that switch, and the configured
	// marketplace is then read.
	Enabled *bool `json:"enabled,omitempty"`
	// Community says the web app lists the community extensions, every one
	// the marketplace does not mark official. Absent is false, so only the
	// official ones are listed.
	Community bool `json:"community,omitempty"`
}

// overlayPreset is one stored gcode sequence. A preset is configuration only:
// the plane constructs no instance for it, so the kind reports Supervised
// false.
type overlayPreset struct {
	Name    string   `json:"name"`
	Summary string   `json:"summary,omitempty"`
	Models  []string `json:"models,omitempty"`
	Steps   []string `json:"steps"`
	Force   bool     `json:"force,omitempty"`
}

// overlayIdentity is what the store keeps about an instance from either
// input, keyed by the instance's path.
type overlayIdentity struct {
	// UID is the identity the host created, fixed for the life of the
	// instance.
	UID string `json:"uid"`
	// Generation counts accepted changes to the declaration.
	Generation int64 `json:"generation"`
	// Digest is the digest of the declaration the generation was last raised
	// for. A load compares it, so a restart with the same declaration does
	// not count as a change.
	Digest string `json:"digest"`
}

// overlayEntry is one instance as the API or the install command wrote it.
//
// The API writes only types compiled into this binary, so its entries carry
// no runtime and no artefact. `module add` writes a module's entry: its
// runtime, where its artefact is and the artefact's digest, in the same keys
// the operator's file uses.
type overlayEntry struct {
	Kind     string                `json:"kind"`
	Name     string                `json:"name"`
	Type     string                `json:"type"`
	Enabled  bool                  `json:"enabled"`
	Required bool                  `json:"required"`
	Runtime  string                `json:"runtime,omitempty"`
	Service  *install.ServiceEntry `json:"service,omitempty"`
	Asset    *install.AssetEntry   `json:"asset,omitempty"`
	Module   *install.ModuleEntry  `json:"module,omitempty"`
	Config   json.RawMessage       `json:"config,omitempty"`
	// HiddenContributions is Spec.HiddenContributions, written for every kind
	// of entry.
	HiddenContributions []string `json:"hidden_contributions,omitempty"`
	// SwitchedOff is Spec.SwitchedOff, written for a module's entry only.
	SwitchedOff *SwitchOff `json:"switched_off,omitempty"`
}

// fileEntry is one instance as an operator writes it. Enabled is a pointer
// because its default is true: a block the operator wrote is meant to run, so
// a missing key must not read as off.
//
// runtime, service, script and asset are fields here so the strict decode
// accepts the keys. internal/app/config/install defines what they mean and
// which values are legal, and this loader calls it. With two copies of the
// rules, a file could pass the build-tag detector and fail in the running
// binary.
type fileEntry struct {
	Type     string                `yaml:"type"`
	Enabled  *bool                 `yaml:"enabled"`
	Required bool                  `yaml:"required"`
	Runtime  string                `yaml:"runtime"`
	Service  *install.ServiceEntry `yaml:"service"`
	Script   string                `yaml:"script"`
	Asset    *install.AssetEntry   `yaml:"asset"`
	Module   *install.ModuleEntry  `yaml:"module"`
	Reads    []string              `yaml:"reads"`
	Hidden   []string              `yaml:"hidden_contributions"`
	Config   yaml.Node             `yaml:"config"`
}

// Load reads both inputs and merges them. A missing file is an empty one: a
// deployment configured entirely through the API has no resource block, and a
// first run has no overlay.
func Load(paths Paths) (Document, error) {
	read, err := readInputs(paths)
	if err != nil {
		return Document{}, err
	}

	return document(read)
}

// document parses inputs that have already been read. A write uses it,
// because the write keeps those bytes to compare before the rename.
func document(read inputs) (Document, error) {
	fileEntries, failures, err := parseFile(read.name, read.file)
	if err != nil {
		return Document{}, err
	}

	overlay, err := parseOverlay(read.overlay)
	if err != nil {
		return Document{}, err
	}

	overlayEntries, overlayFailures, err := overlayDeclarations(overlay)
	if err != nil {
		return Document{}, err
	}

	doc := merge(fileEntries, overlayEntries, read.version)
	doc.Failures = slices.Concat(failures, overlayFailures)
	doc.Endpoints = declaredEndpoints(overlay)

	slices.SortFunc(doc.Failures, func(a, b Failure) int { return strings.Compare(a.Key, b.Key) })

	for i, declared := range doc.Instances {
		doc.Instances[i].UID, doc.Instances[i].Generation = recorded(overlay, declared.Ref)
	}

	return doc, nil
}

// readInputs reads both files and computes the token over them. The token is
// the hash of the two hashes and not of the two files joined, so moving bytes
// from the end of one file to the start of the other changes the token.
func readInputs(paths Paths) (inputs, error) {
	file, err := readOptional(paths.File)
	if err != nil {
		return inputs{}, err
	}

	overlay, err := readOptional(paths.Overlay)
	if err != nil {
		return inputs{}, err
	}

	fileDigest := sha256.Sum256(file)
	overlayDigest := sha256.Sum256(overlay)
	combined := sha256.Sum256(append(fileDigest[:], overlayDigest[:]...))

	return inputs{
		name:    paths.File,
		file:    file,
		overlay: overlay,
		version: hex.EncodeToString(combined[:]),
	}, nil
}

// readOptional reads a file that is allowed not to exist yet.
func readOptional(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return content, nil
}

// parseFile reads the resources block out of the operator's YAML.
//
// The top level accepts unknown keys, because the same file holds the
// settings of every other subsystem. Each instance block is then decoded with
// unknown keys rejected, so a typo in a key is reported and not ignored.
//
// A block that does not load fails only that instance. Only a resources block
// that is not a map of maps fails the whole document, because then no part of
// it can be read as an instance.
func parseFile(name string, content []byte) ([]Declaration, []Failure, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, nil, nil
	}

	var doc struct {
		Resources map[string]map[string]yaml.Node `yaml:"resources"`
	}

	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, nil, fmt.Errorf("%w: parse the resources block: %w", ErrConfigInvalid, err)
	}

	modes, err := install.Parse(name, content)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrConfigInvalid, err)
	}

	failures := installFailures(modes)
	out := make([]Declaration, 0, len(doc.Resources))

	for kind, named := range doc.Resources {
		// An older build's cameras, which FileCameras reads (ADR-20261116-g5x).
		if kind == ref.KindCamera.String() {
			continue
		}

		for instanceName, node := range named {
			key := "resources." + kind + "." + instanceName

			// An instance the install reader refused is already a failure. It
			// is skipped, so no declaration runs that only one of the two
			// readers accepted.
			if slices.ContainsFunc(failures, func(f Failure) bool { return f.Key == key }) {
				continue
			}

			declared, declErr := fileDeclaration(kind, instanceName, node, modes)
			if declErr != nil {
				failures = append(failures, Failure{
					Key: key,
					Err: fmt.Errorf("%w: %s: %w", ErrConfigInvalid, key, declErr),
				})

				continue
			}

			out = append(out, declared)
		}
	}

	slices.SortFunc(failures, func(a, b Failure) int { return strings.Compare(a.Key, b.Key) })

	return out, failures, nil
}

// readBack refuses a declaration of a kind that no code in this build uses
// after the supervision plane has started it.
//
// The plane would construct the instance, start it and report it ready, and
// no caller would ever call it. For an auth instance that is dangerous: an
// operator who declares single sign-on would see the instance ready while
// every request is still served without it. A control that reports itself
// running while it is not used is worse than a missing one, so the
// declaration fails and the operator reads why.
//
// It fails only this instance. Every caller drops the one declaration and
// keeps the rest, so one bad block does not stop the server.
func readBack(instance ref.Ref) error {
	if instance.Kind().Read() {
		return nil
	}

	return fmt.Errorf("nothing in this build reads back an instance of kind %s, so %s would report "+
		"ready and answer nothing. The kinds this build reads are %s",
		instance.Kind(), instance, kindNames(ref.ReadKinds()))
}

// fileDeclaration turns one instance block into its declaration, or reports why
// it will not load.
func fileDeclaration(
	kind, name string, node yaml.Node, modes install.Document,
) (Declaration, error) {
	instance, err := ref.New(ref.Kind(kind), name)
	if err != nil {
		return Declaration{}, err
	}

	if err = readBack(instance); err != nil {
		return Declaration{}, err
	}

	spec, err := fileSpec(node)
	if err != nil {
		return Declaration{}, err
	}

	spec.Runtime, spec.Service, spec.Script, spec.Asset = installModeOf(modes, instance)

	return Declaration{Ref: instance, Spec: spec, Source: SourceFile}, nil
}

// installFailures converts the failures of internal/app/config/install. An
// install mode this build cannot serve is then refused at load and not at the
// first launch, and only its own instance fails.
func installFailures(modes install.Document) []Failure {
	out := make([]Failure, 0, len(modes.Failures))
	for _, failed := range modes.Failures {
		out = append(out, Failure{Key: failed.Key, Err: fmt.Errorf("%w: %w", ErrConfigInvalid, failed.Err)})
	}

	return out
}

// installModeOf is the install mode the install reader found for this
// instance. A block the install reader refused is already a failure and does
// not reach here. An instance that is missing from the install document gets
// the default, builtin.
func installModeOf(
	modes install.Document, instance ref.Ref,
) (install.Runtime, *install.Service, string, *install.Asset) {
	for _, mode := range modes.Instances {
		if mode.Ref == instance {
			return declaredMode(mode)
		}
	}

	return install.RuntimeBuiltin, nil, "", nil
}

// declaredMode is one install.Instance as the fields of a Spec. The overlay
// and the operator's file both go through it, so a module declared in either
// produces the same spec.
func declaredMode(mode install.Instance) (install.Runtime, *install.Service, string, *install.Asset) {
	switch mode.Runtime {
	case install.RuntimeService:
		service := mode.Service

		return mode.Runtime, &service, "", nil
	case install.RuntimeScript:
		return mode.Runtime, nil, mode.Script, nil
	case install.RuntimeAsset:
		asset := mode.Asset

		return mode.Runtime, nil, "", &asset
	case install.RuntimeBuiltin:
		return install.RuntimeBuiltin, nil, "", nil
	}

	return install.RuntimeBuiltin, nil, "", nil
}

// fileSpec decodes one instance block with unknown keys rejected. The node is
// encoded again first, because yaml.Node.Decode has no option like the
// decoder's KnownFields. The encoded bytes are only decoded here and never
// written to the file.
func fileSpec(node yaml.Node) (Spec, error) {
	raw, err := yaml.Marshal(&node)
	if err != nil {
		return Spec{}, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)

	var entry fileEntry
	if err = decoder.Decode(&entry); err != nil {
		return Spec{}, err
	}

	config, err := yamlToJSON(entry.Config)
	if err != nil {
		return Spec{}, err
	}

	enabled := true
	if entry.Enabled != nil {
		enabled = *entry.Enabled
	}

	return Spec{
		Type:     entry.Type,
		Enabled:  enabled,
		Required: entry.Required,
		Runtime:  install.RuntimeBuiltin,
		Module:   entry.Module,
		Reads:    entry.Reads,

		HiddenContributions: entry.Hidden,
		Config:              config,
	}, nil
}

// yamlToJSON converts an implementation's own settings from YAML to the
// canonical JSON the rest of the system keeps. The core does not read inside
// the block. It stores and returns it.
func yamlToJSON(node yaml.Node) (json.RawMessage, error) {
	if node.IsZero() {
		return nil, nil
	}

	var value any
	if err := node.Decode(&value); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if value == nil {
		return nil, nil
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	return canonicalJSON(encoded)
}

// parseOverlay reads the machine-owned file. An absent one is an empty
// overlay, as on a first run.
func parseOverlay(content []byte) (overlayFile, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return overlayFile{Version: overlayFormat}, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()

	var overlay overlayFile
	if err := decoder.Decode(&overlay); err != nil {
		if key, unknown := strings.CutPrefix(err.Error(), unknownOverlayKey); unknown {
			return overlayFile{}, fmt.Errorf("%w: the overlay carries the key %q, which this build does not "+
				"read. A newer build wrote it. Run that build, or remove the key from the file",
				ErrConfigInvalid, keyPath(content, reflect.TypeFor[overlayFile](), "", strings.Trim(key, `"`)))
		}

		return overlayFile{}, fmt.Errorf("%w: parse the overlay: %w", ErrConfigInvalid, err)
	}

	if overlay.Version != overlayFormat {
		return overlayFile{}, fmt.Errorf("%w: the overlay is format %d, this build reads %d",
			ErrConfigInvalid, overlay.Version, overlayFormat)
	}

	return overlay, nil
}

// keyPath is where a key the decoder did not know sits in the document, with
// the keys above it, as in "discovery.announce_as". The decoder names only the
// key, and a key nested under a member this build reads is otherwise hard to
// find. A key the walk cannot place is named alone.
func keyPath(document []byte, typ reflect.Type, above, key string) string {
	if found := findKey(document, typ, above, key); found != "" {
		return found
	}

	return key
}

func findKey(document []byte, typ reflect.Type, above, key string) string {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	// A type with its own UnmarshalJSON, such as a raw settings block, has no
	// keys this walk can check.
	if reflect.PointerTo(typ).Implements(reflect.TypeFor[json.Unmarshaler]()) {
		return ""
	}

	if typ.Kind() == reflect.Struct {
		return findMember(document, above, key, func(name string) (reflect.Type, bool) { return fieldNamed(typ, name) })
	}

	if typ.Kind() == reflect.Map {
		return findMember(document, above, key, func(string) (reflect.Type, bool) { return typ.Elem(), true })
	}

	if typ.Kind() != reflect.Slice {
		return ""
	}

	var items []json.RawMessage
	if json.Unmarshal(document, &items) != nil {
		return ""
	}

	for i, item := range items {
		if found := findKey(item, typ.Elem(), fmt.Sprintf("%s[%d]", above, i), key); found != "" {
			return found
		}
	}

	return ""
}

// findMember walks the members of one JSON object. member is the type a
// member's value has, or false for a member the type does not have.
//
// The members are walked in the order the document writes them, which is the
// order the decoder read them in, so the key found is the one it refused.
func findMember(document []byte, above, key string, member func(name string) (reflect.Type, bool)) string {
	decoder := json.NewDecoder(bytes.NewReader(document))
	if open, err := decoder.Token(); err != nil || open != json.Delim('{') {
		return ""
	}

	for decoder.More() {
		token, err := decoder.Token()
		name, isName := token.(string)

		var value json.RawMessage
		if err != nil || !isName || decoder.Decode(&value) != nil {
			return ""
		}

		typ, known := member(name)
		if !known {
			if strings.EqualFold(name, key) {
				return joinKey(above, name)
			}

			continue
		}

		if found := findKey(value, typ, joinKey(above, name), key); found != "" {
			return found
		}
	}

	return ""
}

// fieldNamed is the type of the struct field a JSON key fills. The decoder
// matches a key to a field without case, and so does this.
func fieldNamed(typ reflect.Type, key string) (reflect.Type, bool) {
	for field := range typ.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" || !field.IsExported() {
			continue
		}

		if name == "" {
			name = field.Name
		}

		if strings.EqualFold(name, key) {
			return field.Type, true
		}
	}

	return nil, false
}

func joinKey(above, key string) string {
	if above == "" {
		return key
	}

	return above + "." + key
}

// overlayDeclarations turns the overlay's entries into declarations, rejecting
// one that does not name a legal instance.
//
// An entry the API refuses to write fails alone and the rest of the document
// loads. An overlay written by an older binary holds whatever that binary
// accepted, and if such an entry failed the whole load, the server would not
// start after an upgrade.
func overlayDeclarations(overlay overlayFile) ([]Declaration, []Failure, error) {
	out := make([]Declaration, 0, len(overlay.Instances))

	var failures []Failure

	// An older build's camera is FileCameras' to move into the database.
	for _, entry := range slices.DeleteFunc(slices.Clone(overlay.Instances), isCameraEntry) {
		instance, err := ref.New(ref.Kind(entry.Kind), entry.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: overlay entry %s/%s: %w",
				ErrConfigInvalid, entry.Kind, entry.Name, err)
		}

		// A kind that is not supervised has no instances. The API never writes
		// such an entry, so this is a hand edit of the machine-owned file. The
		// supervision plane could not construct it.
		if !instance.Kind().Supervised() {
			return nil, nil, fmt.Errorf("%w: overlay entry %s declares an instance of a kind that has none",
				ErrConfigInvalid, instance)
		}

		// An overlay written by a build that accepted this kind still holds
		// the entry, so it is refused here as well as at the API. The entry
		// stays in the file and the operator reads the reason in the log.
		if unread := readBack(instance); unread != nil {
			failures = append(failures, Failure{
				Key: "resources." + entry.Kind + "." + entry.Name,
				Err: fmt.Errorf("%w: overlay entry %s: %w", ErrConfigInvalid, instance, unread),
			})

			continue
		}

		// A module's entry, written by `module add`. It carries a runtime, an
		// artefact and a digest. install checks it by the same rules as the
		// operator's file, so a declaration means the same in both files.
		if strings.TrimSpace(entry.Runtime) != "" &&
			install.Runtime(strings.TrimSpace(entry.Runtime)) != install.RuntimeBuiltin {
			declared, unread := overlayModule(instance, entry)
			if unread != nil {
				failures = append(failures, Failure{
					Key: "resources." + entry.Kind + "." + entry.Name,
					Err: fmt.Errorf("%w: overlay entry %s: %w", ErrConfigInvalid, instance, unread),
				})

				continue
			}

			out = append(out, declared)

			continue
		}

		// Every other overlay entry is builtin, so only the type name decides
		// whether it may be required.
		if entry.Required && !install.IsCore(install.RuntimeBuiltin, entry.Type) {
			failures = append(failures, Failure{
				Key: "resources." + entry.Kind + "." + entry.Name,
				Err: fmt.Errorf("%w: overlay entry %s: required is true and type %q is not part of the core. "+
					"An instance the core did not write may not hold the whole server out of readiness. "+
					"Set required to false on this instance", ErrConfigInvalid, instance, entry.Type),
			})

			continue
		}

		// A module runs in a process of its own and never inside this binary.
		// Every entry that reaches here is builtin, so a type outside the core
		// namespace would be a module run inside this binary, and the registry
		// has no such type. An overlay written by a binary that accepted one
		// still holds it, so the entry is refused here as well as at the API.
		//
		// The entry is dropped and the process serves, so the server still
		// starts after an upgrade.
		if install.ModuleAsBuiltin(install.RuntimeBuiltin, entry.Type) {
			failures = append(failures, Failure{
				Key: "resources." + entry.Kind + "." + entry.Name,
				Err: fmt.Errorf("%w: overlay entry %s: type %q is not part of the core, and every instance "+
					"this file holds is reached inside this binary. A module runs in a process of its own. "+
					"Declare it in the operator's file with runtime: %s, and delete this entry",
					ErrConfigInvalid, instance, entry.Type, install.RuntimeService),
			})

			continue
		}

		config, err := canonicalJSON(entry.Config)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: overlay entry %s: %w", ErrConfigInvalid, instance, err)
		}

		out = append(out, Declaration{
			Ref: instance,
			Spec: Spec{
				Type:     entry.Type,
				Enabled:  entry.Enabled,
				Required: entry.Required,
				Runtime:  install.RuntimeBuiltin,
				Config:   config,

				HiddenContributions: entry.HiddenContributions,
			},
			Source: SourceOverlay,
		})
	}

	return out, failures, nil
}

// overlayModule reads one module's overlay entry with the install reader that
// also reads the operator's file.
//
// The settings block is kept like any other instance's. The host applies the
// defaults the module's manifest declared, writes this block over them key by
// key, and sends the result to the process. `module add` writes no settings,
// so a module that was installed and never configured runs on its defaults.
func overlayModule(instance ref.Ref, entry overlayEntry) (Declaration, error) {
	config, err := canonicalJSON(entry.Config)
	if err != nil {
		return Declaration{}, err
	}

	enabled := entry.Enabled

	mode, err := install.Declared(instance, install.Entry{
		Type:     entry.Type,
		Enabled:  &enabled,
		Required: entry.Required,
		Runtime:  entry.Runtime,
		Service:  entry.Service,
		Asset:    entry.Asset,
	})
	if err != nil {
		return Declaration{}, err
	}

	runtime, service, script, asset := declaredMode(mode)

	return Declaration{
		Ref: instance,
		Spec: Spec{
			Type:     mode.Type,
			Enabled:  enabled,
			Required: entry.Required,
			Runtime:  runtime,
			Service:  service,
			Script:   script,
			Asset:    asset,
			Module:   entry.Module,
			Config:   config,

			HiddenContributions: entry.HiddenContributions,
			SwitchedOff:         entry.SwitchedOff,
		},
		Source: SourceOverlay,
	}, nil
}

// recorded returns the identity and generation the overlay holds for an
// instance. A missing or malformed UID reads as absent and does not fail the
// load. Only the store writes that field, and a bad value in it must not stop
// the server from starting.
func recorded(overlay overlayFile, instance ref.Ref) (ref.UID, int64) {
	identity, ok := overlay.Identities[instance.Path()]
	if !ok {
		return ref.UID{}, 0
	}

	uid, err := ref.ParseUID(identity.UID)
	if err != nil {
		return ref.UID{}, identity.Generation
	}

	return uid, identity.Generation
}
