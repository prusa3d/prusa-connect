// Package config holds the core's configuration types, their defaults and
// their validation.
//
// It reads one block out of the operator's file. The rest of that file belongs
// to other subsystems, and the resources block belongs to
// internal/app/resource, which reads the same path for itself.
//
//revive:disable:file-length-limit
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/prusa3d/prusa-connect/internal/app/config/install"
	"github.com/prusa3d/prusa-connect/internal/kernel/buildprofile"
	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
	"github.com/prusa3d/prusa-connect/internal/platform/auth"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
)

// ErrInvalid reports a configuration the core will not start on. Callers
// inspect it with errors.Is; the message names the key and the value.
var ErrInvalid = errors.New("config: invalid configuration")

// Defaults. Every one of them is a value a deployment can run on unchanged,
// because a first run has no configuration file at all.
const (
	// DefaultAPIAddr is where the HTTP API and the OpenAPI reference are
	// served.
	DefaultAPIAddr = ":8080"
	// DefaultRequestTimeout is how long one API request may take before the
	// server stops waiting for its own handler and answers
	// core.validation/REQUEST_TIMED_OUT. It equals the listener's read timeout.
	DefaultRequestTimeout = 30 * time.Second
	// DefaultStreamTimeout is the same bound for the routes that hold their
	// answer open: the event stream, the camera stream, the printer transfers
	// and the storage content routes. It is long enough for the largest file
	// any of them moves over a slow link, and a stream cut at it costs its
	// reader one reconnection.
	DefaultStreamTimeout = 6 * time.Hour
	// DefaultLogLevel is the slog level the process starts at.
	DefaultLogLevel = "info"
	// DefaultLogFormat is the handler the process starts with.
	DefaultLogFormat = "text"
	// DefaultPrincipal is the subject every request resolves to while no
	// authentication provider is wired.
	DefaultPrincipal = "operator"
	// DefaultSite is the node token every event this process publishes carries.
	// A federated deployment sets its own: two nodes calling themselves the
	// same thing publish events a parent cannot tell apart.
	DefaultSite = "local"
	// DefaultShutdownTimeout bounds both halves of a shutdown: draining the
	// listeners, and then waiting for the supervision plane.
	DefaultShutdownTimeout = 15 * time.Second
	// DefaultHeartbeat is how often an idle event stream writes a comment
	// frame. It is under the 30 to 60 seconds a proxy or a load balancer
	// usually allows an idle connection, so a quiet stream is not closed.
	DefaultHeartbeat = 15 * time.Second
	// DefaultRetry is the reconnection delay an idle event stream hands its
	// reader. It is under the three seconds every major browser engine
	// defaults to. This server is on the same network as its readers, and a
	// shorter gap gives a better chance of resuming inside the replay window.
	DefaultRetry = 2 * time.Second
	// DefaultMaxStreamsPerPrincipal bounds how many event streams one principal
	// may hold. It is stream.DefaultMaxPerPrincipal as an operator key. The
	// hub holds the bound.
	DefaultMaxStreamsPerPrincipal = 64
	// DefaultDeviceStateBudget is how many bytes of one device's state tree
	// this process retains. A full xBuddy tree spends a small fraction of it.
	DefaultDeviceStateBudget = 256 << 10
	// DefaultModuleSettingsBytes is how many bytes one record a module's
	// browser half writes may hold. A settings pane keeps a few choices, and a
	// document that does not fit is a file and belongs on a storage instance.
	DefaultModuleSettingsBytes = 4 << 10
	// DefaultMaxOpenConns bounds how many database queries run at once. Over
	// it a caller waits in the connection pool, where its context can cancel
	// it. Inside the database it cannot.
	DefaultMaxOpenConns = 16
	// DefaultMaxIdleConns is how many of those stay open while nothing is
	// asking. It is half the pool: a quiet server answers a burst without
	// dialling and closes the rest.
	DefaultMaxIdleConns = 8
	// DefaultConnMaxLifetime is how long one connection is used before it is
	// replaced. A failover, a rotated credential and a restarted proxy then
	// reach a server that has been up for weeks.
	DefaultConnMaxLifetime = 30 * time.Minute
	// DefaultStatementTimeout bounds one statement on Postgres. Every query
	// this server sends answers a request, and no request should hold the
	// database for half a minute.
	DefaultStatementTimeout = 30 * time.Second
	// DefaultBackupRetention is how many completed snapshots are retained.
	DefaultBackupRetention = 5
	// DefaultPostgresBackup records that managed PostgreSQL installations own
	// their backup outside this process.
	DefaultPostgresBackup = "external"
	// DefaultUpgradeTrialTimeout bounds the new binary's readiness gate.
	DefaultUpgradeTrialTimeout = time.Minute
	// DefaultUpgradeServer is the machine update API used for online core
	// release discovery and downloads.
	DefaultUpgradeServer = "https://beta.prusa3d.com"

	// DefaultPreviewCacheBytes is how much preview picture the server keeps in
	// memory. The printer build keeps less, and buildprofile holds both values.
	DefaultPreviewCacheBytes = buildprofile.PreviewCacheBytes
	// DefaultSQLitePageCacheBytes bounds the page cache of one SQLite
	// connection. A normal build keeps SQLite's own size.
	DefaultSQLitePageCacheBytes = buildprofile.SQLitePageCacheBytes
	// DefaultPasswordMemoryBytes and DefaultPasswordPasses are the argon2id
	// cost of one password hash. The printer build uses less memory and more
	// passes, which OWASP lists as equal in strength.
	DefaultPasswordMemoryBytes = buildprofile.PasswordMemoryKiB * 1024
	DefaultPasswordPasses      = buildprofile.PasswordPasses
	// DefaultPasswordHashesAtOnce is how many password hashes may run at the
	// same time. The printer build runs one.
	DefaultPasswordHashesAtOnce = buildprofile.PasswordHashesAtOnce

	// defaultOverlayDir and defaultOverlayFile are where the machine-owned
	// overlay lands when the operator names no path: beside their own file, in
	// a directory the API owns, so a write never touches the operator's file.
	defaultOverlayDir  = "config.d"
	defaultOverlayFile = "resources.json"

	// defaultGraphFile is the authorization graph's database, in the same
	// machine-owned directory as the overlay. Both are state this server writes
	// and the operator does not.
	defaultGraphFile = "authz.db"

	// defaultAccountsFile is the account file of an earlier build, in the same
	// machine-owned directory. The accounts are rows in the database, and the
	// server imports this file once when the database holds none.
	defaultAccountsFile = "accounts.json"

	// defaultModulesDir is where `module add` puts what it installs, in the
	// same machine-owned directory as the overlay. An installed module is
	// state this server writes and the operator does not, and the declaration
	// that names it is written beside it.
	defaultModulesDir = "modules"
)

