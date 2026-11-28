package httpapi

import (
	"context"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// defaultThemePath serves the stylesheet of the theme core.frontend.
// extension_theme names. The page links it before it draws, so it is a fixed
// address the page needs no answer to build.
const defaultThemePath = "/api/v1/themes/default.css"

// themeExtensions is what a theme's directory serves with no caller: the
// stylesheets, fonts and pictures a stylesheet loads. A script in the same
// directory still takes view_resource.
//
//nolint:gochecknoglobals // a closed table read per request, never written.
var themeExtensions = []string{".css", ".woff", ".woff2", ".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp"}

// themeFile reports whether a request below a module's mount reads a file in
// the directory of one of its themes' stylesheets, with an extension a theme
// loads. It is a read and nothing else.
func themeFile(inner *http.Request, themes map[string]string) bool {
	if len(themes) == 0 || (inner.Method != http.MethodGet && inner.Method != http.MethodHead) {
		return false
	}

	name, inside := strings.CutPrefix(inner.URL.Path, moduleUISegment)
	if !inside || name == "" {
		return false
	}

	if !slices.Contains(themeExtensions, strings.ToLower(path.Ext(name))) {
		return false
	}

	for _, stylesheet := range themes {
		dir := path.Dir(stylesheet)
		if dir == "." || strings.HasPrefix(name, dir+"/") {
			return true
		}
	}

	return false
}

// ThemeFiles is the stylesheet of each theme a bundle offers, by id, as the
// path inside the bundle that ModuleSurface.Themes holds.
func ThemeFiles(namespace string, ui ResourceUI) map[string]string {
	if len(ui.Themes) == 0 {
		return nil
	}

	prefix := ModuleUIPath(namespace)
	out := make(map[string]string, len(ui.Themes))

	for _, theme := range ui.Themes {
		if name, ok := strings.CutPrefix(theme.StylesheetURL, prefix); ok {
			out[theme.ID] = name
		}
	}

	return out
}

// DefaultThemeURL is where the stylesheet of the theme key names is served,
// as <namespace>/<id>, and false when no running module offers it.
func DefaultThemeURL(plane ModulePlane, key string) (string, bool) {
	if plane == nil {
		return "", false
	}

	namespace, id, found := strings.Cut(key, "/")
	if !found {
		return "", false
	}

	surface, claimed := plane.Module(namespace)
	if !claimed || surface.Mount != "" {
		return "", false
	}

	stylesheet, offered := surface.Themes[id]
	if !offered {
		return "", false
	}

	return ModuleUIPath(namespace) + stylesheet, true
}

// defaultThemeOutput is the theme's address, or an empty stylesheet.
type defaultThemeOutput struct {
	Status       int
	Location     string `header:"Location"      doc:"Where the theme's stylesheet is served. Sent only with 307."`
	ContentType  string `header:"Content-Type"  doc:"text/css with 200."`
	CacheControl string `header:"Cache-Control" doc:"no-cache, so a theme the operator changed is drawn on the next load."`
	Body         []byte
}

// registerDefaultTheme serves the stylesheet the page links before it draws.
func registerDefaultTheme(api huma.API, key string, plane ModulePlane) {
	register(api, huma.Operation{
		OperationID: "get-default-theme",
		Method:      http.MethodGet,
		Path:        defaultThemePath,
		Summary:     "The stylesheet of this deployment's default theme",
		Description: "Redirects with 307 to the stylesheet of the theme core.frontend.extension_theme names, " +
			"when a running module offers it. Otherwise it answers an empty stylesheet with 200, so a page " +
			"that links it draws its own look. It takes no credential, because the page links it before " +
			"anybody has signed in.",
		Tags:          []string{"system"},
		Security:      noSecurity,
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusInternalServerError},
		Responses: map[string]*huma.Response{
			"200": {Description: "An empty stylesheet: no default theme is drawn.", Content: map[string]*huma.MediaType{
				"text/css": {Schema: &huma.Schema{Type: huma.TypeString}},
			}},
			"307": {Description: "The theme's stylesheet is at the address the Location header names."},
		},
	}, func(_ context.Context, _ *struct{}) (*defaultThemeOutput, error) {
		out := &defaultThemeOutput{CacheControl: "no-cache"}

		if target, ok := DefaultThemeURL(plane, key); ok && key != "" {
			out.Status = http.StatusTemporaryRedirect
			out.Location = target

			return out, nil
		}

		out.Status = http.StatusOK
		out.ContentType = "text/css; charset=utf-8"
		out.Body = []byte{}

		return out, nil
	})
}
