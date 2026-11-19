<!-- The box that narrows a listing. Escape and the X empty it. -->
<script lang="ts">
  import XIcon from '@lucide/svelte/icons/x';

  import { t } from '../../i18n';
  import { Input } from '../input';
  import type { ListingSearchProps } from './types';

  let {
    value,
    onValue,
    label,
    placeholder,
    maxlength,
    commit = 'input',
    class: className,
  }: ListingSearchProps = $props();

  let host: HTMLElement | undefined = $state();
  // What the box holds, which runs ahead of `value` while a `change` commit
  // has not reported it yet.
  let text = $derived(value);

  const report = (event: Event & { currentTarget: HTMLInputElement }): void => {
    onValue(event.currentTarget.value);
  };

  const clear = (): void => {
    if (value === '' && text === '') return;
    text = '';
    host?.querySelector<HTMLInputElement>('input')?.focus();
    onValue('');
  };
</script>

<div class={['relative', className]} bind:this={host}>
  <!-- The platform paints its own clear button in a search box, in Chromium
       only; this one is the same in every browser and carries a name. -->
  <Input
    type="search"
    size="sm"
    class="pe-10 [&::-webkit-search-cancel-button]:appearance-none"
    value={text}
    {placeholder}
    {maxlength}
    aria-label={label}
    oninput={(event: Event & { currentTarget: HTMLInputElement }) => {
      text = event.currentTarget.value;
      if (commit === 'input') report(event);
    }}
    onchange={commit === 'change' ? report : undefined}
    onkeydown={(event: KeyboardEvent) => {
      if (event.key === 'Escape') clear();
    }}
  />
  {#if text !== ''}
    <!-- Smaller than the box and inset from its end by the box's own side
         padding, so the border and focus ring stay whole around it; the input
         reserves the room with `pe-10`. The dialog's close X wears the same
         quiet recipe; `Button` has no size below the box's own height. -->
    <button
      type="button"
      data-slot="listing-search-clear"
      aria-label={t('primitives.listing.clearSearch')}
      class="absolute inset-e-control-inline top-1/2 grid size-5 -translate-y-1/2 cursor-pointer place-items-center rounded-sm text-muted-foreground transition-colors outline-none hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background [&_svg]:pointer-events-none [&_svg]:size-3.5"
      onclick={clear}
    >
      <XIcon aria-hidden="true" />
    </button>
  {/if}
</div>
