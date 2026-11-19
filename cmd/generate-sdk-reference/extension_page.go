package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/codegen/openapidoc"
	"github.com/prusa3d/prusa-connect/internal/docsref"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// The official extension pages: one page per folder under extensions/ that
// has a module.yaml, and one index. The region repeats what the manifest and
// the OpenAPI fragment say.

const (
	extDir = "extensions"

	extFix = "change module.yaml or openapi.json and regenerate."

	// extIndexName is the page that lists every extension.
	extIndexName = "index.md"

	// extModulePrefix plus the folder name is an official extension's
	// module path.
	extModulePrefix = "github.com/prusa3d/"

	manifestName = "module.yaml"
)

// officialExtension is one folder under extensions/ and what it declares.
type officialExtension struct {
	// Folder is the folder's name and also the module's name.
	Folder     string
	Module     string
	Namespace  string
	Manifest   manifest.Manifest
	Operations []openapidoc.Operation
}

// readExtensions reads every folder under dir that has a module.yaml, in
// name order.
func readExtensions(dir string) ([]officialExtension, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var out []officialExtension

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name(), manifestName)

		document, readErr := os.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}

		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}

		ext, readErr := readExtension(filepath.Join(dir, entry.Name()), entry.Name(), document)
		if readErr != nil {
			return nil, readErr
		}

		out = append(out, ext)
	}

	return out, nil
}

func readExtension(dir, folder string, document []byte) (officialExtension, error) {
	declared, err := manifest.Parse(filepath.Join(dir, manifestName), document, yaml.Unmarshal)
	if err != nil {
		return officialExtension{}, fmt.Errorf("%s: %w", folder, err)
	}

	module := extModulePrefix + folder

	namespace, err := extension.NamespaceOf(module)
	if err != nil {
		return officialExtension{}, fmt.Errorf("%s: %w", folder, err)
	}

	ext := officialExtension{Folder: folder, Module: module, Namespace: namespace.String(), Manifest: declared}

	if declared.OpenAPI == "" {
		return ext, nil
	}

	fragment, err := openapidoc.Read(filepath.Join(dir, filepath.FromSlash(declared.OpenAPI)))
	if err != nil {
		return officialExtension{}, fmt.Errorf("%s: %w", folder, err)
	}

	if ext.Operations, err = fragment.Operations(); err != nil {
		return officialExtension{}, fmt.Errorf("%s: %w", folder, err)
	}

	slices.SortStableFunc(ext.Operations, func(a, b openapidoc.Operation) int {
		return strings.Compare(a.Path+" "+a.Method, b.Path+" "+b.Method)
	})

	return ext, nil
}

func extPageName(folder string) string { return folder + ".md" }

// extensionPages renders one page per extension and the index.
func extensionPages(referenceDir string, exts []officialExtension) []page {
	pages := make([]page, 0, len(exts)+1)

	for _, ext := range exts {
		region := regionOf("extensions/"+ext.Folder, extFix, renderExtension(ext))

		title := ext.Manifest.Title
		if title == "" {
			title = ext.Folder
		}

		front := frontMatter{
			title:         title,
			description:   strings.TrimSuffix(ext.Manifest.Summary, ".") + ".",
			audience:      "run",
			implementedBy: []string{"extensions/" + ext.Folder, generatorPath},
		}

		placeholder := fmt.Sprintf("The %s extension.\n\n%s: write what the extension is for, who switches it "+
			"on and what to set up first, then delete this line.", title, placeholderMark)

		pages = append(pages, page{
			path:   filepath.Join(referenceDir, extDir, extPageName(ext.Folder)),
			region: region,
			fresh:  freshPage(front, placeholder, region),
		})
	}

	region := regionOf("every folder under extensions/", extFix, renderExtensionIndex(exts))
	front := frontMatter{
		title:         "Official extensions",
		description:   "The extensions Prusa Research ships with the server, and what each adds.",
		audience:      "run",
		order:         1,
		implementedBy: []string{"extensions", generatorPath},
	}

	pages = append(pages, page{
		path:   filepath.Join(referenceDir, extDir, extIndexName),
		region: region,
		fresh: freshPage(front, placeholderMark+": write how an operator gets these extensions, "+
			"then delete this line.", region),
	})

	return pages
}

