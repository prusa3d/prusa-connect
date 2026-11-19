<!--
  One installed module: its switch and state, its settings, the ports it
  listens on, the panes it adds to Settings and what its install granted. The panes are the module's own
  registrations in the `settings` slot, one tab each.

  The page reads the printers because a pane reads them through its host and
  cannot ask for a read itself.
-->
<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import {
    Alert,
    AlertDescription,
    Button,
    FaultAlert,
    NoLongerHere,
    SettingsPane,
    SettingsRow,
  } from '@prusa/connect-ui/primitives';
  import {
    ExtensionDetailScreen,
    ModuleSettingsForm,
  } from '@prusa/connect-ui/shell';
  import {
    getSlotContext,
    hiddenEntries,
    moduleTabs,
    SlotOutlet,
  } from '@prusa/connect-ui/slots';
  import { onMount, untrack } from 'svelte';

  import {
    type Asking,
    type AskingAct,
    ExtensionDialog,
  } from '../extensions/extension-dialog';
  import { faultOf, type ResourceInstance } from '@prusa/connect-ui/data';
  import { databasePlace, databaseSchema } from '../extensions/database-place';
  import {
    dependenciesOfInstance,
    hasDependencies,
  } from '../extensions/dependencies';
  import { ExtensionDependencies } from '../extensions/extension-dependencies';
  import { ExtensionMemoryPanel } from '../extensions/extension-memory-panel';
  import { ModuleLog } from '../extensions/module-log';
  import { ExtensionPorts } from '../extensions/extension-ports';
  import {
    contributionsOf,
    ExtensionVisibility,
  } from '../extensions/extension-visibility';
  import { ExtensionSwitch } from '../extensions/extension-switch';
  import { describeExtensionFault } from '../extensions/fault';
  import { RequestList } from '../extensions/request-list';
  import { listRequests } from '../extensions/requests';
  import {
    type ExtensionRow,
    extensionRowsOf,
    SOURCE_KEY,
  } from '../extensions/rows';
  import type { InstanceChange } from '../stores/module-catalogue.svelte';
  import { observedOf, removableOf, whyOf } from '../extensions/switch';
  import { APP_NAMESPACE } from '../i18n';
  import { PATHS, extensionDetailHref } from '../routes';
  import type { AppContext } from '../screens';

  let { ctx }: { ctx: AppContext } = $props();

  const t = translateFor(APP_NAMESPACE);
  const slots = getSlotContext();
  const catalogue = $derived(ctx.moduleCatalogue);

  onMount(() => {
    void ctx.fleet.load();
    void ctx.moduleCatalogue.load();
    void ctx.moduleCatalogue.readWhy(ctx.params.namespace);
    void ctx.moduleCatalogue.readOutdated();
    void ctx.moduleCatalogue.readMemory();
    void slots?.loader?.activate();
  });

  const namespace = $derived(ctx.params.namespace ?? '');
  const row = $derived(
    extensionRowsOf(catalogue.items, ctx.modules.items, ctx.modules.pages).find(
      (one) => one.namespace === namespace && one.instance !== undefined,
    ),
  );

  // Gone is a module the loaded list no longer holds, or a list this reader
  // may not read.
  const gone = $derived(
    (row?.instance === undefined &&
      ctx.modules.loaded &&
      ctx.modules.error === undefined) ||
      [403, 404].includes(faultOf(ctx.modules.error).status ?? 0),
  );
  const installedVersion = $derived.by(() => {
    if (row === undefined) return '';
    const version = [row.installedVersion, row.instance?.module?.version].find(
      (value) => value !== undefined && value !== '',
    );
    return version === undefined
      ? t('app.settings.extensions.version.unknown')
      : t('app.settings.extensions.version.installed', { version });
  });

  const panes = $derived(
    (slots ? moduleTabs(slots.registry, 'settings') : []).filter((pane) =>
      pane.id.startsWith(`${namespace}.`),
    ),
  );
  const options = $derived(row?.instance?.module?.options ?? []);
  const askedPorts = $derived(row?.instance?.module?.asked?.listen ?? []);
  // The modules it depends on and the installed ones that require it
  // (ADR-20261027-e6k rule 29).
  const dependencies = $derived(dependenciesOfInstance(row?.instance));
  const contributions = $derived(contributionsOf(row?.instance));
  // Only a service module has a memory budget.
  const memory = $derived(row?.instance?.status.memory);
  const tabs = $derived([
    ...(options.length > 0
      ? [
          {
            id: 'settings',
            label: t('app.settings.extensions.tab.settings'),
            panel: settingsPanel,
          },
        ]
      : []),
    ...(askedPorts.length > 0
      ? [
          {
            id: 'ports',
            label: t('app.settings.extensions.tab.ports'),
            panel: portsPanel,
          },
        ]
      : []),
    ...(memory === undefined
      ? []
      : [
          {
            id: 'memory',
            label: t('app.settings.extensions.tab.memory'),
            panel: memoryPanel,
          },
          {
            id: 'log',
            label: t('app.settings.extensions.tab.log'),
            panel: logPanel,
          },
        ]),
    ...panes.map((pane) => ({ ...pane, panel: panePanel })),
    ...(contributions.length > 0
      ? [
          {
            id: 'visibility',
            label: t('app.settings.extensions.tab.visibility'),
            panel: visibilityPanel,
          },
        ]
      : []),
    // An installed module always has the tab: it says why the module is
    // installed and how its release stands, even with nothing to depend on.
    ...(row?.instance?.module !== undefined || hasDependencies(dependencies)
      ? [
          {
            id: 'dependencies',
            label: t('app.settings.extensions.tab.dependencies'),
            panel: dependenciesPanel,
          },
        ]
      : []),
    {
      id: 'permissions',
      label: t('app.settings.extensions.tab.permissions'),
      panel: permissionsPanel,
    },
  ]);
  const tab = $derived(
    tabs.some((one) => one.id === ctx.params.tab)
      ? (ctx.params.tab ?? '')
      : (tabs[0]?.id ?? ''),
  );

  // The usage on the Memory tab is the server's latest sample, and the host
  // may switch a module off at any time, so the listing, the pool, the last
  // hour and the log are read again while either tab is open.
  $effect(() => {
    // Reading the namespace re-runs this for another module's page, which then reads at once.
    if (namespace === '' || (tab !== 'memory' && tab !== 'log')) return;
    const onMemory = tab === 'memory';
    const readHistory = () => {
      const one = row?.instance;
      if (one !== undefined)
        void ctx.moduleOperations.load(one, onMemory ? 'history' : 'log');
    };
    untrack(readHistory);
    const timer = setInterval(() => {
      if (onMemory) {
        void ctx.modules.load();
        void ctx.moduleCatalogue.readMemory();
      }
      readHistory();
    }, 5000);

    return () => {
      clearInterval(timer);
    };
  });

  let asking = $state<Asking | undefined>(undefined);
  const ask = (asked: ExtensionRow, act: AskingAct, version?: string): void => {
    asking = { row: asked, act, version };
  };

  // Unknown is left to the server, which decides again on every request.
  const allowed = $derived(
    row !== undefined && catalogue.mayConfigure(row.kind) !== false,
  );
  // The instance the form's edits started from. A save is fenced on its
  // version, so a change someone else made meanwhile is refused instead of
  // overwritten when the page re-reads.
  let edited: ResourceInstance | undefined;
  const lockedReason = $derived.by(() => {
    if (row?.instance?.source === 'file') {
      return t('app.settings.extensions.switch.locked.file');
    }
    if (row !== undefined && catalogue.mayConfigure(row.kind) === false) {
      return t('app.settings.extensions.switch.locked.access');
    }
    return undefined;
  });
  // Saving a port installs the same release again, which the server refuses
  // while modules are off (ADR-20261019-h76 rules 5 and 15) and which needs the offer
  // this page reads for the module.
  const portsLocked = $derived.by(() => {
    if (lockedReason !== undefined) return lockedReason;
    if (!catalogue.modulesOn) {
      return t(
        catalogue.safeMode
          ? 'app.settings.extensions.switch.locked.safeMode'
          : 'app.settings.extensions.switch.locked.modules',
      );
    }
    if (row?.offer === undefined || row.instance?.module?.asked === undefined) {
      return t('app.settings.extensions.ports.byHand');
    }
    return undefined;
  });
  // The store keeps the last write of any module, so the page shows only a
  // refusal of this module's, beside what the write was for.
  const writeFor = $derived(
    catalogue.writeModule === namespace ? catalogue.writeFor : undefined,
  );
  const switchFault = $derived(
    writeFor === 'switch' && catalogue.writeError !== undefined
      ? describeExtensionFault(catalogue.writeError, t)
      : undefined,
  );
  const blocked = $derived(
    catalogue.acting !== undefined || catalogue.switching,
  );