// logLevels and logFormats are the closed sets Validate holds Log to. They are
// lists so a refusal can name the choices.
//
//nolint:gochecknoglobals // closed sets read at validation time, never written.
var (
	logLevels  = []string{"debug", "info", "warn", "error"}
	logFormats = []string{"text", "json"}
)

// TimeServer is the SNTP server this process can run on the local network.
type TimeServer struct {
	// Enabled is the value the web app's switch starts with. A person's choice
	// there is stored in the machine-owned file, where it wins over this key.
	Enabled bool `yaml:"enabled"`
}

// Core is the block the composition root reads. Every field has a working
// default, so a deployment with no core block starts.
type Core struct {
	// API is the listener the HTTP surface is served on.
	API Listener `yaml:"api"`
	// Log is where and how the process logs.
	Log Log `yaml:"log"`
	// Frontend is the web app served at the origin root, and how it looks
	// before anybody signs in. It is a core key because the page reads it
	// before its first paint, and because `make build` reads the same block to
	// decide whether the app is built at all.
	Frontend Frontend `yaml:"frontend"`
	// Discovery is the announcement this server sends on the local network, so
	// a client on the same network finds it without a typed address. It is a
	// core key because it describes this server's own socket, and `make build`
	// reads nothing from it.
	Discovery Discovery `yaml:"discovery"`
	// TimeServer is the SNTP server on UDP port 123, for devices on a network
	// with no internet.
	TimeServer TimeServer `yaml:"time_server"`
	// Dashboards is the home page as pages of cards. It is a core key because
	// the server reads it, and `make build` reads the same key to leave the
	// dashboards out of a binary that would never serve them.
	Dashboards Dashboards `yaml:"dashboards"`
	// Diagnostics is the support bundle. `make build` reads the same key, and
	// the minimal build leaves the bundle out.
	Diagnostics Diagnostics `yaml:"diagnostics"`
	// Observability is the Prometheus client, the exposition at /metrics, and
	// the listener that may serve it. `make build` reads the `enabled` key to
	// decide whether the client is linked, so an operator who asks for metrics
	// gets a binary that has them.
	Observability Observability `yaml:"observability"`
	// Overlay is the machine-owned file the resource API writes. Empty derives
	// it from the path of the operator's file.
	Overlay string `yaml:"overlay"`
	// Modules is the directory `module add` installs into and the declarations
	// it writes point at. Empty derives it from the operator's file, beside the
	// overlay, so an installed module sits next to the record of it.
	Modules string `yaml:"modules"`
	// ModulesLocal maps a module path to a built release directory on this
	// server, which the Extensions page offers as a local install. The path is
	// named here because a manifest never names its own. A relative directory
	// is read against the working directory, as Modules is.
	ModulesLocal map[string]string `yaml:"modules_local"`
	// ModulesMinimumStability is the lowest stability of a module release an
	// install takes, as Composer's minimum-stability: dev, alpha, beta, RC or
	// stable. Empty is stable.
	ModulesMinimumStability string `yaml:"modules_minimum_stability"`
	// ModulesPreferStable takes the most stable release in a constraint
	// before the newest one, as Composer's prefer-stable.
	ModulesPreferStable bool `yaml:"modules_prefer_stable"`
	// ModulesEnabled false switches off the modules the server installs: an
	// install is refused, and a module in the overlay never starts. A module
	// this file declares still runs, because the operator chose it. It is a
	// pointer so that an absent key is told apart from false, and absent is
	// on.
	ModulesEnabled *bool `yaml:"modules_enabled"`
	// SafeMode is this run started in safe mode: ModulesEnabled is false, and
	// the modules this file declares are held off too. The start sets it from
	// -safe or the one-shot marker, and no file can.
	SafeMode bool `yaml:"-"`
	// Marketplace is where the module catalogue is read from.
	Marketplace Marketplace `yaml:"marketplace"`
	// Principal is the subject every request is served as while no
	// authentication provider is wired. It is a configuration key because an
	// operator chooses to serve an unauthenticated API.
	Principal string `yaml:"principal"`
	// Auth is the identity provider human callers authenticate against. An
	// empty issuer is the deployment that has none. That wires a different
	// resolver; it does not turn a check off.
	Auth Auth `yaml:"auth"`
	// Authz is where the authorization graph lives. Empty derives it from the
	// operator's file, beside the overlay.
	Authz Authz `yaml:"authz"`
	// Persistence is the engine the whole database runs on. It is filled from
	// the top-level `persistence:` block and refused inside `core:`.
	Persistence Persistence `yaml:"-"`
	// TrustedProxies are the networks a reverse proxy reaches this server from,
	// written as CIDR blocks or as single addresses. A request that arrives from
	// one of them has its client address read out of X-Forwarded-For; a request
	// from anywhere else is counted under the address the socket reports.
	//
	// Empty is a server on its own address and is the safe default, because a
	// forwarding header from an undeclared peer is a value the caller chose.
	// Empty behind a proxy is a mistake too: every client is then counted
	// under the proxy's own address, so the login limit becomes one shared
	// limit for everybody.
	TrustedProxies []string `yaml:"trusted_proxies"`
	// Site is this node's token in every event source URN it publishes.
	Site string `yaml:"site"`
	// Events is how the event hub is served over HTTP.
	Events Events `yaml:"events"`
	// Connectors are the printer dialects this process mounts, keyed by the
	// instance name an operator chose. An absent block mounts the shipping
	// dialect; a block that names one connector mounts that one and no other.
	Connectors map[string]Connector `yaml:"connectors"`
	// DeviceStateBudget is how many bytes of one device's state tree this
	// process retains: every path and every value it holds. Over it, a new path
	// is refused and a path already held still takes a new reading, so a device
	// cannot lock itself out of the telemetry it is already reporting.
	//
	// It is a key because it scales with the fleet and with the machine the
	// fleet runs on. An operator raises it for a device that describes itself
	// in more paths than a printer does.
	DeviceStateBudget int `yaml:"device_state_budget"`
	// ModuleSettingsBytes is how many bytes one record a module's settings pane
	// writes may hold, at PUT /api/v1/modules/{namespace}/state/{key}. A larger
	// value is refused and the refusal names this number, so a pane can say
	// which setting it could not save.
	//
	// It is a key because a module the operator trusts may keep more than a
	// few choices. Raising it raises what a browser may write and not what a
	// module's own process may: sdk/stateapi.MaxValueBytes is the ceiling
	// over both, at 64 KiB.
	ModuleSettingsBytes int `yaml:"module_settings_bytes"`
	// PreviewCacheBytes is how many bytes of preview pictures the server keeps
	// in memory, across every print file it has read. Over it the oldest
	// pictures are dropped and read from the file again when asked for.
	PreviewCacheBytes int `yaml:"preview_cache_bytes"`
	// DeviceListingCacheBytes is how many bytes of printer directory listings
	// the server keeps in memory, across every printer. A repeat listing is
	// answered from it at once and marked stale. Over it the listing used
	// longest ago is dropped.
	DeviceListingCacheBytes int `yaml:"device_listing_cache_bytes"`
	// DeviceThumbnailDiskBytes is how many bytes of the pictures printers read
	// out of their own files are kept on disk, beside the overlay, so a
	// restart does not ask again. Over it the pictures written longest ago are
	// removed.
	DeviceThumbnailDiskBytes int `yaml:"device_thumbnail_disk_bytes"`
	// DeviceThumbnailDisk is whether those pictures are kept on disk at all.
	// Off writes nothing there.
	DeviceThumbnailDisk *bool `yaml:"device_thumbnail_disk"`
	// DeviceThumbnailPrefetch is whether the server asks a printer for the
	// pictures of a listed folder's print files before anybody opens them,
	// one at a time while the printer is idle.
	DeviceThumbnailPrefetch *bool `yaml:"device_thumbnail_prefetch"`
	// MemoryLimit is the soft memory limit handed to the Go runtime at start,
	// in bytes. Go collects garbage more often to stay under it. Zero leaves
	// the runtime with the limit it read from GOMEMLIMIT, or with none.
	MemoryLimit int64 `yaml:"memory_limit"`
	// ModulesMemory is the memory every enabled service module's budget must
	// fit in together, in bytes. Zero reads it from the machine: its memory
	// less a reserve for the server and the system.
	ModulesMemory int64 `yaml:"modules_memory"`
	// ModulesSwitchOff is when the host switches off a service module that
	// keeps failing, for every module whose operator set none of its own.
	ModulesSwitchOff install.SwitchOff `yaml:"modules_switch_off"`
	// ShutdownTimeout bounds draining the listeners, and then bounds waiting
	// for the supervision plane, so a resource that will not return cannot hold
	// the process open.
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	// Upgrade controls signed release discovery, snapshots and apply behavior.
	Upgrade Upgrade `yaml:"upgrade"`
}

