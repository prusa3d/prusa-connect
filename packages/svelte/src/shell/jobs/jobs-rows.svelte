<!-- The same facts as the table. -->
<script lang="ts">
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';

  import { PRINTER_STATE_LABEL_KEY } from '../../domain';
  import { formatDateTime, formatDuration, formatUnit, t } from '../../i18n';
  import {
    ABOVE_ROW_LINK,
    Badge,
    DataRow,
    FileThumbnail,
    PrinterGlyphIcon,
    RowList,
  } from '../../primitives';
  import { stateBadgeVariant } from '../printers/presentation';
  import JobsRowActions from './jobs-row-actions.svelte';
  import type { JobFieldId, JobRow } from './types';

  let {
    rows,
    shows,
    printAgainOf,
    thumbnailFor,
  }: {
    rows: readonly JobRow[];
    shows: (field: JobFieldId) => boolean;
    /** The Print again press for one row, or undefined where the row offers none. */
    printAgainOf?: ((row: JobRow) => (() => void) | undefined) | undefined;
    thumbnailFor?: ((row: JobRow) => string | undefined) | undefined;
  } = $props();

  const span = (row: JobRow): string | undefined =>
    row.durationSeconds === undefined
      ? undefined
      : formatDuration(row.durationSeconds);
</script>

<RowList radius="md">
  {#each rows as row (row.id)}
    {#snippet state()}
      <Badge
        variant={stateBadgeVariant(row.state)}
        mono={false}
        class="font-mono"
      >
        {t(PRINTER_STATE_LABEL_KEY[row.state])}
      </Badge>
    {/snippet}
    {#snippet printer()}
      <a
        href={row.printerHref}
        title={row.printerName}
        class={[
          'inline-flex min-w-0 items-center gap-1.5 rounded-sm underline underline-offset-4 hover:text-link focus-visible:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none',
          ABOVE_ROW_LINK,
        ]}
      >
        <PrinterGlyphIcon aria-hidden="true" class="size-3 shrink-0" />
        <span class="min-w-0 truncate">{row.printerName}</span>
      </a>
    {/snippet}
    {#snippet preview()}
      <FileThumbnail
        name={row.fileName}
        glyph={FileCode2Icon}
        thumbnailUrl={thumbnailFor?.(row)}
        size="print"
      />
    {/snippet}
    <!-- A row shows the same fields as the table's columns. -->
    <DataRow
      href={row.detailHref}
      primary={row.fileName}
      mono
      wrapValues
      secondaryValues={[
        shows('state') ? state : undefined,
        shows('printer') ? printer : undefined,
        shows('started') ? formatDateTime(row.started) : undefined,
        shows('duration') ? span(row) : undefined,
        shows('progress') ? formatUnit(row.progress, '%') : undefined,
      ]}
      {...shows('preview') ? { leading: preview } : {}}
      roomy={shows('preview')}
      class="min-h-18"
    >
      {#snippet actions()}
        <JobsRowActions {row} {printAgainOf} />
      {/snippet}
    </DataRow>
  {/each}
</RowList>
