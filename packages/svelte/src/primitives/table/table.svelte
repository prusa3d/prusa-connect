<!--
  Rows of data with a header.

  The table sits in a box that scrolls sideways, so a table wider than its
  column keeps its columns at their own width instead of squeezing every cell.
  The caller's class goes on the table, so a width or a layout the caller sets
  applies to the table and not to the box around it.
-->
<script lang="ts">
  import { resizeTableColumns } from './column-resizing';
  import type { TableProps } from './types';

  let { class: className, children, ...rest }: TableProps = $props();
</script>

<div data-slot="table-container" class="relative w-full overflow-x-auto">
  <table
    data-slot="table"
    class={[
      'w-full caption-bottom text-sm',
      '[&[data-column-insets]>:is(thead,tbody,tfoot)>tr>:is(th,td):not(:first-child,:last-child)]:ps-listing-column',
      '[&[data-column-insets=all]>:is(thead,tbody,tfoot)>tr>:is(th,td):last-child:not(:first-child)]:ps-listing-column',
      className,
    ]}
    {...rest}
    {@attach resizeTableColumns}
  >
    {@render children?.()}
  </table>
</div>

<style>
  table[data-column-widths] {
    table-layout: fixed;
    inline-size: var(--table-resized-width);
  }

  table[data-column-widths] > :global(colgroup > col) {
    inline-size: var(--table-column-width);
  }

  table[data-column-widths]
    > :global(:is(thead, tbody, tfoot) > tr > :is(th, td)) {
    overflow: clip;
    text-overflow: ellipsis;
  }
</style>
