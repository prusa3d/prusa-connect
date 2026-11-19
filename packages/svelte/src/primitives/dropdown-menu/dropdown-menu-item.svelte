<!--
  One command in the menu.

  Under a finger the row stands on this tree's 44px floor for a control a
  finger has to hit; under a mouse it keeps the compact control height, as a
  facet row does. The menu is portalled out of the app shell, so the shell's
  width cannot reach it the way it reaches a facet row.

  `variant="destructive"` paints the row in `text-destructive-text` rather than
  `text-destructive`, which measures 4.48:1 on the popover colour in the dark
  theme and misses WCAG AA's 4.5:1.

  A disabled destructive row needs the compound
  `data-disabled:variant-destructive:*` selector. Two single-attribute
  selectors tie on specificity, and the destructive one can win, which leaves a
  disabled row painted in the live colour.
-->
<script lang="ts">
  import { DropdownMenu as DropdownMenuPrimitive } from 'bits-ui';

  import type { DropdownMenuItemProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    inset = false,
    variant = 'default',
    ...rest
  }: DropdownMenuItemProps = $props();
</script>

<!-- An item reaches the touch floor under a coarse pointer only: the menu is
     portalled out of the app shell, so it cannot read that container. -->
<DropdownMenuPrimitive.Item
  bind:ref
  data-slot="dropdown-menu-item"
  data-inset={inset ? '' : undefined}
  data-variant={variant}
  class={[
    "relative flex min-h-control-height cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none select-none focus:bg-accent focus:text-foreground data-disabled:pointer-events-none data-disabled:text-disabled-foreground data-inset:pl-8 pointer-coarse:min-h-touch-target variant-destructive:text-destructive-text variant-destructive:focus:bg-destructive/10 variant-destructive:focus:text-destructive-text data-disabled:variant-destructive:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground variant-destructive:*:[svg]:text-destructive",
    className,
  ]}
  {...rest}
/>
