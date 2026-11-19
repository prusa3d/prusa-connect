<!--
  The surface itself, in a portal so no ancestor's `overflow` or stacking
  context can clip it.

  bits-ui publishes the anchor's transform origin and the room left below it
  under its own names. The `origin-(--radix-...)` and `max-h-(--radix-...)`
  classes are the React design system's, so the element declares the React names
  from the bits-ui ones.

  `preventScroll` is false. bits-ui locks the page scroll by default, and the
  React design system runs this menu unmodal, where the page behind it still
  scrolls.
-->
<script lang="ts">
  import { ContextMenu as ContextMenuPrimitive } from 'bits-ui';

  import type { ContextMenuContentProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    preventScroll = false,
    ...rest
  }: ContextMenuContentProps = $props();
</script>

<ContextMenuPrimitive.Portal>
  <ContextMenuPrimitive.Content
    bind:ref
    data-slot="context-menu-content"
    {preventScroll}
    style="--radix-context-menu-content-transform-origin: var(--bits-context-menu-content-transform-origin); --radix-context-menu-content-available-height: var(--bits-context-menu-content-available-height);"
    class={[
      'z-50 max-h-(--radix-context-menu-content-available-height) min-w-menu origin-(--radix-context-menu-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border border-border bg-popover p-1 text-popover-foreground state-open:animate-scale-in state-closed:animate-scale-out',
      className,
    ]}
    {...rest}
  />
</ContextMenuPrimitive.Portal>
