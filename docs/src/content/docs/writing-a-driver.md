---
title: Writing a printer driver
description: From an empty directory to a driver that passes the published conformance suite, with every command in the page executed.
audience: extend
rewrite:
  describes: current
  implementedBy:
    - sdk/connectorapi
    - cmd/scaffold-driver
---

A printer driver declares what a printer can do and translates one command into whatever that printer speaks. Nobody here can maintain a hundred of them, so the contract, the scaffold and the conformance suite are published and the driver is yours to own. This page is the whole path.

Every command below was run in the order it appears, and the output is what it printed.

## Links

| what | where |
| --- | --- |
| the contract a driver implements | `sdk/connectorapi` |
| the conformance suite | `sdk/connectorapi/connectorapitest` |
| the reference driver, inbound | `adapters/connector/echo` |
| what the host promises in return | [the connector contract](./connector-contract.md) |
| the capability keys to declare | [the capability vocabulary](./capabilities.md) |
| every registered extension in this build | [extensions](./extensions.md) |

## Commands

| command | does |
| --- | --- |
| `go run ./cmd/scaffold-driver -name <name> -module <path> -dir <dir>` | write a driver skeleton, its descriptor, its conformance test and its build configuration |
| `make scaffold-driver NAME=<name> MODULE=<path>` | the same, writing into `../<name>` |
| `go test ./...` in the driver directory | run the published conformance suite |
| `make compose-build CONFIG=<dir>/build.yaml` | build a server binary carrying this driver, from the file the scaffold wrote |
| `go run ./cmd/generate-capabilities` | rebuild the capability vocabulary page from the profile data |

## Key files

| path | purpose |
| --- | --- |
| `sdk/connectorapi/driver.go` | `Driver`, `Session`, `Command`, `Result`, `StatePatch` |
| `sdk/connectorapi/direction.go` | `Outbound`, `Inbound`, `Arrivals` |
| `sdk/connectorapi/set.go` | the capability set and its intersection |
| `sdk/connectorapi/connectorapitest/contract.go` | every clause the suite asserts |
| `cmd/scaffold-driver/templates/` | what the scaffold writes |
| `cmd/compose-build/main.go` | the tool that reads a `build.yaml` and builds the binary |
| `test/compose/build.yaml` | a working build configuration, for a module this repository does not compile |

## What the toolchain needs

| requirement | why |
| --- | --- |
| Go 1.26.11, the version `sdk/go.mod` declares | the scaffold copies it into the driver's `go.mod` |
| a checkout of `prusa-connect` | the sdk module carries no published version. `go get github.com/prusa3d/prusa-connect/sdk` resolves nothing, so the scaffold writes a `replace` pointing at the `sdk` directory of a checkout |

The version above is checked, not remembered. `scripts/check-go-version.sh` compares this page, `go.mod` and `sdk/go.mod`, and fails when one of the three says something different.

No hardware. The suite stands the driver up against a fake device it starts itself.

## Scaffold

Run this from the checkout. `-name` is what an operator writes in configuration and has to be a DNS-1123 label, because the same string has to be legal as a config key, a metric label, a URL segment and a messaging subject at once.

`-module` has to be a path the host can turn into a namespace, which means every element of it is a DNS-1123 label too. That rules out an uppercase organisation (`github.com/Acme`, `github.com/Masterminds`), an underscore, and the version element of a `gopkg.in` path. Nothing is rewritten to fit, so the flag refuses a path rather than the operator's binary refusing it at start-up:

```console
$ go run ./cmd/scaffold-driver -name acme -module github.com/Acme/prusa-driver-acme
scaffold-driver: cannot scaffold a driver: -module "github.com/Acme/prusa-driver-acme": extension: cannot derive a namespace: module "github.com/Acme/prusa-driver-acme" is not one, because "Acme" is not a DNS-1123 label

$ go run ./cmd/scaffold-driver -name acme -module gopkg.in/prusa-driver-acme.v1
scaffold-driver: cannot scaffold a driver: -module "gopkg.in/prusa-driver-acme.v1": extension: cannot derive a namespace: module "gopkg.in/prusa-driver-acme.v1" is not one, because "prusa-driver-acme.v1" is not a DNS-1123 label
```

The answer to either is to rename the module. Lowercasing a path in the host would put `example.com/a_b`, `example.com/a-b` and `example.com/A-B` in one collision domain.

