/**
 * The one recipe every control that opens a list of choices wears: a select's
 * trigger and a facet popover's, which are the same object to a reader. A
 * bordered box with its subject on the left and a chevron on the right, which
 * drops something open when pressed.
 *
 * They used to be two recipes that had drifted: the facet was 120px at its
 * narrowest and tinted on hover, the select was as wide as its own value and
 * did not, and at `lg` one was 16px type and the other 14px. On the printers
 * screen both sit in the same row, which is where a reader sees the drift as
 * two kinds of control rather than one control twice.
 *
 * Width is deliberately not in here. A facet carries a floor and a cap of its
 * own so a row of filters reads as a set; a select is as wide as the value it
 * holds, and forcing that floor on it overflowed the printer file toolbar at
 * 320px by 19px (measured). Everything that makes the two look like one
 * control is shared; only how wide they choose to be is not.
 *
 * What is not here: the picked-state fill. A facet fills itself when it
 * carries a value, a select never does, because its value is written in the
 * trigger instead. The facet popover appends that itself.
 */
import type { ControlSize } from './types';

/**
 * The height, inset and type of one control at each step of the scale. Shared
 * so a text field, a select and a facet standing in the same toolbar are the
 * same object three times rather than three sizes of control. The input reads
 * this as well as the trigger recipe below.
 */
export const CONTROL_SIZES: Record<ControlSize, string> = {
  sm: 'pointer-coarse:min-h-touch-target @max-3xl/app-shell:min-h-touch-target h-input-height px-control-inline text-sm',
  md: 'pointer-coarse:min-h-touch-target @max-3xl/app-shell:min-h-touch-target text-sm h-input-height px-control-inline',
  lg: 'pointer-coarse:min-h-touch-target @max-3xl/app-shell:min-h-touch-target h-input-height px-control-inline text-sm',
};

/**
 * The trigger itself, without its size, its fill or its focus ring.
 *
 * `cursor-pointer` with `disabled:cursor-default`: Tailwind v4's Preflight
 * dropped the old `button { cursor: pointer }` reset.
 */
export const CONTROL_TRIGGER =
  "flex w-fit cursor-pointer items-center justify-between gap-2 rounded-sm border border-input not-data-on:not-aria-invalid:border-secondary font-normal whitespace-nowrap transition-colors outline-none disabled:pointer-events-none disabled:cursor-default disabled:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground";

/**
 * The fill and the text colour, listed beside the trigger by every caller. A
 * select in another tone takes that tone's fill in their place.
 */
export const CONTROL_FILL =
  'bg-transparent not-data-on:bg-secondary not-data-on:hover:bg-secondary text-foreground hover:bg-accent';

/**
 * The focus ring, listed beside the trigger by every caller but the select,
 * which draws focus as a border the way a text field does.
 */
export const CONTROL_RING =
  'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background';
