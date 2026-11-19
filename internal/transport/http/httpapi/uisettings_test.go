package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// uiSettings is what get-ui-settings answers on this server.
func uiSettings(t *testing.T, handler http.Handler) AppSettings {
	t.Helper()

	rec := call(t, handler, http.MethodGet, "/api/v1/ui", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var settings AppSettings
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &settings))

	return settings
}

// TestTheUISettingsSayWhetherModulesAreOn holds ADR-20261019-h76 rule 15 at the
// handler: a server built without saying anything keeps modules on, as the
// dashboards are, and ModulesOff is what turns the field false.
func TestTheUISettingsSayWhetherModulesAreOn(t *testing.T) {
	t.Parallel()

	require.True(t, uiSettings(t, newTestServer(t).Handler()).Modules)

	cfg := exampleConfig(&gcodeFleetStub{printer: devPrinter()}, seededStore(), seededPresets())
	cfg.ModulesOff = true

	server, err := New(cfg)
	require.NoError(t, err)
	require.False(t, uiSettings(t, server.Handler()).Modules)
}

// TestTheUISettingsCarryNoSkin holds the answer to one appearance: the web app
// has no brand to switch, so the page is told the name, the language and the
// theme, and nothing names a skin.
func TestTheUISettingsCarryNoSkin(t *testing.T) {
	t.Parallel()

	rec := call(t, newTestServer(t).Handler(), http.MethodGet, "/api/v1/ui", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	require.NotContains(t, body, "skin")
	require.Contains(t, body, "theme")
	require.Contains(t, body, "title")
	require.Contains(t, body, "default_language")
}
