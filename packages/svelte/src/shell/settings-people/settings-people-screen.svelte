<!-- Who can sign in to this server, and what each of them may do. -->
<script lang="ts">
  import { compareValues } from '../../primitives/table/ordering';

  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import UsersIcon from '@lucide/svelte/icons/users';

  import type { AccountRoleName, NewAccount } from '../../data';
  import { describeFault, t } from '../../i18n';
  import {
    Button,
    Card,
    CardAction,
    CardContent,
    CardHeader,
    CardTitle,
    ConfirmDialog,
    EmptyState,
    FaultAlert,
    ListingNoMatch,
    ListingSearch,
    ListingSkeleton,
    ListingState,
    ListingToolbar,
    Pager,
    PromptDialog,
    type ListField,
    viewModes,
  } from '../../primitives';
  import AddPersonDialog from './add-person-dialog.svelte';
  import PeopleListing from './people-listing.svelte';
  import {
    browserStore,
    matches,
    personRowsOf,
    roleLabelKey,
  } from './presentation';
  import type {
    PeopleFieldId,
    PersonRow,
    SettingsPeopleScreenProps,
  } from './types';

  /** The core refuses anything shorter, so the field says so before the send. */
  const MINIMUM_PASSWORD = 8;

  let {
    people,
    subject,
    store = browserStore(),
    personHref,
  }: SettingsPeopleScreenProps = $props();

  const FIELDS: ListField[] = [
    { id: 'person', label: t('settingsPeople.username'), required: true },
    { id: 'name', label: t('settingsPeople.name') },
    { id: 'role', label: t('settingsPeople.role') },
    { id: 'added', label: t('settingsPeople.added') },
    { id: 'actions', label: t('settingsPeople.actions'), required: true },
  ];

  let adding = $state(false);
  let removing = $state<PersonRow | undefined>(undefined);
  let resetting = $state<PersonRow | undefined>(undefined);
  let password = $state('');

  let search = $state('');
  let toolbarRegion = $state<HTMLDivElement>();

  const everyone = $derived(personRowsOf(people.items, subject));

  const rows = $derived(
    everyone.filter((row) => matches(search, row.username, row.displayName)),
  );

  const sortValues = {
    person: (row: PersonRow) => row.username,
    name: (row: PersonRow) => row.displayName,
    role: (row: PersonRow) => t(roleLabelKey(row.role)),
    added: (row: PersonRow) => Date.parse(row.createdAt),
  };

  const listing: ListingState<PersonRow> = new ListingState<PersonRow>({
    id: 'settings-people',
    fields: FIELDS,
    views: viewModes(['table', 'rows']),
    initialView: 'table',
    store: {
      read: (key) => store.read(key),
      write: (key, value) => store.write(key, value),
    },
    rows: () => rows,
    compare: (a, b) => {
      const sort = listing.sort;
      if (!sort || !Object.hasOwn(sortValues, sort.field)) return 0;
      const value = sortValues[sort.field as keyof typeof sortValues];
      return compareValues(value(a), value(b), sort.order);
    },
  });

  const shows = (field: PeopleFieldId): boolean =>
    listing.fields.visible.includes(field);

  const isEmpty = $derived(people.loaded && everyone.length === 0);

  const waiting = $derived(people.loading && !people.loaded);

  // A read that never landed has no list to draw the refusal beside.
  const unread = $derived(
    !people.loaded && people.error !== undefined
      ? describeFault(people.error)
      : undefined,
  );
  const refused = $derived(
    people.loaded && people.error !== undefined
      ? describeFault(people.error)
      : undefined,
  );

  const passwordError = $derived(
    password !== '' && password.length < MINIMUM_PASSWORD
      ? t('settingsPeople.add.shortPassword')
      : undefined,
  );

  const create = (account: NewAccount): void => {
    adding = false;
    void people.create(account);
  };

  const changeRole = (row: PersonRow, role: string): void => {
    if (role === row.role) return;
    // The options are the port's own `roles`, so the value is one of them.
    void people.changeRole(row.id, role as AccountRoleName);
  };

  const closeReset = (open: boolean): void => {
    if (open) return;
    resetting = undefined;
    password = '';
  };
