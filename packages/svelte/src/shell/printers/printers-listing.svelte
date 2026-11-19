<!--
  Which of the two row tiers the reader gets.

  It is a container query and not a measurement in script: the screen holds no
  width state, and the switch costs no repaint. Both tiers stay in the DOM and
  the one that does not apply is `display: none`.

  72rem is where the eight columns stop fitting without wrapping a cell.
-->
<script lang="ts">
  import PrintersRows from './printers-rows.svelte';
  import PrintersTable from './printers-table.svelte';
  import type { PrinterFieldId, PrinterRow } from './types';

  let {
    rows,
    shows,
    sortField,
    sortOrder,
    onSort,
  }: {
    rows: readonly PrinterRow[];
    sortField?: string | undefined;
    sortOrder?: 'asc' | 'desc' | undefined;
    onSort?:
      | Partial<
          Record<
            'name' | 'state',
            ((order: 'asc' | 'desc') => void) | undefined
          >
        >
      | undefined;
    shows: (field: PrinterFieldId) => boolean;
  } = $props();
</script>

<div class="@container/printer-listing">
  <div class="hidden @4xl/printer-listing:block">
    <PrintersTable {rows} {shows} {sortField} {sortOrder} {onSort} />
  </div>
  <div class="@4xl/printer-listing:hidden">
    <PrintersRows {rows} {shows} />
  </div>
</div>
