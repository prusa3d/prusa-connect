<!--
  One listing, two ways to look at it.

  Every listing carries two tiers, a table where there is room for columns and
  a stacked row per item where there is not, and the container's width alone
  used to choose between them. This hands that choice to the reader: on a
  phone a table is unreadable, but on a tablet held sideways it is exactly
  what a person came for, and no width threshold can know which of the two
  they want.

  The chosen view is filled off `aria-checked` rather than off the toggle's
  own `data-state`. Every item is also a tooltip trigger, and the item's
  `aria-checked` is the one attribute nothing else writes to.

  `unavailableReason` rather than a hidden button: a view that needs more room
  than the window has stays visible and says so, or the reader learns nothing
  from its absence. It is `aria-disabled` and not `disabled`, because a
  disabled control drops out of the tab order and takes its own tooltip with
  it, which puts the reason out of reach of the reader most likely to need it.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import ToggleGroup from '../toggle-group/toggle-group.svelte';
  import ToggleGroupItem from '../toggle-group/toggle-group-item.svelte';
  import Tooltip from '../tooltip/tooltip.svelte';
  import TooltipContent from '../tooltip/tooltip-content.svelte';
  import TooltipProvider from '../tooltip/tooltip-provider.svelte';
  import TooltipTrigger from '../tooltip/tooltip-trigger.svelte';
  import type { ViewModeToggleProps } from './types';

  let {
    value,
    onValueChange,
    options,
    label,
    class: className,
  }: ViewModeToggleProps = $props();

  const groupLabel = $derived(label ?? t('primitives.viewMode.label'));
</script>

<!-- One provider for the group: bits-ui shares a single open-delay timer
     through it, so moving along the buttons swaps tooltips instead of waiting
     again. -->
<TooltipProvider delayDuration={300}>
  <ToggleGroup
    type="single"
    variant="outline"
    {value}
    onValueChange={(next: string) => {
      // A single-select group lets its own active item be deselected, which
      // would leave the listing with no view at all. An empty value is the one
      // update this ignores.
      if (next) onValueChange(next);
    }}
    aria-label={groupLabel}
    class={className}
  >
    {#each options as option (option.value)}
      {@const Icon = option.icon}
      <Tooltip>
        <TooltipTrigger>
          {#snippet child({ props })}
            <ToggleGroupItem
              {...props}
              size="icon"
              value={option.value}
              aria-label={option.label}
              aria-disabled={option.unavailableReason === undefined
                ? undefined
                : true}
              onclick={(event: MouseEvent) => {
                // `aria-disabled` is a claim, not a behaviour. The primitive
                // fires its own value change unless the click is stopped here.
                if (option.unavailableReason !== undefined) {
                  event.preventDefault();
                }
              }}
              class={[
                '[&_svg]:size-icon-min',
                'aria-checked:border-selected aria-checked:bg-selected aria-checked:text-selected-foreground aria-checked:hover:border-selected-hovered aria-checked:hover:bg-selected-hovered',
                option.unavailableReason !== undefined &&
                  'cursor-not-allowed text-disabled-foreground hover:bg-transparent hover:text-disabled-foreground',
              ]}
            >
              {#if option.iconSrc}
                <img src={option.iconSrc} alt="" class="invert dark:invert-0" />
              {:else}
                <Icon aria-hidden="true" />
              {/if}
            </ToggleGroupItem>
          {/snippet}
        </TooltipTrigger>
        <TooltipContent>
          {option.unavailableReason ?? option.label}
        </TooltipContent>
      </Tooltip>
    {/each}
  </ToggleGroup>
</TooltipProvider>
