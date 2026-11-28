### Added

- [sdk] A module can offer a stylesheet as a theme for the whole app with `ui.themes` in its manifest, and `GET /api/v1/resources` lists each one under the module's `ui.themes`. A person picks one under Settings, Appearance, Extension theme, and the app draws it from the first paint, the sign-in page included.
- The `core.frontend.extension_theme` key names the module theme a browser with no pick of its own draws in. `GET /api/v1/themes/default.css` serves its stylesheet, and `GET /api/v1/ui` reports it as `extension_theme`.
