<!--
  A two state control a person flips.

  The look is drawn by the `<style>` rules at the end. They are unlayered, so
  they outrank every utility in the markup that sets the same property: a
  borderless track sized and rounded by the `switch` tokens, with the thumb
  inset by `--spacing-switch-inset`. An enabled switch is a grey
  `switch-track` when off and `switch-on` when on, with a white thumb in both,
  and the thumb slides by `--spacing-switch-travel`. `size="sm"` takes the
  `-sm` steps.

  The markup's utilities draw what the rules leave alone: the focus ring, which
  is the Button's recipe, and a disabled switch, since the colour rules match
  `:enabled` only. The disabled colours use compound `disabled:state-*:*`
  selectors. A plain `disabled:*` ties on specificity with `state-*:*` and can
  lose, which left a disabled switch painted in the live colour.

  The thumb has no background of its own. The root sets `text-*` and the thumb
  reads it through `bg-current`, so one colour decision covers both. The tick
  is `hidden` and aria-hidden: the state is announced by the root's
  `aria-checked`.
-->
<script lang="ts">
  import CheckIcon from '@lucide/svelte/icons/check';
  import { Switch as SwitchPrimitive } from 'bits-ui';

  import type { SwitchProps } from './types';

  let {
    ref = $bindable(null),
    checked = $bindable(false),
    class: className,
    size = 'md',
    ...rest
  }: SwitchProps = $props();
</script>

<SwitchPrimitive.Root
  bind:ref
  bind:checked
  data-slot="switch"
  data-size={size}
  class={[
    'peer group/switch inline-flex shrink-0 items-center rounded-sm border border-transparent transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none state-checked:bg-control-on state-checked:text-control-on-foreground state-checked:hover:bg-primary-hovered state-checked:active:bg-primary-pressed disabled:state-checked:bg-disabled disabled:state-checked:text-disabled-foreground state-unchecked:border-input state-unchecked:bg-transparent state-unchecked:text-input state-unchecked:hover:bg-accent disabled:state-unchecked:border-transparent disabled:state-unchecked:bg-disabled disabled:state-unchecked:text-disabled-foreground',
    className,
  ]}
  {...rest}
>
  <SwitchPrimitive.Thumb
    data-slot="switch-thumb"
    class="pointer-events-none grid place-content-center rounded-sm bg-current ring-0 transition-transform state-unchecked:translate-x-0"
  >
    {#snippet children({ checked: on })}
      {#if on}
        <CheckIcon
          aria-hidden="true"
          strokeWidth={3}
          class="hidden text-control-on group-disabled/switch:text-disabled group-size-sm/switch:size-2"
        />
      {/if}
    {/snippet}
  </SwitchPrimitive.Thumb>
</SwitchPrimitive.Root>

<style lang="postcss">
  @reference '../../styles/index.css';

  :global([data-slot='switch']) {
    @apply h-switch-height w-switch-width rounded-switch;
    border-width: 0;
    padding: var(--spacing-switch-inset);
  }

  :global([data-slot='switch']:enabled),
  :global([data-slot='switch']:enabled:is(:hover, :active)) {
    @apply bg-switch-track text-white;
  }

  :global([data-slot='switch']:enabled[data-state='checked']),
  :global(
    [data-slot='switch']:enabled[data-state='checked']:is(:hover, :active)
  ) {
    @apply bg-switch-on text-white;
  }

  :global([data-slot='switch'][data-size='sm']) {
    @apply h-switch-height-sm w-switch-width-sm;
    padding: var(--spacing-switch-inset-sm);
  }

  :global([data-slot='switch'] [data-slot='switch-thumb']) {
    @apply size-switch-thumb rounded-switch motion-reduce:transition-none;
  }

  :global([data-slot='switch'][data-size='sm'] [data-slot='switch-thumb']) {
    @apply size-switch-thumb-sm;
  }

  :global(
    [data-slot='switch'][data-state='checked'] [data-slot='switch-thumb']
  ) {
    @apply translate-x-switch-travel;
  }

  :global(
    [data-slot='switch'][data-size='sm'][data-state='checked']
      [data-slot='switch-thumb']
  ) {
    @apply translate-x-switch-travel-sm;
  }
</style>
