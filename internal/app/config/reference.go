package config

import (
	"strconv"
	"strings"
)

// Key is one configuration key as the reference page prints it.
//
// The sentences are in a table and not in struct tags, because one Go type can
// be mounted under two paths, and each path needs its own sentence.
type Key struct {
	// Doc is the sentence the page prints, thirty words at most. The generator
	// refuses a key with an empty Doc.
	Doc string
	// Default is the text the page prints in the default column. Set it for a
	// key whose default depends on the path of the configuration file, because
	// Default() cannot return that value. When it is empty, the page prints the
	// value from Default().
	Default string
	// Required says when a deployment has to set the key. Empty means never:
	// the default works.
	Required string
	// Level is the path of the first-run wizard that asks for the key:
	// LevelBasic or LevelAdvanced. Empty is LevelAdvanced.
	Level string
	// Choices is the closed set of values the key takes, for a key that has
	// one. The wizard offers them and nothing else.
	Choices []string
	// Secret says the value must not be shown back once it is written. The
	// wizard shows only that the key is set.
	Secret bool
	// See is the page of the documentation that explains the part this key
	// belongs to, as its route with a trailing slash, such as certificates/.
	// The wizard links it beside the key's own row in the reference.
	See string
	// DependsOn names the key that turns this one on: a switch, written as its
	// path, or a key with a closed set, written as path=value. A text key
	// written as its path turns this one on when it is not empty. The wizard
	// draws this key under that one and only while it is on.
	DependsOn string
}

// ReferencePage is the route of the page that lists every key.
const ReferencePage = "config/"

// KeyAnchor is the anchor of a key's row on the reference page, such as
// key-core-api-addr. A placeholder segment reads as name.
func KeyAnchor(path string) string {
	path = strings.NewReplacer("<", "", ">", "").Replace(path)

	return "key-" + strings.ReplaceAll(strings.ReplaceAll(path, ".", "-"), "_", "-")
}

// The levels of the first-run wizard. The basic path asks the keys at
// LevelBasic, and the advanced path asks every key.
const (
	LevelBasic    = "basic"
	LevelAdvanced = "advanced"
)

