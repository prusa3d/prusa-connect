<script lang="ts">
  import { ToggleGroup as ToggleGroupPrimitive } from 'bits-ui';

  import { base, itemSizes, tones, variants } from '../toggle/variants';
  import { getGroupLook } from './context';
  import type { ToggleGroupItemProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    variant,
    tone,
    size,
    ...rest
  }: ToggleGroupItemProps = $props();

  const groupLook = getGroupLook();
  const look = $derived(groupLook?.());
  const itemVariant = $derived(variant ?? look?.variant ?? 'default');
  const itemTone = $derived(tone ?? look?.tone ?? 'default');
  const itemSize = $derived(size ?? look?.size ?? 'md');
</script>

<ToggleGroupPrimitive.Item
  bind:ref
  data-slot="toggle-group-item"
  data-variant={itemVariant}
  data-tone={itemTone}
  data-size={itemSize}
  class={[
    base,
    variants[itemVariant],
    tones[itemTone],
    itemSizes[itemSize],
    // The items sit edge to edge, so the group's outer corners are the first
    // and last item's own. Under `outline` each shared edge would be drawn
    // twice, and `border-l-0` with `first:border-l` collapses it to one
    // hairline.
    'w-auto shrink-0 rounded-none first:rounded-l-sm last:rounded-r-sm focus:z-10 focus-visible:z-10 variant-outline:border-l-0 variant-outline:first:border-l',
    className,
  ]}
  {...rest}
/>
