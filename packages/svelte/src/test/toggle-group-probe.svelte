<!-- A three-option toggle group, for the toggle-group tests. `type` picks
     which arm of the primitive's union is rendered, because bits-ui takes the
     selection as a string for one option and an array for several. -->
<script lang="ts">
  import type {
    ToggleSize,
    ToggleTone,
    ToggleVariant,
  } from '../primitives/toggle';
  import { ToggleGroup, ToggleGroupItem } from '../primitives/toggle-group';

  let {
    type = 'single',
    variant = 'default',
    tone = 'default',
    size = 'md',
    disabled,
  }: {
    type?: 'single' | 'multiple';
    variant?: ToggleVariant;
    tone?: ToggleTone;
    size?: ToggleSize;
    /** The one option that is dead, by value. */
    disabled?: string;
  } = $props();

  const options = ['left', 'center', 'right'];
</script>

{#if type === 'single'}
  <ToggleGroup
    type="single"
    value="left"
    aria-label="Text alignment"
    {variant}
    {tone}
    {size}
  >
    {#each options as option (option)}
      <ToggleGroupItem value={option} disabled={option === disabled}>
        {option}
      </ToggleGroupItem>
    {/each}
  </ToggleGroup>
{:else}
  <ToggleGroup
    type="multiple"
    value={['left']}
    aria-label="Text formatting"
    {variant}
    {tone}
    {size}
  >
    {#each options as option (option)}
      <ToggleGroupItem value={option} disabled={option === disabled}>
        {option}
      </ToggleGroupItem>
    {/each}
  </ToggleGroup>
{/if}
