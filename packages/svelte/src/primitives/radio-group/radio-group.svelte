<script lang="ts">
  // bits-ui, for the roving tabindex and the arrow-key selection a set of
  // radios needs: focus enters at the checked item, and an arrow both moves
  // and checks.
  import { RadioGroup as RadioGroupPrimitive } from 'bits-ui';

  import type { RadioGroupProps } from './types';

  let {
    ref = $bindable(null),
    value = $bindable(''),
    gap = 'md',
    class: className,
    ...rest
  }: RadioGroupProps = $props();
</script>

<RadioGroupPrimitive.Root
  bind:ref
  bind:value
  data-slot="radio-group"
  class={[
    'grid',
    gap === 'md' && 'gap-3',
    gap === 'section' && 'gap-section-block',
    className,
  ]}
  {...rest}
/>
