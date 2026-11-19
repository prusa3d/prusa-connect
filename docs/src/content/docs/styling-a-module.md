---
title: Styling a module
description: How a module builds its own Tailwind, what the host page's theme already provides, and what the utilities add to the bundle.
audience: extend
rewrite:
  describes: current
  implementedBy:
    - packages/svelte/build/module.ts
    - packages/svelte/src/styles/tokens.css
    - examples/spools/ui
    - examples/nozzle-watch/ui
---

A module's bundle carries the Tailwind utilities its own markup names. The
values behind them come from the host page, which already loads the theme. Both
halves meet in the browser, and the module ships one of them.

## What the host page already provides

The app loads `@prusa/connect-ui/tokens.css` once. That stylesheet puts
every colour, spacing step, radius, font size, duration and easing on the
page's `:root`, as a CSS custom property. It also declares the `dark` variant
and the library's own utilities.

A module reads those values and never ships them again. A second copy could
disagree with the first, and a reader would see one card in the old palette
beside one in the new.

| what a module draws with | where it comes from |
| --- | --- |
| the theme's custom properties, for light and dark | the host page |
| the `dark` variant and the library's `@utility` definitions | the host page |
| one rule per utility class the module's markup names | the module's own bundle |
| one rule per utility class the library's components name, variants included | the module's own bundle |

## The stylesheet a module writes

One file, and the module's entry imports it. It holds four statements, the last of them two `@source` lines:

```css
@reference 'tailwindcss';
@reference '@prusa/connect-ui/tokens.css';
@layer utilities {
  @tailwind utilities;
}
@source './';
@source '../node_modules/@prusa/connect-ui/dist';
```

| line | what it does |
| --- | --- |
| `@reference 'tailwindcss'` | brings Tailwind's own theme into scope. Without it the build stops on `--spacing` |
| `@reference '@prusa/connect-ui/tokens.css'` | brings the library's theme, its utilities and its `dark` variant into scope |
| `@tailwind utilities` inside `@layer utilities` | the only statement that emits bytes, in the same layer as the host's own utilities |
| `@source './'` | names the tree Tailwind scans for class names |
| `@source '../node_modules/@prusa/connect-ui/dist'` | scans the library's components too, as the next paragraph explains |

The library's components name variants such as `@md/pane:` and `md:`, and those
have to be in this sheet. Its scoped `.flex-col` beats the host's
`@md/pane:flex-row` on any element inside the module, so a variant left to the
host never applies. The scan adds about 16 kB gzipped to a bundle.

Neither `@reference` emits anything. Each one tells Tailwind what a name means,
so `text-muted-foreground` compiles to `color: var(--muted-foreground)`, and the
host page resolves the variable.

The entry imports the file like any other module:

```ts
import { defineModule } from '@prusa/connect-ui/slots';

import SpoolsPage from './spools-page.svelte';
import './styles.css';

export default defineModule((host) => {
  host.register.page('spools', SpoolsPage);
});
```

`moduleBuild()` runs Tailwind over it and puts the result in the bundle.
Nothing else has to be configured, and the manifest gains no new key.

## Why the app's own scan cannot reach a module

The app's stylesheet names the trees Tailwind scans: the app's own source, and
the library's `dist`. A module is in neither, and the app is built long before
any module exists. So a utility that only a module's markup names is compiled
by nobody, and the class would reach the browser with no rule behind it.

This is why the module builds its own Tailwind instead of asking the app to
scan one more directory.

## What the module's styles cost

The CSS travels inside the entry file, as a `<style>` element the bundle adds
to the document when the browser evaluates it. The element's id is a digest of
the CSS, and the bundle skips the work when an element with that id is already
there. One bundle reached at two addresses styles the page once, and two
modules get two elements.

Measured on the two example modules in `examples/`:

| module | utility rules | CSS bytes | gzipped |
| --- | --- | --- | --- |
| `spools` | 26 | 1,638 | 726 |
| `nozzle-watch` | 17 | 1,819 | 749 |

The spools bundle's JavaScript is 9,514 bytes beside that, or 2,754 gzipped.
The CSS grows with the number of distinct utilities and not with the amount of
markup, so a class used forty times is one rule.

Tailwind reads the whole project directory and not the `src` tree alone. A
class name written in a comment or a README is still a class name, and it
compiles. Files named in `.gitignore` are skipped, so the previous build's
`dist` is not scanned.

## A preflight would restyle the host page

The obvious first line to write is `@import 'tailwindcss'`. That line includes
Tailwind's preflight, which is about 26 kB of resets for margins, borders, font
sizes and the box model. The app already has its own. A second set, added by a
module, changes the page around it.

The four statements above never emit a preflight. `@tailwind utilities` is the
utilities alone.

## An unscoped utility would beat the host's variants

The host page has its own `.flex`, `.block` and `md:grid`, in the same
utilities layer. A module's stylesheet arrives later, so its `.flex` wins over
the host's `md:grid` whenever both match one element, and the host's layout
breaks on every screen. So `moduleBuild()` wraps the module's CSS in
`@scope ([data-module])`, and its rules reach only the elements inside the
module's own slot, which carry `data-module`. The stylesheet does not write the
scope itself. A second `@scope ([data-module])` inside the first matches only a
slot inside a slot. The module's utilities then stop applying.

A portal draws content at the end of the page, outside the slot. The list of a
`Select` and the body of a `Dialog` are drawn this way. The module's own
utilities do not reach them, and the library's components there keep the
host's styling.

A portalled panel can carry the module's `data-module` itself, as a
`DialogContent` does. It then receives the module's utilities on everything
inside it, but not on the panel. A scoped rule matches the elements under the
scope's root and never the root. A class written on the panel therefore gets
a rule only when the host emits it too. Cap a dialog's height with the library's
`DIALOG_SCROLLS`, or a theme class such as `max-h-dialog-height`, and never
with a class only the module names.

## A class outside the theme still compiles to nothing

The library's theme resets several of Tailwind's default scales, so their steps
no longer exist. `rounded-4xl` is one. It emits no rule, the build says
nothing, and the corner stays square.

The radius scale is named after the thing being rounded, and
`.agents/rules/frontend-css.md` carries the list:

| class | radius |
| --- | --- |
| `rounded-badge` | 2 px |
| `rounded-button` | 6 px |
| `rounded-card`, `rounded-input` | 8 px |
| `rounded-dialog` | 10 px |

A value the theme does not define belongs in the theme, once, and never in an
arbitrary class such as `p-[13px]`. A module that needs a token the library
does not have should ask for it.

## What to read next

| document | for |
| --- | --- |
| [writing a service module](./writing-a-module.md) | the whole module, from an empty directory to an install |
| `examples/spools/ui/src/styles.css` | the four statements in a module that ships |
| `.agents/rules/frontend-css.md` | the rules a component's markup is held to |
