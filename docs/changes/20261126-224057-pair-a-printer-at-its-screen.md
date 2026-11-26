### Added

- `POST /api/v1/printer-pairings` asks a printer on the local network to pair, and the registration the printer makes after a person presses yes on it is approved with no second step. It needs Buddy firmware with the pairing endpoints of its local API, and older firmware is answered `core.printer/PAIRING_UNSUPPORTED`.
