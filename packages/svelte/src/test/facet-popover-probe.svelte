<!--
  A facet whose panel holds a button that shuts it. The close function reaches
  the list through context, and a test cannot write a component of its own to
  read it.
-->
<script lang="ts">
  import FacetPopover from '../primitives/facet-popover/facet-popover.svelte';
  import ClosingRow from './facet-popover-close-row.svelte';
  import type { FacetPopoverProps } from '../primitives/facet-popover/types';

  let { ...rest }: Omit<FacetPopoverProps, 'children'> = $props();
</script>

<FacetPopover {...rest}>
  <ClosingRow />
</FacetPopover>
