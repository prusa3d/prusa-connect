<!--
  The EasyPrint tab on this module's page under Settings, Extensions: the
  link that opens EasyPrint with this server's address filled in, and what a
  person has to know about HTTPS before EasyPrint can call the server.

  A settings pane takes no props. The host draws it with no heading, so the
  pane names itself, and it is edged because that screen's ground is the
  pane's own colour.
-->
<script lang="ts">
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Button,
    CopyField,
    SettingsPane,
  } from '@prusa/connect-ui/primitives';

  import { EASYPRINT_ORIGIN, isPlainHttp, pairHref } from './easyprint';
  import { heldHost } from './host';
  import Logo from './logo.svelte';

  const host = heldHost();
  const t = host.t;

  // EasyPrint calls the server from this browser, so the address it needs is
  // the one this page was opened on, and not one the server knows itself by.
  const server = window.location.origin;
  const plainHttp = isPlainHttp(window.location.protocol);

  // Undefined until the server answers, and for a person who may not read the
  // list: neither is told the origin is missing.
  let allowed = $state<boolean | undefined>(undefined);

  $effect(() => {
    void host.client.listAllowedOrigins({}, host.signal).then(
      (answer) => {
        allowed = (answer.items ?? []).some(
          (item) => item.origin === EASYPRINT_ORIGIN,
        );
      },
      () => {
        allowed = undefined;
      },
    );
  });
</script>

<SettingsPane title={t('easyprint.settings.title')} edged>
  <div
    class="flex max-w-3xl flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    <div class="flex items-center gap-4">
      <Logo />
      <p class="text-body text-muted-foreground">
        {t('easyprint.settings.about')}
      </p>
    </div>

    {#if allowed === false}
      <Alert variant="warning" data-part="not-allowed">
        <AlertTitle>{t('easyprint.settings.notAllowed.title')}</AlertTitle>
        <AlertDescription>
          {t('easyprint.settings.notAllowed.body', {
            origin: EASYPRINT_ORIGIN,
          })}
        </AlertDescription>
      </Alert>
    {/if}

    <section class="flex flex-col gap-3" data-part="pair">
      <h3 class="text-sm font-semibold">
        {t('easyprint.settings.pair.heading')}
      </h3>
      <p class="text-caption text-muted-foreground">
        {t('easyprint.settings.pair.note')}
      </p>
      <CopyField label={t('easyprint.settings.pair.address')} value={server} />
      <div>
        <!-- EasyPrint is another site, so it opens in a tab of its own. -->
        <Button>
          {#snippet child({ props }: { props: Record<string, unknown> })}
            <a
              {...props}
              href={pairHref(server)}
              target="_blank"
              rel="noopener noreferrer"
              data-part="pair-link"
            >
              {t('easyprint.settings.pair.action')}
            </a>
          {/snippet}
        </Button>
      </div>
    </section>

    <section class="flex flex-col gap-3" data-part="https">
      <h3 class="text-sm font-semibold">
        {t('easyprint.settings.https.heading')}
      </h3>
      {#if plainHttp}
        <Alert variant="warning" data-part="plain-http">
          <AlertTitle>{t('easyprint.settings.https.plainTitle')}</AlertTitle>
          <AlertDescription>
            {t('easyprint.settings.https.plainBody')}
          </AlertDescription>
        </Alert>
      {/if}
      <p class="text-caption text-muted-foreground">
        {t('easyprint.settings.https.note')}
      </p>
    </section>
  </div>
</SettingsPane>
