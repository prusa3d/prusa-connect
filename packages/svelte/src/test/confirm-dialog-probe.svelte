<!--
  The delete confirmation the confirm-dialog tests drive. A trigger is a
  snippet that takes the trigger's own props, and a test cannot write one.
-->
<script lang="ts">
  import Button from '../primitives/button/button.svelte';
  import ConfirmDialog from '../primitives/confirm-dialog/confirm-dialog.svelte';

  interface Props {
    open?: boolean;
    /** Off for a dialog the screen holds open rather than one a button opens. */
    withTrigger?: boolean;
    onconfirm?: () => void;
    confirmDisabled?: boolean;
    variant?: 'destructive' | 'confirm';
    title?: string;
  }

  let {
    open = $bindable(false),
    withTrigger = true,
    title = 'Delete "Bracket_Left_Reinforced_v4_FINAL.gcode"?',
    onconfirm,
    confirmDisabled = false,
    variant,
  }: Props = $props();
</script>

{#snippet trigger({ props }: { props: Record<string, unknown> })}
  <Button {...props} variant="destructive">Delete</Button>
{/snippet}

<ConfirmDialog
  bind:open
  trigger={withTrigger ? trigger : undefined}
  {title}
  description="This removes the file from the store. This cannot be undone."
  confirmLabel="Delete"
  onConfirm={() => onconfirm?.()}
  {confirmDisabled}
  {variant}
/>