func renderExtensionIndex(exts []officialExtension) string {
	var body strings.Builder

	body.WriteString("## The extensions {#the-extensions}\n\n")

	for _, ext := range exts {
		title := ext.Manifest.Title
		if title == "" {
			title = ext.Folder
		}

		fmt.Fprintf(&body, "- **[%s](./%s)** %s. %s\n", docsref.EscapeProse(title), extPageName(ext.Folder),
			ext.Manifest.Version, sentence(ext.Manifest.Summary))
	}

	return body.String()
}

func renderExtension(ext officialExtension) string {
	var body strings.Builder

	writeAtAGlance(&body, ext)
	writeAppPlaces(&body, ext.Manifest)
	writeNeeds(&body, ext.Manifest)
	writeSettings(&body, ext.Manifest.Options)
	writeEvents(&body, ext.Manifest.Events)
	writeRoutes(&body, ext)

	return body.String()
}

func writeAtAGlance(body *strings.Builder, ext officialExtension) {
	m := ext.Manifest

	body.WriteString("## At a glance {#at-a-glance}\n\n")

	if m.Summary != "" {
		body.WriteString(docsref.EscapeProse(m.Summary) + "\n\n")
	}

	body.WriteString("| | |\n|---|---|\n")
	fmt.Fprintf(body, "| version | %s |\n", m.Version)
	fmt.Fprintf(body, "| licence | %s |\n", docsref.EscapeProse(m.License))
	fmt.Fprintf(body, "| module path | `%s` |\n", ext.Module)
	fmt.Fprintf(body, "| namespace | `%s` |\n", ext.Namespace)

	if m.Codeowner != "" {
		fmt.Fprintf(body, "| maintained by | %s |\n", m.Codeowner)
	}

	if m.Lifecycle != "" {
		fmt.Fprintf(body, "| lifecycle | %s |\n", m.Lifecycle)
	}

	fmt.Fprintf(body, "| what it ships | %s |\n", halves(m))

	if len(m.Platforms) > 0 {
		fmt.Fprintf(body, "| platforms | %s |\n", strings.Join(m.Platforms, ", "))
	}

	fmt.Fprintf(body, "| source | `extensions/%s`, with its README and CHANGELOG |\n", ext.Folder)

	if len(m.Egress) > 0 {
		body.WriteString("\nIt connects outside Prusa Connect to:\n\n")

		for _, egress := range m.Egress {
			body.WriteString("- " + docsref.EscapeProse(egress.Line()) + "\n")
		}
	}
}

// halves says whether a release ships a server process, a browser bundle or
// both.
func halves(m manifest.Manifest) string {
	switch {
	case m.ContractMajor > 0 && m.UI != nil:
		return "a server process and a browser bundle"
	case m.ContractMajor > 0:
		return "a server process"
	case m.UI != nil:
		return "a browser bundle. The server serves it and never runs it"
	}

	return "nothing the server runs"
}

// writeAppPlaces lists where the extension draws in the app, in slot table
// order, then its exported components and the file types it analyses.
func writeAppPlaces(body *strings.Builder, m manifest.Manifest) {
	var lines []string

	if m.UI != nil {
		lines = appPlaces(m.UI)
	}

	if m.FileAnalysis != nil {
		lines = append(lines, fmt.Sprintf("the server asks it to describe a file of type %s: the picture in a "+
			"listing and the details on the file's page", codeList(m.FileAnalysis.Types)))
	}

	if len(lines) == 0 {
		return
	}

	body.WriteString("\n## What it adds {#what-it-adds}\n\n")

	for _, line := range lines {
		body.WriteString("- " + line + "\n")
	}
}

