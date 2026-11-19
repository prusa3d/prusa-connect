<!--
  The Overview tab of the Server room: one row per part of the server, saying
  in a line how that part stands and leading to the tab that says more, where
  there is one.
  Every row reads a store the other tabs read, so the overview never
  disagrees with the tab it leads to.
-->
<script lang="ts">
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';

  import { PAIRING_NEEDED_FAULT } from '../../data';
  import { formatDateTime, formatNumber, t } from '../../i18n';
  import { SettingsPane, SettingsRow, Skeleton } from '../../primitives';
  import { networkVerdict } from '../settings-network/verdict';
  import type { ServerOverviewProps } from './types';

  let {
    health,
    maintenance,
    network,
    healthHref,
    updatesHref,
    backupsHref,
    networkHref,
  }: ServerOverviewProps = $props();

  // An upgrade in one of these states has ended, so the row speaks of the
  // releases instead.
  const ENDED = new Set(['committed', 'rolled-back', 'failed-before-swap']);

  // Every key is written out rather than assembled from a value.
  const NETWORK: Readonly<Record<string, string>> = {
    announcing: 'settingsServer.overview.network.announcing',
    bridged: 'settingsServer.overview.network.bridged',
    hidden: 'settingsServer.overview.network.hidden',
    off: 'settingsServer.overview.network.off',
    failed: 'settingsServer.overview.network.failed',
  };

  const readiness = $derived(health.readiness);
  const statusText = $derived.by((): string => {
    if (readiness?.ready === true) return t('settingsServer.summary.healthy');
    if (readiness?.ready === false)
      return t('settingsServer.summary.attention');

    return t('settingsServer.summary.unknown');
  });
  const statusNote = $derived(
    readiness !== undefined && readiness.serving < readiness.declared
      ? t('settingsServer.readiness.serving', {
          serving: readiness.serving,
          declared: readiness.declared,
        })
      : undefined,
  );

  const printers = $derived.by(() => {
    let total = 0;
    let connected = 0;
    for (const rollup of health.connectors) {
      total += rollup.total;
      connected += rollup.counts.connected ?? 0;
    }

    return { total, connected, away: total - connected };
  });
  const printersText = $derived(
    printers.total === 0
      ? t('settingsServer.overview.noPrinters')
      : t('settingsServer.overview.connected', {
          connected: formatNumber(printers.connected),
          total: formatNumber(printers.total),
        }),
  );
  const printersNote = $derived(
    printers.away > 0 && printers.total > 0
      ? t('settingsServer.overview.away', { count: printers.away })
      : undefined,
  );

  const upgrading = $derived(
    maintenance.upgrade !== undefined && !ENDED.has(maintenance.upgrade.state)
      ? maintenance.upgrade
      : undefined,
  );
  const newest = $derived(maintenance.releases[0]);
  const lastBackup = $derived(maintenance.backups[0]);

  const versionNote = $derived.by((): string | undefined => {
    if (updatesHref === undefined)
      return t('settingsServer.overview.updatesOffNote');
    if (upgrading !== undefined)
      return t('settingsServer.overview.upgrading', { state: upgrading.state });
    if (newest !== undefined)
      return t('settingsServer.summary.available', {
        version: newest.version,
      });
    if (maintenance.releaseLoading) return undefined;
    if (maintenance.releaseError !== undefined)
      return maintenance.releaseError.code === PAIRING_NEEDED_FAULT
        ? t('settingsServer.summary.unpaired')
        : t('settingsServer.summary.unchecked');

    return t('settingsServer.summary.current');
  });

  const announcement = $derived(
    networkVerdict(network.announcement, network.described?.bridged),
  );
  const networkText = $derived.by(() => {
    const key = announcement === undefined ? undefined : NETWORK[announcement];

    return key === undefined ? undefined : t(key);
  });
  const announcedAt = $derived.by(() => {
    const described = network.described;
    if (described?.host === undefined) return undefined;

    return described.port === undefined
      ? described.host
      : `${described.host}:${String(described.port)}`;
  });
  const networkNote = $derived.by((): string | undefined => {
    if (announcement === 'announcing') return announcedAt;
    if (announcement === 'off')
      return t('settingsServer.overview.network.offNote');

    return undefined;
  });
</script>

{#snippet row(
  part: string,
  label: string,
  href: string | undefined,
  loading: boolean,
  value: string | undefined,
  note: string | undefined,
  fault = false,
)}
  <li data-part={part}>
    <SettingsRow {label}>
      {#snippet control()}
        {#if loading}
          <Skeleton class="h-5 w-24" />
        {:else}
          <span>
            {#if href === undefined}
              <span class={[fault && 'text-destructive-text']}>{value}</span>
            {:else}
              <a
                {href}
                class={[
                  'inline-flex items-center gap-1 rounded-sm underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
                  fault && 'text-destructive-text',
                ]}
              >
                {value}
                <ChevronRightIcon
                  aria-hidden="true"
                  class="size-4 text-muted-foreground rtl:rotate-180"
                />
              </a>
            {/if}
            {#if note !== undefined}
              <span class="ms-2 text-caption text-muted-foreground">{note}</span
              >
            {/if}
          </span>
        {/if}
      {/snippet}
    </SettingsRow>
  </li>
{/snippet}

<SettingsPane class="w-full" title={t('settingsServer.overview.title')}>
  <ul
    class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    {@render row(
      'status',
      t('settingsServer.overview.status'),
      healthHref,
      !health.loaded,
      statusText,
      statusNote,
      readiness?.ready === false,
    )}
    {@render row(
      'printers',
      t('settingsServer.overview.printers'),
      healthHref,
      !health.loaded,
      printersText,
      printersNote,
    )}
    {@render row(
      'version',
      t('settingsServer.summary.version'),
      updatesHref,
      !maintenance.loaded,
      maintenance.version?.version ?? t('settingsServer.summary.unavailable'),
      versionNote,
    )}
    {#if backupsHref !== undefined}
      {@render row(
        'backup',
        t('settingsServer.summary.backup'),
        backupsHref,
        !maintenance.loaded,
        lastBackup === undefined
          ? t('settingsServer.summary.noBackup')
          : formatDateTime(lastBackup.created_at),
        undefined,
      )}
    {/if}
    {@render row(
      'network',
      t('settingsServer.overview.network'),
      networkHref,
      !network.loaded && network.error === undefined,
      network.loaded ? networkText : t('settingsServer.summary.unavailable'),
      network.loaded ? networkNote : undefined,
    )}
  </ul>
</SettingsPane>
