<!-- Every printer's prints in one list. -->
<script lang="ts">
  import InboxIcon from '@lucide/svelte/icons/inbox';

  import { PRINTER_STATE_LABEL_KEY, printerStateOf } from '../../domain';
  import { describeFault, t } from '../../i18n';
  import {
    Card,
    CardContent,
    clearFacets,
    EmptyState,
    FaultAlert,
    ListingNoMatch,
    ListingSearch,
    ListingSkeleton,
    ListingState,
    ListingToolbar,
    PageHeader,
    Pager,
    type FacetDimension,
    type ListField,
    viewModes,
  } from '../../primitives';
  import { PrintDialog, type PrintableFile } from '../print-picker';
  import { browserStore } from '../printers/presentation';
  import JobsRows from './jobs-rows.svelte';
  import JobsTable from './jobs-table.svelte';
  import {
    compareJobs,
    DEFAULT_SORT,
    filterJobs,
    type JobFacet,
    jobFacetCounts,
    jobFacetValues,
    jobRowsOf,
    NO_JOBS_SELECTED,
    searchTerm,
  } from './presentation';
  import type {
    JobFacetSelection,
    JobFieldId,
    JobRow,
    JobSort,
    JobSortColumn,
    JobsScreenProps,
  } from './types';

  let {
    jobs,
    store = browserStore(),
    devices = [],
    transitions,
    locked,
    onPrint,
    onPrinted,
    thumbnailFor,
    notice,
  }: JobsScreenProps = $props();

  // The print whose file the dialog sends again, while the dialog is open.
  let printing = $state<JobRow | undefined>(undefined);

  // Only the printer that ran the print holds its file.
  const printer = $derived(
    devices.find((device) => device.id === printing?.printerId),
  );

  const again = $derived<PrintableFile | undefined>(
    printing === undefined
      ? undefined
      : { name: printing.fileName, source: printing.path },
  );

  // A running print is not started again, and a printer this page does not
  // know has nowhere to send the file.
  const printAgainOf = $derived(
    onPrint === undefined
      ? undefined
      : (row: JobRow) =>
          row.running || !devices.some((device) => device.id === row.printerId)
            ? undefined
            : () => {
                printing = row;
              },
  );

  const FIELDS: ListField[] = [
    { id: 'preview', label: t('jobs.preview') },
    { id: 'file', label: t('jobs.file'), required: true },
    { id: 'printer', label: t('jobs.printer') },
    { id: 'state', label: t('jobs.state') },
    { id: 'started', label: t('jobs.started') },
    { id: 'duration', label: t('jobs.duration') },
    { id: 'progress', label: t('jobs.progress') },
  ];

  let query = $state('');
  let selection = $state<JobFacetSelection>(NO_JOBS_SELECTED);
  let sort = $state<JobSort>(DEFAULT_SORT);

  const all = $derived(jobRowsOf(jobs));
  const term = $derived(searchTerm(query));
  const matching = $derived(filterJobs(all, selection, term));
  const refusal = $derived(
    jobs.error === undefined ? undefined : describeFault(jobs.error),
  );

  const listing = new ListingState<JobRow>({
    id: 'jobs',
    fields: FIELDS,
    views: viewModes(['table', 'rows']),
    initialView: 'table',
    store: {
      read: (key) => store.read(key),
      write: (key, value) => {
        store.write(key, value);
      },
    },
    rows: () => matching,
    compare: (a, b) => compareJobs(a, b, sort),
  });

  const nameOf = (id: string): string =>
    jobs.printers.find((printer) => printer.id === id)?.name ?? id;

  const dimension = (
    id: JobFacet,
    label: string,
    values: string[],
    labelFor: (value: string) => string,
  ): FacetDimension => ({
    id,
    label,
    values,
    counts: jobFacetCounts(all, selection, term, id, values),
    selected: selection[id],
    onSelectedChange: (next: string[]) => {
      selection = { ...selection, [id]: next };
    },
    labelFor,
  });

  // Counting reads the search term.
  $effect(() => {
    listing.setFacets([
      dimension(
        'printer',
        t('jobs.printer'),
        jobs.printers.map((printer) => printer.id),
        nameOf,
      ),
      dimension(
        'state',
        t('jobs.state'),
        jobFacetValues(all, 'state'),
        (value) => t(PRINTER_STATE_LABEL_KEY[printerStateOf(value)]),
      ),
    ]);
  });

  const shows = (field: JobFieldId): boolean =>
    listing.fields.visible.includes(field);

  const onSearch = (next: string): void => {
    query = next;
    listing.setPage(0);
  };

  const clearFilters = (): void => {
    onSearch('');
    clearFacets(listing.facets);
  };

  const onSort = (column: JobSortColumn, order: 'asc' | 'desc'): void => {
    sort = { column, ascending: order === 'asc' };
    listing.setPage(0);
  };

  const nothingPrintedYet = $derived(jobs.loaded && all.length === 0);
