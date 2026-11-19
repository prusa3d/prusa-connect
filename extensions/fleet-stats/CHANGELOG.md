# Changelog

What an operator sees change when they upgrade this module, newest release
first. `version:` in `module.yaml` is the newest heading below.

Write the entry in the same change as the code. One bullet per behaviour a
person outside this repository can observe: a route, a setting, a screen, a
refusal. Name the route and the setting key, never a function or a file.

## 0.3.4 - 2026-11-17

### Changed

- The page takes the Ember look of the web app. The trend axis starts at zero, printer names in the heatmap have room, and the failures table lists only printers with failures.

## 0.3.3 - 2026-11-16

### Changed

- `module.yaml` names the licence the extension is released under, OCL v1.1 + Micro v1 + NoSaaS v1 + SWAtt v1.

## 0.3.2 - 2026-11-06

### Changed

- Built with source-map-js 1.2.2, the release that fixes an advisory against
  1.2.1. Nothing on the page changes.

## 0.3.1 - 2026-11-04

### Fixed

- The statistics cards are the same tiles as the dashboard, and a card that cannot load says why in an alert.

## 0.3.0 - 2026-10-30

### Changed

- The statistics read the prints the core keeps, `GET /api/v1/jobs`, instead
  of the history. They cover every print since the core first saw the
  printer, and a server with no `history:` resource shows them too.
- A print that ended before the core kept its outcome counts in the jobs, the
  print hours and the files, and not in the success rate.
- Needs Prusa Connect 0.0.1-alpha.8 or later.

### Removed

- The "History is off" page and the warning that part of a period was missing
  from history. Neither can happen any more.

## 0.2.0 - 2026-10-29

### Added

- Four dashboard cards: fleet figures, a daily trend, the utilization heatmap
  and recent failures. Each counts a period of 7, 30 or 90 days and the
  printers a person picks, or a tag, and links to the page showing the same.
- The Statistics page shows a trend day by day, of jobs, success or print
  hours, and a table of the files printed, most printed first.
- The Statistics page narrows to one printer or one tag, and offers 90 days
  beside 7 and 30. The address keeps the choice, so a link opens the same
  view.
- A printer's section links to the page, narrowed to that printer.

### Changed

- Needs Prusa Connect 0.0.1-alpha.7 or later, which the manifest now states
  under `require`. A catalogue does not offer this release to an older core.

## 0.1.5 - 2026-10-26

### Changed

- Comments cite the decision records by their dated ids.

## 0.1.4 - 2026-10-19

### Changed

- Built and tested with Vitest 5.0.5 and Playwright 1.64.1. Nothing on the
  page changes.

## 0.1.3 - 2026-10-16

### Changed

- Built with Svelte 5.57.2, the version the host's web app runs.

## 0.1.2 - 2026-10-09

### Changed

- Utilization and Failures on the Fleet Stats page each sit in a card with its heading.
- The Print statistics section of a printer's Status tab sits in a card, like the other sections there.

## 0.1.1 - 2026-10-09

### Fixed

- Fleet Stats loads on a server that keeps less history than the chosen period and the one before it. It shows the period without the comparison, or what history still keeps with a notice, where it used to show only an error.

## 0.1.0 - 2026-10-08

### Added

- Fleet Stats: a page of its own, with an entry in the sidebar.
- Fleet Stats: one line on each printer's card in the listing.
- Fleet Stats: a section at the foot of each printer's Status tab.

### Fixed

- Fleet Stats loads in the web app. It asked for a component library newer than the one the app ships, so the app refused to run it.
