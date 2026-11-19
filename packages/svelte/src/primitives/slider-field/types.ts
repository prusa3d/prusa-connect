import type { ClassValue } from 'svelte/elements';

export interface SliderFieldProps {
  /** What the machine reports. The draft goes back to it whenever it changes. */
  value: number;
  /** Called once per settled value: on release, on Enter, on blur. */
  onCommit: (next: number) => void;
  min: number;
  max: number;
  /** When supplied, the numeric field also snaps to this increment. */
  step?: number;
  /** Printed after the field, and part of both controls' accessible names. */
  unit: string;
  /** What is being set, `Chamber fan speed`. Names the slider and the field. */
  label: string;
  disabled?: boolean;
  class?: ClassValue | undefined;
}
