<!-- The API credentials room of settings. -->
<script lang="ts">
  import SortableTableHead from '../../primitives/table/sortable-table-head.svelte';

  import { compareValues } from '../../primitives/table/ordering';

  import KeyRoundIcon from '@lucide/svelte/icons/key-round';
  import PlusIcon from '@lucide/svelte/icons/plus';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';

  import { SvelteSet } from 'svelte/reactivity';

  import {
    type ApiCredential,
    CREDENTIAL_SCOPES,
    type CredentialScope,
  } from '../../data';
  import { describeFault, formatDateTime, formatList, t } from '../../i18n';
  import {
    Badge,
    Button,
    Card,
    CardAction,
    CardContent,
    CardDescription,
    CardHeader,
    CardTitle,
    ConfirmDialog,
    CopyButton,
    DataRow,
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    DropdownMenuItem,
    EmptyState,
    FaultAlert,
    Field,
    Input,
    Label,
    ListingSkeleton,
    ListingState,
    ListingToolbar,
    Pager,
    PasswordInput,
    RowActions,
    RowList,
    Table,
    TableBody,
    TableCaption,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    type ListField,
    viewModes,
    SORT_DIRECTION,
  } from '../../primitives';
  import { browserStore } from '../printers/presentation';
  import ScopeChoice from './scope-choice.svelte';
  import {
    CREDENTIAL_DAYS,
    type CredentialFieldId,
    SCOPE_NAME,
    type SettingsCredentialsScreenProps,
    widestScope,
  } from './types';

  let {
    credentials,
    onCreate,
    onDismiss,
    onRevoke,
    pairHref,
    store = browserStore(),
  }: SettingsCredentialsScreenProps = $props();

  const FIELDS: ListField[] = [
    {
      id: 'name',
      label: t('settingsCredentials.columns.name'),
      required: true,
    },
    { id: 'scopes', label: t('settingsCredentials.columns.scopes') },
    { id: 'expires', label: t('settingsCredentials.columns.expires') },
    { id: 'lastUsed', label: t('settingsCredentials.columns.lastUsed') },
    { id: 'created', label: t('settingsCredentials.columns.created') },
    {
      id: 'actions',
      label: t('settingsCredentials.columns.actions'),
      required: true,
    },
  ];

  let creating = $state(false);
  let name = $state('');
  let days = $state(String(CREDENTIAL_DAYS.initial));
  let scope = $state<CredentialScope>('read');
  let pending = $state<ApiCredential | undefined>(undefined);
  // What the form may say so far. A name is missing only once somebody has
  // asked to create the credential; a day count outside the range is wrong as
  // soon as it is typed, so the message appears as the third digit turns 36
  // into 366.
  let submitted = $state(false);
  let daysTouched = $state(false);

  // The heading names the card.
  const headingId = $props.id();
  const tokenId = `${headingId}-token`;

  const sortValues = {
    name: (row: ApiCredential) => row.name,
    expires: (row: ApiCredential) => Date.parse(row.expires_at),
    lastUsed: (row: ApiCredential) => Date.parse(row.last_used_at ?? ''),
    created: (row: ApiCredential) => Date.parse(row.created_at),
  };

  const listing: ListingState<ApiCredential> = new ListingState<ApiCredential>({
    id: 'settings-credentials',
    fields: FIELDS,
    views: viewModes(['table', 'rows']),
    initialView: 'table',
    store: {
      read: (key) => store.read(key),
      write: (key, value) => store.write(key, value),
    },
    rows: () => credentials.items,
    compare: (a, b) => {
      const sort = listing.sort;
      if (!sort || !Object.hasOwn(sortValues, sort.field)) return 0;
      const value = sortValues[sort.field as keyof typeof sortValues];
      return compareValues(value(a), value(b), sort.order);
    },
  });

  const shows = (field: CredentialFieldId): boolean =>
    listing.fields.visible.includes(field);

  const fault = $derived(
    credentials.error === undefined
      ? undefined
      : describeFault(credentials.error),
  );

  // A read that has not answered and has not refused is still on its way.
  const waiting = $derived(
    credentials.loading || (!credentials.loaded && fault === undefined),
  );

  /** The rows are on screen, which is what the toolbar and the pager act on. */
  const listed = $derived(!waiting && credentials.items.length > 0);

  const publicId = (credential: ApiCredential): string => {
    const peers = credentials.items.filter(
      (other) => other.name === credential.name && other.id !== credential.id,
    );
    let length = 8;
    while (
      length < credential.id.length &&
      peers.some((other) => other.id.endsWith(credential.id.slice(-length)))
    ) {
      length += 1;
    }
    return credential.id.slice(-length);
  };

  const duplicateNames = $derived.by(() => {
    const names = credentials.items.map((credential) => credential.name);
    return new SvelteSet(
      names.filter((name, index) => names.indexOf(name) !== index),
    );
  });

  const showId = (credential: ApiCredential): boolean =>
    duplicateNames.has(credential.name);

  const idLabel = (credential: ApiCredential): string =>
    t('settingsCredentials.publicId', { id: publicId(credential) });

  const describedName = (credential: ApiCredential): string =>
    showId(credential)
      ? `${credential.name} · ${idLabel(credential)}`
      : credential.name;

  const FORTNIGHT_MS = 14 * 24 * 60 * 60 * 1000;

  // ponytail: one badge for the fortnight. A credential the server keeps
  // serving after its date has passed needs a word of its own.
  const expiringSoon = (at: string): boolean =>
    new Date(at).getTime() - Date.now() < FORTNIGHT_MS;

  const lasting = $derived(Number(days));
  const lastingValid = $derived(
    days.trim() !== '' &&
      Number.isInteger(lasting) &&
      lasting >= CREDENTIAL_DAYS.min &&
      lasting <= CREDENTIAL_DAYS.max,
  );

  // A control that refuses silently leaves a reader with a button that does
  // nothing and no reason for it, so each refusal is a sentence under the
  // control it is about.
  //
  // The day count says so as it is typed, and not only when the form is sent.
  // The number control carries min and max, so the browser's own validation
  // refuses to submit a form holding 366 at all and this handler never runs
  // for one. A range stated under the field is the only thing a reader sees
  // before they press, and it is the house voice rather than the browser's
  // bubble.
  const nameError = $derived(
    submitted && name.trim() === ''
      ? t('settingsCredentials.create.nameMissing')
      : '',
  );
  const daysError = $derived(
    (submitted || daysTouched) && !lastingValid
      ? t('settingsCredentials.create.daysRange', {
          min: CREDENTIAL_DAYS.min,
          max: CREDENTIAL_DAYS.max,
        })
      : '',
  );

  // The stacked row reads the same facts as words.
  const scopeWords = (credential: ApiCredential): string =>
    formatList(
      [
        t(SCOPE_NAME[widestScope(credential.scopes)]),
        ...(credential.origin === 'app_pairing'
          ? [t('settingsCredentials.origin.app')]
          : []),
      ],
      'unit',
    );

  // The warning promises the token is gone once the reader leaves, and the
  // store that holds it outlives this screen.
  $effect(() => () => onDismiss());

  /**
   * The panel is left as it was first found. A value typed and abandoned does
   * not wait for the next reader, and neither does the message about it.
   *
   * It runs on the way out, and clears the values together with the messages:
   * a value kept without its message would stand on screen with nothing
   * saying what is wrong with it.
   */
  const resetForm = (): void => {
    name = '';
    days = String(CREDENTIAL_DAYS.initial);
    scope = 'read';
    submitted = false;
    daysTouched = false;
  };

  const create = (event: SubmitEvent): void => {
    event.preventDefault();
    submitted = true;
    if (nameError !== '' || daysError !== '') return;

    onCreate(name.trim(), lasting, scope);
    resetForm();
    creating = false;
  };
