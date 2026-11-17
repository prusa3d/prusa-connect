# Changelog

What changed in this module, newest first. The top heading is the version
`module.yaml` declares and the version this process reports to the host, and
`changelog_test.go` fails when the three differ.

## 0.1.1 - 2026-11-24

### Changed

- The build tooling takes minimatch 11 for GHSA-g3hv-75wg-m249. Nothing the extension ships changes.

## 0.1.0 - 2026-10-29

### Added

- Each printer has a queue of print files. A person adds a file, drags it to another place or moves it up or down, and takes it off again. The person must be allowed to print on the printer.
- Each file on a queue is drawn with the picture of its sliced plate, as on the Files tab, where the file carries one.
- The print from the queue shows how far it is on the Queue tab, and a person who may print on the printer pauses, resumes or stops it there. Stop asks first.
- The Queue tab lays the queue out in time, on a bar with a fixed scale of 200 pixels for three hours. It zooms from a quarter of an hour to twelve hours, also with Ctrl and the mouse wheel. A long queue scrolls sideways. Each file shows when it starts and ends at the earliest. The times come from the print time the slicer estimated, read when the file is added. A file that names none is drawn as an hour and marked as unknown.
- Add files on the Queue tab lists the files on the printer and in the server's storages. A person ticks files in several folders and adds them all at once. A file from storage is copied to a connected printer at once, and asks about a name the printer already holds.
- A print file on the printer has Add to queue in its menu on the Files tab. The queue prints it where it is and sends nothing.
- A file in storage can also be added from the print dialog. The person must be allowed to read it, and the queue sends it to the printer when its turn comes.
- A file dropped on a printer's page can be added with Add to queue. It is copied to the printer and waits in the queue from the moment the copy starts. A copy that fails is said on the file, which then waits for a person.
- The next file starts when a person presses Set ready and print, or Print next on a printer with no ready state, or adds a file to a ready printer. It also starts by itself when the printer is ready and the switch Let the queue start the next file by itself is on. That happens after the printer is marked ready, on its own screen or with Set ready, and when a dropped file lands. A finished printer is not ready until a person says its bed is clear, on the printer or with Set ready. So a print never starts on a bed nobody cleared.
- Before the queue starts a file by itself, it asks whether the person who added the file may still print on the printer and read the file in storage. If not, the file waits with a note that says why. This is reliable on a server with local accounts where the install approved `check_absent_people`. A server with an OpenID Connect login or in open mode answers only about a person who used the queue since it started. The other files wait for a person to start them.
- A send that failed goes back to the head of the queue with the reason and waits for a person. A printer that is busy taking another file is not a failed send: the file waits and the queue tries again. The second send writes the file under a numbered name, such as `bracket[1].bgcode`, so it never prints another file of the same name.
- The queue follows each print to its end from the server's events, and starts the next file as soon as a printer's state says it is ready. It also compares each queue with the printer once a minute. Ended prints stay in a history with how they ended, under the queue as one row that opens.
- The queues are kept in a database of the module's own, which the install grants: SQLite in the module's data directory, or Postgres.
- The Print queue page shows the queue of every printer a person may see. It draws them on one timeline with a lane for each printer, or as a row of cards for each printer with the file it prints first. The page remembers the choice for each person on the server. A person who may change a queue drags a waiting card to another place in its row. A printer's name and each card open its Queue tab. The page reads every queue in one request. It reads again as soon as the server's events say a printer changed, and every 30 seconds for a change no event tells of. It links to the core's API reference, which lists every route of the queue.
- Each printer card shows one line about its queue: what it sends or prints, and how many files wait.
- The Queue tab has a switch that allows the queue to start prints by itself on a printer marked ready. It writes the operator grant for the module and withdraws it again, so only a person who administers the printer can turn it. A ready printer with a file waiting and the switch off says so in a box, and Print next starts the file.
