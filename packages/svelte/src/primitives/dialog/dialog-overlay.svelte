<!--
  The wash that puts the page behind the panel out of reach. A press on it
  closes the dialog, which bits-ui handles.

  The fade is opacity alone, so the compositor runs it without the main thread.
-->
<script lang="ts">
  import { Dialog as DialogPrimitive } from 'bits-ui';

  import type { DialogOverlayProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    ...rest
  }: DialogOverlayProps = $props();
</script>

<DialogPrimitive.Overlay
  bind:ref
  data-slot="dialog-overlay"
  class={[
    'fixed inset-0 z-50 bg-black/50 state-open:animate-fade-in state-closed:animate-fade-out',
    className,
  ]}
  {...rest}
/>
