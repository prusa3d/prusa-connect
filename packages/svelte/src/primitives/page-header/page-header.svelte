<!--
  What a screen opens with: its name, and the act it leads with.
-->
<script lang="ts">
  import Button from '../button/button.svelte';
  import type { PageHeaderProps } from './types';

  let { title, action, actions, class: className }: PageHeaderProps = $props();
</script>

<div
  data-slot="page-header"
  class={['flex flex-wrap items-center justify-between gap-3', className]}
>
  <h1 class="text-lg font-semibold">{title}</h1>

  {#if actions ?? action}
    <div
      data-slot="page-header-actions"
      class="flex min-h-button-medium items-center gap-2"
    >
      {#if actions}
        {@render actions()}
      {:else if action}
        <Button variant={action.variant ?? 'default'} onclick={action.onclick}>
          {#if action.icon}
            {@const Glyph = action.icon}
            <Glyph aria-hidden="true" />
          {/if}
          {action.label}
        </Button>
      {/if}
    </div>
  {/if}
</div>
