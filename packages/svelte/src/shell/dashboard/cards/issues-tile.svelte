<!--
  The printers that need attention, worst first, with what each one says is
  wrong. A fleet with nothing wrong says so in a sentence of its own, which is
  the answer most walls show most of the time.
-->
<script lang="ts">
  import { PRINTER_STATE_LABEL_KEY } from '../../../domain';
  import { Badge, EmptyState, Tile, TilePanel } from '../../../primitives';
  import CircleCheckIcon from '@lucide/svelte/icons/circle-check';

  import { needsPerson, ordered, selected } from '../printers';
  import { stateBadgeVariant } from '../../printers/presentation';
  import { t } from '../../../i18n';
  import { dashboardContext } from '../context';
  import { tagSettings } from './config';
  import FleetState from './fleet-state.svelte';
  import type { CardProps } from '../../../slots';

  let { config, title }: CardProps = $props();

  const { printers } = dashboardContext();

  const settings = $derived(tagSettings(config));

  const views = $derived(
    ordered(
      printers.views.filter(
        (view) => selected(view, settings) && needsPerson(view),
      ),
      'state',
    ),
  );
</script>

<TilePanel {title} fallback={t('dashboard.issues.title')} gap="md">
  {#snippet trailing()}
    {#if printers.loaded && views.length > 0}
      <Badge variant="destructive" class="tabular-nums">{views.length}</Badge>
    {/if}
  {/snippet}
  <FleetState>
    {#if views.length === 0}
      <EmptyState
        size="sm"
        icon={CircleCheckIcon}
        heading={t('dashboard.issues.none')}
        description={t('dashboard.issues.noneNote')}
      />
    {:else}
      <ul
        data-part="issues"
        class="me-0 flex min-h-0 flex-1 flex-col gap-dashboard-issue-gap overflow-y-auto pe-0"
        aria-live="polite"
      >
        {#each views as view (view.id)}
          <li>
            <Tile
              href={view.detailHref}
              size="list"
              quiet
              class="@container/issue"
            >
              <span class="flex flex-col gap-1">
                <span
                  class="flex items-center justify-between gap-2 @max-3xs/issue:flex-col @max-3xs/issue:items-start @max-3xs/issue:gap-1"
                >
                  <span class="min-w-0 truncate text-sm font-medium"
                    >{view.name}</span
                  >
                  <Badge
                    variant={stateBadgeVariant(view.state)}
                    class="shrink-0"
                  >
                    {t(PRINTER_STATE_LABEL_KEY[view.state])}
                  </Badge>
                </span>
                {#if view.statusNote !== undefined}
                  <span class="line-clamp-2 text-xs text-muted-foreground">
                    {view.statusNote}
                  </span>
                {/if}
              </span>
            </Tile>
          </li>
        {/each}
      </ul>
    {/if}
  </FleetState>
</TilePanel>
