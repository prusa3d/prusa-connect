<!--
  The other way in: this server asks a printer on the network to pair, and a
  person presses yes on the printer's own screen. No USB drive and no code to
  type. The card shows the form until a pairing is on its way, then the code
  and where the pairing stands. A refusal from the printer or the server is
  shown above the form, with the settings file named as the way that always
  works.
-->
<script lang="ts">
  import { describeFault, t } from '../../i18n';
  import { Card, CardContent, FaultAlert } from '../../primitives';
  import PairForm from './pair-form.svelte';
  import PairStatus from './pair-status.svelte';
  import type {
    AutoConnect,
    NearbyCandidate,
    PairingRequest,
    PrinterPairingView,
  } from './types';

  let {
    connection,
    pairing,
    nearby,
    busy = false,
    error,
    onPair,
    onCancel,
    onAgain,
  }: {
    connection: AutoConnect;
    pairing: PrinterPairingView | undefined;
    nearby?: readonly NearbyCandidate[] | undefined;
    busy?: boolean;
    error?: unknown;
    onPair: (request: PairingRequest) => void;
    onCancel?: (() => void) | undefined;
    onAgain?: (() => void) | undefined;
  } = $props();

  const fault = $derived(
    error === undefined ? undefined : describeFault(error),
  );
</script>

<Card data-part="pair-at-printer">
  <CardContent class="flex flex-col gap-4">
    <div class="flex flex-col gap-1">
      <h2 class="text-lg font-semibold">{t('addPrinter.pair.heading')}</h2>
      <p class="text-sm text-muted-foreground">{t('addPrinter.pair.lead')}</p>
    </div>

    {#if fault !== undefined}
      <FaultAlert {fault} />
      <p class="text-sm text-muted-foreground" data-part="pair-fallback">
        {t('addPrinter.pair.fallback')}
      </p>
    {/if}

    {#if pairing === undefined || pairing.state === 'withdrawn'}
      <PairForm {connection} {nearby} {busy} {onPair} />
    {:else}
      <PairStatus {pairing} {onCancel} {onAgain} />
    {/if}
  </CardContent>
</Card>