type Upgrade struct {
	Enabled         *bool         `yaml:"enabled"`
	Server          string        `yaml:"server"`
	AllowUnsigned   bool          `yaml:"allow_unsigned"`
	TrustedKeys     []UpgradeKey  `yaml:"trusted_keys"`
	BackupRetention int           `yaml:"backup_retention"`
	PostgresBackup  string        `yaml:"postgres_backup"`
	RehearsalDSN    string        `yaml:"rehearsal_dsn"`
	TrialTimeout    time.Duration `yaml:"trial_timeout"`
	// ReleaseRepository is the legacy GitHub source. Naming one opts out of
	// Server's generic machine update API.
	ReleaseRepository string `yaml:"release_repository"`
}

type UpgradeKey struct {
	ID  string `yaml:"id"`
	Key string `yaml:"key"`
}

// Listener is one address to bind, and what it presents on it.
type Listener struct {
	// Addr is a host:port accepted by net.Listen.
	Addr string `yaml:"addr"`
	// TLS is the certificate this listener serves under. Empty is plain HTTP,
	// which a deployment with no certificate, the simulator, the emulator and
	// a local curl use.
	TLS TLS `yaml:"tls"`
	// AllowedOrigins are the browser origins a page may call this API from,
	// each an exact origin such as https://printers.example. The server adds
	// the list a person keeps in Settings and the origins of the extensions
	// that are switched on. While all three are empty, which is the default,
	// it sends no CORS header: only a page served from this server's own origin
	// can call the API from a browser.
	//
	// A wildcard is refused. The API takes a credential, and a browser will
	// not send one to an origin that answers every page with *, so the key
	// would do nothing and no message would say so.
	AllowedOrigins []string `yaml:"allowed_origins"`
	// RequestTimeout is how long one request may take. Over it the server stops
	// waiting for its own handler and answers 504.
	//
	// It is a key because a request waits on a printer, a database or a
	// module, and how slow those are depends on the deployment.
	RequestTimeout time.Duration `yaml:"request_timeout"`
	// StreamTimeout is the same bound for the routes whose answer is a stream
	// or a file: the event stream, the camera stream, the two printer transfers
	// and the storage content routes. Each of those keeps a window per chunk,
	// so this is the ceiling over the whole response. The window notices a
	// peer that stopped.
	//
	// It must be at least RequestTimeout.
	StreamTimeout time.Duration `yaml:"stream_timeout"`
}

// Auth is the OpenID Connect issuer a human login runs against. The flow is an
// authorization code with PKCE. ClientSecret is optional, for a confidential
// client.
type Auth struct {
	// Issuer is the issuer identifier, which the discovery document must agree
	// with. Empty means this deployment has no login provider.
	Issuer string `yaml:"issuer"`
	// ClientID is this server's registration at the issuer.
	ClientID string `yaml:"client_id"`
	// ClientSecret is the secret a confidential client authenticates the code
	// exchange with. Empty is a public client, which PKCE alone protects.
	ClientSecret string `yaml:"client_secret"`
	// RedirectURI is where the issuer sends the browser back, compared exactly
	// by the issuer and sent again at the exchange. Its scheme also decides
	// whether the session cookie is marked Secure, because a browser never
	// returns a Secure cookie to an origin it reaches over http.
	RedirectURI string `yaml:"redirect_uri"`
	// ProviderName is what the sign-in button calls the issuer, as in "Sign
	// in with <name>". Empty takes the issuer's host name.
	ProviderName string `yaml:"provider_name"`
	// Local serves a password login against this server's own account store, so
	// a self-hoster with no issuer can authenticate. The first run creates the
	// administrator; see docs/src/content/docs/local-accounts.md.
	Local bool `yaml:"local"`
	// Accounts is the account file an earlier build wrote. The accounts are
	// rows in the database, and the server imports this file once when the
	// database holds no account. Empty derives it beside the overlay.
	Accounts string `yaml:"accounts"`
	// PasswordMemoryBytes is the memory block one argon2id password hash
	// fills, in bytes. It is a whole number of KiB, because argon2id counts
	// its memory in KiB.
	PasswordMemoryBytes int `yaml:"password_memory_bytes"`
	// PasswordPasses is how many times one password hash goes over that
	// block. Less memory needs more passes for the same strength.
	PasswordPasses int `yaml:"password_passes"`
	// PasswordHashesAtOnce is how many password hashes may run at the same
	// time. A login past it waits within its own request deadline.
	PasswordHashesAtOnce int `yaml:"password_hashes_at_once"`
	// SetupCode is the code the first run asks for, for an install that is
	// set up by a script. Empty makes the server print a new random code at
	// every start while the first run is open.
	SetupCode string `yaml:"setup_code"`
}

