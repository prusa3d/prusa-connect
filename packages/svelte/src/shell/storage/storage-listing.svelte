<!-- The files in the open folder. -->
<script lang="ts">
  import PlayIcon from '@lucide/svelte/icons/play';
  import type { Snippet } from 'svelte';

  import InboxIcon from '@lucide/svelte/icons/inbox';
  import PencilIcon from '@lucide/svelte/icons/pencil';
  import Share2Icon from '@lucide/svelte/icons/share-2';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';

  import { isPrintFile } from '../../domain';
  import { type FaultText, formatBytes, formatDateTime, t } from '../../i18n';
  import {
    ABOVE_ROW_LINK,
    Button,
    ClippedName,
    DropdownMenuCheckboxItem,
    DropdownMenuItem,
    EmptyState,
    FaultAlert,
    FileThumbnail,
    Label,
    LINKED_ROW,
    ListingNoMatch,
    type ListingState,
    ListingSkeleton,
    Pager,
    ROW_LINK,
    RowActions,
    SelectAllCheckbox,
    SelectCheckbox,
    type Selection,
    Table,
    TableBody,
    TableCaption,
    TableCell,
    TableHead,
    SortableTableHead,
    TableHeader,
    TableRow,
    THUMBNAIL_ROW_MIN_HEIGHT,
    Tile,
    Switch,
    SORT_DIRECTION,
  } from '../../primitives';
  import fileIcon from './assets/file.svg';
  import folderIcon from './assets/folder.svg';
  import type {
    FolderSharing,
    StorageFieldId,
    StorageRow,
    StorageRowFold,
  } from './types';

  const iconSrcFor = (entry: StorageRow): string =>
    entry.folder ? folderIcon : fileIcon;

  const FILE_WIDTHS = ['w-36', 'w-44', 'w-52', 'w-60'];

  let {
    listing,
    thumbnailSize = 0,
    filesFault,
    loading,
    emptyFolder,
    searching,
    query,
    writable,
    printable,
    selection,
    shows,
    thumbnailOf,
    fileHref,
    storageName,
    fileActions,
    sharing,
    onOpen,
    onPrint,
    onRename,
    onDelete,
    onClearSearch,
    onShare,
  }: {
    thumbnailSize?: number;
    listing: ListingState<StorageRow>;
    filesFault?: FaultText | undefined;
    loading: boolean;
    emptyFolder: boolean;
    searching: boolean;
    query: string;
    writable: boolean;
    printable: boolean;
    /** The paths of the ticked rows. */
    selection: Selection<string>;
    shows: (field: StorageFieldId) => boolean;
    thumbnailOf: (entry: StorageRow) => string | undefined;
    fileHref?: ((entry: StorageRow) => string) | undefined;
    /** The storage the listed rows are on, handed to `fileActions`. */
    storageName?: string | undefined;
    /**
     * Drawn at the end of each file's row, with its storage, the row, and the
     * row's own acts for the menu, so the row has one menu.
     */
    fileActions?:
      Snippet<[string, StorageRow, StorageRowFold | undefined]> | undefined;
    /** The folders this server shares. With it the table has a Shared column. */
    sharing?: FolderSharing | undefined;
    onOpen: (path: string) => void;
    onPrint: (entry: StorageRow) => void;
    onRename: (entry: StorageRow) => void;
    onDelete: (entry: StorageRow) => void;
    onClearSearch: () => void;
    /** The reader flipped the share of a folder. */
    onShare: (entry: StorageRow, shared: boolean) => void;
  } = $props();

  const rootOf = (entry: StorageRow): string | undefined =>
    sharing?.sharedRoot(storageName ?? '', entry.path);

  // The switch reads the share and never holds it, so a share the reader
  // does not confirm leaves it off.
  const shareOf =
    (entry: StorageRow) =>
    (next: boolean): void => {
      onShare(entry, next);
    };

  // A Shared column takes the room a row's acts had, so with it the acts fold
  // into the row's menu one pane width sooner, and Size goes sooner too.
  const folded = $derived(
    sharing ? '@max-md/storage-files:hidden' : '@max-sm/storage-files:hidden',
  );
  const menuShown = $derived(
    sharing
      ? 'inline-flex @md/storage-files:hidden'
      : 'inline-flex @sm/storage-files:hidden',
  );
  const sizeShown = $derived(
    sharing ? '@max-xl/storage-files:hidden' : '@max-md/storage-files:hidden',
  );

  /** The folder's share the row's menu can flip, when only this row decides it. */
  const sharesHere = (entry: StorageRow): boolean => {
    if (!entry.folder || sharing?.canChange !== true) return false;
    const root = rootOf(entry);
    return root === undefined || root === entry.path;
  };

  const printsFrom = (entry: StorageRow): boolean =>
    printable && !entry.folder && isPrintFile(entry.name);

  const baseName = (path: string): string =>
    path.slice(path.lastIndexOf('/') + 1);
