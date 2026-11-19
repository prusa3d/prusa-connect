# syntax=docker/dockerfile:1

# Go toolchain version, kept in step with the `go` directive in go.mod.
# `scripts/check-go-version.sh` enforces it.
ARG GO_VERSION=1.26.11

# The component library and the client SDK, built once for the two stages
# below: the core's own application and the bundled extensions both read them
# from this checkout rather than from a registry. Every export in the library's
# package.json, the `svelte` condition included, names a file under `dist/`, so
# until its build has run an import-map plugin resolves nothing and `vite
# build` stops with `@prusa/connect-ui did not resolve; is it installed
# in this app?`.
#
# Digest-pinned: a floating tag means two builds of the same commit can differ.
# Bump the tag and the digest together
# (`docker buildx imagetools inspect node:24-alpine`).
# On the build machine's platform, never under an emulator: everything this
# stage and the two built from it produce is JavaScript, the same on every
# processor the image is published for.
FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:06d7adf3e0ef61a13f1d28682726366f97a15aa1888c934d3ecec86c33918b53 AS packages-builder
ENV PNPM_HOME=/pnpm
ENV PATH="$PNPM_HOME:$PATH"
ENV HUSKY=0
ENV CI=true
RUN corepack enable
WORKDIR /app
# The library and the app take their lint and format configuration from
# packages/standards, which their installs copy in through a file: override.
# It has no build step, so the whole folder goes in before any install.
COPY packages/standards ./packages/standards
# Manifests, lockfiles and workspace settings only, so this install layer is
# reused until a dependency changes.
COPY packages/typescript/package.json packages/typescript/pnpm-lock.yaml      packages/typescript/pnpm-workspace.yaml ./packages/typescript/
RUN cd packages/typescript && pnpm install --frozen-lockfile
COPY packages/typescript ./packages/typescript
RUN cd packages/typescript && pnpm run build
# No manifest-only pre-copy for these two, unlike packages/typescript above.
# Their pnpm-workspace.yaml redirects @prusa/frontend-standards to this tree
# through `overrides`, so their lockfiles record `file:../standards` as the
# specifier while package.json still names the version. `--frozen-lockfile`
# applies the override before it compares and accepts that; the
# `verifyDepsBeforeRun: error` check they also set compares the raw manifest
# and rejects it. That check only runs when a manifest is newer than the last
# install, which a later `COPY` of the same project makes it under kaniko,
# the one builder that rewrites mtimes. So the
# sources land before the install and nothing touches them after it.
COPY packages/svelte ./packages/svelte
# The library build writes its catalogues from the error registry, so the
# registry rides into the stage beside the sources.
COPY docs/errors/error-codes.json ./docs/errors/error-codes.json
# --ignore-scripts, because the package's `prepare` hook writes its i18n
# catalogues from sources, and `pnpm run build` writes them anyway.
RUN cd packages/svelte && pnpm install --frozen-lockfile --ignore-scripts && pnpm run build

# The core's own application, which the Go binary embeds and serves at /
# (ADR-20260911-4p6 rule 33).
FROM packages-builder AS app-builder
# The app reads the Fractal module's routes through this client, linked from
# here the way the SDK is.
COPY packages/fractal/client ./packages/fractal/client
RUN cd packages/fractal/client && pnpm install --frozen-lockfile && pnpm run build
COPY frontend ./frontend
RUN cd frontend && pnpm install --frozen-lockfile && pnpm run build && test -f dist/index.html

# The browser halves of the extensions the image carries in /opt/modules. A
# stage apart from app-builder, so an edit to one rebuilds neither the other
# nor the library. Each folder lands whole before its install, for the kaniko
# reason given above packages/svelte; each vite build also reads ../module.yaml.
FROM packages-builder AS extensions-builder
COPY extensions/fleet-stats ./extensions/fleet-stats
RUN cd extensions/fleet-stats/ui && pnpm install --frozen-lockfile && pnpm run build && test -f dist/ui/index.js
COPY extensions/easyprint ./extensions/easyprint
RUN cd extensions/easyprint/ui && pnpm install --frozen-lockfile && pnpm run build && test -f dist/ui/index.js
COPY extensions/parts ./extensions/parts
RUN cd extensions/parts/client && pnpm install --frozen-lockfile && pnpm run build && \
    cd ../ui && pnpm install --frozen-lockfile && pnpm run build && test -f dist/ui/index.js
