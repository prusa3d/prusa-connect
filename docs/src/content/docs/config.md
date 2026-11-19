---
title: Configuration
description: "The one YAML file the server reads: where it comes from, the smallest file that starts a server, and every key with its type and its default."
audience: run
rewrite:
  describes: current
  implementedBy:
    - internal/app/config
    - internal/app/config/build
    - cmd/generate-config-reference
---

The server is configured with one YAML file. It reads no environment
variables of its own, and the flags are a handful of overrides listed at the
end of this page. The setup wizard writes a second file beside yours, read
under it, so a person who never writes a file still configures every part.

The file has three top-level blocks and no others:

| block | what it holds |
|---|---|
| `core` | the listener, the log, the web app, the login and the limits |
| `persistence` | the database engine every part of the server shares |
| `resources` | the storage, sink and other instances a deployment declares |

## Where the file is read from

The path comes from `--config`, and it is `config/config.yaml` when the flag is
left out:

```bash
prusa-connect --config /etc/prusa-connect/config.yaml
```

A file that is not there is the empty file. Every key has a working default,
so a first run with no file at all starts a server, and that server opens the
setup wizard.

The wizard writes its answers to `config.d/setup.yaml` beside your file. The
server reads that file under yours: a key you write wins, a block is merged
key by key, and a list or a single value is replaced whole. The wizard's file
holds only `core` and `persistence`, and a key it does not read is refused
with the file and the line, as in yours.

The server also writes. Beside your file it creates a directory named
`config.d/` and keeps these there:

- `setup.yaml`, the setup wizard's answers
- `resources.json`, what the resource API writes when somebody adds a printer
  or a camera through the interface, and the first run's choices
- `authz.db`, the whole database, when `persistence.type` is `sqlite`
- `certs/`, the certificates the setup wizard made, when it made any
- `history.db`, each printer's history, when the setup wizard keeps it
- `first-run.json` and `first-run-failure.json`, the wizard's decision while
  the server restarts on it, and the last start that failed on it
- `accounts.json`, an account file from an earlier build. The server imports
  it once and does not write it
- `modules/`, the directory an installed module lives in
- `printer-thumbnails/`, the pictures printers read out of their own print
  files, so a restart does not ask every printer again. It holds at most
  `core.device_thumbnail_disk_bytes`, and is safe to delete. With
  `core.device_thumbnail_disk` off it is not created

That directory belongs to the server. It never rewrites the file you wrote, so
the comments in yours survive.

`resources.json`, `authz.db` and `modules/` can be moved with a key of their
own. The thumbnails go in the directory that holds `resources.json`. The
wizard's files always go in `config.d/` beside your file, and no key moves
them. A deployment whose configuration file sits on a read-only mount has to
move the others, and needs `config.d/` writable for the wizard.

## The smallest file that starts a server

```yaml
core:
  api:
    addr: ":8080"

resources:
  storage:
    local:
      type: filesystem
      config:
        root: ./storage
```

That is a server on port 8080 with a place to put uploaded files. Everything
else is a default. The test suite loads a copy of this text.

Such a server first serves only the setup wizard, which asks whether people
sign in. Choose that nobody does, and every request is served as the subject
in `core.principal`, which is `operator`. Keep a server like that away from
anyone you do not trust.

## A key this build does not read is refused

Writing a key the server does not know stops it at startup, and the message
names the key and the line of your file it is written on:

```
config: invalid configuration: /etc/config.yaml: line 8: core.pricipal: this build does not read that key
```

That is deliberate. A typo that was ignored would leave a subsystem
unconfigured, and the first sign of it would be an outage weeks later.

A block inside `resources:` is the one exception. An instance that will not
load fails by itself: the server starts, the other instances run, and the
failure is reported for that instance alone. One wrong block must not take a
print farm down.

## core

`core` is the server itself. Every key has a working default, so the whole
block can be left out.

```yaml
core:
  api:
    addr: ":8080"
    request_timeout: 30s
  log:
    level: info
    format: text
  frontend:
    title: "Workshop printers"
  principal: operator
  site: workshop-a
  shutdown_timeout: 15s
```

A few keys need more than a sentence.

