<script lang="ts">
  import { describeFault, formatDateTime, t } from '../../i18n';
  import {
    DetailTabs,
    FaultAlert,
    KeyValueList,
    SettingsPane,
    Skeleton,
  } from '../../primitives';
  import OfflineUpdate from './offline-update.svelte';
  import OnlineUpdate from './online-update.svelte';
  import type { SettingsUpdatesScreenProps } from './types';
  import ExternalRestore from './external-restore.svelte';
  import UpdateTransaction from './update-transaction.svelte';

  let { source, method = $bindable('online') }: SettingsUpdatesScreenProps =
    $props();

  const fault = $derived(
    source.error === undefined ? undefined : describeFault(source.error),
  );
</script>

{#snippet online()}
  <OnlineUpdate {source} />
{/snippet}

{#snippet offline()}
  <OfflineUpdate {source} />
{/snippet}

<div class="flex w-full flex-col gap-4">
  {#if !source.loaded}
    <SettingsPane class="w-full" title={t('settingsMaintenance.updates.title')}>
      <div
        class="flex flex-col gap-3 p-panel-padding-mobile @lg/pane:p-panel-padding"
        aria-busy="true"
        aria-label={t('settingsMaintenance.loading')}
      >
        <Skeleton class="h-24 w-full" />
        <Skeleton class="h-40 w-full" />
      </div>
    </SettingsPane>
  {:else}
    <FaultAlert {fault} />

    <UpdateTransaction {source} />

    {#if source.version}
      <SettingsPane
        class="w-full"
        title={t('settingsMaintenance.version.heading')}
      >
        <div
          class="flex flex-col gap-form-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
        >
          <KeyValueList
            rows={[
              {
                id: 'current',
                label: t('settingsMaintenance.version.current'),
                value: source.version.version,
                mono: true,
              },
              {
                id: 'platform',
                label: t('settingsMaintenance.version.platform'),
                value: `${source.version.goos}/${source.version.goarch}`,
                mono: true,
              },
              {
                id: 'profile',
                label: t('settingsMaintenance.version.profile'),
                value: `${source.version.build_profile}/${source.version.build_variant}`,
              },
              {
                id: 'management',
                label: t('settingsMaintenance.version.management'),
                value: source.version.managed_externally
                  ? t('settingsMaintenance.version.managed')
                  : t('settingsMaintenance.version.local'),
              },
            ]}
          />
          {#if source.version.managed_reason}
            <p class="text-body text-muted-foreground">
              {source.version.managed_reason}
            </p>
          {/if}
        </div>
      </SettingsPane>
    {/if}

    <DetailTabs
      tabs={[
        {
          id: 'online',
          label: t('settingsMaintenance.method.online'),
          panel: online,
        },
        {
          id: 'offline',
          label: t('settingsMaintenance.method.offline'),
          panel: offline,
        },
      ]}
      active={method}
      onChange={(id: string) => {
        method = id === 'offline' ? 'offline' : 'online';
      }}
      segmented
    />

    <SettingsPane
      class="w-full"
      title={t('settingsMaintenance.history.heading')}
    >
      <div
        class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
      >
        {#if source.history.length === 0}
          <p class="text-body text-muted-foreground">
            {t('settingsMaintenance.history.empty')}
          </p>
        {:else}
          <ul class="divide-y divide-border">
            {#each source.history as entry (entry.id)}
              <li
                class="text-body grid gap-1 py-3 first:pt-0 last:pb-0 @md/pane:grid-cols-3 @md/pane:items-center"
              >
                <span>
                  <span class="font-mono">{entry.from_version}</span>
                  →
                  <span class="font-mono">{entry.to_version}</span>
                </span>
                <span>{entry.state}</span>
                <time
                  class="text-muted-foreground @md/pane:text-end"
                  datetime={entry.updated_at}
                >
                  {formatDateTime(entry.updated_at)}
                </time>
                <ExternalRestore
                  class="@md/pane:col-span-3"
                  databases={entry.external_restore ?? []}
                />
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </SettingsPane>
  {/if}
</div>
