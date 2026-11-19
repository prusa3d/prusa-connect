package config_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/app/config"
)

// TestAKeyTheCoreDoesNotReadIsRefusedWithItsLineNumber is the whole point of
// the strict loader. The retired server's blocks are the ones an upgraded
// deployment still holds, and an operator who configured one of them would
// otherwise discover on the next outage that it was never running.
//
// The line number is what makes the refusal actionable: the shipped file is
// hundreds of lines long, and a message naming only the block sends somebody
// searching it.
func TestAKeyTheCoreDoesNotReadIsRefusedWithItsLineNumber(t *testing.T) {
	t.Parallel()

	path := write(t, `core:
  principal: someone
server:
  host: "0.0.0.0"
`)

	_, err := config.Load(path)
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), "line 3")
	require.Contains(t, err.Error(), "server")
	require.Contains(t, err.Error(), path, "the refusal does not say which file it read")
	require.Contains(t, err.Error(), "core, persistence, resources",
		"the refusal does not say which blocks are read instead")
}

// TestARetiredKeyNamesTheKeyThatReplacedIt is the upgrade an operator makes
// without reading a changelog. A key that moved is a line they wrote on purpose
// and that nothing reads any more, so the refusal carries the key that does
// that work now and the process does not start pretending to obey it.
func TestARetiredKeyNamesTheKeyThatReplacedIt(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		file  string
		line  string
		names []string
	}{
		"the printer listener, which belongs to its connector now": {
			file: `core:
  printer:
    addr: ":443"
`,
			line:  "line 2",
			names: []string{"core.printer", "core.connectors.<name>.config.listener"},
		},
		"the compatibility switch, which is the resource row now": {
			file: `core:
  compat:
    graphql:
      enabled: false
`,
			line:  "line 2",
			names: []string{"core.compat", "resources.sink.graphql.enabled"},
		},
		"the MQTT block, which is a sink now": {
			file: `core:
  principal: operator
mqtt:
  enabled: true
`,
			line:  "line 3",
			names: []string{"mqtt", "resources.sink", "type mqtt"},
		},
		"the metrics block, which is a core key now": {
			file: `observability:
  enabled: true
`,
			line:  "line 1",
			names: []string{"observability", "core.observability.enabled"},
		},
		"the link block, which is a resource instance now": {
			file: `fractal:
  enabled: true
`,
			line:  "line 1",
			names: []string{"fractal", "resources.link.<name>.enabled"},
		},
		"the skin, which nothing replaces": {
			file: `core:
  frontend:
    skin: slicer
`,
			line:  "line 3",
			names: []string{"core.frontend.skin", "one appearance", "Delete the line"},
		},
		"the skin with no value": {
			file: `core:
  frontend:
    skin:
`,
			line:  "line 3",
			names: []string{"core.frontend.skin", "one appearance", "Delete the line"},
		},
		"the skin no build ever drew": {
			file: `core:
  frontend:
    skin: neon
`,
			line:  "line 3",
			names: []string{"core.frontend.skin", "one appearance", "Delete the line"},
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := config.Load(write(t, testCase.file))
			require.ErrorIs(t, err, config.ErrInvalid)
			require.Contains(t, err.Error(), testCase.line)

			for _, names := range testCase.names {
				require.Contains(t, err.Error(), names)
			}
		})
	}
}

// TestARetiredKeyInTheSetupFileNamesThatFile is the deployment whose operator
// never wrote the key: the first-run wizard did, into its own file in the
// overlay folder. A refusal that named only the operator's file would send them
// looking for a line that is not there.
func TestARetiredKeyInTheSetupFileNamesThatFile(t *testing.T) {
	t.Parallel()

	path := write(t, "core:\n  principal: operator\n")
	setup := writeSetup(t, path, "core:\n  frontend:\n    skin: slicer\n")

	_, err := config.Load(path)
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), setup)
	require.Contains(t, err.Error(), "line 3")
	require.Contains(t, err.Error(), "core.frontend.skin")
}

// TestAnUnknownKeyInsideTheCoreBlockIsRefusedWithItsLineNumber is the typo an
// operator makes rather than the block they inherited. A misspelled address key
// would leave the process listening somewhere nobody asked for.
func TestAnUnknownKeyInsideTheCoreBlockIsRefusedWithItsLineNumber(t *testing.T) {
	t.Parallel()

	path := write(t, `core:
  api:
    addr: ":8080"
    address: ":9000"
`)

	_, err := config.Load(path)
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), "line 4")
	require.Contains(t, err.Error(), "core.api.address")
}