// Authz is the authorization graph's storage. There is no key that turns the
// graph off: a deployment that cannot open it refuses to start, because a
// server without one enforces nothing.
type Authz struct {
	// Database is the SQLite file the tuples and their projection live in. It
	// is created empty on first run and never fetched, so an air-gapped
	// install needs no network.
	Database string `yaml:"database"`
	// OrgAdminHandover lets an administrator hand an org over through the
	// API, without editing core.principal and restarting.
	//
	// Off is the default. The admin role on an org is then not writable through
	// the grant surface, so a deployment whose only administrator locks
	// themselves out recovers at the configuration file. On, that grant is
	// writable and an object's org edge is not, because a caller could
	// otherwise escalate through that edge.
	OrgAdminHandover bool `yaml:"org_admin_handover"`
}

// bytesPerKiB converts the password memory key, which is in bytes, to the KiB
// argon2id counts in.
const bytesPerKiB = 1024

// PasswordCost is the argon2id cost the two password keys name. A key left at
// zero means the default of this build, as WithDefaults fills it. Call it
// after Validate, which refuses the values that do not fit.
func (a Auth) PasswordCost() auth.Cost {
	a = a.withDefaults(Default().Auth)

	if a.PasswordMemoryBytes <= 0 || a.PasswordMemoryBytes/bytesPerKiB > int(auth.MaxCostMemoryKiB) ||
		a.PasswordPasses <= 0 || a.PasswordPasses > int(auth.MaxCostPasses) {
		return auth.Cost{}
	}

	return auth.Cost{
		MemoryKiB: uint32(a.PasswordMemoryBytes / bytesPerKiB), //nolint:gosec // bounded to MaxCostMemoryKiB above.
		Passes:    uint32(a.PasswordPasses),
	}
}

// validatePasswordCost refuses a password cost below every argon2id setting
// OWASP lists, and a value that is not a size or a count.
func (a Auth) validatePasswordCost() error {
	if a.PasswordMemoryBytes <= 0 || a.PasswordMemoryBytes%bytesPerKiB != 0 {
		return fmt.Errorf("%w: core.auth.password_memory_bytes is %d, expected a byte count that is a "+
			"whole number of KiB, such as %d", ErrInvalid, a.PasswordMemoryBytes, DefaultPasswordMemoryBytes)
	}

	if a.PasswordPasses <= 0 {
		return fmt.Errorf("%w: core.auth.password_passes is %d, expected a positive count",
			ErrInvalid, a.PasswordPasses)
	}

	if a.PasswordHashesAtOnce <= 0 {
		return fmt.Errorf("%w: core.auth.password_hashes_at_once is %d, expected a positive count",
			ErrInvalid, a.PasswordHashesAtOnce)
	}

	if a.PasswordMemoryBytes/bytesPerKiB > int(auth.MaxCostMemoryKiB) {
		return fmt.Errorf("%w: core.auth.password_memory_bytes is %d, more than the upper limit of %d",
			ErrInvalid, a.PasswordMemoryBytes, int(auth.MaxCostMemoryKiB)*bytesPerKiB)
	}

	if a.PasswordPasses > int(auth.MaxCostPasses) {
		return fmt.Errorf("%w: core.auth.password_passes is %d, more than the upper limit of %d",
			ErrInvalid, a.PasswordPasses, auth.MaxCostPasses)
	}

	if err := a.PasswordCost().Check(); err != nil {
		return fmt.Errorf("%w: core.auth.password_memory_bytes is %d and core.auth.password_passes is %d. %w",
			ErrInvalid, a.PasswordMemoryBytes, a.PasswordPasses, err)
	}

	return nil
}

// The engines the database runs on. Both are first class, and every query and
// every migration runs against both.
const (
	EngineSQLite   = "sqlite"
	EnginePostgres = "postgres"
)

// Persistence is the database, and there is one of it. The authorization graph,
// the device records and the command queue share the engine named here, so they
// share one connection pool and one migration ledger. A deployment cannot put
// the graph on one engine and the records on another, because queries join
// the two.
//
// It is read from the top-level `persistence:` block, because
// `cmd/detect-build-tags` reads the same key to decide whether the Postgres
// driver is linked in. One key means a build cannot be derived from a value
// the runtime ignores.
type Persistence struct {
	// Type is the engine, sqlite or postgres. Empty is sqlite, which a
	// deployment with no configuration file runs on.
	Type string `yaml:"type"`
	// MaxOpenConns is the most database connections open at once, across every
	// store this process opens. Raise it for a server answering more requests
	// at a time than it has connections for; lower it for a database shared
	// with something else.
	MaxOpenConns int `yaml:"max_open_conns"`
	// MaxIdleConns is how many connections stay open while nothing is asking.
	// Below MaxOpenConns a burst pays to dial; above it the extra is ignored.
	MaxIdleConns int `yaml:"max_idle_conns"`
	// ConnMaxLifetime is how long one connection is used before it is replaced.
	// A failover or a rotated credential then reaches a long-running server
	// without a restart.
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	// SQLite carries what only that engine needs.
	SQLite SQLiteEngine `yaml:"sqlite"`
	// Postgres carries what only that engine needs.
	Postgres PostgresEngine `yaml:"postgres"`
}

