<!--
  The Prusa printer logs page: whether the ports are receiving, the lines, the
  metrics and the senders, and the two ways to set a printer up.

  The page takes no props: the route is the whole context. What it draws comes
  from the stores the module's setup built.
-->
<script lang="ts">
  import { formatBytes, formatNumber } from '@prusa/connect-ui/i18n';
  import {
    Alert,
    AlertDescription,
    Button,
    ConfirmDialog,
    DetailTabs,
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
    EmptyState,
    FaultAlert,
    PageHeader,
  } from '@prusa/connect-ui/primitives';
  import DownloadIcon from '@lucide/svelte/icons/download';
  import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
  import Trash2Icon from '@lucide/svelte/icons/trash-2';
  import ScrollTextIcon from '@lucide/svelte/icons/scroll-text';
  import { onMount } from 'svelte';

  import { DownloadStore, saveInBrowser } from '../download.svelte';
  import { PageAddress } from '../page-address.svelte';
  import { KEEP_HOURS, type KeepHours, PurgeStore } from '../purge.svelte';
  import type { LogFilter } from '../logs.svelte';
  import { type MetricsChoice, TABS, type Tab } from '../page-state';
  import { faultText } from '../faults';
  import { sinceOf } from '../presentation';
  import { heldSession } from '../session';
  import { portsOf, SetupStore } from '../setup.svelte';
  import LogView from './log-view.svelte';
  import MetricsView from './metrics-view.svelte';
  import SetupDialog from './setup-dialog.svelte';
  import SetupGuide from './setup-guide.svelte';
  import SourcesView from './sources-view.svelte';
  import StatusStrip from './status-strip.svelte';

  const { host, api, status } = heldSession();
  const t = host.t;

  const setup = new SetupStore(api, host.client, window.location.hostname);
  const downloads = new DownloadStore(api, saveInBrowser);
  const purges = new PurgeStore(api);

  // What the person picked to remove, while the question is open: hours kept,
  // or 'all'.
  let asking = $state<KeepHours | 'all' | undefined>(undefined);
  // Moves after a purge, so the views read again what is left.
  let purged = $state(0);
  // The open views, for the refresh to read them again. Reading them again
  // keeps live mode, and drawing them anew would stop it.
  let logView = $state<{ reload: () => Promise<void> }>();
  let metricsView = $state<{ reload: () => Promise<void> }>();

  const refresh = (): void => {
    void status.load();
    void logView?.reload();
    void metricsView?.reload();
  };

  const purge = async (): Promise<void> => {
    const keep = asking === 'all' ? undefined : asking;
    asking = undefined;
    if (await purges.run(keep)) {
      purged += 1;
      void status.load();
    }
  };

  // The tab, the filters and the folds live in the address bar, so a reload
  // or a link lands on the same view.
  const address = new PageAddress(window);
  // Read before the first draw, so the views start from the address.
  address.attach()();
  let setupOpen = $state(false);

  onMount(() => {
    void status.load();
    void setup.loadHost();
    const unwatch = status.watch();
    const detach = address.attach();

    return () => {
      unwatch();
      detach();
    };
  });

  const openSetup = (): void => {
    setup.open();
    setupOpen = true;
  };

  const ports = $derived(portsOf(status.status));
  const statusFault = $derived(
    status.fault === undefined ? undefined : faultText(status.fault, t),
  );
  const downloadFault = $derived(
    downloads.fault === undefined ? undefined : faultText(downloads.fault, t),
  );
  const purgeFault = $derived(
    purges.fault === undefined ? undefined : faultText(purges.fault, t),
  );

  const DOWNLOAD_HOURS = 24;
</script>

