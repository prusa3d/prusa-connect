package config

import "strings"

// Page is one page of the configuration section of the documentation: the
// part of the server it describes, and every key of that part. The first-run
// wizard links each key to its row there.
type Page struct {
	// Slug is the page's route under ReferencePage, such as network.
	Slug string
	// Title is the page's title.
	Title string
}

// Pages are the parts of the configuration in the order the documentation
// lists them.
//
//nolint:gochecknoglobals // the inventory of pages, read and never written.
var Pages = []Page{
	{Slug: "network", Title: "Network and API"},
	{Slug: "encryption", Title: "Encryption"},
	{Slug: "sign-in", Title: "Sign-in and access"},
	{Slug: "web-app", Title: "Web app"},
	{Slug: "discovery", Title: "Discovery on the local network"},
	{Slug: "printers", Title: "Printers"},
	{Slug: "resources", Title: "Storage, history and integrations"},
	{Slug: "database", Title: "Database"},
	{Slug: "logging", Title: "Logging and metrics"},
	{Slug: "extensions", Title: "Extensions"},
	{Slug: "server", Title: "Server internals"},
}

// pageRules map a key's path to its page. The first rule whose prefix the
// path starts with wins, so a narrower prefix is written before a wider one.
//
//nolint:gochecknoglobals // a lookup table, read-only after init.
var pageRules = []struct{ prefix, slug string }{
	{"core.api.tls.", "encryption"},
	{"core.frontend.printer_custom_ca", "encryption"},
	{"core.frontend.printer_address", "printers"},
	{"core.api.", "network"},
	{"core.trusted_proxies", "network"},
	{"core.time_server.", "network"},
	{"core.webrtc.", "network"},
	{"core.events.", "network"},
	{"core.auth.", "sign-in"},
	{"core.authz.database", "database"},
	{"core.authz.", "sign-in"},
	{"core.principal", "sign-in"},
	{"core.frontend.", "web-app"},
	{"core.dashboards.", "web-app"},
	{"core.discovery.", "discovery"},
	{"core.connectors.", "printers"},
	{"core.device_", "printers"},
	{"core.preview_cache_bytes", "printers"},
	{"persistence.", "database"},
	{"core.log.", "logging"},
	{"core.observability.", "logging"},
	{"core.marketplace.", "extensions"},
	{"core.modules", "extensions"},
	{"core.module_settings_bytes", "extensions"},
	{"resources.", "resources"},
}

// PageOf is the slug of the page a key is described on. A key no rule names
// is a server internal.
func PageOf(path string) string {
	for _, rule := range pageRules {
		if strings.HasPrefix(path, rule.prefix) {
			return rule.slug
		}
	}

	return "server"
}

// KeyReference is a key's row in the documentation, relative to the
// documentation's address, such as config/network/#key-core-api-addr.
func KeyReference(path string) string {
	return ReferencePage + PageOf(path) + "/#" + KeyAnchor(path)
}