</script>

{#snippet sortHead(field: string, label: string)}
  {@const order =
    listing.sort?.field === field ? listing.sort.order : undefined}
  <SortableTableHead
    {label}
    class="font-normal"
    direction={order && SORT_DIRECTION[order]}
    onSort={(order: 'asc' | 'desc' | undefined) => {
      if (order !== undefined) listing.setSort(field, order);
    }}
  />
{/snippet}

{#snippet rowActions(credential: ApiCredential)}
  <RowActions name={describedName(credential)}>
    <DropdownMenuItem
      variant="destructive"
      onSelect={() => {
        pending = credential;
      }}
    >
      <Trash2Icon aria-hidden="true" />
      {t('settingsCredentials.revoke')}
    </DropdownMenuItem>
  </RowActions>
{/snippet}

{#snippet expiringBadge()}
  <Badge variant="warning">{t('settingsCredentials.expiring.badge')}</Badge>
{/snippet}

<!-- What a credential may do, and whether an app received it through a
     pairing rather than a person copying it by hand. -->
{#snippet scopeBadges(credential: ApiCredential)}
  <span class="flex flex-wrap items-center gap-1">
    <Badge variant="outline">
      {t(SCOPE_NAME[widestScope(credential.scopes)])}
    </Badge>
    {#if credential.origin === 'app_pairing'}
      <Badge variant="outline">{t('settingsCredentials.origin.app')}</Badge>
    {/if}
  </span>
{/snippet}

{#snippet keyIcon()}
  <KeyRoundIcon aria-hidden="true" class="size-5 text-muted-foreground" />
{/snippet}

<!-- The column names carry less weight than the values under them. -->
{#snippet columnName(text: string)}
  <span class="font-normal">{text}</span>
{/snippet}

{#snippet table()}
  <Table>
    <TableCaption class="sr-only">
      {t('settingsCredentials.caption')}
    </TableCaption>
    <TableHeader>
      <TableRow>
        {@render sortHead('name', t('settingsCredentials.columns.name'))}
        {#if shows('scopes')}
          <TableHead>
            {@render columnName(t('settingsCredentials.columns.scopes'))}
          </TableHead>
        {/if}
        {#if shows('expires')}
          {@render sortHead(
            'expires',
            t('settingsCredentials.columns.expires'),
          )}
        {/if}
        {#if shows('lastUsed')}
          {@render sortHead(
            'lastUsed',
            t('settingsCredentials.columns.lastUsed'),
          )}
        {/if}
        {#if shows('created')}
          {@render sortHead(
            'created',
            t('settingsCredentials.columns.created'),
          )}
        {/if}
        <TableHead>
          <span class="sr-only">
            {t('settingsCredentials.columns.actions')}
          </span>
        </TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {#each listing.page as credential (credential.id)}
        <TableRow>
          <TableCell class="max-w-64">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <div class="min-w-0">
                <span class="block truncate" title={credential.name}
                  >{credential.name}</span
                >
                {#if showId(credential)}
                  <span
                    class="block truncate font-mono text-xs text-muted-foreground"
                    >{idLabel(credential)}</span
                  >
                {/if}
              </div>
              {#if expiringSoon(credential.expires_at)}
                {@render expiringBadge()}
              {/if}
            </div>
          </TableCell>
          {#if shows('scopes')}
            <TableCell>{@render scopeBadges(credential)}</TableCell>
          {/if}
          {#if shows('expires')}
            <TableCell
              class="font-mono whitespace-nowrap text-muted-foreground tabular-nums"
            >
              {formatDateTime(credential.expires_at)}
            </TableCell>
          {/if}
          {#if shows('lastUsed')}
            <TableCell
              class="font-mono whitespace-nowrap text-muted-foreground tabular-nums"
            >
              {credential.last_used_at === undefined
                ? t('settingsCredentials.lastUsed.never')
                : formatDateTime(credential.last_used_at)}
            </TableCell>
          {/if}
          {#if shows('created')}
            <TableCell
              class="font-mono whitespace-nowrap text-muted-foreground tabular-nums"
            >
              {formatDateTime(credential.created_at)}
            </TableCell>
          {/if}
          <TableCell>{@render rowActions(credential)}</TableCell>
        </TableRow>
      {/each}
    </TableBody>
  </Table>
{/snippet}

{#snippet stacked()}
  <RowList>
    {#each listing.page as credential (credential.id)}
      <DataRow
        primary={credential.name}
        leading={keyIcon}
        wrapValues
        secondaryValues={[
          showId(credential) && idLabel(credential),
          shows('scopes') && scopeWords(credential),
          expiringSoon(credential.expires_at) && expiringBadge,
          shows('expires') &&
            t('settingsCredentials.expires.at', {
              when: formatDateTime(credential.expires_at),
            }),
          shows('lastUsed') &&
            (credential.last_used_at === undefined
              ? t('settingsCredentials.lastUsed.never')
              : t('settingsCredentials.lastUsed.at', {
                  when: formatDateTime(credential.last_used_at),
                })),
          shows('created') &&
            t('settingsCredentials.created.at', {
              when: formatDateTime(credential.created_at),
            }),
        ]}
      >
        {#snippet actions()}
          {@render rowActions(credential)}
        {/snippet}
      </DataRow>
    {/each}
  </RowList>
{/snippet}

<Card data-settings-panel role="region" aria-labelledby={headingId}>
  <CardHeader>
    <CardTitle>
      <h2 id={headingId} class="contents">
        {t('settingsCredentials.title')}
      </h2>
    </CardTitle>
    <CardAction class="flex flex-wrap items-center gap-2">
      <Button
        size="md"
        onclick={() => {
          creating = true;
        }}
      >
        <PlusIcon aria-hidden="true" />
        {t('settingsCredentials.new')}
      </Button>
      <!-- A link drawn as the button, because pairing an app is a page of its own. -->
      <Button variant="secondary" size="md">
        {#snippet child({ props }: { props: Record<string, unknown> })}
          <a {...props} href={pairHref}>
            {t('settingsCredentials.pair')}
          </a>
        {/snippet}
      </Button>
    </CardAction>
  </CardHeader>

  <CardContent class="flex flex-col gap-4">
    <CardDescription>{t('settingsCredentials.note')}</CardDescription>
    {#if listed}
      <ListingToolbar
        fields={FIELDS}
        fieldState={listing.fields}
        view={listing.view}
      />
    {/if}
    {#if fault !== undefined}
      <FaultAlert {fault} />
    {/if}

    {#if waiting}
      <ListingSkeleton label={t('settingsCredentials.loading')} rows={3} />
    {:else if credentials.items.length === 0}
      <EmptyState
        icon={KeyRoundIcon}
        heading={t('settingsCredentials.empty.heading')}
        headingLevel="h3"
        description={t('settingsCredentials.empty.note')}
      />
    {:else}
      <!-- The table needs four columns and the actions beside them. -->
      <div class="@container/credentials">
        {#if listing.view.value === 'table'}
          <div class="hidden @[48rem]/credentials:block">{@render table()}</div>
          <div class="@[48rem]/credentials:hidden">{@render stacked()}</div>
        {:else}
          {@render stacked()}
        {/if}
      </div>

      {#if listing.pager}
        <Pager {...listing.pager} total={listing.total} />
      {/if}
    {/if}
  </CardContent>
</Card>

<!-- Creating one is the occasional act and reading the listing is why.

     Held rather than bound, so that leaving by Escape, by the close button or
     by a press outside all empty the form on the way through. -->
<Dialog
  open={creating}
  onOpenChange={(next: boolean) => {
    creating = next;
    if (!next) resetForm();
  }}
>
  <DialogContent class="max-w-md">
    <DialogHeader>
      <DialogTitle>{t('settingsCredentials.create.title')}</DialogTitle>
      <DialogDescription>
        {t('settingsCredentials.create.note')}
      </DialogDescription>
    </DialogHeader>
    <!-- The browser's own validation is off, and the refusals under the
         fields are the whole of it. `min` and `max` stay on the control for
         assistive technology and for the spinner's steps. With the browser
         enforcing them, a press carrying an out-of-range day count stops in
         the browser's bubble before the handler runs, and a missing name
         beside it is never reported. -->
    <form class="flex flex-col gap-4" novalidate onsubmit={create}>
      <Field
        label={t('settingsCredentials.create.name')}
        error={nameError}
        control={Input}
        controlProps={{
          value: name,
          placeholder: t('settingsCredentials.create.namePlaceholder'),
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            name = event.currentTarget.value;
          },
        }}
      />
      <fieldset class="flex flex-col gap-2">
        <legend class="text-body font-medium">
          {t('settingsCredentials.create.scope')}
        </legend>
        <ScopeChoice
          offered={CREDENTIAL_SCOPES}
          value={scope}
          label={t('settingsCredentials.create.scope')}
          onChange={(next: CredentialScope) => {
            scope = next;
          }}
        />
      </fieldset>
      <Field
        label={t('settingsCredentials.create.days')}
        description={t('settingsCredentials.create.daysNote', {
          min: CREDENTIAL_DAYS.min,
          max: CREDENTIAL_DAYS.max,
        })}
        error={daysError}
        control={Input}
        controlProps={{
          type: 'number',
          min: CREDENTIAL_DAYS.min,
          max: CREDENTIAL_DAYS.max,
          step: 1,
          value: days,
          oninput: (event: Event & { currentTarget: HTMLInputElement }) => {
            days = event.currentTarget.value;
            daysTouched = true;
          },
        }}
      />
      <DialogFooter>
        <!-- Enabled whatever the form holds. A disabled button is the one
             state that cannot say what is wrong with the value, and the
             refusals above are what the press draws. -->
        <Button type="submit" size="lg">
          <PlusIcon aria-hidden="true" />
          {t('settingsCredentials.create')}
        </Button>
      </DialogFooter>
    </form>
  </DialogContent>
</Dialog>

<!-- The token is drawn only while the store holds it, so closing the dialog
     by any route removes it from the page. It starts masked, because the
     screen may be shared. -->
<Dialog
  open={credentials.minted !== undefined}
  onOpenChange={(next: boolean) => {
    if (!next) onDismiss();
  }}
>
  <DialogContent class="max-w-md">
    <DialogHeader>
      <DialogTitle>{t('settingsCredentials.shown.title')}</DialogTitle>
      <DialogDescription>
        {t('settingsCredentials.shown.note')}
      </DialogDescription>
    </DialogHeader>
    {#if credentials.minted !== undefined}
      <div class="flex flex-col gap-2">
        <Label for={tokenId}>{t('settingsCredentials.shown.label')}</Label>
        <div
          class="flex items-start gap-2 *:data-[slot=password-input]:max-w-none *:data-[slot=password-input]:flex-1"
        >
          <PasswordInput
            id={tokenId}
            readonly
            label={t('settingsCredentials.shown.label')}
            value={credentials.minted.token}
            class="font-mono"
          />
          <CopyButton value={credentials.minted.token} size="icon-lg" />
        </div>
      </div>
    {/if}
    <DialogFooter>
      <Button size="lg" onclick={onDismiss}>
        {t('settingsCredentials.shown.dismiss')}
      </Button>
    </DialogFooter>
  </DialogContent>
</Dialog>

<ConfirmDialog
  open={pending !== undefined}
  onOpenChange={(next: boolean) => {
    if (!next) pending = undefined;
  }}
  title={t('settingsCredentials.revoke.title', {
    name: pending ? describedName(pending) : '',
  })}
  description={t('settingsCredentials.revoke.note')}
  confirmLabel={t('settingsCredentials.revoke')}
  onConfirm={() => {
    if (pending !== undefined) onRevoke(pending);
    pending = undefined;
  }}
/>
