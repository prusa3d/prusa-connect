# Repository map

This page tells you where things are. It covers the folders of the
repository, the documents under `docs/` and who they are for, and the commands
you type. If the page and the tree disagree, the tree is right and the page
needs a fix.

## Where to start

| you want to | read |
|---|---|
| read the tree for the first time | [the onboarding pages](src/content/docs/onboarding/index.md) |
| see the whole system in one picture | [architecture.md](src/content/docs/architecture.md) |
| read the core file by file | [the internals chapter](src/content/docs/internals/the-application-core.md) |
| write a printer driver | [writing-a-driver.md](src/content/docs/writing-a-driver.md) |
| write a module | [writing-a-module.md](src/content/docs/writing-a-module.md) |
| know the rules that bind the code | [AGENTS.md](../AGENTS.md) and `.agents/rules/` |
| know what is left to build, or ask a question | the GitHub issues |
| know what a decision says | [the decision records](decisions/README.md) |
| know what changed for a person who runs the server | [the changelog](CHANGELOG.md) |
| know what the next release ships | [the pending changes](changes/README.md), one file per change |

## Top level

| path | what it is |
|---|---|
| `cmd/` | one directory per binary. Only `main` packages are here |
| `compose/` | the entry point every binary starts through, so a build file can add adapters to it |
| `sdk/` | a separate Go module with the contracts an extension compiles against. It imports only the standard library |
| `internal/` | the application, grouped by role. See below |
| `adapters/` | the extensions that ship: identity, cameras, the echo driver, databases, event sinks and storage. They import `sdk/` and never `internal/` |
| `extensions/` | the first-party extensions, gated in their own workflow. See [extensions/README.md](../extensions/README.md) |
| `packages/` | what is published on its own: the TypeScript client, the Svelte component library, the Fractal module, the GraphQL module, and the lint configuration the others share |
| `frontend/` | the web application. The full build embeds it and serves it at `/` |
| `examples/` | extensions that run and can be copied: adapters, modules, a wire client, a Fractal tree |
| `tools/` | programs that are not the product: the printer and camera simulators, the firmware emulator lane, the upgrade compatibility fixture, and the API and vocabulary compatibility checks |
| `test/` | tests of the whole server: `e2e/`, `fleet/`, `firmware/`, `modules/`, `moduledatabase/`, `upgrade/`, `upgradeprocess/` and `compose/` |
| `scripts/` | the shell scripts behind the `make` checks |
| `config/` | the shipped default configuration and example profiles |
| `docker/` | one directory per service: its definition, its local overrides, its Dockerfile |
| `deploy/kubernetes/` | the manifests for one instance |
| `deploy/systemd/` | the systemd unit that runs a release binary as a service on Linux |
| `docs/` | every document. See the two tables below |
| `.agents/rules/` | the binding rules |
| `.githooks/` | the hooks that `make install-hooks` installs |
| `.github/` | the CI workflows |

## internal, by role

Each group may only import what its row allows. The tests in `internal/arch`
check the real imports against this table.

| group | holds | may import |
|---|---|---|
| `kernel/` | primitives with no business meaning: identity, conditions, device issues, logging, version | nothing internal |
| `connector/` | the xBuddy driver for Prusa printers, and the client that asks a printer to pair at its own screen | kernel and one domain |
| `transport/` | inbound: HTTP, the event stream, WebRTC | kernel and one domain |
| `platform/` | identity backends and observability | kernel and one domain |
| `extension/` | the host that runs extensions: registry, supervisor, lifecycle, the plugin wire | kernel |
| `app/` | the composition root: configuration, construction, lifecycle | every group |
| `arch/` | the tests of this repository's own rules | nothing internal |
| every other folder | one domain each: `authz`, `betaportal`, `codegen`, `dashboard`, `discovery`, `docsref`, `printer`, `printfile`, `timeserver`, `upgrade` | kernel, and never another domain |

Two `kernel/` packages hold the operating system's side of a module's
memory budget. `kernel/procgroup` starts a child as a group it can kill and
measure. `kernel/sysmem` reads the machine's memory and its container limit.

When two domains need the same code, it goes into `kernel`. When an adapter
and an internal package need the same code, it goes into `sdk/`.

## The documents in docs

| path | who reads it |
|---|---|
| [src/](src/content/docs/index.mdx) | the published documentation site. A person installing, using or extending the server reads it |
| [decisions/](decisions/README.md) | the decision records, one file per decision. Anyone who wants the rule that binds a piece of code |
| [CHANGELOG.md](CHANGELOG.md) | what changed for a person who runs the server, by release |
| [changes/](changes/README.md) | the news no release has shipped yet, one file per change. A release files it into the changelog and deletes it |
| `vocabularies/`, `wire/`, `errors/`, `extensions/` | the generated closed sets, as data. A generator writes them and a check compares them with the code |
| `scripts/` | the checks the documentation site runs on itself |

## Commands

`make help` prints every target with one sentence about it. These are the ones
you will type most:

| command | does |
|---|---|
| `make run` | build the core and run it in the foreground |
| `make demo` | run the core with simulated printers and the web app, for a machine with no hardware |
| `make build` | build `bin/server` with the features the configuration asks for |
| `make test-unit` | the unit suite of every module |
| `make lint` | golangci-lint over every Go module |
| `make ci-local` | every CI job that can run on this machine, and a list of what it skipped |
| `make style-check` | the writing rules, over the documents, the comments and the Makefile |
| `make docs-check` | build the documentation site and check its links and its tables |
| `make docker-check` | the runtime image runs unprivileged and the stack comes up |
| `make install-hooks` | make git run the checks before a commit and before a push |

[build-gates-and-testing.md](src/content/docs/internals/build-gates-and-testing.md)
explains which checks run on your machine and which only run in CI.
