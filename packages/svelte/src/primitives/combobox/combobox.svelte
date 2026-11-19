<!--
  A text field over a list of choices: it lists them as soon as it has the
  focus and narrows them to what is typed, case-insensitively. With `onCreate`
  a name no item has offers a Create row; with `noneLabel` a row clears the
  pick.

  Enter after typing, with no row walked to by the arrows, takes the text as
  the answer: the item of that exact name, or else a new one. bits-ui would
  pick the highlighted row instead, which is the first partial match, so this
  handler runs first and stops it. Once the arrows have walked, Enter picks the
  walked row as bits-ui does.

  The field always shows the picked item's label; typed text lives only until
  the list shuts, so a blur or Escape gives the label back. bits-ui keeps its
  own copy of the text, and the `value` written after its props is what
  overrides it.

  The list is the Select's surface and rows. bits-ui publishes the anchor's
  measurements under `combobox` names, so the style maps them onto the names
  the Select surface reads. Escape inside a dialog shuts only the list, because
  bits-ui answers it in the top layer alone.
-->
<script lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
  import { Combobox as ComboboxPrimitive } from 'bits-ui';

  import {
    CONTROL_FILL,
    CONTROL_RING,
    CONTROL_SIZES,
    CONTROL_TRIGGER,
  } from '../control-trigger/control-trigger';
  import SelectContent from '../select/select-content.svelte';
  import SelectItem from '../select/select-item.svelte';
  import type { ComboboxProps } from './types';

  let {
    items,
    value,
    onPick,
    label,
    placeholder,
    size = 'md',
    onCreate,
    createLabel,
    noneLabel,
    id,
    'aria-describedby': describedby,
    'aria-invalid': invalid,
    class: className,
  }: ComboboxProps = $props();

  // Row values for the two rows that are not items. A NUL never reaches an
  // item's value from a form or a server.
  const NONE = '\u0000none';
  const CREATE = '\u0000create';

  let open = $state(false);
  /** What is typed since the list opened; undefined shows the pick. */
  let typed = $state<string | undefined>(undefined);
  let walked = $state(false);

  const picked = $derived(items.find((item) => item.value === value));
  const needle = $derived((typed ?? '').trim());
  const matches = $derived(
    items.filter((item) =>
      item.label.toLowerCase().includes(needle.toLowerCase()),
    ),
  );
  const exact = $derived(
    items.find((item) => item.label.toLowerCase() === needle.toLowerCase()),
  );
  const showNone = $derived(
    noneLabel !== undefined && value !== undefined && needle === '',
  );
  const showCreate = $derived(
    onCreate !== undefined && needle !== '' && exact === undefined,
  );
  const rowCount = $derived(
    matches.length + (showNone ? 1 : 0) + (showCreate ? 1 : 0),
  );

  const shut = (): void => {
    open = false;
    typed = undefined;
    walked = false;
  };

  const take = (next: string): void => {
    if (next === NONE) onPick(undefined);
    else if (next === CREATE) void onCreate?.(needle);
    else if (next !== '') onPick(next);
    shut();
  };

  const onkeydown = (event: KeyboardEvent): void => {
    if (
      ['ArrowDown', 'ArrowUp', 'Home', 'End', 'PageDown', 'PageUp'].includes(
        event.key,
      )
    ) {
      walked = true;
    } else if (event.key === 'Enter' && typed !== undefined && !walked) {
      event.preventDefault();
      if (needle === '') take(noneLabel === undefined ? '' : NONE);
      else if (exact) take(exact.value);
      else take(onCreate ? CREATE : '');
    }
  };
</script>

<ComboboxPrimitive.Root
  type="single"
  allowDeselect={false}
  bind:value={() => value ?? '', take}
  bind:open={
    () => open && rowCount > 0, (next) => (next ? (open = true) : shut())
  }
>
  <div class="relative">
    <ComboboxPrimitive.Input
      aria-label={label}
      {placeholder}
      autocomplete="off"
      onfocus={() => (open = true)}
      onclick={() => (open = true)}
      onblur={shut}
      oninput={(event) => {
        open = true;
        walked = false;
        typed = event.currentTarget.value;
      }}
      {onkeydown}
    >
      {#snippet child({ props })}
        <input
          {...props}
          {...id ? { id } : {}}
          aria-describedby={describedby}
          aria-invalid={invalid}
          value={typed ?? picked?.label ?? ''}
          data-slot="combobox-input"
          data-size={size}
          class={[
            CONTROL_TRIGGER,
            CONTROL_FILL,
            CONTROL_RING,
            CONTROL_SIZES[size],
            'w-full cursor-text placeholder:text-muted-foreground',
            className,
          ]}
        />
      {/snippet}
    </ComboboxPrimitive.Input>
    <ChevronDownIcon
      aria-hidden="true"
      class="pointer-events-none absolute top-1/2 right-2 size-4 -translate-y-1/2 text-muted-foreground"
    />
  </div>
  <SelectContent
    aria-label={label}
    style="--radix-select-content-available-height: var(--bits-combobox-content-available-height); --radix-select-content-transform-origin: var(--bits-combobox-content-transform-origin); --radix-select-trigger-height: var(--bits-combobox-anchor-height); --radix-select-trigger-width: var(--bits-combobox-anchor-width);"
  >
    {#if showNone && noneLabel !== undefined}
      <SelectItem value={NONE} label={noneLabel} class="text-muted-foreground">
        {noneLabel}
      </SelectItem>
    {/if}
    {#each matches as item (item.value)}
      <SelectItem value={item.value} label={item.label}>{item.label}</SelectItem
      >
    {/each}
    {#if showCreate}
      <SelectItem value={CREATE} label={needle} class="text-muted-foreground">
        {createLabel ? createLabel(needle) : needle}
      </SelectItem>
    {/if}
  </SelectContent>
</ComboboxPrimitive.Root>
