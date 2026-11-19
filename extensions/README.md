# First-party extensions

Each folder here is an extension that Prusa Research ships through the
marketplace. It is versioned and released apart from the core. It builds
against the in-repo `sdk/` and the packages under `packages/`, so a change to
either is tested against it before it merges.

| folder | what it is |
|---|---|
| `parts/` | the parts catalogue: a module with a Go half and a browser half. See [its README](parts/README.md) |
| `fleet-stats/` | job KPIs, a utilization heatmap and a failure breakdown read from history: a browser bundle alone. See [its README](fleet-stats/README.md) |
| `threemf/` | 3MF projects: a reader other modules import, file reads for the core, and the project view. See [its README](threemf/README.md) |
| `buddy-debug-logs/` | Prusa printer logs: what printers send over UDP, with a Go half and a browser half. See [its README](buddy-debug-logs/README.md) |
| `server-slicing/` | slices 3MF projects into G-code with the PrusaSlicer command line, with a Go half and a browser half. See [its README](server-slicing/README.md) |
| `materials/` | OpenPrintTag materials and the spools kept of them: a Go half and a browser half. See [its README](materials/README.md) |
| `prusaslicer2-connector/` | PrusaSlicer 2 prints on each printer here as on a printer with PrusaLink. See [its README](prusaslicer2-connector/README.md) |
| `prusaslicer3-connector/` | lets PrusaSlicer 3 in offline mode use this server as Prusa Connect. See [its README](prusaslicer3-connector/README.md) |
| `simple-queue/` | a print queue for each printer, with a Go half and a browser half. See [its README](simple-queue/README.md) |
| `easyprint/` | lets EasyPrint on Printables call the server from a browser: a browser bundle alone. See [its README](easyprint/README.md) |

## Who maintains them

Every extension here is official: Prusa Research maintains it and publishes
it to the marketplace under its own name. `.github/CODEOWNERS` asks the
maintainers for a review of every change under `extensions/`. GitHub does not
make that review a condition of the merge, so the reviewer of a pull request
checks that a maintainer approved it.

A fix or an improvement to an extension here is welcome as a pull request.
A new extension from outside Prusa Research is not accepted here. Its author
keeps it in a repository of their own and publishes it to the marketplace.
The marketplace review and gates apply to it there, and its author maintains
it.

## Gates

The extensions stay out of the root `Makefile` and out of
`.github/workflows/ci.yml`. A core pull request does not wait for them.
`extensions/Makefile` holds their gates. It finds every folder holding a
`module.yaml` and runs its `check` target:

```sh
make -C extensions check            # the shape, then every extension's gate
make -C extensions check EXT=parts  # the shape, then one extension's gate
make -C extensions lint             # golangci-lint over each extension
make -C extensions shape            # the shape alone
```

`.github/workflows/extensions.yml` runs them. It starts for a change to
anything an extension gate builds or reads:

| path | why the gate needs it |
|---|---|
| `extensions/`, `sdk/` | the extensions and the contracts they build against |
| `packages/svelte/`, `packages/typescript/`, `packages/standards/` | the browser libraries a bundle links to |
| `internal/`, `cmd/`, `go.mod`, `go.sum` | the core and the demo a gate builds and installs into |
| `.nvmrc`, `Makefile`, `.golangci.yml` | the Node version, the golangci-lint pin and its configuration |
| `.github/workflows/extensions.yml` | the workflow itself |

It also runs every night, because a core change can break an extension through
a path the filter does not list. It is not part of the core's `CI OK`.

The workflow first checks the shape and lists the folders holding a
`module.yaml`. Then it runs `make -C extensions check EXT=<name>` for each
one in a job of its own, `Extensions / <name>`, all at the same time. A new
extension gets its job without an edit to the workflow. The version check
runs in `Extensions / Versions`. `Extensions / Check` passes only when the
shape, the version check and every extension's gate passed.

That workflow is the only one allowed a path filter.
`internal/arch/changefilter_test.go` names it and refuses one anywhere else.

## The shape of an extension

`scripts/check-shape.sh` holds every extension to one layout, and `check` runs
it first. Every extension has these files:

| file | what it holds |
|---|---|
| `module.yaml` | the manifest, with a `version` |
| `CHANGELOG.md` | a `## <version> - YYYY-MM-DD` heading per release, newest first. The top one is the `module.yaml` version |
| `README.md` | what the extension is and how to run its gate |
| `Makefile` | `include ../extension.mk`, and a `check` target running everything its gate needs |