COPY extensions/buddy-debug-logs ./extensions/buddy-debug-logs
RUN cd extensions/buddy-debug-logs/client && pnpm install --frozen-lockfile && pnpm run build && \
    cd ../ui && pnpm install --frozen-lockfile && pnpm run build && test -f dist/ui/index.js
COPY extensions/threemf ./extensions/threemf
RUN cd extensions/threemf/client && pnpm install --frozen-lockfile && pnpm run build && \
    cd ../ui && pnpm install --frozen-lockfile && pnpm run build && test -f dist/ui/index.js
COPY extensions/server-slicing ./extensions/server-slicing
RUN cd extensions/server-slicing/client && pnpm install --frozen-lockfile && pnpm run build && \
    cd ../ui && pnpm install --frozen-lockfile && pnpm run build && test -f dist/ui/index.js
COPY extensions/materials ./extensions/materials
RUN cd extensions/materials/client && pnpm install --frozen-lockfile && pnpm run build && \
    cd ../ui && pnpm install --frozen-lockfile && pnpm run build && test -f dist/ui/index.js
COPY extensions/simple-queue ./extensions/simple-queue
RUN cd extensions/simple-queue/client && pnpm install --frozen-lockfile && pnpm run build && \
    cd ../ui && pnpm install --frozen-lockfile && pnpm run build && test -f dist/ui/index.js

# Build Go server (matches Makefile: go build -o bin/server ./cmd/prusa-connect)
# The -trixie variant matches the debian:trixie-slim runtime stage below. The
# build links no cgo, so the binary is static and the two glibcs never have to
# agree, but a builder whose libc differs from the runtime's is a trap the next
# reader would have to rediscover.
#
# Digest-pinned to the same golang build docker/race/Dockerfile names. A
# digest wins over the tag, so scripts/check-go-version.sh requires this digest
# to equal the race image's, whose tag must name the release go.mod pins. A Go
# bump therefore moves the digest here too, or the check fails.
#
# The stage runs on the build machine's platform and Go compiles for the
# image's platform, so a multi-platform build runs no compiler under an
# emulator. Only `go build` is pointed at the target. `go run` builds a tool
# for this machine and runs it here, so GOOS and GOARCH are never set for the
# whole stage.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-trixie@sha256:fa340cb59ebeb1d53ac1cccfc334201d938fe5f3458003cccb767096d8e032b8 AS builder

# No apt step. The image built with a C toolchain and the SQLite headers while
# a cgo-only driver was linked. Nothing reaches that driver now, so the build is
# CGO_ENABLED=0 and needs neither.

WORKDIR /app

# Copy go mod files
COPY go.* ./
# The nested sdk module is resolved through a local replace in go.mod, so its
# manifest has to exist before any module-graph command runs - `go mod
# download` fails outright when a replace target directory is missing.
#
# There is no sdk/go.sum. The sdk imports nothing outside the standard library,
# so it requires nothing and records no checksums, and
# internal/arch/layout_test.go refuses the import that would give it one.
COPY sdk/go.mod ./sdk/

# Warm the module cache. Every module this build needs is public, so the step
# takes no credential and no BuildKit secret.
RUN go mod download

# Copy all source files
COPY . ./

# The core's own application, into the directory //go:embed reads. It lands
# after `COPY . ./`, which brings in the host's placeholder-only directory, and
# before `go build`, which embeds it. Without it the binary compiles and serves
# no page at all.
COPY --from=app-builder /app/frontend/dist ./internal/transport/http/httpapi/appdist

