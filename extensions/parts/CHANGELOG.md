# Changelog

What changed in this module, newest first. The top heading is the version
`module.yaml` declares and the version this process reports to the host, and
`changelog_test.go` fails when the three differ.

## 0.3.8 - 2026-11-17

### Changed

- The page takes the Ember look of the web app. On a narrow page the folder tree folds above the listing, behind a toggle.

### Fixed

- The page no longer asks for the picture of a print file that carries none, so no request ends in a 404. Each file version records whether the storage found a thumbnail in it.

## 0.3.7 - 2026-11-16

### Changed

- `module.yaml` names the licence the extension is released under, OCL v1.1 + Micro v1 + NoSaaS v1 + SWAtt v1.

## 0.3.6 - 2026-11-11

### Changed

- Built with Go 1.26.11 for GO-2026-6859.

## 0.3.5 - 2026-11-04

### Fixed

- Part cards, the parts listing, tags and the drop tray look like the rest of Prusa Connect.

## 0.3.4 - 2026-11-04

### Fixed

- A search or a filter that matches nothing offers the way back to every row.

## 0.3.3 - 2026-11-02

### Fixed

- Every button that shows only an icon names itself in a tooltip on hover and on keyboard focus,
  and the folder tree's expand arrows are buttons of the same kind.

## 0.3.2 - 2026-10-28

### Changed

- `module.yaml` names what the module needs under `require`, the map the server reads now.

## 0.3.1 - 2026-10-26

### Changed

- Comments cite the decision records by their dated ids.

## 0.3.0 - 2026-10-22

### Changed

- The catalogue is kept in a database of the module's own, and the server no
  longer needs a record backend for it. On a server set up by the first-run
  wizard, the catalogue now works.
- The operator chooses where the database goes when the module is installed.
  The default is a SQLite file in the module's data directory, and a database
  on a Postgres server is the other choice. A core before 0.0.1-alpha.7
  refuses this release.
- The first request after an upgrade from 0.2 moves the catalogue out of the
  record store into the database, once and in one transaction. The records
  stay in the record store.
- Deleting a part removes its versions and comments in one transaction.
- The module asks for no room in the record store.

## 0.2.3 - 2026-10-21

### Changed

- Built with Go 1.26.10 for GO-2026-6809.

## 0.2.2 - 2026-10-19

### Changed

- A select, such as the sort order of a folder, tells a screen reader it is
  a combobox, so the reader announces its name and the value apart.
- Built with Lucide icons 1.58.0.

## 0.2.1 - 2026-10-16

### Changed

- Built with Svelte 5.57.2, the version the host's web app runs.

## 0.2.0 - 2026-10-16

### Added

- Two cards for a dashboard. **Parts folder** shows the newest parts of one
  folder, or its parts by name, with their pictures. It can take in the
  folders inside it and keep only the parts with one tag. **Chosen parts**
  shows parts a person chose one by one. Each part links to its page in the
  catalogue.
- Each card draws its own settings: the folder from the tree and the tag from
  the tags in use, or a search of the catalogue to add parts with.

## 0.1.1 - 2026-10-09

### Changed

- The Parts page header keeps one button, New part. Files are uploaded from a
  dropzone at the head of the folder column, as on the Storage page. A new
  folder is made from the button beside the folder tree.
- With another extension installed, the folder column sits beside the listing
  on a wide screen again. It used to stack above it, with a line down the
  listing's side. At the top of the catalogue the listing no longer repeats
  "Catalogue" as a trail above itself.

## 0.1.0 - 2026-10-05

### Added

- A person keeps a private catalogue of parts on the server. The Parts page
  shows a tree of folders, and each folder holds parts. The module serves
  `/overview`, `/folders`, `/parts` and the routes under them at the namespace
  the operator installed it at.
- A part has a name, a one-line summary, a long description, tags, comments
  and versions. A version is the set of files the part was at, with a note on
  what changed. Any file the storage takes can be in a version: print files,
  models, drawings, pictures.
- The files are uploaded to one of the server's storages, which the `storage`
  and `root` settings name. The module looks each file up in the storage
  before it records it, as the person who asked.
- An invalid `storage` or `root` setting refuses the start, and the instance
  shows the reason.
- A print file of a version is printed from the part's page, through the same
  printer picker the Storage page uses.
- Two people changing one part at the same time do not overwrite each other.
  The second change is refused with `parts/CHANGED`, and the page offers to
  load the part again.
- Files dropped onto a folder of the page, in the tree or onto the listing,
  become parts of that folder, each with the file as its first version. Files
  that differ only by their extension, such as a model and its print, become
  one part, and a file the storage does not take makes no part.
- Files dropped onto a part's page open its next version with those files
  already uploading, so a new revision is one drag away.
- The module asks for 128 MiB for its records with `records_mib`, room for tens
  of thousands of parts. The Parts page measures the catalogue against the
  limit the operator granted.
- The folder tree opens and closes folder by folder, or all at once, and keeps
  what the person closed across visits.
