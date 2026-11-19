<!--
  The fleet in five numbers: printing, needing attention, done, idle, and
  silent. Each printer is counted once. A printer that needs attention counts
  there, by the rule the issues card lists by (needsPerson in printers.ts),
  and every other printer counts by the tint of its state.

  It answers the question a wall is hung for, "is anything wrong", before any
  other: the count that needs attention is second only to what is printing,
  and it takes the fault tint the moment one printer is in error. The numbers
  grow with the tile (frontend-css.md, style math in CSS), so a summary
  across a whole wall reads from across the room.
-->
<script lang="ts">
  import { PRINTER_STATE_TONE, type PrinterStateTone } from '../../../domain';
  import { formatNumber } from '../../../i18n';

  import { needsPerson, selected } from '../printers';
  import { t } from '../../../i18n';
  import { dashboardContext } from '../context';
  import { tagSettings } from './config';
  import FleetState from './fleet-state.svelte';
  import type { CardProps } from '../../../slots';

  import attentionIcon from './assets/summary-attention.svg';
  import doneIcon from './assets/summary-done.svg';
  import idleIcon from './assets/summary-idle.svg';
  import printingIcon from './assets/summary-printing.svg';
  import silentIcon from './assets/summary-silent.svg';

  let { config, title }: CardProps = $props();

  const { printers } = dashboardContext();

  const settings = $derived(tagSettings(config));

  type Group = 'printing' | 'attention' | 'done' | 'idle' | 'silent';

  // Only for a printer that needs no attention. An error and a question on the
  // screen always need one, so the two loud tints never reach this map.
  const GROUP_OF: Readonly<Record<PrinterStateTone, Group>> = {
    active: 'printing',
    busy: 'printing',
    moving: 'printing',
    fault: 'attention',
    attention: 'attention',
    paused: 'attention',
    ready: 'done',
    finished: 'done',
    stopped: 'done',
    idle: 'idle',
    quiet: 'silent',
    offline: 'silent',
  };

  const GROUP_KEY: Readonly<Record<Group, string>> = {
    printing: 'dashboard.summary.printing',
    attention: 'dashboard.summary.attention',
    done: 'dashboard.summary.done',
    idle: 'dashboard.summary.idle',
    silent: 'dashboard.summary.silent',
  };

  const ORDER: readonly Group[] = [
    'printing',
    'attention',
    'done',
    'idle',
    'silent',
  ];

  const ICON: Readonly<Record<Group, string>> = {
    printing: printingIcon,
    attention: attentionIcon,
    done: doneIcon,
    idle: idleIcon,
    silent: silentIcon,
  };

  const views = $derived(
    printers.views.filter((view) => selected(view, settings)),
  );

  const counts = $derived.by(() => {
    const held: Record<Group, number> = {
      printing: 0,
      attention: 0,
      done: 0,
      idle: 0,
      silent: 0,
    };
    for (const view of views) {
      const group = needsPerson(view)
        ? 'attention'
        : GROUP_OF[PRINTER_STATE_TONE[view.state]];
      held[group] += 1;
    }

    return held;
  });

  const fallback = $derived(
    settings.tag === ''
      ? t('dashboard.summary.title', { count: views.length })
      : t('dashboard.summary.tagged', {
          count: views.length,
          tag: settings.tag,
        }),
  );
</script>

