<!-- The History tab: the window's measurements as a chart and its events as a log. -->
<script lang="ts">
  import ChartLineIcon from '@lucide/svelte/icons/chart-line';
  import DatabaseXIcon from '@lucide/svelte/icons/database-x';
  import InboxIcon from '@lucide/svelte/icons/inbox';
  import { onMount } from 'svelte';

  import type { DataFault } from '../../../data';
  import { mergeSeries, readingsOf, thinSeries } from '../../../domain';
  import { describeFault, t } from '../../../i18n';
  import {
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    EmptyState,
    FaultAlert,
    LineChart,
    ListingSkeleton,
    type ChartSeries,
  } from '../../../primitives';
  import HistoryLog from './history-log.svelte';
  import {
    defaultChartMetrics,
    metricLabel,
    metricUnit,
    minStepOf,
    sharedUnit,
  } from './metrics';
  import type { PrinterHistoryPanelProps } from './types';

  let {
    facts,
    samples,
    live,
    reported,
    span,
    drawn,
    describeType,
    onDrawnChange,
    onMore,
    onOpen,
    onClose,
  }: PrinterHistoryPanelProps = $props();

  // How many points a chart is drawn from once the stored and live points for
  // one metric are merged. A constant of this file: nothing else needs it.
  const DRAWING_BUDGET = 600;

  // What the core answers when a deployment declares no history store. Every
  // other fault, including a query the provider refuses in this shape, goes
  // through `describeFault` instead.
  const HISTORY_NOT_CONFIGURED_FAULT = 'core.resource/HISTORY_NOT_CONFIGURED';

  const isRefusal = (fault: DataFault | undefined): boolean =>
    fault?.code === HISTORY_NOT_CONFIGURED_FAULT;

  // The readings the printer describes name its lines and set their units.
  const readings = $derived(readingsOf(reported ?? {}));
  const usualMetrics = $derived(defaultChartMetrics(readings));

  // The tab reads when it opens, and only the readings it will draw. The
  // remembered choice decides both, so what is read and what is drawn cannot
  // disagree: asking for the usual readings while drawing none of them is a
  // query nobody sees the answer to.
  onMount(() => {
    onOpen?.(remembered ?? usualMetrics);

    return () => onClose?.();
  });

  // Each half is refused, fails and loads on its own, so each is drawn on its
  // own card.
  const factsRefused = $derived(isRefusal(facts.error));
  const samplesRefused = $derived(isRefusal(samples.error));

  // A network that is down or a credential that expired fails both reads
  // with one code. One alert over both cards then says the whole of it.
  const shared = $derived(
    facts.error !== undefined &&
      samples.error !== undefined &&
      !factsRefused &&
      facts.error.code === samples.error.code
      ? facts.error
      : undefined,
  );
  const factsFault = $derived(
    shared === undefined && !factsRefused ? facts.error : undefined,
  );
  const samplesFault = $derived(
    shared === undefined && !samplesRefused ? samples.error : undefined,
  );

  // A metric the printer only started reporting after the window opened has
  // live points and no stored ones, so the lines are drawn from both sides.
  const pointsOf = (metric: string): ChartSeries['points'] =>
    thinSeries(
      mergeSeries(samples.series[metric] ?? [], live?.points(metric) ?? []),
      DRAWING_BUDGET,
    );

  /**
   * The readings of a list this chart can draw, in the order given. A reading
   * with no unit has nothing to measure it against and would share the axis
   * with nothing, so it is not a reading this chart has a line for.
   *
   * One place, because the legend and the chosen list filter by one rule. A
   * choice is remembered in this browser between visits, so a reading an
   * older build offered, `/job/progress` or `/dialog/id`, can still be in the
   * remembered list after an upgrade. Filtering the legend alone would leave
   * such a reading invisible and still counted: every later toggle would read
   * it as a reading in another unit and replace the chart instead of adding
   * to it.
   */
  const drawable = (metrics: readonly string[]): string[] =>
    metrics.filter((one) => metricUnit(one, readings) !== undefined);

  // The measurements are read only for the readings drawn, so a legend built
  // from what the read returned would never offer one nobody asked for. It
  // offers every reading the printer holds in a known unit, from its state
  // tree and its live frames.
  const offered = $derived(
    drawable([
      ...new Set([
        ...Object.keys(samples.series),
        ...Object.keys(live?.values ?? {}),
        ...Object.keys(reported ?? {}),
      ]),
    ]),
  );

  /**
   * The remembered choice, narrowed to the readings this build can draw.
   *
   * A choice that narrows to nothing is no choice at all, and reads as
   * absent: the reader is given the usual readings rather than a blank chart
   * whose legend is all struck through. A choice that was empty to begin
   * with, every line turned off by hand, is left as it is, because that one
   * is a reader's own answer and not an older build's leftovers.
   */
  const remembered = $derived.by(() => {
    if (drawn === undefined) return undefined;
    if (drawn.length === 0) return drawn;

    const kept = drawable(drawn);

    return kept.length > 0 ? kept : undefined;
  });

  // A reader who has chosen is given their choice. One who has not is given
  // the readings that share a unit, or every reading where the printer
  // reports none of those.
  const chosen = $derived.by(() => {
    if (remembered !== undefined) return remembered;

    const usual = offered.filter((one) => usualMetrics.includes(one));

    return usual.length > 0 ? usual : offered;
  });

  // A chosen reading stays in the legend while it has nothing to draw, so it
  // can still be turned off. The order is fixed, defaults first and then by
  // path, so a line keeps its colour and its place from one load to the next.
  const metrics = $derived(
    [...new Set([...offered, ...chosen])].sort((a, b) => {
      const rank = (one: string): number =>
        usualMetrics.includes(one)
          ? usualMetrics.indexOf(one)
          : usualMetrics.length;

      return rank(a) - rank(b) || a.localeCompare(b, 'en');
    }),
  );

  const chartSeries = $derived<ChartSeries[]>(
    metrics.map((metric) => ({
      id: metric,
      label: metricLabel(metric, readings),
      unit: metricUnit(metric, readings),
      hidden: !chosen.includes(metric),
      points: pointsOf(metric),
    })),
  );

  const showing = $derived(
    chartSeries.some((one) => !one.hidden && one.points.length > 0),
  );

  const unit = $derived(
    sharedUnit(
      chartSeries.filter((one) => !one.hidden).map((one) => one.id),
      readings,
    ),
  );

  // The chart has one value axis, so it draws one unit at a time. A reading
  // in the unit already drawn joins the others; one in another unit takes
  // their place.
  const toggled = (id: string): readonly string[] => {
    if (chosen.includes(id)) return chosen.filter((one) => one !== id);

    const own = metricUnit(id, readings);

    return chosen.every((one) => metricUnit(one, readings) === own)
      ? [...chosen, id]
      : [id];
  };

  // Both halves answered, neither holds anything and neither failed, so one
  // sentence says the window was quiet.
  const quiet = $derived(
    facts.loaded &&
      samples.loaded &&
      facts.error === undefined &&
      samples.error === undefined &&
      facts.rows.length === 0 &&
      chartSeries.length === 0,
  );

  // The server reports a hole rather than answering a quiet one, on either
  // read. One mark covers both, because it is the same window.
  const incomplete = $derived(facts.incomplete || samples.incomplete);

  // Live readings keep arriving after the window was fixed, and the chart
  // does not clip, so the axis reaches the newest point drawn.
  const newest = $derived(
    Math.max(
      0,
      ...chartSeries.map((one) => one.points[one.points.length - 1]?.at ?? 0),
    ),
  );

  // The axis spans the window a reader asked for rather than the rows that
  // came back, so a quiet printer and a busy one draw the same span.
  const xDomain = $derived<[number, number] | undefined>(
    span === undefined
      ? undefined
      : [samples.since ?? span.from, Math.max(span.to, newest)],
  );
