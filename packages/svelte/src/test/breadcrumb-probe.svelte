<!--
  A whole trail, for the test to render. The parts only work together, and a
  test cannot write markup of its own.

  Every word comes in as a prop, so the test says what it asserts on.
-->
<script lang="ts">
  import BreadcrumbEllipsis from '../primitives/breadcrumb/breadcrumb-ellipsis.svelte';
  import BreadcrumbItem from '../primitives/breadcrumb/breadcrumb-item.svelte';
  import BreadcrumbLink from '../primitives/breadcrumb/breadcrumb-link.svelte';
  import BreadcrumbList from '../primitives/breadcrumb/breadcrumb-list.svelte';
  import BreadcrumbPage from '../primitives/breadcrumb/breadcrumb-page.svelte';
  import BreadcrumbSeparator from '../primitives/breadcrumb/breadcrumb-separator.svelte';
  import Breadcrumb from '../primitives/breadcrumb/breadcrumb.svelte';

  interface Props {
    label: string;
    more: string;
    links: { href: string; label: string }[];
    current: string;
    /** Renders the links through the `child` snippet, as buttons. */
    asButton?: boolean;
  }

  let { label, more, links, current, asButton = false }: Props = $props();
</script>

<Breadcrumb {label}>
  <BreadcrumbList>
    <BreadcrumbItem>
      <BreadcrumbEllipsis moreText={more} />
    </BreadcrumbItem>
    <BreadcrumbSeparator />
    {#each links as link (link.href)}
      <BreadcrumbItem>
        {#if asButton}
          <BreadcrumbLink>
            {#snippet child({ props })}
              <button type="button" {...props}>{link.label}</button>
            {/snippet}
          </BreadcrumbLink>
        {:else}
          <BreadcrumbLink href={link.href}>{link.label}</BreadcrumbLink>
        {/if}
      </BreadcrumbItem>
      <BreadcrumbSeparator />
    {/each}
    <BreadcrumbItem>
      <BreadcrumbPage>{current}</BreadcrumbPage>
    </BreadcrumbItem>
  </BreadcrumbList>
</Breadcrumb>