</script>

{#if gone}
  <NoLongerHere
    headingLevel="h1"
    heading={t('app.settings.extensions.missing')}
    description={t('app.settings.extensions.missingNote')}
    linkLabel={t('app.settings.extensions.missingList')}
    href={PATHS.settingsExtensions}
  />
{:else}
  <ExtensionDetailScreen
    title={row?.title ?? namespace}
    version={installedVersion}
    source={row === undefined ? '' : t(SOURCE_KEY[row.source])}
    module={row?.module}
    observed={row?.instance === undefined
      ? 'stopped'
      : observedOf(row.instance)}
    result={row?.instance === undefined
      ? undefined
      : (whyOf(row.instance, t)?.title ?? row.instance.status.result)}
    control={row === undefined ? undefined : control}
    actions={row === undefined ? undefined : actions}
    {tabs}
    {tab}
    onTabChange={(next: string) => {
      ctx.go(extensionDetailHref(namespace, next));
    }}
    extensionsHref={PATHS.settingsExtensions}
    loading={!ctx.modules.loaded}
  />
{/if}

<ExtensionDialog
  {catalogue}
  {asking}
  rows={row ? [row] : []}
  onclose={() => {
    asking = undefined;
  }}
/>

{#snippet control()}
  {#if row}
    {#if row.pageUrl}<a
        href={row.pageUrl}
        target="_blank"
        rel="noopener noreferrer"
        class="text-sm break-all text-muted-foreground underline underline-offset-4"
        >{row.pageUrl}</a
      >{/if}
    <ExtensionSwitch {row} {catalogue} {ask} />
  {/if}
  {#if writeFor === 'switch' && catalogue.writeConflict}
    <Alert variant="warning" data-part="switch-fault">
      <AlertDescription>
        {t('app.settings.extensions.write.conflict')}
      </AlertDescription>
    </Alert>
  {:else if switchFault !== undefined}
    <div data-part="switch-fault"><FaultAlert fault={switchFault} /></div>
  {/if}
  {#if row?.instance && catalogue.restartErrorOf(row.instance) !== undefined}
    <div data-part="restart-fault">
      <FaultAlert
        fault={describeExtensionFault(
          catalogue.restartErrorOf(row.instance),
          t,
        )}
      />
    </div>
  {/if}
{/snippet}

{#snippet actions()}
  {#if row?.instance && catalogue.mayOperate(row.kind) !== false}
    {@const instance = row.instance}
    <Button
      size="sm"
      variant="secondary"
      disabled={blocked ||
        catalogue.restarting !== undefined ||
        !catalogue.modulesOn}
      onclick={() => {
        void catalogue.restart(instance);
      }}
    >
      {catalogue.restarting === `${instance.kind}:${instance.name}`
        ? t('app.settings.extensions.restarting')
        : t('app.settings.extensions.restart')}
    </Button>
  {/if}
  {#if row && allowed}
    {#if row.state === 'update'}
      <Button
        size="sm"
        disabled={blocked}
        onclick={() => {
          ask(row, 'update');
        }}
      >
        {t('app.settings.extensions.offer.update', { version: row.version })}
      </Button>
    {/if}
    <Button
      size="sm"
      variant="secondary"
      disabled={blocked || !removableOf(row, catalogue.mayConfigure(row.kind))}
      onclick={() => {
        ask(row, 'remove');
      }}
    >
      {t('app.settings.extensions.offer.remove')}
    </Button>
  {/if}
{/snippet}

{#snippet portsPanel()}
  {#if row?.instance}
    {@const asked = row.instance.module?.asked}
    {@const offer = row.offer}
    <SettingsPane class="w-full" title={t('app.settings.extensions.tab.ports')}>
      <div
        class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
      >
        <ExtensionPorts
          asked={askedPorts}
          listening={row.instance.module?.listening ?? []}
          locked={portsLocked}
          busy={blocked}
          fault={catalogue.acting === undefined &&
          catalogue.actionError !== undefined
            ? catalogue.actionError
            : undefined}
          onsave={async (listen: Record<string, number>) => {
            if (offer === undefined || asked === undefined) return false;
            const saved = await catalogue.install(offer, {
              expect: asked,
              version: row.installedVersion ?? row.version,
              approveListen: true,
              listen,
            });
            catalogue.dismissOutcome();
            void ctx.modules.load();

            return saved;
          }}
        />
      </div>
    </SettingsPane>
  {/if}
{/snippet}

{#snippet memoryPanel()}
  {#if row?.instance && memory}
    {@const instance = row.instance}
    <ExtensionMemoryPanel
      {instance}
      {memory}
      pool={catalogue.memoryPool}
      locked={lockedReason}
      saving={catalogue.writing !== undefined}
      fault={writeFor === 'memory' ? catalogue.writeError : undefined}
      conflict={writeFor === 'memory' && catalogue.writeConflict}
      history={ctx.moduleOperations.history(instance)}
      historyFault={ctx.moduleOperations.historyFault(instance)}
      missing={ctx.moduleOperations.missing(instance)}
      write={(changes: InstanceChange) =>
        catalogue.writeInstance(instance, changes).then(async (written) => {
          if (written) await ctx.modules.load();
          return written;
        })}
    />
  {/if}
{/snippet}

{#snippet logPanel()}
  {#if row?.instance}
    {@const instance = row.instance}
    <SettingsPane class="w-full" title={t('app.settings.extensions.tab.log')}>
      <div
        class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
      >
        <ModuleLog
          log={ctx.moduleOperations.log(instance)}
          refused={ctx.moduleOperations.logRefused(instance)}
          fault={ctx.moduleOperations.logFault(instance)}
          missing={ctx.moduleOperations.missing(instance)}
        />
      </div>
    </SettingsPane>
  {/if}
{/snippet}

{#snippet panePanel()}
  <SlotOutlet screen="settings" name="settings" entry={tab} />
{/snippet}

{#snippet dependenciesPanel()}
  <SettingsPane
    class="w-full"
    title={t('app.settings.extensions.tab.dependencies')}
  >
    <div
      class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
    >
      <ExtensionDependencies
        {dependencies}
        {namespace}
        why={catalogue.whyOf(namespace)}
        outdated={catalogue.outdated}
      />
    </div>
  </SettingsPane>
{/snippet}

{#snippet permissionsPanel()}
  {@const asked = row?.instance?.module?.asked}
  <SettingsPane
    class="w-full"
    title={t('app.settings.extensions.tab.permissions')}
    description={asked === undefined
      ? t('app.settings.extensions.grantedNothing')
      : t('app.settings.extensions.granted')}
  >
    {#if asked !== undefined}
      <RequestList
        rows
        instance={row?.instance?.name ?? ''}
        requests={listRequests(asked)}
        label={t('app.settings.extensions.granted')}
      />
      {@const place = databasePlace(row?.instance?.module?.database, t)}
      {@const schema = databaseSchema(row?.instance?.module?.database, t)}
      {#if place !== '' || schema !== ''}
        <div
          class="flex flex-col gap-section-gap px-panel-padding-mobile pb-panel-padding-mobile @lg/pane:px-panel-padding @lg/pane:pb-panel-padding"
        >
          {#if place !== ''}
            <SettingsRow
              layout="switch"
              label={t('app.settings.extensions.database.placeLabel')}
              description={place}
            />
          {/if}
          {#if schema !== ''}
            <SettingsRow
              layout="switch"
              label={t('app.settings.extensions.database.schemaLabel')}
              description={schema}
            />
          {/if}
        </div>
      {/if}
    {/if}
  </SettingsPane>
{/snippet}

{#snippet visibilityPanel()}
  {#if row?.instance}
    {@const instance = row.instance}
    <SettingsPane
      class="w-full"
      title={t('app.settings.extensions.tab.visibility')}
    >
      <div
        class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
      >
        <ExtensionVisibility
          {instance}
          {contributions}
          locked={lockedReason}
          saving={catalogue.writing !== undefined}
          fault={writeFor === 'visibility' ? catalogue.writeError : undefined}
          conflict={writeFor === 'visibility' && catalogue.writeConflict}
          onhide={async (next: string[]) => {
            const written = await catalogue.writeInstance(instance, {
              hidden_contributions: next,
            });
            // The loader declares the modules once, when the app starts.
            if (written) {
              slots?.registry.hide(
                instance.namespace ?? namespace,
                hiddenEntries({ hidden_contributions: next, ui: instance.ui }),
              );
              void ctx.modules.load();
            }

            return written;
          }}
        />
      </div>
    </SettingsPane>
  {/if}
{/snippet}

{#snippet settingsPanel()}
  {#if row?.instance}
    {@const instance = row.instance}
    <ModuleSettingsForm
      options={row.instance.module?.options ?? []}
      config={row.instance.config}
      locked={lockedReason}
      saving={catalogue.writing !== undefined}
      fault={writeFor === 'settings' ? catalogue.writeError : undefined}
      conflict={writeFor === 'settings' && catalogue.writeConflict}
      onsave={(next: Record<string, unknown>) =>
        catalogue.writeInstance(edited ?? instance, { config: next })}
      ondirty={(dirty: boolean) => {
        edited = dirty ? (edited ?? instance) : undefined;
      }}
      onreload={() => {
        void ctx.modules.load();
      }}
    />
  {/if}
{/snippet}