// TestAnUnknownKeyInsideThePersistenceBlockIsRefused closes the last permissive
// block. It held the retired server's keys, so a deployment could name a
// sqlite_path there and believe the database was going where it said.
func TestAnUnknownKeyInsideThePersistenceBlockIsRefused(t *testing.T) {
	t.Parallel()

	path := write(t, `persistence:
  type: sqlite
  sqlite_path: ./data/state.db
`)

	_, err := config.Load(path)
	require.ErrorIs(t, err, config.ErrInvalid)
	require.Contains(t, err.Error(), "line 3")
	require.Contains(t, err.Error(), "persistence.sqlite_path")
}

// TestAnUnknownKeyIsFoundHoweverDeepItIs walks the nested cases the loader has
// to reach: a listener's certificate block, an instance name the operator chose
// under core.connectors, and the settings block under that name, which is the
// driver's and must not be refused.
func TestAnUnknownKeyIsFoundHoweverDeepItIs(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		content string
		key     string
		line    int
	}{
		"under a listener's certificate": {
			content: "core:\n  api:\n    tls:\n      cert_file: api.crt\n",
			key:     "core.api.tls.cert_file",
			line:    4,
		},
		"under an instance name the operator chose": {
			content: "core:\n  connectors:\n    xbuddy:\n      type: xbuddy\n      ws_path: /p/ws\n",
			key:     "core.connectors.xbuddy.ws_path",
			line:    5,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := config.Load(write(t, testCase.content))
			require.ErrorIs(t, err, config.ErrInvalid)
			require.Contains(t, err.Error(), testCase.key)
			require.Contains(t, err.Error(), "line "+strconv.Itoa(testCase.line))
		})
	}
}

// TestADriversOwnSettingsAreNotHeldToTheCoresFieldNames is the other side of
// the walk. The core carries a connector's settings block to the dialect
// unread, because the dialect owns that schema and is the only thing that can
// say whether it is right.
func TestADriversOwnSettingsAreNotHeldToTheCoresFieldNames(t *testing.T) {
	t.Parallel()

	path := write(t, `core:
  connectors:
    xbuddy:
      type: xbuddy
      config:
        max_registrations: 400
        anything_at_all: true
`)

	core, err := config.Load(path)
	require.NoError(t, err)
	require.NoError(t, core.Validate())
}

// TestTheShippedConfigurationAndItsExamplesLoad is the gate that keeps this
// repository's own files honest. A key deleted in Go and left behind in one of
// them now stops a deployment at startup, and nothing else in the tree would
// see it.
func TestTheShippedConfigurationAndItsExamplesLoad(t *testing.T) {
	t.Parallel()

	shipped := filepath.Join("..", "..", "..", "config")

	entries, err := os.ReadDir(filepath.Join(shipped, "examples"))
	require.NoError(t, err)

	files := []string{filepath.Join(shipped, "config.yaml")}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".yaml") {
			files = append(files, filepath.Join(shipped, "examples", entry.Name()))
		}
	}

	// Five examples and the shipped file. An empty sweep would otherwise pass.
	require.Len(t, files, 6)

	for _, file := range files {
		// Load and not Validate. The TLS profile names certificate files that
		// only exist once `make certificate-all` has run, and validation reads
		// them off the disk; what is under test here is the schema.
		_, loadErr := config.Load(file)
		require.NoError(t, loadErr, "%s names a key this build does not read", file)
	}
}

// TestThePrinterConfigurationSetsWhatThePrinterNeeds reads the file the
// printer's operating system installs beside the printer binary. It must
// start, and it must carry the small pool and the memory limit, because the
// printer binary does not default either of them.
func TestThePrinterConfigurationSetsWhatThePrinterNeeds(t *testing.T) {
	t.Parallel()

	core, err := config.Load(filepath.Join("..", "..", "..", "config", "examples", "config.printer.yaml"))
	require.NoError(t, err)
	require.NoError(t, core.Validate())

	require.Equal(t, 4, core.Persistence.MaxOpenConns)
	require.Equal(t, 2, core.Persistence.MaxIdleConns)
	require.Equal(t, int64(12<<20), core.MemoryLimit)
	require.True(t, core.Frontend.On(), "the printer serves the same web app as every build")
}
