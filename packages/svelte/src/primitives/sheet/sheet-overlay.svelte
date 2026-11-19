<!--
  The layer that dims the page behind the panel. A press on it shuts the sheet,
  because bits-ui counts the press as an interaction outside the content.

  This design system draws no shadow, so the scrim is the only thing that puts
  a sheet in front of the page rather than beside it. Over the dark theme's
  black page a translucent black wash changes nothing a person can see, and the
  blur is what still reads there. It stays at 3px because a blur costs real
  time on a Pi-class GPU, where a phone would use 8px.

  `pointer-events-auto` is what makes the press on the scrim land. It is the
  one class here that is not decoration.

  This is `Dialog.Overlay` itself, where React draws a plain `div` whenever its
  root is not modal. `sheet.svelte` says why that branch has nothing to answer
  to here.
-->
<script lang="ts">
  import { Dialog as SheetPrimitive } from 'bits-ui';

  import type { SheetOverlayProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    ...rest
  }: SheetOverlayProps = $props();
</script>

<SheetPrimitive.Overlay
  bind:ref
  data-slot="sheet-overlay"
  class={[
    'pointer-events-auto fixed inset-0 z-50 bg-black/50 backdrop-blur-scrim state-open:animate-fade-in state-closed:animate-fade-out',
    className,
  ]}
  {...rest}
/>
