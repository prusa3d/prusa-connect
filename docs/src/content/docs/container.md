---
title: Container Deployment
description: Running the published image or building it from a release, what it runs, its ports, the configuration file it reads, the writable paths, the two probes, and the security settings compose applies.
audience: run
rewrite:
  describes: current
  implementedBy:
    - docker/server
    - .github/workflows/container-image.yml
    - docker-compose.local.yml
    - scripts/build-version.sh
    - internal/app/probe.go
    - scripts/check-container-nonroot.sh
    - scripts/check-container-probes.sh
    - internal/arch/container_test.go
    - internal/arch/container_environment_test.go
---

This page is for a person who runs Prusa Connect in Docker. Every release publishes an image to
GitHub's container registry, and Docker Compose starts it. You can also build the image yourself
from a release's source code. A hosted deployment runs one instance per tenant, so this page also covers a definition somebody else
generates.

The image is Debian slim holding the server binary and the built web app. It runs as an
unprivileged user with a read-only root filesystem and an empty Linux capability set.

[Install from a release](./install-from-a-release.md) covers running the binary with no Docker at
all, which is the shorter way on a Raspberry Pi.

## Run the published image

The image is `ghcr.io/prusa3d/prusa-connect`, for `linux/amd64` and `linux/arm64`. A Raspberry Pi
with a 64-bit system is `arm64`. A 32-bit system has no image, so run the binary there, as
[Install from a release](./install-from-a-release.md) describes.

Docker Hub holds the same image as `prusaresearch/prusa-connect`, with the same tags and the same
signature. Use either name.

| tag | names |
|---|---|
| `0.2.1` | that release. It never moves |
| `0.2` | the newest `0.2.x` release |
| `0` | the newest `0.x.x` release |
| `latest` | the newest release |

A prerelease, such as `0.2.0-beta.1`, gets only its own tag. Name an exact version, so the server
changes only when you change the version.

Download the image with the version of the release you install:

```bash
docker pull ghcr.io/prusa3d/prusa-connect:0.2.0
```

### Start it with Docker Compose

Docker Compose starts the container with the right volumes and security settings. Make an empty
folder, download the release's `config.yaml` into it, and save the example below beside it as
`compose.yaml`:

```bash
mkdir prusa-connect && cd prusa-connect
curl -fLO https://github.com/prusa3d/prusa-connect/releases/download/v0.2.0/config.yaml
```

```yaml
services:
  prusa-connect:
    # An exact version. Change it here to upgrade.
    image: ghcr.io/prusa3d/prusa-connect:0.2.0
    container_name: prusa_connect
    restart: unless-stopped
    ports:
      # The browser, the API and the printers.
      - "8080:8080"
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      # The database and the settings the server writes.
      - prusa_connect_config:/config/config.d
      # The uploaded print files.
      - prusa_connect_storage:/storage
      - prusa_connect_data:/data
      - prusa_connect_files:/files
      # Large uploads pass through /tmp.
      - prusa_connect_tmp:/tmp
    read_only: true
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "5"

# Fixed names, so a NAS app that renames the project still finds the same data.
volumes:
  prusa_connect_config:
    name: prusa-connect_prusa_connect_config
  prusa_connect_storage:
    name: prusa-connect_prusa_connect_storage
  prusa_connect_data:
    name: prusa-connect_prusa_connect_data
  prusa_connect_files:
    name: prusa-connect_prusa_connect_files
  prusa_connect_tmp:
    name: prusa-connect_prusa_connect_tmp
```

Then start it:

```bash
docker compose up -d
```

Your settings are in the `config.yaml` in this folder, which the container reads but never writes.
The server reads it only when it starts, so run `docker compose restart` after an edit. The
image has its own health check, and the container reports itself healthy within 30 seconds of the
start:

```bash
docker compose ps
curl -s http://localhost:8080/api/v1/health
```

