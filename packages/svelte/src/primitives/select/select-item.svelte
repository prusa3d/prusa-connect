<!--
  One option in the list. The tick sits in a box on the trailing edge, and the
  label is the last child, which is what the `*:[span]:last:*` classes line up.

  The element is drawn here through the `child` snippet so the tick can read
  whether the option is chosen. bits-ui reports that as `selected` on the
  snippet, where Radix draws it from an `ItemIndicator` part.

  bits-ui leaves focus on the surface and marks the option under the keyboard
  with `data-highlighted`, which is why the fill for that option is a style
  block here rather than the `focus:` classes React writes.
-->
<script lang="ts">
  import CheckIcon from '@lucide/svelte/icons/check';
  import { Select as SelectPrimitive } from 'bits-ui';

  import type { SelectItemProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    children,
    ...rest
  }: SelectItemProps = $props();
</script>

<SelectPrimitive.Item bind:ref {...rest}>
  {#snippet child({ props, selected })}
    <div
      {...props}
      data-slot="select-item"
      class={[
        "relative flex w-full cursor-pointer items-center gap-2 rounded-sm py-1.5 pr-8 pl-2 text-sm outline-none select-none focus:bg-accent focus:text-foreground data-disabled:pointer-events-none data-disabled:cursor-default data-disabled:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground *:[span]:last:flex *:[span]:last:items-center *:[span]:last:gap-2",
        className,
      ]}
    >
      <span
        data-slot="select-item-indicator"
        class="absolute right-2 flex size-3.5 items-center justify-center"
      >
        {#if selected}
          <CheckIcon class="size-4" />
        {/if}
      </span>
      <span>{@render children?.()}</span>
    </div>
  {/snippet}
</SelectPrimitive.Item>

<style>
  [data-slot='select-item'][data-highlighted] {
    background-color: var(--color-accent);
    color: var(--color-foreground);
  }
</style>
