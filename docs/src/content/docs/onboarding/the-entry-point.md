---
title: From main to a serving process
description: The startup order of the core, function by function, from flag parsing to bound listeners, and the shutdown order in reverse.
sidebar:
  order: 2
audience: build
rewrite:
  describes: current
  implementedBy:
    - cmd/prusa-connect
    - compose
    - internal/app
---

Follow one process from the shell to a bound socket. Every step names the
function that does it, so a stack trace or a startup log line lands somewhere
on this page.

## The subcommand

`main` (`cmd/prusa-connect/main.go:16`) hands every argument to compose.
The word `core` is accepted rather than required, because the image, the
manifests and the end-to-end suite all pass it, and no argument at all runs the
same server.

```
prusa-connect core --config config/config.yaml
```

Everything after the subcommand is `compose.Main` (`compose/compose.go:127`).
It is not in this file for one reason. A build-composed binary is
its own module and cannot reach `internal/app`, so the core publishes one
function and this command is its first caller. A composed binary's `main.go` is
that call and nothing else.

`runCore` (`compose/compose.go:218`) does only three things, because
everything else belongs in `internal/app`:

1. `parseFlags` (`compose/compose.go:421`) reads the flags. Five of them
   override one configuration key each and defer to the file when empty.
   `-version` prints the version and stops
