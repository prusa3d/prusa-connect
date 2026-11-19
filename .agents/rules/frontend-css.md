---
paths:
  - "**/*.css"
  - "**/*.html"
  - "**/*.svelte"
  - "**/tailwind.config.*"
---

# Frontend CSS

Tailwind is the CSS tool here. A component is styled by small utility classes
in its markup (`p-4`, `text-primary`), and the theme is the one file that says
what each utility means.

## Tailwind, with the theme as the only source of values

- every colour, spacing, radius, font size, duration and easing lives in the
  Tailwind theme, as a CSS custom property the theme exposes. A component reads
  the theme and never restates a value
- no arbitrary values. `p-[13px]`, `text-[#333]` and `w-[calc(...)]` are
  refused in review. A value the theme lacks is added to the theme, once
- a component is composed from utilities in its markup. A repeated group of
  utilities becomes a component or an `@apply` class in the component's own
  `<style>`, never a global stylesheet
- one custom property carries a value from the script into the styles, on the
  model below. A class string built in the script is not a style

## OKLCH colours only

OKLCH is a way of writing a colour by lightness, chroma and hue, so two
colours with the same lightness number look equally bright.

- all colours as oklch tokens in the theme. No hex, rgb or hsl values written
  in a component
- use the theme's names: `bg-surface`, `text-primary`, and so on, backed by
  `--color-primary`, `--color-surface`

## Radii

- the design system's scale, and a component names the thing it rounds, not
  a step: `rounded-badge` 2 px, `rounded-button` 6 px, `rounded-card` and
  `rounded-input` 8 px, `rounded-dialog` 10 px. Nothing is rounder than a
  dialog

## Logical properties

A logical property says "start" and "end" instead of "left" and "right", so
the layout mirrors itself for a right-to-left language.

- use the logical utilities: `ms-`, `me-`, `ps-`, `pe-`, `mbs-`, `mbe-`,
  `start-`, `end-`, `border-s-`. No directional utilities
- this keeps right-to-left and left-to-right layouts compatible

## Container queries

A container query styles a component by the size of the box it sits in, not
by the size of the window.

- prefer container queries to viewport breakpoints for responsive components.
  Mark the parent with `@container` and style the children with `@md:` and
  `@lg:`, which apply at that container size and up
- `@max-md:` applies below a container size, and `@sm:@max-md:` targets a
  range
- nested containers are named: `@container/main` on the parent, `@sm/main:`
  on the child, so a variant says which container it reads
- a component should not assume where on the page it is placed

## Style math belongs to CSS, app logic belongs to the script

If the answer is a length, a colour, a count of columns or a duration, CSS
computes it. JavaScript computes what the app means, never what it looks
like.

```svelte
<div class="grid" style="--rows: {rows}">
```

```css
.grid {
  grid-template-rows: repeat(var(--rows), minmax(0, 1fr));
  height: min(calc(var(--base-height) / var(--divider)), 100vh);
}
```

- `calc()`, `min()`, `max()`, `clamp()`, `round()`, custom properties,
  `aspect-ratio`, `attr()` and relative colour syntax replace almost every
  layout number a component once computed in JS
- the browser evaluates these in the style pass. The JS version costs a
  script execution, a layout read and a write, on every frame when it is
  wired to resize or scroll. The layout read often forces the browser to lay
  the page out first
- the framework cost is the real one. A `$derived` returning a pixel value
  re-runs the reactive graph, re-renders and re-writes an inline style on every
  dependency change. A `clamp()` re-runs nothing. Do not carry a React
  measure-and-set habit into Svelte
- pass the input to CSS, never the result. Set `style="--rows: {n}"` from the
  script and let the stylesheet do the arithmetic. One custom property beats a
  computed style string
- a design token inside a `calc()` stays a token. Never write the resolved
  number "because it is faster"

### When JS is really required

- a value only the layout knows and CSS cannot express: `ResizeObserver`
  where container queries do not reach, windowing in a virtualised list,
  canvas sizing
- a browser bug CSS cannot work around. The comment at the site records each
  CSS-only attempt that failed, which is the standard of evidence required
- reading a measurement to send somewhere: a metric, a persisted pane width

In those cases leave a one-line comment saying what CSS could not do, so the
next reader does not "simplify" it back into a broken layout.

## Forbidden

- `transform: translate()` in hover states, except drag ghosts
- border *width* changes on hover. Use outline or box-shadow. A colour
  change is allowed: the box does not reflow
- `!important`, except to outrank an inline style that a library or the
  browser writes, which no selector can. The comment at the site names the
  writer
- inline styles beyond a custom property that carries an input
- pill-shaped badges. Use less rounded

## Transitions

- token-based: the theme's `--duration-*` and `--ease-*` values through
  `duration-` and `ease-` utilities
- respect `prefers-reduced-motion` with the `motion-reduce:` variant
