<!-- The readings beside the printer's name. -->
<script lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import type { Snippet } from 'svelte';

  import {
    BED_PATH,
    BED_TARGET_PATH,
    CHAMBER_PATH,
    CHAMBER_TARGET_PATH,
    FLOW_PATH,
    currentHeadOf,
    nozzlePath,
    nozzleTargetPath,
    numberAt,
    PREHEAT_PRESETS,
    activeMaterialOf,
    toolsOf,
    toolArgument,
    SPEED_PATH,
    type Device,
    type PreheatPreset,
  } from '../../domain';
  import { formatUnit, t } from '../../i18n';
  import {
    Badge,
    Button,
    ChamberIcon,
    HeatbedIcon,
    NozzleIcon,
    PopoverContent,
    PopoverDescription,
    PopoverHeader,
    PopoverTitle,
    PopoverTrigger,
    Popover,
    SpeedIcon,
    SpoolIcon,
    TemperatureReading,
    ValueEditor,
    type ValueChoice,
    type ValuePreset,
  } from '../../primitives';
  import {
    BED_MAX_CELSIUS,
    blocked,
    chamberMaxCelsius,
    declaresArgument,
    degreeScale,
    groupReason,
    NOZZLE_MAX_CELSIUS,
    refusalText,
    resolverFor,
    VERBS,
  } from './controls/machine-controls';
  import type { CommandPort, TelemetrySource } from './types';

  let {
    telemetry,
    device,
    commands,
  }: {
    telemetry: TelemetrySource;
    device: Device;
    /** Absent, every chip is a reading and nothing here can be set. */
    commands?: CommandPort | undefined;
  } = $props();

  // What a chip adds to `Badge`'s outline.

  const number = (path: string): number | undefined => {
    const live = telemetry.value(path);

    return typeof live === 'number' && Number.isFinite(live)
      ? live
      : numberAt(device, path);
  };

  // The material printing now. Each head's own is in its row of the editor.
  const material = $derived(activeMaterialOf(telemetry, device));
  // The head the nozzle chip reads: the one printing now, else head 0.
  const currentHead = $derived(currentHeadOf(telemetry, device));
  // A printer with more than one head in its tool tree gets a row per head
  // and the active head named beside the nozzle (ADR-20261023-gna rule 12).
  const heads = $derived(toolsOf(telemetry, device));
  const tools = $derived(heads.length > 1 ? heads : []);
  const activeTool = $derived(tools.find((tool) => tool.active)?.index);
  const speed = $derived(number(SPEED_PATH));
  const flow = $derived(number(FLOW_PATH));

  // The same projection the Controls tab puts every one of its controls.
  const resolve = $derived(
    commands === undefined || !device.connected
      ? () => ({ kind: 'absent' as const })
      : resolverFor(device, commands),
  );
  const busy = $derived(commands?.sending === true);

  const load = $derived(resolve(VERBS.loadFilament));
  const unload = $derived(resolve(VERBS.unloadFilament));
  const filamentReason = $derived(groupReason([load, unload]));
  // A load verb that declares no filament argument loads whatever the printer
  // asks for, so it gets one plain button and no type picker.
  const typed = $derived(
    declaresArgument(device, VERBS.loadFilament, 'filament'),
  );
  let filamentOpen = $state(false);

  const sendFilament = (verb: string, args?: Record<string, string>): void => {
    filamentOpen = false;
    commands?.onCommand(verb, args);
  };

  const celsius = (value: number | undefined): string =>
    value === undefined
      ? t('printerDetailControls.noReading')
      : formatUnit(value, '°C', { maximumFractionDigits: 0 });

  // The chips read in the unit the reader chose, so the editors do too.
  const scale = $derived(degreeScale());

  // A heater with no material column, the chamber, offers Off alone.
  const presetsFor = (
    column?: (preset: PreheatPreset) => number,
  ): ValuePreset[] => [
    { label: t('printerDetailControls.preset.off'), value: scale.shown(0) },
    ...(column === undefined
      ? []
      : PREHEAT_PRESETS.map((preset) => ({
          label: preset.material,
          value: scale.shown(column(preset)),
        }))),
  ];

  /** One heater, as this row draws it and as its editor sends it. */
  interface HeaterChip {
    /** The chip's own word, for a reader who cannot see the glyph. */
    name: string;
    label: string;
    current: number | undefined;
    target: number | undefined;
    /** Whether the printer offers the verb at all. */
    settable: boolean;
    /** Why it will not take the value right now, in the language on screen. */
    reason: string | undefined;
    presets: ValuePreset[];
    max: number;
    verb: string;
    /** What the dialect calls the setpoint on the wire. */
    argument: string;
    /** A chamber, which is off with no target whatever the air. */
    chamber: boolean;
  }

  const heater = (spec: {
    name: string;
    label: string;
    path: string;
    targetPath: string;
    verb: string;
    argument: string;
    max: number;
    column?: (preset: PreheatPreset) => number;
    chamber?: boolean;
  }): HeaterChip => {
    const projection = resolve(spec.verb);

    return {
      name: spec.name,
      label: spec.label,
      current: number(spec.path),
      target: number(spec.targetPath),
      settable: projection.kind !== 'absent',
      reason:
        projection.kind === 'disabled'
          ? refusalText(projection.reason)
          : undefined,
      presets: presetsFor(spec.column),
      max: spec.max,
      verb: spec.verb,
      argument: spec.argument,
      chamber: spec.chamber === true,
    };
  };

  const nozzle = $derived(
    heater({
      name: t('printerDetail.chip.nozzle'),
      label: t('printerDetailControls.nozzle.label'),
      path: nozzlePath(currentHead),
      targetPath: nozzleTargetPath(currentHead),
      verb: VERBS.nozzle,
      argument: 'nozzle_temperature',
      max: NOZZLE_MAX_CELSIUS,
      column: (preset) => preset.nozzleCelsius,
    }),
  );

  const bed = $derived(
    heater({
      name: t('printerDetail.chip.bed'),
      label: t('printerDetailControls.bed.label'),
      path: BED_PATH,
      targetPath: BED_TARGET_PATH,
      verb: VERBS.bed,
      argument: 'bed_temperature',
      max: BED_MAX_CELSIUS,
      column: (preset) => preset.bedCelsius,
    }),
  );

  // The air inside an enclosed printer. A CORE One reports it and takes a
  // target; an XL with its enclosure on reports it and takes none.
  const chamber = $derived(
    heater({
      name: t('printerDetail.chip.chamber'),
      label: t('printerDetailControls.chamberTemp.label'),
      path: CHAMBER_PATH,
      targetPath: CHAMBER_TARGET_PATH,
      verb: VERBS.chamberTemp,
      argument: 'chamber.target_temp',
      max: chamberMaxCelsius(device.model),
      chamber: true,
    }),
  );
  const hasChamber = $derived(
    chamber.settable || chamber.current !== undefined,
  );

  // A tool changer whose verb takes no tool can only heat the active head,
  // which is the plain editor.
  const toolChoices = $derived<ValueChoice[] | undefined>(
    tools.length > 0 && declaresArgument(device, VERBS.nozzle, 'tool')
      ? tools.map((tool) => ({
          key: toolArgument(tool.index),
          subject: t('printerDetailControls.tools.label', { n: tool.index }),
          currentLabel: celsius(tool.temperature),
          target: scale.shown(tool.target ?? 0),
          targetLabel: t('printerDetail.chip.toolTarget', {
            n: tool.index,
            unit: scale.unit,
          }),
        }))
      : undefined,
  );

  const send = (chip: HeaterChip, next: number, choice?: ValueChoice): void => {
    commands?.onCommand(chip.verb, {
      [chip.argument]: String(Math.round(scale.celsius(next))),
      ...(choice === undefined ? {} : { tool: choice.key }),
    });
  };