`core.api.stream_timeout` is the ceiling over a response that is a stream or a
file: the event stream, the camera stream, the printer transfers and the
storage content routes. Every other route gets `core.api.request_timeout`
instead, and over that budget the server stops waiting for its own handler and
answers 504. The stream budget may not be shorter than the request budget, and
a file saying otherwise is refused.

`core.api.allowed_origins` decides which other web pages a browser will let
call this API. Empty means only the pages this server serves itself. Each entry
is one exact origin as a browser writes it, such as `https://printers.example`.
That form is lowercase, has no path and no trailing slash, and names a port
only when it is not the scheme's default. A `*` is refused, because the API
takes a credential and no browser sends one to an origin that answers every
page.

`core.trusted_proxies` is the list of networks a reverse proxy reaches this
server from. Only a request arriving from one of them has its client address
read out of `X-Forwarded-For`. Empty is right for a server reached directly.
Leaving it empty behind a proxy is the other mistake: every client is then
counted under the proxy's own address, so one person getting a password wrong
slows down everybody.

`core.frontend.enabled` is read twice. `make build` reads it out of the same
file to decide whether the web app is compiled at all, and the server reads it
to decide whether to serve it. With the key off, the root address answers 404,
which is what a deployment putting its own page in front of this API wants.
The API reference at `/docs` is a different thing: it is in every build except
the printer build, and is served either way.

`core.connectors` holds the printer dialects this server mounts, keyed by a
name you choose. Leave the block out and the xBuddy dialect is mounted with its
own defaults, and its printers dial the address in `core.api.addr`. Write a
socket under a connector and printers dial that one instead:

```yaml
core:
  connectors:
    xbuddy:
      type: xbuddy
      config:
        listener:
          addr: ":443"
          tls:
            certificate: certs/server/printer.crt
            key: certs/server/printer.key
          handshake_timeout: 60s
```

Everything under `config:` belongs to the dialect and not to the server, and
the server carries the block across without reading inside it. The second
socket serves only the paths a device dials and answers 404 to everything else,
so a printer network reaches the printers' wire and nothing more. The
certificate must be ECDSA: the firmware offers one cipher suite and that suite
needs an ECDSA key. See [Custom certificates](./certificates.md).

A connector is mounted when the server is built, so a change to this block
takes effect at the next start.

## persistence

One engine carries the whole database: the permission graph, the device records
and the command queue share one connection pool. It sits at the top level
rather than inside `core:` because `make build` reads the same key to decide
whether the Postgres driver is linked in. Writing it inside `core:` is
refused, so that one file cannot name two engines.

```yaml
persistence:
  type: postgres
  max_open_conns: 16
  postgres:
    dsn: "postgres://user:secret@db.example:5432/prusa?sslmode=require"
```

The connection string is handed to the driver unread, so both forms work: a
`postgres://` URL and a `key=value` string. The password never reaches a log
line.

Two refusals are worth knowing before a deployment meets them. `memory` is not
an engine here: a permission graph in memory forgets every grant on restart.
And a binary built from a SQLite profile refuses to start on a Postgres
connection string, with a message naming the build tag it needs.

## resources

A resource is something the server runs on your behalf and can start and stop
while it is running. A storage backend, an event sink, a history store and a
link to another server are all resources. Each one is a block under its kind
and then under a name you choose.

A camera is not declared here. It is a device kept in the server's database,
beside the printers: add one on a printer's Cameras tab, or with
`PUT /api/v1/cameras/<name>`. An older version read cameras from a
`resources.camera` block. The first start of this version moves that block into
the database, and does not read it after that.

```yaml
resources:
  storage:
    local:
      type: filesystem
      enabled: true
      required: true
      config:
        root: /storage
```

`type` says what is behind the instance. `runtime` says how it is reached:
`builtin` inside this binary, `service` as a child process the server launches
and verifies, `script` as an expression written in the file. `enabled` says
whether it runs at all. An instance that is switched off keeps its settings and
its identity and does nothing.

`required: true` holds the whole server out of readiness while that instance is
not running. The server still starts, binds its ports and serves; the
supervisor never gives up on the instance, and `GET /api/v1/readiness` answers
503 and names it until it serves. It is right for the disk every uploaded file
goes on and wrong for an optional sink.

