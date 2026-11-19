<!--
  The column a dashboard tile draws in: the heading a person wrote over it,
  else the one its kind gives it, then the body, given what is left of the
  tile's height. With no heading and nothing trailing there is no heading
  row, so a clock or a note fills its tile.
-->
<script lang="ts">
  import type { TilePanelProps } from './types';

  let {
    title,
    fallback,
    subline,
    trailing,
    gap = 'sm',
    children,
    class: className,
  }: TilePanelProps = $props();

  const shown = $derived(title.trim() === '' ? (fallback ?? '') : title);
</script>

<div
  data-slot="tile-panel"
  class={[
    'flex min-h-0 flex-1 flex-col',
    gap === 'sm' && 'gap-2',
    gap === 'md' && 'gap-dashboard-heading-gap',
    className,
  ]}
>
  {#if shown !== '' || trailing}
    <div
      class="flex min-h-dashboard-heading min-w-0 items-center justify-between gap-2"
    >
      <div class="min-w-0">
        {#if shown !== ''}
          <h2 class="min-w-0 truncate text-sm font-semibold">{shown}</h2>
        {/if}
        {#if subline}
          <p class="truncate text-xs text-muted-foreground">{subline}</p>
        {/if}
      </div>
      {#if trailing}
        <span class="flex shrink-0 items-center gap-1"
          >{@render trailing()}</span
        >
      {/if}
    </div>
  {/if}
  {@render children()}
</div>
