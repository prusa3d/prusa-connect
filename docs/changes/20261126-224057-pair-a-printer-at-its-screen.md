### Added

- `POST /api/v1/printer-pairings` asks a printer on the local network to pair, and the registration the printer makes after a person presses yes on it is approved with no second step. It needs Buddy firmware with the pairing endpoints of its local API, and older firmware is answered `core.printer/PAIRING_UNSUPPORTED`.
- The Add a printer page can pair a printer at its own screen. You type the printer's address, check that the printer shows the same code, and press Yes on the printer. The page warns about a server address the printer cannot store, and a printer whose firmware cannot pair is told to use the settings file.
