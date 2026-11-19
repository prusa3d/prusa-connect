<script lang="ts">
  import { formatBytes, formatDateTime, formatNumber, t } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    Badge,
    Button,
    Card,
    CardContent,
    Checkbox,
    Input,
    Label,
    Progress,
    RadioGroup,
    RadioGroupItem,
    SettingsPane,
  } from '../../primitives';
  import { reviewsNeedingApproval, type UpgradeJournal } from '../../data';
  import ExtensionReviews from './extension-reviews.svelte';
  import ExternalRestore from './external-restore.svelte';
  import type { MaintenanceSource } from './types';

  let { source }: { source: MaintenanceSource } = $props();

  let schedule = $state<'now' | 'when-idle' | 'at'>('when-idle');
  let scheduledFor = $state('');
  let forcePrinting = $state(false);
  let postgresAck = $state(false);
  // The approval names the reviews it was given for. When the server reports
  // other reviews, the box is unticked again and the operator reads them first.
  let approvedFor = $state<string | undefined>(undefined);

  const working = $derived(source.busy !== undefined);
  const printing = $derived(source.upgrade?.preflight?.printing ?? []);
  const reviews = $derived(source.upgrade?.extension_reviews ?? []);
  const needingApproval = $derived(reviewsNeedingApproval(source.upgrade));
  const approvalKey = $derived(JSON.stringify(needingApproval));
  const approved = $derived(
    needingApproval.length > 0 && approvedFor === approvalKey,
  );
  // What the server accepts next: prepare before the rehearsal, apply only
  // on a rehearsed update, and on a finished or scheduled one only cancel,
  // which clears the record. On a finished one that changes nothing
  // installed, so the button says dismiss.
  const stage = $derived.by(
    (): 'prepare' | 'apply' | 'cancel' | 'dismiss' | 'none' => {
      switch (source.upgrade?.state) {
        case 'staged':
        case 'verified':
        case 'preflighted':
          return 'prepare';
        case 'rehearsed':
          return 'apply';
        case 'scheduled':
          return 'cancel';
        case 'committed':
        case 'rolled-back':
        case 'failed-before-swap':
          return 'dismiss';
        default:
          return 'none';
      }
    },
  );
  // Asked only where the server would refuse without it: a PostgreSQL
  // database the update's backup does not hold.
  const postgresAckRequired = $derived(
    source.version?.postgres_backup_ack_required === true,
  );
  const canApply = $derived(
    stage === 'apply' &&
      (needingApproval.length === 0 || approved) &&
      (!postgresAckRequired || postgresAck) &&
      (schedule !== 'at' ||
        (scheduledFor !== '' &&
          !Number.isNaN(new Date(scheduledFor).getTime()))),
  );

  const progressOf = (upgrade: UpgradeJournal): number => {
    if (
      upgrade.state === 'committed' ||
      upgrade.state === 'rolled-back' ||
      upgrade.state === 'failed-before-swap'
    )
      return 100;
    if (upgrade.schedule !== undefined) return 90;
    if (upgrade.rehearsal !== undefined) return 75;
    if (upgrade.preflight !== undefined) return 50;
    if (upgrade.verified !== undefined) return 25;

    return 10;
  };

  const apply = (): void => {
    const at =
      schedule === 'at' && scheduledFor !== ''
        ? new Date(scheduledFor).toISOString()
        : undefined;
    void source.apply({
      schedule,
      ...(at === undefined ? {} : { scheduled_for: at }),
      force_while_printing: forcePrinting,
      postgres_backup_ack: postgresAck,
      ...(needingApproval.length > 0 && approved
        ? { approve_extensions: true }
        : {}),
    });
  };
</script>