```console
$ go run ./cmd/scaffold-driver -name acme -module example.com/prusa-driver-acme -dir ../prusa-driver-acme
OK ..\prusa-driver-acme\CHANGELOG.md
OK ..\prusa-driver-acme\README.md
OK ..\prusa-driver-acme\acme.go
OK ..\prusa-driver-acme\acme_test.go
OK ..\prusa-driver-acme\build.yaml
OK ..\prusa-driver-acme\faults.go
OK ..\prusa-driver-acme\go.mod

Next: cd ../prusa-driver-acme && go test ./...
```

Seven files, and nothing to delete. `-kind` writes an eighth, and the scaffold writes no others.

| file | holds | written |
| --- | --- | --- |
| `go.mod` | the module, and the `replace` that points at the checkout | always |
| `acme.go` | the driver: its descriptor, its `Baseline`, its `Connect`, and one session | always |
| `acme_test.go` | `connectorapitest.RunDriverContract`, and the fake device it runs against | always |
| `build.yaml` | the core and the adapter modules a binary is built from. It is the whole of how this driver gets into a server | always |
| `faults.go` | the codes this driver fails with, each registered with its documentation | always |
| `README.md` | the driver's own page | always |
| `CHANGELOG.md` | the driver's release notes | always |
| `kind.go` | the device kind this module declares, and the verbs a grant is checked against | with `-kind` |

A printer driver declares no kind: its devices are printers, and the printer verb table is already in the core. `-kind` is for a machine that is not a printer, such as a filament shelf:

```console
$ go run ./cmd/scaffold-driver -name shelfy -module example.com/prusa-driver-shelfy -kind shelf -dir ../prusa-driver-shelfy
OK ..\prusa-driver-shelfy\CHANGELOG.md
OK ..\prusa-driver-shelfy\README.md
OK ..\prusa-driver-shelfy\build.yaml
OK ..\prusa-driver-shelfy\faults.go
OK ..\prusa-driver-shelfy\go.mod
OK ..\prusa-driver-shelfy\kind.go
OK ..\prusa-driver-shelfy\shelfy.go
OK ..\prusa-driver-shelfy\shelfy_test.go

Next: cd ../prusa-driver-shelfy && go test ./...
```

## Run the suite

```console
$ cd ../prusa-driver-acme && ls
acme.go
acme_test.go
build.yaml
CHANGELOG.md
faults.go
go.mod
README.md

$ go test ./...
ok  	example.com/prusa-driver-acme	1.997s
```

The clauses, which is what actually got proved:

```console
$ go test -v -run TestContract ./...
=== RUN   TestContract
--- PASS: TestContract (0.01s)
    --- PASS: TestContract/baseline (0.00s)
        --- PASS: TestContract/baseline/satisfies_the_descriptor_rules (0.00s)
        --- PASS: TestContract/baseline/is_pure_and_unshared (0.00s)
    --- PASS: TestContract/outbound (0.01s)
        --- PASS: TestContract/outbound/resolution (0.00s)
            --- PASS: TestContract/outbound/resolution/a_profile_cannot_add_a_capability_the_code_does_not_have (0.00s)
            --- PASS: TestContract/outbound/resolution/a_profile_subtracts_what_it_leaves_out (0.00s)
            --- PASS: TestContract/outbound/resolution/an_empty_profile_leaves_nothing (0.00s)
        --- PASS: TestContract/outbound/session (0.00s)
            --- PASS: TestContract/outbound/session/capabilities_do_not_move_within_an_epoch (0.00s)
            --- PASS: TestContract/outbound/session/the_capability_set_handed_out_is_not_the_one_in_force (0.00s)
            --- PASS: TestContract/outbound/session/identity_reports_a_model (0.00s)
            --- PASS: TestContract/outbound/session/every_declared_verb_validates_against_its_own_descriptor (0.00s)
            --- PASS: TestContract/outbound/session/a_verb_the_printer_does_not_have_is_not_carried_out (0.00s)
            --- PASS: TestContract/outbound/session/a_cancelled_command_is_not_reported_as_done (0.00s)
            --- PASS: TestContract/outbound/session/a_safe_verb_round-trips_through_Do (0.00s)
            --- SKIP: TestContract/outbound/session/a_filesystem_half_refuses_a_path_that_leaves_its_storage (0.00s)
            --- PASS: TestContract/outbound/session/close_is_idempotent_and_closes_the_state_channel (0.00s)
PASS
ok  	example.com/prusa-driver-acme	1.337s
```

