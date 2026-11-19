import type { ProgressSize, ProgressTone } from './types';

/** The fill of the filled part, one class per tone. */
export const tones: Record<ProgressTone, string> = {
  primary: 'bg-progress',
  muted: 'bg-muted-foreground',
  success: 'bg-success',
  warning: 'bg-warning',
  destructive: 'bg-destructive',
};

/** The track's height, one class per size. */
export const sizes: Record<ProgressSize, string> = {
  sm: 'h-1',
  md: 'h-2',
};

/** The fill of a bar with no reading: grey, so it is not read as done. */
export const UNKNOWN = 'bg-muted-foreground/40';
