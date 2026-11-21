### Added
- The container image can keep its database in PostgreSQL, with `persistence.type: postgres`, and serves `/metrics` when `core.observability.enabled` is true, as the full release binaries already do. Before, an image set up for PostgreSQL stopped at startup.
