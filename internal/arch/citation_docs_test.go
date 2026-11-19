package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode"
)

// A citation is a coordinate: `internal/transport/stream/hub.go:321`. A reader
// jumps to it. Earlier research measured what that coordinate is worth here: of
// 1,027 symbol-and-line citations in the internals chapters, 166 pointed at the
// wrong line after one day, and not one named a symbol or a file that had gone.
// The claim survived every time and the coordinate did not.
//
// So the gate is about the coordinate. `internal/arch/onboarding_docs_test.go`
// already refuses a path that does not exist and `make docs-check` already
// refuses a link that does not resolve. Neither of them opens the file.
//
// The rule this holds is containment, not distance: the cited line must fall
// inside the declaration of the symbol the page names, counting the doc comment
// above it. That is exactly the promise a citation makes. A reader who lands on
// the doc comment of `Publish` has arrived; a reader who lands three lines above
// it, in the tail of the function before, has been sent to the wrong place, and
// three lines is not a smaller lie than thirty. Nothing here is measured in
// lines, so there is no tolerance to argue about.
//
// Three shapes carry a coordinate here, and all three are held by the same
// rule. A table row, `| symbol | line |`, whose file is the heading above it.
// A sentence, "`Publish` (`hub.go:321`)", which carries its own file. And the
// bare-line form, "`PrinterResult` (113)", written in the `symbols:` list under
// a file heading and in the prose below it, which carries a line number and no
// file at all. The bare-line form is the largest of the three.
//
// A citation naming a symbol its file does not declare is a wrong citation, and
// all three shapes report it by name. The bare-line form is the one that has to
// be made to say so. A bare number carries no file, so a group the heading's
// file declares nothing of is either a citation to a symbol that has gone or a
// code span that was never a symbol, and both are one identifier in backticks
// in front of a number.
//
// Discarding both is the cheap way to tell them apart, and it costs the whole
// check. A rename then takes its citations out of the population instead of
// turning them red. The only signal left is the bare-line floor, which reports
// that the chapter cites too few lines and names none of them. A reader of that
// failure goes looking for deleted prose. One rename put sixteen citations in
// internals/transport-and-the-stream.md into that state.
//
// So the shapes that are not citations are excused by name, in
// aSymbolThisFileCouldDeclare, and everything else the file does not declare is
// reported with its file, its symbol and the line it is written on. What each
// excuse costs is written on that function.
//
// Seven things it cannot catch, written down so the next person does not have to
// infer them from the code:
//
//   - a citation that drifts inside the body of the symbol it names. The span
//     runs to the closing brace, so a line that moves from the top of a long
//     function to the middle of it still lands on that function
//   - a row whose symbol column is prose, such as "the bounds block" or "the
//     eight `ErrNo...` sentinels". There is no identifier to resolve, so only
//     the file and the line bound are checked. The floor is what stops a page
//     converting its rows into that shape
//   - the third column. A page may describe a symbol wrongly at the right line,
//     and nothing here reads prose
//   - a heading naming two files. A symbol found in either satisfies the row,
//     so a citation swapped between the two is missed
//   - a file that is not Go. Only its line count is checked, and a bare-line
//     citation under such a heading names no symbol this gate can resolve, so
//     TestEveryBareLineCitationNamesALineItsFileHas is all that holds it
//   - a bare-line citation in a `failures:` paragraph. There a number after a
//     symbol is the HTTP status the refusal becomes, "`ErrUnknownCode` (404)",
//     so the whole paragraph is skipped and a real coordinate written in one
//     would be skipped with it
//   - a bare-line citation whose section heading names no file. Nothing binds
//     its numbers to a file, so the group is not read.
//     TestEverySymbolListIsUnderAFileHeading refuses the `symbols:` list in
//     that position, which is where the chapters write most of them
const citationDirs = "docs/src/content/docs"

// citedPages are the pages whose citations this gate reads, and the number of
// checkable symbol-and-line citations each carried when the gate was written.
//
// The floor is what stops the gate passing vacuously. A chapter rewritten in
// generalities, or one whose tables lose the symbol column, cites nothing and
// would otherwise leave a green test over a page that promises nothing
// checkable. Seven gates in this repository have reported green over nothing.
// A page that legitimately sheds citations has its number edited in the same
// change.
//
// A floor of zero is refused, because zero is what a page that stopped citing
// anything scores. A page that really cites nothing goes in pagesCitingNothing
// below, where saying so is a claim the gate checks rather than a blank.
//
// bare is the same floor for the bare-line form, counted on its own so that
// adding that form cannot be satisfied by the citations the gate already held.
// Zero is a claim there rather than a blank: a page listed with zero is checked
// to write none, the way pagesCitingNothing is checked, and a page that starts
// writing them fails until its number is edited in.
//
//nolint:gochecknoglobals // a table five tests read; there is nowhere cheaper.
var citedPages = []struct {
	file  string
	cites int
	bare  int
}{
	{file: "onboarding/registries-and-names.md", cites: 7, bare: 0},
	{file: "onboarding/the-data-flow.md", cites: 5, bare: 0},
	{file: "onboarding/the-entry-point.md", cites: 21, bare: 0},
	{file: "onboarding/the-model.md", cites: 6, bare: 0},
	{file: "onboarding/the-sdk.md", cites: 3, bare: 0},
	{file: "onboarding/when-things-go-wrong.md", cites: 14, bare: 0},
}

