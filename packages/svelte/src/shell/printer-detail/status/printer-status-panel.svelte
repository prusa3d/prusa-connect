<script lang="ts">
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';
  import InfoIcon from '@lucide/svelte/icons/info';
  import Maximize2Icon from '@lucide/svelte/icons/maximize-2';
  import WifiOffIcon from '@lucide/svelte/icons/wifi-off';
  import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';

  import { PRINTER_STATE_PROGRESS_TONE } from '../../../domain';
  import {
    formatDate,
    formatDateTime,
    formatDuration,
    formatUnit,
    t,
  } from '../../../i18n';
  import {
    AspectRatio,
    Alert,
    AlertDescription,
    AlertTitle,
    Badge,
    Button,
    Card,
    CardContent,
    ClippedName,
    Dialog,
    DialogContent,
    DialogTitle,
    EmptyState,
    FileThumbnail,
    Progress,
    Skeleton,
  } from '../../../primitives';
  import CameraPicture from '../../printer-cameras/camera-picture.svelte';
  import {
    jobFallbackOf,
    printerRowOf,
    SEVERITY_LABEL_KEY,
  } from '../../printers/presentation';
  import FileAccess from './file-access.svelte';
  import {
    jobOf,
    PRINTER_STATE_VARIANT,
    SPLIT_LEADING,
    SPLIT_ROW,
    SPLIT_TRAILING,
    stateOf,
    wireStateOf,
  } from './presentation';
  import PrintControls from './print-controls.svelte';
  import PrinterDialog from './printer-dialog.svelte';
  import type { PrinterStatusPanelProps } from './types';

  let {
    device,
    telemetry,
    commands,
    issues,
    problemHref,
    now,
    camera,
    loading = false,
    transfers,
    uploads,
    onCancelUpload,
    history,
    recent,
    files,
    historyHref,
    thumbnailFor,
    onUpload,
    uploadBlock,
    keep,
    checkingStorages,
    onRecheck,
  }: PrinterStatusPanelProps = $props();

  /** The one label treatment every block on this panel names its parts with. */
  const BLOCK_LABEL = 'text-xs font-medium text-muted-foreground';

  const MS_PER_SECOND = 1000;

  const printerState = $derived(stateOf(telemetry, device));
  const variant = $derived(PRINTER_STATE_VARIANT[printerState]);
  const finished = $derived(variant === 'finished');
  const job = $derived(jobOf(telemetry, device));
  const progress = $derived(finished ? 100 : job.progress);
  const jobFallback = $derived(
    jobFallbackOf({ state: printerState, jobRunning: device.job?.running }),
  );

  // A bar that advances on a stopped print would be a lie.
  const tone = $derived(
    variant === 'notReporting'
      ? 'muted'
      : PRINTER_STATE_PROGRESS_TONE[printerState],
  );

  const split = $derived(variant === 'running' || variant === 'trouble');
  const showsCamera = $derived(
    (split || variant === 'finished') && camera !== undefined,
  );

  // Leaving the Now tab takes the camera away before the block that draws it
  // is gone, and the bindings below still read it once more.
  const cameraName = (): string | undefined => camera?.camera.name;

  // The camera whose live view runs, so another camera starts on its still.
  let playingName = $state<string | undefined>(undefined);
  const playing = $derived(
    camera !== undefined && playingName === camera.camera.name,
  );

  // Whether the camera has a live view to open, rather than only its still.
  // A module's own view is left out: the picture offers it only once a still
  // has loaded, which the panel cannot see.
  const streams = $derived(
    camera !== undefined &&
      (camera.pictures.live !== undefined ||
        (camera.camera.watch && camera.watch !== undefined)),
  );
  const openable = $derived(
    camera !== undefined &&
      camera.camera.serving &&
      (camera.pictures.still !== undefined || streams),
  );
  // The camera open at the size of the page rather than the column, and the
  // one whose live view runs there, so another camera neither opens nor plays.
  let expandedName = $state<string | undefined>(undefined);
  let expandedPlayingName = $state<string | undefined>(undefined);
  let expandedPanel = $state<HTMLElement | null>(null);
  // The enlarged picture opens on the live view when there is one, and takes
  // it over from the column, so the camera is never streamed twice.
  const expand = (): void => {
    if (camera === undefined) return;
    expandedPlayingName = streams ? camera.camera.name : undefined;
    playingName = undefined;
    expandedName = camera.camera.name;
  };

  // Counted from when the time left arrived. A default taken when the panel
  // opened would count it from then, however long ago that was.
  const finishesAt = $derived(
    job.remainingSeconds === undefined || variant !== 'running'
      ? undefined
      : formatDate(
          new Date(
            (now?.getTime() ?? Date.now()) +
              job.remainingSeconds * MS_PER_SECOND,
          ),
          {
            timeStyle: 'short',
          },
        ),
  );
  const finishedAt = $derived.by(() => {
    const id = device.job?.id;
    if (!finished || !id) return undefined;

    const last = history?.jobs?.last;
    const ended =
      recent?.find((print) => print.id === id)?.ended ??
      (last?.id === id ? last.ended : undefined);
    return ended === undefined
      ? undefined
      : formatDate(ended, { timeStyle: 'short' });
  });
  const remaining = $derived(
    job.remainingSeconds === undefined || variant !== 'running'
      ? ''
      : formatDuration(job.remainingSeconds),
  );
  const elapsed = $derived(
    job.elapsedSeconds === undefined || variant !== 'running'
      ? ''
      : formatDuration(job.elapsedSeconds),
  );

  // What is left and what is behind.
  const timing = $derived(
    [
      remaining === ''
        ? undefined
        : t('printerDetailStatus.remaining', { duration: remaining }),
      elapsed === ''
        ? undefined
        : t('printerDetailStatus.elapsed', { duration: elapsed }),
    ]
      .filter((part) => part !== undefined)
      .join(' · '),
  );

  // The bar is named after the print it measures.
  const progressLabel = $derived(
    job.name === undefined
      ? t('printerDetailStatus.progress.label')
      : t('printerDetailStatus.progress.labelFor', { name: job.name }),
  );

  const problem = $derived(
    printerRowOf(
      device,
      issues && ((reason: string) => issues.entryOf(reason)),
    ),
  );
  const departed = $derived(device.departed?.at);
  const hasReported = $derived(Object.keys(device.state).length > 0);
  const notReportingNote = $derived.by(() => {
    if (departed !== undefined) {
      return t('printerDetailStatus.notReporting.lastSeen', {
        when: formatDateTime(departed),
      });
    }

    return t(
      hasReported
        ? 'printerDetailStatus.notReporting.unreachable'
        : 'printerDetailStatus.notReporting.never',
    );
  });
  const problemNote = $derived(
    problem.statusNote?.trim() === '' ? undefined : problem.statusNote,
  );
  const problemAction = $derived(
    problem.statusAction?.trim() === '' ? undefined : problem.statusAction,
  );

  const needsAttention = $derived(
    printerState === 'ERROR' ||
      printerState === 'ATTENTION' ||
      (variant === 'trouble' && problemNote !== undefined),
  );
  const problemVariant = $derived.by(() => {
    if (printerState === 'ERROR' || problem.severity === 'error') {
      return 'destructive';
    }
    if (needsAttention || problem.severity === 'warning') return 'warning';
    return 'info';
  });
  const problemTitle = $derived.by(() => {
    if (needsAttention) {
      return t(
        printerState === 'ERROR'
          ? 'printerDetailStatus.problem.error'
          : 'printerDetailStatus.problem.attention',
      );
    }
    return problem.severity
      ? t(SEVERITY_LABEL_KEY[problem.severity])
      : t('printers.issue');
  });

  // A bar with no reading draws itself as indeterminate — a full, pulsing
  // track. On a printer holding no print at all that reads as a print running
  // at full tilt, so the bar belongs only to a print that exists.
  const measured = $derived(job.name !== undefined || job.progress !== null);

  // A stopped send leaves the machine naming a file it never kept, so the
  // files the prints name are looked up before Print again is offered, and
  // the printer reads the folder again for a print that was stopped or failed.
  $effect(() => {
    const ended = [history?.jobs?.last, ...(recent ?? [])];
    void files?.checkHeld(
      [job.path, ...ended.map((row) => row?.path)].filter(
        (path) => path !== undefined,
      ),
      [
        printerState === 'STOPPED' || printerState === 'ERROR'
          ? job.path
          : undefined,
        ...ended
          .filter(
            (row) => row?.outcome === 'cancelled' || row?.outcome === 'failed',
          )
          .map((row) => row?.path),
      ].filter((path) => path !== undefined),
    );
  });
  const showBar = $derived(measured || finishedAt !== undefined);
  // A printer that holds no print, such as one asking for a person between
  // prints, has nothing to say beside its camera, so the camera takes the card.
  const cameraAlone = $derived(
    showsCamera &&
      job.name === undefined &&
      jobFallback === undefined &&
      !showBar,
  );
