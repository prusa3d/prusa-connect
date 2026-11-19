<!--
  A whole table, for the test to render. The parts only work together, and a
  test cannot write markup of its own.

  The first column of each row is a header cell, so the test can prove a row
  header overrides the `col` scope a column header takes.
-->
<script lang="ts">
  import { Checkbox } from '../primitives/checkbox';
  import TableBody from '../primitives/table/table-body.svelte';
  import TableCaption from '../primitives/table/table-caption.svelte';
  import TableCell from '../primitives/table/table-cell.svelte';
  import TableFooter from '../primitives/table/table-footer.svelte';
  import TableHead from '../primitives/table/table-head.svelte';
  import TableHeader from '../primitives/table/table-header.svelte';
  import TableRow from '../primitives/table/table-row.svelte';
  import Table from '../primitives/table/table.svelte';

  interface Props {
    caption: string;
    columns: string[];
    /** One row each. The first entry is the row's own header cell. */
    rows: string[][];
    total: string;
    /** The row header of the row drawn as selected. */
    selected?: string;
    /** The label of a tick box drawn as a narrow first column, as a listing's select-all. */
    selectAll?: string;
  }

  let { caption, columns, rows, total, selected, selectAll }: Props = $props();
  let ticked = $state(false);
</script>

<Table>
  <TableCaption>{caption}</TableCaption>
  <TableHeader>
    <TableRow>
      {#if selectAll}
        <TableHead class="w-8">
          <Checkbox bind:checked={ticked} aria-label={selectAll} />
        </TableHead>
      {/if}
      {#each columns as column (column)}
        <TableHead>{column}</TableHead>
      {/each}
    </TableRow>
  </TableHeader>
  <TableBody>
    {#each rows as row (row[0])}
      <TableRow data-state={row[0] === selected ? 'selected' : undefined}>
        {#if selectAll}
          <TableCell></TableCell>
        {/if}
        <TableHead scope="row">{row[0]}</TableHead>
        {#each row.slice(1) as cell, index (columns[index + 1])}
          <TableCell>{cell}</TableCell>
        {/each}
      </TableRow>
    {/each}
  </TableBody>
  <TableFooter>
    <TableRow>
      <TableCell colspan={columns.length + (selectAll ? 1 : 0)}
        >{total}</TableCell
      >
    </TableRow>
  </TableFooter>
</Table>
