<!--
  The log lines, newest first, with the filters a person narrows them by.

  Three empty answers are three sentences (ui-integration.md): nothing has
  arrived in the window, nothing matches the filters, or the lines could not
  be read. Live mode asks for newer lines every two seconds and stops when
  the view goes.

  On a printer's page the printer is fixed and its column is left out. Each
  line shows the address it came from, because a UDP datagram carries no
  proof of its sender and anybody on the network can send one.
-->
<script lang="ts">
  import { formatDate } from '@prusa/connect-ui/i18n';
  import {
    Badge,
    Button,
    EmptyState,
    type FacetDimension,
    ListingNoMatch,
    FaultAlert,
    Input,
    Label,
    ListingSearch,
    ListingToolbar,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Skeleton,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
  } from '@prusa/connect-ui/primitives';
  import ScrollTextIcon from '@lucide/svelte/icons/scroll-text';
  import { onMount, untrack } from 'svelte';

  import type { LogLine } from '../api';
  import { faultText } from '../faults';
  import { type LogFilter, LogsStore } from '../logs.svelte';
  import {
    FILTER_LEVELS,
    type FilterLevel,
    originOf,
    originValue,
    severityVariant,
    WINDOWS,
    type Window,
  } from '../presentation';
  import { heldSession } from '../session';

  let {
    deviceId,
    start,
    onfilter,
  }: {
    deviceId?: string | undefined;
    /** The filters the page opens with, read once. */
    start?: LogFilter | undefined;
    /** Told after each change of the filters, for the page to keep them. */
    onfilter?: ((filter: LogFilter) => void) | undefined;
  } = $props();

  const { host, api, status } = heldSession();
  const t = host.t;
  const uid = $props.id();

  const fixed = untrack(() => deviceId);
  const logs = new LogsStore(api, {
    ...untrack(() => start),
    ...(fixed === undefined ? {} : { origin: { kind: 'device', id: fixed } }),
  });

  // Every change of the filters goes through here, so the page hears of each.
  const narrow = (next: Partial<LogFilter>): void => {
    void logs.set(next);
    onfilter?.(logs.filter);
  };

  // Every filter that narrows the lines, but not the window or a printer the
  // page is fixed to.
  const clearFilters = (): void => {
    component = '';
    narrow({
      origin: fixed === undefined ? { kind: 'all' } : logs.filter.origin,
      level: undefined,
      component: '',
      text: '',
    });
  };

  // What clearFilters can take back: a printer the page is fixed to is not a
  // filter.
  const filtered = $derived(
    logs.filter.level !== undefined ||
      logs.filter.component !== '' ||
      logs.filter.text !== '' ||
      (fixed === undefined && logs.filter.origin.kind !== 'all'),
  );

  onMount(() => {
    void logs.load();

    return () => {
      logs.dispose();
    };
  });

  /** Reads the newest page again, for the page's refresh. */
  export const reload = (): Promise<void> => logs.load();

  const origins = $derived([
    ...host.fleet.devices.map((one) => ({
      value: originValue({ kind: 'device', id: one.id }),
      label: one.name || one.id,
    })),
    ...status.unmatched.map((one) => ({
      value: originValue({ kind: 'mac', mac: one.mac }),
      label: t('buddyDebugLogs.filter.unmatched', { mac: one.mac }),
    })),
  ]);

  const windowLabel = (hours: number): string =>
    t('buddyDebugLogs.filter.windowHours', { hours });

  // A window always holds a value, so clearing it falls back to the default.
  const facets = $derived<FacetDimension[]>([
    ...(fixed === undefined
      ? [
          {
            id: 'origin',
            label: t('buddyDebugLogs.filter.origin'),
            mode: 'single' as const,
            allLabel: t('buddyDebugLogs.filter.allPrinters'),
            values: origins.map((one) => one.value),
            labelFor: (value: string) =>
              origins.find((one) => one.value === value)?.label ?? value,
            selected:
              logs.filter.origin.kind === 'all'
                ? []
                : [originValue(logs.filter.origin)],
            onSelectedChange: (next: string[]) => {
              narrow({ origin: originOf(next[0] ?? 'all') });
            },
          },
        ]
      : []),
    {
      id: 'level',
      label: t('buddyDebugLogs.filter.level'),
      mode: 'single',
      allLabel: t('buddyDebugLogs.filter.allLevels'),
      values: [...FILTER_LEVELS],
      labelFor: (value: string) =>
        t(`buddyDebugLogs.filter.levelAtLeast.${value}`),
      selected: logs.filter.level === undefined ? [] : [logs.filter.level],
      onSelectedChange: (next: string[]) => {
        narrow({ level: next[0] as FilterLevel | undefined });
      },
    },
  ]);

  const windows = $derived(
    WINDOWS.map((hours) => ({
      value: String(hours),
      label: windowLabel(hours),
    })),
  );

  const fault = $derived(
    logs.fault === undefined ? undefined : faultText(logs.fault, t),
  );

  const printerOf = (line: LogLine): string => {
    if (line.device_id) {
      const name = host.fleet.device(line.device_id)?.name ?? '';

      return name === '' ? line.device_id : name;
    }

    return line.mac ?? t('buddyDebugLogs.logs.unmatchedSender');
  };

  const when = (at: string): string =>
    formatDate(at, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });

  // The component filter is read when the person leaves the box or presses
  // Enter, so each keystroke does not cost a request.
  let component = $state(untrack(() => logs.filter.component));
