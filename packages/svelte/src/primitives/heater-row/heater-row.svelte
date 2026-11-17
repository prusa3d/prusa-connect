<!--
  One row per physical heater: a tool head, the bed, or the chamber. An MMU-fed
  head adds `filamentPositions` and stays one row. The line reads icon, label,
  material, then the reading and its editor trigger on the trailing edge, the
  same shape and weight as the motion rows beside it.

  Nothing stacks. The material line truncates as a whole rather than spilling
  into the reading, while the icon, the label, the reading, the arrow and the
  editor trigger stay `shrink-0`: those are the things a person reads or
  presses, so they are the last to give up width. No spool glyph precedes the
  material, because the row already opens with an icon naming the head, and a
  second icon ahead of a word that is unambiguously a material name costs 14px
  of forced width plus a 4px gap that `truncate` can never claw back.
-->
<script lang="ts">
  import type { Component } from 'svelte';

  import { chamberState, type HeaterState, heaterState } from '../../domain';
  import { formatUnit, t } from '../../i18n';
  import { ChamberIcon } from '../control-icon';
  import { HeatReading } from '../heating-display';
  import { HeatbedIcon, NozzleIcon } from '../telemetry-icon';
  import type { HeaterKind, HeaterRowProps } from './types';

  let {
    label,
    kind,
    nozzleTemp,
    targetTemp,
    material,
    active,
    filamentPositions,
    action,
    class: className,
    ...rest
  }: HeaterRowProps = $props();

  const STATE_KEY: Record<HeaterState, string> = {
    heating: 'primitives.heaterRow.heating',
    cooling: 'primitives.heaterRow.cooling',
    'at-temperature': 'primitives.heaterRow.atTemperature',
    off: 'primitives.heaterRow.off',
    'not-reported': 'primitives.heaterRow.notReported',
  };

  // The chamber draws the same icon as its chip in the printer's header.
  const KIND_ICON: Record<HeaterKind, Component<Record<string, unknown>>> = {
    tool: NozzleIcon,
    bed: HeatbedIcon,
    chamber: ChamberIcon,
  };

  // The reading and the target are written whole, with the unit where the
  // language on screen puts it: `235°C` in English, `235 °C` in Czech.
  const degrees = (value: number): string =>
    formatUnit(value, '°C', { maximumFractionDigits: 0 });

  const state = $derived(
    (kind === 'chamber' ? chamberState : heaterState)({
      current: nozzleTemp,
      target: targetTemp,
    }),
  );
  const Icon = $derived(KIND_ICON[kind]);

  const loaded = $derived(
    filamentPositions?.find((position) => position.loaded),
  );

  // The loaded position rides on the material line's own truncating span. A
  // strip of five badges needs a row to wrap into, which is the vertical room
  // a one-line row does not have; a suffix costs only width, and `truncate`
  // already absorbs that.
  const materialLine = $derived.by(() => {
    if (material === undefined) return undefined;
    if (loaded === undefined || filamentPositions === undefined)
      return material;

    return t('primitives.heaterRow.loaded', {
      material,
      position: loaded.position,
      total: filamentPositions.length,
    });
  });

  // With no `action` nothing else confirms the target, so a preheat watched
  // from here would show only where the reading is and never where it is
  // headed. `off` and `not-reported` are left out: a switched-off heater's
  // target is noise, and there is no reading to pair it with.
  const targetDegrees = $derived(
    action === undefined &&
      targetTemp !== undefined &&
      state !== 'off' &&
      state !== 'not-reported'
      ? degrees(targetTemp)
      : undefined,
  );

  const stateWord = $derived(t(STATE_KEY[state]));
  const spoken = $derived(
    targetDegrees === undefined
      ? stateWord
      : t('primitives.heaterRow.stateWithTarget', {
          state: stateWord,
          temperature: targetDegrees,
        }),
  );
</script>

<div
  {...rest}
  data-slot="heater-row"
  data-active={active ? true : undefined}
  aria-current={active ? true : undefined}
  title={targetDegrees === undefined
    ? undefined
    : t('primitives.heaterRow.target', { temperature: targetDegrees })}
  class={['flex min-w-0 items-center gap-2 py-1', className]}
>
  <div class="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
    <Icon aria-hidden="true" class="size-4 shrink-0 text-muted-foreground" />
    <span
      class={[
        'shrink-0 text-xs',
        active
          ? 'font-semibold text-foreground'
          : 'font-medium text-muted-foreground',
      ]}
    >
      {label}
    </span>
    {#if materialLine !== undefined}
      <span class="min-w-0 truncate font-mono text-xs text-muted-foreground">
        {materialLine}
      </span>
    {/if}
  </div>
  <div class="flex shrink-0 items-center gap-1.5">
    <!-- The five-word vocabulary survives here. `off` is the one state that
         would otherwise read the same as at-temperature, both untinted, so it
         is drawn; the reading's tint and arrow already carry the rest. -->
    {#if state === 'off'}
      <span class="text-xs text-muted-foreground">{stateWord}</span>
    {:else}
      <span class="sr-only">{spoken}</span>
    {/if}
    <HeatReading
      {state}
      class="font-mono text-sm whitespace-nowrap tabular-nums"
    >
      {#if nozzleTemp === undefined}&mdash;{:else}{degrees(nozzleTemp)}{/if}
    </HeatReading>
    {#if action}{@render action()}{/if}
  </div>
</div>
