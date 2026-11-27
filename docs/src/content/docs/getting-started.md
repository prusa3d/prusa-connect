---
title: Run Prusa Connect
description: Start the server with a simulated fleet in one command, read one printer over the API, then connect a real one.
audience: run
rewrite:
  describes: current
  implementedBy:
    - docker/server
    - Makefile
    - cmd/prusa-connect/main.go
    - cmd/demo/main.go
    - internal/app/demo.go
    - config/config.yaml
---

:::note[You downloaded a release]
This page is for a developer with a copy of the source code. If you downloaded
a binary from a release, read these pages instead:

- [Install from a release](./install-from-a-release.md): Linux, a Raspberry
  Pi, Windows and macOS, the first run, and adding a printer
- [Container deployment](./container.md#build-the-image-from-a-release): the
  image, built from a release and started with Docker Compose
- [Run it on a NAS](./nas.md): Synology, unRAID and TrueNAS
- [Kubernetes](./kubernetes.md): the manifests for one instance
- [Upgrade, back up and uninstall](./upgrade-back-up-and-uninstall.md)
:::

Prusa Connect is one server on your own network. Printers dial it, and
you drive them over its API. This page starts the server with a fleet of
simulated printers, reads one of them, and then connects a real one.

You need Go 1.26 or Docker, and port `8080` has to be free. No configuration
file and no certificate is needed.

| you want | run |
|---|---|
| the server with a simulated fleet | `make demo` |
| the server alone, from source | `make run` |
| the server alone, in a container | `docker compose -f docker-compose.local.yml up -d prusa-connect` |

Run `make demo` first. The other two give you an empty server that asks you to
set it up, and no printer until one dials in.

## Start the server with simulated printers

`make demo` compiles one binary and runs it. That binary is the server and the
simulated printers in one process. The server half needs Go and nothing else.

It also builds the example modules in `examples/` and the extensions in
`extensions/`, and offers them on the extensions page, so you can install one
without a network. That half needs pnpm. Without pnpm the demo prints one line,
starts anyway, and offers nothing. An extension that does not build prints a
warning and is not offered, and the demo starts anyway.

```bash
make demo
```

It says nothing for about ten seconds while the printers connect. Then it
prints this and stays running. The password and the printer ids are new on
every run:

```
Starting the demo. Ctrl+C stops the printers and the server.

4 simulated printers are connected to the core on 127.0.0.1:8080.

  DEMO-01    6968e07e-2fe8-498a-9ce3-cdebcf415f94  is printing usb/tangle.bgcode
  DEMO-02    6ebb41c8-a207-43e1-bc82-3cde0aef8c5b  is idle
  DEMO-03    5d3fe564-2dc4-4d33-b946-8297b5c138be  is idle
  DEMO-06    02505460-cdb0-45d7-8c03-8c8466b1eccc  is idle

  DEMO-03 goes away for 15s every 1m0s, and comes back.

Each printer has a simulated camera. The live view of each:

  DEMO-01    http://127.0.0.1:8080/api/v1/cameras/cam-demo-01/stream
  DEMO-02    http://127.0.0.1:8080/api/v1/cameras/cam-demo-02/stream
  DEMO-03    http://127.0.0.1:8080/api/v1/cameras/cam-demo-03/stream
  DEMO-06    http://127.0.0.1:8080/api/v1/cameras/cam-demo-06/stream

Sign in as the demo account. Its password is new on every run:

  username  demo
  password  qpyjqby73wcwpnakt7ia
  owns every printer, so raw gcode and the other dangerous verbs are reachable

Open these while this command keeps running:

  http://127.0.0.1:8080/docs               the API reference
  http://127.0.0.1:8080/dev                the fleet, live, once this browser holds a session
  http://127.0.0.1:8080/api/v1/devices     the same fleet as JSON, with a token

A browser gets its session from the console of the /docs page:

  fetch("/api/v1/auth/login", {method: "POST", headers: {"content-type": "application/json"}, body: '{"username":"demo","password":"qpyjqby73wcwpnakt7ia"}'})

Read it from another terminal. Log in, mint a token, and send it:

  curl -s -c cookies.txt -X POST http://127.0.0.1:8080/api/v1/auth/login -H 'content-type: application/json' -d '{"username":"demo","password":"qpyjqby73wcwpnakt7ia"}'
  curl -s -b cookies.txt -X POST http://127.0.0.1:8080/api/v1/auth/credentials -H 'content-type: application/json' -d '{"name":"terminal"}'
  curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/devices

The demo read config/config.yaml and laid its own settings over it.
5 modules are on the extensions page. They install off this machine, with no network.
It writes the result and its database to
  C:\Users\you\AppData\Local\Temp\prusa-connect-demo-3472093854
and removes that directory when it stops.

The server's own log is quiet here. Add -log-level info to read it.

Ctrl+C stops the printers and the server.
```

Leave it in that terminal. The addresses answer only while it runs.

Three of those printers speak the xBuddy dialect Prusa firmware speaks.
`DEMO-06` is a Klipper machine on the reference dialect, so the fleet shows
that this server is not a Prusa-only product.

The demo signs itself in, so it needs no first-run step. A server you start
any other way does, and [the first run](#the-first-run) says what it asks for.

## Read one printer

Every read takes a credential. Get one with the two `curl` lines the demo
printed, keep the `token` field from the second, and put it in `TOKEN`.

`DEMO-01` is printing. Ask for it by the id beside its name, from a second
terminal:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  http://127.0.0.1:8080/api/v1/devices/6968e07e-2fe8-498a-9ce3-cdebcf415f94
```

The answer is one device record: what the machine is, what it accepts, and its
state tree under `state`. The state half of it reads like this:

```json
{"state":{"/axis/x":0,"/axis/y":0,"/axis/z":16.6,"/flow":100,"/job/id":2,
 "/job/path":"/usb/tangle.bgcode","/job/progress":27,"/job/state":"PRINTING",
 "/job/time_printing":83,"/job/time_remaining":217,"/speed":100,
 "/state":"PRINTING","/temperature/bed":60,"/temperature/bed_target":60,
 "/tools/0/active":true,"/tools/0/nozzle_temp":215,
 "/tools/0/nozzle_target_temp":215,
 "/tools/0/material_package_instance/package/material/abbreviation":"PLA"}}
```

Progress climbs while you watch. The id is a UUID the server minted when the
printer joined, and the same printer comes back under the same id.

`DEMO-03` leaves every minute and returns fifteen seconds later, so
`connected` in `/api/v1/devices` goes false and then true again. That is how
you watch the server notice a printer leaving.

`http://127.0.0.1:8080/dev` shows the same fleet in a browser, and `/docs` is
the API reference. Both want the session the login wrote into the cookie jar.

## Prove the journey a person takes

`make journey-check` does the whole thing a person does, in a real browser,
against a server that is really running. It starts the demo with simulated
printers and a storage on disk. It writes a G-code file that carries a
picture. Then it signs in, uploads the file on the storage page, presses Print
on its row, picks a printer and confirms. Last it reads the core back, to
prove the machine took the print.

It prints one numbered line per step, under the heading
`=== The customer journey ===`:

```text
 1. the client SDK builds...
 2. the component library builds...
 3. the application installs...
 4. the application type-checks...
 5. the core and the demo build...
 6. the print file is written with a picture in it...
 7. the demo serves...
 8. the demo prints an account to sign in with...
 9. the demo account signs in...
10. a printer in the fleet is free to take a print...
11. the journey runs in a browser...
```

The browser lane prints its own test output under the last line, and the run
ends with `OK    the journey passed 11 steps` and the file, the printer and
the demo directory it used.

It needs Go, pnpm, Node, curl and a Playwright Chromium. Everything it writes
goes under `bin/journey`.

## Change the fleet with flags

Pass flags through `DEMO_ARGS`:

```bash
make demo DEMO_ARGS="-printers 8 -flap 0"
```

| flag | default | does |
|---|---|---|
| `-printers` | `3` | how many xBuddy printers dial in. The Klipper machine is there as well |
| `-addr` | `127.0.0.1:8080` | the address the core binds and the printers dial |
| `-flap` | `1m` | how often one printer goes away and comes back, `0` to keep the fleet connected |
| `-cameras` | `true` | start a simulated camera for each printer and declare it against that printer |
| `-print-gap` | `0` | the wait between prints, which also makes each print take seconds. `0` is the demo's own pace |
| `-transfer-rate` | `0` | how many bytes a second each printer fetches a file at. `0` is as fast as the server serves it |
| `-listing-delay` | `0` | how long each printer takes to list a folder, answering no other command meanwhile. `0` lists at once |
| `-log-level` | `error` | the server's own log level: `debug`, `info`, `warn` or `error` |
| `-dir` | a temporary directory | where the configuration and the database go, kept when you name it |
| `-config` | `config/config.yaml` | the file the demo lays its own settings over, ignored when there is none |
| `-examples` | `bin/demo-modules` | where the packed example module and extension releases are, which `make demo` writes |
| `-password` | a new one each run | the demo account's password, at least eight bytes |
| `-tls-cert`, `-tls-key` | none | a certificate and key as `core.api.tls` takes them. With both, the demo serves HTTPS |

The xBuddy models cycle through CoreOne, MK4S and MINI, so
`GET /api/v1/devices` shows three different capability sets beside the
Klipper one.

## Start the server alone

`make run` compiles the Go binary and runs the server in the foreground.
Ctrl+C stops it.

```bash
make run
```

It prints the tags it built with, then the log:

```
Building with tags: full
Starting the core. Ctrl+C stops it.
time=... level=INFO msg=starting version=0.1.0+bf828740465a config=config/config.yaml
  overlay=config/config.d/resources.json
time=... level=INFO msg="authorization graph ready" engine=sqlite
  database=config/config.d/authz.db epoch=3
time=... level=INFO msg="connector mounted" connector=xbuddy type=core/xbuddy
  routes="[/p/register /p/auth /p/ws]" capabilities=102
time=... level=WARN msg="this server has recorded nothing about identity; it
  serves the first run and nothing else, and asks for this setup code"
  setup="POST /api/v1/setup" setup_code=K7QD-3M9X-TP2A

  This server is not set up yet. Open it in a browser and enter
  this setup code:

      K7QD-3M9X-TP2A

  The code works once. A restart prints a new one.

time=... level=INFO msg=serving api=127.0.0.1:8080
  reference=http://127.0.0.1:8080/docs openapi=http://127.0.0.1:8080/openapi.json
  health=http://127.0.0.1:8080/api/v1/health
```

The tag list after `full` is empty while the shipped configuration switches on
nothing that needs a tag. Turn on MQTT or the Prometheus metrics in
`config/config.yaml` and the names appear there.

The `WARN` line says the server has recorded nothing about identity.
[The first run](#the-first-run) is what clears it. It asks for the setup code
printed under it.

The container route builds the image itself, so no Go toolchain is involved.
Run this in the directory holding this repository:

```bash
docker compose -f docker-compose.local.yml up -d prusa-connect
```

`docker ps` reports `prusa_connect` as `healthy` once the container's
own probe has passed, and `docker logs prusa_connect` prints the same
log. `PCL_API_PORT` in `.env` moves the published port off `8080`. Stop it
with `docker compose -f docker-compose.local.yml stop prusa-connect`.
[Container Deployment](./container.md) covers the volumes and the probes.

## The first run

A server that has recorded nothing about identity serves the first run and
nothing else. Every other route answers 503 with the code
`core.auth/SETUP_PENDING`, including `/api/v1/devices` and `/dev`.

The first run asks for a setup code. It proves that you can read the server's
log, so another machine on your network cannot set the server up before you
do. The server prints the code when it starts, in the log and in a block on
standard error:

| how the server runs | where to read the code |
|---|---|
| in a terminal | the terminal, as in the log above |
| in Docker | `docker logs prusa_connect` |
| as the systemd service | `journalctl -u prusa-connect` |

The code works once, and a restart prints a new one. On a server that has
waited a long time, the log may have rotated and no longer show the code.
Restart the server to print a new one. A scripted install sets its own code
with `core.auth.setup_code` in the configuration file, and the server then
prints none. No setting turns the code off.

Open the web app at `http://127.0.0.1:8080/` and it asks for the code and the
two questions below, or answer them over the API:

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/setup \
  -H 'content-type: application/json' \
  -d '{"code":"K7QD-3M9X-TP2A","mode":"open","admission":"open"}'
```

A missing or wrong code answers 403 with `core.auth/SETUP_CODE_REFUSED`. After
five wrong codes from one address in fifteen minutes, that address gets 429
with `core.auth/SETUP_CODE_LIMITED` until the fifteen minutes are over. A
request from the machine itself, on loopback, is never refused for wrong
codes. A restart clears the count.

`GET /api/v1/setup` reads the same state back, as a list of steps with one
sentence each.

`mode` says whether a caller has to present a credential. `required` asks for
one, and the same call creates the first administrator from the `account`
field. `open` serves every request as `core.principal`, which is `operator`
unless a configuration file sets it.

`admission` says how a new printer joins. `ask` holds the printer at a code
until somebody approves it. `open` lets a printer in the moment it dials.

The admission answer is read when the server starts, so a choice you record
while it is running takes effect the next time you start it. The mode takes
effect at once.

With `admission: ask`, `prusa-connect registrations list` prints what is
waiting and `prusa-connect registrations approve <code>` lets one in.
[Local accounts](./local-accounts.md) and
[Human authentication](./human-auth.md) cover the identity half.

## Check which server holds the port

`curl http://127.0.0.1:8080/api/v1/health` answers
`{"status":"ok","version":"0.1.0+bf828740465a"}` from a source build. That is
`VERSION` and the commit it was built from. A released binary names the
release, such as `0.1.0`. That route answers before the first run, so it
is the one address that says whether this server is there at all.

The word `core` is accepted and not required: `./bin/server` and
`./bin/server core` run the same server.

The full build serves the web app at `/`; the minimal build embeds no page
and answers 404 there. `/dev` watches printers and `/docs` reads and calls
the API in both.

If port `8080` is taken, the start fails with `bind the api listener on
127.0.0.1:8080` and the operating system's own message. The usual holder is
another copy of this server, or a container publishing that port.
`./bin/server core -addr 127.0.0.1:9090` binds a different one.

## Connect a real printer

Create a file named `prusa_printer_settings.ini` in the root of a USB flash
drive:

```ini
[service::connect]
hostname = 192.168.1.100
port = 8080
tls = false
```

`hostname` is the address of the machine running the server, at most 20
characters unless it ends in `prusa3d.com`, see
[Hostname length](./printer-configuration.md#hostname-length). The file has no
`token`: the printer gets one from the server when it is approved. On the
printer: Settings, Network, Prusa Connect, Load Settings, then Add Printer to
Connect. The printer shows a code.

With `admission: open` the server approves the code at once, the printer is in
the fleet within a few seconds, and `GET /api/v1/devices` lists it. With
`admission: ask` the printer waits. `prusa-connect registrations list`
prints the code and `prusa-connect registrations approve <code>` lets
the printer in. `/dev` shows it live either way.

The **Add a printer** page walks the same steps and shows the printer the
moment it asks. [Printer Configuration](./printer-configuration.md) has the
details.

[Bench preflight](./bench-preflight.md) walks a printer on the bench through
the cable, the address, the firewall and which server holds the port.

For a printer on Prusa's real MK4 firmware with no hardware, `make firmware-up`
boots one in an emulator against a running server. The comment above that
target in `Makefile` lists what it needs.

## Read these next

| page | covers |
|---|---|
| [Install from a release](./install-from-a-release.md) | the released binaries, the first run and a printer, with no source code |
| [Container Deployment](./container.md) | volumes, the healthcheck, Kubernetes, and the unprivileged runtime user |
| [Configuration](./config.md) | every key the core reads, and the build tags a profile implies |
| [Developer view](./developer-view.md) | the panels on `/dev` and where each one's data comes from |
| [Printer Configuration](./printer-configuration.md) | the INI file and the printer menus |
| [Custom Certificates](./certificates.md) | the xBuddy connector's `listener.tls`, a private CA, and what the firmware demands of a certificate |
| [Local accounts](./local-accounts.md) | `core.auth.local`, the first administrator and the login limit |
| [Error Codes](./error-codes.md) | the `code` field in an error reply |
