<!--
  This draws the tags as tiles, for a narrow container and for a wall tablet.
  Many tags read faster in three columns than in one long list.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  import TagPrinters from './tag-printers.svelte';
  import type { TagFieldId, TagRow } from './types';

  let {
    rows,
    shows,
    action,
  }: {
    rows: readonly TagRow[];
    shows: (field: TagFieldId) => boolean;
    action?: Snippet<[TagRow]> | undefined;
  } = $props();
</script>

<!-- The query reads the container, so the grid is inside it and not on it. -->
<div class="@container/tag-cards">
  <div
    class="grid grid-cols-1 gap-3 @[32rem]/tag-cards:grid-cols-2 @[56rem]/tag-cards:grid-cols-3"
  >
    {#each rows as row (row.id)}
      <div
        data-slot="tag-card"
        class="flex flex-col gap-2 rounded-md border border-border p-3"
      >
        <p class="min-w-0 text-sm font-medium">{row.tag}</p>
        <div class="flex flex-wrap items-center justify-between gap-2">
          {#if shows('printers')}
            <TagPrinters {row} />
          {/if}
          {#if action}
            {@render action(row)}
          {/if}
        </div>
      </div>
    {/each}
  </div>
</div>
