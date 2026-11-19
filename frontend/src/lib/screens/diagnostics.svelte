<!-- The Diagnostics room inside Settings: a redacted bundle for a support
     ticket, carrying this browser's own log. -->
<script lang="ts">
  import { translateFor } from '@prusa/connect-ui/i18n';
  import {
    type DiagnosticsChoice,
    downloadDiagnostics,
    sendDiagnostics,
  } from '@prusa/connect-ui/data';
  import { setReportHref } from '@prusa/connect-ui/primitives';
  import {
    type SendIntent,
    SettingsDiagnosticsScreen,
    SettingsScreen,
  } from '@prusa/connect-ui/shell';

  import { onMount } from 'svelte';

  import { client } from '../client';
  import { saveBlob } from '../download';
  import { entries } from '../log';
  import { APP_NAMESPACE } from '../i18n';
  import type { AppContext } from '../screens';
  import { sendDiagnosticsToPortal } from '../send-diagnostics';
  import { DiagnosticsExtensions } from '../stores/diagnostics-extensions.svelte';
  import NotFound from './not-found.svelte';

  const t = translateFor(APP_NAMESPACE);

  let { ctx }: { ctx: AppContext } = $props();

  // A failure on this page would otherwise link to this page.
  setReportHref(undefined);

  // Read once per visit: an extension installed meanwhile is named the next
  // time the page opens.
  const extensions = new DiagnosticsExtensions(client);

  onMount(() => {
    if (ctx.settings.diagnostics) void extensions.load();
  });

  const onDownload = async (choice: DiagnosticsChoice): Promise<void> => {
    const { blob, name } = await downloadDiagnostics(client, entries(), choice);
    saveBlob(blob, name);
  };

  const onCopy = (): string => JSON.stringify(entries(), null, 2);

  // Undefined where the operator switched sending off, and then the screen
  // draws no Send button.
  const target = $derived(ctx.settings.diagnosticsSendUrl);

  const send = (
    to: string,
    choice: DiagnosticsChoice,
    intent: SendIntent,
  ): Promise<void> =>
    sendDiagnosticsToPortal(intent, {
      open: () => window.open('about:blank', '_blank'),
      build: async () =>
        (await downloadDiagnostics(client, entries(), choice)).blob,
      version: async () => {
        if (ctx.appVersion.version === undefined) await ctx.appVersion.load();

        return ctx.appVersion.version ?? 'unknown';
      },
      upload: (bundle, version) => sendDiagnostics(bundle, to, version),
      go: (url) => {
        window.location.assign(url);
      },
    });

  const onSend = $derived(
    target === undefined
      ? undefined
      : (choice: DiagnosticsChoice, intent: SendIntent) =>
          send(target, choice, intent),
  );
</script>

<!-- A server that leaves the bundle out does not answer this address. -->
{#if ctx.settings.diagnostics}
  <SettingsScreen title={t('app.nav.settings.diagnostics')}>
    <SettingsDiagnosticsScreen
      {onDownload}
      {onCopy}
      {onSend}
      extensions={extensions.titles}
    />
  </SettingsScreen>
{:else}
  <NotFound {ctx} />
{/if}
