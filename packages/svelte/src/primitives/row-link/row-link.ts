/**
 * One row, one destination: clicking anywhere on a row opens what the row is
 * about.
 *
 * Three class strings rather than a component. A `<tr>` admits no wrapper
 * element between itself and its cells, so the pattern has to arrive as
 * classes: the row becomes the containing block, the row's own primary
 * control paints an empty box over the whole of it, and anything else
 * interactive in the row is lifted above that box.
 *
 * One control, one tab stop. The overlay belongs on the link or button the row
 * already has, never on a new element and never on the row itself. A
 * whole-row click handler is unreachable by keyboard and invisible to a screen
 * reader, and a second anchor covering the row would announce every row twice.
 *
 * `position: relative` on a `<tr>` does hold under `border-collapse:
 * collapse`: measured in Chromium against a listing table, an overlay in
 * the name cell resolves to the row's own box (1262x76 against the row's
 * 1262x77, the difference being the collapsed bottom border) and answers
 * `elementFromPoint` at the row's left edge, its centre and its trailing end.
 * The cells' own contents stay selectable because the overlay carries no text
 * of its own.
 */

/**
 * On the row: makes it the overlay's containing block and shows the row is
 * activatable.
 *
 * `isolate` so `ABOVE_ROW_LINK`'s `z-10` means "above this row's own overlay"
 * and nothing more. Without it those controls join the root stacking context,
 * where they tie with the app shell's own `z-10` header and, being later in
 * the document, paint over it. A row's key glyph and delete button showed
 * through the header as the list scrolled under it.
 */
export const LINKED_ROW =
  'relative isolate cursor-pointer with-link-focus:bg-accent with-link-focus:ring-2 with-link-focus:ring-inset with-link-focus:ring-ring with-button-focus:bg-accent';

/**
 * On the row's primary link or button: stretches its hit area over the whole
 * row.
 *
 * A pseudo-element exists only with a `content` value. Tailwind's `after:`
 * variant writes `content: var(--tw-content)`, which is the empty string until
 * a `content-*` utility sets it, so the hit area needs no utility of its own.
 */
export const ROW_LINK = 'after:absolute after:inset-0';

/** On any other control in the row: an actions menu, a preview trigger, a second link. */
export const ABOVE_ROW_LINK = 'relative z-10';
