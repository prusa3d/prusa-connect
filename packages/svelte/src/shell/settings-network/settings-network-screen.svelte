<!--
  The network room: whether slicers and apps on the same network find this
  server, and what it announces to them. The switch shows what the server
  holds, and a change is drawn once the server has applied it. A refusal,
  such as a viewer who may not change it, is shown by its code.
-->
<script lang="ts">
  import { describeFault, t } from '../../i18n';
  import BookOpenIcon from '@lucide/svelte/icons/book-open';

  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Button,
    FaultAlert,
    KeyValueList,
    type KeyValueRow,
    SettingsPane,
    SettingsRow,
    Skeleton,
    Switch,
  } from '../../primitives';
  import type { SettingsNetworkScreenProps } from './types';
  import { networkVerdict } from './verdict';

  let { source, timeServer, docsHref }: SettingsNetworkScreenProps = $props();

  // Every key is written out rather than assembled from a value.
  const ANNOUNCEMENT: Readonly<Record<string, string>> = {
    announcing: 'settingsNetwork.announcement.announcing',
    hidden: 'settingsNetwork.announcement.hidden',
    off: 'settingsNetwork.announcement.off',
    failed: 'settingsNetwork.announcement.failed',
  };

  const described = $derived(source.described);
  const verdict = $derived(
    networkVerdict(source.announcement, described?.bridged),
  );

  const announcementText = $derived.by(() => {
    const key = verdict === undefined ? undefined : ANNOUNCEMENT[verdict];

    return key === undefined ? undefined : t(key);
  });

  // The names stay readable while the server is hidden, because they are what
  // showing it again announces.
  const announced = $derived.by((): KeyValueRow[] => {
    // A server that announces nothing has nothing to name.
    if (described?.host === undefined) return [];

    const addresses = described.addresses ?? [];
    const services = described.services ?? [];
    // The port is absent until the server has bound it.
    const address =
      described.port === undefined
        ? described.host
        : `${described.host}:${String(described.port)}`;

    return [
      {
        id: 'name',
        label: t('settingsNetwork.announced.name'),
        value: described.name,
      },
      {
        id: 'address',
        label: t('settingsNetwork.announced.address'),
        value: address,
        mono: true,
        copyable: true,
      },
      {
        id: 'addresses',
        label: t('settingsNetwork.announced.addresses'),
        value:
          addresses.length === 0
            ? t('settingsNetwork.announced.none')
            : addresses.join('\n'),
        mono: addresses.length > 0,
        wrap: addresses.length > 0,
      },
      {
        id: 'services',
        label: t('settingsNetwork.announced.services'),
        value: services.length === 0 ? undefined : services.join(', '),
        mono: true,
      },
    ];
  });

  const fault = $derived(
    source.error === undefined ? undefined : describeFault(source.error),
  );

  // The switch is bound through a getter and a setter, so it draws what the
  // server holds. A click asks for the change, and a refused change leaves
  // the switch where the server has it.
  const setHidden = (next: boolean): void => {
    void source.setHidden(next);
  };

  const switchId = $props.id();

  // Every key is written out rather than assembled from a value.
  const TIME_STATE: Readonly<Record<string, string>> = {
    serving: 'settingsNetwork.timeServer.serving',
    off: 'settingsNetwork.timeServer.off',
    failed: 'settingsNetwork.timeServer.failed',
  };

  // A failure with a known cause says what to do about it.
  const TIME_CAUSE: Readonly<Record<string, string>> = {
    permission: 'settingsNetwork.timeServer.failedPermission',
    in_use: 'settingsNetwork.timeServer.failedInUse',
  };

  const timeStateText = $derived.by(() => {
    const state = timeServer?.state;
    const cause = timeServer?.cause;
    let key = state === undefined ? undefined : TIME_STATE[state];
    if (state === 'failed' && cause !== undefined)
      key = TIME_CAUSE[cause] ?? key;

    // Every answer names the port, so the fallback only fills the type.
    return key === undefined
      ? undefined
      : t(key, { port: timeServer?.port ?? 123 });
  });

  const timeFault = $derived(
    timeServer?.error === undefined
      ? undefined
      : describeFault(timeServer.error),
  );

  const setTimeServer = (next: boolean): void => {
    void timeServer?.setEnabled(next);
  };

  const timeSwitchId = `${switchId}-time`;
