<script lang="ts">
  import { untrack, type Snippet } from 'svelte';

  import {
    activeMaterialOf,
    FLOW_PATH,
    printerStateOf,
    SPEED_PATH,
    toolsOf,
    STATE_PATH,
    type PrinterState,
  } from '../../../domain';
  import { t } from '../../../i18n';
  import {
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    Collapsible,
    CollapsibleContent,
    CollapsibleTrigger,
    EmptyState,
    NozzleIcon,
    Skeleton,
    SpeedIcon,
  } from '../../../primitives';
  import PrinterCommandsPanel from '../commands/printer-commands-panel.svelte';
  import {
    bedTravelsOnZ,
    chamberMaxCelsius,
    declaresArgument,
    readingOf,
    resolverFor,
    textOf,
    VERBS,
  } from './machine-controls';
  import MachineChamber from './machine-chamber.svelte';
  import MachineMotion from './machine-motion.svelte';
  import MachineNozzles from './machine-nozzles.svelte';
  import MachineTemperatures from './machine-temperatures.svelte';
  import MachineTuningRow from './machine-tuning-row.svelte';
  import { getMotionPreview } from './motion-preview';
  import type { PrinterControlsPanelProps } from './types';

  let {
    device,
    telemetry,
    commands,
    loading = false,
    advanced,
    material,
  }: PrinterControlsPanelProps = $props();

  const motionPreview = getMotionPreview();
  const waiting = $derived(loading || device === undefined);

  // A disconnected machine reports nothing.
  const observedState: PrinterState = $derived(
    !device?.connected
      ? 'OFFLINE'
      : printerStateOf(textOf(telemetry, device, STATE_PATH) ?? 'UNKNOWN'),
  );
  const previewEnabled = $derived(motionPreview !== undefined);
  const reporting = $derived(
    observedState !== 'OFFLINE' && observedState !== 'UNKNOWN',
  );

  // Which wording the reset dialog takes.
  const PRINTING_LIKE: readonly PrinterState[] = [
    'PRINTING',
    'PAUSED',
    'BUSY',
    'ATTENTION',
  ];

  const busy = $derived(commands.sending === true);
  const resolve = $derived(
    device === undefined
      ? () => ({ kind: 'absent' as const })
      : resolverFor(
          device,
          commands,
          device.connected ? textOf(telemetry, device, STATE_PATH) : undefined,
        ),
  );
  const reading = $derived((path: string) =>
    device === undefined ? undefined : readingOf(telemetry, device, path),
  );

  // Every head the tool tree holds. A printer with more than one gets a row
  // per head under the extruder (ADR-20261023-gna rule 12).
  const heads = $derived(
    device === undefined ? [] : toolsOf(telemetry, device),
  );
  const slots = $derived(heads.length > 1 ? heads : []);

  const speed = $derived(resolve(VERBS.speed));
  const flow = $derived(resolve(VERBS.flow));
  const offersSomething = $derived(
    Object.values(VERBS).some((verb) => resolve(verb).kind !== 'absent'),
  );

  const basicCommands = $derived.by(() => {
    if (!device || !reporting || !offersSomething) return new Set<string>();
    return new Set<string>([
      VERBS.bed,
      VERBS.beep,
      observedState === 'READY'
        ? VERBS.cancelPrinterReady
        : VERBS.setPrinterReady,
      VERBS.chamberFan,
      VERBS.chamberLight,
      VERBS.disableSteppers,
      VERBS.flow,
      VERBS.hostname,
      VERBS.meshBedLevelling,
      VERBS.nozzle,
      VERBS.resetPrinter,
      VERBS.setIdle,
      VERBS.speed,
      VERBS.toolDiameter,
      VERBS.toolHardened,
      VERBS.toolHighFlow,
      // Controls homes all axes. The catalogue also accepts one.
      ...(declaresArgument(device, VERBS.home, 'axis') ? [] : [VERBS.home]),
    ]);
  });

  let basicPanel = $state<HTMLDivElement | null>(null);
  let advancedOpen = $state(false);

  $effect(() => {
    const target = basicPanel;
    if (waiting || !advanced?.requested || target === null) return;
    const verb = advanced.highlight;
    if (verb && untrack(() => basicCommands.has(verb))) {
      advancedOpen = false;
      const frame = requestAnimationFrame(() => target.focus());
      return () => cancelAnimationFrame(frame);
    } else {
      advancedOpen = true;
    }
  });

  const send = (verb: string, argument: string, next: number) =>
    commands.onCommand(verb, { [argument]: String(Math.round(next)) });
