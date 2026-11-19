<!--
  A facet: a button that opens a list of values.

  Deliberately not shaped like the toggles beside it. It is outlined and it
  carries a chevron, because it opens something rather than switching
  something. Once values are picked it says which, so nobody has to reopen a
  panel to remember what the list is filtered by. That is the whole reason this
  exists rather than an icon-only trigger: a slider glyph with a number beside
  it names neither the dimension nor the choice.

  Its metrics are this tree's. The input, the select's trigger and the toggle
  share one scale, and a facet standing in a row beside a search box and a
  select has to be on it, or the row's controls end at three different heights.
  The text size tracks the height for the same reason.

  `swatches` is for a colour facet. A colour cannot be summarised in words, so
  the trigger carries the picked swatches. `active` then forces the on state
  that `summary` would otherwise decide.
-->
<script lang="ts">
  import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';

  import { formatNumber, t } from '../../i18n';
  import {
    CONTROL_FILL,
    CONTROL_RING,
    CONTROL_SIZES,
    CONTROL_TRIGGER,
  } from '../control-trigger/control-trigger';
  import Popover from '../popover/popover.svelte';
  import PopoverContent from '../popover/popover-content.svelte';
  import PopoverTrigger from '../popover/popover-trigger.svelte';
  import { provideFacetPopoverClose } from './context';
  import type { FacetPopoverProps } from './types';
  import { FACET_TRIGGER } from './variants';

  let {
    label,
    size = 'md',
    summary,
    count,
    active,
    swatches,
    width,
    class: className,
    children,
  }: FacetPopoverProps = $props();

  let open = $state(false);

  provideFacetPopoverClose(() => {
    open = false;
  });

  const on = $derived(active ?? Boolean(summary));

  // A summary that already carries a plus has counted the rest itself.
  const overflow = $derived(
    count !== undefined && count > 1 && summary && !summary.includes('+')
      ? count - 1
      : 0,
  );
</script>

<Popover bind:open>
  <PopoverTrigger>
    {#snippet child({ props })}
      <button
        {...props}
        type="button"
        data-slot="facet-popover-trigger"
        data-on={on ? true : undefined}
        class={[
          CONTROL_TRIGGER,
          CONTROL_FILL,
          CONTROL_RING,
          CONTROL_SIZES[size],
          FACET_TRIGGER,
          className,
        ]}
      >
        {#if swatches && swatches.length > 0}
          <span class="flex flex-none gap-0.5" aria-hidden="true">
            {#each swatches as swatch, index (`${swatch}-${String(index)}`)}
              <span
                class="size-3 rounded-xs ring-1 ring-border ring-inset"
                style="background: {swatch};"
              ></span>
            {/each}
          </span>
        {/if}
        <!-- The dimension and the value are told apart by weight, not by
             opacity. A dimmed prefix over this tree's primary fill measures
             3.04:1 and fails WCAG AA. Weight costs no contrast. -->
        <span class="flex-1 truncate text-left font-normal">
          {#if !summary}{label}{:else}{label}:
            <b class="font-semibold">{summary}</b
            >{#if overflow > 0}{` +${formatNumber(overflow)}`}{/if}{/if}
        </span>
        <ChevronDownIcon class="size-6 flex-none" aria-hidden="true" />
      </button>
    {/snippet}
  </PopoverTrigger>
  <!-- bits-ui puts no role on the surface, so nothing needs naming there the
       way Radix's dialog role does. The label stays for a reader who reaches
       the panel by another route: what opened is the list of values for this
       facet. -->
  <PopoverContent
    aria-label={t('primitives.facetPopover.options', { label })}
    sideOffset={6}
    align="start"
    collisionPadding={12}
    class="p-2"
    style="width: {width}px;"
  >
    {@render children()}
  </PopoverContent>
</Popover>
