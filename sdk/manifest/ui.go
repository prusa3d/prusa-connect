package manifest

import (
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"strings"
)

// UI is the browser half of a module: the bundle's entry file, the range of
// the library it compiled against, and what it contributes to the app.
type UI struct {
	// Entry is the bundle's entry file, relative to the bundle root, such as
	// ui/index.js. The host serves it under the module's namespace. It is
	// empty only for a bundle that offers themes and nothing else.
	Entry string
	// Library is the range of @prusa/connect-ui the bundle compiled
	// against, in the form ParseRange reads.
	Library string
	// Contributes is the pages and the sections the bundle may register. A
	// registration it did not declare is refused at run time.
	Contributes Contributes
	// Exports is the components this bundle offers other modules' bundles,
	// by name. A component's props are part of the module's published
	// interface and follow its version.
	Exports []UIExport
	// Imports is the components of other modules this bundle draws.
	Imports []UIImport
	// Themes is the stylesheets the bundle offers as a look for the whole
	// app. A person picks at most one.
	Themes []Theme
}

// UIExport is one component a bundle hands other bundles.
type UIExport struct {
	// Name is the component's name, one label, unique in the module.
	Name string
}

// UIImport is one component of another module a bundle draws.
type UIImport struct {
	// Module is the namespace of the module that exports the component.
	Module string
	// Name is the component's name in that module.
	Name string
}

// MaxUIExports is how many components one bundle may export, and how many it
// may import.
const MaxUIExports = 16

// Contributes is what a bundle adds to the app, one list per slot the slot
// table declares. Every list is optional, and a bundle that declares none
// draws nothing.
type Contributes struct {
	// Pages are whole screens the module adds, each with a route of its own.
	Pages []Page
	// Sections is a FormSection slot's entries by the slot's name, such as
	// SlotDeviceCard or SlotSettings. A slot with no entry is absent.
	Sections map[SlotName][]Section
	// Actions is a FormAction slot's entries by the slot's name. A slot with
	// no entry is absent.
	Actions map[SlotName][]Action
	// Cards are the cards a person may place on a dashboard.
	Cards []Card
}

// Page is one screen a module adds. The host builds the route from the
// module's namespace and this path, and draws the sidebar entry from the title
// and the icon.
type Page struct {
	// Path is the last segment of the route, one label with no slash.
	Path string
	// Title is what the sidebar entry and the page heading read.
	Title string
	// Icon is the name of an icon the library publishes, and empty for a page
	// that gets the default.
	Icon Icon
	// Hidden keeps the page out of the sidebar. The route still works.
	Hidden bool
}

// Section is one FormSection slot's entry, such as a device-card section or a
// settings pane.
type Section struct {
	// ID is the section's name within its list, one label with no slash.
	ID string
	// Title is what the section heading or the settings tab reads.
	Title string
	// FileTypes limits an entry of a file slot to files of these types. Each
	// passes ValidFileType. Empty is every file. Other slots take none.
	FileTypes []string
	// Devices limits an entry of a device slot to the devices it selects. The
	// zero value is every device. Other slots take none.
	Devices DeviceSelector
	// CrossOriginIsolated asks for the entry's page to be sent cross-origin
	// isolated, so a device page it frames can use SharedArrayBuffer. Only a
	// slot that allows it takes it.
	CrossOriginIsolated bool
}

// Action is one FormAction slot's entry: a run button.
type Action struct {
	// ID is the action's name within its list, one label with no slash.
	ID string
	// Title is what the button reads.
	Title string
	// Icon is the name of an icon the library publishes, and empty for the
	// default.
	Icon Icon
	// Destructive marks a run drawn as a destructive item. The host asks no
	// confirmation before it starts. The module confirms it if it needs to.
	Destructive bool
	// FileTypes limits an entry of a slot handed a file, as Section.FileTypes
	// does.
	FileTypes []string
	// Primary draws the action as a button on its subject's own page, where
	// the slot allows it. It stays a menu item everywhere else.
	Primary bool
	// Devices limits an entry of a slot handed a device, as Section.Devices
	// does.
	Devices DeviceSelector
}

// contributesKeys is the lists ui.contributes may have, one per slot.
func contributesKeys() []string {
	keys := make([]string, 0, len(slots))
	for _, slot := range slots {
		keys = append(keys, slot.Name.Key())
	}

	return keys
}

// The keys each block of the ui section may have.
//
//nolint:gochecknoglobals // closed tables read at parse, never written.
var (
	uiKeys      = []string{"entry", "library", "contributes", "exports", "imports", "themes"}
	pageKeys    = []string{"path", "title", "icon", "hidden"}
	sectionKeys = []string{"id", "title"}
	actionKeys  = []string{"id", "title", "icon", "destructive"}
)