`Subject.Connectable` in the generated test is what turns the second half on. Setting `Subject.NoLoopback` instead is a statement that the driver cannot be dialled in a unit test, and it turns off every assertion about the ceiling, `Do`, `State` and `Close`, which is most of the suite. Prefer standing up a fake.

## Watch it fail

A suite nobody has seen red proves nothing. Break the one rule the whole model rests on, by ignoring the profile the host resolved:

```go
resolved := d.Baseline()
```

```console
$ go test ./...
--- FAIL: TestContract (0.01s)
    --- FAIL: TestContract/outbound (0.01s)
        --- FAIL: TestContract/outbound/resolution (0.00s)
            --- FAIL: TestContract/outbound/resolution/a_profile_subtracts_what_it_leaves_out (0.00s)
                contract.go:400: core.command.send_state_info survived a profile that does not declare it
            --- FAIL: TestContract/outbound/resolution/an_empty_profile_leaves_nothing (0.00s)
                contract.go:407: a profile declaring nothing resolved to 3 capabilities
FAIL
FAIL	example.com/prusa-driver-acme	1.706s
FAIL
```

Put the `.Intersect(profile)` back.

## Replace the wire

The skeleton speaks JSON over HTTP. Three functions carry that and nothing else does.

| function | sends | replace it with |
| --- | --- | --- |
| `identify` | `GET /identity` | the vendor's handshake, however the device announces its model, firmware and serial |
| `Session.Do` | `POST /command` | one request per command, and the vendor's own reply mapped onto `connectorapi.Result` |
| `Session.stream` | `GET /state`, one JSON patch per line | whatever pushes telemetry: a websocket, an MQTT subscription, a poll loop |

Everything else in the file is the contract and stays as it is. Change the fake device in `acme_test.go` at the same time, and the suite keeps holding the driver to the same rules.

`Result.Ignored` and `Result.Substituted` are worth filling in rather than leaving empty. They are how a printer says it carried out a command differently instead of silently disagreeing with its caller, and the suite refuses a name in either that was not sent.

## Declare what the printer can do

`Baseline()` is the ceiling: everything this driver's code can do for any device of this dialect. A profile and the device's own probe may subtract from it, and neither can add to it. There is deliberately no union in `connectorapi.Set`, so the worst a wrong profile can produce is a missing capability rather than one the printer has not got.

A verb accepts exactly the arguments for which the set carries an argument key, built with `connectorapi.ArgumentKey`. Presence is the declaration: leaving a key out is how a tier subtracts one.

```go
VerbNozzleTarget: {Flag: true},
ArgNozzleTemperature: {
	Flag: true, Kind: connectorapi.KindInteger, Required: true,
	Min: 0, HasMin: true, Limit: 300, HasLimit: true, Unit: connectorapi.UnitCelsius,
},
```

The host checks the value against that descriptor before `Do` is reached, in `internal/printer/capability/session.go`, and fills in a declared default. A driver that re-checks an argument it declared is writing the branch the descriptor exists to delete.

`Unit` says what a number counts in. It is one of `celsius`, `percent`, `mm`, `mm/min`, `s` and `byte`, and a number with no unit, such as a head index, declares none. The host checks nothing against it. A client reads it, with the type and the range, from `GET /api/v1/devices/{device_id}/capabilities`, so it labels a control without reading your source.

Draw the keys from [the capability vocabulary](./capabilities.md). Nothing refuses a key that is not there, so an invented key resolves and validates like any other; what it does not do is reach a caller that knows what to send, because the API, the frontend and every profile in this tree speak the published keys.

## Name the machines the driver reaches

A page shows a device under a name and a drawing it reads from `GET /api/v1/vocabularies/device-models`. It finds the row by the model the device reports. The core has rows only for the Prusa machines its profiles cover, so a driver for any other machine declares the rows in the `Dialect` it registers:

```go
connectorapi.Register(Type, doc(), New, connectorapi.Dialect{
	Needs: connectorapi.NeedsAddress,
	Models: []connectorapi.DeviceModel{{
		Model:       "Acme_One",
		DisplayName: "Acme One",
		Vendor:      "Acme",
	}},
})
```

