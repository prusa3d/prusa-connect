<script lang="ts">
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
  import HardDriveDownloadIcon from '@lucide/svelte/icons/hard-drive-download';
  import PlayIcon from '@lucide/svelte/icons/play';

  import { formatBytes, formatDateTime, t } from '../../../i18n';
  import { fileRowPreview } from '../../../primitives/file-thumbnail/variants';
  import {
    ABOVE_ROW_LINK,
    Badge,
    Button,
    ClippedName,
    FileThumbnail,
    LINKED_ROW,
    ROW_LINK,
    SelectCheckbox,
    TableCell,
    TableRow,
    THUMBNAIL_ROW_MIN_HEIGHT,
  } from '../../../primitives';
  import fileIcon from '../../storage/assets/file.svg';
  import folderIcon from '../../storage/assets/folder.svg';
  import PrinterFileActions from './printer-file-actions.svelte';
  import PrinterFileRow from './printer-file-row.svelte';
  import type { PrinterFileEntry, PrinterFileRowContext } from './types';

  let {
    entry,
    depth,
    thumbnailSize = 0,
    row,
  }: {
    entry: PrinterFileEntry;
    /** How far in a disclosed folder's children are drawn. */
    depth: number;
    thumbnailSize?: number;
    row: PrinterFileRowContext;
  } = $props();

  const print = $derived(row.printControl(entry));
  const open = $derived(row.isOpen(entry));
  const children = $derived(open ? row.childrenOf(entry) : undefined);
</script>

{#snippet badges()}
  <!-- The row's own Flash button drops its label in a narrow container. -->
  {#if row.isFirmware(entry)}
    <Badge variant="outline" class="shrink-0">
      {t('printerDetail.files.badge.firmware')}
    </Badge>
  {:else if row.isModel(entry)}
    <Badge variant="outline" class="shrink-0">
      {t('printerDetailFiles.badge.modelOnly')}
    </Badge>
  {/if}
{/snippet}

<TableRow
  data-state={row.selection.has(entry.path) ? 'selected' : undefined}
  class={[THUMBNAIL_ROW_MIN_HEIGHT, entry.folder ? LINKED_ROW : undefined]}
>
  <TableCell class="w-11">
    <span class={['flex size-11 items-center justify-center', ABOVE_ROW_LINK]}>
      <SelectCheckbox
        selection={row.selection}
        key={entry.path}
        label={t('printerDetailFiles.select.row', {
          name: row.nameOf(entry),
        })}
      />
    </span>
  </TableCell>
  <TableCell>
    <div
      class="flex items-center gap-3 font-semibold"
      style="padding-inline-start: calc(var(--spacing) * {depth * 4})"
    >
      {#if row.discloses() && entry.folder}
        <!-- The row's own press walks into the folder. -->
        <Button
          variant="ghost"
          size="icon-md"
          class={['@max-md/printer-file-listing:hidden', ABOVE_ROW_LINK]}
          aria-expanded={open}
          label={open
            ? t('printerDetailFiles.tree.collapse', { name: row.nameOf(entry) })
            : t('printerDetailFiles.tree.expand', { name: row.nameOf(entry) })}
          onclick={() => row.onDisclose(entry)}
        >
          <ChevronRightIcon
            aria-hidden="true"
            class={['transition-transform', open && 'rotate-90']}
          />
        </Button>
      {:else}
        <span
          class="hidden size-9 shrink-0 @md/printer-file-listing:block"
          aria-hidden="true"
        ></span>
      {/if}
      <FileThumbnail
        name={row.nameOf(entry)}
        thumbnailUrl={row.thumbnailOf(entry)}
        glyphSrc={entry.folder ? folderIcon : fileIcon}
        size={entry.folder ? 'md' : 'wide'}
        class={entry.folder
          ? '@max-sm/printer-file-listing:hidden'
          : [
              fileRowPreview(thumbnailSize),
              '@max-sm/printer-file-listing:hidden',
            ]}
      />
      {#if entry.folder}
        <!-- eslint-disable-next-line svelte/no-restricted-html-elements -- a ROW_LINK button, the documented composition for a row that opens a folder -->
        <button
          type="button"
          class={['flex-1 text-start', ROW_LINK]}
          onclick={() => row.onOpenFolder(entry.path)}
        >
          <ClippedName name={row.nameOf(entry)} />
        </button>
      {:else}
        <!-- A badge goes under the name once the name is down to its tail,
             so a narrow column never trades the name for the badge. -->
        <div class="flex flex-1 flex-wrap items-center gap-x-3 gap-y-1">
          <ClippedName name={row.nameOf(entry)} class="flex-1" />
          {@render badges()}
        </div>
      {/if}
    </div>
  </TableCell>
  {#if row.shows('size')}
    <TableCell
      class="font-mono text-xs whitespace-nowrap @max-lg/printer-file-listing:hidden"
    >
      {entry.folder ? '' : formatBytes(entry.size)}
    </TableCell>
  {/if}
  <TableCell
    class="hidden whitespace-nowrap @4xl/printer-file-listing:table-cell"
  >
    {entry.modified === undefined ? '' : formatDateTime(entry.modified)}
  </TableCell>
  <TableCell class="text-end">
    <div class="flex items-center justify-end gap-1">
      {#if children !== undefined}
        <!-- Only an expanded folder carries a count. -->
        <span class="shrink-0 text-xs text-muted-foreground">
          {t('printerDetailFiles.tree.items', { count: children.length })}
        </span>
      {/if}
      <!-- On the row while the pane has room; the row's menu holds it too. -->
      {#if print}
        <Button
          size="sm"
          class={['@max-sm/printer-file-listing:hidden', ABOVE_ROW_LINK]}
          disabled={print.disabled}
          aria-label={print.reason === undefined
            ? t('printerDetail.files.printFor', { name: row.nameOf(entry) })
            : t('printerDetailFiles.action.printRefusedFor', {
                name: row.nameOf(entry),
                reason: print.reason,
              })}
          {...print.reason === undefined ? {} : { title: print.reason }}
          onclick={() => row.onPrint(entry)}
        >
          <PlayIcon aria-hidden="true" />
          <!-- The word goes where the pane has no room for it. -->
          <span class="hidden @2xl/printer-file-listing:inline"
            >{t('printerDetailFiles.action.print')}</span
          >
        </Button>
      {/if}
      <!-- Where Print sits on a plate. -->
      {#if row.canFlash(entry)}
        <Button
          size="sm"
          class={['@max-md/printer-file-listing:hidden', ABOVE_ROW_LINK]}
          aria-label={t('printerDetail.files.flashFor', {
            name: row.nameOf(entry),
          })}
          onclick={() => {
            row.onFlash(entry);
          }}
        >
          <HardDriveDownloadIcon aria-hidden="true" />
          {t('printerDetail.files.action.flash')}
        </Button>
      {/if}
      <PrinterFileActions {entry} {row} />
    </div>
  </TableCell>
</TableRow>
{#if children !== undefined}
  {#each children as child (child.path)}
    <PrinterFileRow entry={child} depth={depth + 1} {row} {thumbnailSize} />
  {/each}
{/if}
