<script lang="ts">
  import { Tabs as TabsPrimitive } from 'bits-ui';

  import type { TabsTriggerProps } from './types';

  let {
    ref = $bindable(null),
    class: className,
    ...rest
  }: TabsTriggerProps = $props();
</script>

<!-- `NavTabs` keeps a copy of this look keyed off `aria-current`. The bottom border is the only mark of the active tab. A trigger sizes to
     its own content rather than sharing the row equally, so the row reads as
     labels and not as segments. The disabled and active colours need the
     compound `disabled:state-active:*` selector: a plain `disabled:*`
     ties with `state-active:*` on specificity. -->
<TabsPrimitive.Trigger
  bind:ref
  data-slot="tabs-trigger"
  class={[
    "inline-flex items-center justify-center gap-2 rounded-t-sm border-b-2 border-transparent px-1 pb-2 text-sm font-medium tracking-wider whitespace-nowrap text-muted-foreground uppercase transition-colors outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:text-disabled-foreground state-active:border-selected state-active:text-foreground disabled:state-active:border-transparent disabled:state-active:text-disabled-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
    'enabled:state-active:border-control-on',
    className,
  ]}
  {...rest}
/>
