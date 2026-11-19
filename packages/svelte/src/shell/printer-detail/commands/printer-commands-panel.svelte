<script lang="ts">
  import { onMount, type Snippet } from 'svelte';

  import {
    FLOW_PATH,
    LIFECYCLE_TRANSITIONS,
    SPEED_PATH,
    numberAt,
    printerStateOf,
    STATE_PATH,
    textAt,
  } from '../../../domain';
  import { describeFault, t } from '../../../i18n';
  import { FaultAlert, Skeleton } from '../../../primitives';
  import {
    bedTravelsOnZ,
    readingOf,
    resolverFor,
    textOf,
    VERBS,
  } from '../controls/machine-controls';
  import MachineMotion from '../controls/machine-motion.svelte';
  import { getMotionPreview } from '../controls/motion-preview';
  import { jobOf } from '../status/presentation';
  import CommandCatalogue from './command-catalogue.svelte';
  import GcodeConsole from './gcode-console.svelte';
  import { verbsOf, type PrinterCommandsPanelProps } from './types';
  import { isOfferedHere } from './vocabulary';

  let {
    commands,
    device,
    telemetry,
    transitions,
    highlight,
    locked = false,
    loading = false,
    readingDrive = false,
    basicCommands,
  }: PrinterCommandsPanelProps = $props();

  // How long the mark stays lit.
  const MARK_MS = 4000;

  let panel = $state<HTMLElement | null>(null);
  let marked = $state<string | undefined>(undefined);

  onMount(() => {
    void commands.loadPresets();
  });

  // Not `state`: that name shadows the `$state` rune for svelte2tsx.
  const printerState = $derived.by(() => {
    if (!device) return undefined;
    return telemetry
      ? textOf(telemetry, device, STATE_PATH)
      : textAt(device, STATE_PATH);
  });

  const motionPreview = getMotionPreview();
  const observedState = $derived(
    device && !device.connected
      ? 'OFFLINE'
      : printerStateOf(printerState ?? 'UNKNOWN'),
  );
  const activeJob = $derived.by(() => {
    if (!device) return undefined;
    const running = observedState === 'PRINTING' || observedState === 'PAUSED';
    const interrupted =
      device.job?.running === true &&
      (observedState === 'ATTENTION' ||
        observedState === 'BUSY' ||
        observedState === 'MANIPULATING');
    return running || interrupted ? jobOf(telemetry, device) : undefined;
  });
  const motionPort = $derived({
    transitions: transitions ?? LIFECYCLE_TRANSITIONS,
    locked,
    sending: commands.sending,
    onCommand: (verb: string, args?: Readonly<Record<string, string>>) =>
      commands.send(verb, args),
  });
  // What every row and the coordinate plate are projected through.
  const resolve = $derived(
    device
      ? resolverFor(
          device,
          motionPort,
          device.connected ? printerState : undefined,
        )
      : undefined,
  );
  const reading = (path: string) => {
    if (!device) return undefined;
    return telemetry
      ? readingOf(telemetry, device, path)
      : numberAt(device, path);
  };
  const motionAvailable = $derived(
    basicCommands === undefined &&
      (motionPreview !== undefined || device?.build_volume != null) &&
      (device?.capabilities?.some(
        (verb) => verb === VERBS.move || verb === VERBS.moveZ,
      ) ??
        false),
  );

  const rows = $derived(
    resolve === undefined
      ? []
      : verbsOf(device?.capabilities ?? null)
          .filter(
            (entry) =>
              isOfferedHere(entry.verb) && !basicCommands?.has(entry.verb),
          )
          .map((entry) => ({ entry, projection: resolve(entry.verb) })),
  );

  // The verb to mark, once there is a row carrying it.
  const wanted = $derived(
    rows.some((listing) => listing.entry.verb === highlight)
      ? highlight
      : undefined,
  );

  const refusal = $derived(
    commands.error === undefined ? undefined : describeFault(commands.error),
  );

  const send = (verb: string, args?: Readonly<Record<string, string>>) => {
    void commands.send(verb, args);
  };

  // The mark expires even when the requested command stays the same.
  $effect(() => {
    const verb = wanted;
    const target = panel;
    if (verb === undefined || target === null) return;

    marked = verb;

    const frame = requestAnimationFrame(() => {
      const row = target.querySelector(`#${CSS.escape(`command-${verb}`)}`);
      if (!(row instanceof HTMLElement)) return;

      const reduced = window.matchMedia('(prefers-reduced-motion: reduce)');
      row.scrollIntoView({
        block: 'center',
        behavior: reduced.matches ? 'auto' : 'smooth',
      });
      row.focus({ preventScroll: true });
    });

    const timer = setTimeout(() => {
      marked = undefined;
    }, MARK_MS);
    return () => {
      cancelAnimationFrame(frame);
      clearTimeout(timer);
    };
  });
</script>

<div
  bind:this={panel}
  class="@container/commands flex flex-col gap-6"
  aria-busy={commands.sending}
>
  <FaultAlert fault={refusal} />

  {#if loading}
    <!-- The panel's own shape (ADR-20260917-a7d rule 7). -->
    <div
      class="flex flex-col gap-3 rounded-md border border-border p-6"
      aria-label={t('printerDetailCommands.loading.label')}
      role="status"
    >
      <Skeleton class="h-5 w-32" />
      {#each [0, 1, 2, 3, 4] as row (row)}
        <Skeleton class="h-8 w-full" />
      {/each}
    </div>
  {:else}
    {#if rows.length > 0 || basicCommands === undefined}
      <CommandCatalogue
        {rows}
        {marked}
        embedded={basicCommands !== undefined}
        printerState={observedState}
        {activeJob}
        tuning={{ speed: reading(SPEED_PATH), flow: reading(FLOW_PATH) }}
        motion={motionAvailable ? coordinates : undefined}
        sending={commands.sending}
        pending={commands.pending}
        {readingDrive}
        onSend={send}
      />
    {/if}
    <GcodeConsole {commands} />
  {/if}
</div>

{#snippet coordinates()}
  {#if device && resolve}
    {#key device.id}
      <MachineMotion
        {resolve}
        {reading}
        busy={commands.sending}
        homing={commands.pending === VERBS.home}
        onCommand={motionPort.onCommand}
        printing={observedState === 'PRINTING' || observedState === 'PAUSED'}
        bedOnZ={bedTravelsOnZ(device.model)}
        buildVolume={device.build_volume}
        printerState={observedState}
        motion={motionPreview ? preview : undefined}
        coordinatesOnly
      />
    {/key}
  {/if}
{/snippet}

{#snippet preview(commandPanel: Snippet<[Snippet]>)}
  {#if device}
    {@render motionPreview?.(device, observedState, commandPanel)}
  {/if}
{/snippet}