{#snippet buttons()}
  {#if stage === 'prepare'}
    <Button disabled={working} onclick={() => void source.prepare()}>
      {t('settingsMaintenance.action.prepare')}
    </Button>
  {/if}
  {#if stage === 'apply'}
    <Button
      disabled={working ||
        !canApply ||
        source.version?.managed_externally === true}
      onclick={apply}
    >
      {t('settingsMaintenance.action.apply')}
    </Button>
  {/if}
  <Button
    variant="secondary"
    disabled={working}
    onclick={() => void source.cancel()}
  >
    {stage === 'dismiss'
      ? t('settingsMaintenance.action.dismiss')
      : t('settingsMaintenance.action.cancel')}
  </Button>
{/snippet}

{#snippet stateBadge()}
  <Badge>{source.upgrade?.state}</Badge>
{/snippet}

{#if source.upgrade}
  <SettingsPane
    class="w-full"
    title={t('settingsMaintenance.status.heading')}
    headerEnd={stateBadge}
    actions={stage === 'none' ? undefined : buttons}
  >
    <div
      class="flex flex-col gap-section-gap p-panel-padding-mobile @lg/pane:p-panel-padding"
    >
      <div class="flex flex-col gap-2">
        <p class="text-body">
          <span class="font-mono">{source.upgrade.from_version}</span>
          →
          <span class="font-mono">{source.upgrade.to_version}</span>
        </p>
        <Progress
          value={progressOf(source.upgrade)}
          aria-label={t('settingsMaintenance.status.progress')}
        />
        <div
          class="flex flex-wrap justify-between gap-2 text-caption text-muted-foreground"
        >
          <span>
            {t('settingsMaintenance.status.revision', {
              revision: formatNumber(source.upgrade.revision),
            })}
          </span>
          <span>
            {t('settingsMaintenance.status.updated', {
              at: formatDateTime(source.upgrade.updated_at),
            })}
          </span>
        </div>
      </div>

      {#if source.upgrade.failure}
        <Alert variant="destructive">
          <AlertDescription>{source.upgrade.failure}</AlertDescription>
        </Alert>
      {/if}

      <ExternalRestore databases={source.upgrade.external_restore ?? []} />

      {#if source.upgrade.preflight}
        <div class="text-body flex flex-col gap-1">
          <h3 class="font-medium">
            {t('settingsMaintenance.preflight.heading')}
          </h3>
          <p class="text-muted-foreground">
            {t('settingsMaintenance.preflight.disk', {
              available: formatBytes(source.upgrade.preflight.disk_available),
              required: formatBytes(source.upgrade.preflight.disk_required),
            })}
          </p>
          {#if source.upgrade.preflight.managed_reason}
            <p>{source.upgrade.preflight.managed_reason}</p>
          {/if}
          {#if (source.upgrade.preflight.warnings ?? []).length > 0}
            <ul class="list-disc ps-5">
              {#each source.upgrade.preflight.warnings ?? [] as warning (warning)}
                <li>{warning}</li>
              {/each}
            </ul>
          {/if}
          {#if (source.upgrade.preflight.module_impacts ?? []).length > 0}
            <h4 class="mt-2 font-medium">
              {t('settingsMaintenance.preflight.modules')}
            </h4>
            <ul class="flex flex-col gap-1">
              {#each source.upgrade.preflight.module_impacts ?? [] as impact (impact.module)}
                <li>
                  <span class="font-mono">{impact.module}</span>:
                  {impact.outcome}{impact.reason ? ` — ${impact.reason}` : ''}
                  {#if impact.needed_to_start}
                    <Badge variant="warning">
                      {t('settingsMaintenance.preflight.neededToStart')}
                    </Badge>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
          {#if printing.length > 0}
            <h4 class="mt-2 font-medium">
              {t('settingsMaintenance.preflight.printing')}
            </h4>
            <ul class="list-disc ps-5">
              {#each printing as printer (printer)}
                <li class="font-mono">{printer}</li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}

      {#if reviews.length > 0}
        <ExtensionReviews {reviews} />
      {/if}

      {#if stage === 'apply'}
        <RadioGroup
          bind:value={schedule}
          aria-labelledby="upgrade-schedule-legend"
          class="gap-2"
        >
          <div id="upgrade-schedule-legend" class="text-body mb-1 font-medium">
            {t('settingsMaintenance.apply.when')}
          </div>
          <Label variant="option">
            <RadioGroupItem value="now" />
            {t('settingsMaintenance.apply.now')}
          </Label>
          <Label variant="option">
            <RadioGroupItem value="when-idle" />
            {t('settingsMaintenance.apply.idle')}
          </Label>
          <Label variant="option">
            <RadioGroupItem value="at" />
            {t('settingsMaintenance.apply.scheduled')}
          </Label>
        </RadioGroup>
        {#if schedule === 'at'}
          <div class="max-w-xs">
            <Label for="maintenance-scheduled">
              {t('settingsMaintenance.apply.time')}
            </Label>
            <Input
              id="maintenance-scheduled"
              type="datetime-local"
              value={scheduledFor}
              oninput={(event: Event & { currentTarget: HTMLInputElement }) => {
                scheduledFor = event.currentTarget.value;
              }}
            />
          </div>
        {/if}
        {#if printing.length > 0}
          <Label variant="option">
            <Checkbox bind:checked={forcePrinting} class="mt-0.5" />
            <span>{t('settingsMaintenance.apply.forcePrinting')}</span>
          </Label>
        {/if}
        {#if needingApproval.length > 0}
          <Card tone="attention" data-part="extension-approval">
            <CardContent class="flex flex-col gap-2 text-sm">
              <Label variant="option">
                <Checkbox
                  bind:checked={
                    () => approved,
                    (ticked) => {
                      approvedFor = ticked ? approvalKey : undefined;
                    }
                  }
                  class="mt-0.5"
                />
                <span>{t('settingsMaintenance.apply.approve')}</span>
              </Label>
              <ul class="flex flex-col gap-1 ps-6">
                {#each needingApproval as review (review.module)}
                  <li>
                    <span class="font-mono break-all">{review.module}</span>
                    {#if review.downgrade}
                      <span class="block text-muted-foreground">
                        {t('settingsMaintenance.apply.approveOlder', {
                          installed: review.installed_version ?? '',
                          version: review.version,
                        })}
                      </span>
                    {/if}
                    {#if (review.new_asks ?? []).length > 0}
                      <span class="block text-muted-foreground">
                        {t('settingsMaintenance.apply.approveAsks', {
                          asks: (review.new_asks ?? []).join(', '),
                        })}
                      </span>
                    {/if}
                  </li>
                {/each}
              </ul>
            </CardContent>
          </Card>
        {/if}
        {#if postgresAckRequired}
          <Label variant="option">
            <Checkbox bind:checked={postgresAck} class="mt-0.5" />
            <span>{t('settingsMaintenance.apply.postgres')}</span>
          </Label>
        {/if}
        {#if source.version?.managed_externally === true}
          <p class="text-body text-muted-foreground" data-part="apply-managed">
            {t('settingsMaintenance.apply.managed', {
              reason: source.version.managed_reason ?? '',
            })}
          </p>
        {/if}
      {/if}
    </div>
  </SettingsPane>
{/if}
