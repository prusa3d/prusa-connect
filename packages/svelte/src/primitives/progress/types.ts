import type { Progress as ProgressPrimitive } from 'bits-ui';

/**
 * What the filled part says: `primary` for a job in motion, `muted` for one
 * that has stopped advancing, `success` for one that finished, `warning` for a
 * reading close to its limit and `destructive` for one past it. A tone is not
 * a status by itself. The row around the bar says what happened.
 */
export type ProgressTone =
  'primary' | 'muted' | 'success' | 'warning' | 'destructive';

/** The track's height. `sm` is the hairline bar of a printer's card or row. */
export type ProgressSize = 'sm' | 'md';

export type ProgressProps = ProgressPrimitive.RootProps & {
  tone?: ProgressTone;
  size?: ProgressSize;
};