<div class="summary-body min-h-0 flex-1 flex-col">
  <div
    class="summary-heading shrink-0"
    data-default={title.trim() === '' && settings.tag === ''}
  >
    <h2 class="text-sm font-semibold">{title.trim() || fallback}</h2>
  </div>
  <FleetState>
    <dl data-part="fleet-counts" class="summary-counts min-h-0 flex-1">
      {#each ORDER as group (group)}
        <div
          data-pickup={group === 'done' && counts.done > 0 ? '' : undefined}
          data-group={group}
          style:--summary-icon={`url("${ICON[group]}")`}
          class="summary-group @container min-w-0 justify-center"
        >
          <dt class="summary-label text-xs text-muted-foreground">
            {t(GROUP_KEY[group])}
            {#if group === 'done' && counts.done > 0}
              <span class="block text-xs font-normal text-muted-foreground">
                {t('dashboard.summary.pickup')}
              </span>
            {/if}
          </dt>
          <dd
            class={[
              'summary-figure font-semibold tabular-nums',
              counts[group] === 0 && 'text-muted-foreground',
            ]}
          >
            {formatNumber(counts[group])}
          </dd>
        </div>
      {/each}
    </dl>
  </FleetState>
</div>

<style>
  .summary-counts {
    display: grid;
    grid-template-columns: repeat(
      auto-fit,
      minmax(min(100%, calc(var(--spacing) * 25)), 1fr)
    );
    align-items: stretch;
    gap: calc(var(--spacing) * 3);
    overflow: auto;
  }

  .summary-body {
    display: flex;
    --dashboard-heading-gap: var(--spacing-dashboard-summary-gap);
    gap: var(--spacing-dashboard-summary-gap);
  }

  .summary-heading[data-default='true'] h2 {
    color: var(--color-muted-foreground);
    font-size: var(--text-xs);
    line-height: var(--text-xs--line-height);
  }

  .summary-group {
    --summary-accent: var(--state-quiet-text);
    display: grid;
    grid-template: 'icon figure' auto 'label label' auto 'line line' auto / auto minmax(
        0,
        1fr
      );
    align-content: center;
    align-items: center;
    column-gap: calc(var(--spacing) * 3);
    border-inline-start: 1px solid var(--color-border);
    padding-inline-start: calc(var(--spacing) * 3);
  }

  .summary-group:first-child {
    border-inline-start: 0;
    padding-inline-start: 0;
    --summary-line-start: 0px;
  }

  .summary-group[data-group='printing'] {
    --summary-accent: var(--state-active-text);
  }

  .summary-group[data-group='attention'] {
    --summary-accent: var(--color-warning);
  }

  .summary-group[data-group='done'] {
    --summary-accent: var(--state-done-text);
  }

  .summary-group::before {
    grid-area: icon;
    inline-size: clamp(
      calc(var(--spacing) * 5),
      18cqi,
      calc(var(--spacing) * 10)
    );
    aspect-ratio: 1;
    background: var(--summary-accent);
    mask: var(--summary-icon) center / contain no-repeat;
    content: '';
  }

  .summary-group[data-group='printing']::before {
    background: linear-gradient(
      to bottom,
      var(--color-foreground) 75%,
      var(--summary-accent) 75%
    );
  }

  .summary-group::after {
    grid-area: line;
    block-size: 2px;
    margin-block-start: calc(var(--spacing) * 2);
    background: var(--summary-accent);
    content: '';
  }

  .summary-figure {
    grid-area: figure;
    font-size: clamp(var(--text-2xl), 20cqi, calc(var(--text-2xl) * 1.75));
    line-height: 1;
  }

  /* The label wraps between words; a word longer than a narrow tile's column
     ends in an ellipsis rather than running past the strip's edge. */
  .summary-label {
    grid-area: label;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: normal;
  }

  @container dashboard-tile (14rem <= width < 40rem) {
    .summary-group[data-pickup] {
      grid-column: span 2;
    }
  }

  @container dashboard-tile (40rem <= width < 56rem) {
    .summary-counts:has([data-pickup]) {
      grid-template-columns:
        repeat(2, minmax(0, 1fr)) minmax(0, 1.5fr)
        repeat(2, minmax(0, 1fr));
    }

    .summary-group {
      column-gap: var(--spacing);
      padding-inline-start: var(--spacing);
      --summary-line-start: var(--spacing);
    }

    .summary-group:first-child {
      padding-inline-start: 0;
      --summary-line-start: 0px;
    }

    .summary-group::before {
      inline-size: calc(var(--spacing) * 6);
    }
  }

  @container dashboard-tile (width >= 40rem) {
    .summary-group {
      position: relative;
      grid-template: 'icon figure label' auto / auto auto minmax(0, 1fr);
      padding-block-end: var(--spacing);
    }

    .summary-figure,
    .summary-label {
      align-self: baseline;
    }

    .summary-group[data-pickup] .summary-label {
      align-self: center;
    }

    .summary-group::after {
      position: absolute;
      inset-block-end: 0;
      inset-inline: var(--summary-line-start, calc(var(--spacing) * 3))
        var(--summary-line-end, 0px);
      margin-block-start: 0;
    }
  }

  @container dashboard-tile (width >= 64rem) {
    .summary-counts {
      column-gap: 0;
    }

    .summary-group {
      --summary-inline-gap: clamp(
        calc(var(--spacing) * 3),
        1.4cqi,
        calc(var(--spacing) * 5)
      );
      column-gap: var(--summary-inline-gap);
      padding-inline: var(--summary-inline-gap);
      --summary-line-start: var(--summary-inline-gap);
      --summary-line-end: var(--summary-inline-gap);
    }

    .summary-group:first-child {
      padding-inline-start: 0;
      --summary-line-start: 0px;
    }

    .summary-group:last-child {
      padding-inline-end: 0;
      --summary-line-end: 0px;
    }
  }
</style>