// SQLiteEngine is how the core's SQLite database is opened.
type SQLiteEngine struct {
	// PageCacheBytes bounds the page cache of one connection. SQLite keeps the
	// pages it read last in that cache, so each open connection may hold up to
	// this much memory.
	PageCacheBytes int `yaml:"page_cache_bytes"`
}

// PostgresEngine is the Postgres connection.
type PostgresEngine struct {
	// DSN is handed to the driver unread, so both forms it takes are accepted:
	// a postgres:// URL and a key=value string.
	DSN string `yaml:"dsn"`
	// ModuleAdminDSN is the connection the server makes a module's own
	// database and role with, when an operator asks it to. Empty uses DSN. It
	// is a postgres:// URL, because the module's own connection string is
	// built from it.
	ModuleAdminDSN string `yaml:"module_admin_dsn"`
	// StatementTimeout bounds one statement. It reaches the server as the
	// connection setting of the same name, added to the DSN, so every
	// connection the pool opens carries it. A DSN that already names
	// statement_timeout keeps what it says and this key is not applied.
	StatementTimeout time.Duration `yaml:"statement_timeout"`
}

// validate holds the selector to the engines this binary knows. "memory" is
// refused: an authorization graph in memory forgets every grant on restart.
func (p Persistence) validate() error {
	if err := p.validatePool(); err != nil {
		return err
	}

	switch p.Type {
	case EngineSQLite:
		if p.Postgres.ModuleAdminDSN != "" {
			return fmt.Errorf("%w: persistence.postgres.module_admin_dsn is set and persistence.type is %q. "+
				"This server makes a module's database only on its own Postgres server. Remove the key",
				ErrInvalid, EngineSQLite)
		}

		return nil
	case EnginePostgres:
		if p.Postgres.DSN == "" {
			return fmt.Errorf("%w: persistence.postgres.dsn is empty and persistence.type is %q",
				ErrInvalid, EnginePostgres)
		}

		return p.Postgres.validateModuleAdmin()
	default:
		return fmt.Errorf("%w: persistence.type is %q, expected %q or %q",
			ErrInvalid, p.Type, EngineSQLite, EnginePostgres)
	}
}

// validatePool refuses a negative bound. database/sql reads a negative as "no
// limit", so an operator who typed one to mean "smaller" would get a pool with
// no bound and no message.
func (p Persistence) validatePool() error {
	bounds := []struct {
		key   string
		value int64
	}{
		{"persistence.max_open_conns", int64(p.MaxOpenConns)},
		{"persistence.max_idle_conns", int64(p.MaxIdleConns)},
		{"persistence.conn_max_lifetime", int64(p.ConnMaxLifetime)},
		{"persistence.postgres.statement_timeout", int64(p.Postgres.StatementTimeout)},
		{"persistence.sqlite.page_cache_bytes", int64(p.SQLite.PageCacheBytes)},
	}

	for _, bound := range bounds {
		if bound.value < 0 {
			return fmt.Errorf("%w: %s is negative, which is no bound at all", ErrInvalid, bound.key)
		}
	}

	if p.Type == EnginePostgres && p.SQLite.PageCacheBytes != 0 {
		return fmt.Errorf("%w: persistence.sqlite.page_cache_bytes is set and persistence.type is %q, "+
			"which has no page cache to bound. Remove the key", ErrInvalid, EnginePostgres)
	}

	// SQLite sizes the cache in KiB. A smaller value is almost always a size
	// written in KiB where the key asks for bytes.
	if cache := p.SQLite.PageCacheBytes; cache > 0 && cache < minPageCacheBytes {
		return fmt.Errorf("%w: persistence.sqlite.page_cache_bytes is %d, expected a byte count of at least %d",
			ErrInvalid, cache, minPageCacheBytes)
	}

	return nil
}

// minPageCacheBytes is the smallest page cache persistence.sqlite takes.
const minPageCacheBytes = 1 << 10

// Enabled reports whether an issuer is configured.
func (a Auth) Enabled() bool { return a.Issuer != "" }

// Anonymous reports whether this deployment authenticates nobody, which is the
// wiring that serves every request as core.principal.
func (a Auth) Anonymous() bool { return !a.Enabled() && !a.Local }

// Secure reports whether the browser reaches this server over https, read from
// the redirect URI the issuer is told to send it back to.
func (a Auth) Secure() bool {
	parsed, err := url.Parse(a.RedirectURI)

	return err == nil && parsed.Scheme == schemeHTTPS
}

// Events is what the HTTP event stream needs that an operator can change. The
// coalescing window and the queue bounds are the hub's and are not keys.
type Events struct {
	// Heartbeat is how often an idle stream writes a comment frame. It is a
	// key because the proxy or load balancer that closes an idle connection
	// differs between deployments.
	Heartbeat time.Duration `yaml:"heartbeat"`
	// Retry is the reconnection delay a reader is handed. It is a key for the
	// same reason: the right delay depends on the network between the two.
	Retry time.Duration `yaml:"retry"`
	// MaxStreamsPerPrincipal bounds how many streams one principal holds at
	// once. A deployment with no issuer configured serves every request as
	// core.principal, so in that mode this bounds the whole deployment.
	MaxStreamsPerPrincipal int `yaml:"max_streams_per_principal"`
}

// Log is the handler the process installs on stderr.
type Log struct {
	// Level is one of debug, info, warn or error.
	Level string `yaml:"level"`
	// Format is text or json.
	Format string `yaml:"format"`
}