{#snippet actions()}
  <div class="flex flex-wrap gap-2">
    <Button
      variant="ghost"
      size="sm"
      aria-label={t('buddyDebugLogs.action.refresh')}
      onclick={refresh}
    >
      <RefreshCwIcon aria-hidden="true" />
    </Button>
    <DropdownMenu>
      <DropdownMenuTrigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="secondary"
            size="sm"
            disabled={downloads.busy}
          >
            <DownloadIcon aria-hidden="true" />
            <span class="max-sm:sr-only"
              >{t('buddyDebugLogs.action.download')}</span
            >
          </Button>
        {/snippet}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          onSelect={() => {
            void downloads.run('syslog', sinceOf(DOWNLOAD_HOURS, Date.now()));
          }}
        >
          {t('buddyDebugLogs.action.downloadLogs')}
        </DropdownMenuItem>
        <DropdownMenuItem
          onSelect={() => {
            void downloads.run('metrics', sinceOf(DOWNLOAD_HOURS, Date.now()));
          }}
        >
          {t('buddyDebugLogs.action.downloadMetrics')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
    <DropdownMenu>
      <DropdownMenuTrigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="secondary"
            size="sm"
            disabled={purges.busy}
            aria-busy={purges.busy}
          >
            <Trash2Icon aria-hidden="true" />
            <span class="max-sm:sr-only"
              >{t('buddyDebugLogs.purge.action')}</span
            >
          </Button>
        {/snippet}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {#each KEEP_HOURS as hours (hours)}
          <DropdownMenuItem
            onSelect={() => {
              asking = hours;
            }}
          >
            {t('buddyDebugLogs.purge.olderThan', { hours })}
          </DropdownMenuItem>
        {/each}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          onSelect={() => {
            asking = 'all';
          }}
        >
          {t('buddyDebugLogs.purge.all')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
    <Button size="sm" onclick={openSetup}
      >{t('buddyDebugLogs.action.setup')}</Button
    >
  </div>
{/snippet}

<!-- Keyed on the visits, so the back button redraws a view from the
     address it went back to. -->
{#snippet logsPanel()}
  {#key `${String(address.visits)}-${String(purged)}`}
    <LogView
      bind:this={logView}
      start={address.state.logs}
      onfilter={(logs: LogFilter) => {
        address.update({ logs });
      }}
    />
  {/key}
{/snippet}

{#snippet metricsPanel()}
  {#key `${String(address.visits)}-${String(purged)}`}
    <MetricsView
      bind:this={metricsView}
      start={address.state.metrics}
      onchoice={(metrics: MetricsChoice) => {
        address.update({ metrics });
      }}
    />
  {/key}
{/snippet}

{#snippet sourcesPanel()}
  <SourcesView />
{/snippet}

<section class="@container/logs grid gap-y-4" data-part="buddy-debug-logs-page">
  <PageHeader title={t('buddyDebugLogs.title')} {actions} />

  {#if statusFault !== undefined && status.status === undefined}
    <FaultAlert fault={statusFault} />
    <EmptyState
      icon={ScrollTextIcon}
      headingLevel="h2"
      heading={t('buddyDebugLogs.unreadable.heading')}
      description={t('buddyDebugLogs.unreadable.note')}
    />
  {:else}
    <FaultAlert fault={downloadFault} />
    <FaultAlert fault={purgeFault} />
    {#if purges.removed !== undefined}
      <Alert variant="success" data-part="purged" aria-live="polite">
        <AlertDescription>
          {t('buddyDebugLogs.purge.done', {
            records: formatNumber(purges.removed.records),
            size: formatBytes(purges.removed.bytes),
          })}
        </AlertDescription>
      </Alert>
    {/if}
    <StatusStrip
      open={address.state.status}
      onOpenChange={(status: boolean) => {
        address.update({ status });
      }}
    />
    <SetupGuide
      host={setup.host ?? window.location.hostname}
      syslogPort={ports.syslog}
      metricsPort={ports.metrics}
      open={address.state.guide}
      onOpenChange={(guide: boolean) => {
        address.update({ guide });
      }}
    />
    <DetailTabs
      tabs={[
        { id: 'logs', label: t('buddyDebugLogs.tab.logs'), panel: logsPanel },
        {
          id: 'metrics',
          label: t('buddyDebugLogs.tab.metrics'),
          panel: metricsPanel,
        },
        {
          id: 'sources',
          label: t('buddyDebugLogs.tab.sources'),
          panel: sourcesPanel,
        },
      ]}
      active={address.state.tab}
      onChange={(next: string) => {
        if ((TABS as readonly string[]).includes(next)) {
          address.openTab(next as Tab);
        }
      }}
    />
  {/if}
</section>

<ConfirmDialog
  open={asking !== undefined}
  onOpenChange={(open: boolean) => {
    if (!open) asking = undefined;
  }}
  title={asking === 'all'
    ? t('buddyDebugLogs.purge.confirmAllTitle')
    : t('buddyDebugLogs.purge.confirmOlderTitle', { hours: asking ?? 0 })}
  description={asking === 'all'
    ? t('buddyDebugLogs.purge.confirmAllNote')
    : t('buddyDebugLogs.purge.confirmOlderNote', { hours: asking ?? 0 })}
  confirmLabel={t('buddyDebugLogs.purge.confirm')}
  onConfirm={() => {
    void purge();
  }}
/>

<SetupDialog
  {setup}
  open={setupOpen}
  onClose={() => {
    setupOpen = false;
  }}
/>
