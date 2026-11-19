<script lang="ts">
  import { t } from '@prusa/connect-ui/i18n';
  import {
    SettingsAppearanceScreen,
    SettingsScreen,
  } from '@prusa/connect-ui/shell';

  import type { AppContext } from '../screens';

  let { ctx }: { ctx: AppContext } = $props();
</script>

<SettingsScreen title={t('settingsAppearance.title')}>
  <SettingsAppearanceScreen
    home={ctx.preferences}
    dashboards={ctx.settings.dashboards}
  />
</SettingsScreen>
