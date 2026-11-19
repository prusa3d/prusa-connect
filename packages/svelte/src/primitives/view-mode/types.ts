import type { LucideIcon } from '@lucide/svelte';
import type { ClassValue } from 'svelte/elements';

/**
 * The shapes a listing can take, named once for the whole tree.
 *
 * There are three shapes plus the camera wall. Each has one name, one glyph
 * and one place in the order, on every screen:
 *
 * - `table`, aligned columns, for comparing values down a column.
 * - `rows`, one stacked row per item, which is what a table becomes where
 *   there is no room for columns.
 * - `cards`, a tile per item, for recognising a thing by its picture.
 * - `frames`, every camera at once. The printers screen alone has it.
 *
 * Dense first, roomy last, cameras after all of them. `viewModes()` returns
 * any subset in that sequence, so no screen can put its own two in its own
 * order by accident.
 */
export type ViewModeName = 'table' | 'rows' | 'cards' | 'frames';

export interface ViewModeOption {
  value: string;
  /**
   * The view's own name, as an accessible name. The buttons are icons only,
   * since a listing's toolbar is the narrowest row on the screen.
   */
  label: string;
  icon: LucideIcon;
  iconSrc?: string | undefined;
  /**
   * Set when this view needs more room than the listing has. The item stays,
   * marked unavailable, and says this in a tooltip.
   */
  unavailableReason?: string | undefined;
}

export interface ViewModeToggleProps {
  value: string;
  onValueChange: (next: string) => void;
  options: ViewModeOption[];
  /**
   * The group's own accessible name. Defaults to the words every screen used
   * before this component existed.
   */
  label?: string | undefined;
  class?: ClassValue | undefined;
}
