<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ServerSettingsShell from './server-settings-shell.svelte';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Server Settings Shell',
    component: ServerSettingsShell,
  });
</script>

<Story name="Overview" asChild>
  <ServerSettingsShell
    active="overview"
    overviewHref="/settings/server/overview"
    healthHref="/settings/server/health"
    updatesHref="/settings/server/updates"
    backupsHref="/settings/server/backups"
    networkHref="/settings/server/network"
    onNavigate={() => undefined}
  >
    <div></div>
  </ServerSettingsShell>
</Story>

<Story name="Updates Off" asChild>
  <ServerSettingsShell
    active="overview"
    overviewHref="/settings/server/overview"
    healthHref="/settings/server/health"
    networkHref="/settings/server/network"
    onNavigate={() => undefined}
  >
    <div></div>
  </ServerSettingsShell>
</Story>
