package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// appSettingsPath is where the page reads how this deployment wants to look. It
// is inside the sealed surface because a generated client and the app itself
// both call it, and everything this server answers is published in its OpenAPI
// document.
const appSettingsPath = "/api/v1/ui"

// AppSettings is core.frontend on the wire: the name, the language and the
// theme the page draws with, the address printers dial, and how a new printer
// joins.
//
// It takes no credential, and it carries nothing private. Whether printers are
// approved on arrival is not a secret: anybody who can dial /p/ws learns it
// from the answer.
//
// It is its own operation and not a field of get-auth-info because the page
// reads it before its first paint. A title and a theme fetched after the
// login form has drawn would change while the person watches.
type AppSettings struct {
	Enabled            bool           `json:"enabled"                        doc:"Whether this server serves the web app at its origin root. False means the root answers 404 and the page a client is reading came from somewhere else."`
	Title              string         `json:"title"                          doc:"The name in the browser tab and at the head of the sidebar."`
	DefaultLanguage    string         `json:"default_language"               doc:"The catalogue the page falls back to when the browser asks for one this app does not ship."`
	Theme              string         `json:"theme"                          doc:"system, light or dark. system follows whatever the operating system asks for."`
	PrinterAddress     string         `json:"printer_address,omitempty"      doc:"The origin a printer dials, which the add-printer page writes into the printer's settings file. Absent means the origin the page was opened on."`
	SupportURL         string         `json:"support_url,omitempty"          doc:"Where the app's Support link points. Empty keeps the assistant on the beta portal."`
	APIReference       string         `json:"api_reference,omitempty"        doc:"The path of this server's API reference page. Absent in a build that leaves the page out."                                                                                                                                                    readOnly:"true"`
	PrinterAdmission   string         `json:"printer_admission"              doc:"How a new printer joins. approval means a person approves the code each printer shows; automatic means every printer that asks is approved at once. With several connectors it is automatic only when every one of them approves on arrival." readOnly:"true" enum:"approval,automatic"`
	PrinterCustomCA    bool           `json:"printer_custom_ca"              doc:"Whether printers are served a certificate signed by this deployment's own authority, so a printer's settings file asks it to trust the connect.der on its USB stick."`
	Documentation      string         `json:"documentation,omitempty"        doc:"The address of the documentation written for this build, ending in a slash. A page path is appended to it, such as config/. Absent in a build that names none."                                                                               readOnly:"true"`
	Dashboards         bool           `json:"dashboards"                     doc:"Whether this server serves the dashboards. False in a build that leaves them out and where core.dashboards.enabled is off, and then the home page is the printer list."                                                                       readOnly:"true"`
	Modules            bool           `json:"modules"                        doc:"Whether this server installs modules and runs the ones it installed. False where the operator set core.modules_enabled to false, and in safe mode, where safe_mode says so; then the app shows no catalogue, no marketplace and no install."  readOnly:"true"`
	Upgrades           bool           `json:"upgrades"                       doc:"Whether this server prepares upgrades and keeps backups. False where core.upgrade.enabled is off, and the app then shows neither."                                                                                                            readOnly:"true"`
	SafeMode           *SafeModeState `json:"safe_mode,omitempty"            doc:"Present while this run is in safe mode: every module is off, whatever the configuration says, until the server restarts normally. It never names who asked for it."                                                                           readOnly:"true"`
	DiagnosticsSendURL string         `json:"diagnostics_send_url,omitempty" doc:"Where the browser sends the diagnostics bundle. Absent when core.diagnostics.send_url is empty."`
	Diagnostics        bool           `json:"diagnostics"                    doc:"Whether this server serves the diagnostics bundle. False in a build that leaves it out, where core.diagnostics.enabled is off, and on an install with open access that does not switch it on; then the app hides the download."               readOnly:"true"`
}

// SafeModeState is how the running server entered safe mode.
type SafeModeState struct {
	Reason string    `json:"reason" enum:"flag,restart" doc:"flag when the server was started with -safe; restart when an administrator asked for a restart in safe mode."`
	Since  time.Time `json:"since"                      doc:"When this run took safe mode."`
}

// appSettingsOutput is the one answer this operation has.
type appSettingsOutput struct {
	Body AppSettings
}

// registerAppSettings publishes how the page is to look. It is registered
// whatever this deployment was wired with, so a client generated from the
// document does not depend on whether this build carries an app: a build
// without one answers the same shape with enabled false.
func registerAppSettings(
	api huma.API, settings AppSettings, dashboards, modules, upgrades bool, diagnostics func() bool,
) {
	register(api, huma.Operation{
		OperationID: "get-ui-settings",
		Method:      http.MethodGet,
		Path:        appSettingsPath,
		Summary:     "How this deployment's web app looks",
		Description: "Reports the name, the fallback language and the theme this " +
			"server's own page draws with, and whether that page is served here at all. It takes no " +
			"credential, because the page reads it before it paints and before anybody has signed in. " +
			"It names no account, no printer and no module. It does say whether a new printer waits " +
			"for a person's approval, which anybody who can reach the printer port learns anyway.",
		Tags:     []string{"system"},
		Security: noSecurity,
		Errors:   []int{http.StatusInternalServerError},
	}, func(_ context.Context, _ *struct{}) (*appSettingsOutput, error) {
		settings.APIReference, _ = ReferencePage()
		settings.Dashboards = dashboards
		settings.Modules = modules
		settings.Upgrades = upgrades
		settings.Diagnostics = diagnostics()

		return &appSettingsOutput{Body: settings}, nil
	})
}
