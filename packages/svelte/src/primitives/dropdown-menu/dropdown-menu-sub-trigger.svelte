<!--
  The row that opens a submenu. The chevron at its end says a submenu is there.
-->
<script lang="ts">
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
  import { DropdownMenu as DropdownMenuPrimitive } from 'bits-ui';

  import type { DropdownMenuSubTriggerProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    inset = false,
    children,
    ...rest
  }: DropdownMenuSubTriggerProps = $props();
</script>

<DropdownMenuPrimitive.SubTrigger
  bind:ref
  data-slot="dropdown-menu-sub-trigger"
  data-inset={inset ? '' : undefined}
  class={[
    "flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none select-none focus:bg-accent focus:text-foreground data-inset:pl-8 state-open:bg-accent state-open:text-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground",
    className,
  ]}
  {...rest}
>
  {@render children?.()}
  <ChevronRightIcon class="ml-auto size-4" />
</DropdownMenuPrimitive.SubTrigger>
