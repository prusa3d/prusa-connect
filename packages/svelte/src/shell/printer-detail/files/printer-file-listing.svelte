<script lang="ts">
  import type { Snippet } from 'svelte';

  import InboxIcon from '@lucide/svelte/icons/inbox';

  import type { UploadBlock } from '../../../domain';
  import { type FaultText, formatDuration, t } from '../../../i18n';
  import {
    Button,
    EmptyState,
    FaultAlert,
    Label,
    ListingNoMatch,
    type ListingState,
    ListingSkeleton,
    Pager,
    SelectAllCheckbox,
    Spinner,
    Table,
    TableBody,
    TableCaption,
    TableHead,
    SortableTableHead,
    TableHeader,
    TableRow,
  } from '../../../primitives';
  import UploadBlocked from '../upload-blocked.svelte';
  import PrinterFileRow from './printer-file-row.svelte';
  import PrinterFileTiles from './printer-file-tiles.svelte';
  import type { PrinterFileEntry, PrinterFileRowContext } from './types';

  const DIRECTION = { asc: 'ascending', desc: 'descending' } as const;

  let {
    listing,
    thumbnailSize = 0,
    row,
    actions,
    refusal,
    onRetry,
    loading,
    busy = false,
    readingSince,
    unread = false,
    searching,
    query,
    empty,
    atRoot,
    rootBlock = 'noStorage',
    checkingStorages = false,
    onRecheck,
    onClearSearch,
  }: {
    listing: ListingState<PrinterFileEntry>;
    thumbnailSize?: number;
    row: PrinterFileRowContext;
    actions: Snippet<[PrinterFileEntry]>;
    refusal?: FaultText | undefined;
    /** Asks the printer to read the folder again after a refusal. */
    onRetry?: (() => void) | undefined;
    loading: boolean;
    /** The folder is being read again, and its rows stay on screen. */
    busy?: boolean | undefined;
    /** When this page heard the printer began reading this folder, by the browser's clock. */
    readingSince?: string | undefined;
    /** The printer is reading this folder and nothing of it is held yet. */
    unread?: boolean | undefined;
    searching: boolean;
    query: string;
    empty: boolean;
    atRoot: boolean;
    /** Why the root lists no drive: none in the printer, or no answer yet. */
    rootBlock?: UploadBlock | undefined;
    checkingStorages?: boolean | undefined;
    onRecheck?: (() => void) | undefined;
    onClearSearch: () => void;
  } = $props();

  const reading = $derived(readingSince !== undefined);

  let now = $state(Date.now());

  // The read can run for minutes, so the reader is told it is still going.
  // Only the state with no rows shows how long.
  $effect(() => {
    if (readingSince === undefined || !unread || refusal || loading) return;

    now = Date.now();
    const tick = setInterval(() => {
      now = Date.now();
    }, 1000);

    return () => {
      clearInterval(tick);
    };
  });

  const elapsed = $derived(
    readingSince === undefined
      ? ''
      : formatDuration((now - Date.parse(readingSince)) / 1000),
  );
</script>

{#snippet column(field: 'name' | 'size' | 'modified', className?: string)}
  {@const sort = listing.sort}
  <SortableTableHead
    label={t(`printerDetailFiles.column.${field}`)}
    class={className}
    direction={sort?.field === field ? DIRECTION[sort.order] : undefined}
    onSort={(order: 'asc' | 'desc' | undefined) => {
      if (order === undefined) return;
      listing.setSort(field, order);
      listing.setPage(0);
    }}
  />
{/snippet}

<div
  class="@container/printer-file-listing flex flex-col gap-4"
  aria-busy={busy || reading}
>
  {#if refusal}
    {#if onRetry}
      <FaultAlert fault={refusal} actions={retry} />
    {:else}
      <FaultAlert fault={refusal} />
    {/if}
  {/if}
  {#if !loading && !unread && (!refusal || listing.total > 0)}
    {@render updating()}
  {/if}
  <!-- A fault with no rows held is the whole answer. A failed re-read keeps the
       rows the server still holds under it. -->
  {#if refusal && listing.total === 0}
    <!-- The alert above says it all. -->
  {:else if loading}
    <ListingSkeleton label={t('printerDetailFiles.loading')} art rows={6} />
  {:else if unread}
    <div class="flex flex-col gap-1 text-sm text-muted-foreground">
      <p>{t('printerDetailFiles.reading.note')}</p>
      {#if elapsed !== ''}
        <p class="tabular-nums">
          {t('printerDetailFiles.reading.elapsed', { seconds: elapsed })}
        </p>
      {/if}
    </div>
    <ListingSkeleton label={t('printerDetailFiles.loading')} art rows={6} />
  {:else if searching && listing.total === 0}
    <ListingNoMatch
      heading={t('printerDetailFiles.noMatch.heading')}
      headingLevel="h2"
      description={t('printerDetailFiles.noMatch.note', {
        query: query.trim(),
      })}
      clearLabel={t('primitives.listing.clearSearch')}
      onClear={onClearSearch}
    />
  {:else if empty && atRoot}
    <UploadBlocked block={rootBlock} checking={checkingStorages} {onRecheck} />
  {:else if empty}
    <EmptyState
      icon={InboxIcon}
      heading={t('printerDetailFiles.empty.heading')}
      headingLevel="h2"
      description={t('printerDetailFiles.empty.note')}
    />
  {:else if listing.view.value === 'cards'}
    <PrinterFileTiles
      rows={listing.page}
      name={(entry: PrinterFileEntry) => row.nameOf(entry)}
      thumbnail={(entry: PrinterFileEntry) => row.thumbnailOf(entry)}
      shows={(field: string) => row.shows(field)}
      selection={row.selection}
      onOpen={(path: string) => row.onOpenFolder(path)}
      {actions}
      {thumbnailSize}
    />
  {:else}
    <Table>
      <TableCaption class="sr-only">
        {t('printerDetailFiles.caption')}
      </TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead class="w-11">
            <!-- A label, so the whole cell ticks the box: the column's resize
                 handle sits a few pixels from the 16px box itself. -->
            <Label class="h-9 w-11 justify-center">
              <SelectAllCheckbox
                selection={row.selection}
                keys={listing.page.map((entry) => entry.path)}
                label={t('printerDetailFiles.select.page')}
              />
            </Label>
          </TableHead>
          {@render column('name', '@xl/printer-file-listing:w-1/2')}
          {#if row.shows('size')}{@render column(
              'size',
              '@max-lg/printer-file-listing:hidden @xl/printer-file-listing:w-1/10',
            )}{/if}
          <!-- Drawn whenever the pane has room, and never offered as a
               field: a toggle the pane overrules would do nothing. -->
          {@render column(
            'modified',
            'hidden @4xl/printer-file-listing:table-cell',
          )}
          <TableHead class="sr-only">
            {t('printerDetailFiles.column.actions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {#each listing.page as entry (entry.path)}
          <PrinterFileRow {entry} depth={0} {row} {thumbnailSize} />
        {/each}
      </TableBody>
    </Table>
  {/if}

  {#if listing.pager && listing.total > 0}
    <Pager {...listing.pager} total={listing.total} />
  {/if}
</div>

{#snippet retry()}
  <Button
    size="sm"
    variant="secondary"
    pending={busy || reading}
    onclick={onRetry}
  >
    {t('printerDetailFiles.retry')}
  </Button>
{/snippet}

{#snippet updating()}
  {#if reading}
    <p
      role="status"
      class="flex items-center gap-2 text-sm text-muted-foreground"
    >
      <Spinner tone="muted" />
      {t('printerDetailFiles.updating')}
    </p>
  {/if}
{/snippet}
