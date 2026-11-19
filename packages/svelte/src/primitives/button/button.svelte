<!--
  The button.

  A caller's own attributes come last, so a call site can set its own
  `data-slot` or override a data attribute this component sets.

  `type` defaults to `button`, so a button inside a form does not submit it by
  accident. The `child` snippet is the exception: it receives the caller's
  `type` as it was given, because the element it draws may not take one at all.

  A square button has a `label`. It is the accessible name, and a tooltip
  shows it on hover and on keyboard focus. The button brings its own tooltip
  provider, because bits-ui throws for a tooltip with no provider above it and
  the app has no provider of its own. The tooltip's trigger attributes are
  merged under the caller's, so a menu or dialog trigger that spreads its own
  props on the button keeps its handlers, its ARIA and its `data-state`.

  While the menu or dialog the button opens is showing (`aria-expanded`), the
  tooltip stays shut. It would cover the menu, and bits-ui gives every open
  tooltip a focus scope of its own: one that opens over a menu pauses the
  menu's scope, and focus never moves onto the first item.
-->
<script lang="ts">
  import { mergeProps } from 'bits-ui';

  import { Spinner } from '../spinner';
  import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
  } from '../tooltip';
  import type { ButtonProps, ButtonTextSize } from './types';
  import {
    base,
    iconLinkBase,
    linkBase,
    linkSize,
    radii,
    sizes,
    variants,
  } from './variants';

  let {
    variant = 'default',
    size = 'md',
    radius = 'sm',
    type,
    label,
    pending = false,
    class: className,
    child,
    children,
    ...rest
  }: ButtonProps = $props();

  const attrs = $derived({
    'data-slot': 'button',
    'data-variant': variant,
    'data-size': size,
    type,
    'aria-label': label,
    class: [
      variant !== 'link' && base,
      variant !== 'link' && radii[radius],
      variant === 'link' && size !== 'inline-icon' && linkBase,
      variant === 'link' && size === 'inline-icon' && iconLinkBase,
      variants[variant],
      variant === 'link' ? linkSize : sizes[size as ButtonTextSize],
      className,
    ],
    ...rest,
    ...(pending && { 'aria-busy': true, disabled: true }),
  });

  const expanded = $derived(String(rest['aria-expanded']) === 'true');

  // bits-ui points `aria-describedby` at the open tooltip. Here the tooltip
  // repeats the accessible name, so a screen reader would read it twice.
  // `data-disabled` says only that the tooltip is shut while expanded; on the
  // button it would read as a disabled control. The trigger's props arrive
  // untyped from the bits-ui snippet.
  const withTooltip = (trigger: unknown): Record<string, unknown> => {
    const own = { ...(trigger as Record<string, unknown>) };
    delete own['aria-describedby'];
    delete own['data-disabled'];
    return { ...mergeProps(own, attrs), type };
  };
</script>

{#snippet control(props: Record<string, unknown>)}
  {#if child}
    {@render child({ props })}
  {:else}
    <button
      {...props}
      type={type ?? 'button'}
      class={[
        variant !== 'link' && base,
        variant !== 'link' && radii[radius],
        variant === 'link' && size !== 'inline-icon' && linkBase,
        variant === 'link' && size === 'inline-icon' && iconLinkBase,
        variants[variant],
        variant === 'link' ? linkSize : sizes[size as ButtonTextSize],
        className,
      ]}
    >
      {#if pending}<Spinner />{/if}
      {@render children?.()}
    </button>
  {/if}
{/snippet}

{#if label === undefined}
  {@render control(attrs)}
{:else}
  <TooltipProvider>
    <Tooltip disabled={expanded}>
      <TooltipTrigger>
        {#snippet child({ props })}
          {@render control(withTooltip(props))}
        {/snippet}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  </TooltipProvider>
{/if}
