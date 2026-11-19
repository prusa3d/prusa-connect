<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import TilePanel from './tile-panel.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/TilePanel',
    component: TilePanel,
    args: { title: 'Printers', children: body },
  });
</script>

{#snippet body()}<p class="text-sm">Body</p>{/snippet}
{#snippet more()}<span class="text-xs">Open</span>{/snippet}

<Story name="Default" />

<Story name="With Subline" args={{ subline: 'Fleet, last 30 days' }} />

<Story name="With Trailing" args={{ trailing: more }} />

<Story name="Fallback Title" args={{ title: '  ', fallback: 'Clock' }} />

<Story name="No Heading" args={{ title: '' }} />

<Story name="Medium Gap" args={{ gap: 'md' }} />
