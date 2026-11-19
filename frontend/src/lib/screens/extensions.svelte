<!--
  The Extensions room inside Settings: the marketplace this server reads, and
  every module it has installed or can install, in one listing.

  A refusal is state and never a console line. "It is installed and it is not
  running" is the first question an operator asks, so the reason is on the
  screen with its code beside it, in the language the person reads.

  The listing joins two reads. The index is what can be installed, from this
  server's own index and from the marketplace core.marketplace.url names
  (ADR-20261005-64j); the resources listing is what is installed. A reader searches
  it and narrows it by where a module comes from and what state it is in, the
  way the other listings narrow theirs. The install itself runs on the
  server, which downloads the release and checks its digest, so this page
  never fetches a module.

  Below the listing, the databases extensions left behind are listed with a
  purge each (ADR-20261021-bqc), and the section is absent while there are none.

  The marketplace sits in the head of the listing as one word, and its
  settings open from there: closed by default, because a person comes here
  for the modules and the marketplace is on unless they switched it off.

  A community extension is one the marketplace marks trusted or unverified,
  from an author outside Prusa Research. It is not listed until an
  organization administrator switches community extensions on in those
  settings. The page says how many it leaves out. An installed one is always
  listed, so it can still be updated and removed.

  The marketplace links here as ?install=<module>&version=<version>. The page
  narrows the listing to that module, marks its row and asks the install
  question for that release, once: the query is taken off the address so a
  reload does not ask again. The server still resolves the release and its
  digest itself. A person the server does not let install is told so instead,
  and so is a person who sees nothing they may install or remove.

  A module's settings panes open on its own page, one tab each.

  While the operator switches installed modules off (ADR-20261019-h76 rule 15), the
  page offers nothing to install, and an installed module keeps only Remove.
