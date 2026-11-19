package arch_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// ADR-20261025-pze: the tool tree is the only place a tool lives. No source and no
// current document names the flat paths an earlier release wrote beside the
// tree, the path that named the active tool, or the /tool/, /slot/, /mmu/
// and /tool_mapping paths an earlier driver wrote. These files may, and each
// says why. The start_print argument named tool_mapping is not a path and
// is not matched.
//
//nolint:gochecknoglobals // the allowance list.
var flatPathReaders = map[string]string{
	"internal/arch/flatpaths_test.go":                "this test",
	"internal/connector/xbuddy/tools.go":             "removes the old paths from a stored tree",
	"internal/connector/xbuddy/tools_test.go":        "proves the old paths are not reserved",
	"internal/connector/xbuddy/tools_sim_test.go":    "proves the driver never writes them",
	"internal/app/devicetools_test.go":               "proves the record names no active tool",
	"sdk/connectorapi/tools_test.go":                 "proves the JSON form names no active tool",
	"frontend/test/journey/tools.test.ts":            "proves the demo fleet holds none of them",
	"internal/app/tooltree_contract_test.go":         "proves no printer family holds them",
	"examples/adapters/moonraker/dialect_test.go":    "proves the Klipper driver never writes them",
	"sdk/printcompat/evaluate_test.go":               "proves a reader takes them for no tool",
	"packages/svelte/src/domain/state-paths.test.ts": "proves a page reads no head from them",
}

// flatPathSkipped are the trees this test does not read: no source of the
// core, a local scratch tree of working notes that git never tracks, and the
// records of how the design got here. The decisions, the
// changelog and its pending fragments say what an earlier release wrote, and
// a record keeps its history.
//
//nolint:gochecknoglobals // a fixed skip set.
var flatPathSkipped = []string{
	".git", ".codex", ".superpowers", "node_modules", "dist", "bin", "storybook-static", "generated",
	"docs/decisions", "docs/changes", "docs/CHANGELOG.md",
	"internal/arch/testdata",
}

// flatPathRead are the kinds of file this test reads: source, the data and
// fixtures a test loads, and the documents.
//
//nolint:gochecknoglobals // a fixed set.
var flatPathRead = []string{".go", ".ts", ".js", ".svelte", ".json", ".yaml", ".yml", ".md", ".mdx", ".tsv"}

// flatPathPattern is any spelling of one of the old paths, in a source, in a
// document, and inside a JSON string, where the quotes are escaped. A path
// under /tool/, /slot/ or /mmu/, or /tool_mapping, is matched where a quote
// opens it, so an MQTT topic such as data/slot/active is not a path here.
var flatPathPattern = regexp.MustCompile( //nolint:gochecknoglobals // compiled once.
	`/temperature/nozzle|active_tool|["'` + "`" + `]/(?:material\\?["'` + "`" + `]|(?:tool|slot|mmu)/|tool_mapping)`)

func TestNoSourceNamesAPathOutsideTheToolTree(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var found []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)

		if entry.IsDir() {
			if slices.Contains(flatPathSkipped, entry.Name()) || slices.Contains(flatPathSkipped, relative) {
				return filepath.SkipDir
			}

			return nil
		}

		if !slices.Contains(flatPathRead, filepath.Ext(path)) || slices.Contains(flatPathSkipped, relative) {
			return nil
		}

		if _, allowed := flatPathReaders[relative]; allowed {
			return nil
		}

		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		for n, line := range strings.Split(string(body), "\n") {
			if at := flatPathPattern.FindStringIndex(line); at != nil {
				found = append(found, relative+":"+strconv.Itoa(n+1)+": "+aroundMatch(line, at))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(found) > 0 {
		t.Errorf("a source names a path outside the tool tree (ADR-20261025-pze). Read the tree:\n  %s",
			strings.Join(found, "\n  "))
	}

	for path := range flatPathReaders {
		if _, statErr := os.Stat(filepath.Join(root, path)); statErr != nil {
			t.Errorf("%s is allowed to name the old paths and does not exist; remove the line", path)
		}
	}
}

// TestTheFlatPathPatternCatchesEachSpelling proves the search finds each of
// the old paths in each quoting a source, a document or a JSON fixture uses,
// so a green run means none is there.
func TestTheFlatPathPatternCatchesEachSpelling(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		`p := "/temperature/nozzle"`,
		`'/temperature/nozzle_target': 0,`,
		`connectorapi.ActiveToolPath // "/active_tool"`,
		`out["active_tool"] = id`,
		`state["/material"]`,
		"textAt(device, '/material')",
		"`/material`",
		`"body": "{\"state\":{\"/material\":\"PLA\"}}"`,
		`keyed by path such as ` + "`/temperature/nozzle`",
		`"/tool/0/material": "PLA"`,
		"`/tool/${index}/temperature`",
		`'/slot/1/temperature': 100,`,
		`"/slot/" + strconv.Itoa(n)`,
		`absent: []string{"/mmu/enabled"}`,
		`\"/mmu/enabled\":true`,
		`state["/tool_mapping"]`,
		"| `/tool_mapping/0` | the old map |",
	} {
		if !flatPathPattern.MatchString(line) {
			t.Errorf("the search misses %s", line)
		}
	}

	for _, line := range []string{
		`"/tools/0/material_package_instance/package/material/abbreviation"`,
		`"/temperature/bed"`,
		`import MaterialDetail from './material-detail.svelte'`,
		`"/tool_count": 1`,
		`"/tools/0/feeder/slot_count": 0`,
		`\"/tools/0/material_package_instance/package/material/abbreviation\":\"PLA\"`,
		`"tool_mapping": {Flag: true, Kind: connectorapi.KindOpaque},`,
		`"core.command.start_print#tool_mapping"`,
		`Pattern: printerTopic("data/slot/active")`,
		`return Base + "/" + printerPlaceholder + "/data/slot/" + slotPlaceholder`,
		`"/gcode_tool_map/0": 1`,
	} {
		if flatPathPattern.MatchString(line) {
			t.Errorf("the search takes %s for an old path", line)
		}
	}
}

// aroundMatch is the part of a line around a match, so a fixture that holds a
// whole response on one line is reported by the words that matched.
func aroundMatch(line string, at []int) string {
	const margin = 60

	from, to := max(at[0]-margin, 0), min(at[1]+margin, len(line))

	return strings.ToValidUTF8(strings.TrimSpace(line[from:to]), "")
}
