<!--
  One printer that asked to join, as the waiting step shows it: what it says it
  is, and the code and fingerprint a person compares with its display before
  letting it in. In automatic admission nobody decides, so the card says the
  printer is joining instead of offering the two buttons.
-->
<script lang="ts">
  import CheckIcon from '@lucide/svelte/icons/check';
  import XIcon from '@lucide/svelte/icons/x';

  import { t } from '../../i18n';
  import { Button, PrinterIcon, Spinner } from '../../primitives';
  import {
    printerModelOf,
    printerModelLabelOf,
  } from '../printers/presentation';
  import { formatFingerprint } from './presentation';
  import type { PendingPrinter, PrinterAdmission } from './types';

  let {
    printer,
    admission,
    onApprove,
    onReject,
  }: {
    printer: PendingPrinter;
    admission: PrinterAdmission;
    onApprove: (code: string) => void;
    onReject: (printer: PendingPrinter) => void;
  } = $props();

  // The code and the fingerprint are the pair a person checks against the
  // printer's display, so both are drawn at the heavier weight the
  // approvals table uses.
  const COMPARE = 'font-mono font-semibold text-foreground tabular-nums';

  const name = $derived(
    printer.model === undefined
      ? t('addPrinter.unknownModel')
      : printerModelLabelOf(printer.model),
  );
  const facts = $derived(
    [
      printer.serial,
      printer.firmware === undefined
        ? undefined
        : t('addPrinter.row.firmware', { version: printer.firmware }),
    ].filter((fact) => fact !== undefined),
  );
</script>

<!-- A bordered panel, not a second card: it sits inside the step's card, and
     a card's own padding there pushed the code away from the text above. -->
<div
  class="@container/pending rounded-md border border-border p-4"
  data-part="pending-card"
  data-code={printer.code}
>
  <div class="flex flex-col gap-4">
    <div class="flex items-start gap-3">
      <PrinterIcon
        model={printerModelOf(printer.icon ?? printer.model ?? '')}
        reported={printer.reported}
        class="size-14 shrink-0"
      />
      <div class="flex min-w-0 flex-col gap-0.5">
        <h3 class="text-base font-semibold">{name}</h3>
        {#each facts as fact (fact)}
          <span class="truncate text-xs text-muted-foreground">{fact}</span>
        {/each}
      </div>
    </div>

    <div class="flex flex-col gap-1">
      <span class="text-sm text-muted-foreground">
        {t('addPrinter.wait.compare')}
      </span>
      <span class={[COMPARE, 'text-3xl tracking-wider']} data-part="code">
        {printer.code}
      </span>
      <span class="text-xs text-muted-foreground">
        {t('addPrinter.wait.fingerprint')}
        <span class={[COMPARE, 'text-sm']}>
          {formatFingerprint(printer.fingerprint)}
        </span>
      </span>
    </div>

    {#if admission === 'automatic'}
      <p class="flex items-center gap-2 text-sm" aria-live="polite">
        <Spinner tone="primary" />
        {t('addPrinter.wait.joining', { printer: name })}
      </p>
    {:else}
      <!-- `lg`: admitting an unknown printer onto the network is a higher
           stakes press than an ordinary row action. -->
      <div class="flex flex-wrap gap-2">
        <Button
          size="lg"
          class="flex-1 basis-40 @lg/pending:flex-none"
          aria-label={t('addPrinter.approveFor', {
            printer: name,
            code: printer.code,
          })}
          onclick={() => {
            onApprove(printer.code);
          }}
        >
          <CheckIcon aria-hidden="true" />
          {t('addPrinter.approve')}
        </Button>
        <Button
          variant="destructive"
          size="lg"
          class="flex-1 basis-40 @lg/pending:flex-none"
          aria-label={t('addPrinter.rejectFor', {
            printer: name,
            code: printer.code,
          })}
          onclick={() => {
            onReject(printer);
          }}
        >
          <XIcon aria-hidden="true" />
          {t('addPrinter.reject')}
        </Button>
      </div>
    {/if}
  </div>
</div>