-->
<script lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import PackageIcon from '@lucide/svelte/icons/package';
  import Settings2Icon from '@lucide/svelte/icons/settings-2';
  import ShieldAlertIcon from '@lucide/svelte/icons/shield-alert';
  import { SettingsScreen } from '@prusa/connect-ui/shell';
  import {
    describeFault,
    formatDateTime,
    translateFor,
  } from '@prusa/connect-ui/i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Button,
    Card,
    CardAction,
    CardContent,
    CardHeader,
    CardTitle,
    clearFacets,
    EmptyState,
    type FacetDimension,
    FaultAlert,
    type ListField,
    ListingNoMatch,
    ListingSearch,
    ListingSkeleton,
    ListingState,
    ListingToolbar,
    Label,
    Switch,
    Pager,
    viewModes,
  } from '@prusa/connect-ui/primitives';
  import { describeRefusal, getSlotContext } from '@prusa/connect-ui/slots';
  import { onMount, tick } from 'svelte';

  import {
    type Asking,
    type AskingAct,
    ExtensionDialog,
  } from '../extensions/extension-dialog';
  import { ExtensionsListing } from '../extensions/extensions-listing';
  import { describeExtensionFault } from '../extensions/fault';
  import { InstallOutcome } from '../extensions/install-outcome';
  import { LeftDatabases } from '../extensions/left-databases';
  import { browserStore } from '../extensions/listing-store';
  import { poolBudgetsOf } from '../extensions/memory';
  import { MemoryPool } from '../extensions/memory-pool';
  import { MarketplaceBadge } from '../extensions/marketplace-badge';
  import { MarketplaceSettings } from '../extensions/marketplace-settings';
  import {
    compareExtensions,
    type ExtensionFacet,
    extensionFacetCounts,
    type ExtensionRow,
    extensionRowsOf,
    type ExtensionSelection,
    filterExtensions,
    hidesCommunity,
    NO_EXTENSIONS_SELECTED,
    searchTerm,
    SOURCES,
    SOURCE_KEY,
    STATES,
    TRUSTS,
  } from '../extensions/rows';
  import { OFFICIAL } from '../extensions/trust';
  import { SwitchedOffAlerts } from '../extensions/switched-off-alerts';
  import { compareVersions } from '../semver';
  import { removableOf } from '../extensions/switch';
  import type { ExtensionFieldId } from '../extensions/types';
  import { APP_NAMESPACE } from '../i18n';
  import { extensionDetailHref, PATHS } from '../routes';
  import type { AppContext } from '../screens';

  const t = translateFor(APP_NAMESPACE);

  let { ctx }: { ctx: AppContext } = $props();

  const context = getSlotContext();
  const refusals = $derived(context?.registry.refusals() ?? []);

  const catalogue = $derived(ctx.moduleCatalogue);

  const FIELDS: ListField[] = [
    {
      id: 'module',
      label: t('app.settings.extensions.column.module'),
      required: true,
    },
    { id: 'author', label: t('app.settings.extensions.column.author') },
    { id: 'version', label: t('app.settings.extensions.column.version') },
    { id: 'source', label: t('app.settings.extensions.column.source') },
    { id: 'state', label: t('app.settings.extensions.column.state') },
    {
      id: 'memory',
      label: t('app.settings.extensions.column.memory'),
      hiddenByDefault: true,
    },
    {
      id: 'actions',
      label: t('app.settings.extensions.column.actions'),
      required: true,
    },
  ];

  let query = $state('');
  let settingsOpen = $state(false);
  let selection = $state<ExtensionSelection>(NO_EXTENSIONS_SELECTED);
  let asking = $state<Asking | undefined>(undefined);
  /** The row a link pointed at. */
  let highlighted = $state<string | undefined>(undefined);

  /** What became of a link that asked to install a module. */
  type LinkNotice =
    | {
        kind: 'refused' | 'missing' | 'installed' | 'modulesOff' | 'community';
        module: string;
      }
    | { kind: 'localOnly'; module: string; version: string; latest: string };
  let linkNotice = $state<LinkNotice | undefined>(undefined);

  const all = $derived(
    extensionRowsOf(catalogue.items, ctx.modules.items, ctx.modules.pages),
  );
  // The rows the listing draws: every row but the community ones nobody
  // chose to list.
  const communityShown = $derived(catalogue.marketplace?.community ?? false);
  const listed = $derived(
    all.filter((row) => !hidesCommunity(row, communityShown)),
  );
  const hiddenCommunity = $derived(all.length - listed.length);
  // Official is always a value to narrow by. Trusted and unverified are while
  // community extensions are listed, or while a listed row carries them.
  const trusts = $derived(
    TRUSTS.filter(
      (value) =>
        communityShown ||
        value === OFFICIAL ||
        listed.some((row) => row.trust === value),
    ),
  );
  const hasMemory = $derived(
    all.some((row) => row.instance?.status.memory !== undefined),
  );
  const shownFields = $derived(
    hasMemory ? FIELDS : FIELDS.filter(({ id }) => id !== 'memory'),
  );
  const term = $derived(searchTerm(query));
  const matching = $derived(filterExtensions(listed, selection, term));

  const store = browserStore();
  const listing: ListingState<ExtensionRow> = new ListingState<ExtensionRow>({
    id: 'settings-extension-modules',
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
    compare: (a: ExtensionRow, b: ExtensionRow): number => {
      if (listing.sort?.field === 'author') {
        if (a.author === '' || b.author === '') {
          return Number(a.author === '') - Number(b.author === '');
        }
        const order = listing.sort.order === 'asc' ? 1 : -1;
        return (
          order *
            a.author.localeCompare(b.author, undefined, {
              sensitivity: 'base',
              numeric: true,
            }) ||
          a.title.localeCompare(b.title, undefined, {
            sensitivity: 'base',
            numeric: true,
          })
        );
      }
      if (listing.sort?.field === 'title') {
        const order = listing.sort.order === 'asc' ? 1 : -1;
        return (
          order *
          a.title.localeCompare(b.title, undefined, {
            sensitivity: 'base',
            numeric: true,
          })
        );
      }
      if (listing.sort) {
        const { field, order } = listing.sort;
        const direction = order === 'asc' ? 1 : -1;
        if (field === 'version') {
          const versionOf = (row: ExtensionRow): string =>
            [
              row.installedVersion,
              row.instance?.module?.version,
              row.version,
            ].find((version) => Boolean(version)) ?? '';
          const left = versionOf(a);
          const right = versionOf(b);
          if (!left || !right) return Number(!left) - Number(!right);
          return (
            direction *
            (compareVersions(left, right) ??
              left.localeCompare(right, undefined, { numeric: true }))
          );
        }
        if (field === 'source') {
          return (
            direction *
            t(SOURCE_KEY[a.source]).localeCompare(t(SOURCE_KEY[b.source]))
          );
        }
        if (field === 'state') {
          return (
            direction * STATES.indexOf(a.state) -
            direction * STATES.indexOf(b.state)
          );
        }
      }
      return compareExtensions(a, b);
    },
  });

  // Every key is written out rather than assembled from a value.
  const VALUE_KEY: Readonly<Record<string, string>> = {
    local: 'app.settings.extensions.source.local',
    marketplace: 'app.settings.extensions.source.marketplace',
    unlisted: 'app.settings.extensions.source.unlisted',
    available: 'app.settings.extensions.offer.state.available',
    installed: 'app.settings.extensions.offer.state.installed',
    official: 'app.settings.extensions.offer.trust.official',
    verified: 'app.settings.extensions.offer.trust.verified',
    community: 'app.settings.extensions.offer.trust.community',
    update: 'app.settings.extensions.offer.state.update',
  };

  const labelFor = (value: string): string => {
    const key = VALUE_KEY[value];

    return key === undefined ? value : t(key);
  };

  const dimension = (
    id: ExtensionFacet,
    label: string,
    values: readonly string[],
  ): FacetDimension => ({
    id,
    label,
    values: [...values],
    counts: extensionFacetCounts(listed, selection, term, id, values),
    selected: selection[id],
    onSelectedChange: (next: string[]) => {
      selection = { ...selection, [id]: next };
      listing.setPage(0);
    },
    labelFor,
  });

  // Counting reads the search term and the other dimensions' choices.
  $effect(() => {
    listing.setFacets([
      dimension('source', t('app.settings.extensions.facet.source'), SOURCES),
      dimension('trust', t('app.settings.extensions.facet.trust'), trusts),
      dimension('state', t('app.settings.extensions.facet.state'), STATES),
    ]);
  });

  // What the enabled service modules promise, as the listing names them.
  const budgets = $derived(poolBudgetsOf(all));
  const poolDocs = $derived(
    ctx.settings.documentation === undefined
      ? undefined
      : `${ctx.settings.documentation}config/extensions/#key-core-modules-memory`,
  );

  const shows = (field: ExtensionFieldId): boolean =>
    (field !== 'memory' || hasMemory) && listing.fields.visible.includes(field);

  const onSearch = (next: string): void => {
    query = next;
    listing.setPage(0);
  };

  const narrowed = $derived(
    term !== '' || Object.values(selection).some((values) => values.length > 0),
  );

  const clearFilters = (): void => {
    onSearch('');
    clearFacets(listing.facets);
  };

  // An index that could not be read is said above the listing, and the
  // modules this server has installed are still listed: what runs here must
  // stay removable whether or not the index answers. A person who may not
  // manage modules is told that instead of the refusal it caused.
  const readOnly = $derived(catalogue.mayManage === false);
  // Whether a module this server installed is listed, so the page does not
  // say one can be removed when every row comes from config.yaml.
  const removable = $derived(
    all.some(
      (row) =>
        row.instance !== undefined &&
        removableOf(row, catalogue.mayConfigure(row.kind)),
    ),
  );
  // Whether a module this server installed is listed, whoever may remove it.
  const installed = $derived(
    all.some(
      (row) => row.instance !== undefined && row.instance.source !== 'file',
    ),
  );
  // What the page says at its top while modules are off. A person who may not
  // manage modules is not told about removing or switching them on again.
  const offNotice = $derived.by((): string => {
    if (catalogue.safeMode) return t('app.settings.extensions.notice.safeMode');
    if (readOnly) return t('app.settings.extensions.notice.modulesOffReadOnly');

    return removable
      ? t('app.settings.extensions.notice.modulesOff')
      : t('app.settings.extensions.notice.modulesOffNothingToRemove');
  });
  const refused = $derived(
    catalogue.error === undefined || readOnly
      ? undefined
      : describeFault(catalogue.error),
  );
  const waiting = $derived(
    !catalogue.loaded && catalogue.loading && all.length === 0,
  );
  const nothing = $derived(
    catalogue.loaded && refused === undefined && all.length === 0,
  );
  // Said while community rows are left out, and not while a link already
  // says why the one it named is not listed.
  const hiddenNote = $derived(
    hiddenCommunity > 0 && linkNotice?.kind !== 'community'
      ? t(
          catalogue.mayAdministerOrg === false
            ? 'app.settings.extensions.community.hiddenAsk'
            : 'app.settings.extensions.community.hidden',
          { count: hiddenCommunity },
        )
      : undefined,
  );

  // Why the listing is empty. "Modules are off", "the marketplace is off",
  // "it never answered" and "it has nothing for this server" are different
  // facts.
  const emptyNote = $derived.by((): string => {
    if (!catalogue.modulesOn) {
      if (catalogue.safeMode)
        return t('app.settings.extensions.empty.safeMode');

      return readOnly
        ? t('app.settings.extensions.empty.modulesOffReadOnly')
        : t('app.settings.extensions.empty.modulesOff');
    }

    const marketplace = catalogue.marketplace;
    if (marketplace === undefined) {
      return t('app.settings.extensions.empty.configOff');
    }
    if (!marketplace.enabled) {
      return catalogue.mayAdministerOrg === false
        ? t('app.settings.extensions.empty.switchedOffAsk')
        : t('app.settings.extensions.empty.switchedOff');
    }
    if (marketplace.reachable) {
      return t('app.settings.extensions.empty.marketplace');
    }

    return marketplace.stale && marketplace.read_at !== undefined
      ? t('app.settings.extensions.empty.stale', {
          when: formatDateTime(marketplace.read_at),
        })
      : t('app.settings.extensions.empty.unreachable');
  });
  const emptyProblem = $derived(
    catalogue.marketplace?.enabled && !catalogue.marketplace.reachable
      ? (catalogue.marketplace.problem ?? '')
      : '',
  );

  // A refused install, update or removal names the module it was for, so the
  // reader knows which row the code belongs to.
  const actionFault = $derived(
    catalogue.actionError === undefined
      ? undefined
      : describeExtensionFault(catalogue.actionError, t),
  );
  // A refused switch write names its module too. A stale version is told
  // apart from other refusals, because the listing already shows what is current.
  // A refused settings save belongs to the module's own page, not here.
  const switchWrite = $derived(catalogue.writeFor === 'switch');
  const writeFault = $derived(
    !switchWrite || catalogue.writeError === undefined
      ? undefined
      : describeExtensionFault(catalogue.writeError, t),
  );
  const titleOf = (module: string | undefined): string =>
    all.find((row) => row.module === module)?.title ?? module ?? '';
  const actionTitle = $derived(titleOf(catalogue.actionModule));
  const writeTitle = $derived(
    all.find((row) => row.namespace === catalogue.writeModule)?.title ??
      catalogue.writeModule ??
      '',
  );
  const outcomeTitle = $derived(titleOf(catalogue.outcome?.offer.module));

  const ask = (row: ExtensionRow, act: AskingAct, version?: string): void => {
    asking = { row, act, version };
  };

  // Takes the link's query once the index and the person's access are read.
  const followLink = async (): Promise<void> => {
    const params = new URLSearchParams(ctx.search);
    const module = params.get('install')?.trim() ?? '';
    if (module === '') return;
    const version = params.get('version')?.trim() ?? '';

    await ctx.moduleCatalogue.load();
    ctx.go(PATHS.settingsExtensions, { replace: true });

    if (!catalogue.modulesOn) {
      linkNotice = { kind: 'modulesOff', module };
      return;
    }

    if (catalogue.mayManage === false) {
      linkNotice = { kind: 'refused', module };
      return;
    }

    const row = all.find(
      (one) => one.module === module && one.offer !== undefined,
    );
    if (row === undefined) {
      linkNotice = { kind: 'missing', module };
      return;
    }

    // An unverified extension is installed only once community extensions
    // are listed, so the link opens the settings that list them.
    if (hidesCommunity(row, communityShown)) {
      linkNotice = { kind: 'community', module };
      settingsOpen = catalogue.mayAdministerOrg !== false;
      return;
    }

    // Narrowed to the one module, so its row is on the first page whatever
    // the reader had chosen before.
    query = row.module;
    selection = NO_EXTENSIONS_SELECTED;
    listing.setPage(0);
    highlighted = row.key;

    const wanted = version === '' ? row.version : version;
    if (row.source !== 'marketplace' && wanted !== row.version) {
      // A local row names the address of one release only.
      linkNotice = {
        kind: 'localOnly',
        module,
        version: wanted,
        latest: row.version,
      };
    } else if (row.state !== 'available' && wanted === row.installedVersion) {
      linkNotice = { kind: 'installed', module };
    } else {
      asking = {
        row,
        act: row.state === 'available' ? 'install' : 'update',
        version: wanted,
      };
    }

    await tick();
    document
      .querySelector('[data-highlighted]')
      ?.scrollIntoView({ block: 'center' });
  };

  const linkText = $derived.by((): string => {
    switch (linkNotice?.kind) {
      case undefined:
        return '';
      case 'refused':
        return t('app.settings.extensions.link.refused', {
          module: linkNotice.module,
        });
      case 'missing':
        return t('app.settings.extensions.link.missing', {
          module: linkNotice.module,
        });
      case 'modulesOff':
        return t(
          catalogue.safeMode
            ? 'app.settings.extensions.link.safeMode'
            : 'app.settings.extensions.link.modulesOff',
          { module: linkNotice.module },
        );
      case 'installed':
        return t('app.settings.extensions.link.installed', {
          name: titleOf(linkNotice.module),
        });
      case 'community':
        return t(
          catalogue.mayAdministerOrg === false
            ? 'app.settings.extensions.link.communityAsk'
            : 'app.settings.extensions.link.community',
          { name: titleOf(linkNotice.module) },
        );
      case 'localOnly':
        return t('app.settings.extensions.link.localOnly', {
          name: titleOf(linkNotice.module),
          version: linkNotice.version,
          latest: linkNotice.latest,
        });
    }
  });

  onMount(() => {
    void ctx.moduleCatalogue.load();
    void ctx.moduleCatalogue.readMemory();
    // A refusal is recorded only once the loader runs, and this page may be
    // the first one opened.
    void context?.loader?.activate();
    void followLink();

    // A module that started after the listing loaded reports its memory only
    // once it runs, and only a state change re-reads the listing.
    const timer = setInterval(() => {
      void ctx.modules.load();
      void ctx.moduleCatalogue.readMemory();
    }, 5000);

    return () => {
      clearInterval(timer);
    };
  });

  const go = (href: string): void => {
    ctx.go(href);
  };

  const refusedId = $props.id();
  const settingsId = `${refusedId}-marketplace`;
  const marketplaceSwitchId = `${refusedId}-marketplace-header-switch`;
