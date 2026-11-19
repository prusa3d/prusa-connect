<script lang="ts">
  import {
    Badge,
    Button,
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    EmptyState,
    FaultAlert,
    PageHeader,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Skeleton,
    StatusCard,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    ToggleGroup,
    ToggleGroupItem,
  } from '@prusa/connect-ui/primitives';
  import { describeFault, formatNumber } from '@prusa/connect-ui/i18n';
  import { printerHref } from '@prusa/connect-ui/shell';
  import { onMount } from 'svelte';

  import { daysOf, PERIODS, statsFor } from './data/stats.svelte';
  import { heldHost } from './host';
  import { UtilizationGrid } from './utilization-grid';
  import FilesTable from './view/files-table.svelte';
  import { comparison, metricsFor, rateOf } from './view/metrics';
  import {
    type PageView,
    pageViewOf,
    searchOf,
    WHOLE_PAGE,
  } from './view/page-query';
  import { TREND_METRICS, type TrendMetric } from './view/trend';
  import TrendChart from './view/trend-chart.svelte';

  const host = heldHost();
  const t = host.t;
  const stats = statsFor(host);
  const metrics = metricsFor(t);
  // The core's `state.*` words are reachable through `t`. A state it has no
  // word for is drawn as the printer reported it.
  const stateLabel = (state: string): string => {
    const label = t(`state.${state}`);
    return label === `state.${state}` ? state : label;
  };

  /** Every option of a select that narrows nothing. */
  const ALL = '*';

  let view = $state.raw<PageView>(WHOLE_PAGE);
  let metric = $state<TrendMetric>('jobs');

  const current = $derived(
    stats.view(view.days, {
      printers: view.printer === '' ? [] : [view.printer],
      tag: view.tag,
    }),
  );
  const printers = $derived(
    [...host.fleet.devices].sort((a, b) => a.name.localeCompare(b.name)),
  );
  const tags = $derived(
    [
      ...new Set(host.fleet.devices.flatMap((device) => device.tags ?? [])),
    ].sort((a, b) => a.localeCompare(b)),
  );

  /** Shows another view and writes it into the address, without a new history entry. */
  const show = (next: PageView) => {
    view = next;
    stats.setDays(next.days);
    const { pathname } = window.location;
    window.history.replaceState(
      window.history.state,
      '',
      `${pathname}${searchOf(next)}`,
    );
  };

  onMount(() => {
    view = pageViewOf(window.location.search);
    stats.setDays(view.days);
  });
</script>

