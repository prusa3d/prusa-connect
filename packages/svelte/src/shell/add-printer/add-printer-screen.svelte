<!--
  Adding a printer, as four steps: the settings file, what to do on the
  printer, waiting for it to ask, and the printer connected.

  The waiting step reacts to what the host hands in, which the host keeps
  current from the event stream, so nothing on this screen asks the server on
  a timer. When a printer joins while the screen waits, the screen shows the
  last step without a click.

  A host that can pair a printer over the network offers that beside the
  first step: the person types the printer's address and presses yes on the
  printer, and the printer joins with no file and no code to type.

  Printers that asked without this guide still need a decision. While the
  guide is on another step, they are listed above the open step with the same
  approve and reject buttons.

  A wide region shows the steps down the side and the open step in a card
  beside them, as the first-run wizard does. A narrow one shows which step of
  how many is open, with a bar.
-->
<script lang="ts">
  import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
  import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
  import { onMount, tick } from 'svelte';

  import { describeFault, formatNumber, t } from '../../i18n';
  import {
    Badge,
    Button,
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    FaultAlert,
    ListingSkeleton,
    PageHeader,
    PageTrail,
    Progress,
    StepList,
  } from '../../primitives';
  import PairAtPrinter from './pair-at-printer.svelte';
  import PendingApprovals from './pending-approvals.svelte';
  import StepConnected from './step-connected.svelte';
  import StepPrepare from './step-prepare.svelte';
  import StepPrinter from './step-printer.svelte';
  import StepWaiting from './step-waiting.svelte';
  import type { AddPrinterScreenProps, AddPrinterStep } from './types';

  let {
    connection,
    admission,
    pending,
    joining,
    joined,
    refused,
    printersHref,
    error,
    authority,
    docsHref,
    helpAfterMs = 150_000,
    initialStep = 'prepare',
    onBegin,
    onApprove,
    onReject,
    onPair,
    onCancelPairing,
    onPairAgain,
    pairing,
    nearby,
    pairingBusy = false,
    pairingError,
  }: AddPrinterScreenProps = $props();

  const ORDER: readonly AddPrinterStep[] = [
    'prepare',
    'printer',
    'waiting',
    'connected',
  ];

  // Which step opens is the caller's, and which step is open from there on is
  // the reader's. Reading the prop once is the whole of that rule.
  // svelte-ignore state_referenced_locally
  let step = $state<AddPrinterStep>(initialStep);
  let heading = $state<HTMLElement>();
  // How many printers were asking and had joined when the person last chose a
  // step. Only a printer beyond those moves the guide on by itself, so a
  // step chosen by hand, Back included, stays open.
  let seen = $state({ asking: 0, joined: 0 });

  const asking = $derived(pending.length + joining.length);

  // The guide follows the printer: one that asks opens the waiting step from
  // the first two steps, and one that joins opens the last step from any step
  // before it, wherever on this page it was approved.
  const shown = $derived.by<AddPrinterStep>(() => {
    if (step !== 'connected' && joined.length > seen.joined) {
      return 'connected';
    }
    if ((step === 'prepare' || step === 'printer') && asking > seen.asking) {
      return 'waiting';
    }
    return step;
  });
  const position = $derived(ORDER.indexOf(shown));
  // On the last step the printer is in, so every step is drawn as done.
  const finished = $derived(shown === 'connected');
  const automatic = $derived(admission === 'automatic');

  const fault = $derived(
    error === undefined ? undefined : describeFault(error),
  );

  const steps = $derived([
    {
      id: 'prepare',
      label: t('addPrinter.steps.prepare'),
      description: t('addPrinter.steps.prepare.caption'),
    },
    {
      id: 'printer',
      label: t('addPrinter.steps.printer'),
      description: t('addPrinter.steps.printer.caption'),
    },
    automatic
      ? {
          id: 'waiting',
          label: t('addPrinter.steps.waiting.automatic'),
          description: t('addPrinter.steps.waiting.automatic.caption'),
        }
      : {
          id: 'waiting',
          label: t('addPrinter.steps.waiting.approval'),
          description: t('addPrinter.steps.waiting.approval.caption'),
        },
    {
      id: 'connected',
      label: t('addPrinter.steps.connected'),
      description: t('addPrinter.steps.connected.caption'),
    },
  ]);

  const title = $derived.by(() => {
    switch (shown) {
      case 'prepare':
        return t('addPrinter.prepare.title');
      case 'printer':
        return t('addPrinter.printer.title');
      case 'waiting':
        return automatic
          ? t('addPrinter.wait.title.automatic')
          : t('addPrinter.wait.title.approval');
      case 'connected':
        return t('addPrinter.connected.heading');
    }
  });

  const lead = $derived.by(() => {
    switch (shown) {
      case 'prepare':
        return t('addPrinter.prepare.lead');
      case 'printer':
        return t('addPrinter.printer.lead');
      case 'waiting':
        return automatic
          ? t('addPrinter.wait.lead.automatic')
          : t('addPrinter.wait.lead.approval');
      case 'connected':
        return t('addPrinter.connected.lead');
    }
  });

  // The printers asking while the guide is elsewhere. On the waiting step the
  // same printers are the step's own cards, so they are not listed twice.
  const elsewhere = $derived(
    shown !== 'waiting' && admission === 'approval' && pending.length > 0,
  );

  // Pairing at the printer's screen is offered beside the first step, which
  // it replaces. A pairing on its way stays in view whatever step the guide
  // moved to, until the printer has joined.
  const pairingShown = $derived(
    onPair !== undefined &&
      shown !== 'connected' &&
      (shown === 'prepare' ||
        (pairing !== undefined && pairing.state !== 'withdrawn')),
  );

  const go = async (next: AddPrinterStep): Promise<void> => {
    step = next;
    seen = { asking, joined: joined.length };
    // The heading takes the focus, so a keyboard and a screen reader start
    // the new step from its top (svelte-5.md, focus on a change of view).
    await tick();
    heading?.focus();
  };

  onMount(() => {
    onBegin();
  });
