package config_test

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/app/config/install"
)

// TestAFileWithNoCoreBlockLoadsTheDefaults is the first-run case: the operator's
// file declares a resource and says nothing about the core.
func TestAFileWithNoCoreBlockLoadsTheDefaults(t *testing.T) {
	t.Parallel()

	path := write(t, "resources:\n  storage:\n    local:\n      type: filesystem\n")

	core, err := config.Load(path)
	require.NoError(t, err)

	require.Equal(t, config.DefaultAPIAddr, core.API.Addr)
	require.Equal(t, config.DefaultLogLevel, core.Log.Level)
	require.Equal(t, config.DefaultShutdownTimeout, core.ShutdownTimeout)
	require.Equal(t, install.SwitchOff{Failures: new(3), Window: time.Hour}, core.ModulesSwitchOff)
	require.Equal(t, config.DefaultUpgradeServer, core.Upgrade.Server)
	require.Equal(t, config.DefaultOverlay(path), core.Overlay)
	require.Equal(t, config.DefaultGraph(path), core.Authz.Database)
	require.NoError(t, core.Validate())
}

// A failure count of 0 is never, not absent, so the default does not replace
// it. The window the file leaves out still takes its default.
func TestASwitchOffOfZeroFailuresIsKept(t *testing.T) {
	t.Parallel()

	path := write(t, "core:\n  modules_switch_off:\n    failures: 0\n")

	core, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, install.SwitchOff{Failures: new(0), Window: time.Hour}, core.ModulesSwitchOff)
	require.NoError(t, core.Validate())
}

func TestUpgradeServerIsConfigurableAndValidated(t *testing.T) {
	t.Parallel()

	path := write(t, "core:\n  upgrade:\n    server: http://127.0.0.1:8123/testing\n")
	core, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8123/testing", core.Upgrade.Server)

	path = write(t, "core:\n  upgrade:\n    server: http://updates.example\n")
	core, err = config.Load(path)
	require.NoError(t, err)
	err = core.Validate()
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), "core.upgrade.server")
}

// TestAMissingFileIsADefaultConfiguration keeps a first run startable. A server
// that refuses to run without a file cannot be started at all.
func TestAMissingFileIsADefaultConfiguration(t *testing.T) {
	t.Parallel()

	core, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)
	require.Equal(t, config.DefaultAPIAddr, core.API.Addr)
	require.NoError(t, core.Validate())
}

// TestEveryCoreKeyIsRead proves the block an operator writes is the block that
// takes effect, including the duration, which yaml resolves as a string.
func TestEveryCoreKeyIsRead(t *testing.T) {
	t.Parallel()

	path := write(t, `
core:
  api:
    addr: "127.0.0.1:9000"
    allowed_origins:
      - https://printers.example
      - http://localhost:5173
      - http://127.0.0.1:8080
      - http://[::1]:5173
  log:
    level: debug
    format: json
  overlay: /var/lib/prusa/resources.json
  principal: someone
  authz:
    database: /var/lib/prusa/authz.db
  trusted_proxies:
    - 10.0.0.0/8
    - 192.0.2.9
  shutdown_timeout: 3s
`)

	core, err := config.Load(path)
	require.NoError(t, err)

	require.Equal(t, "127.0.0.1:9000", core.API.Addr)
	require.Equal(t, []string{
		"https://printers.example", "http://localhost:5173", "http://127.0.0.1:8080", "http://[::1]:5173",
	}, core.API.AllowedOrigins)
	require.Equal(t, "debug", core.Log.Level)
	require.Equal(t, "json", core.Log.Format)
	require.Equal(t, "/var/lib/prusa/resources.json", core.Overlay)
	require.Equal(t, "someone", core.Principal)
	require.Equal(t, "/var/lib/prusa/authz.db", core.Authz.Database)
	require.Equal(t, []string{"10.0.0.0/8", "192.0.2.9"}, core.TrustedProxies)
	require.Equal(t, 3*time.Second, core.ShutdownTimeout)
	require.NoError(t, core.Validate())

	// A single address is read as its own host route, so an operator who names
	// one proxy does not have to write the mask.
	trusted, err := core.TrustedPrefixes()
	require.NoError(t, err)
	require.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.0.2.9/32"),
	}, trusted)
}

