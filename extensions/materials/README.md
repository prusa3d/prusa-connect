# Materials

A Prusa Connect module with both halves: a Go process serving the
materials routes, and a browser bundle drawing on them. It keeps a catalogue of
filament materials and the spools a person has of them, in a database of its
own (SQLite or Postgres, the operator's choice at install).

- **Catalogue.** The OpenPrintTag database, downloaded from
  <https://github.com/OpenPrintTag/openprinttag-database> (a ZIP of the main
  branch, at most 64 MiB, https only) and imported in the background. The ZIP
  is written to the module's data directory, never held in memory, and removed
  once it is read. One left there by a module that stopped during an import is
  removed when the module is next configured. The
  database is released under the MIT licence; its licence travels with the
  data it describes. Load it from the Catalogue tab under Settings,
  Extensions, or with `POST /catalogue/download`.
- **Custom materials and favourites** belong to the person who made them.
- **Spools.** A person adds 1 to 100 spools of a material at once, edits them
  and deletes them. The module refuses a spool past `max_spools`. The stock
  belongs to the server and is shared by everyone who uses it. Everyone the host lets read sees
  every spool. Everyone it lets write changes or deletes any spool, whoever
  added it. A spool of a custom material shows that material's name to
  everyone.
- **Archive.** A spool used up, lost or spoiled is archived from its Edit
  dialog, with an optional reason and note. It stays in the material's spool
  list, marked Archived, but leaves the stock totals and the location counts,
  and does not count against `max_spools`. It cannot go on a printer tool, and
  one on a tool cannot be archived. Unticking Archived brings it back and
  clears the reason and note. A printer that reads its NFC tag brings it back
  too.
- **Upload ZIP** (loading the catalogue from a file) comes with a later Core.
  A module request body is capped at 1 MiB today, and a catalogue ZIP does not
  fit. Until then the module downloads it itself.

Its namespace is `com.github.aux.prusa3d.materials`, derived by the host from the repository
address `github.com/prusa3d/materials`. You never write the namespace: every address the module
answers on is built from it.

| path | purpose |
| --- | --- |
| `module.yaml` | the manifest: the kind and the version, and the `ui` block with the ceiling this bundle may register into |
| `install.sh` | build, install and see it. The one command below |
| `Makefile`, `scripts/check.sh` | the gate: lint, tests, build both halves, install into a demo server and drive it |
| `CHANGELOG.md` | what an operator sees change when they upgrade this module |
| `main.go`, `catalogue.go` | the process: the handshake, the lifecycle set, the settings and the wiring |
| `internal/assign`, `internal/printers` | the rules for changing a tool's material, and the follower of what printers report |
| `internal/stock` | the database: materials, spools, favourites, the catalogue tables |
| `internal/importer`, `internal/opt` | the download and the reading of the OpenPrintTag ZIP |
| `internal/routes` | the handlers, and `codes.go`: every code this module refuses with, each documented once |
| `openapi.json` | the fragment describing the routes, with every path relative to the mount |
| `client/` | the typed client generated from that fragment |
| `ui/src` | the bundle: the pages, the add dialog, the settings tab, and `locales/en.json`, every text it draws |

## Where it builds from

The module is one of the first-party extensions under `extensions/` in the
Core's repository. It builds against the sdk and packages of that checkout.
`go.mod` has `replace .../sdk => ../../sdk`, and `client/package.json` links
`../../../packages/typescript`. `install.sh` and `scripts/check.sh` run the
Core's tools from the checkout two levels up.

## Installation

```bash
sh install.sh
```

That builds the bundle and this platform's binary, and installs the release with
`prusa-connect module add` into the demo's data, under `bin/materials-demo`
in the Core's checkout. `TARGET=app sh install.sh` installs it into the real app's
data, `bin/materials-app`, instead. What the tools print goes to `install.log`
there, and a good run prints one line saying where the module is installed.

To see it, run this in the Core's checkout:

```bash
go run ./cmd/new-module -start demo -dir extensions/materials
```

It starts the demo on a free port with this module installed, and this
module's dev server in front of it. It prints the address to open and the
account to sign in with. `-start app` starts the real app instead. Ctrl+C stops
everything it started.

