<!--
  The same facts as the table, stacked, for a container too narrow to hold
  eight columns without wrapping every cell.
-->
<script lang="ts">
  import { PRINTER_STATE_LABEL_KEY } from '../../domain';
  import type { Snippet } from 'svelte';
  import { formatUnit, t } from '../../i18n';
  import {
    Badge,
    DataRow,
    PrinterIcon,
    type DataRowValue,
  } from '../../primitives';
  import { jobFallbackOf, stateBadgeVariant } from './presentation';
  import type { PrinterFieldId, PrinterRow } from './types';

  let {
    rows,
    shows,
  }: {
    rows: readonly PrinterRow[];
    shows: (field: PrinterFieldId) => boolean;
  } = $props();

  const stateOf = (row: PrinterRow): string | undefined => {
    if (row.retired) return t('printers.retired');

    return shows('state') ? t(PRINTER_STATE_LABEL_KEY[row.state]) : undefined;
  };

  const secondaries = (row: PrinterRow, status: Snippet): DataRowValue[] => [
    stateOf(row) === undefined ? undefined : status,
    shows('job') ? (row.jobName ?? jobFallbackOf(row)) : undefined,
    shows('progress') && row.progress !== null
      ? formatUnit(row.progress, '%')
      : undefined,
    shows('material') ? row.material : undefined,
  ];
</script>

<div class="rounded-md border border-border">
  {#each rows as row (row.id)}
    {#snippet status()}
      <span class="inline-flex min-w-28">
        <Badge
          variant={row.retired ? 'secondary' : stateBadgeVariant(row.state)}
          class="font-mono uppercase"
        >
          {stateOf(row)}
        </Badge>
      </span>
    {/snippet}
    <DataRow
      href={row.detailHref}
      primary={row.name}
      secondaryValues={secondaries(row, status)}
      class="min-h-18"
    >
      {#snippet leading()}
        <PrinterIcon
          model={row.artwork}
          reported={row.model}
          class="size-14 shrink-0"
        />
      {/snippet}
    </DataRow>
  {/each}
</div>
