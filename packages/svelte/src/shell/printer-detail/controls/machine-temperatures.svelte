<script lang="ts">
  import type { Snippet } from 'svelte';

  import {
    nozzlePath,
    nozzleTargetPath,
    PREHEAT_PRESETS,
    type PreheatPreset,
    type Device,
    type Reading,
    readingName,
    readingWithRole,
    type ToolReading,
  } from '../../../domain';
  import { formatUnit, t } from '../../../i18n';
  import {
    HeaterRow,
    TemperatureIcon,
    ValueEditor,
    type ValuePreset,
  } from '../../../primitives';
  import { toolLabel } from '../../printers/tool-label';
  import FilamentPositions from '../filament-positions.svelte';
  import type { TelemetrySource } from '../types';
  import NozzleEditor from './nozzle-editor.svelte';
  import {
    BED_MAX_CELSIUS,
    blocked,
    degreeScale,
    groupReason,
    READING_TILE,
    SECTION_TITLE,
    VERBS,
    type SectionProps,
  } from './machine-controls';

  let {
    resolve,
    reading,
    busy,
    onCommand,
    material,
    slots = [],
    perTool = false,
    chamberMax,
    device,
    telemetry,
    assign,
    readings = [],
  }: SectionProps & {
    material: string | undefined;
    device?: Device | undefined;
    telemetry?: TelemetrySource | undefined;
    /**
     * Draws the material of a tool the reader may assign, handed its id and
     * the chip to fall back on: the Tools table's cell, so both assign alike.
     */
    assign?: Snippet<[string, Snippet]> | undefined;
    /** The readings the printer describes, the bed first. */
    readings?: readonly Reading[];
    /** The toolheads of a tool changer. Empty draws nothing. */
    slots?: readonly ToolReading[];
    /** Whether the nozzle verb takes a tool, so each head is set alone. */
    perTool?: boolean;
    /** The highest chamber target the model takes, in Celsius. */
    chamberMax: number;
  } = $props();

  // The nozzle row shows the head printing now, and head 0 of a printer that
  // has one nozzle or picks none. The tree is the only place it is read.
  const head = $derived(slots.find((slot) => slot.active)?.index ?? 0);
  const headNozzlePath = $derived(nozzlePath(head));
  const headTargetPath = $derived(nozzleTargetPath(head));

  const nozzle = $derived(resolve(VERBS.nozzle));
  const bed = $derived(resolve(VERBS.bed));
  const chamber = $derived(resolve(VERBS.chamberTemp));
  const reason = $derived(groupReason([nozzle, bed, chamber]));

  // The set controls stay where the verbs are: the bed and the chamber.
  const bedReading = $derived(readingWithRole(readings, 'bed'));
  const chamberReading = $derived(readingWithRole(readings, 'chamber'));

  const ROLE_LABEL = {
    bed: 'printerDetailControls.bed.label',
    chamber: 'printerDetailControls.chamberTemp.label',
  } as const;

  const labelOf = (one: Reading): string =>
    one.role === undefined ? readingName(one) : t(ROLE_LABEL[one.role]);

  const targetOf = (one: Reading | undefined): number | undefined =>
    one?.targetPath === undefined ? undefined : reading(one.targetPath);

  const celsius = (value: number | undefined): string =>
    value === undefined
      ? t('printerDetailControls.noReading')
      : formatUnit(value, '°C', { maximumFractionDigits: 0 });

  // The rows read in the unit the reader chose, so the editors do too.
  const scale = $derived(degreeScale());

  const presetsFor = (
    column: (preset: PreheatPreset) => number,
  ): ValuePreset[] => [
    { label: t('printerDetailControls.preset.off'), value: scale.shown(0) },
    ...PREHEAT_PRESETS.map((preset) => ({
      label: preset.material,
      value: scale.shown(column(preset)),
    })),
  ];

  // A head with a feeder holds no material, its slots do, and a head the
  // printer switched off holds none to assign.
  const assignable = (id: number): boolean => {
    const tool = device?.tools?.tools[String(id)];

    return tool !== null && !tool?.feeder?.slot_count;
  };

  const bedPresets = $derived(presetsFor((preset) => preset.bedCelsius));

  const send = (verb: string, argument: string, next: number) =>
    onCommand(verb, { [argument]: String(Math.round(scale.celsius(next))) });
</script>

