<!--
  Many printers in one tile: every printer, or the ones with a tag, narrowed
  to a state and sorted the way the settings ask.

  A farm with hundreds of printers puts them on a wall, so the printers flow
  into as many columns as the tile is wide, and the tile scrolls inside
  itself rather than growing the dashboard. On a wall nobody scrolls, so the
  list is cut at the tile's edge there. Either way a line under the list
  counts the printers below the edge. Row size draws one line per printer,
  for a long list.
-->
<script lang="ts">
  import { PRINTER_STATE_LABEL_KEY } from '../../../domain';
  import { formatDuration, formatNumber, formatUnit } from '../../../i18n';
  import {
    Badge,
    Card,
    CardContent,
    EmptyState,
    TilePanel,
  } from '../../../primitives';
  import LayoutGridIcon from '@lucide/svelte/icons/layout-grid';

  import { ordered, selected } from '../printers';
  import { cardToneOf, stateBadgeVariant } from '../../printers/presentation';
  import { t } from '../../../i18n';
  import { dashboardContext } from '../context';
  import { printersSettings } from './config';
  import { SCOPE } from './labels';
  import { inScope } from './scope';
  import FleetState from './fleet-state.svelte';
  import PrinterFace from './printer-face.svelte';
  import JobBar from './job-bar.svelte';
  import type { CardProps } from '../../../slots';

  let { config, title }: CardProps = $props();

  const { printers } = dashboardContext();

  const settings = $derived(printersSettings(config));

  const views = $derived(
    ordered(
      printers.views.filter(
        (view) => selected(view, settings) && inScope(view, settings.scope),
      ),
      settings.order,
    ),
  );

  let below = $state(0);

  // CSS cannot count the items a scrolling box cuts off, so the list
  // measures them: on a resize of the list or of any item, on a printer
  // added or removed, and on a scroll.
  //
  // The count lands in the next frame. The line it shows or hides changes the
  // list's height, and a size changed inside the observer's own callback makes
  // the browser report "ResizeObserver loop completed with undelivered
  // notifications" as an error.
  const countBelow = (list: HTMLElement): (() => void) => {
    let frame = 0;
    const count = (): void => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const edge = list.getBoundingClientRect().bottom;
        below = [...list.children].filter(
          (item) => item.getBoundingClientRect().bottom > edge + 1,
        ).length;
      });
    };
    const sizes = new ResizeObserver(count);
    const watch = (): void => {
      sizes.disconnect();
      sizes.observe(list);
      for (const item of list.children) sizes.observe(item);
    };
    const items = new MutationObserver(watch);
    watch();
    items.observe(list, { childList: true });
    list.addEventListener('scroll', count, { passive: true });

    return () => {
      cancelAnimationFrame(frame);
      sizes.disconnect();
      items.disconnect();
      list.removeEventListener('scroll', count);
    };
  };

  const fallback = $derived(
    settings.tag === ''
      ? t(SCOPE[settings.scope])
      : t('dashboard.printers.tagged', {
          tag: settings.tag,
          scope: t(SCOPE[settings.scope]),
        }),
  );
</script>

<TilePanel
  {title}
  {fallback}
  class="printer-list-panel @container/printer-list"
>
  {#snippet trailing()}
    {#if printers.loaded}
      <Badge variant="secondary" class="tabular-nums">{views.length}</Badge>
    {/if}
  {/snippet}
  <FleetState>
    {#if views.length === 0}
      <EmptyState
        size="sm"
        icon={LayoutGridIcon}
        heading={t('dashboard.printers.none')}
        description={t('dashboard.printers.noneNote')}
      />
    {:else if settings.size === 'row'}
      <ul
        data-part="printer-rows"
        class="scroller me-0 min-h-0 flex-1 divide-y divide-border overflow-y-auto pe-0"
        {@attach countBelow}
      >
        {#each views as view (view.id)}
          <li>
            <a
              href={view.detailHref}
              class="grid grid-cols-3 items-center gap-3 py-1.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring @max-md/printer-list:grid-cols-1 @max-md/printer-list:gap-1 @max-md/printer-list:py-3"
            >
              <span class="min-w-0 truncate font-medium">{view.name}</span>
              <span class="flex min-w-0 items-center gap-2">
                <JobBar {view} />
                <span
                  class="w-10 shrink-0 text-end font-mono text-xs tabular-nums"
                >
                  {view.progress === null ? '' : formatUnit(view.progress, '%')}
                </span>
              </span>
              <span class="flex min-w-0 items-center justify-end gap-2">
                {#if view.remaining !== undefined}
                  <span
                    class="truncate font-mono text-xs text-muted-foreground tabular-nums"
                  >
                    {formatDuration(view.remaining)}
                  </span>
                {/if}
                <Badge
                  variant={stateBadgeVariant(view.state)}
                  class="shrink-0 font-mono leading-none tracking-wide uppercase"
                >
                  {t(PRINTER_STATE_LABEL_KEY[view.state])}
                </Badge>
              </span>
            </a>
          </li>
        {/each}
      </ul>
    {:else}
      <ul
        data-part="printer-cards"
        class="printer-cards scroller me-0 min-h-0 flex-1 overflow-y-auto pe-0"
        {@attach countBelow}
      >
        {#each views as view (view.id)}
          <li>
            <Card
              tone={cardToneOf(view.tier)}
              density="tight"
              class={[
                'h-full',
                view.tier !== 'critical' &&
                  view.tier !== 'attention' &&
                  'dark:border-border/60',
              ]}
            >
              <CardContent class="dashboard-printer-content flex-1">
                <PrinterFace
                  {view}
                  fields={settings.fields}
                  picture="art"
                  dense
                />
              </CardContent>
            </Card>
          </li>
        {/each}
      </ul>
    {/if}
    {#if views.length > 0 && below > 0}
      <p
        data-part="printers-more"
        class="shrink-0 text-xs text-muted-foreground tabular-nums"
      >
        {t('dashboard.printers.more', { count: formatNumber(below) })}
      </p>
    {/if}
  </FleetState>
</TilePanel>

<style>
  :global(.printer-list-panel) > :global(:first-child) {
    margin-block-end: max(
      0px,
      calc(var(--spacing-dashboard-heading-gap) - var(--spacing) * 2)
    );
  }

  /* The wall view leaves no one to scroll. */
  :global([data-part='kiosk']) .scroller {
    overflow-y: hidden;
  }

  /* As many columns as fit at the width one printer reads well at, four at
     most; narrower panels keep the readable minimum. */
  .printer-cards {
    display: grid;
    grid-template-columns: repeat(
      auto-fill,
      minmax(
        min(
          100%,
          max(
            var(--spacing-printer-card-min),
            calc((100% - var(--spacing) * 9) / 4)
          )
        ),
        1fr
      )
    );
    align-content: start;
    gap: calc(var(--spacing) * 3);
  }
  .printer-cards :global([data-slot='card']) {
    padding-block: 0;
  }
  .printer-cards :global(.dashboard-printer-content) {
    padding: var(--spacing-printer-card-padding);
  }
</style>
