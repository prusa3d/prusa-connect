<script lang="ts">
  import CircleIcon from '@lucide/svelte/icons/circle';
  import { RadioGroup as RadioGroupPrimitive } from 'bits-ui';

  import type { RadioGroupItemProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    ...rest
  }: RadioGroupItemProps = $props();
</script>

<!-- `rounded-full` is what says radio rather than checkbox. The checked and
     disabled colours need the compound `disabled:state-checked:*`
     selector, because a plain `disabled:*` ties with `state-checked:*`
     on specificity and the checked colour wins the tie. -->
<RadioGroupPrimitive.Item
  bind:ref
  data-slot="radio-group-item"
  class={[
    'aspect-square size-4 shrink-0 rounded-full border border-input transition-colors outline-none hover:border-primary focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background enabled:hover:border-control-on disabled:pointer-events-none disabled:border-disabled-foreground state-checked:border-control-on state-checked:text-control-on enabled:state-checked:border-control-on enabled:state-checked:text-control-on enabled:state-checked:hover:border-control-on enabled:state-checked:hover:text-control-on enabled:state-checked:active:text-control-on disabled:state-checked:border-disabled-foreground disabled:state-checked:text-disabled-foreground',
    className,
  ]}
  {...rest}
>
  <!-- bits-ui has no indicator part. The item's children snippet reports
       whether it is checked, and the dot is drawn here from that. It is
       filled with the item's own text colour, so one `text-*` rule above
       colours both the ring and the dot. -->
  {#snippet children({ checked })}
    {#if checked}
      <span
        data-slot="radio-group-indicator"
        class="relative flex items-center justify-center"
      >
        <CircleIcon
          class="absolute top-1/2 left-1/2 size-2 -translate-x-1/2 -translate-y-1/2 fill-current"
        />
      </span>
    {/if}
  {/snippet}
</RadioGroupPrimitive.Item>
