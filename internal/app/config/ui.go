package config

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

// The web app's own keys. The server reads each one and hands it to the page
// before it paints, so a deployment changes its name or its language here and
// needs no new build.
const (
	// DefaultTitle is what the browser tab and the sidebar head say. The title
	// is a key because a company running this server puts its own name on it.
	DefaultTitle = "Prusa Connect"
	// DefaultLanguage is the catalogue the page starts in. The browser's own
	// preference wins when the app carries that language.
	DefaultLanguage = "en"
	// DefaultTheme lets the operating system decide between light and dark.
	DefaultTheme = ThemeSystem
	// MaxTitleBytes bounds core.frontend.title. It is served unauthenticated
	// and written into a browser tab.
	MaxTitleBytes = 64
	// MaxPrinterHostBytes is what a Prusa printer stores of the hostname in its
	// settings file, for a name that does not end in prusa3d.com. A longer name
	// loads without an error and then fails on the printer as `DNS error`
	// (docs/src/content/docs/printer-configuration.md, "Hostname length").
	MaxPrinterHostBytes = 20
)

// The three values core.frontend.theme takes. System is not a third palette:
// the page follows what the operating system asks for.
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// languages is the catalogue set the app ships. It is copied here because the
// binary carries no catalogue, and the core must not serve a value the page has
// no catalogue for.
//
// TestTheLanguagesAreTheCataloguesTheAppShips compares the list with the tree it
// was copied from.
//
//nolint:gochecknoglobals // two closed sets read at validation time, never written.
var (
	languages = []string{"en", "cs", "de", "es", "fr", "it", "pl"}
	themes    = []string{ThemeSystem, ThemeLight, ThemeDark}
)

// Frontend is the web app served at the origin root, and how it looks before
// anybody signs in.
//
// Every key here reaches the browser through GET /api/v1/ui, which takes no
// credential, because the page reads it before its first paint. There is
// nothing private in it: a name, a language, a palette and the address every
// printer is already told to dial.
//
// Enabled is a pointer so that an operator who wrote `enabled: false` is told
// apart from one who wrote no block. The default is on: a server nobody
// configured serves its own app.
type Frontend struct {
	// Enabled serves the app at the origin root. Off, the root answers the same
	// 404 a build carrying no app answers, so a deployment can put its own
	// page in front of this API.
	Enabled *bool `yaml:"enabled"`
	// Title is the name in the browser tab and at the head of the sidebar.
	Title string `yaml:"title"`
	// DefaultLanguage is the catalogue the page falls back to. One of the
	// languages the app ships; a language it has no catalogue for would leave
	// every label as its key.
	DefaultLanguage string `yaml:"default_language"`
	// Theme is system, light or dark.
	Theme string `yaml:"theme"`
	// PrinterAddress is the origin a printer dials, as the add-printer page
	// writes it into prusa_printer_settings.ini. Empty means the address the
	// page was opened on. It is set when printers reach this server by another
	// name than browsers do, as behind a proxy that does not pass the printer's
	// TLS through.
	PrinterAddress string `yaml:"printer_address"`
	// PrinterCustomCA says the certificate printers are served is signed by an
	// authority of this deployment's own, which a printer trusts only from the
	// connect.der on its USB stick. The add-printer page then writes
	// custom_cert into the settings file.
	PrinterCustomCA bool `yaml:"printer_custom_ca"`
	// SupportURL is where the footer's Support link points. Empty keeps the
	// assistant on the beta portal.
	SupportURL string `yaml:"support_url"`
}

// On reports whether the app is served. An absent key is on.
func (f Frontend) On() bool { return f.Enabled == nil || *f.Enabled }

// validate reports the first key the app will not be served on. Each value
// here is handed to a browser, so a value the page has no palette or catalogue
// for is refused at startup.
func (f Frontend) validate() error {
	if strings.TrimSpace(f.Title) == "" {
		return fmt.Errorf("%w: core.frontend.title is empty", ErrInvalid)
	}

	if len(f.Title) > MaxTitleBytes {
		return fmt.Errorf("%w: core.frontend.title is %d bytes, expected at most %d",
			ErrInvalid, len(f.Title), MaxTitleBytes)
	}

	if !slices.Contains(languages, f.DefaultLanguage) {
		return fmt.Errorf("%w: core.frontend.default_language is %q, expected one of %v",
			ErrInvalid, f.DefaultLanguage, languages)
	}

	if !slices.Contains(themes, f.Theme) {
		return fmt.Errorf("%w: core.frontend.theme is %q, expected one of %v",
			ErrInvalid, f.Theme, themes)
	}

	if err := validPrinterAddress(f.PrinterAddress); err != nil {
		return err
	}

	return validSupportURL(f.SupportURL)
}

// validSupportURL refuses a link the browser would not open as a page: one that
// is not an absolute http or https URL. Unlike the printer address it may carry
// a path and a query.
func validSupportURL(address string) error {
	if address == "" {
		return nil
	}

	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return fmt.Errorf("%w: core.frontend.support_url is %q, expected an http or https URL",
			ErrInvalid, address)
	}

	return nil
}

// validPrinterAddress refuses an address the printer could not dial: one that
// is not a bare http or https origin, a loopback or unspecified address that on
// the printer names the printer itself, and one whose hostname the firmware
// would cut short. Only names ending in prusa3d.com are shortened by the
// firmware and may be longer, so those are left to it.
func validPrinterAddress(address string) error {
	if address == "" {
		return nil
	}

	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
		parsed.User != nil || strings.TrimSuffix(parsed.Path, "/") != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: core.frontend.printer_address is %q, expected an origin such as "+
			"https://printers.example.com", ErrInvalid, address)
	}

	host := parsed.Hostname()
	if ip := net.ParseIP(host); IsLoopbackHost(host) || (ip != nil && ip.IsUnspecified()) {
		return fmt.Errorf("%w: core.frontend.printer_address host %q is this machine's own address, "+
			"which a printer cannot reach; give the address printers reach it at on the network",
			ErrInvalid, host)
	}

	if !strings.HasSuffix(host, "prusa3d.com") && len(host) > MaxPrinterHostBytes {
		return fmt.Errorf("%w: core.frontend.printer_address host %q is %d bytes, a printer keeps %d",
			ErrInvalid, host, len(host), MaxPrinterHostBytes)
	}

	return nil
}
