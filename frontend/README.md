# The web app

The Svelte application that the core serves at `/`. It talks to the core only
through `@prusa/connect`, and it builds its screens from
`@prusa/connect-ui` in `packages/svelte`.

## Storybook

The Storybook in this directory shows the whole product: the screens of this
app in all their states, and every story of the component library in
`packages/svelte/src/`. It needs no running core. The stories answer the app's
requests from recorded data.

```sh
make fleet-stats-ui-build            # once: builds the SDK, the library and the fleet-stats bundle
pnpm --dir frontend run storybook    # dev server at http://localhost:6006
make frontend-storybook              # builds storybook-static/, as CI does
```

The dev server serves the fleet-stats bundle from
`extensions/fleet-stats/ui/dist/ui`, so that directory has to exist before it
starts. `make frontend-storybook` builds it first.

### The groups in the sidebar

| group        | holds                                                                        |
| ------------ | ---------------------------------------------------------------------------- |
| `Tour`       | the product in the order a person meets it, for one, two and many printers   |
| `Screens`    | every screen the app routes to, in each of its states                        |
| `Parts`      | the larger parts a screen is built from, from the app and the library        |
| `Shell`      | the frame around a screen: shell, rail, phone bar, footer, menus and dialogs |
| `Primitives` | the small components from the library                                        |

The `Tour` pages are in `src/tour/`. A screen's stories are in
`src/lib/screens/<screen>.stories.svelte`, beside the screen. Every other story
file is beside the component it shows.

### How a screen story runs the real app

`AppAt` in `test/story/app-at.svelte` mounts the real `App` at one address.
It replaces `fetch` with the story core in `test/story/api.ts`, which answers
from the UX fixture table, `test/ux/fixtures/api.json`. When the story closes,
`AppAt` restores the real `fetch` and the Storybook address.

```svelte
<AppAt path="/jobs" overrides={{ 'GET /api/v1/jobs': emptyList }} />
```

- `path` is the address the app opens.
- `size` sets the fleet size: `one`, `two` or `many`. Without it, `many`.
- `overrides` replaces rows of the table. A key is `"<METHOD> <path>"`, with the
  query added when the row answers one query only. A value is a `Response` or a
  function that returns one.

`api.ts` exports helpers for the usual states:

| helper        | answers                                 |
| ------------- | --------------------------------------- |
| `pending()`   | never, so a loading state stays loading |
| `refused()`   | 403, for a person without the grant     |
| `failed()`    | 500                                     |
| `emptyList()` | a list with no items                    |

The event stream, `GET /api/v1/events`, stays open unless a story overrides it.
A stream that closes would show the connection as lost on every screen.

### The fleet-stats module on the Module page

`Screens/Module page` shows a page that an installed module adds. Storybook
serves the built fleet-stats bundle at `modules/fleet-stats/`. The overrides
`withFleetStats` in `test/story/fleet-stats.ts` make the story core report the
module as installed. Spread them into a story's own overrides.

### A new screen or component needs a story

[The Storybook rule](../.agents/rules/storybook.md) is binding, and two tests
hold it:

- `src/lib/screens-storied.test.ts` fails when a screen in `SCREENS` has no
  `<screen>.stories.svelte` beside it. It also fails when the file has no
  story named `Loading`, `Failed` or `Refused`, or `Empty` for a screen that
  is a list. A state the screen cannot show is named in the top comment, on
  a line of its own: `Omitted Refused: <reason>`
- `src/lib/stories-cover-components.test.ts` fails when a component that a
  barrel `index.ts` exports is imported by no story file

Write a screen's file with `AppAt` at the screen's path, one story for each
state the screen has.
