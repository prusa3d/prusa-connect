<!--
  The question the panel asks. bits-ui reports its `id` to the content, which
  points its `aria-labelledby` here.

  bits-ui draws a `div` with `role="heading"` and `aria-level="2"`, where Radix
  draws an `h2`. A caller that needs another level passes `level`.

  A file name can have no space. `wrap-anywhere`, unlike `break-words`, also
  lowers the grid column's min-content, so the name and buttons stay inside.
-->
<script lang="ts">
  import { AlertDialog as AlertDialogPrimitive } from 'bits-ui';

  import type { AlertDialogTitleProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    ...rest
  }: AlertDialogTitleProps = $props();
</script>

<AlertDialogPrimitive.Title
  bind:ref
  data-slot="alert-dialog-title"
  class={[
    'text-lg font-semibold wrap-anywhere sm:group-size-default/alert-dialog-content:group-has-slot-alert-dialog-media/alert-dialog-content:col-start-2',
    className,
  ]}
  {...rest}
/>
