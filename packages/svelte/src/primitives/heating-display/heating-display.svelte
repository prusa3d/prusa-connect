<!--
  A whole heater reading, ready to drop into a chip, a card slot, a table cell
  or a dense row: the glyph that names the heater, the current value, and the
  target it is working towards.

  Every listing in this product renders the same four heaters. Before this
  existed each did it its own way, so the fleet's cards printed a bare `22°C`
  with a grey glyph while the printer's own header printed a tinted `22°/215°`.
  The same machine in the same state read as two different facts depending on
  which screen you were on. One component is what stops that.

  The glyph sits inside the tint, so `currentColor` carries the state to the
  drawing as well as to the number. At a chip's size the glyph is the larger of
  the two marks, and colouring only the smaller half spends the colour badly.

  The target is left out when the printer reports none, or reports zero. A
  heater told to go to zero is off, and `48°/0°` reads as a target rather than
  as the absence of one. It keeps `--muted-foreground` of its own, because it
  is a setting rather than a reading and should not pulse with one.
-->
<script lang="ts">
  import { chamberState, heaterState } from '../../domain';
  import { wholeDegrees } from './degrees';
  import HeatReading from './heat-reading.svelte';
  import type { TemperatureReadingProps } from './types';

  let {
    icon,
    current,
    target,
    muted = false,
    chamber = false,
    class: className,
  }: TemperatureReadingProps = $props();

  const state = $derived(
    (chamber ? chamberState : heaterState)({ current, target }),
  );
</script>

{#if current === undefined}
  <span data-slot="heating-display" class={['text-muted-foreground', className]}
    >&mdash;</span
  >
{:else}
  <HeatReading {state} {muted} class={className}>
    {@render icon()}
    {wholeDegrees(current)}&deg;
    {#if target !== undefined && target > 0}
      <span class="text-muted-foreground">/{wholeDegrees(target)}&deg;</span>
    {/if}
  </HeatReading>
{/if}