An extension that keeps more than a few settings asks for a database of its
own with `needs.database` in `module.yaml`, as `parts/` does. The operator
chooses where it goes at install. See
[a database of its own](../docs/src/content/docs/writing-a-module.md#a-database-of-its-own).

`extension.mk` gives each extension a `lint` target running golangci-lint over
its Go module. It reads the version from `GOLANGCI_VERSION` in the root
`Makefile`, so the repository pins it in one place. An extension with no Go module sets
`NO_GO_MODULE = 1` before the include. A folder here with no `module.yaml` is
refused, because no walk would ever run its gates.

## Where a change is recorded

A change to an extension goes into that extension's `CHANGELOG.md`, in the same
commit. The core does not ship the extension, so `docs/CHANGELOG.md` has no
entry for it, and the commit carries `Changelog: none`. A commit that also
changes the core adds the core's entry as usual. Rule 14 of
[changelog discipline](../.agents/rules/changelog-discipline.md) says the same.

The newest heading is always a release heading, `## 0.1.0 - 2026-10-05`, of
the version `module.yaml` declares. `scripts/check-shape.sh` and the
extension's own tests refuse any other heading at the top. So a change after a
release adds a new version to both files.

A pull request that changes what an extension ships, anything but Markdown,
tests and its `scripts/`, has to raise that `version`.
`scripts/check-version-bump.sh` refuses it otherwise, and the workflow runs it
on every pull request. The server image carries the extensions, and a server
that already has one is offered the image's copy only as an update, which
needs a new version.

## In the demo

`make demo` offers every extension here on its extensions page, next to the
example modules. `scripts/pack-demo-extensions.sh` builds each one and lays it
out with `scripts/pack-release.sh`, under `bin/demo-modules/extensions`. It is
best effort and outside CI: an extension that does not build prints a warning
and is skipped, and the demo starts without it. So the demo is not a gate for
an extension. Its own gate is.

## In the server image

`docker/server/Dockerfile` builds every extension and puts its release in
`/opt/modules/<folder>`. A release with a Go half is laid out by
`scripts/pack-release.sh`, the script its own gate packs with. An operator
offers them by naming them in `core.modules_local`; see
[bundled extensions](../docs/src/content/docs/container.md#bundled-extensions).
A new extension adds its build to the Dockerfile.

## Releasing to the marketplace

A person starts each release. In GitHub, open Actions, then **Release an
extension**, then **Run workflow**. The workflow
`.github/workflows/extension-release.yml` takes three inputs:

| input | what it sets |
|---|---|
| `extension` | the folder to release, from the list in the workflow's own `options` block |
| `channel` | `stable` or `beta`, the marketplace channel |
| `dry_run` | builds, signs and packs, then stops. Nothing is uploaded and no tag is pushed |

The version is the one in `module.yaml`. The top heading of `CHANGELOG.md` must
name the same version, and the text under it goes to the marketplace as the
release notes.

The workflow has three jobs:

1. **check** refuses a version that already has its tag and runs the
   extension's gate, `make -C extensions check EXT=<name>`.
2. **publish** calls the marketplace's builder,
   [`prusa3d/prusa-connect-marketplace-publish`](https://github.com/prusa3d/prusa-connect-marketplace-publish)
   (`publish.yml@v1`). It builds one binary for every platform in
   `module.yaml` (`go build -trimpath`, no cgo). It builds the bundle in a job
   of its own with no secret. Then, in the GitHub environment
   `marketplace-production`, it signs `checksums.txt` and records a build
   provenance attestation. It uploads the release with its source, using an
   OIDC token from GitHub. OIDC is a standard way for a service to check which
   repository and workflow sent a request. The marketplace reviews the source
   and checks that every binary was built from it.
3. **tag** tags the commit `extensions/<name>/v<version>` once the release is
   uploaded.

The marketplace does not publish the release by itself. It waits in review
until a person at the marketplace approves it. The summary of the run links to
that review.

### The one-time setup

The workflow needs three things that only an owner of the repository and of
the marketplace author account can set up:

1. The environment `marketplace-production` in the repository settings. A
   required reviewer on it makes a person approve each upload before the
   publish job starts.
2. In that environment, the secret `PRUSA_CONNECT_EXTENSION_SIGNING_KEY`. It
   holds the private key of the Prusa Research author account, as PEM text.
   This is the RSA key whose public half the marketplace holds for that author.
3. In the marketplace author console, a trusted publisher for each extension.
   It names the extension id `com.github.aux.prusa3d.<name>`, the
   repository `prusa3d/prusa-connect`, the workflow
   `extension-release.yml` and the environment `marketplace-production`. The
   marketplace refuses an upload whose OIDC token names anything else.

The repository must be public: GitHub records the attestation only for a
public repository, and the builder stops before the upload without it.

### How to release without the workflow

Without the workflow, build the release, sign and pack it with
`publish-release.sh --dry-run`, and upload the archive in the author console:

```sh
make -C extensions check EXT=parts
sh extensions/scripts/build-release.sh extensions/parts bin/release/parts
```

Run `publish-release.sh` with no arguments to see its flags.

## Adding an extension

1. Make a folder with its own `go.mod`. Point `replace` at `../../sdk`.
2. Add the four files above. Its `check` target runs `lint` and its own
   end-to-end gate, as `parts/Makefile` does.
3. Run `make -C extensions check EXT=<name>`.

Neither `extensions/Makefile` nor the workflow needs an edit. If the extension
has a `pnpm-lock.yaml`, the workflow's cache already covers it. A gate with
browser tests installs the Chromium of its own playwright, as
`parts/scripts/check.sh` does.