Run `sh install.sh` again after a change to `module.yaml` or to the process.
`module add` records the bytes it installed and the host serves nothing it
cannot verify against that record, so a rebuilt release is a new install. A
core that is already running read its declarations when it started, so it is
restarted too.

## Iterating

`-start` runs the module's own dev server, `pnpm dev` in `ui/`. Open the
address it prints, not the core's. Without `-start`, `pnpm dev` reads the core's
address from `PCL_CORE_URL`, `http://localhost:8080` by default.

## Tests

```bash
go test ./...                              # the Go half, on SQLite
make -C extensions check EXT=materials     # from the Core's root: lint, the Go and UI tests, and the end-to-end drive
```

The store tests run on every engine in `internal/dbtest`. Postgres is skipped
unless `AUTHZ_TEST_POSTGRES_DSN` names a server, so run the suite once with it
set before a merge:

```bash
AUTHZ_TEST_POSTGRES_DSN='postgres://user:pass@localhost/materials_test?sslmode=disable' go test ./...
```

## Configuration

`options:` in `module.yaml` names every setting this module reads, each with its
type, its default and one sentence saying what it controls.

The host applies the defaults, writes the instance's `config:` block over them
key by key, and sends the result on `plugin.configure` before the first request
arrives. A key the manifest does not name is refused before the process starts.

| key | type | default | what it does |
| --- | --- | --- | --- |
| `max_spools` | int | `2000` | How many spools the module holds before it refuses to add another. |
| `catalogue_url` | string | `https://github.com/OpenPrintTag/openprinttag-database/archive/refs/heads/main.zip` | The https address the OpenPrintTag catalogue ZIP is downloaded from. |
| `catalogue_ca` | string | `` | A PEM file the catalogue download trusts beside the system roots. |

```yaml
resources:
  sink:
    materials:
      config:
        max_spools: 500
```

`catalogue_url` and `catalogue_ca` point the module at a mirror of the
catalogue, such as one inside a network with no way out. The address must be
https. `scripts/check.sh` uses the two to serve the end-to-end run a catalogue
of its own, so the run needs no network.

## Limits of search, size and imports

- Search folds case for ASCII letters only on SQLite, whose `lower()` knows no
  more. `Prusament PLA` matches `prusament pla`, and `Šedá` matches `Šedá`,
  but `Ž` does not match `ž`. Postgres folds the whole alphabet.
- Every request and response body is at most 1 MiB, and a list answers at most
  100 rows, as does a material's spools. Adding spools answers their ids, not
  the spools. A text a person writes is at most 200 bytes, notes 2000, with
  no control character but a tab or a line break.
- The module keeps at most 100 locations, so `GET /locations` answers every
  one of them inside that bound.
- An import fills staging tables in short batches and swaps them in with one
  short transaction, so requests keep being answered while it runs. A spool
  write SQLite still finds locked is answered `BUSY`, which the
  page shows as "try again in a moment".

## Permissions

The host takes `view_resource` on the instance before a read and
`operate_resource` before a write, and `openapi.json` states that on each
operation.

The module also works as itself. An operator reads what it asks for in
`module.yaml` before `module add -approve` approves it. The install prints
every grant it wrote, and `module remove` takes them back.

- `observe`, under `permissions:`, lets the module list every printer and
  subscribe to the host's event stream. It uses both to follow what each
  printer reports of its tools.
- `operate`, under `checks:`, lets the module ask the host with
  `access.check` whether a person may operate a printer. The module never
  operates a printer as itself, so the install writes no grant for it and
  withholds nothing.

The module checks the person on every printer route. `GET /printers` and
`GET /printers/tools` list the printers the host's own device listing returns
for that person. That takes one call to the host per 200 printers. A change to a tool needs
the person to operate that printer. The module asks the host with
`access.check` and keeps each yes or no for 30 seconds per person, action and
printer. The host rate-limits that call, and the cache keeps a page of
printers inside the limit. A permission taken away therefore takes up to 30 seconds to apply. A failed question is refused and is not kept.

## Materials on printers

Each printer's page has a Tools tab. It has a tile per tool, and an MMU's slots
as tiles under one header. A tile shows the nozzle, the material's colour
circle, name and brand, and the spool with what is left of it. The module
follows the printers through the host, so the tab shows what a printer reports
and what a person assigned.

