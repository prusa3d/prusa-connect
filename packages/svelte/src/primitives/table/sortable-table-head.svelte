<!--
  A column header that sorts its column.

  The sort order itself stays with the caller: it passes the current
  `direction` and takes the click in `onSort`, which is handed the order the
  press asks for: ascending first, then the reverse of the order shown.
  `aria-sort` is `none` while the column is not the one sorted, so a reader
  hears that it can be.
-->
<script lang="ts">
  import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
  import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
  import ArrowUpDownIcon from '@lucide/svelte/icons/arrow-up-down';

  import TableHead from './table-head.svelte';
  import type { SortableTableHeadProps } from './types';

  let {
    label,
    direction,
    onSort,
    align = 'start',
    class: className,
  }: SortableTableHeadProps = $props();

  const end = $derived(align === 'end');
  const GLYPH = {
    ascending: ArrowUpIcon,
    descending: ArrowDownIcon,
    none: ArrowUpDownIcon,
  };
  const Glyph = $derived(GLYPH[direction ?? 'none']);
</script>

<TableHead
  aria-sort={direction ?? 'none'}
  class={[end && 'text-end', className]}
>
  <!-- One button per header, the label and glyph its target: compact under a
       mouse at Button's small height, never under 24px, and the touch target
       under a finger. -->
  <button
    type="button"
    class={[
      'inline-flex min-h-control-height min-w-control-height items-center gap-1 rounded-sm outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background @max-3xl/app-shell:min-h-touch-target @max-3xl/app-shell:min-w-touch-target pointer-coarse:min-h-touch-target pointer-coarse:min-w-touch-target',
      end && 'flex-row-reverse',
    ]}
    onclick={() => onSort(direction === 'ascending' ? 'desc' : 'asc')}
  >
    {label}
    <Glyph
      aria-hidden="true"
      class={['size-3', direction === undefined && 'opacity-50']}
    />
  </button>
</TableHead>