</script>

{#snippet unavailable(level: 'h2' | 'h3')}
  <EmptyState
    icon={DatabaseXIcon}
    heading={t('printerDetailHistory.unavailable.heading')}
    headingLevel={level}
    description={t('printerDetailHistory.unavailable.note')}
  />
{/snippet}

<div class="flex flex-col gap-6">
  {#if factsRefused && samplesRefused}
    <Card>
      <CardContent>{@render unavailable('h2')}</CardContent>
    </Card>
  {:else if quiet}
    <Card>
      <CardContent>
        <EmptyState
          icon={InboxIcon}
          heading={t('printerDetailHistory.empty.heading')}
          headingLevel="h2"
          description={t('printerDetailHistory.empty.note')}
        />
      </CardContent>
    </Card>
  {:else}
    {#if shared}
      <FaultAlert fault={describeFault(shared)} />
    {/if}

    {#if incomplete}
      <p class="text-sm text-muted-foreground">
        {t('printerDetailHistory.incomplete')}
      </p>
    {/if}

    <Card>
      <CardHeader>
        <CardTitle>
          <h2 class="contents">{t('printerDetailHistory.chart.heading')}</h2>
        </CardTitle>
      </CardHeader>
      <CardContent class="flex flex-col gap-2">
        {#if !samples.loaded}
          <ListingSkeleton
            label={t('printerDetailHistory.chart.loading')}
            rows={3}
          />
        {:else if samplesRefused}
          {@render unavailable('h3')}
        {:else if samplesFault}
          <FaultAlert fault={describeFault(samplesFault)} />
        {:else if chartSeries.length === 0}
          <EmptyState
            icon={ChartLineIcon}
            heading={t('printerDetailHistory.chart.empty.heading')}
            headingLevel="h3"
          />
        {:else}
          <LineChart
            series={chartSeries}
            {xDomain}
            {unit}
            minStep={minStepOf(unit)}
            label={t('printerDetailHistory.chart.heading')}
            legendLabel={t('printerDetailHistory.chart.legend')}
            toggleLabels={{
              shown: t('printerDetailHistory.chart.hide'),
              hidden: t('printerDetailHistory.chart.draw'),
            }}
            onToggle={onDrawnChange === undefined
              ? undefined
              : (id: string) => onDrawnChange(toggled(id))}
          />
          {#if samples.truncated && showing}
            <p class="text-sm text-muted-foreground">
              {t('printerDetailHistory.chart.truncated')}
            </p>
          {/if}
        {/if}
      </CardContent>
    </Card>

    {#if !facts.loaded}
      <Card>
        <CardContent>
          <ListingSkeleton
            label={t('printerDetailHistory.log.loading')}
            rows={5}
          />
        </CardContent>
      </Card>
    {:else if factsRefused}
      <Card>
        <CardHeader>
          <CardTitle>
            <h2 class="contents">{t('printerDetailHistory.log.heading')}</h2>
          </CardTitle>
        </CardHeader>
        <CardContent>{@render unavailable('h3')}</CardContent>
      </Card>
    {:else}
      <!--
        A failed `Read more` keeps the rows it already had, so the alert is
        drawn beside what was read rather than instead of it.
      -->
      {#if factsFault}
        <FaultAlert fault={describeFault(factsFault)} />
      {/if}
      <HistoryLog rows={facts.rows} more={facts.more} {describeType} {onMore} />
    {/if}
  {/if}
</div>
