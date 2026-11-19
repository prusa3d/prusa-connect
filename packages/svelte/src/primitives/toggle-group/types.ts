import type { ToggleGroup as ToggleGroupPrimitive } from 'bits-ui';

import type {
  ToggleItemSize,
  ToggleSize,
  ToggleTone,
  ToggleVariant,
} from '../toggle/types';

export type ToggleGroupProps = ToggleGroupPrimitive.RootProps & {
  variant?: ToggleVariant;
  tone?: ToggleTone;
  size?: ToggleSize;
};

export type ToggleGroupItemProps = ToggleGroupPrimitive.ItemProps & {
  variant?: ToggleVariant;
  tone?: ToggleTone;
  size?: ToggleItemSize;
};
