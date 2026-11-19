<!--
  The submenu's own surface.

  bits-ui positions every submenu through its shared menu layer, so the
  transform origin arrives under `--bits-menu-content-transform-origin` and not
  under a context-menu name. The element declares the React name from it.
-->
<script lang="ts">
  import { ContextMenu as ContextMenuPrimitive } from 'bits-ui';

  import type { ContextMenuSubContentProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    ...rest
  }: ContextMenuSubContentProps = $props();
</script>

<ContextMenuPrimitive.SubContent
  bind:ref
  data-slot="context-menu-sub-content"
  style="--radix-context-menu-content-transform-origin: var(--bits-menu-content-transform-origin);"
  class={[
    'z-50 min-w-menu origin-(--radix-context-menu-content-transform-origin) overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground state-open:animate-scale-in state-closed:animate-scale-out',
    className,
  ]}
  {...rest}
/>
