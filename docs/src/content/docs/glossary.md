---
title: Glossary
description: Every word this project uses as a term of art, the Go concepts its documentation assumes, and the outside standards it leans on, each with what it means here and where it is defined.
audience: build
rewrite:
  describes: current
  implementedBy:
    - sdk
    - internal/authz
    - internal/kernel/ref
    - internal/printer
    - internal/extension
---

Three vocabularies meet in this documentation and a reader is assumed to hold
all three. This page holds them instead. Each entry says what the thing is,
names the file that defines it, and belongs to one of three groups, because the
group decides where the answer to a further question is: this repository, the Go
specification, or a published standard.

Every anchor is the term itself, so `#origin` and `#goroutine` can be linked
from anywhere without opening this page first.

## How to read a page that links here

| convention | what it means |
|---|---|
| a term linked to this page | its first use on that page. Later uses are not linked |
| a path in a code span, such as `sdk/event/origin.go` | a file in this repository, relative to its root |
| a path with a line, such as `internal/authz/tuple.go:41` | that file, at the line where the symbol starts |
| a name in a code span with no path, such as `Can` | a Go symbol in the package the surrounding text names |

### The linking rule

1. a page links a term **once**, at its first use in its body prose, and never
   again
2. never in a heading, never in a table cell, never inside a code span
3. a page that is itself the fuller treatment of a term does not link that term.
   It is what this page points at

The reason is measured rather than aesthetic. Studies of reading behaviour show that
nobody reads a reference page top to bottom: the behaviour is scanning, and the
recommendation drawn from thirteen years of eyetracking is front-loaded headings
and lists. A link repeated fourteen times carries no information after the first
and costs the scan every time, because a link renders as something worth
stopping at.

Rule 3 is why `docs/src/content/docs/onboarding/the-model.md` links none of the
nouns it defines, and why the chapters under
`docs/src/content/docs/internals/` link neither the file nor the invariant they
are the reference for. Each carries one pointer to this page instead.

## The index

