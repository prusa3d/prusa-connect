<!--
  Gives whatever the story draws the demo filament preview and the favourite
  filaments. Nothing is wrapped around the story.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  import { provideFilamentPreview } from '../../packages/svelte/src/shell/printers/filament-colors';
  import {
    provideFilamentFavoritesPreview,
    type FavoriteFilament,
  } from '../../packages/svelte/src/shell/printers/filament-preferences';
  import { demoFilamentPreview } from './fleet';

  interface Props {
    children?: Snippet;
  }

  const { children }: Props = $props();

  provideFilamentPreview(demoFilamentPreview);

  let favorites = $state.raw<readonly FavoriteFilament[]>([
    {
      id: 'demo-pla-yellow',
      name: 'Yellow PLA',
      material: 'PLA',
      brand: 'Prusament',
      color: 'yellow',
    },
    {
      id: 'demo-petg-orange',
      name: 'Orange PETG',
      material: 'PETG',
      brand: 'Prusament',
      color: 'orange',
    },
    {
      id: 'demo-asa-gray',
      name: 'Gray ASA',
      material: 'ASA',
      brand: 'Prusament',
      color: 'gray',
    },
    {
      id: 'demo-pla-purple',
      name: 'Purple PLA',
      material: 'PLA',
      brand: '',
      color: 'purple',
    },
    {
      id: 'demo-flex-teal',
      name: 'Teal FLEX',
      material: 'FLEX',
      brand: '',
      color: 'teal',
    },
  ]);
  provideFilamentFavoritesPreview({
    read: () => favorites,
    save: (next) => {
      favorites = next;
    },
  });
</script>

{@render children?.()}