// Keys has one row for each key the core reads. The map key is the full dotted
// path an operator writes. cmd/generate-config-reference reads the paths, the
// types and the defaults from the configuration structs, and the sentences
// from this map. The command fails when a struct has a key with no row here,
// and when a row here names a key that no struct has.
//
//nolint:gochecknoglobals // the inventory of documented keys, read by the generator and never written.
var Keys = map[string]Key{
	"core.api.addr": {
		Doc:   "The host and port the HTTP API, the web app and the event stream are served on.",
		Level: LevelBasic,
	},
	"core.api.tls.certificate": {
		Doc: "PEM file with the certificate of this server, followed by the certificates of the authorities " +
			"that signed it. When both TLS keys are empty, the server uses plain HTTP.",
		See: "certificates/",
	},
	"core.api.tls.key": {
		Doc:       "PEM file with the private key for that certificate.",
		DependsOn: "core.api.tls.certificate",
		See:       "certificates/",
	},
	"core.api.allowed_origins": {
		Doc: "Browser origins that may call this API from a page this server did not serve itself. " +
			"A wildcard is refused.",
		See: "using-the-api-from-a-page/",
	},
	"core.api.request_timeout": {
		Doc: "How long one request may take before the server stops waiting for its own handler and answers 504.",
	},
	"core.api.stream_timeout": {
		Doc: "The same limit for the routes that stream events or transfer a file. " +
			"It may not be shorter than request_timeout.",
	},
	"core.log.level": {
		Doc:     "How much the process writes to its log: debug, info, warn or error.",
		Choices: []string{"debug", "info", "warn", "error"},
		See:     "logging/",
	},
	"core.log.format": {
		Doc:     "How one log line is written: text or json.",
		Choices: []string{"text", "json"},
		See:     "logging/",
	},
	"core.frontend.enabled": {
		Doc: "Serve the web app at this server's root address. When false, the root address answers 404 " +
			"and make build does not compile the app.",
	},
	"core.frontend.title": {
		Doc: "The name in the browser tab, at the top of the sidebar and on the local network over " +
			"mDNS, 64 bytes at most.",
		Level: LevelBasic,
	},
	"core.frontend.default_language": {
		Doc:       "The language the page falls back to: en, cs, de, es, fr, it or pl.",
		Level:     LevelBasic,
		Choices:   []string{"en", "cs", "de", "es", "fr", "it", "pl"},
		DependsOn: "core.frontend.enabled",
	},
	"core.frontend.theme": {
		Doc:       "The palette the page opens in: system, light or dark. system follows the operating system.",
		Level:     LevelBasic,
		Choices:   []string{"system", "light", "dark"},
		DependsOn: "core.frontend.enabled",
	},
	"core.frontend.printer_address": {
		Doc: "The origin printers dial, such as http://192.168.1.50:8080, written into the add-printer " +
			"page's settings file. Not a loopback name. Empty uses the page's own address.",
		DependsOn: "core.frontend.enabled",
		See:       "printer-configuration/",
	},
	"core.frontend.printer_custom_ca": {
		Doc: "The printers' certificate is signed by an authority of your own, so the add-printer page " +
			"asks the printer to trust the connect.der on its USB stick.",
		DependsOn: "core.frontend.printer_address",
		See:       "certificates/",
	},
	"core.frontend.support_url": {
		Doc:       "Where the app's Support link points. Empty keeps the assistant on the beta portal.",
		DependsOn: "core.frontend.enabled",
	},
	"core.dashboards.enabled": {
		Doc: "Serve the dashboards the home page draws. Off, the home page is the printer list, or the " +
			"printer's page when there is one. `make build` leaves them out.",
		Default:   "`true`",
		DependsOn: "core.frontend.enabled",
		See:       "dashboards/",
	},
	"core.diagnostics.enabled": {
		Doc: "Serve the diagnostics bundle an org admin downloads from Settings. Off, `make build` leaves " +
			"it out. The minimal build never carries it. With no sign-in, off unless true.",
	},
	"core.diagnostics.send_url": {
		Doc: "The portal Settings sends the diagnostics bundle to. The browser uploads it, never the " +
			"server. Empty hides Send to Prusa.",
		DependsOn: "core.diagnostics.enabled",
	},
	"core.discovery.enabled": {
		Doc:   "Announce this server on the local network over mDNS, and list the other printers it hears there.",
		Level: LevelBasic,
	},
	"core.discovery.services": {
		Doc:       "The mDNS service names this server announces itself under, one record for each.",
		Default:   "`_prusalink._tcp` and `_prusa-connect._tcp`",
		DependsOn: "core.discovery.enabled",
	},
	"core.discovery.hidden": {
		Doc: "Start hidden: the server announces nothing, so PrusaSlicer does not find it, and still hears " +
			"the printers. The web app's switch wins once a person uses it.",
		Level:     LevelBasic,
		DependsOn: "core.discovery.enabled",
	},
	"core.discovery.host_records": {
		Doc: "Who answers for this machine's host name: own, this server, or system, another mDNS responder " +
			"on this machine such as Avahi. With system the server announces only its services.",
		Choices:   []string{HostRecordsOwn, HostRecordsSystem},
		DependsOn: "core.discovery.enabled",
	},
	"core.time_server.enabled": {
		Doc: "Answer SNTP requests on UDP port 123 with this machine's clock, for devices on a network with no " +
			"internet. The web app's switch wins once a person uses it.",
	},
	"core.observability.enabled": {
		Doc: "Serve the metrics at /metrics. Off, the server answers 404 there. A binary built without the " +
			"Prometheus client refuses true.",
		See: "metrics/",
	},
	"core.observability.addr": {
		Doc: "Serve the metrics on a port of their own, written as host:port. That port asks for no " +
			"credential, so publish it to your collector and to nothing else.",
		Default:   "empty, so the metrics stay on the API port, where a scrape signs in",
		DependsOn: "core.observability.enabled",
		See:       "metrics/",
	},
	"core.overlay": {
		Doc: "The file where the resource API saves the resources it declares. The server never changes " +
			"your configuration file.",
		Default: "`config.d/resources.json` beside your file",
	},
	"core.modules": {
		Doc:     "The directory the server and the module add command install a module into.",
		Default: "`config.d/modules` beside your file",
	},
	"core.modules_local": {
		Doc: "Release directories on this server the Extensions page offers to install, " +
			"each under the module path written as its key.",
	},
	"core.modules_minimum_stability": {
		Doc: "The lowest stability of a module release an install takes: dev, alpha, beta, RC or stable, " +
			"as Composer's minimum-stability. A constraint such as ^1.2@beta lowers it for one module.",
		Default: "`stable`",
	},
	"core.modules_prefer_stable": {
		Doc:     "Take the most stable release a constraint allows before the newest one, as Composer's prefer-stable.",
		Default: "`false`",
	},
	"core.modules_enabled": {
		Doc: "Write false to refuse every module install and to start no module the server installed. " +
			"Modules declared in this file still run. Read at start.",
		Default: "`true`",
		See:     "extensions/",
	},
	"core.marketplace.enabled": {
		Doc: "Whether the Extensions page reads the marketplace catalogue. Write false to read none, " +
			"and the server then fetches nothing from it.",
		Level: LevelBasic,
		See:   "extensions/",
	},
	"core.marketplace.url": {
		Doc: "The marketplace whose module catalogue the Extensions page lists and installs from. " +
			"An https address; http only for a loopback host.",
		DependsOn: "core.marketplace.enabled",
		See:       "extensions/",
	},
	"core.marketplace.timeout": {
		Doc:       "How long one read of the marketplace catalogue waits.",
		DependsOn: "core.marketplace.enabled",
		See:       "extensions/",
	},
	"core.upgrade.enabled": {
		Doc: "Whether this server prepares upgrades, keeps backups and pairs with the Beta Portal. " +
			"Keep it true under Docker and Kubernetes.",
		Default: "`true`",
		See:     "upgrade-back-up-and-uninstall/",
	},
	"core.upgrade.server": {
		Doc:     "The Beta Portal base URL used for device pairing, online release discovery and downloads.",
		Default: "`https://beta.prusa3d.com`",
	},
	"core.upgrade.allow_unsigned": {
		Doc: "Whether an uploaded core release may omit its Ed25519 signature. Keep false outside local development.",
	},
	"core.upgrade.trusted_keys": {
		Doc: "Additional Ed25519 public keys accepted for operator-built releases, each with an id and base64 key.",
	},
	"core.upgrade.backup_retention": {
		Doc:     "How many completed core backups remain after a new backup is published.",
		Default: "`5`",
	},
	"core.upgrade.postgres_backup": {
		Doc:     "How PostgreSQL is protected before an upgrade: pg_dump, external or none.",
		Default: "`external`",
	},
	"core.upgrade.rehearsal_dsn": {
		Doc: "An optional PostgreSQL clone used for migration rehearsal instead of rolling back a transaction on the live server.",
	},
	"core.upgrade.trial_timeout": {
		Doc:     "How long the upgraded child process may take to satisfy its readiness gate.",
		Default: "`1m`",
	},
	"core.upgrade.release_repository": {
		Doc: "A legacy GitHub owner and repository used instead of core.upgrade.server when explicitly configured.",
	},
	"core.principal": {
		Doc: "The first administrator, and the user every request runs as while nobody signs in. " +
			"With core.auth.issuer set, it is the administrator's sub claim at the issuer.",
	},
	"core.auth.issuer": {
		Doc: "The OpenID Connect issuer people sign in against. Empty is a deployment with no such provider.",
		See: "human-auth/",
	},
	"core.auth.provider_name": {
		Doc:       "What the sign-in button calls the issuer, as in Sign in with <name>. Empty takes the issuer's host name.",
		DependsOn: "core.auth.issuer",
		See:       "human-auth/",
	},
	"core.auth.client_id": {
		Doc:       "The client id this server is registered under at that issuer.",
		Required:  "when core.auth.issuer is set",
		DependsOn: "core.auth.issuer",
		See:       "human-auth/",
	},
	"core.auth.client_secret": {
		Doc: "The secret a confidential client authenticates the code exchange with. Empty is a public " +
			"client that PKCE alone protects. Google and most providers' web clients need it.",
		Secret:    true,
		DependsOn: "core.auth.issuer",
		See:       "human-auth/",
	},
	"core.auth.redirect_uri": {
		Doc: "The address the issuer sends the browser back to. Its scheme also decides whether the session " +
			"cookie is marked Secure.",
		Required:  "when core.auth.issuer is set",
		DependsOn: "core.auth.issuer",
		See:       "human-auth/",
	},
	"core.auth.local": {
		Doc: "Serve a password login against this server's own account store. It cannot be set together with " +
			"an issuer.",
		See: "local-accounts/",
	},
	"core.auth.accounts": {
		Doc: "An account file from an earlier build, imported into the database once when it holds no account. Setting it while core.auth.local is false is refused.",
		Default: "`config.d/accounts.json` beside your file, and unset while " +
			"core.auth.local is false",
		See: "local-accounts/",
	},
	"core.auth.password_memory_bytes": {
		Doc: "The memory, in bytes and a whole number of KiB, that one argon2id hash fills. A cost below " +
			"OWASP's settings is refused. The printer build defaults to 7340032.",
		See: "local-accounts/",
	},
	"core.auth.password_passes": {
		Doc: "How many passes one argon2id password hash makes over that memory. Less memory needs more " +
			"passes. The printer build defaults to 5.",
		See: "local-accounts/",
	},
	"core.auth.password_hashes_at_once": {
		Doc: "How many password hashes may run at the same time. A login past it waits within its request " +
			"timeout. The printer build defaults to 1.",
		See: "local-accounts/",
	},
	"core.auth.setup_code": {
		Doc: "The first-run code: 12 to 64 printable ASCII characters besides dashes and spaces, 6 of them " +
			"different. Empty prints a new random code at each start until setup is done.",
		Secret: true,
	},
	"core.authz.database": {
		Doc: "The SQLite file that holds the whole database when persistence.type is sqlite. Postgres does " +
			"not use it. The server creates it on the first run.",
		Default: "`config.d/authz.db` beside your file",
	},
	"core.authz.org_admin_handover": {
		Doc: "Let the grant API write an organisation's admin role. Off, a person becomes an administrator " +
			"only on the People page or through set-account-role.",
	},
	"core.trusted_proxies": {
		Doc: "The networks a reverse proxy reaches this server from, as CIDR blocks or single addresses. " +
			"X-Forwarded-For is read only from those.",
	},
	"core.site": {
		Doc: "This node's name in every event this server publishes. Give each node of a federation its own.",
	},
	"core.events.heartbeat": {
		Doc: "How often an idle event stream writes a keep-alive frame. Keep it under whatever closes idle " +
			"connections on the way to the reader.",
		See: "sse/",
	},
	"core.events.retry": {
		Doc: "How long a reader is told to wait before reconnecting after the event stream drops.",
		See: "sse/",
	},
	"core.events.max_streams_per_principal": {
		Doc: "How many event streams one user or module may have open at once. " +
			"A request over the limit is answered 429.",
		See: "sse/",
	},
	"core.connectors.<name>.type": {
		Doc:      "The printer driver this instance runs, for example xbuddy.",
		Required: "when the instance is written",
	},
	"core.connectors.<name>.enabled": {
		Doc:     "Start this instance. An instance with no enabled key is started.",
		Default: "`true`",
	},
	"core.connectors.<name>.config": {
		Doc: "The settings of the driver. The core passes the block to the driver and does not read it.",
	},
	"core.device_state_budget": {
		Doc: "How many bytes of reported state the server keeps for one device. At the limit the server " +
			"refuses a new state path and still updates the paths it has.",
	},
	"core.module_settings_bytes": {
		Doc: "The largest size, in bytes, of one record that the settings pane of a module writes. " +
			"At most 65536.",
	},
	"core.preview_cache_bytes": {
		Doc: "How many bytes of preview pictures from print files the server keeps in memory. " +
			"The printer build defaults to 1048576.",
	},
	"core.device_listing_cache_bytes": {
		Doc: "How many bytes of printer folder listings the server keeps in memory, across every " +
			"printer. A repeat listing is answered from it at once. The printer build defaults to 262144.",
	},
	"core.device_thumbnail_disk_bytes": {
		Doc: "How many bytes of printer file pictures the server keeps on disk, in printer-thumbnails " +
			"beside the overlay. Over it the oldest go. The printer build defaults to 4194304.",
		DependsOn: "core.device_thumbnail_disk",
	},
	"core.device_thumbnail_disk": {
		Doc: "Whether the server keeps printer file pictures on disk so a restart does not ask again. " +
			"Off writes nothing there. The printer build defaults to false.",
	},
	"core.device_thumbnail_prefetch": {
		Doc: "Whether the server asks an idle printer for the pictures of a listed folder's print " +
			"files before anybody opens them. The printer build defaults to false.",
	},
	"core.memory_limit": {
		Doc: "The soft memory limit of the Go runtime, in bytes. When it is not set, Go reads " +
			"GOMEMLIMIT. The server logs the limit at start when it sets one.",
	},
	"core.modules_memory": {
		Doc: "The bytes the budgets of all enabled service modules must fit in, at least 67108864. " +
			"When absent, the machine's memory less a quarter, keeping at least 256 MiB.",
		DependsOn: "core.modules_enabled",
	},
	"core.modules_switch_off.failures": {
		Doc: "How many failed starts or stops inside the window switch a service module off, from 0 to 100. " +
			"0 never switches one off. A module may set its own.",
		DependsOn: "core.modules_enabled",
	},
	"core.modules_switch_off.window": {
		Doc: "How far back failed starts and stops count towards switching a service module off, from 1m " +
			"to 24h. A module may set its own.",
		DependsOn: "core.modules_enabled",
	},
	"core.shutdown_timeout": {
		Doc: "How long a shutdown waits for open requests to end, and then how long it waits for the " +
			"resources to stop.",
	},
	"persistence.type": {
		Doc:     "The database engine: sqlite or postgres. Any other value stops the server.",
		Choices: []string{EngineSQLite, EnginePostgres},
	},
	"persistence.max_open_conns": {
		Doc: "How many database connections this process opens at once, in total for all its stores.",
	},
	"persistence.max_idle_conns": {
		Doc: "How many of those connections stay open while no query runs.",
	},
	"persistence.conn_max_lifetime": {
		Doc: "How long one connection is used before it is replaced. A running server then picks up a " +
			"database failover or a new password.",
	},
	"persistence.sqlite.page_cache_bytes": {
		Doc: "How many bytes of database pages one SQLite connection keeps in memory. The default is " +
			"SQLite's own. The printer build defaults to 524288. Refused on Postgres.",
		Default:   "`" + strconv.Itoa(DefaultSQLitePageCacheBytes) + "`",
		DependsOn: "persistence.type=sqlite",
	},
	"persistence.postgres.dsn": {
		Doc: "The connection string. The server passes it to the database driver and does not parse it. " +
			"A postgres:// URL and a key=value string both work.",
		Required:  "when persistence.type is postgres",
		Secret:    true,
		DependsOn: "persistence.type=postgres",
	},
	"persistence.postgres.module_admin_dsn": {
		Doc: "The postgres:// URL the server makes and drops a module's database and role with. " +
			"Its role needs CREATEDB and CREATEROLE. Empty uses dsn. Refused on sqlite.",
		Secret:    true,
		DependsOn: "persistence.type=postgres",
	},
	"persistence.postgres.statement_timeout": {
		Doc: "How long one statement may run on Postgres. A connection string that already names " +
			"statement_timeout keeps its own value.",
		DependsOn: "persistence.type=postgres",
	},
}