A person who may operate the printer assigns or changes the material or spool of
a tool in one picker. The header chip and the Tools table open the same one. On a
connected printer a print head offers Unload, which sends the printer the same
command as the Controls tab. The printer unloads its
active tool, so only that tool's tile offers it. An MMU has one Unload
filament for its head. A slot, and any tool of a printer that is not
connected, offers Unassign instead. The module applies these rules:

- A printing or paused printer refuses a change.
- A busy printer refuses a change to a tool that reports its material. A
  tool that does not report its material accepts the change.
- A printer in a state the rules do not trust, such as ATTENTION, refuses it.
- A tool that reports a loaded material accepts only a material that matches.
- A tool that reports itself empty refuses an assignment.
- A tool whose spool the printer read off an NFC tag refuses an assignment.
  A person may still unassign it.
- An offline printer accepts a change, because its report may be old.

`GET /printers/{device}/tools` says why a change would be refused, and the
tab shows that reason behind an info mark beside the disabled buttons.

A spool the printer read off an NFC tag shows an NFC mark. Its tile has Add to
Materials, which creates a spool with the tag's UUID and the source `printer`.
A tag that a spool already carries adds nothing. A manual spool of the same
material can be merged into it. The printer's spool keeps its UUID and takes
the location and the notes of the other spool where it has none. A spool on a
tool cannot be deleted or ticked until it is unassigned. Its notes and its
location still change, but not its weight or its archive: the printer counts
its weight down.

A printer can leave the host, or lose a tool, while the module is not following
it. Its spools then stay on a tool that is gone, and so does a deleted
printer's assignment. The row of a spool on a tool that is gone also has
Unassign. For a printer the host no longer lists, write access to Materials is
enough. A printer the host still lists needs a person who may operate it.

The row of a spool on a tool that is still there offers what the Tools tab
offers on that tool, by the same rules. Unload is on the print head a connected
printer would unload. Unassign is on a slot, or on any tool of a printer that
is not connected. A refused one is disabled, with the reason behind an info
mark, such as while the printer prints or is paused.

The Tools tab and a material's page follow the module's own `tool`
and `spool` events on the host's event stream. A spool's weight drops as it
prints, without a reload.

A spool on a tool loses the filament the tool feeds while it is on it: prints,
loads, unloads and purges all count. The module reads each tool's odometer,
the millimetres fed since the printer started. It weighs them with the spool's
density and diameter, else the density for the type and 1.75 mm. A reading
lower than the last one, or a new odometer id, means the printer restarted,
and counts nothing. While the tool reads the spool's own NFC tag, the tag's
used weight is what the spool lost. Firmware that does not report an odometer
per tool is not counted.

| route | purpose |
| --- | --- |
| `GET /printers` | the printers the person may observe |
| `GET /printers/{device}/tools` | the tools of one printer, with why a change would be refused |
| `PUT /printers/{device}/tools/{tool}` | assign a material or a spool to a tool |
| `DELETE /printers/{device}/tools/{tool}` | unassign a tool |
| `POST /printers/{device}/tools/{tool}/spool` | add the spool the printer read off its NFC tag |
| `POST /spools/{id}/merge` | merge a manual spool into the spool a printer read |

## For other extensions

Another extension reads Materials and is told when it changes, through the
host and without a person's token. Materials depends on no extension that
uses it. Farm is the first extension that does.

Its `module.yaml` suggests Materials and asks to call it. The operator
approves both at install:

```yaml
suggest:
  com.github.aux.prusa3d.materials: "The material and the spool on each printer"
calls:
  - com.github.aux.prusa3d.materials
```

It reads with `routes.call` (`sdk/routesapi`), as itself or for the person it
serves. Materials shows a printer, and the printer a spool is on, only where
the host says the caller may observe it. Without Materials installed, a call is
refused and the extension goes on without it.

| route | what another extension reads |
| --- | --- |
| `GET /printers/tools` | every tool of a page of printers: material, brand, colour family, spool, remaining and full weight |
| `GET /spools` | every spool, paged, or one material's with `material`; an archived one carries `archived_at` |
| `GET /materials?in_stock=true` | the materials with a spool in stock |
| `GET /materials/{uuid}` | one material's stock: spools, those on a printer, grams left |

