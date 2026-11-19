<!-- One printer, and everything this product knows about it. -->
<script lang="ts">
  import CameraIcon from '@lucide/svelte/icons/camera';
  import FolderIcon from '@lucide/svelte/icons/folder';
  import HammerIcon from '@lucide/svelte/icons/hammer';
  import HistoryIcon from '@lucide/svelte/icons/history';
  import InfoIcon from '@lucide/svelte/icons/info';
  import LayersIcon from '@lucide/svelte/icons/layers';
  import ListIcon from '@lucide/svelte/icons/list';
  import PuzzleIcon from '@lucide/svelte/icons/puzzle';
  import SlidersHorizontalIcon from '@lucide/svelte/icons/sliders-horizontal';
  import VideoIcon from '@lucide/svelte/icons/video';

  import { faultOf } from '../../data';
  import {
    PRINTER_STATE_LABEL_KEY,
    printerStateOf,
    STATE_PATH,
    textAt,
  } from '../../domain';
  import { describeFault, t } from '../../i18n';
  import {
    Badge,
    Card,
    CardContent,
    DetailHeader,
    DetailTabs,
    EmptyState,
    FaultAlert,
    NoLongerHere,
    PageTrail,
    PrinterIcon,
    Spinner,
    type DetailTab,
  } from '../../primitives';
  import {
    printerArtworkOf,
    retiredReasonOf,
    stateBadgeVariant,
  } from '../printers/presentation';
  import { mayAdminister } from './info/permission';
  import PrinterRename from './info/printer-rename.svelte';
  import PrinterSwitcher from './printer-switcher.svelte';
  import FilamentPositions from './filament-positions.svelte';
  import TelemetryChips from './telemetry-chips.svelte';
  import { PRINTER_DETAIL_TABS } from './types';
  import type { PrinterDetailScreenProps, PrinterDetailTabId } from './types';

  let {
    deviceId,
    device,
    telemetry,
    commands,
    peer,
    printerCount,
    onSelectPeer,
    tab,
    onTabChange,
    panels,
    moduleTabs,
    filamentPositions,
    actions,
    prompt,
    fault,
    loading = false,
    administration,
    printersHref,
  }: PrinterDetailScreenProps = $props();

  // The path always names the printer.
  const name = $derived(device?.name ?? deviceId);
  const waiting = $derived(loading || device === undefined);

  // The fleet answered and holds no such printer, or refused it. Any other
  // refusal is a fault to show, not a printer that is gone.
  const missing = $derived(
    !loading &&
      device === undefined &&
      (fault === undefined || [403, 404].includes(faultOf(fault).status ?? 0)),
  );

  // A disconnected machine reports nothing.
  const state = $derived(
    device?.connected === true
      ? printerStateOf(textAt(device, STATE_PATH) ?? 'UNKNOWN')
      : 'OFFLINE',
  );
  const refusal = $derived(
    fault === undefined ? undefined : describeFault(fault),
  );

  // Controls holds the printer commands, so `commands` is never a tab of its
  // own. It stays an id the host can ask for: it opens Controls with its
  // Advanced part requested.
  type ShownTabId = Exclude<PrinterDetailTabId, 'commands'>;

  const label: Record<ShownTabId, string> = $derived({
    status: t('printerDetail.tab.status'),
    controls: t('printerDetail.tab.controls'),
    files: t('printerDetail.tab.files'),
    cameras: t('printerDetail.tab.cameras'),
    info: t('printerDetail.tab.info'),
    history: t('printerDetail.tab.history'),
  });

  const icons: Record<ShownTabId, DetailTab['icon']> = {
    status: CameraIcon,
    controls: SlidersHorizontalIcon,
    files: FolderIcon,
    cameras: VideoIcon,
    info: InfoIcon,
    history: HistoryIcon,
  };
  const moduleIcons = new Map<string, DetailTab['icon']>([
    ['tools', LayersIcon],
    ['logs', ListIcon],
  ]);

  // A retired printer's page is its record: no tab sends a command to it.
  const retired = $derived(device?.retired);
  const RETIRED_TABS: readonly PrinterDetailTabId[] = ['info', 'history'];

  const tabs: DetailTab[] = $derived([
    ...PRINTER_DETAIL_TABS.filter(
      (id): id is ShownTabId =>
        (retired === undefined || RETIRED_TABS.includes(id)) &&
        // The Cameras tab is drawn only where the page hands it a panel: a
        // tab that would only say this build draws nothing is left out.
        (id !== 'cameras' || panels?.cameras !== undefined) &&
        id !== 'commands',
    ).map((id) => ({
      id,
      label: label[id],
      icon: icons[id],
      panel: panels?.[id],
    })),
    ...(retired === undefined
      ? (moduleTabs ?? []).map((row) => ({
          ...row,
          icon:
            row.icon ??
            moduleIcons.get(row.id.slice(row.id.lastIndexOf('.') + 1)) ??
            PuzzleIcon,
        }))
      : []),
  ]);
  const active = $derived.by(() => {
    if (
      retired !== undefined &&
      !RETIRED_TABS.includes(tab as PrinterDetailTabId)
    )
      return 'history';
    return tab === 'commands' ? 'controls' : tab;
  });

  const admin = $derived(
    device !== undefined && retired === undefined && mayAdminister(device)
      ? administration
      : undefined,
  );

  // The pair is the only shape with a switcher.
  const pair = $derived.by(() => {
    if (device === undefined || peer === undefined || printerCount !== 2) {
      return undefined;
    }

    return peer.side === 'before'
      ? [peer.device, device]
      : [device, peer.device];
  });
