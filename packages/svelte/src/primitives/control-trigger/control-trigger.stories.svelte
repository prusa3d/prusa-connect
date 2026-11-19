<script module lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import {
    CONTROL_FILL,
    CONTROL_RING,
    CONTROL_SIZES,
    CONTROL_TRIGGER,
  } from './control-trigger';
  import type { ControlSize } from './types';

  const { Story } = defineMeta({
    title: 'Primitives/Control Trigger',
  });

  const SIZES: ControlSize[] = ['sm', 'md', 'lg'];
</script>

<!--
  A class recipe, not a component: the select and the facet popover wear it.
  These stories put it on a plain button to show the three sizes and the
  disabled state.
-->

<Story name="Sizes" asChild>
  <div class="flex flex-wrap items-center gap-4">
    {#each SIZES as size (size)}
      <button
        type="button"
        class={[
          CONTROL_TRIGGER,
          CONTROL_FILL,
          CONTROL_RING,
          CONTROL_SIZES[size],
        ]}
      >
        <span>Model: {size}</span>
        <ChevronDownIcon />
      </button>
    {/each}
  </div>
</Story>

<Story name="Disabled" asChild>
  <button
    type="button"
    disabled
    class={[CONTROL_TRIGGER, CONTROL_FILL, CONTROL_RING, CONTROL_SIZES.md]}
  >
    <span>Model</span>
    <ChevronDownIcon />
  </button>
</Story>
