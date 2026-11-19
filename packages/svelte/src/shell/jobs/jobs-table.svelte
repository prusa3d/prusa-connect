<script lang="ts">
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';

  import { PRINTER_STATE_LABEL_KEY } from '../../domain';
  import { formatDateTime, formatDuration, formatUnit, t } from '../../i18n';
  import {
    ABOVE_ROW_LINK,
    Badge,
    ClippedName,
    FileThumbnail,
    LINKED_ROW,
    Progress,
    ROW_LINK,
    SortableTableHead,
    Table,
    TableBody,
    TableCaption,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    THUMBNAIL_ROW_MIN_HEIGHT,
    PrinterGlyphIcon,
  } from '../../primitives';
  import { stateBadgeVariant } from '../printers/presentation';
  import JobsRowActions from './jobs-row-actions.svelte';
  import { progressTone } from './presentation';
  import type { JobFieldId, JobRow, JobSort, JobSortColumn } from './types';

  let {
    rows,
    shows,
    sort,
    onSort,
    printAgainOf,
    thumbnailFor,
  }: {
    rows: readonly JobRow[];
    shows: (field: JobFieldId) => boolean;
    sort: JobSort;
    onSort: (column: JobSortColumn, order: 'asc' | 'desc') => void;
    /** The Print again press for one row, or undefined where the row offers none. */
    printAgainOf?: ((row: JobRow) => (() => void) | undefined) | undefined;
    thumbnailFor?: ((row: JobRow) => string | undefined) | undefined;
  } = $props();

  const directionOf = (
    column: JobSortColumn,
  ): 'ascending' | 'descending' | undefined => {
    if (sort.column !== column) return undefined;

    return sort.ascending ? 'ascending' : 'descending';
  };
</script>

{#snippet heading(label: string, column: JobSortColumn, className?: string)}
  <SortableTableHead
    {label}
    class={className}
    direction={directionOf(column)}
    onSort={(order: 'asc' | 'desc' | undefined) => {
      if (order !== undefined) onSort(column, order);
    }}
  />
{/snippet}

<div class="jobs-table @container/jobs-table">
  <Table>
    <TableCaption class="sr-only">{t('jobs.caption')}</TableCaption>
    <TableHeader>
      <TableRow>
        {#if shows('preview')}
          <TableHead class="sr-only @max-lg/jobs-table:hidden"
            >{t('jobs.preview')}</TableHead
          >
        {/if}
        {@render heading(t('jobs.file'), 'file')}
        {#if shows('printer')}{@render heading(
            t('jobs.printer'),
            'printer',
          )}{/if}
        {#if shows('state')}{@render heading(t('jobs.state'), 'state')}{/if}
        {#if shows('started')}{@render heading(
            t('jobs.started'),
            'started',
            '@max-lg/jobs-table:hidden',
          )}{/if}
        {#if shows('duration')}{@render heading(
            t('jobs.duration'),
            'duration',
            '@max-lg/jobs-table:hidden @xl/jobs-table:w-(--jobs-duration-column)',
          )}{/if}
        {#if shows('progress')}{@render heading(
            t('jobs.progress'),
            'progress',
            '@max-lg/jobs-table:hidden @xl/jobs-table:w-1/12',
          )}{/if}
        <!-- The menu acts on the row the File column names. -->
        <TableHead class="sr-only">{t('jobs.actions')}</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {#each rows as row (row.id)}
        <TableRow class={[THUMBNAIL_ROW_MIN_HEIGHT, LINKED_ROW]}>
          {#if shows('preview')}
            <TableCell roomy class="@max-lg/jobs-table:hidden">
              <FileThumbnail
                name={row.fileName}
                glyph={FileCode2Icon}
                thumbnailUrl={thumbnailFor?.(row)}
                size="print"
                class={[
                  ABOVE_ROW_LINK,
                  'pointer-events-none enabled:pointer-events-auto',
                ]}
              />
            </TableCell>
          {/if}
          <TableCell class="max-w-72">
            <a href={row.detailHref} class={['block rounded-sm', ROW_LINK]}>
              <ClippedName name={row.fileName} class="font-mono text-sm" />
            </a>
          </TableCell>
          {#if shows('printer')}
            <TableCell class="max-w-56 overflow-hidden">
              <!-- Lifted over the row's own overlay. -->
              <a
                href={row.printerHref}
                title={row.printerName}
                class={[
                  'flex max-w-full min-w-0 items-center gap-1.5 rounded-sm underline underline-offset-4 hover:text-link focus-visible:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none',
                  ABOVE_ROW_LINK,
                ]}
              >
                <PrinterGlyphIcon aria-hidden="true" class="size-3 shrink-0" />
                <span class="min-w-0 truncate">{row.printerName}</span>
              </a>
            </TableCell>
          {/if}
          {#if shows('state')}
            <TableCell>
              <Badge
                variant={stateBadgeVariant(row.state)}
                mono={false}
                class="font-mono"
              >
                {t(PRINTER_STATE_LABEL_KEY[row.state])}
              </Badge>
            </TableCell>
          {/if}
          {#if shows('started')}
            <TableCell
              class="whitespace-nowrap tabular-nums @max-lg/jobs-table:hidden"
            >
              {formatDateTime(row.started)}
            </TableCell>
          {/if}
          {#if shows('duration')}
            <TableCell
              class="text-start whitespace-nowrap tabular-nums @max-lg/jobs-table:hidden"
            >
              {row.durationSeconds === undefined
                ? ''
                : formatDuration(row.durationSeconds)}
            </TableCell>
          {/if}
          {#if shows('progress')}
            <TableCell class="w-28 @max-lg/jobs-table:hidden">
              <div class="flex items-center gap-2">
                <Progress
                  value={row.progress}
                  tone={progressTone(row)}
                  aria-label={t('jobs.progressFor', { name: row.fileName })}
                  class="flex-1"
                />
                <span class="shrink-0 tabular-nums">
                  {formatUnit(row.progress, '%')}
                </span>
              </div>
            </TableCell>
          {/if}
          <!-- Lifted over the row's own overlay. -->
          <TableCell class={ABOVE_ROW_LINK}>
            <JobsRowActions {row} {printAgainOf} />
          </TableCell>
        </TableRow>
      {/each}
    </TableBody>
  </Table>
</div>

<style>
  .jobs-table {
    --jobs-duration-column: 15%;
  }
</style>