// entryKeys is the keys one entry of a slot may have: its form's, plus
// file_types, devices, primary and cross_origin_isolated where the slot takes
// them.
func entryKeys(keys []string, slot Slot) []string {
	out := slices.Clone(keys)

	switch slot.Subject {
	case SubjectFile, SubjectPrinterFile:
		out = append(out, "file_types")
	case SubjectDevice:
		out = append(out, "devices")
	case SubjectNone, SubjectCamera, SubjectJob:
	}

	if slot.Primary {
		out = append(out, "primary")
	}

	if slot.CrossOriginIsolated {
		out = append(out, "cross_origin_isolated")
	}

	return out
}

// entrySuffix is what a bundle's entry file must end in. The app imports it
// as an ES module.
const entrySuffix = ".js"

// maxLabel is the longest DNS-1123 label. Every name in the ui block is one
// label, which is legal in a URL path and in an element attribute.
const maxLabel = 63

// readUI reads the ui block, and answers nil for a module that draws nothing.
func readUI(tree map[string]any) (*UI, error) {
	block, declared, err := node(tree, "", "ui")
	if err != nil || !declared {
		return nil, err
	}

	if err = refuseUnknown(block, "ui", uiKeys); err != nil {
		return nil, err
	}

	var held UI

	if held.Library, err = requiredText(block, "ui", "library"); err != nil {
		return nil, err
	}

	if _, err = ParseRange(held.Library); err != nil {
		return nil, fmt.Errorf("ui.library: %w", err)
	}

	if held.Contributes, err = readContributes(block); err != nil {
		return nil, err
	}

	if held.Exports, err = readExports(block); err != nil {
		return nil, err
	}

	if held.Imports, err = readImports(block); err != nil {
		return nil, err
	}

	if held.Themes, err = readThemes(block); err != nil {
		return nil, err
	}

	if held.Entry, err = readEntry(block, held); err != nil {
		return nil, err
	}

	return &held, nil
}

// readEntry reads ui.entry. A bundle that only offers themes runs no script,
// so it may leave the entry out. Every other bundle names one.
func readEntry(block map[string]any, held UI) (string, error) {
	themesOnly := len(held.Themes) > 0 && reflect.ValueOf(held.Contributes).IsZero() &&
		len(held.Exports) == 0 && len(held.Imports) == 0

	entry, err := text(block, "ui", "entry")
	if err != nil {
		return "", err
	}

	if entry == "" && themesOnly {
		return "", nil
	}

	if entry == "" {
		return "", fmt.Errorf("%w: ui.entry is empty", ErrInvalid)
	}

	if err = insideBundle("ui.entry", entry); err != nil {
		return "", err
	}

	if !strings.HasSuffix(entry, entrySuffix) {
		return "", fmt.Errorf("%w: ui.entry is %q and the app imports it as an ES module, so it ends in %s",
			ErrInvalid, entry, entrySuffix)
	}

	return entry, nil
}

// readExports reads ui.exports.
func readExports(block map[string]any) ([]UIExport, error) {
	entries, err := nodeList(block, "ui", "exports")
	if err != nil || entries == nil {
		return nil, err
	}

	if len(entries) > MaxUIExports {
		return nil, fmt.Errorf("%w: ui.exports names %d components, and the limit is %d",
			ErrInvalid, len(entries), MaxUIExports)
	}

	out := make([]UIExport, 0, len(entries))

	for index, entry := range entries {
		where := fmt.Sprintf("ui.exports[%d]", index)

		if err = refuseUnknown(entry, where, []string{"name"}); err != nil {
			return nil, err
		}

		var held UIExport

		if held.Name, err = label(entry, where, "name"); err != nil {
			return nil, err
		}

		if slices.ContainsFunc(out, func(o UIExport) bool { return o.Name == held.Name }) {
			return nil, fmt.Errorf("%w: ui.exports names %s twice", ErrInvalid, held.Name)
		}

		out = append(out, held)
	}

	return out, nil
}

// readImports reads ui.imports.
func readImports(block map[string]any) ([]UIImport, error) {
	entries, err := nodeList(block, "ui", "imports")
	if err != nil || entries == nil {
		return nil, err
	}

	if len(entries) > MaxUIExports {
		return nil, fmt.Errorf("%w: ui.imports names %d components, and the limit is %d",
			ErrInvalid, len(entries), MaxUIExports)
	}

	out := make([]UIImport, 0, len(entries))

	for index, entry := range entries {
		where := fmt.Sprintf("ui.imports[%d]", index)

		if err = refuseUnknown(entry, where, []string{"module", "name"}); err != nil {
			return nil, err
		}

		var held UIImport

		if held.Module, err = requiredText(entry, where, "module"); err != nil {
			return nil, err
		}

		if !validNamespace(held.Module) {
			return nil, fmt.Errorf("%w: %s.module is %q, expected a module namespace, such as "+
				"com.github.aux.acme.spools", ErrInvalid, where, held.Module)
		}

		if held.Name, err = label(entry, where, "name"); err != nil {
			return nil, err
		}

		if slices.Contains(out, held) {
			return nil, fmt.Errorf("%w: ui.imports names %s %s twice", ErrInvalid, held.Module, held.Name)
		}

		out = append(out, held)
	}

	return out, nil
}