# Each text file of the app is replaced with a gzip copy, which the server
# sends to a browser as it is stored.
RUN go run ./cmd/compress-app ./internal/transport/http/httpapi/appdist

# Build with config-driven feature tags (matches `make build-config`).
#
# The two arguments are the Makefile's own knobs under the Makefile's own names
# (CONFIG_FILE and EXTRA_BUILD_TAGS, Makefile:5-6) and carry its defaults, so a
# plain `docker build` builds what `make build-config` builds and this file
# states no policy of its own.
#
# They exist because the shipped configuration is the project's default and not
# every operator's. A build that needs a part this checkout leaves off asks for
# the tag directly, `--build-arg EXTRA_BUILD_TAGS=observability`, and a build
# that answers to a whole configuration of its own puts that file in the context
# and names it, `--build-arg CONFIG_FILE=config/config.mine.yaml`. Neither
# edits config/config.yaml, which is what a fork would otherwise have to carry
# forever.
#
# BUILD_VERSION is the version the binary reports in its log, at
# /api/v1/health and for --version. The build context carries no .git, so it
# cannot be read here. `make docker-check` passes what scripts/build-version.sh
# prints, and a build from a release archive passes the release,
# `--build-arg BUILD_VERSION=0.2.0`. Empty stamps nothing.
ARG CONFIG_FILE=config/config.yaml
ARG EXTRA_BUILD_TAGS=""
ARG BUILD_VERSION=""
ARG UPGRADE_PUBLIC_KEYS=""
# Where the binary links its documentation: the beta portal while the project
# is private. A public build passes --build-arg DOCS_URL=<the public site>.
ARG DOCS_URL=https://beta.prusa3d.com/connect-local/docs/{version}/
# The platform the image is built for, set by BuildKit. TARGETVARIANT is v7
# for linux/arm/v7, and GOARM takes the number alone. Empty on amd64 and arm64,
# where Go ignores GOARM.
ARG TARGETOS TARGETARCH TARGETVARIANT
RUN TAGS="$(go run ./cmd/detect-build-tags --config "$CONFIG_FILE" --variant full --extra "$EXTRA_BUILD_TAGS")" && \
    echo "Building with tags: full $TAGS" && \
    GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" CGO_ENABLED=0 \
    go build -v -tags "full $TAGS" \
    -ldflags="-X github.com/prusa3d/prusa-connect/internal/kernel/version.stamped=${BUILD_VERSION} -X github.com/prusa3d/prusa-connect/internal/kernel/version.docsURL=${DOCS_URL} -X github.com/prusa3d/prusa-connect/internal/upgrade.officialPublicKeys=${UPGRADE_PUBLIC_KEYS}" \
    -o /prusa_connect ./cmd/prusa-connect

# The parts release, laid out by the script its own gate packs with. Its Go
# module binds the sdk to this checkout through a replace, and its two
# database drivers are at the versions the core pins, which `go mod download`
# above already fetched. So the build downloads nothing. The binary is named
# for the image's platform: the export at the start of each step points Go at
# it, and `go env` then reports it.
COPY --from=extensions-builder /app/extensions/parts/ui/dist ./extensions/parts/ui/dist
RUN export GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" && \
    BINARY="/tmp/parts_$(go env GOOS)_$(go env GOARCH)" && \
    CGO_ENABLED=0 go -C extensions/parts build -o "$BINARY" . && \
    sh extensions/scripts/pack-release.sh extensions/parts/ui/dist "$BINARY" /opt/modules/parts

# The Prusa printer logs release, laid out the same way.
COPY --from=extensions-builder /app/extensions/buddy-debug-logs/ui/dist ./extensions/buddy-debug-logs/ui/dist
RUN export GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" && \
    BINARY="/tmp/buddy-debug-logs_$(go env GOOS)_$(go env GOARCH)" && \
    CGO_ENABLED=0 go -C extensions/buddy-debug-logs build -o "$BINARY" . && \
    sh extensions/scripts/pack-release.sh extensions/buddy-debug-logs/ui/dist "$BINARY" /opt/modules/buddy-debug-logs