// Default returns the configuration a process with no core block runs on.
func Default() Core {
	return Core{
		API: Listener{
			Addr:           DefaultAPIAddr,
			RequestTimeout: DefaultRequestTimeout,
			StreamTimeout:  DefaultStreamTimeout,
		},
		Log: Log{Level: DefaultLogLevel, Format: DefaultLogFormat},
		Frontend: Frontend{
			Enabled:         new(true),
			Title:           DefaultTitle,
			DefaultLanguage: DefaultLanguage,
			Theme:           DefaultTheme,
		},
		Diagnostics: Diagnostics{SendURL: new(DefaultDiagnosticsSendURL)},
		Auth: Auth{
			PasswordMemoryBytes:  DefaultPasswordMemoryBytes,
			PasswordPasses:       DefaultPasswordPasses,
			PasswordHashesAtOnce: DefaultPasswordHashesAtOnce,
		},
		Discovery: Discovery{
			Enabled:     new(true),
			Services:    []string{ServicePrusaLink, ServicePrusaConnect},
			HostRecords: HostRecordsOwn,
		},
		Marketplace: Marketplace{
			Enabled: new(true),
			URL:     DefaultMarketplaceURL,
			Timeout: DefaultMarketplaceTimeout,
		},
		Principal: DefaultPrincipal,
		Site:      DefaultSite,
		Persistence: Persistence{
			Type:            EngineSQLite,
			MaxOpenConns:    DefaultMaxOpenConns,
			MaxIdleConns:    DefaultMaxIdleConns,
			ConnMaxLifetime: DefaultConnMaxLifetime,
			Postgres:        PostgresEngine{StatementTimeout: DefaultStatementTimeout},
		},
		Events: Events{
			Heartbeat:              DefaultHeartbeat,
			Retry:                  DefaultRetry,
			MaxStreamsPerPrincipal: DefaultMaxStreamsPerPrincipal,
		},
		DeviceStateBudget:        DefaultDeviceStateBudget,
		ModuleSettingsBytes:      DefaultModuleSettingsBytes,
		PreviewCacheBytes:        DefaultPreviewCacheBytes,
		DeviceListingCacheBytes:  DefaultDeviceListingCacheBytes,
		DeviceThumbnailDiskBytes: DefaultDeviceThumbnailDiskBytes,
		DeviceThumbnailDisk:      new(DefaultDeviceThumbnailDisk),
		DeviceThumbnailPrefetch:  new(DefaultDeviceThumbnailPrefetch),
		ShutdownTimeout:          DefaultShutdownTimeout,
		ModulesSwitchOff: install.SwitchOff{
			Failures: new(install.DefaultSwitchOffFailures),
			Window:   install.DefaultSwitchOffWindow,
		},
		Upgrade: Upgrade{
			Enabled:         new(true),
			Server:          DefaultUpgradeServer,
			BackupRetention: DefaultBackupRetention,
			PostgresBackup:  DefaultPostgresBackup,
			TrialTimeout:    DefaultUpgradeTrialTimeout,
		},
	}
}

// DefaultOverlay is where the overlay goes for a given operator file: a
// directory beside it that the API owns, so a write never rewrites the
// operator's own file.
func DefaultOverlay(file string) string {
	return filepath.Join(filepath.Dir(file), defaultOverlayDir, defaultOverlayFile)
}

// DefaultModules is where installed modules go for a given operator file:
// beside the overlay, in the same directory this server owns.
func DefaultModules(file string) string {
	return filepath.Join(filepath.Dir(file), defaultOverlayDir, defaultModulesDir)
}

// DefaultGraph is where the authorization graph goes for a given operator file:
// beside the overlay, in the same directory this server owns.
func DefaultGraph(file string) string {
	return filepath.Join(filepath.Dir(file), defaultOverlayDir, defaultGraphFile)
}

// DefaultAccounts is where the local accounts go for a given operator file,
// beside the graph. It holds password hashes, so it goes in the directory this
// server owns.
func DefaultAccounts(file string) string {
	return filepath.Join(filepath.Dir(file), defaultOverlayDir, defaultAccountsFile)
}

// SecureCookies reports whether a browser reaches this server over https. The
// session cookie is marked Secure when it does.
//
// The two identity providers read different keys. A redirect login has the
// origin in the redirect URI the issuer sends the browser back to. A password
// login has no such key, so the API listener's own certificate decides.
func (c Core) SecureCookies() bool {
	if c.Auth.Enabled() {
		return c.Auth.Secure()
	}

	return c.API.TLS.Enabled()
}

// Validate reports the first key the core will not start on. It runs before
// anything is constructed, so a rejected configuration binds no port, writes no
// file and starts no goroutine.
func (c Core) Validate() error {
	if err := c.API.validate(); err != nil {
		return err
	}

	if !slices.Contains(logLevels, c.Log.Level) {
		return fmt.Errorf("%w: core.log.level is %q, expected one of %v", ErrInvalid, c.Log.Level, logLevels)
	}

	if !slices.Contains(logFormats, c.Log.Format) {
		return fmt.Errorf("%w: core.log.format is %q, expected one of %v", ErrInvalid, c.Log.Format, logFormats)
	}

	if err := c.Frontend.validate(); err != nil {
		return err
	}

	if err := c.Observability.validate(); err != nil {
		return err
	}

	if err := c.Diagnostics.validate(); err != nil {
		return err
	}

	if err := c.Discovery.validate(); err != nil {
		return err
	}

	if c.Principal == "" {
		return fmt.Errorf("%w: core.principal is empty", ErrInvalid)
	}

	if c.Site == "" {
		return fmt.Errorf("%w: core.site is empty", ErrInvalid)
	}

	if err := ref.ValidateID(c.Site); err != nil {
		return fmt.Errorf("%w: core.site is %q: %w", ErrInvalid, c.Site, err)
	}

	if err := c.Auth.validate(); err != nil {
		return err
	}

	if err := c.validateConnectors(); err != nil {
		return err
	}

	if err := c.Persistence.validate(); err != nil {
		return err
	}

	if err := c.validateBounds(); err != nil {
		return err
	}

	if err := c.validateRecords(); err != nil {
		return err
	}

	if err := c.validateMemoryLimit(); err != nil {
		return err
	}

	if err := c.Upgrade.validate(); err != nil {
		return err
	}

	if err := c.validateModules(); err != nil {
		return err
	}

	if c.Authz.Database == "" {
		return fmt.Errorf("%w: core.authz.database is empty", ErrInvalid)
	}

	if _, err := c.TrustedPrefixes(); err != nil {
		return err
	}

	return nil
}

// On reports whether this server prepares upgrades, keeps backups and pairs
// with the Beta Portal. An absent key is on.
func (u Upgrade) On() bool { return u.Enabled == nil || *u.Enabled }