- `Model` is the spelling the driver reports in the device record. The web app finds a device's row the way the host's profile lookup does. It compares the folded spelling, as the host does for collisions below, and then a device type code as written.
- The host writes the connector from the name the driver registered under. A row cannot name another dialect.
- The host refuses a row whose spelling names a Prusa machine. That is a model, an alias, or the device type code of a profile the build loads, such as `8.1.0`, also as `810`. When two drivers in the build declare one spelling, it refuses both rows. Case and every character that is not an ASCII letter or digit are ignored in that comparison. The server starts either way.
- Each row left out is an `ERROR` line at start, `a declared device model was refused`. It names the model, the driver, what the row collides with and what to change. The vocabulary lists the same refusals under `refused`, so a test that runs the composed binary reads them there and checks that its own row is in `items`.
- `ToolChanger: true` makes a page draw a row per tool. It adds no command. A per-tool target is an argument the driver declares in `Baseline()`.
- `Artwork` is a drawing of the machine, in SVG, PNG or WebP, up to `connectorapi.MaxArtworkBytes`. Embed the file with `go:embed`, so it ships with the driver. The web app shows it for a model it has no drawing of its own for.
- The host serves the rows and their drawings to any caller before sign-in, so declare nothing that belongs to one deployment.
- The rows appear in a build that carries the driver, and in no published reference, so a driver can name a machine the project does not.

`dialect()` in `examples/adapters/moonraker/moonraker.go` declares the Klipper row and its drawing this way. `assertKlipperRowIsDeclared` in `test/compose/modules_test.go` is the test the fourth point describes: it reads `refused`, then the row and the drawing, out of a composed server.

## Declare the states

The word a driver writes at `/state` is its own. Declare every word in the `Dialect`, with what a person sees for it, the moves between the words, and the verbs a device accepts in each:

```go
States: connectorapi.StateSet{
	States: []connectorapi.DeviceState{
		{Name: "IDLE", DisplayName: "Idle", Accepts: []connectorapi.Capability{VerbStartPrint, VerbStatus}},
		{Name: "PRINTING", DisplayName: "Printing", Accepts: []connectorapi.Capability{VerbStatus}},
	},
	Transitions: []connectorapi.Transition{
		{From: "IDLE", To: "PRINTING", Verb: VerbStartPrint},
		{From: "PRINTING", To: "IDLE"},
	},
},
```

- A move the machine makes on its own, such as a print that ends, names no verb.
- List `Accepts` for every state or for none. A state that accepts nothing has an empty list, never a nil one. A verb that moves a device out of a state must be accepted in that state.
- Write the rule the machine keeps, never a stricter one of your own. The list is a hint for which controls a client offers, and the machine's answer to the command still decides.
- `connectorapi.Register` panics on a set `StateSet.Problems` refuses, so a bad set fails your own build.

The host serves the set at `GET /api/v1/vocabularies/device-states`, in one entry for your connector type. The entry lists every instance of your type the server runs, and a client finds a device's entry by the instance the device names under `driver`. `adapters/connector/echo/states.go` and `examples/adapters/moonraker/states.go` are two declarations to start from.

## Report the tools

A program that picks a printer for a file needs each tool's nozzle and the
filament loaded in it. A driver reports them as state patches, at the paths
`sdk/connectorapi/tools.go` names. The tools are a tree. A head is a nozzle
with its own heater, and the heads are `0`, `1` and on. A feeder, such as a
multi-material unit, sits on a tool and has slots. Slot 2 of the feeder on
head 0 is `0.2`, and a slot may carry a feeder of its own, so `3.1.0` is a
legal id. There is no fixed number of tools.