// TestAMisspelledCoreKeyIsRefused is why the file is decoded strictly: a
// misspelled address key would otherwise leave the process listening somewhere
// nobody asked for.
func TestAMisspelledCoreKeyIsRefused(t *testing.T) {
	t.Parallel()

	path := write(t, "core:\n  api:\n    address: \"127.0.0.1:9000\"\n")

	_, err := config.Load(path)
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), "address")
}

// TestValidateNamesTheKeyItRefused is what an operator reads on a failed start.
func TestValidateNamesTheKeyItRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		mutate func(*config.Core)
		names  string
	}{
		"no api address": {func(c *config.Core) { c.API.Addr = "" }, "core.api.addr"},
		"api address has no port": {
			func(c *config.Core) { c.API.Addr = "127.0.0.1" }, "core.api.addr",
		},
		"api port is not a number": {
			func(c *config.Core) { c.API.Addr = "127.0.0.1:http" }, "not a port number",
		},
		// A zero budget is no bound rather than a short one, and a stream budget
		// under the request budget is the longer key shortening a response.
		"request budget is not positive": {
			func(c *config.Core) { c.API.RequestTimeout = 0 }, "core.api.request_timeout",
		},
		"stream budget is not positive": {
			func(c *config.Core) { c.API.StreamTimeout = 0 }, "core.api.stream_timeout",
		},
		"stream budget under the request budget": {
			func(c *config.Core) { c.API.StreamTimeout = time.Second }, "the longer budget is the shorter one",
		},
		"unknown log level":  {func(c *config.Core) { c.Log.Level = "chatty" }, "core.log.level"},
		"unknown log format": {func(c *config.Core) { c.Log.Format = "xml" }, "core.log.format"},
		"no principal":       {func(c *config.Core) { c.Principal = "" }, "core.principal"},
		"shutdown budget is not positive": {
			func(c *config.Core) { c.ShutdownTimeout = 0 }, "core.shutdown_timeout",
		},
		"switch-off failures over 100": {
			func(c *config.Core) { c.ModulesSwitchOff.Failures = new(101) }, "core.modules_switch_off.failures",
		},
		"switch-off failures negative": {
			func(c *config.Core) { c.ModulesSwitchOff.Failures = new(-1) }, "core.modules_switch_off.failures",
		},
		"switch-off window under a minute": {
			func(c *config.Core) { c.ModulesSwitchOff.Window = 30 * time.Second }, "core.modules_switch_off.window",
		},
		"switch-off window over a day": {
			func(c *config.Core) { c.ModulesSwitchOff.Window = 25 * time.Hour }, "core.modules_switch_off.window",
		},
		"no overlay":           {func(c *config.Core) { c.Overlay = "" }, "core.overlay"},
		"no modules directory": {func(c *config.Core) { c.Modules = "" }, "core.modules"},
		// There is no key that turns the graph off, so a deployment that names
		// no database refuses to start rather than serving without one.
		"no authorization graph": {
			func(c *config.Core) { c.Authz.Database = "" }, "core.authz.database",
		},
		// A proxy this cannot read is refused rather than dropped, because a
		// proxy the server thinks it does not know is a deployment where every
		// client is counted under one address.
		"a trusted proxy that is not a network": {
			func(c *config.Core) { c.TrustedProxies = []string{"the office router"} },
			"core.trusted_proxies",
		},
		"a trusted proxy whose mask is not a number": {
			func(c *config.Core) { c.TrustedProxies = []string{"10.0.0.0/wide"} },
			"core.trusted_proxies",
		},
		// An origin is matched byte for byte against what a browser sends, so
		// a value in any other form is one that would never match. Each is
		// refused with the reason, rather than left to allow nobody.
		"a wildcard origin": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"*"} },
			"core.api.allowed_origins has \"*\"",
		},
		"an origin with a path": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://printers.example/"} },
			"more than a scheme and a host",
		},
		"an origin with no scheme": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"printers.example"} },
			"scheme is not http or https",
		},
		"an origin in uppercase": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://Printers.Example"} },
			"lowercase",
		},
		"an origin carrying the default port": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://printers.example:443"} },
			"default port",
		},
		"an origin with a userinfo": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://me@printers.example"} },
			"more than a scheme and a host",
		},
		"a wildcard host": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://*.printers.example"} },
			"wildcard",
		},
		"an origin with no host": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://:8080"} },
			"names no host",
		},
		"an origin with an empty port": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://printers.example:"} },
			"a port as a number",
		},
		"an origin with a padded port": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://printers.example:08443"} },
			"a port as a number",
		},
		"an origin with a port out of range": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://printers.example:99999"} },
			"a port as a number",
		},
		"an origin with a host in unicode": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://bücher.example"} },
			"ASCII",
		},
		"an origin with a long IPv6 address": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"https://[0:0::1]"} },
			"shortest form",
		},
		"an origin with a number a browser reads as an address": {
			func(c *config.Core) { c.API.AllowedOrigins = []string{"http://0x7f.1"} },
			"shortest form",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core := config.Default()
			core.Overlay = "overlay.json"
			core.Modules = "modules"
			core.Authz.Database = "authz.db"
			tc.mutate(&core)

			err := core.Validate()
			require.ErrorIs(t, err, config.ErrInvalid)
			require.Contains(t, err.Error(), tc.names)
		})
	}
}

