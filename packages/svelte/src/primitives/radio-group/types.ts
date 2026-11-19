import type { RadioGroup as RadioGroupPrimitive } from 'bits-ui';

export type RadioGroupProps = RadioGroupPrimitive.RootProps & {
  /** The space between the options. `section` for options that each carry a picture. */
  gap?: 'md' | 'section' | undefined;
};

/**
 * The item draws its own indicator, so the two snippets bits-ui takes are not
 * a consumer's to pass. A label sits beside the item, never inside it.
 */
export type RadioGroupItemProps = Omit<
  RadioGroupPrimitive.ItemProps,
  'child' | 'children'
>;
