import type { AlertVariant } from './types';

// Static strings chosen by key (svelte-5.md). Tailwind reads this file, so a
// utility written here is emitted; one assembled at run time is not.

// The grid is two columns. The first is zero wide until a direct `svg` child
// appears, and `with-icon` then opens it to one icon, so a caller adds an
// icon without saying so anywhere else.
export const base =
  'relative grid w-full grid-cols-alert items-start gap-y-0.5 border px-4 py-3 text-sm with-icon:grid-cols-alert-icon with-icon:gap-x-3 [&>svg]:size-4 [&>svg]:translate-y-0.5 [&>svg]:text-current rounded-md';

// Each status is a hairline border in its own colour over the shared bg-card
// fill. The text reads a `-text` token, tuned for bg-card, rather than a
// `-foreground` one, which is tuned for a saturated fill. Measured contrast
// against bg-card, light/dark: default 16.42/15.09, destructive 5.21/7.97,
// success 5.05/8.67, warning 10.98/12.00, info 5.17/8.49, all clear of WCAG AA
// at 4.5:1.
export const variants: Record<AlertVariant, string> = {
  default: 'border-border bg-card text-card-foreground',
  destructive: 'border-destructive bg-card text-destructive-text',
  success: 'border-success bg-card text-success-text',
  warning: 'border-warning bg-card text-warning-text',
  info: 'border-info bg-card text-info-text',
};
