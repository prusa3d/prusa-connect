<!-- What this printer has printed. -->
<script lang="ts">
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';
  import InboxIcon from '@lucide/svelte/icons/inbox';

  import { PRINTER_STATE_LABEL_KEY, printStateOf } from '../../../domain';
  import {
    describeFault,
    formatDateTime,
    formatDuration,
    formatUnit,
    t,
  } from '../../../i18n';
  import {
    Badge,
    Button,
    Card,
    CardContent,
    CardHeader,
    CardTitle,
    DataRow,
    EmptyState,
    FaultAlert,
    FileThumbnail,
    RowList,
    Skeleton,
    THUMBNAIL_ROW_MIN_HEIGHT,
  } from '../../../primitives';
  import { jobHref } from '../../jobs/presentation';
  import { stateBadgeVariant } from '../../printers/presentation';
  import type { PrintHistoryProps, PrintRow } from './types';

  let { history, recent, jobsHref, live, thumbnailFor }: PrintHistoryProps =
    $props();

  const refusal = $derived(
    history.jobsError === undefined
      ? undefined
      : describeFault(history.jobsError),
  );

  // The running print is never a row of the ended page.
  const ended = $derived(
    recent ?? (history.jobs?.last === undefined ? [] : [history.jobs.last]),
  );
  // The two reads run side by side, so a print that ended between them is in
  // both. An end is final, so the ended copy is the one kept.
  const rows = $derived.by<readonly PrintRow[]>(() => {
    const current = history.jobs?.current;
    const running =
      current === undefined || ended.some((one) => one.id === current.id)
        ? []
        : [current];

    return [...running, ...ended];
  });
  const nothing = $derived(history.jobsLoaded && rows.length === 0);

  const stateOf = (job: PrintRow) =>
    printStateOf({
      state: job.state,
      ended: job.ended !== undefined,
      outcome: job.outcome,
    });

  // Absent while the print runs.
  const spanOf = (job: PrintRow): string | undefined => {
    if (job.ended === undefined) return undefined;
    const seconds =
      (new Date(job.ended).getTime() - new Date(job.started).getTime()) / 1000;

    return Number.isFinite(seconds) ? formatDuration(seconds) : undefined;
  };
</script>

{#snippet row(job: PrintRow)}
  {#snippet state()}
    <Badge
      variant={stateBadgeVariant(stateOf(job))}
      mono={false}
      class="font-mono"
    >
      {t(PRINTER_STATE_LABEL_KEY[stateOf(job)])}
    </Badge>
  {/snippet}
  {#snippet plate()}
    <!-- Every print here is a plate. -->
    <FileThumbnail
      name={job.display_name}
      glyph={FileCode2Icon}
      thumbnailUrl={thumbnailFor?.(job.path)}
    />
  {/snippet}
  <DataRow
    class={THUMBNAIL_ROW_MIN_HEIGHT}
    href={jobHref(job.id)}
    leading={plate}
    primary={job.display_name}
    mono
    wrapValues
    secondaryValues={[
      state,
      formatUnit(
        job.ended === undefined && live?.id === job.id
          ? live.progress
          : job.progress,
        '%',
      ),
      t('printerDetailInfo.history.started', {
        when: formatDateTime(job.started),
      }),
      job.ended !== undefined &&
        t('printerDetailInfo.history.ended', {
          when: formatDateTime(job.ended),
        }),
      spanOf(job),
    ]}
  />
{/snippet}

<Card radius="md">
  <CardHeader>
    <CardTitle
      ><h2 class="contents">
        {t('printerDetailInfo.history.heading')}
      </h2></CardTitle
    >
  </CardHeader>
  <CardContent class="flex flex-col gap-4">
    {#if refusal}
      <FaultAlert fault={refusal} />
    {:else if !history.jobsLoaded}
      <!-- The card's own shape while the read is in flight. -->
      <div
        class="flex flex-col gap-4"
        role="status"
        aria-label={t('printerDetailInfo.history.loading')}
      >
        <Skeleton class="h-15 w-full rounded-sm" />
        <Skeleton class="h-15 w-full rounded-sm" />
      </div>
    {:else if nothing}
      <EmptyState
        icon={InboxIcon}
        heading={t('printerDetailInfo.history.empty.heading')}
        headingLevel="h3"
        description={t('printerDetailInfo.history.empty.note')}
      />
    {:else}
      <RowList>
        {#each rows as job (job.id)}
          {@render row(job)}
        {/each}
      </RowList>
    {/if}
    {#if jobsHref}
      <Button variant="link" class="self-start">
        {#snippet child({ props })}
          <a {...props} href={jobsHref}>
            {t('printerDetailInfo.history.all')}
          </a>
        {/snippet}
      </Button>
    {/if}
  </CardContent>
</Card>