It is told on the host's event stream (`bus.subscribe`):

| type | key | payload |
| --- | --- | --- |
| `com.github.aux.prusa3d.materials.tool` | `<device_id>/<tool_id>` | `device_id`, `tool_id`, `material_uuid`, `spool_id` |
| `com.github.aux.prusa3d.materials.spool` | `spool/<id>` | `spool_id`, `spool_uuid`, `material_uuid`, `remaining_weight_g`, `deleted` |

An event says what changed, and the routes say the rest. A burst of changes to
one key is one event. A spool that only lost weight while printing is told of
once per whole gram. Delivery is best-effort: an extension that resumes past
the stream's replay window reads the routes again.

The stream is not filtered by printer. Whoever may read Materials receives the
`tool` event of every printer: its device id, the tool and the material on it.
That includes printers the host does not let them see. The host settles an
event's origin, so a module cannot scope one to a printer. The routes do
filter: `GET /printers/tools` answers only the printers the caller may see.

These routes, event types and payloads are the extension's public contract. A
change that breaks them takes a new major version.

## Errors

When the module cannot do what a request asks, it answers with an error. The
error has a `code` and a `title` in English. Every code of this module starts
with `materials/`.

A program that calls the module should check the `code`. The title is for
people. You can change the wording of a title without breaking a caller.

`internal/routes/codes.go` lists every code, with what went wrong. Add a new
code in that file and in the `Refusal` enum of `openapi.json`; a test holds the
two together.

## What the pieces are

The bundle's build writes `ui/dist`, and that directory is already the release
layout: `module.yaml` at its root and the entry at `ui/index.js`, with the
binary `materials_<GOOS>_<GOARCH>` beside them. `-from ui/dist` is what the install
reads. A `checksums.txt` is optional and verified when it is there; a release
you just built has nothing to prove against.

```bash
prusa-connect module add github.com/prusa3d/materials@0.1.0 -from ui/dist
prusa-connect module list
prusa-connect module remove github.com/prusa3d/materials
```

Without `-approve` the declaration is written switched off, and the command
prints what the module asked for so an operator can read it first.

## The ceiling

`contributes` in `module.yaml` is the whole of what this bundle may register.
A `host.register` call the block does not declare throws, and the host drops
this module whole. The generator wrote both sides from one answer:

- `pages: materials` ← `host.register.page('materials', Page)`
- `pages: material` ← `host.register.page('material', Material)`
- `settings: catalogue` ← `host.register.settings('catalogue', Settings)`

Add one to the manifest and the other to the entry, or neither.

## Styling

`ui/src/styles.css` carries four directives and only the third emits anything.
They let this module name any Tailwind utility the library's theme defines,
without shipping Tailwind's preflight into the host page. The page behind them
is `styling-a-module` in the core's documentation.

## Texts

Every text a person reads is a key in `ui/src/locales/en.json`, read with
`host.t('<key>')`. The host merges that file under this module's namespace, so
a key here may be one the core already names. Copy the file to `<tag>.json` to
translate it.

## Type the routes in the browser

`client/` is the typed client generated from the fragment. Regenerate it after
the fragment changes, as `client/README.md` says:

```bash
prusa-connect-generate-client-sdk -module \
  -document openapi.json -namespace com.github.aux.prusa3d.materials \
  -name materials-client -out ./client
```

The bundle imports `bind` from that package and calls
`bind(host.client).listMaterials()`. There is no `fetch` and no token in a
module's browser half: the credential is the session the person already has.

## Troubleshooting

| what you see | what it means |
| --- | --- |
| `module add` refuses the release | `ui/dist` is not the release layout. `module.yaml` sits at its root and the entry at `ui/index.js` |
| the module is listed and switched off | `module add` ran without `-approve`. Read what it printed, then switch the instance on |
| the sidebar entry is missing | the app refused the bundle. The browser console names each registration `contributes` does not declare |
| a screen shows a key instead of a sentence | the key is missing from `ui/src/locales/en.json` |
| a response carries a code and an English sentence | the process refused the request. Look the code up in `internal/routes/codes.go` |
| a change does not show | the install was not run again. Every change is a new install |