// readContributes walks the slot table and reads each slot's list by its
// form.
func readContributes(block map[string]any) (Contributes, error) {
	declared, present, err := node(block, "ui", "contributes")
	if err != nil || !present {
		return Contributes{}, err
	}

	if err = refuseUnknown(declared, "ui.contributes", contributesKeys()); err != nil {
		return Contributes{}, err
	}

	var held Contributes

	for _, slot := range slots {
		switch slot.Form {
		case FormPage:
			held.Pages, err = readPages(declared)
		case FormSection:
			err = addSection(&held, declared, slot)
		case FormAction:
			err = addAction(&held, declared, slot)
		case FormCard:
			held.Cards, err = readCards(declared, slot.Name.Key())
		}

		if err != nil {
			return Contributes{}, err
		}
	}

	return held, nil
}

// addSection reads one FormSection slot's list into held.Sections. An absent
// or empty key leaves held untouched.
func addSection(held *Contributes, declared map[string]any, slot Slot) error {
	got, err := readSections(declared, slot)
	if err != nil || len(got) == 0 {
		return err
	}

	if held.Sections == nil {
		held.Sections = map[SlotName][]Section{}
	}

	held.Sections[slot.Name] = got

	return nil
}

// addAction reads one FormAction slot's list into held.Actions. An absent or
// empty key leaves held untouched.
func addAction(held *Contributes, declared map[string]any, slot Slot) error {
	got, err := readActions(declared, slot)
	if err != nil || len(got) == 0 {
		return err
	}

	if held.Actions == nil {
		held.Actions = map[SlotName][]Action{}
	}

	held.Actions[slot.Name] = got

	return nil
}

// readPages reads the pages list. Each path is one label, each title is not
// empty, and no path is written twice.
func readPages(declared map[string]any) ([]Page, error) {
	entries, err := nodeList(declared, "ui.contributes", "pages")
	if err != nil || entries == nil {
		return nil, err
	}

	out := make([]Page, 0, len(entries))
	taken := make([]string, 0, len(entries))

	for index, entry := range entries {
		where := fmt.Sprintf("ui.contributes.pages[%d]", index)

		if err = refuseUnknown(entry, where, pageKeys); err != nil {
			return nil, err
		}

		var page Page

		if page.Path, err = label(entry, where, "path"); err != nil {
			return nil, err
		}

		if page.Title, err = title(entry, where); err != nil {
			return nil, err
		}

		if page.Icon, err = readIcon(entry, where); err != nil {
			return nil, err
		}

		if page.Hidden, err = truth(entry, where, "hidden"); err != nil {
			return nil, err
		}

		if err = refuseRepeat(where+".path", append(taken, page.Path)); err != nil {
			return nil, err
		}

		taken = append(taken, page.Path)
		out = append(out, page)
	}

	return out, nil
}

// readSections reads one FormSection slot's id-and-title list.
func readSections(declared map[string]any, slot Slot) ([]Section, error) {
	key := slot.Name.Key()

	entries, err := nodeList(declared, "ui.contributes", key)
	if err != nil || entries == nil {
		return nil, err
	}

	out := make([]Section, 0, len(entries))
	taken := make([]string, 0, len(entries))

	for index, entry := range entries {
		where := fmt.Sprintf("ui.contributes.%s[%d]", key, index)

		if err = refuseUnknown(entry, where, entryKeys(sectionKeys, slot)); err != nil {
			return nil, err
		}

		var section Section

		if section.FileTypes, err = FileTypes(entry, where, "file_types"); err != nil {
			return nil, err
		}

		if section.Devices, err = readDevices(entry, where); err != nil {
			return nil, err
		}

		if section.CrossOriginIsolated, err = truth(entry, where, "cross_origin_isolated"); err != nil {
			return nil, err
		}

		if section.ID, err = label(entry, where, "id"); err != nil {
			return nil, err
		}

		if section.Title, err = title(entry, where); err != nil {
			return nil, err
		}

		if err = refuseRepeat(where+".id", append(taken, section.ID)); err != nil {
			return nil, err
		}

		taken = append(taken, section.ID)
		out = append(out, section)
	}

	return out, nil
}

