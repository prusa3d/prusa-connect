import type { Snippet } from 'svelte';
import type { ClassValue } from 'svelte/elements';

export interface RowActionsProps {
  /** The row's subject: a file name, an email, a printer. It names the trigger. */
  name: string;
  /** Menu items. Their height is settled by the menu, so none needs its own. */
  children: Snippet;
  class?: ClassValue | undefined;
  /** Told each time the menu opens or shuts. */
  onOpenChange?: ((open: boolean) => void) | undefined;
}