# The 3MF projects release, laid out the same way. Its Go module needs the sdk
# alone.
COPY --from=extensions-builder /app/extensions/threemf/ui/dist ./extensions/threemf/ui/dist
RUN export GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" && \
    BINARY="/tmp/threemf_$(go env GOOS)_$(go env GOARCH)" && \
    CGO_ENABLED=0 go -C extensions/threemf build -o "$BINARY" . && \
    sh extensions/scripts/pack-release.sh extensions/threemf/ui/dist "$BINARY" /opt/modules/threemf

# The server-slicing release, laid out the same way. It carries no PrusaSlicer.
COPY --from=extensions-builder /app/extensions/server-slicing/ui/dist ./extensions/server-slicing/ui/dist
RUN export GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" && \
    BINARY="/tmp/server-slicing_$(go env GOOS)_$(go env GOARCH)" && \
    CGO_ENABLED=0 go -C extensions/server-slicing build -o "$BINARY" . && \
    sh extensions/scripts/pack-release.sh extensions/server-slicing/ui/dist "$BINARY" /opt/modules/server-slicing

# The materials release, laid out the same way.
COPY --from=extensions-builder /app/extensions/materials/ui/dist ./extensions/materials/ui/dist
RUN export GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" && \
    BINARY="/tmp/materials_$(go env GOOS)_$(go env GOARCH)" && \
    CGO_ENABLED=0 go -C extensions/materials build -o "$BINARY" . && \
    sh extensions/scripts/pack-release.sh extensions/materials/ui/dist "$BINARY" /opt/modules/materials
# The print queue release, laid out the same way.
COPY --from=extensions-builder /app/extensions/simple-queue/ui/dist ./extensions/simple-queue/ui/dist
RUN export GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" && \
    BINARY="/tmp/simple-queue_$(go env GOOS)_$(go env GOARCH)" && \
    CGO_ENABLED=0 go -C extensions/simple-queue build -o "$BINARY" . && \
    sh extensions/scripts/pack-release.sh extensions/simple-queue/ui/dist "$BINARY" /opt/modules/simple-queue

# The Fractal release. It has no web page, so its bundle is the manifest and
# the route document alone. Its NATS dependencies are not the core's, so this
# build downloads them.
RUN export GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" && \
    BINARY="/tmp/fractal_$(go env GOOS)_$(go env GOARCH)" && \
    CGO_ENABLED=0 go -C packages/fractal build -o "$BINARY" . && \
    mkdir /tmp/fractal-bundle && \
    cp packages/fractal/module.yaml packages/fractal/openapi.json /tmp/fractal-bundle/ && \
    sh extensions/scripts/pack-release.sh /tmp/fractal-bundle "$BINARY" /opt/modules/fractal

# Digest-pinned: see the note on the app-builder stage.
FROM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258

# Fixed uid/gid rather than whatever useradd hands out next. The runtime writes
# into /data, /files and /storage, and an operator who bind-mounts a host
# directory over one of those has to chown it to a number they can predict.
ARG APP_UID=10001
ARG APP_GID=10001

# Copy built binary
COPY --from=builder /prusa_connect /prusa_connect

# First-party extensions as releases, one directory each. They do nothing until
# the operator's `core.modules_local` names them: the Extensions page then
# offers each, and switching one on installs a copy into `core.modules`. Owned
# by root, readable by the runtime user, because nothing writes here.
COPY --from=extensions-builder /app/extensions/fleet-stats/ui/dist /opt/modules/fleet-stats
COPY --from=extensions-builder /app/extensions/easyprint/ui/dist /opt/modules/easyprint
COPY --from=builder /opt/modules/parts /opt/modules/parts
COPY --from=builder /opt/modules/buddy-debug-logs /opt/modules/buddy-debug-logs
COPY --from=builder /opt/modules/threemf /opt/modules/threemf
COPY --from=builder /opt/modules/server-slicing /opt/modules/server-slicing
COPY --from=builder /opt/modules/materials /opt/modules/materials
COPY --from=builder /opt/modules/fractal /opt/modules/fractal
COPY --from=builder /opt/modules/simple-queue /opt/modules/simple-queue

