import type { Snippet } from 'svelte';
import type { ClassValue, HTMLAttributes } from 'svelte/elements';

export interface FilamentPosition {
  /** The physical position number printed on the MMU unit, 1-based. */
  position: number;
  /** Whether this position is the one currently feeding the nozzle. */
  loaded: boolean;
}

/** What kind of heater a row names. It decides which glyph the row carries. */
export type HeaterKind = 'tool' | 'bed' | 'chamber';

export interface HeaterRowProps extends Omit<
  HTMLAttributes<HTMLDivElement>,
  'class' | 'children' | 'title'
> {
  /** What this heater is called: `Tool 1`, `Bed`, `Chamber`. */
  label: string;
  /** Which glyph this row carries. */
  kind: HeaterKind;
  /** The reading, or `undefined` when the printer has reported none. */
  nozzleTemp: number | undefined;
  /**
   * The target. Ignored when `nozzleTemp` is `undefined`. With an `action` the
   * target lives on the action's own accessible name, so the row says nothing
   * about it and the two cannot disagree. Without one the row is the only
   * surface left, so it carries the target in its `title` and in a
   * screen-reader-only suffix beside the state word.
   */
  targetTemp: number | undefined;
  /**
   * What is loaded in this heater. Leave it out for a heater that cannot hold
   * anything, a bed or a chamber, and the line disappears. A tool head always
   * passes something, because "nothing is loaded" is a fact a person acts on.
   * A dash would read as a fault, the way a dashed axis does.
   */
  material?: string;
  /**
   * The material drawn as a control the reader can act on, in place of the
   * `material` text. The row only gives it room; what it opens is its own.
   */
  materialChip?: Snippet;
  /**
   * Whether this is the tool head currently feeding filament. A bold label
   * carries it, because the row has neither a border nor a fill.
   */
  active?: boolean;
  /** MMU-fed heads only. The loaded position and the count ride on the material line. */
  filamentPositions?: FilamentPosition[];
  /**
   * Trailing-edge content, in practice a value editor trigger. The row reports
   * a heater and has no opinion on how it is commanded.
   */
  action?: Snippet;
  class?: ClassValue | undefined;
}

export interface HeaterRowsProps extends Omit<
  HTMLAttributes<HTMLDivElement>,
  'class'
> {
  /**
   * How many rows the grid holds. One row stacks; two or more flow into as
   * many columns as the width has room for. Svelte cannot count what a snippet
   * renders, so the caller says.
   */
  rows?: number;
  class?: ClassValue | undefined;
  children: Snippet;
}
