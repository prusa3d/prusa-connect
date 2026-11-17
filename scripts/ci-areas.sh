#!/usr/bin/env bash
# Says which areas of the tree a pull request changes, so that a job in
# .github/workflows/ci.yml whose inputs did not change can skip. ADR-20260923-xcq
# rule 1 says which jobs may skip and why.
#
# Usage:
#   bash scripts/ci-areas.sh BASE HEAD   reads `git diff --name-status -M BASE HEAD`
#   bash scripts/ci-areas.sh             reads lines of that form on standard input
#   bash scripts/ci-areas.sh --all       says that every area changed, for a run
#                                        that is not a pull request
#
# A line is a status letter, a tab and a path. A rename or a copy carries two
# paths. A line with no tab is read as a changed path.
#
# Standard output is one line per area, `name=true` or `name=false`. That is
# the form GITHUB_OUTPUT reads. Standard error names the areas each path gave,
# so a run says why a job ran.
#
# The areas:
#
#   all      a path every job reads, or a path no rule below names. Every area
#            is then true
#   go       the Go modules, and every file outside them that Go code reads
#   web      the client packages, the web app, and every file they read
#   docs     the prose: docs/ and every Markdown file
#   changes  the changelog fragments under docs/changes/
#   removed  a path was deleted or renamed. A link in the documentation can
#            name any file in the tree
#   sign_in  a path that reaches the sign-in at an identity provider
#
# A path that only the jobs that run on every change read gives no area.
# internal/arch/ci_areas_test.go runs this script. It also refuses a file that
# Go code or the web packages read when this script puts that file in another
# area.
set -euo pipefail

all=false
go=false
web=false
docs=false
changes=false
removed=false
sign_in=false

# The paths that decide whether the sign-in works. This file and ci.yml are in
# the list of paths that run everything, which sets sign_in as well.
sign_in_paths='^scripts/check-oidc-providers\.sh$
^docker/(keycloak|authentik|authelia)-fixture/
^docker/mock-oidc/
^frontend/test/oidc/
^sdk/authapi/
^adapters/auth/
^internal/platform/auth/
^internal/app/(auth|authsetup|accounts|bearer|principal|setup[a-z]*)(_test)?\.go$
^internal/app/config/setup
^internal/app/resourcehost/principal
^internal/app/builtin/adapters\.go$
^internal/authz/(bootstrap|orgrole)(_test)?\.go$
^internal/transport/http/httpapi/(auth|setup[a-z]*)(_test)?\.go$
^internal/transport/http/loginguard/
^packages/svelte/src/shell/(setup-wizard|sign-in|first-run)/
^packages/svelte/src/data/(session|setup-flow)\.
^packages/svelte/src/data/services/auth\.
^packages/svelte/src/domain/principal\.
^packages/svelte/locales/en\.(setupWizard|signIn|firstRun)\.json$
^frontend/src/app\.svelte$
^frontend/src/lib/first-run\.
^frontend/src/lib/stores/(session-expiry|signed-in)\.'

