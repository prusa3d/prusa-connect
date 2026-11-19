<!--
  Where a page sits, and the way back out of it. One control, one place, on
  every screen that is not a destination of its own.

  The trail is the page's first element, above the heading, and never in the
  shell's top bar. The narrow tier has no top bar, so a trail placed there
  would disappear where a person most needs the way back.

  It keeps the breadcrumb's own landmark name. A file browser's folder trail
  answers a different question, where the reader is inside a drive, and names
  itself. A printer's Files tab draws both, and two landmarks with one name
  are indistinguishable (axe `landmark-unique`).

  A listing takes no trail. Printers, Jobs, Storage, Labels and
  Settings are top-level destinations, the rail already marks which one is
  current, and a one-crumb trail above a heading that says the same word is
  furniture.
-->
<script lang="ts">
  import BreadcrumbItem from '../breadcrumb/breadcrumb-item.svelte';
  import BreadcrumbLink from '../breadcrumb/breadcrumb-link.svelte';
  import BreadcrumbList from '../breadcrumb/breadcrumb-list.svelte';
  import BreadcrumbPage from '../breadcrumb/breadcrumb-page.svelte';
  import BreadcrumbSeparator from '../breadcrumb/breadcrumb-separator.svelte';
  import Breadcrumb from '../breadcrumb/breadcrumb.svelte';
  import type { PageTrailProps } from './types';

  let { trail, page, sansPage = false }: PageTrailProps = $props();
</script>

<Breadcrumb>
  <BreadcrumbList>
    {#each trail as crumb (crumb.href ?? crumb.label)}
      <BreadcrumbItem>
        {#if crumb.onSelect}
          <BreadcrumbLink>
            {#snippet child({ props })}
              <button type="button" {...props} onclick={crumb.onSelect}>
                {crumb.label}
              </button>
            {/snippet}
          </BreadcrumbLink>
        {:else}
          <BreadcrumbLink href={crumb.href}>{crumb.label}</BreadcrumbLink>
        {/if}
      </BreadcrumbItem>
      <!-- The separator is its own `<li>`, a sibling of the crumbs rather
           than a child of one. -->
      <BreadcrumbSeparator />
    {/each}
    <BreadcrumbItem>
      <BreadcrumbPage class={sansPage ? 'font-sans' : undefined}
        >{page}</BreadcrumbPage
      >
    </BreadcrumbItem>
  </BreadcrumbList>
</Breadcrumb>
