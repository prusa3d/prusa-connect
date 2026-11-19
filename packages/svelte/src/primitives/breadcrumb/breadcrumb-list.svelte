<!--
  The ordered list the crumbs sit in. It wraps when the trail is longer than
  the row.

  `min-w-0` is here and on the item so the last crumb can truncate. A
  `flex-wrap` list wraps between items and never inside one, so without it a
  trail ending in a long file name pushed a 320px screen sideways.
-->
<script lang="ts">
  import type { BreadcrumbListProps } from './types';

  let { children, class: className, ...rest }: BreadcrumbListProps = $props();
</script>

<ol
  data-slot="breadcrumb-list"
  class={[
    'flex min-w-0 flex-wrap break-words text-muted-foreground',
    'items-baseline gap-form-gap text-lg font-normal',
    className,
  ]}
  {...rest}
>
  {@render children?.()}
</ol>
