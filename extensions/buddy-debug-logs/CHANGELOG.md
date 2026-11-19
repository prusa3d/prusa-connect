# Changelog

What changed in this module, newest first. The top heading is the version
`module.yaml` declares and the version this process reports to the host, and
`changelog_test.go` fails when the three differ.

## 0.1.10 - 2026-11-17

### Changed

- The page takes the Ember look of the web app. Its header fits a phone, the component filter is wider, and only warnings and errors show as badges.

## 0.1.9 - 2026-11-16

### Changed

- `module.yaml` names the licence the extension is released under, OCL v1.1 + Micro v1 + NoSaaS v1 + SWAtt v1.

## 0.1.8 - 2026-11-11

### Changed

- Built with Go 1.26.11 for GO-2026-6859.

## 0.1.7 - 2026-11-05

### Fixed

- Refresh on the logs page reads the log lines and the metrics again, and leaves live mode as it was.

## 0.1.6 - 2026-11-04

### Fixed

- The log view, its status strip and the setup guide look like the rest of Prusa Connect.

## 0.1.5 - 2026-11-04

### Fixed

- A search or a filter that matches nothing offers the way back to every row.

## 0.1.4 - 2026-10-28

### Changed

- `module.yaml` names what the module needs under `require`, the map the server reads now.

## 0.1.3 - 2026-10-26

### Changed

- Comments cite the decision records by their dated ids.

## 0.1.2 - 2026-10-25

### Changed

- Comments and the README no longer point to a removed research note.

## 0.1.1 - 2026-10-21

### Changed

- Built with Go 1.26.10 for GO-2026-6809.

## 0.1.0 - 2026-10-19

### Added

- Receives the log lines and the metrics a Prusa printer sends when its
  Metrics & Log setting is on. They arrive on the UDP ports 13514 and 8514,
  or on the ports the operator chose. Each one is matched to a printer by its
  MAC address or its network address. Both the syslog form of firmware 6.4
  and the one of 6.10 are read. A metric value that is not a number, such as
  the nan of a failed sensor, is kept as text.
- Treats a datagram larger than 2 KiB as unreadable. The same goes for a
  datagram not named by a MAC address, and for one with more points or fields
  than the firmware sends. Control characters in a line are escaped. A full
  list of senders forgets an unmatched MAC first, then a MAC matched by its
  address alone.
- Keeps them in its data directory for 24 hours or 128 MiB, whichever comes
  first, with a quarter of the room kept for the log lines.
- Serves the lines filtered by printer, level, component, time and text, the
  metrics as series for a chart, and both as a download. A person sees only
  the printers they may see. Lines from a MAC matched to no printer go only to
  a person who may see every printer, and to nobody while the module knows no
  printer.
- Sets a printer up with M334, each stream on or off alone. The printer asks a
  person to confirm the change on its display, and the page says so before
  and after sending. A person who may not send G-code is shown the steps on
  the printer's own display. Stop sending turns both streams off on the
  printer.
- Removes kept records on request: everything, or what is older than 1, 6 or
  24 hours, after a confirmation. Only a person who may see every printer can
  do this.
- Adds the newest lines and points to the diagnostics bundle. A file name holds
  a printer's device id, or an alias for a MAC matched to no printer.
- The Prusa printer logs page shows whether each port is receiving, in a row
  that unfolds into the counts and the time of the newest data. A port that
  got nothing for two minutes shows Waiting for data. It lists the log lines
  with filters and a live mode, draws each metric as a chart, and lists the
  senders and their printers. Each log line shows the address it came from.
  It keeps its tab, filters and folds in the address. A printer's page gets a
  Logs tab with its own lines.
