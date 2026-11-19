<!--
  A line between two pieces of content.

  A decorative line gets `role="none"`, so a screen reader walks past it. A
  line that marks a real break reports `role="separator"`.

  Only a vertical separator reports `aria-orientation`. Horizontal is the
  default of the role, so saying it again adds nothing.

  The thickness comes from the orientation on the element itself, through the
  two `orientation-*` pairs, so one class string covers both ways.
-->
<script lang="ts">
  import type { SeparatorProps } from './types';

  let {
    orientation = 'horizontal',
    decorative = true,
    class: className,
    ...rest
  }: SeparatorProps = $props();

  const announced = $derived(
    !decorative && orientation === 'vertical' ? 'vertical' : undefined,
  );
</script>

<div
  {...rest}
  data-slot="separator"
  data-orientation={orientation}
  role={decorative ? 'none' : 'separator'}
  aria-orientation={announced}
  class={[
    'shrink-0 bg-border orientation-horizontal:h-px orientation-horizontal:w-full orientation-vertical:h-full orientation-vertical:w-px',
    className,
  ]}
></div>
