<script lang="ts">
  import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
  } from '@prusa/connect-ui/primitives';

  import { formatNumber } from '@prusa/connect-ui/i18n';
  import { printerHref } from '@prusa/connect-ui/shell';

  import type { DeviceUtilization } from '../domain/job-stats';
  import { heldHost } from '../host';

  interface Props {
    rows: DeviceUtilization[];
    /** Draws the weekday header and the printer names. */
    days?: boolean;
    /** Fits the card it sits in instead of scrolling: a dashboard card. */
    compact?: boolean;
  }

  let { rows, days = true, compact = false }: Props = $props();
  const t = heldHost().t;
  const percentOf = (value: number) =>
    formatNumber(value, { style: 'percent', maximumFractionDigits: 0 });
  const dayName = (cell: number) =>
    t(`fleet-stats.day.${String(Math.floor(cell / 24))}`);
  const summaryOf = (row: DeviceUtilization) => {
    const peak = row.cells.reduce(
      (best, share, cell) => (share > (row.cells[best] ?? 0) ? cell : best),
      0,
    );
    const average =
      row.cells.reduce((sum, share) => sum + share, 0) / row.cells.length;
    return average === 0
      ? t('fleet-stats.utilization.labelIdle', { name: row.name })
      : t('fleet-stats.utilization.label', {
          name: row.name,
          day: dayName(peak),
          hour: String(peak % 24).padStart(2, '0'),
          share: percentOf(average),
        });
  };
  const DAYS = [0, 1, 2, 3, 4, 5, 6];
</script>

<TooltipProvider>
  <div class="overflow-x-auto">
    <div class="flex flex-col gap-1" class:wide={days} class:compact>
      {#if days}
        <div class="row grid gap-1 text-xs text-muted-foreground">
          <span></span>
          <div class="days grid">
            {#each DAYS as day (day)}<span
                >{t(`fleet-stats.day.${String(day)}`)}</span
              >{/each}
          </div>
        </div>
      {/if}
      {#each rows as row (row.deviceId)}
        <div class={days ? 'row grid items-center gap-1' : ''}>
          {#if days}
            <a
              class="truncate text-sm hover:underline"
              href={printerHref(row.deviceId)}>{row.name}</a
            >
          {/if}
          <div class="cells grid gap-px" role="img" aria-label={summaryOf(row)}>
            {#each row.cells as share, cell (cell)}
              <Tooltip>
                <TooltipTrigger>
                  {#snippet child({ props })}
                    <span
                      {...props}
                      class="cell rounded-badge block h-4 bg-primary"
                      style:--share={share}
                    ></span>
                  {/snippet}
                </TooltipTrigger>
                <TooltipContent>
                  {t('fleet-stats.utilization.cell', {
                    day: dayName(cell),
                    hour: String(cell % 24).padStart(2, '0'),
                    share: percentOf(share),
                  })}
                </TooltipContent>
              </Tooltip>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>
</TooltipProvider>

<style>
  .wide {
    min-width: calc(var(--spacing) * 200);
  }
  .days {
    grid-template-columns: repeat(7, minmax(0, 1fr));
  }
  .row {
    grid-template-columns: 10.5rem minmax(0, 1fr);
  }
  /* A dashboard card fits the grid to its width rather than scrolling it. */
  .compact.wide,
  .compact .cells {
    min-width: 0;
  }
  .compact .row {
    grid-template-columns: 8.5rem minmax(0, 1fr);
  }
  .cell {
    opacity: calc(0.08 + var(--share) * 0.92);
  }
  .cells {
    min-width: calc(var(--spacing) * 168);
    grid-template-columns: repeat(168, minmax(0, 1fr));
  }
</style>
