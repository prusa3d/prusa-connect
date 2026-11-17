import type { Snippet } from 'svelte';
import type { ClassValue, HTMLAttributes } from 'svelte/elements';

import type { HeaterState } from '../../domain';

export interface HeatReadingProps extends Omit<
  HTMLAttributes<HTMLSpanElement>,
  'class' | 'children'
> {
  /** What the heater is doing, from `heaterState` in the domain. */
  state: HeaterState;
  /**
   * A last-known reading from a printer this instance can no longer hear
   * from: no trend to report, so no tint and no arrow. A stale number's
   * direction is not one to claim.
   */
  muted?: boolean;
  class?: ClassValue | undefined;
  children: Snippet;
}

export interface TemperatureReadingProps {
  /** The glyph that names the heater. Already hidden or named by its caller. */
  icon: Snippet;
  /** The reading, or `undefined` when the printer has reported none. */
  current: number | undefined;
  /** The target, or `undefined`. Zero is a heater switched off, not a target. */
  target: number | undefined;
  /** A last-known reading from a printer this instance can no longer hear from. */
  muted?: boolean;
  /**
   * A chamber: a heater against its target, and off with none whatever the
   * air. See `chamberState`.
   */
  chamber?: boolean;
  class?: ClassValue | undefined;
}