// readActions reads one FormAction slot's list of run buttons: an id, a
// title, an optional icon and whether running the button is destructive.
func readActions(declared map[string]any, slot Slot) ([]Action, error) {
	key := slot.Name.Key()

	entries, err := nodeList(declared, "ui.contributes", key)
	if err != nil || entries == nil {
		return nil, err
	}

	out := make([]Action, 0, len(entries))
	taken := make([]string, 0, len(entries))

	for index, entry := range entries {
		where := fmt.Sprintf("ui.contributes.%s[%d]", key, index)

		if err = refuseUnknown(entry, where, entryKeys(actionKeys, slot)); err != nil {
			return nil, err
		}

		var action Action

		if action, err = readAction(entry, where); err != nil {
			return nil, err
		}

		if err = refuseRepeat(where+".id", append(taken, action.ID)); err != nil {
			return nil, err
		}

		taken = append(taken, action.ID)
		out = append(out, action)
	}

	return out, nil
}

// readAction reads the keys of one run button.
func readAction(entry map[string]any, where string) (Action, error) {
	var (
		action Action
		err    error
	)

	if action.FileTypes, err = FileTypes(entry, where, "file_types"); err != nil {
		return Action{}, err
	}

	if action.ID, err = label(entry, where, "id"); err != nil {
		return Action{}, err
	}

	if action.Title, err = title(entry, where); err != nil {
		return Action{}, err
	}

	if action.Icon, err = readIcon(entry, where); err != nil {
		return Action{}, err
	}

	if action.Destructive, err = truth(entry, where, "destructive"); err != nil {
		return Action{}, err
	}

	if action.Primary, err = truth(entry, where, "primary"); err != nil {
		return Action{}, err
	}

	if action.Devices, err = readDevices(entry, where); err != nil {
		return Action{}, err
	}

	return action, nil
}

// readIcon reads an optional icon. It must be in the list the app ships.
func readIcon(entry map[string]any, where string) (Icon, error) {
	value, err := text(entry, where, "icon")
	if err != nil {
		return "", err
	}

	if _, present := entry["icon"]; present && value == "" {
		return "", fmt.Errorf("%w: %s.icon is empty; drop the key for the default icon", ErrInvalid, where)
	}

	if value != "" && !slices.Contains(icons, Icon(value)) {
		names := make([]string, len(icons))
		for i, icon := range icons {
			names[i] = string(icon)
		}

		return "", fmt.Errorf("%w: %s.icon is %q, and the icons are %s",
			ErrInvalid, where, value, strings.Join(names, ", "))
	}

	return Icon(value), nil
}

// title reads the title key every contribution has.
func title(entry map[string]any, where string) (string, error) {
	return requiredText(entry, where, "title")
}

// label reads a name key. It must be one DNS-1123 label.
func label(entry map[string]any, where, key string) (string, error) {
	value, err := requiredText(entry, where, key)
	if err != nil {
		return "", err
	}

	if err = validLabel(at(where, key), value); err != nil {
		return "", err
	}

	return value, nil
}

// validLabel checks one DNS-1123 label of at most 63 characters. A slash is
// refused, so a page cannot claim an address below its own.
func validLabel(where, value string) error {
	if len(value) > maxLabel {
		return fmt.Errorf("%w: %s is %d characters and one label holds %d", ErrInvalid, where, len(value), maxLabel)
	}

	for index := range len(value) {
		char := value[index]

		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			continue
		case char == '-' && index > 0 && index < len(value)-1:
			continue
		default:
			return fmt.Errorf("%w: %s is %q, and one label is lowercase letters, digits and inner "+
				"hyphens with no slash", ErrInvalid, where, value)
		}
	}

	return nil
}

// insideBundle checks that a path names a file inside its bundle. It refuses a
// leading slash, a climbing element and a backslash.
func insideBundle(where, value string) error {
	switch {
	case strings.HasPrefix(value, "/"):
		return fmt.Errorf("%w: %s is %q, and a path inside the bundle starts with no slash", ErrInvalid, where, value)
	case strings.Contains(value, `\`):
		return fmt.Errorf("%w: %s is %q, and a path inside the bundle is written with forward slashes",
			ErrInvalid, where, value)
	case value == ".." || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") ||
		strings.HasSuffix(value, "/.."):
		return fmt.Errorf("%w: %s is %q, and a path inside the bundle cannot climb out of it", ErrInvalid, where, value)
	case !fs.ValidPath(value):
		return fmt.Errorf("%w: %s is %q, which does not name a file inside the bundle", ErrInvalid, where, value)
	default:
		return nil
	}
}
