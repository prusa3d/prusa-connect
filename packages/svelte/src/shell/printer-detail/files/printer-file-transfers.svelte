<!-- What is on its way to the printer, and what is still going up to this server. -->
<script lang="ts">
  import { TRANSFER_RUNNING } from '../../../data';
  import { formatUnit, t } from '../../../i18n';
  import { Button, ClippedName, Progress } from '../../../primitives';
  import type { SendsIndicatorRow } from '../../sends-indicator';
  import type { PrinterFileTransfer, PrinterFileTransfers } from './types';

  const PERCENT = 100;

  const STATE: Record<string, string> = {
    cancelled: 'printerDetailFiles.transfers.cancelled',
    done: 'printerDetailFiles.transfers.done',
    failed: 'printerDetailFiles.transfers.failed',
    interrupted: 'printerDetailFiles.transfers.interrupted',
    queued: 'printerDetailFiles.transfers.queued',
    sending: 'printerDetailFiles.transfers.sending',
  };

  let {
    queue,
    transfers,
    uploads,
    onCancelUpload,
  }: {
    queue: readonly PrinterFileTransfer[];
    transfers?: PrinterFileTransfers | undefined;
    /** The files this tab is still sending from the browser to this server. */
    uploads?: readonly SendsIndicatorRow[] | undefined;
    onCancelUpload?: ((id: string) => void) | undefined;
  } = $props();

  const nameOf = (transfer: PrinterFileTransfer): string =>
    transfer.path.split('/').pop() ?? transfer.path;

  /** Null while the length is unknown, which draws a bar with no fill. */
  const percentOf = (transfer: PrinterFileTransfer): number | null =>
    transfer.bytesTotal > 0
      ? (transfer.bytesSent / transfer.bytesTotal) * PERCENT
      : null;

  const running = (transfer: PrinterFileTransfer): boolean =>
    TRANSFER_RUNNING.includes(transfer.state);
</script>

{#if queue.length > 0 || (uploads?.length ?? 0) > 0}
  <!-- Capped, so ten files scroll inside it rather than stretching the column. -->
  <section
    aria-label={t('printerDetailFiles.transfers.heading')}
    class="flex max-h-64 flex-col gap-3 overflow-y-auto rounded-md bg-surface p-3"
  >
    <h2 class="text-xs font-medium text-muted-foreground">
      {t('printerDetailFiles.transfers.heading')}
    </h2>
    <ul class="flex flex-col gap-3">
      {#each uploads ?? [] as upload (upload.id)}
        {@const percent =
          upload.progress === null ? null : upload.progress * PERCENT}
        <li class="flex flex-col gap-1">
          <div class="flex items-baseline justify-between gap-3">
            <ClippedName name={upload.name} class="min-w-0 font-mono text-sm" />
            <span class="shrink-0 text-xs text-muted-foreground">
              {t('printerDetailFiles.transfers.uploading')}
            </span>
          </div>
          <div class="flex items-center gap-3">
            <Progress
              value={percent}
              aria-label={t('printerDetailFiles.transfers.uploadLabel', {
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
              aria-label={t('printerDetailFiles.transfers.cancelUploadFor', {
                name: upload.name,
              })}
              onclick={() => onCancelUpload?.(upload.id)}
            >
              {t('printerDetailFiles.transfers.cancelUpload')}
            </Button>
          </div>
        </li>
      {/each}
      {#each queue as transfer (transfer.transferId)}
        {@const name = nameOf(transfer)}
        {@const percent = percentOf(transfer)}
        <li class="flex flex-col gap-1">
          <div class="flex items-baseline justify-between gap-3">
            <ClippedName {name} class="min-w-0 font-mono text-sm" />
            <span class="shrink-0 text-xs text-muted-foreground">
              {t(
                STATE[transfer.state] ?? 'printerDetailFiles.transfers.sending',
              )}
            </span>
          </div>
          {#if running(transfer)}
            <div class="flex items-center gap-3">
              <Progress
                value={percent}
                aria-label={t('printerDetailFiles.transfers.label', { name })}
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
                aria-label={t('printerDetailFiles.transfers.stopFor', { name })}
                onclick={() => void transfers?.stop(transfer.transferId)}
              >
                {t('printerDetailFiles.transfers.stop')}
              </Button>
            </div>
          {:else if transfer.reason !== undefined && transfer.reason !== ''}
            <p class="text-xs text-muted-foreground">{transfer.reason}</p>
          {/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}
