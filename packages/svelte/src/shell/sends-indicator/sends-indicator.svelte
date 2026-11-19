<!--
  The files on their way to printers, drawn on every page: this tab's own,
  and every other one the server is sending to a printer the person may
  read, whoever started it. A file somebody else started says so.

  It shows while one of them is on its way, and a row leaves it the moment
  its file ends, whether it landed, failed or was cancelled. A row offers to
  stop the file only to a person who may stop it.

  While the browser still holds a file's bytes, closing or reloading the
  tab loses the file, so the tab asks first. The listener is added only
  then: a page with a beforeunload listener is not kept in the browser's
  back-forward cache.
-->
<script lang="ts">
  import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
  import UploadIcon from '@lucide/svelte/icons/upload';
  import UserIcon from '@lucide/svelte/icons/user';

  import { formatUnit, t } from '../../i18n';
  import {
    Button,
    ClippedName,
    Popover,
    PopoverContent,
    PopoverTrigger,
    Progress,
  } from '../../primitives';
  import { printerHref } from '../printers/presentation';
  import type { SendsIndicatorRow, SendsSource } from './types';

  const PERCENT = 100;

  const STEP: Record<SendsIndicatorRow['step'], string> = {
    sending: 'sendsIndicator.sending',
    uploading: 'sendsIndicator.uploading',
  };

  let { sends }: { sends: SendsSource } = $props();

  // The server names a person by their principal, such as `user:ada`. The
  // part after the kind is the name a person reads.
  const personOf = (principal: string): string => {
    const at = principal.indexOf(':');

    return at < 0 ? principal : principal.slice(at + 1);
  };

  const startedByOf = (row: SendsIndicatorRow): string =>
    row.startedBy === undefined
      ? t('sendsIndicator.startedBySomeone')
      : t('sendsIndicator.startedBy', { person: personOf(row.startedBy) });

  $effect(() => {
    if (!sends.uploading) return;
    const ask = (event: BeforeUnloadEvent): void => {
      event.preventDefault();
    };
    window.addEventListener('beforeunload', ask);

    return () => {
      window.removeEventListener('beforeunload', ask);
    };
  });
</script>

{#snippet trigger({ props }: { props: Record<string, unknown> })}
  <Button
    {...props}
    variant="ghost"
    size="icon-md"
    label={t('sendsIndicator.trigger', { count: sends.rows.length })}
  >
    <UploadIcon
      aria-hidden="true"
      class={sends.active ? 'animate-pulse' : undefined}
    />
  </Button>
{/snippet}

{#if sends.rows.length > 0}
  <Popover>
    <PopoverTrigger child={trigger} />
    <PopoverContent class="flex w-80 flex-col gap-3">
      <h2 class="text-sm leading-none font-semibold">
        {t('sendsIndicator.heading')}
      </h2>
      <ul class="flex max-h-64 flex-col gap-3 overflow-y-auto">
        {#each sends.rows as row (row.id)}
          {@const percent =
            row.progress === null ? null : row.progress * PERCENT}
          <li class="flex flex-col gap-1 text-xs">
            <div class="flex items-baseline justify-between gap-3">
              <ClippedName name={row.name} class="min-w-0 font-mono" />
              <span class="shrink-0 text-muted-foreground">
                {t(STEP[row.step])}
              </span>
            </div>
            <a
              href={printerHref(row.deviceId, 'files')}
              class="flex min-w-0 items-center gap-1 rounded-sm text-muted-foreground outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
            >
              <ArrowRightIcon aria-hidden="true" class="size-3 shrink-0" />
              <span class="min-w-0 truncate">{row.printerName}</span>
            </a>
            {#if !row.mine}
              <p class="flex min-w-0 items-center gap-1 text-muted-foreground">
                <UserIcon aria-hidden="true" class="size-3 shrink-0" />
                <span class="min-w-0 truncate">{startedByOf(row)}</span>
              </p>
            {/if}
            <div class="flex items-center gap-3">
              <Progress
                value={percent}
                aria-label={t('sendsIndicator.label', {
                  name: row.name,
                  printer: row.printerName,
                })}
                size="sm"
                class="flex-1"
              />
              {#if percent !== null}
                <span
                  class="shrink-0 font-mono text-muted-foreground tabular-nums"
                >
                  {formatUnit(Math.round(percent), '%')}
                </span>
              {/if}
              {#if row.canCancel && row.step === 'uploading'}
                <Button
                  variant="ghost"
                  size="sm"
                  class="shrink-0"
                  aria-label={t('sendsIndicator.cancelFor', {
                    name: row.name,
                  })}
                  onclick={() => {
                    sends.cancel(row.id);
                  }}
                >
                  {t('sendsIndicator.cancel')}
                </Button>
              {:else if row.canCancel}
                <Button
                  variant="ghost"
                  size="sm"
                  class="shrink-0"
                  disabled={row.stopping}
                  aria-label={t('sendsIndicator.stopFor', {
                    name: row.name,
                    printer: row.printerName,
                  })}
                  onclick={() => {
                    sends.cancel(row.id);
                  }}
                >
                  {t('sendsIndicator.stop')}
                </Button>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    </PopoverContent>
  </Popover>
{/if}