// pagesCitingNothing name no symbol at any line.
//
// The two onboarding pages here are maps of the tree: they say where a thing
// lives and never which line it is on, so there is no coordinate to rot. The
// paths they do name are held by onboarding_docs_test.go.
//
// The rest are the published pages outside `internals/` and `onboarding/`.
// Every one of them was measured at zero when this list was widened, so none
// enters with a floor. They are listed rather than skipped because the value
// is the refusal: a reader-facing page that starts writing line numbers fails
// here until somebody gives it a floor, instead of rotting unread.
//
// This list is checked, not trusted. A page here that starts citing code fails
// TestAPageCitingNothingSaysSo and moves to citedPages with a floor.
//
//nolint:gochecknoglobals // read beside citedPages, for the same reason.
var pagesCitingNothing = []string{
	// The design decisions name a folder at most, never a line.
	"design/index.md",
	"design/the-core-knows-devices.md",
	"design/features-are-modules.md",
	"design/the-sdk-is-the-contract.md",
	"design/a-driver-declares.md",
	"design/one-registry.md",
	"design/permissions.md",
	"design/events.md",
	"design/no-history-in-the-core.md",
	"design/api-compatibility.md",
	"design/modules-ask.md",
	// The Fractal link names packages and files and never a line: the code it
	// describes is the first task of ADR-20260921-wkf and moves with every task after.
	"internals/the-fractal-link.md",
	// The extension host names a package, a file or a function and never a
	// line, so a moved declaration leaves the page correct.
	"internals/the-extension-host.md",
	// The application core names a file or a function and never a line.
	"internals/the-application-core.md",
	// Transport and the stream names a file or a function and never a line.
	"internals/transport-and-the-stream.md",
	// The authorization graph names a file or a function and never a line.
	"internals/authorization.md",
	// Printers and drivers names a file or a function and never a line.
	"internals/printers-and-drivers.md",
	// The sdk and extensions names a file or a function and never a line.
	"internals/the-sdk-and-extensions.md",
	// Build, gates and testing names a target, a script or a test and never a
	// line.
	"internals/build-gates-and-testing.md",
	"architecture.md",
	"authorization-vocabulary.md",
	"bench-preflight.md",
	"capabilities.md",
	"certificates.md",
	"config.md",
	// The configuration pages: prose around the key tables cmd/generate-config-reference
	// writes from the reference. Neither names a line.
	"config/database.md",
	"config/discovery.md",
	"config/encryption.md",
	"config/extensions.md",
	"config/logging.md",
	"config/network.md",
	"config/printers.md",
	"config/resources.md",
	"config/server.md",
	"config/sign-in.md",
	"config/web-app.md",
	"connector-contract.md",
	"container.md",
	"creating-connectors.md",
	"dashboards.md",
	"demo-walkthrough.md",
	"developer-view.md",
	"device-issues.md",
	"device-models.md",
	"emulator.md",
	"error-codes.md",
	"event-types.md",
	"extending/build-time-modules.md",
	"extension-concepts.md",
	"extensions.md",
	"getting-started.md",
	"glossary.md",
	"graphql.md",
	"home-assistant.md",
	"human-auth.md",
	"index.mdx",
	"install-from-a-release.md",
	"kubernetes.md",
	"local-accounts.md",
	"logging.md",
	"metrics.md",
	"mqtt-asyncapi.mdx",
	"mqtt-overview.md",
	"nas.md",
	"onboarding/index.md",
	"onboarding/where-things-live.md",
	"plugin-architecture.md",
	"plugin-wire-protocol.md",
	"printer-configuration.md",
	"prusa-command-architecture.md",
	"prusa-xbuddy-websocket-protocol.md",
	"scoped-credentials-and-app-pairing.md",
	// The reference pages of ADR-20260910-he3: a person's prose around a region the
	// generator writes from the OpenAPI document or the sdk doc comments.
	// Neither source cites a line, and the prose is held to the run and
	// extend tiers, which name no line either.
	"reference/api/access.md",
	"reference/api/auth.md",
	"reference/api/cameras.md",
	"reference/api/dashboards.md",
	"reference/api/developer.md",
	"reference/api/discovery.md",
	"reference/api/devices.md",
	"reference/api/events.md",
	"reference/api/grants.md",
	"reference/api/history.md",
	"reference/api/registrations.md",
	"reference/api/resources.md",
	"reference/api/storages.md",
	"reference/api/system.md",
	"reference/api/vocabularies.md",
	"reference/beta-portal-machine-api.md",
	"reference/go-sdk/authapi-authapitest.md",
	"reference/go-sdk/authapi-oidctoken.md",
	"reference/go-sdk/authapi.md",
	"reference/go-sdk/busapi.md",
	"reference/go-sdk/cameraapi-cameraapitest.md",
	"reference/go-sdk/cameraapi.md",
	"reference/go-sdk/connectorapi-connectorapitest.md",
	"reference/go-sdk/connectorapi.md",
	"reference/go-sdk/deviceapi.md",
	"reference/go-sdk/event.md",
	"reference/go-sdk/eventsapi-eventsapitest.md",
	"reference/go-sdk/eventsapi.md",
	"reference/go-sdk/extension.md",
	"reference/go-sdk/fault-faulttest.md",
	"reference/go-sdk/fault.md",
	"reference/go-sdk/fractal-files.md",
	"reference/go-sdk/fractal-fractaltest.md",
	"reference/go-sdk/fractal-join.md",
	"reference/go-sdk/fractal-node.md",
	"reference/go-sdk/fractal-prusa.md",
	"reference/go-sdk/fractal-schema.md",
	"reference/go-sdk/fractal-shadow.md",
	"reference/go-sdk/fractal.md",
	"reference/go-sdk/fileapi.md",
	"reference/go-sdk/historyapi.md",
	"reference/go-sdk/linkapi.md",
	"reference/go-sdk/manifest.md",
	"reference/go-sdk/moduledb.md",
	"reference/go-sdk/printcompat.md",
	"reference/go-sdk/routesapi.md",
	"reference/go-sdk/scriptapi.md",
	"reference/go-sdk/stateapi.md",
	"reference/go-sdk/storageapi-storageapitest.md",
	"reference/go-sdk/accessapi.md",
	"reference/go-sdk/diagnosticsapi.md",
	"reference/go-sdk/storageapi.md",
	"reference/go-sdk/transferapi.md",
	"reference/go-sdk/uiapi.md",
	"reference/go-sdk/vocabulary.md",
	"reference/go-sdk/wire-wiretest.md",
	"reference/go-sdk/wire.md",
	"reference/go-sdk/wirerpc.md",
	// The official extension pages: prose around a region the generator
	// writes from each extension's module.yaml and openapi.json.
	"reference/extensions/buddy-debug-logs.md",
	"reference/extensions/easyprint.md",
	"reference/extensions/fleet-stats.md",
	"reference/extensions/index.md",
	"reference/extensions/materials.md",
	"reference/extensions/parts.md",
	"reference/extensions/prusaslicer2-connector.md",
	"reference/extensions/prusaslicer3-connector.md",
	"reference/extensions/server-slicing.md",
	"reference/extensions/simple-queue.md",
	"reference/extensions/threemf.md",
	"reference/module-slots.md",
	"resource-kinds.md",
	"sse-asyncapi.mdx",
	"sse-overview.md",
	"storage.md",
	"styling-a-module.md",
	"tracing.md",
	"upgrade-back-up-and-uninstall.md",
	"using-the-api-from-a-page.md",
	"writing-a-driver.md",
	"writing-a-module.md",
	"writing-your-first-module.md",
}

// everyCitedPage is both lists as one, for the two checks that read a page
// whether or not it is expected to cite anything.
func everyCitedPage() []string {
	pages := make([]string, 0, len(citedPages)+len(pagesCitingNothing))
	for _, page := range citedPages {
		pages = append(pages, page.file)
	}

	return append(pages, pagesCitingNothing...)
}

// citedFile matches a repository path with a file extension, optionally
// followed by a line reference. The leading boundary is a character that cannot
// appear in a path, so a path in prose, in a table cell, in a code span or in a
// mermaid node is all found the same way.
var citedFile = regexp.MustCompile(
	`(?:^|[^A-Za-z0-9_./-])((?:[a-z][a-z0-9._-]*/)+[A-Za-z0-9_.-]+\.[a-z]+)(?::(\d+))?`)

