<!--
  A listing's whole toolbar: what it is narrowed to, and how it is drawn.

  One layout for every listing, in one order: the search, the facets, the
  filter toggles, `afterFacets`, Clear filters, then `beforeControls` (the
  sort) pushed to the end of its line. Those wrap among themselves: the search
  grows to fill the first line and the facets carry on under it once they no
  longer fit beside it. `trailing` and the field picker and view switch sit
  apart from them, at the end of the first line, and drop to a line of their
  own only when they would leave the filters too narrow to use.
-->
<script lang="ts">
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
  import { Toggle as TogglePrimitive } from 'bits-ui';

  import { t } from '../../i18n';
  import Button from '../button/button.svelte';
  import {
    Collapsible,
    CollapsibleContent,
    CollapsibleTrigger,
  } from '../collapsible';
  import {
    CONTROL_FILL,
    CONTROL_RING,
    CONTROL_SIZES,
    CONTROL_TRIGGER,
  } from '../control-trigger/control-trigger';
  import FacetPopover from '../facet-popover/facet-popover.svelte';
  import { facetListSummary } from '../facet-popover/summary';
  import { CONTROL_ON } from '../facet-popover/variants';
  import ListingControls from '../listing-controls/listing-controls.svelte';
  import type { FacetDimension } from '../listing-state/types';
  import SelectFacet from '../select-facet/select-facet.svelte';
  import { clearFacets } from './clear-facets';
  import type { ListingToolbarProps } from './types';

  /** Past this many values the list gets a search box. */
  const SEARCHABLE_ABOVE = 8;

  const PANEL_WIDTH = 272;

  let {
    search,
    afterFacets,
    beforeControls,
    trailing,
    facets = [],
    filtersActive,
    onClearFilters,
    toggles = [],
    fields,
    fieldState,
    view,
    class: className,
  }: ListingToolbarProps = $props();

  const wordFor = (facet: FacetDimension, value: string): string =>
    facet.labelFor?.(value) ?? value;

  const optionsOf = (facet: FacetDimension) =>
    facet.values.map((value) => ({
      value,
      label: wordFor(facet, value),
      count: facet.counts?.get(value),
    }));

  const pick = (facet: FacetDimension, value: string): void => {
    if (facet.mode === 'single') {
      facet.onSelectedChange(
        value === '' || facet.selected.includes(value) ? [] : [value],
      );
      return;
    }
    facet.onSelectedChange(
      facet.selected.includes(value)
        ? facet.selected.filter((held) => held !== value)
        : [...facet.selected, value],
    );
  };

  const activeCount = $derived(
    facets.reduce((sum, facet) => sum + facet.selected.length, 0) +
      toggles.filter((flag) => flag.pressed).length,
  );

  const narrowed = $derived(
    filtersActive ??
      (facets.some((facet) => facet.selected.length > 0) ||
        toggles.some((flag) => flag.pressed)),
  );

  const clearFilters = (): void => {
    if (onClearFilters) {
      onClearFilters();
      return;
    }
    clearFacets(facets);
    for (const flag of toggles) {
      if (flag.pressed) flag.onPressedChange(false);
    }
  };
</script>

{#snippet facetControls()}
  {#each facets as facet (facet.id)}
    <FacetPopover
      label={facet.label}
      size="sm"
      width={PANEL_WIDTH}
      swatches={facet.swatches}
      summary={facetListSummary(
        facet.selected.map((value) => wordFor(facet, value)),
      )}
    >
      {#if facet.panel}
        {@render facet.panel()}
      {:else}
        <SelectFacet
          options={optionsOf(facet)}
          selected={[...facet.selected]}
          onToggle={(value: string) => {
            pick(facet, value);
          }}
          mode={facet.mode}
          allLabel={facet.allLabel}
          searchable={facet.searchable ??
            facet.values.length > SEARCHABLE_ABOVE}
        />
      {/if}
    </FacetPopover>
  {/each}
  {#each toggles as flag (flag.id)}
    <!-- A filter in the facets' row, so it wears their box and their picked
         fill rather than the toggle's own look. -->
    <TogglePrimitive.Root
      data-slot="listing-toggle"
      data-on={flag.pressed ? true : undefined}
      class={[
        CONTROL_TRIGGER,
        CONTROL_FILL,
        CONTROL_RING,
        CONTROL_SIZES.sm,
        CONTROL_ON,
      ]}
      pressed={flag.pressed}
      onPressedChange={flag.onPressedChange}
    >
      {@render flag.icon?.()}
      {flag.label}
    </TogglePrimitive.Root>
  {/each}
{/snippet}

<!-- `items-start`: when the filters run onto further lines, `trailing` and
     the view switch stay level with the first one. -->
<div
  data-slot="listing-toolbar"
  class={['@container/toolbar flex flex-wrap items-start gap-2', className]}
>
  <!-- The basis is room for one facet or picker. As a size container the
       side has no content minimum, so with a search it takes the search's
       own: narrower than that, the other side wraps under this one rather
       than the search running under it. -->
  <div
    class={[
      '@container/listing flex grow basis-24 flex-wrap items-center gap-2',
      search && 'min-w-40 @max-sm/toolbar:basis-full',
    ]}
  >
    {#if search}
      <!-- A block, so the search's own box (which holds its clear button)
           takes the whole width this gives it. -->
      <div
        class="max-w-sm min-w-40 grow basis-48 @max-sm/toolbar:max-w-none @max-sm/toolbar:basis-full"
      >
        {@render search()}
      </div>
    {/if}
    <div class="contents @max-md/listing:hidden">
      {@render facetControls()}
    </div>
    {#if facets.length > 0 || toggles.length > 0}
      <Collapsible class="hidden @max-md/listing:block">
        <CollapsibleTrigger
          class="min-h-touch-target gap-2 rounded-sm border border-input px-control-inline"
        >
          {t('primitives.listing.filters', { count: activeCount })}
          <ChevronRightIcon
            aria-hidden="true"
            class="size-4 group-aria-expanded/collapsible-trigger:rotate-90"
          />
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div class="flex flex-wrap gap-2 pt-2">{@render facetControls()}</div>
        </CollapsibleContent>
      </Collapsible>
    {/if}
    {@render afterFacets?.()}
    {#if narrowed}
      <Button variant="ghost" size="sm" onclick={clearFilters}>
        {t('primitives.listing.clearFilters')}
      </Button>
    {/if}
    {#if beforeControls}
      <div
        data-slot="listing-toolbar-actions"
        class="ms-auto flex max-w-full flex-wrap items-center gap-2"
      >
        {@render beforeControls()}
      </div>
    {/if}
  </div>

  {#if trailing !== undefined || view !== undefined}
    <div
      data-slot="listing-toolbar-actions"
      class="ms-auto flex shrink-0 items-center gap-2"
    >
      {@render trailing?.()}
      {#if view}
        <ListingControls {fields} {fieldState} {view} />
      {/if}
    </div>
  {/if}
</div>
