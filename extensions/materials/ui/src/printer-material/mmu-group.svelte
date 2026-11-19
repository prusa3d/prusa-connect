<!--
  An MMU on the Tools tab: a header with the feeder's model, the nozzle its
  slots print through and one Unload filament for the head, then a row of slot
  tiles across the whole width.
-->
<script lang="ts">
  import { formatUnit } from '@prusa/connect-ui/i18n';
  import type { Snippet } from 'svelte';

  import type { Tool } from '../api';
  import { heldHost } from '../host';

  let {
    feeder,
    nozzle,
    unload,
    children,
  }: {
    /** The feeder's model, such as MMU3S, empty when not reported. */
    feeder: string;
    nozzle: Tool['nozzle'];
    /** The head's Unload filament, when the printer offers it. */
    unload: Snippet | undefined;
    children: Snippet;
  } = $props();

  const t = heldHost().t;
</script>

<section
  data-part="mmu"
  class="overflow-hidden rounded-md border border-border"
>
  <header
    class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-border bg-muted px-4 py-3"
  >
    <span class="flex flex-wrap items-baseline gap-x-3">
      <span class="font-semibold">
        {feeder === '' ? t('materials.tools.mmu') : feeder}
      </span>
      <span class="text-sm text-muted-foreground">
        {[
          nozzle.diameter_mm === undefined
            ? ''
            : formatUnit(nozzle.diameter_mm, 'mm'),
          nozzle.high_flow === true ? t('materials.tools.hf') : '',
          nozzle.high_temp === true ? t('materials.tools.ht') : '',
        ]
          .filter((s) => s !== '')
          .join(' · ')}
      </span>
    </span>
    {@render unload?.()}
  </header>
  <ul class="slots grid gap-2 p-3">
    {@render children()}
  </ul>
</section>

<style>
  /* As many slots to a row as hold a circle, sharing the width. */
  .slots {
    grid-template-columns: repeat(
      auto-fit,
      minmax(calc(var(--spacing) * 36), 1fr)
    );
  }
</style>
