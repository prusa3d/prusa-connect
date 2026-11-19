<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import Button from '../button/button.svelte';
  import Input from '../input/input.svelte';
  import SelectContent from './select-content.svelte';
  import SelectGroup from './select-group.svelte';
  import SelectItem from './select-item.svelte';
  import SelectLabel from './select-label.svelte';
  import SelectSeparator from './select-separator.svelte';
  import SelectTrigger from './select-trigger.svelte';
  import SelectValue from './select-value.svelte';
  import Select from './select.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Select',
    component: Select,
  });

  const models = [
    { value: 'mk4s', label: 'MK4S' },
    { value: 'mini', label: 'MINI+' },
    { value: 'xl', label: 'XL' },
    { value: 'sl1s', label: 'SL1S SPEED' },
  ];

  const mk4sOnly = [{ value: 'mk4s', label: 'MK4S' }];
</script>

{#snippet printerModel()}
  <Select value="mk4s" items={models}>
    <SelectTrigger aria-label="Printer model" class="w-48">
      <SelectValue placeholder="Select a printer" />
    </SelectTrigger>
    <SelectContent>
      <SelectGroup>
        <SelectLabel>FDM</SelectLabel>
        <SelectItem value="mk4s" label="MK4S">MK4S</SelectItem>
        <SelectItem value="mini" label="MINI+">MINI+</SelectItem>
        <SelectItem value="xl" label="XL">XL</SelectItem>
      </SelectGroup>
      <SelectSeparator />
      <SelectGroup>
        <SelectLabel>Resin</SelectLabel>
        <SelectItem value="sl1s" label="SL1S SPEED">SL1S SPEED</SelectItem>
      </SelectGroup>
    </SelectContent>
  </Select>
{/snippet}

<Story name="Default" asChild children={printerModel} />

<Story name="All Sizes" asChild>
  <div class="flex items-center gap-3">
    <Select value="mk4s" items={mk4sOnly}>
      <SelectTrigger size="sm" aria-label="Printer model, small" class="w-40">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="mk4s" label="MK4S">MK4S</SelectItem>
      </SelectContent>
    </Select>
    <Select value="mk4s" items={mk4sOnly}>
      <SelectTrigger aria-label="Printer model, medium" class="w-40">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="mk4s" label="MK4S">MK4S</SelectItem>
      </SelectContent>
    </Select>
  </div>
</Story>

<!-- The info fill, as the people listing marks an administrator's role. -->
<Story name="Info Tone" asChild>
  <Select value="mk4s" items={mk4sOnly}>
    <SelectTrigger
      size="sm"
      tone="info"
      aria-label="Printer model"
      class="w-40"
    >
      <SelectValue />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="mk4s" label="MK4S">MK4S</SelectItem>
    </SelectContent>
  </Select>
</Story>

<Story name="Shares A Baseline With Input And Button" asChild>
  <div class="flex items-center gap-2">
    <Select value="mk4s" items={mk4sOnly}>
      <SelectTrigger aria-label="Printer model" class="w-40">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="mk4s" label="MK4S">MK4S</SelectItem>
      </SelectContent>
    </Select>
    <Input placeholder="Job name" class="w-40" />
    <Button>Print</Button>
  </div>
</Story>

<Story
  name="Opens By Keyboard Arrow Keys Move Highlight Enter Selects"
  tags={['!dev', '!autodocs']}
  asChild
  children={printerModel}
/>

<Story
  name="Escape Closes And Returns Focus To The Trigger"
  tags={['!dev', '!autodocs']}
  asChild
  children={printerModel}
/>

<Story
  name="Disabled Trigger Is Not Reachable By Keyboard"
  tags={['!dev', '!autodocs']}
  asChild
>
  <Select value="mk4s" items={mk4sOnly} disabled>
    <SelectTrigger aria-label="Printer model" class="w-48">
      <SelectValue />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="mk4s" label="MK4S">MK4S</SelectItem>
    </SelectContent>
  </Select>
</Story>
