<!--
  Pick values from a list inside a facet popover, one or several.

  One component with a `mode` rather than two, so a screen picks a behaviour
  and every select on every screen looks alike.

  - `multiple`, the default: a pick toggles one value and the panel stays open,
    so several can be picked in a row while the counts beside the remaining
    values update under the cursor.
  - `single`: a pick replaces the value and closes the panel, which is what
    every other select on the screen does. The All row clears the filter.

  `onCreate` turns the search box into a create field as well: type a name no
  option carries and one more row offers to make it. That is what keeps a
  screen from sending a reader to a second page to add the value they are
  already looking for.

  Zero-count options stay visible but dimmed, so the list does not jump around
  while you tick things. `FacetRow` owns that rule.

  `onToggle` hands back one value and the caller decides what the new selection
  is. The component holds no selection of its own, because the counts are
  computed from a selection the caller already owns.

  Deliberately unopinionated about its container. It is drawn inside a
  `FacetPopover` on a wide screen and inside a drawer section on a narrow one,
  so the scroll cap lives here and the chrome does not.
-->
<script lang="ts">
  import PlusIcon from '@lucide/svelte/icons/plus';
  import type { Attachment } from 'svelte/attachments';
  import type { FormEventHandler } from 'svelte/elements';
  import { on } from 'svelte/events';

  import { t } from '../../i18n';
  import { facetPopoverClose } from '../facet-popover/context';
  import FacetRow from '../facet-row/facet-row.svelte';
  import Input from '../input/input.svelte';
  import ScrollArea from '../scroll-area/scroll-area.svelte';
  import type {
    SelectFacetGroup,
    SelectFacetOption,
    SelectFacetProps,
  } from './types';

  let {
    options,
    selected,
    onToggle,
    groups,
    mode = 'multiple',
    allLabel,
    allCount,
    searchable,
    searchPlaceholder,
    searchAriaLabel,
    onCreate,
    createLabel,
  }: SelectFacetProps = $props();

  const closePopover = facetPopoverClose();

  let query = $state('');

  const onSearch: FormEventHandler<HTMLInputElement> = (event) => {
    query = event.currentTarget.value;
  };

  // Escape empties the box before it may close the panel. The listener is
  // attached by hand: a `onkeydown` attribute is delegated to the root, which
  // runs after the document-level listener that closes the panel.
  const escapeEmpties: Attachment<HTMLInputElement> = (box) =>
    on(box, 'keydown', (event) => {
      if (event.key === 'Escape' && query !== '') {
        query = '';
        event.stopPropagation();
      }
    });

  const allText = $derived(allLabel ?? t('primitives.selectFacet.all'));
  const placeholder = $derived(
    searchPlaceholder ?? t('primitives.selectFacet.search'),
  );
  const searchName = $derived(
    searchAriaLabel ?? t('primitives.selectFacet.searchLabel'),
  );
  const createVerb = $derived(
    createLabel ?? t('primitives.selectFacet.create'),
  );

  const needle = $derived(query.trim().toLowerCase());
  const single = $derived(mode === 'single');

  // A rich label has no words of its own to match, so a caller passes them.
  const searchTextOf = (option: SelectFacetOption): string =>
    option.searchText ?? (typeof option.label === 'string' ? option.label : '');

  // A flat list is one unnamed group, so there is a single render path below
  // rather than two that drift.
  const allGroups: SelectFacetGroup[] = $derived(
    groups ?? [
      {
        id: 'all',
        heading: '',
        options: options ?? [],
        selected: selected ?? [],
        onToggle: onToggle ?? (() => undefined),
      },
    ],
  );

  // Search narrows within each group, and a group left with nothing drops its
  // heading too, so the panel never shows a title over an empty space.
  const visibleGroups = $derived(
    needle === ''
      ? allGroups
      : allGroups
          .map((group) => ({
            ...group,
            options: group.options.filter((option) =>
              searchTextOf(option).toLowerCase().includes(needle),
            ),
          }))
          .filter((group) => group.options.length > 0),
  );

  const nothingMatches = $derived(visibleGroups.length === 0);

  // An exact name already in the list is picked, never made again. Two labels
  // called Office are two rows one lookup cannot tell apart.
  const creatable = $derived(
    onCreate !== undefined &&
      searchable === true &&
      needle !== '' &&
      !allGroups.some((group) =>
        group.options.some(
          (option) => searchTextOf(option).trim().toLowerCase() === needle,
        ),
      ),
  );

  // The All row counts only where the options carry counts of their own. A
  // lone number over a plain list of choices reads as a stray.
  const counted = $derived(
    allGroups.some((group) =>
      group.options.some((option) => option.count !== undefined),
    ),
  );

  const clear = (): void => {
    for (const group of allGroups) group.onToggle('');
    closePopover();
  };

  const create = (): void => {
    onCreate?.(query.trim());
    query = '';
    if (single) closePopover();
  };
