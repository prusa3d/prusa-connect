<!--
  A row's action menu with the two items a listing usually offers. The items
  are children, and a test cannot pass a snippet holding components without a
  component of its own.
-->
<script lang="ts">
  import DropdownMenuItem from '../primitives/dropdown-menu/dropdown-menu-item.svelte';
  import RowActions from '../primitives/row-actions/row-actions.svelte';

  let { onPick }: { onPick?: (item: string) => void } = $props();
</script>

<RowActions name="Bracket_Left_Reinforced_v4_FINAL.gcode">
  <DropdownMenuItem onSelect={() => onPick?.('again')}>
    Print again
  </DropdownMenuItem>
  <DropdownMenuItem variant="destructive" onSelect={() => onPick?.('delete')}>
    Delete
  </DropdownMenuItem>
</RowActions>
