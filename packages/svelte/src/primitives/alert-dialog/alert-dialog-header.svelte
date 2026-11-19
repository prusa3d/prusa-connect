<!--
  The title and the sentence under it, and the mark beside them when there is
  one.

  The rows are centred on a narrow window and on the small panel. On a wide
  window the default panel puts them on the start edge, and a mark takes a
  column of its own beside them.
-->
<script lang="ts">
  import type { AlertDialogHeaderProps } from './types';

  let {
    children,
    class: className,
    ...rest
  }: AlertDialogHeaderProps = $props();
</script>

<div
  data-slot="alert-dialog-header"
  class={[
    'grid grid-rows-alert-dialog-header place-items-center gap-1.5 text-center has-slot-alert-dialog-media:grid-rows-alert-dialog-header-media has-slot-alert-dialog-media:gap-x-6 sm:group-size-default/alert-dialog-content:place-items-start sm:group-size-default/alert-dialog-content:text-left sm:group-size-default/alert-dialog-content:has-slot-alert-dialog-media:grid-rows-alert-dialog-header',
    className,
  ]}
  {...rest}
>
  {@render children?.()}
</div>
