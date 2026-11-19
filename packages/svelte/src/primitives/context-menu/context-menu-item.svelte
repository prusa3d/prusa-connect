<!--
  One command in the menu.

  `variant="destructive"` paints the row in `text-destructive-text` rather than
  `text-destructive`, which measures 4.48:1 on the popover colour in the dark
  theme and misses WCAG AA's 4.5:1.

  A disabled destructive row needs the compound
  `data-disabled:variant-destructive:*` selector. Two single-attribute
  selectors tie on specificity, and the destructive one can win, which leaves a
  disabled row painted in the live colour.
-->
<script lang="ts">
  import { ContextMenu as ContextMenuPrimitive } from 'bits-ui';

  import type { ContextMenuItemProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    inset = false,
    variant = 'default',
    ...rest
  }: ContextMenuItemProps = $props();
</script>

<ContextMenuPrimitive.Item
  bind:ref
  data-slot="context-menu-item"
  data-inset={inset ? '' : undefined}
  data-variant={variant}
  class={[
    "relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none select-none focus:bg-accent focus:text-foreground data-disabled:pointer-events-none data-disabled:text-disabled-foreground data-inset:pl-8 variant-destructive:text-destructive-text variant-destructive:focus:bg-destructive/10 variant-destructive:focus:text-destructive-text data-disabled:variant-destructive:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground variant-destructive:*:[svg]:text-destructive",
    className,
  ]}
  {...rest}
/>
