#!/bin/sh
# The three shipped modules, built and laid out as releases:
#
#   examples/spools            a module with a Go half and a browser half
#   examples/nozzle-watch      a module that is one browser bundle
#   examples/ocean-theme       a module that is one stylesheet, offered as a theme
#   packages/graphql    a module that is one Go process, with no page
#
# Two callers read what this writes, and that is why the packing lives here
# rather than in either of them. scripts/check-example-spools.sh installs the
# release directories with `prusa-connect module add`, and `make demo`
# offers the release directories on the extensions page. A copy of this shell in both
# would drift, and the first sign of it would be a demo installing a layout the
# gate never proved.
#
# What it writes, under $PACK_ROOT:
#
#   spools/                    the release directory `module add` reads
#   spools-<version>.tar.gz    the same tree as one archive
#   nozzle-watch/
#   nozzle-watch-<version>.tar.gz
#   graphql/
#   graphql-<version>.tar.gz
#
# A release directory holds the manifest at its root, this platform's binary
# beside it, the built bundle under ui/, and a checksums.txt over all of them.
# The checksum file is computed from the files that were just built.
#
# Settings, all read from the environment:
#
#   PACK_ROOT      where the releases go, bin/demo-modules by default
#   PACK_OPTIONAL  1 turns a missing tool into one line and a clean exit, which
#                  is what `make demo` needs on a machine with no pnpm
#   PACK_FORCE     1 builds even when the archives are newer than the sources
#
# The whole run is one function, called on the last line, so the shell parses
# this file to its end before executing any of it. sh reads a script while it
# runs, and an edit landing mid-run moves the read offset under the reader.
main() {

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT" || exit 1

PACK_ROOT=${PACK_ROOT:-bin/demo-modules}
PACK_OPTIONAL=${PACK_OPTIONAL:-0}
PACK_FORCE=${PACK_FORCE:-0}

SPOOLS_DIR=examples/spools
SPOOLS_CLIENT_DIR=$SPOOLS_DIR/client
SPOOLS_UI_DIR=$SPOOLS_DIR/ui
NOZZLE_DIR=examples/nozzle-watch
NOZZLE_UI_DIR=$NOZZLE_DIR/ui
OCEAN_DIR=examples/ocean-theme
GRAPHQL_DIR=packages/graphql
LIBRARY_DIR=packages/svelte
# The client SDK the spools typed client links to. A fresh clone has no dist
# under it, and the typed client cannot build without one.
CLIENT_SDK_DIR=packages/typescript

SPOOLS_RELEASE=$PACK_ROOT/spools
NOZZLE_RELEASE=$PACK_ROOT/nozzle-watch
OCEAN_RELEASE=$PACK_ROOT/ocean-theme
GRAPHQL_RELEASE=$PACK_ROOT/graphql

CHECKSUMS_FILE=checksums.txt

# CI=true keeps pnpm from asking anything, and keeps its output readable.
CI=true
export CI

# The version each release is named after is the one its manifest declares.
# `module add` refuses a release whose manifest says another version, so
# reading it here is the only way the two agree without being kept in step by
# hand.
manifest_version() {
	sed -n 's/^version:[ 	]*//p' "$1/module.yaml" | head -1
}

SPOOLS_VERSION=$(manifest_version "$SPOOLS_DIR")
NOZZLE_VERSION=$(manifest_version "$NOZZLE_DIR")
OCEAN_VERSION=$(manifest_version "$OCEAN_DIR")
GRAPHQL_VERSION=$(manifest_version "$GRAPHQL_DIR")

SPOOLS_ARCHIVE=$PACK_ROOT/spools-$SPOOLS_VERSION.tar.gz
NOZZLE_ARCHIVE=$PACK_ROOT/nozzle-watch-$NOZZLE_VERSION.tar.gz
OCEAN_ARCHIVE=$PACK_ROOT/ocean-theme-$OCEAN_VERSION.tar.gz
GRAPHQL_ARCHIVE=$PACK_ROOT/graphql-$GRAPHQL_VERSION.tar.gz

for tool in go pnpm sha256sum tar; do
	if command -v "$tool" > /dev/null 2>&1; then
		continue
	fi

	if [ "$PACK_OPTIONAL" = 1 ]; then
		echo "SKIP  $tool is not on PATH, so the example modules were not packed."
		exit 0
	fi

	echo "FAIL  $tool is not on PATH, and packing the examples needs it."
	exit 1
done

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT INT TERM

STEPS=0
step() {
	WHAT=$1
	shift
	STEPS=$((STEPS + 1))

	printf '%2d. %s...\n' "$STEPS" "$WHAT"

	if "$@" > "$WORK/step.log" 2>&1; then
		return 0
	fi

	echo "FAIL  $WHAT"
	cat "$WORK/step.log"
	exit 1
}

# stale answers whether an archive has to be built again: it is not there, or
# a source file under one of the named directories is newer than it. A built
# bundle and an installed node_modules are outputs, so neither counts.
#
# Without this every `make demo` would rebuild two browser bundles before it
# started a server, and the command people run to see this project work would
# take a minute instead of a second.
stale() {
	archive=$1
	shift

	[ "$PACK_FORCE" = 1 ] && return 0
	[ -f "$archive" ] || return 0

	for dir in "$@"; do
		found=$(find "$dir" -type f ! -path '*/node_modules/*' ! -path '*/dist/*' \
			-newer "$archive" -print 2> /dev/null | head -1)
		[ -n "$found" ] && return 0
	done

	return 1
}

SPOOLS_STALE=0
NOZZLE_STALE=0
OCEAN_STALE=0
GRAPHQL_STALE=0

stale "$SPOOLS_ARCHIVE" "$SPOOLS_DIR" "$LIBRARY_DIR/src" "packages/typescript/src" && SPOOLS_STALE=1
stale "$NOZZLE_ARCHIVE" "$NOZZLE_DIR" "$LIBRARY_DIR/src" "packages/typescript/src" && NOZZLE_STALE=1
stale "$OCEAN_ARCHIVE" "$OCEAN_DIR" && OCEAN_STALE=1
# The GraphQL module's schema is generated from the core's own document, so
# that document is one of its sources: a core that moved and a module that was
# not rebuilt would ship a schema behind it.
stale "$GRAPHQL_ARCHIVE" "$GRAPHQL_DIR" internal/transport/http/httpapi/openapi.json && GRAPHQL_STALE=1

if [ "$SPOOLS_STALE" = 0 ] && [ "$NOZZLE_STALE" = 0 ] && [ "$OCEAN_STALE" = 0 ] && [ "$GRAPHQL_STALE" = 0 ]; then
	echo "OK    all four releases are newer than their sources, so nothing was rebuilt."
	echo "      $SPOOLS_ARCHIVE"
	echo "      $NOZZLE_ARCHIVE"
	echo "      $OCEAN_ARCHIVE"
	echo "      $GRAPHQL_ARCHIVE"
	exit 0
fi

# --frozen-lockfile is a fast no-op when node_modules already matches, so this
# runs every time. Skipping it on a node_modules that merely exists leaves a
# stale tree behind a branch switch, and pnpm then refuses to build against it.
install_deps() {
	(cd "$1" && pnpm install --frozen-lockfile)
}

# The component library ships the Vite configuration both bundles build with,
# and each module reads it through the package's `build` export, which points
# at the library's dist.
if [ ! -f "$LIBRARY_DIR/dist/build/module.js" ]; then
	step "the component library builds" sh -c \
		"cd '$LIBRARY_DIR' && pnpm install --frozen-lockfile && pnpm run build"
fi

# The same for the client SDK: the spools typed client imports it through a
# workspace link, which points at a folder with no dist in a fresh clone.
if [ ! -f "$CLIENT_SDK_DIR/dist/index.js" ]; then
	step "the client SDK builds" sh -c \
		"cd '$CLIENT_SDK_DIR' && pnpm install --frozen-lockfile && pnpm run build"
fi

# built_bundle refuses a directory the host would have nothing to serve out of.
built_bundle() {
	if [ ! -f "$1/module.yaml" ] || [ ! -f "$1/ui/index.js" ]; then
		echo "FAIL  $1 is missing module.yaml or ui/index.js"
		exit 1
	fi
}

# checksums prints one line per file, in the two-column form sha256sum writes,
# over every file in the release but itself. The redirection creates the file
# before the walk runs, so a walk that did not exclude it would record the
# digest of an empty file and then overwrite it.
checksums() {
	(
		cd "$1" || exit 1
		find . -type f ! -name "$CHECKSUMS_FILE" | sed 's|^\./||' | LC_ALL=C sort | while IFS= read -r file; do
			sha256sum "$file"
		done
	) > "$1/$CHECKSUMS_FILE"
}

# archive packs one release directory. The members are named from inside the
# directory, and the directory itself is not one of them: an archive carrying a
# `./` entry is refused by the installer, which reads every name as a file
# inside the release.
archive() {
	target=$(cd "$(dirname "$2")" && pwd)/$(basename "$2")
	rm -f "$target"
	(cd "$1" && tar -czf "$target" -- *)
}

mkdir -p "$PACK_ROOT"

if [ "$SPOOLS_STALE" = 1 ]; then
	# The Go half. The file name is <name>_<GOOS>_<GOARCH>, which is what the
	# host looks for in the directory the declaration points at.
	PLUGIN_NAME="spools_$(go env GOOS)_$(go env GOARCH)$(go env GOEXE)"

	step "the spools Go half builds" sh -c \
		"go build -o '$WORK/$PLUGIN_NAME' ./$SPOOLS_DIR"

	step "the spools typed client installs" install_deps "$SPOOLS_CLIENT_DIR"
	step "the spools typed client builds" sh -c "cd '$SPOOLS_CLIENT_DIR' && pnpm run build"

	step "the spools browser half installs" install_deps "$SPOOLS_UI_DIR"
	step "the spools browser half builds into a bundle" sh -c "cd '$SPOOLS_UI_DIR' && pnpm run build"

	built_bundle "$SPOOLS_UI_DIR/dist"

	rm -rf "$SPOOLS_RELEASE"
	mkdir -p "$SPOOLS_RELEASE"
	cp -R "$SPOOLS_UI_DIR/dist"/. "$SPOOLS_RELEASE"/
	cp "$WORK/$PLUGIN_NAME" "$SPOOLS_RELEASE/$PLUGIN_NAME"
	# The host launches this file. A tar entry with no executable bit is
	# unpacked read-only, and the module would then never start.
	chmod +x "$SPOOLS_RELEASE/$PLUGIN_NAME"
	checksums "$SPOOLS_RELEASE"

	step "the spools release packs" archive "$SPOOLS_RELEASE" "$SPOOLS_ARCHIVE"
fi

if [ "$NOZZLE_STALE" = 1 ]; then
	step "nozzle-watch installs" install_deps "$NOZZLE_UI_DIR"
	step "nozzle-watch builds into a bundle" sh -c "cd '$NOZZLE_UI_DIR' && pnpm run build"

	built_bundle "$NOZZLE_UI_DIR/dist"

	rm -rf "$NOZZLE_RELEASE"
	mkdir -p "$NOZZLE_RELEASE"
	cp -R "$NOZZLE_UI_DIR/dist"/. "$NOZZLE_RELEASE"/
	checksums "$NOZZLE_RELEASE"

	step "the nozzle-watch release packs" archive "$NOZZLE_RELEASE" "$NOZZLE_ARCHIVE"
fi

if [ "$OCEAN_STALE" = 1 ]; then
	# One stylesheet and no script, so there is nothing to build: the release
	# is the example's own files.
	rm -rf "$OCEAN_RELEASE"
	mkdir -p "$OCEAN_RELEASE"
	cp "$OCEAN_DIR/module.yaml" "$OCEAN_RELEASE/module.yaml"
	cp -R "$OCEAN_DIR/ui" "$OCEAN_RELEASE/ui"
	if [ ! -f "$OCEAN_RELEASE/ui/ocean.css" ]; then
		echo "FAIL  $OCEAN_RELEASE is missing ui/ocean.css"
		exit 1
	fi
	checksums "$OCEAN_RELEASE"

	step "the ocean-theme release packs" archive "$OCEAN_RELEASE" "$OCEAN_ARCHIVE"
fi

if [ "$GRAPHQL_STALE" = 1 ]; then
	# One Go process and no bundle. The module carries its own go.mod, so the
	# build runs inside its directory and the output path is absolute.
	GRAPHQL_NAME="graphql_$(go env GOOS)_$(go env GOARCH)$(go env GOEXE)"

	# The schema is written again before the binary is built, so a release can
	# never carry a schema older than the document beside it. The module's own
	# tests refuse a committed copy that differs from what this writes.
	step "the GraphQL module's schema is regenerated" sh -c 		"cd '$GRAPHQL_DIR' && go generate ./..."

	step "the GraphQL module builds" sh -c 		"cd '$GRAPHQL_DIR' && go build -o '$WORK/$GRAPHQL_NAME' ."

	rm -rf "$GRAPHQL_RELEASE"
	mkdir -p "$GRAPHQL_RELEASE"
	cp "$GRAPHQL_DIR/module.yaml" "$GRAPHQL_RELEASE/module.yaml"
	# The schema is in the release beside the manifest, so a person who
	# installed the module can read what it serves without the repository.
	cp "$GRAPHQL_DIR/schema.graphql" "$GRAPHQL_RELEASE/schema.graphql"
	cp "$WORK/$GRAPHQL_NAME" "$GRAPHQL_RELEASE/$GRAPHQL_NAME"
	chmod +x "$GRAPHQL_RELEASE/$GRAPHQL_NAME"
	checksums "$GRAPHQL_RELEASE"

	step "the GraphQL release packs" archive "$GRAPHQL_RELEASE" "$GRAPHQL_ARCHIVE"
fi

echo "OK    all four releases are packed under $PACK_ROOT"
echo
echo "      $SPOOLS_RELEASE  and  $SPOOLS_ARCHIVE"
echo "      $NOZZLE_RELEASE  and  $NOZZLE_ARCHIVE"
echo "      $OCEAN_RELEASE  and  $OCEAN_ARCHIVE"
echo "      $GRAPHQL_RELEASE  and  $GRAPHQL_ARCHIVE"

}
main "$@"
