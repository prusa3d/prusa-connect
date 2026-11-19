<script lang="ts">
  import type { Snippet } from 'svelte';

  import {
    BED_PATH,
    BED_TARGET_PATH,
    CHAMBER_PATH,
    CHAMBER_TARGET_PATH,
    nozzlePath,
    nozzleTargetPath,
    PREHEAT_PRESETS,
    type PreheatPreset,
    type Device,
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
  }: SectionProps & {
    material: string | undefined;
    device?: Device | undefined;
    telemetry?: TelemetrySource | undefined;
    /**
     * Draws the material of a tool the reader may assign, handed its id and
     * the chip to fall back on: the Tools table's cell, so both assign alike.
     */
    assign?: Snippet<[string, Snippet]> | undefined;
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
  // A printer with no chamber has neither the verb nor the reading.
  const hasChamber = $derived(
    chamber.kind !== 'absent' || reading(CHAMBER_PATH) !== undefined,
  );
  const reason = $derived(groupReason([nozzle, bed, chamber]));

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
    <HeaterRow
      class={READING_TILE}
      label={t('printerDetailControls.bed.label')}
      kind="bed"
      nozzleTemp={reading(BED_PATH)}
      targetTemp={reading(BED_TARGET_PATH)}
      {...bed.kind === 'absent' ? {} : { action: bedEditor }}
    />
    {#if hasChamber}
      <HeaterRow
        class={READING_TILE}
        label={t('printerDetailControls.chamberTemp.label')}
        kind="chamber"
        nozzleTemp={reading(CHAMBER_PATH)}
        targetTemp={reading(CHAMBER_TARGET_PATH)}
        {...chamber.kind === 'absent' ? {} : { action: chamberEditor }}
      />
    {/if}
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
    currentLabel={celsius(reading(BED_PATH))}
    target={scale.shown(reading(BED_TARGET_PATH) ?? 0)}
    unit={scale.unit}
    min={scale.shown(0)}
    max={scale.shown(BED_MAX_CELSIUS)}
    step={scale.step}
    presets={bedPresets}
    disabled={blocked(bed, busy)}
    announceTarget={reading(BED_TARGET_PATH) !== undefined}
    onCommit={(next: number) => send(VERBS.bed, 'bed_temperature', next)}
  />
{/snippet}

{#snippet chamberEditor()}
  <ValueEditor
    label={t('printerDetailControls.chamberTemp.label')}
    currentLabel={celsius(reading(CHAMBER_PATH))}
    target={scale.shown(reading(CHAMBER_TARGET_PATH) ?? 0)}
    unit={scale.unit}
    min={scale.shown(0)}
    max={scale.shown(chamberMax)}
    step={scale.step}
    presets={[
      { label: t('printerDetailControls.preset.off'), value: scale.shown(0) },
    ]}
    disabled={blocked(chamber, busy)}
    announceTarget={reading(CHAMBER_TARGET_PATH) !== undefined}
    onCommit={(next: number) =>
      send(VERBS.chamberTemp, 'chamber.target_temp', next)}
  />
{/snippet}
