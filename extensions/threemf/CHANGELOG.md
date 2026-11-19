# Changelog

What changed in this module, newest first. The top heading is the version
`module.yaml` declares and the version this process reports to the host, and
`changelog_test.go` fails when the three differ.

## 0.1.12 - 2026-11-17

### Changed

- The project view takes the Ember look of the web app, and its Czech strings are complete.

## 0.1.11 - 2026-11-16

### Changed

- `module.yaml` names the licence the extension is released under, OCL v1.1 + Micro v1 + NoSaaS v1 + SWAtt v1.

## 0.1.10 - 2026-11-11

### Changed

- Built with Go 1.26.11 for GO-2026-6859.

## 0.1.9 - 2026-11-10

### Fixed

- A black or unset filament colour no longer hides the model in the 3D view, in the dark theme too.

## 0.1.8 - 2026-11-05

### Changed

- Shift-click ticks a range of copies in an object's panel, and a partly ticked box shows a dash.

## 0.1.7 - 2026-11-04

### Fixed

- The bed switcher, the object list and the issues in the 3D view look like the rest of Prusa Connect.

## 0.1.6 - 2026-11-02

### Fixed

- The button that shows only an icon in the 3D view names itself in a tooltip on hover and on
  keyboard focus.

## 0.1.5 - 2026-10-30

### Changed

- The module asks the host for 256 MiB: its caches and two whole-project
  reads at once.

## 0.1.4 - 2026-10-29

### Fixed

- A project read for one person could be refused once their request ended, because the read goes on for everyone who waits on it. The refusal was then kept, and every later reader of the file got "you may not read this file". A refusal is no longer kept, and a reader who waited reads the file again as themselves.

## 0.1.3 - 2026-10-29

### Changed

- The project view reads in Czech in full.
- A page that does nothing with ticked copies, such as a file's page, has
  no boxes to tick. A press on a copy opens its object.
- The settings are a card of their own, in two columns on a wide page, and
  the filaments of one setup are named once, in the overview. The detail of
  a bed, such as its changes by height, stands under the plan of that bed.
- A copy's name on the plan is drawn whole or, where at least six letters
  fit, cut short. A name cut to two or three letters is no longer drawn.
- The bed cards are as wide as their names, so "All beds" is no longer cut
  short.

### Fixed

- A copy left out of the print was drawn black. The outlines of a ticked,
  lit or focused copy were missing too. The plan read colour names the page
  does not define.
- The 3D view of a long object ran off the edge of the frame when it
  opened. It now opens with the whole object in sight.

## 0.1.2 - 2026-10-29

### Changed

- An object opens in a dialog over the page rather than in a panel at the
  side. Its picture is large on one side, and its size, filaments, parts and
  copies are on the other.

## 0.1.1 - 2026-10-29

### Changed

- `threemf.Arrange` turns a copy by a right angle when it fits no empty bed as it stands, and `Rewrite` writes the turn a position asks for.
- A setup that prints with a wipe tower and names no place for it keeps the place PrusaSlicer gives the tower free.

## 0.1.0 - 2026-10-28

### Added

- The Go package `github.com/prusa3d/threemf/threemf` reads a 3MF
  project into its printer setups, beds, objects and instances, and rewrites
  a PrusaSlicer project to print a chosen selection. Other modules import it.
- `threemf.Arrange` places copies of objects onto as few beds of one setup
  as hold them, and a `Selection` with `Positions` writes such a plate into
  the rewritten project.
- `threemf.ReadMesh` reads the model parts of one object as triangles, made
  coarser to a bound a browser draws at once.
- The module reads `.3mf` files for the server: the picture in a listing,
  the title, the printer, the facts and the objects on the file's page.
- The routes `project`, `project/thumbnail` and `project/mesh` answer what a
  project holds, its pictures and one object as triangles. The module reads
  the file as the person who asks.
