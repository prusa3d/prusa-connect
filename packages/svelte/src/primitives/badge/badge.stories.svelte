<script module lang="ts">
  import StoreIcon from '@lucide/svelte/icons/store';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { PrinterIcon } from '../printer-icon';
  import Badge from './badge.svelte';
  import type { BadgeVariant } from './types';

  const { Story } = defineMeta({
    title: 'Primitives/Badge',
    component: Badge,
  });

  const states: [BadgeVariant, string][] = [
    ['stateActive', 'Printing'],
    ['stateBusy', 'Busy'],
    ['stateMoving', 'Moving'],
    ['stateReady', 'Ready'],
    ['stateFinished', 'Finished'],
    ['stateStopped', 'Stopped'],
    ['statePaused', 'Paused'],
    ['stateIdle', 'Idle'],
    ['stateAttention', 'Needs attention'],
    ['stateFault', 'Error'],
    ['stateQuiet', 'Unknown'],
    ['stateOffline', 'Offline'],
  ];
</script>

<Story name="Default">Ready</Story>

<Story name="All Variants" asChild>
  <div class="flex flex-wrap gap-2">
    <Badge variant="default">Default</Badge>
    <Badge variant="secondary">Secondary</Badge>
    <Badge variant="outline">Outline</Badge>
    <Badge variant="success">Online</Badge>
    <Badge variant="warning">Attention</Badge>
    <Badge variant="info">Queued</Badge>
    <Badge variant="destructive">Failed</Badge>
  </div>
</Story>

<Story name="Status Variants Clear AA Contrast" asChild>
  <div class="flex flex-wrap gap-2">
    <Badge variant="default">Default</Badge>
    <Badge variant="secondary">Secondary</Badge>
    <Badge variant="outline">Outline</Badge>
    <Badge variant="success">Online</Badge>
    <Badge variant="warning">Attention</Badge>
    <Badge variant="info">Queued</Badge>
    <Badge variant="destructive">Failed</Badge>
  </div>
</Story>

<!-- Beyond the React set -->

<Story name="Printer States" asChild>
  <div class="flex flex-wrap gap-2">
    {#each states as [variant, label] (variant)}
      <Badge {variant}>{label}</Badge>
    {/each}
  </div>
</Story>

<Story name="As A Link" asChild>
  <Badge variant="default">
    {#snippet child({ props })}
      <a {...props} href="#printers">Printers</a>
    {/snippet}
  </Badge>
</Story>

<Story name="Mono" asChild>
  <div class="flex flex-wrap gap-2">
    <Badge variant="outline" mono>Mono outline</Badge>
    <Badge variant="stateActive" mono={false}>Plain state</Badge>
  </div>
</Story>

<Story name="Sizes" asChild>
  <div class="flex flex-wrap items-center gap-2">
    <Badge size="sm">Small</Badge>
    <Badge size="md">Medium</Badge>
    <Badge size="lg">Large</Badge>
  </div>
</Story>

<Story name="Large, with a picture" asChild>
  <Badge variant="outline" size="lg">
    {#snippet child({ props })}
      <a {...props} href="#printer">
        <PrinterIcon model="mk4" class="size-9" />
        Bench one
      </a>
    {/snippet}
  </Badge>
</Story>

<Story name="Readings" asChild>
  <div class="flex flex-wrap gap-2">
    <Badge variant="secondary" size="md" numeric>120 mm/s</Badge>
    <Badge variant="secondary" size="md" numeric>215 °C</Badge>
  </div>
</Story>

<!-- In the size and weight of the text it stands in. -->
<Story name="Text Size" asChild>
  <Badge size="text" variant="outline">Marketplace</Badge>
</Story>

<Story name="Plain Size" asChild>
  <Badge size="plain" variant="outline"><StoreIcon />Marketplace</Badge>
</Story>
