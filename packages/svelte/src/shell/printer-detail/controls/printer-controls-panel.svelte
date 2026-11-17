<script lang="ts">
  import {
    activeMaterialOf,
    FLOW_PATH,
    printerStateOf,
    SPEED_PATH,
    toolsOf,
    type PrinterState,
  } from '../../../domain';
  import { t } from '../../../i18n';
  import {
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    EmptyState,
    NozzleIcon,
    Skeleton,
    SpeedIcon,
  } from '../../../primitives';
  import {
    bedTravelsOnZ,
    chamberMaxCelsius,
    declaresArgument,
    readingOf,
    resolverFor,
    stateOf,
    VERBS,
  } from './machine-controls';
  import MachineChamber from './machine-chamber.svelte';
  import MachineMotion from './machine-motion.svelte';
  import MachineNozzles from './machine-nozzles.svelte';
  import MachineTemperatures from './machine-temperatures.svelte';
  import MachineTuningRow from './machine-tuning-row.svelte';
  import MachineUtilities from './machine-utilities.svelte';
  import type { PrinterControlsPanelProps } from './types';

  let {
    device,
    telemetry,
    commands,
    loading = false,
  }: PrinterControlsPanelProps = $props();

  const waiting = $derived(loading || device === undefined);

  // A disconnected machine reports nothing.
  const state: PrinterState = $derived(
    !device?.connected
      ? 'OFFLINE'
      : printerStateOf(stateOf(device) ?? 'UNKNOWN'),
  );
  const reporting = $derived(state !== 'OFFLINE' && state !== 'UNKNOWN');

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
      : resolverFor(device, commands),
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

  const send = (verb: string, argument: string, next: number) =>
    commands.onCommand(verb, { [argument]: String(Math.round(next)) });
</script>

{#snippet speedIcon()}
  <SpeedIcon aria-hidden="true" class="size-4 text-muted-foreground" />
{/snippet}

{#snippet flowIcon()}
  <NozzleIcon aria-hidden="true" class="size-4 text-muted-foreground" />
{/snippet}

{#if waiting}
  <Card aria-busy="true" aria-label={t('printerDetailControls.loading')}>
    <CardContent class="@container">
      <div class="grid gap-6 @2xl:grid-cols-2">
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
      <!-- Utilities is reached after the right column and drawn under. -->
      <div class="grid gap-6 @2xl:grid-cols-2">
        <MachineMotion
          {resolve}
          {reading}
          {busy}
          onCommand={commands.onCommand}
          printing={PRINTING_LIKE.includes(state)}
          bedOnZ={bedTravelsOnZ(device.model)}
        />

        <div
          class="flex flex-col gap-4 @2xl:col-start-2 @2xl:row-span-2 @2xl:row-start-1 @2xl:border-s @2xl:border-border @2xl:ps-6"
        >
          <MachineTemperatures
            {resolve}
            {reading}
            {busy}
            onCommand={commands.onCommand}
            material={activeMaterialOf(telemetry, device)}
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

        <MachineUtilities
          {resolve}
          {busy}
          onCommand={commands.onCommand}
          ready={state === 'READY'}
        />
      </div>
    </CardContent>
  </Card>
{/if}
