<!--
  The versions as a table. The version names the row and opens its detail;
  the menu at the end acts on it. Columns the container has no room for fold
  away, so the page never scrolls sideways.
-->
<script lang="ts">
  import { formatBytes, formatDate } from '@prusa/connect-ui/i18n';
  import {
    ABOVE_ROW_LINK,
    Badge,
    LINKED_ROW,
    ROW_LINK,
    Table,
    TableBody,
    TableCaption,
    TableCell,
    TableHead,
    TableHeader,
    SortableTableHead,
    TableRow,
  } from '@prusa/connect-ui/primitives';

  import type { Translate } from '../../api';
  import type { ReleaseActs } from '../../release-acts.svelte';
  import type {
    ReleaseRow,
    ReleaseSort,
    ReleaseSortColumn,
  } from '../../releases';
  import { DigestBadge } from '../digest-badge';
  import { ReleaseActions } from '../release-actions';
  import { ReleaseState } from '../release-state';

  let {
    rows,
    shows,
    sort,
    onSort,
    hrefOf,
    administrator,
    acts,
    t,
  }: {
    rows: readonly ReleaseRow[];
    shows: (field: string) => boolean;
    sort: ReleaseSort;
    onSort: (column: ReleaseSortColumn) => void;
    hrefOf: (row: ReleaseRow) => string;
    administrator: boolean;
    acts: ReleaseActs;
    t: Translate;
  } = $props();

  const day = (date: string) =>
    formatDate(date, { day: 'numeric', month: 'short', year: 'numeric' });

  const directionOf = (
    column: ReleaseSortColumn,
  ): 'ascending' | 'descending' | undefined => {
    if (sort.column !== column) return undefined;

    return sort.ascending ? 'ascending' : 'descending';
  };
</script>

{#snippet sortable(label: string, column: ReleaseSortColumn, cls?: string)}
  <SortableTableHead
    {label}
    direction={directionOf(column)}
    onSort={() => onSort(column)}
    class={cls}
  />
{/snippet}

<Table data-part="release-table">
  <TableCaption class="sr-only">{t('slicer.releases.caption')}</TableCaption>
  <TableHeader>
    <TableRow>
      {@render sortable(t('slicer.column.version'), 'version')}
      {#if shows('channel')}
        <TableHead class="hidden @xl/releases:table-cell"
          >{t('slicer.column.channel')}</TableHead
        >
      {/if}
      {#if shows('released')}
        {@render sortable(
          t('slicer.column.released'),
          'released',
          'hidden @3xl/releases:table-cell',
        )}
      {/if}
      {#if shows('state')}
        <TableHead>{t('slicer.column.state')}</TableHead>
      {/if}
      {#if shows('size')}
        <TableHead class="hidden text-end @4xl/releases:table-cell"
          >{t('slicer.column.size')}</TableHead
        >
      {/if}
      {#if shows('digest')}
        <TableHead class="hidden @2xl/releases:table-cell"
          >{t('slicer.column.digest')}</TableHead
        >
      {/if}
      <TableHead class="w-0"
        ><span class="sr-only">{t('slicer.column.actions')}</span></TableHead
      >
    </TableRow>
  </TableHeader>
  <TableBody>
    {#each rows as row (row.version)}
      <TableRow
        class={[
          LINKED_ROW,
          row.state === 'unavailable' && 'text-muted-foreground',
        ]}
        data-part="release-row"
        data-version={row.version}
      >
        <TableCell class="max-w-64">
          <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <a
              href={hrefOf(row)}
              class={['min-w-0 truncate rounded-sm font-medium', ROW_LINK]}
              >{row.version}</a
            >
            {#if row.isDefault}
              <Badge variant="secondary">{t('slicer.default')}</Badge>
            {/if}
          </div>
        </TableCell>
        {#if shows('channel')}
          <TableCell class="hidden @xl/releases:table-cell">
            <Badge variant="outline">{t(`slicer.channel.${row.channel}`)}</Badge
            >
          </TableCell>
        {/if}
        {#if shows('released')}
          <TableCell
            class="hidden whitespace-nowrap tabular-nums @3xl/releases:table-cell"
          >
            {row.released ? day(row.released) : ''}
          </TableCell>
        {/if}
        {#if shows('state')}
          <TableCell class="min-w-32">
            <ReleaseState {row} {t} />
          </TableCell>
        {/if}
        {#if shows('size')}
          <TableCell
            class="hidden text-end whitespace-nowrap tabular-nums @4xl/releases:table-cell"
          >
            {row.size ? formatBytes(row.size) : ''}
          </TableCell>
        {/if}
        {#if shows('digest')}
          <TableCell class="hidden @2xl/releases:table-cell">
            <DigestBadge digest={row.digest} {t} />
          </TableCell>
        {/if}
        <TableCell class={ABOVE_ROW_LINK}>
          <ReleaseActions
            {row}
            {administrator}
            {acts}
            detailHref={hrefOf(row)}
            {t}
          />
        </TableCell>
      </TableRow>
    {/each}
  </TableBody>
</Table>
