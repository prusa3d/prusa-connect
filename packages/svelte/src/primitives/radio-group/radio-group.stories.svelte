<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import RadioGroup from './radio-group.svelte';
  import RadioGroupItem from './radio-group-item.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/RadioGroup',
    component: RadioGroup,
  });
</script>

<Story name="Default" asChild>
  <RadioGroup value="pla" aria-label="Filament type">
    <div class="flex items-center gap-2">
      <RadioGroupItem value="pla" id="default-pla" />
      <label for="default-pla">PLA</label>
    </div>
  </RadioGroup>
</Story>

<Story name="Section Gap" asChild>
  <RadioGroup value="pla" gap="section" aria-label="Filament type">
    <div class="flex items-center gap-2">
      <RadioGroupItem value="pla" id="section-pla" />
      <label for="section-pla">PLA</label>
    </div>
    <div class="flex items-center gap-2">
      <RadioGroupItem value="petg" id="section-petg" />
      <label for="section-petg">PETG</label>
    </div>
  </RadioGroup>
</Story>

<Story name="All States" asChild>
  <div class="flex flex-wrap gap-8">
    <RadioGroup value="pla" aria-label="Filament type">
      <div class="flex items-center gap-2">
        <RadioGroupItem value="pla" id="all-pla" />
        <label for="all-pla">PLA (checked)</label>
      </div>
      <div class="flex items-center gap-2">
        <RadioGroupItem value="petg" id="all-petg" />
        <label for="all-petg">PETG (unchecked)</label>
      </div>
      <div class="flex items-center gap-2">
        <RadioGroupItem value="abs" id="all-abs" disabled />
        <label for="all-abs">ABS (disabled)</label>
      </div>
    </RadioGroup>
    <RadioGroup value="asa" aria-label="Locked filament type">
      <div class="flex items-center gap-2">
        <RadioGroupItem value="asa" id="all-asa" disabled />
        <label for="all-asa">ASA (disabled and checked)</label>
      </div>
    </RadioGroup>
  </div>
</Story>

<Story
  name="Arrow Keys Move The Selection And Skip Disabled"
  tags={['!dev', '!autodocs']}
  asChild
>
  <RadioGroup value="pla" aria-label="Filament type">
    <div class="flex items-center gap-2">
      <RadioGroupItem value="pla" id="arrow-pla" />
      <label for="arrow-pla">PLA</label>
    </div>
    <div class="flex items-center gap-2">
      <RadioGroupItem value="petg" id="arrow-petg" disabled />
      <label for="arrow-petg">PETG</label>
    </div>
    <div class="flex items-center gap-2">
      <RadioGroupItem value="abs" id="arrow-abs" />
      <label for="arrow-abs">ABS</label>
    </div>
  </RadioGroup>
</Story>
