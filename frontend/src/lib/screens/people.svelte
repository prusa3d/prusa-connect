<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import {
    SettingsPeopleScreen,
    SettingsScreen,
  } from '@prusa/connect-ui/shell';

  import { personHref } from '../routes';
  import { APP_NAMESPACE } from '../i18n';
  import type { AppContext } from '../screens';

  const t = translateFor(APP_NAMESPACE);

  let { ctx }: { ctx: AppContext } = $props();

  const people = $derived(ctx.accounts);

  $effect(() => {
    void people.load();
  });
</script>

<SettingsScreen title={t('app.nav.settings.people')}>
  <SettingsPeopleScreen
    {people}
    subject={ctx.session.principal?.id}
    {personHref}
  />
</SettingsScreen>
