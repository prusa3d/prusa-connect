<!--
  How far along a job is.

  `value` goes to the primitive as well as to the fill. Left off, the primitive
  reads as indeterminate: the bar showed the right fill while `aria-valuenow`
  stayed unset and `data-state` stayed `indeterminate`.

  The track is `bg-border` and not `bg-muted`. `--muted` and `--card` resolve to
  the same value in the dark theme, so a track on a card had no edge at all and
  a part filled bar read as a short rule floating in space.

  With no reading the transform is dropped and the whole track pulses in grey,
  so it is not read as a full bar.
  `translateX(-100%)` renders an empty bar, which nobody can tell apart from
  0 per cent. `animate-pulse` and not `animate-spin`: tokens.css zeroes it
  under prefers-reduced-motion.
-->
<script lang="ts">
  import { Progress as ProgressPrimitive } from 'bits-ui';

  import type { ProgressProps } from './types';
  import { sizes, tones, UNKNOWN } from './variants';

  let {
    ref = $bindable(null),
    class: className,
    value,
    max = 100,
    tone = 'primary',
    size = 'md',
    ...rest
  }: ProgressProps = $props();

  const indeterminate = $derived(value === null || value === undefined);
  // How much of the track is still empty, as a percentage the fill slides left
  // by. The headless layer has no indicator part, so the arithmetic is here.
  const empty = $derived(String(100 - ((value ?? 0) / max) * 100));
</script>

<ProgressPrimitive.Root
  bind:ref
  data-slot="progress"
  value={value ?? null}
  {max}
  class={[
    'relative w-full overflow-hidden rounded-sm bg-border',
    sizes[size],
    className,
  ]}
  {...rest}
>
  <div
    data-slot="progress-indicator"
    class={[
      'h-full w-full flex-1 rounded-sm transition-transform',
      indeterminate ? UNKNOWN : tones[tone],
      indeterminate && 'animate-pulse',
    ]}
    style={indeterminate ? undefined : `transform: translateX(-${empty}%)`}
  ></div>
</ProgressPrimitive.Root>
