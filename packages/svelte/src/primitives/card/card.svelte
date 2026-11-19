<!--
  A bordered surface holding one thing: a printer, a job, a group of settings.

  Padding is 16px until the card itself clears 24rem, and 24px above that. On a
  320px screen the page's own 16px inset plus 24px of card each side left the
  content 240px wide, a third of the width spent on air. The card names its own
  container, so the step keys off the card's width and not the viewport's: a
  card in a narrow column on a wide screen has the same problem.
-->
<script lang="ts">
  import type { CardProps } from './types';

  let {
    tone = 'default',
    density = 'default',
    radius = 'sm',
    class: className,
    children,
    ...rest
  }: CardProps = $props();
</script>

<div
  data-slot="card"
  data-tone={tone}
  data-density={density}
  class={[
    '@container/card flex flex-col text-card-foreground data-settings-panel:rounded-md',
    radius === 'sm' && 'rounded-sm',
    radius === 'md' && 'rounded-md',
    density === 'default' &&
      'gap-4 py-4 @[24rem]/card:gap-6 @[24rem]/card:py-6',
    density === 'compact' && 'gap-3 py-4',
    density === 'snug' && 'gap-2 py-4',
    density === 'tight' && 'gap-1 py-2',
    density === 'flush' && 'gap-0 py-0',
    density === 'panel' && 'gap-4 py-0 @[24rem]/card:gap-6',
    tone === 'default' &&
      'border border-border bg-card not-dark:border-panel-outline',
    tone === 'critical' && 'border border-destructive bg-destructive/5',
    tone === 'attention' && 'border border-warning bg-warning/5',
    tone === 'surface' && 'bg-surface',
    className,
  ]}
  {...rest}
>
  {@render children?.()}
</div>

<style lang="postcss">
  @reference '../../styles/index.css';

  [data-settings-panel] {
    @apply relative gap-0 overflow-clip border-0 py-0 shadow-settings-pane;
  }

  :global(.dark) [data-settings-panel] {
    @apply shadow-none;
  }

  [data-settings-panel] > :global([data-slot='card-header']) {
    @apply min-h-settings-header gap-form-note bg-secondary px-settings-header-inline py-settings-header-inset;
  }

  [data-settings-panel]
    > :global([data-slot='card-header'] [data-slot='card-title']) {
    @apply font-normal;
  }

  [data-settings-panel]
    > :global(
      [data-slot='card-header']:not(:has([data-slot='card-description']))
        [data-slot='card-title']
    ) {
    @apply row-span-2 self-center;
  }

  [data-settings-panel]
    > :global([data-slot='card-header'] [data-slot='card-action']) {
    @apply self-center;
  }

  [data-settings-panel] > :global([data-slot='card-content']) {
    @apply min-w-0 gap-section-gap p-panel-padding-mobile @lg/card:p-panel-padding;
  }

  [data-density='panel'] > :global([data-slot='card-content']) {
    @apply px-panel-padding-mobile @lg/card:px-panel-padding;
  }
</style>
