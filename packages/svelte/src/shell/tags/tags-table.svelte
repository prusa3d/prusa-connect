<!-- This draws every tag as a row, with the printers carrying it. -->
<script lang="ts">
  import SortableTableHead from '../../primitives/table/sortable-table-head.svelte';
  import type { ListingState } from '../../primitives';

  import type { Snippet } from 'svelte';

  import { t } from '../../i18n';
  import {
    Table,
    TableBody,
    TableCaption,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    SORT_DIRECTION,
  } from '../../primitives';
  import TagPrinters from './tag-printers.svelte';
  import type { TagFieldId, TagRow } from './types';

  let {
    rows,
    listing,
    shows,
    action,
  }: {
    rows: readonly TagRow[];
    listing?: ListingState<TagRow> | undefined;
    shows: (field: TagFieldId) => boolean;
    /** The row's own action, written once by the screen and drawn in both views. */
    action?: Snippet<[TagRow]> | undefined;
  } = $props();
</script>

{#snippet sortHead(field: string, label: string)}
  {@const state = listing}
  {@const order = state?.sort?.field === field ? state.sort.order : undefined}
  {#if state}
    <SortableTableHead
      {label}
      direction={order === undefined ? undefined : SORT_DIRECTION[order]}
      onSort={(order: 'asc' | 'desc' | undefined) => {
        if (order !== undefined) state.setSort(field, order);
      }}
    />
  {:else}
    <TableHead>{label}</TableHead>
  {/if}
{/snippet}

<Table>
  <TableCaption class="sr-only">{t('tags.caption')}</TableCaption>
  <TableHeader>
    <TableRow>
      {@render sortHead('tag', t('tags.tag'))}
      {#if shows('printers')}{@render sortHead(
          'printers',
          t('tags.printers'),
        )}{/if}
      {#if action}<TableHead class="sr-only">{t('tags.actions')}</TableHead
        >{/if}
    </TableRow>
  </TableHeader>
  <TableBody>
    {#each rows as row (row.id)}
      <TableRow>
        <TableCell class="font-medium">{row.tag}</TableCell>
        {#if shows('printers')}
          <TableCell><TagPrinters {row} /></TableCell>
        {/if}
        {#if action}
          <TableCell class="text-end">{@render action(row)}</TableCell>
        {/if}
      </TableRow>
    {/each}
  </TableBody>
</Table>