| path | value | from |
| --- | --- | --- |
| `/tool_count` | how many heads the machine has room for, enabled or not | architecture |
| `/tools/<id>/enabled` | `true`, or `false` for a tool the machine has switched off | Prusa Connect |
| `/tools/<id>/type` | `FFF_PRINT_HEAD` or `FFF_FEEDER_SLOT` | architecture |
| `/tools/<id>/active` | `true` for a head that is picked and for a slot loaded into its head | architecture |
| `/tools/<id>/features/nozzle_diameter` | the nozzle in millimetres | architecture |
| `/tools/<id>/features/nozzle_high_flow`, `nozzle_hardened` | the nozzle's kind, as booleans | architecture |
| `/tools/<id>/features/nozzle_high_temp` | a nozzle for high temperatures, as a boolean | architecture, not yet public |
| `/tools/<id>/feeder/slot_count` | the slots of the feeder on the tool, and `0` for no feeder | architecture |
| `/tools/<id>/feeder/model`, `base_model`, `type` | the feeder's model and its family, such as `MMU3`, and its kind, `MMU` | architecture |
| `/tools/<id>/feeder/mmu_progress_code`, `mmu_command_in_progress` | the step the unit is at, and the command it runs as one character | architecture |
| `/tools/<id>/material_package_instance/package/material/abbreviation` | the filament loaded, such as `PLA`, and `""` for nothing loaded | architecture |
| `/tools/<id>/nozzle_temp`, `nozzle_target_temp` | a head's reading and target, and a target of `0` while the heater is off | architecture |
| `/tools/<id>/odometer_id`, `odometer_e`, `odometer_weight` | a number that changes at each reset of the odometers, and the filament extruded since, in millimetres and in grams | architecture |
| `/gcode_tool_map/<n>` | the tool id a G-code `T<n>` selects, when the machine says | Prusa Connect |
| `/build_volume/x`, `y`, `z` | the space a print may fill, in millimetres | Prusa Connect |

