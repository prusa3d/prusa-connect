# Changelog

What changed in this module, newest first. The top heading is the version
`module.yaml` declares and the version this process reports to the host.

## 0.1.1 - 2026-11-24

### Changed

- The build tooling takes minimatch 11 for GHSA-g3hv-75wg-m249. Nothing the extension ships changes.

## 0.1.0 - 2026-11-12

### Added

- PrusaSlicer 3 in offline mode lists the printers of this server and sends a print, with the requests and replies of Prusa Connect. It signs in through the server's app pairing, and the module answers its Prusa Account me request.
- A person with only the viewer role sees the printers and their state in PrusaSlicer 3. A print they send is refused with the cloud's 403 reply.
- A print to a printer that is not ready, or by a person who may not operate it, is refused before the slicer sends the file. The person is checked when the operator approves the module's `operate` check at install.
- The server deletes a printed file once its print has settled. A file whose send failed stays at most a day.
- A file the printer already holds is printed under a free name, as the cloud does when a person sends the same file again.
- PrusaSlicer 3 in offline mode reads each printer's state from the printer list, which it reads again every 30 seconds.
- The largest G-code file is the limit the operator approves for the upload route at install, 1024 MiB as the module asks. The operator may change it on the extension's Uploads tab.
- PrusaSlicer 3 shows the signed-in person's initials in a coloured circle as their avatar. The picture comes from a public route the operator approves at install, and its address alone decides it.
- Inside PrusaSlicer 3, every printer in the listing and the top of each printer's page offer Set as current. It selects that printer in the slicer, as Prusa Connect does.
- Needs Prusa Connect 0.1.0-beta.2 or later. It is the first release that answers the slicer's app pairing with `access_token`, takes the upload on a staged route and deletes a printed file for the module.