# The root certificates the server verifies an https peer against: the module
# marketplace, the releases it points at, and an OIDC provider. trixie-slim
# ships none, so without this every https read fails with "certificate signed
# by unknown authority". The builder stage is Debian too, and this is its
# ca-certificates bundle, at the path Go's crypto/x509 reads on Linux.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# A printer-facing listener may be put below port 1024: the core's is a
# connector's own listener.addr and the shipping server's is
# connectors.prusa-xbuddy.port, which defaults to :80. This image grants NO
# capability for that, deliberately.
#
# Container runtimes set net.ipv4.ip_unprivileged_port_start=0, so inside a
# container there is no privileged port range and an unprivileged process binds
# :80 like any other port. Measured on this image, running as uid 10001 with
# `--cap-drop ALL`, no cap_add and no-new-privileges: the listener comes up and
# serves.
#
# An earlier revision of this file put `cap_net_bind_service=+ep` on the binary
# as a file capability. Do not reinstate it. It bought nothing the sysctl was
# not already giving, and it broke `cap_drop: [ALL]`: the kernel refuses the
# execve outright when a binary's file capabilities fall outside the bounding
# set, so the container died at startup with `exec /prusa_connect:
# operation not permitted` - an error naming neither capabilities nor the port.
#
# If you deploy onto a runtime that keeps the traditional 1024 boundary, add
# NET_BIND_SERVICE to the container's capabilities, or move
# connectors.prusa-xbuddy.port above 1024. Both are runtime-side changes; the
# image stays capability-free.
RUN set -eux; \
    groupadd --gid "${APP_GID}" prusa; \
    useradd --uid "${APP_UID}" --gid "${APP_GID}" --no-create-home --shell /usr/sbin/nologin prusa; \
    \
    # Runtime write targets, owned by the runtime user before any volume is
    # mounted. Docker seeds an empty NAMED volume from the image path and carries
    # this ownership across with it, so the compose volumes land writable with no
    # entrypoint chown. A BIND mount does not get that treatment: the host
    # directory keeps its own ownership and has to be chowned to APP_UID:APP_GID
    # by whoever mounts it.
    #
    # /config/config.d is here because the operator's file is mounted read-only
    # and the server writes two things beside it: the machine-owned resource
    # overlay the API rewrites, and the authorization graph. Without a writable
    # directory of its own the core refuses to start under `read_only: true`
    # with `mkdir config/config.d: read-only file system`, and a Kubernetes
    # deployment mounting a ConfigMap gets the same answer.
    install -d -o prusa -g prusa -m 0750 /data /files /storage /config/config.d

# Relative paths in config/config.yaml (./data, ./files, ./storage) and the
# default --config path resolve from here.
WORKDIR /

USER prusa

# Probes GET /api/v1/readiness through the binary itself, so the image needs no
# curl or wget. Readiness rather than liveness: a container reported healthy is
# one a caller may be sent to, and readiness is the endpoint that answers that.
# start-period covers the cold path: opening the authorization graph and
# starting every declared resource.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD ["/prusa_connect", "core", "healthcheck"]

# The core is what this image runs. ADR-0013 rule 13 holds the binary's own
# default at the shipping server until every condition in earlier research passes.
# The container is the deployment target the hosted product ships, so it runs
# the core and serves the Svelte application the app-builder stage put in the
# embed directory above.
#
# CMD rather than a longer ENTRYPOINT, so `docker run <image> core healthcheck`
# and any other subcommand still reach the binary.
ENTRYPOINT ["/prusa_connect"]
CMD ["core"]