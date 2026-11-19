<script lang="ts">
  // bits-ui, for the scrollbar it draws in place of the platform one: the
  // native bar is hidden on the viewport and the thumb below tracks it.
  import { ScrollArea as ScrollAreaPrimitive } from 'bits-ui';

  import ScrollBar from './scroll-bar.svelte';
  import type { ScrollAreaProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    children,
    ...rest
  }: ScrollAreaProps = $props();
</script>

<!-- `overflow-hidden` on the root. The primitive leaves it visible and scrolls
     the viewport alone, which holds while the root is sized by its content and
     fails the moment it is not: given a definite height, a flex item or a grid
     row, the viewport keeps the full height of its content and paints outside
     the box. -->
<ScrollAreaPrimitive.Root
  bind:ref
  data-slot="scroll-area"
  class={['relative overflow-hidden', className]}
  {...rest}
>
  <!-- `tabindex` and the ring it draws are what let somebody scroll this with
       the keyboard: a browser only scrolls a container that is, or contains,
       the focused element. -->
  <ScrollAreaPrimitive.Viewport
    data-slot="scroll-area-viewport"
    tabindex={0}
    class="size-full rounded-scroll-viewport transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
  >
    {@render children?.()}
  </ScrollAreaPrimitive.Viewport>
  <!-- Both axes. Each one's track and thumb is gated on its own overflow, so
       an axis that does not overflow draws nothing. -->
  <ScrollBar orientation="vertical" />
  <ScrollBar orientation="horizontal" />
  <ScrollAreaPrimitive.Corner />
</ScrollAreaPrimitive.Root>