func (u Upgrade) validate() error {
	if u.Enabled == nil {
		return fmt.Errorf("%w: core.upgrade.enabled has no value", ErrInvalid)
	}
	if err := ValidRemoteURL(u.Server); err != nil {
		return fmt.Errorf("%w: core.upgrade.server is %q, %w. Expected an address such as "+
			"https://beta.example", ErrInvalid, u.Server, err)
	}
	if u.BackupRetention <= 0 {
		return fmt.Errorf("%w: core.upgrade.backup_retention is %d, expected a positive count",
			ErrInvalid, u.BackupRetention)
	}
	if !slices.Contains([]string{"pg_dump", "external", "none"}, u.PostgresBackup) {
		return fmt.Errorf("%w: core.upgrade.postgres_backup is %q, expected pg_dump, external or none",
			ErrInvalid, u.PostgresBackup)
	}
	if u.TrialTimeout <= 0 {
		return fmt.Errorf("%w: core.upgrade.trial_timeout is %s, expected a positive duration",
			ErrInvalid, u.TrialTimeout)
	}

	ids := map[string]bool{}
	for _, key := range u.TrustedKeys {
		if strings.TrimSpace(key.ID) == "" || strings.TrimSpace(key.Key) == "" {
			return fmt.Errorf("%w: core.upgrade.trusted_keys needs a non-empty id and key", ErrInvalid)
		}
		if ids[key.ID] {
			return fmt.Errorf("%w: core.upgrade.trusted_keys repeats id %q", ErrInvalid, key.ID)
		}
		ids[key.ID] = true
	}

	return nil
}

// validateBounds refuses a timeout, a delay or a count that is zero or
// negative. Each is a bound, and such a value would mean no bound.
func (c Core) validateBounds() error {
	durations := []struct {
		key   string
		value time.Duration
	}{
		{"core.shutdown_timeout", c.ShutdownTimeout},
		{"core.events.heartbeat", c.Events.Heartbeat},
		{"core.events.retry", c.Events.Retry},
	}

	for _, bound := range durations {
		if bound.value <= 0 {
			return fmt.Errorf("%w: %s is %s, expected a positive duration", ErrInvalid, bound.key, bound.value)
		}
	}

	counts := []struct {
		key   string
		value int
	}{
		{"core.events.max_streams_per_principal", c.Events.MaxStreamsPerPrincipal},
		{"core.device_state_budget", c.DeviceStateBudget},
		{"core.preview_cache_bytes", c.PreviewCacheBytes},
		{"core.device_listing_cache_bytes", c.DeviceListingCacheBytes},
		{"core.device_thumbnail_disk_bytes", c.DeviceThumbnailDiskBytes},
	}

	for _, bound := range counts {
		if bound.value <= 0 {
			return fmt.Errorf("%w: %s is %d, expected a positive count", ErrInvalid, bound.key, bound.value)
		}
	}

	return nil
}

// TrustedPrefixes is core.trusted_proxies as networks.
//
// A single address is read as its own /32 or /128, so an operator who names one
// proxy does not have to write the mask. A value that is neither is refused.
// If the server did not know its proxy, every client would be counted under
// the proxy's address.
func (c Core) TrustedPrefixes() ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(c.TrustedProxies))

	for _, declared := range c.TrustedProxies {
		if network, err := netip.ParsePrefix(declared); err == nil {
			out = append(out, network.Masked())

			continue
		}

		address, err := netip.ParseAddr(declared)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: core.trusted_proxies has %q, expected a CIDR block such as 10.0.0.0/8 or a single address",
				ErrInvalid, declared)
		}

		out = append(out, netip.PrefixFrom(address, address.BitLen()))
	}

	return out, nil
}

// validate reports the first key the API listener will not start on.
func (l Listener) validate() error {
	if err := validAddr("core.api.addr", l.Addr); err != nil {
		return err
	}

	if err := l.TLS.validate("core.api.tls"); err != nil {
		return err
	}

	if err := l.validateBudgets(); err != nil {
		return err
	}

	return validOrigins("core.api.allowed_origins", l.AllowedOrigins)
}

// validateBudgets refuses a request budget that is zero or negative, and a
// stream budget under the request budget.
//
// The stream budget is refused and not raised: the two keys disagree, and the
// operator has to say which one they meant.
func (l Listener) validateBudgets() error {
	for _, bound := range []struct {
		key   string
		value time.Duration
	}{
		{"core.api.request_timeout", l.RequestTimeout},
		{"core.api.stream_timeout", l.StreamTimeout},
	} {
		if bound.value <= 0 {
			return fmt.Errorf("%w: %s is %s, expected a positive duration", ErrInvalid, bound.key, bound.value)
		}
	}

	if l.StreamTimeout < l.RequestTimeout {
		return fmt.Errorf("%w: core.api.stream_timeout is %s and core.api.request_timeout is %s, so the "+
			"longer budget is the shorter one", ErrInvalid, l.StreamTimeout, l.RequestTimeout)
	}

	return nil
}

// validate reports the first key a configured issuer will not start on. It
// checks what can be checked before a socket is opened. Discovery finds an
// unreachable issuer later.
func (a Auth) validate() error {
	if err := a.validatePasswordCost(); err != nil {
		return err
	}

	// One identity provider answers a login. Two would leave a person with two
	// accounts on one server and no rule for which grants belong to which.
	if a.Enabled() && a.Local {
		return fmt.Errorf("%w: core.auth.issuer and core.auth.local are both set, and one deployment "+
			"serves one identity provider", ErrInvalid)
	}

	if err := validSetupCode(a.SetupCode); err != nil {
		return err
	}

	if a.Accounts != "" && !a.Local {
		return fmt.Errorf("%w: core.auth.accounts names %q and core.auth.local is not set, so nothing "+
			"reads it", ErrInvalid, a.Accounts)
	}

	if !a.Enabled() {
		if a.ClientID != "" || a.ClientSecret != "" || a.RedirectURI != "" {
			return fmt.Errorf("%w: core.auth.issuer is empty but core.auth carries a client id, "+
				"client secret or redirect uri, so no login can be started", ErrInvalid)
		}

		return nil
	}

	if a.ClientID == "" {
		return fmt.Errorf("%w: core.auth.client_id is empty", ErrInvalid)
	}

	if a.RedirectURI == "" {
		return fmt.Errorf("%w: core.auth.redirect_uri is empty", ErrInvalid)
	}

	parsed, err := url.Parse(a.RedirectURI)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return fmt.Errorf("%w: core.auth.redirect_uri is %q, expected an absolute url the issuer can "+
			"send a browser back to", ErrInvalid, a.RedirectURI)
	}

	if parsed.Scheme != schemeHTTP && parsed.Scheme != schemeHTTPS {
		return fmt.Errorf("%w: core.auth.redirect_uri is %q, expected an http or https url",
			ErrInvalid, a.RedirectURI)
	}

	return nil
}

