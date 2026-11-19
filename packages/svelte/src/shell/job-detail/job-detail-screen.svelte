<script lang="ts">
  import ClockIcon from '@lucide/svelte/icons/clock';
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';
  import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
  import SearchXIcon from '@lucide/svelte/icons/search-x';

  import {
    PRINTER_STATE_LABEL_KEY,
    PRINTER_STATE_PROGRESS_TONE,
    printStateOf,
  } from '../../domain';
  import {
    describeFault,
    formatDateTime,
    formatDuration,
    formatUnit,
    t,
  } from '../../i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Badge,
    Button,
    Card,
    CardContent,
    DetailHeader,
    EmptyState,
    FaultAlert,
    FileThumbnail,
    KeyValueList,
    PageTrail,
    PrinterIcon,
    Progress,
    Separator,
    Skeleton,
    type KeyValueRow,
    PrinterGlyphIcon,
  } from '../../primitives';
  import { PrintDialog, type PrintableFile } from '../print-picker';
  import {
    printerArtworkOf,
    stateBadgeVariant,
  } from '../printers/presentation';
  import type { JobDetailScreenProps } from './types';

  const PARTS_ROW =
    'flex flex-col gap-6 @3xl/job-parts:flex-row @3xl/job-parts:items-start @3xl/job-parts:gap-10';

  const PARTS_ASIDE =
    'flex min-w-0 flex-1 flex-col gap-4 border-t border-border pt-6 @3xl/job-parts:border-t-0 @3xl/job-parts:border-s @3xl/job-parts:ps-10 @3xl/job-parts:pt-0';

  let {
    jobId,
    source,
    device,
    jobsHref,
    printerHref,
    fileHref,
    thumbnailUrl,
    transitions,
    locked,
    onPrint,
    onPrinted,
  }: JobDetailScreenProps = $props();

  // The print dialog sends the file again, to the printer that ran it.
  let reprinting = $state(false);

  const refusal = $derived(
    source.error === undefined ? undefined : describeFault(source.error),
  );

  /** The print, with the printer's own live reading over it while it runs. */
  const live = $derived(
    device?.job?.id === jobId && device.job.running ? device.job : undefined,
  );

  const job = $derived.by(() => {
    const row = source.job;
    if (row === undefined || live === undefined) return row;

    return {
      ...row,
      display_name:
        live.display_name === '' ? row.display_name : live.display_name,
      progress: live.progress,
      state: live.state === '' ? row.state : live.state,
      time_remaining_ms: live.time_remaining_ms,
    };
  });

  const waiting = $derived(!source.loaded && refusal === undefined);

  const missing = $derived(source.loaded && job === undefined);

  // The path names the print, so the trail has a subject before the row lands.
  const name = $derived(job?.display_name ?? jobId);

  const printState = $derived(
    job === undefined
      ? 'UNKNOWN'
      : printStateOf({
          state: job.state,
          ended: job.ended !== undefined,
          outcome: job.outcome,
        }),
  );

  // `ended` is what tells a running print from a finished one.
  const running = $derived(job !== undefined && job.ended === undefined);

  const span = $derived.by(() => {
    if (job?.ended === undefined) return undefined;
    const seconds =
      (new Date(job.ended).getTime() - new Date(job.started).getTime()) / 1000;

    return Number.isFinite(seconds) ? formatDuration(seconds) : undefined;
  });

  // The printer already holds the file, so the dialog names no storage.
  const again = $derived<PrintableFile | undefined>(
    job === undefined
      ? undefined
      : { name: job.display_name, source: job.path, thumbnailUrl },
  );

  const fileLink = $derived(
    job === undefined ? undefined : fileHref?.(job.path),
  );

  // A running print is not started again, and a printer this page does not
  // know has nowhere to send the file.
  const reprintable = $derived(
    onPrint !== undefined &&
      device !== undefined &&
      job !== undefined &&
      !running,
  );

  const timeLeft = $derived(
    running && job !== undefined
      ? formatDuration(job.time_remaining_ms / 1000)
      : undefined,
  );

  // The number the machine issued for the print, beside the print's own id.
  const printerJobId = $derived(
    job === undefined || job.device_job_id === ''
      ? undefined
      : job.device_job_id,
  );

  /** A row with no value is dropped rather than drawn with an absent word. */
  const known = (rows: readonly KeyValueRow[]): KeyValueRow[] =>
    rows.filter((row) => row.value !== undefined && row.value !== '');

  const run = $derived(
    job === undefined
      ? []
      : known([
          {
            id: 'started',
            label: t('jobDetail.run.started'),
            value: formatDateTime(job.started),
          },
          {
            id: 'ended',
            label: t('jobDetail.run.ended'),
            value:
              job.ended === undefined ? undefined : formatDateTime(job.ended),
          },
          { id: 'duration', label: t('jobDetail.run.duration'), value: span },
          { id: 'left', label: t('jobDetail.run.timeLeft'), value: timeLeft },
        ]),
  );

  const origin = $derived(
    job === undefined
      ? []
      : known([
          {
            id: 'model',
            label: t('jobDetail.origin.model'),
            value: device?.model,
          },
          {
            id: 'job',
            label: t('jobDetail.origin.jobId'),
            value: job.id,
            mono: true,
            copyable: true,
          },
          {
            id: 'printer-job',
            label: t('jobDetail.origin.printerJobId'),
            value: printerJobId,
            mono: true,
            copyable: true,
          },
        ]),
  );

  const file = $derived(
    job === undefined
      ? []
      : known([
          {
            id: 'name',
            label: t('jobDetail.file.name'),
            value: job.display_name,
            mono: true,
          },
          {
            id: 'path',
            label: t('jobDetail.file.path'),
            value: job.path,
            mono: true,
            copyable: true,
          },
        ]),
  );
