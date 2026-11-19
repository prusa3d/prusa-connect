<!--
  One tile on the grid: a card at its place, with what its kind draws inside.

  While the dashboard is being changed, the tile carries its own controls: a
  grip to drag it by, its settings, a copy and a delete in the corner, and a
  handle at the far corner to resize it. What the tile draws is made inert
  then, so a click on a printer moves the tile rather than opening the
  printer, and Tab walks from tile to tile rather than through every link in
  them.

  The tile is placed by four custom properties and the stylesheet does the
  arithmetic (frontend-css.md). The one transform is the drag ghost, which
  follows the pointer between the cells it snaps to.
-->
<script lang="ts">
  import { Button } from '../../../primitives';
  import CopyIcon from '@lucide/svelte/icons/copy';
  import GripVerticalIcon from '@lucide/svelte/icons/grip-vertical';
  import MoveDiagonal2Icon from '@lucide/svelte/icons/move-diagonal-2';
  import Settings2Icon from '@lucide/svelte/icons/settings-2';
  import TrashIcon from '@lucide/svelte/icons/trash';

  import type { DashboardCard } from '../../../domain';
  import { t } from '../../../i18n';
  import { dashboardContext } from '../context';
  import CardBody from './card-body.svelte';

  let {
    tile,
    name,
    order,
    editing,
    selected,
    dragged,
    offsetX,
    offsetY,
    describedBy,
    onkeydown,
    ongrab,
    onstretch,
    onsettings,
    onremove,
  }: {
    tile: DashboardCard;
    /** What the tile is called, for a control that names it. */
    name: string;
    /** Its place in reading order, for a narrow screen. */
    order: number;
    editing: boolean;
    selected: boolean;
    dragged: 'move' | 'resize' | undefined;
    offsetX: number;
    offsetY: number;
    describedBy: string | undefined;
    onkeydown: (event: KeyboardEvent) => void;
    ongrab: (event: PointerEvent) => void;
    onstretch: (event: PointerEvent) => void;
    onsettings: () => void;
    onremove: () => void;
  } = $props();

  const { editor } = dashboardContext();
</script>

{#snippet control(
  label: string,
  Icon: typeof CopyIcon,
  run: () => void,
  part: string,
)}
  <Button size="icon-sm" variant="ghost" {label} data-part={part} onclick={run}>
    <Icon aria-hidden="true" />
  </Button>
{/snippet}

<div
  data-part="tile"
  data-slot="dashboard-card"
  data-kind={tile.type}
  data-tile={tile.id}
  data-selected={selected ? '' : undefined}
  data-dragged={dragged}
  class="board-tile group/tile relative min-w-0"
  style="--x: {tile.x}; --y: {tile.y}; --w: {tile.w}; --h: {tile.h}; --order: {order}; --dx: {offsetX}px; --dy: {offsetY}px"
>
  <!-- The look of the library's card without its spacing: a tile spaces its
       body itself, and the card's own padding would show around a tinted note.
       Each card is an article: it stands on its own, under its own heading.
       It takes focus only while the dashboard is changed, as a group the
       arrow keys move. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <article
    role={editing ? 'group' : undefined}
    aria-roledescription={editing ? t('dashboard.grid.tileRole') : undefined}
    aria-label={editing ? name : undefined}
    aria-describedby={describedBy}
    tabindex={editing ? 0 : undefined}
    {onkeydown}
    onfocus={() => {
      if (editing) editor.selected = tile.id;
    }}
    class={[
      'isolate flex h-full flex-col overflow-hidden rounded-md border border-border/65 bg-card text-card-foreground outline-none not-dark:border-panel-outline',
      editing &&
        'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
      selected && 'ring-2 ring-primary',
      dragged !== undefined && 'shadow-lg',
    ]}
  >
    <div class="tile-body flex h-full min-h-0 flex-col" inert={editing}>
      <CardBody card={tile} {editing} />
    </div>

    {#if editing}
      <div
        data-part="tile-controls"
        class="absolute end-1 inset-bs-1 flex items-center gap-0.5 rounded-sm border border-border bg-popover p-0.5 opacity-0 shadow-sm transition-opacity duration-150 group-focus-within/tile:opacity-100 group-hover/tile:opacity-100 group-data-selected/tile:opacity-100 motion-reduce:transition-none"
      >
        <Button
          size="icon-sm"
          variant="ghost"
          data-part="tile-grip"
          data-grip="move"
          label={t('dashboard.grid.move', { name })}
          onpointerdown={ongrab}
        >
          <GripVerticalIcon aria-hidden="true" />
        </Button>
        {@render control(
          t('dashboard.grid.settings', { name }),
          Settings2Icon,
          onsettings,
          'tile-settings',
        )}
        {@render control(
          t('dashboard.grid.duplicate', { name }),
          CopyIcon,
          () => {
            editor.duplicate(tile.id);
          },
          'tile-duplicate',
        )}
        {@render control(
          t('dashboard.grid.remove', { name }),
          TrashIcon,
          onremove,
          'tile-remove',
        )}
      </div>

      <Button
        size="icon-sm"
        variant="ghost"
        data-part="tile-stretch"
        data-grip="resize"
        label={t('dashboard.grid.resize', { name })}
        tabindex={-1}
        class="absolute end-0 inset-be-0 opacity-0 transition-opacity duration-150 group-focus-within/tile:opacity-100 group-hover/tile:opacity-100 group-data-selected/tile:opacity-100 motion-reduce:transition-none"
        onpointerdown={onstretch}
      >
        <MoveDiagonal2Icon aria-hidden="true" class="rotate-90" />
      </Button>
    {/if}
  </article>
</div>

<style>
  .board-tile {
    grid-column: calc(var(--x) + 1) / span var(--w);
    grid-row: calc(var(--y) + 1) / span var(--h);
  }

  /* The drag ghost: the tile sits in the cell it would land in and is drawn
     where the pointer holds it, above the others. */
  .board-tile[data-dragged] {
    z-index: 10;
  }

  .board-tile[data-dragged='move'] {
    transform: translate(var(--dx), var(--dy));
  }

  .board-tile {
    container: dashboard-tile / inline-size;
  }

  .tile-body {
    --dashboard-panel-padding: var(--spacing-dashboard-panel-padding);
    --dashboard-heading-gap: var(--spacing-dashboard-heading-gap);
    padding-block: var(--spacing-dashboard-padding-block);
    padding-inline: var(--dashboard-panel-padding);
  }

  .board-tile[data-kind='core/summary'] .tile-body {
    padding-block: var(--spacing-dashboard-summary-padding);
  }

  /* A pointer on a grip drags. Without this a touch screen scrolls instead.
     The grip is the library's button, so the rule reaches into it by its
     data attribute, and it outranks the button's own pointer cursor. */
  .board-tile :global([data-grip]) {
    touch-action: none;
  }

  .board-tile :global([data-grip='move']) {
    cursor: grab;
  }

  .board-tile :global([data-grip='resize']) {
    cursor: nwse-resize;
  }

  @container board (width < 40rem) {
    .board-tile {
      grid-column: 1 / -1;
      grid-row: auto / span var(--h);
      order: var(--order);
    }
  }
</style>
