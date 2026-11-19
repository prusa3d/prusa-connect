<script lang="ts">
  import ClockIcon from '@lucide/svelte/icons/clock';

  import {
    PAIRING_NEEDED_FAULT,
    UPGRADE_SERVER_LIMITED_FAULT,
  } from '../../data';
  import {
    describeFault,
    formatBytes,
    formatDateTime,
    formatDuration,
    t,
  } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Button,
    FaultAlert,
    SettingsPane,
    Skeleton,
  } from '../../primitives';
  import { PortalPairingWizard } from '../portal-pairing';
  import type { MaintenanceSource } from './types';

  let { source }: { source: MaintenanceSource } = $props();

  // The clock the wait is counted on. It ticks only while the update server
  // asks this server to wait.
  let now = $state(Date.now());

  const working = $derived(source.busy !== undefined);
  const limited = $derived(
    source.releaseError?.code === UPGRADE_SERVER_LIMITED_FAULT,
  );
  const waitSeconds = $derived(
    source.releaseRetryAt === undefined
      ? 0
      : Math.max(0, Math.ceil((source.releaseRetryAt - now) / 1000)),
  );
  // formatDuration drops the seconds of a longer wait. Rounding up to the
  // minute keeps the shown time from ending before the real one.
  const shownWait = $derived(
    waitSeconds < 60 ? waitSeconds : Math.ceil(waitSeconds / 60) * 60,
  );
  // The pairing panel above already says what to do, so the release list
  // only points at it instead of repeating the refusal.
  const unpaired = $derived(
    source.releaseError?.code === PAIRING_NEEDED_FAULT &&
      source.portal !== undefined,
  );
  const releaseFault = $derived(
    source.releaseError === undefined || limited || unpaired
      ? undefined
      : describeFault(source.releaseError),
  );

  $effect(() => {
    const until = source.releaseRetryAt;
    if (until === undefined) return;
    now = Date.now();
    const timer = setInterval(() => {
      now = Date.now();
      if (now >= until) clearInterval(timer);
    }, 1000);

    return () => {
      clearInterval(timer);
    };
  });
</script>

<SettingsPane class="w-full" title={t('settingsMaintenance.online.heading')}>
  <div
    class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
  >
    {#if source.portal === undefined}
      <p class="text-body text-muted-foreground">
        {t('settingsMaintenance.delivery.unavailable')}
      </p>
    {:else}
      <PortalPairingWizard pairing={source.portal} />
    {/if}

    <p class="text-body text-muted-foreground">
      {t('settingsMaintenance.releases.description')}
    </p>
    {#if source.releaseLoading}
      <div class="flex flex-col gap-2" aria-busy="true">
        <Skeleton class="h-20 w-full" />
        <Skeleton class="h-20 w-full" />
      </div>
    {:else if limited}
      <div class="flex flex-col gap-3" data-part="release-limited">
        <Alert variant="warning">
          <ClockIcon aria-hidden="true" />
          <AlertTitle>{t('settingsMaintenance.releases.limited')}</AlertTitle>
          <AlertDescription>
            {waitSeconds > 0
              ? t('settingsMaintenance.releases.limitedWait', {
                  wait: formatDuration(shownWait),
                })
              : t('settingsMaintenance.releases.limitedOver')}
          </AlertDescription>
        </Alert>
        <Button
          class="self-start"
          variant="secondary"
          disabled={working || waitSeconds > 0}
          onclick={() => void source.refreshReleases()}
        >
          {t('settingsMaintenance.releases.retry')}
        </Button>
      </div>
    {:else if unpaired}
      <p class="text-body text-muted-foreground" data-part="release-unpaired">
        {t('settingsMaintenance.releases.unpaired')}
      </p>
    {:else if releaseFault}
      <div class="flex flex-col gap-3">
        <FaultAlert fault={releaseFault} />
        <Button
          class="self-start"
          variant="secondary"
          disabled={working}
          onclick={() => void source.refreshReleases()}
        >
          {t('settingsMaintenance.releases.retry')}
        </Button>
      </div>
    {:else if source.releases.length === 0}
      <p class="text-body text-muted-foreground">
        {t('settingsMaintenance.releases.empty')}
      </p>
    {:else}
      <div class="divide-y divide-border">
        {#each source.releases as release (release.version)}
          <article class="py-3 first:pt-0 last:pb-0">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 class="text-body font-medium">{release.name}</h3>
                <p class="text-caption text-muted-foreground">
                  {formatDateTime(release.published_at)}
                  · {formatBytes(release.binary.size)}
                </p>
              </div>
              <Button
                size="sm"
                disabled={working}
                onclick={() => void source.stage(release)}
              >
                {t('settingsMaintenance.action.stage')}
              </Button>
            </div>
            {#if release.notes}
              <p class="text-body mt-2 whitespace-pre-line">{release.notes}</p>
            {/if}
          </article>
        {/each}
      </div>
    {/if}
  </div>
</SettingsPane>
