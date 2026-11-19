<!--
  One pickable value. With a count it is a facet: a value that would return
  nothing stays visible and dimmed rather than disappearing, because a list
  that reshuffles as you tick things is impossible to aim at. Without one it is
  a plain choice.

  A button with `aria-pressed`, not a checkbox. The whole row is the target and
  the box is a drawing that follows the row's own state.

  `cursor-pointer` with `disabled:cursor-default`: Tailwind v4's Preflight
  dropped the old `button { cursor: pointer }` reset, so every bare button in
  this tree restates it.

  The checked box takes `bg-selected`, the fill this theme gives every chosen
  control.
-->
<script lang="ts">
  import CheckIcon from '@lucide/svelte/icons/check';

  import { formatNumber } from '../../i18n';
  import type { FacetRowProps } from './types';

  let {
    label,
    count,
    checked,
    onToggle,
    disabled,
    hint,
    class: className,
  }: FacetRowProps = $props();

  // A zero that is already ticked stays clickable, otherwise the only way out
  // of an empty result would be the Clear all button. A caller's own
  // `disabled` is unconditional: it means "this cannot be picked", not "this
  // would return nothing".
  const dead = $derived(disabled === true || (count === 0 && !checked));
</script>

<button
  type="button"
  data-slot="facet-row"
  data-on={checked || undefined}
  disabled={dead}
  aria-pressed={checked}
  onclick={onToggle}
  class={[
    'flex w-full cursor-pointer items-center rounded-md px-1.5 py-1 text-left outline-none disabled:cursor-default',
    'hover:not-disabled:bg-accent',
    'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
    'disabled:pointer-events-none disabled:opacity-35',
    'min-h-control-height gap-4 @max-3xl/app-shell:min-h-touch-target pointer-coarse:min-h-touch-target',
    className,
  ]}
>
  <!-- A square box, matched to the radius the rest of the interface uses. -->
  <span
    aria-hidden="true"
    data-slot="facet-row-indicator"
    data-on={checked || undefined}
    class={[
      'grid flex-none place-content-center place-items-center border bg-card text-transparent',
      'size-checkbox rounded-checkbox border-input data-on:border-checkbox-on data-on:bg-checkbox-on data-on:text-control-on-foreground',
    ]}
  >
    <CheckIcon class="size-checkbox-mark stroke-2" strokeWidth={3} />
  </span>
  <!-- Wraps rather than truncating: a value that does not fit stays readable on
       a second line, and the panel never scrolls sideways to reach it.
       `min-w-0` so the flex item may shrink to the panel's width before it
       wraps. -->
  <span
    class={['min-w-0 flex-1 text-xs break-words', checked && 'font-semibold']}
  >
    <!-- A plain hyphen between the label and the hint. writing-style.md
         refuses an em dash anywhere, and the React source writes one here. The
         separator is an expression because Svelte drops the whitespace at the
         start of an element. -->
    {@render label()}{#if hint}<span class="text-muted-foreground"
        >{` - ${hint}`}</span
      >{/if}
  </span>
  <!-- `tabular-nums` gives every digit the same width, so a column of counts
       does not jitter as the numbers change. -->
  {#if count != null}
    <span class="text-xs text-muted-foreground tabular-nums">
      {formatNumber(count)}
    </span>
  {/if}
</button>