</script>

{#snippet part(text: string, Glyph: typeof ClockIcon)}
  <h2 class="flex items-center gap-2 text-sm font-medium">
    <Glyph aria-hidden="true" class="size-4 text-muted-foreground" />
    {text}
  </h2>
{/snippet}

<div class="mx-auto flex w-full max-w-7xl flex-col gap-6">
  <!-- A print is named by the file it ran. -->
  <PageTrail
    trail={[{ label: t('jobDetail.jobs'), href: jobsHref }]}
    page={name}
    sansPage
  />

  <FaultAlert fault={refusal} />

  {#if missing}
    <Card>
      <CardContent>
        <EmptyState
          icon={SearchXIcon}
          heading={t('jobDetail.missing.heading')}
          headingLevel="h1"
          description={t('jobDetail.missing.detail')}
        />
      </CardContent>
    </Card>
  {:else}
    <DetailHeader
      title={name}
      loading={waiting}
      {media}
      {titleAside}
      {meta}
      {actions}
    />

    {#if job}
      <!-- The print's measure, at the page's own width under the header. -->
      <div class="flex items-center gap-3">
        <Progress
          value={job.progress}
          tone={PRINTER_STATE_PROGRESS_TONE[printState]}
          aria-label={t('jobDetail.progress.label', { name })}
          size="sm"
          class="flex-1"
        />
        <span class="shrink-0 font-mono text-sm tabular-nums">
          {formatUnit(job.progress, '%', { maximumFractionDigits: 0 })}
        </span>
      </div>
    {/if}

    {#if printState === 'ERROR'}
      <Alert variant="destructive">
        <AlertTitle>{t('jobDetail.failed.heading')}</AlertTitle>
        <AlertDescription>{t('jobDetail.failed.detail')}</AlertDescription>
      </Alert>
    {/if}

    <Card data-settings-panel>
      <CardContent class="@container/job-parts flex flex-col gap-6">
        {#if waiting}
          <!-- The card's own shape while the read is in flight. -->
          <div
            class="flex flex-col gap-2"
            role="status"
            aria-label={t('jobDetail.loading')}
          >
            {#each [0, 1, 2, 3] as row (row)}
              <Skeleton class="h-5 w-full" />
            {/each}
          </div>
          <Separator />
          <Skeleton class="h-20 w-full" />
          <Separator />
          <Skeleton class="h-20 w-full" />
        {:else if job}
          <div class={PARTS_ROW}>
            <div class="flex min-w-0 flex-1 flex-col gap-6">
              <section class="flex flex-col gap-4">
                {@render part(t('jobDetail.run.heading'), ClockIcon)}
                <KeyValueList rows={run} />
              </section>

              <Separator />

              <section class="flex flex-col gap-4">
                {@render part(t('jobDetail.file.heading'), FileCode2Icon)}
                <KeyValueList rows={file} />
                {#if fileLink}
                  <Button variant="link" size="inline-icon" class="w-fit">
                    {#snippet child({ props })}
                      <a {...props} href={fileLink}>
                        <FileCode2Icon
                          aria-hidden="true"
                          class="size-3.5 shrink-0"
                        />
                        {t('jobDetail.file.open')}
                      </a>
                    {/snippet}
                  </Button>
                {/if}
              </section>
            </div>

            <section class={PARTS_ASIDE}>
              {@render part(t('jobDetail.origin.heading'), PrinterGlyphIcon)}
              <KeyValueList rows={origin} />
            </section>
          </div>
        {/if}
      </CardContent>
    </Card>
  {/if}
</div>

{#snippet media()}
  <FileThumbnail {name} glyph={FileCode2Icon} {thumbnailUrl} class="size-24" />
{/snippet}

{#snippet titleAside()}
  {#if job}
    <Badge size="md" variant={stateBadgeVariant(printState)}>
      {t(PRINTER_STATE_LABEL_KEY[printState])}
    </Badge>
  {/if}
{/snippet}

{#snippet meta()}
  {#if device}
    <Badge variant="outline" size="lg">
      {#snippet child({ props })}
        <a {...props} href={printerHref}>
          <PrinterIcon
            model={printerArtworkOf(device)}
            reported={device.model}
            class="size-9"
          />
          {device.name}
        </a>
      {/snippet}
    </Badge>
  {/if}
{/snippet}

{#snippet actions()}
  {#if job}
    {#if reprintable}
      <Button
        size="md"
        onclick={() => {
          reprinting = true;
        }}
      >
        <RotateCcwIcon aria-hidden="true" />
        {t('jobDetail.action.printAgain')}
      </Button>
    {/if}
  {/if}
{/snippet}

{#if onPrint}
  <PrintDialog
    open={reprinting}
    onOpenChange={(next: boolean) => {
      reprinting = next;
    }}
    file={again}
    devices={device === undefined ? [] : [device]}
    {transitions}
    {locked}
    {onPrint}
    {onPrinted}
  />
{/if}
