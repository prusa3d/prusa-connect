<!--
  A command that carries a tick.

  bits-ui has no indicator part. It hands the tick state to its `children`
  snippet, so the box and the mark are drawn here, on the markup Radix renders
  for the same row.
-->
<script lang="ts">
  import CheckIcon from '@lucide/svelte/icons/check';
  import { DropdownMenu as DropdownMenuPrimitive } from 'bits-ui';

  import type { DropdownMenuCheckboxItemProps } from './types';

  let {
    ref = $bindable(null),
    checked = $bindable(false),
    class: className,
    children: label,
    ...rest
  }: DropdownMenuCheckboxItemProps = $props();
</script>

<DropdownMenuPrimitive.CheckboxItem
  bind:ref
  bind:checked
  data-slot="dropdown-menu-checkbox-item"
  class={[
    "relative flex cursor-default items-center gap-2 rounded-sm py-1.5 pr-2 pl-8 text-sm outline-none select-none focus:bg-accent focus:text-foreground data-disabled:pointer-events-none data-disabled:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    className,
  ]}
  {...rest}
>
  {#snippet children({ checked: ticked })}
    <span
      class="pointer-events-none absolute left-2 flex size-3.5 items-center justify-center"
    >
      {#if ticked}
        <CheckIcon class="size-4" />
      {/if}
    </span>
    {@render label?.()}
  {/snippet}
</DropdownMenuPrimitive.CheckboxItem>