func appPlaces(ui *manifest.UI) []string {
	var lines []string

	for _, slot := range manifest.Slots() {
		for _, entry := range slotEntries(ui.Contributes, slot.Name) {
			line := fmt.Sprintf("**%s**: %s", docsref.EscapeProse(entry.title), slot.Offer)
			if len(entry.fileTypes) > 0 {
				line += ", for " + codeList(entry.fileTypes)
			}

			if entry.devices.Resident {
				line += ", but only on the printer this server runs on"
			}

			if entry.isolated {
				line += ", as a cross-origin isolated page"
			}

			lines = append(lines, line)
		}
	}

	for _, export := range ui.Exports {
		lines = append(lines,
			fmt.Sprintf("**`%s`**: a component other modules may draw on their own pages", export.Name))
	}

	for _, imported := range ui.Imports {
		lines = append(lines, fmt.Sprintf("draws the component `%s` of `%s`", imported.Name, imported.Module))
	}

	return lines
}

// slotEntry is one registration of the ui block, for any kind of slot.
type slotEntry struct {
	title     string
	fileTypes []string
	devices   manifest.DeviceSelector
	isolated  bool
}

func slotEntries(c manifest.Contributes, slot manifest.SlotName) []slotEntry {
	var out []slotEntry

	// Pages and cards have their own lists. Every other slot is looked up by
	// name in the sections and the actions.
	if slot == manifest.SlotPage {
		for _, p := range c.Pages {
			out = append(out, slotEntry{title: p.Title})
		}
	}

	if slot == manifest.SlotDashboardCard {
		for _, card := range c.Cards {
			out = append(out, slotEntry{title: card.Title})
		}
	}

	for _, section := range c.Sections[slot] {
		out = append(out, slotEntry{
			title: section.Title, fileTypes: section.FileTypes, devices: section.Devices,
			isolated: section.CrossOriginIsolated,
		})
	}

	for _, action := range c.Actions[slot] {
		out = append(out, slotEntry{title: action.Title, fileTypes: action.FileTypes, devices: action.Devices})
	}

	return out
}

// writeNeeds lists what the extension needs from the server and what an
// operator approves at install.
func writeNeeds(body *strings.Builder, m manifest.Manifest) {
	var lines []string

	if m.Core != "" {
		lines = append(lines, fmt.Sprintf("a server release `%s`", m.Core))
	}

	var capabilities []string

	for _, required := range m.Requires {
		switch {
		case required.Capability != "":
			capabilities = append(capabilities, required.Capability)
		case required.Module != "":
			lines = append(lines, fmt.Sprintf("the module `%s` at `%s`", required.Module, required.Version))
		}
	}

	if len(capabilities) > 0 {
		lines = append(lines, "the server methods "+codeList(capabilities))
	}

	for _, suggested := range m.Suggests {
		lines = append(lines, fmt.Sprintf("works better beside `%s`: %s", suggested.Module,
			sentence(suggested.Reason)))
	}

	for _, conflict := range m.Conflicts {
		lines = append(lines, fmt.Sprintf("refuses to run beside `%s` at `%s`", conflict.Module, conflict.Version))
	}

	lines = append(lines, approvals(m)...)

	body.WriteString("\n## What it needs {#what-it-needs}\n\n")

	if len(lines) == 0 {
		body.WriteString("Nothing beyond the server. It asks for no permission of its own.\n")

		return
	}

	for _, line := range lines {
		body.WriteString("- " + line + "\n")
	}

	if len(m.Permissions) == 0 {
		body.WriteString("\nIt asks for no permission of its own. It acts only for the person who asks.\n")
	}
}

