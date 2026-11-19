<script lang="ts">
  // bits-ui, for the roving tabindex, the arrow-key activation and the
  // `aria-controls` pairing between a trigger and its panel.
  import { Tabs as TabsPrimitive } from 'bits-ui';

  import type { TabsProps } from './types';

  let {
    ref = $bindable(null),
    value = $bindable(''),
    class: className,
    ...rest
  }: TabsProps = $props();
</script>

<!-- The row of triggers stacks above the panel. Only the horizontal case is
     styled, because nothing here renders vertical tabs; the keyboard and ARIA
     handling for a vertical set still works, it is the layout that is
     unstyled. `data-orientation` is the primitive's own attribute. -->
<TabsPrimitive.Root
  bind:ref
  bind:value
  data-slot="tabs"
  class={['group/tabs flex gap-2 orientation-horizontal:flex-col', className]}
  {...rest}
/>
