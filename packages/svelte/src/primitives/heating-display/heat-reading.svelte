<!--
  A heater reading with its trend on it: the tint that says what the heater is
  doing and the arrow that says which way the number is going.

  Two glyphs, never one icon turned upside down. Colour is not this reading's
  only carrier (WCAG 1.4.1), and one shape rotated is still one shape doing two
  jobs rather than two shapes that each mean one thing. The arrow is hidden
  from a screen reader, because the word that names the state is beside it and
  already carries the direction in language.

  Every listing renders this, so the fleet's cards, the fleet's table and a
  printer's own header cannot land on different tints for the same state.

  React writes the tint as a ternary chain inside `cn()`, because its
  `keyword-colours.test.ts` reads a call's own literals and cannot follow a
  lookup. That gate is not in this tree, and svelte-5.md asks for a static map
  chosen by key, so the tints are a record in variants.ts here.
-->
<script lang="ts">
  import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
  import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';

  import type { HeatReadingProps } from './types';
  import { arrow, base, tints } from './variants';

  let {
    state,
    muted = false,
    class: className,
    children,
    ...rest
  }: HeatReadingProps = $props();
</script>

<span
  data-slot="heating-display"
  data-variant={state}
  {...rest}
  class={[base, !muted && tints[state], className]}
>
  {@render children()}
  {#if !muted && state === 'heating'}
    <ArrowUpIcon aria-hidden="true" class={arrow} />
  {:else if !muted && state === 'cooling'}
    <ArrowDownIcon aria-hidden="true" class={arrow} />
  {/if}
</span>
