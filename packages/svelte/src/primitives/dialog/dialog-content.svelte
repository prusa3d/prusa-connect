<!--
  The panel. It draws the wash under itself, so a caller writes one element and
  gets both.

  The close control in the corner is part of the panel rather than a part a
  caller composes, because every dialog in the design system has one. A caller
  that wants none passes `showCloseButton={false}`.
-->
<script lang="ts">
  import XIcon from '@lucide/svelte/icons/x';
  import { Dialog as DialogPrimitive } from 'bits-ui';

  import { t } from '../../i18n';
  import DialogOverlay from './dialog-overlay.svelte';
  import DialogPortal from './dialog-portal.svelte';
  import type { DialogContentProps, DialogSize } from './types';

  const WIDTH: Record<DialogSize, string> = {
    md: 'sm:max-w-lg',
    lg: 'sm:max-w-2xl',
    xl: 'sm:max-w-4xl',
  };

  let {
    ref = $bindable(null),
    class: className,
    showCloseButton = true,
    size = 'md',
    children,
    ...rest
  }: DialogContentProps = $props();

  // Read when the panel draws, so a change of language rewrites it
  // (frontend-i18n.md rule 1).
  const closeText = $derived(t('primitives.close'));
</script>

<DialogPortal>
  <DialogOverlay />
  <DialogPrimitive.Content
    bind:ref
    data-slot="dialog-content"
    data-size={size}
    class={[
      'fixed top-1/2 left-1/2 z-50 grid w-full max-w-dialog -translate-x-1/2 -translate-y-1/2 grid-cols-1 gap-4 rounded-md border border-border bg-background p-6 outline-none state-open:animate-scale-in state-closed:animate-scale-out',
      WIDTH[size],
      className,
    ]}
    {...rest}
  >
    {@render children?.()}
    {#if showCloseButton}
      <DialogPrimitive.Close
        data-slot="dialog-close"
        class="absolute top-4 right-4 rounded-sm text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"
      >
        <XIcon />
        <span class="sr-only">{closeText}</span>
      </DialogPrimitive.Close>
    {/if}
  </DialogPrimitive.Content>
</DialogPortal>