</script>

<div class="flex w-full flex-col gap-4">
  <SettingsPane class="w-full" title={t('settingsNetwork.title')}>
    <div
      class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
    >
      {#if !source.loaded && fault === undefined}
        <div aria-busy="true" aria-label={t('settingsNetwork.loading')}>
          <Skeleton class="h-10 w-full" />
        </div>
      {:else}
        <SettingsRow
          layout="switch"
          label={t('settingsNetwork.hidden.label')}
          description={t('settingsNetwork.hidden.note')}
        >
          {#snippet control()}
            <Switch
              id={switchId}
              aria-label={t('settingsNetwork.hidden.label')}
              bind:checked={() => source.hidden, setHidden}
              disabled={source.saving || !source.loaded}
            />
          {/snippet}
        </SettingsRow>

        {#if announcementText}
          <p
            class="max-w-3xl text-caption text-muted-foreground"
            aria-live="polite"
          >
            {announcementText}
          </p>
        {/if}

        {#if verdict === 'bridged'}
          <Alert variant="warning" data-part="bridged">
            <AlertTitle>{t('settingsNetwork.bridged.title')}</AlertTitle>
            <AlertDescription
              >{t('settingsNetwork.bridged.body')}</AlertDescription
            >
          </Alert>
        {/if}
      {/if}

      <FaultAlert {fault} />
    </div>
  </SettingsPane>

  {#if announced.length > 0}
    <SettingsPane class="w-full" title={t('settingsNetwork.announced.heading')}>
      <div class="p-panel-padding-mobile @lg/pane:p-panel-padding">
        <KeyValueList rows={announced} />
      </div>
    </SettingsPane>
  {/if}

  {#if timeServer !== undefined}
    <SettingsPane
      class="w-full"
      title={t('settingsNetwork.timeServer.heading')}
    >
      <div
        class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
        data-part="time-server"
      >
        {#if !timeServer.loaded && timeFault === undefined}
          <div
            aria-busy="true"
            aria-label={t('settingsNetwork.timeServer.loading')}
          >
            <Skeleton class="h-10 w-full" />
          </div>
        {:else if timeServer.loaded}
          <SettingsRow
            layout="switch"
            label={t('settingsNetwork.timeServer.label')}
            description={t('settingsNetwork.timeServer.note')}
          >
            {#snippet control()}
              <Switch
                id={timeSwitchId}
                aria-label={t('settingsNetwork.timeServer.label')}
                bind:checked={() => timeServer.enabled, setTimeServer}
                disabled={timeServer.saving}
              />
            {/snippet}
          </SettingsRow>

          {#if timeServer.state === 'failed'}
            <Alert variant="warning" data-part="time-server-failed">
              <AlertTitle>{timeStateText}</AlertTitle>
              {#if timeServer.reason}
                <AlertDescription class="font-mono"
                  >{timeServer.reason}</AlertDescription
                >
              {/if}
              {#if timeServer.cause === 'permission' && docsHref !== undefined}
                {@const docs = docsHref}
                <AlertDescription>
                  <Button variant="link" size="inline-icon">
                    {#snippet child({ props })}
                      <a
                        {...props}
                        href={`${docs}install-from-a-release/#let-the-time-server-open-its-port`}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        <BookOpenIcon aria-hidden="true" />
                        {t('settingsNetwork.timeServer.permissionDocs')}
                      </a>
                    {/snippet}
                  </Button>
                </AlertDescription>
              {/if}
            </Alert>
          {:else if timeStateText}
            <p
              class="max-w-3xl text-caption text-muted-foreground"
              aria-live="polite"
            >
              {timeStateText}
            </p>
          {/if}
        {/if}

        <FaultAlert fault={timeFault} />
      </div>
    </SettingsPane>
  {/if}
</div>
