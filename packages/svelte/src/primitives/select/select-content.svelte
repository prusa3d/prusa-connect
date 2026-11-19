<!--
  The surface that holds the options, in a portal so no ancestor's `overflow`
  or stacking context can clip it.

  The viewport is the scrolling box inside the surface, and it has no
  `data-slot` of its own on either side. Its classes belong to the surface.
  `tabindex` makes the box reachable, which is what axe's
  `scrollable-region-focusable` rule asks of a region that scrolls.

  bits-ui publishes the anchor's measurements and the transform origin under
  its own names. The classes are the React design system's, so the element
  declares the React names from the bits-ui ones.

  bits-ui positions the surface beside the trigger and has no mode that lines
  the chosen option up over it, so the classes React keeps for that mode are
  always on. One of them, the viewport's height, never takes effect: bits-ui
  gives the viewport an inline `flex: 1`, whose `flex-basis: 0` decides the
  height instead, and the list grows to its options up to the surface's
  `max-height`. Removing the class would leave the surface short of the React
  class set.
-->
<script lang="ts">
  import { Select as SelectPrimitive } from 'bits-ui';

  import SelectScrollDownButton from './select-scroll-down-button.svelte';
  import SelectScrollUpButton from './select-scroll-up-button.svelte';
  import type { SelectContentProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    children,
    ...rest
  }: SelectContentProps = $props();
</script>

<SelectPrimitive.Portal>
  <SelectPrimitive.Content
    bind:ref
    data-slot="select-content"
    style="--radix-select-content-available-height: var(--bits-select-content-available-height); --radix-select-content-transform-origin: var(--bits-select-content-transform-origin); --radix-select-trigger-height: var(--bits-select-anchor-height); --radix-select-trigger-width: var(--bits-select-anchor-width);"
    class={[
      'relative z-50 max-h-(--radix-select-content-available-height) min-w-menu origin-(--radix-select-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border border-border bg-popover text-popover-foreground state-open:animate-scale-in state-closed:animate-scale-out',
      'side-bottom:translate-y-1 side-left:-translate-x-1 side-right:translate-x-1 side-top:-translate-y-1',
      className,
    ]}
    {...rest}
  >
    <SelectScrollUpButton />
    <SelectPrimitive.Viewport
      tabindex={0}
      class="h-(--radix-select-trigger-height) w-full min-w-(--radix-select-trigger-width) scroll-my-1 p-1"
    >
      {@render children?.()}
    </SelectPrimitive.Viewport>
    <SelectScrollDownButton />
  </SelectPrimitive.Content>
</SelectPrimitive.Portal>