Everything under `config:` belongs to the implementation named in `type`. The
server carries the block across and reads nothing inside it. The settings each
type takes are listed under
[Adapters, modules and runtimes](./extension-concepts.md) and are served by the
running build at `GET /api/v1/vocabularies/extensions`.

`hidden_contributions` hides single entries of a module's interface for every
viewer. A module can add a page, a settings pane, a line on each printer card,
a section, a tab and an action to a printer. It can also add an item to each
file's menu and a section to the print dialog. Each entry is written as
`<slot>/<identity>`. The slot is `pages`, `settings`, `device_card`,
`device_section`, `device_tab`, `device_action`, `file_action` or
`print_section`. The identity is the page's `path`, or the `id` of any other
entry. This keeps the statistics page and hides its line on every printer card:

```yaml
resources:
  asset:
    fleet-stats:
      hidden_contributions:
        - device_card/fleet-stats
```

A file action is hidden in the menu on each file's row of the storage listing
and in the menu on the file's own page. This hides a planner module's Plan a print item
and its section in the print dialog:

```yaml
resources:
  sink:
    planner:
      hidden_contributions:
        - file_action/plan
        - print_section/planner
```

A dashboard card cannot be hidden this way. A card shows only where a person
placed it, and that person removes it from the dashboard.

An omitted or empty list hides nothing. Hiding changes what people see, not
what they may do: a hidden page loses its menu entry, and a link to it still
opens. An entry the module does not declare is kept and does nothing, so a
later version that declares it is hidden at once. The server accepts up to 64
entries of at most 200 characters, without duplicates. The Extensions page
lists every entry of an extension, with a switch for each, and writes this list
for an instance that is not set in the file.

You do not declare a module here. `prusa-connect module add` installs one
and records it, with the digest of what it installed, in the file the server
owns.

You do configure one here. A module ships its settings with itself. Its
`module.yaml` lists every key it reads, with the key's type and a sentence
saying what it does. Each key also carries what it is worth when nobody writes
it. Those defaults are the module's own configuration, and `module add` writes
none of them into a file. Writing a `config:` block for the instance overrides a key, and only
where you write that key. A nested block merges key by key; a list is replaced
whole.

`GET /api/v1/resources` shows a module's keys with their types and defaults,
and `module add` prints them at install. A key the module does not declare
stops that instance and the message names it. A key the module cannot start
without and nobody wrote stops it the same way. A value the module declared as
a secret is never returned by the API. The answer carries `xxxxx` in its
place.

## Every key, part by part

Each part of the server has a page of its own: what it does, when to change
it, and every key it reads, with its type and its default. The tables there are
written from the structs the server decodes this file into, so a key it reads
and a key listed there are the same set. Each key has an anchor of its own,
which the setup wizard links to from the field that sets it.

| part | what it covers |
|---|---|
| [Network and API](config/network.md) | the port, browser origins, proxies, time limits, the event stream |
| [Encryption](config/encryption.md) | HTTPS for the web app and TLS for the printers |
| [Sign-in and access](config/sign-in.md) | passwords, a sign-in provider, who an anonymous request is |
| [Web app](config/web-app.md) | the name, the language, the theme, the dashboards |
| [Discovery on the local network](config/discovery.md) | mDNS: how printers and slicers find the server |
| [Printers](config/printers.md) | the printers' own listener, their address, pictures and caches |
| [Storage, history and integrations](config/resources.md) | the folder for uploads, each printer's history, MQTT and webhooks |
| [Database](config/database.md) | SQLite or Postgres, and the connection pool |
| [Logging and metrics](config/logging.md) | the log and the Prometheus metrics |
| [Extensions](config/extensions.md) | the marketplace and installed modules |
| [Server internals](config/server.md) | the keys a person rarely changes |

## Checking a file before you start the server

There is no command that reads a configuration file and reports on it without
starting. Starting the server is the check: it validates everything before it
binds a port, writes a file or starts anything, so a configuration it refuses
has changed nothing.

What can be checked separately is which build a file implies:

```bash
make print-build-tags CONFIG_FILE=config/examples/config.postgres.yaml
```

That reads the file the same way the build does and stops on a key the build no
longer knows.

Ready-made profiles live beside the shipped file, and the test suite loads each
one:

