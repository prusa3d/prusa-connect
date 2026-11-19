import type { ClassValue } from 'svelte/elements';

/**
 * `prusa` is the PRUSA wordmark, which the app draws until design ships a new
 * Connect logo. `full` is the word Connect alone, the simple logo. `mark` is
 * the orange square alone, for a slot with no width to spare, such as the app
 * bar.
 */
export type ConnectLogoShape = 'prusa' | 'full' | 'mark';

/** How tall the logo is drawn. The width follows from the shape. */
export type ConnectLogoSize = 'xs' | 'sm' | 'md' | 'lg';

export interface ConnectLogoProps {
  shape?: ConnectLogoShape;
  size?: ConnectLogoSize;
  /**
   * What a screen reader hears. A link that holds only the logo takes its
   * name from here. Empty marks the logo as decoration, for a slot where the
   * name is already written beside it.
   */
  label?: string;
  class?: ClassValue | undefined;
}
