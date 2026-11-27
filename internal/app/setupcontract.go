package app

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/authz"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/platform/observability/prom"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

// wizardHidden are the keys the first-run wizard never offers.
//
// Most decide where the server writes its own files, and the wizard writes
// into one of them. The org handover opens a part of the grant API for
// integrators who run several organisations. A new install has one
// organisation, so the wizard does not ask. The modules key belongs to the
// operator: a value the wizard wrote would apply wherever the operator's file
// says nothing.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var wizardHidden = map[string]bool{
	"core.overlay":                  true,
	"core.modules":                  true,
	"core.authz.database":           true,
	"core.auth.accounts":            true,
	"core.auth.setup_code":          true,
	"core.authz.org_admin_handover": true,
	"core.modules_enabled":          true,
}

// groupServer and groupDatabase are the groups of the keys that sit directly
// under core, and of the persistence block.
const (
	groupServer   = "server"
	groupDatabase = "database"
)

// defaultStorageFolder is the folder a storage the wizard declares keeps its
// files in, relative to the folder the server starts in. It is the folder the
// shipped configuration names.
const defaultStorageFolder = "storage"

// wizardFields are the keys the wizard may ask, in the order the reference
// page lists them, with the choices this binary can run.
func wizardFields() []config.Field {
	defaults := config.Default()

	fields := slices.Concat(
		config.Describe("core", reflect.TypeFor[config.Core](), reflect.ValueOf(defaults)),
		config.Describe("persistence", reflect.TypeFor[config.Persistence](), reflect.ValueOf(defaults.Persistence)),
	)

	return slices.DeleteFunc(fields, func(field config.Field) bool { return !offered(field) })
}

// optionalPart is a part of the server that a build tag adds or takes out, the
// keys that configure it, and whether this binary carries it.
type optionalPart struct {
	// keys is one key path, or a prefix that ends in a dot.
	keys string
	// built is the build-tagged constant that says this binary carries it.
	built bool
	// without says why this binary cannot run the keys, naming the tag.
	without string
}

// optionalParts are the parts whose keys the wizard offers only when this
// binary carries them. adapters/tags.json maps the same keys onto the same tags
// for `make build`.
func optionalParts() []optionalPart {
	return []optionalPart{
		{
			keys: "core.observability.", built: prom.Compiled,
			without: "this binary was built without the observability tag, so it links no metrics client",
		},
		{
			keys: "core.diagnostics.", built: diagnosticsBuilt,
			without: "this binary was built without the diagnostics tag, so it carries no support bundle",
		},
		{
			keys: "core.dashboards.enabled", built: dashboardsBuilt,
			without: "this binary was built with the nodashboards tag, so it carries no dashboards",
		},
		{
			keys: "persistence.postgres.", built: authz.PostgresLinked,
			without: "this binary was built without the postgres tag, so it links no PostgreSQL driver",
		},
	}
}

// notBuilt returns the part a key belongs to when this binary was built
// without it.
func notBuilt(path string) (optionalPart, bool) {
	for _, part := range optionalParts() {
		matches := path == part.keys || (strings.HasSuffix(part.keys, ".") && strings.HasPrefix(path, part.keys))
		if matches && !part.built {
			return part, true
		}
	}

	return optionalPart{}, false
}

// offered says whether the wizard asks a key. It leaves out what it cannot
// draw, a key under a name the operator chooses or a raw mapping, and a key
// whose part this binary was built without.
func offered(field config.Field) bool {
	_, missing := notBuilt(field.Path)

	switch {
	case wizardHidden[field.Path],
		field.Kind == config.KindMapping,
		strings.Contains(field.Path, config.Placeholder),
		missing:
		return false
	}

	// An engine selector with one engine to select is not a choice.
	return len(choicesOf(field.Path)) != 1
}

// choicesOf is the closed set a key takes in this binary.
func choicesOf(path string) []string {
	choices := config.Keys[path].Choices

	if path == "persistence.type" && !authz.PostgresLinked {
		return slices.DeleteFunc(slices.Clone(choices), func(choice string) bool {
			return choice == config.EnginePostgres
		})
	}

	return choices
}

// blockedDepth is the length of a path that names a block under core, such as
// core.api.addr. A shorter one is a key directly under core.
const blockedDepth = 3

// groupOf is the section of the wizard a key is drawn in: the block under core
// it belongs to, the server for a key directly under core, and the database
// for the persistence block.
func groupOf(path string) string {
	parts := strings.Split(path, ".")

	switch {
	case parts[0] == "persistence":
		return groupDatabase
	case len(parts) >= blockedDepth:
		return parts[1]
	}

	return groupServer
}

// levelOf is the level a key is asked at, advanced when its row names none.
func levelOf(path string) string {
	if level := config.Keys[path].Level; level != "" {
		return level
	}

	return config.LevelAdvanced
}