</script>

<div class="grid gap-3" data-part="log-view">
  <div data-part="log-filters">
    <ListingToolbar
      {facets}
      filtersActive={filtered}
      onClearFilters={clearFilters}
    >
      {#snippet search()}
        <ListingSearch
          value={logs.filter.text}
          onValue={(next: string) => {
            narrow({ text: next });
          }}
          maxlength={256}
          label={t('buddyDebugLogs.filter.text')}
          placeholder={t('buddyDebugLogs.filter.textPlaceholder')}
        />
      {/snippet}
      {#snippet afterFacets()}
        <Select
          value={String(logs.filter.window)}
          items={windows}
          onValueChange={(next: string) => {
            narrow({ window: Number(next) as Window });
          }}
        >
          <SelectTrigger
            size="sm"
            class="min-w-36"
            aria-label={t('buddyDebugLogs.filter.window')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {#each windows as option (option.value)}
              <SelectItem value={option.value} label={option.label}>
                {option.label}
              </SelectItem>
            {/each}
          </SelectContent>
        </Select>
        <div class="w-56">
          <Input
            aria-label={t('buddyDebugLogs.filter.component')}
            size="sm"
            maxlength={64}
            placeholder={t('buddyDebugLogs.filter.componentPlaceholder')}
            value={component}
            oninput={(event: Event) => {
              component = (event.currentTarget as HTMLInputElement).value;
            }}
            onchange={() => {
              narrow({ component });
            }}
          />
        </div>
      {/snippet}
      {#snippet trailing()}
        <div class="flex h-8 items-center gap-2">
          <Switch
            id={`${uid}-live`}
            checked={logs.live}
            onCheckedChange={(next: boolean) => {
              logs.setLive(next);
            }}
          />
          <Label for={`${uid}-live`}>
            {t('buddyDebugLogs.filter.live')}
          </Label>
        </div>
      {/snippet}
    </ListingToolbar>
  </div>

  <FaultAlert {fault} />

  {#if logs.lines === undefined}
    {#if fault === undefined}
      <div class="grid gap-2" data-part="log-skeleton">
        {#each [0, 1, 2, 3, 4] as row (row)}
          <Skeleton class="h-6" />
        {/each}
      </div>
    {/if}
  {:else if logs.lines.length === 0}
    {#if filtered}
      <div data-part="log-no-match">
        <ListingNoMatch
          headingLevel="h3"
          heading={t('buddyDebugLogs.logs.noMatch.heading')}
          description={t('buddyDebugLogs.logs.noMatch.note')}
          onClear={clearFilters}
        />
      </div>
    {:else}
      <EmptyState
        icon={ScrollTextIcon}
        headingLevel="h3"
        heading={t('buddyDebugLogs.logs.empty.heading')}
        description={t('buddyDebugLogs.logs.empty.note')}
        data-part="log-empty"
      />
    {/if}
  {:else}
    <div
      class="overflow-x-auto rounded-md border border-border"
      aria-busy={logs.loading}
    >
      <Table data-part="log-table">
        <TableHeader>
          <TableRow>
            <TableHead class="w-36">{t('buddyDebugLogs.column.time')}</TableHead
            >
            {#if fixed === undefined}
              <TableHead>{t('buddyDebugLogs.column.printer')}</TableHead>
            {/if}
            <TableHead class="w-28"
              >{t('buddyDebugLogs.column.address')}</TableHead
            >
            <TableHead class="w-24"
              >{t('buddyDebugLogs.column.severity')}</TableHead
            >
            <TableHead class="w-28"
              >{t('buddyDebugLogs.column.component')}</TableHead
            >
            <TableHead>{t('buddyDebugLogs.column.text')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {#each logs.lines as line (line.cursor)}
            <TableRow data-part="log-line">
              <TableCell
                class="text-xs whitespace-nowrap text-muted-foreground tabular-nums"
              >
                {when(line.at)}
              </TableCell>
              {#if fixed === undefined}
                <TableCell class="text-sm whitespace-nowrap"
                  >{printerOf(line)}</TableCell
                >
              {/if}
              <TableCell
                class="font-mono text-xs whitespace-nowrap text-muted-foreground"
                data-part="log-address">{line.ip ?? ''}</TableCell
              >
              <TableCell>
                {#if severityVariant(line.severity) === 'outline'}
                  <span class="text-sm text-muted-foreground"
                    >{t(`buddyDebugLogs.severity.${line.severity}`)}</span
                  >
                {:else}
                  <Badge variant={severityVariant(line.severity)}>
                    {t(`buddyDebugLogs.severity.${line.severity}`)}
                  </Badge>
                {/if}
              </TableCell>
              <TableCell class="font-mono text-xs"
                >{line.component ?? ''}</TableCell
              >
              <TableCell
                class="min-w-64 font-mono text-xs break-all whitespace-pre-wrap"
              >
                {line.text}
              </TableCell>
            </TableRow>
          {/each}
        </TableBody>
      </Table>
    </div>
    {#if logs.hasOlder}
      <div>
        <Button
          variant="secondary"
          size="sm"
          disabled={logs.loading}
          aria-busy={logs.loading}
          onclick={() => {
            void logs.loadOlder();
          }}
        >
          {t('buddyDebugLogs.action.older')}
        </Button>
      </div>
    {/if}
  {/if}
</div>