# areas_of prints the areas of one path. The first pattern that matches wins,
# so a narrow pattern comes before a wide one.
areas_of() {
	case "$1" in
	# The sign-in lane's own inputs: the script `make oidc-providers-check`
	# runs and the fixtures of the three providers. No other job reads them,
	# and the sign-in list above names each of them.
	scripts/check-oidc-providers.sh | docker/keycloak-fixture/* | docker/authentik-fixture/* | \
		docker/authelia-fixture/*)
		echo "" ;;

	# Read by every job, or deciding what a job runs: the workflow and this
	# script, the build, the shared scripts, the toolchain pins, the
	# dependency manifests and lockfiles, the fixtures, and what git and
	# Docker leave out of a checkout or an image.
	.github/workflows/ci.yml | Makefile | scripts/* | go.mod | go.sum | go.work | go.work.sum | \
		.nvmrc | VERSION | package.json | */package.json | pnpm-lock.yaml | */pnpm-lock.yaml | \
		pnpm-workspace.yaml | */pnpm-workspace.yaml | .npmrc | */.npmrc | \
		docker/* | docker-compose*.yml | .dockerignore | .gitattributes | .gitignore | */.gitignore | \
		.githooks/*)
		echo all ;;

	# The changelog fragments. The changelog gates run on every change, and the
	# documentation site checks the links in them.
	docs/changes/*)
		echo changes ;;

	# Under docs/, and read by Go code and by the client package's tests.
	docs/errors/* | docs/vocabularies/*)
		echo go web docs ;;

	# Under docs/, and read or written by Go code: the catalogues and the pages
	# the generators under cmd/ write, and the record one test reads.
	docs/wire/* | docs/extensions/* | docs/decisions/20260825-dee-* | \
		docs/src/content/docs/reference/* | \
		docs/src/content/docs/authorization-vocabulary.md | docs/src/content/docs/capabilities.md | \
		docs/src/content/docs/config.md | docs/src/content/docs/device-issues.md | \
		docs/src/content/docs/device-models.md | docs/src/content/docs/error-codes.md | \
		docs/src/content/docs/event-types.md | docs/src/content/docs/extensions.md | \
		docs/src/content/docs/plugin-wire-protocol.md | docs/src/content/docs/resource-kinds.md | \
		docs/src/content/docs/writing-a-module.md)
		echo go docs ;;

	docs/*)
		echo docs ;;

	# Read by Go code and by the web packages: the generated client, the
	# files of the component library a Go test or generator reads, the
	# modules with a Go half and a browser half, the extensions the web app's
	# Storybook and the image build, the test modules both sides load, the
	# document the client is generated from, the Fractal module's client
	# with the fragment it is generated from, and the setpoint bounds the
	# component library holds its editors to.
	packages/typescript/* | internal/printer/profile/bounds.json | packages/svelte/src/slots/* | packages/svelte/src/i18n/locale.svelte.ts | \
		packages/svelte/locales/en.printerCode.json | \
		packages/fractal/client/* | packages/fractal/openapi.json | \
		packages/svelte/src/styles/tokens.css | examples/* | extensions/* | \
		test/modules/* | internal/transport/http/httpapi/openapi.json)
		echo go web ;;

	frontend/* | packages/svelte/* | packages/standards/*)
		echo web ;;

	cmd/* | internal/* | adapters/* | sdk/* | test/* | tools/* | config/* | compose/* | deploy/* | \
		packages/fractal/* | packages/graphql/* | .golangci.yml)
		echo go ;;

	# Read only by the jobs that run on every change: actionlint and the
	# architecture tests.
	.github/*)
		echo "" ;;

	.agents/* | AGENTS.md | CLAUDE.md | README.md)
		echo docs ;;

	# A path no rule names. Nobody has said what reads it, so everything runs.
	*)
		echo all ;;
	esac
}

mark() {
	local path=$1 found
	found=$(areas_of "$path")
	case " $found " in
	*" docs "*) ;;
	*)
		case "$path" in
		docs/*) ;;
		*.md | *.mdx) found="$found docs" ;;
		esac
		;;
	esac
	if printf '%s\n' "$path" | grep -qE "$sign_in_paths"; then
		found="$found sign_in"
	fi
	echo "$path: ${found:-only the jobs that run on every change}" >&2
	for area in $found; do
		case "$area" in
		all) all=true ;;
		go) go=true ;;
		web) web=true ;;
		docs) docs=true ;;
		changes) changes=true ;;
		sign_in) sign_in=true ;;
		esac
	done
}

read_changes() {
	local status first second
	while IFS=$'\t' read -r status first second || [ -n "$status" ]; do
		[ -n "$status" ] || continue
		if [ -z "$first" ]; then
			first=$status
			status=M
		fi
		case "$status" in
		D* | R*) removed=true ;;
		esac
		mark "$first"
		if [ -n "$second" ]; then
			mark "$second"
		fi
	done
}

if [ "$#" -eq 1 ] && [ "$1" = --all ]; then
	all=true
elif [ "$#" -eq 2 ]; then
	diff=$(git diff --name-status -M "$1" "$2")
	read_changes <<<"$diff"
elif [ "$#" -eq 0 ]; then
	read_changes
else
	echo "usage: $0 BASE HEAD | $0 --all | $0 < output of git diff --name-status" >&2
	exit 2
fi

if [ "$all" = true ]; then
	go=true
	web=true
	docs=true
	changes=true
	removed=true
	sign_in=true
fi

echo "all=$all"
echo "go=$go"
echo "web=$web"
echo "docs=$docs"
echo "changes=$changes"
echo "removed=$removed"
echo "sign_in=$sign_in"
