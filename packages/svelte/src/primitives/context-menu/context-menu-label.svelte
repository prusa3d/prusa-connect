<!--
  The heading over a run of items.

  It is a plain div. Radix draws this part as a plain div too, and bits-ui's
  `GroupHeading` asks for a `ContextMenuGroup` above it, which the React
  design system does not put there.
-->
<script lang="ts">
  import type { ContextMenuLabelProps } from './types';

  let {
    children,
    class: className,
    inset = false,
    ...rest
  }: ContextMenuLabelProps = $props();
</script>

<div
  data-slot="context-menu-label"
  data-inset={inset ? '' : undefined}
  class={['px-2 py-1.5 text-sm font-semibold data-inset:pl-8', className]}
  {...rest}
>
  {@render children?.()}
</div>
