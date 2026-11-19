<!--
  Step three: the screen waits for the printer to ask. It reacts the moment a
  code arrives on the stream, with one card per printer, so several printers
  added at once each get their own. A printer that was let in stays here,
  marked as joining, until it dials in. A printer turned away at the door is
  said so, with what to do about it.
-->
<script lang="ts">
  import OctagonAlertIcon from '@lucide/svelte/icons/octagon-alert';

  import { t } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    ConfirmDialog,
    Spinner,
  } from '../../primitives';
  import { printerModelLabelOf } from '../printers/presentation';
  import PendingCard from './pending-card.svelte';
  import type {
    JoiningPrinter,
    PendingPrinter,
    PrinterAdmission,
    RefusedPrinter,
  } from './types';
  import WaitingHelp from './waiting-help.svelte';

  let {
    admission,
    pending,
    joining,
    refused,
    helpAfterMs,
    onApprove,
    onReject,
  }: {
    admission: PrinterAdmission;
    pending: readonly PendingPrinter[];
    joining: readonly JoiningPrinter[];
    refused: RefusedPrinter | undefined;
    helpAfterMs: number;
    onApprove: (code: string) => void;
    onReject: (code: string) => void;
  } = $props();

  let helpOpen = $state(false);
  let rejecting = $state<PendingPrinter | null>(null);

  const quiet = $derived(pending.length === 0 && joining.length === 0);
  const refusedTitle = $derived(
    refused?.model === undefined
      ? t('addPrinter.wait.refused.titleUnknown')
      : t('addPrinter.wait.refused.title', { printer: refused.model }),
  );

  // The help opens by itself when nothing has happened for a while. The
  // timer is the outside world, so it lives in an effect with its cleanup,
  // and it starts again whenever the step goes quiet again.
  $effect(() => {
    if (!quiet) return;
    const timer = setTimeout(() => {
      helpOpen = true;
    }, helpAfterMs);

    return () => {
      clearTimeout(timer);
    };
  });
</script>

<div class="flex flex-col gap-5" data-part="step-waiting">
  {#if refused !== undefined}
    <Alert
      variant="destructive"
      data-part="refused"
      data-reason={refused.reason}
    >
      <OctagonAlertIcon aria-hidden="true" />
      <AlertTitle>{refusedTitle}</AlertTitle>
      <AlertDescription>
        {refused.reason === 'not_admitted'
          ? t('addPrinter.wait.refused.notAdmitted')
          : t('addPrinter.wait.refused.unknownCredentials')}
      </AlertDescription>
    </Alert>
  {/if}

  <div aria-live="polite" class="flex flex-col gap-4">
    {#if quiet}
      <div
        class="flex items-start gap-3 rounded-md border border-dashed border-border p-4"
        data-part="listening"
      >
        <Spinner size="md" tone="primary" class="mt-0.5" />
        <div class="flex flex-col gap-1">
          <p class="font-medium">{t('addPrinter.wait.listening')}</p>
          <p class="text-sm text-muted-foreground">
            {admission === 'automatic'
              ? t('addPrinter.wait.listeningNote.automatic')
              : t('addPrinter.wait.listeningNote.approval')}
          </p>
        </div>
      </div>
    {/if}

    {#each pending as printer (printer.code)}
      <PendingCard
        {printer}
        {admission}
        {onApprove}
        onReject={(asked: PendingPrinter) => {
          rejecting = asked;
        }}
      />
    {/each}

    {#each joining as printer (printer.code)}
      <div
        class="flex items-start gap-3 rounded-md border border-border p-4"
        data-part="joining"
      >
        <Spinner size="md" tone="primary" class="mt-0.5" />
        <div class="flex flex-col gap-1">
          <p class="font-medium">
            {t('addPrinter.wait.approved', {
              printer:
                printer.model === undefined
                  ? t('addPrinter.unknownModel')
                  : printerModelLabelOf(printer.model),
            })}
          </p>
          <p class="text-sm text-muted-foreground">
            {t('addPrinter.wait.approvedNote')}
          </p>
        </div>
      </div>
    {/each}
  </div>

  <WaitingHelp bind:open={helpOpen} />
</div>

<ConfirmDialog
  open={rejecting !== null}
  onOpenChange={(open: boolean) => {
    if (!open) rejecting = null;
  }}
  title={rejecting?.model === undefined
    ? t('addPrinter.reject.unknownTitle')
    : t('addPrinter.reject.title', { printer: rejecting.model })}
  description={t('addPrinter.reject.note')}
  confirmLabel={t('addPrinter.reject')}
  onConfirm={() => {
    if (rejecting !== null) onReject(rejecting.code);
    rejecting = null;
  }}
/>