</script>

{#snippet content()}
  {#if catalogue.memoryPool?.pool_bytes !== undefined || (catalogue.memoryPool?.committed_bytes ?? 0) > 0}
    <MemoryPool pool={catalogue.memoryPool} {budgets} configHref={poolDocs} />
  {/if}

  <SwitchedOffAlerts
    rows={all}
    href={(row: ExtensionRow) => extensionDetailHref(row.namespace ?? '')}
  />

  {#if refusals.length > 0}
    <section class="flex flex-col gap-2" aria-labelledby={refusedId}>
      <h2 id={refusedId} class="text-sm font-semibold">
        {t('app.settings.extensions.refused')}
      </h2>
      {#each refusals as refusal (refusal.namespace + refusal.reason)}
        <Alert variant="warning" data-module={refusal.namespace}>
          <AlertTitle>{refusal.namespace}</AlertTitle>
          <AlertDescription>
            {describeRefusal(refusal)}
            <code class="block text-xs">{refusal.reason}</code>
          </AlertDescription>
        </Alert>
      {/each}
    </section>
  {/if}

  <Card data-settings-panel data-part="catalogue" aria-busy={catalogue.loading}>
    <CardHeader>
      <CardTitle>
        <h2 class="contents">
          {catalogue.modulesOn
            ? t('app.settings.extensions.list.title')
            : t('app.settings.extensions.list.titleModulesOff')}
        </h2>
      </CardTitle>

      {#if catalogue.loaded && catalogue.modulesOn}
        <CardAction class="flex flex-wrap items-center gap-2">
          {#if catalogue.marketplace !== undefined}
            <div
              class="flex items-center gap-2"
              title={catalogue.mayAdministerOrg === false
                ? t('app.settings.extensions.marketplace.locked')
                : undefined}
            >
              <Switch
                id={marketplaceSwitchId}
                aria-label={t('app.settings.extensions.source.marketplace')}
                bind:checked={
                  () => catalogue.marketplace?.enabled ?? false,
                  (enabled: boolean) => {
                    void catalogue.setMarketplace(enabled);
                  }
                }
                disabled={catalogue.mayAdministerOrg === false ||
                  catalogue.switching ||
                  catalogue.acting !== undefined}
                data-part="marketplace-header-switch"
              />
              <Label for={marketplaceSwitchId} class="text-sm font-normal">
                {t('app.settings.extensions.source.marketplace')}
              </Label>
            </div>
            {#if catalogue.marketplace.enabled && !catalogue.marketplace.reachable}
              <MarketplaceBadge marketplace={catalogue.marketplace} />
            {/if}
          {:else}
            <MarketplaceBadge marketplace={catalogue.marketplace} />
          {/if}
          <Button
            variant="ghost"
            size="control"
            aria-expanded={settingsOpen}
            aria-controls={settingsId}
            data-part="marketplace-settings"
            onclick={() => {
              settingsOpen = !settingsOpen;
            }}
          >
            <Settings2Icon aria-hidden="true" class="size-6" />
            {t('app.settings.extensions.marketplace.settings')}
            <ChevronDownIcon
              aria-hidden="true"
              class={[
                'size-6 transition-transform motion-reduce:transition-none',
                settingsOpen && 'rotate-180',
              ]}
            />
          </Button>
        </CardAction>
      {/if}
    </CardHeader>

    <CardContent class="flex flex-col gap-4">
      {#if !settingsOpen && catalogue.switchError !== undefined}
        <FaultAlert fault={describeFault(catalogue.switchError)} />
      {/if}
      {#if !catalogue.modulesOn}
        <!-- An empty listing says it in its own note. A person who may not
             manage modules is not told that one can be removed. -->
        {#if !nothing}
          <Alert variant="info" data-part="modules-off">
            <AlertDescription>
              {offNotice}
            </AlertDescription>
          </Alert>
        {/if}
      {:else if settingsOpen}
        <MarketplaceSettings {catalogue} id={settingsId} />
      {/if}

      {#if linkNotice !== undefined}
        <Alert
          variant={linkNotice.kind === 'installed' ? 'info' : 'warning'}
          data-part="link-notice"
          data-kind={linkNotice.kind}
        >
          <AlertDescription>{linkText}</AlertDescription>
        </Alert>
      {/if}

      <!-- With modules off, a viewer is told about removing only when an
           installed module is listed: an empty page has its own sentence,
           and a module config.yaml declares cannot be removed here. -->
      {#if readOnly && (catalogue.modulesOn || installed)}
        <Alert variant="info" data-part="read-only">
          <AlertDescription>
            {catalogue.modulesOn
              ? t('app.settings.extensions.access.readOnly')
              : t('app.settings.extensions.access.readOnlyModulesOff')}
          </AlertDescription>
        </Alert>
      {/if}

      {#if refused !== undefined}
        <FaultAlert fault={refused} />
      {/if}

      <InstallOutcome {catalogue} name={outcomeTitle} />

      {#if actionFault !== undefined}
        <div class="flex flex-col gap-1" data-part="action-fault">
          <p class="text-sm font-medium">{actionTitle}</p>
          <FaultAlert fault={actionFault} />
        </div>
      {/if}

      {#if (switchWrite && catalogue.writeConflict) || writeFault !== undefined}
        <div class="flex flex-col gap-1" data-part="write-fault">
          <p class="text-sm font-medium">{writeTitle}</p>
          {#if writeFault === undefined}
            <Alert variant="warning">
              <AlertDescription>
                {t('app.settings.extensions.write.conflict')}
              </AlertDescription>
            </Alert>
          {:else}
            <FaultAlert fault={writeFault} />
          {/if}
        </div>
      {/if}

      {#if hiddenNote !== undefined}
        <Alert variant="info" data-part="community-hidden">
          <ShieldAlertIcon aria-hidden="true" />
          <AlertDescription class="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span>{hiddenNote}</span>
            {#if catalogue.mayAdministerOrg !== false && !settingsOpen}
              <Button
                variant="link"
                size="sm"
                class="h-auto p-0"
                aria-controls={settingsId}
                data-part="community-settings"
                onclick={() => {
                  settingsOpen = true;
                }}
              >
                {t('app.settings.extensions.community.settings.label')}
              </Button>
            {/if}
          </AlertDescription>
        </Alert>
      {/if}

      {#if waiting}
        <ListingSkeleton
          label={t('app.settings.extensions.catalogue.loading')}
          rows={3}
        />
      {:else if nothing}
        <EmptyState
          icon={PackageIcon}
          heading={catalogue.modulesOn
            ? t('app.settings.extensions.empty.heading')
            : t('app.settings.extensions.empty.headingModulesOff')}
          headingLevel="h3"
          description={emptyNote}
          data-part="empty"
        />
        {#if emptyProblem !== ''}
          <p
            class="text-center text-sm text-muted-foreground"
            data-part="empty-problem"
          >
            {emptyProblem}
          </p>
        {/if}
      {:else if listed.length > 0}
        <ListingToolbar
          filtersActive={narrowed}
          onClearFilters={clearFilters}
          facets={listing.facets}
          fields={shownFields}
          fieldState={listing.fields}
          view={listing.view}
        >
          {#snippet search()}
            <ListingSearch
              value={query}
              onValue={onSearch}
              label={t('app.settings.extensions.search.label')}
              placeholder={t('app.settings.extensions.search.placeholder')}
            />
          {/snippet}
        </ListingToolbar>

        {#if listing.total === 0}
          <ListingNoMatch
            heading={t('app.settings.extensions.filtered.heading')}
            headingLevel="h3"
            description={t('app.settings.extensions.filtered.note')}
            onClear={clearFilters}
          />
        {:else}
          <ExtensionsListing
            rows={listing.page}
            view={listing.view.value}
            {catalogue}
            {shows}
            {go}
            {ask}
            {highlighted}
            {listing}
          />

          {#if listing.pager}
            <Pager
              {...listing.pager}
              total={listing.total}
              class="border-t border-border pt-listing-footer"
            />
          {/if}
        {/if}
      {/if}
    </CardContent>
  </Card>

  <LeftDatabases store={ctx.leftDatabases} />
{/snippet}

<SettingsScreen title={t('app.settings.extensions.title')}>
  {@render content()}
</SettingsScreen>

<ExtensionDialog
  {catalogue}
  {asking}
  rows={all}
  onclose={() => {
    asking = undefined;
  }}
/>
