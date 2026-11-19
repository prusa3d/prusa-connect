<!--
  The box a person presses to open the list.

  The box, its border and its hover come from the shared control recipe, so a
  select and a facet popover standing in the same toolbar are one control
  twice rather than two kinds of control. The placeholder colour, the layout
  of the value and the focus are the select's own: it takes no ring, and
  focus shows as the primary border, the way a text field's does.

  bits-ui puts no `role` on the button, so it is set here to `combobox`, as
  Radix does. A plain button takes its name from its content, and WCAG 2.5.3
  then wants the shown value inside the `aria-label` a caller names it with. A
  combobox keeps the value apart from the name, which is what a select is.

  React draws the chevron through Radix's `Icon` part. bits-ui has no such
  part, so the glyph sits in the markup.
-->
<script lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import { Select as SelectPrimitive } from 'bits-ui';

  import {
    CONTROL_FILL,
    CONTROL_SIZES,
    CONTROL_TRIGGER,
  } from '../control-trigger/control-trigger';
  import type { SelectTriggerProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    size = 'md',
    tone = 'default',
    children,
    ...rest
  }: SelectTriggerProps = $props();
</script>

<SelectPrimitive.Trigger
  bind:ref
  role="combobox"
  data-slot="select-trigger"
  data-size={size}
  class={[
    CONTROL_TRIGGER,
    '@max-3xl/app-shell:min-h-touch-target pointer-coarse:min-h-touch-target',
    CONTROL_SIZES[size],
    'not-aria-invalid:border-secondary data-placeholder:text-muted-foreground disabled:data-placeholder:text-disabled-foreground *:slot-select-value:line-clamp-1 *:slot-select-value:flex *:slot-select-value:items-center *:slot-select-value:gap-2',
    'not-data-on:not-aria-invalid:focus-visible:border-primary not-data-on:not-aria-invalid:state-open:border-primary',
    tone === 'default' && CONTROL_FILL,
    tone === 'info' && 'bg-info text-info-foreground',
    className,
  ]}
  {...rest}
>
  {@render children?.()}
  <ChevronDownIcon class="size-6" />
</SelectPrimitive.Trigger>
