# Changelog

What changed in this module, newest first. The top heading is the version
`module.yaml` declares and the version this process reports to the host, and
`changelog_test.go` fails when the three differ.

## 0.2.3 - 2026-11-16

### Changed

- The job page's name moves from the monospaced face to the theme's sans face. The page sets it through the page trail's `sansPage` property, which replaces `monoPage`.

## 0.2.2 - 2026-11-16

### Changed

- `module.yaml` lists under `egress` every connection the module makes outside Prusa Connect, with why and what it sends.

## 0.2.1 - 2026-11-16

### Changed

- `module.yaml` names the licence the extension is released under, OCL v1.1 + Micro v1 + NoSaaS v1 + SWAtt v1.

## 0.2.0 - 2026-11-12

### Changed

- The module is named `server-slicing`, and its title is Server slicing. Its module path is `github.com/prusa3d/server-slicing`, so its namespace and its marketplace id are `com.github.aux.prusa3d.server-slicing`. The server sees it as a new module: install it again. The new install has a new data folder, so install the PrusaSlicer versions again there, and remove the old `slicer` module. The jobs of the old module stay in its data folder.
- Every error code starts with `server-slicing/`, so `slicer/OUTPUT_EXISTS` is now `server-slicing/OUTPUT_EXISTS`. A client that matches a code changes its prefix.
- The page is titled Slicing. Its address is the same.

## 0.1.10 - 2026-11-11

### Fixed

- The Slicers page explains how to get a build when PrusaSlicer publishes none for this system, and no longer only when the list is empty.

## 0.1.9 - 2026-11-11

### Changed

- Built with Go 1.26.11 for GO-2026-6859.

## 0.1.8 - 2026-11-09

### Added

- A job that stops with `slicer/OUTPUT_EXISTS` names the file that is there already under `error.existing`, as `storage` and `path`. A client that wants that output reads the name there and no longer works it out from the request.

## 0.1.7 - 2026-11-04

### Fixed

- The Slicer page carries a page title, and its tabs and its job and release lists look like the rest of Prusa Connect.

## 0.1.6 - 2026-11-04

### Fixed

- A search or a filter that matches nothing offers the way back to every row.

## 0.1.5 - 2026-11-03

### Fixed

- The API tab no longer shows the row of document links in the framed reference on a server reached over plain HTTP. Its `Core API` link opened a page that refuses to be framed.

## 0.1.4 - 2026-10-29

### Changed

- A slicer run is bounded by the memory budget the host gives the module, less 64 MiB for the module, shared by `max_jobs`. The `memory_mib` setting is gone. An install or an update drops it from the slicer's settings by itself. A slicer declared in `config.yaml` keeps the key and fails to start with `RESOURCE_PLUGIN_SETTINGS` until you delete it there. The module asks the host for 2560 MiB. The settings answer still shows `memory_mib` as the derived figure, read only.

## 0.1.3 - 2026-10-29

### Added

- The page of a 3MF project has a Slice button. It opens a dialog on the Slicer page for the bed, the kind of G-code and whether to replace a file of the same name. Sending it opens the job it queued.

## 0.1.2 - 2026-10-29

### Changed

- A position may turn its copy by a right angle, as `threemf.Arrange` places a copy that fits only turned.

## 0.1.1 - 2026-10-29

### Changed

- The API tab shows the module's routes in the core's API reference page, framed below the getting-started part, so they read the way the core's routes do. It keeps the address, the token scopes, the curl example and the reasons a job fails, and links to the reference on its own.

## 0.1.0 - 2026-10-27

### Added

- Slices a 3MF project from a storage, or one sent with the request, into G-code with the PrusaSlicer command line, and writes the G-code into a storage. A job slices the whole project, or one bed and some of its objects. It reports its progress, the print time, the filament and the pictures, and the objects a printer can cancel with the build item each came from.
- An administrator installs any PrusaSlicer release from 2.7.0 that GitHub publishes for the server's platform, or any version from a manifest of builds they host, and chooses the default. The release list is read from GitHub every six hours. Each download is checked against the SHA-256 GitHub publishes, and a release without one installs only when the administrator confirms it. Several versions are installed side by side, and a newer final release is announced but never installed by itself.
- A job may name positions instead of objects: copies of objects on the bed, as `threemf.Arrange` places them. The project's settings are kept, so an object prints twenty times from a project that holds it once.
- The G-code file name starts with eight characters of a hash of the path asked for, then the name asked for, at most 167 bytes in all. Outputs of one project then differ in their first six characters, so a printer that stores files on FAT keeps each one under its own short name.
- A project another program saved, such as an OrcaSlicer project, fails before the slicer runs, and the job names that program.
- The Slicer page lists the PrusaSlicer versions and the jobs the way the app's other lists do, one row each. A search and filters narrow the versions by channel, state and version line, and the jobs by state, slicer and project. A row opens a detail. A version shows the file it downloads, the SHA-256 it is checked against and the installed executable. A job shows its settings, its G-code, and the end of what PrusaSlicer printed when it failed. The address keeps the tab, the filters and the open row.
- The Slicer page has an API tab. It shows the address of this server, the scope a token needs for each route and a curl example that slices a project from start to end. Every route is listed with its parameters, an example answer and the codes it refuses with. The tab is drawn from the module's OpenAPI document, which now names the scope and the codes of each route.
