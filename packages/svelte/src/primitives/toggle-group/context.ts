import { getContext, setContext } from 'svelte';

import type { ToggleSize, ToggleTone, ToggleVariant } from '../toggle/types';

/** What a group tells its own items to look like. */
export interface ToggleGroupLook {
  variant: ToggleVariant;
  tone: ToggleTone;
  size: ToggleSize;
}

const key = Symbol('toggle-group-look');

/**
 * A getter and not a value. The group's props can change, and a look read once
 * at mount would leave the items on the pair the group started with.
 */
export const setGroupLook = (look: () => ToggleGroupLook): void => {
  setContext(key, look);
};

/** The look of the group an item sits in, or nothing when it sits alone. */
export const getGroupLook = (): (() => ToggleGroupLook) | undefined =>
  getContext<(() => ToggleGroupLook) | undefined>(key);