</script>

<div class="flex flex-col gap-4">
  <!-- This screen is reached from the printers list. -->
  <PageTrail
    trail={[{ label: t('addPrinter.trail.printers'), href: printersHref }]}
    page={t('addPrinter.title')}
  />
  <PageHeader title={t('addPrinter.title')} />

  {#if fault !== undefined}
    <FaultAlert {fault} />
  {/if}

  <!-- Only the admission word holds the guide back. The waiting list is
       read beside it, and the waiting step says it is listening while the
       list arrives, so the guide never disappears under a reload. -->
  {#if admission === undefined}
    <ListingSkeleton label={t('addPrinter.loading.label')} rows={4} />
  {:else}
    <!-- Two columns where the region is wide enough: the steps down the side
         and the open step beside them. A narrow region gets one line and a
         bar instead. The width read is the region's, never the window's. -->
    <div class="@container/add" data-part="wizard" data-step={shown}>
      <div class="flex flex-col gap-4 @3xl/add:flex-row @3xl/add:gap-8">
        <aside class="hidden w-56 shrink-0 pbs-6 @3xl/add:block">
          <StepList
            label={t('addPrinter.steps.label')}
            {steps}
            current={finished ? ORDER.length : position}
            connected
          />
        </aside>

        <div class="flex flex-col gap-2 @3xl/add:hidden" data-part="progress">
          <p class="text-sm text-muted-foreground">
            {t('addPrinter.steps.position', {
              current: position + 1,
              total: ORDER.length,
              step: steps[position]?.label ?? '',
            })}
          </p>
          <!-- The bar draws at its own height, and a class handed in cannot
               take it lower, so this box clips it to a thin line. -->
          <div class="h-1 overflow-hidden rounded-sm">
            <Progress
              value={Math.round(((position + 1) / ORDER.length) * 100)}
              aria-label={t('addPrinter.steps.label')}
            />
          </div>
        </div>

        <div class="flex min-w-0 flex-1 flex-col gap-4 @3xl/add:max-w-2xl">
          {#if elsewhere}
            <Card id="approvals" data-part="approvals">
              <CardHeader>
                <CardTitle>
                  <h2 class="contents">{t('addPrinter.waiting.heading')}</h2>
                  <Badge variant="warning">{formatNumber(pending.length)}</Badge
                  >
                </CardTitle>
              </CardHeader>
              <CardContent class="flex flex-col gap-4">
                <p class="text-sm text-muted-foreground">
                  {t('addPrinter.waiting.note')}
                </p>
                <PendingApprovals {pending} {onApprove} {onReject} />
              </CardContent>
            </Card>
          {/if}

          {#if pairingShown && onPair !== undefined}
            <PairAtPrinter
              {connection}
              {pairing}
              {nearby}
              busy={pairingBusy}
              error={pairingError}
              {onPair}
              onCancel={onCancelPairing}
              onAgain={onPairAgain}
            />
          {/if}

          <Card data-part="step-card">
            <CardContent class="flex flex-col gap-6">
              <header class="flex flex-col gap-1">
                <h2
                  bind:this={heading}
                  tabindex="-1"
                  class="text-xl font-semibold outline-none"
                >
                  {title}
                </h2>
                <p class="text-sm text-muted-foreground">{lead}</p>
              </header>

              {#if shown === 'prepare'}
                <StepPrepare {connection} {authority} {docsHref} />
              {:else if shown === 'printer'}
                <StepPrinter {admission} />
              {:else if shown === 'waiting'}
                <StepWaiting
                  {admission}
                  {pending}
                  {joining}
                  {refused}
                  {helpAfterMs}
                  {onApprove}
                  {onReject}
                />
              {:else}
                <StepConnected
                  {joined}
                  onAnother={() => {
                    onBegin();
                    void go('printer');
                  }}
                />
              {/if}

              {#if shown !== 'connected'}
                <nav
                  aria-label={t('addPrinter.steps.nav')}
                  class="flex items-center justify-between gap-3 border-t border-border pt-4"
                >
                  {#if position > 0}
                    <Button
                      variant="ghost"
                      onclick={() => {
                        void go(ORDER[position - 1] ?? 'prepare');
                      }}
                    >
                      <ArrowLeftIcon aria-hidden="true" />
                      {t('addPrinter.steps.back')}
                    </Button>
                  {:else}
                    <span></span>
                  {/if}
                  {#if shown === 'prepare' || shown === 'printer'}
                    <Button
                      onclick={() => {
                        void go(shown === 'prepare' ? 'printer' : 'waiting');
                      }}
                    >
                      {t('addPrinter.steps.next')}
                      <ArrowRightIcon aria-hidden="true" />
                    </Button>
                  {/if}
                </nav>
              {/if}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  {/if}
</div>
