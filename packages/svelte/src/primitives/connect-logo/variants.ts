import type { ConnectLogoShape, ConnectLogoSize } from './types';

// Static strings chosen by key (svelte-5.md). Tailwind reads this file, so a
// utility written here is emitted; one assembled at run time is not.

// Only the height is set. An inline SVG with a viewBox and no width draws as
// wide as that box's ratio, so each shape keeps its own proportions.
export const base = 'block w-auto shrink-0';

export const sizes: Record<ConnectLogoSize, string> = {
  xs: 'h-4.5',
  sm: 'h-6',
  md: 'h-9',
  lg: 'h-12',
};

/** Each shape is drawn in its own box. */
export const viewBoxes: Record<ConnectLogoShape, string> = {
  prusa: '0 0 69.87 16.32',
  full: '0 0 124 25',
  mark: '0 0 179.54 179.54',
};
