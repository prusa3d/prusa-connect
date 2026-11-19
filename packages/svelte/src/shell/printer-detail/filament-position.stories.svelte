<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import FilamentPosition from './filament-position.svelte';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Printer Detail/Filament position',
    component: FilamentPosition,
  });
</script>

<Story name="Reported material">
  <FilamentPosition
    deviceId="story-printer"
    toolId="0"
    position="Extruder"
    reported="PLA"
  />
</Story>
