<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Slider from './slider.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Slider',
    component: Slider,
    args: {
      type: 'single',
      value: 50,
      class: 'w-64',
      'aria-label': 'Volume',
    },
  });
</script>

<Story name="Default" />

<Story
  name="Two Thumb Range"
  args={{ type: 'multiple', value: [25, 75], 'aria-label': 'Price range' }}
/>

<Story name="Disabled" args={{ disabled: true }} />

<!-- `primary` is for a value that sets the machine, such as a print speed. -->
<Story name="Primary Tone" args={{ tone: 'primary', 'aria-label': 'Speed' }} />

<Story
  name="Arrow Keys Move The Thumb And Aria Valuenow Follows"
  tags={['!dev', '!autodocs']}
/>

<Story name="Home And End Reach The Bounds" tags={['!dev', '!autodocs']} />