// validAddr holds an address to what net.Listen will take, so a bad one is
// found before any port is bound.
func validAddr(key, addr string) error {
	if addr == "" {
		return fmt.Errorf("%w: %s is empty", ErrInvalid, key)
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%w: %s is %q, expected host:port", ErrInvalid, key, addr)
	}

	if port == "" {
		return fmt.Errorf("%w: %s is %q, expected a port", ErrInvalid, key, addr)
	}

	// A numeric port only. A named one resolves through /etc/services, so
	// whether the process starts would depend on that file.
	if _, err = strconv.Atoi(port); err != nil {
		return fmt.Errorf("%w: %s is %q, %q is not a port number", ErrInvalid, key, addr, port)
	}

	return nil
}

// validOrigins holds an allow-list to what a browser will send. An origin is
// compared byte for byte against the Origin header, and a browser writes that
// header in one form: a lowercase scheme and host, a port only when it is not
// the scheme's default, and nothing after the host. An entry in any other form
// would never match, so it is refused.
func validOrigins(key string, origins []string) error {
	for _, origin := range origins {
		if origin == "*" {
			return fmt.Errorf("%w: %s has \"*\", and a wildcard is refused: the API takes a credential "+
				"and a browser sends none to an origin that answers every page. Name each origin",
				ErrInvalid, key)
		}

		if err := CheckOrigin(origin); err != nil {
			return fmt.Errorf("%w: %s has %q, %w. Expected an exact origin such as https://printers.example",
				ErrInvalid, key, origin, err)
		}
	}

	return nil
}

// The two schemes a browser origin can carry.
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// The reasons CheckOrigin refuses a value, each phrased to follow the value it
// names.
var (
	errOriginScheme      = errors.New("its scheme is not http or https")
	errOriginHost        = errors.New("it names no host")
	errOriginCase        = errors.New("a browser writes the scheme and the host in lowercase")
	errOriginPath        = errors.New("it carries more than a scheme and a host")
	errOriginDefaultPort = errors.New("a browser writes its origin without the scheme's default port")
	errOriginWildcard    = errors.New("a wildcard is refused, so name each host")
	errOriginForm        = errors.New("a browser writes the host in ASCII, an IP address in its shortest form, " +
		"and a port as a number from 1 to 65535")
)

// CheckOrigin refuses a value a browser never sends as its Origin header, and
// says why in words that follow the value. The settings list is held to it as
// core.api.allowed_origins is.
func CheckOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != schemeHTTP && parsed.Scheme != schemeHTTPS) {
		return errOriginScheme
	}

	if parsed.Hostname() == "" {
		return errOriginHost
	}

	if strings.ToLower(origin) != origin {
		return errOriginCase
	}

	if strings.Contains(parsed.Host, "*") {
		return errOriginWildcard
	}

	// The serialized form is only the scheme and the host, so a value with a
	// path, a query, a fragment or a userinfo differs from it.
	if parsed.Scheme+"://"+parsed.Host != origin {
		return errOriginPath
	}

	port := parsed.Port()
	if (parsed.Scheme == schemeHTTP && port == "80") || (parsed.Scheme == schemeHTTPS && port == "443") {
		return errOriginDefaultPort
	}

	if !browserHost(parsed) {
		return errOriginForm
	}

	return nil
}

// browserHost reports whether the host and the port are in the one form a
// browser writes them in its Origin header: an ASCII host (a browser sends a
// name in punycode), an IP address in its shortest form, and a port as a plain
// number from 1 to 65535. sdk/manifest holds a manifest's origins to the same
// form in IsAllowedOrigin.
func browserHost(parsed *url.URL) bool {
	if strings.HasSuffix(parsed.Host, ":") {
		return false
	}

	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
			return false
		}
	}

	host := parsed.Hostname()
	for i := range len(host) {
		if host[i] >= utf8.RuneSelf {
			return false
		}
	}

	// A browser reads a host whose last label is a number as an IPv4 address,
	// so 0x7f.1 reaches it as 127.0.0.1.
	last := host[strings.LastIndexByte(host, '.')+1:]
	if strings.HasPrefix(parsed.Host, "[") || strings.HasPrefix(last, "0x") || isDigits(last) {
		addr, err := netip.ParseAddr(host)

		return err == nil && addr.Zone() == "" && addr.String() == host
	}

	return true
}

// isDigits reports whether text is one or more ASCII digits.
func isDigits(text string) bool {
	return text != "" && strings.Trim(text, "0123456789") == ""
}

// validateRecords holds the two keys about what the core writes for a module:
// the overlay it records declarations in, and the byte count one settings
// record may hold, between one and the ceiling the SDK puts over every value.
func (c Core) validateRecords() error {
	if c.ModuleSettingsBytes <= 0 || c.ModuleSettingsBytes > stateapi.MaxValueBytes {
		return fmt.Errorf("%w: core.module_settings_bytes is %d, expected a byte count from 1 to %d",
			ErrInvalid, c.ModuleSettingsBytes, stateapi.MaxValueBytes)
	}

	if c.Overlay == "" {
		return fmt.Errorf("%w: core.overlay is empty", ErrInvalid)
	}

	return nil
}

// MinModulesMemory is the smallest core.modules_memory the core takes: less
// than this holds no module at the default budget of 128 MiB, and a smaller
// value is almost always a size written in MiB where the key asks for bytes.
const MinModulesMemory = 64 << 20

// validateModulesMemory refuses a pool too small to be meant, and a
// switch-off out of bounds. Zero reads the pool from the machine.
func (c Core) validateModulesMemory() error {
	if c.ModulesMemory != 0 && c.ModulesMemory < MinModulesMemory {
		return fmt.Errorf("%w: core.modules_memory is %d, expected 0 or a byte count of at least %d, "+
			"such as 536870912 for 512 MiB", ErrInvalid, c.ModulesMemory, MinModulesMemory)
	}

	if err := install.CheckSwitchOff("core.modules_switch_off", c.ModulesSwitchOff); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	return nil
}

// MinMemoryLimit is the smallest core.memory_limit the core takes. The Go
// runtime alone needs more than this, so a smaller value is almost always a
// size written in MiB where the key asks for bytes.
const MinMemoryLimit = 1 << 20

// validateMemoryLimit refuses a limit that is not a byte count the server can
// run under. Zero is no limit from the key.
func (c Core) validateMemoryLimit() error {
	if c.MemoryLimit == 0 {
		return nil
	}

	if c.MemoryLimit < MinMemoryLimit {
		return fmt.Errorf("%w: core.memory_limit is %d, expected 0 or a byte count of at least %d, "+
			"such as 12582912 for 12 MiB", ErrInvalid, c.MemoryLimit, MinMemoryLimit)
	}

	return nil
}