{#snippet heading(text: string)}
  <CardTitle><h2 class="contents">{text}</h2></CardTitle>
{/snippet}

<div class="flex flex-col gap-6">
  <PageHeader title={t('fleet-stats.page.title')}>
    {#snippet actions()}
      <div class="flex flex-wrap items-center justify-end gap-2">
        <Select
          value={view.printer === '' ? ALL : view.printer}
          onValueChange={(value: string) =>
            show({ ...view, printer: value === ALL ? '' : value })}
          items={[
            { value: ALL, label: t('fleet-stats.scope.allPrinters') },
            ...printers.map((device) => ({
              value: device.id,
              label: device.name,
            })),
          ]}
        >
          <SelectTrigger
            size="sm"
            aria-label={t('fleet-stats.scope.printer')}
            class="w-fit"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent aria-label={t('fleet-stats.scope.printer')}>
            <SelectItem value={ALL}
              >{t('fleet-stats.scope.allPrinters')}</SelectItem
            >
            {#each printers as device (device.id)}
              <SelectItem value={device.id}>{device.name}</SelectItem>
            {/each}
          </SelectContent>
        </Select>
        {#if tags.length > 0}
          <Select
            value={view.tag === '' ? ALL : view.tag}
            onValueChange={(value: string) =>
              show({ ...view, tag: value === ALL ? '' : value })}
            items={[
              { value: ALL, label: t('fleet-stats.scope.allTags') },
              ...tags.map((tag) => ({ value: tag, label: tag })),
            ]}
          >
            <SelectTrigger
              size="sm"
              aria-label={t('fleet-stats.scope.tag')}
              class="w-fit"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent aria-label={t('fleet-stats.scope.tag')}>
              <SelectItem value={ALL}
                >{t('fleet-stats.scope.allTags')}</SelectItem
              >
              {#each tags as tag (tag)}
                <SelectItem value={tag}>{tag}</SelectItem>
              {/each}
            </SelectContent>
          </Select>
        {/if}
        <ToggleGroup
          type="single"
          aria-label={t('fleet-stats.range.label')}
          value={String(view.days)}
          onValueChange={(value: string) =>
            value && show({ ...view, days: daysOf(value) })}
        >
          {#each PERIODS as days (days)}
            <ToggleGroupItem value={String(days)}
              >{t('fleet-stats.range.days', { days })}</ToggleGroupItem
            >
          {/each}
        </ToggleGroup>
      </div>
    {/snippet}
  </PageHeader>

  {#if current.kind === 'loading'}
    <Skeleton class="h-24" />
    <Skeleton class="h-48" />
  {:else if current.kind === 'error'}
    <EmptyState heading={t('fleet-stats.error.heading')} headingLevel="h2">
      {#snippet action()}
        <FaultAlert fault={describeFault(current.thrown)} />
        <Button onclick={() => stats.reload()}
          >{t('fleet-stats.error.retry')}</Button
        >
      {/snippet}
    </EmptyState>
  {:else}
    {@const summary = current.stats}
    {@const failing = summary.byDevice.filter(
      (row) => row.failed + row.cancelled > 0,
    )}
    {#if summary.kpis.jobs === 0 && summary.kpis.printHours === 0}
      <EmptyState heading={t('fleet-stats.empty.heading')} headingLevel="h2" />
    {:else}
      <div class="grid grid-cols-2 gap-3 md:grid-cols-5">
        {#each metrics as item (item.key)}
          {@const change = comparison(t, item, summary.kpis, summary.previous)}
          <StatusCard
            title={t(item.label)}
            edge
            class="border-s-border!"
            value={item.show(summary.kpis)}
            trend={change?.trend}
            trendLabel={change?.label}
          />
        {/each}
      </div>

      <Card>
        <CardHeader
          class="flex flex-row flex-wrap items-center justify-between gap-2"
          >{@render heading(t('fleet-stats.trend.heading'))}
          <ToggleGroup
            type="single"
            aria-label={t('fleet-stats.trend.pick')}
            value={metric}
            onValueChange={(value: string) =>
              value && (metric = value as TrendMetric)}
          >
            {#each TREND_METRICS as item (item)}
              <ToggleGroupItem value={item}
                >{t(`fleet-stats.trend.metric.${item}`)}</ToggleGroupItem
              >
            {/each}
          </ToggleGroup>
        </CardHeader>
        <CardContent>
          <TrendChart trend={summary.trend} {metric} class="h-56" />
        </CardContent>
      </Card>

      <Card>
        <CardHeader
          >{@render heading(t('fleet-stats.utilization.heading'))}</CardHeader
        >
        <CardContent>
          <UtilizationGrid rows={summary.utilization} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader
          >{@render heading(t('fleet-stats.files.heading'))}</CardHeader
        >
        <CardContent>
          <FilesTable files={summary.files} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader
          >{@render heading(t('fleet-stats.failures.heading'))}</CardHeader
        >
        <CardContent class="flex flex-col gap-2">
          {#if failing.length === 0}
            <p class="text-sm text-muted-foreground">
              {t('fleet-stats.cards.failures.none')}
            </p>
          {:else}
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('fleet-stats.failures.printer')}</TableHead>
                  <TableHead class="text-end"
                    >{t('fleet-stats.failures.jobs')}</TableHead
                  >
                  <TableHead class="text-end"
                    >{t('fleet-stats.failures.success')}</TableHead
                  >
                  <TableHead class="text-end"
                    >{t('fleet-stats.failures.failed')}</TableHead
                  >
                  <TableHead class="text-end"
                    >{t('fleet-stats.failures.cancelled')}</TableHead
                  >
                </TableRow>
              </TableHeader>
              <TableBody>
                {#each failing as row (row.key)}
                  <TableRow>
                    <TableCell
                      ><a class="hover:underline" href={printerHref(row.key)}
                        >{row.label}</a
                      ></TableCell
                    >
                    <TableCell class="text-end"
                      >{formatNumber(row.jobs)}</TableCell
                    >
                    <TableCell class="text-end"
                      >{rateOf(t, row.successRate)}</TableCell
                    >
                    <TableCell class="text-end"
                      >{formatNumber(row.failed)}</TableCell
                    >
                    <TableCell class="text-end"
                      >{formatNumber(row.cancelled)}</TableCell
                    >
                  </TableRow>
                {/each}
              </TableBody>
            </Table>
          {/if}
          {#if summary.byMachineState.length > 0}
            <p class="text-sm text-muted-foreground">
              {t('fleet-stats.failures.byState')}
              {#each summary.byMachineState as { state, count } (state)}
                <Badge variant="secondary" class="ms-2"
                  ><span>{stateLabel(state)}</span>
                  {formatNumber(count)}</Badge
                >
              {/each}
            </p>
          {/if}
        </CardContent>
      </Card>
    {/if}
  {/if}
</div>