// backtickSpan matches one code span. It reads the symbol column of a citation
// table, and it reads a line of prose looking for the symbol beside a citation.
var backtickSpan = regexp.MustCompile("`([^`\n]+)`")

// spanIsCitation matches the content of a code span that is itself a citation,
// `internal/transport/stream/hub.go:321`.
var spanIsCitation = regexp.MustCompile(
	`^((?:[a-z][a-z0-9._-]*/)+[A-Za-z0-9_.-]+\.[a-z]+):(\d+)$`)

// inlineReach is how far, in characters, a code span may sit from a citation
// and still be read as the symbol that citation is about. The chapters write
// "`Sym` (`file:12`)" and "`file:12` (`Sym`)", and the widest separator in
// either direction is "is a mechanical function of the code (".
const inlineReach = 40

// goIdentifier is what a symbol must look like after its type parameters are
// stripped: a name, or a name qualified by the type that declares it. It
// rejects an import path, an error code, an ellipsis and a sentence.
var goIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// citationTableHeader is the row that opens a citation table. Reading the
// header rather than guessing from the cell contents is what keeps a table
// whose second column is an HTTP status out of the population.
const citationTableHeader = "| symbol | line |"

// symbolListPrefix opens the bare-line list a file heading carries: "symbols:
// `DeviceFleet` (29), `Printer` (57)". A section may write the same form in
// its prose, and those are read too; this constant is only what
// TestEverySymbolListIsUnderAFileHeading looks for.
const symbolListPrefix = "symbols:"

// failuresParagraph opens the one paragraph in a file section where a number in
// brackets after a symbol is not a line. "`ErrUnknownCode` (404)" and
// "`httpfault.FieldImmutable` (422)" are the statuses those refusals become,
// and the chapters write five of them.
const failuresParagraph = "failures:"

// bareLineTarget matches the line number a bare-line citation carries, "(113)",
// and the paired list a group of them carries, "(18, 19)". It is anchored at
// the end of a code span and allows one line break, because these chapters are
// hard wrapped and write "`Limit`\n(55)".
//
// Every part has to be a number. That is what keeps "`naming` (six model-name
// spellings)" and "`models` (13 entries, one per model)" out of the population.
var bareLineTarget = regexp.MustCompile(`^[ \t]*\n?[ \t]*\((\d+(?:\s*,\s*\d+)*)\)`)

// citationForm is which of the three shapes a citation was written in. The
// floor counts the bare-line form on its own, so a chapter cannot satisfy the
// bare-line floor with citations that already carry their file.
type citationForm int

const (
	formCarriesItsFile citationForm = iota
	formBareLine
	// formBareLineOutsideGo is the bare-line form under a heading naming no Go
	// file: a key in adapters/tags.json, an identifier in a scaffold template.
	// There is no declaration index to resolve a name against, so the symbol
	// rule has nothing to say and only the line bound holds it.
	formBareLineOutsideGo
)

// TestEveryCitedLineHoldsItsSymbol is the gate. Every citation that names a
// symbol lands inside that symbol's declaration.
func TestEveryCitedLineHoldsItsSymbol(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	index := newSourceIndex(root)

	// The per-page floors below hold what one page cites. This holds the two
	// lists themselves: emptied, every citation test here reports green having
	// read no page at all. The number rises when a chapter is added and is
	// edited only when one is removed.
	requireScanned(t, "pages in citedPages or pagesCitingNothing", len(everyCitedPage()), 15)

	var violations []string

	for _, page := range everyCitedPage() {
		for _, cite := range citationsIn(t, index, page) {
			violations = append(violations, cite.check(t, index)...)
		}
	}

	reportCitations(t, violations)
}

// TestEveryCitedLineExists is the wider and weaker half: every `path:N` on
// these pages, symbol or no symbol, names a line the file has. It catches a
// citation past the end of a file that shrank, which the symbol check cannot
// see because there is no symbol to look for.
func TestEveryCitedLineExists(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	index := newSourceIndex(root)

	var violations []string

	for _, page := range everyCitedPage() {
		body := citedPage(t, root, page)

		for number, line := range strings.Split(body, "\n") {
			for _, match := range citedFile.FindAllStringSubmatch(line, -1) {
				if match[2] == "" {
					continue
				}

				source, err := index.read(match[1])
				if err != nil {
					continue // the path check in onboarding_docs_test.go owns this.
				}

				target, convErr := strconv.Atoi(match[2])
				if convErr != nil || target < 1 || target > source.lines {
					violations = append(violations, fmt.Sprintf(
						"%s/%s:%d cites %s:%s, and that file has %d lines",
						citationDirs, page, number+1, match[1], match[2], source.lines))
				}
			}
		}
	}

	reportCitations(t, violations)
}

// TestEveryBareLineCitationNamesALineItsFileHas is the bare-line form's half of
// TestEveryCitedLineExists. That test reads the `path:N` shape, so it never sees
// a number written under a file heading with no path beside it, and nothing
// else bounds those.
//
// It is what holds the two shapes the symbol rule excuses. A heading naming no
// Go file has no declarations to resolve a name against, and a name Go itself
// owns is not a symbol of anybody's file. In both cases the number is still a
// coordinate and the file still has a length, which is the check that stays
// available. It is a weaker rule than containment and it is not nothing: a
// chapter naming line 73 of a 32-line file is wrong whatever the symbol is.
func TestEveryBareLineCitationNamesALineItsFileHas(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	index := newSourceIndex(root)

	var violations []string

	read := 0

	for _, page := range everyCitedPage() {
		for _, cite := range citationsIn(t, index, page) {
			if cite.form == formCarriesItsFile {
				continue
			}

			read++

			violations = append(violations, cite.outOfBounds(index)...)
		}
	}

	// The chapters listed with a bare floor in citedPages write this form, and
	// the floor here is the sum of theirs. An empty read is then a paragraph
	// walk that does not reach them. When no chapter lists a bare floor there
	// is nothing to read, and TestEveryChapterStillCitesItsBareLines checks
	// that each of them writes none.
	expected := 0
	for _, page := range citedPages {
		expected += page.bare
	}

	if expected > 0 {
		requireScanned(t, "bare-line citations", read, expected,
			"Either a chapter stopped writing `Symbol` (line) or bareLineCitations stopped finding it.")
	}

	reportCitations(t, violations)
}

