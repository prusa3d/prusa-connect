<!--
  One tool, or one MMU slot, as a tile of the Tools tab: the label with where
  its spool came from, the nozzle and the mark that says why a button is
  refused on one line, the material's circle centred in a block of fixed
  height, its name and brand, then the grams with a bar of what is left, and
  the buttons at the foot. Every row keeps its height when empty, and the
  buttons keep room for two, so tiles side by side line up whatever they hold.
  The tile whose picker is open is outlined.
-->
<script lang="ts">
  import { formatNumber } from '@prusa/connect-ui/i18n';
  import type { Snippet } from 'svelte';

  import { heldHost } from '../host';
  import TapTooltip from '../tap-tooltip.svelte';
  import MaterialCircle from './material-circle.svelte';
  import SpoolBar from './spool-bar.svelte';
  import ToolIcon from './tool-icon.svelte';
  import { shareLeft, type ToolView } from './tool-view';

  let {
    view,
    open,
    reasons,
    actions,
  }: {
    view: ToolView;
    /** Why a button is refused, each reason once; none draws no mark. */
    reasons: readonly string[];
    /** The tile's picker is open. */
    open: boolean;
    actions: Snippet;
  } = $props();

  const t = heldHost().t;

  const grams = (g: number): string =>
    formatNumber(g, { maximumFractionDigits: 0 });
</script>

<li
  data-part="tool"
  data-tool={view.tool.id}
  class={[
    'flex flex-col items-center gap-1.5 rounded-md border p-3 text-center',
    open ? 'border-primary' : 'border-border',
  ]}
>
  <div
    class="flex w-full flex-wrap items-center justify-between gap-x-2 gap-y-0.5 text-xs text-muted-foreground"
  >
    <span class="flex min-w-0 items-center gap-1.5">
      <span class="truncate">{view.label}</span>
      {#if reasons.length > 0}
        <TapTooltip text={reasons}>
          <span data-part="reasons" class="inline-flex text-primary">
            <ToolIcon name="info" class="size-3.5" />
          </span>
        </TapTooltip>
      {/if}
      {#if view.fromMaterials}
        <ToolIcon
          name="spool"
          class="size-3.5 shrink-0"
          title={t('materials.printer.fromMaterials')}
        />
      {/if}
      {#if view.tool.from_nfc}
        <ToolIcon
          name="nfc"
          class="size-3.5 shrink-0"
          title={t('materials.printer.nfcTitle')}
        />
      {/if}
    </span>
    {#if view.tool.nozzle.diameter_mm !== undefined}
      <span class="flex items-center gap-1">
        {formatNumber(view.tool.nozzle.diameter_mm)}
        <ToolIcon
          name="nozzle"
          class="size-3.5 text-primary"
          title={t('materials.tools.nozzle')}
        />
      </span>
    {/if}
  </div>
  <div class="flex h-36 items-center justify-center">
    <MaterialCircle
      text={view.circle}
      colorRgba={view.colorRgba}
      size="small"
      ring={view.ring}
      title={view.tool.active
        ? t('materials.tools.inHead')
        : t('materials.tools.loaded')}
    />
  </div>
  <span
    class={[
      'line-clamp-2 min-h-8 text-xs font-medium',
      view.name === undefined && 'text-muted-foreground',
    ]}
  >
    {view.name ?? t('materials.printer.none')}
  </span>
  <span class="min-h-5 w-full truncate text-xs leading-5 text-muted-foreground">
    {view.brand}
  </span>
  <div class="flex w-full flex-col gap-1.5">
    <span class="min-h-5 text-xs leading-5">
      {#if view.leftG !== undefined}
        {view.fullG === undefined
          ? t('materials.printer.grams', { g: grams(view.leftG) })
          : t('materials.printer.weight', {
              left: grams(view.leftG),
              full: grams(view.fullG),
            })}
      {/if}
    </span>
    <div class={view.leftG === undefined ? 'invisible' : undefined}>
      <SpoolBar left={shareLeft(view)} colorRgba={view.colorRgba} thick />
    </div>
  </div>
  <div class="mt-auto flex min-h-25 w-full flex-col justify-end pt-2">
    {@render actions()}
  </div>
</li>
