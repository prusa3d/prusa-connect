#!/bin/sh
# EasyPrint, from source to installed on a core of the checkout.
#
#   sh install.sh             build it and install it into the demo's data
#   TARGET=app sh install.sh  install it into the real app's data instead
#
# Each step prints its number as it starts, from 1, and a good run ends with
# one line naming where the module is installed. Getting any one of them
# wrong shows nothing on the screen and says nothing, so they are written down
# once, here:
#
#   - the component library in the checkout is built, if it has not been
#   - this module's dependencies are installed and the bundle is built. The
#     build writes ui/dist, which is already the release layout: module.yaml
#     at its root and the entry under ui/
#   - `module add` verifies the release, records its digest, and writes the
#     declaration into a machine-owned overlay of this module's own
#
# What the tools print goes to $LOG. The end of that file is shown when a step
# fails, and nothing of it otherwise.
#
# `go run ./cmd/new-module -start demo -dir <this folder>`, run in the
# checkout, starts the core with this module and the module's dev server in
# front of it. The dev server answers the bundle from source, so run this file
# again only when module.yaml changes: the core reads what a module
# contributes when it is installed.
set -e

HERE=$(cd "$(dirname "$0")" && pwd)
# The module lives at extensions/easyprint inside the checkout it builds against.
CHECKOUT=$(cd "$HERE/../.." && pwd)

NAME=easyprint
MODULE=github.com/prusa3d/easyprint
# `module add` refuses a release whose manifest names another version, so the
# version is read from the manifest rather than kept in step by hand.
VERSION=$(sed -n 's/^version:[ 	]*//p' "$HERE/module.yaml" | head -1)

# Everything this writes goes under the checkout's bin/, which is ignored, and
# that is where a build artefact belongs. The demo keeps its records in demo/
# under $WORK. The real app keeps them in $WORK itself, beside its
# configuration file.
# WORK names another directory, as the extension's gate does to install into
# a scratch core of its own.
TARGET=${TARGET:-demo}
WORK=${WORK:-$CHECKOUT/bin/$NAME-$TARGET}

case "$TARGET" in
demo) DATA=$WORK/demo ;;
app) DATA=$WORK ;;
*)
	echo "FAIL  TARGET is demo or app, not $TARGET."
	exit 1
	;;
esac

CONFIG=$WORK/config.yaml
RELEASE=$HERE/ui/dist
LOG=$WORK/install.log

# CI=true keeps pnpm from asking anything.
CI=true
export CI

for tool in go pnpm; do
	if ! command -v "$tool" > /dev/null 2>&1; then
		echo "FAIL  $tool is not on PATH, and this needs it."
		exit 1
	fi
done

mkdir -p "$WORK" "$DATA"
: > "$LOG"

STEP=0

# step prints the number of the step that starts now, and what it does.
step() {
	STEP=$((STEP + 1))
	echo "$STEP. $*"
}

# quietly runs one command and adds what it prints to $LOG. When the command
# fails, the end of the log is printed and the install stops. pnpm warns on
# every run when ~/.npmrc names an environment variable that is not set, and
# that line says nothing about this module, so it is left out.
quietly() {
	if ! "$@" >> "$LOG" 2>&1; then
		echo "FAIL  $* failed. The end of $LOG:"
		grep -v 'Failed to replace env in config' "$LOG" | tail -n 30
		exit 1
	fi
}

# The component library ships the Vite configuration this bundle builds with,
# and the module reads it through the package's `build` export, which points at
# the library's dist.
if [ ! -f "$CHECKOUT/packages/svelte/dist/build/module.js" ]; then
	step "the component library builds..."
	quietly sh -c "cd '$CHECKOUT/packages/svelte' && pnpm install --frozen-lockfile && pnpm run build"
fi

step "$NAME installs and builds into a bundle..."
# pnpm runs inside each project, not with --dir: corepack picks the pnpm
# version from the package.json of the directory it starts in, and a version
# other than packageManager names refuses to run.
quietly sh -c "cd '$HERE/ui' && pnpm install && pnpm run build"

if [ ! -f "$RELEASE/module.yaml" ] || [ ! -f "$RELEASE/ui/index.js" ]; then
	echo "FAIL  ui/dist is missing module.yaml or ui/index.js, so there is nothing to install."
	exit 1
fi

# The operator's file. It names where the machine-owned overlay goes, where
# modules are installed and where the authorization graph is, and nothing else.
# It is written once and kept, so a second run installs over the first rather
# than beside it.
#
# The graph is under $DATA because that is where the demo keeps its own,
# whatever this file says. A graph named anywhere else is one the install
# writes the grants of your module into and the core never reads.
if [ ! -f "$CONFIG" ]; then
	cat > "$CONFIG" <<- YAML
		core:
		  overlay: $DATA/resources.json
		  modules: $WORK/modules
		  authz:
		    database: $DATA/config.d/authz.db
	YAML
fi

# A file written by an earlier run names no graph, so the install would write
# the grants of your module into a database beside the file and the core would
# read its own and find nothing. The file is yours, so this says what to add
# rather than editing it.
if ! grep -q 'database:' "$CONFIG"; then
	echo
	echo "NOTE  $CONFIG names no authorization graph, so the grants this install"
	echo "      writes go to a database the core does not read. Add these two lines"
	echo "      under core: and run this again:"
	echo
	echo "        authz:"
	echo "          database: $DATA/config.d/authz.db"
	echo
fi

step "$MODULE@$VERSION installs..."
# -approve writes the permissions the manifest asks for into the declaration,
# grants your module what those permissions name, and switches the instance on.
# That is what an operator does after reading what the module asked for, and
# this is your own module on your own machine, so it is approved here.
quietly go -C "$CHECKOUT" run ./cmd/prusa-connect module add "$MODULE@$VERSION" \
	-config "$CONFIG" -from "$RELEASE" -approve

echo "OK    $NAME is installed in $WORK"
