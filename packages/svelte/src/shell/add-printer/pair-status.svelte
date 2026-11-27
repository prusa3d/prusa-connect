<!--
  A pairing on its way: the code the printer shows, set large so a person can
  hold it next to the printer's screen, and what is happening now. The line
  under the code is a live region, so a screen reader hears the printer's
  answer when it comes.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import { Button, Spinner } from '../../primitives';
  import type { PrinterPairingView } from './types';

  let {
    pairing,
    onCancel,
    onAgain,
  }: {
    pairing: PrinterPairingView;
    onCancel?: (() => void) | undefined;
    onAgain?: (() => void) | undefined;
  } = $props();

  const open = $derived(
    pairing.state === 'waiting' || pairing.state === 'approved',
  );

  const said = $derived.by(() => {
    switch (pairing.state) {
      case 'waiting':
        return t('addPrinter.pair.state.waiting', { printer: pairing.printer });
      case 'approved':
        return t('addPrinter.pair.state.approved');
      case 'admitted':
        return t('addPrinter.pair.state.admitted');
      case 'refused':
        return t('addPrinter.pair.state.refused');
      case 'expired':
        return t('addPrinter.pair.state.expired');
      case 'withdrawn':
        return t('addPrinter.pair.state.withdrawn');
    }
  });
</script>

<div
  class="flex flex-col gap-4"
  data-part="pair-status"
  data-state={pairing.state}
>
  <div class="flex flex-col gap-1">
    <p class="text-sm text-muted-foreground">{t('addPrinter.pair.code')}</p>
    <p
      class="font-mono text-3xl font-semibold tracking-widest text-foreground"
      data-part="pair-code"
    >
      {pairing.code}
    </p>
  </div>

  <p class="flex items-center gap-2 text-sm" aria-live="polite">
    {#if open}<Spinner size="sm" />{/if}
    <span>{said}</span>
  </p>

  {#if pairing.state === 'waiting' && onCancel !== undefined}
    <div>
      <Button variant="secondary" onclick={onCancel}>
        {t('addPrinter.pair.cancel')}
      </Button>
    </div>
  {:else if !open && pairing.state !== 'admitted' && onAgain !== undefined}
    <div>
      <Button variant="secondary" onclick={onAgain}>
        {t('addPrinter.pair.again')}
      </Button>
    </div>
  {/if}
</div>
