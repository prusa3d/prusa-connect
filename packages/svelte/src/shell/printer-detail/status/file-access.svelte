<!-- Every file question on this tab in one card. -->
<script lang="ts">
  import FileCode2Icon from '@lucide/svelte/icons/file-code-2';
  import InboxIcon from '@lucide/svelte/icons/inbox';
  import PlayIcon from '@lucide/svelte/icons/play';

  import {
    capabilityFor,
    PRINTER_STATE_LABEL_KEY,
    printStateOf,
  } from '../../../domain';
  import {
    describeFault,
    formatBytes,
    formatDateTime,
    formatDuration,
    formatUnit,
    t,
  } from '../../../i18n';
  import {
    ABOVE_ROW_LINK,
    Badge,
    Button,
    Card,
    CardContent,
    ClippedName,
    DataRow,
    DropdownMenuItem,
    EmptyState,
    FaultAlert,
    FileDropzone,
    FileThumbnail,
    Progress,
    RowActions,
    RowList,
    THUMBNAIL_ROW_MIN_HEIGHT,
  } from '../../../primitives';
  import { jobHref } from '../../jobs/presentation';
  import { stateBadgeVariant } from '../../printers/presentation';
  import type { SendsIndicatorRow } from '../../sends-indicator';
  import type { PrintRow } from '../info/types';
  import { SaveToStorage } from '../save-to-storage';
  import UploadBlocked from '../upload-blocked.svelte';
  import {
    commandControl,
    commandProjection,
    SPLIT_LEADING,
    SPLIT_ROW,
    SPLIT_TRAILING,
  } from './presentation';
  import type { PrinterStatusPanelProps, PrinterTransferRow } from './types';

  let {
    device,
    commands,
    state,
    stale,
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
  }: Pick<
    PrinterStatusPanelProps,
    | 'checkingStorages'
    | 'commands'
    | 'device'
    | 'files'
    | 'history'
    | 'historyHref'
    | 'onRecheck'
    | 'onCancelUpload'
    | 'onUpload'
    | 'recent'
    | 'thumbnailFor'
    | 'transfers'
    | 'uploadBlock'
    | 'keep'
    | 'uploads'
  > & {
    /** The machine's own word for what it is doing, as the wire spells it. */
    state: string;
    /** Whether this server can hear from the printer at all. */
    stale: boolean;
  } = $props();

  /**
   * What a printer takes on its own storage: a print, or a firmware image,
   * which lands at the top of the drive where M997 reads it. The firmware
   * refuses a .3mf.
   */
  const UPLOAD_ACCEPT = '.gcode,.bgcode,.bbf';

  const PERCENT = 100;
  const MS_PER_SECOND = 1000;

  /** How many prints the block shows before deferring to the tab that holds them all. */
  const LAST_PRINT_LIMIT = 3;

  /** The one label treatment every block on this panel names its parts with. */
  const BLOCK_LABEL = 'text-xs font-medium text-muted-foreground';

  const moving = $derived(transfers?.inFlight ?? []);

  const lastPrint = $derived(
    history === undefined ||
      history.jobsError !== undefined ||
      !history.jobsLoaded ||
      history.jobs?.last === undefined
      ? []
      : [history.jobs.last],
  );
  const prints = $derived((recent ?? lastPrint).slice(0, LAST_PRINT_LIMIT));

  // The taker itself rather than a flag.
  const dropzone = $derived(stale ? undefined : onUpload);
  // A printer this server cannot hear has its own story, told above.
  const blocked = $derived(
    stale || dropzone !== undefined ? undefined : uploadBlock,
  );
  const ingest = $derived(dropzone !== undefined || blocked !== undefined);
  const lists = $derived(prints.length > 0);

  // The read answered and no print has ended here. It is a different answer
  // from a read still on its way or one the server refused, and the block
  // says so in place of the rows rather than leaving the column blank.
  const nothingPrinted = $derived(
    !lists &&
      history !== undefined &&
      history.jobsError === undefined &&
      history.jobsLoaded,
  );
  // A printer on its first print has printed something, and the card above
  // shows it, so the block says that print lands here once it ends.
  const firstOnBed = $derived(device.job?.running === true);
  // A refused read is said where the rows would be, so the block keeps its
  // place beside the upload area instead of leaving the card half empty.
  const refusal = $derived(
    !lists && history?.jobsError !== undefined
      ? describeFault(history.jobsError)
      : undefined,
  );
  const block = $derived(lists || nothingPrinted || refusal !== undefined);
  const anything = $derived(
    moving.length > 0 || (uploads?.length ?? 0) > 0 || ingest || block,
  );

  const nameOf = (transfer: PrinterTransferRow): string =>
    transfer.path.split('/').pop() ?? transfer.path;

  /** Null while the length is unknown, which draws a bar with no fill. */
  const percentOf = (transfer: PrinterTransferRow): number | null =>
    transfer.bytesTotal > 0
      ? (transfer.bytesSent / transfer.bytesTotal) * PERCENT
      : null;

  const stateOf = (job: PrintRow) =>
    printStateOf({
      state: job.state,
      ended: job.ended !== undefined,
      outcome: job.outcome,
    });

  // `ended` is what tells a finished print from the running one.
  const spanOf = (job: PrintRow): string | undefined => {
    if (job.ended === undefined) return undefined;
    const seconds =
      (new Date(job.ended).getTime() - new Date(job.started).getTime()) /
      MS_PER_SECOND;

    return Number.isFinite(seconds) ? formatDuration(seconds) : undefined;
  };

  // A printer that cannot start a print now still has a row that says so.
  const printControl = $derived(
    commandControl(
      commandProjection(device, commands, state, 'start_print'),
      commands.sending === true,
    ),
  );

  const printAgain = (job: PrintRow): void => {
    void commands.onCommand(capabilityFor('start_print'), { path: job.path });
  };
