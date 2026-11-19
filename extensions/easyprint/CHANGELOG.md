# Changelog

What an operator sees change when they upgrade this module, newest release
first. `version:` in `module.yaml` is the newest heading below.

Write the entry in the same change as the code. One bullet per behaviour a
person outside this repository can observe: a route, a setting, a screen, a
refusal. Name the route and the setting key, never a function or a file.

## 0.1.0 - 2026-11-19

### Added

- Asks the server to answer `https://slicer.printables.com`, so EasyPrint on Printables can call the API from a browser while the extension is switched on. The operator approves it at install.
- An EasyPrint tab on the extension's page under Settings, Extensions, with the link Add this server to EasyPrint. The link opens `https://www.printables.com/slice` with this server's address in `pair_connect_local`.
- The tab warns when the server does not answer EasyPrint's origin, and when the page is open over plain HTTP.
