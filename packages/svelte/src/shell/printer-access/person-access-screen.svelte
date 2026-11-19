<!--
  One person, and their access to every printer. The printers are paged and
  searched by the server, so four hundred printers cost one read per page plus
  one read of this person's grants.
-->
<script lang="ts">
  import UserXIcon from '@lucide/svelte/icons/user-x';
  import { onMount } from 'svelte';

  import type { AccessChoice } from '../../domain';
  import { t } from '../../i18n';
  import {
    Badge,
    Card,
    CardContent,
    DetailHeader,
    EmptyState,
    PageTrail,
    type PageSize,
    type PagerProps,
  } from '../../primitives';
  import AccessList from './access-list.svelte';
  import { displayNameOf, orgRoleLabel, printerRowsOf } from './presentation';
  import type { AccessRow, PersonAccessScreenProps } from './types';

  let {
    account,
    accountsLoaded,
    printers,
    grants,
    self,
    peopleHref,
  }: PersonAccessScreenProps = $props();

  // Read on mount and again when the tab comes back into view, since the
  // server tells no one that a grant changed.
  onMount(() => {
    if (account === undefined) return undefined;
    void grants.load();
    const onVisible = (): void => {
      if (document.visibilityState === 'visible') void grants.load();
    };
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      document.removeEventListener('visibilitychange', onVisible);
    };
  });

  const name = $derived(account === undefined ? '' : displayNameOf(account));

  const rows = $derived(
    account === undefined
      ? []
      : printerRowsOf(printers.items, grants, account, self),
  );

  // The listing's own page size is one the pager offers (PAGE_SIZES).
  const pageSize = $derived(printers.query.pageSize as PageSize);

  const pager = $derived<PagerProps>({
    page: printers.query.page,
    pageCount: Math.max(1, Math.ceil(printers.total / pageSize)),
    pageSize,
    total: printers.total,
    onPageChange: (page: number) => {
      printers.setQuery({ page });
    },
    onPageSizeChange: (next: PageSize) => {
      printers.setQuery({ pageSize: next, page: 0 });
    },
  });
</script>

{#snippet admin()}
  <p class="text-sm text-muted-foreground">
    {t('printerAccess.person.admin', { name })}
  </p>
{/snippet}

{#snippet meta()}
  {#if account !== undefined && name !== account.username}
    <span class="text-sm text-muted-foreground">{name}</span>
  {/if}
{/snippet}

{#snippet titleAside()}
  {#if account !== undefined}
    <Badge variant={account.role === 'admin' ? 'info' : 'outline'}>
      {orgRoleLabel(account.role)}
    </Badge>
  {/if}
{/snippet}

<div class="mx-auto flex w-full max-w-7xl flex-col gap-section-gap">
  <PageTrail
    trail={[{ label: t('printerAccess.person.people'), href: peopleHref }]}
    page={account?.username ?? t('printerAccess.person.missing')}
  />

  {#if account === undefined && accountsLoaded}
    <Card data-settings-panel>
      <CardContent>
        <EmptyState
          icon={UserXIcon}
          heading={t('printerAccess.person.missing')}
          headingLevel="h1"
        />
      </CardContent>
    </Card>
  {:else}
    <DetailHeader
      title={account?.username ?? ''}
      loading={account === undefined}
      {titleAside}
      {meta}
    />

    {#if account !== undefined}
      <AccessList
        settingsPanel
        title={t('printerAccess.person.title')}
        description={t('printerAccess.person.description', { name })}
        {rows}
        waiting={(!printers.loaded || !grants.loaded) &&
          grants.error === undefined}
        fault={grants.error}
        search={printers.query.search}
        onSearch={(next: string) => {
          printers.setQuery({ search: next });
        }}
        searchLabel={t('printerAccess.searchPrinters')}
        empty={{ heading: t('printerAccess.emptyPrinters.heading') }}
        {pager}
        instead={account.role === 'admin' ? admin : undefined}
        onOn={(row: AccessRow, on: boolean) => {
          void grants.setOn(row.deviceId, row.subject, row.orgRole, on);
        }}
        onChoice={(row: AccessRow, choice: AccessChoice) => {
          void grants.setChoice(row.deviceId, row.subject, choice);
        }}
      />
    {/if}
  {/if}
</div>
