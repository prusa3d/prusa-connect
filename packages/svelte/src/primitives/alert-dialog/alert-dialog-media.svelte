<!--
  The mark beside the question, a glyph in a filled square. The header and the
  title both key their layout on this slot being present, so a caller adds it
  and the rows rearrange themselves.
-->
<script lang="ts">
  import type { AlertDialogMediaProps } from './types';

  let { children, class: className, ...rest }: AlertDialogMediaProps = $props();
</script>

<div
  data-slot="alert-dialog-media"
  class={[
    "mb-2 inline-flex size-16 items-center justify-center rounded-sm bg-muted sm:group-size-default/alert-dialog-content:row-span-2 *:[svg:not([class*='size-'])]:size-8",
    className,
  ]}
  {...rest}
>
  {@render children?.()}
</div>
