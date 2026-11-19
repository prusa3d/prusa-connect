import type {
  ButtonIconSize,
  ButtonRadius,
  ButtonTextSize,
  ButtonVariant,
} from './types';

// Static strings chosen by key (svelte-5.md). Tailwind reads this file, so a
// utility written here is emitted; one assembled at run time is not.

// `base` carries no corner: `radii` adds it, so a round button does not stack
// a second radius over the first.
//
// `cursor-pointer` closes a gap Tailwind v4's Preflight opened when it dropped
// the `button { cursor: pointer }` reset. `disabled:cursor-default` overrides
// it: `disabled:pointer-events-none` already stops the cursor from flipping on
// hover, but `getComputedStyle` reports the declared value whatever
// hit-testing does, so the override is what makes it measurable.
export const base =
  "inline-flex items-center justify-center gap-2 whitespace-nowrap shrink-0 cursor-pointer transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:cursor-default [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 font-normal pointer-coarse:min-h-touch-target pointer-coarse:min-w-touch-target @max-3xl/app-shell:min-h-touch-target @max-3xl/app-shell:min-w-touch-target";

// A link sits in running text, so it flows and wraps with it instead of being
// a flex box that keeps to one line. It has a base of its own rather than
// classes stacked over `base`: no tailwind-merge resolves conflicting
// utilities.
export const linkBase =
  "inline whitespace-normal shrink-0 cursor-pointer rounded-sm font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:cursor-default [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4";

// Every boxed variant (all but `link`) paints an `active:` fill, and each one
// differs from that variant's `hover:` fill as well as from its resting one. The tablet this UI
// is opened on has no hover at all, so without a press fill Stop and Delete
// printer answer a tap with nothing, and a command that takes about a second
// gets tapped twice.
//
// `destructive` is an outline, and its resting text is `text-destructive-text`
// rather than `text-destructive`: --destructive measures 4.48:1 against --card
// and --popover, under WCAG AA's 4.5:1, while --destructive-text clears every
// surface. `ghost` rests on `text-muted-foreground`, because inheriting the
// body colour reads black in the dark theme.
export const variants: Record<ButtonVariant, string> = {
  default:
    'border-primary text-primary-foreground active:border-transparent active:bg-primary-pressed disabled:border-transparent aria-disabled:border-transparent disabled:bg-disabled aria-disabled:bg-disabled disabled:text-disabled-foreground aria-disabled:text-disabled-foreground variant-default:font-bold border-0 bg-button-accent hover:border-transparent hover:bg-button-accent-hovered aria-disabled:hover:bg-button-accent-hovered aria-disabled:active:bg-primary-pressed aria-disabled:disabled:bg-disabled',
  secondary:
    'border-secondary bg-secondary text-secondary-foreground hover:bg-secondary-hovered active:border-primary active:bg-primary active:text-primary-foreground disabled:border-transparent aria-disabled:border-transparent aria-disabled:bg-disabled aria-disabled:text-disabled-foreground border-0 hover:border-transparent dark:hover:bg-button-neutral-hovered dark:active:bg-primary disabled:bg-secondary disabled:text-secondary-foreground disabled:opacity-50 hover:active:border-transparent aria-disabled:disabled:bg-secondary aria-disabled:disabled:text-secondary-foreground',
  ghost:
    'border border-transparent bg-transparent text-muted-foreground hover:bg-accent hover:text-foreground active:bg-secondary active:text-secondary-foreground disabled:text-disabled-foreground aria-disabled:text-disabled-foreground',
  destructive:
    'border border-destructive bg-transparent text-destructive-text hover:bg-destructive hover:text-destructive-foreground active:bg-destructive-hovered active:text-destructive-foreground disabled:border-transparent aria-disabled:border-transparent disabled:bg-disabled aria-disabled:bg-disabled disabled:text-disabled-foreground aria-disabled:text-disabled-foreground',
  link: 'text-link underline-offset-4 hover:underline disabled:text-disabled-foreground aria-disabled:text-disabled-foreground',
};

// A square size carries no horizontal padding: the glyph is centred by the
// base utilities and the box is as tall as it is wide. `lg` measures at least
// 44px, the floor for a control operated with a gloved hand on a wall-mounted
// tablet. `control` is `sm` in the body size, for an action standing among
// settings, and `field` is `md` at the height of the text field it stands
// beside. `avatar` is a person's picture that is pressed. Every size reaches
// 44px under a finger or in a narrow shell, from `base`.
export const sizes: Record<ButtonTextSize | ButtonIconSize, string> = {
  sm: 'text-xs h-control-height px-control-inline',
  control: 'text-sm h-control-height px-control-inline',
  compact: 'text-xs h-button-compact px-button-compact-inline',
  md: 'text-sm h-button-medium px-section-inline',
  field: 'text-sm h-input-height px-section-inline',
  lg: 'text-base h-button-large px-button-large-text-inline [&>svg:first-child]:-ms-button-icon-inset',
  'icon-sm': 'text-sm size-control-height',
  'icon-md': 'size-9 text-sm',
  'icon-lg': 'text-sm size-input-height',
  avatar: 'text-sm size-profile-avatar',
};

export const radii: Record<ButtonRadius, string> = {
  sm: 'rounded-sm',
  full: 'rounded-full',
};

// A link has no box: it takes the surrounding text's size and line, so `size`
// does not apply to it. `base` carries no font size for the same reason.
export const linkSize = 'h-auto p-0';

// A link with an icon is a flex row, so the glyph and the words sit on one
// line, and the words wrap inside the row's width.
export const iconLinkBase =
  "inline-flex max-w-full items-center gap-1.5 whitespace-normal shrink-0 cursor-pointer rounded-sm text-start font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:cursor-default [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4";
