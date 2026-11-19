<!--
  The surface itself, in a portal so no ancestor's `overflow` or stacking
  context can clip it.

  bits-ui puts no `role` on this element, where Radix puts `role="dialog"`. A
  name given with `aria-label` or `aria-labelledby` would sit on a plain `div`
  and reach no one, so the heading inside the surface is what names it.

  bits-ui publishes the anchor's transform origin and the room left beside the
  anchor under names of its own. The `origin-(--radix-...)` class and the width
  caps that read `--radix-popper-available-width` are the React design
  system's, so the element declares both React names from the bits-ui ones. A
  surface that caps its own width reads the second, and the cap has to sit on
  this box, because the measurement runs from this box's own edge with its
  padding and border counted in.
-->
<script lang="ts">
  import { Popover as PopoverPrimitive } from 'bits-ui';

  import type { PopoverContentProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    align = 'center',
    sideOffset = 4,
    ...rest
  }: PopoverContentProps = $props();

  // The caller's own declarations follow the two names below rather than
  // replacing them, because an attribute spread later in the list would drop
  // both and leave the surface with no transform origin and no width to
  // measure against.
  const callerStyle = $derived(
    typeof rest.style === 'string' ? rest.style : '',
  );
  const declarations = $derived(
    `--radix-popover-content-transform-origin: var(--bits-popover-content-transform-origin); --radix-popper-available-width: var(--bits-popover-content-available-width); ${callerStyle}`,
  );
</script>

<PopoverPrimitive.Portal>
  <PopoverPrimitive.Content
    bind:ref
    {...rest}
    data-slot="popover-content"
    {align}
    {sideOffset}
    style={declarations}
    class={[
      'z-50 w-72 origin-(--radix-popover-content-transform-origin) rounded-md border border-border bg-popover p-4 text-popover-foreground outline-none state-open:animate-scale-in state-closed:animate-scale-out',
      className,
    ]}
  />
</PopoverPrimitive.Portal>
