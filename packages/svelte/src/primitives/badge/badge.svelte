<!--
  A short label beside the thing it describes: a printer's state, a count, a
  tag.

  A badge is a span, so it says nothing to a screen reader beyond its text. A
  caller's own attributes come last, so a call site can set its own `data-slot`
  or override a data attribute this component sets.
-->
<script lang="ts">
  import type { BadgeProps } from './types';
  import {
    base,
    isState,
    mono as monoClass,
    numeric as numericClass,
    sizes,
    variants,
  } from './variants';

  let {
    variant = 'default',
    size = 'sm',
    mono = isState(variant),
    numeric = false,
    class: className,
    child,
    children,
    ...rest
  }: BadgeProps = $props();

  const attrs = $derived({
    'data-slot': 'badge',
    'data-variant': variant,
    class: [
      base,
      variants[variant],
      sizes[size],
      mono && monoClass,
      numeric && numericClass,
      className,
    ],
    ...rest,
  });
</script>

{#if child}
  {@render child({ props: attrs })}
{:else}
  <span {...attrs}>
    {@render children?.()}
  </span>
{/if}