The names marked architecture are those of the public Prusa architecture's
[printer configuration](https://specs.prusa3d.tech/architecture/#/printer_configuration).
Its public pages do not list `nozzle_high_temp` yet, and no Prusa firmware
sends it. Four of its values are `null`, which a state patch cannot hold,
because a null removes a path. A disabled tool is `enabled` false, no feeder
is `slot_count` 0, nothing loaded is `""`, and a heater that is off is a
`nozzle_target_temp` of 0. So `enabled` is how the tree holds a null, and the
architecture has no such member.

More than one tool can be `active` at once. On a machine with an MMU, head
`0` is always active and so is the slot loaded into it. On a machine with a
feeder on each head, a slot loaded into a head that is not picked is active
too. Write `active` for each tool your machine says it for. A reader that
needs the tool printing now derives it with `connectorapi.ActiveToolOf`: the
deepest active tool whose tools above it are all active.

The tree is the only place a tool lives. Write no path for the nozzle or the
material outside it, and no path naming the active tool. A machine with one
nozzle is head `0`: its temperature is `/tools/0/nozzle_temp`.

The paths marked Prusa Connect are this project's own. A client that reads a
Prusa printer or Prusa Connect does not find them there.
`/gcode_tool_map/<n>` is not the `tool_mapping` argument of a print command.
That argument tells the printer which tool to use for each filament, and the
path says which tool a G-code `T<n>` selects.

The driver normalizes. Your machine numbers its tools, names an empty tool and
changes between firmware versions in its own way, and only the driver knows
how. Convert all of it where you read the frame, so nothing past the driver
branches on your vendor. The Prusa firmware keys its filament paths from 1 and
writes `---` for an empty one. On an XL each path is a head, so its path 1 is
head `0`. In front of an MMU each path is a slot, so its path 1 is slot `0.0`.

A file names its filaments `T0`, `T1` and on. When the driver reports no
`/gcode_tool_map`, a reader takes the default. On a machine with one head that
carries a feeder, `T<n>` is slot `0.<n>`. On any other machine it is head
`<n>`. That includes a machine with a feeder on more than one head, so such
a driver writes a mapping whenever the machine tells it one.

Write only what the machine says. A path left out is unknown, and the host
never reads it as zero, false or nothing loaded. When the machine stops listing
a tool, write `null` to its paths. The host keeps nothing of a removal of a path
it does not hold, so removing a tool that was never there costs nothing.
Report the space a print may fill at `/build_volume/x`, `/build_volume/y` and
`/build_volume/z`, in millimetres. The host keeps no table of machines, so
the driver carries the figures for the models it knows, with the public
source of each. A Klipper machine is defined by its own configuration, and
the Moonraker driver reads the travel of each axis from it.

## Describe the readings

A reading is one measured value of the whole machine, such as the bed or the
chamber temperature. A value of one tool stays in the tool tree. A client
learns from the state which readings a machine has. So describe each reading
that the driver writes, under `/readings/<id>/`. The value stays at its own
path.

| path | value |
| --- | --- |
| `/readings/<id>/kind` | `temperature` |
| `/readings/<id>/unit` | `celsius` |
| `/readings/<id>/value_path` | the state path of the value, such as `/temperature/bed` |
| `/readings/<id>/target_path` | the state path of the target, when the machine has one |
| `/readings/<id>/name` | what the machine or its owner calls the reading, at most 64 bytes |
| `/readings/<id>/role` | `bed` or `chamber`. One device has at most one reading per role |

The id is 1 to 64 characters from `a` to `z`, `0` to `9` and `_`, and it
starts with a letter. Do not type the paths in the driver. Build a
`connectorapi.Reading` and send its `Patches`:

```go
bed := connectorapi.Reading{
	ID:         "bed",
	Kind:       connectorapi.ReadingTemperature,
	Unit:       connectorapi.UnitCelsius,
	ValuePath:  "/temperature/bed",
	TargetPath: "/temperature/bed_target",
	Role:       connectorapi.RoleBed,
}

for _, patch := range bed.Patches() {
	report(patch)
}
```

Send the description once per session, and again only when it changes. The
host publishes an event for every patch, also for a value that did not change.
When the machine loses a reading, send the patches of its `Removal`. Check
each reading with `Reading.Problems` in a test. A reader builds the readings
with `connectorapi.ReadReadings`.

## The name an operator writes

A driver is the main module of its own test binary, so everything it registers there lands in namespace `core`. Installed in a host it is not the main module, and its namespace is its module path with the host reversed and the label `aux` where the host ends. A module path that contains the label `aux` is refused. The label keeps `github.com/acme/mk4` and `acme.github.com/mk4` in two collision domains rather than one.

| module path | namespace | what an operator writes |
| --- | --- | --- |
| `example.com/prusa-driver-acme` | `com.example.aux.prusa-driver-acme` | `type: acme`, and `type: com.example.aux.prusa-driver-acme/acme` once another module has registered `acme` too |

The generated `acme_test.go` carries the qualified form as `Qualified` and fails when the module path stops rendering as it, so the name a driver's README quotes is the name a host answers to and not the `core/acme` its own test binary reports.

## Register it into a server

The driver registers itself from `init`, which the scaffold already wrote. A build reaches it through a blank import, and the namespace comes from the registering module rather than from anything the author passes, so two authors cannot land on one name.

You do not write that blank import. The `build.yaml` the scaffold wrote names the core and this module. `compose-build` generates a throwaway module of blank imports around the two and builds it. Run it from the core checkout. It prints three lines. These are the last two, and the first names a scratch directory the tool discards:

```console
$ make compose-build CONFIG=../prusa-driver-acme/build.yaml
Building with tags: full
Wrote C:\src\prusa-connect\bin\server-acme
```

That is the whole admission. Nothing in the core changes: no blank import, no adoption list entry, no build tag and no row in `adapters/tags.json`. A binary built from a configuration that does not name the module has never heard of `acme`. `test/compose/build.yaml` is the same file for a module this repository does not compile, and `test/compose/gate_test.go` builds it and reads the driver back out of the running server.

A driver contributed to this repository takes the other route. Add the blank import beside the others in `internal/app/app.go`, and a `replace` in the root `go.mod` while the driver is unpublished.

An inbound driver is mounted at `/connectors/<name>/` whichever route it took, and the host strips that prefix before the driver's handler sees a request. `internal/app/connectors.go` is the whole of that wiring.

## What is not built yet

Stated plainly, because an author will otherwise find it the hard way.

| gap | where | what it means |
| --- | --- | --- |
| the sdk module is unpublished | no tag answers `go get github.com/prusa3d/prusa-connect/sdk` | a driver is built against a checkout, which the scaffold's `replace` does |
| a driver ships no profiles of its own | `internal/app/app.go` builds one resolver over `profile.Builtin()`, the profiles for Prusa hardware | a device those profiles do not cover is held to the union of them, so a key no Prusa profile declares is subtracted from a third-party driver's baseline at runtime |
| out of process, the catalogue, signing | not built | the wire and its host are built, and no configured instance reaches them. A driver today is compiled in |

## What to read next

| document | for |
| --- | --- |
| [the connector contract](./connector-contract.md) | what the host promises, what it does not, and what a driver must never do |
| [the capability vocabulary](./capabilities.md) | every key the shipped profiles declare, with its argument shapes |
| `adapters/connector/echo` | the inbound shape, written to be copied |
