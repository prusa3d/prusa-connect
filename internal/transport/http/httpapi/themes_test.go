package httpapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
)

// themedPlanner is the planner bundle offering style.css as a theme, behind a
// graph that grants the caller nothing, so a file served here was served with
// no permission checked.
func themedPlanner(t *testing.T) (fakeModulePlane, *fakeModuleAccess) {
	t.Helper()

	instance, err := ref.Parse("asset:planner")
	require.NoError(t, err)

	plane := fakeModulePlane{
		namespace: moduleNamespace, instance: instance, handler: ModuleUIHandler(aBundle()),
		themes: map[string]string{"ocean": "style.css"},
	}

	return plane, &fakeModuleAccess{who: "user:operator", instance: instance}
}

// TestAThemeAndWhatItLoadsAreServedWithNoGrant is why a theme draws on the
// sign-in page: its stylesheet, and the pictures and fonts beside it, need no
// caller. A script beside it still takes view_resource.
func TestAThemeAndWhatItLoadsAreServedWithNoGrant(t *testing.T) {
	t.Parallel()

	plane, access := themedPlanner(t)
	server := moduleServer(t, plane, access, "user:operator")
	base := "/api/v1/modules/" + moduleNamespace + "/ui/"

	for _, file := range []string{"style.css", "logo.svg"} {
		served := ask(t, server, http.MethodGet, base+file, "")
		require.Equal(t, http.StatusOK, served.Code, file)
	}

	script := ask(t, server, http.MethodGet, base+"index.js", "")
	require.Equal(t, http.StatusForbidden, script.Code, script.Body.String())
}

// TestABundleWithNoThemeServesNothingWithNoGrant holds the files of every
// other bundle to the instance's grants.
func TestABundleWithNoThemeServesNothingWithNoGrant(t *testing.T) {
	t.Parallel()

	plane, access := themedPlanner(t)
	plane.themes = nil
	server := moduleServer(t, plane, access, "user:operator")

	served := ask(t, server, http.MethodGet, "/api/v1/modules/"+moduleNamespace+"/ui/style.css", "")
	require.Equal(t, http.StatusForbidden, served.Code)
}

func TestThemeFileReadsOnlyATheme(t *testing.T) {
	t.Parallel()

	themes := map[string]string{"ocean": "themes/ocean.css"}
	cases := map[string]struct {
		method, path string
		public       bool
	}{
		"the stylesheet":            {http.MethodGet, "/ui/themes/ocean.css", true},
		"a font beside it":          {http.MethodGet, "/ui/themes/fonts/a.woff2", true},
		"a script beside it":        {http.MethodGet, "/ui/themes/ocean.js", false},
		"a stylesheet elsewhere":    {http.MethodGet, "/ui/other.css", false},
		"a write to the stylesheet": {http.MethodPost, "/ui/themes/ocean.css", false},
		"a route of the module":     {http.MethodGet, "/themes/ocean.css", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request, err := http.NewRequest(tc.method, tc.path, nil)
			require.NoError(t, err)
			require.Equal(t, tc.public, themeFile(request, themes))
		})
	}
}

// TestTheDefaultThemeRedirectsToTheStylesheetOfTheModuleThatOffersIt covers
// both answers the page links before it draws.
func TestTheDefaultThemeRedirectsToTheStylesheetOfTheModuleThatOffersIt(t *testing.T) {
	t.Parallel()

	plane, _ := themedPlanner(t)

	cases := map[string]struct {
		key      string
		status   int
		location string
	}{
		"a theme a module offers": {
			key: moduleNamespace + "/ocean", status: http.StatusTemporaryRedirect,
			location: ModuleUIPath(moduleNamespace) + "style.css",
		},
		"a theme no module offers": {key: moduleNamespace + "/paper", status: http.StatusOK},
		"no theme named":           {key: "", status: http.StatusOK},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server, err := New(Config{
				Version:      "test",
				App:          AppSettings{ExtensionTheme: tc.key},
				Principals:   fixedPrincipal("user:operator"),
				Resources:    seededStore(),
				Presets:      seededPresets(),
				Endpoints:    seededEndpoints(),
				Grants:       &fakeGrantStore{},
				Sessions:     newFakeSessions(),
				Accounts:     &fakeAccounts{info: AuthInfo{Provider: "local", Credentials: true}},
				Readiness:    &fakeReadiness{report: readyReport()},
				Devices:      noPrinters{},
				Stores:       noStorages{},
				Events:       noEvents{},
				Modules:      plane,
				ModuleAccess: &fakeModuleAccess{},
			})
			require.NoError(t, err)

			served := ask(t, server, http.MethodGet, defaultThemePath, "")
			require.Equal(t, tc.status, served.Code, served.Body.String())
			require.Equal(t, tc.location, served.Header().Get("Location"))
			require.Equal(t, "no-cache", served.Header().Get("Cache-Control"))

			if tc.status == http.StatusOK {
				require.Equal(t, "text/css; charset=utf-8", served.Header().Get("Content-Type"))
				require.Empty(t, served.Body.String())
			}
		})
	}
}
