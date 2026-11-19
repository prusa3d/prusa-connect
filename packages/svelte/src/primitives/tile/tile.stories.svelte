<script module lang="ts">
  import BoxIcon from '@lucide/svelte/icons/box';
  import FolderIcon from '@lucide/svelte/icons/folder';
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Checkbox from '../checkbox/checkbox.svelte';
  import Tile from './tile.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Tile',
    component: Tile,
  });

  const noop = () => undefined;
</script>

{#snippet bed(name: string, note: string)}
  <span class="flex items-center gap-2">
    <span
      class="flex size-10 shrink-0 items-center justify-center rounded-xs bg-muted"
    >
      <BoxIcon aria-hidden="true" class="size-5 text-muted-foreground" />
    </span>
    <span class="grid min-w-0">
      <span class="truncate text-sm font-medium">{name}</span>
      <span class="truncate text-xs text-muted-foreground">{note}</span>
    </span>
  </span>
{/snippet}

{#snippet picture(name: string, facts: string)}
  <span class="flex flex-col gap-2">
    <span
      class="flex aspect-square w-full items-center justify-center rounded-sm bg-surface"
    >
      <BoxIcon aria-hidden="true" class="size-12 text-muted-foreground" />
    </span>
    <span class="truncate text-xs">{name}</span>
    <span class="text-xs text-muted-foreground">{facts}</span>
  </span>
{/snippet}

<Story name="Default" asChild>
  <Tile class="w-60" onclick={noop}>
    {@render bed('Bed 1', '4 objects')}
  </Tile>
</Story>

<Story name="Link" asChild>
  <Tile class="w-60" href="#prints">
    <span class="flex items-center gap-2 text-sm">
      <FolderIcon aria-hidden="true" class="size-5 text-muted-foreground" />
      prints
    </span>
  </Tile>
</Story>

<Story name="Pressed" asChild>
  <Tile class="w-60" onclick={noop} pressed>
    {@render bed('Bed 1', '4 objects')}
  </Tile>
</Story>

<Story name="Selected With Checkbox Top" asChild>
  <Tile class="w-44" onclick={noop} selected>
    {#snippet top()}
      <Checkbox checked aria-label="Select benchy.bgcode" />
    {/snippet}
    {@render picture('benchy.bgcode', '1 h 12 min · 14 g')}
  </Tile>
</Story>

<Story name="Disabled" asChild>
  <Tile class="w-60" onclick={noop} disabled>
    {@render bed('Bed 2', 'Nothing to print')}
  </Tile>
</Story>

<Story name="Off" asChild>
  <Tile class="w-60" onclick={noop} off>
    {@render bed('Bed 3', 'Switched off')}
  </Tile>
</Story>

<Story name="Small" asChild>
  <Tile class="w-60" onclick={noop} size="sm">
    {@render bed('Bed 1', '4 objects')}
  </Tile>
</Story>

<Story name="Picture Column" asChild>
  <div class="grid w-96 grid-cols-2 gap-3">
    <Tile onclick={noop}>
      {@render picture('benchy.bgcode', '1 h 12 min · 14 g')}
    </Tile>
    <Tile onclick={noop}>
      {@render picture('calibration-cube.bgcode', '22 min · 4 g')}
    </Tile>
  </div>
</Story>

<Story name="Row" asChild>
  <div class="flex w-max gap-2">
    <Tile onclick={noop} pressed>{@render bed('Bed 1', '4 objects')}</Tile>
    <Tile onclick={noop}>{@render bed('Bed 2', '2 objects')}</Tile>
    <Tile onclick={noop} off>{@render bed('Bed 3', 'Switched off')}</Tile>
  </div>
</Story>

<Story name="Frame Only" asChild>
  <Tile class="w-44">
    {#snippet top()}
      <Checkbox aria-label="Select benchy.bgcode" />
    {/snippet}
    {@render picture('benchy.bgcode', '1 h 12 min · 14 g')}
  </Tile>
</Story>

<Story name="Custom Press Element" asChild>
  <Tile class="w-60">
    {#snippet child({ props })}
      <a {...props} href="#bracket">{@render bed('Bracket', 'v3 · 2 files')}</a>
    {/snippet}
  </Tile>
</Story>

<!-- One of a list of tiles: the list padding, a fainter border and hover. -->
<Story name="Quiet List" asChild>
  <Tile class="w-60" href="#bed-1" size="list" quiet>
    {@render bed('Bed 1', '4 objects')}
  </Tile>
</Story>

<!-- A tile whose content draws to its edges, with the larger corner. -->
<Story name="Flush With A Medium Corner" asChild>
  <Tile class="w-60 overflow-hidden" onclick={noop} size="flush" radius="md">
    {@render bed('Bed 1', '4 objects')}
  </Tile>
</Story>
