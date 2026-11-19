<script lang="ts">
  import { onMount } from 'svelte';

  import { t } from '../../i18n';
  import { getSlotContext } from '../context';
  import type { ModulePageProps } from './types';

  let {
    namespace,
    path,
    registry,
    loader,
    fallback,
    class: className,
  }: ModulePageProps = $props();

  const context = getSlotContext();
  const registryInUse = $derived(registry ?? context?.registry);
  const loaderInUse = $derived(loader ?? context?.loader);

  const page = $derived(
    registryInUse
      ?.pages()
      .find((row) => row.namespace === namespace && row.path === path),
  );

  // The bundle is imported when its page first opens (ADR-20260911-4p6 rule 25).
  onMount(() => {
    void loaderInUse?.activate();
  });

  const dropped = (error: unknown) => {
    registryInUse?.refuse({
      namespace,
      reason: 'threw',
      detail: error instanceof Error ? error.message : String(error),
    });
  };
</script>

{#if page}
  {@const Page = page.component}
  <!-- The page renders inside a boundary, so a module that throws costs its
       own screen and not the app (ADR-20260911-4p6 rule 26). -->
  <svelte:boundary onerror={dropped}>
    <div class={className} data-slot="page" data-module={namespace}>
      <Page />
    </div>
    {#snippet failed()}
      <p class="text-sm text-muted-foreground">{t('modulePage.failed')}</p>
    {/snippet}
  </svelte:boundary>
{:else}
  {@render fallback?.()}
{/if}

<style lang="postcss">
  @reference '../../styles/index.css';

  /* Match the space below the Printers header in both module layouts. */
  [data-slot='page'] > :global([data-slot='page-header']:not(:last-child)),
  [data-slot='page']
    > :global([data-slot='page-header'] + [data-slot='listing-toolbar']) {
    @apply mb-6;
  }

  [data-slot='page']
    > :global(
      :is(.grid, .flex.flex-col):has(> [data-slot='page-header']:first-child)
    ) {
    @apply gap-y-6;
  }

  [data-slot='page'] :global([data-slot='page-header-actions']) {
    @apply ms-auto max-w-full flex-wrap justify-end;
  }

  [data-slot='page']
    :global([data-slot='page-header-actions'] > div:not([data-slot])) {
    @apply justify-end;
  }

  [data-slot='page']
    :global(
      :is(
          [data-slot='page-header-actions'],
          [data-slot='listing-toolbar-actions']
        )
        :is(
          [data-slot='button'],
          [data-slot='dropdown-menu-trigger'][data-variant],
          [data-slot='tooltip-trigger'][data-variant],
          [data-slot='select-trigger'],
          [data-slot='toggle'],
          [data-slot='toggle-group-item']
        ):not([data-variant='link'])
    ) {
    @apply h-input-height min-h-input-height text-xs @max-3xl/app-shell:min-h-touch-target pointer-coarse:min-h-touch-target;
  }

  [data-slot='page']
    :global(
      :is(
          [data-slot='page-header-actions'],
          [data-slot='listing-toolbar-actions']
        )
        :is(
          [data-slot='button'][data-variant],
          [data-slot='dropdown-menu-trigger'][data-variant]
        ):not([data-variant='default'], [data-variant='link'])
    ) {
    @apply font-normal;
  }

  [data-slot='page']
    :global(
      [data-slot='listing-toolbar-actions']
        [data-slot='toggle-group'][data-variant='outline']
        [data-slot='tooltip-trigger'][role='radio']
    ) {
    @apply w-input-height min-w-input-height p-0 @max-3xl/app-shell:min-w-touch-target pointer-coarse:min-w-touch-target;
  }

  [data-slot='page']
    :global(
      [data-slot='page-header-actions']
        [data-slot='toggle-group'][data-variant='default']
        [data-slot='toggle-group-item']:not([data-state='on'], :disabled)
    ) {
    @apply border-border;
  }

  [data-slot='page']
    :global(
      [data-slot='page-header-actions']
        [data-slot='toggle-group'][data-variant='default']
        [data-slot='toggle-group-item']:not(:first-child)
    ) {
    @apply border-s-0;
  }
</style>
