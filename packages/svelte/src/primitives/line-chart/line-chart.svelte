<!--
  One line per series against a shared time axis, with a legend under it and
  a hidden text summary that a screen reader reads.

  Every label is HTML and the SVG holds only shapes. The y labels sit in their
  own column, so the browser sizes the column to the widest label and never
  cuts one off. The lines are drawn in a fixed 1000 by 1000 box that the SVG
  stretches to the plot, and `vector-effect` keeps the stroke at 2 px. A point,
  a grid line and a label are placed in percent of the plot. So the chart needs
  no script to measure its own width.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import {
    BOX,
    drawnOf,
    highestOf,
    latestOf,
    lowestOf,
    plotOf,
    valueText,
    xLabel,
  } from './geometry';
  import { timeTicks } from './scale';
  import { chartPalette } from './palette';
  import type { LineChartProps } from './types';

  let {
    series,
    label,
    unit,
    minStep,
    zeroBaseline,
    threshold,
    markers,
    regions,
    xDomain,
    onToggle,
    legendLabel,
    toggleLabels,
    class: className,
  }: LineChartProps = $props();

  // A line takes its colour from its place in `series`, so it keeps the
  // colour when another line is hidden.
  const hueOf = $derived((id: string) => {
    const at = series.findIndex((one) => one.id === id);

    return (
      chartPalette[(at < 0 ? 0 : at) % chartPalette.length] ?? chartPalette[0]
    );
  });

  const drawn = $derived(drawnOf(series));

  const plot = $derived(
    plotOf({
      drawn,
      unit,
      minStep,
      zeroBaseline,
      threshold,
      markers,
      regions,
      xDomain,
    }),
  );
  const xTicks = $derived(timeTicks(plot.xRange[0], plot.xRange[1]));

  const summary = $derived(
    drawn.map((one) => {
      const own = one.unit ?? unit;

      return {
        id: one.id,
        text: t('primitives.lineChart.summary', {
          name: one.label,
          latest: valueText(latestOf(one), own),
          lowest: valueText(lowestOf(one), own),
          highest: valueText(highestOf(one), own),
        }),
      };
    }),
  );
</script>