// TestAWildcardOriginIsRefusedAtLoad is the value an operator reaches for
// first, refused where every other key is: read from the file, and refused by
// the validation the process runs before it binds a socket.
func TestAWildcardOriginIsRefusedAtLoad(t *testing.T) {
	t.Parallel()

	path := write(t, "core:\n  api:\n    allowed_origins: [\"*\"]\n")

	core, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, []string{"*"}, core.API.AllowedOrigins)

	err = core.Validate()
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), "core.api.allowed_origins")
	require.Contains(t, err.Error(), "wildcard")
}

// TestTwoListenersMayNotShareASocket is the configuration that puts the whole
// API on the network the printers are on. A wildcard API address beside a
// loopback device address on one port binds on Windows, and the machine's LAN
// address then answers GET /openapi.json in the clear while the device
// listener answers TLS on loopback. Linux refuses the second bind, so the
// platform decides whether it is a trap or an exposure and the configuration is
// wrong on both.
//
// The refusal is the composition root's, because the second address belongs to
// a connector. What this holds is the pairing rule it refuses on.
func TestTwoListenersMayNotShareASocket(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		api      string
		devices  string
		overlaps bool
	}{
		"the same address twice":                    {":8080", ":8080", true},
		"a wildcard api beside a loopback device":   {"0.0.0.0:8080", "127.0.0.1:8080", true},
		"a loopback api beside a wildcard device":   {"127.0.0.1:8080", "0.0.0.0:8080", true},
		"an unspecified host on both":               {":8080", "0.0.0.0:8080", true},
		"a dual-stack wildcard beside a loopback":   {"[::]:8080", "127.0.0.1:8080", true},
		"different ports on the same host":          {"127.0.0.1:8080", "127.0.0.1:8443", false},
		"different ports on wildcards":              {":8080", ":8443", false},
		"one port, two addresses neither wildcards": {"127.0.0.1:8080", "127.0.0.2:8080", false},
		"no device listener at all":                 {":8080", "", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.overlaps, config.SameSocket(tc.api, tc.devices))
		})
	}
}