</script>

{#snippet addButton()}
  <Button
    size="md"
    onclick={() => {
      adding = true;
    }}
  >
    <PlusIcon aria-hidden="true" />
    {t('settingsPeople.add')}
  </Button>
{/snippet}

<div class="flex flex-col gap-4">
  {#if refused !== undefined}
    <FaultAlert fault={refused} />
  {/if}

  <!-- The listing sits on a surface of its own. -->
  <Card data-settings-panel>
    <CardHeader>
      <CardTitle>
        <h2 class="contents">{t('settingsPeople.title')}</h2>
      </CardTitle>

      {#if !isEmpty && unread === undefined}
        <CardAction>
          {@render addButton()}
        </CardAction>
      {/if}
    </CardHeader>

    <CardContent class="flex flex-col gap-4">
      {#if waiting}
        <ListingSkeleton label={t('settingsPeople.loading')} rows={4} />
      {:else if unread !== undefined}
        <EmptyState
          icon={TriangleAlertIcon}
          heading={unread.title}
          headingLevel="h3"
          description={unread.detail}
        />
      {:else if isEmpty}
        <EmptyState
          icon={UsersIcon}
          heading={t('settingsPeople.empty.heading')}
          headingLevel="h3"
          description={t('settingsPeople.empty.note')}
          action={addButton}
        />
      {:else}
        <div bind:this={toolbarRegion}>
          <ListingToolbar
            fields={FIELDS}
            fieldState={listing.fields}
            view={listing.view}
            search={searchBox}
          />
        </div>

        {#if rows.length === 0}
          <ListingNoMatch
            heading={t('settingsPeople.noMatch.heading', {
              search: search.trim(),
            })}
            headingLevel="h3"
            clearLabel={t('primitives.listing.clearSearch')}
            onClear={() => {
              search = '';
              listing.setPage(0);
              toolbarRegion?.querySelector('input')?.focus();
            }}
          />
        {:else}
          <PeopleListing
            {listing}
            rows={listing.page}
            roles={people.roles}
            view={listing.view.value}
            busy={people.writing}
            {shows}
            onRoleChange={changeRole}
            onPassword={(row: PersonRow) => {
              resetting = row;
            }}
            onRemove={(row: PersonRow) => {
              removing = row;
            }}
            {personHref}
          />

          {#if listing.pager}
            <Pager {...listing.pager} total={listing.total} />
          {/if}
        {/if}
      {/if}
    </CardContent>
  </Card>
</div>

<AddPersonDialog
  open={adding}
  onOpenChange={(open: boolean) => {
    adding = open;
  }}
  roles={people.roles}
  taken={people.items.map((account) => account.username)}
  busy={people.writing}
  onAdd={create}
/>

{#if removing !== undefined}
  <ConfirmDialog
    open
    onOpenChange={(open: boolean) => {
      if (!open) removing = undefined;
    }}
    title={t('settingsPeople.remove.title', { name: removing.username })}
    description={t('settingsPeople.remove.note')}
    confirmLabel={t('settingsPeople.remove.confirm')}
    onConfirm={() => {
      if (removing !== undefined) void people.remove(removing.id);
      removing = undefined;
    }}
  />
{/if}

{#if resetting !== undefined}
  <PromptDialog
    open
    onOpenChange={closeReset}
    title={t('settingsPeople.password.title', { name: resetting.username })}
    description={t('settingsPeople.password.note')}
    label={t('settingsPeople.add.password')}
    type="password"
    autocomplete="new-password"
    value={password}
    onValueChange={(next: string) => {
      password = next;
    }}
    error={passwordError}
    confirmLabel={t('settingsPeople.password.confirm')}
    onConfirm={() => {
      if (resetting !== undefined) {
        void people.changePassword(resetting.id, password);
      }
      closeReset(false);
    }}
  />
{/if}

{#snippet searchBox()}
  <ListingSearch
    value={search}
    onValue={(next: string) => {
      search = next;
    }}
    label={t('settingsPeople.search')}
    placeholder={t('settingsPeople.search')}
  />
{/snippet}
