#!/bin/sh
# The easyprint module, checked and installed.
#
#   extensions/easyprint  a module that is one browser bundle. It asks the
#                         server to answer EasyPrint's origin and adds a tab
#                         to its own page under Settings, Extensions
#
# What it does, in order:
#
#   1. builds the component library and the client SDK the bundle links to,
#      when a fresh clone has no dist under either
#   2. lints and type-checks the bundle and runs its tests in Chromium
#   3. builds the release and installs it with `module add`, which is what
#      install.sh does for a person, so the two cannot drift apart
#   4. reads the declaration the install wrote and holds it to the one origin
#      the manifest asks for
#
# The whole run is one function, called on the last line, so the shell parses
# this file to its end before executing any of it.
main() {

ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
cd "$ROOT" || exit 1

MODULE_DIR=extensions/easyprint
UI_DIR=$MODULE_DIR/ui
LIBRARY_DIR=packages/svelte
CLIENT_SDK_DIR=packages/typescript

# CI=true keeps pnpm from asking anything, and keeps its output readable.
CI=true
export CI

for tool in go pnpm; do
	if ! command -v "$tool" > /dev/null 2>&1; then
		echo "FAIL  $tool is not on PATH, and this check needs it."
		exit 1
	fi
done

WORK=bin/easyprint-check
rm -rf "$WORK"
mkdir -p "$WORK"

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

if [ ! -f "$LIBRARY_DIR/dist/build/module.js" ]; then
	step "the component library builds" sh -c \
		"cd '$LIBRARY_DIR' && pnpm install --frozen-lockfile && pnpm run build"
fi

if [ ! -f "$CLIENT_SDK_DIR/dist/index.js" ]; then
	step "the client SDK builds" sh -c \
		"cd '$CLIENT_SDK_DIR' && pnpm install --frozen-lockfile && pnpm run build"
fi

step "the bundle installs" sh -c "cd '$UI_DIR' && pnpm install --frozen-lockfile"

# The tests run in the Chromium of the bundle's own playwright, so that is the
# copy installed. On a GitHub runner, which is Linux, the libraries Chromium
# needs are installed with it.
PLAYWRIGHT_FLAGS=
if [ "${GITHUB_ACTIONS:-}" = true ] && [ "$(uname -s)" = Linux ]; then
	PLAYWRIGHT_FLAGS=--with-deps
fi
step "Chromium for the tests installs" sh -c \
	"cd '$UI_DIR' && pnpm exec playwright install $PLAYWRIGHT_FLAGS chromium"
step "the bundle lints, type-checks and its tests pass in Chromium" sh -c \
	"cd '$UI_DIR' && pnpm run check"
step "the release builds and installs into a scratch core" \
	env WORK="$ROOT/$WORK/install" sh "$MODULE_DIR/install.sh"

# The approved install is what grants the origin. A declaration without it is
# an extension that is switched on and that the server still refuses.
granted_origin() {
	grep -q '"allowed_origins"' "$WORK/install/demo/resources.json" &&
		grep -q 'https://slicer.printables.com' "$WORK/install/demo/resources.json"
}
step "the approved install grants the origin of EasyPrint" granted_origin

echo "OK    easyprint passes its gate"

}
main "$@"
