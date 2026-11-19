<!--
  One slicing job: where it is, what it sliced with which slicer, the G-code
  it wrote and what the file says about the print, or why it failed with the
  end of what PrusaSlicer printed. It is drawn the way the app's other detail
  screens are: a trail back to the list, a header with the actions, and the
  facts in sections.
-->
<script lang="ts">
  import {
    formatBytes,
    formatDateTime,
    formatDuration,
  } from '@prusa/connect-ui/i18n';
  import {
    Alert,
    AlertDescription,
    AlertTitle,
    Badge,
    Button,
    Card,
    CardContent,
    CodeBlock,
    DetailHeader,
    EmptyState,
    KeyValueList,
    type KeyValueRow,
    PageTrail,
    Progress,
    Separator,
  } from '@prusa/connect-ui/primitives';
  import CircleXIcon from '@lucide/svelte/icons/circle-x';
  import ClockIcon from '@lucide/svelte/icons/clock';
  import FileCodeIcon from '@lucide/svelte/icons/file-code';
  import FolderOpenIcon from '@lucide/svelte/icons/folder-open';
  import ScrollTextIcon from '@lucide/svelte/icons/scroll-text';
  import SearchXIcon from '@lucide/svelte/icons/search-x';
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

  import type { Job, Translate } from '../../api';
  import {
    durationOf,
    fileHref,
    isActive,
    outputOf,
    projectOf,
    slicerOf,
  } from '../../jobs';
  import { JobState } from '../job-state';

  let {
    id,
    job,
    backHref,
    onCancel,
    t,
  }: {
    /** The id the address names. */
    id: string;
    /** Undefined when the list holds no job of that id. */
    job: Job | undefined;
    /** The Jobs tab, with the filters the person left it with. */
    backHref: string;
    onCancel: (id: string) => void;
    t: Translate;
  } = $props();

  const name = $derived(job ? projectOf(job) : id);

  const known = (rows: readonly KeyValueRow[]): KeyValueRow[] =>
    rows.filter((entry) => entry.value !== undefined && entry.value !== '');

  const number = (value: number | undefined, unit: string, digits = 2) =>
    value === undefined
      ? undefined
      : `${value.toLocaleString(undefined, { maximumFractionDigits: digits })} ${unit}`;

  const run = $derived.by(() => {
    if (!job) return [];
    const took = durationOf(job);

    return known([
      {
        id: 'created',
        label: t('slicer.column.created'),
        value: formatDateTime(job.created_at),
      },
      {
        id: 'started',
        label: t('slicer.detail.started'),
        value: job.started_at ? formatDateTime(job.started_at) : undefined,
      },
      {
        id: 'finished',
        label: t('slicer.detail.finished'),
        value: job.finished_at ? formatDateTime(job.finished_at) : undefined,
      },
      {
        id: 'took',
        label: t('slicer.column.duration'),
        value: took === undefined ? undefined : formatDuration(took),
      },
      { id: 'slicer', label: t('slicer.column.slicer'), value: slicerOf(job) },
      { id: 'by', label: t('slicer.column.by'), value: job.by, mono: true },
      {
        id: 'id',
        label: t('slicer.detail.jobId'),
        value: job.id,
        mono: true,
        copyable: true,
      },
    ]);
  });

  const project = $derived.by(() => {
    if (!job) return [];
    const items = job.selection.items?.length ?? 0;

    return known([
      {
        id: 'source',
        label: t('slicer.detail.source'),
        value: job.source.upload
          ? t('slicer.jobs.uploaded')
          : `${job.source.storage ?? ''}: ${job.source.path ?? ''}`,
        mono: job.source.upload === undefined,
        copyable: job.source.upload === undefined,
      },
      {
        id: 'bed',
        label: t('slicer.column.bed'),
        value: t('slicer.jobs.bed', {
          bed: String((job.selection.bed ?? 0) + 1),
        }),
      },
      {
        id: 'items',
        label: t('slicer.detail.items'),
        value:
          items > 0
            ? t('slicer.jobs.items', { count: String(items) })
            : t('slicer.detail.everyItem'),
      },
      {
        id: 'target',
        label: t('slicer.detail.target'),
        value:
          job.output.storage || job.output.path
            ? `${job.output.storage ?? ''}: ${job.output.path ?? ''}`
            : undefined,
        mono: true,
      },
      { id: 'format', label: t('slicer.detail.format'), value: job.format },
      {
        id: 'overwrite',
        label: t('slicer.detail.overwrite'),
        value: job.overwrite ? t('slicer.detail.yes') : t('slicer.detail.no'),
      },
    ]);
  });

  const output = $derived.by(() => {
    const result = job?.result;
    if (!result) return [];

    return known([
      {
        id: 'path',
        label: t('slicer.detail.path'),
        value: `${result.storage}: ${result.path}`,
        mono: true,
        copyable: true,
      },
      {
        id: 'size',
        label: t('slicer.column.size'),
        value: formatBytes(result.size),
      },
      { id: 'format', label: t('slicer.detail.format'), value: result.format },
      {
        id: 'sha256',
        label: t('slicer.detail.sha256'),
        value: result.sha256,
        mono: true,
        copyable: true,
      },
      {
        id: 'time',
        label: t('slicer.detail.printTime'),
        value: result.estimated_seconds
          ? formatDuration(result.estimated_seconds)
          : undefined,
      },
      {
        id: 'printer',
        label: t('slicer.detail.printer'),
        value: result.printer_model,
      },
      {
        id: 'filament',
        label: t('slicer.detail.filament'),
        value: [
          result.filament_type,
          number(result.filament_g, 'g'),
          number(result.filament_mm, 'mm', 0),
        ]
          .filter((part) => part !== undefined)
          .join(' · '),
      },
      {
        id: 'nozzle',
        label: t('slicer.detail.nozzle'),
        value: number(result.nozzle_mm, 'mm'),
      },
      {
        id: 'layer',
        label: t('slicer.detail.layer'),
        value: number(result.layer_height_mm, 'mm'),
      },
      {
        id: 'objects',
        label: t('slicer.detail.objects'),
        value: String(result.objects.length),
      },
      {
        id: 'sliced',
        label: t('slicer.detail.slicedIn'),
        value: formatDuration(
          Math.max(1, Math.round(result.duration_ms / 1000)),
        ),
      },
      {
        id: 'memory',
        label: t('slicer.detail.memory'),
        value: result.peak_memory_bytes
          ? formatBytes(result.peak_memory_bytes)
          : undefined,
      },
    ]);
  });
