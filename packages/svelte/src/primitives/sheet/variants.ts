import type { SheetSide } from './types';

/**
 * Where the panel sits and which way it travels.
 *
 * A panel on the left or the right takes three quarters of the width, up to
 * the `sm` breakpoint. One on the top or the bottom is as tall as its content.
 * The border is only on the edge that faces the page.
 */
export const sides: Record<SheetSide, string> = {
  top: 'state-open:animate-slide-in-from-top state-closed:animate-slide-out-to-top inset-x-0 top-0 h-auto border-b',
  right:
    'state-open:animate-slide-in-from-right state-closed:animate-slide-out-to-right inset-y-0 right-0 h-full w-3/4 border-l sm:max-w-sm',
  bottom:
    'state-open:animate-slide-in-from-bottom state-closed:animate-slide-out-to-bottom inset-x-0 bottom-0 h-auto border-t',
  left: 'state-open:animate-slide-in-from-left state-closed:animate-slide-out-to-left inset-y-0 left-0 h-full w-3/4 border-r sm:max-w-sm',
};