</script>

{#snippet transferRow(transfer: PrinterTransferRow)}
  {@const name = nameOf(transfer)}
  {@const percent = percentOf(transfer)}
  <div class="flex flex-col gap-2 rounded-sm bg-surface px-3 py-2">
    <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
      <ClippedName {name} class="min-w-0 font-mono text-sm" />
      {#if transfer.bytesTotal > 0}
        <span
          class="shrink-0 font-mono text-xs text-muted-foreground tabular-nums"
        >
          {formatBytes(transfer.bytesSent)} / {formatBytes(transfer.bytesTotal)}
        </span>
      {/if}
    </div>
    <div class="flex items-center gap-3">
      <Progress
        value={percent}
        aria-label={t('printerDetailStatus.transfer.label', { name })}
        size="sm"
        class="flex-1"
      />
      {#if percent !== null}
        <span
          class="shrink-0 font-mono text-xs text-muted-foreground tabular-nums"
        >
          {formatUnit(Math.round(percent), '%')}
        </span>
      {/if}
      <Button
        variant="ghost"
        size="sm"
        class="shrink-0"
        aria-label={t('printerDetailStatus.transfer.stopFor', { name })}
        onclick={() => void transfers?.stop(transfer.transferId)}
      >
        {t('printerDetailStatus.transfer.stop')}
      </Button>
    </div>
  </div>
{/snippet}

{#snippet uploadRow(upload: SendsIndicatorRow)}
  {@const percent = upload.progress === null ? null : upload.progress * PERCENT}
  <div class="flex flex-col gap-2 rounded-sm bg-surface px-3 py-2">
    <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
      <ClippedName name={upload.name} class="min-w-0 font-mono text-sm" />
      <span class="shrink-0 text-xs text-muted-foreground">
        {t('printerDetailStatus.transfer.uploading')}
      </span>
    </div>
    <div class="flex items-center gap-3">
      <Progress
        value={percent}
        aria-label={t('printerDetailStatus.transfer.label', {
          name: upload.name,
        })}
        size="sm"
        class="flex-1"
      />
      {#if percent !== null}
        <span
          class="shrink-0 font-mono text-xs text-muted-foreground tabular-nums"
        >
          {formatUnit(Math.round(percent), '%')}
        </span>
      {/if}
      <Button
        variant="ghost"
        size="sm"
        class="shrink-0"
        aria-label={t('printerDetailStatus.transfer.cancelUploadFor', {
          name: upload.name,
        })}
        onclick={() => onCancelUpload?.(upload.id)}
      >
        {t('printerDetailStatus.transfer.cancelUpload')}
      </Button>
    </div>
  </div>
{/snippet}

{#snippet printRow(job: PrintRow)}
  {#snippet outcome()}
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
  {#snippet again()}
    <!-- A row's acts sit behind the one menu every listing uses, as on the
      Files tab, so the card above keeps the one visible Print again. -->
    {#if printControl}
      {@const gone = files?.holds(job.path) === false}
      {@const reason = gone
        ? t('printerDetailStatus.command.fileGone')
        : printControl.reason}
      <RowActions name={job.display_name} class={ABOVE_ROW_LINK}>
        <DropdownMenuItem
          disabled={printControl.disabled || gone}
          onSelect={() => {
            printAgain(job);
          }}
        >
          <PlayIcon aria-hidden="true" />
          <span class="flex max-w-64 flex-col">
            <span>{t('printerDetailStatus.command.printAgain')}</span>
            {#if reason}
              <span class="text-xs text-muted-foreground">
                {reason}
              </span>
            {/if}
          </span>
        </DropdownMenuItem>
      </RowActions>
    {/if}
  {/snippet}
  <DataRow
    class={THUMBNAIL_ROW_MIN_HEIGHT}
    href={jobHref(job.id)}
    leading={plate}
    primary={job.display_name}
    mono
    wrapValues
    secondaryValues={[
      outcome,
      formatUnit(job.progress, '%'),
      t('printerDetailStatus.lastPrint.started', {
        when: formatDateTime(job.started),
      }),
      job.ended !== undefined &&
        t('printerDetailStatus.lastPrint.ended', {
          when: formatDateTime(job.ended),
        }),
      spanOf(job),
    ]}
    {...printControl === undefined ? {} : { actions: again }}
  />
{/snippet}

{#if anything}
  <Card radius="md">
    <CardContent class="flex flex-col gap-6">
      {#if moving.length > 0 || (uploads?.length ?? 0) > 0}
        <div class="flex flex-col gap-2">
          <span class={BLOCK_LABEL}>
            {t('printerDetailStatus.transfer.heading')}
          </span>
          {#each uploads ?? [] as upload (upload.id)}
            {@render uploadRow(upload)}
          {/each}
          {#each moving as transfer (transfer.transferId)}
            {@render transferRow(transfer)}
          {/each}
        </div>
      {/if}

      {#if ingest || block}
        <div class={SPLIT_ROW}>
          {#if ingest}
            <div class={['flex flex-col gap-2', SPLIT_LEADING]}>
              <span class={BLOCK_LABEL}>
                {t('printerDetailStatus.upload.heading')}
              </span>
              {#if dropzone}
                <FileDropzone
                  accept={UPLOAD_ACCEPT}
                  onFiles={dropzone}
                  class="flex-1"
                />
                {#if keep}
                  <SaveToStorage {...keep} />
                {/if}
              {:else if blocked}
                <UploadBlocked
                  block={blocked}
                  checking={checkingStorages}
                  {onRecheck}
                />
              {/if}
            </div>
          {/if}

          {#if block}
            <div
              class={[
                'flex min-w-0 flex-1 flex-col gap-2',
                ingest && SPLIT_TRAILING,
              ]}
            >
              <div class="flex flex-wrap items-baseline justify-between gap-2">
                <span class={BLOCK_LABEL}>
                  {t('printerDetailStatus.prints.heading')}
                </span>
                {#if historyHref !== undefined}
                  <Button variant="link">
                    {#snippet child({ props })}
                      <a {...props} href={historyHref}>
                        {t('printerDetailStatus.prints.all')}
                      </a>
                    {/snippet}
                  </Button>
                {/if}
              </div>
              {#if refusal}
                <FaultAlert fault={refusal} />
              {:else}
                <RowList>
                  {#if nothingPrinted}
                    <EmptyState
                      icon={InboxIcon}
                      heading={firstOnBed
                        ? t('printerDetailStatus.prints.firstOnBed.heading')
                        : t('printerDetailStatus.prints.empty.heading')}
                      headingLevel="h3"
                      description={firstOnBed
                        ? t('printerDetailStatus.prints.firstOnBed.note')
                        : t('printerDetailStatus.prints.empty.note')}
                    />
                  {:else}
                    {#each prints as job (job.id)}
                      {@render printRow(job)}
                    {/each}
                  {/if}
                </RowList>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    </CardContent>
  </Card>
{/if}
