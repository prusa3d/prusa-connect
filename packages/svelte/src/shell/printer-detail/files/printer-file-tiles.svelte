<!-- The same listing as a grid of tiles. -->
<script lang="ts">
  import type { Snippet } from 'svelte';

  import { formatBytes, formatDateTime, t } from '../../../i18n';
  import {
    ClippedName,
    SelectCheckbox,
    type Selection,
    Tile,
  } from '../../../primitives';
  import {
    FILE_CARD_DETAILS,
    FILE_CARD_GRID,
  } from '../../../primitives/file-thumbnail/variants';
  import fileIcon from '../../storage/assets/file.svg';
  import folderIcon from '../../storage/assets/folder.svg';
  import type { PrinterFileEntry } from './types';

  let {
    rows,
    thumbnailSize = 0,
    name,
    thumbnail,
    shows,
    selection,
    onOpen,
    actions,
  }: {
    rows: readonly PrinterFileEntry[];
    thumbnailSize?: number;
    /** What a person reads the entry as, which is not what the printer addresses it by. */
    name: (entry: PrinterFileEntry) => string;
    /** The plate the file was sliced from, absent for one this server never sent. */
    thumbnail: (entry: PrinterFileEntry) => string | undefined;
    shows: (field: string) => boolean;
    selection: Selection<string>;
    onOpen: (path: string) => void;
    actions: Snippet<[PrinterFileEntry]>;
  } = $props();

  // The thumbnail route answers 404 for a copy that carries no picture, and
  // nothing in the listing says which copies do, so a tile draws the image and
  // falls back to its glyph on the failure. Keyed by the printer's own path,
  // which is what the listing keys a tile by as well.
  const broken = $state<Record<string, boolean>>({});
</script>

<div class={['grid', FILE_CARD_GRID]} style:--thumbnail-step={thumbnailSize}>
  {#each rows as entry (entry.path)}
    {@const plate = broken[entry.path] === true ? undefined : thumbnail(entry)}
    <Tile
      data-part="printer-file-tile"
      size="flush"
      radius="md"
      class="overflow-hidden [&>div:first-child]:pointer-events-none [&>div:first-child]:absolute [&>div:first-child]:inset-x-0 [&>div:first-child]:top-0"
      selected={selection.has(entry.path)}
      onclick={entry.folder ? () => onOpen(entry.path) : undefined}
    >
      {#snippet top()}
        <div
          class="flex items-start justify-between gap-1 [&>*]:pointer-events-auto"
        >
          <span class="flex size-8 shrink-0 items-center justify-center">
            <SelectCheckbox
              {selection}
              key={entry.path}
              label={t('printerDetailFiles.select.row', {
                name: name(entry),
              })}
            />
          </span>
          {@render actions(entry)}
        </div>
      {/snippet}
      <span class="flex min-w-0 flex-col gap-0">
        <span
          class="flex aspect-video w-full items-center justify-center overflow-hidden rounded-none bg-surface"
        >
          {#if plate === undefined}
            <img
              src={entry.folder ? folderIcon : fileIcon}
              alt=""
              class="opacity-75 invert dark:invert-0"
            />
          {:else}
            <!-- Decoration: the name sits below it in the tile. -->
            <img
              src={plate}
              alt=""
              class="size-full object-cover"
              onerror={() => {
                broken[entry.path] = true;
              }}
            />
          {/if}
        </span>
        <span class={['contents', FILE_CARD_DETAILS]}>
          <ClippedName
            name={name(entry)}
            class="w-full min-w-0 text-sm font-semibold"
          />
          {#if shows('modified') && entry.modified !== undefined}
            <span class="text-xs text-muted-foreground"
              >{formatDateTime(entry.modified)}</span
            >
          {/if}
          {#if shows('size') && !entry.folder}
            <span class="font-mono text-xs text-muted-foreground"
              >{formatBytes(entry.size)}</span
            >
          {/if}
        </span>
      </span>
    </Tile>
  {/each}
</div>