func write(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// TestTheEngineSelectorIsReadFromOutsideTheCoreBlock is the one key the core
// takes from the file around it. `cmd/detect-build-tags` reads the same key to
// decide whether the Postgres driver is linked in, so a second copy under
// `core:` would let a build be derived from a value the runtime ignores.
func TestTheEngineSelectorIsReadFromOutsideTheCoreBlock(t *testing.T) {
	t.Parallel()

	path := write(t, `
persistence:
  type: postgres
  postgres:
    dsn: "postgres://someone:secret@db:5432/connect?sslmode=disable"
core:
  principal: someone
`)

	core, err := config.Load(path)
	require.NoError(t, err)

	require.Equal(t, config.EnginePostgres, core.Persistence.Type)
	require.Equal(t, "postgres://someone:secret@db:5432/connect?sslmode=disable",
		core.Persistence.Postgres.DSN)
	require.NoError(t, core.Validate())
}

// TestTheEngineSelectorInsideTheCoreBlockIsRefused keeps the selector in one
// place. Two spellings of it would disagree, and the one the build reads would
// win in silence.
func TestTheEngineSelectorInsideTheCoreBlockIsRefused(t *testing.T) {
	t.Parallel()

	_, err := config.Load(write(t, "core:\n  persistence:\n    type: postgres\n"))
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), "persistence")
}

// TestAFileWithNoEngineSelectorRunsOnSQLite is what every deployment that has
// not been touched since this key existed runs on.
func TestAFileWithNoEngineSelectorRunsOnSQLite(t *testing.T) {
	t.Parallel()

	core, err := config.Load(write(t, "core:\n  principal: someone\n"))
	require.NoError(t, err)
	require.Equal(t, config.EngineSQLite, core.Persistence.Type)
	require.NoError(t, core.Validate())

	empty, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)
	require.Equal(t, config.EngineSQLite, empty.Persistence.Type)
}

// TestAnEngineTheCoreCannotOpenIsRefused covers the value the shipping server
// takes and this one cannot: an authorization graph in memory forgets every
// grant on restart, so it is refused at start-up rather than accepted quietly.
func TestAnEngineTheCoreCannotOpenIsRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		engine string
		dsn    string
		names  string
	}{
		"memory":            {"memory", "", "persistence.type"},
		"a typo":            {"postgresql", "", "persistence.type"},
		"postgres, no dsn":  {config.EnginePostgres, "", "persistence.postgres.dsn"},
		"postgres with dsn": {config.EnginePostgres, "postgres://db/connect", ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core := config.Default()
			core.Overlay = "overlay.json"
			core.Modules = "modules"
			core.Authz.Database = "authz.db"
			core.Persistence.Type = tc.engine
			core.Persistence.Postgres.DSN = tc.dsn

			err := core.Validate()

			if tc.names == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, config.ErrInvalid)
			require.Contains(t, err.Error(), tc.names)
		})
	}
}

// The old backlog. A handle with no pool bound lets a burst open a connection per
// waiting request, so every deployment gets a bound whether or not its file
// names one.
func TestTheDatabasePoolIsBoundedWithoutBeingConfigured(t *testing.T) {
	t.Parallel()

	for name, path := range map[string]string{
		"no file at all":     filepath.Join(t.TempDir(), "absent.yaml"),
		"a file with no key": write(t, "core:\n  principal: someone\n"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core, err := config.Load(path)
			require.NoError(t, err)

			require.Equal(t, config.DefaultMaxOpenConns, core.Persistence.MaxOpenConns)
			require.Equal(t, config.DefaultMaxIdleConns, core.Persistence.MaxIdleConns)
			require.Equal(t, config.DefaultConnMaxLifetime, core.Persistence.ConnMaxLifetime)
			require.Equal(t, config.DefaultStatementTimeout, core.Persistence.Postgres.StatementTimeout)
			require.NoError(t, core.Validate())
		})
	}
}

