---
title: MQTT overview
slug: mqtt
description: How the core publishes to a broker you run through an event sink, the frozen v1 topic namespace, and the routing rules.
audience: run
rewrite:
  describes: current
  implementedBy:
    - adapters/events/mqtt
---

:::note[Four things here are a frozen contract]
Each of these is client-visible and does not change:

- the `v1/` topic namespace
- the QoS and retain choices
- the change-only publishing rule
- the routing-rule semantics

The server has no broker inside it. It sends to one you already run.
:::

MQTT carries printer state and telemetry out to whatever subscribes: a home
automation system, a dashboard, an IoT pipeline. The topics are the Prusa
Connect naming convention, so an existing client keeps working.

The complete topic list, payload shapes, QoS and retain flags are in the
generated [MQTT topic reference](./mqtt-asyncapi.mdx).

## How the core publishes

The core has no broker in it. `adapters/events/mqtt` is an event sink: it
subscribes to the event bus and publishes to a broker you already run. Declare
it as a resource instance and it starts with the server.

It does not wait for the broker. A sink that refused to start because a broker
was down would turn an optional integration into a boot dependency. So the
connection is retried in the background for as long as the process runs.

Its options, their types and their defaults are published in the generated
[extensions reference](./extensions.md#core-mqtt), which is read from the
registry, so it cannot drift. The ones worth knowing before you read it:

| Option | Default | Does |
|---|---|---|
| `url` | required | the broker, as `tcp://host:port` or `ssl://host:port` |
| `queue_depth` | `256` | how many messages may wait for a slow broker before the oldest are dropped |
| `routes` | none | rewrite and filter rules, applied in order, first match wins |
| `default_action` | `forward` | what happens to a topic no route matched: `forward` or `drop` |

## Topic model

The namespace has three roots:

| root | carries |
|---|---|
| `v1/server/online` | server availability |
| `v1/devices/printers/{uuid}/...` | the frozen per-printer and per-job topics |
| `v1/events/{origin}/{event type}` | each durable event of the server, as its JSON envelope |

The [MQTT topic reference](./mqtt-asyncapi.mdx) lists every topic of the
frozen namespace. The sink publishes only some of them today, and the next
section names which.

## What is published, and when

A value is published when it changes and not on a timer.

| topic | QoS | retained | published |
|---|---|---|---|
| `v1/server/online` | 1 | yes | `1` when the server connects to the broker, `0` when it leaves |
| `v1/devices/printers/{uuid}/data/state` | 1 | yes | when the printer's state changes |
| `v1/devices/printers/{uuid}/data/online` | 1 | yes | `0` when the printer is deleted |
| `v1/devices/printers/{uuid}/data/current-job` | 1 | yes | the job the printer works on, and `0` when the job ends |
| `v1/devices/printers/{uuid}/jobs/{job_id}/data/state` | 1 | yes | when a job the printer works on changes state |
| `v1/devices/printers/{uuid}/data/tools/{tool_id}/active` | 1 | yes | `1` or `0` when the tool becomes active or stops being active, empty when not known |
| `v1/devices/printers/{uuid}/data/tools/{tool_id}/material` | 1 | yes | when the material in the tool changes, empty when nothing is loaded or not known |
| `v1/devices/printers/{uuid}/data/tools/{tool_id}/temp/nozzle/current` | 0 | no | when a head's nozzle moves by a whole degree |
| `v1/devices/printers/{uuid}/data/tools/{tool_id}/temp/nozzle/target` | 1 | no | when a head's nozzle target changes, `0` when the heater is off |
| `v1/devices/printers/{uuid}/data/temp/heatbed/current` | 0 | no | when the bed moves by a whole degree |
| `v1/devices/printers/{uuid}/data/temp/heatbed/target` | 1 | no | when the bed target changes, `0` when the heater is off |
| `v1/devices/printers/{uuid}/data/temp/chamber/current` | 0 | no | when the chamber moves by a whole degree, on a printer that reports a chamber |
| `v1/events/{origin}/{event type}` | 1 | no | once for each durable event |

The other topics of the frozen namespace are not published. They are the axis
positions and the speed. They are also the job progress, the time remaining,
the dialog and the blocking reasons. The chamber target has no topic. A client
reads those values from the HTTP API or from the event stream.

## Tool topics

The tool topics carry the printer's tool tree, which is where the core keeps
every head and every feeder slot. `{tool_id}` is the id the tree gives the
tool, so it counts from 0:

| tool | `{tool_id}` | topics |
|---|---|---|
| the one head of an MK4S or a CORE One | `0` | all four |
| the third head of an XL | `2` | all four |
| slot 2 of the MMU in front of head 0 | `0.2` | `active` and `material` |

A feeder slot has no nozzle of its own. It prints through the nozzle of its
head, so it has no temperature topics.

The material is the short name, such as `PLA`. The nozzle target is `0`
while the heater is off, because a target of 0 °C is what an off heater
means.

A value that is not known has an empty payload, a message of zero bytes. The
sink never publishes the text `null`. A value is not known in these cases:

- the printer stopped reporting it
- the tool is switched off, such as a head disabled on an XL
- the tool left the tree, such as the slots of an MMU that is switched off
- the tool has nothing loaded. Its material is then empty too

On a retained topic, an empty payload also deletes the retained message. So
a subscriber that joins later reads nothing on that topic, and not an old
value. A subscriber reads an empty payload as a value that is not known.

The sink needs a grant on the printer, like every reader. When it can first
read a printer, it publishes every tool topic once, so the retained topics
hold a value before anything changes. At that moment it also clears the
topics of each tool it published before that is no longer in the tree.
Retained topics that a server left on the broker before a restart are not
cleared unless the printer reports that tool as switched off.

The printer-wide `data/material` and `data/temp/nozzle/...` topics, and the
`data/slot/...` topics, are not published. They named one tool for the whole
printer, or counted slots from a different base, and the tool topics replace
them.

QoS and retain are not tuning options. A subscriber that joined an hour ago
sees a retained topic at once, and a non-retained one only on the next change.
Moving either changes what an installed client observes.

## Routing rules

Routing rewrites or drops a topic before it reaches the broker. Three cases
want it:

- the broker uses a topic schema other than the `v1/devices/...` namespace
- only some topics should reach the broker, which is an allowlist
- one high-frequency topic should be silenced and the rest forwarded

Each rule has three fields:

| Field | Required | Description |
|---|---|---|
| `from` | yes | pattern matching the full published topic, with the wildcards below |
| `to` | no | target topic template. A `{name}` captured by `from` is substituted. Empty forwards without renaming |
| `action` | no | `forward`, the default, or `drop` |

Rules are evaluated in order and the first match wins, so a later rule matching
the same topic is never reached. A topic matching no rule takes
`default_action`. That ordering is the contract: an operator's rule set was
written against it, and evaluating in any other order silently reroutes an
installed subscriber.

Routing decisions are logged at `debug` level, and a rule an earlier rule
shadows is warned about at start-up.

### Wildcards

Each token occupies one whole path segment.

| Token | Matches | May appear in `to` |
|---|---|---|
| `{name}` | exactly one segment, captured under that name | yes, as `{name}` |
| `+` | exactly one segment, not captured | no |
| `#` | one or more trailing segments, and must be last | no |

Neither `{name}` nor `+` matches a `/`. Only `#` spans more than one segment.
Use `{name}` when the captured value goes into `to`, and `+` or `#` when the
segment's value does not matter.

### Depth, which is where rules go wrong

A single `{name}` or `+` reaches one level below `data/`, and this project's
topics vary in depth.

| Published topic | Depth after `data/` | `data/{metric}` | `data/#` |
|---|---|---|---|
| `data/state` | 1 | yes | yes |
| `data/current-job` | 1 | yes | yes |
| `data/temp/heatbed/current` | 3 | no | yes |
| `data/tools/0/temp/nozzle/current` | 5 | no | yes |

A topic of the frozen namespace the sink does not publish yet is in the table
because a rule written today still meets it once it is published.

Job topics hang off `jobs/{jobId}/data/...` and no `data/#` rule ever matches
one.

### Rewriting into another namespace

```yaml
routes:
  - from: "v1/devices/printers/{id}/data/state"
    to:   "printers/{id}/state"
  - from: "v1/devices/printers/{id}/data/current-job"
    to:   "printers/{id}/job"
  # {metric} takes single-segment topics only: state, online, current-job
  - from: "v1/devices/printers/{id}/data/{metric}"
    to:   "printers/{id}/{metric}"
```

### Forwarding only what is listed

`default_action: drop` discards everything no rule names. `#` allows a whole
subtree without listing each leaf.

```yaml
default_action: "drop"
routes:
  - from: "v1/devices/printers/+/data/#"
  - from: "v1/devices/printers/+/jobs/#"
  - from: "v1/server/online"
```

### Dropping one noisy subtree

```yaml
routes:
  - from: "v1/events/printer/+/#"
    action: "drop"
  - from: "v1/events/#"
```

This drops the events of every printer and forwards the events of anything
else.

Order the specific rule before the general one. Reversed, the `#` rule matches
first and the drop rule is never reached.
