<script lang="ts">
  import type { Snippet } from 'svelte';

  import { t } from '../../i18n';
  import { DetailTabs } from '../../primitives';
  import type { ServerSettingsTab } from './types';

  let {
    active,
    overviewHref,
    healthHref,
    updatesHref,
    backupsHref,
    networkHref,
    onNavigate,
    children,
  }: {
    active: ServerSettingsTab;
    overviewHref: string;
    healthHref: string;
    /** Absent where core.upgrade.enabled is off, and the tab is not drawn. */
    updatesHref?: string | undefined;
    /** Absent where core.upgrade.enabled is off, and the tab is not drawn. */
    backupsHref?: string | undefined;
    networkHref: string;
    /** Opens a tab's address; the router is the caller's. */
    onNavigate: (href: string) => void;
    children: Snippet;
  } = $props();

  const tabs = $derived([
    {
      id: 'overview' as const,
      href: overviewHref,
      label: t('settingsServer.tab.overview'),
    },
    {
      id: 'health' as const,
      href: healthHref,
      label: t('settingsServer.tab.health'),
    },
    ...(updatesHref === undefined
      ? []
      : [
          {
            id: 'updates' as const,
            href: updatesHref,
            label: t('settingsServer.tab.updates'),
          },
        ]),
    ...(backupsHref === undefined
      ? []
      : [
          {
            id: 'backups' as const,
            href: backupsHref,
            label: t('settingsServer.tab.backups'),
          },
        ]),
    {
      id: 'network' as const,
      href: networkHref,
      label: t('settingsServer.tab.network'),
    },
  ]);
</script>

<DetailTabs
  {tabs}
  {active}
  onChange={(id: string) => {
    const href = tabs.find((tab) => tab.id === id)?.href;
    if (href !== undefined) onNavigate(href);
  }}
  fallback={children}
  segmented
/>
