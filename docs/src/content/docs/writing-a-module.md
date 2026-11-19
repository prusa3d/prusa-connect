---
title: Writing a service module
description: From an empty directory to a module installed in a running server, with every command in the page executed and its output quoted.
audience: extend
rewrite:
  describes: current
  implementedBy:
    - sdk/wire
    - sdk/wirerpc
    - sdk/busapi
    - sdk/routesapi
    - sdk/stateapi
    - internal/app/resourcehost
    - examples/service-plugin
---

A service module is a binary of your own that the host launches, supervises and serves. It needs no rebuild of the host, so it is the kind a customer can install. This page builds one from an empty directory: a spool inventory with three routes, stored in the host's record store, reachable over the host's API, and publishing on the host's event stream.

Every command below was run in the order it appears, and the output is what it printed.

## Links

| what | where |
| --- | --- |
| which kind of extension to write | [adapters, modules and runtimes](./extension-concepts.md) |
| a first module, built and read in one sitting | [write your first module](./writing-your-first-module.md) |
| a module in its own repository, with CI and a release workflow | [prusa3d/prusa-connect-extension-example](https://github.com/prusa3d/prusa-connect-extension-example) |
| the GitHub Action that publishes a release to the marketplace | [prusa3d/prusa-connect-marketplace-publish](https://github.com/prusa3d/prusa-connect-marketplace-publish) |
| the wire the child process speaks | [the plugin wire protocol](./plugin-wire-protocol.md) |
| the larger working example | `examples/service-plugin/main.go`, which adds burst, panic and not-ready routes |
| a module that reads the API | `extensions/fleet-stats`: a page, a card line and a printer section over history facts |
| a module that reads files for the core and hands a component to other modules | `extensions/threemf`: `file_analysis`, its own routes and `ui.exports` |
| how the host runs the child | [the extension host](./internals/the-extension-host.md) |

## Commands

| command | does |
| --- | --- |
| `make new-module NAME=<name> REPO=<repository address>` | write a new module from a skeleton: UI only, process only, or both |
| `sh install.sh` | in a generated module: build it and install it into the demo's data |
| `go run ./cmd/new-module -start demo -dir <folder>` | in the checkout: start a generated module in the demo, with its dev server in front |
| `go build -o <name>_<GOOS>_<GOARCH>[.exe] .` | build the artefact under the name the host launches |
| `prusa-connect module add <module path>@<version> -from <release>` | install a release and write its declaration |
| `prusa-connect module list` | every module this deployment declares |
| `prusa-connect module remove <module or kind:name>` | take the declaration and the files away |
| `curl http://127.0.0.1:8080/api/v1/readiness` | read whether the instance is serving |
| `curl http://127.0.0.1:8080/api/v1/modules/<namespace>/...` | call the module's own routes |
| `curl -sN http://127.0.0.1:8080/api/v1/events` | read the event stream the module publishes on |

## What the toolchain needs

| requirement | why |
| --- | --- |
| Go as `sdk/go.mod` declares it, 1.26.11 | the sdk module pins it |
| a checkout of `prusa-connect` | the sdk module carries no published version, so `go.mod` carries a `replace` pointing at the `sdk` directory of a checkout |
| a running server | [run the server](./getting-started.md) covers both ways to start one |

Go is one choice, not a requirement of the wire. The protocol is specified, and `examples/wire-protocol/child.js` implements it in JavaScript for Node from the page alone. This guide takes the Go route because the sdk carries it.

## The name, the namespace and the address

Pick these three before writing code, because the operator's file and your routes both depend on them.

| thing | value in this guide | rule |
| --- | --- | --- |
| your repository | `github.com/acme/spool-tracker` | every path element is a DNS-1123 label |
| the namespace | `com.github.aux.acme.spool-tracker` | the host reversed, the label `aux`, then the rest of the path |
| the type an operator writes | `com.github.aux.acme.spool-tracker/spool-tracker` | `<namespace>/<name>`, and the name is a DNS-1123 label |
| the artefact file | `spool-tracker_windows_amd64.exe` | `<name>_<GOOS>_<GOARCH>`, with `.exe` on Windows only |
| the module's routes | `/api/v1/modules/com.github.aux.acme.spool-tracker/...` | the host mounts the namespace, the module owns everything under it |

The namespace rendering is the one every compiled-in extension gets from `extension.NamespaceOf`, described in [registries and names](./onboarding/registries-and-names.md). One namespace is one mount: a second instance declaring the same namespace fails alone and the first keeps serving.

## The module

Two files. `go.mod` names the sdk and where to find it:

```go
module github.com/acme/spool-tracker

go 1.26.11

require github.com/prusa3d/prusa-connect/sdk v0.0.0

replace github.com/prusa3d/prusa-connect/sdk => ./.core/sdk
```

`go mod tidy` fills in the rest. `main.go` is the whole program. The first half is the wire: one handshake line on standard output, then a session the host drives.

```go
// Command spool-tracker is a service module for Prusa Connect. It keeps
// a spool inventory in the host's record store and serves it over the host's
// API at /api/v1/modules/com.github.aux.acme.spool-tracker/.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/signal"

	"github.com/prusa3d/prusa-connect/sdk/busapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
	"github.com/prusa3d/prusa-connect/sdk/routesapi"
	"github.com/prusa3d/prusa-connect/sdk/stateapi"
	"github.com/prusa3d/prusa-connect/sdk/wire"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

const (
	name    = "spool-tracker"
	release = extension.Version("0.1.0")
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
}

// run writes the handshake line, then serves calls until the host closes the
// channel. Standard output carries the protocol; standard error is where this
// process's own words go, and the host copies it into its log.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	line, err := wire.Handshake{
		Protocol: wire.ProtocolMajor,
		MinMinor: wire.MinMinor,
		MaxMinor: wire.MaxMinor,
		Address:  wire.StdioAddress,
	}.Line()
	if err != nil {
		return err
	}

	if _, err = os.Stdout.WriteString(line); err != nil {
		return err
	}

	serves := wirerpc.Methods{}

	conn, err := wirerpc.NewConn(stdio{}, wirerpc.Options{
		Role:    wirerpc.RoleChild,
		Methods: serves,
	})
	if err != nil {
		return err
	}

	// The host's key/value store, already bound to this module's namespace.
	records := stateapi.NewClient(conn)

	// The host's event stream, already bound to this instance's origin and to
	// the endpoints the operator declared for it. This process names neither.
	events := busapi.NewClient(conn)

	maps.Copy(serves, wirerpc.Methods{
		wirerpc.MethodInfo: func(context.Context, wirerpc.Call) (any, error) {
			return wirerpc.Info{Name: name, Version: release, Contract: wirerpc.Contract}, nil
		},
		wirerpc.MethodShutdown: func(context.Context, wirerpc.Call) (any, error) {
			return nil, nil
		},
		routesapi.MethodServe: routesapi.Serve(routes(records, events)),
	})

	return conn.Serve(ctx)
}
```

Three methods are the whole surface this module answers. `plugin.info` says what this is, `plugin.shutdown` is how the host asks it to stop, and `routes.serve` is the HTTP surface. The two clients go the other way: the module calls the host on those. `routesapi.Serve` adapts an ordinary `http.Handler`, so the second half is plain `net/http`:

```go
// routes is this module's HTTP surface, written as any Go program writes one.
// The paths carry no prefix: /spools here is
// /api/v1/modules/com.github.aux.acme.spool-tracker/spools to a caller.
func routes(records stateapi.Records, events *busapi.Client) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /spools", func(w http.ResponseWriter, r *http.Request) {
		keys, err := records.List(r.Context(), "spool/")
		if err != nil {
			answer(w, statusFor(err), map[string]any{"error": err.Error()})

			return
		}

		spools := make([]string, 0, len(keys))
		for _, key := range keys {
			spools = append(spools, string(key[len("spool/"):]))
		}

		answer(w, http.StatusOK, map[string]any{"spools": spools})
	})

	mux.HandleFunc("PUT /spools/{name}", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			answer(w, http.StatusBadRequest, map[string]any{"error": err.Error()})

			return
		}

		key := stateapi.Key("spool/" + r.PathValue("name"))

		version, err := records.Put(r.Context(), key, body)
		if err != nil {
			answer(w, statusFor(err), map[string]any{"error": err.Error()})

			return
		}

		// Now say so. "changed" is a name the operator declared; the host
		// turns it into an event type and publishes it under this instance's
		// own origin.
		//
		// A refusal costs this event and not the write. The spool is stored
		// either way, and an operator who declared no endpoint reads the reason
		// in the answer.
		published := "published"
		if err = events.EmitRaw(r.Context(), "changed", string(key), body); err != nil {
			published = err.Error()
		}

		answer(w, http.StatusOK, map[string]any{
			"stored":  r.PathValue("name"),
			"version": version,
			"event":   published,
			// Who the host authorized. The host's answer, not a header.
			"by": routesapi.PrincipalOf(r),
		})
	})

	mux.HandleFunc("GET /spools/{name}", func(w http.ResponseWriter, r *http.Request) {
		held, err := records.Get(r.Context(), stateapi.Key("spool/"+r.PathValue("name")))
		if err != nil {
			answer(w, statusFor(err), map[string]any{"error": err.Error()})

			return
		}

		answer(w, http.StatusOK, json.RawMessage(held.Bytes))
	})

	return mux
}

// statusFor maps a store refusal onto a status a client can act on.
func statusFor(err error) int {
	switch {
	case errors.Is(err, stateapi.ErrNoRecord):
		return http.StatusNotFound
	case errors.Is(err, stateapi.ErrKey), errors.Is(err, stateapi.ErrValueTooLarge):
		return http.StatusBadRequest
	case errors.Is(err, stateapi.ErrNotReady), errors.Is(err, stateapi.ErrBusy):
		return http.StatusServiceUnavailable
	}

	return http.StatusBadGateway
}

// answer writes one JSON body, which is every route above.
func answer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// stdio is the channel the host handed this process.
type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdin.Close() }
```

The SDK also answers `plugin.ping` by itself, before any of your handlers run, so there is nothing to write for it. The host pings every 15 seconds, and a module that misses three pings in a row is killed without `plugin.shutdown` and restarted, which takes about 45 seconds. A handler that blocks every call gets the same treatment: when calls keep timing out for two minutes, the module is restarted. A handler that answers with an error does not. Each stop counts toward the module's switch-off, and `on_over_memory: warn` does not exempt a module from it.

Nothing here opens a port, reads a namespace or checks a credential. The host authorizes every request before the module sees it, and `routesapi.PrincipalOf` is the host's answer about who called.

## Build and checksum

The artefact is named for the type and the platform, so one directory can hold every platform's build:

```console
$ go build -o plugins/spool-tracker_windows_amd64.exe .

$ sha256sum plugins/spool-tracker_windows_amd64.exe
7c13d276a5a5bb8a854ee4ce26b5ea6d1fa577a2a379c3ef0f43d65021dc1821 *plugins/spool-tracker_windows_amd64.exe
```

On Linux the file is `spool-tracker_linux_amd64`, with no suffix. Build one per platform the deployment runs.

## The declaration an operator writes

Add the instance to the `resources:` block of the file the server reads, `config/config.yaml` by default:

```yaml
resources:
  sink:
    spools:
      type: com.github.aux.acme.spool-tracker/spool-tracker
      enabled: true
      runtime: service
      service:
        path: C:/src/spool-tracker/plugins
        checksum: sha256:7c13d276a5a5bb8a854ee4ce26b5ea6d1fa577a2a379c3ef0f43d65021dc1821
        contract_major: 1
        events:
          changed: spool.changed
```

| key | is |
| --- | --- |
| `sink: spools:` | the kind and the instance name. A module is declared under an existing kind, and `sink` is the usual one |
| `type` | `<namespace>/<name>`. The namespace is the mount, the name is the artefact's file name |
| `service.path` | the directory holding the artefacts. The host picks its own platform's file |
| `service.checksum` | `sha256:` and 64 hex digits over the artefact, verified before every launch |
| `service.contract_major` | the wire major the child speaks. There is no default |
| `service.events` | the endpoints this instance may publish on, and the name each one publishes under. Absent means it may publish nothing |
| `service.permissions` | the actions this instance may take as itself, such as `observe`. Absent means none. See [what a module may do as itself](#what-a-module-may-do-as-itself) |
| `service.checks` | the actions this instance only asks about. See [asking about an action](#asking-about-an-action-the-module-does-not-take) |
| `service.check_absent_people` | whether this instance may ask about a person it is not serving. See [asking about a person who is away](#asking-about-a-person-who-is-away) |
| `service.list_people` | whether this instance may list the people this server holds. See [listing the people](#listing-the-people) |
| `service.calls` | the namespaces of the other modules whose routes this instance may call. See [calling another module](#calling-another-module) |
| `service.records_mib` | the room this instance's records may take, from 1 to 1024 MiB. Absent means 8 MiB. See [more room for records](#more-room-for-records) |
| `service.listen` | the ports this instance may listen on, by name, as `udp/<port>` or `tcp/<port>`. See [ports](#ports-and-a-data-directory) |
| `service.data` | whether this instance gets a writable directory of its own |
| `service.assets` | the browser half: a directory and the digest over it. See [a module with both halves](#a-module-with-both-halves) |
| `config` | the settings for this instance. It overrides a key of the module's own defaults. See [settings and their defaults](#settings-and-their-defaults) |

### Settings and their defaults

A module ships its settings with itself. `module.yaml` carries an `options:`
list, one entry per key, and every entry says what the key does, what shape it
holds and what it is worth when nobody writes it:

```yaml
options:
  - key: shelf_size
    type: int
    default: "500"
    summary: How many spools this module keeps. A write over the limit is refused.
  - key: upstream_token
    type: secret
    required: true
    summary: What this module presents to the inventory service.
```

`type` is one of `string`, `int`, `number`, `bool`, `duration`, `url`,
`secret`, `list` and `object`. A `list` or an `object` writes its default as
JSON. A `secret` is masked: the resources listing carries `xxxxx` in place of its
value, and its default is never published. `required: true` says the module
cannot start until an operator writes the key, so a required key has no
default.

That list is the module's own configuration. An operator can also edit it on the
module's page, under Settings, Extensions, on the Settings tab. An option has
no title field, so the form labels each key by its name with spaces in place of
the underscores: `shelf_size` is labelled "Shelf size". The form checks each
value against its type (`int`, `number`, `bool`, `list` and so on) before it
saves. A `secret` shows empty, with a note that a value is stored. Left
untouched, the stored value is kept. An operator who writes no `config`
block gets every default, and `module add` writes no settings into any file.

An operator overrides a key by writing that key:

```yaml
resources:
  sink:
    spools:
      config:
        shelf_size: 2000
        upstream_token: "..."
```

`shelf_size` is now 2000 and every other key is still its default. A nested
block merges key by key, so writing one key of it keeps the module's defaults
for the rest of it. A list is replaced whole.

The server checks the result before it starts the process. A key the list does
not name is refused and the message names it beside the keys the module does
read. A required key nobody wrote is refused the same way. Neither is retried:
the instance waits for the file to be fixed.

The module reads the settings on `plugin.configure`, which the host calls once
after `plugin.info` and before any other call. Say so in `plugin.info`, and
register the handler:

```go
type Settings struct {
	ShelfSize int `json:"shelf_size"`
}

wirerpc.Methods{
	wirerpc.MethodInfo: func(context.Context, wirerpc.Call) (any, error) {
		return wirerpc.Info{
			Name:     name,
			Version:  release,
			Contract: wirerpc.Contract,
			Serves:   []wirerpc.Method{wirerpc.MethodConfigure},
		}, nil
	},
	wirerpc.MethodConfigure: wirerpc.Configure(func(_ context.Context, held Settings) error {
		shelf.resize(held)

		return nil
	}),
}
```

`wirerpc.Configure` decodes strictly, so a key the struct does not carry
refuses the start rather than being dropped. An error the handler returns
refuses it too, and the operator reads the reason on the instance.

A module that reads no settings leaves `plugin.configure` out of `Serves`. The
server then sends nothing, and it refuses to start an instance an operator
wrote a `config` block for, rather than running a process that will never read
it.

The settings never travel as an environment variable or a command-line
argument. A setting may be a credential, and both of those are readable in a
process listing.

`examples/spools` is a worked module with one setting.

### What a module may do as itself

A module calls the host's fleet and event stream in two ways. A call it makes
while serving a person carries that person's principal, the one the host handed
it on `routes.serve`, and the host answers as that person. A call it makes with
nobody behind it, a sync loop or a poll, is its own work.

`service.permissions` is the ceiling on that own work. It lists the actions the
instance may take as itself. `observe` reads devices and subscribes to the
stream, and `operate` or `configure` sends a verb of that class.
`admit_device` approves a registration, and `read_storage` reads a file one of
the server's storages holds. A call outside the list is refused. A call inside
it is then decided by the authorization graph for the instance's own principal,
`sink:spools` here. So the module reaches what it declared and what the operator
granted, and never more than either. An absent list is an instance that may do
nothing as itself.

The list takes any action the authorization graph holds, `view_audit` among
them. That one is a declaration and not yet a gate. What a module receives
on the event stream is decided by the graph alone. So an instance the operator
granted `view_audit` on a printer reads that printer's
`com.prusa.connect.device.command.authorized` frames, whether or not the word
is in the list. Write it anyway: the install then prints what the module asks
for, and an operator reads the request before they approve it.

### Saying what the module needs

A module that calls `device.list` on a server serving no fleet, or that was
built against a newer server, answers an error to every request it gets. The
`require:` map in `module.yaml` is how a module says so up front, and the
server checks every entry before it starts the instance:

```yaml
require:
  # The core, as a platform package: a constraint on the server's version.
  core: ">=1.2"
  # A wire method the server has to serve this instance.
  core-device.list: "*"
  # Another module, by its namespace, and a constraint on its releases.
  com.github.aux.acme.spools: "^1.2"
```

The map follows Composer's `require`: `core` is the core, as `php` is in
Composer, and `core-<method>` is a capability, as `ext-json` is. The next
section says how a dependency on another module works. The map is recorded
in the declaration at install, so the server holds the module to it at every
start and does not read a manifest off disk.

An unmet requirement stops that instance alone. The rest of the server keeps
serving, and `GET /api/v1/resources/sink/spools` carries the reason in the
`DependenciesSatisfied` condition:

| what is missing | what happens | code |
| --- | --- | --- |
| a capability this build does not serve | the instance stays `failed`. A restart brings the same answer, because what a build serves is settled when it is compiled | `RESOURCE_DEPENDENCY_UNSERVED` |
| a server version outside the range | the same | `RESOURCE_DEPENDENCY_UNSERVED` |
| a module required that is not serving, or out of range | the instance retries, and starts by itself once that module serves | `RESOURCE_DEPENDENCY_MISSING` |
| a `state.*` capability while no record backend serves | the instance retries, and starts by itself once a backend under `resources.db` serves | `RESOURCE_RECORDS_UNSERVED` |
| two modules requiring each other | every instance in the ring stays `failed`, and the message names the ring | `RESOURCE_DEPENDENCY_CYCLE` |

`module add` prints the list, and refuses to install a module asking for a
capability this build does not serve or a server version this build is not.

### Depending on another module

Dependencies between modules follow Composer: its keys, its constraints, its
stabilities, its lock and its commands.

```yaml
require:
  com.github.aux.acme.threemf: "^1.2"
suggest:
  com.github.aux.acme.parts: "Shows the parts of a plan"
conflict:
  com.github.aux.acme.old-planner: "<2.0"
```

A dependency names the other module by its namespace. `require` is what your
module cannot run without. `suggest` names a module and why, and installs
nothing. `conflict` refuses to install your module beside the releases its
constraint holds. A constraint has Composer's meaning: `^1.2` is
`>=1.2 <2.0`, `^0.9` is `>=0.9 <0.10`, `~1.2` is `>=1.2 <2.0`, and
`^1.2@beta` also takes beta releases of that module. Raise your major when a
module that talks to yours would break. A Go package of another module that
you compile in is a dependency of your `go.mod`, and the manifest does not
name it.

| command | what it does |
| --- | --- |
| `module add <path>@<constraint>` | installs the module and the modules it lacks, and keeps the rest |
| `module update [<module>...] [-with-dependencies]` | takes the newest releases every constraint allows |
| `module install` | installs exactly what `modules.lock.json` names |
| `module remove <module> [-with-unused]` | removes a module nothing requires, and with the flag the modules installed only for it |
| `module why <module>` | says what keeps a module installed |
| `module why-not <module> <version>` | says what refuses a release |
| `module outdated` | lists each module against the index |

The page installs through `POST /api/v1/modules/plan`, which lists the
modules an install adds, and the person approves them all at once. A server
has one release of each module, and an install never changes another module.
A module another module requires cannot be removed or switched off. The
server starts a module after the modules it requires, and stops it while one
of them is down. `core.modules_minimum_stability` and
`core.modules_prefer_stable` are Composer's `minimum-stability` and
`prefer-stable`.

| refusal | when |
| --- | --- |
| `MODULE_DEPENDENCY_CONFLICT` | no release fits every constraint; it names who needs what |
| `MODULE_DEPENDENCY_NOT_OFFERED` | no index offers a module the install needs |
| `MODULE_PLAN_CHANGED` | the approved list differs from what the server resolves now |
| `MODULE_DEPENDENCY_MISSING` | a module it requires is missing, off or out of range |
| `MODULE_REQUIRED_BY` | another module requires the one being removed, updated or switched off |

### A module that ships files and no process

A module whose whole job is a page for the browser runs nothing. Declare it
under the `asset` kind, with the directory its build produced and the digest
recorded over every file in it. Write no `runtime` key: the kind offers one
runtime, so there is nothing to choose.

```yaml
resources:
  asset:
    spools:
      type: com.github.aux.acme.spool-tracker/spool-tracker
      asset:
        path: C:/src/spool-tracker/ui/dist
        checksum: sha256:3f1c9b5e0a7d2c8e6b4a1f9d0c2e7b5a8d6f4c1e9b3a7d5f2c0e8b6a4d1f9c3e
```

The host checks the digest at every start and serves the files at
`/api/v1/modules/<namespace>/ui/`, so `index.js` in that directory is reached
at `/api/v1/modules/com.github.aux.acme.spool-tracker/ui/index.js`. A request
takes `view_resource` on the instance, the same permission a module's own
route takes, and a directory that no longer matches its digest is refused and
never served. The instance is enabled, disabled and listed at
`/api/v1/resources` like any other.

### A module with both halves

A module that runs a process and draws a page is one declaration, one instance
and one namespace. The process is the `service` block, and the bundle beside it
is `service.assets`:

```yaml
resources:
  sink:
    spools:
      type: com.github.aux.acme.spool-tracker/spool-tracker
      runtime: service
      enabled: true
      service:
        path: modules/spools/bin
        checksum: sha256:7c13d276a5a5bb8a854ee4ce26b5ea6d1fa577a2a379c3ef0f43d65021dc1821
        contract_major: 1
        assets:
          path: modules/spools/bundle
          checksum: sha256:3f1c9b5e0a7d2c8e6b4a1f9d0c2e7b5a8d6f4c1e9b3a7d5f2c0e8b6a4d1f9c3e
```

The routes answer at `/api/v1/modules/<namespace>/`, the files at
`/api/v1/modules/<namespace>/ui/`, and one `enabled` flag switches both. The
listing shows one line with a `ui` object on it.

You do not write this block yourself. `module add` writes it, with the two
digests of what it installed.

## Starting a module from a skeleton

`make new-module` writes a directory you can build. It runs `cmd/new-module`,
a program of its own beside `cmd/scaffold-driver`. It is not a word after the
server, because the templates would add 330 to 390 KB to the server on every
device, and a release ships one binary. Piping the command's input, the way a
script or CI does, is what skips the questions below and goes straight to
writing the files:

```console
$ : | make new-module NAME=spools REPO=github.com/acme/spools ARGS="-kind both"
1. wrote 24 files in ../spools, which is /home/you/src/spools
     ../spools/.gitignore
     ../spools/.vscode/extensions.json
     ../spools/CHANGELOG.md
     ../spools/README.md
     ../spools/faults.go
     ../spools/go.mod
     ../spools/install.sh
     ../spools/main.go
     ../spools/main_test.go
     ../spools/module.yaml
     ../spools/openapi.json
     ../spools/ui/.prettierignore
     ../spools/ui/.prettierrc.js
     ../spools/ui/eslint.config.js
     ../spools/ui/package.json
     ../spools/ui/pnpm-workspace.yaml
     ../spools/ui/src/host.ts
     ../spools/ui/src/index.ts
     ../spools/ui/src/locales/en.json
     ../spools/ui/src/page.svelte
     ../spools/ui/src/styles.css
     ../spools/ui/svelte.config.js
     ../spools/ui/tsconfig.json
     ../spools/ui/vite.config.ts
2. made it a git repository with a first commit

✔ spools is ready
  folder  ../spools

  start it later  go run ./cmd/new-module -start demo -dir ../spools
```

`REPO` is the repository address, and the namespace is derived from it. Leave
it out and the skeleton uses `github.com/example/<name>`, which you change
before you publish. With no terminal to ask on, `NAME`, `REPO` and a `-kind`
in `ARGS` are all required. `make new-module` on its own, from a terminal,
asks for every one of them instead.

### Where it goes

| run from | the module goes to | it builds against |
| --- | --- | --- |
| anywhere inside a checkout | `../<name>`, beside the checkout | the checkout: `go.mod` has a `replace`, and `ui/pnpm-workspace.yaml` maps the three packages to the checkout with `overrides` |
| anywhere else | `./<name>` | the three `@prusa` packages and `sdk/v0.1.0` at the tree's version, from the local registry |

`-dir` puts it anywhere else. A folder that exists and holds anything is
refused, and nothing is written. For a module written anywhere else,
`ui/.npmrc` points `@prusa` at the local registry `make registry-up` starts.
In a checkout, run `make registry-up` and `make registry-publish
VERSION_TAG=0.1.0` first, then `pnpm install` in the module's `ui/` resolves
the three packages from it. It also has no `install.sh`, because that script
runs the checkout's server and demo, and its README gives the commands
instead.

Nothing is overwritten. A second run over a directory that already holds
anything refuses, because half a module and half a skeleton compiles.

### What it needs on your machine

Before it asks anything else, it checks the tools the module's halves are
built with. A UI needs Node 22.12.0 or newer and pnpm, and a process needs Go
1.26.11 or newer. pnpm only has to be installed, at any version. It switches
itself to the version the generated `ui/package.json` pins. A missing or too-old tool
stops it before anything is written, with one line per tool saying how to
get it.

### A git repository from the start

The module's folder is a git repository with one commit when the command
ends. `.gitignore` keeps out what a build writes. The question "Where should
its code live?" says where that commit goes, and each answer has a flag:

| answer | flag | what happens |
| --- | --- | --- |
| a new private repository on GitHub | `-github` | `gh repo create <login>/<name> --private --source . --remote origin --push` runs in the module's folder |
| a repository I already made | `-remote <url>` | the address is added as `origin` and the commit is pushed there |
| only here, a local git repository | none | the commit stays in the folder |
| no git | `-git=false` | the folder is not a git repository |

The GitHub answer is offered only when `gh`, the GitHub command line tool, is
logged in. `gh api user` gives the account, and it is then the default answer.
The repository address then defaults to `github.com/<login>/<name>` too.
`-github` and `-remote` together are refused.

A repository you already made must be empty. On a terminal the address is
checked with `git ls-remote` before anything is written. An address that
cannot be reached, or a repository that already has commits, is asked for
again.

When `git init` or the commit fails, the files stay where they are and the
error says what git said. A push or a `gh repo create` that fails does not
fail the run, because the module is written and committed by then. The step
prints a warning with the reason in one line, then the command that tries
again.

### What it asks

Run on a terminal it asks for what the flags do not carry, and every question
offers the flag's value as its default:

```console
$ make new-module

? What kind of module is it?
  A UI adds something to the app's screens. A process is a program the server runs for the module.
  1) UI only: it draws on the app's screens, and runs nothing
  2) process only: a Go program the server runs, with nothing on screen
  3) UI + process: screens, and the Go program they read from
› [1] 
✔ UI only

? What is its short name?
  Lower-case letters, digits and dashes, like nozzle-watch. It names the folder, the files and the address of its page.
› nozzle-watch
✔ nozzle-watch
  Its title will be: Nozzle Watch

? Where will its code be published?
  A repository address, like github.com/you/nozzle-watch. The server makes the module's namespace from it: the unique name all its addresses start with.
› [github.com/you/nozzle-watch] github.com/acme/nozzle-watch
✔ github.com/acme/nozzle-watch
  Its namespace will be: com.github.aux.acme.nozzle-watch

? What does it add to the app?
  Type one or more numbers, separated by commas, like 1,3. Each one is a place in the app the module may draw in, and module.yaml lists them under contributes.
  The app: the sidebar and a page of its own
  1) a page of its own, with an entry in the sidebar
  A printer's page
  2) a section at the foot of each printer's Status tab
  3) a tab of its own on each printer's page
  4) an item in the action menu of each printer's page
  The printer listing
  5) one line on each printer's card in the listing
  The settings screen
  6) a pane on the settings screen
› [1] 
✔ a page of its own, with an entry in the sidebar

? Where should its folder go?
  A path from here. The folder must not exist yet, or must be empty.
› [../nozzle-watch] /home/you/src/nozzle-watch
✔ /home/you/src/nozzle-watch
  That is /home/you/src/nozzle-watch

? Where should its code live?
  git records every change to the module's code, and a repository on a server keeps a copy of it.
  1) a new private repository on GitHub, you/nozzle-watch: gh makes it and pushes the first commit to it
  2) a repository I already made (paste its address): it must be empty, and the first commit is pushed to it
  3) only here, a local git repository: one first commit, and nothing is pushed
  4) no git: you set it up yourself later
› [1] 3
✔ only here, a local git repository

? Change the title, the page address or the ids?
  Most modules keep the defaults. The command printed at the end writes the title you change, and the defaults for the addresses and ids.
› [y/N] n
✔ no

  kind       UI only
  name       nozzle-watch, shown as Nozzle Watch
  namespace  com.github.aux.acme.nozzle-watch  (the unique name its addresses start with)
  adds       page nozzle-watch  (the contributes list in module.yaml)
  folder     /home/you/src/nozzle-watch
  files      16
  git        a first commit, only here

? Write it?
  Nothing is written until you choose 1.
  1) yes, write it
  2) no, change the answers
  3) no, cancel
› [1] 1
✔ yes, write it
1. wrote 19 files in ../nozzle-watch, which is /home/you/src/nozzle-watch
     ../nozzle-watch/.gitignore
     ../nozzle-watch/.vscode/extensions.json
     ../nozzle-watch/CHANGELOG.md
     ../nozzle-watch/README.md
     ../nozzle-watch/install.sh
     ../nozzle-watch/module.yaml
     ../nozzle-watch/ui/.prettierignore
     ../nozzle-watch/ui/.prettierrc.js
     ../nozzle-watch/ui/eslint.config.js
     ../nozzle-watch/ui/package.json
     ../nozzle-watch/ui/pnpm-workspace.yaml
     ../nozzle-watch/ui/src/host.ts
     ../nozzle-watch/ui/src/index.ts
     ../nozzle-watch/ui/src/locales/en.json
     ../nozzle-watch/ui/src/page.svelte
     ../nozzle-watch/ui/src/styles.css
     ../nozzle-watch/ui/svelte.config.js
     ../nozzle-watch/ui/tsconfig.json
     ../nozzle-watch/ui/vite.config.ts
2. made it a git repository with a first commit

? Build it and install it now?
  This runs sh install.sh. It builds the module and installs it on a demo server. The first run takes a few minutes.
› [Y/n] y
✔ yes
3. ✔ built and installed

✔ nozzle-watch is ready
  folder  ../nozzle-watch
  flags   go run ./cmd/new-module -name nozzle-watch -kind ui -repo github.com/acme/nozzle-watch -contributes page -dir ../nozzle-watch

? Start it now?
  A server starts on a free port with the module installed, and for a UI its dev server in front of it. Ctrl+C stops them.
  1) demo, with simulated printers
  2) the real app
  3) not now
› [1] 1
✔ demo, with simulated printers

✔ running  http://localhost:5173   (demo · sign in as demo / wCm6hJ6NSsIT9xs_)
  edit ../nozzle-watch/ui/src/page.svelte, the page updates as you save
  Ctrl+C stops both
  logs ./bin/nozzle-watch-demo/core.log, ./bin/nozzle-watch-demo/vite.log
^C
```

Every question has a line beneath it saying what the answer is used for. A
question starts with `?`, you type after `›`, and `✔` repeats the answer it
took. On a terminal the question is also bold, the hint dim and the prompt in
color, unless `NO_COLOR` is set. The
repository address is where the namespace comes from, and the namespace is
printed as soon as the address is typed. An address pasted from a browser,
with `https://` and `.git`, is read without them. The summary shows what is
about to be written, with the absolute path, and asks to write it, to change
the answers, or to cancel. After the install, the `flags` line, dimmed, is the
command line that writes the same module with no questions. The start and
what it prints are in [seeing it on a screen](#seeing-it-on-a-screen). With
"not now", or with no terminal, the run ends with the `-start` command that
starts the module later instead. The advanced step changes the title, the page
address and the ids. `-title` carries the title; the addresses and ids have
no flag, so that line writes their defaults and says so.

Nothing is asked when stdin is not a terminal, so a script, CI and a test run
on the flags alone. Then `-name`, `-kind` and `-repo` are required, and a run
without them names every one that is missing:

```bash
go run ./cmd/new-module -name nozzle-watch -kind ui \
  -repo github.com/acme/nozzle-watch -contributes device-card
```

`-contributes` takes any slot in the table under
[Where each contribution lands](#where-each-contribution-lands), such as
`page`, `device-card` or `file-action`, comma separated, and defaults to `page`.
Only what was asked for is written: a module with no page
has no `page.svelte`, no `pages` entry in the manifest and no
`host.register.page` call. That pairing is the point of the generator.
`contributes` is the ceiling, and a registration the manifest does not declare
throws and the host drops the module whole, so the two are rendered from one
answer.

A module with a process is also asked how much memory the process needs, in
MiB, and the answer is written to `needs.memory_mib` in `module.yaml`.
`-memory-mib` answers it from the command line. A figure from 16 to 8192 is
written. An empty answer writes the line commented out, and the server then
gives the module 128 MiB.

`-start demo` or `-start app`, with `-dir` naming a module's folder, writes
nothing. It starts that module the way the question after the install does,
and any other flag is ignored.

### The three kinds

`-kind` says which halves the module has:

| `-kind` | writes | the manifest |
| --- | --- | --- |
| `ui` | `ui/` and no Go file | `kind: asset`, no `contract_major`, no `platforms`, no OpenAPI fragment |
| `process` | `main.go` and its tests, no `ui/` | `kind: sink` and no `ui` block. The app draws nothing for it |
| `both` | both | `kind: sink`, with the bundle declared beside the process |

The host verifies a `ui` module against its digest, serves it, and never runs
it. `-contributes` belongs to a bundle, so `-kind process` refuses it. Every kind
gets `.vscode/extensions.json`, which recommends the Svelte extension for a
bundle and the Go extension for a process.

### Seeing it on a screen

After the install the generator asks whether to start the module. Choose the
demo and it starts the demo's core on the first free port from 8080 up, with
the module installed. For a module with a bundle it also runs `pnpm dev` in
`ui/`, the module's own dev server, on the first free port from 5173 up. It
then prints one block: the address to open, the account to sign in with, the
file to edit, and where the logs are. Ctrl+C stops both.

The same start, later, from the checkout:

```bash
go run ./cmd/new-module -start demo -dir ../nozzle-watch
```

`-start app` starts the real app instead, on its own data under
`bin/<name>-app` in the checkout. The real app asks for an account on its
first run. `-start` writes nothing, and when that core does not have the
module yet it runs `install.sh` first.

`install.sh` in the generated tree is the build and the install:

```bash
cd ../nozzle-watch && sh install.sh
```

It prints each step's number as the step starts, from 1. It builds the
component library if that has not been built, installs the module's
dependencies and builds the bundle. It then builds this platform's binary when
there is a process. A process-only module skips pnpm and writes its release to
`dist/`. Last it installs the release with `prusa-connect module add
-approve`, into a configuration of the module's own under the checkout's
`bin/<name>-demo`, and prints one line saying where. `TARGET=app sh
install.sh` installs into `bin/<name>-app`, the real app's data, instead.

What the tools print goes to `install.log` in that directory. It is shown only
when a step fails, and then only its last thirty lines. The core and the dev
server that `-start` runs write `core.log` and `vite.log` beside it.

A changed release is a new install. Every rebuild moves the digest the install
recorded, and the core reads its declarations when it starts. There is no mode
in which the core reads a release off disk without verifying it.

### Iterating

The dev server answers the bundle from `src/`, in front of the core, so an
edit to a component is replaced in place with no reload. The rest of the page
is the core's app, reached through the dev server. To start it yourself, run

    cd ../nozzle-watch/ui && pnpm dev

`PCL_CORE_URL` names the core, and `http://localhost:8080` is the default. It
has to serve the built app, so it is a core or `vite preview` of `frontend/`,
never `frontend/`'s dev server, which answers the same paths this one does.

Two things still need `sh install.sh`. One is a change to `module.yaml`,
because the core reads what a module contributes when it installs it. The
other is a look at the built release. The core has no dev mode.

### Where each contribution lands

Sign in, and the app draws what the manifest declared:

<!-- generated by make generate-slots for the slot table. Do not edit: change sdk/manifest/slots.go and run make generate-slots -->

| slot | manifest key | where it lands |
| --- | --- | --- |
| `page` | `pages` | A whole screen with a route of its own, and an entry in the sidebar unless it is hidden. |
| `settings` | `settings` | A tab on the module's own page under Settings, Extensions. |
| `device-card` | `device_card` | One line on each printer's card in the listing, under its status. Text past one line is cut. |
| `device-section` | `device_section` | A section at the foot of a printer's Status tab, after the core's own. |
| `device-tab` | `device_tab` | A tab of the module's own on a printer's page, after the core's tabs. |
| `dashboard-card` | `cards` | A card a person places on a dashboard, sized on its grid and drawn with the settings the person chose. |
| `device-action` | `device_action` | An item in the action menu at the top of a printer's page. The module runs it with the printer's id. |
| `file-action` | `file_action` | An item in a file's menu, in the listing and on its page. A primary one is a button on the page. |
| `print-section` | `print_section` | A section of the print dialog, below the printers, for a file on a storage of this server. |
| `file-view` | `file_view` | What the file holds, drawn in place of the core's part, in the Contents tab of the file's page. |
| `file-tab` | `file_tab` | A tab of the module's own on a file's page, after the core's tabs. |
| `camera-section` | `camera_section` | A section under each camera in a printer's Cameras tab. A module draws nothing for a camera it does not know. |
| `camera-view` | `camera_view` | The live view of a camera of a type the module offers, drawn over its picture while a person plays it. |

<!-- end generated -->

A rail entry sits below the app's own destinations and a section below the
core's own. A settings pane is a tab on the module's page, not drawn under the
list of what is installed. The heading over each one is the `title` the
manifest gave it.

Settings, Extensions also lists every installed module that has a browser half,
with its `summary` and a link to each page it declares. A module the app would
not run is named there too, with the reason and its code. A `library` range this
app is outside of reads `library-mismatch`. A bundle that threw while it
registered reads `threw`. A registration the manifest never declared reads
`undeclared`. Nothing goes to the browser console, and a module that fails costs
its own page and never the app.

The rail is drawn from the listing and not from your bundle, so your page is in
the menu before the bundle is fetched. The bundle is imported when your page
first opens or your first section first renders.

## Installing a module

Three commands, on the binary the server already is. They work on a server that
is not running. None of them writes your own configuration file: the
declaration goes into the machine-owned overlay `core.overlay` names.

| command | does |
| --- | --- |
| `prusa-connect module add <module path>@<version> -from <release>` | installs a release and writes its declaration |
| `prusa-connect module list` | every module this deployment declares, and where its files are |
| `prusa-connect module remove <module or kind:name>` | takes the declaration and the files away. `<module or kind:name>` is the module path, the instance name, or `kind:name` |

A release is one directory. It holds the manifest at its root, one binary per
platform named `<name>_<GOOS>_<GOARCH>`, the built bundle under `ui/`, and an
optional `checksums.txt` the install verifies. A release that carries
`checksums.txt` has to list every file in it but `checksums.txt.sig`, and one
holding a file the list leaves out is refused. `-from` also takes a `.tar.gz`
of that tree. An archive entry whose name starts with `._`, the AppleDouble
file macOS `tar` adds, is skipped and never installed. Fetching a release
from a repository by tag is not in this build.

```console
$ prusa-connect module add github.com/acme/spools@1.0.0 -from ./release
installed com.github.aux.acme.spools/spools as sink:spools
  files       config/config.d/modules/spools
  namespace   com.github.aux.acme.spools
  checksums   4 files verified against checksums.txt
  enabled     no. Run the same line with -approve, or switch it on through the API
  asks for    permission observe

The record is written to config/config.d/resources.json. No core is answering
on :8080, so the next start reads it.
```

The declaration is written switched off. The keys that are a request rather
than a grant, `permissions`, `checks`, `check_absent_people`, `list_people`, `calls`, `records_mib` and `events`, are left out
until you say yes. `-approve` writes them and switches the instance on.

`-approve` also writes what the module needs to reach a printer. A module reads
the fleet as its own principal, which is `sink:` and the name of the instance,
and that principal holds nothing until a grant is written for it. So `-approve`
writes, for each permission the manifest asks for, the smallest role on the
organization that reaches it, and prints every grant it wrote:

```console
$ prusa-connect module add github.com/acme/spools@1.0.0 -from ./release -approve
installed com.github.aux.acme.spools/spools as sink:spools
  files       config/config.d/modules/spools
  namespace   com.github.aux.acme.spools
  checksums   4 files verified against checksums.txt
  granted     org:local#viewer@sink:spools for observe. The viewer role also carries view_resource, read_storage
  enabled     yes
```

### Offering a release on the Extensions page

To install from the page instead of the command, name the release directory in
`core.modules_local`, under the module path it installs as. A manifest never
names its own path, so the key is where you say it.

```yaml
core:
  modules_local:
    github.com/acme/spools: /srv/modules/spools
```

The page lists one card per entry, with the version, kind and requests the
directory's manifest states, and installs from that directory. The directory is
read on each listing: rebuild it with a higher version and the card shows an
Update. A directory that cannot be read is skipped and logged, and a row in
`index.json` for the same module path wins over the entry.

### What the viewer role on the organization carries

Read the second half of that line. A grant is a role and not an action, and the
role reaches more than the permission it was written to answer. The viewer role
on the organization gives the module three things:

| action | on what | what the module can then do |
| --- | --- | --- |
| `observe` | every printer of this server, including one admitted tomorrow | read telemetry, state, the current job and the file listing |
| `view_resource` | every resource kind | list the instances of each kind and read their declarations, with the settings marked secret redacted |
| `read_storage` | every resource kind | browse the entries of every storage instance and download a file's bytes |

So a module that asked for `observe` can also read this server's configured
storages and the files in them. Approve it on that basis, or write the grant on
a single printer instead, which reaches that printer and no resource kind:

```
POST /api/v1/grants/device/{id}
{"relation": "viewer", "subject": "sink:spools"}
```

### What `-approve` does not grant

A module holds the viewer role on the organization and no role above it, so a
permission that needs a higher role is named and not granted. You write that
one on each printer you choose to give the module. The module may still ask
whether a person holds it. A module that only asks lists the action under
`checks:` instead, and nothing is withheld. A permission the
authorization graph does not hold, and one no role on the organization reaches,
both stop the install before anything is written.

The grant is written as the principal `core.principal` names. The server makes
that principal the admin of the organization at the first start, and again at
the first start after `core.principal` changes. If an administrator withdrew
those grants, the graph holds nothing for it. The install then refuses and writes
nothing, rather than leaving a module switched on with its permissions recorded
and no grant.

`prusa-connect module remove` takes these grants back with the module,
and so does `DELETE /api/v1/modules/<namespace>`. Installing the same module
again from a release whose manifest dropped a permission takes back the grant
that permission justified.

Approving an install over HTTP writes the same grants. `POST /api/v1/modules`
with `approve` writes them as the person who called it, so that person needs
the admin role on the organization. The answer lists each permission a grant
was written for under `approved`. Its `settings` field takes what `-settings`
reads from a file, and its `state` says whether the module started, with the
reason under `reason` when it did not.

The same admin role is needed whenever `approve` records what the module may
do as itself. That includes a permission no grant answers, such as operate,
`checks`, `check_absent_people` and `list_people`. A reinstall that takes a grant back needs it too, and a
caller without it is refused before any file changes.

A reinstall cannot tell a grant it wrote from the same grant an administrator
wrote. If the new manifest no longer requests observe, the reinstall withdraws
the viewer grant, whoever wrote it.

`DELETE /api/v1/modules/<namespace>` takes the grants back as the principal
`core.principal` names, and not as the caller. Taking a grant away gives nobody
anything, so the caller's own role does not matter for it.

A reinstall that sends no settings keeps each stored setting the new manifest
still declares with the same type. A credential sent back as `xxxxx`, which is
what a read shows in its place, keeps the stored credential, the same as
put-resource does. A release from another module path under the same name is
refused with `MODULE_NAME_TAKEN`, so one module never receives another's
credentials.

The version after the `@` is a range. `@1.0.0` takes that release, `@^1.0.0`
takes whatever release of that line the directory holds, and `@*` takes what
the manifest says.

### Giving a module more room for its records

A module keeps its records (its own rows, such as names, notes and lists) in
the server's record store. Each module has 8 MiB there unless you grant it
more. Its files are in a storage you chose and do not count.

The record store is served only while a record backend is declared under
`resources.db`, and the first-run wizard declares none. On a server without
one, a module that requires a `state.*` capability waits and is not ready. A
module that does not require one reads `CodeMethodNotFound` on every `state.*`
call. A module that keeps more than settings is better off with a database of
its own.

A module that needs more says so in its manifest, as `records_mib`. The install
lists the request, and `-approve` grants it:

```console
$ prusa-connect module add github.com/acme/parts@1.0.0 -from ./release
  asks for    records 128 MiB

$ prusa-connect module add github.com/acme/parts@1.0.0 -from ./release -approve
  records     128 MiB, where the default is 8 MiB (records_mib)
```

To grant a different amount, more or less, name it with `-records-mib`. It
works only with `-approve`, and it takes 1 to 1024:

```console
$ prusa-connect module add github.com/acme/parts@1.0.0 -from ./release -approve -records-mib 512
  records     512 MiB, where the default is 8 MiB (records_mib)
```

Run the same line again with another number to change the grant later. The new
limit holds from the module's next start. A lower limit deletes nothing: the
module keeps every record, and a new write is refused until it is back under
the limit. Over HTTP the same grant is `records_mib` in the body of
`POST /api/v1/modules`, beside `approve`.

The grant is written into the declaration as `service.records_mib`.
`GET /api/v1/state/namespaces` answers what each module holds, as `bytes`,
beside the `limit` it is held to.

**A core that is already running does not read this.** It reads the
declaration file when it starts, so restart it. The command says which of the
two happened.

### Letting a module listen on a port

A module that receives a protocol of its own, such as printer log lines over
UDP, lists its ports in its manifest. A port can be reached from the network,
so `-approve` does not grant it. The install says which ports are still
waiting:

```console
$ prusa-connect module add github.com/acme/spools@1.0.0 -from ./release -approve
  not granted listen udp 13514 as syslog (printer log lines). Run the same line with -approve -approve-listen
```

Add `-approve-listen` to grant them. To use another port, name it with
`-listen`, as name and port pairs:

```console
$ prusa-connect module add github.com/acme/spools@1.0.0 -from ./release -approve -approve-listen -listen syslog=5514
  listens on  udp/5514 as syslog (printer log lines)
```

The install refuses a port below 1024, two names on one port, and a port
another instance or the server's own API already uses. It refuses before it
stops the installed module, so that module keeps running. Move the port with
`-listen` and run the line again.

The grant is written into the declaration as `service.listen`. An update keeps
the port you chose, as long as the port's `why` did not change. A new `why` is
a new request, and the update needs `-approve-listen` again. Over HTTP the same approval is `approve_listen` in the body
of `POST /api/v1/modules`, and `listen` moves a port. The server does not stop
a module from opening a port it was not granted.

A module that asks for a data directory gets `module-data/<name>` beside the
modules directory, with `-approve`. An update and a removal keep it. The
server writes a `.owner` file into it that names the module. A different
module installed later under the same name does not start on that folder.
Remove the folder or move it away first.

### Giving a module a database of its own

A module that asks for a database of its own gets one with `-approve`. By
default it is a SQLite file in the module's data directory. Choose another
place with `-database`:

```console
$ prusa-connect module add github.com/acme/notes@1.0.0 -from ./release -approve -database postgres -database-dsn "postgres://notes:secret@db.lan:5432/notes"
  database    postgres db.lan:5432/notes
```

`-database provision` makes a new database and a login for the module on the
server's own Postgres server. An update keeps the place the module has unless
you name another. A removal keeps the database, and `module purge` deletes it.
[The database page](config/database.md#a-modules-own-database) says what each
place needs, how to move and back up a module's database, and how to purge
one. Over HTTP the choice is `database` in the body of `POST /api/v1/modules`,
as `{"mode": "postgres", "dsn": "..."}`.

### Installing from a marketplace

`core.marketplace.url` names a marketplace whose Connect catalogue this server
reads.
`GET /api/v1/modules/index` then lists its modules after the local
`index.json`, for this build's version and platform, with the release on
offer, its digest and the permissions it asks for. An installed module carries
`installed_version`, so a row whose `version` is newer is an update.

The server reads version 1 of the marketplace's API, at
`/api/connect/v1/extensions`. That version only grows compatibly, and a
breaking change gets a new path.

The catalogue is read a page at a time. The server follows each page's `next`
address while it stays on the marketplace's own origin and on the catalogue's
path. It reads 20 pages and 4 MiB at most, within one `core.marketplace.timeout`.
A walk that breaks any of those keeps the copy from the last walk that
finished. A marketplace that does not page answers one page, and that page is
the catalogue.

```yaml
core:
  marketplace:
    url: https://plugins.example
```

To read no marketplace, switch it off. The server then fetches nothing from
it, and an install needs a release path or address in `from`:

```yaml
core:
  marketplace:
    enabled: false
```

The server reads the catalogue only when the module index is asked for, and
downloads only when an administrator installs. A request carries this build's
version and platform, and no credential and no instance id. The marketplace
also sees this server's address.

A person can switch the marketplace off on the Extensions page, which is
`PUT /api/v1/modules/marketplace` with `{"enabled": false}`. It takes the
admin role on the organization. The switch is
stored in the machine-owned overlay and applies at once. The index then lists
only the local rows and nothing is fetched from the marketplace. An install
that names no `from` is refused with `MARKETPLACE_DISABLED`. Modules already
installed from it keep running. `GET /api/v1/modules/marketplace` reads the
switch back, and both answer 404 on a deployment whose configuration switches the
marketplace off.

`POST /api/v1/modules` with no `from` installs what the marketplace offers for
that module and exact version, from its address and against its digest. A
release fetched from anywhere is hashed while it is written, and a `sha256`
that does not match refuses it with `MODULE_DIGEST_MISMATCH` before anything is
unpacked. A `from` URL takes `sha256`. A download is https, and so is every
redirect it follows. It never connects to this machine or the local network,
except to loopback when the marketplace itself is configured on loopback over
http, for local development. A failed download answers `MODULE_DOWNLOAD_FAILED`.

An install with `approve` grants only what the operator was shown. The
release's permissions, `checks`, events, `check_absent_people`, `list_people`, `records_mib` and `memory_mib` must be
within what the marketplace lists for it, and within `expect` when the request
carries it. Anything more is refused with `MODULE_REQUESTS_CHANGED`. A release
whose name another module already holds is refused with `MODULE_NAME_TAKEN`.

Installing another release over an installed module is the update. The running
process is stopped before its files are replaced, and the instance starts on
the new release. With `approve`, a permission or an event the operator took
away stays away, and a records room they set is kept. The answer's `added`
lists what the new release asks for that the old one did not.

A release may carry `checksums.txt.sig`, the author's signature over
`checksums.txt` that a marketplace checks when the release is published. The
install does not read it.

### Installed modules switched off by the operator

An operator who wants only the modules they declare themselves writes this in
the configuration file:

```yaml
core:
  modules_enabled: false
```

The server then refuses every install with `MODULES_DISABLED` (409), before it
reads or downloads anything. A module it installed earlier does not start, so
its bundle and its routes are not served, and the resources listing shows it as
`disabled` with the reason `ModulesDisabled`. A module the operator declares
under `resources` in the configuration file still runs, because the operator
chose it. If your module must run on such a server, ask the operator to declare
it there, as [the declaration an operator writes](#the-declaration-an-operator-writes)
shows. In safe mode no module runs, a declared one too, and the listing shows
the reason `SafeMode`; see [safe mode](config/extensions.md#safe-mode).

### Connecting outside Prusa Connect

A module that connects anywhere outside Prusa Connect lists each connection
under `egress` in `module.yaml`. The operator reads the list at install, and
the marketplace checks it against the code of every release. A module with no
`egress` connects nowhere outside.

```yaml
egress:
  - host: materials.example.net   # a host, *.domain one level deep, or an IP address
    ports: [443]                  # 443 when absent
    from: process                 # process, or ui for the page in the browser
    why: Downloads the material catalogue.
    sends: Only the request for the catalogue.
  - option: printer_url           # the host the operator writes in this option
    why: Sends jobs to the server the operator picks.
    sends: The G-code and the name of the job.
```

Each entry names one `host` or one `option`. The option has to be a `string`
or `url` option of the module. `why` takes at most 200 characters and `sends`
at most 300.

### Publishing to the marketplace

A release reaches the [Prusa Plugin Marketplace](https://plugin-marketplace.prusa3d.com)
through one path: the GitHub Action
[prusa3d/prusa-connect-marketplace-publish](https://github.com/prusa3d/prusa-connect-marketplace-publish),
run by a workflow in the module's own repository. The Action checks the release
the way the marketplace does, signs `checksums.txt` with the author's key and
uploads it. The marketplace knows the workflow by its OIDC token, so the
repository stores the signing key and no marketplace password.

Set it up once, as the Action's README says:

1. Register the public half of an RSA key under **Signing keys** in the author
   console. Store the private half as the secret
   `PRUSA_CONNECT_EXTENSION_SIGNING_KEY` in a GitHub environment.
2. Add a trusted publisher under **Trusted publishing**: the module's id, the
   repository, the workflow file and the environment.
3. Publish a GitHub release whose tag ends with the `version` in `module.yaml`.

The marketplace holds each release in review until a person approves it.
[prusa3d/prusa-connect-extension-example](https://github.com/prusa3d/prusa-connect-extension-example)
is a module with this workflow in place. Start from it with **Use this
template** on GitHub.

## Version ranges

Three keys carry a range: `core` in the manifest, `core` in a `requires` entry,
and `ui.library`. They are written the way a module author writes a range in a
`package.json`:

| written | means |
| --- | --- |
| `1.2.3` | that release and no other |
| `>=1.2.3` | that release or above |
| `>1.2.3 <2.0.0` | both bounds have to hold |
| `^1.2.3` | up to the next change that may break: `>=1.2.3 <2.0.0`, and `<0.3.0` for `^0.2.3` |
| `~1.2.3` | patches of that minor: `>=1.2.3 <1.3.0` |
| `1.2` or `1.2.x` | any patch of that minor |
| `1` or `1.x` | any minor of that major |
| `*` | any version |
| `1.2.3 - 2.3.4` | at least the first, at most the second |
| `^1 \|\| ^2` | either alternative |

A version carrying a prerelease, such as `1.2.3-rc.1`, is in a range only where
the range names a prerelease of the same three numbers. So `>=1.0.0` refuses
`2.0.0-rc.1`. A candidate for a major release is where a breaking change
lands.

A core that is a prerelease is the one exception. `core` and `requires` hold
it as its three numbers, so a tester of `0.1.0-beta.2` can install a module
whose range is `>=0.1.0`. A range that names a prerelease of the core's own
three numbers holds the whole version: `>=0.1.0-beta.2` refuses
`0.1.0-beta.1`.

A range no version can satisfy is refused when the manifest is read, rather
than matching nothing at install.

## Start it

Restart the server and watch the log. The host verifies the digest, launches the process, and asks it `plugin.info`:

```
level=INFO msg="the child process is serving" kind=sink resource=spools
  path=C:\src\spool-tracker\plugins\spool-tracker_windows_amd64.exe
  plugin=spool-tracker plugin_version=0.1.0 contract=1.0.0 wire_minor=0
level=INFO msg="resource state changed" resource=sink:spools
  type=com.github.aux.acme.spool-tracker/spool-tracker state=ready result=success serving=true
```

Readiness says the same thing to a program:

```console
$ curl -s http://127.0.0.1:8080/api/v1/readiness
{"ready":true,"serving":1,"declared":1,"blocked":[],"instances":[{"resource":"sink:spools","state":"ready","required":false,"serving":true}]}
```

A module has 30 seconds to reach serving, which is the supervisor's start budget for every instance.

## Prove it works

The four requests, in order, against a fresh install:

```console
$ curl -s http://127.0.0.1:8080/api/v1/modules/com.github.aux.acme.spool-tracker/spools
{"spools":[]}

$ curl -s -X PUT -d '{"material":"PETG","colour":"black","grams":720}' \
    http://127.0.0.1:8080/api/v1/modules/com.github.aux.acme.spool-tracker/spools/petg-black
{"by":"user:operator","event":"published","stored":"petg-black","version":1}

$ curl -s http://127.0.0.1:8080/api/v1/modules/com.github.aux.acme.spool-tracker/spools/petg-black
{"material":"PETG","colour":"black","grams":720}

$ curl -s http://127.0.0.1:8080/api/v1/modules/com.github.aux.acme.spool-tracker/spools
{"spools":["petg-black"]}
```

`"by":"user:operator"` is the principal the host authorized. On a server with no identity provider that is the one subject every request is served as; see [run the server](./getting-started.md).

The record lives in the host's database, not in the process, so it survives a restart. Stop the server, start it again, and ask:

```console
$ curl -s http://127.0.0.1:8080/api/v1/modules/com.github.aux.acme.spool-tracker/spools
{"spools":["petg-black"]}
```

The operator can see what the module is keeping, without a second door onto the data:

```console
$ curl -s http://127.0.0.1:8080/api/v1/state/namespaces
{"items":[{"namespace":"com.github.aux.acme.spool-tracker","instance":"sink:spools","keys":1,"bytes":64}]}
```

## Say that something happened

The `PUT` above answered `"event":"published"`. That is the module telling the rest of the deployment that a spool moved, on the same event stream every other producer publishes on. A dashboard watching the stream sees it without knowing this module exists.

Open the stream before writing a spool, because a subscriber receives what is published after it attaches:

```console
$ curl -sN http://127.0.0.1:8080/api/v1/events
retry: 2000

id: 01M13V2DQN2SV1SQ1XH0S3ARAW:sink/spools:4
event: com.github.aux.acme.spool-tracker.spool.changed
data: {"specversion":"1.0","id":"01M13V37FJ1R7FTQDPCE1YMFZR","source":"urn:prusa:site:local/sink/spools","type":"com.github.aux.acme.spool-tracker.spool.changed","time":"2026-08-28T09:26:57.7784398Z","subject":"sink:spools","sequence":"00000000000000000004","epoch":"01M13V2DQN2SV1SQ1XH0S3ARAW","hops":0,"evclass":"state","data":{"material":"PETG","colour":"black","grams":720}}
```

It is an ordinary frame. Same envelope, same cursor, same `id:` line to resume from, and the module's own body in `data`.

### Where the two halves come from

A consumer routes on two things, and the module names neither.

| half | comes from | why not the module |
| --- | --- | --- |
| `type` | `com.github.aux.acme.spool-tracker` is the namespace the host derived from the type the operator installed. `spool.changed` is the name the operator wrote under `service.events` | a module that could mint a type could shadow a core type, publish under another module's namespace, or emit something no subscriber can route |
| `source` and `subject` | the instance, `sink:spools`, minted by the host from the ref in the operator's file | the hub routes on origin, so this is what decides who receives the frame |

What the module names is the endpoint, `changed`, which selects a row of the operator's table, and the payload. There is no field on the wire for anything else, which is the same rule a script tier sink follows.

`com.prusa.connect.` is closed. Every type in it is a published, versioned identifier, so a name that composes into it is refused whoever wrote it.

### What the operator writes

```yaml
      service:
        # ...
        events:
          changed: spool.changed
```

The name on the right is the part after the namespace. Write `spool.changed` and the wire carries `com.github.aux.acme.spool-tracker.spool.changed`.

A name the event grammar refuses never starts. The instance fails alone and the log names the key and the type it would have produced:

```
level=WARN msg="a declared resource instance will not load and is not running" instance=resources.sink.spools
  error="resource: invalid configuration: install: invalid declaration: config/config.yaml: resources.sink.spools:
  service.events[\"changed\"] publishes \"com.github.aux.acme.spool-tracker.Spool Changed\": invalid envelope:
  type \"com.github.aux.acme.spool-tracke...\" carries \"Spool Changed\", and a segment is lowercase letters,
  digits, '-' and '_'"
```

### A module with no table publishes nothing

The table is the whole permission. Take it out, restart, and the same binary can no longer emit, because the host installs no `bus.emit` method on its session:

```console
$ curl -s -X PUT -d '{"material":"PLA","colour":"galaxy","grams":1000}' \
    http://127.0.0.1:8080/api/v1/modules/com.github.aux.acme.spool-tracker/spools/pla-galaxy
{"by":"user:operator","event":"busapi: bus.emit: wirerpc: -32601: this side serves no method bus.emit","stored":"pla-galaxy","version":2}
```

The write still happened. An event is one call and a record is another, and a module that could not publish is not a module that could not work.

That is the default: an installed module is silent until an operator says what it may publish. An endpoint the table does not carry is refused the same way, and the refusal names the endpoints it does carry.

### What a reader receives

Nothing you have to grant twice. A subscriber is attached to the origins it may see, and a module is a configured instance like any other, so a caller who holds `view_resource` on `sink:spools` receives what it emits and a caller who holds nothing about it receives none of it. The permission is the module's, not the event type's.

Two bounds are worth knowing before writing a producer. One payload is 8 KiB, so a module with more to say keeps it in a record and emits the key. One instance publishes one event at a time, and eight emissions in flight is the limit; past that the call is refused with `busapi.ErrBusy` rather than queued, and the refusal names the width.

### Following a print

A module that wants to know what the fleet is printing reads two event types instead of polling. `bus.subscribe` with `com.prusa.connect.device.job.started` and `com.prusa.connect.device.job.settled` gives one frame when a print begins on any printer the module may see, and one when it stops. Both carry `job_id`, which is the id of the job row and joins the pair. The settled frame also carries `outcome`, one of `finished`, `failed`, `cancelled` and `unknown`. Beside it are the machine's own last word, the last progress, how long the machine says it printed and how much filament it says the print used. Both are state events, so a module that saved its cursor and was restarted is given the frames it missed, and neither is published twice for one print.

Who started the print is not on either frame. The started frame carries the `command_id` of the instruction that began it, and none for a print somebody made at the machine's own screen. A module that holds `view_audit` reads the person from `com.prusa.connect.device.command.authorized` under the same id.

```go
err := busapi.Subscribe(ctx, conn, busapi.SubscribeParams{
    Types:  []string{"com.prusa.connect.device.job.started", "com.prusa.connect.device.job.settled"},
    Resume: saved,
}, func(window busapi.Window) error {
    for _, envelope := range window.Events {
        // Each one is a CloudEvents envelope. Read its type, then decode its
        // data member into event.JobStarted or event.JobSettled.
    }

    return nil
})
```

### Reacting when something is wrong with a printer

A module that tells somebody about a printer in trouble, or acts on it, reads the same issues a person sees in the listing. Each device that `device.get` and `device.list` answer carries `Issues`, worst first. An issue has a `Reason` to branch on, such as `awaiting_operator` or `disconnected`, its `Severity` and a `Detail` in the machine's own words. [Device issues](./device-issues.md) lists every reason.

`com.prusa.connect.device.issues.changed` says when that list changes, with the whole list in the frame, so a module does not poll. Changes that arrive together, such as the paths of one dialog, leave as one frame, and a list that did not change publishes nothing.

A printer that waits for a person carries `Dialog` on its record and in the frame. It holds the firmware's `Code`, the `Title` and `Text` the screen shows and the `Buttons`. A module answers it with `core.command.dialog_action`, passing `ID` as `dialog_id` and one of the buttons as `button`. That command takes `operate` on the printer, as a person's answer does.

```go
err := busapi.Subscribe(ctx, conn, busapi.SubscribeParams{
    Types:  []string{"com.prusa.connect.device.issues.changed"},
    Resume: saved,
}, func(window busapi.Window) error {
    for _, envelope := range window.Events {
        var change struct {
            DeviceID string             `json:"device_id"`
            Issues   []deviceapi.Issue  `json:"issues"`
            Dialog   *deviceapi.Dialog  `json:"dialog"`
        }
        if err := json.Unmarshal(envelope.Data, &change); err != nil {
            continue
        }
        // Tell somebody, or answer change.Dialog when its Code is one this
        // module knows how to handle.
    }

    return nil
})
```

## Reading what a fleet printed

`device.jobs` answers the print on the bed and the one before it. `device.job_history` answers the rest: a page of the prints that have ended, newest first, on one device or on every device the caller may see.

Leave `id` out for the whole fleet. The permission is resolved inside the query. So the page carries the prints of the devices the caller may observe and nothing of the others, in its rows and in its paging. `from` and `to` bound when the print ended, which is what a report over one month asks for. Paging follows `cursor`: send back what the last page answered, and stop when it comes back empty.

```go
page, err := fleet.JobHistory(ctx, deviceapi.JobHistoryParams{
    From:  month,
    To:    month.AddDate(0, 1, 0),
    Limit: 200,
})
```

It takes `observe`, which is what `device.jobs` takes: a print is a reading about a machine. A module's own read is held to its `permissions:` list first, so an instance installed without `observe` reads an empty page.

The same range is on the HTTP route, `GET /api/v1/devices/{device_id}/jobs/history?from=...&to=...`, through the same query.

## Printing a file

`device.print` starts a print of a file one of the server's storages holds. It takes the storage, the file's path inside it, and where the file lands on the printer. A request naming no storage prints a file the printer already holds, and `path` is then the file that is printed.

It takes the two permissions the HTTP print route takes: `operate` on the printer, and `read_storage` on the storage kind when a storage is named. A module's own print is held to its `permissions:` list on both. So an instance installed with `operate` alone prints what a machine already holds, and reads no file out of a storage.

The call answers when the transfer has started, not when the print has begun. A file of a few hundred megabytes takes minutes to reach a printer. The server goes on relaying it after the call has answered, and sends the start command itself when the last byte has arrived. The answer carries `transfer_id`, and a module follows the rest with `device.transfer.settled` and `device.command.settled` on the event stream. A print of a file the printer already holds moves nothing, so that one answers with `command_id` instead.

A module whose data directory was granted can print a file it keeps itself. It writes the file under `print/` in that directory and names the storage `deviceapi.ModuleDataStorage`, which is `module:data`, with `source` relative to `print/`. The server reads that folder for this module's prints and for nothing else, and takes the same two permissions. A module without a data directory is refused with `deviceapi.ErrDenied`.

```go
started, err := fleet.Print(ctx, deviceapi.PrintParams{
    Principal: principal,
    ID:        deviceID,
    Storage:   "shelf",
    Source:    "models/bracket.bgcode",
    Path:      "usb/bracket.bgcode",
})
```

A storage this server does not declare and a file it does not hold are one refusal, `deviceapi.ErrNoFile`. A module cannot read the list of storages out of the refusals it collects. A source that climbs out of its storage is `deviceapi.ErrInvalidQuery`, and it reaches neither the storage nor the printer. A path where the printer already holds a file or a folder is `deviceapi.ErrNameTaken`, and nothing is sent. The exception is a copy this server sent there itself from the same, unchanged file, which the printer starts with no transfer. A module may print the file again under another name.

A print that answers with `command_id` has already had the printer's answer to the start command. When the printer refused it, the call fails with `deviceapi.ErrPrintRefused`. Nothing happened, so the module may start it again. When the printer took the command and then failed, the call fails with `deviceapi.ErrPrintFailed`. The printer may have begun, so the module reads its state before it starts the print again. Both messages carry the printer's own reason where it gave one. A print that answers with `transfer_id` sends the start command later, so a refused or failed start reaches the module as `device.command.settled` with `ok` false.

## Reading a file in a storage

A module that has to look inside a G-code file reads it through `storage.stat` and `storage.read`. `stat` gives the file's length, when it last changed, and the digest the storage kept if it keeps one. `read` answers one range of at most 512 KiB, so a module walks a large file with the offset and neither side holds it whole.

Both take `read_storage` on the storage kind, which is what the HTTP storage routes take. A call made for a person is decided for that person. A call the module makes as itself is held to its `permissions:` list first and then decided for `sink:<instance>`.

```go
files := storageapi.NewClient(conn)

info, err := files.Stat(ctx, storageapi.StatParams{
    Principal: principal, Storage: "shelf", Path: "models/bracket.bgcode",
})
if err != nil {
    return err
}

// The slicer writes its settings at the head of the file.
head, err := files.Read(ctx, storageapi.ReadParams{
    Principal: principal, Storage: "shelf", Path: "models/bracket.bgcode",
    Length: 8 << 10,
})
```

A storage this server does not declare, a file it does not hold and a path that leaves the storage all answer `storageapi.ErrNoFile`, one refusal. The storage contract joins absence and escape on purpose, so neither says which names in a store resolve outside it. Reading a directory answers `storageapi.ErrUnreadable` instead.

`storage.list` answers the entries of one folder: the name, whether it is a folder, a file's size, the time it last changed and the SHA-256 the storage kept. It takes what `storage.read` takes. A page holds at most 1000 entries in the order of their names. A module passes `Next` back as `Cursor` until `Next` is empty, and names `core-storage.list: "*"` under `require`.

### Reading a large file

`storage.read` answers base64 inside JSON, at most 512 KiB a call. A module that reads a whole project, or any file of many megabytes, opens the file instead. `Open` answers a `storageapi.File`, which is an `io.ReaderAt`. Each read asks `storage.fetch`, which sends up to 16 MiB of bytes as a stream with no JSON around them. A 100 MiB file reads about fifty times faster this way.

```go
file, err := files.Open(ctx, storageapi.Target{
    Principal: principal, Storage: "shelf", Path: "models/bracket.3mf",
})
if err != nil {
    return err
}

// archive/zip reads the central directory and one entry, not the whole file.
archive, err := zip.NewReader(file, file.Size())
```

`Open` takes what `Stat` takes, and every read takes what `Read` takes. The session the client is built over must open streams, which `*wirerpc.Conn` does.

## Writing a file into a storage

A module that makes a file, such as a slicer that turns a project into G-code,
puts it into a storage through `storage.write`. The call goes on a stream of
its own, and the bytes of the file follow it on that stream. So neither side
holds the whole file, and a file of a few hundred megabytes costs no larger
frame than a small one.

The call names the storage, the path and the size in bytes. The host writes
the bytes under a temporary name and publishes the file when the last byte has
arrived. A stream that carries fewer or more bytes than the size is
`storageapi.ErrShortWrite`, and the storage keeps the file it held before. A
file of that name is replaced. A folder on the way is made.

```go
files := storageapi.NewClient(conn) // conn is the *wirerpc.Conn the module serves on

written, err := files.Write(ctx, storageapi.WriteParams{
    Storage: "shelf", Path: "sliced/bracket.bgcode", Size: info.Size(),
}, output)
// written.SHA256 is the digest the storage computed while it wrote
```

It takes `write_storage` on the storage kind, which is what the HTTP upload takes. A call made for a person runs as that person while their request is open. When the module writes as itself, the host checks its `permissions:` list before the graph, so an instance installed without `write_storage` writes nothing. Approving the install writes the grant on the storage kind for a module that lists `write_storage`, so the operator writes no grant of their own. A module that writes long after the request ended, as a queue does, asks `access.check` whether the person may still write before it writes as itself.

A file is at most 4 GiB, and an empty file is refused.

## Handing a frame to a camera

Some cameras speak a protocol of their own, such as a cloud camera pointed at
a port the module listens on. A module that serves one hands each picture to
a camera through `camera.accept`. The push camera keeps the last frame, and the
snapshot and stream routes serve it like a frame a device sent over HTTP. The
bytes follow the call on a stream of its own, as they do for `storage.write`.

```go
cameras := cameraapi.NewClient(conn) // conn is the *wirerpc.Conn the module serves on

err := cameras.Accept(ctx, "", "bed-cam", jpeg)
// errors.Is(err, cameraapi.ErrNoCamera): not declared, not visible, or not a push camera
```

It takes `operate` on the camera, which is what the HTTP push route takes. When the module sends as itself, the host checks its `permissions:` list before the graph, so the manifest lists `operate`. Approving the install writes no grant for it. Whoever sets up the camera writes `device:<camera>#operator@sink:<instance>` through the grants API, so a module reaches only the cameras it was given. A frame is a JPEG of at most 8 MiB (ADR-20261106-2er).

A module that feeds cameras can show what it knows about each one, such as whether it is connected. It can also offer the settings only it can change. Declare a `camera-section` for that. It is drawn under every camera in a printer's Cameras tab and handed the camera's name, so the section draws nothing for a camera the module does not feed (ADR-20261106-xm4).

## Offering a camera type

A module that serves a kind of camera offers it as a camera type. A person then picks it when adding a camera, beside the core's generic camera:

```yaml
camera_types:
  - name: buddy3d
    title: Buddy3D camera
```

A camera of the type is declared as `<namespace>/buddy3d`. The core runs it on its own generic camera, so no code of the module runs in the core. The module can hand it pictures with `camera.accept` at once, with no grant, for as long as the camera has the module's type. The module knows its cameras by their type, which the camera listing names.

To draw a camera's live view, such as a direct WebRTC connection to the camera, declare a `camera-view`. It is drawn over the camera's picture while a person plays it, and only for a camera of a type the module offers (ADR-20261106-km2).

## Checking that a file fits a printer

A module that picks a printer for a file asks whether the printer will take
it. `storage.stat` answers the file's needs in `Print`. `Filaments` holds one
entry per filament, by the T number that selects it, with the material, the
nozzle and whether the print uses it. `Checks` holds the lines at the start of
the G-code that the printer's firmware reads before it prints. `device.get`
answers the printer, whose state tree carries its tool tree at `/tools/<id>`
and the build volume at `/build_volume`. Each filament lands on the tool the
tree resolves its T number to: a head, or a slot of the feeder on a head.

`printcompat` holds one against the other. It is a plain function and makes
no call, so it needs no permission beyond the two reads.

```go
printer := printcompat.PrinterFromState(device.Model, device.Firmware, device.State)

verdict := printcompat.Evaluate(info.Print, printer)
if verdict.Level == printcompat.LevelRefusal {
    return fmt.Errorf("%s cannot print this file: %v", device.Name, verdict.Findings)
}
```

The verdict is `ok`, `warning` or `refusal`, and each finding names the rule,
the filament, the tool it lands on, what the file wants and what the printer
has. A warning is a print
the printer stops at a dialog for, where a person at the printer may go on.
A check whose facts are unknown is listed under `Unverified` and never counted
as passed.

## Asking whether somebody may still do something

A module acts for a person while that person's request is open. The rest of the time it acts as itself. A print queue takes a job in the afternoon and starts it at two in the morning, as itself. The person's permission on that printer is not checked at the start. So somebody who lost access on Monday would still have their job printed on Tuesday night.

`access.check` is how the queue asks before it starts. It takes the subject, the action and the object, and answers one bit.

```go
decision, err := access.Check(ctx, accessapi.CheckParams{
    Subject:    submitter,
    Action:     "operate",
    ObjectType: "device",
    ObjectID:   deviceID,
})
```

The answer is one bit. A caller that could ask it about any subject, any action and any object would read the whole permission graph one bit at a time. So the question is bounded on two sides. The action is one the instance's `permissions:` or `checks:` list names, and anything else is `accessapi.ErrDenied`. The subject is one the server handed this instance since it started, and anybody else is `accessapi.ErrUnknownSubject`. An empty subject is the instance itself.

That memory is the instance's, it holds the last 512 people, and it is lost when the instance restarts. A module that has to act for somebody it has not served since the restart reads `ErrUnknownSubject` rather than a decision, unless the operator gave it the permission below.

### Asking about an action the module does not take

A module that only asks whether a person may do something does not need to do it itself. Materials asks whether a person may operate a printer before it lets them change a tool's material, and never operates a printer. It lists the action under `checks:` in `module.yaml`, not under `permissions:`:

```yaml
checks:
  - operate
```

`module add` lists it as `check operate` and writes it only with `-approve`. A check writes no grant, so the install withholds nothing and shows no warning for it. `access.check` answers about the action for a person the module serves, and a call the module makes as itself is still held to `permissions:`. An action may not be under both keys.

### Asking about a person who is away

A queue keeps its entries across a restart, and the people who added them are not there to open the queue again. So a module may ask for one more permission in `module.yaml`:

```yaml
check_absent_people: true
```

`module add` lists it as `permission check_absent_people` and writes it only with `-approve`. With it, the same `Check` call answers about any person this server holds, at any time. The call does not change. These are the rules:

| the question | the answer |
|---|---|
| a person with an account and the grant | `allowed` true |
| a person with an account and no grant | `allowed` false |
| a deleted person, or one who never had an account | `allowed` false |
| a subject that is not `user:<id>` | `accessapi.ErrUnknownSubject` |
| a server with an OpenID Connect login or in open mode, whatever the action | `accessapi.ErrUnknownSubject`, because it keeps no list of people |
| an action outside the `permissions:` and `checks:` lists, on any other server | `accessapi.ErrDenied` |
| a burst of more than 600, or more than 10 a second after it | `accessapi.ErrTooManyQuestions`. Ask again a moment later |

The server reads the person's rights for every question, so a revoked grant shows in the next answer. On a server with local accounts, a deleted person answers `allowed` false on every path, also when the module served them since it started. Every question about a person the module is not serving counts against the bound, a refused one too. The server writes one log line for each question the bound lets through, naming the module, the person, the action, the object and the answer. The permission never lets the module act as the person. A print the queue starts is still started as the queue.

The object is not bounded and does not need to be. An object this server does not have answers exactly as one the subject reaches nothing on, `allowed` false, so the call says nothing about which printers this server holds.

### Listing the people

A module that assigns work to people has to offer a list of them. A planner assigns a run to a person and names the author of each record. So a module may ask for the list in `module.yaml`:

```yaml
list_people: true
```

The operator sees `permission list_people` among the requests, and the key is written only on an approved install. The module then calls `People` on the same client:

```go
people, err := accessapi.NewClient(conn).People(ctx)
```

Each row has three fields: `Subject` is `user:<id>`, the same principal `Check` takes. `Name` is the display name, or the username when there is none. `Kind` is `user`. The rows are sorted by name, and there are at most 5000 (`accessapi.MaxPeople`). A row never carries an e-mail address, a role or a grant. These are the rules:

| the server | the answer |
|---|---|
| keeps local accounts | one row per account |
| has an OpenID Connect login or is in open mode | an empty list, because it keeps no list of people |
| did not get the operator's approval for this module | `CodeMethodNotFound` |
| a burst of more than 600 questions and listings, or more than 10 a second after it | `accessapi.ErrTooManyQuestions`. Ask again a moment later |

The list is what the server holds at the moment of the call. Keep the subject, not the name, and ask `Check` again before you act for that person. The server writes one log line for each listing, naming the module and the number of rows.

## Calling another module

A module may call the routes of another module through the host, as a page
does. It names each module it calls under `calls:` in its manifest, by the
namespace its routes are mounted under, and names the same module under
`require:` or `suggest:`. The operator approves the list at install, as
they approve events.

```yaml
suggest:
  com.github.aux.acme.slicer: "Slices a project before it is planned"
calls:
  - com.github.aux.acme.slicer
```

The wire method is `routes.call`, and `routesapi.NewClient` makes the call
from Go. A call carries one HTTP request and answers the other module's
response, each at most 1 MiB. A file moves through a storage instead. The
path is relative to the other module's mount and stays inside it. A path
with a `.` or `..` segment answers `ErrCallInvalid`. So does a path with an
empty segment, a backslash, an escaped `/` (`%2f`) or anything escaped twice.

```go
answered, err := routesapi.NewClient(conn).Call(ctx, routesapi.CallParams{
	Module: "com.github.aux.acme.slicer",
	Method: http.MethodPost, Path: "/jobs", Body: body,
})
```

A call that names a person runs as that person, while their request to your
module is open, and only when they may use the other module. A call that
names no principal runs as your module, `sink:<instance>`. What the other
module does for it is then held to your module's own grants. So a module
that asks a slicer to write G-code into a storage asks for `write_storage`
itself. An approved install grants `read_storage` on the organization and
`write_storage` on the storage kind, because the organization cannot give
`write_storage` to a module. A module that is not installed, or not
running, answers `ErrCallUnavailable`, and one you did not name answers
`ErrCallNotDeclared`. A module that names `routes.call` in `requires` still
starts when the operator approved none of its calls, and each call answers
`ErrCallNotDeclared`.

## Who may call it

A `GET` needs `view_resource` on the instance and everything else needs `operate_resource`, decided by the same graph as every other path under `/api/v1`. A caller the graph gives nothing is refused with `core.auth/ACCESS_DENIED` before the module runs. On a fresh server the one subject is administrator of `org:local` and holds both.

## Watch it fail

A guide that only shows success proves little. Change one digit of the recorded checksum and restart:

```
level=WARN msg="resource failed to start" kind=sink resource=spools attempt=1 class=permanent
  error="core.internal/RESOURCE_PLUGIN_CHECKSUM: the artefact does not match its recorded
  checksum: C:\\src\\spool-tracker\\plugins\\spool-tracker_windows_amd64.exe
  is sha256:7c13d276a5a5bb8a854ee4ce26b5ea6d1fa577a2a379c3ef0f43d65021dc1821: wirehost: the
  artefact does not match its recorded digest"
```

```console
$ curl -s http://127.0.0.1:8080/api/v1/readiness
{"ready":true,"serving":0,"declared":1,"blocked":[],"instances":[{"resource":"sink:spools","state":"failed","required":false,"serving":false}]}
```

The instance fails alone, the failure is `permanent` rather than retried, and the server keeps serving. The same shape holds for a missing artefact, for a `contract_major` this host does not serve, and for a `require:` entry this host cannot meet. `GET /api/v1/resources/sink/spools` carries the same message in the instance's `Started` condition.

The digest proves the file is the one that was installed. It says nothing about who produced it, so record checksums only for artefacts you trust.

## The limits the host applies

| limit | value |
| --- | --- |
| one request or response body through the mount | 1 MiB |
| one record key | 150 bytes |
| one record value | 64 KiB |
| everything one namespace holds | 8 MiB, or what the operator granted |
| one event payload | 8 KiB |
| one event routing key | 128 bytes |
| emissions in flight for one instance | 8, and the excess is refused |
| start budget | 30 seconds |

A store refusal arrives as a typed error the sdk names, which is what `statusFor` above maps onto statuses.

### The headers a route sees

The host passes a route only the headers it needs to answer: `Accept`, `Accept-Language`, `Content-Language`, `Content-Type`, the `If-*` conditions, `Range` and `User-Agent`. The person's `Cookie` and `Authorization` never reach the module, so a module cannot call the server as that person. `routesapi.PrincipalOf` says who is calling. A call from another module passes the same headers.

An answer reaches the person with these headers only: `Accept-Ranges`, `Cache-Control`, `Content-Disposition`, `Content-Language`, `Content-Range`, `Content-Type`, `ETag`, `Expires`, `Last-Modified`, `Retry-After`, `Vary` and `X-Content-Type-Options`. The host drops every other header. A cookie, a redirect and the security headers of the server's origin are the server's to set. These include the content security policy, HSTS, CORS and framing.

### More room for records

A module that keeps many records asks for more room in its manifest:

```yaml
records_mib: 128
```

`module add` lists it as `records 128 MiB` and writes it only with `-approve`, as `service.records_mib`. The operator can grant another amount, as [giving a module more room for its records](#giving-a-module-more-room-for-its-records) says. Files the module keeps in a storage do not count toward it.

`Usage` on the records client answers what the namespace holds and its limit, so a page can say how close the next refusal is:

```go
usage, err := records.Usage(ctx)
// usage.Bytes of usage.Limit
```

A host older than the call answers `CodeMethodNotFound`. Read `stateapi.DefaultNamespaceBytes` then.

### Ports and a data directory

A module that listens on the network, or keeps more than records hold, says
so in a `needs` block:

```yaml
needs:
  listen:
    - name: syslog
      udp: 13514
      why: printer log lines
  data: true
  memory_mib: 192
```

Each port has a name, `udp` or `tcp` with a number, and a sentence the
operator reads. Ask for a port of 1024 or above, because the server does not
run as the system administrator and refuses a lower one. `data: true` asks
for one writable directory. Leave the `.owner` file in it alone. The operator
approves the directory with `-approve` and the ports with `-approve-listen`,
and may move a port. `memory_mib` recommends a memory budget for the module's
process, from 16 to 8192 MiB. Leave it out, or write `0`, and the server
default of 128 MiB applies. `module add` lists it as `memory 192 MiB`, a release
that recommends more than the one installed is a new request, and the operator
can set another budget with `-memory-mib` beside `-approve`. When the budgets of
the enabled modules do not fit in `core.modules_memory`, the Extensions page
shows a warning, and the install, the switch-on and the raise still go through.

The budget covers your process and every process it starts. On Linux the
server counts their anonymous memory, the heap and the stacks, and not the
files the kernel can drop. A Go module gets `GOMEMLIMIT` at 85 % of the budget,
so its collector works harder before it reaches the limit. The server also
sets `PCL_MEMORY_BUDGET_BYTES` to the budget in bytes, for a module in any
language. A Go module reads it with `wire.MemoryBudget()`, which returns
`ok` as false when the variable is not set, for example when you start the
module yourself.

A module over its budget is stopped and started again, unless the operator set
`on_over_memory: warn`, which keeps it running. A module runs in a session of
its own, and a stop kills every process the server finds under it. Three
failures within one hour switch it off until the operator switches it on.
Measure your module under real load
and recommend that figure with some room above it.

The module reads what it got beside its settings:

```go
wirerpc.MethodConfigure: wirerpc.ConfigureGranted(func(_ context.Context, held Config, grants wirerpc.Grants) error {
	port, granted := grants.Listen["syslog"] // the port may differ from the manifest's
	dir := grants.Data                       // an absolute path, empty when not granted
	...
}),
```

Listen only on a port in `grants.Listen`. A module that was granted none runs
without the listener and says so in its own status.

### A database of its own

The record store holds a few MiB of keys and values. A module that keeps more,
such as the history of every job, asks for a database of its own:

```yaml
core: ">=0.0.1-alpha.7"
needs:
  database: true
```

`core` names the first release that reads `needs.database`. An older core
refuses the manifest with `needs carries database, which this version does
not read`. A range that names a prerelease of the core's own release is held
against the whole version. So `>=0.0.1-alpha.7` tells `alpha.6` and `alpha.7`
apart. [Version ranges](#version-ranges) has the rest of the grammar.

#### Migrations

The operator chooses the engine, SQLite or Postgres, so ship your migrations
for both, one directory each, with the same file names. Name a new file by
the date and time you write it, in UTC and to the second, then a few lower case
words joined by underscores:

```text
migrations/sqlite/20261027T093015_note_tags.sql
migrations/postgres/20261027T093015_note_tags.sql
```

`date -u +%Y%m%dT%H%M%S` prints the date part. Two branches that each add a
migration then never pick the same name, and the order of two migrations is the
order they were written in. A module that already shipped files named by a
version, such as `0003_note_words.sql`, keeps those names. They run first, by
their number, and the dated files run after them, by their date.

A change SQL cannot say, such as data read, changed and written back, is a
step: Go code with a date of its own. It runs after the file of the same date:

```go
var steps = []moduledb.Step{
	{Stamp: "20261027T093015", Name: "count_tags", Up: countTags},
}

func countTags(ctx context.Context, tx moduledb.Tx, engine wirerpc.DatabaseEngine) error {
	// Read the notes kept before 20261027T093015_note_tags.sql added the
	// column, close the rows, then write each count with tx.ExecContext.
}
```

A step that shipped with a `Version`, such as `{Version: 3, Name:
"count_words"}` in `examples/database-module`, keeps it.

At each start the module applies every migration the database has not
recorded, in that order and in **one transaction**. That includes a migration
dated before one the database already holds, which is what a merge of two
branches ships. A file or a step that fails undoes the whole upgrade, and the
database stays as it was. On Postgres an advisory lock, and on SQLite the write
lock, keep a second start from migrating at the same time. A statement that
cannot run in a transaction is refused before anything runs. Those are `CREATE
INDEX CONCURRENTLY`, `VACUUM`, `ALTER SYSTEM` and a statement on a whole
database. So are the SQLite pragmas `foreign_keys` and `journal_mode`, and a
`BEGIN`, `COMMIT`, `END` or `ROLLBACK` of its own. A trigger's own `BEGIN ... END` is fine. Do
such work in the module's code after it starts.

The rules for what has shipped:

- never change, rename or delete a file or a step that has shipped. Add a new
  dated one
- an applied migration the release does not ship is refused when it sorts
  before one the release ships, because it was renamed or deleted. One that
  sorts after all of them belongs to a newer release, and the compat floor
  decides whether this release may open the database
- there are no down migrations. The way back from an upgrade is the backup
  taken before it

#### The compat floor

`Schema.Floor` names the newest migration an older release cannot survive,
such as `20261028T080000_drop_note_title.sql` when an older release still
writes that column. A release that does not ship that migration refuses the
database, and the host keeps the newer release running. A release that ships
it opens the database, applies what it lacks, and leaves the floor where it
is. Leave `Floor` empty while every migration only adds.

`Schema.Compat` is the same floor for a module whose files are named by a
version. It is a version: a release whose highest version is below it refuses
the database.

#### Opening the database

`sdk/moduledb` opens the database the operator granted and applies the
migrations. Call it inside `plugin.configure`, so the module reports ready
only on a schema it has applied, and answer its report, so the operator sees
the schema version. The sdk imports no driver, so the module imports the two it
needs:

```go
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	_ "github.com/lib/pq"  // Postgres
	_ "modernc.org/sqlite" // SQLite

	"github.com/prusa3d/prusa-connect/sdk/moduledb"
	"github.com/prusa3d/prusa-connect/sdk/wirerpc"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrations embed.FS

var schema = moduledb.Schema{Migrations: migrations, Steps: steps}

type notes struct {
	db *sql.DB
}

func (n *notes) configure(ctx context.Context, _ struct{}, grants wirerpc.Grants) (wirerpc.Configured, error) {
	if grants.Database == nil {
		return wirerpc.Configured{}, errors.New("this module keeps its data in a database of its own, and none was granted")
	}

	db, report, err := moduledb.Open(ctx, *grants.Database, schema)
	if err != nil {
		// The cause first, and String() leaves the password out.
		return wirerpc.Configured{}, fmt.Errorf("%w (the database is %s)", err, grants.Database)
	}

	n.db = db

	return wirerpc.Configured{Database: report.State()}, nil
}

// In the methods the module serves:
//	wirerpc.MethodConfigure: wirerpc.ConfigureReporting(n.configure),
```

The `dsn` is a credential. Never log it: `DatabaseGrant.String` is the form a
log line takes. A SQLite handle holds one connection, and the file is readable
by the module's own account only. `examples/database-module` is a whole module
built this way.

#### Testing against both engines

`examples/database-module/moduledb_test.go` is the pattern. Each case opens a
fresh database of each engine: a file in `t.TempDir()` for SQLite, and a new
database on the Postgres server `AUTHZ_TEST_POSTGRES_DSN` names. `make
test-engines` starts that server and sets the variable, and a run without it
skips Postgres and says so. Test at least that:

- the migrations apply once, and a second open applies nothing
- a step fills the rows an older release kept
- a failing step leaves the database as it was
- a database whose floor is above the release is refused

`make module-database-e2e` runs the example through installs, updates,
removal and purge on the built server, with SQLite and two Postgres
containers.

#### The record store beside it

The module keeps its record namespace and the default room for records. Keep
settings and cursors there, and everything else in the database. A module
with a database of its own asks for no more room for its records. The host
counts nothing in the database, and its size is bounded by the disk or the
Postgres server.

## Add files to the diagnostics bundle

An admin downloads a diagnostics bundle from Settings and attaches it to a
support ticket. A module that keeps evidence of a fault, such as the lines a
printer logged, can add text files to it. Serve `diagnostics.collect` and
answer it page by page:

```go
wirerpc.Methods{
	diagnosticsapi.MethodCollect: diagnosticsapi.Serve(func(ctx context.Context, cursor string) (diagnosticsapi.Page, error) {
		lines, next := newestLines(cursor) // at most diagnosticsapi.MaxPageBytes
		return diagnosticsapi.Page{
			Files:  []diagnosticsapi.File{{Name: "syslog/mk4.log", Text: lines}},
			Cursor: next, // empty on the last page
		}, nil
	}),
}
```

Name the method in `Info.Serves`. The host asks only when the admin ticks
the files from extensions. A page holds at most 512 files. The host takes at
most 8 MiB, 1024 files and 64 pages from one module, and gives it at most one
minute, so put the newest lines first. It scrubs every line before it writes
it under `modules/<kind>.<name>/` in the zip, and the admin can read the zip
before sending it. A part of a file name that holds an address or a MAC is
replaced with a hash, so name a file after the printer's id where you can.

## Describe the routes so a client can be generated

A page or a script that calls the module wants types for its routes. The core's client is generated from the core's OpenAPI document, and a module's client is generated the same way from a document the module ships. The document is OpenAPI 3.1 with every path relative to the mount. The host serves it at `/api/v1/modules/<namespace>/openapi.json` with the paths rebased under the mount. Nothing is fetched at runtime by a client: an author generates once and commits the result.

Write the document beside the routes, embed it, and attach it to the `plugin.info` answer:

```go
//go:embed openapi.json
var openAPI []byte

wirerpc.MethodInfo: func(context.Context, wirerpc.Call) (any, error) {
	return routesapi.Describe(
		wirerpc.Info{Name: name, Version: release, Contract: wirerpc.Contract}, openAPI)
},
```

`routesapi.Describe` refuses what the host refuses, so the reason is read where the file is written. The rule is the one the core holds its own document to. The document parses and declares `3.1`. Every path starts with `/` and carries no `/api/` prefix. Every operation carries an `operationId`, a `summary` and a `description`. `examples/service-plugin/openapi.json` is one that passes, and `openapi_test.go` beside it is the test an author copies.

A module that serves routes and ships no document does not start. `routesapi.Describe` names `routes.serve` on `plugin.info`, and a module that names it without a document is refused at launch with `core.internal/RESOURCE_PLUGIN_UNDOCUMENTED`. The log line names the module and points at `routesapi.Describe`. A module with no routes ships no document.

The API reference page renders the same document. Open `/docs?module=<namespace>` and the routes read the way the core's own do at `/docs`. The page fetches the document with your own credential. At the top of `/docs` a row of links lists `Core API` and each running module whose document you may read, by the title in its manifest. You move between them with these links, and the back button works. A module that ships no document is not in that row. A module's page may show its document in a frame, so a reader sees the routes without leaving that page. Add `embedded=1` to the address in the frame's `src`, and the row of links is left out there.

To generate a client, read the listing and then the document:

```
$ curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/resources | jq '.items[] | select(.runtime == "service") | {namespace, openapi_url}'
{"namespace": "com.github.aux.acme.spool-tracker", "openapi_url": "/api/v1/modules/com.github.aux.acme.spool-tracker/openapi.json"}
$ curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/modules/com.github.aux.acme.spool-tracker/openapi.json > spool-tracker.json
```

Then run the core's generator in its module mode. It reads the fragment from a path, or fetches the served document with the token in `PCL_TOKEN` or `-token`. It rebases every path under the mount and writes a package that binds the module's operations onto a `@prusa/connect` client. Three commands: generate, build, and the check a CI lane runs.

```sh
go run ./cmd/generate-client-sdk -module -document spool-tracker.json \
  -namespace com.github.aux.acme.spool-tracker -name @acme/prusa-connect-spool-tracker -out client
cd client && pnpm install && pnpm run generate:types && pnpm run build
go run ./cmd/generate-client-sdk -module -check -document spool-tracker.json \
  -namespace com.github.aux.acme.spool-tracker -name @acme/prusa-connect-spool-tracker -out client
```

The package holds `src/generated/operations.ts`, the rebased `openapi.json` that `openapi-typescript` renders `module-api.ts` from, and a `package.json` with a `peerDependencies` entry on `@prusa/connect` at the core's version and no `dependencies`. `src/index.ts`, `tsconfig.json` and `README.md` are written on the first run and are the author's after it. A caller writes `const spools = bind(local)` over a `PrusaConnect` and gets one method per operation, sent with the client's credential. A fragment carries no error registry and no event types, so the package ships none. A refusal is the core's `ApiError` with the module's own code as a string. `examples/service-plugin/client` is the package generated from the example's fragment, and a test in `cmd/generate-client-sdk` regenerates it and compares.

## Giving a module a page

A module with a browser part declares a `ui` block in its `module.yaml`.
The block says where the bundle starts, which library version it was built
against, and what it adds to the app.

```yaml
ui:
  entry: ui/index.js
  library: ">=0.3.0 <1.0.0"
  contributes:
    pages:
      - path: spools
        title: Spools
        icon: layers
        hidden: false
    settings:
      - id: spools
        title: Spools
```

| key | what it does |
| --- | --- |
| `entry` | the file the app imports as an ES module, relative to the bundle root |
| `library` | the range of `@prusa/connect-ui` the bundle was built against |
| `contributes.pages` | a whole screen the module adds, at `/m/<namespace>/<path>`, with an entry in the rail under Extensions. |
| `contributes.settings` | a tab the module adds to its own page under Settings, Extensions |

`entry` ends in `.js` and cannot climb out of the bundle root. `library` is a
version range in the grammar npm uses, so write it the way you write one in a
`package.json`. See [version ranges](#version-ranges).

A page carries four keys. `path` is the last part of its route. `title` names
it in the sidebar. A module with more than one page in the sidebar gets one row
there with its own `title`. Its pages are listed under that row, in the order
the manifest lists them. That row opens the first page, and its icon is
the first page's icon. `icon` is optional. It names one of the icons the
library publishes as `MODULE_ICONS`. The names are `blocks`, `box`,
`calendar-clock`, `chart-line`, `clipboard-list`, `flask-conical`, `layers`,
`list-checks`, `materials`, `package`, `palette`, `printer`, `puzzle`, `qr-code`, `spool`,
`swatch-book`, `tag`, `thermometer` and `wrench`. Any other name is refused when the manifest is
parsed, with the list in the refusal; leaving `icon` out draws the puzzle
piece. The frontend still falls back to the puzzle piece for a manifest that
was installed before this refusal existed and carries a name outside the
list. `hidden` keeps the page out of the
sidebar, and its route still works. A settings pane carries an `id` and a
`title`. [Adding to a printer](#adding-to-a-printer) covers the four keys
that reach a printer's page and its listing card instead.

Every `path` and every `id` is one DNS label: lowercase letters, digits and
inner hyphens. Each has to be different from the others in its own list. Every
`title` has to say something. A key this version does not read is refused, so
a misspelt key is reported rather than ignored.

The manifest names no namespace and no address. The host derives the namespace
from the module path you installed, and builds every address from it.

Put the `module.yaml` at the root of the bundle directory the `asset`
declaration points at. The digest covers it, so the host reads the manifest
that was installed and no other. A bundle with no manifest serves its files
and renders nothing; the core's own app does the same.

The host then publishes the block in `GET /api/v1/resources`, beside the
instance, for an enabled and ready module only:

```json
{
  "kind": "asset",
  "name": "spools",
  "namespace": "com.github.aux.acme.spool-tracker",
  "ui": {
    "entry_url": "/api/v1/modules/com.github.aux.acme.spool-tracker/ui/ui/index.js",
    "library": ">=0.3.0 <1.0.0",
    "contributes": {
      "pages": [{ "path": "spools", "title": "Spools", "icon": "layers", "hidden": false }]
    }
  }
}
```

The app reads that listing, checks `library` against its own version, and
imports `entry_url` when your page first opens or your first section first
renders. A range this app is outside of stops it before the import, and the
module is named on Settings, Extensions with `library-mismatch` as its reason.

The host object carries `fleet`, the printers the person may see. Your bundle
reads it and cannot ask for a read. The app reads the printers when your page
opens, and when the module's page under Settings, Extensions opens. So a page or
a settings tab opened straight from its address sees the printers too. Check
`fleet.loaded` and `fleet.error` before you say that no printer has something.

### Hiding a page while a feature is off

A module with a process can take its own page out of the sidebar while it
runs, and put it back. A planner can hide its Customers page while its
customer records are switched off. Name `ui.hide` in `requires`, and call `Hide` from
`sdk/uiapi` with every entry you hide now:

```go
ui := uiapi.NewClient(conn)

// The customer records are off: hide the page.
err := ui.Hide(ctx, "pages/customers")

// They are on again: hide nothing.
err = ui.Hide(ctx)
```

An entry is written as an operator writes one in `hidden_contributions`: a
slot key, a slash, and the page's `path` or the entry's `id`. So a module can
hide a file action or a print dialog section the same way, and never a
dashboard card. Each call carries the whole list, at most 64 entries.

The host keeps the list in memory and forgets it when your process starts
again. So call `Hide` after each start, once you can read what is switched
off, and again each time it changes. The listing carries the list as
`ui.hidden`, and the server publishes `com.prusa.connect.resource.ui.changed`,
so an open app takes the page out of its sidebar at once. The page's address
still opens it, so the page itself says that the feature is off. You cannot
show an entry the operator hid.

## Adding to a printer

A module with a browser part can also add to a printer's own page and its
card in the listing:

```yaml
ui:
  entry: ui/index.js
  library: ">=0.1.0 <1.0.0"
  contributes:
    device_card:
      - id: nozzle
        title: Nozzle
    device_section:
      - id: nozzle-detail
        title: Nozzle
    device_tab:
      - id: nozzle
        title: Nozzle
    device_action:
      - id: mark-changed
        title: Mark nozzle changed
        icon: wrench
        destructive: false
```

| key | what it does |
| --- | --- |
| `contributes.device_card` | one line on the printer's card in the listing's `cards` view, cut to fit |
| `contributes.device_section` | a section at the foot of the printer's Status tab, after the core's own |
| `contributes.device_tab` | a tab of its own after the core's tabs, at `/printers/<id>/<namespace>.<id>` |
| `contributes.device_action` | an item in the kebab menu at the top of the printer's page |

Every one of the four hands its component, or its `run` function, the
printer's id as `deviceId`. A `device-card`, `device-section` and
`device-tab` entry carry an `id` and a `title`, like a settings pane. A
`device-action` entry also carries an `icon` and `destructive`. The bundle
registers `run(deviceId)` for it instead of a component. The host draws the
title and icon from the manifest before the bundle loads, disables the item
while `run` is pending, and shows a rejection as a toast. It asks no
confirmation of its own.

See the [module slots reference](./reference/module-slots.md) for every
slot's area, form and a YAML example of its own.

### Limiting an entry to the printer the server runs on

A server can run on a printer. The driver of such a printer marks it
resident, and its device record then has `resident: true`. Any other printer
the same server reaches, such as one that connects to it over the network,
has no such field.

An entry of any of the four printer slots may carry a `devices` block, which
names the printers the entry is drawn for. Today it reads one condition,
`resident: true`, and the entry is then drawn only for a resident printer. On
any other printer there is no line, section, tab or menu item, and a link to
the tab opens the Now tab. Until the page has read the printer's record, no
such entry is drawn. An entry with no `devices`, or with the key and no value,
is drawn for every printer.

```yaml
    device_tab:
      - id: display
        title: Display
        devices:
          resident: true
```

Use it for a part that only makes sense on the machine the server runs on,
such as a tab that shows the printer's own local web interface. The
component still gets only `deviceId`.

The server refuses the manifest, with an error that names the entry, when:

- a slot that is handed no printer, such as `file_action` or `settings`,
  carries `devices`;
- the block names no condition, as `devices: {}` does;
- it sets `resident: false`. Leave `devices` out to draw the entry for every
  printer;
- it carries any key other than `resident`.

A core from before `devices` does not read the key, so it refuses the
manifest. Name the first release that reads it in `require.core`, as
[saying what the module needs](#saying-what-the-module-needs) shows.

No printer of the demo is resident, so on the demo such an entry shows on no
printer. To see it while you develop, remove `devices`, and add it again
before you publish the module.

## Adding to a file in storage

A module can add an item to the menu of each file on the Storage page, and a
section to the print dialog of such a file:

```yaml
ui:
  entry: ui/index.js
  library: ">=0.1.0 <1.0.0"
  contributes:
    file_action:
      - id: print-later
        title: Print later
        icon: calendar-clock
        destructive: false
    print_section:
      - id: print-later
        title: Print later
```

Both are handed a file reference, `fileRef`. It is the storage's name, a
colon, and the file's path in that storage, such as
`local:models/bracket.bgcode`. A storage name never holds a colon.
`parseFileRef` from `@prusa/connect-ui/slots` splits a reference into
`storage` and `path`, and `fileRefOf` joins the two.

```ts
host.register.fileAction('print-later', async (fileRef) => {
  const file = parseFileRef(fileRef);
  // ...
});
host.register.printSection('print-later', PrintLater);
```

The `file-action` item is in the menu of each file's row and in the menu at
the top of the file's own page. A folder has no such item. The host runs it as
it runs a `device-action`: the item is disabled while `run` is pending, and a
rejection is shown as a toast.

The `print-section` component is drawn below the list of printers, and only
for a file on a storage of the server. A file that is already on a printer has
no reference. The component reads the dialog through `printDialogContext()`,
called while it initialises. `selected` is the ids of the printers ticked in
the dialog, and `close()` closes the dialog. Outside a print dialog nothing is
ticked and `close()` does nothing.

### Limiting an entry to some file types

An entry of `file_action`, `print_section` and `file_view` may name `file_types`. The entry is then drawn only for a file of one of those types. A type is a lowercase extension with its dot, such as `.3mf`. An entry with no `file_types` is drawn for every file.

```yaml
    file_action:
      - id: slice
        title: Slice
        file_types: [.3mf, .stl]
```

### Drawing what a file holds

`file_view` draws on a file's own page, in place of the part that says what the file holds. The page keeps its header, its actions, the print button, the file's facts and the prints that used it. The component is handed `fileRef`, as the other file slots are.

```yaml
    file_view:
      - id: project
        title: Project
        file_types: [.3mf]
```

```ts
host.register.fileView('project', ProjectView);
```

While the bundle loads, the page holds the place with a placeholder. If the bundle fails, or loads and registers no view, the page draws its own part with one line saying the module's view is not available.

### Reading a file for the core

A module can describe files of its types for the core. That is the picture in a listing, the details on the file's page, and what a printer needs to print the file. The manifest names the types, and the module answers `file.analyze`:

```yaml
file_analysis:
  types: [.demo]
```

```go
methods := wirerpc.Methods{
    // ...
    fileapi.MethodAnalyze: fileapi.Serve(storageapi.NewClient(conn), analyze),
}

func analyze(ctx context.Context, params fileapi.Params, file *storageapi.File) (fileapi.Analysis, error) {
    // file reads the one file params names, through a read handle.
    return fileapi.Analysis{
        Title:   "Storage box",
        Print:   &storageapi.PrintInfo{PrinterModel: "MK4S", NozzleMM: 0.4},
        Facts:   []fileapi.Fact{{Key: "plates", Label: "Plates", Value: "2"}},
        Objects: []fileapi.Object{{ID: "1", Name: "Lid"}},
    }, nil
}
```

The call carries a read handle for the one file. Through it the module reads that file without `read_storage`, and reads no other file. The handle ends when the call ends. `examples/service-plugin/analysis.go` reads `.demo` files this way.

The core keeps its own reader. When the module is not serving, fails, answers over a bound of `sdk/fileapi` or takes too long, the core reads the file itself and a person sees no error. A listing waits two seconds for the picture, and a file's page waits twenty. A late answer is kept for the next time the file is shown. Answers are kept per file version and per release of the module, so an update of the module asks again.

The stat route of the file carries `analysis`, with the module that answered, the title, the facts and the objects. A `file_view` reads it from there.

## Sharing a component with other modules

A module's bundle can hand a component to other modules' bundles. A 3MF module could hand its project view to a planning module, which draws it inside its own page. The provider names the component under `ui.exports` and registers it:

```yaml
ui:
  exports:
    - name: spool-picker
```

```ts
host.register.component('spool-picker', SpoolPicker);
```

The module that draws it names it under `ui.imports`, with the namespace of the module that exports it. It also names that module under `require` or `suggest`, which says which of its releases it works with. A manifest that imports from a module it does not name there is refused.

```yaml
suggest:
  com.github.aux.acme.spools: "Draws the spool picker on the tab"

ui:
  imports:
    - module: com.github.aux.acme.spools
      name: spool-picker
```

The component is loaded at run time, in the same Svelte runtime and with the same library, so it looks like the rest of the page. `ModuleComponent` draws a placeholder while it loads and a fallback when it is not available:

```svelte
<ModuleComponent
  load={() => host.components.load<PickerProps>('com.github.aux.acme.spools', 'spool-picker')}
  props={{ items, onselect: (spool) => (chosen = spool) }}
>
  {#snippet fallback(why)}
    <p>{host.t('planner.noPicker')}</p>
  {/snippet}
</ModuleComponent>
```

`load` rejects with `ComponentUnavailable`. Its `reason` is `undeclared`, `absent`, `not-exported`, `unregistered`, `failed` or `setup`. Call `load` from a component, never from the setup function: the other module's bundle may load after yours. The props are part of what the exporting module publishes, and change with its version as its routes do. Publish their type in the module's client package.

## Adding a card to a dashboard

A person lays out a dashboard from cards, and the home page draws one of
them. A module can add a card of its own to the picker. The manifest declares
it under `cards`, with its size and the fields of its settings:

```yaml
ui:
  entry: ui/index.js
  library: ">=0.1.0 <1.0.0"
  contributes:
    cards:
      - id: spool
        title: Spool
        description: The spool on one printer, and how much is left.
        icon: layers
        size: {w: 4, h: 3}
        least: {w: 2, h: 2}
        settings:
          - key: device
            kind: printer
            label: Printer
          - key: warn
            kind: number
            label: Warn below
            min: 0
            max: 1000
            step: 10
            default: 100
```

| key | what it holds |
| --- | --- |
| `id` | the card's name in the module, one DNS label. The card is placed by its type, `<namespace>/<id>` |
| `title` | what the picker shows, and the card's heading when a person writes none |
| `description` | one sentence the picker shows under the title, at most 200 bytes |
| `icon` | an icon from the list the slot reference gives. It is optional |
| `size` | the size a new card starts at, in cells: `w` from 1 to 12 columns, `h` from 1 to 32 rows |
| `least` | the smallest a person may make the card. It is never larger than `size` |
| `settings` | at most 16 fields. The core draws the settings form from them |

Every field has a `key`, a `kind` and a `label`. The key is a lower-case
letter followed by at most 31 lower-case letters, digits and underscores.
The kinds are a closed set, so the form has a control for each of them:

| kind | what a person picks | the value | what the field may add |
| --- | --- | --- | --- |
| `printer` | one printer | its id, or `""` | `default` |
| `printers` | any number of printers | a list of ids | `default` |
| `tag` | one tag | its name, or `""` | `default` |
| `camera` | one camera | its id, or `""` | `default` |
| `choice` | one of the options | the option's `value` | `options`, `default` |
| `choices` | any of the options | a list of `value`s | `options`, `default` |
| `number` | a number | a number between the bounds, on a step | `min`, `max`, `step`, `default` |
| `text` | one line of text | a string | `max_bytes` (200 unless given, 2000 at most), `default` |
| `note` | several lines of text | a string | `max_bytes` (2000 unless given, 3000 at most), `default` |
| `switch` | on or off | `true` or `false` | `default` |

A field with no `default` starts empty: `""`, an empty list, `false`, the
first option of a `choice`, or the lower bound of a `number`. A field that
names a key its kind does not take, such as `options` on a `switch`, is
refused at install.

The bundle registers the component for each declared card:

```ts
import { defineModule } from '@prusa/connect-ui/slots';

import Spool from './spool.svelte';

export default defineModule((host) => {
  host.register.card('spool', Spool);
});
```

The component receives `CardProps` from `@prusa/connect-ui/slots`, and
nothing else:

| prop | what it is |
| --- | --- |
| `config` | the card's settings, one value for every field |
| `title` | the heading a person wrote over the card, or `""` |
| `size` | the card's size on the grid, in cells |
| `editing` | `true` while the person changes the dashboard |

Every value in `config` was checked against its field. A value that does not
fit is replaced by the field's default before the card sees it.

Draw `title` when it is there. When it is empty, draw a heading of your own.
It may come from the settings, such as "Temperatures of MK4 by the door" or
"9 printers".

Read `size` to leave things out of a small card. While `editing` is `true`,
draw no link, because a click there moves the card.

A card often links to the module's own page. A module does not know the
namespace it was installed under, so ask the host for the address:
`host.pageHref('parts')` answers `/m/<namespace>/parts`, and a query such as
`?part=12` can follow it.

A few rules keep a wall of cards cheap for the printers and safe for the
page:

- A card opens no stream and polls nothing of its own. It reads the fleet
  from the host it was registered with, which the app keeps current from the
  one event stream it already holds.
- A card fills its cell. It draws no frame and sets no outer size; the
  dashboard draws the frame.
- A card that throws is caught. The dashboard draws a short note in its cell,
  and every other card keeps working.
- A dashboard imports only the bundles of the cards it shows. Until a bundle
  has registered its card, or when its module is no longer installed, the
  dashboard draws a placeholder in the card's cell.

When a schema cannot say what a card needs, the bundle can pass an editor of
its own with `host.register.card('spool', Spool, { editor: SpoolEditor })`.
The editor receives `config` and `onchange`, and the dashboard checks what it
hands back against the schema before it keeps it. The schema is still
required: it sets the defaults, and it is what every value is checked
against.

## See the components in Storybook

Your page is built from `@prusa/connect-ui`. The library ships a
Storybook of every component in it. Each story shows the variants, the sizes,
the disabled and loading states and the empty and error states, and the whole
screens are drawn on fake data. It is the fastest way to find the component
you need and read the props it takes.

```bash
cd packages/svelte
pnpm install
pnpm run storybook        # opens http://localhost:6006
pnpm run build-storybook  # writes storybook-static/, which make client-ui-check builds
```

A story file sits beside the component it draws, as `<name>.stories.svelte`.
Write one for a component your own module adds and you get the same thing for
your tree.

## What the extensions page prints about your module

Four manifest keys reach the card a person reads before they switch your
module on:

| key | what it is |
|---|---|
| `version` | the release, `MAJOR.MINOR.PATCH` |
| `title` | the name a person reads, such as `Spool tracker`; the module name stands in without it |
| `summary` | one sentence saying what the module does |
| `codeowner` | who answers for it |

The install copies them into the declaration while the release is still
readable. The release directory is staged and thrown away, and a module that
runs a process keeps no manifest on disk. `GET /api/v1/resources` then carries
them under `module`, beside `ui`, for every installed module whether it is
switched on or not:

```json
{
  "kind": "asset",
  "name": "spools",
  "module": {
    "version": "1.4.0",
    "title": "Spool tracker",
    "summary": "Counts what each print used and warns before a spool runs out.",
    "author": "acme"
  }
}
```

`title`, `summary` and `codeowner` are optional. A module that carries none is still
installable, and its card prints what there is. The release is under `module`
rather than beside `version` on the instance, because `version` there is the
fencing token a write quotes in `If-Match`.

## Translating a module's texts

The frontend is read in seven languages, and more come from the community. No
English text is written into a page. Every text is a key, and the words come
from a catalogue at the moment the page draws.

Your bundle ships its own catalogues. Put them in `src/locales/`, one file per
language, with `en.json` as the source:

```json
{
  "spools.title": "Spools",
  "spools.empty": "No spool is on the shelf yet.",
  "spools.weight": "{grams} g",
  "spools.count": "{count, plural, one {# spool} other {# spools}}"
}
```

The host reads `locales/en.json` beside your entry file, and the file for the
language on screen. Copy the folder into the bundle in your `vite.config.ts`,
next to the manifest:

```ts
cpSync('src/locales', `${BUNDLE_ROOT}ui/locales`, { recursive: true });
```

The host object carries `t`, bound to your namespace. Hold the host the way
you hold the rest of it, and read `host.t` in your page:

```svelte
<script lang="ts">
  import { formatBytes } from '@prusa/connect-ui/i18n';

  const t = heldHost().t;
</script>

<h1>{t('spools.title')}</h1>
<p>{t('spools.count', { count: rows.length })}</p>
<span>{formatBytes(file.size)}</span>
```

Four rules to follow:

- a key says where the text is used, not what it says. A reworded message keeps
  its key and keeps every translation with it
- parameters are named. The order of the parts of a sentence is the language's
  own, so never glue one together from pieces
- numbers, dates, sizes and plurals go through `Intl`, which the library wraps
  as `formatNumber`, `formatDate`, `formatBytes`, `formatDuration` and the
  `{count, plural, ...}` form above. Czech has four plural forms and Polish
  three, so a test against one is wrong in both
- a fault is drawn by its code. `describeFault(thrown)` answers a title from
  `error.<code>`, the server's English message as the detail, and the code

A key your catalogue does not hold falls back to the core's catalogue, then to
English. A module with no catalogue at all still loads, and draws whatever its
code carries.

The gate is on in the module's own `eslint.config.js`, so a literal cannot
land. It is one of the rules `@prusa/frontend-standards` holds for a module,
beside the ban on `fetch` and on Tailwind's arbitrary values:

```js
import { moduleConfig } from '@prusa/frontend-standards/eslint/module';

import svelteConfig from './svelte.config.js';

export default moduleConfig({
  tsconfigRootDir: import.meta.dirname,
  svelteConfig,
});
```

The Prettier configuration and the base `tsconfig.json` come from the same
package, so `pnpm check` in `ui/` holds a new module to the rules the core's
own screens are held to.

`.agents/rules/frontend-i18n.md` carries the whole rule.

## Building the bundle

The bundle is one JavaScript file. The library ships the Vite config that
produces it, so a module's `vite.config.ts` is three lines:

```ts
import { moduleBuild } from '@prusa/connect-ui/build';
import { defineConfig } from 'vite';

export default defineConfig(moduleBuild({ entry: 'src/index.ts' }));
```

`moduleBuild()` takes three keys. `entry` is the module's own entry file.
`fileName` is what the output is called, and it defaults to `index.js`.
`outDir` is where it is written, and it defaults to `dist`.

`moduleDev()`, the dev-server counterpart `pnpm dev` runs, takes five keys,
all optional. `entry` (default `src/index.ts`) must name the same file passed
to `moduleBuild`, because it is what the dev server redirects the installed
entry to. `locales` (default `src/locales`) is where it reads catalogues
beside that entry. `manifest` (default `../module.yaml`) is the module's
manifest, relative to the Vite root. `namespace` picks one install when the
module's name is installed under two namespaces. `core` is the origin the dev
server proxies everything else to, and defaults to `PCL_CORE_URL` if set, then
`http://localhost:8080`.

The entry default-exports a setup function. `defineModule` types the host
object at the call site and returns the function unchanged:

```ts
import { defineModule } from '@prusa/connect-ui/slots';

import SpoolsPage from './spools-page.svelte';

export default defineModule((host) => {
  host.register.page('spools', SpoolsPage);
});
```

The host belongs to the person who is signed in. When that person signs out,
or another person signs in in the same tab, the app aborts `host.signal`. It
calls your setup again with a new host for the next person.

When the signal fires, every request of `host.client` ends. A request in
flight is aborted, and a request made later fails before it is sent. The
store stops too, and sends nothing more. Your own work is yours to stop:

- stop every timer and stream you started, when the signal fires
- pass `host.signal` to a call that takes a signal
- let go of every object you built from the host
- hold nothing when the signal has already fired. A setup that awaits can
  resume after the person left

```ts
let held: ModuleHost | undefined;

export default defineModule((host) => {
  const timer = setInterval(refresh, 5000);
  host.signal.addEventListener('abort', () => {
    clearInterval(timer);
    held = undefined;
  });
  if (!host.signal.aborted) held = host;
  host.register.page('spools', SpoolsPage);
});
```

`host.store` keeps its records on the server for each person.

The manifest's `entry` names the built file, relative to the bundle root. With
the defaults above, and `dist/` as the directory the `asset` declaration
points at, the manifest reads:

```yaml
ui:
  entry: index.js
  library: ">=0.3.0 <1.0.0"
  contributes:
    pages:
      - path: spools
        title: Spools
```

The bundle shares the app's Svelte, its component library and its client SDK
through an import map in the app's page, so it carries none of them. That is
why the build leaves those imports bare, and why the file is under a kilobyte.
Two copies of Svelte on one page do not run slowly, they fail.

The build refuses two mistakes rather than leaving them for the browser. A
bundle that carries Svelte's own code fails with the files that brought it in.
A bundle with a bare import the app's map does not answer fails with the
specifier, because that import would stop the module on its first line.

The build also runs Tailwind over the module's own tree, so the bundle carries
a rule for every utility class the module's markup names. The stylesheet that
turns it on, what the theme already provides and what it costs are in [styling
a module](./styling-a-module.md).

## What is not built yet

| gap | what it means today |
| --- | --- |
| the sdk module is unpublished | build against a checkout, which the `replace` in `go.mod` does |
| nothing signs an artefact | the checksum is integrity, not provenance |

## What to read next

| document | for |
| --- | --- |
| [adapters, modules and runtimes](./extension-concepts.md) | whether a module is the right shape at all |
| [styling a module](./styling-a-module.md) | the theme, the utilities a bundle carries, and what they cost |
| [the plugin wire protocol](./plugin-wire-protocol.md) | the wire itself, for an author outside Go |
| `examples/service-plugin/main.go` | the fuller example: bursts, panics, versioned writes, the not-ready window |
