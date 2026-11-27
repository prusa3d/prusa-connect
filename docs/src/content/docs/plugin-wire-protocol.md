---
title: The plugin wire protocol
description: The bytes an out-of-process extension and this host exchange, specified so that an implementation in Python, Rust or TypeScript needs no Go.
audience: extend
rewrite:
  describes: current
  implementedBy:
    - sdk/wire
    - sdk/wire/wiretest
    - sdk/wirerpc
    - internal/extension/wirehost
    - cmd/generate-wire-protocol
    - examples/wire-protocol
---

This page is the contract. An extension that speaks it runs as its own process, in its own language, and the host never links its code.

Read it end to end and you can write the wire in an afternoon. That is the point of the choice: LSP and MCP both have large communities and neither generates bindings.

`sdk/wire` is one implementation of this page. It is not the definition. Where the two disagree, this page and the vectors it publishes are right and the Go is a defect.

## What is built today

| part | state |
| --- | --- |
| the frame, the handshake and version negotiation | specified here, implemented in `sdk/wire`, held by 39 vectors |
| the vectors, published as a file any language reads | [`docs/wire/plugin-wire-protocol.json`](https://github.com/prusa3d/prusa-connect/blob/main/docs/wire/plugin-wire-protocol.json) |
| a host that launches a child and speaks this wire | `internal/extension/wirehost` |
| the session above the frame: streams, JSON-RPC and credit | `sdk/wirerpc`, used by both sides |
| credit accounting across a stream | a balance per stream, in `sdk/wirerpc`. A write waits for credit and a sender past its balance ends the connection |
| a `runtime: service` instance reaching that host | built. The server launches a `runtime: service` instance as a child process |
| a contract's own method set: a printer driver, an event sink | **not built.** The lifecycle set below is the only one published |

The last row is not built yet. A driver written against this page can talk to a host today. It has no driver contract to implement yet, and the page says so, because a reader deserves that before writing code.

## The shape in one paragraph

The host launches a child process. The child prints one JSON line on its standard output saying which version it speaks and where the host reaches it. Everything after that is frames on one duplex channel. A frame carries a stream id, so many conversations share the channel. Control frames carry JSON-RPC 2.0. Bulk bytes travel in their own frame kind and never inside JSON.

## The channel

The host decides what the channel is and hands it to the child.

| system | channel | why |
| --- | --- | --- |
| Linux, macOS | the standard input and output pair the host already holds | the same wire as on Windows, with no directory for the host to own |
| Windows | the standard input and output pair the host already holds | measured 3 to 4 times faster there than a local socket |

The wire is the same on both. A child reads frames from one stream of bytes and writes frames to another, and it never needs to know which channel it was given.

Standard error is logs. The host reads it line by line and copies it into its own log with the instance name bound. A child that writes its logs to standard output corrupts the protocol.

## The handshake

The child writes exactly one line to standard output before its first frame:

```json
{"protocol":1,"min_minor":0,"max_minor":3,"address":"stdio"}
```

The line ends with a single newline, `0x0A`. It is UTF-8 and holds at most 4096 bytes, the newline counted.

| member | type | meaning |
| --- | --- | --- |
| `protocol` | integer above zero | the wire major the child implements |
| `min_minor` | integer | the oldest minor the child implements |
| `max_minor` | integer | the newest minor the child implements |
| `address` | string | where the host reaches the child, or `stdio` when the channel is the pair the host handed over |

A member this specification does not define is ignored. That rule is what lets one child serve a host older than the version it was built against, and a host that refuses an unknown member breaks every future release.

A host refuses the line when `protocol` is zero, when `min_minor` is above `max_minor`, when `address` is missing, or when `address` carries a control character. The child has run no work at this point, so the refusal costs nothing.

## The frame

Every frame is a nine-byte header and a payload.

```
+-----------+-----------+--------+---------------------+
|  len u32  |  sid u32  | kind u8|       payload       |
+-----------+-----------+--------+---------------------+
     4 B         4 B       1 B          len bytes
```

| field | width | meaning |
| --- | --- | --- |
| `len` | 4 bytes | the payload length in bytes. It does not count the header |
| `sid` | 4 bytes | the stream this frame belongs to |
| `kind` | 1 byte | what the payload means |

Both integers are big-endian, which is also called network byte order. A reader that assumes its own machine's order parses every field wrongly on most hardware.

A worked example. A control frame on stream 1 carrying the two bytes `{}`:

```
00 00 00 02   len = 2
00 00 00 01   sid = 1
01            kind = control
7b 7d         payload = {}
```

## The five kinds

| code | name | payload |
| --- | --- | --- |
| 1 | `control` | one JSON-RPC 2.0 object, encoded as UTF-8 |
| 2 | `data` | raw bytes, at most 32768 of them |
| 3 | `end` | empty |
| 4 | `reset` | a reason as UTF-8, at most 1024 bytes, and it may be empty |
| 5 | `credit` | four bytes, a big-endian count, and never zero |

Kind `0` is not a kind. The five start at one so that a header of nothing but zeroes is refused instead of parsing as a frame about something.

A kind a reader does not know is a protocol error and never a frame to skip. The header carries no way to tell a payload a future version added from a payload a broken peer wrote.

## Streams

A stream id names one conversation inside the channel.

- `0` is not a stream. A frame carrying it is refused
- an odd id was opened by the host
- an even id was opened by the child
- an id is used once per connection and never opened again

The parity rule is HTTP/2's, and it is there so that neither side asks the other before it opens a stream. Both sides allocate from their own half and they never collide.

A stream is opened by the first frame that names it. There is no frame that opens one. A stream is finished when both sides have sent `end` on it, or at once when either side sends `reset`.

## The sizes, and what a reader refuses

| name | value | what it bounds |
| --- | --- | --- |
| header | 9 bytes | fixed, on every frame |
| default payload cap | 1048576 bytes | the largest payload a reader accepts when its host states no smaller cap |
| data payload | 32768 bytes | the largest a single `data` frame carries |
| reset reason | 1024 bytes | diagnostic text, never a value to branch on |
| credit payload | exactly 4 bytes | one big-endian count |
| handshake line | 4096 bytes | including the newline |

The payload cap is the first thing that bounds a peer that is hostile or broken. A reader checks `len` against it from the header alone, before it allocates anything. A host that knows what a given child sends states a smaller cap.

These refusals are named in the vectors, and an implementation reports each one:

| reason | when |
| --- | --- |
| `stream_zero` | the stream id is 0 |
| `unknown_kind` | the kind byte is not one of the five |
| `payload_over_the_cap` | `len` is above the reader's cap |
| `data_over_the_chunk` | a `data` payload is above 32768 bytes |
| `end_carries_a_payload` | an `end` frame has a payload |
| `credit_length` | a `credit` payload is not 4 bytes |
| `credit_zero` | a `credit` frame grants nothing |
| `reset_over_the_limit` | a `reset` reason is above 1024 bytes |
| `truncated` | the channel closed inside a frame |

A refusal ends the connection. The bytes behind a header a reader declined belong to a frame it never parsed. The position in the stream is lost, so no later frame can be trusted. Close the channel and let the host restart the child.

An end of channel between two frames is ordinary. An end of channel inside one is `truncated`.

## Credit

A receiver grants a sender room, and a sender that has no room waits. This is the flow control that gRPC ships and this wire writes.

- a `credit` frame carries a big-endian count of payload bytes, granted on the stream that frame names
- grants add up. A sender keeps one balance per stream
- a sender may send a `data` payload only when its balance covers it, and it subtracts what it sent
- a balance is never negative, and the total on one stream never passes 4294967295
- `control`, `end`, `reset` and `credit` frames cost nothing. Only `data` spends

A receiver that stops granting stops the sender. That is the whole mechanism, and it is why a slow consumer here becomes back pressure and not an unbounded queue.

A sender that puts more payload bytes on a stream than its balance covers is a protocol error. The receiver made room for what it granted and no more, so it closes the connection rather than buffering the rest. This is not a frame a reader refuses, because the header alone cannot tell: the balance is state each side keeps.

## Control frames

A control payload is one JSON-RPC 2.0 object. The specification is [JSON-RPC 2.0](https://www.jsonrpc.org/specification), and this wire adds three rules to it.

1. One object per frame. A JSON-RPC batch is refused, because the frame already multiplexes and two multiplexers disagree about ordering
2. A request and its response travel on the same stream
3. An id is unique within its stream and not across the connection

A contract's own methods are not in this document. They belong to the contract the extension implements, such as a printer driver or an event sink, and each contract publishes its own. One method set is not a contract's, and it is below.

## The lifecycle method set

Every child answers the first three, whatever else it serves. They are what a host calls before it knows what the child is, before it stops one, and while it checks the child is still reading. The fourth is answered by a child that reads settings, and a child that does not answer it says so by leaving it out of `serves`.

| method | params | answers |
| --- | --- | --- |
| `plugin.info` | none | the child's name, its own release, and the lifecycle contract version it was built against |
| `plugin.shutdown` | none | nothing. The child finishes and closes the channel |
| `plugin.ping` | none | nothing, JSON `null`. The connection answers it, before any method the child serves |
| `plugin.configure` | `{"config": {...}}`, the child's settings as one object | nothing. An error refuses the start |

`plugin.info` answers three members, and a module that serves routes adds two more:

```json
{"name":"example-blob","version":"1.0.0","contract":"1.1.0"}
```

| member | means |
| --- | --- |
| `name` | the extension's own name, the one its descriptor carries |
| `version` | the release of the extension itself, as MAJOR.MINOR.PATCH |
| `contract` | the lifecycle contract the child was built against, as MAJOR.MINOR.PATCH |
| `serves` | the methods beyond `plugin.info`, `plugin.shutdown` and `plugin.ping`, such as `routes.serve` and `plugin.configure` |
| `openapi` | an OpenAPI 3.1 document for the routes on `routes.serve`, at most 512 KiB, required when `serves` names it |

Every path in `openapi` is relative to the mount the host assigns. The host refuses an `openapi` member it cannot publish, and the launch fails with the reason. The reasons are four: the document does not parse, it does not declare OpenAPI 3.1, a path is not relative, or an operation carries no `operationId`, `summary` or `description`. A child that names `routes.serve` and sends no `openapi` member is refused at the same point, with a reason naming the child and the call that attaches one.

The host refuses a `contract` whose major is not the one it serves, and one whose minor is above its own. The host serves 1.1.0. A child that reports 2.0.0 is refused before it is asked to do anything.

A host calls `plugin.info` once, straight after the handshake, and it stops a child whose answer it cannot read. A host calls `plugin.shutdown` before it stops a child, waits, and kills the process if it does not go.

A host pings a child with `plugin.ping` every 15 seconds, after the start has finished, with a timeout of 5 seconds. A child that misses 3 pings in a row is stopped, and any answer resets the count. An answer of any kind counts as alive, including the `-32601` a child built against contract 1.0.0 returns, because it proves the child read the request and wrote a reply. A pong proves the process is scheduled, its read loop runs and its output drains. It does not prove that a handler is alive: each request runs in its own goroutine, so a child with a deadlocked handler still answers.

A host calls `plugin.configure` once, after `plugin.info` and before every other call, on a child whose `serves` names it. The `config` member is one JSON object: the host builds it from the defaults the child's manifest declared and the operator's own settings block written over them, key by key. The child reads it strictly, and an error refuses the start. A child that leaves `plugin.configure` out of `serves` is never called, and a host refuses to start one an operator wrote settings for.

Settings travel on this call and never as an environment variable or a command-line argument. A setting may be a credential, and both of those are readable in a process listing.

## What a host verifies before it runs anything

The digest is checked before every launch and not only at install, so a file swapped after installation is a refusal rather than a run. It is written `sha256:` and 64 hex characters. It covers the installed artefact, so for a child run through an interpreter it covers the script and not the interpreter.

## Version negotiation

The major is in the handshake. Two peers with different majors exchange no frame at all, and the host refuses before it runs anything the child would have served.

The minor is the newest version both sides implement:

```
minor = min(host max_minor, child max_minor)
```

That value is refused when it falls below either side's `min_minor`. A pair with no overlap is a refusal naming both ranges.

| host | child | settles on |
| --- | --- | --- |
| 0 to 0 | 0 to 0 | 0 |
| 0 to 3 | 0 to 1 | 1 |
| 2 to 3 | 0 to 5 | 3 |
| 0 to 1 | 2 to 3 | refused, `no_common_minor` |

This is Terraform's rule. A minor adds something a peer may ignore, so the newer side offers it and the older side never sees it. A major changes the meaning of a frame the older side would still parse, which is why it is never negotiated.

## The conformance vectors

[`docs/wire/plugin-wire-protocol.json`](https://github.com/prusa3d/prusa-connect/blob/main/docs/wire/plugin-wire-protocol.json) holds the sizes, the kinds and 39 vectors. Every implementation is held to that file, and this repository's Go is checked against it like any other.

| section | holds |
| --- | --- |
| `frames` | 13 frames, each with the exact bytes it encodes to and decodes from |
| `refusals` | 13 byte strings, each with the reason it is refused for |
| `handshakes` | 8 lines, three accepted and five refused |
| `negotiations` | 5 version pairings and what each settles on |

Bytes are written as hex. A vector at a size boundary states `payload_repeat` instead, which is an object of a byte and a count. A vector at the payload cap is a megabyte, and a megabyte of hex is unreadable.

```json
{
  "name": "credit_one_chunk",
  "doc": "A grant of 32768 payload bytes, big-endian in four bytes.",
  "stream": 2,
  "kind": "credit",
  "header_hex": "000000040000000205",
  "payload_hex": "00008000"
}
```

An implementation passes on four counts. It encodes every `frames` entry to those exact bytes and decodes them back to the same frame. It refuses every `refusals` entry with the stated reason. It reads every handshake line as stated, and it settles every negotiation as stated.

`examples/wire-protocol/` holds a second implementation, in JavaScript for Node, written from this page and checked against the same file. It uses Node's standard library and no package, which is the measure of how much wire there is to write.

## What a host owes a child

Stated here because a guarantee an author cannot check is not a guarantee. The last column names the file that keeps it, or says that nothing does yet.

| the host | kept by |
| --- | --- |
| launches the child. A child never launches itself, so the checksum can be verified before anything runs | `internal/extension/wirehost/launch.go` |
| verifies the recorded digest before every launch, and not only at install | `internal/extension/wirehost/digest.go` |
| refuses an incompatible wire major before it exchanges a frame | `sdk/wire`, negotiated from the handshake line |
| refuses a lifecycle contract it does not serve, before it asks the child for anything | `internal/extension/wirehost/launch.go` |
| copies the child's standard error into its own log, with the instance bound | `internal/extension/wirehost/launch.go` |
| bounds the payload and times out every call | the payload cap in `sdk/wire`, the call timeout in `sdk/wirerpc` |
| grants credit and stops a sender that spends more than it was granted | `sdk/wirerpc` |
| recovers from a panic in a handler of its own, and answers the call rather than losing the session | `sdk/wirerpc` |
| never puts third-party code between a decoded printer frame and the state update | a design rule. Decode, state update and fan-out call concrete types |

The last row is the one that shapes everything else. A connector that pushes decoded frames from outside the process sits upstream of that path, so the rule holds and the driver still runs elsewhere.

## Why this wire and not gRPC

gRPC does not fit the binary this project promised. The three full targets carry 5.81 to 6.61 MiB of headroom and a bare gRPC server and client measure 7.31 MiB. It is also 5 to 7 times the round trip of a plain frame on the same machine.

| shape | small round trip | 64 KiB |
| --- | --- | --- |
| this wire, over stdio pipes | 16.4 µs | 50.1 µs |
| JSON-RPC with every payload inside JSON | 17.7 µs | 1069.4 µs |
| gRPC | 118 µs | measured separately |

The 64 KiB column is why bulk bytes have a kind of their own. Base64 inside a JSON string costs 20 times what the same bytes cost in a frame.

## Where the pieces live

| path | what |
| --- | --- |
| `sdk/wire` | the frame codec, the handshake and negotiation, standard library alone |
| `sdk/wire/wiretest` | the vectors, as bytes a person wrote down |
| `sdk/wirerpc` | the session above the frame: streams, JSON-RPC control, the credit balance, and the lifecycle method set |
| `internal/extension/wirehost` | the host side: the digest, the process, the handshake, the log |
| `cmd/generate-wire-protocol` | writes the published artefact from the first two |
| [`docs/wire/plugin-wire-protocol.json`](https://github.com/prusa3d/prusa-connect/blob/main/docs/wire/plugin-wire-protocol.json) | the artefact an implementation is held to |
| `examples/wire-protocol/wire.js` | the frame codec in JavaScript, written from this page |
| `examples/wire-protocol/conformance.js` | that codec against the artefact |
| [`examples/wire-protocol/child.js`](https://github.com/prusa3d/prusa-connect/blob/main/examples/wire-protocol/child.js) | a Node child answering the lifecycle set and streaming under credit, written from this page |

`make wire-check` runs five gates. The fourth runs the JavaScript codec against the artefact, which is the evidence that two implementations of this page agree. The fifth has a Go host launch a Go child as a second process and read bulk bytes from it under credit.
