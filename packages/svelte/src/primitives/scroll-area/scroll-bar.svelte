<script lang="ts">
  import { ScrollArea as ScrollAreaPrimitive } from 'bits-ui';

  import type { ScrollBarProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    orientation = 'vertical',
    ...rest
  }: ScrollBarProps = $props();
</script>

<ScrollAreaPrimitive.Scrollbar
  bind:ref
  data-slot="scroll-area-scrollbar"
  {orientation}
  class={[
    'flex touch-none p-px transition-colors select-none',
    orientation === 'vertical' && 'h-full w-2.5 border-l border-l-transparent',
    orientation === 'horizontal' &&
      'h-2.5 flex-col border-t border-t-transparent',
    className,
  ]}
  {...rest}
>
  <!-- The thumb is the muted foreground rather than the border step, which is
       too near invisible to aim a pointer at. -->
  <ScrollAreaPrimitive.Thumb
    data-slot="scroll-area-thumb"
    class="relative flex-1 rounded-scrollbar bg-scrollbar-thumb transition-colors hover:bg-scrollbar-thumb-hovered"
  />
</ScrollAreaPrimitive.Scrollbar>
