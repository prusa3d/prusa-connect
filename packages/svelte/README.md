# @prusa/connect-ui

The Svelte components that Prusa Connect's own pages use. A module's
page uses the same components, so it looks like part of the product. The
package needs Svelte 5 and Tailwind 4 and no other framework; plain Vite is
enough to build with it. It talks to the core only through
`@prusa/connect`.

## What is in it

| subpath        | holds                                                                                                                 |
| -------------- | --------------------------------------------------------------------------------------------------------------------- |
| `./tokens.css` | the theme: every colour in OKLCH, light and dark, radii, the type and spacing rhythm, durations, easings              |
| `./primitives` | the small parts every screen is built from, named under [the primitives](#the-primitives)                             |
| `./data`       | the stores over the SDK: the fleet, a printer, registrations, accounts, storage, transfers, links, cameras, telemetry |
| `./slots`      | the ten slots, the registry, the host object and the module loader                                                    |
| `./i18n`       | every text as a key: `t`, the locale store, `describeFault`, and the `Intl` formatters                                |
| `./domain`     | plain TypeScript rules: lifecycle, heating, notification, printer state                                               |
| `./router`     | the route matcher: pattern matching, history and the current match, with no route table or path literal in it         |
| `./shell`      | whole screens, and the frame they sit in: header, rail, phone bar, footer and the toggles between them                |

The root export re-exports `./domain`, `./i18n`, `./primitives`, `./data` and
`./slots`, and carries the theme store as well. `./router` and
`./shell` are each imported from their own subpath.

## The primitives

`./primitives` holds the small parts every screen is built from.

The specification for them is a React storybook, which is not in this
repository. It is its own project, and a contributor reads it beside the
Svelte source rather than guessing what a component should look like. A state
it draws and a Svelte component does not is an unfinished component, and the
empty, loading and refused states count.

There are 74 of them. `Art`, `GuidedDialog`, `KeyValueList`, `SettingsPane`,
`StatusCard` and `Stepper` have no React counterpart and compose the ones that
do.

`GuidedDialog` is the shape every flow with more than one question takes. It
has a title, "Step n of m", and one question or one wait per step. Below that
are Back and one primary button. At the end is a success or a failure screen,
with room for a help link.
The step number belongs to the caller, because which step comes next depends on
the answer.

`Art` draws the line drawings an empty page shows instead of a grey box.
`StatusCard` is one reading with a title, a trend and room for a badge and an
action. `SettingsPane` is a titled panel with a row per setting and a footer.
`KeyValueList` is a list of names and values, each value copyable.

```svelte
<script lang="ts">
  import { StatusCard } from '@prusa/connect-ui/primitives';
</script>

<StatusCard
  title="Bench one"
  tone="active"
  value="52 %"
  caption="tower.gcode"
/>
```

## The domain rules

`./domain` holds the rules about printing, as plain functions. There is no
network code and no component in it, so it runs under Node and in tests
without a browser. `heaterState` compares a heater with its target.
`projectLifecycleVerb` says whether a command such as pause is available,
disabled or absent for the printer's current state, and why. Other functions
give a notification its tone and a printer state its severity. The functions
take the SDK's generated types, so you pass a device record in exactly as the
API returns it.

```ts
import { heaterState, projectLifecycleVerb } from '@prusa/connect-ui/domain';

heaterState({ current: 234.6, target: 235 }); // 'at-temperature'
projectLifecycleVerb('pause_print', {
  capabilities: device.capabilities,
  state: 'IDLE',
}); // { kind: 'disabled', reason: 'Available when printing.' }
```

## The data stores

`./data` holds the stores over the SDK. Nothing in it calls `fetch`,
`XMLHttpRequest` or `EventSource`; every request goes through
`@prusa/connect`.

`Fleet` holds the printers. It loads them once, no matter how many screens
show them, and updates them from the event stream. Rows are `$state.raw` and
are replaced as a whole, so a fleet of four hundred printers does not create
a reactive proxy per row. `listen()` opens the stream and returns the
function that closes it.

```ts
import { createFleet } from '@prusa/connect-ui/data';

const fleet = createFleet(client);
await fleet.load();
const close = fleet.listen();
fleet.device('01JB8XN…')?.name;
```

`readPrincipal(client)` returns who is signed in and which grants they have.
`createModuleStore(namespace)` is a module's own key-value store. Today it
writes to `localStorage` under the namespace, because the core has no
per-key state route yet. The API of the store will stay the same when it
moves to the SDK, so a module written against it will not need to change.

## The slots

`./slots` is how a module adds to the interface. A module can fill any of
ten slots; the core's resources listing says what each module may register
(ADR-20260911-4p6). The host reads `GET /api/v1/resources`
and checks each module's `library` range against `LIBRARY_VERSION`. It
imports the module's entry file the first time one of its slots or routes is
rendered, and passes the bundle one host object.

```ts
import { createFleet } from '@prusa/connect-ui/data';
import {
  createModuleLoader,
  setSlotContext,
  SlotRegistry,
} from '@prusa/connect-ui/slots';

const registry = new SlotRegistry();
const loader = createModuleLoader({ client, registry, fleet });
setSlotContext({ registry, loader });
```

The app renders the outlets, and module items come after the app's own:

```svelte
<script lang="ts">
  import {
    ModulePage,
    SlotOutlet,
    withModuleNav,
  } from '@prusa/connect-ui/slots';
</script>

<SlotOutlet screen="printers" name="device-card" deviceId={device.id} />
<SlotOutlet screen="settings" name="settings" />
<ModulePage namespace="spools" path="spools" />
```

A module's bundle default-exports one function:

```ts
import { defineModule } from '@prusa/connect-ui/slots';

export default defineModule((host) => {
  host.register.page('spools', SpoolsPage);
  host.register.deviceCard('spool-status', SpoolSection);
});
```

`host` has five fields: `client` (the app's `PrusaConnect` in session mode),
`principal`, `register`, `store` and `fleet`. It has no `fetch` and no token.
A bundle can reach exactly what the signed-in person can reach, and the core
authorizes every request.

Registering something the module's `contributes` block did not declare
throws, and the whole module is refused. Refusals are kept as state: read
`loader.refusals`, and use `describeRefusal(refusal)` for the sentence a
settings screen shows. Each page renders inside an error boundary, and a
section that throws is removed without breaking the card around it.

## Using it

Three lines in your stylesheet, then the components:

```css
@import 'tailwindcss';
@source '../node_modules/@prusa/connect-ui/dist';
@import '@prusa/connect-ui/tokens.css';
```

```svelte
<script lang="ts">
  import { Button, Card } from '@prusa/connect-ui/primitives';
</script>

<Card title="Printer">
  <Button onclick={() => pause()}>Pause</Button>
</Card>
```

Tailwind does not scan `node_modules`, so without the `@source` line the
components' utility classes would be missing from your build. The dark theme
is the `dark` class on an ancestor element.

The tests measure every pair of colours that meets on screen against WCAG AA,
in the light theme and in the dark one.

The interface family is Inter, with the platform's own faces behind it.
The theme names it in `--font-sans` and ships no file, so an application serves
the font itself. A reading that changes in place is set with `tabular-nums`,
which answers with digits of one width.

## Building it here

```sh
make client-ui-check   # lint, formatting, svelte-check, the tests
make client-ui-build   # writes dist/
```

Inside this directory, `pnpm run check` runs the same gates. The component
tests run in a real Chromium through Playwright; `pnpm exec playwright install
chromium` fetches one. The token tests run under Node and hold every text pair
the theme declares to WCAG AA.

## Storybook

The Storybook that shows the whole product is in `frontend/`. It reads the
stories in this package as well, under `Parts`, `Shell` and `Primitives`, so a
story written here appears there too. `frontend/README.md` says how to run it.

This package keeps its own Storybook for work on the library alone. It starts
faster and does not need the app:

```sh
pnpm --dir packages/svelte run storybook        # http://localhost:6006
pnpm --dir packages/svelte run build-storybook  # writes storybook-static/, which make client-ui-check builds
```

A story file sits beside the component it shows, as `<name>.stories.svelte`.
Every component a barrel exports needs one, by
[the Storybook rule](../../.agents/rules/storybook.md).
`src/stories-cover-components.test.ts` fails on a component that no story
file imports. `stories.baseline` lists the components that have no story yet,
and the test also fails on a line whose component now has one, so the list
only shrinks.
