import type { Snippet } from 'svelte';

import type { ActionOutletTarget } from '../generated';
import type { ModuleLoader } from '../loader.svelte';
import type { SlotRegistry } from '../registry.svelte';
import type { WithSlotDevice } from '../device-selector';

/**
 * An action slot's menu: the slot, and the id the host hands whichever entry
 * runs, under the name the slot's subject gives it (`deviceId` for
 * `device-action`, `fileRef` for `file-action`).
 */
export type SlotActionsProps = WithSlotDevice<ActionOutletTarget> & {
  /** The subject's name, read into the menu button's accessible label. */
  label: string;
  /**
   * The screen's own items, drawn above the modules' entries in the same
   * menu, so a screen that has a menu of its own shows one and not two. The
   * menu is drawn when this is given or a manifest declares an entry.
   */
  items?: Snippet | undefined;
  /** Classes for the menu's button. */
  class?: string | undefined;
  /**
   * Classes the menu's button adds while it holds only `items`, for a screen
   * that draws those acts on the subject itself at some widths and needs the
   * menu only where they fold.
   */
  itemsOnlyClass?: string | undefined;
  /**
   * Draws the entries marked primary as buttons before the menu, which is
   * what the subject's own page does. Everywhere else they stay menu items
   * (ADR-20261029-dgv).
   */
  buttons?: boolean | undefined;
  /**
   * The registry to read. It defaults to the one in context, so an app that
   * called `setSlotContext` passes neither this nor `loader`.
   */
  registry?: SlotRegistry | undefined;
  /** The loader the menu wakes on mount. Defaults to the one in context. */
  loader?: ModuleLoader | undefined;
};
