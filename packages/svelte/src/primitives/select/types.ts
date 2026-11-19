import type { Select as SelectPrimitive } from 'bits-ui';
import type { Snippet } from 'svelte';
import type { ClassValue, HTMLAttributes } from 'svelte/elements';

import type { ControlSize } from '../control-trigger/types';

/**
 * bits-ui's root is a union over `type`, `single` against `multiple`. Radix's
 * select holds one value and so does this one, so the arm is fixed and a
 * caller never names it. Fixing it is also what gives `value` one type and
 * makes it bindable.
 */
export type SelectProps = Omit<
  SelectPrimitive.RootProps,
  'type' | 'value' | 'onValueChange'
> & {
  /** The value of the chosen option, or the empty string while none is. */
  value?: string;
  onValueChange?: (value: string) => void;
  /**
   * Every option as a value and the text that stands for it.
   *
   * Radix mirrors the chosen option's text into the trigger and keeps it there
   * after the list shuts. bits-ui reads the text off the option's own element,
   * which is unmounted by then, and falls back to the raw value. Passing the
   * pairs here is what keeps the trigger reading `MK4S` rather than `mk4s`,
   * and it is also what lets a person type at the shut trigger to pick.
   */
  items?: { value: string; label: string; disabled?: boolean }[];
};

export type SelectGroupProps = SelectPrimitive.GroupProps;

export type SelectValueProps = SelectPrimitive.ValueProps;

export type SelectTriggerProps = SelectPrimitive.TriggerProps & {
  /**
   * The shared control scale: `sm` 28px, `md` 36px, `lg` 44px. A select and a
   * text field standing in the same toolbar read it from the same record.
   */
  size?: ControlSize;
  /** `info` fills the trigger in the info colour, at rest and on hover. */
  tone?: SelectTriggerTone | undefined;
};

export type SelectTriggerTone = 'default' | 'info';

export type SelectContentProps = SelectPrimitive.ContentProps;

/** bits-ui names the group's heading `GroupHeading`; React names it the label. */
export type SelectLabelProps = SelectPrimitive.GroupHeadingProps;

/**
 * bits-ui hands an item's `children` snippet whether the option is chosen, so
 * that a caller can draw its own tick. This item draws the tick itself, and
 * its content is a plain snippet.
 */
export type SelectItemProps = Omit<
  SelectPrimitive.ItemProps,
  'child' | 'children'
> & {
  children?: Snippet;
};

/**
 * bits-ui has no separator part for a select, so the rule is a plain element
 * with the classes on it.
 */
export type SelectSeparatorProps = HTMLAttributes<HTMLDivElement> & {
  class?: ClassValue | undefined;
};

export type SelectScrollUpButtonProps = SelectPrimitive.ScrollUpButtonProps;

export type SelectScrollDownButtonProps = SelectPrimitive.ScrollDownButtonProps;