// approvals lists the requests an operator grants at install.
func approvals(m manifest.Manifest) []string {
	var lines []string

	if len(m.Permissions) > 0 {
		lines = append(lines, "the permissions "+codeList(m.Permissions)+", approved at install")
	}

	if len(m.Checks) > 0 {
		lines = append(lines, "to ask whether a person may "+codeList(m.Checks)+", approved at install")
	}

	for _, listen := range m.Needs.Listen {
		line := fmt.Sprintf("the %s port %d, `%s`, approved at install", strings.ToUpper(string(listen.Network)),
			listen.Port, listen.Name)
		if listen.Why != "" {
			line += ", for " + listen.Why
		}

		lines = append(lines, line)
	}

	if m.Needs.Data {
		lines = append(lines, "a data directory of its own, which an update and a removal keep")
	}

	if m.Needs.Database {
		lines = append(lines, "a database of its own, SQLite or Postgres, chosen at install")
	}

	if m.RecordsMiB > 0 {
		lines = append(lines, fmt.Sprintf("%d MiB for its records", m.RecordsMiB))
	}

	if m.ListPeople {
		lines = append(lines, "the list of the people on the server, with their names")
	}

	if m.CheckAbsentPeople {
		lines = append(lines, "asking what a person may do while that person is not using it")
	}

	if len(m.AllowedOrigins) > 0 {
		lines = append(lines, "pages of "+codeList(m.AllowedOrigins)+
			" to call the API from a browser while it is switched on, approved at install")
	}

	if len(m.Calls) > 0 {
		lines = append(lines, "calls to the routes of "+codeList(m.Calls))
	}

	if len(m.Routes.Public) > 0 {
		public := make([]string, 0, len(m.Routes.Public))
		for _, route := range m.Routes.Public {
			public = append(public, route.String())
		}

		lines = append(lines, "routes anybody may call without signing in: "+codeList(public))
	}

	if len(m.Routes.Staged) > 0 {
		staged := make([]string, 0, len(m.Routes.Staged))
		for _, route := range m.Routes.Staged {
			staged = append(staged, fmt.Sprintf("%s up to %d MiB", route.Route, route.MaxMiB))
		}

		lines = append(lines, "routes whose request body the server writes into its data directory, "+
			"with the largest body each asks for: "+codeList(staged))
	}

	if len(m.Routes.Headers) > 0 {
		headers := make([]string, 0, len(m.Routes.Headers))
		for _, header := range m.Routes.Headers {
			headers = append(headers, header.Name)
		}

		lines = append(lines, "request headers the server passes to its routes beyond the usual ones: "+
			codeList(headers))
	}

	return lines
}

func writeSettings(body *strings.Builder, options []manifest.Option) {
	if len(options) == 0 {
		return
	}

	body.WriteString("\n## Settings {#settings}\n\n")
	body.WriteString("An operator writes them under `config:` in the extension's entry of the configuration.\n\n")
	body.WriteString("| key | type | default | what it sets |\n|---|---|---|---|\n")

	for _, option := range options {
		def := "none"

		switch {
		case option.Required:
			def = "required"
		case option.Default != "":
			def = "`" + option.Default + "`"
		}

		fmt.Fprintf(body, "| `%s` | %s | %s | %s |\n", option.Key, option.Type, def, sentence(option.Summary))
	}
}

func writeEvents(body *strings.Builder, events map[string]string) {
	if len(events) == 0 {
		return
	}

	body.WriteString("\n## Events {#events}\n\n")

	names := make([]string, 0, len(events))
	for name := range events {
		names = append(names, name)
	}

	slices.Sort(names)

	for _, name := range names {
		fmt.Fprintf(body, "- `%s` publishes `%s`\n", name, events[name])
	}
}

func writeRoutes(body *strings.Builder, ext officialExtension) {
	if len(ext.Operations) == 0 {
		return
	}

	body.WriteString("\n## Routes {#routes}\n\n")
	fmt.Fprintf(body, "Every path is under `/api/v1/modules/%s`. The extension's `%s` describes each of them, "+
		"and the server serves it at `openapi.json` under the same path.\n\n", ext.Namespace, ext.Manifest.OpenAPI)
	body.WriteString("| method and path | what it does |\n|---|---|\n")

	for _, op := range ext.Operations {
		fmt.Fprintf(body, "| `%s %s` | %s |\n", strings.ToUpper(op.Method), op.Path, sentence(op.Summary))
	}
}
