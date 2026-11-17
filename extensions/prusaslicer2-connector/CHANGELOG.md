# Changelog

What changed in this module, newest first. The top heading is the version
`module.yaml` declares and the version this process reports to the host.

## 0.1.2 - 2026-11-24

### Changed

- The build tooling takes minimatch 11 for GHSA-g3hv-75wg-m249. Nothing the extension ships changes.

## 0.1.1 - 2026-11-24

### Changed

- Upload puts the file on the printer's USB drive and starts nothing, as with PrusaLink. It no longer keeps the file in a storage of this server. The `storage` setting is gone, and a config block that still names it is read and ignored.
- The address, the API key and the steps in PrusaSlicer moved from the foot of the printer's Status tab to a card on its Printer tab. The steps fold away once PrusaSlicer has reached the printer. An operator who hid the old section hides the new card again under Settings, Extensions.
- The Uploads tab names each upload route by what it takes, under its method and path.
- The core removes a sent file from the module's print folder once it landed, for Upload and Print as for Upload. Before, it stayed a day.

### Added

- The card says when PrusaSlicer last reached the printer, or that it has not yet. PrusaSlicer holds no connection open, so this is the last Test or upload.
- Save to storage on the card also saves each upload to that printer in a storage and folder of this server. It works as for a file dropped on the printer's page.
- Each saved upload is written as the person whose API key sent it.
- A saved upload never replaces a file of the same name in the folder. It is saved beside it under a numbered name, as on the printer.
- Needs a Prusa Connect build that lands a file on a printer for a module with `device.send`.

## 0.1.0 - 2026-11-18

### Added

- PrusaSlicer 2.4 and later sends a print to each printer of this server as it sends one to a printer with PrusaLink. Each printer has its own address under the module, and PrusaSlicer signs in with an API credential of this server as the API key.
- Upload and Print sends the file to the printer and starts it. A printer that already holds a file of that name gets the file under a numbered name.
- Upload keeps the file in the `local` storage of this server, or in the storage the `storage` setting names, and prints nothing.
- PrusaSlicer 2.6 and later sends the file with PUT, and PrusaSlicer 2.5 and older with POST in a form. Both take the file on a staged route, up to the limit the operator approves at install, 1024 MiB as the module asks.
- A print to a printer that is not ready is refused before the printer is asked. So is a print with a key whose person may not operate the printer. PrusaSlicer shows the reason.
- The foot of each printer's Status tab shows the address to paste into PrusaSlicer and makes the API key, with the steps in PrusaSlicer.
- The module keeps a printed file for a day, so the printer can download it, and then removes it.
- Needs a Prusa Connect build that takes an API credential in `X-Api-Key` and passes a header a module asked for: 0.1.0-beta.2 or later.
