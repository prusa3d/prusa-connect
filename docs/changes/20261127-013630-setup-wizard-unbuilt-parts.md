### Fixed
- The first-run wizard no longer offers the core.diagnostics keys on a binary built without the diagnostics tag, where they changed nothing. An answer for a key whose part the binary was built without, such as core.observability.enabled, is refused with 422 naming the build tag.
