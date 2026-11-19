<!--
  One row.

  Every rule in the table pairs its width utility with `border-border`. A
  Tailwind `border-*` utility sets the width alone, and an unpainted border
  falls back to `currentColor`, which is the text colour of whatever it sits
  in.

  A selected row takes the muted fill. `state-selected:hover:bg-accent`
  is a compound selector, so a selected row under the pointer still reads as
  hovered: a plain `hover:bg-accent` ties with the selected rule on specificity
  and loses on order.
-->
<script lang="ts">
  import type { TableRowProps } from './types';

  let { class: className, children, ...rest }: TableRowProps = $props();
</script>

<tr
  data-slot="table-row"
  class={[
    'border-b border-border transition-colors hover:bg-accent has-aria-expanded:bg-accent state-selected:bg-muted state-selected:hover:bg-accent',
    className,
  ]}
  {...rest}
>
  {@render children?.()}
</tr>