</script>

{#snippet column(field: 'name' | 'size' | 'modified', className?: string)}
  {@const sort = listing.sort}
  <SortableTableHead
    label={t(`storage.column.${field}`)}
    class={className}
    direction={sort?.field === field ? SORT_DIRECTION[sort.order] : undefined}
    onSort={(order: 'asc' | 'desc' | undefined) => {
      if (order === undefined) return;
      listing.setSort(field, order);
      listing.setPage(0);
    }}
  />
{/snippet}

{#snippet printAction(entry: StorageRow)}
  {#if printsFrom(entry)}
    <!-- The word goes where the pane has no room for it; the name stays. On
         a phone-width pane the act moves into the row's menu. -->
    <Button
      size="sm"
      class={[folded, ABOVE_ROW_LINK]}
      aria-label={t('storage.printFor', {
        name: entry.name,
      })}
      onclick={() => {
        onPrint(entry);
      }}
    >
      <PlayIcon aria-hidden="true" />
      <span class="hidden @2xl/storage-files:inline"
        >{t('fileDetail.action.print')}</span
      >
    </Button>
  {/if}
{/snippet}

{#snippet pick(entry: StorageRow)}
  <SelectCheckbox
    {selection}
    key={entry.path}
    label={t('storage.select.row', { name: entry.name })}
  />
{/snippet}

{#snippet tile(entry: StorageRow)}
  <div class="flex w-full min-w-0 flex-col items-center gap-0 text-start">
    <FileThumbnail
      name={entry.name}
      thumbnailUrl={thumbnailOf(entry)}
      glyphSrc={iconSrcFor(entry)}
      size="wide"
      radius="none"
      class="w-full"
    />
    <div class="flex w-full flex-col gap-form-note p-3">
      <ClippedName name={entry.name} class="max-w-full text-sm font-semibold" />
      {#if shows('modified') && entry.modified}
        <span class="block text-xs text-muted-foreground"
          >{formatDateTime(entry.modified)}</span
        >
      {/if}
      {#if shows('size')}
        {#if entry.folder}
          {#if entry.entries !== undefined}
            <span class="text-xs text-muted-foreground">
              {t('storage.entries', { count: entry.entries })}
            </span>
          {/if}
        {:else}
          <span class="font-mono text-xs">{formatBytes(entry.size)}</span>
        {/if}
      {/if}
      <!-- The tile only marks a share. The switch is in the table. -->
      {#if entry.folder && rootOf(entry) !== undefined}
        <span
          class="inline-flex items-center gap-1 text-xs text-muted-foreground"
        >
          <Share2Icon aria-hidden="true" class="size-3.5" />
          {t('storage.share.on')}
        </span>
      {/if}
    </div>
  </div>
{/snippet}

{#if filesFault}
  <FaultAlert fault={filesFault} />
{:else if loading}
  <ListingSkeleton label={t('storage.files.loading')} art rows={6} />
{:else if emptyFolder}
  <EmptyState
    icon={InboxIcon}
    heading={t('storage.empty.heading')}
    headingLevel="h2"
    description={t('storage.empty.note')}
  />
{:else if searching && listing.total === 0}
  <ListingNoMatch
    heading={t('storage.noMatch.heading')}
    headingLevel="h2"
    description={t('storage.noMatch.note', { query: query.trim() })}
    clearLabel={t('primitives.listing.clearSearch')}
    onClear={onClearSearch}
  />
{:else}
  {#if listing.view.value === 'cards'}
    <div class="grid file-preview-grid" style:--thumbnail-step={thumbnailSize}>
      {#each listing.page as entry (entry.path)}
        <Tile
          size="flush"
          class="overflow-hidden"
          selected={selection.has(entry.path)}
          onclick={entry.folder ? () => onOpen(entry.path) : undefined}
        >
          {#snippet top()}
            <span class="absolute start-2 top-2 self-start"
              >{@render pick(entry)}</span
            >
          {/snippet}
          {@render tile(entry)}
        </Tile>
      {/each}
    </div>
  {:else}
    <Table>
      <TableCaption class="sr-only">
        {t('storage.caption')}
      </TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead class="w-11">
            <!-- A label, so the whole cell ticks the box: the column's resize
                 handle sits a few pixels from the 16px box itself. -->
            <Label class="h-9 w-11 justify-center">
              <SelectAllCheckbox
                {selection}
                keys={listing.page.map((entry) => entry.path)}
                label={t('storage.select.page')}
              />
            </Label>
          </TableHead>
          {@render column('name', '@xl/storage-files:w-1/2')}
          {#if shows('size')}{@render column('size', sizeShown)}{/if}
          <!-- Drawn whenever the pane has room, and never offered as a
               field: a toggle the pane overrules would do nothing. -->
          {@render column('modified', 'hidden @4xl/storage-files:table-cell')}
          {#if sharing}
            <!-- Gone where the acts fold, so the switch and the menu's
                 checkbox never both flip one share. -->
            <TableHead class="@max-md/storage-files:hidden"
              >{t('storage.column.shared')}</TableHead
            >
          {/if}
          <TableHead class="sr-only">
            {t('storage.column.actions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {#each listing.page as entry (entry.path)}
          <!-- The row's acts as menu items, for whichever menu the row draws. -->
          {#snippet folds()}
            {#if printsFrom(entry)}
              <DropdownMenuItem onSelect={() => onPrint(entry)}>
                <PlayIcon aria-hidden="true" />
                {t('fileDetail.action.print')}
              </DropdownMenuItem>
            {/if}
            {#if sharesHere(entry)}
              <DropdownMenuCheckboxItem
                checked={rootOf(entry) !== undefined}
                onCheckedChange={shareOf(entry)}
              >
                {t('storage.share.on')}
              </DropdownMenuCheckboxItem>
            {/if}
            {#if writable}
              <DropdownMenuItem onSelect={() => onRename(entry)}>
                <PencilIcon aria-hidden="true" />
                {t('storage.action.rename')}
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => onDelete(entry)}
              >
                <Trash2Icon aria-hidden="true" />
                {t('storage.action.delete')}
              </DropdownMenuItem>
            {/if}
          {/snippet}
          <TableRow
            data-state={selection.has(entry.path) ? 'selected' : undefined}
            class={[
              THUMBNAIL_ROW_MIN_HEIGHT,
              // A file's name is a stretched link too, and without the row to
              // position it against its hit area covers the whole table.
              entry.folder || fileHref !== undefined ? LINKED_ROW : undefined,
            ]}
          >
            <TableCell class="w-11">
              <!-- Over the folder row's own link, so the tick is not a step in. -->
              <span
                class={[
                  'flex size-11 items-center justify-center',
                  ABOVE_ROW_LINK,
                ]}
              >
                {@render pick(entry)}
              </span>
            </TableCell>
            <TableCell>
              <div class="flex items-center gap-3 font-semibold">
                <FileThumbnail
                  name={entry.name}
                  thumbnailUrl={thumbnailOf(entry)}
                  glyphSrc={iconSrcFor(entry)}
                  size={entry.folder ? 'md' : 'wide'}
                  class={entry.folder
                    ? '@max-sm/storage-files:hidden'
                    : [
                        'max-w-file-preview @max-sm/storage-files:hidden',
                        FILE_WIDTHS[thumbnailSize],
                      ]}
                />
                {#if entry.folder}
                  <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- ROW_LINK button stretched over the table row -->
                  <button
                    type="button"
                    class={['text-start', ROW_LINK]}
                    onclick={() => onOpen(entry.path)}
                  >
                    <ClippedName name={entry.name} />
                  </button>
                {:else if fileHref}
                  <a href={fileHref(entry)} class={['text-start', ROW_LINK]}>
                    <ClippedName name={entry.name} />
                  </a>
                {:else}
                  <ClippedName name={entry.name} />
                {/if}
              </div>
            </TableCell>
            {#if shows('size')}
              <!-- A folder holds no bytes of its own; what it holds is rows. -->
              <TableCell class={['text-xs whitespace-nowrap', sizeShown]}>
                {#if entry.folder}
                  {#if entry.entries !== undefined}
                    <span class="text-muted-foreground">
                      {t('storage.entries', { count: entry.entries })}
                    </span>
                  {/if}
                {:else}
                  <span class="font-mono">{formatBytes(entry.size)}</span>
                {/if}
              </TableCell>
            {/if}
            <TableCell
              class="hidden whitespace-nowrap @4xl/storage-files:table-cell"
            >
              {formatDateTime(entry.modified)}
            </TableCell>
            {#if sharing}
              <!-- A folder inside a shared one is shared with it, so it says
                   which, and only the folder that is shared has the switch. -->
              <TableCell class="whitespace-nowrap @max-md/storage-files:hidden">
                {#if entry.folder}
                  {@const root = rootOf(entry)}
                  {#if root !== undefined && root !== entry.path}
                    <span class="text-xs text-muted-foreground">
                      {t('storage.share.inside', { folder: baseName(root) })}
                    </span>
                  {:else if sharing.canChange}
                    <Switch
                      size="sm"
                      class={ABOVE_ROW_LINK}
                      aria-label={t('storage.share.for', { name: entry.name })}
                      bind:checked={() => root !== undefined, shareOf(entry)}
                    />
                  {:else if root !== undefined}
                    <span
                      class="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
                    >
                      <Share2Icon aria-hidden="true" class="size-3.5" />
                      {t('storage.share.on')}
                    </span>
                  {/if}
                {/if}
              </TableCell>
            {/if}
            <TableCell class="text-end whitespace-nowrap">
              {#if writable}
                <Button
                  variant="ghost"
                  size="sm"
                  class={[folded, ABOVE_ROW_LINK]}
                  aria-label={t('storage.renameFor', {
                    name: entry.name,
                  })}
                  onclick={() => {
                    onRename(entry);
                  }}
                >
                  <PencilIcon aria-hidden="true" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class={[folded, ABOVE_ROW_LINK]}
                  aria-label={t('storage.deleteFor', {
                    name: entry.name,
                  })}
                  onclick={() => {
                    onDelete(entry);
                  }}
                >
                  <Trash2Icon aria-hidden="true" />
                </Button>
              {/if}
              {@render printAction(entry)}
              {@const folding =
                writable || printsFrom(entry) || sharesHere(entry)}
              {#if fileActions && storageName !== undefined && !entry.folder}
                <!-- A module's menu takes the row's acts, so the row has one
                     kebab. -->
                {@render fileActions(
                  storageName,
                  entry,
                  folding ? { items: folds, shown: menuShown } : undefined,
                )}
              {:else if folding}
                <!-- A phone-width pane has room for one control per row, so
                     every act folds into the row's menu there. -->
                <span class={[menuShown, ABOVE_ROW_LINK]}>
                  <RowActions name={entry.name}>{@render folds()}</RowActions>
                </span>
              {/if}
            </TableCell>
          </TableRow>
        {/each}
      </TableBody>
    </Table>
  {/if}

  {#if listing.pager}
    <Pager {...listing.pager} total={listing.total} />
  {/if}
{/if}
