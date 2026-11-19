<!--
  A box a person ticks. bits-ui, for the role, the keyboard and the `data-state`
  every colour below is keyed on.

  The focus ring is the Button's recipe, so a keyboard walk across a form rings
  every control the same way.

  The disabled colours use compound `disabled:state-checked:*` selectors.
  A plain `disabled:*` ties on specificity with `state-checked:*` and can
  lose, which leaves a disabled ticked box painted in the live colour. The tick
  itself is `text-current`, so the root's `text-*` covers both.
-->
<script lang="ts">
  import CheckIcon from '@lucide/svelte/icons/check';
  import MinusIcon from '@lucide/svelte/icons/minus';
  import { Checkbox as CheckboxPrimitive } from 'bits-ui';

  import type { CheckboxProps } from './types';

  let {
    ref = $bindable(null),
    checked = $bindable(false),
    indeterminate = $bindable(false),
    class: className,
    ...rest
  }: CheckboxProps = $props();
</script>

<CheckboxPrimitive.Root
  bind:ref
  bind:checked
  bind:indeterminate
  data-slot="checkbox"
  class={[
    'peer size-checkbox shrink-0 rounded-checkbox border border-input transition-colors outline-none hover:border-checkbox-on focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:text-disabled-foreground state-checked:border-control-on state-checked:bg-control-on state-checked:text-control-on-foreground enabled:state-checked:border-checkbox-on enabled:state-checked:bg-checkbox-on enabled:state-checked:text-control-on-foreground enabled:state-checked:hover:border-checkbox-on enabled:state-checked:hover:bg-checkbox-on enabled:state-checked:active:bg-checkbox-on disabled:state-checked:bg-disabled disabled:state-checked:text-disabled-foreground state-indeterminate:border-control-on state-indeterminate:bg-control-on state-indeterminate:text-control-on-foreground',
    'disabled:state-checked:border-border disabled:state-indeterminate:border-border disabled:state-unchecked:border-border',
    className,
  ]}
  {...rest}
>
  {#snippet children({ checked: ticked, indeterminate: mixed })}
    <!-- bits-ui has no indicator part of its own, so the mark is drawn here:
         a tick, or a dash for a box that stands for rows of which only some
         are ticked. -->
    {#if mixed}
      <span
        data-slot="checkbox-indicator"
        class="grid place-content-center text-current transition-none"
      >
        <MinusIcon class="size-3.5" />
      </span>
    {:else if ticked}
      <span
        data-slot="checkbox-indicator"
        class="grid place-content-center text-current transition-none"
      >
        <CheckIcon class="size-checkbox-mark" />
      </span>
    {/if}
  {/snippet}
</CheckboxPrimitive.Root>
