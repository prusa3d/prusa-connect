<!--
  The row a person presses to open the section. Upstream shadcn ships it
  unstyled; here it is a full row, because that is how every screen in this
  product uses it.

  `variant` sets the row: `default` is bare, `row` is bordered, `filled` is a
  control-height row on the secondary fill, `inline` is borderless. The
  start-aligned ones lead with a chevron that turns a quarter when the section
  is open; `chevron` overrides that either way.

  `group/collapsible-trigger` names the row so a caller's own icon can turn with
  the state: `group-state-open/collapsible-trigger:rotate-180`.
-->
<script lang="ts">
  import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
  import { Collapsible as CollapsiblePrimitive } from 'bits-ui';

  import type { CollapsibleTriggerProps } from './types';

  let {
    ref = $bindable(null),
    variant = 'default',
    tone = 'default',
    chevron = variant !== 'default',
    class: className,
    children,
    ...rest
  }: CollapsibleTriggerProps = $props();
</script>

<CollapsiblePrimitive.Trigger
  bind:ref
  data-slot="collapsible-trigger"
  class={[
    "group/collapsible-trigger flex w-full items-center gap-2 text-sm font-semibold transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    tone === 'default' && 'text-muted-foreground hover:text-foreground',
    variant === 'default' && 'justify-between',
    variant === 'row' &&
      'min-h-11 justify-start rounded-sm border border-border px-3 py-2 text-start hover:bg-accent',
    variant === 'filled' &&
      'min-h-control-height justify-start rounded-sm bg-secondary px-control-inline py-2 text-start hover:bg-accent',
    variant === 'inline' &&
      'min-h-11 justify-start rounded-sm px-1 text-start hover:bg-accent',
    tone === 'warning' && 'text-warning-text hover:text-warning-text',
    className,
  ]}
  {...rest}
>
  {#if chevron}
    <ChevronRightIcon
      aria-hidden="true"
      class={[
        tone === 'warning' ? 'text-warning-text' : 'text-muted-foreground',
        'transition-transform group-state-open/collapsible-trigger:rotate-90 motion-reduce:transition-none',
      ]}
    />
  {/if}
  {@render children?.()}
</CollapsiblePrimitive.Trigger>
