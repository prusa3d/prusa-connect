<!--
  The name of a form control.

  A plain `<label>` element is the whole component. The browser already moves
  focus to the control named by `for`, so nothing here adds behaviour.

  A disabled control greys the words instead of fading them. A label has no
  fill, so opacity would leave the same colour in a lighter paint and the
  words would still read as live. `peer-disabled:` answers a disabled control
  that stands before the label, `group-disabled-true:` a disabled field
  around it.
-->
<script lang="ts">
  import type { LabelProps } from './types';

  let {
    variant = 'default',
    class: className,
    children,
    ...rest
  }: LabelProps = $props();
</script>

<label
  {...rest}
  data-slot="label"
  data-variant={variant}
  class={[
    'flex items-center gap-2 text-sm leading-none font-normal select-none group-disabled-true:pointer-events-none group-disabled-true:text-disabled-foreground peer-disabled:cursor-not-allowed peer-disabled:text-disabled-foreground',
    variant === 'option' && 'items-start leading-snug font-normal',
    variant === 'hint' && 'text-xs font-normal text-muted-foreground',
    className,
  ]}>{@render children?.()}</label
>