// outOfBounds names every line this citation cites that no file it names has.
func (c citation) outOfBounds(index *sourceIndex) []string {
	var violations []string

	for _, target := range c.targets {
		within := false
		length := 0

		for _, file := range c.files {
			source, err := index.read(file)
			if err != nil {
				continue // the path check in onboarding_docs_test.go owns this.
			}

			length = source.lines
			within = within || (target >= 1 && target <= source.lines)
		}

		if length > 0 && !within {
			violations = append(violations, fmt.Sprintf(
				"%s/%s:%d puts %s at line %d of %s, and that file has %d lines",
				citationDirs, c.page, c.docLine, strings.Join(c.symbols, ", "),
				target, strings.Join(c.files, " or "), length))
		}
	}

	return violations
}

// TestEveryChapterStillCitesItsCode is the floor. A page that stopped citing
// code stopped making the claim this gate checks, and the gate above would pass
// on it without noticing.
func TestEveryChapterStillCitesItsCode(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	index := newSourceIndex(root)

	var violations []string

	for _, page := range citedPages {
		if page.cites < 1 {
			violations = append(violations, fmt.Sprintf(
				"%s/%s is listed with a floor of %d, and a floor of zero checks nothing. "+
					"A page that cites no code belongs in pagesCitingNothing",
				citationDirs, page.file, page.cites))

			continue
		}

		if checked, _ := checkableOn(t, index, page.file); checked < page.cites {
			violations = append(violations, fmt.Sprintf(
				"%s/%s makes %d checkable symbol-and-line citations, and this gate expects at least %d",
				citationDirs, page.file, checked, page.cites))
		}
	}

	reportCitations(t, violations)
}

// TestEveryChapterStillCitesItsBareLines is the floor under the bare-line form.
// It is counted apart from the floor above, because a chapter that dropped
// every "`Sym` (12)" it writes would otherwise still clear that one on its
// tables alone.
//
// A page listed with zero is checked to write none. Zero as a floor checks
// nothing, so it is read here as the claim that the page writes this form
// nowhere, and a page that starts writing it fails until its number is edited
// in.
func TestEveryChapterStillCitesItsBareLines(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	index := newSourceIndex(root)

	var violations []string

	for _, page := range citedPages {
		_, bare := checkableOn(t, index, page.file)

		if page.bare == 0 && bare > 0 {
			violations = append(violations, fmt.Sprintf(
				"%s/%s is listed as writing no `Symbol` (line) citations and writes %d. "+
					"Put that number in its bare field",
				citationDirs, page.file, bare))

			continue
		}

		if page.bare > 0 && bare < page.bare {
			violations = append(violations, fmt.Sprintf(
				"%s/%s makes %d checkable `Symbol` (line) citations, and this gate expects at least %d",
				citationDirs, page.file, bare, page.bare))
		}
	}

	reportCitations(t, violations)
}

// TestAPageCitingNothingSaysSo is the other half of the floor. A page listed as
// citing nothing is checked to really cite nothing, so the list is a claim and
// not a way past the floor. A page that starts citing code moves to citedPages
// and gets a number.
func TestAPageCitingNothingSaysSo(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	index := newSourceIndex(root)

	var violations []string

	for _, page := range pagesCitingNothing {
		checked, bare := checkableOn(t, index, page)
		if checked+bare > 0 {
			violations = append(violations, fmt.Sprintf(
				"%s/%s is listed as citing no code and makes %d checkable symbol-and-line citations. "+
					"Move it to citedPages with that number as its floor",
				citationDirs, page, checked+bare))
		}
	}

	reportCitations(t, violations)
}

// checkableOn counts the citations on one page that resolve far enough to be
// held by TestEveryCitedLineHoldsItsSymbol, split by form: those that carry
// their own file, then the bare-line ones that take it from a heading.
func checkableOn(t *testing.T, index *sourceIndex, page string) (int, int) {
	t.Helper()

	checked, bare := 0, 0

	for _, cite := range citationsIn(t, index, page) {
		if !cite.checkable(index) {
			continue
		}

		if cite.form == formBareLine {
			bare++

			continue
		}

		checked++
	}

	return checked, bare
}

// TestEveryCitationTableIsUnderAFileHeading refuses a citation table that sits
// under a heading naming no file. Its line numbers have nothing to be resolved
// against, so every row in it would pass without being read.
func TestEveryCitationTableIsUnderAFileHeading(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var violations []string

	for _, page := range everyCitedPage() {
		body := citedPage(t, root, page)

		var heading string

		var files []string

		for number, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "#") {
				heading, files = trimmed, headingFiles(root, trimmed)
			}

			if strings.HasPrefix(strings.ToLower(trimmed), citationTableHeader) && len(files) == 0 {
				violations = append(violations, fmt.Sprintf(
					"%s/%s:%d opens a symbol-and-line table under %q, which names no file its lines belong to",
					citationDirs, page, number+1, heading))
			}
		}
	}

	reportCitations(t, violations)
}

// TestEverySymbolListIsUnderAFileHeading refuses a `symbols:` list that sits
// under a heading naming no file. Its numbers are bare line numbers, so with no
// file to resolve them against every one of them would pass unread.
func TestEverySymbolListIsUnderAFileHeading(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var violations []string

	for _, page := range everyCitedPage() {
		body := citedPage(t, root, page)

		var heading string

		var files []string

		for number, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "#") {
				heading, files = trimmed, headingFiles(root, trimmed)
			}

			if strings.HasPrefix(strings.ToLower(trimmed), symbolListPrefix) && len(files) == 0 {
				violations = append(violations, fmt.Sprintf(
					"%s/%s:%d opens a symbol list under %q, which names no file its lines belong to",
					citationDirs, page, number+1, heading))
			}
		}
	}

	reportCitations(t, violations)
}

// TestEveryCitedPageIsListed refuses a page added to either directory and named
// in neither list, which would otherwise be a page nothing checks.
func TestEveryCitedPageIsListed(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	listed := map[string]struct{}{}
	for _, page := range citedPages {
		listed[page.file] = struct{}{}
	}

	for _, page := range pagesCitingNothing {
		listed[page] = struct{}{}
	}

	var violations []string

	// The walk is the whole content directory and not the two chapter
	// directories alone. A reader-facing page writes line numbers as readily as
	// a chapter does, and 39 of them sit outside the two.
	pages := publishedPages(t, root)

	for _, page := range pages {
		if _, ok := listed[page]; !ok {
			violations = append(violations, fmt.Sprintf(
				"%s/%s is in neither citedPages nor pagesCitingNothing, so nothing checks the citations it makes",
				citationDirs, page))
		}
	}

	// A directory walk that matched no markdown file finds no unlisted page and
	// reports green, which is the state this whole check exists to refuse.
	requireScanned(t, "markdown files under "+citationDirs, len(pages), 15)

	reportCitations(t, violations)
}

// citation is one claim of the form "symbol S is at line L of file F". A row
// may name several symbols and several lines, and a page names two files where
// one heading covers a pair.
type citation struct {
	page    string
	docLine int
	files   []string
	symbols []string
	targets []int
	form    citationForm
}

