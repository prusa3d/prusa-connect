<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import type { ApiCredential, CredentialScope } from '@prusa/connect-ui/data';
  import {
    SettingsCredentialsScreen,
    SettingsScreen,
  } from '@prusa/connect-ui/shell';

  import { APP_NAMESPACE } from '../i18n';
  import { PATHS } from '../routes';
  import type { AppContext } from '../screens';

  const t = translateFor(APP_NAMESPACE);

  let { ctx }: { ctx: AppContext } = $props();

  const credentials = $derived(ctx.credentials);

  $effect(() => {
    void credentials.load();
  });
</script>

<SettingsScreen title={t('app.nav.settings.tokens')}>
  <SettingsCredentialsScreen
    {credentials}
    pairHref={PATHS.pair}
    onCreate={(name: string, days: number, scope: CredentialScope) => {
      void credentials.mint(name, days, scope);
    }}
    onDismiss={() => {
      credentials.dismiss();
    }}
    onRevoke={(credential: ApiCredential) => {
      void credentials.revoke(credential.id);
    }}
  />
</SettingsScreen>
