<!--
  The panel itself, with its scrim, in a portal so no ancestor's `overflow` or
  stacking context can clip it.

  bits-ui gives the element `role="dialog"`, `aria-modal` and an
  `aria-labelledby` pointing at the title, so a panel with a `SheetTitle` in it
  is named without the caller doing anything.

  `pointer-events-auto` and `overscroll-contain` are on the panel: the first so
  a press lands on the panel and not on the page under it, the second so a
  fling inside the panel does not carry on into the page.

  The cross in the corner is drawn here rather than through `SheetClose`,
  because the class names belong to the panel's own layout and not to a control
  a caller places.
-->
<script lang="ts">
  import XIcon from '@lucide/svelte/icons/x';
  import { Dialog as SheetPrimitive } from 'bits-ui';

  import { t } from '../../i18n';
  import SheetOverlay from './sheet-overlay.svelte';
  import SheetPortal from './sheet-portal.svelte';
  import type { SheetContentProps } from './types';
  import { sides } from './variants';

  let {
    ref = $bindable(null),
    class: className,
    side = 'right',
    showCloseButton = true,
    overlayClass,
    children,
    ...rest
  }: SheetContentProps = $props();

  // Read when the panel draws, so a change of language rewrites it
  // (frontend-i18n.md rule 1).
  const closeText = $derived(t('primitives.close'));
</script>

<SheetPortal>
  <SheetOverlay class={overlayClass} />
  <SheetPrimitive.Content
    bind:ref
    data-slot="sheet-content"
    class={[
      'pointer-events-auto fixed z-50 flex flex-col gap-4 overscroll-contain rounded-md border-border bg-background outline-none',
      sides[side],
      className,
    ]}
    {...rest}
  >
    {@render children?.()}
    {#if showCloseButton}
      <SheetPrimitive.Close
        data-slot="sheet-close"
        class="absolute top-4 right-4 rounded-sm text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"
      >
        <XIcon />
        <span class="sr-only">{closeText}</span>
      </SheetPrimitive.Close>
    {/if}
  </SheetPrimitive.Content>
</SheetPortal>