The first page in the browser asks how people sign in and how printers join.
[Set it up in the wizard](./install-from-a-release.md#set-it-up-in-the-wizard) explains each
question.

The container name and the volume names are the ones the build below uses. The commands on the
rest of this page and on
[Upgrade, back up and uninstall](./upgrade-back-up-and-uninstall.md#upgrade-in-docker) work for
both. A server you built before keeps its data when you move to the published image.

### Verify the image signature

Every image is signed with [cosign](https://docs.sigstore.dev/), with no key of its own. The
signature names the workflow file that built the image and the tag or branch it ran from. A release
is published from its tag. A release that was out before images were published, such as
`0.1.0-beta.1`, was published later from `main`. With cosign installed:

```bash
cosign verify ghcr.io/prusa3d/prusa-connect:0.2.0 \
  --certificate-identity-regexp '^https://github\.com/prusa3d/prusa-connect/\.github/workflows/container-image\.yml@refs/(tags/v.+|heads/main)$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The image also carries its build provenance and an SBOM, a software bill of materials: the list of
the packages inside the image. `docker buildx imagetools inspect` reads both.

## Build the image from a release

Build the image yourself to change it, or where the registry cannot be reached.

A release carries its source code as `prusa-connect-<version>-source.tar.gz`, beside the
binaries and listed in `SHA256SUMS`, so you can check it before you build. Unpack it and build the image in the folder it makes. The build
needs Docker with Compose and a network connection, and it takes several minutes. The image cannot
read the version from git, so give it the release's version in `.env`:

```text
$ tar -xzf prusa-connect-0.1.0-source.tar.gz
$ cd prusa-connect-0.1.0
$ echo PCL_BUILD_VERSION=0.1.0 > .env
$ docker compose -f docker-compose.local.yml build prusa-connect 2>&1 | tail -4

#39 [prusa-connect] resolving provenance for metadata file
#39 DONE 0.0s
 prusa-connect  Built
$ docker compose -f docker-compose.local.yml run --rm --no-deps prusa-connect --version
prusa-connect 0.1.0
```

Then start it. Compose publishes port 8080 and mounts `config/config.yaml` from this folder
read-only, so that is the file you edit:

```bash
docker compose -f docker-compose.local.yml up -d prusa-connect
```

`PCL_API_PORT` in `.env` moves the published port off 8080. Open `http://<this machine>:8080/` and
answer the first-run questions, as
[Install from a release](./install-from-a-release.md#set-it-up-in-the-wizard) describes.

A container started from this image reports itself healthy about 30 seconds after the start, and
the health route names the version from `.env`. This container was started with a name and a port
of its own, because another copy of the stack held port 8080 on that machine. A container started
with `up` is named `prusa_connect` and answers on port 8080:

```text
$ docker compose -f docker-compose.local.yml run -d --no-deps --name pcl-beta-test-core -p 18097:8080 prusa-connect
571278b0d98a006b7593219db47891721e184e6d2528ebac593d60278ceb191f
$ docker ps --filter name=pcl-beta-test-core --format '{{.Names}} {{.Status}}'
pcl-beta-test-core Up 40 seconds (healthy)
$ curl -s http://127.0.0.1:18097/api/v1/health
{"status":"ok","version":"0.1.0"}
$ docker logs pcl-beta-test-core 2>&1 | head -1
time=2026-09-29T13:52:12.828Z level=INFO msg=starting version=0.1.0 config=config/config.yaml overlay=config/config.d/resources.json
```

The server writes its database and your print files into named volumes, which stay when you run
`docker compose down` or rebuild the image. `docker compose down -v` deletes them.
[Upgrade, back up and uninstall](./upgrade-back-up-and-uninstall.md#upgrade-in-docker) covers a new
release.

## What the image runs

The entry point is the binary and the command is `core`, so the image runs the core.

| you run | you get |
|---|---|
| `docker compose ... up -d prusa-connect` | the core |
| `docker compose ... run --rm --no-deps prusa-connect --version` | the version, and the container stops |
| `docker exec prusa_connect /prusa_connect core healthcheck` | the probe, against the core running in that container |

The core serves its own web app at `/`, from files built into the binary. An address that names no
file in it answers the same document, so reloading a page inside the app works.

## Image reference

| | |
|---|---|
| Registry | `ghcr.io/prusa3d/prusa-connect` |
| Platforms | `linux/amd64`, `linux/arm64` |
| Base | `debian:trixie-slim`, digest-pinned |
| Entrypoint | `/prusa_connect` |
| Command | `core` |
| Working directory | `/` |
| Runtime user | `prusa`, uid `10001`, gid `10001`, no home, `/usr/sbin/nologin` |
| Config path | `/config/config.yaml`, the default `config/config.yaml` relative to `/` |
| Extensions | read-only folders in `/opt/modules`, one for each extension in the list below |

The uid is fixed at build time rather than assigned by `useradd`, so a host directory mounted over
one of the data paths can be chowned to a known number. Both `APP_UID` and `APP_GID` are build args.

## Bundled extensions

The image carries the first-party extensions under `extensions/` and the Fractal module from
`packages/fractal` as releases, one directory each.
They do nothing until your configuration names them in `core.modules_local`:

```yaml
core:
  modules: /data/modules
  modules_local:
    github.com/prusa3d/parts: /opt/modules/parts
    github.com/prusa3d/fleet-stats: /opt/modules/fleet-stats
    github.com/prusa3d/buddy-debug-logs: /opt/modules/buddy-debug-logs
    github.com/prusa3d/threemf: /opt/modules/threemf
    github.com/prusa3d/server-slicing: /opt/modules/server-slicing
    github.com/prusa3d/materials: /opt/modules/materials
    github.com/prusa3d/buddy3d-camera: /opt/modules/buddy3d-camera
    github.com/prusa3d/easyprint: /opt/modules/easyprint
    github.com/prusa3d/fractal-link: /opt/modules/fractal
    github.com/prusa3d/simple-queue: /opt/modules/simple-queue
```

Settings → Extensions then offers each one. Switching it on installs a copy into `core.modules`,
so that directory has to be writable and kept across restarts. A newer image offers an update for
every extension whose version changed.

An extension that asks for a data folder gets `module-data/<name>` beside `core.modules`, which is
`/data/module-data/<name>` with the configuration above. Removing the extension keeps that folder.
Mount `/data` as a volume, or the folder is lost with the container.

The Fractal module does not start without its settings, and the Extensions page sends none.
Install it with `prusa-connect module add -from /opt/modules/fractal -settings <file>`, as
the module's README in `packages/fractal` says. It also opens the NATS ports its `accept` block names
and its `files` port, `8443` unless you choose another. Publish the ones a server outside the
container needs.

The server-slicing extension carries no PrusaSlicer, and GitHub publishes no Linux build of it. In the
image, name a manifest of builds you host in its `manifest_url` setting, or mount a PrusaSlicer
command line and name it in `local_binaries`. Its README says how.

## Ports

The core binds one listener.

| Port | Listener | Config key |
|---|---|---|
| `8080` | Browser, REST API, SSE, and the printers | `core.api.addr` |

Printers dial that same listener unless a second one is configured. Set
`core.connectors.<name>.config.listener.addr` and there are two sockets. The printer one serves
`/p/register` and `/p/ws` and answers 404 to everything else, so a printer VLAN reaches the
printers' wire and nothing more. Publish that port too when you set it.

A port set in `config.yaml` or in the setup wizard is the container's port. Publish it as well, or
nothing outside the container reaches it. The addresses the server prints when it starts are the
container's own, `localhost` and the bridge address with the container's port. From another machine,
open the host's address with the port you published.

The Prusa printer logs extension, `buddy-debug-logs`, listens on its own UDP ports once you approve
them: `13514` for log lines and `8514` for metrics, or the ports you chose. Publish them with the
protocol, as `-p 13514:13514/udp -p 8514:8514/udp`.

The Buddy3D camera extension, `buddy3d-camera`, listens on TCP `9443` for the cameras' pictures
once you approve it. Publish it, as `-p 9443:9443`. The cameras dial the host's address, so set the extension's
`address` to it: the container's own address is not one the cameras reach.

### Camera streams

A browser plays an RTSP camera over WebRTC, and the video reaches it on UDP.
By default the server takes a random UDP port for each viewer and offers the
browser only the addresses it holds. In a container those are bridge
addresses, so the browser cannot reach them.

The live view then falls back to a websocket on the page's own port, which
needs no setting. It stalls more than WebRTC on a network that drops packets.
For WebRTC, set one port and the address of the host, then publish that port:

```yaml
core:
  webrtc:
    udp_port: 8189
    addresses:
      - 192.168.1.20
```

```bash
-p 8189:8189/udp
```

The published port has to be the same number on the host and in the container,
because the server offers the container's port to the browser. Host networking
also works, with neither key set.

Nearby needs host networking. mDNS packets stay on the container's bridge, so
the printers page lists nearby printers only with `network_mode: host`. The
server then ignores the interfaces Docker makes for other containers.

A printer listener may be put below port 1024; see
[Port 80 without capabilities](#port-80-without-capabilities).

## Configuration

Nothing is baked into the image. The core reads one file and a few flags, and reads no environment
variable at all.

| source | what it sets |
|---|---|
| `/config/config.yaml` | everything. Mount it read-only |
| `-config <path>` | that file, somewhere else |
| `-addr <host:port>` | overrides `core.api.addr` |
| `-overlay <path>` | overrides `core.overlay` |
| `-printer-addr <host:port>` | sets the mounted connector's `listener.addr`. Refused when the file declares more than one connector |
| `-log-level`, `-log-format` | override `core.log.*` |

**A container with no configuration file starts.** The core treats a missing file as a default
configuration. A first run has no file, and a server that refuses to start without one cannot be
started at all. An instance a fleet controller spawns with nothing but an image comes up,
binds `:8080` and serves. `scripts/check-container-probes.sh` runs the image that way on purpose.
Such a server declares no storage, so it has nowhere to keep uploaded files until a configuration
declares one.

### Mounting the configuration file in Kubernetes

Mount the ConfigMap as a directory of its own and read it with `-config`. The bundled manifests put
it at `/etc/prusa-connect` and pass
`-config /etc/prusa-connect/config.yaml`.

Two mount modes are wrong here, for different reasons:

| mount | what goes wrong |
|---|---|
| the ConfigMap over `/config` | it hides `/config/config.d`, which the core writes |
| one key over `/config/config.yaml` with `subPath` | the file never changes again |

Kubernetes states the second plainly: a container using a ConfigMap or a Secret as a `subPath`
volume mount does not receive automated updates. Every other volume mount does, within about the
kubelet sync period.

**No mount mode makes a running instance re-read its configuration.** The core reads the file once
at startup and nothing re-reads it. There is no `SIGHUP` handler and no file watch, so a change to
the ConfigMap needs `kubectl rollout restart deployment/prusa-connect` whichever way it is
mounted. The mount mode decides only what the next start finds on disk.

Set `core.overlay` and `core.authz.database` to absolute paths under a writable volume whenever the
file is mounted anywhere other than `/config`. Both keys derive from the directory holding the
configuration file when they are left unset, and that directory is the read-only mount.

### Credentials arrive as mounted files

A key that needs a credential holds a path to it, never the credential. `core.api.tls.certificate`,
`core.api.tls.key` and `core.authz.database` are all filenames, and the core has no key that holds
a secret value.

That is what makes a hosted tenant work. Mount a Kubernetes Secret as a volume and point the key at
a file inside it:

```yaml
volumes:
  - name: api-tls
    secret:
      secretName: prusa-connect-tls
volumeMounts:
  - name: api-tls
    mountPath: /tls
    readOnly: true
```

With `core.api.tls.certificate: /tls/tls.crt` and `core.api.tls.key: /tls/tls.key`.

| route | survives a rotation with no restart |
|---|---|
| a Secret volume | yes for the printer listener, and the file is updated either way |
| a Secret volume with `subPath` | no. The file never changes |
| `env.valueFrom.secretKeyRef` | no. A variable is fixed at `exec`, and the core reads none |

A mounted file is the only route where the new material reaches the disk at all. Reaching the
running process is a second question, and the answer depends on which listener holds the
certificate. The printer listener is the one a connector's `listener.tls` block configures. It reads
the certificate file again when the modification time moves, so the kubelet replacing a mounted
Secret is enough, and no printer's connection is dropped. It looks at the certificate and the key at
most once a second, and it watches both. A writer that replaces them one at a time is therefore read
again when the second file lands, rather than held at the mismatch the first one made. A replacement the
firmware could not verify is logged and refused, and the listener carries on with the certificate it
already serves.

The API listener, `core.api.tls`, still reads its certificate once when the listener is built, so
rotating that one needs `kubectl rollout restart`. `docker/server/server.yml` and the manifests both
use volumes rather than variables, so the same fix reaches it without moving the mount.

## Volumes and ownership

| Path | Contents | Config key |
|---|---|---|
| `/config/config.d` | the resource overlay the API writes, and the authorization graph | `core.overlay`, `core.authz.database` |
| `/storage` | the `local` storage instance | `resources.storage.local.config.root` |
| `/data`, `/files` | created by the image and mounted by compose. The shipped configuration writes nothing there | |
| `/tmp` | scratch space, because the root filesystem is read-only | |

`/config/config.d` is the one that has to be there. The operator's file is mounted read-only and
the root filesystem is immutable, so without a writable directory the core stops at startup:

```
prusa-connect core: create the directory for the authorization graph:
mkdir config/config.d: read-only file system
```

Give it a volume, or point `core.overlay` and `core.authz.database` at a path under `/data` if one
mutable path per instance suits the deployment better.

`/config/config.d`, `/data`, `/files` and `/storage` are created in the image owned by
`prusa:prusa`, mode `0750`.

The bundled compose file gives each volume a fixed `name:`. A compose manager that renames the
project, as some NAS apps do, still mounts the existing volumes. A compose file of your own must pin
the names the same way, or use bind mounts.

**Named volumes** inherit those permissions. Docker seeds an empty named volume from the image path,
carrying owner and mode across with it, so the bundled compose file needs no entrypoint chown.

**Bind mounts** do not. The host directory keeps its own ownership and must be chowned before first
start:

```bash
sudo chown -R 10001:10001 /srv/prusa-connect/data
```

## Liveness and readiness

Two questions, two paths, and they are not the same question.

| path | answers | a failure means |
|---|---|---|
| `GET /api/v1/health` | this process is serving. It reads no dependency and takes no identity | the process is wedged and wants restarting |
| `GET /api/v1/readiness` | every instance the configuration declares **required** is serving | the process is running correctly and refusing traffic on purpose |

Readiness answers `200` while it can serve and `503` while it cannot, with the same body either way:

```json
{"ready":true,"serving":1,"declared":1,"blocked":[],
 "instances":[{"resource":"storage:local","state":"ready","required":false,"serving":true}]}
```

`blocked` names the required instances that are not serving. An instance that is not required never
makes the process unready, so a failing optional sink cannot take the printer path out of service.
Neither path needs a credential, because the probe that reads it cannot hold one.

In Kubernetes, read both over HTTP and let the two mean different things:

```yaml
livenessProbe:
  httpGet: { path: /api/v1/health, port: 8080 }
  periodSeconds: 10
readinessProbe:
  httpGet: { path: /api/v1/readiness, port: 8080 }
  periodSeconds: 5
startupProbe:
  httpGet: { path: /api/v1/health, port: 8080 }
  failureThreshold: 30
  periodSeconds: 2
```

## Health check

Docker and compose have one health status rather than two, and it reads readiness: a container
reported healthy is one a caller may be sent to. The probe is the binary itself, so the image
carries no `curl` and no `wget`:

```bash
docker exec prusa_connect /prusa_connect core healthcheck; echo $?
```

| | |
|---|---|
| Probe | `GET /api/v1/readiness` on the API listener |
| Exit codes | `0` ready, `1` not ready or unreachable, `2` bad invocation |
| Flags | the core's own, before the word: `core -config <path> healthcheck` |
| Interval / timeout / retries | `30s` / `5s` / `3`, after a `30s` start period |

It reads the same configuration file the server reads, so a moved address or a configured
certificate needs no second place to update. Against a TLS listener it does not verify the
certificate: the peer is this same process over loopback, and the certificate is routinely one the
deployment signed itself.

## Shutdown

Kubernetes sends `SIGTERM` on every rolling update, and the core treats it the way it treats
`SIGINT`. The listeners drain first, so no request reaches a resource being torn down. Then the
supervision plane stops every instance. The event hub closes last, so a reader watching the
shutdown still sees it. `core.shutdown_timeout` bounds the drain and defaults to `15s`.

An idle container stops in about two seconds. Give the pod a `terminationGracePeriodSeconds`
comfortably above `core.shutdown_timeout`, or the orchestrator kills the drain it asked for.

## Rollouts, replicas and the printer websocket

Three costs are built into running this on a cluster, and none of them is a bug to fix later. The
manifests are at [Kubernetes](./kubernetes.md).

| cost | why |
|---|---|
| every rollout ends every printer session | a printer holds one long-lived websocket to one pod |
| one replica, not two | the authorization graph is one SQLite file with one writer |
| a replacement, not a rolling update | two pods cannot hold one `ReadWriteOnce` claim |

A printer reconnects on its own, and how fast it notices depends on the keepalive settings under its
connector. Until it does, that printer is unreachable and its commands are refused.

The replica count moves when the graph runs on a shared engine rather than a file. Nothing else in
the process holds state that two pods would fight over.

## Port 80 without capabilities

A printer-facing listener can be put on port 80, below the traditional privileged boundary, as uid
10001 and with the capability set empty.

This works because container runtimes set `net.ipv4.ip_unprivileged_port_start=0`, which removes the
privileged range inside the container. Docker and containerd default to it. Check a running
container with:

```bash
docker exec prusa_connect cat /proc/sys/net/ipv4/ip_unprivileged_port_start
```

`0` means any port is bindable by any user. A runtime that reports `1024` keeps the traditional
boundary, and the bind fails with `listen tcp 0.0.0.0:80: bind: permission denied`. Two fixes, both
runtime-side:

- grant `NET_BIND_SERVICE` to the container, or
- put the connector's `listener.addr` above 1024 and publish `80:<port>`

Kubernetes equivalent of the compose settings:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 10001
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: [ALL]
```

:::caution
Do not solve a `permission denied` bind by putting `cap_net_bind_service` on the binary as a file
capability. The kernel refuses `execve` when a binary's file capabilities fall outside the bounding
set. The image then fails to start under `cap_drop: [ALL]` with `exec /prusa_connect:
operation not permitted`, an error naming neither capabilities nor the port. Grant the capability
to the container instead.
:::

## The time server

The time server answers on UDP port 123. Inside the container it opens the
port for the reason a printer listener opens port 80, and the state then says
`serving`. A device on your network reaches it only when the port is
published, so add `"123:123/udp"` under `ports` in compose, or a `UDP` port
123 to the Kubernetes Service.

## Compose layout

| path | purpose |
|---|---|
| `docker-compose.yml` | production-shaped composition; includes the service files, publishes no host ports |
| `docker-compose.local.yml` | the composition this page starts; adds host ports and smaller resource limits |
| `docker/server/server.yml` | the server service definition |
| `docker/server/server.local.yml` | host ports and local resource overrides |
| `docker/server/Dockerfile` | the image build; its context is the repository root |

`PCL_API_PORT` in `.env` moves the published port, so two people can run the stack on different
ports without editing tracked files. `PCL_BUILD_VERSION` is the version the image's binary
reports. See `.env.example`.

## Security settings applied by compose

| Setting | Value | Effect |
|---|---|---|
| `security_opt` | `no-new-privileges:true` | Blocks privilege gain through setuid binaries |
| `cap_drop` | `ALL` | Default-deny on capabilities |
| `read_only` | `true` | Immutable root filesystem; writes go to the volumes above |
| `logging` | `json-file`, `max-size: 10m`, `max-file: 5` | Caps container log disk use at 50 MiB |
| Config mount | `:ro` | The server only reads `config.yaml` |

## Resource limits

The bundled compose file sets 2 CPU / 2 GiB limits and 0.5 CPU / 256 MiB reservations. These are
starting values, not measurements. Raise the ceiling on a host serving many printers, and lower both
on a single-board host once a real workload has been measured.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `no matching manifest for linux/arm/v7` | a 32-bit system. Run the binary instead |
| `mkdir config/config.d: read-only file system` | no writable volume at `/config/config.d` |
| `/api/v1/health` reports version `0.0.0-dev` | the image was built with no `PCL_BUILD_VERSION` in `.env` |
| `lookup … on 127.0.0.11:53` in the marketplace or sign-in message | the container has no working DNS. Test with `docker exec prusa_connect getent hosts plugin-marketplace.prusa3d.com` |
| `x509: certificate signed by unknown authority` in the marketplace or sign-in message | an image with no CA bundle. Copy in `/etc/ssl/certs/ca-certificates.crt`, as `docker/server/Dockerfile` does |
| `listen tcp 0.0.0.0:80: bind: permission denied` | Runtime keeps the traditional privileged range; see [Port 80 without capabilities](#port-80-without-capabilities) |
| `exec /prusa_connect: operation not permitted` | A file capability was added to the binary and falls outside the bounding set |
| `permission denied` opening a file under `/storage` | Bind mount not chowned to `10001:10001` |
| `read-only file system` | A write path with no volume; add one |
| Container stuck `starting` past 30s | Readiness is false. `curl /api/v1/readiness` and read `blocked` |

## Checking a modified image

```bash
make docker-check
```

It builds the image and asserts it does not run as uid 0. It asserts the runtime user can write
every path in [Volumes and ownership](#volumes-and-ownership). It runs the image with no
configuration file and proves `/api/v1/health`, `/api/v1/readiness`, `/openapi.json` and `/docs`
all answer 200, and that `SIGTERM` stops it cleanly. Then it brings the compose stack up until the
health check passes. The same checks run in CI as the **Docker / Hardened image** job.

A second, cheaper gate runs under `go test` and builds nothing. It reads the Dockerfile, the compose
service and the command's source, and refuses four things:

- a `CMD` naming something the binary does not route to the core
- a health check pointed at a path the core does not serve
- a directory the image creates that nothing proves writable
- an `environment:` key on the service that nothing reads
