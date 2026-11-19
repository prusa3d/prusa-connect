<!--
  One row of a listing: a name, a few values beside it, and one press that
  opens what the row is about.

  `@container/data-row` reads the row's own width and never the viewport's.
  The `@[40rem]` threshold was measured against a file browser row, which is
  about 236px wide on a 320px viewport and about 668px on a 768px one, so
  640px clears both. Those call sites set their own `min-h-18` floor from a
  56px leading slot. The bare `min-h-15` here is the floor a row meets where
  nothing in `leading` is that tall.

  `actions` stays a DOM sibling of the link or button and never its child. A
  row cannot be `<a><button></button></a>`. `row-link.ts` carries the overlay
  that stretches the primary control across the row, and the lift that keeps
  `leading` and `actions` above it. The tables that activate the same way
  share those three class strings.

  `isolate` keeps this row's own `z-10` children above the row's overlay and
  no higher. In the root stacking context they tie with the shell's sticky
  header and paint through it as the list scrolls past.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  import { ABOVE_ROW_LINK, ROW_LINK } from '../row-link/row-link';
  import Separator from '../separator/separator.svelte';
  import type { DataRowProps } from './types';

  let {
    primary,
    mono = false,
    href,
    onActivate,
    activateExpanded,
    leading,
    secondaryValues,
    wrapValues = false,
    roomy = false,
    actions,
    class: className,
    ...rest
  }: DataRowProps = $props();

  const values = $derived(
    (secondaryValues ?? []).filter(
      (value): value is string | Snippet =>
        value !== null && value !== undefined && value !== false,
    ),
  );

  const activatable = $derived(href !== undefined || onActivate !== undefined);

  const primaryClasses =
    'block w-full truncate text-left text-sm font-medium text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background';
</script>

<div
  {...rest}
  data-slot="data-row"
  class={[
    '@container/data-row relative isolate flex min-h-15 items-center gap-3 border-b border-border px-2 last:border-b-0',
    roomy ? 'py-4' : 'py-2',
    activatable &&
      'hover:bg-accent with-link-focus:bg-accent with-activate-focus:bg-accent',
    className,
  ]}
>
  {#if leading}
    <div class={[ABOVE_ROW_LINK, 'shrink-0']}>{@render leading()}</div>
  {/if}
  <div class="min-w-0 flex-1">
    {#if href}
      <a {href} class={[primaryClasses, ROW_LINK, mono && 'font-mono']}
        >{primary}</a
      >
    {:else if onActivate}
      <button
        type="button"
        data-slot="data-row-activate"
        onclick={onActivate}
        aria-expanded={activateExpanded}
        class={[
          primaryClasses,
          ROW_LINK,
          mono && 'font-mono',
          'cursor-pointer',
        ]}>{primary}</button
      >
    {:else}
      <div
        class={[
          'truncate text-sm font-medium text-foreground',
          mono && 'font-mono',
        ]}
      >
        {primary}
      </div>
    {/if}
    {#if values.length > 0}
      <div
        class={[
          'mt-1 flex items-center gap-2 font-mono text-xs text-muted-foreground',
          wrapValues && 'flex-wrap',
        ]}
      >
        {#each values as value, index (index)}
          <span
            class={[
              'items-center gap-2',
              // A flex item's min-width is auto, so a span cannot shrink and
              // truncate cannot clip it. Only the first value keeps shrink-0.
              index === 0 ? 'shrink-0' : 'min-w-0',
              index < 2 && 'inline-flex',
              index >= 2 &&
                (wrapValues
                  ? 'inline-flex'
                  : 'hidden @[40rem]/data-row:inline-flex'),
            ]}
          >
            {#if index > 0}
              <Separator orientation="vertical" class="h-3 shrink-0" />
            {/if}
            <!-- `[&>*]:max-w-full`: a value that is itself a control, a
                 printer link or a badge row, carries `white-space: nowrap`,
                 which makes its shrink-to-fit width its max-content width.
                 Without a cap it runs out of this box and is cut at the row's
                 edge with no ellipsis, under whatever sits there. Measured: a
                 221px link in a 181px slot, cut against the actions menu.
                 Capped, the child truncates inside its own box. -->
            <span class="min-w-0 truncate [&>*]:max-w-full">
              {#if typeof value === 'string'}{value}{:else}{@render value()}{/if}
            </span>
          </span>
        {/each}
      </div>
    {/if}
  </div>
  {#if actions}
    <div class={[ABOVE_ROW_LINK, 'shrink-0']}>{@render actions()}</div>
  {/if}
</div>
