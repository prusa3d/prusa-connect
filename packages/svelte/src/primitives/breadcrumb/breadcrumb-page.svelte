<!--
  The page a person is on. It is a span and not a link, with
  `aria-current="page"` to say which crumb it is.

  Upstream shadcn gives it `role="link"` and `aria-disabled`. A screen reader
  still reports that as a link, and the W3C APG breadcrumb pattern asks for the
  current item to be named rather than linked. So only the marker is kept.

  It truncates. The last crumb is often a file name, and the list lets it
  shrink instead of pushing the page sideways.
-->
<script lang="ts">
  import type { BreadcrumbPageProps } from './types';

  let { children, class: className, ...rest }: BreadcrumbPageProps = $props();
</script>

<span
  data-slot="breadcrumb-page"
  aria-current="page"
  class={['min-w-0 truncate font-semibold text-foreground', className]}
  {...rest}
>
  {@render children?.()}
</span>
