<!--
  A row of colour swatches, one of which may be picked. The picked swatch has a
  primary ring; a focused swatch has the focus ring in its place, because both
  are `ring-*`, so focus replaces the selected mark while it is there.
-->
<script lang="ts">
  import type { SwatchPickerProps } from './types';

  let {
    swatches,
    value,
    onPick,
    label,
    deselectable = false,
    size = 'sm',
    class: className,
  }: SwatchPickerProps = $props();

  const SIZES = {
    xs: 'size-5 rounded-full',
    sm: 'size-6 rounded-sm',
    md: 'size-8 rounded-sm',
  } as const;
</script>

<div
  role="group"
  data-slot="swatch-picker"
  aria-label={label}
  class={['flex flex-wrap items-center gap-2', className]}
>
  {#each swatches as swatch (swatch.key)}
    {@const on = value === swatch.key}
    <button
      type="button"
      data-slot="swatch"
      aria-label={swatch.label}
      title={swatch.label}
      aria-pressed={on}
      class={[
        'cursor-pointer border border-border bg-(--swatch) focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none',
        SIZES[size],
        on && 'ring-2 ring-primary',
      ]}
      style="--swatch: {swatch.hex};"
      onclick={() => onPick(on && deselectable ? undefined : swatch.key)}
    ></button>
  {/each}
</div>
