<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import ToggleGroup from './toggle-group.svelte';
  import ToggleGroupItem from './toggle-group-item.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/ToggleGroup',
    component: ToggleGroup,
  });
</script>

<!-- No `states` tag: the roving-focus group puts a tab stop on the active item
     only, so a single-item group resolves to two focusable matches. -->
<Story name="Default" asChild>
  <ToggleGroup type="single" value="left" aria-label="Text alignment">
    <ToggleGroupItem value="left">Left</ToggleGroupItem>
    <ToggleGroupItem value="center">Center</ToggleGroupItem>
    <ToggleGroupItem value="right">Right</ToggleGroupItem>
  </ToggleGroup>
</Story>

<Story name="All Variants And Sizes" asChild>
  <div class="flex flex-col items-start gap-4">
    <ToggleGroup type="multiple" value={['bold']} aria-label="Text formatting">
      <ToggleGroupItem value="bold">Bold</ToggleGroupItem>
      <ToggleGroupItem value="italic">Italic</ToggleGroupItem>
      <ToggleGroupItem value="underline">Underline</ToggleGroupItem>
    </ToggleGroup>
    <ToggleGroup
      type="multiple"
      variant="outline"
      value={['bold']}
      aria-label="Text formatting, outline"
    >
      <ToggleGroupItem value="bold">Bold</ToggleGroupItem>
      <ToggleGroupItem value="italic">Italic</ToggleGroupItem>
      <ToggleGroupItem value="underline">Underline</ToggleGroupItem>
    </ToggleGroup>
    <ToggleGroup
      type="single"
      size="sm"
      variant="outline"
      value="left"
      aria-label="Text alignment, small"
    >
      <ToggleGroupItem value="left">Left</ToggleGroupItem>
      <ToggleGroupItem value="center">Center</ToggleGroupItem>
    </ToggleGroup>
    <ToggleGroup
      type="single"
      size="lg"
      variant="outline"
      value="left"
      aria-label="Text alignment, large"
    >
      <ToggleGroupItem value="left">Left</ToggleGroupItem>
      <ToggleGroupItem value="center">Center</ToggleGroupItem>
    </ToggleGroup>
  </div>
</Story>

<!-- `primary` is for a choice that sets the machine, such as a fan mode.
     The default tone is for one that only changes what the page shows. -->
<Story name="Primary Tone" asChild>
  <ToggleGroup
    type="single"
    variant="outline"
    tone="primary"
    value="auto"
    aria-label="Chamber fan"
  >
    <ToggleGroupItem value="off">Off</ToggleGroupItem>
    <ToggleGroupItem value="auto">Auto</ToggleGroupItem>
    <ToggleGroupItem value="manual">Manual</ToggleGroupItem>
  </ToggleGroup>
</Story>

<Story
  name="Arrow Keys Move Focus And Space Selects For Single Type"
  tags={['!dev', '!autodocs']}
  asChild
>
  <ToggleGroup type="single" value="left" aria-label="Text alignment">
    <ToggleGroupItem value="left">Left</ToggleGroupItem>
    <ToggleGroupItem value="center">Center</ToggleGroupItem>
    <ToggleGroupItem value="right">Right</ToggleGroupItem>
  </ToggleGroup>
</Story>

<Story
  name="Space Toggles Each Item Independently For Multiple Type"
  tags={['!dev', '!autodocs']}
  asChild
>
  <ToggleGroup type="multiple" value={['bold']} aria-label="Text formatting">
    <ToggleGroupItem value="bold">Bold</ToggleGroupItem>
    <ToggleGroupItem value="italic">Italic</ToggleGroupItem>
  </ToggleGroup>
</Story>
