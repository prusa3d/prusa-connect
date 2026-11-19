<!--
  A value a person drags along a track, with one thumb or two.

  Track and thumb are `rounded-sm`, the shape the rest of the library uses, and
  the thumb answers hover and press with its border colour rather than the
  growing ring upstream draws. The drag position is deliberately untransitioned
  so the thumb follows the pointer one to one.

  Disabled is read from `data-disabled`: the thumb is a `<span role="slider">`
  and not a native control, so `:disabled` never matches it. The disabled thumb
  takes a border and a fill of its own, because `border-transparent` made it
  disappear into the page.

  `aria-label` and `aria-labelledby` go to every thumb and not to the root,
  since `role="slider"` lives on the thumb and that is the element axe reads a
  name off (`aria-input-field-name`).
-->
<script lang="ts">
  import { Slider as SliderPrimitive } from 'bits-ui';

  import type { SliderProps } from './types';

  let {
    ref = $bindable(null),
    value = $bindable(),
    orientation = 'horizontal',
    tone = 'default',
    class: className,
    'aria-label': ariaLabel,
    'aria-labelledby': ariaLabelledby,
    ...rest
  }: SliderProps = $props();

  // Static strings chosen by key, so Tailwind emits every one (svelte-5.md).
  const ranges = {
    default: 'bg-control-on',
    primary: 'bg-primary',
  } as const;
  const edges = {
    default:
      'border-control-on hover:border-control-on active:border-control-on',
    primary: 'border-primary hover:border-primary active:border-primary',
  } as const;
</script>

<SliderPrimitive.Root
  bind:ref
  bind:value={value as never}
  {orientation}
  data-slot="slider"
  data-tone={tone}
  class={[
    'relative flex w-full touch-none items-center select-none data-disabled:pointer-events-none orientation-vertical:h-full orientation-vertical:min-h-44 orientation-vertical:w-auto orientation-vertical:flex-col',
    className,
  ]}
  {...rest}
>
  {#snippet children({ thumbItems })}
    <!-- The headless layer has no track part, so the track is plain markup and
         takes the orientation from here rather than from the primitive. -->
    <span
      data-slot="slider-track"
      data-orientation={orientation}
      class="relative grow overflow-hidden rounded-full bg-muted orientation-horizontal:h-1.5 orientation-horizontal:w-full orientation-vertical:h-full orientation-vertical:w-1.5"
    >
      <SliderPrimitive.Range
        data-slot="slider-range"
        class={[
          'absolute data-disabled:bg-disabled orientation-horizontal:h-full orientation-vertical:w-full',
          ranges[tone],
        ]}
      />
    </span>
    {#each thumbItems as thumb (thumb.index)}
      <SliderPrimitive.Thumb
        index={thumb.index}
        data-slot="slider-thumb"
        aria-label={ariaLabel}
        aria-labelledby={ariaLabelledby}
        class={[
          'block size-4 shrink-0 rounded-full border bg-background transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background data-disabled:border-disabled-foreground data-disabled:bg-disabled',
          edges[tone],
        ]}
      />
    {/each}
  {/snippet}
</SliderPrimitive.Root>
