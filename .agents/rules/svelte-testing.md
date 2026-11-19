---
paths:
  - "**/*.svelte"
  - "**/*.svelte.ts"
  - "**/*.svelte.test.ts"
---

# Svelte testing and performance

We use Svelte to get good performance and good practices. A screen that misses
a budget below has failed on its own terms. General test shapes are in
`engineering-standards.md`. This file adds what Svelte 5 demands.

## Tools

| layer | tool |
|---|---|
| stores, translator, services | Vitest, `*.svelte.test.ts` when runes are involved |
| component behaviour | Vitest in browser mode with `vitest-browser-svelte` on the Playwright provider |
| user journeys, measurement across navigations | Playwright, approved and expected |

A component test runs in a real Chromium, never in jsdom. jsdom is a copy of
the browser's document model that runs in Node without a real browser. The
test renders with `render()` from `vitest-browser-svelte`, finds elements
through locators and asserts with `expect.element`. Svelte's own testing guide
recommends this provider. jsdom has no layout, so it cannot see overflow,
focus landing off-screen or a CSP refusal. A test that passes there says
nothing about the browser. Browser mode gives a component test the engine the
Playwright layer drives, at unit-test speed.

A test of what a finger gets (`pointer: coarse`) goes in a
`*.touch.svelte.test.ts` file, which runs in a page of its own with a touch
screen. Never switch touch emulation on over CDP. Switched off again, it leaves
a headless Chromium on Linux with no hover and no pointer for every later file
in that page. Every `hover:` rule then stops matching there.

Rune tests wrap effect-driven code in `$effect.root(...)` and flush with
`flushSync()`. Dispose the root at the end.

## Every component gets tests

Never a token render assertion. Test behaviour:

- keyboard navigation, focus management, focus trapping in dialogs
- `aria-*` state transitions: `aria-busy` while loading, `aria-pressed`, `aria-expanded`, labels on icon-only controls
- disabled and loading states really block interaction. A click while `loading` must not fire the callback
- variants render differently. Assert the class or attribute that tells them apart, never only "it rendered"

## Store tests are first-class

Class stores are plain classes, so most of this needs no DOM. Required
coverage per store:

- initial state
- every setter's validation path. A validating setter is proven to reject bad input (state unchanged, nothing persisted). Accepting good input alone proves nothing
- derived correctness against values you computed yourself and wrote as literals
- error and loading transitions, including the difference between not loaded and empty

## Write behaviour: count recomputations

The single most valuable test class here. Svelte 5 skips downstream updates
when a new value is identical to the previous one, by reference or by
primitive value. A store that reassigns an object on every write silently
defeats that and causes a cascade of recomputation. Only a counter catches it.

- assert how many times an effect fires or a derived recomputes for a given write sequence. Replacing 100 rows as a whole is one recomputation, never 100. A write that changes nothing is zero
- assert reference stability. An unrelated field write (a `loading` flip) must not replace the `rows` reference

Do not test that Svelte itself works. "`$derived` recomputes when its
dependency changes" tests the framework. Count our store's write behaviour.

## Derived traps a review rejects

| trap | why it is wrong |
|---|---|
| `$derived` with a side effect inside | derived must be pure. The side effect runs at unpredictable times |
| `$effect` writing one `$state` from another | that is a `$derived`. The effect version produces glitchy intermediate states |
| destructuring a store or reactive object | the destructured binding is a dead snapshot. Reactivity is lost silently |
| deep `$state` on large server collections | proxy cost per row for nothing. `$state.raw` plus wholesale replace is required (`svelte-5.md`) |

## Memory leaks are a required test category

The realistic mechanisms:

- `$effect` without a cleanup return, leaving listeners, intervals or observers alive after unmount
- `{@attach}` cleanup not running
- a subscription to a long-lived singleton store never released
- an SDK event subscription without `close()`
- detached DOM retained by a chart closure

What each layer catches:

- browser-mode Vitest layer: instrument `EventTarget.addEventListener` and `removeEventListener`, and timers where relevant. Mount and unmount the component N times and assert the counts return to where they started
- that layer catches listener and timer leaks inside one page. It cannot see retained detached DOM across navigations
- Playwright layer: CDP heap snapshots (CDP is Chrome's debugging protocol) or `performance.measureUserAgentSpecificMemory()` across repeated navigations. Assert the trend is flat, never rising
- page-lifetime singletons (a store's `matchMedia` subscription) are exempt by declared design. A comment at the subscription site says so, or it counts as a leak

## Performance budgets the build enforces

Measured with Playwright tracing plus the Performance API on the built app.
Budgets are per-route ceilings, and CI fails the build when one is crossed.
The Playwright suite lands with the first real Svelte view. Until then these
numbers are the contract new code is written against.

The JavaScript budget is not one number for every app. Each app sets its own
when its first route is measured, and the rule then holds the app to that
number:

- `scripts/route-js.baseline` records it, on the model of
  `scripts/tier-paths.baseline`: one line per route, `app`, `route` and the
  gzip kilobytes of entry plus route chunks, tab separated, without lazy
  locale catalogs
- the gate reads the file. A route over its line fails. A route under its line
  fails until the line is lowered, so the number only moves down
- a route with no line is a new route, and its first measured size is the line
  its author adds in the same commit
- raising a line is a decision, not a fix. The commit body says what the
  kilobytes buy

| budget | ceiling |
|---|---|
| route JS, gzip (entry plus route chunks, without lazy locale catalogs) | the app's line in `scripts/route-js.baseline` |
| route CSS, gzip | 30 kB |
| mount-to-interactive on the fleet route, mid-range laptop, cold | 200 ms |
| interaction latency on the heaviest screen (event list filter and scroll) | 100 ms |
| DOM nodes in the event list, whatever the row count | 400 |

- virtualisation for long lists is mandatory. Virtualisation means only the rows on screen exist in the DOM. The event list holds thousands of rows and a fleet holds up to four hundred printers
- the DOM-node budget is also the anti-regression test. A Playwright test renders 5k rows and asserts the node count stays bounded. That is the test that stops someone quietly removing virtualisation later
- locale catalogs beyond English stay lazy chunks (`svelte-5.md`). A catalog appearing in the entry chunk is a budget regression
- compare built CSS and JS size before and after a restructure (`frontend-structure.md`). Duplicated rules across chunks are a bug

## Open items

- `scripts/route-js.baseline` does not exist. It is written with the first measured route
