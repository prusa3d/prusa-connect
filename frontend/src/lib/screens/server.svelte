<!-- The Server room inside Settings: an overview of the server, readiness and
     the printers behind each connector, updates, backups, and how the server
     appears on the local network. -->
<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import type { RestartMode } from '@prusa/connect-ui/data';
  import {
    ServerOverview,
    ServerRestartPanel,
    ServerSettingsShell,
    SettingsBackupsScreen,
    SettingsNetworkScreen,
    SettingsScreen,
    SettingsServerScreen,
    SettingsUpdatesScreen,
    type ServerSettingsTab,
  } from '@prusa/connect-ui/shell';
  import { untrack } from 'svelte';

  import { restartAllowed } from '../restart-access';
  import { APP_NAMESPACE } from '../i18n';
  import { extensionDetailHref, PATHS } from '../routes';
  import type { AppContext } from '../screens';
  import NotFound from './not-found.svelte';

  const t = translateFor(APP_NAMESPACE);

  let { ctx }: { ctx: AppContext } = $props();

  const health = $derived(ctx.health);
  const maintenance = $derived(ctx.maintenance);
  const discovery = $derived(ctx.discovery);
  const timeServer = $derived(ctx.timeServer);
  const allowedOrigins = $derived(ctx.allowedOrigins);
  const restart = $derived(ctx.restart);
  let confirming = $state(false);
  const tabOf = (path: string): ServerSettingsTab => {
    if (path === PATHS.settingsServerHealth) return 'health';
    if (path === PATHS.settingsServerBackups) return 'backups';
    if (path === PATHS.settingsServerUpdates) return 'updates';
    if (path === PATHS.settingsServerNetwork) return 'network';

    return 'overview';
  };
  const active = $derived(tabOf(ctx.path));
  // core.upgrade.enabled off: the server answers 404 for upgrades and backups,
  // so neither tab is drawn, and opening either address shows the not-found
  // screen.
  const upgrades = $derived(ctx.settings.upgrades);
  const updatesHref = $derived(
    upgrades ? PATHS.settingsServerUpdates : undefined,
  );
  const backupsHref = $derived(
    upgrades ? PATHS.settingsServerBackups : undefined,
  );

  $effect(() => {
    void health.load();
  });

  // Maintenance loads again when the switch changes, and not when a new read
  // of the settings leaves the switch as it was.
  let loadedFor: boolean | undefined;
  $effect(() => {
    if (loadedFor === upgrades) return;
    loadedFor = upgrades;
    untrack(() => {
      void maintenance.load();
    });
  });

  $effect(() => {
    if (active === 'network' || active === 'overview') void discovery.load();
  });

  $effect(() => {
    if (active === 'network') void timeServer.load();
  });

  $effect(() => {
    if (active === 'network') void allowedOrigins.load();
  });
</script>

{#if !upgrades && (active === 'updates' || active === 'backups')}
  <NotFound {ctx} />
{:else}
  <SettingsScreen title={t('app.nav.settings.server')}>
    <ServerSettingsShell
      {active}
      overviewHref={PATHS.settingsServerOverview}
      healthHref={PATHS.settingsServerHealth}
      {updatesHref}
      {backupsHref}
      networkHref={PATHS.settingsServerNetwork}
      onNavigate={ctx.go}
    >
      {#if active === 'overview'}
        <div class="flex flex-col gap-4">
          <ServerOverview
            {health}
            {maintenance}
            network={discovery}
            healthHref={PATHS.settingsServerHealth}
            {updatesHref}
            {backupsHref}
            networkHref={PATHS.settingsServerNetwork}
          />
          <ServerRestartPanel
            safeMode={ctx.settings.safeMode !== undefined}
            allowed={restartAllowed(maintenance)}
            phase={restart.phase}
            refusal={restart.refusal}
            error={restart.error}
            readError={restartAllowed(maintenance) === undefined
              ? maintenance.error
              : undefined}
            onRetryRead={() => {
              void maintenance.load();
            }}
            bind:confirming
            onRestart={(mode: RestartMode) => {
              void restart.restart(mode);
            }}
          />
        </div>
      {:else if active === 'health'}
        <SettingsServerScreen source={health} />
      {:else if active === 'updates'}
        <SettingsUpdatesScreen source={maintenance} />
      {:else if active === 'network'}
        <SettingsNetworkScreen
          source={discovery}
          {timeServer}
          origins={allowedOrigins}
          extensionHref={ctx.settings.modules
            ? (namespace: string) => extensionDetailHref(namespace)
            : undefined}
          docsHref={ctx.settings.documentation}
        />
      {:else}
        <SettingsBackupsScreen source={maintenance} />
      {/if}
    </ServerSettingsShell>
  </SettingsScreen>
{/if}
