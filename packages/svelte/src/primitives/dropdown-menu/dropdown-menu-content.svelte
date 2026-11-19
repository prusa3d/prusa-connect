<!--
  The surface itself, in a portal so no ancestor's `overflow` or stacking
  context can clip it.

  bits-ui publishes the anchor's transform origin and the room left below it
  under its own names. The `origin-(--radix-...)` and `max-h-(--radix-...)`
  classes are the React design system's, so the element declares the React names
  from the bits-ui ones.

  `preventScroll` is false. bits-ui locks the page scroll by default, and the
  React design system runs this menu unmodal, where the page behind it still
  scrolls and the trigger stays reachable by assistive tech.
-->
<script lang="ts">
  import { DropdownMenu as DropdownMenuPrimitive } from 'bits-ui';

  import type { DropdownMenuContentProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    sideOffset = 4,
    preventScroll = false,
    ...rest
  }: DropdownMenuContentProps = $props();
</script>

<DropdownMenuPrimitive.Portal>
  <DropdownMenuPrimitive.Content
    bind:ref
    data-slot="dropdown-menu-content"
    {sideOffset}
    {preventScroll}
    style="--radix-dropdown-menu-content-transform-origin: var(--bits-dropdown-menu-content-transform-origin); --radix-dropdown-menu-content-available-height: var(--bits-dropdown-menu-content-available-height);"
    class={[
      'z-50 max-h-(--radix-dropdown-menu-content-available-height) min-w-menu origin-(--radix-dropdown-menu-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border border-border bg-popover p-1 text-popover-foreground state-open:animate-scale-in state-closed:animate-scale-out',
      className,
    ]}
    {...rest}
  />
</DropdownMenuPrimitive.Portal>
