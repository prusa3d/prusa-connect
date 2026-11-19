<!--
  The remove printer alert dialog the alert-dialog tests drive. The parts only
  work together, and a test cannot write markup of its own.

  The action shuts the panel here, because bits-ui leaves that to the caller.
-->
<script lang="ts">
  import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
    AlertDialogTrigger,
  } from '../primitives/alert-dialog';
  import type { AlertDialogSize } from '../primitives/alert-dialog';
  import Button from '../primitives/button/button.svelte';

  interface Props {
    open?: boolean;
    size?: AlertDialogSize;
    onremove?: () => void;
  }

  let { open = $bindable(false), size, onremove }: Props = $props();
</script>

<AlertDialog bind:open>
  <AlertDialogTrigger>
    {#snippet child({ props })}
      <Button {...props} variant="destructive">Remove printer</Button>
    {/snippet}
  </AlertDialogTrigger>
  <AlertDialogContent {size}>
    <AlertDialogHeader>
      <AlertDialogTitle>Remove printer</AlertDialogTitle>
      <AlertDialogDescription>
        This removes the printer from your farm. Its job history is kept.
      </AlertDialogDescription>
    </AlertDialogHeader>
    <AlertDialogFooter>
      <AlertDialogCancel>Cancel</AlertDialogCancel>
      <AlertDialogAction
        variant="destructive"
        onclick={() => {
          onremove?.();
          open = false;
        }}
      >
        Remove
      </AlertDialogAction>
    </AlertDialogFooter>
  </AlertDialogContent>
</AlertDialog>
