<!--
  The printer settings sheet the sheet tests drive. The parts only work
  together, and a test cannot write markup of its own.
-->
<script lang="ts">
  import Button from '../primitives/button/button.svelte';
  import {
    Sheet,
    SheetClose,
    SheetContent,
    SheetDescription,
    SheetFooter,
    SheetHeader,
    SheetTitle,
    SheetTrigger,
  } from '../primitives/sheet';
  import type { SheetSide } from '../primitives/sheet';

  interface Props {
    open?: boolean;
    side?: SheetSide;
    showCloseButton?: boolean;
    overlayClass?: string;
  }

  let { open = false, side, showCloseButton, overlayClass }: Props = $props();
</script>

<Sheet {open}>
  <SheetTrigger>
    {#snippet child({ props })}
      <Button {...props}>Printer settings</Button>
    {/snippet}
  </SheetTrigger>
  <SheetContent {side} {showCloseButton} {overlayClass}>
    <SheetHeader>
      <SheetTitle>Printer settings</SheetTitle>
      <SheetDescription>
        Changes apply the next time this printer connects.
      </SheetDescription>
    </SheetHeader>
    <SheetFooter>
      <SheetClose>
        {#snippet child({ props })}
          <Button {...props} variant="secondary">Cancel</Button>
        {/snippet}
      </SheetClose>
      <Button>Save</Button>
    </SheetFooter>
  </SheetContent>
</Sheet>
