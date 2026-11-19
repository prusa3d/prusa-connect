<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import TableBody from './table-body.svelte';
  import TableCaption from './table-caption.svelte';
  import TableCell from './table-cell.svelte';
  import TableFooter from './table-footer.svelte';
  import SortableTableHead from './sortable-table-head.svelte';
  import TableHead from './table-head.svelte';
  import TableHeader from './table-header.svelte';
  import TableRow from './table-row.svelte';
  import Table from './table.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Table',
    component: Table,
  });

  const files = [
    {
      name: 'firmware.bbl',
      size: '18.2 MB',
      modified: '2026-08-12',
      status: 'Ready',
    },
    {
      name: 'benchy.3mf',
      size: '4.1 MB',
      modified: '2026-08-14',
      status: 'Printing',
    },
    {
      name: 'bracket.stl',
      size: '820 KB',
      modified: '2026-08-15',
      status: 'Queued',
    },
  ];
</script>

{#snippet fileTable()}
  <Table>
    <TableCaption>A list of files stored on this printer.</TableCaption>
    <TableHeader>
      <TableRow>
        <TableHead>Name</TableHead>
        <TableHead>Size</TableHead>
        <TableHead>Modified</TableHead>
        <TableHead>Status</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {#each files as file (file.name)}
        <TableRow>
          <TableCell class="font-mono">{file.name}</TableCell>
          <TableCell class="font-mono">{file.size}</TableCell>
          <TableCell>{file.modified}</TableCell>
          <TableCell>{file.status}</TableCell>
        </TableRow>
      {/each}
    </TableBody>
    <TableFooter>
      <TableRow>
        <TableCell colspan={3}>Total</TableCell>
        <TableCell class="font-mono">{files.length} files</TableCell>
      </TableRow>
    </TableFooter>
  </Table>
{/snippet}

<Story name="Default" asChild children={fileTable} />

<Story name="Selected Row" asChild>
  <Table>
    <TableHeader>
      <TableRow>
        <TableHead>Name</TableHead>
        <TableHead>Status</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow>
        <TableCell class="font-mono">firmware.bbl</TableCell>
        <TableCell>Ready</TableCell>
      </TableRow>
      <TableRow data-state="selected">
        <TableCell class="font-mono">benchy.3mf</TableCell>
        <TableCell>Printing</TableCell>
      </TableRow>
    </TableBody>
  </Table>
</Story>

<Story
  name="Header Cell Association Is Exposed To Assistive Tech"
  tags={['!dev', '!autodocs']}
  asChild
  children={fileTable}
/>

{#snippet sortableTable(
  direction: 'ascending' | 'descending' | undefined,
  align: 'start' | 'end' = 'start',
)}
  <Table>
    <TableHeader>
      <TableRow>
        <SortableTableHead
          label="Name"
          {direction}
          {align}
          onSort={() => undefined}
        />
        <TableHead>Status</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow>
        <TableCell class="font-mono">firmware.bbl</TableCell>
        <TableCell>Ready</TableCell>
      </TableRow>
    </TableBody>
  </Table>
{/snippet}

<Story name="Sortable Unsorted" asChild>
  {@render sortableTable(undefined)}
</Story>

<Story name="Sortable Ascending" asChild>
  {@render sortableTable('ascending')}
</Story>

<Story name="Sortable Descending" asChild>
  {@render sortableTable('descending')}
</Story>

<Story name="Sortable Aligned To The End" asChild>
  {@render sortableTable('ascending', 'end')}
</Story>

<Story name="Roomy Cells" asChild>
  <Table>
    <TableHeader>
      <TableRow>
        <TableHead>Name</TableHead>
        <TableHead>Status</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow>
        <TableCell roomy class="font-mono">benchy.3mf</TableCell>
        <TableCell roomy>Printing</TableCell>
      </TableRow>
    </TableBody>
  </Table>
</Story>
