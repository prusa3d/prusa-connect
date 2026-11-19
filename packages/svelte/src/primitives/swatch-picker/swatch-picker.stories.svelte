<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import SwatchPicker from './swatch-picker.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Swatch Picker',
    component: SwatchPicker,
    args: {
      swatches: [
        { key: 'black', hex: '#1A1A1A', label: 'Black' },
        { key: 'white', hex: '#F5F5F5', label: 'White' },
        { key: 'red', hex: '#D32F2F', label: 'Red' },
        { key: 'blue', hex: '#1976D2', label: 'Blue' },
        { key: 'green', hex: '#388E3C', label: 'Green' },
      ],
      value: undefined,
      onPick: () => undefined,
      label: 'Colour',
    },
  });
</script>

<Story name="Nothing Picked" />

<Story name="Picked" args={{ value: 'red' }} />

<Story name="Deselectable" args={{ value: 'blue', deselectable: true }} />

<Story name="Medium" args={{ value: 'green', size: 'md' }} />

<Story name="Small Round" args={{ value: 'red', size: 'xs' }} />
