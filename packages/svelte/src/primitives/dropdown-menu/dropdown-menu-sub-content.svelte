<!--
  The submenu's own surface.

  bits-ui positions every submenu through its shared menu layer, so the
  transform origin arrives under `--bits-menu-content-transform-origin` and not
  under a dropdown-menu name. The element declares the React name from it.
-->
<script lang="ts">
  import { DropdownMenu as DropdownMenuPrimitive } from 'bits-ui';

  import type { DropdownMenuSubContentProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    ...rest
  }: DropdownMenuSubContentProps = $props();
</script>

<DropdownMenuPrimitive.SubContent
  bind:ref
  data-slot="dropdown-menu-sub-content"
  style="--radix-dropdown-menu-content-transform-origin: var(--bits-menu-content-transform-origin);"
  class={[
    'z-50 min-w-menu origin-(--radix-dropdown-menu-content-transform-origin) overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground state-open:animate-scale-in state-closed:animate-scale-out',
    className,
  ]}
  {...rest}
/>