</script>

{#snippet progressBar(summary = false)}
  {#if summary && (finishesAt !== undefined || finishedAt !== undefined || timing !== '' || (showBar && progress !== null))}
    <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1">
      {@render printTiming()}
      {#if showBar && progress !== null}
        <span class="ms-auto shrink-0 font-mono text-2xl tabular-nums">
          {formatUnit(progress, '%')}
        </span>
      {/if}
    </div>
  {/if}
  {#if showBar}
    <div class="flex items-center gap-3">
      <Progress
        value={progress}
        {tone}
        aria-label={progressLabel}
        size="sm"
        class="flex-1"
      />
      {#if !summary && progress !== null}
        <span
          class="shrink-0 font-mono text-xs text-muted-foreground tabular-nums"
        >
          {formatUnit(progress, '%')}
        </span>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet printTiming()}
  {#if finishedAt !== undefined}
    <span class="font-mono text-2xl tabular-nums">
      {t('printerDetailStatus.finishedAt', { when: finishedAt })}
    </span>
  {:else if finishesAt !== undefined}
    <span class="font-mono text-2xl whitespace-nowrap tabular-nums">
      {t('printerDetailStatus.finishes', { when: finishesAt })}
    </span>
  {/if}
  {#if timing !== ''}
    <span class="font-mono text-sm text-muted-foreground tabular-nums">
      {timing}
    </span>
  {/if}
{/snippet}

{#snippet jobName(row = false)}
  {#if job.name === undefined}
    {#if jobFallback !== undefined}
      <span class="min-w-0 flex-1 truncate text-base">{jobFallback}</span>
    {/if}
  {:else}
    <!-- Beside the controls, the name asks for 14rem: the plate and the
         clipped name's tail, with room for part of its head. In a row
         narrower than that plus the controls, the controls move to a line of
         their own, so they never draw over the name. Stacked over the bar,
         the same basis would be a height. -->
    <div class={['flex min-w-0 flex-1 items-center gap-3', row && 'basis-56']}>
      {@render plate(job.name)}
      <ClippedName name={job.name} class="min-w-0 flex-1 font-mono text-base" />
    </div>
  {/if}
{/snippet}

{#snippet plate(name: string)}
  <FileThumbnail
    {name}
    glyph={FileCode2Icon}
    class="shrink-0"
    thumbnailUrl={job.path === undefined ? undefined : thumbnailFor?.(job.path)}
  />
{/snippet}

{#snippet controls()}
  <PrintControls
    {device}
    {commands}
    state={wireStateOf(telemetry, device)}
    path={job.path}
    held={job.path === undefined ? undefined : files?.holds(job.path)}
    largePrintAgain={finished}
  />
{/snippet}

{#snippet currentPrint()}
  <div
    class={[
      'flex min-w-0 flex-1 flex-col gap-2',
      showsCamera && SPLIT_TRAILING,
    ]}
  >
    {#if job.name !== undefined || jobFallback !== undefined}
      <span class={BLOCK_LABEL}>{t('printerDetailStatus.currentPrint')}</span>
    {/if}
    <div class="flex flex-col gap-3">
      <div class="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
        {@render jobName(true)}
        {@render controls()}
      </div>
      {@render progressBar(variant === 'running')}
      {#if variant !== 'running' && (finishesAt !== undefined || timing !== '')}
        <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1">
          {@render printTiming()}
        </div>
      {/if}
    </div>
  </div>
{/snippet}

{#snippet compactPrint()}
  <div
    class={finished
      ? 'grid min-w-0 items-center gap-4 @lg/now:grid-cols-content-action @lg/now:gap-x-section-block'
      : 'flex flex-wrap items-center gap-x-3 gap-y-4'}
  >
    <div class="flex min-w-0 flex-1 flex-col gap-2">
      {#if finished}
        <span class={BLOCK_LABEL}>{t('printerDetailStatus.currentPrint')}</span>
      {/if}
      {@render jobName()}
      {@render progressBar(finished)}
    </div>
    <div class="ms-auto flex flex-wrap items-center gap-2">
      {@render controls()}
    </div>
  </div>
{/snippet}

<div class="@container/now flex flex-col gap-6">
  {#if loading}
    <!-- The panel's own shape while the record is on its way. -->
    <Card density="panel">
      <CardContent
        class="flex flex-col gap-6 pt-section-block pb-panel-padding-mobile @lg/card:pb-panel-padding"
      >
        <div
          class={SPLIT_ROW}
          aria-busy="true"
          aria-label={t('printerDetailStatus.loading')}
        >
          <div class={['flex flex-col gap-2', SPLIT_LEADING]}>
            <AspectRatio ratio={16 / 9}>
              <Skeleton class="size-full" />
            </AspectRatio>
          </div>
          <div class={['flex min-w-0 flex-1 flex-col gap-3', SPLIT_TRAILING]}>
            <Skeleton class="h-5 w-2/3" />
            <Skeleton class="h-1 w-full" />
            <Skeleton class="h-8 w-40" />
          </div>
        </div>
      </CardContent>
    </Card>
  {:else}
    {#if needsAttention || problemNote !== undefined}
      <Alert
        variant={problemVariant}
        role={needsAttention ? 'alert' : 'note'}
        data-slot="printer-problem"
      >
        {#if needsAttention || problem.severity === 'error' || problem.severity === 'warning'}
          <TriangleAlertIcon aria-hidden="true" />
        {:else}
          <InfoIcon aria-hidden="true" />
        {/if}
        <AlertTitle>
          {problemTitle}
        </AlertTitle>
        <AlertDescription>
          <p>
            {problemNote ??
              device.issues?.[0]?.detail ??
              t('printerDetailStatus.problem.unknown')}
          </p>
          {#if problemAction !== undefined || needsAttention}
            <p>
              {problemAction ?? t('printerDetailStatus.problem.checkPrinter')}
            </p>
          {/if}
          {#if needsAttention && problemHref}
            <a class="underline" href={problemHref}
              >{t('printerDetailStatus.problem.history')}</a
            >
          {/if}
        </AlertDescription>
      </Alert>
    {/if}
    <PrinterDialog {device} {telemetry} {commands} />

    <Card density="panel">
      <CardContent
        class="flex flex-col gap-6 pt-section-block pb-panel-padding-mobile @lg/card:pb-panel-padding"
      >
        {#if variant === 'notReporting'}
          <EmptyState
            icon={WifiOffIcon}
            heading={t('printerDetailStatus.notReporting.heading')}
            headingLevel="h2"
            description={notReportingNote}
          />
          <p class="text-sm text-muted-foreground">{t('printers.noData')}</p>
          {#if job.name !== undefined}
            <div class="flex min-w-0 flex-col gap-3">
              <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
                {@render plate(job.name)}
                <ClippedName
                  name={job.name}
                  class="min-w-0 font-mono text-sm"
                />
                <Badge variant="outline">
                  {t('printerDetailStatus.lastJob.badge')}
                </Badge>
              </div>
              {@render progressBar()}
            </div>
          {/if}
        {:else if split || showsCamera}
          <div
            class={['min-w-0', cameraAlone ? 'flex flex-col gap-6' : SPLIT_ROW]}
          >
            {#if showsCamera && camera !== undefined}
              <div
                class={[
                  'flex flex-col gap-2',
                  cameraAlone ? 'mx-auto w-full max-w-4xl' : SPLIT_LEADING,
                ]}
              >
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <span class={BLOCK_LABEL}>
                    {t('printerDetailStatus.camera.label')}
                  </span>
                  {#if openable}
                    <Button
                      variant="secondary"
                      size="sm"
                      aria-label={t(
                        streams
                          ? 'printerDetailStatus.camera.expandStream'
                          : 'printerDetailStatus.camera.expandSnapshot',
                        { printer: device.name },
                      )}
                      onclick={expand}
                    >
                      <Maximize2Icon aria-hidden="true" />
                      {t(
                        streams
                          ? 'printerDetailStatus.camera.openStream'
                          : 'printerDetailStatus.camera.openSnapshot',
                      )}
                    </Button>
                  {/if}
                </div>
                <CameraPicture
                  {...camera}
                  alt={t('printerDetailStatus.camera.alt')}
                  bind:playing={
                    () => playing,
                    (on: boolean) => {
                      playingName = on ? cameraName() : undefined;
                    }
                  }
                  class="rounded-sm"
                >
                  {#if playing}
                    <Badge
                      variant="secondary"
                      class="absolute start-2 top-2 gap-1.5 uppercase"
                    >
                      <span
                        class="size-1.5 rounded-full bg-destructive"
                        aria-hidden="true"
                      ></span>
                      {t('printerDetailStatus.camera.live')}
                    </Badge>
                  {/if}
                </CameraPicture>
                <Dialog
                  bind:open={
                    () =>
                      expandedName !== undefined &&
                      expandedName === cameraName(),
                    (on: boolean) => {
                      expandedName = on ? cameraName() : undefined;
                    }
                  }
                >
                  <DialogContent
                    size="xl"
                    bind:ref={expandedPanel}
                    onOpenAutoFocus={(event: Event) => {
                      // Default focus lands on Stop, whose tooltip would take
                      // the first Escape.
                      event.preventDefault();
                      expandedPanel
                        ?.querySelector<HTMLElement>(
                          '[data-slot="dialog-close"]',
                        )
                        ?.focus();
                    }}
                  >
                    <DialogTitle>
                      {t('printerDetailStatus.camera.title', {
                        printer: device.name,
                      })}
                    </DialogTitle>
                    <CameraPicture
                      {...camera}
                      alt={t('printerDetailStatus.camera.alt')}
                      bind:playing={
                        () =>
                          expandedPlayingName !== undefined &&
                          expandedPlayingName === cameraName(),
                        (on: boolean) => {
                          expandedPlayingName = on ? cameraName() : undefined;
                        }
                      }
                      class="rounded-sm"
                    />
                  </DialogContent>
                </Dialog>
              </div>
            {/if}
            {#if cameraAlone}
              {@render controls()}
            {:else if split}
              {@render currentPrint()}
            {:else}
              <div
                class={[
                  'flex min-w-0 flex-1 flex-col justify-center',
                  SPLIT_TRAILING,
                ]}
              >
                {@render compactPrint()}
              </div>
            {/if}
          </div>
        {:else}
          {@render compactPrint()}
        {/if}
      </CardContent>
    </Card>

    <FileAccess
      {device}
      {commands}
      state={wireStateOf(telemetry, device)}
      stale={variant === 'notReporting'}
      {transfers}
      {uploads}
      {onCancelUpload}
      {history}
      {recent}
      {files}
      {historyHref}
      {thumbnailFor}
      {onUpload}
      {uploadBlock}
      {keep}
      {checkingStorages}
      {onRecheck}
    />
  {/if}
</div>
