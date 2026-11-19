<!-- The Info tab: what this printer is, and what it has done. -->
<script lang="ts">
  import { FEATURES } from '../../../domain';
  import {
    describeFault,
    formatDuration,
    formatNumber,
    t,
  } from '../../../i18n';
  import {
    Badge,
    Card,
    CardAction,
    CardContent,
    CardHeader,
    CardTitle,
    FaultAlert,
    KeyValueList,
    Skeleton,
    type BadgeVariant,
    type KeyValueRow,
  } from '../../../primitives';
  import { printerProductNameOf } from '../../printers/presentation';
  import { verbsOf } from '../commands/types';
  import AcceptedCommands from './accepted-commands.svelte';
  import DangerZone from './danger-zone.svelte';
  import { mayAdminister, mayRemove } from './permission';
  import PrintHistory from './print-history.svelte';
  import PrinterRename from './printer-rename.svelte';
  import PrinterTags from './printer-tags.svelte';
  import PrinterTools from './printer-tools.svelte';
  import type { PrinterInfoPanelProps } from './types';

  let {
    device,
    history,
    recent,
    jobsHref,
    thumbnailFor,
    administration,
    onRemoved,
    onPickCommand,
    access,
    telemetry,
    commands,
    material,
  }: PrinterInfoPanelProps = $props();

  // A caller the record says may not administer the printer is offered no
  // control that the server would refuse.
  const admin = $derived(
    device !== undefined && mayAdminister(device) ? administration : undefined,
  );

  // A refused rename or tag write is drawn on the card that sent it.
  const recordFault = $derived(
    admin?.error !== undefined &&
      (admin.failed === 'rename' || admin.failed === 'tags')
      ? describeFault(admin.error)
      : undefined,
  );

  const rate = (value: number): string =>
    t('printerDetailInfo.link.perMinute', {
      rate: formatNumber(value, { maximumFractionDigits: 1 }),
    });

  const specification = $derived<KeyValueRow[]>(
    device === undefined
      ? []
      : [
          {
            id: 'model',
            label: t('printerDetailInfo.details.model'),
            value: printerProductNameOf(device.model),
          },
          {
            id: 'vendor',
            label: t('printerDetailInfo.details.vendor'),
            value: device.vendor,
          },
          {
            id: 'kind',
            label: t('printerDetailInfo.details.kind'),
            value: device.kind,
          },
          {
            id: 'firmware',
            label: t('printerDetailInfo.details.firmware'),
            value: device.firmware,
            mono: true,
            copyable: true,
          },
        ],
  );

  // What this instance knows it by.
  const instance = $derived<KeyValueRow[]>(
    device === undefined
      ? []
      : [
          {
            id: 'id',
            label: t('printerDetailInfo.details.id'),
            value: device.id,
            mono: true,
            copyable: true,
          },
          {
            id: 'serial',
            label: t('printerDetailInfo.details.serial'),
            value: device.serial,
            mono: true,
            copyable: true,
          },
          {
            id: 'unit',
            label: t('printerDetailInfo.details.unitUid'),
            value: device.unit_uid,
            mono: true,
            copyable: true,
          },
          {
            id: 'driver',
            label: t('printerDetailInfo.details.driver'),
            value: device.driver,
          },
        ],
  );

  const link = $derived.by<KeyValueRow[]>(() => {
    if (device === undefined) return [];
    const counters = device.link.counted ? device.link : undefined;

    return [
      {
        id: 'session',
        label: t('printerDetailInfo.link.session'),
        value: device.connected
          ? t('printerDetailInfo.link.sessionOpen')
          : t('printerDetailInfo.link.sessionClosed'),
      },
      {
        id: 'dropped',
        label: t('printerDetailInfo.link.droppedTelemetry'),
        value:
          counters === undefined
            ? undefined
            : formatNumber(counters.dropped_telemetry),
      },
      {
        id: 'drop-rate',
        label: t('printerDetailInfo.link.dropRate'),
        value:
          counters === undefined
            ? undefined
            : rate(counters.dropped_telemetry_per_minute),
      },
      {
        id: 'refused',
        label: t('printerDetailInfo.link.refusedFrames'),
        value:
          counters === undefined
            ? undefined
            : formatNumber(counters.refused_frames),
      },
      {
        id: 'refusal-rate',
        label: t('printerDetailInfo.link.refusalRate'),
        value:
          counters === undefined
            ? undefined
            : rate(counters.refused_frames_per_minute),
      },
      {
        id: 'window',
        label: t('printerDetailInfo.link.measuredOver'),
        value:
          counters === undefined
            ? undefined
            : formatDuration(counters.measured_over_seconds),
      },
    ];
  });

  // The capability list carries a verb's arguments on the same run of strings.
  const capabilities = $derived(
    verbsOf(device?.capabilities ?? null).map((entry) => entry.verb),
  );
  const issues = $derived(device?.issues ?? []);

  // Colour is never the only carrier of a severity (WCAG 1.4.1).
  const severityVariant: Readonly<Record<string, BadgeVariant>> = {
    error: 'destructive',
    info: 'info',
    warning: 'warning',
  };
  const severityText = $derived<Readonly<Record<string, string>>>({
    error: t('printerDetailInfo.issues.severity.error'),
    info: t('printerDetailInfo.issues.severity.info'),
    warning: t('printerDetailInfo.issues.severity.warning'),
  });
</script>

