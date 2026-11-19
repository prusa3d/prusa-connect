import type { Switch as SwitchPrimitive } from 'bits-ui';

/**
 * How tall the track is: `sm` 14px, `md` 18.4px. The size is a `data-size`
 * attribute on the root and a pair of `size-*` classes, so the thumb
 * reads it off the root through the `group/switch` name rather than taking a
 * size of its own.
 */
export type SwitchSize = 'sm' | 'md';

export type SwitchProps = SwitchPrimitive.RootProps & {
  size?: SwitchSize;
};