// checkable reports whether this citation resolves far enough to be counted by
// the floor: at least one named symbol that at least one named file declares.
func (c citation) checkable(index *sourceIndex) bool {
	for _, file := range c.files {
		source, err := index.read(file)
		if err != nil {
			continue
		}

		for _, symbol := range c.symbols {
			if len(source.spans[symbol]) > 0 {
				return true
			}
		}
	}

	return false
}

// check runs the rule.
//
// Every symbol the row names is declared by one of the files the heading names.
// Then the coordinates have to match up, and how they are matched depends on
// whether the row names as many symbols as it cites lines.
//
// As many of each, and they are read in pairs: the first symbol is at the first
// line, the second at the second. That is "`eventStreamBuffer`,
// `defaultHeartbeat`, `defaultRetry` | 40, 45, 51", three members of one const
// block, each with its own coordinate. Pairing is the only rule that catches
// this row when the block moves, because all three members share the block's
// span, so any one correct line would otherwise vouch for the other two.
//
// A different count of each, and the row is right if either holds:
//
//   - every line it cites lands inside a symbol it names. That is "`reserve` /
//     `release` / `releaseLocked` | 666", three symbols at one line, where the
//     line belongs to the first of them
//   - every symbol it names is at a line it cites. That is "`Type` and the nine
//     constants | 6, 10", where 6 is the type and 10 is a block the cell
//     describes in prose and never names
//
// Requiring both there would fail the second shape, whose claim is true.
// Requiring neither would accept a row where nothing lines up.
func (c citation) check(t *testing.T, index *sourceIndex) []string {
	t.Helper()

	// A heading naming no Go file has no declarations to resolve against, so
	// every name under it would be reported as missing and none of them would
	// be wrong. TestEveryBareLineCitationNamesALineItsFileHas holds these.
	if c.form == formBareLineOutsideGo {
		return nil
	}

	var violations []string

	declared := map[string][]span{}

	for _, file := range c.files {
		source, err := index.read(file)
		if err != nil {
			continue
		}

		for _, symbol := range c.symbols {
			declared[symbol] = append(declared[symbol], source.spans[symbol]...)
		}
	}

	resolved := 0

	for _, symbol := range c.symbols {
		if len(declared[symbol]) == 0 {
			violations = append(violations, fmt.Sprintf(
				"%s/%s:%d names %s, which %s does not declare",
				citationDirs, c.page, c.docLine, symbol, strings.Join(c.files, " or ")))

			continue
		}

		resolved++
	}

	// Where nothing the citation names is there, the line it cites has nothing
	// to be measured against, and "no named symbol resolves" repeats what the
	// line above already said with the symbol's name in it. A citation where
	// some of the names resolve keeps both reports: the missing name, and where
	// the ones that are there actually sit.
	if resolved == 0 || c.matches(declared) {
		return violations
	}

	targets := make([]string, 0, len(c.targets))
	for _, target := range c.targets {
		targets = append(targets, strconv.Itoa(target))
	}

	return append(violations, fmt.Sprintf("%s/%s:%d puts %s at %s:%s, and %s",
		citationDirs, c.page, c.docLine, strings.Join(c.symbols, ", "),
		c.files[0], strings.Join(targets, ", "), whereItIs(declared)))
}

// matches runs the rule the comment on check describes.
func (c citation) matches(declared map[string][]span) bool {
	if len(c.symbols) == len(c.targets) {
		for index, symbol := range c.symbols {
			if !holds(declared[symbol], c.targets[index]) {
				return false
			}
		}

		return true
	}

	return everyLineLands(declared, c.targets) || everySymbolIsCited(declared, c.targets)
}

// holds reports whether a line falls inside any of one symbol's declarations.
func holds(spans []span, target int) bool {
	for _, s := range spans {
		if target >= s.from && target <= s.to {
			return true
		}
	}

	return false
}

// everyLineLands reports whether each cited line falls inside a declaration the
// row names.
func everyLineLands(declared map[string][]span, targets []int) bool {
	for _, target := range targets {
		found := false

		for _, spans := range declared {
			found = found || holds(spans, target)
		}

		if !found {
			return false
		}
	}

	return true
}

// everySymbolIsCited reports whether each declaration the row names holds one of
// the lines it cites.
func everySymbolIsCited(declared map[string][]span, targets []int) bool {
	if len(declared) == 0 {
		return false
	}

	for _, spans := range declared {
		found := false

		for _, target := range targets {
			found = found || holds(spans, target)
		}

		if !found {
			return false
		}
	}

	return true
}

// whereItIs names the line to repair the citation to, because a gate that only
// says "wrong" leaves the work of finding the truth to a person, and an earlier
// study measured 166 of these at once.
func whereItIs(declared map[string][]span) string {
	var where []string

	for symbol, spans := range declared {
		for _, s := range spans {
			where = append(where, fmt.Sprintf("%s is at %d", symbol, s.declaredAt))
		}
	}

	if len(where) == 0 {
		return "no named symbol resolves"
	}

	sort.Strings(where)

	return strings.Join(where, ", ")
}

// citationsIn reads one page and returns every citation on it: the rows of its
// symbol-and-line tables, the inline `Sym` (`file:N`) form its prose and its
// diagrams use, and the bare-line `Sym` (N) form written under a file heading.
func citationsIn(t *testing.T, index *sourceIndex, page string) []citation {
	t.Helper()

	root := index.root
	body := citedPage(t, root, page)

	var found []citation

	var heading []string

	inTable := false

	for number, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)

		if strings.HasPrefix(line, "#") {
			heading, inTable = headingFiles(root, line), false
		}

		if strings.HasPrefix(strings.ToLower(line), citationTableHeader) {
			inTable = true

			continue
		}

		if inTable && !strings.HasPrefix(line, "|") {
			inTable = false
		}

		if inTable && len(heading) > 0 {
			if cite, ok := tableCitation(line, heading, page, number+1); ok {
				found = append(found, cite)
			}
		}
	}

	found = append(found, bareLineCitations(index, root, body, page)...)

	return append(found, inlineCitations(index, body, page)...)
}