{#snippet heading(text: string)}
  <CardTitle><h2 class="contents">{text}</h2></CardTitle>
{/snippet}

<div class="@container/info flex flex-col gap-6">
  {#if device === undefined}
    <!-- The panel's own shape while the record is on its way. -->
    <Card role="status" aria-label={t('printerDetailInfo.loading')}>
      <CardHeader><Skeleton class="h-5 w-32" /></CardHeader>
      <CardContent class="flex flex-col gap-3">
        {#each [0, 1, 2, 3, 4, 5, 6, 7] as line (line)}
          <Skeleton class="h-4 w-full" />
        {/each}
      </CardContent>
    </Card>
  {:else}
    <Card>
      <CardHeader>
        {@render heading(t('printerDetailInfo.details.heading'))}
        {#if admin}
          <CardAction>
            <PrinterRename name={device.name} administration={admin} />
          </CardAction>
        {/if}
      </CardHeader>
      {#if recordFault}
        <CardContent><FaultAlert fault={recordFault} /></CardContent>
      {/if}
      <!-- Queried against the card's own container, which `Card` names. -->
      <CardContent
        class="grid grid-cols-1 gap-6 @2xl/card:grid-cols-2 @2xl/card:gap-0"
      >
        <section class="flex flex-col gap-4 @2xl/card:pe-panel-padding">
          <h3 class="text-sm font-medium text-foreground">
            {t('printerDetail.info.specification')}
          </h3>
          <KeyValueList rows={specification} />
        </section>
        <section
          class="flex flex-col gap-4 border-t border-border pt-6 @2xl/card:border-s @2xl/card:border-t-0 @2xl/card:ps-panel-padding @2xl/card:pt-0"
        >
          <h3 class="text-sm font-medium text-foreground">
            {t('printerDetail.info.instance')}
          </h3>
          <KeyValueList rows={instance} />
        </section>
        <!-- ADR-20260930-rq4. -->
        {#if FEATURES.tags}
          <section
            class="flex flex-col gap-4 border-t border-border pt-6 @2xl/card:col-span-2 @2xl/card:mt-6"
          >
            <h3 class="text-sm font-medium text-foreground">
              {t('printerDetailInfo.tags.heading')}
            </h3>
            <PrinterTags tags={device.tags ?? []} administration={admin} />
          </section>
        {/if}
      </CardContent>
    </Card>
  {/if}

  <!-- What the machine is fitted with, next to what it is. -->
  {#if device !== undefined}
    <PrinterTools
      tools={device.tools}
      buildVolume={device.build_volume}
      live={telemetry ? { device, telemetry, commands } : undefined}
      {material}
    />
  {/if}

  <!-- The prints stand between the record and the diagnostics. -->
  <PrintHistory
    {history}
    {recent}
    {jobsHref}
    live={device?.job}
    {thumbnailFor}
  />

  {#if device !== undefined}
    <div class="grid gap-6 @3xl/info:grid-cols-2">
      <Card>
        <CardHeader>
          {@render heading(t('printerDetailInfo.link.heading'))}
        </CardHeader>
        <CardContent><KeyValueList rows={link} /></CardContent>
      </Card>

      <Card>
        <CardHeader>
          {@render heading(t('printerDetailInfo.capabilities.heading'))}
          <CardAction>
            <Badge variant="secondary">
              {t('printerDetailInfo.capabilities.count', {
                count: capabilities.length,
              })}
            </Badge>
          </CardAction>
        </CardHeader>
        <CardContent class="flex flex-col gap-4">
          <KeyValueList
            rows={[
              {
                id: 'profile',
                label: t('printerDetailInfo.capabilities.profile'),
                value: device.profile_matched
                  ? t('printerDetailInfo.capabilities.matched')
                  : t('printerDetailInfo.capabilities.baseline'),
              },
            ]}
          />
          {#if capabilities.length === 0}
            <p class="text-sm text-muted-foreground">
              {t('printerDetailInfo.capabilities.none')}
            </p>
          {:else}
            <AcceptedCommands verbs={capabilities} {onPickCommand} />
          {/if}
        </CardContent>
      </Card>
    </div>

    <Card>
      <CardHeader>
        {@render heading(t('printerDetailInfo.issues.heading'))}
      </CardHeader>
      <CardContent>
        {#if issues.length === 0}
          <p class="text-sm text-muted-foreground">
            {t('printerDetailInfo.issues.none')}
          </p>
        {:else}
          <ul class="flex flex-col gap-3">
            {#each issues as issue (issue.reason)}
              <li class="flex flex-col gap-1">
                <div class="flex items-center gap-2">
                  <Badge
                    variant={severityVariant[issue.severity] ?? 'secondary'}
                  >
                    {severityText[issue.severity] ?? issue.severity}
                  </Badge>
                  <span class="font-mono text-sm">{issue.reason}</span>
                </div>
                {#if issue.detail}
                  <p class="text-sm text-muted-foreground">{issue.detail}</p>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </CardContent>
    </Card>
  {/if}

  {#if device !== undefined && mayAdminister(device) && access !== undefined}
    {@render access()}
  {/if}

  <!-- Last on the page, where the act nobody goes looking for belongs. -->
  {#if device !== undefined && admin !== undefined}
    <DangerZone
      name={device.name}
      administration={admin}
      removable={mayRemove(device)}
      {onRemoved}
    />
  {/if}
</div>
