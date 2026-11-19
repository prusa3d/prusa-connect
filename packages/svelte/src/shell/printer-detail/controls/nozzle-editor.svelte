<!--
  The one editor of a head's temperature: the nozzle chip of the header, a head
  row of Controls and a row of the Tools table all draw it. A printer that
  refuses the value still opens the editor, which says why; only a command on
  its way disables it.
-->
<script lang="ts">
  import {
    PREHEAT_PRESETS,
    toolArgument,
    type ToolReading,
  } from '../../../domain';
  import { formatUnit, t } from '../../../i18n';
  import {
    ValueEditor,
    type ValueChoice,
    type ValueEditorProps,
    type ValuePreset,
  } from '../../../primitives';
  import { toolLabel } from '../../printers/tool-label';
  import type { CommandPort } from '../types';
  import {
    degreeScale,
    NOZZLE_MAX_CELSIUS,
    refusalText,
    VERBS,
    type Resolve,
  } from './machine-controls';

  let {
    resolve,
    busy,
    onCommand,
    perTool,
    temperature,
    target,
    tool,
    heads,
    choice,
    trigger,
  }: {
    resolve: Resolve;
    busy: boolean;
    onCommand: CommandPort['onCommand'];
    /** Whether the nozzle verb takes a tool. */
    perTool: boolean;
    temperature: number | undefined;
    target: number | undefined;
    /** The head this editor is for. Absent, it sets the head printing now. */
    tool?: number | undefined;
    /** With more than one and a verb that takes a tool, the editor lists them first. */
    heads?: readonly ToolReading[] | undefined;
    choice?: ValueEditorProps['choice'];
    trigger?: ValueEditorProps['trigger'];
  } = $props();

  const projection = $derived(resolve(VERBS.nozzle));

  const celsius = (value: number | undefined): string =>
    value === undefined
      ? t('printerDetailControls.noReading')
      : formatUnit(value, '°C', { maximumFractionDigits: 0 });

  // The editor reads in the unit the reader chose.
  const scale = $derived(degreeScale());

  const presets = $derived<ValuePreset[]>([
    { label: t('printerDetailControls.preset.off'), value: scale.shown(0) },
    ...PREHEAT_PRESETS.map((preset) => ({
      label: preset.material,
      value: scale.shown(preset.nozzleCelsius),
    })),
  ]);

  const choices = $derived<ValueChoice[] | undefined>(
    heads !== undefined && heads.length > 1 && perTool
      ? heads.map((head) => ({
          key: toolArgument(head.index),
          subject: toolLabel(head.index),
          currentLabel: celsius(head.temperature),
          target: scale.shown(head.target ?? 0),
          targetLabel: t('printerDetail.chip.toolTarget', {
            tool: toolLabel(head.index),
            unit: scale.unit,
          }),
        }))
      : undefined,
  );

  const send = (next: number, picked?: ValueChoice): void => {
    const which =
      picked?.key ??
      (tool !== undefined && perTool ? toolArgument(tool) : undefined);
    void onCommand(VERBS.nozzle, {
      nozzle_temperature: String(Math.round(scale.celsius(next))),
      ...(which === undefined ? {} : { tool: which }),
    });
  };
</script>

{#if projection.kind !== 'absent'}
  <ValueEditor
    label={t('printerDetailControls.nozzle.label')}
    subject={tool === undefined ? undefined : toolLabel(tool)}
    currentLabel={celsius(temperature)}
    target={scale.shown(target ?? 0)}
    unit={scale.unit}
    min={scale.shown(0)}
    max={scale.shown(NOZZLE_MAX_CELSIUS)}
    step={scale.step}
    {presets}
    disabled={busy}
    announceTarget={target !== undefined}
    {choices}
    {choice}
    {trigger}
    reason={projection.kind === 'disabled'
      ? refusalText(projection.reason)
      : undefined}
    onCommit={send}
  />
{/if}
