package arch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// The build recipes, read rather than repeated.
//
// Three gates need to know what the project actually compiles: the import graph
// gates in this package, the cgo gate beside them, and the error-code registry
// gate in cmd/generate-error-codes. Each of them answers a different question,
// and each of them is wrong in the same way if it answers it about a
// configuration nobody builds.
//
// Two ways that goes wrong, both of them seen here. A graph gate running one
// untagged `go list` polices the single configuration this project never ships.
// A gate growing its own list of tag sets, none of them the one the image is
// built with, lets a file behind `//go:build full && !fractal` compile into the
// shipped binary and register a code no gate can see.
//
// So the sets are computed once, here, from the recipes themselves:
// detect-build-tags, the Makefile's FULL_TAGS and the release Dockerfile. A list
// typed into a test drifts from `make build` in silence; a pattern that stops
// matching fails loudly, which is the point.

// TagSets is every tag set the build recipes compile under, by name.
//
//	untagged       no tags at all: plain code, and the only set that is not built
//	build          what `make build` computes from CONFIG_FILE, and a plain `docker build` of the image
//	build-minimal  the minimal variant of the same
//	build-ci       FULL_TAGS, the widest set, which `make build-ci` and the tests use
//	build-printer  what `make build-printer` compiles, from PRINTER_TAGS and PRINTER_CONFIG
//	release        the list the cross-architecture release binaries are built with, and the published image
//
// The answer is computed once per process: it shells out to the project's own
// detector, and several gates ask for it.
func TagSets(root string) (map[string]string, error) {
	tagSetsOnce.Do(func() { tagSetsValue, errTagSets = readTagSets(root) })

	return tagSetsValue, errTagSets
}

var (
	tagSetsOnce  sync.Once
	tagSetsValue map[string]string
	errTagSets   error
)

func readTagSets(root string) (map[string]string, error) {
	configFile, err := makefileValue(root, `(?m)^CONFIG_FILE\s*\?=\s*(\S+)`, "CONFIG_FILE")
	if err != nil {
		return nil, err
	}

	full, err := detectTags(root, configFile, "full")
	if err != nil {
		return nil, err
	}

	minimal, err := detectTags(root, configFile, "minimal")
	if err != nil {
		return nil, err
	}

	allTags, err := makefileValue(root, FullTagsPattern, "FULL_TAGS")
	if err != nil {
		return nil, err
	}

	release, err := ReleaseImageTags(root)
	if err != nil {
		return nil, err
	}

	printer, err := printerTags(root)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"untagged":      "",
		"build":         withVariant("full", full),
		"build-minimal": withVariant("minimal", minimal),
		"build-ci":      withVariant("full", ExpandMakeVars(allTags)),
		"build-printer": printer,
		"release":       release,
	}, nil
}

// assignmentFields is the variable name and the operator that start a Makefile
// assignment.
const assignmentFields = 2

// ImageTags is the tag set the published image compiles under. The Dockerfile
// builds with `full` plus what detect-build-tags reports for the shipping
// configuration with the extra tags the workflow passes in, so this runs that
// same detector over those same inputs.
func ImageTags(root, extra string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "go", "run", "./cmd/detect-build-tags",
		"--config", "config/config.yaml", "--variant", "full", "--extra", extra)
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("detect-build-tags --variant full --extra %q: %w%s", extra, err, StderrOf(err))
	}

	return withVariant("full", strings.TrimSpace(string(out))), nil
}

