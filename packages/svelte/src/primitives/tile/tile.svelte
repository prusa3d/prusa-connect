<!--
  A framed card that opens or picks one thing: a file, a folder, a bed, a part.

  The frame is a `div`; the press element inside it (a link, a button, or the
  caller's own element through `child`) is stretched over the whole frame with
  ROW_LINK, so the tile has one tab stop and `top` controls sit above the hit
  area. The focus ring is drawn on the frame, where nothing clips it. Without
  `href`, `onclick` or `child` the tile is the frame alone.
-->
<script lang="ts">
  import { ABOVE_ROW_LINK, ROW_LINK } from '../row-link';
  import type { TilePressProps, TileProps } from './types';

  let {
    href,
    onclick,
    pressed,
    selected = false,
    disabled = false,
    off = false,
    size = 'md',
    radius = 'sm',
    quiet = false,
    label,
    top,
    child,
    children,
    class: className,
    ...rest
  }: TileProps = $props();

  const pressable = $derived(
    href !== undefined || onclick !== undefined || child !== undefined,
  );
  const picked = $derived(pressed === true || selected);

  // `data-slot` is what the frame's `with-press-focus` selectors find, so a
  // focused checkbox in `top` keeps its own ring and does not light the frame.
  const press: TilePressProps = $derived({
    'data-slot': 'tile-press',
    class: `${ROW_LINK} min-w-0 text-start outline-none disabled:cursor-default`,
    'aria-label': label,
    'aria-disabled': disabled || undefined,
  });
</script>

<div
  {...rest}
  data-slot="tile"
  class={[
    'relative isolate flex flex-col gap-2 border',
    radius === 'md' ? 'rounded-md' : 'rounded-sm',
    size === 'sm' && 'p-2',
    size === 'md' && 'p-3',
    size === 'list' && 'px-3 py-2',
    size === 'flush' && 'p-0',
    picked && 'border-primary bg-accent',
    !picked && off && 'border-dashed border-destructive',
    !picked && !off && !quiet && 'border-border',
    !picked && !off && quiet && 'border-border/60',
    pressable &&
      'with-press-focus:ring-2 with-press-focus:ring-ring with-press-focus:ring-offset-2 with-press-focus:ring-offset-background',
    pressable &&
      !disabled &&
      'cursor-pointer transition-colors motion-reduce:transition-none',
    pressable && !disabled && !quiet && 'hover:bg-accent',
    pressable && !disabled && quiet && 'hover:bg-foreground/5',
    disabled && 'text-disabled-foreground',
    className,
  ]}
>
  {#if top}
    <div class={ABOVE_ROW_LINK}>{@render top()}</div>
  {/if}
  {#if child}
    {@render child({ props: press })}
  {:else if href !== undefined && disabled}
    <a {...press} role="link" aria-disabled="true">{@render children?.()}</a>
  {:else if href !== undefined}
    <a {...press} {href}>{@render children?.()}</a>
  {:else if onclick !== undefined}
    <button
      {...press}
      type="button"
      aria-pressed={pressed}
      {disabled}
      {onclick}
    >
      {@render children?.()}
    </button>
  {:else}
    {@render children?.()}
  {/if}
</div>
