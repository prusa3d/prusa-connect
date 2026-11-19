<!-- Drawn via the child snippet, not by bits-ui directly: bits-ui never
     writes its popper id to the DOM, so its aria-describedby has no match. -->
<script lang="ts">
  import { Tooltip as TooltipPrimitive } from 'bits-ui';

  import type { TooltipContentProps } from './types';

  const uid = $props.id();

  let {
    ref = $bindable(null),
    class: className,
    sideOffset = 0,
    id = `tooltip-${uid}`,
    children,
    ...rest
  }: TooltipContentProps = $props();
</script>

<TooltipPrimitive.Portal>
  <TooltipPrimitive.Content
    bind:ref
    {sideOffset}
    style="--radix-tooltip-content-transform-origin: var(--bits-tooltip-content-transform-origin);"
    {...rest}
  >
    <!-- wrapperProps is what floating-ui positions, so it renders alongside props. -->
    {#snippet child({ props, wrapperProps })}
      <div {...wrapperProps}>
        <div
          {...props}
          {id}
          role="tooltip"
          data-slot="tooltip-content"
          class={[
            'z-50 w-fit origin-(--radix-tooltip-content-transform-origin) rounded-sm bg-foreground px-3 py-1.5 text-xs text-balance text-background state-open:animate-scale-in state-closed:animate-scale-out',
            className,
          ]}
        >
          {@render children?.()}
          <!-- bits-ui's own triangle, coloured via currentColor: its inline
               transform-origin fits only that shape, and !important is forbidden. -->
          <TooltipPrimitive.Arrow class="z-50 text-foreground" />
        </div>
      </div>
    {/snippet}
  </TooltipPrimitive.Content>
</TooltipPrimitive.Portal>
