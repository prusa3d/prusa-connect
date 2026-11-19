<script lang="ts">
  import {
    materialOf,
    reportedToolIds,
    toolPath,
    type Device,
  } from '../../domain';
  import { t } from '../../i18n';
  import { slotLabel, toolLabel } from '../printers/tool-label';
  import FilamentPosition from './filament-position.svelte';
  import { compareIds, toolRowsOf } from './tool-rows';
  import type { TelemetrySource } from './types';

  let {
    device,
    telemetry,
    only,
  }: {
    device: Device;
    telemetry: TelemetrySource;
    /** The one tool to draw, in a row that already names it. */
    only?: string | undefined;
  } = $props();

  const positions = $derived.by(() => {
    const reported = reportedToolIds(device.state, telemetry.paths);
    const parents = new Set(
      reported
        .filter((id) => id.includes('.'))
        .map((id) => id.slice(0, id.lastIndexOf('.'))),
    );
    const hasTree = device.tools !== undefined;
    const rows = hasTree
      ? toolRowsOf(device.tools)
      : (reported.length ? reported : ['0']).sort(compareIds).map((id) => ({
          id,
          slot: id.includes('.'),
          tool: undefined,
        }));
    const enabled = (id: string): boolean =>
      (telemetry.value(toolPath(id, 'enabled')) ??
        device.state[toolPath(id, 'enabled')]) !== false &&
      (!id.includes('.') || enabled(id.slice(0, id.lastIndexOf('.'))));
    return rows.filter(
      ({ id, tool }) =>
        (only === undefined || id === only) &&
        tool !== null &&
        !tool?.feeder?.slot_count &&
        (hasTree || !parents.has(id)) &&
        enabled(id),
    );
  });

  const readingOf = (row: (typeof positions)[number]) => {
    const reading = materialOf(telemetry, device, row.id);
    if (reading !== undefined)
      return { reported: reading || undefined, empty: reading === '' };
    const loaded = row.tool?.material_package_instance;
    return {
      reported: loaded?.package.material.abbreviation,
      empty: loaded === null,
    };
  };
</script>

<div
  class={[
    'flex flex-wrap items-center gap-x-2 gap-y-1.5',
    only === undefined && 'mt-1',
  ]}
>
  {#each positions as row (row.id)}
    {@const position = row.slot
      ? slotLabel(row.id.slice(row.id.lastIndexOf('.') + 1))
      : toolLabel(row.id)}
    {@const reading = readingOf(row)}
    <FilamentPosition
      deviceId={device.id}
      toolId={row.id}
      {position}
      bare={only !== undefined}
      {...reading}
    />
  {:else}
    {#if only === undefined}
      <span class="text-sm text-muted-foreground"
        >{t('printerDetailInfo.tools.none')}</span
      >
    {/if}
  {/each}
</div>