</script>

<div class="printer-detail mx-auto flex w-full max-w-7xl flex-col gap-6">
  <PageTrail
    trail={[{ label: t('printerDetail.printers'), href: printersHref }]}
    page={name}
  />

  {#if pair && device}
    <PrinterSwitcher
      printers={pair}
      chosenId={device.id}
      onChoose={() => onSelectPeer?.()}
    />
  {/if}

  {#if !missing}
    <FaultAlert fault={refusal} />
  {/if}

  {#if missing}
    <NoLongerHere
      headingLevel="h1"
      heading={t('printerDetail.missing.heading')}
      description={t('printerDetail.missing.detail')}
      linkLabel={t('printerDetail.printers')}
      href={printersHref}
    />
  {:else}
    <DetailHeader
      title={name}
      loading={waiting}
      {media}
      {titleAside}
      {meta}
      actions={retired ? undefined : actions}
    />

    {#if retired}
      <p class="text-sm text-muted-foreground">
        {retiredReasonOf(retired.reason)}
      </p>
    {/if}

    {#if prompt}{@render prompt()}{/if}

    <DetailTabs
      class={device && !retired ? 'mt-section-gap' : undefined}
      segmented
      {tabs}
      {active}
      onChange={(id: string) => onTabChange(id)}
      fallback={notBuilt}
    />
  {/if}
</div>

{#snippet media()}
  {#if device}
    <PrinterIcon
      model={printerArtworkOf(device)}
      reported={device.model}
      class="size-32"
    />
  {/if}
{/snippet}

{#snippet titleAside()}
  {#if device}
    <Badge
      mono
      size="md"
      variant={retired ? 'secondary' : stateBadgeVariant(state)}
    >
      {retired ? t('printers.retired') : t(PRINTER_STATE_LABEL_KEY[state])}
    </Badge>
    {#if commands.readingDrive && !retired}
      <Badge variant="secondary" size="md">
        <Spinner />
        {t('printerDetail.readingDrive')}
      </Badge>
    {/if}
    {#if admin}
      <PrinterRename name={device.name} administration={admin} trigger="icon" />
    {/if}
  {/if}
{/snippet}

{#snippet meta()}
  {#if device && !retired}
    <TelemetryChips {telemetry} {device} {commands} />
    {#key device.id}
      {@const shown = device}
      {#snippet defaultPositions()}
        <FilamentPositions device={shown} {telemetry} />
      {/snippet}
      {#if filamentPositions}
        {@render filamentPositions(defaultPositions)}
      {:else}
        {@render defaultPositions()}
      {/if}
    {/key}
  {/if}
{/snippet}

{#snippet notBuilt()}
  <Card>
    <CardContent>
      <EmptyState
        icon={HammerIcon}
        heading={t('printerDetail.panel.notBuilt')}
        headingLevel="h2"
        description={t('printerDetail.panel.notBuiltDetail')}
      />
    </CardContent>
  </Card>
{/snippet}

<style lang="postcss">
  @reference '../../styles/index.css';

  .printer-detail
    :global(
      [data-slot='tabs-content']
        [data-slot='card']:not([data-slot='card'] [data-slot='card'])
    ) {
    @apply py-0 @[24rem]/card:py-0;

    > :global(:is([data-slot='card-header'], [data-slot='card-content'])) {
      @apply px-panel-padding-mobile @lg/card:px-panel-padding;
    }

    > :global(
      :is([data-slot='card-header'], [data-slot='card-content']):first-child
    ) {
      @apply pt-section-block;
    }

    > :global([data-slot='card-content']:last-child) {
      @apply pb-panel-padding-mobile @lg/card:pb-panel-padding;
    }
  }
</style>
