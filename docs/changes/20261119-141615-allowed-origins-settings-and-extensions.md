### Added
- Settings, Server, Network lists the browser origins that may call the API and lets an administrator add one, which the server answers at once with no restart. `GET` and `PUT /api/v1/allowed-origins` read and replace that list, which is added to `core.api.allowed_origins`.
