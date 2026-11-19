<!--
  Where one version stands on this server, as the listing and the detail draw
  it: a badge, and while an install runs, how far it is.
-->
<script lang="ts">
  import { formatBytes } from '@prusa/connect-ui/i18n';
  import {
    Badge,
    type BadgeVariant,
    Progress,
  } from '@prusa/connect-ui/primitives';

  import type { Translate } from '../../api';
  import type { ReleaseRow, ReleaseState } from '../../releases';

  let {
    row,
    t,
    detailed = false,
  }: {
    row: ReleaseRow;
    t: Translate;
    /** Also draws the bytes downloaded, for the detail. */
    detailed?: boolean;
  } = $props();

  const variants: Record<ReleaseState, BadgeVariant> = {
    installed: 'success',
    installing: 'info',
    failed: 'destructive',
    available: 'outline',
    unavailable: 'secondary',
  };

  const progress = $derived(row.progress);
  const percent = $derived(
    progress?.phase === 'downloading' && progress.total_bytes
      ? Math.round((progress.done_bytes / progress.total_bytes) * 100)
      : null,
  );
</script>

<div
  class="grid min-w-0 grid-cols-1 content-start gap-1"
  data-part="release-state"
>
  {#if row.state === 'available' || row.state === 'unavailable'}
    <span class="text-sm text-muted-foreground"
      >{t(`slicer.releaseState.${row.state}`)}</span
    >
  {:else}
    <Badge variant={variants[row.state]} class="w-fit">
      {row.state === 'installing' && progress
        ? t(`slicer.phase.${progress.phase}`)
        : t(`slicer.releaseState.${row.state}`)}
    </Badge>
  {/if}
  {#if row.state === 'installing' && progress}
    <div class="flex items-center gap-2">
      <Progress
        value={percent}
        aria-label={t('slicer.release.progress', { version: row.version })}
        class="w-24 flex-1"
      />
      {#if percent !== null}
        <span class="shrink-0 text-xs tabular-nums">{percent} %</span>
      {/if}
    </div>
    {#if detailed && progress.total_bytes}
      <span class="text-xs text-muted-foreground tabular-nums">
        {formatBytes(progress.done_bytes)} / {formatBytes(progress.total_bytes)}
      </span>
    {/if}
  {/if}
</div>