// The operator's numbers are the ones that take effect, durations included,
// which yaml resolves as a string.
func TestEveryDatabasePoolKeyIsRead(t *testing.T) {
	t.Parallel()

	path := write(t, `
persistence:
  type: postgres
  max_open_conns: 40
  max_idle_conns: 10
  conn_max_lifetime: 5m
  postgres:
    dsn: "postgres://someone@db:5432/connect"
    statement_timeout: 90s
`)

	core, err := config.Load(path)
	require.NoError(t, err)
	require.NoError(t, core.Validate())

	require.Equal(t, 40, core.Persistence.MaxOpenConns)
	require.Equal(t, 10, core.Persistence.MaxIdleConns)
	require.Equal(t, 5*time.Minute, core.Persistence.ConnMaxLifetime)
	require.Equal(t, 90*time.Second, core.Persistence.Postgres.StatementTimeout)
}

// A negative reaches database/sql as "no limit", so an operator who typed one
// meaning "smaller" would get a pool with no bound at all and no message.
func TestANegativeDatabaseBoundIsRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"max_open_conns":    "persistence:\n  max_open_conns: -1\n",
		"max_idle_conns":    "persistence:\n  max_idle_conns: -1\n",
		"conn_max_lifetime": "persistence:\n  conn_max_lifetime: -1m\n",
		"statement_timeout": "persistence:\n  postgres:\n    statement_timeout: -1s\n",
	}

	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core, err := config.Load(write(t, file))
			require.NoError(t, err)

			err = core.Validate()
			require.ErrorIs(t, err, config.ErrInvalid)
			require.Contains(t, err.Error(), name)
		})
	}
}

// A key the file does not spell is refused with its line, which is what stops
// a typo silently leaving a bound unset.
func TestAMisspelledDatabasePoolKeyIsRefused(t *testing.T) {
	t.Parallel()

	_, err := config.Load(write(t, "persistence:\n  max_open_connections: 40\n"))
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), "max_open_connections")
}

// TestADSNIsPrintedWithoutItsPassword is what keeps a database password out of
// a start-up line, a journal and a bug report. Both forms the driver accepts
// carry one.
func TestADSNIsPrintedWithoutItsPassword(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		dsn  string
		want string
	}{
		"a url": {
			"postgres://someone:secret@db:5432/connect?sslmode=disable",
			"postgres://someone:xxxxx@db:5432/connect?sslmode=disable",
		},
		"a url with no password": {
			"postgres://db:5432/connect", "postgres://db:5432/connect",
		},
		"key and value": {
			"host=db user=someone password=secret dbname=connect",
			"host=db user=someone password=xxxxx dbname=connect",
		},
		"a quoted password with a space": {
			"host=db password='two words' dbname=connect",
			"host=db password=xxxxx dbname=connect",
		},
		"a dsn nothing can parse": {"postgres://%%%", "postgres"},
		"nothing at all":          {"", ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			engine := config.PostgresEngine{DSN: tc.dsn}
			require.Equal(t, tc.want, engine.RedactedDSN())
			require.NotContains(t, engine.RedactedDSN(), "secret")
		})
	}
}

func TestModulesLocalNamesAModulePathAndADirectory(t *testing.T) {
	t.Parallel()

	for name, local := range map[string]map[string]string{
		"no namespace": {"spools": "/srv/spools"},
		"no directory": {"github.com/acme/spools": " "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core := config.Default().WithDefaults(filepath.Join(t.TempDir(), "config.yaml"))
			core.ModulesLocal = local

			require.ErrorIs(t, core.Validate(), config.ErrInvalid)
		})
	}

	core := config.Default().WithDefaults(filepath.Join(t.TempDir(), "config.yaml"))
	core.ModulesLocal = map[string]string{"github.com/acme/spools": "/srv/spools"}
	require.NoError(t, core.Validate())
}