**This project:** [action](#action) · [adapter](#adapter) ·
[the airlock](#the-airlock) · [capability](#capability) ·
[class](#class) · [composition root](#composition-root) ·
[connector](#connector) · [coverage tier](#coverage-tier) ·
[cursor](#cursor) · [descriptor](#descriptor) ·
[device plane](#device-plane) · [driver baseline](#driver-baseline) ·
[envelope](#envelope) · [epoch](#epoch) · [event hub](#event-hub) ·
[fault](#fault) · [generation](#generation) · [grant](#grant) ·
[inbound and outbound](#inbound-and-outbound) · [kind](#kind) ·
[namespace](#namespace) · [offset](#offset) · [origin](#origin) ·
[position](#position) · [principal](#principal) ·
[profile tier](#profile-tier) · [projection](#projection) ·
[reading](#reading) · [relation tuple](#relation-tuple) · [resource instance](#resource-instance) ·
[role](#role) · [sealing](#sealing) · [sink](#sink) · [site](#site) ·
[standing](#standing) · [supervision plane](#supervision-plane) ·
[supervisor](#supervisor) · [userset](#userset) · [verb](#verb) ·
[vocabulary](#vocabulary)

**Go:** [build tag](#build-tag) · [cgo](#cgo) · [channel](#channel) ·
[context](#context) · [go:embed](#go-embed) · [goroutine](#goroutine) ·
[interface](#interface) · [module](#module) · [race detector](#race-detector) ·
[struct tag](#struct-tag) · [table-driven test](#table-driven-test)

**Outside standards:** [AsyncAPI](#asyncapi) · [CloudEvents](#cloudevents) ·
[DNS-1123](#dns-1123) · [mbedTLS](#mbedtls) ·
[OAuth 2.1 and PKCE](#oauth-2-1-and-pkce) · [OpenAPI](#openapi) ·
[RFC 9457](#rfc-9457) · [semantic versioning](#semantic-versioning) ·
[server-sent events](#server-sent-events) ·
[Zanzibar and ReBAC](#zanzibar-and-rebac)

## This project's own terms

Searching this repository answers a question about any of these. None of them is
an industry term, and several collide with one: an epoch here is not a Unix
epoch, and a class here is not a Go type.

### action {#action}

What a caller performs, and the only thing an HTTP handler ever asks the
authorization graph about. `observe`, `operate`, `configure` and `administer`
are the device ones, with `view_resource`, `configure_resource` and the rest on
a resource kind. An action belongs to exactly one object type, so asking a
device action of a resource kind is a caller mistake rather than a refusal.

- defined in `internal/authz/vocabulary.go`, one constant and one bit each. The
  whole set with the relations that confer it is the
  [authorization vocabulary](./authorization-vocabulary.md)
- no call site names a [role](#role), so the role layer changes without
  touching a handler

### adapter {#adapter}

A reference implementation of one `sdk/` contract, living under `adapters/`.
The directories there cover identity providers, event sinks, cameras, record
and history backends, a printer dialect and storage backends. They are written
to be copied by a community author and shipped second.

- an adapter compiles against `sdk/` alone, never against `internal/`

### the airlock {#the-airlock}

The single file where an operator's configuration is allowed to influence what
the compiler is asked to build: `adapters/tags.json`, which maps a resource kind
and type onto a [build tag](#build-tag). Configuration is operator input and a
build tag is a compiler argument, so the two meet in one place or in an
uncountable number of them.

- the file is `adapters/tags.json`, read by `cmd/detect-build-tags`
- `internal/arch/airlock_test.go` refuses a registered type with no entry, an
  entry naming a type nothing registers, and a config file whose key names a
  build tag

### capability {#capability}

The set of [verbs](#verb) one printer accepts, each with a descriptor for its
arguments. It is resolved per printer rather than declared, by intersecting
three tiers: the [driver baseline](#driver-baseline), the
[profile tier](#profile-tier) and the machine's own probe.

- the merge is `internal/printer/capability/resolver.go`
- the check that every command passes is
  `internal/printer/capability/session.go`, above the driver and never inside it

### class {#class}

The four-way discriminator on an [envelope](#envelope) that decides queueing,
propagation and expiry: `telemetry`, `state`, `command`, `audit`. It exists so a
node that has never heard of an event type still knows what to do with an event
of it.

- defined in `sdk/event/envelope.go`
- `telemetry` is conflated and at most once; the other three are FIFO, and
  `state` and `audit` are durable

### composition root {#composition-root}

The one package allowed to import everything: `internal/app`. It reads the
configuration, constructs every part in an order the parts cannot enforce among
themselves, wires the mappings neither side of a boundary may own, serves, and
stops in reverse. It holds no business rules of its own.

- the order and the reason for each step are the package doc at
  `internal/app/app.go`

### connector {#connector}

The code that speaks one printer dialect: the wire format, the handshake, the
telemetry keys and the command frames of a particular firmware family. `xbuddy`
is the only one with machines in the field.

- `internal/connector/xbuddy` is the dialect; `sdk/connectorapi` is the contract
  a third party implements
- a connector is built once at startup and mounted on the router, never
  supervised as a [resource instance](#resource-instance), because a handler
  swapped underneath a mounted route would be a device wire that changed
  identity while a printer was on it (`internal/app/connectors.go`)

### coverage tier {#coverage-tier}

What the profiles a driver ships know about between them, taken as a whole.
Where no profile matches a particular machine, the coverage is what the
[capability](#capability) merge narrows to instead, so an unrecognised printer
stands in the narrowest place the data supports rather than the widest.

- computed by `Registry.Coverage` in `internal/printer/profile`
- a driver that ships no profiles has no coverage, and an absent tier narrows
  nothing, so that driver's whole baseline stands

### cursor {#cursor}

A reader's place across every stream it follows, grouped by the
[generation](#generation) each place belongs to. It is what a browser sends back
in `Last-Event-ID` to resume, and it survives one node restarting without
invalidating the positions held on the others.

- defined in `sdk/event/cursor.go`
- it renders as `epoch:kind/id=offset,kind/id=offset;epoch:...`

### descriptor {#descriptor}

The fixed set of attributes every registered extension carries, the way the
[envelope](#envelope) gives every event the same ones: a summary, a description,
its own version, the contract version it compiled against, a stability level,
and optionally its config keys, the events it emits and what it needs of the
host.

- defined in `sdk/extension/descriptor.go`
- a missing mandatory field refuses the registration, because an empty string
  would otherwise ship as documentation

### device plane {#device-plane}

The host's half of every printer dialect: it admits a printer that has dialled
in, resolves what that printer can do, registers it in the authorization graph
so somebody may see it, holds its state tree until the session ends, and answers
the printer half of the HTTP surface.

- it is `internal/app/devices.go`
- a printer stays in its table for as long as its driver session does, not for
  as long as its socket does, which is what lets a command queue for a machine
  that is switched off

### driver baseline {#driver-baseline}

Everything one dialect's code can do for any device at all, before any narrowing
by [profile](#profile-tier) or probe. It is the ceiling of the
[capability](#capability) merge: nothing can add to it.

- declared by `Driver.Baseline()` in `sdk/connectorapi/driver.go`

### envelope {#envelope}

One event, as it travels: an id, a source, a type, a time, a subject, a
[class](#class) and a JSON payload. It is a [CloudEvents](#cloudevents) 1.0
envelope by attribute name and this project's by definition, with no CloudEvents
library behind it.

- defined in `sdk/event/envelope.go`
- one of the four things this project never makes pluggable, per `AGENTS.md`

### epoch {#epoch}

A counter that says which generation of something an answer belongs to. Two
unrelated ones carry the name, and telling them apart is worth doing once.

| epoch | what it counts | where |
|---|---|---|
| the authorization epoch | permission changes. One atomic load, never a query | `internal/authz/store.go` |
| the capability epoch | one printer's arrivals. It moves on every arrival, not only when the merged set differs | `internal/app/devices.go` |
| the numbering epoch | the process that assigned a stream's offsets, so a restart invalidates them | `sdk/event/cursor.go` |

### event hub {#event-hub}

The in-process fan-out: one channel per [origin](#origin), two queue classes per
subscriber, and one marshal shared by every subscriber of an event. It never
blocks a publisher and allocates nothing per subscriber.

- it is `internal/transport/stream`
- there is no broker in the default deployment. A [sink](#sink) is a subscriber
  of the hub like any other reader

### fault {#fault}

A failure on its way to a client, carrying a registered `domain/REASON` code
such as `core.printer/NOT_FOUND`. One value renders as
[RFC 9457](#rfc-9457) `problem+json` over HTTP, as an `envelope`'s `data.error`
on the bus, and as the error member of a printer command result.

- defined in `sdk/fault/fault.go`, rendered from `sdk/fault/payload.go`
- every registered code is on the [error code reference](./error-codes.md),
  which is generated from the registry

### generation {#generation}

The set of places a reader holds inside one numbering [epoch](#epoch). A
[cursor](#cursor) carries one generation per node it follows, so one node
recreating its log invalidates that node's offsets and leaves the rest
resumable.

- defined in `sdk/event/cursor.go`

### grant {#grant}

One [relation tuple](#relation-tuple), written from the point of view of the
person writing it. `device:p4f2a#viewer@user:alice` grants alice read access to
that device.

- the API surface is `internal/authz/grant.go`, which bounds every write by what
  the caller already holds
- grants on a resource are written on the [kind](#kind) rather than on the
  instance, because creating an instance names no instance

### inbound and outbound {#inbound-and-outbound}

The two directions a [connector](#connector) can face. An inbound driver is
dialled by its printers and contributes HTTP routes for them to dial. An
outbound driver is one the host reaches, and contributes no routes.

- reported by `connectorapi.Directions` in `sdk/connectorapi/direction.go`
- `internal/app/connectors.go` mounts the routes of the first and skips the
  second

### kind {#kind}

The closed set of resource kinds a [ref](#resource-instance) can name:
`storage`, `auth`, `sink`, `driver`, `db`, `camera`, `history`, `link`,
`asset`, `gcode-preset`, `dashboard`, `printer` and `node`. There is no
registration hook. A new kind is a change to one file.

- declared in `internal/kernel/ref/ref.go`, and published as the
  [resource kinds page](./resource-kinds.md)
- everything up to `dashboard` is configurable, meaning an operator declares
  instances of it; `printer` and `node` are intrinsic and nothing declares them

### namespace {#namespace}

The prefix that qualifies a registered extension's name, derived from the Go
[module](#module) of the package that registered it. Nothing an author writes
selects it, which is what stops a third-party plugin claiming a built-in name.

- derived in `sdk/extension/namespace.go`
- `github.com/acme/mk4` renders as `com.github.aux.acme.mk4`. The label `aux`
  marks where the host name ends. Go refuses it as a path element and as the
  first label of a host. A module path carrying it as a label is refused here
  too, so the rendering reads back as the module path it came from
- only the main module of a build holds the namespace `core`

### offset {#offset}

One [origin](#origin)'s own count of its durable events. It is per origin and
never global, because the distance between two values of a global counter is a
tally of events on origins the reader may not be allowed to see.

- defined in `sdk/event/cursor.go`
- telemetry carries no offset at all, since a reader that never receives one
  must not be handed a number that moves when other origins publish

### origin {#origin}

What an event came from, written `kind:id`, such as `device:p4f2a91c0e33b` or
`sink:audit`. It is both the key the [event hub](#event-hub) routes on and the
key authorization is resolved against, so a device id and a resource instance
name obey one grammar. Every device carries the kind `device`, whatever sort of
machine it is.

- defined in `sdk/event/origin.go`
- the set of kinds is owned by `internal/kernel/ref`, not by `sdk/`. The sdk
  module owns the shape an origin has to survive being rendered into

### position {#position}

One event's place in one stream: an [origin](#origin) and an
[offset](#offset). An SSE frame's `id` is a position, and a browser replays it
with no client code involved.

- defined in `sdk/event/cursor.go`

### principal {#principal}

Who is calling, as the authorization graph keys them: `type:id`. A principal is
a `user`, a `group`, a `service_account`, a `device_credential`, a `node`, a
`sink` or a `history` provider. `device` is the object a grant is written on
and not a principal.

- declared in `internal/authz/vocabulary.go`, and published as the
  [authorization vocabulary](./authorization-vocabulary.md)
- the type prefix is applied in exactly one place,
  `internal/app/auth.go`, because an identity provider names a subject in its
  own namespace and knows nothing of the graph's types

### profile tier {#profile-tier}

The middle tier of the [capability](#capability) merge: a firmware matrix
compiled into the binary that says what one printer model running one firmware
band is known to support. It narrows the [driver baseline](#driver-baseline) and
can never widen it.

- the data is `internal/printer/profile/profiles.json`, embedded with
  [go:embed](#go-embed) at `internal/printer/profile/builtin_all.go`
- firmware bands are ordered by [semantic versioning](#semantic-versioning)
  precedence, which is the one place a version comparison is allowed

### projection {#projection}

The denormalized tables the authorization graph actually reads: for every
principal and object, the mask of actions they hold. A permission check never
walks the graph, it reads one indexed row that a write already compiled.

- compiled by `internal/authz/project.go`, inside the transaction that changed a
  tuple
- the tables are `device_access` and `resource_access`

### reading {#reading}

One measured value of a whole machine, such as the bed or the chamber
temperature. A driver describes each reading under `/readings/<id>/` in the
device's state: its kind, its unit and the path of its value. A client finds
the readings there and holds no list of its own.

- the type is `connectorapi.Reading` in `sdk/connectorapi/readings.go`
- a value of one tool is not a reading. It stays in the tool tree

### relation tuple {#relation-tuple}

The only fact the authorization graph stores: `object#relation@subject`, for
example `device:p4f2a#viewer@user:alice`. The subject may itself be an
`object#relation` pair, a [userset](#userset), which is what makes groups,
nested groups and hierarchy inheritance one mechanism instead of three.

- constructed and validated in `internal/authz/tuple.go`
- the form is taken verbatim from [Zanzibar](#zanzibar-and-rebac)

### resource instance {#resource-instance}

One configured, running copy of a resource type: a storage root, an event
[sink](#sink), an identity provider, a printer driver. Several instances of one
type is the normal case, and each is named `kind:name` by the operator.

- the identity type is `ref.Ref` in `internal/kernel/ref/ref.go`, with a
  host-minted `ref.UID` beside it so renaming an instance does not orphan the
  rows that reference it
- declared in the `resources:` block of the configuration file, merged with a
  machine-owned overlay by `internal/app/resource`

### role {#role}

The ordered layer between a [grant](#grant) and an [action](#action):
`viewer < operator < manager`, plus `admin` on an organization. A principal's
effective role on a device is the highest of its direct grant, the grants on
every group above it, and the org role.

- the ordering is `internal/authz/vocabulary.go`
- `blocked` is not a role. It deletes a principal from the role layer entirely,
  so one deny covers every action and cannot be forgotten when an action is added

### sealing {#sealing}

Closing a registration list so nothing further can be added. Extensions register
themselves before `main` runs; the host drains the list once when it builds, and
from that moment a registration is an error rather than a silent no-op, and the
resulting table is immutable and needs no lock on a lookup.

- `Set.Drain` in `sdk/extension/set.go`
- the same word is used of the HTTP router in
  `internal/transport/http/httpapi/server.go`, which refuses to return a server
  whose routes and OpenAPI document disagree

### sink {#sink}

An outbound destination that events are delivered to: a webhook, an MQTT broker,
an AMQP exchange, a Kafka topic. It is a [resource instance](#resource-instance)
of kind `sink`, and it reads the bus as a [principal](#principal) of type
`sink`, named after the instance. It is deliberately not a `service_account`: a
service account of the same name would otherwise be the same principal, holding
one set of grants between them.

- the contract is `sdk/eventsapi`; the wiring is `internal/app/sinks.go`
- declaring a sink grants it nothing. A sink with no grant receives nothing

### site {#site}

This node's token, carried in the source of every event it publishes. It is what
distinguishes two servers' events from each other once they are federated.

- the config key is `core.site`, defaulting to `local`
  (`internal/app/config/config.go`)
- an event's source renders as `urn:prusa:site:<site>/<kind>/<id>`
  (`sdk/event/envelope.go`)

### standing {#standing}

A principal's level on one object, split into the half written on the object
itself and the half arriving from a group or an organization above it. The
enforcement surfaces need only the sum. The grant surface needs the split,
because a caller who administers a device may write back what is written on the
device and may not write back what the org confers.

- defined in `internal/authz/project.go`
- without the split, a manager delegated one device could permanently block the
  org admin who delegated it

### supervision plane {#supervision-plane}

The part of the process that carries out a reconcile plan: starting, stopping
and replacing [resource instances](#resource-instance) so that what is running
matches what is declared. Six outcomes are possible for one instance: none,
in-place, restart, replace, start, stop.

- `internal/app/resourcehost` drives it; `internal/app/plane.go` owns its
  lifetime
- its context is deliberately detached from the caller's, because every
  supervised instance is started under it and it has to outlive the listeners

### supervisor {#supervisor}

The wrapper around one running [resource instance](#resource-instance) through
which every call into third-party code passes. It owns the queue, the drop
policy, the per-call deadline, panic recovery, the circuit breaker and the
outcome classification, so no individual extension has to be trusted with any of
them.

- `internal/extension/supervisor`
- two things it cannot do are stated at
  `internal/extension/supervisor/instance.go`: a deadline stops only work that
  reads its [context](#context), and a recovery catches a panic only on the
  [goroutine](#goroutine) that deferred it

### userset {#userset}

A subject written as `object#relation` rather than as a principal, naming
everyone who holds that relation on that object. `group:ops#member` is every
member of the ops group.

- constructed by `NewUserset` in `internal/authz/tuple.go`
- only `group#member` actually resolves during projection
  (`internal/authz/project.go`), and a row that reached the table without passing
  the constructor cannot borrow a group's membership

### verb {#verb}

One key in the closed, dotted command vocabulary, such as
`core.command.start_print`. A verb composes with one of its arguments using
`#`, as in `core.command.start_print#path`, because argument names contain dots
and a second dot would be ambiguous.

- declared in `sdk/connectorapi/capability.go`
- many xBuddy verbs never reach the socket under the name the vocabulary gives
  them. `internal/connector/xbuddy/verbs.go` rewrites those as gcode or as
  `SET_VALUE`

### vocabulary {#vocabulary}

A closed set this codebase's behaviour depends on. Each one is enumerable from
code by an exported function returning the set in a stable order, is gated in
CI, is generated into a committed page, and is served by the API. All three,
not one of the three.

- they are error codes, extensions, capability verbs, resource kinds,
  authorization, event types, device issues, device states and device models.
  `GET /api/v1/vocabularies` is the index over them

## Go

A reader who does not write Go meets these constantly in this documentation.
Every one is a language or toolchain concept rather than something this project
invented, so the Go specification and `go.dev` are where a fuller answer is.

### build tag {#build-tag}

A comment at the top of a Go file that decides whether the compiler includes it
at all, based on flags passed to `go build`. It is how one source tree produces a
minimal binary and a full one without a runtime switch.

- written `//go:build sqlite`
- in this project a build tag is reached only through
  [the airlock](#the-airlock), never named in a configuration file

### cgo {#cgo}

The mechanism by which Go code calls C code. It requires a C compiler at build
time and it costs static linking, which is why this project's SQLite driver is
`modernc.org/sqlite`, a pure-Go implementation, rather than the more common
C-backed one.

- `internal/arch/layout_test.go` refuses a production build that reaches a
  cgo-only driver
- the [race detector](#race-detector) needs cgo, which is why `make test-race`
  runs in a container on a machine with no C compiler

### channel {#channel}

A typed, optionally buffered queue that Go [goroutines](#goroutine) use to hand
values to each other. Sending on a full channel blocks the sender, which is
exactly what the bounded queues in this codebase exist to avoid on the printer
frame path.

- the language reference is [Go's spec on channel types](https://go.dev/ref/spec#Channel_types)
- a nil channel blocks forever, which this codebase uses deliberately: a `select`
  arm reading a nil channel is simply never chosen, so an absent feature needs no
  branch (`internal/app/devicepresence.go`)

### context {#context}

The standard-library value carried through a call chain that says when to give
up: a deadline, a cancellation signal, and request-scoped values. Cancelling one
is how a shutdown starts here.

- the package is [`context`](https://pkg.go.dev/context)
- a context only stops work that reads it. A third-party method that ignores the
  one it is given blocks its caller, which is the limit
  `internal/extension/supervisor/instance.go` names

### go:embed {#go-embed}

A compiler directive that reads a file from the source tree at build time and
makes its bytes a variable in the binary. It is how this project ships one static
binary with the frontend, the firmware matrix and the SQL migrations inside it.

- written `//go:embed profiles.json` above a variable declaration
- used at `internal/printer/profile/builtin_all.go`, `internal/authz/schema.go` and
  `internal/transport/devtools/page.go`, among others

### goroutine {#goroutine}

A function running concurrently with everything else in the process, scheduled by
the Go runtime rather than by the operating system. They are cheap enough that
this codebase gives one to each printer's pump, each subscriber's pump and each
sink's drain.

- started with the keyword `go`
- a panic on a goroutine takes the whole process down unless that same goroutine
  recovered it, which is the second of the two limits at
  `internal/extension/supervisor/instance.go`

### interface {#interface}

A named set of method signatures. A Go type satisfies an interface by having
those methods, with no declaration that it does, which is what lets `sdk/`
describe what an extension must do without either side importing the other's
concrete types.

- the house rule is that an interface is declared where it is consumed, not
  where it is implemented. `internal/app/connectors.go` and
  `internal/app/stream.go` are both written that way

### module {#module}

The unit a Go project is versioned and depended on as, declared by a `go.mod`
file. This repository holds several. The two that matter most are the
application and `sdk/`, which is separate so that an extension author inherits
the standard library and nothing else.

- a module is also what a [namespace](#namespace) is derived from

### race detector {#race-detector}

A build mode that instruments memory access and reports two goroutines touching
the same variable without synchronisation. It finds real defects that no amount
of reading finds, and it needs [cgo](#cgo).

- run by `make test-race`, in a container where the machine has no C compiler
- a skip there is worse than a failure, because a skipped run prints `ok`

### struct tag {#struct-tag}

A string literal after a struct field that libraries read by reflection to
decide how the field is encoded. This project's configuration types carry
`yaml:"..."` tags, and its wire types carry `json:"..."` tags.

- `internal/app/config/config.go` declares the site key as

  ```go
  Site string `yaml:"site"`
  ```

- config keys are exempt from the [vocabulary](#vocabulary) rule for this
  reason: the field, its tag and its doc comment are already the descriptor

### table-driven test {#table-driven-test}

The standard Go test shape: a slice of cases, each a struct of inputs and an
expected result, run in a loop as named subtests. Adding a case is one line, and
the failure names the case rather than a line number.

- used throughout, for example
  `internal/printer/profile/coverage_test.go`

## Outside standards

These are published elsewhere and searching the wider internet answers a question
about them. What each entry adds is the part this project actually uses, and
where it deliberately departs.

### AsyncAPI {#asyncapi}

A specification for describing message-driven APIs, the counterpart to
[OpenAPI](#openapi) for streams and topics rather than request and response.

- [asyncapi.com](https://www.asyncapi.com/)
- this project carries one document for the event stream and one for MQTT.
  Both are frozen captures of the shipping server, and no generator writes
  either one

### CloudEvents {#cloudevents}

A CNCF specification for a common event metadata envelope: `id`, `source`,
`type`, `time`, `subject`, `data` and a `specversion`.

- [cloudevents.io](https://cloudevents.io/)
- this project's [envelope](#envelope) matches version 1.0 by attribute name and
  uses no CloudEvents library, because the attribute set is short enough to write
  out and the library would pull two dependencies into a binary that has neither

### DNS-1123 {#dns-1123}

The label grammar from RFC 1123 that Kubernetes made the common name for: at
most 63 characters, lowercase alphanumerics and hyphens, starting and ending
alphanumeric.

- validated by `ref.ValidateID` in `internal/kernel/ref/ref.go`
- every id in this system obeys it, which is what makes one string legal as an
  authorization key, a URL segment and an event subject at once. A printer id is
  a random UUID rather than the reported serial, and its canonical form obeys it

### mbedTLS {#mbedtls}

A small TLS library for embedded systems. Prusa's xBuddy firmware links the 2.x
branch of it, and its behaviour is a constraint this server cannot change.

- [github.com/Mbed-TLS/mbedtls](https://github.com/Mbed-TLS/mbedtls)
- it compares the configured hostname against DNS entries in a certificate only,
  so a certificate carrying an IP address with no matching DNS name fails every
  IP-configured printer at once. `internal/connector/xbuddy/listener.go` warns about it at
  startup
- it offers one cipher suite this server can meet, which is why the
  device-facing listener speaks TLS 1.2 while the browser-facing one is held to
  TLS 1.3

### OAuth 2.1 and PKCE {#oauth-2-1-and-pkce}

OAuth 2.1 is the consolidated revision of the OAuth 2.0 authorization framework.
PKCE, Proof Key for Code Exchange, is the extension that makes the
authorization-code flow safe for a client that cannot keep a secret: the client
sends a hash of a random value up front and the original value at the exchange.

- [oauth.net/2.1](https://oauth.net/2.1/), [RFC 7636](https://www.rfc-editor.org/rfc/rfc7636)
- implemented in `internal/platform/auth/oidc`, against an OpenID Connect issuer
  discovered at startup. A configured issuer that cannot be reached refuses the
  start rather than refusing every login afterwards

### OpenAPI {#openapi}

A specification for describing HTTP APIs as a machine-readable document, from
which clients, documentation and validation can be generated.

- [openapis.org](https://www.openapis.org/)
- the committed document is
  `internal/transport/http/httpapi/openapi.json`, and the router constructor
  refuses to return a server whose routes and document disagree

### RFC 9457 {#rfc-9457}

The IETF standard for a machine-readable HTTP error body,
`application/problem+json`. It defines `type`, `title`, `status`, `detail` and
`instance`, and permits extension members that a consumer which does not know
them must ignore.

- [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457)
- a [fault](#fault) renders as one. `code`, `params`, `retryable` and `trace_id`
  are this project's extension members

### semantic versioning {#semantic-versioning}

The `MAJOR.MINOR.PATCH` convention, with rules for what may change at each level
and for how two versions compare. Rule 10 puts build metadata after a `+` and
excludes it from precedence; rule 11 defines that precedence, including how a
prerelease tag after a `-` sorts below the release it precedes.

- [semver.org](https://semver.org/)
- firmware versions are compared by rule 11 in
  `internal/printer/profile/version.go`, which is the one comparison this project
  allows
- an extension's `Version` is deliberately **not** semver: it is a plain triple,
  because nothing here orders two extension versions beyond asking whether they
  are compatible (`sdk/extension/descriptor.go`)

### server-sent events {#server-sent-events}

A one-way streaming format over ordinary HTTP, part of the HTML standard. The
server writes `data:` lines with a blank line between events; the browser's
`EventSource` reconnects on its own and replays its last id in a
`Last-Event-ID` header with no client code involved.

- [the HTML specification](https://html.spec.whatwg.org/multipage/server-sent-events.html)
- served at `GET /api/v1/events` by
  `internal/transport/http/httpapi/events.go`. The id it writes is a
  [position](#position) or a whole [cursor](#cursor)

### Zanzibar and ReBAC {#zanzibar-and-rebac}

ReBAC, relationship-based access control, decides permission from the graph of
relationships between subjects and objects rather than from roles attached to
users. Zanzibar is Google's 2019 paper describing the system that made it
practical at scale, and it is where `object#relation@subject` comes from.

- [the Zanzibar paper](https://research.google/pubs/pub48190/)
- this project takes the tuple form, [userset](#userset) subjects, the
  denormalized index and a single consistency counter. It does not take the
  distributed store, the watch API, the new-enemy protocol, or rewrite rules as
  configuration. What is taken and what is not is tabulated in
  `docs/src/content/docs/internals/authorization.md`