</script>

{#snippet toolbar()}
  <ListingToolbar
    filtersActive={term !== '' ||
      selection.printer.length > 0 ||
      selection.state.length > 0}
    onClearFilters={clearFilters}
    facets={listing.facets}
    fields={FIELDS}
    fieldState={listing.fields}
    view={listing.view}
  >
    {#snippet search()}
      <ListingSearch
        value={query}
        onValue={onSearch}
        label={t('jobs.search.label')}
        placeholder={t('jobs.search.placeholder')}
      />
    {/snippet}
  </ListingToolbar>
{/snippet}

<div class="flex flex-col gap-4">
  <PageHeader title={t('jobs.title')} />
  {@render notice?.()}

  {#if refusal}
    <FaultAlert fault={refusal} />
  {:else if jobs.loading && !jobs.loaded}
    <!-- As tall as the window: the list's rows are taller than the
         skeleton's, and a shorter card leaves the footer on screen for the
         arriving list to push down. -->
    <Card density="panel" class="min-h-svh">
      <CardContent
        class="flex flex-col gap-section-inline pt-section-block pb-panel-padding-mobile @lg/card:pb-panel-padding"
      >
        {@render toolbar()}
        <ListingSkeleton label={t('jobs.loading')} art rows={5} />
      </CardContent>
    </Card>
  {:else if nothingPrintedYet}
    <!-- No toolbar: there is nothing to filter, sort or reshape yet. -->
    <Card density="panel">
      <CardContent
        class="pt-section-block pb-panel-padding-mobile @lg/card:pb-panel-padding"
      >
        <EmptyState
          icon={InboxIcon}
          heading={t('jobs.empty.heading')}
          headingLevel="h2"
          description={t('jobs.empty.note')}
        />
      </CardContent>
    </Card>
  {:else}
    <Card density="panel">
      <CardContent
        class="flex flex-col gap-section-inline pt-section-block pb-panel-padding-mobile @lg/card:pb-panel-padding"
      >
        {@render toolbar()}

        {#if listing.total === 0}
          <ListingNoMatch
            heading={t('jobs.filtered.heading')}
            headingLevel="h2"
            description={t('jobs.filtered.note')}
            onClear={clearFilters}
          />
        {:else}
          {#if listing.view.value === 'rows'}
            <JobsRows
              rows={listing.page}
              {shows}
              {printAgainOf}
              {thumbnailFor}
            />
          {:else}
            <JobsTable
              rows={listing.page}
              {shows}
              {sort}
              {onSort}
              {printAgainOf}
              {thumbnailFor}
            />
          {/if}

          {#if listing.pager}
            <!-- The noun is a plural message and not the word `jobs`. -->
            <Pager
              {...listing.pager}
              total={listing.total}
              unit={t('jobs.unit', { count: listing.total })}
            />
          {/if}
        {/if}
      </CardContent>
    </Card>
  {/if}
</div>

{#if onPrint}
  <PrintDialog
    open={printing !== undefined}
    onOpenChange={(next: boolean) => {
      if (!next) printing = undefined;
    }}
    file={again}
    devices={printer === undefined ? [] : [printer]}
    {transitions}
    {locked}
    {onPrint}
    {onPrinted}
  />
{/if}
