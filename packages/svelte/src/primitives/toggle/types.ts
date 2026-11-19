import type { Toggle as TogglePrimitive } from 'bits-ui';

/** The resting look: no edge at all, or a hairline border around it. */
export type ToggleVariant = 'default' | 'outline';

/**
 * The pressed fill. `default` is the selected blue, for a choice that only
 * changes what the page shows. `primary` is the orange of the primary button,
 * for a choice that changes the machine, such as a fan mode.
 */
export type ToggleTone = 'default' | 'primary';

/** The three heights, the same set Button offers. */
export type ToggleSize = 'sm' | 'md' | 'lg';

/** A group item's sizes: the three heights, and `icon` for a glyph alone. */
export type ToggleItemSize = ToggleSize | 'icon';

export type ToggleProps = TogglePrimitive.RootProps & {
  variant?: ToggleVariant;
  tone?: ToggleTone;
  size?: ToggleSize;
};
