<!-- What is on its way up, under the drop zone that sent it. -->
<script lang="ts">
  import { formatBytes, t } from '../../i18n';
  import { Button, ClippedName, Progress } from '../../primitives';
  import type { StorageUpload } from './types';

  const UPLOAD_STATE: Record<string, string> = {
    cancelled: 'storage.uploads.cancelled',
    done: 'storage.uploads.done',
    failed: 'storage.uploads.failed',
    sending: 'storage.uploads.sending',
  };

  let {
    uploads,
    onClear,
    onCancel,
  }: {
    uploads: readonly StorageUpload[];
    onClear: () => void;
    onCancel: (upload: StorageUpload) => void;
  } = $props();
</script>

{#if uploads.length > 0}
  <section
    aria-label={t('storage.uploads.heading')}
    class="flex flex-col gap-3 rounded-md bg-surface p-3"
  >
    <div class="flex items-center justify-between gap-2">
      <h2 class="text-sm font-medium">
        {t('storage.uploads.heading')}
      </h2>
      <Button
        variant="ghost"
        size="sm"
        onclick={() => {
          onClear();
        }}
      >
        {t('storage.uploads.clear')}
      </Button>
    </div>
    <ul class="flex max-h-64 flex-col gap-2 overflow-y-auto">
      {#each uploads as sending (`${sending.storage}/${sending.path}`)}
        <li class="flex flex-col gap-1 text-xs">
          <div class="flex items-center justify-between gap-3">
            <ClippedName name={sending.path} class="min-w-0" />
            <span class="whitespace-nowrap">
              {t(UPLOAD_STATE[sending.state] ?? 'storage.uploads.sending')}
              · {formatBytes(sending.total)}
            </span>
          </div>
          {#if sending.state === 'sending'}
            <div class="flex items-center gap-3">
              <Progress
                value={sending.total > 0
                  ? (sending.sent / sending.total) * 100
                  : null}
                aria-label={t('storage.uploads.label', { name: sending.path })}
                size="sm"
                class="flex-1"
              />
              <Button
                variant="ghost"
                size="sm"
                class="shrink-0"
                aria-label={t('storage.uploads.cancelFor', {
                  name: sending.path,
                })}
                onclick={() => {
                  onCancel(sending);
                }}
              >
                {t('storage.uploads.cancel')}
              </Button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}