// setupContract is the configuration the wizard reads, for the operator file
// at configPath.
func setupContract(configPath string) (httpapi.SetupConfiguration, error) {
	written, err := config.Written(configPath)
	if err != nil {
		return httpapi.SetupConfiguration{}, err
	}

	fields := wizardFields()
	keys := make([]httpapi.SetupKey, 0, len(fields))

	for _, field := range fields {
		documented := config.Keys[field.Path]

		key := httpapi.SetupKey{
			Path:      field.Path,
			Group:     groupOf(field.Path),
			Level:     levelOf(field.Path),
			Kind:      string(field.Kind),
			Default:   defaultValue(field),
			Choices:   choicesOf(field.Path),
			Doc:       documented.Doc,
			Required:  documented.Required,
			Secret:    documented.Secret,
			DependsOn: documented.DependsOn,
			Reference: config.KeyReference(field.Path),
			Guide:     documented.See,
		}

		if value, set := written[field.Path]; set {
			key.Locked = true

			if !key.Secret {
				key.Value = value
			}
		}

		keys = append(keys, key)
	}

	declared, err := declaresStorage(configPath)
	if err != nil {
		return httpapi.SetupConfiguration{}, err
	}

	return httpapi.SetupConfiguration{
		Hosts:       suggestedNames(),
		Printers:    !declaresConnectors(written),
		PrintersTLS: declaresPrinterTLS(written),
		ConfigFile:  setupFileOf(configPath),
		SetupFile:   setupFileOf(config.SetupFile(configPath)),
		Keys:        keys,
		Storage:     httpapi.SetupStorage{Root: defaultStorageRoot(), Declared: declared},
	}, nil
}

// implicitDefaults are the defaults Default() leaves unset because a reader
// fills them: an absent dashboards switch is on, and the SQLite page cache is
// filled only when the engine is SQLite. The wizard shows what the server
// will run with.
func implicitDefaults() map[string]any {
	return map[string]any{
		"core.dashboards.enabled":             true,
		"core.frontend.enabled":               true,
		"persistence.sqlite.page_cache_bytes": int64(config.DefaultSQLitePageCacheBytes),
	}
}

// defaultValue is the default of one key as JSON carries it: a duration in the
// spelling a person writes, and nil for a key with no default.
func defaultValue(field config.Field) any {
	if implicit, ok := implicitDefaults()[field.Path]; ok {
		return implicit
	}

	value := field.Default
	if !value.IsValid() {
		return nil
	}

	if field.Kind == config.KindDuration {
		if value.Int() == 0 {
			return nil
		}

		return config.WrittenDuration(time.Duration(value.Int()))
	}

	//exhaustive:ignore // the kinds a configuration key can be.
	switch value.Kind() {
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int64:
		if value.Int() == 0 {
			return nil
		}

		return value.Int()
	case reflect.String:
		if value.String() == "" {
			return nil
		}

		return value.String()
	case reflect.Slice:
		out := make([]string, 0, value.Len())

		for index := range value.Len() {
			out = append(out, fmt.Sprint(value.Index(index).Interface()))
		}

		return out
	}

	return nil
}

// setupFileOf is one file as the wizard names it to a person: absolute, and whether
// it is there.
func setupFileOf(path string) httpapi.SetupFile {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}

	_, statErr := os.Stat(path)

	return httpapi.SetupFile{Path: absolute, Exists: statErr == nil}
}

// defaultStorageRoot is the folder the wizard offers for uploads: storage in
// the folder the server starts in, where every relative path of the
// configuration resolves.
func defaultStorageRoot() string {
	absolute, err := filepath.Abs(defaultStorageFolder)
	if err != nil {
		return defaultStorageFolder
	}

	return absolute
}

// declaresStorage says whether the operator's file declares a storage, in
// which case the wizard asks for none.
func declaresStorage(configPath string) (bool, error) {
	written, err := config.Written(configPath)
	if err != nil {
		return false, err
	}

	prefix := "resources." + string(ref.KindStorage) + "."
	for path := range written {
		if strings.HasPrefix(path, prefix) {
			return true, nil
		}
	}

	return false, nil
}

// declaresConnectors says whether the operator's file writes the connectors,
// in which case the printers' listener is theirs and the wizard writes none.
func declaresConnectors(written map[string]any) bool {
	for path := range written {
		if strings.HasPrefix(path, "core.connectors.") {
			return true
		}
	}

	return false
}

// declaresPrinterTLS says whether the operator's file gives a connector's
// listener a certificate, so a wizard that may not touch the connectors still
// shows that printers are encrypted.
func declaresPrinterTLS(written map[string]any) bool {
	for path, value := range written {
		if strings.HasPrefix(path, "core.connectors.") &&
			strings.HasSuffix(path, ".config.listener.tls.certificate") && value != "" {
			return true
		}
	}

	return false
}

// maxAnnouncedAddresses bounds the network addresses the setup banner prints,
// so a machine with many interfaces does not bury the code.
const maxAnnouncedAddresses = 3

// browserAddresses are the addresses a person opens the first run at: this
// machine for a browser on it, and its addresses on the local network for one
// on another computer. A listener bound to one host is reached at that host.
func browserAddresses(api config.Listener) []string {
	scheme := "http"
	if api.TLS.Enabled() {
		scheme = "https"
	}

	host, port, err := net.SplitHostPort(api.Addr)
	if err != nil {
		return nil
	}

	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{scheme + "://" + net.JoinHostPort(host, port)}
	}

	out := []string{scheme + "://" + net.JoinHostPort("localhost", port)}

	for _, name := range suggestedNames() {
		if net.ParseIP(name) == nil || len(out) > maxAnnouncedAddresses {
			continue
		}

		out = append(out, scheme+"://"+net.JoinHostPort(name, port))
	}

	return out
}