// printerTags is the tag set `make build-printer` compiles under: full,
// printer, every tag the Makefile adds to PRINTER_TAGS, and the tags the
// printer configuration implies.
func printerTags(root string) (string, error) {
	content, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return "", fmt.Errorf("read the Makefile: %w", err)
	}

	tags := []string{"full", "printer"}
	configFile := ""

	for line := range strings.SplitSeq(string(content), "\n") {
		value, _, _ := strings.Cut(line, "#")
		fields := strings.Fields(value)

		if len(fields) < assignmentFields {
			continue
		}

		switch {
		case fields[0] == "PRINTER_TAGS" && fields[1] == "+=":
			tags = append(tags, fields[2:]...)
		case fields[0] == "PRINTER_CONFIG" && fields[1] == ":=" && len(fields) == 3:
			configFile = fields[2]
		}
	}

	if configFile == "" {
		return "", errors.New("the Makefile sets no PRINTER_CONFIG, so this gate does not know " +
			"what the printer build compiles")
	}

	detected, err := detectTags(root, configFile, "full")
	if err != nil {
		return "", err
	}

	return withVariant(strings.Join(tags, " "), detected), nil
}

// detectTags shells out to the project's own detector, so a gate tracks
// whatever `make build` computes rather than a second copy of the rules.
func detectTags(root, configFile, variant string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "go", "run", "./cmd/detect-build-tags",
		"--config", configFile, "--variant", variant)
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("detect-build-tags --variant %s: %w%s", variant, err, StderrOf(err))
	}

	return strings.TrimSpace(string(out)), nil
}

// makefileValue pulls one literal out of the Makefile.
func makefileValue(root, pattern, what string) (string, error) {
	return FileValue(filepath.Join(root, "Makefile"), pattern, what)
}

// ReleaseImageTags is the tag list the released cross-architecture binaries are
// built with. It is a second literal in a second file.
func ReleaseImageTags(root string) (string, error) {
	return FileValue(
		filepath.Join(root, ".github", "dockerfiles", "Dockerfile.build"),
		`go build[^\n]*-tags "([^"]+)"`,
		"the release build's -tags list",
	)
}

// FileValue pulls one literal out of a build recipe.
//
// Comment lines go first, because both recipe languages here take `#` to the end
// of the line and a commented-out build line matches every pattern the real one
// does. Without that, a line parked above the real one decides what the gates
// cover: `# historic: RUN go build -tags "full sqlite"` narrows the release set
// to two tags, with no error, because the pattern still matches. Every edge
// behind a tag outside the parked list then goes unchecked, green.
//
// A pattern that no longer matches means the recipe moved, and a configuration
// the caller believes it covers is no longer covered, so it is a failure rather
// than a fallback.
func FileValue(path, pattern, what string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	match := regexp.MustCompile(pattern).FindStringSubmatch(hashComment.ReplaceAllString(string(content), ""))
	if match == nil {
		return "", fmt.Errorf(
			"no %s in %s: the pattern %q matched nothing outside the comments, so this gate no "+
				"longer knows what that build compiles. Update the pattern to the recipe's new shape",
			what, filepath.Base(path), pattern)
	}

	return strings.TrimSpace(match[1]), nil
}

// FullTagsPattern reads the widest tag set out of the Makefile. Three gates
// need it, and each one asking with its own spelling is three ways to stop
// matching, so the pattern is written here and they name it.
//
// It matches a make variable rather than a shell assignment inside a recipe.
// The list was written out in three recipes and read from whichever one the
// pattern reached first, so two of the three could drift and nothing compared
// them. There is one assignment now and every recipe expands it.
const FullTagsPattern = `(?m)^FULL_TAGS\s*:?=[ \t]*(\S.*)$`

// ExpandMakeVars drops make variable references. EXTRA_BUILD_TAGS is empty
// unless a caller sets it, and the gates cover the default build.
func ExpandMakeVars(tags string) string {
	return strings.Join(strings.Fields(makeVar.ReplaceAllString(tags, " ")), " ")
}

func withVariant(variant, tags string) string {
	return strings.Join(strings.Fields(variant+" "+tags), " ")
}

