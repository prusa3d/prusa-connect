import type { Collapsible as CollapsiblePrimitive } from 'bits-ui';

export type CollapsibleProps = CollapsiblePrimitive.RootProps;

export type CollapsibleTriggerVariant = 'default' | 'row' | 'filled' | 'inline';

export type CollapsibleTriggerTone = 'default' | 'warning';

export type CollapsibleTriggerProps = CollapsiblePrimitive.TriggerProps & {
  variant?: CollapsibleTriggerVariant;
  /** `warning` draws the row, and its chevron, in the warning text colour. */
  tone?: CollapsibleTriggerTone;
  /** A leading chevron that turns when open. On for `row` and `inline`. */
  chevron?: boolean;
};

export type CollapsibleContentProps = CollapsiblePrimitive.ContentProps;
