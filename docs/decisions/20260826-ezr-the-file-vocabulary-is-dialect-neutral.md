---
title: "ADR-20260826-ezr: the file vocabulary is dialect-neutral"
description: "The core names file operations for what they do, a driver maps them onto its own wire commands, and no surface above the driver learns a dialect's spelling."
date: 2026-08-26
status: accepted
tags:
  - adr
  - printers
  - storage
---
# ADR-20260826-ezr: the file vocabulary is dialect-neutral

- status: accepted
- date: 2026-08-26

<!-- nav:related -->
> [!NOTE]
> Index: [decisions](README.md) | Builds on [ADR-20260825-dee](20260825-dee-the-capability-vocabulary.md)

The capability vocabulary was extracted mechanically from the shipping xBuddy
firmware matrix, so every key in it is spelled the way one
dialect's firmware spells it. That was harmless while the keys named things
every printer does under some name: `start_print`, `create_folder`,
`delete_file`.

It stopped being harmless when the file transfer was ported. The transfer needed
a key that surfaces above the driver could ask about, and the extracted name for
it was `core.command.start_inline_download`. A second connector, for hardware
that has never heard of an inline download, would have had to declare that key
for the core's file endpoint to work on its printers.

## rules

1. a capability key the core defines is named for what the operation does, never
   for what one dialect's wire calls it
2. a driver maps a core key onto its own commands inside the driver. Those
   command names may stay in the driver's own baseline, because a dialect's
   verbs are its own business, and nothing outside the driver package may name
   them
3. an operation that is not one frame is a `core.feature.*` key, not a
   `core.command.*` one. A command key is invokable through
   `POST /api/v1/printers/{printer}/commands` and a feature is not, so a key
   that would be refused there does not belong in that namespace
4. a core key the firmware matrix has no row for is host-minted through
   `profile.HostMinted`, so the profile tier does not subtract a capability no
   profile was ever going to mention (ADR-20260825-z65 rule 18)
5. a refusal that reaches a caller carries the device's own machine reason to
   branch on and its human sentence to show, and neither is invented by the
   driver. A firmware that sends only one of them leaves the other empty
6. a file operation is reached through a contract interface probed off the
   session, not through the command endpoint: `Filesystem` reads, `FileManager`
   changes, `Transfers` writes bytes. A dialect implements the ones it can serve
   and a host refuses the rest with `CAPABILITY_NOT_SUPPORTED`
7. a wire command a driver sends only for itself, and that a caller must not
   start, stays in the driver's wire table and out of what `Baseline()`
   publishes. The published set is what a host resolves profiles against and
   validates callers against, so a key kept out of it is not invokable. Each
   exclusion carries a reason next to the wire form

## what this cost

`core.feature.file_upload` is the only key minted for it. The four xBuddy
transfer commands moved from `sdk/connectorapi` into
`internal/connector/xbuddy/verbs.go` as package constants.

Rule 7 costs one more thing: `START_INLINE_DOWNLOAD` and
`START_CONNECT_DOWNLOAD` are sent and not published, so an xBuddy session's own
capability set is wider than the baseline the driver hands the host. Two sets
inside one driver is the price of a wire command that has to be sendable and must
not be startable. `STOP_TRANSFER` and `SEND_TRANSFER_INFO` stay published: both
are answerable on their own, and an operator stopping a transfer is a real
request.

`create_folder`, `delete_file` and `delete_folder` were already dialect-neutral
names and were promoted into `sdk/connectorapi` unchanged, which is why
`FileManager` has three methods rather than a delete with a flag: the two
deletes are separate keys because devices refuse one aimed at the other's kind.

## rejected

| option | why not |
|---|---|
| keep `core.command.start_inline_download` as the core key | a second connector would declare a Prusa wire command to mean "can take a file". The name is the whole problem |
| rename the extracted command keys in the matrix | the divergence gate compares the profile data against `legacy/` by name. Renaming there makes the gate meaningless, and the extracted names are correct for the dialect that owns them |
| one `FileManager.Delete(path, recursive bool)` | the firmware refuses a file delete aimed at a folder and the reverse, so a caller that could not say which it meant would be asking the device to guess. Two keys also let a profile withhold one |
| gate the whole interface on one capability | a printer that can create a folder and not remove one is a real answer, and one check reports it as neither |
| drop the two download starts out of the wire table as well | `Transfers.Send` is what puts one on the wire. A verb with no wire form cannot be sent at all |
| let a caller start a download through the command endpoint | the device then asks for ranges against a transfer nothing is holding, so no file ever arrives, and the driver answers such a request by stopping the transfer. That is the recorded reason `START_ENCRYPTED_DOWNLOAD` was never ported |

## enforcement

`TestTheBaselineIsExactlyTheWireTable` holds rule 3: a baseline key with no wire
form fails unless it is listed in `servedNotSent` with a reason.
`TestThePublishedBaselineIsTheWireTableLessWhatOnlyTheDriverSends` holds rule 7,
in both directions: a wire form missing from the published set needs an entry in
`sentNotInvokable` with a reason, and an entry for a verb that is published
anyway fails as stale. `TestTheShippedProfilesCoverTheWholeDriverBaseline` holds
rule 4.

There is no gate on rule 2. Nothing stops a future package importing
`xbuddy.VerbStartInlineDownload`, because it is exported for the driver's own
tests. An arch test refusing a dialect verb outside its package is the open
item.

## open items

- rule 2 has no enforcement gate
- rule 7 leaves a driver whose session capabilities are wider than its published
  baseline. Nothing outside the xBuddy package can gate that the difference is
  exactly `sentNotInvokable`, so a second dialect doing this has to bring its own
  test
- `core.command.send_info` is an xBuddy command name in `sdk/connectorapi`, the
  same debt as `send_file_info` below and settled by the same migration
- `core.command.send_file_info` is still the key `Filesystem.List` is gated on,
  and it is an xBuddy command name. It predates this record and renaming it
  changes behaviour a caller already has, so it is a migration rather than an
  edit
- reading a file back off a device has no contract surface at all
