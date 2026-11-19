<!--
  A dashboard card of the fleet's figures over a period: jobs, success, print
  hours and utilization, each with its change against the period before.
-->
<script lang="ts">
  import type { CardProps } from '@prusa/connect-ui/slots';

  import type { JobStats } from '../domain/job-stats';
  import { heldHost } from '../host';
  import { comparison, metricsFor } from '../view/metrics';
  import CardFrame from './card-frame.svelte';
  import { cardViewOf } from './settings';

  let { config, title, size, editing }: CardProps = $props();

  const t = heldHost().t;
  const shown = metricsFor(t).filter((metric) => metric.key !== 'averageJobMs');
  const view = $derived(cardViewOf(config));
  // Four in a row on a wide card, two by two on a narrow one.
  const columns = $derived(size.w >= 6 ? 'grid-cols-4' : 'grid-cols-2');
</script>

<CardFrame
  heading={title.trim() === '' ? t('fleet-stats.cards.kpis.heading') : title}
  {view}
  {editing}
>
  {#snippet children(stats: JobStats)}
    <dl class={`grid ${columns} gap-3`}>
      {#each shown as metric (metric.key)}
        {@const change = comparison(t, metric, stats.kpis, stats.previous)}
        <div class="flex min-w-0 flex-col">
          <dt class="truncate text-xs text-muted-foreground">
            {t(metric.label)}
          </dt>
          <dd class="text-xl font-semibold tabular-nums">
            {metric.show(stats.kpis)}
          </dd>
          {#if change !== undefined}
            <dd class="text-xs text-muted-foreground">
              {change.label}
            </dd>
          {/if}
        </div>
      {/each}
    </dl>
  {/snippet}
</CardFrame>