</script>

{#snippet speedIcon()}
  <SpeedIcon aria-hidden="true" class="size-4 text-muted-foreground" />
{/snippet}

{#snippet flowIcon()}
  <NozzleIcon aria-hidden="true" class="size-4 text-muted-foreground" />
{/snippet}

{#snippet preview(commandPanel: Snippet<[Snippet]> | undefined)}
  {#if device}
    {#key device.id}
      {@render motionPreview?.(device, observedState, commandPanel)}
    {/key}
  {/if}
{/snippet}

{#if advanced && !waiting}
  <div class="flex flex-col gap-6">
    <div
      bind:this={basicPanel}
      tabindex="-1"
      role="group"
      aria-label={t('printerDetailControls.heading')}
    >
      {@render basic()}
    </div>
    <div class="advanced-controls">
      <Collapsible bind:open={advancedOpen} data-part="advanced-controls">
        <h2 class="advanced-heading">
          <CollapsibleTrigger variant="row">
            {t('printerDetail.tab.commands')}
          </CollapsibleTrigger>
        </h2>
        <CollapsibleContent class="advanced-content">
          {#if advancedOpen}
            <PrinterCommandsPanel
              {device}
              {telemetry}
              {basicCommands}
              commands={advanced.commands}
              highlight={advanced.highlight}
              readingDrive={commands.readingDrive}
            />
          {/if}
        </CollapsibleContent>
      </Collapsible>
    </div>
  </div>
{:else}
  {@render basic()}
{/if}

{#snippet basic()}
  {#if waiting}
    <Card aria-busy="true" aria-label={t('printerDetailControls.loading')}>
      <CardContent class="@container">
        <div class="grid gap-6 @max-2xl:grid-cols-1 @2xl:grid-cols-2">
          <div class="flex flex-col gap-3">
            <Skeleton class="h-5 w-32" />
            <Skeleton class="h-11 w-48" />
            <Skeleton class="h-40 w-40" />
          </div>
          <div class="flex flex-col gap-3">
            <Skeleton class="h-5 w-32" />
            <Skeleton class="h-16 w-full" />
            <Skeleton class="h-16 w-full" />
            <Skeleton class="h-24 w-full" />
          </div>
        </div>
      </CardContent>
    </Card>
  {:else if device && previewEnabled && (!reporting || !offersSomething)}
    <Card>
      <CardContent>
        {@render preview(undefined)}
      </CardContent>
    </Card>
  {:else if device && !reporting}
    <p class="text-sm text-muted-foreground">
      {t('printerDetailControls.notReporting', { name: device.name })}
    </p>
  {:else if device && !offersSomething}
    <Card>
      <CardContent>
        <EmptyState
          heading={t('printerDetailControls.none.heading')}
          headingLevel="h2"
          description={t('printerDetailControls.none.detail')}
        />
      </CardContent>
    </Card>
  {:else if device}
    <Card>
      <CardHeader>
        <CardTitle>
          <h2 class="contents">{t('printerDetailControls.heading')}</h2>
        </CardTitle>
      </CardHeader>
      <CardContent class="@container">
        <div
          class="grid gap-6 @max-3xl:grid-cols-1 @3xl:grid-cols-2 @3xl:gap-x-panel-padding"
        >
          <MachineMotion
            {resolve}
            {reading}
            {busy}
            homing={commands.pending === VERBS.home}
            onCommand={commands.onCommand}
            printing={PRINTING_LIKE.includes(observedState)}
            bedOnZ={bedTravelsOnZ(device.model)}
            buildVolume={device.build_volume}
            printerState={observedState}
            motion={previewEnabled ? preview : undefined}
          />

          <div
            class="flex min-w-0 flex-col gap-4 empty:hidden @3xl:col-start-2 @3xl:row-span-2 @3xl:row-start-1 @3xl:border-s @3xl:border-border @3xl:ps-panel-padding"
          >
            <MachineTemperatures
              {resolve}
              {reading}
              {busy}
              onCommand={commands.onCommand}
              material={activeMaterialOf(telemetry, device)}
              {device}
              {telemetry}
              assign={material}
              {slots}
              perTool={declaresArgument(device, VERBS.nozzle, 'tool')}
              chamberMax={chamberMaxCelsius(device.model)}
            />

            {#if speed.kind !== 'absent'}
              <MachineTuningRow
                label={t('printerDetailControls.speed.label')}
                icon={speedIcon}
                percent={reading(SPEED_PATH)}
                projection={speed}
                {busy}
                onCommit={(next: number) => send(VERBS.speed, 'speed', next)}
              />
            {/if}

            {#if flow.kind !== 'absent'}
              <MachineTuningRow
                label={t('printerDetailControls.flow.label')}
                icon={flowIcon}
                percent={reading(FLOW_PATH)}
                projection={flow}
                {busy}
                onCommit={(next: number) => send(VERBS.flow, 'flow', next)}
              />
            {/if}

            <MachineChamber
              {resolve}
              {reading}
              {busy}
              onCommand={commands.onCommand}
            />

            <MachineNozzles
              {resolve}
              {busy}
              onCommand={commands.onCommand}
              {heads}
            />
          </div>
        </div>
      </CardContent>
    </Card>
  {/if}
{/snippet}

<style>
  .advanced-controls {
    container: advanced / inline-size;
    --advanced-padding-inline: var(--spacing-panel-padding-mobile);
  }

  @container advanced (width >= 32rem) {
    .advanced-heading {
      --advanced-padding-inline: var(--spacing-panel-padding);
    }
  }

  .advanced-heading :global([data-slot='collapsible-trigger']) {
    padding-block: calc(
      (var(--spacing-section-block) + var(--spacing-form-note)) / 2
    );
    padding-inline: var(--advanced-padding-inline);
    background: var(--color-card);
    color: var(--color-card-foreground);
    font-size: var(--text-base);
    border-end-start-radius: var(--radius-md);
    border-end-end-radius: var(--radius-md);
  }

  .advanced-heading :global([data-slot='collapsible-trigger'] > svg) {
    order: 1;
    margin-inline-start: auto;
  }

  :global(:root:not(.dark))
    .advanced-heading
    :global([data-slot='collapsible-trigger']) {
    border-color: var(--color-panel-outline);
  }

  .advanced-controls:has(:global([data-part='embedded-command-catalogue']))
    .advanced-heading
    :global([data-state='open']) {
    border-block-end-color: transparent;
    border-end-start-radius: 0;
    border-end-end-radius: 0;
  }

  .advanced-controls :global([data-part='embedded-command-catalogue']) {
    border-block-start: 0;
    border-start-start-radius: 0;
    border-start-end-radius: 0;
  }

  .advanced-controls
    :global(
      [data-part='embedded-command-catalogue']
        > [data-slot='card-content']:first-child
    ) {
    padding-block-start: var(--spacing-form-gap);
  }

  .advanced-controls
    :global(
      .advanced-content:not(:has([data-part='embedded-command-catalogue']))
    ) {
    padding-block-start: calc(var(--spacing) * 4);
  }
</style>
