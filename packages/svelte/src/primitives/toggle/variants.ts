import type {
  ToggleItemSize,
  ToggleSize,
  ToggleTone,
  ToggleVariant,
} from './types';

// Static strings chosen by key (svelte-5.md). Tailwind reads this file, so a
// utility written here is emitted; one assembled at run time is not.

// What both variants share. The pressed fill is the tone's, below.
//
// Three selectors are compound on purpose. A plain `disabled:bg-disabled` ties
// with `state-on:bg-selected` on specificity and loses, so a disabled
// pressed toggle would keep the brand fill and read as usable. A plain
// `active:text-secondary-foreground` ties with `state-on:text-selected-foreground`
// the same way, and what a tie loses to there is the selected colour on itself.
//
// `cursor-pointer` closes the Tailwind v4 Preflight gap that `button.svelte`
// closes too, with the same `disabled:cursor-default` override.
export const base =
  "inline-flex pointer-coarse:min-h-touch-target pointer-coarse:min-w-touch-target @max-3xl/app-shell:min-h-touch-target @max-3xl/app-shell:min-w-touch-target cursor-pointer items-center justify-center gap-2 text-sm font-medium whitespace-nowrap transition-colors outline-none disabled:pointer-events-none disabled:cursor-default disabled:text-disabled-foreground disabled:state-on:border-transparent disabled:state-on:bg-disabled disabled:state-on:text-disabled-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4";

// The pressed fill. `primary` is the primary button's fill and its hover, and
// takes the primary button's text: dark text on the orange measures 6.47:1 in
// the dark theme, where white would measure 2.96:1.
export const tones: Record<ToggleTone, string> = {
  default:
    'state-on:border-selected state-on:bg-selected state-on:text-selected-foreground state-on:hover:border-selected-hovered state-on:hover:bg-selected-hovered state-on:active:bg-selected-hovered state-on:active:text-selected-foreground',
  primary:
    'state-on:border-button-accent state-on:bg-button-accent state-on:text-primary-foreground state-on:hover:border-button-accent-hovered state-on:hover:bg-button-accent-hovered state-on:active:bg-primary-pressed state-on:active:text-primary-foreground',
};

// The corner, apart from the rest. Nothing here merges conflicting utilities,
// so a component that takes this recipe and squares its corners, as
// `toggle-group-item` does, has to be able to leave this one out: two
// border-radius utilities on one element are settled by the order of the
// stylesheet and `rounded-sm` is the later of the two.
export const radius = 'rounded-sm';

// The resting pair is Button's `ghost`: `bg-transparent` on its own left the
// text unreadable in the dark theme. The `active:` fill is for the off state,
// and it is what says a tap landed on a tablet, where `hover:` never fires.
// secondary-foreground on secondary measures 9.98:1 light and 11.98:1 dark.
export const variants: Record<ToggleVariant, string> = {
  default:
    'border border-transparent bg-transparent text-muted-foreground hover:bg-accent hover:text-foreground active:bg-secondary active:text-secondary-foreground',
  outline:
    'border border-border bg-transparent text-muted-foreground hover:bg-accent hover:text-foreground active:bg-secondary active:text-secondary-foreground',
};

// The heights are Button's. `min-w-*` holds a one-character label to the same
// minimum touch target as a wide one.
export const sizes: Record<ToggleSize, string> = {
  sm: 'h-control-height min-w-control-height px-2 text-xs',
  md: 'h-9 min-w-9 px-2.5 text-sm',
  lg: 'h-11 min-w-11 px-3 text-base',
};

// A group item's steps: the toggle's heights with 12px of inline padding at
// every one, so the options of one control read as one row. `icon` is a
// square of the input's height with no padding, for a glyph alone.
export const itemSizes: Record<ToggleItemSize, string> = {
  sm: 'h-control-height min-w-control-height px-3 text-xs',
  md: 'h-9 min-w-9 px-3 text-sm',
  lg: 'h-11 min-w-11 px-3 text-base',
  icon: 'h-input-height min-w-input-height text-xs',
};