<div class={['flex flex-col gap-3', className]}>
  <div
    role="img"
    aria-label={label}
    class="flex h-64 w-full text-xs text-muted-foreground"
  >
    {#if drawn.length > 0}
      <div class="grid flex-none pe-2 pbs-4 pbe-8">
        {#each plot.yScale.ticks as tick (tick)}
          <span
            data-slot="line-chart-value"
            class="tick-y relative col-start-1 row-start-1 self-start text-end tabular-nums"
            style="--at: {plot.yAt(tick)}"
          >
            {valueText(tick, unit)}
          </span>
        {/each}
      </div>
    {/if}
    <div class="@container min-w-0 flex-1 pbs-4 pbe-8">
      <div class="relative size-full">
        <svg
          aria-hidden="true"
          class="absolute inset-0 size-full overflow-visible rtl:-scale-x-100"
        >
          {#if drawn.length > 0}
            {#each plot.yScale.ticks as tick (tick)}
              <line
                x1="0"
                x2="100%"
                y1="{plot.yAt(tick) * 100}%"
                y2="{plot.yAt(tick) * 100}%"
                class="stroke-border"
              />
            {/each}
          {/if}
          {#each plot.bands as band, index (index)}
            <rect
              data-slot="line-chart-region"
              x="{band.left * 100}%"
              width="{band.width * 100}%"
              y="0"
              height="100%"
              class="fill-muted"
            />
          {/each}
          {#if threshold !== undefined && drawn.length > 0}
            <line
              data-slot="line-chart-threshold"
              x1="0"
              x2="100%"
              y1="{plot.yAt(threshold.value) * 100}%"
              y2="{plot.yAt(threshold.value) * 100}%"
              stroke-dasharray="6 4"
              class="stroke-muted-foreground"
            />
          {/if}
          <svg
            viewBox="0 0 {BOX} {BOX}"
            preserveAspectRatio="none"
            width="100%"
            height="100%"
            class="overflow-visible"
          >
            {#each plot.lines as line (line.id)}
              {#if line.path !== ''}
                <path
                  d={line.path}
                  data-slot="line-chart-series"
                  vector-effect="non-scaling-stroke"
                  stroke-linejoin="round"
                  class={['fill-none stroke-2', hueOf(line.id)?.stroke]}
                />
              {/if}
            {/each}
          </svg>
          {#each plot.lines as line (line.id)}
            {#each line.dots as dot, index (index)}
              {#if dot}
                <circle
                  cx="{plot.xAt(dot.at) * 100}%"
                  cy="{plot.yAt(dot.value) * 100}%"
                  r="3"
                  data-slot="line-chart-series"
                  class={['stroke-none', hueOf(line.id)?.fill]}
                />
              {/if}
            {/each}
          {/each}
          {#each plot.flags as flag, index (index)}
            <line
              data-slot="line-chart-marker"
              x1="{plot.xAt(flag.at) * 100}%"
              x2="{plot.xAt(flag.at) * 100}%"
              y1="0"
              y2="8"
              stroke-width="2"
              class="stroke-destructive-text"
            />
            {#if flag.nearest}
              <circle
                cx="{plot.xAt(flag.nearest.at) * 100}%"
                cy="{plot.yAt(flag.nearest.value) * 100}%"
                r="4"
                class="fill-destructive-text stroke-none"
              />
            {/if}
          {/each}
        </svg>
        {#each xTicks as tick, index (tick.at)}
          <!-- Every second label is left out where the plot is narrow. -->
          <span
            data-slot="line-chart-time"
            class={[
              'tick-x absolute mbs-1 whitespace-nowrap tabular-nums',
              index % 2 === 1 && '@max-md:hidden',
            ]}
            style="--at: {plot.xAt(tick.at)}"
          >
            {xLabel(tick)}
          </span>
        {/each}
      </div>
    </div>
  </div>

  {#if summary.length > 0}
    <ul class="sr-only">
      {#each plot.extras as text, index (index)}
        <li>{text}</li>
      {/each}
      {#each summary as line (line.id)}
        <li>{line.text}</li>
      {/each}
    </ul>
  {/if}

  {#if series.length > 0}
    <ul class="flex flex-wrap gap-x-4 gap-y-1" aria-label={legendLabel}>
      {#each series as one (one.id)}
        {@const off = one.hidden === true}
        <li class="flex items-center text-xs text-muted-foreground">
          {#if onToggle}
            <button
              type="button"
              aria-pressed={!off}
              title={off ? toggleLabels?.hidden : toggleLabels?.shown}
              class="flex min-h-control-height items-center gap-1.5 rounded-sm px-1 py-0.5 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none @max-3xl/app-shell:min-h-touch-target pointer-coarse:min-h-touch-target"
              onclick={() => onToggle(one.id)}
            >
              <span
                aria-hidden="true"
                class={[
                  'size-2 shrink-0 rounded-full',
                  off ? 'bg-muted-foreground/40' : hueOf(one.id)?.swatch,
                ]}
              ></span>
              <span class={off ? 'line-through opacity-60' : undefined}>
                {one.label}
              </span>
            </button>
          {:else}
            <span class="flex items-center gap-1.5">
              <span
                aria-hidden="true"
                class={['size-2 shrink-0 rounded-full', hueOf(one.id)?.swatch]}
              ></span>
              {one.label}
            </span>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  /*
   * Every y label shares one grid cell as tall as the plot, so the column is
   * as wide as the widest label.
   */
  .tick-y {
    inset-block-start: calc(var(--at) * 100%);
    translate: 0 -50%;
  }

  /*
   * An x label moves from its start edge at the start of the axis to its end
   * edge at the end, so no label runs past the plot.
   */
  .tick-x {
    inset-block-start: 100%;
    inset-inline-start: calc(var(--at) * 100%);
    translate: calc(var(--at) * -100%) 0;
  }

  .tick-x:dir(rtl) {
    translate: calc(var(--at) * 100%) 0;
  }
</style>
