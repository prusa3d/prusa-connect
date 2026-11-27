<!--
  One printer as a picture, for a wall a reader watches rather than reads.

  A printer with a camera draws it as the printer's Cameras tab does: its
  latest still frame, whole and never cropped, with the live view a reader
  can play over it.

  A printer with no camera draws its own artwork at the tile's ratio. That is
  the design's answer and not a placeholder: an empty grey box on a wall of
  them says a camera is loading, which is a different and wrong thing.
-->
<script lang="ts">
  import {
    PRINTER_STATE_LABEL_KEY,
    PRINTER_STATE_PROGRESS_TONE,
  } from '../../domain';
  import { t } from '../../i18n';
  import {
    ABOVE_ROW_LINK,
    AspectRatio,
    Badge,
    Card,
    CardTitle,
    ClippedName,
    PrinterIcon,
    Progress,
    Separator,
  } from '../../primitives';
  import CameraPicture from '../printer-cameras/camera-picture.svelte';
  import { cardToneOf, stateBadgeVariant } from './presentation';
  import type { PrinterFrameProps } from './types';

  let { row, camera, rowEnd }: PrinterFrameProps = $props();

  const titleId = $props.id();

  const progress = $derived(row.progress);

  const loud = $derived(row.tier === 'critical' || row.tier === 'attention');
</script>

<Card
  data-slot="printer-frame"
  role="region"
  aria-labelledby={titleId}
  tone={cardToneOf(row.tier)}
  density="flush"
  class="relative overflow-hidden"
>
  <div class="relative">
    {#if camera === undefined}
      <AspectRatio ratio={16 / 9} class="bg-muted">
        <div class="flex size-full items-center justify-center">
          <PrinterIcon
            model={row.artwork}
            reported={row.model}
            class="size-24"
          />
        </div>
      </AspectRatio>
    {:else}
      <!-- Keyed, so another camera starts as a new picture, playing while
           it is seen (ADR-20261119-pxn). -->
      {#key camera.camera.name}
        <CameraPicture
          {...camera}
          alt={t('printers.camera.alt', { name: row.name })}
          autoplay
        />
      {/key}
    {/if}

    {#if progress !== null}
      <div
        data-slot="printer-frame-overlay"
        class="absolute inset-x-0 bottom-0 bg-black/70 px-3 py-1.5 text-end font-mono text-xs text-white tabular-nums"
      >
        {progress}%
      </div>
    {/if}
  </div>

  <div class="flex h-1 items-center">
    {#if progress === null}
      <Separator />
    {:else}
      <!-- The percentage is already written over the picture, so the bar is a
           second telling of it and is hidden from a screen reader. -->
      <Progress
        aria-hidden="true"
        value={progress}
        tone={PRINTER_STATE_PROGRESS_TONE[row.state]}
        size="sm"
        class="rounded-none"
      />
    {/if}
  </div>

  <div class="flex flex-col gap-1 p-3">
    <div class="flex items-baseline justify-between gap-2">
      <CardTitle id={titleId} class="min-w-0 text-sm font-bold">
        <h2 class="contents">
          <a
            href={row.detailHref}
            title={row.name}
            class="block truncate after:absolute after:inset-0 focus-visible:outline-none focus-visible:after:ring-2 focus-visible:after:ring-ring focus-visible:after:ring-inset"
          >
            {row.name}
          </a>
        </h2>
      </CardTitle>
      <span class="flex shrink-0 items-center gap-1">
        <!-- A module's control for this printer (ADR-20261112-62y), lifted
             above the frame's link. Empty, it takes no room. -->
        {#if rowEnd}
          <span class={[ABOVE_ROW_LINK, 'flex items-center empty:hidden']}
            >{@render rowEnd(row.id)}</span
          >
        {/if}
        <Badge variant={stateBadgeVariant(row.state)} class="shrink-0">
          {t(PRINTER_STATE_LABEL_KEY[row.state])}
        </Badge>
      </span>
    </div>

    {#if row.jobName === undefined}
      <p class="text-xs text-muted-foreground">{t('printers.noJob')}</p>
    {:else}
      <ClippedName name={row.jobName} class="text-xs" />
    {/if}

    {#if loud && row.statusNote !== undefined}
      <p
        class={[
          'text-xs',
          row.tier === 'critical'
            ? 'text-destructive-text'
            : 'text-warning-text',
        ]}
      >
        {row.statusNote}
      </p>
    {/if}
  </div>
</Card>