var (
	// hashComment is a whole comment line in either recipe language. Dockerfile
	// and make both comment with `#` and neither has any other form.
	hashComment    = regexp.MustCompile(`(?m)^[ \t]*#[^\n]*\n?`)
	dockerContinue = regexp.MustCompile(`\\[ \t]*\r?\n`)
	cgoAssignment  = regexp.MustCompile(`CGO_ENABLED=([01])`)
	goBuildCommand = regexp.MustCompile(`\bgo build\b`)
	makeVar        = regexp.MustCompile(`\$\([^)]*\)`)
)

// DockerfileBuildsWithCgo reports whether the go build in a Dockerfile runs with
// cgo on. Comment lines are removed first, the way the Docker parser removes
// them, so a literal parked in a comment decides nothing. Line continuations are
// joined so an ENV on one line and the build on another are one instruction to
// read, and both spellings count: the inline `CGO_ENABLED=1 go build` and the
// `ENV CGO_ENABLED=1` that the release recipe uses.
//
// ENV does not cross an image boundary. `FROM golang AS b` starts with nothing
// in scope whatever an earlier stage set, and `FROM builder AS b` inherits what
// builder left, so each stage's setting is remembered under its name. Carrying
// the value across an unrelated FROM reports cgo on for a build that has no
// CGO_ENABLED at all, which is the one case this function exists to reject.
//
// An unset value is an error rather than a default. The toolchain's own default
// depends on whether the build is cross-compiling and whether a C compiler is
// present, so a recipe that does not say is a recipe nobody can read the answer
// out of.
func DockerfileBuildsWithCgo(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}

	text := dockerContinue.ReplaceAllString(hashComment.ReplaceAllString(string(content), ""), " ")

	leftByStage := map[string]string{}
	inherited := ""
	stage := ""
	decided := false
	withCgo := false

	for line := range strings.SplitSeq(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		if strings.EqualFold(fields[0], "FROM") {
			inherited, stage = enterStage(leftByStage, fields)

			continue
		}

		if len(fields) >= 2 && strings.EqualFold(fields[0], "ENV") {
			if match := cgoAssignment.FindStringSubmatch(line); match != nil {
				inherited = match[1]
				leftByStage[stage] = inherited
			}
		}

		if !goBuildCommand.MatchString(line) {
			continue
		}

		setting := inherited
		if match := cgoAssignment.FindStringSubmatch(line); match != nil {
			setting = match[1]
		}

		if setting == "" {
			return false, fmt.Errorf(
				"a `go build` here runs with no CGO_ENABLED in scope, so whether it links a cgo\n"+
					"driver depends on the toolchain's cross-compilation default. Set it explicitly:\n\t%s",
				strings.TrimSpace(line))
		}

		decided = true
		withCgo = withCgo || setting == "1"
	}

	if !decided {
		return false, fmt.Errorf("no `go build` in %s, so this gate is reading nothing", filepath.Base(path))
	}

	return withCgo, nil
}

// The fields of `FROM <base> AS <name>`, which is the only shape that names a
// stage.
const (
	fromBase = 1
	fromAs   = 2
	fromName = 3
)

// enterStage reports what a FROM leaves in scope and what the new stage is
// called. A base that names an earlier stage carries that stage's setting; a
// base that names an image carries nothing.
func enterStage(leftByStage map[string]string, fields []string) (string, string) {
	inherited := ""
	if len(fields) > fromBase {
		inherited = leftByStage[strings.ToLower(fields[fromBase])]
	}

	stage := ""
	if len(fields) > fromName && strings.EqualFold(fields[fromAs], "AS") {
		stage = strings.ToLower(fields[fromName])
	}

	leftByStage[stage] = inherited

	return inherited, stage
}

// StderrOf is what a failed exec.Cmd wrote to stderr, ready to append to an
// error string. `go list` reports the reason a configuration would not load
// there and nowhere else, so a gate that only wraps the exit status says
// "exit status 1" and nothing a reader can act on.
//
// It is exported because the gates in cmd/generate-error-codes and
// cmd/generate-extensions run `go list` the same way.
func StderrOf(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return "\n" + string(exitErr.Stderr)
	}

	return ""
}
