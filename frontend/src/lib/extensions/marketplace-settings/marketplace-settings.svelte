<!--
  The marketplace settings, opened from the head of the listing: the switch
  that stops this server reading the marketplace, the switch that lists its
  community extensions, how the last read went, and the way to the
  marketplace itself.

  Community extensions, the trusted and the unverified ones, are listed only
  once an organization administrator switches them on. The official ones are
  always listed.

  The switch shows what the server holds, and a change is drawn once the
  server has applied it, as the network room's hidden switch is. Only an
  organization administrator may move it, so for anybody else it is drawn
  disabled with the reason beside it. A server whose configuration file
  switches the marketplace off says how to switch it on there, and has no
  switch, because the page cannot change that file.

  A marketplace that did not answer the last read is said with the server's
  reason, and with when the copy still listed was read, if there is one.
-->
<script lang="ts">
  import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
  import {
    describeFault,
    formatDateTime,
    translateFor,
  } from '@prusa/connect-ui/i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    FaultAlert,
    SettingsRow,
    Switch,
  } from '@prusa/connect-ui/primitives';

  import { APP_NAMESPACE } from '../../i18n';
  import type { ModuleCatalogue } from '../../stores/module-catalogue.svelte';

  let { catalogue, id }: { catalogue: ModuleCatalogue; id: string } = $props();

  const t = translateFor(APP_NAMESPACE);

  const marketplace = $derived(catalogue.marketplace);
  const fault = $derived(
    catalogue.switchError === undefined
      ? undefined
      : describeFault(catalogue.switchError),
  );

  // The switch is bound through a getter and a setter, so it draws what the
  // server holds. A refused change leaves it where the server has it.
  const setEnabled = (next: boolean): void => {
    void catalogue.setMarketplace(next);
  };
  const setCommunity = (next: boolean): void => {
    void catalogue.setCommunity(next);
  };

  const readAt = $derived(
    marketplace?.read_at === undefined
      ? undefined
      : formatDateTime(marketplace.read_at),
  );

  // Unknown is left to the server, which refuses a person it does not allow.
  const locked = $derived(catalogue.mayAdministerOrg === false);
  const stale = $derived(
    marketplace !== undefined &&
      marketplace.enabled &&
      !marketplace.reachable &&
      marketplace.stale &&
      readAt !== undefined,
  );

  const switchId = $props.id();
  const communityId = `${switchId}-community`;
</script>

<!-- A pane container, because the settings row lays its switch beside its
     label by the width of the pane it sits in. -->
<section
  {id}
  class="@container/pane flex flex-col gap-2 rounded-md border-0 bg-secondary p-control-inline"
  aria-label={t('app.settings.extensions.marketplace.settings')}
  data-part="marketplace"
>
  {#if marketplace === undefined}
    <p class="text-sm text-muted-foreground" data-part="marketplace-none">
      {t('app.settings.extensions.marketplace.configOff')}
    </p>
  {:else}
    <SettingsRow
      layout="switch"
      label={t('app.settings.extensions.marketplace.enabled.label')}
      description={t('app.settings.extensions.marketplace.enabled.note', {
        url: marketplace.url,
      })}
    >
      {#snippet control()}
        <Switch
          id={switchId}
          aria-label={t('app.settings.extensions.marketplace.enabled.label')}
          bind:checked={() => marketplace?.enabled ?? false, setEnabled}
          disabled={locked ||
            catalogue.switching ||
            catalogue.acting !== undefined}
          data-part="marketplace-switch"
        />
      {/snippet}
    </SettingsRow>

    {#if marketplace.enabled}
      <SettingsRow
        layout="switch"
        label={t('app.settings.extensions.marketplace.community.label')}
        description={t('app.settings.extensions.marketplace.community.note')}
      >
        {#snippet control()}
          <Switch
            id={communityId}
            aria-label={t(
              'app.settings.extensions.marketplace.community.label',
            )}
            bind:checked={() => marketplace?.community ?? false, setCommunity}
            disabled={locked ||
              catalogue.switching ||
              catalogue.acting !== undefined}
            data-part="community-switch"
          />
        {/snippet}
      </SettingsRow>
    {/if}

    {#if locked}
      <p class="text-sm text-muted-foreground" data-part="marketplace-locked">
        {t('app.settings.extensions.marketplace.locked')}
      </p>
    {/if}

    <div
      class="flex flex-wrap items-center justify-between gap-2 text-sm"
      aria-live="polite"
    >
      <span class="text-muted-foreground" data-part="marketplace-status">
        {#if !marketplace.enabled}
          {t('app.settings.extensions.marketplace.off')}
        {:else if readAt !== undefined && marketplace.reachable}
          {t('app.settings.extensions.marketplace.read', { when: readAt })}
        {/if}
      </span>
      {#if marketplace.enabled}
        <a
          class="inline-flex items-center gap-1 rounded-sm underline underline-offset-2 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
          href={marketplace.browse_url}
          target="_blank"
          rel="noopener noreferrer"
          data-part="browse-marketplace"
        >
          {t('app.settings.extensions.marketplace.browse')}
          <ExternalLinkIcon aria-hidden="true" class="size-3.5" />
        </a>
      {/if}
    </div>

    {#if marketplace.enabled && !marketplace.reachable}
      <Alert
        variant="warning"
        data-part="marketplace-unreachable"
        data-stale={stale || undefined}
      >
        <AlertTitle>
          {stale
            ? t('app.settings.extensions.catalogue.stale', {
                when: readAt ?? '',
              })
            : t('app.settings.extensions.catalogue.unreachable')}
        </AlertTitle>
        {#if marketplace.problem}
          <AlertDescription>{marketplace.problem}</AlertDescription>
        {/if}
      </Alert>
    {/if}
  {/if}

  <FaultAlert {fault} />
</section>
