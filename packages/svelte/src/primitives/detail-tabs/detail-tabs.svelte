<!--
  The strip scrolls and fades its edges (`ScrollStrip`).

  The open tab is the caller's: the strip follows `active` and reports the
  press, so a screen is free to keep the open tab in the path.
-->
<script lang="ts">
  import Badge from '../badge/badge.svelte';
  import Skeleton from '../skeleton/skeleton.svelte';
  import TabsContent from '../tabs/tabs-content.svelte';
  import TabsList from '../tabs/tabs-list.svelte';
  import TabsTrigger from '../tabs/tabs-trigger.svelte';
  import Tabs from '../tabs/tabs.svelte';
  import ScrollStrip from './scroll-strip.svelte';
  import type { DetailTabsProps } from './types';

  let {
    tabs,
    active,
    onChange,
    fallback,
    segmented = false,
    class: className,
  }: DetailTabsProps = $props();
</script>

<Tabs
  bind:value={() => active, (next: string) => onChange(next)}
  class={['w-full gap-6', className]}
>
  {#if segmented}
    <div
      class="segmented-strip"
      style:--tab-count={tabs.length}
      data-dense={tabs.length > 7 ? '' : undefined}
    >
      {@render tabList()}
    </div>
  {:else}
    <ScrollStrip {active} {tabs}>
      {@render tabList()}
    </ScrollStrip>
  {/if}

  <!-- The primitive mounts a panel per tab and hides the closed ones, so the
       body is drawn only for the open tab: a closed panel would otherwise
       hold live telemetry or pulse a skeleton nobody is looking at. -->
  {#each tabs as tab (tab.id)}
    <TabsContent value={tab.id}>
      {#if tab.id === active}
        {#if tab.panel}
          {@render tab.panel()}
        {:else if fallback}
          {@render fallback()}
        {:else}
          <div class="flex flex-col gap-4">
            <Skeleton class="h-32 w-full" />
            <Skeleton class="h-4 w-2/3" />
            <Skeleton class="h-4 w-1/2" />
          </div>
        {/if}
      {/if}
    </TabsContent>
  {/each}
</Tabs>

{#snippet tabList()}
  <TabsList class="h-auto gap-6">
    {#each tabs as tab (tab.id)}
      <TabsTrigger value={tab.id} class="min-h-11">
        {#if segmented && tab.icon}
          <tab.icon aria-hidden="true" />
        {/if}
        <span>{tab.label}</span>
        {#if tab.count !== undefined}
          <Badge variant="default" class="ms-1 tabular-nums">{tab.count}</Badge>
        {/if}
      </TabsTrigger>
    {/each}
  </TabsList>
{/snippet}

<style>
  .segmented-strip {
    container-type: inline-size;
  }

  .segmented-strip :global([data-slot='tabs-list']) {
    display: grid;
    inline-size: 100%;
    grid-template-columns: repeat(var(--tab-count), minmax(0, 1fr));
    align-items: stretch;
    gap: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: var(--color-surface);
  }

  .segmented-strip :global([data-slot='tabs-trigger']) {
    position: relative;
    min-inline-size: 0;
    min-block-size: var(--spacing-detail-tab-height);
    padding: calc(var(--spacing) * 2);
    border: 0;
    border-radius: var(--radius-sm) var(--radius-sm) 0 0;
    text-transform: none;
    letter-spacing: normal;
    white-space: normal;
    overflow-wrap: anywhere;
  }

  .segmented-strip :global([data-slot='tabs-trigger'][data-state='active']) {
    background: color-mix(
      in oklab,
      var(--color-card) 60%,
      var(--color-secondary)
    );
    font-weight: var(--font-weight-semibold);
  }

  .segmented-strip :global([data-slot='tabs-trigger']:focus-visible) {
    outline: 2px solid var(--color-ring);
    outline-offset: -2px;
    box-shadow: none;
  }

  .segmented-strip :global([data-slot='tabs-trigger']:not(:last-child))::after {
    content: '';
    position: absolute;
    inset-block: 27%;
    inset-inline-end: 0;
    inline-size: 1px;
    background: var(--color-border);
  }

  .segmented-strip
    :global([data-slot='tabs-trigger'][data-state='active'])::after,
  .segmented-strip
    :global([data-slot='tabs-trigger']:has(+ [data-state='active']))::after {
    content: none;
  }

  @container (width <= 46rem) {
    .segmented-strip :global([data-slot='tabs-list']) {
      grid-template-columns: repeat(min(var(--tab-count), 4), minmax(0, 1fr));
    }
  }

  /* Many tabs in one row leave a label no room beside its icon, so the label
     would break inside a word. */
  @container (46rem < width <= 64rem) {
    .segmented-strip[data-dense]
      :global([data-slot='tabs-trigger'] > svg:first-child) {
      display: none;
    }
  }

  @container (28rem < width <= 46rem) {
    .segmented-strip :global([data-slot='tabs-trigger']:nth-child(4n))::after {
      content: none;
    }
  }

  @container (width <= 28rem) {
    .segmented-strip :global([data-slot='tabs-list']) {
      grid-template-columns: repeat(min(var(--tab-count), 3), minmax(0, 1fr));
    }

    .segmented-strip :global([data-slot='tabs-trigger']:nth-child(3n))::after {
      content: none;
    }
  }

  @media (pointer: coarse) {
    .segmented-strip :global([data-slot='tabs-trigger']) {
      min-block-size: var(--spacing-touch-target);
    }
  }
</style>
