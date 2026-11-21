package arch_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/prusa3d/prusa-connect/internal/arch"
)

// The published image and the full release binaries share one tag list, kept
// in three places. The release recipe in .github/dockerfiles/Dockerfile.build
// names it, the full binary in release.yml repeats it, and container-image.yml
// passes it as EXTRA_BUILD_TAGS for docker/server/Dockerfile to add `full` to.
//
// Nothing compared those copies. Removing one tag from the workflow line left
// every gate green while the image lost the part the tag carries, which is how
// the image once shipped without the postgres driver. This gate reads all
// three and refuses the run when they stop naming the same set.
func TestThePublishedImageMatchesTheReleaseBinaries(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	release, err := arch.ReleaseImageTags(root)
	if err != nil {
		t.Fatalf("reading the release build's tags: %v", err)
	}

	binaryTags, err := arch.FileValue(
		filepath.Join(root, ".github", "workflows", "release.yml"),
		`go build[^\n]*-tags "([^"]+)"`,
		"the full binary's -tags list",
	)
	if err != nil {
		t.Fatalf("reading the full binary's tags out of release.yml: %v", err)
	}

	requireSameTagSet(t, "the full binary in release.yml", binaryTags,
		"the release build in .github/dockerfiles/Dockerfile.build", release)

	extra, err := arch.FileValue(
		filepath.Join(root, ".github", "workflows", "container-image.yml"),
		`echo "EXTRA_BUILD_TAGS=([^"]+)"`,
		"EXTRA_BUILD_TAGS",
	)
	if err != nil {
		t.Fatalf("reading EXTRA_BUILD_TAGS out of container-image.yml: %v", err)
	}

	imageTags, err := arch.ImageTags(root, extra)
	if err != nil {
		t.Fatalf("computing the published image's tags: %v", err)
	}

	requireSameTagSet(t, "the published image", imageTags,
		"the release build in .github/dockerfiles/Dockerfile.build", release)
}

// requireSameTagSet refuses two tag lists that name different sets. Order is
// not compared: recipes write the same set in different orders, and only the
// set decides what compiles.
func requireSameTagSet(t *testing.T, haveWhat, have, wantWhat, want string) {
	t.Helper()

	haveSet := slices.Sorted(slices.Values(strings.Fields(have)))
	wantSet := slices.Sorted(slices.Values(strings.Fields(want)))

	requireScanned(t, "tags in "+haveWhat, len(haveSet), 1,
		"The pattern is what decides coverage, so a pattern that stopped matching must fail.")

	if !slices.Equal(haveSet, wantSet) {
		t.Errorf("%s builds with tags %q, and %s builds with tags %q.\n"+
			"Both ship the same release, so the two sets match.",
			haveWhat, have, wantWhat, want)
	}
}
