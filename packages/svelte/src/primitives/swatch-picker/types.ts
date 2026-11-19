import type { ClassValue } from 'svelte/elements';

export interface Swatch {
  key: string;
  hex: string;
  /** The swatch's name, for assistive technology and the hover title. */
  label: string;
}

export interface SwatchPickerProps {
  swatches: readonly Swatch[];
  /** The picked swatch's key, undefined when none is picked. */
  value: string | undefined;
  onPick: (key: string | undefined) => void;
  /** The group's name. */
  label: string;
  /** Pressing the picked swatch again reports `undefined`. */
  deselectable?: boolean;
  /** `xs` is a small round dot, for a filter row that shares its line. */
  size?: 'xs' | 'sm' | 'md';
  class?: ClassValue | undefined;
}
