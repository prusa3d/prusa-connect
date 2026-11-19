---
title: "ADR-20261119-zha: A device describes its readings in its state"
description: "A driver describes each device-wide reading of a machine under /readings/ in that machine's state, and a client shows the readings it finds there."
date: 2026-11-19
status: proposed
supersedes: []
amends: []
tags:
  - adr
  - printer
  - telemetry
---
# ADR-20261119-zha: A device describes its readings in its state

<!-- nav:related -->
> [!NOTE]
> Index: [decisions](README.md)

The web app knows the bed and the chamber from a list in its own code. A
machine with another sensor cannot show it, and the app infers a chamber from
the data that arrives. A Klipper machine names its heaters and sensors in a
configuration file, so the set is known only while the machine is connected.

## rules

1. A reading is one device-wide measured value of a machine, such as the bed
   or the chamber temperature. A value that belongs to one tool stays in the
   tool tree.
2. A driver describes a reading in the state of the device that has it, under
   `/readings/<id>/`. The members are `kind`, `unit`, `value_path`, and the
   optional `name`, `role` and `target_path`.
3. `<id>` is 1 to 64 characters from `a` to `z`, `0` to `9` and `_`, and it
   starts with a letter. It is stable for one device.
4. `kind` is `temperature` and `unit` is `celsius`. A reader skips a reading
   whose kind or unit it does not know.
5. `value_path` and `target_path` are state paths of the same device. The
   value is not copied under `/readings/`. A path a client or a sink reads
   today does not move.
6. `role` is `bed` or `chamber`, and one device has at most one reading per
   role. A client translates the label of a reading with a role. For any
   other reading it shows `name`.
7. A device has a reading while the three required members are in its state.
   A driver removes the members when the machine loses the reading.
8. `connectorapi.ReadReadings` builds the readings from a state, and
   `Reading.Patches` writes one. A reader and a driver use them and do not
   spell the paths.
9. The web app holds no list of readings. The chart, the printer card and the
   history offer the readings of the device shown.
10. A sink with a fixed topic for a role, such as the MQTT bed topic, selects
    a reading by its role.

## enforcement

- `sdk/connectorapi/readings_test.go` holds the grammar of rules 2 to 7.
- Each driver's tests assert that the paths it describes are paths it writes.
- `packages/svelte` tests assert that a described reading is drawn and that a
  device without one draws none.

## rejected

| option | why not |
|---|---|
| The driver declares its readings in its dialect, beside its states | A dialect is fixed at build time. A sensor a person names on a Klipper machine could never appear. |
| A session method that returns the readings | Every reader of a device already receives its state. A second channel needs its own events and storage. |
| Move each value under `/readings/<id>/value` | Clients, sinks and stored history read the current paths. A move breaks them for no gain. |
| A fixed list with more entries | Each new reading still changes the web app, and a third-party driver cannot add one. |
| Infer the kind from the path, as Klipper clients do from a section name | Every client then repeats the guess, and a unit is never stated. |

## open

- A reading a module describes, not a driver.
- Kinds other than temperature: humidity, pressure, fan speed.
- A bound on the number of readings one device may describe.
