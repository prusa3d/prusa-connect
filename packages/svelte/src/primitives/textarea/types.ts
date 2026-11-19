import type { ClassValue, HTMLTextareaAttributes } from 'svelte/elements';

export interface TextareaProps extends Omit<
  HTMLTextareaAttributes,
  'class' | 'value'
> {
  class?: ClassValue | undefined;
  resizable?: boolean | undefined;
  /** The text in the box. Bindable, so a form reads what a person typed. */
  value?: string;
}