</script>

<div>
  {#if searchable}
    <Input
      type="search"
      size="sm"
      value={query}
      oninput={onSearch}
      {@attach escapeEmpties}
      {placeholder}
      aria-label={searchName}
      class="mb-2"
    />
  {/if}
  <!-- A scroll area and not a bare `overflow-y-auto`: the browser's own
       scrollbar is painted by the platform and ignores the theme, so a dark
       panel got a light track. This tree's scrollbar paints its thumb in
       `muted-foreground`, which is defined on both sides of the theme.

       The cap goes on the viewport and not the root. The viewport is
       `size-full`, which against a root that only has a maximum height
       resolves to `auto`: the root then clips the overflow and nothing
       scrolls, putting the rows past 280px out of reach.

       The viewport wraps its children in a box of its own, and forcing that
       box to a full-width block is what lets a row be as wide as the panel and
       no wider. Nothing here ever scrolls sideways. -->
  <ScrollArea
    class={[
      '[&_[data-slot=scroll-area-viewport]]:max-h-70',
      '[&_[data-slot=scroll-area-viewport]>div]:!block',
      '[&_[data-slot=scroll-area-viewport]>div]:w-full',
    ]}
  >
    <!-- `single` only, and never inside a group: All clears the whole filter,
         so it belongs above the first heading rather than in one dimension. -->
    {#if single}
      <FacetRow
        count={counted ? allCount : undefined}
        checked={allGroups.every((group) => group.selected.length === 0)}
        onToggle={clear}
      >
        {#snippet label()}{allText}{/snippet}
      </FacetRow>
    {/if}
    {#each visibleGroups as group, index (group.id)}
      <div>
        {#if index > 0}
          <div class="my-3 border-t border-border"></div>
        {/if}
        {#if group.heading !== ''}
          <p
            class="mt-2.5 mb-1 text-xs font-bold tracking-facet-heading text-muted-foreground uppercase first:mt-0"
          >
            {group.heading}
          </p>
        {/if}
        {#each group.options as option (option.value)}
          <FacetRow
            count={option.count}
            disabled={option.disabled}
            hint={option.hint}
            checked={group.selected.includes(option.value)}
            onToggle={() => {
              group.onToggle(option.value);
              // A single pick is the whole interaction, and staying open would
              // make this behave unlike every other select.
              if (single) closePopover();
            }}
          >
            <!-- A plain string is the common label, and a snippet is what a
                 caller passes for a row that carries more than words. -->
            {#snippet label()}
              {#if typeof option.label === 'string'}{option.label}{:else}{@render option.label()}{/if}
            {/snippet}
          </FacetRow>
        {/each}
      </div>
    {/each}
    {#if searchable && nothingMatches && !creatable}
      <p class="px-1.5 py-2 text-xs text-muted-foreground">
        {t('primitives.selectFacet.noMatch', { query })}
      </p>
    {/if}
    <!-- Under the matches, never above them. The list is what a reader came
         for, and a create row on top would be the first thing the arrow keys
         reach on the way to an existing value. -->
    {#if creatable}
      <button
        type="button"
        data-slot="select-facet-create"
        onclick={create}
        class="mt-1 flex w-full cursor-pointer items-center gap-2 rounded-sm border-t border-border px-1.5 py-2 text-left text-xs outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
      >
        <PlusIcon
          class="size-4 flex-none text-muted-foreground"
          aria-hidden="true"
        />
        <span class="min-w-0 flex-1 break-words">
          {t('primitives.selectFacet.createNamed', {
            verb: createVerb,
            name: query.trim(),
          })}
        </span>
      </button>
    {/if}
  </ScrollArea>
</div>
