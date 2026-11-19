<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import RowList from './row-list.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/RowList',
    component: RowList,
    args: { children: rows },
  });
</script>

{#snippet rows()}
  {#each ['Original Prusa MK4', 'Prusa XL', 'Prusa MINI'] as name (name)}
    <div class="border-b border-border px-4 py-3 text-sm last:border-b-0">
      {name}
    </div>
  {/each}
{/snippet}

<Story name="Default" />

<Story name="Single Row" args={{ children: one }} />

{#snippet one()}
  <div class="px-4 py-3 text-sm">Original Prusa MK4</div>
{/snippet}

<Story name="Medium Corner" args={{ radius: 'md' }} />