<section class="flex flex-col gap-3">
  <h3 class={SECTION_TITLE}>
    <TemperatureIcon aria-hidden="true" class="size-4 text-muted-foreground" />
    {t('printerDetailControls.temperatures.heading')}
  </h3>

  <!-- One row per heater, each the full width of the column. -->
  <div class="grid gap-2">
    {#if slots.length === 0 || !perTool}
      <HeaterRow
        class={READING_TILE}
        label={t('printerDetailControls.nozzle.label')}
        kind="tool"
        nozzleTemp={reading(headNozzlePath)}
        targetTemp={reading(headTargetPath)}
        material={material ?? t('printerDetailControls.noFilament')}
        {...assign && assignable(head) ? { materialChip: headChip } : {}}
        {...nozzle.kind === 'absent' ? {} : { action: nozzleEditor }}
      />
    {/if}
    {#if slots.length > 0}
      <div class={READING_TILE}>
        <h4 class="text-xs font-medium text-muted-foreground">
          {t('printerDetailControls.tools.heading')}
        </h4>
        {#each slots as slot (slot.index)}
          {@const target = slot.target}
          <HeaterRow
            label={toolLabel(slot.index)}
            kind="tool"
            nozzleTemp={slot.temperature}
            targetTemp={target}
            material={slot.material ?? t('printerDetailControls.noFilament')}
            active={slot.active}
            {...assign && assignable(slot.index)
              ? { materialChip: toolChip }
              : {}}
            {...perTool && nozzle.kind !== 'absent'
              ? { action: toolEditor }
              : {}}
          />

          {#snippet toolChip()}{@render chip(slot.index)}{/snippet}
          {#snippet toolEditor()}
            <NozzleEditor
              {resolve}
              {busy}
              {onCommand}
              perTool
              temperature={slot.temperature}
              target={slot.target}
              tool={slot.index}
            />
          {/snippet}
        {/each}
      </div>
    {/if}
    {#each readings as one (one.id)}
      {@const settable =
        (one === bedReading && bed.kind !== 'absent') ||
        (one === chamberReading && chamber.kind !== 'absent')}
      <HeaterRow
        class={READING_TILE}
        label={labelOf(one)}
        kind={one.role ?? 'chamber'}
        nozzleTemp={reading(one.valuePath)}
        targetTemp={targetOf(one)}
        {...settable
          ? { action: one === bedReading ? bedEditor : chamberEditor }
          : {}}
      />
    {/each}
  </div>

  {#if reason}
    <p class="text-xs text-muted-foreground">{reason}</p>
  {/if}
</section>

{#snippet headChip()}{@render chip(head)}{/snippet}

{#snippet chip(id: number)}
  {#snippet own()}
    {#if device !== undefined && telemetry !== undefined}
      <FilamentPositions {device} {telemetry} only={String(id)} />
    {/if}
  {/snippet}
  {#if assign}{@render assign(String(id), own)}{/if}
{/snippet}

{#snippet nozzleEditor()}
  <NozzleEditor
    {resolve}
    {busy}
    {onCommand}
    {perTool}
    temperature={reading(headNozzlePath)}
    target={reading(headTargetPath)}
  />
{/snippet}

{#snippet bedEditor()}
  <ValueEditor
    label={t('printerDetailControls.bed.label')}
    currentLabel={celsius(
      bedReading === undefined ? undefined : reading(bedReading.valuePath),
    )}
    target={scale.shown(targetOf(bedReading) ?? 0)}
    unit={scale.unit}
    min={scale.shown(0)}
    max={scale.shown(BED_MAX_CELSIUS)}
    step={scale.step}
    presets={bedPresets}
    disabled={blocked(bed, busy)}
    announceTarget={targetOf(bedReading) !== undefined}
    onCommit={(next: number) => send(VERBS.bed, 'bed_temperature', next)}
  />
{/snippet}

{#snippet chamberEditor()}
  <ValueEditor
    label={t('printerDetailControls.chamberTemp.label')}
    currentLabel={celsius(
      chamberReading === undefined
        ? undefined
        : reading(chamberReading.valuePath),
    )}
    target={scale.shown(targetOf(chamberReading) ?? 0)}
    unit={scale.unit}
    min={scale.shown(0)}
    max={scale.shown(chamberMax)}
    step={scale.step}
    presets={[
      { label: t('printerDetailControls.preset.off'), value: scale.shown(0) },
    ]}
    disabled={blocked(chamber, busy)}
    announceTarget={targetOf(chamberReading) !== undefined}
    onCommit={(next: number) =>
      send(VERBS.chamberTemp, 'chamber.target_temp', next)}
  />
{/snippet}
