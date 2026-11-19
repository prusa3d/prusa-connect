<!--
  A card of access rows with a search and a pager. The printer page lists the
  people on it, and the person page lists the printers; both are this card.
-->
<script lang="ts">
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
  import UsersIcon from '@lucide/svelte/icons/users';

  import { describeFault, t } from '../../i18n';
  import {
    Card,
    CardContent,
    CardDescription,
    CardHeader,
    CardTitle,
    EmptyState,
    FaultAlert,
    ListingNoMatch,
    ListingSearch,
    ListingSkeleton,
    Pager,
    RowList,
  } from '../../primitives';
  import AccessRowView from './access-row.svelte';
  import type { AccessListProps } from './types';

  let {
    title,
    description,
    rows,
    waiting,
    fault,
    search,
    onSearch,
    searchLabel,
    empty,
    pager,
    onOn,
    onChoice,
    instead,
    settingsPanel,
  }: AccessListProps = $props();

  const described = $derived(
    fault === undefined ? undefined : describeFault(fault),
  );
</script>

<Card
  data-part="access-list"
  data-settings-panel={settingsPanel ? true : undefined}
>
  <CardHeader>
    <CardTitle><h2 class="contents">{title}</h2></CardTitle>
    {#if !settingsPanel}
      <CardDescription>{description}</CardDescription>
    {/if}
  </CardHeader>

  <CardContent class="flex flex-col gap-4">
    {#if settingsPanel}
      <CardDescription>{description}</CardDescription>
    {/if}
    {#if described !== undefined}
      <FaultAlert fault={described} />
    {/if}

    {#if instead === undefined && !waiting}
      <ListingSearch
        class="max-w-sm min-w-56"
        value={search}
        onValue={onSearch}
        label={searchLabel}
        placeholder={searchLabel}
      />
    {/if}

    {#if instead !== undefined}
      {@render instead()}
    {:else if waiting}
      <ListingSkeleton label={t('printerAccess.loading')} rows={3} />
    {:else if rows.length === 0 && search.trim() !== ''}
      <ListingNoMatch
        heading={t('printerAccess.noMatch.heading', { search: search.trim() })}
        headingLevel="h3"
        clearLabel={t('primitives.listing.clearSearch')}
        onClear={() => {
          onSearch('');
        }}
      />
    {:else if rows.length === 0}
      <EmptyState
        icon={described === undefined ? UsersIcon : TriangleAlertIcon}
        heading={empty.heading}
        headingLevel="h3"
        description={empty.note}
      />
    {:else}
      <RowList>
        {#each rows as row (row.key)}
          <AccessRowView {row} {onOn} {onChoice} />
        {/each}
      </RowList>
    {/if}

    {#if pager !== undefined && instead === undefined && !waiting}
      <Pager {...pager} />
    {/if}
  </CardContent>
</Card>