</script>

{#snippet part(text: string, Glyph: typeof ClockIcon)}
  <h2 class="flex items-center gap-2 text-sm font-medium">
    <Glyph aria-hidden="true" class="size-4 text-muted-foreground" />
    {text}
  </h2>
{/snippet}

{#snippet titleAside()}
  {#if job}
    <Badge variant="outline" numeric>{slicerOf(job)}</Badge>
  {/if}
{/snippet}

{#snippet meta()}
  {#if job}<JobState {job} {t} />{/if}
{/snippet}

{#snippet actions()}
  {#if job?.result}
    {@const href = fileHref(job.result.storage, job.result.path)}
    <Button>
      {#snippet child({ props })}
        <a {...props} {href}>
          <FileCodeIcon aria-hidden="true" />
          {t('slicer.action.openOutput')}
        </a>
      {/snippet}
    </Button>
  {/if}
  {#if job && isActive(job)}
    <Button variant="secondary" onclick={() => onCancel(job.id)}>
      <CircleXIcon aria-hidden="true" />
      {t('slicer.action.cancel')}
    </Button>
  {/if}
{/snippet}

<div class="grid w-full grid-cols-1 content-start gap-4" data-part="job-detail">
  <PageTrail
    trail={[{ label: t('slicer.tab.jobs'), href: backHref }]}
    page={name}
    sansPage
  />

  {#if job === undefined}
    <Card>
      <CardContent>
        <EmptyState
          icon={SearchXIcon}
          heading={t('slicer.detail.jobMissing')}
          headingLevel="h1"
          description={t('slicer.detail.jobMissingNote')}
        />
      </CardContent>
    </Card>
  {:else}
    <DetailHeader title={name} {titleAside} {meta} {actions} />

    {#if job.state === 'running'}
      <Progress
        value={job.progress >= 0 ? job.progress : null}
        aria-label={t('slicer.jobs.progress', { name })}
        class="h-1"
      />
    {/if}

    {#if job.error}
      <Alert variant="destructive" data-part="job-error">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>{t(`error.${job.error.code}`)}</AlertTitle>
        <AlertDescription class="break-words">
          <span>{job.error.message}</span>
          <span class="font-mono text-xs">{job.error.code}</span>
        </AlertDescription>
      </Alert>
    {/if}
    {#each job.result?.warnings ?? [] as warning (warning)}
      <Alert>
        <TriangleAlertIcon aria-hidden="true" />
        <AlertDescription class="break-words">{warning}</AlertDescription>
      </Alert>
    {/each}

    <Card>
      <CardContent class="grid grid-cols-1 content-start gap-6">
        <section class="grid grid-cols-1 content-start gap-4">
          {@render part(t('slicer.detail.run'), ClockIcon)}
          <KeyValueList rows={run} labelWidth="14rem" />
        </section>
        <Separator />
        <section class="grid grid-cols-1 content-start gap-4">
          {@render part(t('slicer.detail.project'), FolderOpenIcon)}
          <KeyValueList rows={project} labelWidth="14rem" />
        </section>
        {#if job.result}
          {@const result = job.result}
          <Separator />
          <section
            class="grid grid-cols-1 content-start gap-4"
            data-part="output"
          >
            {@render part(t('slicer.detail.output'), FileCodeIcon)}
            <KeyValueList rows={output} labelWidth="14rem" />
            <div class="text-sm">
              <Button variant="link" size="inline-icon">
                {#snippet child({ props })}
                  <a {...props} href={fileHref(result.storage, result.path)}>
                    <FileCodeIcon
                      aria-hidden="true"
                      class="size-3.5 shrink-0"
                    />
                    <span class="truncate">{outputOf(job)}</span>
                  </a>
                {/snippet}
              </Button>
            </div>
          </section>
        {/if}
        {#if job.error?.log && job.error.log.length > 0}
          <Separator />
          <section
            class="grid min-w-0 grid-cols-1 content-start gap-4"
            data-part="log"
          >
            {@render part(t('slicer.detail.log'), ScrollTextIcon)}
            <CodeBlock
              label={t('slicer.detail.log')}
              value={job.error.log.join('\n')}
              heightClass="h-64"
            />
          </section>
        {/if}
      </CardContent>
    </Card>
  {/if}
</div>
