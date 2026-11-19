<!--
  The panel itself, with its scrim, in a portal so no ancestor's `overflow` or
  stacking context can clip it.

  `size` is a data attribute and not a class recipe. The header, the footer and
  the title all read it through `group-size-*`, so one attribute on the
  panel lays out everything inside it.

  `group/alert-dialog-content` is what those selectors name.
-->
<script lang="ts">
  import { AlertDialog as AlertDialogPrimitive } from 'bits-ui';

  import AlertDialogOverlay from './alert-dialog-overlay.svelte';
  import AlertDialogPortal from './alert-dialog-portal.svelte';
  import type { AlertDialogContentProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    size = 'default',
    ...rest
  }: AlertDialogContentProps = $props();
</script>

<AlertDialogPortal>
  <AlertDialogOverlay />
  <AlertDialogPrimitive.Content
    bind:ref
    data-slot="alert-dialog-content"
    data-size={size}
    class={[
      'group/alert-dialog-content fixed top-1/2 left-1/2 z-50 grid w-full max-w-dialog -translate-x-1/2 -translate-y-1/2 gap-4 rounded-md border border-border bg-background p-6 state-open:animate-scale-in state-closed:animate-scale-out size-default:sm:max-w-lg size-sm:max-w-xs',
      className,
    ]}
    {...rest}
  />
</AlertDialogPortal>