2. `signal.NotifyContext` installs the handler for interrupt and `SIGTERM`. A
   cancelled [context](../glossary.md#context) is the only way a shutdown starts
3. `app.Run(ctx, options)`

The version the server reports is `install.CoreVersion`
(`internal/app/config/install/coreversion.go:27`). A release build stamps it
into `internal/kernel/version` with the linker, and `scripts/build-version.sh`
prints the value. A build with no stamp reads it out of the module graph the
linker embedded. A stamp on the main package would name whoever composed the
binary rather than the core inside it.

Exit codes are separated so a supervisor can tell a typo from an outage: `2`
for a usage error, `1` for a failure to run (`compose/compose.go:60`). Exit
code `3` asks the supervisor to start the process again.

When the Windows service control manager starts the binary, `compose.Main`
runs the same `runCore` under it, and a stop of the service cancels the
context. A restart then ends the process with exit code `3`, and the recovery
action of the service starts it again
([ADR-20261125-jqp](../../../../decisions/20261125-jqp-prusa-connect-runs-as-a-windows-service-that-an-installer.md)).

## The flags

| flag | overrides | default |
|---|---|---|
| `--config` | the file every block is read from | `config/config.yaml` |
| `--overlay` | `core.overlay` | `config.d/resources.json` beside the config file |
| `--addr` | `core.api.addr` | `:8080` |
| `--printer-addr` | `core.connectors.<name>.config.listener.addr` | empty, so printers dial the API listener |
| `--log-level` | `core.log.level` | `info` |
| `--log-format` | `core.log.format` | `text` |
| `--safe` | nothing. This one run starts with every module off | off |
| `--version` | nothing. It prints the version and stops | off |
| `--json` | nothing. It makes `--version` write machine-readable output | off |
| `--workdir` | nothing. The folder the server runs in, from which relative paths resolve | the current folder |

`Options.applyTo` (`internal/app/options.go:119`) lays them over the loaded
configuration, and validation runs afterwards, so a flag can no more start a
broken process than a file key can.

`--printer-addr` names no instance, so it sets the address of the one connector
that is mounted. A file declaring more than one connector refuses it.

## The startup order

`app.Run` (`internal/app/app.go:84`) is the outer frame: config, logger, then
`serve`. `serve` (`internal/app/app.go:184`) opens the graph and the hub and
starts the plane, then hands off to `serveOver` (`internal/app/app.go:269`),
which builds everything that reads the configured inventory. The order is not
arbitrary; each step depends on the one above it.

```
  app.Run                                    internal/app/app.go:84
    |
    1. config.Load(opts.ConfigPath)          internal/app/app.go:85
       opts.applyTo, core.Validate           internal/app/app.go:90, :92
    |     a rejected config binds no port, writes no file, starts no goroutine
    |
    2. newLogger(core.Log)                   internal/app/app.go:113
    |                                        internal/app/logging.go:23
    |     every line after this point is the operator's chosen level and format
    |
    3. openGraph                             internal/app/app.go:142, :530
    |     openDatabase, Open, Migrate, Reconcile, bootstrapGraph
    |     a first run creates the file empty; nothing is ever fetched
    |
    4. stream.New                            internal/app/app.go:155
    |     the hub, before anything that publishes to it
    |
    5. startPlane                            internal/app/app.go:175
    |                                        internal/app/plane.go:97
    |     builtin.Registry, resourcehost.New, host.Run on a goroutine,
    |     sinkFeed.start
    |
    6. resource.New                          internal/app/app.go:202
    |     the operator's file and the machine-owned overlay, merged
    |
    7. newDevices                            internal/app/app.go:219
    |                                        internal/app/devices.go:288
    |     the device plane, before any connector is mounted onto it
    |
    8. mountConnectors                       internal/app/app.go:286
    |                                        internal/app/connectors.go:146
    |     every registered inbound driver is built and its routes collected
    |
    9. store.Reload                          internal/app/app.go:330
    |     the startup reconcile. Instances are registered in the graph's
    |     directory as part of it
    |
   10. buildIdentity                         internal/app/app.go:347
    |                                        internal/app/auth.go:59
    |     reaches the issuer. An unreachable configured issuer refuses the start
    |
   11. httpapi.New                           internal/app/app.go:422
    |                        internal/transport/http/httpapi/server.go:382
    |     the sealed router, every operation, the OpenAPI document, and the
    |     refusal to return a server whose routes and document disagree
    |
   12. listen                                internal/app/app.go:514
                                             internal/app/listen.go:35
          bind, then serve, then drain when the context ends
```

### Why the graph comes before the hub

The hub asks the graph who may subscribe, and the resource store asks it who
may see an instance. `openGraph` also runs `graph.Reconcile`, which writes the
relation vocabulary into the schema; the tuple table's foreign key points at
that, so no tuple is storable until it has run.

`bootstrapGraph` (`internal/app/app.go:980`) writes the first grant into a
graph that holds none: `core.principal` becomes admin of `org:local`. It is a
write and not a rule about emptiness, so there is no later state in which it is
consulted again. On a graph that already has tuples it checks whether the
configured principal still holds anything and warns when it does not, which is
the difference between a server that looks broken and one that says which key
to put back.

### Why the plane runs before the store loads

`resourcehost.Reconcile` hands its plan to the
[goroutine](../glossary.md#goroutine) `Host.Run` owns and waits for the verdict.
A `Reload` against a [supervision plane](../glossary.md#supervision-plane) that
is not running would block, so `startPlane` puts `host.Run` on a goroutine
(`internal/app/plane.go:112`) before anything can reconcile against it. The
plane's context is detached from the caller's (`internal/app/plane.go:59`),
because every supervised instance is started under it and it has to outlive the
listeners.

## Connectors are mounted, not supervised

`mountConnectors` (`internal/app/connectors.go:147`) builds every
[inbound](../glossary.md#inbound-and-outbound) driver this binary carries, once,
at startup. A [connector](../glossary.md#connector) is not a supervised
[resource instance](../glossary.md#resource-instance): its routes go on the
sealed router, which is built once, and a handler swapped underneath a mounted
route would be a device wire that changed identity while a printer was on it. A
connector's lifetime is the process's, and changing one is a restart.

For each registered driver it:

1. builds the driver from its registration (`mountOne`,
   `internal/app/connectors.go:319`)
2. asks `connectorapi.Directions` which half it implements, and skips an
   outbound one, which has no routes to mount (`internal/app/connectors.go:351`)
3. builds a [capability](../glossary.md#capability) resolver over that driver's
   own profiles (`resolverFor`, `internal/app/connectors.go:705`)
4. calls `inbound.Handler(arrivals)` (`internal/app/connectors.go:407`), which
   ties the mounted route to the device plane
5. decides where the routes go: `mountsFor` (`internal/app/connectors.go:720`)

Where they go depends on the dialect. A driver that implements
`dialsAbsolutePaths` (`internal/app/connectors.go:42`) is mounted on the paths
it names, verbatim: xBuddy firmware in the field has `/p/register` and `/p/ws`
compiled in and is not updated on this project's schedule
(`internal/connector/xbuddy/xbuddy.go:325`). Any other driver is mounted under
`/connectors/<instance>/` and the prefix is stripped before the driver sees a
request, which is what `connectorapi.Inbound` promises.

The same driver also implements `profiled` (`internal/app/connectors.go:125`),
so its capability data is its own: `internal/connector/xbuddy/xbuddy.go:336`
returns `profile.Builtin()`, the firmware matrix embedded at
`internal/printer/profile/profile.go:38` and compiled into the binary. A driver
that ships no [profiles](../glossary.md#profile-tier) gets an empty registry,
whose [coverage](../glossary.md#coverage-tier) narrows nothing, so its whole
[baseline](../glossary.md#driver-baseline) stands.

## Binding: one listener, or two, or three

`listen` (`internal/app/listen.go:37`) binds everything before it serves
anything, so a port already in use is an error `Run` returns rather than a
goroutine that dies alone. `bind` (`internal/app/listen.go:113`) takes the API
listener first, always. A printer listener is bound only when the connector's
`listener.addr` is set (`bindDevices`, `internal/app/listen.go:179`), and a
metrics listener only when `core.observability.addr` is
(`bindMetrics`, `internal/app/listen.go:224`).

```
                 listener.addr empty                listener.addr set
                 -------------------                -----------------
 API listener    api.Handler()                      api.Handler()
                 the whole origin, including        the whole origin
                 /p/register and /p/ws
 printer         none. Printers dial the API        api.DeviceHandler()
 listener        listener                           only the mounts marked
                                                    Device. Everything else
                                                    answers a problem document
```

Two listeners because two clients cannot be satisfied at once. A printer
verifies against one CA certificate loaded from a USB stick and speaks TLS 1.2
with a single ECDSA cipher suite; a browser verifies against its own trust
store and is held to TLS 1.3. Each side holds its own profile. `apiTLS`
(`internal/app/apitls.go:19`) sets `MinVersion: tls.VersionTLS13`, and the
dialect's `firmwareTLS` (`internal/connector/xbuddy/listener.go:188`) sets TLS
1.2 with the one suite the firmware offers
(`internal/connector/xbuddy/listener.go:27`). The dialect also warns when the
certificate carries an IP address with no matching DNS name, because
[mbedTLS](../glossary.md#mbedtls) 2.x in the firmware compares the configured
hostname against DNS entries only and would fail every IP-configured printer at
once (`internal/connector/xbuddy/listener.go:350`).

The third listener is the Prometheus exposition, and it is there for a
different reason. `MetricsHandler`
(`internal/transport/http/httpapi/server.go:613`) serves `/metrics` and no other
path, to a caller holding no credential. A collector running beside this server
holds no account on it, so it can carry no grant. The address a deployment
publishes is the boundary instead, which is how etcd, CoreDNS and ingress-nginx
are scraped. The API listener publishes no `/metrics` operation while that key
is set, so exactly one socket answers it.

`open` (`internal/app/listen.go:287`) wraps the listener in TLS rather than
telling the server to load a keypair, so the handshake is the only thing the
two paths differ by and the drain is one code path either way. A failure on a
later bind closes the ones already taken (`closeAll`,
`internal/app/listen.go:360`), so a refused start leaves nothing listening.

`DeviceServer` (`internal/transport/http/httpapi/server.go:652`) sets all three
of `ReadHeaderTimeout`, `ReadTimeout` and `WriteTimeout` to the configured
handshake budget, because Go takes the TLS handshake budget from the smallest
positive value among them.

## The shutdown order

Cancelling the context is how a shutdown starts. `listen`
(`internal/app/listen.go:44`) runs its goroutines in an `errgroup`, a standard
helper that cancels a shared context as soon as the first of them fails and
returns that first error. It holds one goroutine per listener, one running the
readiness watcher, and one waiting on the group context to call `drain`.

```
   1. drain                                  internal/app/listen.go:217
      every listener stops inside one shared budget, in parallel. The budget
      is what a shutdown is allowed to take, not what each socket is allowed
      to take. An open event stream is told the listener is draining through
      Server.RegisterOnShutdown -> beginShutdown, because Shutdown waits for
      in-flight requests and a stream is in flight for as long as its reader
      wants it
                          internal/transport/http/httpapi/server.go:617
   |
   2. fleet.Close                            internal/app/app.go:237
                                             internal/app/devices.go:542
      every printer session is closed and every pump goroutine waited for, so
      a printer publishing its way out of service still reaches a subscriber
   |
   3. plane.shutdown                         internal/app/app.go:184
                                             internal/app/plane.go:226
      the history feed is drained first, then the plane's context is cancelled
      and every instance stops with it. The supervisor bounds each Close with
      its own StopTimeout; this bounds the sum of them, because the sum is not
      bounded otherwise. A plane that does not return inside the budget is
      abandoned and reported as ErrPlaneWedged at internal/app/plane.go:27,
      which is cheaper than a process an operator cannot stop
   |
   4. awaitFeeds                             internal/app/plane.go:245
      each sink's subscription is closed here rather than at hub.Close, so a
      sink still draining a window cannot be handed a fact after the instance
      behind it has been closed
   |
   5. hub.Close                              internal/app/app.go:168
      last, so an instance publishing its way out of service still reaches
      whoever was watching the shutdown
   |
   6. closeGraph                             internal/app/app.go:147
      after all of them, because every step above can still ask it a question
```

`serve` joins the serving error and the shutdown error
(`internal/app/app.go:184`) deliberately: a shutdown that abandoned a wedged
resource is worth reporting even when everything above it went cleanly.

## Reading the startup log

A healthy start prints these, in this order. Every one of them names a value
worth checking against what was configured.

| line | from | says |
|---|---|---|
| `starting` | `internal/app/app.go` | version, config path, overlay path |
| `device kinds registered` | `internal/app/devicekindinstall.go:57` | every class of device this build carries |
| `authorization graph ready` | `internal/app/app.go:590` | the engine, the database path and the graph's epoch |
| `resource types registered` | `internal/app/plane.go:110` | every type this build carries, qualified |
| `the fleet was read back from the record store` | `internal/app/devicerecords.go:326` | how many printers survived the last stop |
| `connector mounted` | `internal/app/connectors.go:412` | the connector, its routes, its capability count |
| `resources reconciled` | `internal/app/app.go:334` | how many instances the merged document declares |
| `identity provider ready` | `internal/app/auth.go:96` | issuer, client id, redirect uri, ID token algorithms |
| `event hub ready` | `internal/app/app.go:496` | the site token and the hub epoch |
| `the web app` | `internal/app/app.go:501` | whether the app is served, and its title, language and theme |
| `the documentation site` | `internal/app/app.go:505` | whether the site was built beside the binary |
| `the optional parts` | `internal/app/app.go:510` | whether MQTT, the broker and the metrics are on |
| `serving` | `internal/app/listen.go:86` | the API address and pasteable URLs for `/docs` and `/api/v1/health` |
| `serving devices` | `internal/app/listen.go:135` | the printer address, whether it is TLS, and the routes |
| `readiness` | `internal/app/readiness.go:236` | ready, serving count, declared count |

Two lines are warnings worth acting on.
`no identity provider is configured; every request is served as one subject`
(`internal/app/auth.go:137`) means what it says. `the configured principal
holds no grant on any resource kind` (`internal/app/app.go:754`) means
`core.principal` was changed after the first run. The graph still names the
old one.
