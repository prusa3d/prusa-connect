<!--
  One material: a swatch, the name that opens its page, the brand and type, a
  star, how many spools there are, with a printer and a count in brackets when
  some of them are on a printer, the weight left against the full weight of
  those spools, and a + that adds spools of it. A spool with no full weight
  recorded leaves the row the weight alone: a bar would draw that spool as
  untouched.

  The whole row opens the material's page: the name's link is stretched over
  the row (the host's row-link pattern), and the printer count, the + and the
  star sit above it.

  The row is the same table row at both widths; below `@md` it is set to a card
  (see filament's spool table).
-->
<script lang="ts">
  import PlusIcon from '@lucide/svelte/icons/plus';
  import {
    ABOVE_ROW_LINK,
    Button,
    LINKED_ROW,
    MaterialSwatch,
    PrinterGlyphIcon,
    Progress,
    ROW_LINK,
    TableCell,
    TableRow,
    Toggle,
  } from '@prusa/connect-ui/primitives';

  import type { Row } from '../api';
  import { heldHost } from '../host';
  import { heldList } from '../list.svelte';
  import TapTooltip from '../tap-tooltip.svelte';
  import { kilograms } from './weights';

  let {
    row,
    onadd,
  }: {
    row: Row;
    /** Opens the add dialog on this material's spool form. */
    onadd: (uuid: string) => void;
  } = $props();

  const list = heldList();
  // A row with no spools has nothing to measure against.
  const capacity = $derived(
    row.capacity_g === undefined || row.capacity_g <= 0
      ? undefined
      : row.capacity_g,
  );
  const host = heldHost();
  const t = host.t;

  // A card's own field name, hidden once the header row is back.
  const NAME = 'text-xs font-medium text-muted-foreground @md:hidden';
  const CELL =
    '@max-md:flex @max-md:items-baseline @max-md:justify-between @max-md:gap-3 @max-md:whitespace-normal @max-md:px-0 @max-md:py-1';
</script>

<TableRow
  data-part="material"
  data-material={row.uuid}
  class={[
    LINKED_ROW,
    '@max-md:rounded-card @max-md:block @max-md:border @max-md:border-border @max-md:p-3',
  ]}
>
  <TableCell class={CELL}>
    <span class={NAME}>{t('materials.column.colour')}</span>
    <MaterialSwatch
      class="size-6"
      material={{ name: row.name, colorRgba: row.color_rgba }}
    />
  </TableCell>

  <TableCell class={[CELL, 'font-medium']}>
    <span class={NAME}>{t('materials.column.name')}</span>
    <a
      href={host.pageHref('material') + '?uuid=' + row.uuid}
      class={[ROW_LINK, 'underline-offset-4 hover:underline']}
    >
      {row.name}
    </a>
  </TableCell>

  <TableCell class={CELL}>
    <span class={NAME}>{t('materials.column.brand')}</span>
    <span>{row.brand_name}</span>
  </TableCell>

  <TableCell class={CELL}>
    <span class={NAME}>{t('materials.column.type')}</span>
    <span>{row.type}</span>
  </TableCell>

  <TableCell class={CELL}>
    <span class={NAME}>{t('materials.column.spools')}</span>
    <span class="inline-flex items-center gap-2 whitespace-nowrap">
      <span>{row.spools}</span>
      {#if row.assigned_spools > 0}
        <TapTooltip
          class={[ABOVE_ROW_LINK, 'gap-0.5 text-muted-foreground']}
          text={t('materials.row.assigned', { n: row.assigned_spools })}
        >
          <PrinterGlyphIcon aria-hidden="true" class="size-4 shrink-0" />
          <span aria-hidden="true">{row.assigned_spools}</span>
        </TapTooltip>
      {/if}
    </span>
  </TableCell>

  <TableCell class={[CELL, '@md:w-56']}>
    <span class={NAME}>{t('materials.column.weight')}</span>
    <div class="flex flex-col items-end gap-1 @md:w-full @md:items-start">
      <span class="text-sm whitespace-nowrap">
        {capacity === undefined
          ? t('materials.row.weight', { kg: kilograms(row.remaining_g) })
          : t('materials.row.weightOf', {
              kg: kilograms(row.remaining_g),
              full: kilograms(capacity),
            })}
      </span>
      {#if capacity !== undefined}
        <Progress
          value={Math.min(row.remaining_g, capacity)}
          max={capacity}
          aria-label={t('materials.row.bar', { name: row.name })}
          class="w-full min-w-24"
        />
      {/if}
    </div>
  </TableCell>

  <TableCell class={[CELL, 'whitespace-nowrap @md:text-right']}>
    {#if list.mayWrite}
      <Button
        variant="ghost"
        size="sm"
        class={ABOVE_ROW_LINK}
        aria-label={t('materials.row.add', { name: row.name })}
        title={t('materials.row.addTitle')}
        onclick={() => {
          onadd(row.uuid);
        }}
      >
        <PlusIcon aria-hidden="true" />
      </Button>
    {/if}
    <Toggle
      size="sm"
      class={ABOVE_ROW_LINK}
      bind:pressed={
        () => row.is_favourite, (on: boolean) => void list.star(row.uuid, on)
      }
      aria-label={t('materials.row.star', { name: row.name })}
      disabled={!list.mayWrite || list.busy}
    >
      <span
        class={[
          'text-base',
          row.is_favourite &&
            'text-(--color-orange-light-9) dark:text-(--color-orange-dark-9)',
        ]}
        aria-hidden="true"
      >
        {row.is_favourite ? '★' : '☆'}
      </span>
    </Toggle>
  </TableCell>
</TableRow>
