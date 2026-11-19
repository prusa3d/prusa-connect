<!--
  The two answers. They stack on a narrow window, with the action on top, and
  sit in a row at the end edge on a wide one. The small panel puts them in two
  equal columns whatever the window.
-->
<script lang="ts">
  import type { AlertDialogFooterProps } from './types';

  let {
    children,
    class: className,
    ...rest
  }: AlertDialogFooterProps = $props();
</script>

<div
  data-slot="alert-dialog-footer"
  class={[
    'flex flex-col-reverse gap-2 group-size-sm/alert-dialog-content:grid group-size-sm/alert-dialog-content:grid-cols-2 sm:flex-row sm:justify-end',
    className,
  ]}
  {...rest}
>
  {@render children?.()}
</div>
