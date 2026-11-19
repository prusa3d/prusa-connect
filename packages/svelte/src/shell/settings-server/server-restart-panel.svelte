<!--
  Restarting the server, by hand. A normal run offers the restart into safe
  mode behind a confirmation, because it stops every connection and module;
  a safe-mode run offers the one way out. The page draws what the data layer
  reports and asks it nothing: `phase` says how far a restart has got.
-->
<script lang="ts">
  import { describeFault, t } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    Button,
    ConfirmDialog,
    FaultAlert,
    SettingsPane,
  } from '../../primitives';
  import type { ServerRestartPanelProps } from './types';

  let {
    safeMode,
    allowed,
    phase,
    refusal,
    error,
    readError,
    onRetryRead,
    confirming = $bindable(false),
    onRestart,
  }: ServerRestartPanelProps = $props();

  const busy = $derived(phase === 'sending' || phase === 'waiting');
  const fault = $derived(
    phase === 'failed' && error !== undefined && refusal === undefined
      ? describeFault(error)
      : undefined,
  );
  const readFault = $derived(
    allowed === undefined && readError !== undefined
      ? describeFault(readError)
      : undefined,
  );
  const PROGRESS = {
    sending: 'settingsServer.restart.sending',
    waiting: 'settingsServer.restart.waiting',
    timeout: 'settingsServer.restart.timeout',
  } as const;
  const progress = $derived.by(() => {
    if (refusal === 'upgrade-busy') return 'settingsServer.restart.conflict';
    if (phase === 'sending' || phase === 'waiting' || phase === 'timeout')
      return PROGRESS[phase];
    return undefined;
  });
</script>

{#snippet safeTrigger({ props }: { props: Record<string, unknown> })}
  <Button {...busy ? {} : props} variant="secondary" aria-disabled={busy}>
    {t('settingsServer.restart.safe')}
  </Button>
{/snippet}

{#snippet restartActions()}
  {#if safeMode}
    <Button
      aria-disabled={busy}
      aria-busy={busy}
      onclick={() => {
        if (!busy) onRestart('normal');
      }}
    >
      {t('settingsServer.restart.normal')}
    </Button>
  {:else}
    <ConfirmDialog
      bind:open={confirming}
      trigger={safeTrigger}
      title={t('settingsServer.restart.confirmTitle')}
      description={t('settingsServer.restart.confirm')}
      confirmLabel={t('settingsServer.restart.safe')}
      onConfirm={() => {
        confirming = false;
        onRestart('safe');
      }}
    />
  {/if}
{/snippet}

<SettingsPane
  class="w-full"
  title={t('settingsServer.restart.title')}
  actions={allowed === true && refusal !== 'forbidden'
    ? restartActions
    : undefined}
>
  <div
    class="flex flex-col gap-form-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    <p class="text-body">
      {safeMode
        ? t('settingsServer.restart.safeOn')
        : t('settingsServer.restart.description')}
    </p>
    {#if allowed === false || refusal === 'forbidden'}
      <p class="text-body text-muted-foreground">
        {t('settingsServer.restart.locked')}
      </p>
    {:else if allowed !== true}
      {#if readFault !== undefined}
        <p class="text-body text-muted-foreground">
          {t('settingsServer.restart.unread')}
        </p>
      {/if}
    {:else if !safeMode}
      <p class="text-body text-muted-foreground">
        {t('settingsServer.restart.safeHelp')}
      </p>
    {/if}
    {#if progress !== undefined}
      <Alert
        variant={phase === 'timeout' || refusal === 'upgrade-busy'
          ? 'warning'
          : 'info'}
        role="status"
      >
        <AlertDescription>{t(progress)}</AlertDescription>
      </Alert>
    {/if}
    <FaultAlert {fault} />
    <FaultAlert fault={readFault}>
      {#snippet actions()}
        <Button variant="secondary" onclick={onRetryRead}>
          {t('settingsServer.restart.retry')}
        </Button>
      {/snippet}
    </FaultAlert>
  </div>
</SettingsPane>
