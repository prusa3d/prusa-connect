<!--
  The heaters of one printer over the last minutes, as lines: what each one
  reads, and the target it is held to.

  The chart starts from what the server stored before the page opened and
  carries on with every frame after it, so it is full the moment it draws.
  Every chart of the page shares one stream (shared-telemetry.svelte.ts).
-->
<script lang="ts">
  import {
    BED_PATH,
    BED_TARGET_PATH,
    CHAMBER_PATH,
    CHAMBER_TARGET_PATH,
    NOZZLE_PATH,
    NOZZLE_TARGET_PATH,
  } from '../../../domain';
  import {
    type ChartSeries,
    EmptyState,
    LineChart,
    Skeleton,
    TilePanel,
  } from '../../../primitives';
  import ActivityIcon from '@lucide/svelte/icons/activity';
  import ThermometerIcon from '@lucide/svelte/icons/thermometer';
  import { untrack } from 'svelte';

  import type { PrinterView } from '../printers';
  import { t } from '../../../i18n';
  import { dashboardContext } from '../context';
  import { SHARED_TELEMETRY_MAX_DEVICES } from '../../../data';
  import { type Heater, temperatureSettings } from './config';
  import { HEATER } from './labels';
  import OnePrinter from './one-printer.svelte';
  import type { CardProps } from '../../../slots';

  let { config, title }: CardProps = $props();

  const { telemetry, clock, printers } = dashboardContext();

  const settings = $derived(temperatureSettings(config));
  const device = $derived(
    settings.device === '' ? undefined : printers.view(settings.device),
  );

  // The nozzle is the first head's, the head every stored nozzle reading of
  // the chart is kept under.
  const PATHS: Readonly<Record<Heater, { reading: string; target: string }>> = {
    nozzle: { reading: NOZZLE_PATH, target: NOZZLE_TARGET_PATH },
    bed: { reading: BED_PATH, target: BED_TARGET_PATH },
    chamber: { reading: CHAMBER_PATH, target: CHAMBER_TARGET_PATH },
  };

  /** The line of each heater's target, beside the line of its reading. */
  const TARGET_KEY: Readonly<Record<Heater, string>> = {
    nozzle: 'dashboard.temperature.nozzleTarget',
    bed: 'dashboard.temperature.bedTarget',
    chamber: 'dashboard.temperature.chamberTarget',
  };

  // The id alone, so a fleet update that leaves the printer the same does
  // not let the printer go and follow it again.
  const deviceId = $derived(device?.id);

  /** The printer this card follows, once the page took it. */
  let followed = $state<string | undefined>(undefined);
  /**
   * How many printers the page followed when it refused this card, or
   * undefined while it did not refuse it.
   */
  let refusedAt = $state<number | undefined>(undefined);

  // The printer is followed while this card shows it and let go when the card
  // goes or shows another printer. This is a subscription to the outside
  // world, which is what an effect is for (svelte-data.md).
  $effect(() => {
    const id = deviceId;
    if (id === undefined) return;
    // Taken outside the effect's graph, so following does not make the card
    // let its own printer go and take it again.
    const release = untrack(() => telemetry.follow(id));
    if (release === undefined) {
      // The count is read only while the page is full, so the effect runs
      // again, and the card tries again, once another card lets one go.
      refusedAt = telemetry.following;

      return () => {
        refusedAt = undefined;
      };
    }
    followed = id;

    return () => {
      release();
      followed = undefined;
    };
  });

  $effect(() => clock.hold());

  const since = $derived(clock.now - settings.minutes * 60_000);

  const series = $derived.by((): ChartSeries[] => {
    const id = followed;
    if (id === undefined) return [];

    return settings.heaters.flatMap((heater) => {
      const lines = [
        {
          id: heater,
          label: t(HEATER[heater]),
          points: telemetry
            .points(id, PATHS[heater].reading)
            .filter((point) => point.at >= since),
        },
        {
          id: `${heater}-target`,
          label: t(TARGET_KEY[heater]),
          points: telemetry
            .points(id, PATHS[heater].target)
            .filter((point) => point.at >= since),
        },
      ];
      // A printer with no chamber never reports one, so it gets no chamber
      // lines and no chamber entry in the legend.
      return heater === 'chamber' &&
        lines.every((line) => line.points.length === 0)
        ? []
        : lines;
    });
  });

  const empty = $derived(series.every((line) => line.points.length === 0));
</script>

<TilePanel
  {title}
  fallback={device === undefined
    ? t('dashboard.temperature.title')
    : t('dashboard.temperature.of', { name: device.name })}
>
  <OnePrinter
    device={settings.device}
    icon={ThermometerIcon}
    pickNote={t('dashboard.temperature.pickNote')}
  >
    {#snippet children(view: PrinterView)}
      {#if refusedAt !== undefined}
        <EmptyState
          size="sm"
          icon={ActivityIcon}
          heading={t('dashboard.temperature.full')}
          description={t('dashboard.temperature.fullNote', {
            most: SHARED_TELEMETRY_MAX_DEVICES,
          })}
        />
      {:else if empty && (followed === undefined || telemetry.loading(followed))}
        <Skeleton class="w-full flex-1" />
      {:else if empty}
        <EmptyState
          size="sm"
          icon={ThermometerIcon}
          heading={t('dashboard.temperature.quiet')}
          description={t('dashboard.temperature.quietNote')}
        />
      {:else}
        <LineChart
          {series}
          unit="°C"
          xDomain={[since, clock.now]}
          label={t('dashboard.temperature.label', {
            name: view.name,
            minutes: settings.minutes,
          })}
          class="min-h-0 flex-1"
        />
      {/if}
    {/snippet}
  </OnePrinter>
</TilePanel>