// bareLineCitations reads the form whose file comes from the heading above it
// rather than from the citation itself, "`PrinterResult` (113)". It is what the
// `symbols:` and `tests:` lists under a file heading are made of, and the
// sections write it in their prose too.
//
// It works a paragraph at a time. A paragraph is one claim about one file, and
// these chapters are hard wrapped, so "`Limit`\n(55)" needs more than a line
// and less than a section.
//
// A paragraph led by `failures:` is skipped. There, and only there, the number
// after a symbol is the HTTP status the refusal becomes: "`ErrUnknownCode`
// (404)". Five of those are written across the chapters and none of them is a
// coordinate.
func bareLineCitations(index *sourceIndex, root, body, page string) []citation {
	var found []citation

	var files, paragraph []string

	paragraphAt := 0

	flush := func() {
		if len(files) > 0 && len(paragraph) > 0 &&
			!strings.HasPrefix(strings.ToLower(paragraph[0]), failuresParagraph) {
			found = append(found, bareLinesInParagraph(
				index, strings.Join(paragraph, "\n"), files, page, paragraphAt)...)
		}

		paragraph = nil
	}

	for number, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)

		if line == "" || strings.HasPrefix(line, "#") {
			flush()
		}

		if strings.HasPrefix(line, "#") {
			files = headingFiles(root, line)

			continue
		}

		if line == "" {
			continue
		}

		if len(paragraph) == 0 {
			paragraphAt = number + 1
		}

		paragraph = append(paragraph, line)
	}

	flush()

	return found
}

// bareLinesInParagraph reads one paragraph of one file's section.
//
// Consecutive code spans separated by nothing but whitespace and commas are one
// group, so "`boolTrue`, `boolFalse` (18, 19)" is two symbols paired with two
// lines and not two claims about the second. Any other text between two spans
// starts a new group, because a sentence that puts words between them is not
// listing the members of one block.
//
// A span the heading's file could not be declaring is not a symbol here, and
// breaks the group the way any other non-symbol span does.
// aSymbolThisFileCouldDeclare is that rule and carries what each of its excuses
// costs. What is left is a citation whether or not it resolves, so a symbol the
// file no longer declares is reported by name instead of vanishing.
func bareLinesInParagraph(
	index *sourceIndex, paragraph string, files []string, page string, startLine int,
) []citation {
	spans := backtickSpan.FindAllStringSubmatchIndex(paragraph, -1)

	form, indexed := formBareLineOutsideGo, headingIsIndexed(index, files)
	if indexed {
		form = formBareLine
	}

	var found []citation

	var group []string

	groupAt := 0

	for position, match := range spans {
		if position > 0 && !joinsAGroup(paragraph[spans[position-1][1]:match[0]]) {
			group = nil
		}

		symbol, ok := normaliseSymbol(paragraph[match[2]:match[3]])
		if !ok || (indexed && !aSymbolThisFileCouldDeclare(index, files, symbol)) {
			group = nil

			continue
		}

		if len(group) == 0 {
			groupAt = match[0]
		}

		group = append(group, symbol)

		targets := bareLineTarget.FindStringSubmatch(paragraph[match[1]:])
		if targets == nil {
			continue
		}

		if numbers, read := lineNumbers(targets[1]); read {
			found = append(found, citation{
				page: page, docLine: startLine + strings.Count(paragraph[:groupAt], "\n"),
				files: files, symbols: group, targets: numbers, form: form,
			})
		}

		group = nil
	}

	return found
}

// joinsAGroup reports whether the text between two code spans keeps them in one
// group: whitespace and commas, and nothing else.
func joinsAGroup(between string) bool {
	return strings.TrimFunc(between, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	}) == ""
}

// goOwnsTheName is every identifier Go itself owns: the keywords and the
// predeclared names. A chapter writes these as ordinary words, and a word in
// backticks in front of a number is the same shape as a citation.
//
// Excusing them costs nothing that can be lost. A file that really declares a
// `min` or an `error` of its own resolves the span and is checked against the
// declaration like any other, so this list is only ever reached by a name the
// file does not declare, which is the case it is here to excuse.
//
//nolint:gochecknoglobals // a fixed property of the language, read on every span.
var goOwnsTheName = map[string]struct{}{
	"any": {}, "append": {}, "bool": {}, "break": {}, "byte": {}, "cap": {},
	"case": {}, "chan": {}, "clear": {}, "close": {}, "comparable": {},
	"complex": {}, "complex64": {}, "complex128": {}, "const": {}, "continue": {},
	"copy": {}, "default": {}, "defer": {}, "delete": {}, "else": {},
	"error": {}, "fallthrough": {}, "false": {}, "float32": {}, "float64": {},
	"for": {}, "func": {}, "go": {}, "goto": {}, "if": {}, "imag": {},
	"import": {}, "int": {}, "int8": {}, "int16": {}, "int32": {}, "int64": {},
	"interface": {}, "iota": {}, "len": {}, "make": {}, "map": {}, "max": {},
	"min": {}, "new": {}, "nil": {}, "package": {}, "panic": {}, "print": {},
	"println": {}, "range": {}, "real": {}, "recover": {}, "return": {},
	"rune": {}, "select": {}, "string": {}, "struct": {}, "switch": {},
	"true": {}, "type": {}, "uint": {}, "uint8": {}, "uint16": {}, "uint32": {},
	"uint64": {}, "uintptr": {}, "var": {},
}

// aSymbolThisFileCouldDeclare reports whether a code span sitting in front of a
// line number under a file heading is a claim about a symbol of that file at
// all. It is the whole of the rule that decides which bare-line groups the gate
// reads, and it is stated here rather than left to whether the symbol resolves,
// because those are different questions and conflating them is what let
// sixteen citations disappear in one rename.
//
// Three shapes are excused, and each costs something named:
//
//   - a heading naming no Go file. There is no declaration index to ask, so a
//     JSON key or a template's identifier resolves to nothing whatever it says.
//     The cost is that a stale key under such a heading is held only by
//     TestEveryBareLineCitationNamesALineItsFileHas, which bounds the line and
//     reads no names
//   - an identifier Go owns. The cost is written on goOwnsTheName: nothing
//   - a package one of the heading's files refers to, whole or as the qualifier
//     of a dotted name. "`connectorapitest`" under echo_test.go and
//     "`storageapi.ProviderSpec`" under local.go are both about another package,
//     and neither file declares them. The cost is a real symbol of this file
//     that happens to share a name with one of its imports: a `stream` declared
//     in a file that imports stream/ is excused rather than checked. No file in
//     this tree does that
//
// Everything else is a citation, and a citation the file does not declare is
// reported by TestEveryCitedLineHoldsItsSymbol with the file, the symbol and
// the documentation line it is written on.
// headingIsIndexed reports whether a heading names at least one Go file, which
// is whether there is a declaration index to resolve its symbols against at all.
func headingIsIndexed(index *sourceIndex, files []string) bool {
	for _, file := range files {
		source, err := index.read(file)
		if err == nil && source.isGo {
			return true
		}
	}

	return false
}

func aSymbolThisFileCouldDeclare(index *sourceIndex, files []string, symbol string) bool {
	if _, owned := goOwnsTheName[symbol]; owned {
		return false
	}

	qualifier := symbol
	if cut := strings.Index(symbol, "."); cut > 0 {
		qualifier = symbol[:cut]
	}

	anyGo := false

	for _, file := range files {
		source, err := index.read(file)
		if err != nil {
			continue
		}

		if !source.isGo {
			continue
		}

		anyGo = true

		// A file that declares the symbol answers the question outright, even
		// where the name also matches one of its imports.
		if len(source.spans[symbol]) > 0 {
			return true
		}

		if source.refersToPackage(qualifier) {
			return false
		}
	}

	return anyGo
}

