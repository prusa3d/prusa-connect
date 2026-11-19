<!--
  The period day by day, one metric at a time: jobs, success or print hours.
-->
<script lang="ts">
  import { LineChart } from '@prusa/connect-ui/primitives';

  import type { TrendDay } from '../domain/job-stats';
  import { heldHost } from '../host';
  import { pointsOf, type TrendMetric, unitOf } from './trend';

  interface Props {
    trend: readonly TrendDay[];
    metric: TrendMetric;
    class?: string;
  }

  let { trend, metric, class: className }: Props = $props();
  const t = heldHost().t;

  const series = $derived([
    {
      id: metric,
      label: t(`fleet-stats.trend.metric.${metric}`),
      points: pointsOf(trend, metric),
    },
  ]);
  const xDomain = $derived<[number, number] | undefined>(
    trend.length === 0
      ? undefined
      : [trend[0]?.start ?? 0, trend[trend.length - 1]?.start ?? 0],
  );
</script>

<LineChart
  {series}
  {xDomain}
  unit={unitOf(metric)}
  zeroBaseline
  minStep={metric === 'hours' ? 0.5 : 1}
  label={t('fleet-stats.trend.label', {
    metric: t(`fleet-stats.trend.metric.${metric}`),
  })}
  class={className}
/>
