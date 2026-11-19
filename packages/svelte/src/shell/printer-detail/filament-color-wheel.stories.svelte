<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { FILAMENT_COLORS } from '../printers/filament-colors';
  import FilamentColorWheel from './filament-color-wheel.svelte';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Printer Detail/Filament colour wheel',
    component: FilamentColorWheel,
  });
</script>

<Story name="Filament colours" asChild>
  <FilamentColorWheel
    colors={FILAMENT_COLORS}
    outerCount={12}
    selected={FILAMENT_COLORS[0]}
    onChoose={() => undefined}
  />
</Story>