// tableCitation reads one row of a symbol-and-line table. The line column is a
// comma-separated list of numbers, pairing with the symbol column where a row
// names two members of one block.
func tableCitation(line string, files []string, page string, number int) (citation, bool) {
	cells := strings.Split(strings.Trim(line, "|"), "|")
	if len(cells) < 2 {
		return citation{}, false
	}

	targets, ok := lineNumbers(cells[1])
	if !ok {
		return citation{}, false
	}

	symbols := symbolsIn(cells[0])
	if len(symbols) == 0 {
		return citation{}, false
	}

	return citation{
		page: page, docLine: number, files: files, symbols: symbols, targets: targets,
		form: formCarriesItsFile,
	}, true
}

// inlineCitations reads the prose form: a citation in a code span with the
// symbol it is about in a code span beside it. The chapters write it both ways
// round, "`Publish` (`hub.go:321`)" and "`filter.go:53` (`ListPrinters`)", so
// both sides are candidates.
//
// The symbol taken is the nearest candidate that the cited file actually
// declares, and never the merely nearest. "`actionSet` is `uint32`, declared at
// `vocabulary.go:90`" puts a builtin between the symbol and its citation, and
// a scan that stopped at the first span would report the page wrong about
// `uint32`. Where no candidate resolves the citation is left to the line-bound
// check, because prose names types this repository does not own.
//
// It reads the whole page rather than a line at a time. These chapters are hard
// wrapped, so "`TestPointCheckMatchesTheGraphWalk` at\n`equivalence_test.go:398`"
// puts the symbol and its citation on different lines, and a line-at-a-time
// scan pairs that citation with the next symbol along instead.
func inlineCitations(index *sourceIndex, body, page string) []citation {
	spans := backtickSpan.FindAllStringSubmatchIndex(body, -1)

	var found []citation

	for position, match := range spans {
		cited := spanIsCitation.FindStringSubmatch(body[match[2]:match[3]])
		if cited == nil {
			continue
		}

		source, err := index.read(cited[1])
		if err != nil {
			continue
		}

		target, err := strconv.Atoi(cited[2])
		if err != nil {
			continue
		}

		symbol, ok := nearestDeclared(body, spans, position, source)
		if !ok {
			continue
		}

		found = append(found, citation{
			page: page, docLine: strings.Count(body[:match[0]], "\n") + 1,
			files: []string{cited[1]}, symbols: []string{symbol}, targets: []int{target},
		})
	}

	return found
}

// nearestDeclared walks outwards from the citation, one code span at a time in
// each direction, and returns the first identifier the cited file declares.
func nearestDeclared(body string, spans [][]int, position int, source *source) (string, bool) {
	type candidate struct {
		text     string
		between  string
		distance int
	}

	var candidates []candidate

	for offset := 1; offset <= 2; offset++ {
		if before := position - offset; before >= 0 {
			candidates = append(candidates, candidate{
				text:     body[spans[before][2]:spans[before][3]],
				distance: len(body[spans[before][1]:spans[position][0]]),
				between:  body[spans[before][1]:spans[position][0]],
			})
		}

		if after := position + offset; after < len(spans) {
			candidates = append(candidates, candidate{
				text:     body[spans[after][2]:spans[after][3]],
				distance: len(body[spans[position][1]:spans[after][0]]),
				between:  body[spans[position][1]:spans[after][0]],
			})
		}
	}

	sort.SliceStable(candidates, func(a, b int) bool {
		return candidates[a].distance < candidates[b].distance
	})

	for _, found := range candidates {
		// A pipe between the two is a table cell boundary and a blank line is a
		// paragraph boundary. Across either, the symbol and the citation are
		// not about each other.
		if found.distance > inlineReach ||
			strings.Contains(found.between, "|") ||
			strings.Count(found.between, "\n") > 1 {
			continue
		}

		symbol, ok := normaliseSymbol(found.text)
		if !ok {
			continue
		}

		if len(source.spans[symbol]) > 0 {
			return symbol, true
		}
	}

	return "", false
}

// lineNumbers reads a line column. It refuses anything that is not a list of
// numbers, so a table whose second column holds something else is not read as a
// citation table by accident.
func lineNumbers(cell string) ([]int, bool) {
	var numbers []int

	for part := range strings.SplitSeq(cell, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}

		numbers = append(numbers, number)
	}

	return numbers, len(numbers) > 0
}

// symbolsIn reads the symbol column: every code span in it that is a Go
// identifier. A cell whose spans are an import path, an error code or an
// ellipsis yields none, and the row is then held only by the line bound.
func symbolsIn(cell string) []string {
	var symbols []string

	for _, match := range backtickSpan.FindAllStringSubmatch(cell, -1) {
		for part := range strings.SplitSeq(match[1], ",") {
			if symbol, ok := normaliseSymbol(part); ok {
				symbols = append(symbols, symbol)
			}
		}
	}

	return symbols
}

// normaliseSymbol reduces the forms a chapter writes a symbol in to the name
// itself, and refuses anything that is not an identifier.
//
// The three suffixes matter, because dropping one sends the check to the wrong
// symbol rather than to none. "`conflationKey{origin, key}`
// (`stream/queue.go:11`)" reduces to `conflationKey`; left whole it resolves to
// nothing, the scan walks on to the next code span, and the page is then
// checked against whatever that is.
func normaliseSymbol(text string) (string, bool) {
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "*"))

	if index := strings.IndexAny(text, "[{("); index > 0 {
		text = text[:index]
	}

	if !goIdentifier.MatchString(text) {
		return "", false
	}

	return text, true
}

// headingFiles reads the repository files a heading names. A heading covering
// two files names both, and a heading naming none returns none, which is what
// TestEveryCitationTableIsUnderAFileHeading refuses a table under.
func headingFiles(root, heading string) []string {
	var files []string

	for _, match := range citedFile.FindAllStringSubmatch(heading, -1) {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(match[1])))
		if err != nil || info.IsDir() {
			continue
		}

		files = append(files, match[1])
	}

	return files
}

// span is where one declaration sits: its first line, counting the doc comment
// above it, its last line, and the line a reader should have been sent to.
type span struct {
	from       int
	to         int
	declaredAt int
}

// source is one file of this repository as the gate needs it: how long it is,
// where each of its declarations sits, whether it is Go at all, and the package
// names it refers to other packages by.
//
// imports is what tells a stale symbol from a package name. Both are a code
// span the file does not declare sitting in front of a line number, and only
// the import list separates them.
type source struct {
	lines   int
	isGo    bool
	spans   map[string][]span
	imports map[string]struct{}
}

