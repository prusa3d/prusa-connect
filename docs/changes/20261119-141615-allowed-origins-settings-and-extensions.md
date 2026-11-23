### Changed
- `core.api.allowed_origins` refuses an entry no browser sends, such as a wildcard host, a port with a leading zero or a host outside ASCII. Such an entry never matched a request, and the server now refuses to start and names it.

### Added
- Settings, Server, Network lists the browser origins that may call the API and lets an administrator add one, which the server answers at once with no restart. `GET` and `PUT /api/v1/allowed-origins` read and replace that list, which is added to `core.api.allowed_origins`.
- [sdk] A module can ask for a browser origin with `allowed_origins` in its manifest, a list of exact https origins. An operator approves it at install, and the server answers pages of that origin while the module is switched on.

### Fixed
- The switch of an extension that runs no process, such as Fleet stats, now switches it off. The server refused the change with `REQUEST_INVALID`, and the switch stayed on.
