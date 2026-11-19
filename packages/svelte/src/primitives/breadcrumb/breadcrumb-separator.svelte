<!--
  The mark between two crumbs. A caller can supply its own mark.
  Screen readers use the list order.
-->
<script lang="ts">
  import type { BreadcrumbSeparatorProps } from './types';

  let {
    children,
    class: className,
    ...rest
  }: BreadcrumbSeparatorProps = $props();
</script>

<li
  data-slot="breadcrumb-separator"
  role="presentation"
  aria-hidden="true"
  class={['[&>svg]:size-3.5', className]}
  {...rest}
>
  {#if children}
    {@render children()}
  {:else}
    <span>&gt;</span>
  {/if}
</li>
