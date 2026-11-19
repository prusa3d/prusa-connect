<!-- This screen lists the tags on the printers, and the printers under each. -->
<script lang="ts">
  import { compareValues } from '../../primitives/table/ordering';

  import TagIcon from '@lucide/svelte/icons/tag';

  import { describeFault, t } from '../../i18n';
  import {
    Card,
    CardContent,
    EmptyState,
    FaultAlert,
    ListingSkeleton,
    ListingState,
    ListingToolbar,
    PageHeader,
    Pager,
    type ListField,
    viewModes,
  } from '../../primitives';
  import { browserStore } from '../printers/presentation';
  import { byTag, tagRowsOf } from './presentation';
  import TagsCards from './tags-cards.svelte';
  import TagsTable from './tags-table.svelte';
  import type { TagFieldId, TagRow, TagsScreenProps } from './types';

  let {
    fleet,
    printersHref,
    store = browserStore(),
  }: TagsScreenProps = $props();

  const FIELDS: ListField[] = [
    { id: 'tag', label: t('tags.tag'), required: true },
    { id: 'printers', label: t('tags.printers') },
    { id: 'actions', label: t('tags.actions'), required: true },
  ];

  const rows = $derived(tagRowsOf(fleet.devices));

  const sortValues = {
    tag: (row: TagRow) => row.tag,
    printers: (row: TagRow) => row.printers.length,
  };

  const listing: ListingState<TagRow> = new ListingState<TagRow>({
    id: 'tags',
    fields: FIELDS,
    views: viewModes(['table', 'cards']),
    initialView: 'table',
    store: {
      read: (key) => store.read(key),
      write: (key, value) => {
        store.write(key, value);
      },
    },
    rows: () => rows,
    compare: (a, b) => {
      const sort = listing.sort;
      if (!sort || !Object.hasOwn(sortValues, sort.field)) return byTag(a, b);
      const value = sortValues[sort.field as keyof typeof sortValues];
      return compareValues(value(a), value(b), sort.order);
    },
  });

  const shows = (field: TagFieldId): boolean =>
    listing.fields.visible.includes(field);

  const fault = $derived(
    fleet.error === undefined ? undefined : describeFault(fleet.error),
  );

  const nothingTagged = $derived(fleet.loaded && rows.length === 0);
</script>

{#snippet showPrinters(row: TagRow)}
  <a
    href={printersHref?.(row.tag)}
    aria-label={t('tags.show.for', { tag: row.tag })}
    class="inline-flex min-h-11 items-center rounded-sm text-sm underline underline-offset-4 hover:text-link focus-visible:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
  >
    {t('tags.show')}
  </a>
{/snippet}

<div class="flex flex-col gap-4">
  <PageHeader title={t('tags.title')} />

  <Card>
    <CardContent class="flex flex-col gap-4">
      {#if fault}
        <FaultAlert {fault} />
      {:else if fleet.loading && !fleet.loaded}
        <ListingSkeleton label={t('tags.loading')} rows={4} />
      {:else if nothingTagged}
        <EmptyState
          icon={TagIcon}
          heading={t('tags.empty.heading')}
          headingLevel="h2"
          description={t('tags.empty.note')}
        />
      {:else}
        <ListingToolbar
          class="self-end"
          fields={FIELDS}
          fieldState={listing.fields}
          view={listing.view}
        />

        {#if listing.view.value === 'cards'}
          <TagsCards
            rows={listing.page}
            {shows}
            action={printersHref ? showPrinters : undefined}
          />
        {:else}
          <TagsTable
            {listing}
            rows={listing.page}
            {shows}
            action={printersHref ? showPrinters : undefined}
          />
        {/if}

        {#if listing.pager}
          <Pager {...listing.pager} total={listing.total} />
        {/if}
      {/if}
    </CardContent>
  </Card>
</div>
