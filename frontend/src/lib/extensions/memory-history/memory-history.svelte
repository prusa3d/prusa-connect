<!--
  A module's memory over the last hour against its budget, and the stops the
  host recorded. The stops follow the switch-off window, so a window shorter
  than the hour lists fewer of them than the chart covers.
-->
<script lang="ts">
  import { formatDate, translateFor } from '@prusa/connect-ui/i18n';
  import {
    FaultAlert,
    LineChart,
    Separator,
  } from '@prusa/connect-ui/primitives';

  import { APP_NAMESPACE } from '../../i18n';
  import { describeExtensionFault } from '../fault';
  import type { ResourceMemoryHistory } from '../../stores/module-operations.svelte';
  import { memorySeries } from '../memory-series';
  import { type ResourceMemory, stopText, windowText } from '../stops';

  let {
    history,
    memory,
    missing = false,
    fault,
  }: {
    /** Undefined until the first read answers. */
    history?: ResourceMemoryHistory | undefined;
    memory: ResourceMemory;
    /** The server no longer knows the module. */
    missing?: boolean;
    /** What the last read failed with; shown only while no reading is on screen. */
    fault?: unknown;
  } = $props();

  const t = translateFor(APP_NAMESPACE);
  const KEY = 'app.settings.extensions.history';
  const SHARED = 'app.settings.extensions.operations';

  // The chart's hour ends when the history was read.
  const series = $derived(
    history === undefined
      ? undefined
      : memorySeries(history, memory, Date.now(), t),
  );
  const stops = $derived([...(history?.stops ?? [])].reverse());
  const short = $derived(memory.switch_off_window_seconds < 3600);
</script>

<div class="flex flex-col gap-3" data-part="memory-history">
  <h3 class="text-sm font-medium">{t(`${KEY}.title`)}</h3>
  {#if missing}
    <p class="text-sm" data-part="history-gone">{t(`${SHARED}.gone`)}</p>
  {:else if series === undefined && fault !== undefined}
    <div data-part="history-failed">
      <FaultAlert fault={describeExtensionFault(fault, t)} />
    </div>
  {:else if series === undefined}
    <p class="text-sm text-muted-foreground" data-part="history-loading">
      {t(`${SHARED}.loading`)}
    </p>
  {:else if series.points.length === 0}
    <p class="text-sm text-muted-foreground" data-part="history-empty">
      {t(`${KEY}.empty`)}
    </p>
  {:else}
    <LineChart
      series={[
        {
          id: 'memory',
          label: t(`${KEY}.series`),
          points: series.points,
        },
      ]}
      label={t(`${KEY}.chart`)}
      unit={t('app.settings.extensions.memory.budget.unit')}
      minStep={16}
      threshold={series.threshold}
      markers={series.markers}
      regions={series.regions}
      xDomain={series.domain}
    />
  {/if}
  <p class="text-xs text-muted-foreground">{t(`${SHARED}.cleared`)}</p>

  {#if !missing && history !== undefined}
    <Separator />
    <div class="flex flex-col gap-2" data-part="recent-stops">
      <h4 class="text-sm font-medium">{t(`${KEY}.stops`)}</h4>
      {#if stops.length === 0}
        <p class="text-sm text-muted-foreground">{t(`${KEY}.stopsNone`)}</p>
      {:else}
        <ul class="flex flex-col gap-1">
          {#each stops as stop (stop.at)}
            <li class="flex flex-wrap justify-between gap-x-3 text-sm">
              <span>{stopText(stop, t, memory)}</span>
              <time
                class="text-muted-foreground tabular-nums"
                datetime={stop.at}
              >
                {formatDate(stop.at, {
                  dateStyle: 'medium',
                  timeStyle: 'medium',
                })}
              </time>
            </li>
          {/each}
        </ul>
      {/if}
      {#if short}
        <p class="text-xs text-muted-foreground">
          {t(`${KEY}.stopsWindow`, {
            window: windowText(memory.switch_off_window_seconds),
          })}
        </p>
      {/if}
    </div>
  {/if}
</div>