</script>

{#snippet materialText()}
  {#if material === undefined}
    <span class="text-muted-foreground">&mdash;</span>
  {:else}
    {material}
  {/if}
{/snippet}

{#snippet nozzleIcon()}
  <NozzleIcon aria-hidden="true" class="size-3.5" />
{/snippet}

{#snippet bedIcon()}
  <HeatbedIcon aria-hidden="true" class="size-3.5" />
{/snippet}

{#snippet chamberIcon()}
  <ChamberIcon aria-hidden="true" class="size-3.5" />
{/snippet}

{#snippet toolRow(choice: ValueChoice)}
  {@const tool = tools.find(
    (entry) => toolArgument(entry.index) === choice.key,
  )}
  <span
    class={[
      'flex w-full items-center gap-2',
      tool?.active !== true && 'text-muted-foreground',
    ]}
  >
    <NozzleIcon aria-hidden="true" class="size-3.5 text-primary" />
    <span class={tool?.active === true ? 'font-semibold text-foreground' : ''}>
      {choice.subject}
    </span>
    <span>{tool?.material ?? ''}</span>
    {#if tool?.active === true}
      <Badge variant="secondary" size="sm">
        {t('printerDetail.chip.toolActive')}
      </Badge>
    {/if}
    <span class="ms-auto tabular-nums">{choice.currentLabel}</span>
  </span>
{/snippet}

{#snippet heaterChip(chip: HeaterChip, icon: Snippet, choices?: ValueChoice[])}
  <div>
    <dt class="sr-only">{chip.name}</dt>
    <dd>
      {#if chip.settable}
        <ValueEditor
          label={chip.label}
          currentLabel={celsius(chip.current)}
          target={scale.shown(chip.target ?? 0)}
          unit={scale.unit}
          min={scale.shown(0)}
          max={scale.shown(chip.max)}
          step={scale.step}
          presets={chip.presets}
          disabled={busy}
          announceTarget={chip.target !== undefined}
          {choices}
          choice={toolRow}
          {...chip.reason === undefined ? {} : { reason: chip.reason }}
          onCommit={(next: number, choice?: ValueChoice) => {
            send(chip, next, choice);
          }}
        >
          {#snippet trigger({ props }: { props: Record<string, unknown> })}
            <Badge variant="outline" size="md" numeric>
              {#snippet child({ props: badge })}
                <!-- The badge's attributes come last. The editor's name is
                     read out ahead of the reading rather than replacing it,
                     since WCAG 2.5.3 wants the shown text inside the name. -->
                <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- a Badge child is the documented way to make a pressable badge -->
                <button
                  {...props}
                  {...badge}
                  aria-label={undefined}
                  type="button"
                >
                  <span class="sr-only">{props['aria-label']},</span>
                  <TemperatureReading
                    {icon}
                    current={chip.current}
                    target={chip.target}
                    chamber={chip.chamber}
                  />
                  <ChevronDownIcon aria-hidden="true" />
                </button>
              {/snippet}
            </Badge>
          {/snippet}
        </ValueEditor>
      {:else}
        <Badge variant="outline" size="md" numeric>
          <TemperatureReading
            {icon}
            current={chip.current}
            target={chip.target}
            chamber={chip.chamber}
          />
        </Badge>
      {/if}
    </dd>
  </div>
{/snippet}

<dl class="flex flex-wrap items-center gap-2">
  {@render heaterChip(nozzle, nozzleIcon, toolChoices)}

  {#if activeTool !== undefined}
    <div>
      <dt class="sr-only">{t('printerDetail.chip.tool')}</dt>
      <dd>
        <Badge variant="outline" size="md" numeric>
          {t('printerDetail.chip.toolNumber', { n: activeTool })}
        </Badge>
      </dd>
    </div>
  {/if}
  {@render heaterChip(bed, bedIcon)}
  {#if hasChamber}
    {@render heaterChip(chamber, chamberIcon)}
  {/if}

  <div>
    <dt class="sr-only">{t('printerDetail.chip.material')}</dt>
    <dd>
      {#if load.kind === 'absent' && unload.kind === 'absent'}
        <Badge variant="outline" size="md" numeric>
          <SpoolIcon role="img" aria-label={t('printerDetail.chip.material')} />
          {@render materialText()}
        </Badge>
      {:else}
        <Popover bind:open={filamentOpen}>
          <PopoverTrigger>
            {#snippet child({ props: trigger })}
              <Badge variant="outline" size="md" numeric>
                {#snippet child({ props: badge })}
                  <!-- The badge's attributes come last. -->
                  <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- a Badge child is the documented way to make a pressable badge -->
                  <button {...trigger} {...badge} type="button">
                    <SpoolIcon
                      role="img"
                      aria-label={t('printerDetail.chip.material')}
                    />
                    {@render materialText()}
                    <ChevronDownIcon aria-hidden="true" />
                  </button>
                {/snippet}
              </Badge>
            {/snippet}
          </PopoverTrigger>
          <PopoverContent class="flex flex-col gap-3">
            <PopoverHeader>
              <PopoverTitle>{t('printerDetail.filament.heading')}</PopoverTitle>
              {#if filamentReason !== undefined}
                <PopoverDescription>{filamentReason}</PopoverDescription>
              {/if}
            </PopoverHeader>
            <!-- A filament type, never a material from a catalogue. -->
            {#if load.kind !== 'absent' && !typed}
              <Button
                variant="secondary"
                size="sm"
                disabled={blocked(load, busy)}
                onclick={() => {
                  sendFilament(VERBS.loadFilament);
                }}
              >
                {t('printerDetail.filament.load')}
              </Button>
            {:else if load.kind !== 'absent'}
              <div class="grid grid-cols-3 gap-2">
                {#each PREHEAT_PRESETS as preset (preset.material)}
                  <Button
                    variant="secondary"
                    size="sm"
                    disabled={blocked(load, busy)}
                    onclick={() => {
                      sendFilament(VERBS.loadFilament, {
                        filament: preset.material,
                      });
                    }}
                  >
                    {preset.material}
                  </Button>
                {/each}
              </div>
            {/if}
            {#if unload.kind !== 'absent'}
              <Button
                variant="ghost"
                size="sm"
                disabled={blocked(unload, busy)}
                onclick={() => {
                  sendFilament(VERBS.unloadFilament);
                }}
              >
                {t('printerDetail.filament.unload')}
              </Button>
            {/if}
          </PopoverContent>
        </Popover>
      {/if}
    </dd>
  </div>

  {#if speed !== undefined}
    <div>
      <dt class="sr-only">{t('printerDetail.chip.speed')}</dt>
      <dd>
        <Badge variant="outline" size="md" numeric>
          <SpeedIcon aria-hidden="true" />
          {formatUnit(speed, '%')}
        </Badge>
      </dd>
    </div>
  {/if}

  {#if flow !== undefined}
    <div>
      <dt class="sr-only">{t('printerDetail.chip.flow')}</dt>
      <dd>
        <Badge variant="outline" size="md" numeric>
          <NozzleIcon aria-hidden="true" />
          {formatUnit(flow, '%')}
        </Badge>
      </dd>
    </div>
  {/if}
</dl>