- `config/examples/config.core-only.yaml`, the server with nothing declared
- `config/examples/config.sqlite.yaml`, the shipped engine
- `config/examples/config.postgres.yaml`, the other engine
- `config/examples/config.tls.yaml`, a certificate on the API listener and a
  second socket for printers
- `config/examples/config.printer.yaml`, the printer build's own settings

## Flags

Every flag below overrides the file. Leave one out and the file decides.

| flag | what it sets |
|---|---|
| `--config` | the file to read, `config/config.yaml` by default |
| `--overlay` | the file the resource API writes, `config.d/resources.json` beside the config by default |
| `--addr` | `core.api.addr` |
| `--printer-addr` | the address the device-facing listener binds |
| `--log-level` | `core.log.level` |
| `--log-format` | `core.log.format` |
| `--safe` | starts once with every module off (safe mode). A restart the server performs drops it |
| `--version` | prints the version of this build and stops |

`prusa-connect core healthcheck` asks a server already running on this
host whether it is ready. It reads the same file, so it dials whatever address
that file names.

## Keys that moved

A file that still names one of these is refused at startup, and the message
carries the line and the key that does the work now.

| the key that moved | where that work is done now |
|---|---|
| `core.printer` | `core.connectors.<name>.config.listener` |
| `core.compat` | `resources.sink.<name>.enabled` |
| `mqtt` | a `resources.sink` row of type `mqtt` |
| `observability` | `core.observability.enabled` |
| `fractal` | `resources.link.<name>.enabled` |

`core.frontend.skin` is retired with no replacement. The web app has one
appearance and no key chooses it, so the server refuses the key and you delete
the line.

`make build` refuses the three top-level blocks in this table, and two more of
its own, `server.api` and `documentation.enabled`, naming
`core.frontend.enabled` and `make docs` in their place. It does not check the
keys inside `core`: only the server refuses `core.printer`, `core.compat` and
any key it does not read.

The server has no MQTT transport and runs no broker. What sends the printers'
state to a broker is a sink: a `resources.sink` row of type `mqtt` with the
broker's `url`.

## Build tags a configuration implies

`make build` derives the optional Go build tags from the file it is given, so a
build follows a profile instead of a remembered flag.

```bash
make print-build-tags CONFIG_FILE=config/examples/config.postgres.yaml
make build CONFIG_FILE=config/examples/config.postgres.yaml
make test-config CONFIG_FILE=config/examples/config.postgres.yaml
```

`make build-full` and `make test-full` compile every optional feature whatever
the file says.

| what the file says | build tag |
|---|---|
| `persistence.type: sqlite` | none |
| `persistence.type: postgres` | `postgres` |
| `core.observability.enabled: true` | `observability` |
| `core.dashboards.enabled: false` | `nodashboards` |
| a `resources.db` or `resources.history` instance of type `postgres` | `postgres` |

Every condition here is also a key the running server reads, so a build and a
run cannot disagree about which optional part is in the binary. A binary built
without the metrics client or the Postgres driver refuses to start when that
key asks for it, and the message names the build tag it needs. A binary built
without dashboards starts and serves none.

A configuration file never names a build tag itself. `adapters/tags.json` is
where a condition or a declared type maps onto a tag.

`persistence.type: sqlite` implies no tag, because SQLite is linked into every
build: a binary that cannot open its permission graph refuses every request.

## Where the documentation site is served

This site is published on a host of its own. `make docs` builds it into
`docs/dist/`, and the beta portal serves that build. `pnpm run build:static`
in `docs/` lays the build out in `docs/out/` under its own `/docs` base, for a
host that serves a directory at its root. The running server does not serve
it. It links to it: the build
variable `DOCS_URL` names the address, the beta portal by default, and
`{version}` in it is replaced by the version of the binary. The default names
the version, so a build opens the documentation of its own release. The portal
keeps each release's documentation apart and sends a version it holds no copy
of to the current one.

To read the site the way it is deployed, `make docs-up` builds it inside a
container and serves it at `http://127.0.0.1:54322/docs/`, on the port
`DOCS_PORT` in `.env`. `make docs-down` removes the container.

`/docs` on the server is the interactive API reference, and `/openapi.json` is
the document behind it. `/openapi.json` is in every build. The printer build
leaves out the `/docs` page.
