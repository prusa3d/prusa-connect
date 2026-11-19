<!-- One cell of data. -->
<script lang="ts">
  import type { TableCellProps } from './types';

  let {
    roomy = false,
    class: className,
    children,
    ...rest
  }: TableCellProps = $props();
</script>

<td
  data-slot="table-cell"
  class={[
    'px-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-checkbox-baseline',
    roomy ? 'py-4' : 'py-2',
    className,
  ]}
  {...rest}
>
  {@render children?.()}
</td>