// sourceIndex reads a file once however many citations name it. The six
// chapters cite 175 distinct files and the tests run in parallel, so this is
// the difference between a gate that runs in under a second and one somebody
// switches off.
type sourceIndex struct {
	root   string
	mu     sync.Mutex
	parsed map[string]*source
	fail   map[string]error
}

func newSourceIndex(root string) *sourceIndex {
	return &sourceIndex{root: root, parsed: map[string]*source{}, fail: map[string]error{}}
}

func (i *sourceIndex) read(path string) (*source, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if cached, ok := i.parsed[path]; ok {
		return cached, nil
	}

	if err, ok := i.fail[path]; ok {
		return nil, err
	}

	parsed, err := readSource(i.root, path)
	if err != nil {
		i.fail[path] = err

		return nil, err
	}

	i.parsed[path] = parsed

	return parsed, nil
}

// readSource counts a file's lines and, where it is Go, records the span of
// every declaration in it.
func readSource(root, path string) (*source, error) {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return nil, err
	}

	parsed := &source{
		lines:   strings.Count(string(body), "\n") + 1,
		spans:   map[string][]span{},
		imports: map[string]struct{}{},
	}

	if !strings.HasSuffix(path, ".go") {
		return parsed, nil
	}

	set := token.NewFileSet()

	file, err := parser.ParseFile(set, path, body, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	parsed.isGo = true

	// The file's own package name counts as one it refers to: a `_test` file
	// writes `echo.New` about the package beside it, and a chapter quoting that
	// sentence names `echo` without the file declaring it.
	parsed.imports[strings.TrimSuffix(file.Name.Name, "_test")] = struct{}{}

	for _, spec := range file.Imports {
		parsed.imports[importIdentifier(spec)] = struct{}{}
	}

	record := func(name string, first, at, last token.Pos) {
		parsed.spans[name] = append(parsed.spans[name], span{
			from:       set.Position(first).Line,
			to:         set.Position(last).Line,
			declaredAt: set.Position(at).Line,
		})
	}

	for _, decl := range file.Decls {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			recordFunc(record, typed)
		case *ast.GenDecl:
			recordGen(record, typed)
		}
	}

	return parsed, nil
}

// importIdentifier is the identifier this file writes an import under: the name it
// was given where there is one, and the last element of the path where there is
// not. It is deliberately the last element rather than the package's real
// clause, because that is what a chapter's prose writes and this list is only
// ever matched against prose.
func importIdentifier(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}

	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return ""
	}

	if cut := strings.LastIndex(path, "/"); cut >= 0 {
		return path[cut+1:]
	}

	return path
}

// refersToPackage reports whether this file names another package by this
// identifier.
func (s *source) refersToPackage(name string) bool {
	_, ok := s.imports[name]

	return ok
}

type recorder func(name string, first, at, last token.Pos)

// recordFunc records a function under its own name, and a method under both its
// own name and the qualified one the chapters write, `Hub.Publish`.
func recordFunc(record recorder, decl *ast.FuncDecl) {
	first := decl.Pos()
	if decl.Doc != nil {
		first = decl.Doc.Pos()
	}

	record(decl.Name.Name, first, decl.Pos(), decl.End())

	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return
	}

	if receiver := receiverName(decl.Recv.List[0].Type); receiver != "" {
		record(receiver+"."+decl.Name.Name, first, decl.Pos(), decl.End())
	}
}

// recordGen records a type, const or var. A parenthesised block is recorded
// whole for each of its members, because a page citing the line the block opens
// is citing every member of it and is right to.
func recordGen(record recorder, decl *ast.GenDecl) {
	first := decl.Pos()
	if decl.Doc != nil {
		first = decl.Doc.Pos()
	}

	for _, spec := range decl.Specs {
		switch typed := spec.(type) {
		case *ast.TypeSpec:
			specFirst, specAt := first, decl.Pos()
			if decl.Lparen.IsValid() {
				specFirst, specAt = typed.Pos(), typed.Pos()

				if typed.Doc != nil {
					specFirst = typed.Doc.Pos()
				}
			}

			record(typed.Name.Name, specFirst, specAt, typed.End())
			recordMembers(record, typed)
		case *ast.ValueSpec:
			if decl.Lparen.IsValid() {
				// The span is the whole block, because citing the line it opens
				// is citing every member of it. The line to repair to is the
				// member's own, which is what the chapters write.
				for _, name := range typed.Names {
					record(name.Name, first, typed.Pos(), decl.End())
				}

				continue
			}

			for _, name := range typed.Names {
				record(name.Name, first, decl.Pos(), typed.End())
			}
		}
	}
}

// recordMembers records a struct's fields and an interface's methods under the
// qualified name a chapter writes them with, `Subscription.allowed`. The type's
// own span is added too, so a page citing a field at the line the struct opens
// is not called wrong for it.
//
// A member is deliberately not recorded under its bare name. `key`, `id`,
// `state` and `origin` are all field names in this tree and all ordinary words
// in a sentence, so a bare member index turns any of them into a symbol the
// scan can match, and it then checks the page against a field it was never
// talking about.
func recordMembers(record recorder, spec *ast.TypeSpec) {
	var fields *ast.FieldList

	switch typed := spec.Type.(type) {
	case *ast.StructType:
		fields = typed.Fields
	case *ast.InterfaceType:
		fields = typed.Methods
	default:
		return
	}

	if fields == nil {
		return
	}

	for _, field := range fields.List {
		for _, name := range field.Names {
			record(spec.Name.Name+"."+name.Name, field.Pos(), field.Pos(), field.End())
			record(spec.Name.Name+"."+name.Name, spec.Pos(), field.Pos(), spec.End())
		}
	}
}

// receiverName is the type a method hangs off, through a pointer and through
// the type parameters a generic receiver carries.
func receiverName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return receiverName(typed.X)
	case *ast.Ident:
		return typed.Name
	case *ast.IndexExpr:
		return receiverName(typed.X)
	case *ast.IndexListExpr:
		return receiverName(typed.X)
	}

	return ""
}

func citedPage(t *testing.T, root, page string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(root, citationDirs, filepath.FromSlash(page)))
	if err != nil {
		t.Fatalf("read %s/%s: %v", citationDirs, page, err)
	}

	return string(body)
}

// reportCitations fails with every violation listed. Repairing one citation at
// a time across six chapters is one run of the suite each, and there were 166
// of them the day this was measured.
func reportCitations(t *testing.T, violations []string) {
	t.Helper()

	if len(violations) == 0 {
		return
	}

	sort.Strings(violations)
	t.Errorf("%d citation(s) are wrong about the tree:\n\t%s",
		len(violations), strings.Join(violations, "\n\t"))
}
