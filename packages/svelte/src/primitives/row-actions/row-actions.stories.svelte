<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import DropdownMenuItem from '../dropdown-menu/dropdown-menu-item.svelte';
  import RowActions from './row-actions.svelte';

  const { Story } = defineMeta({
    title: 'Primitives/Row Actions',
    component: RowActions,
  });
</script>

{#snippet fileActions()}
  <RowActions name="Bracket_Left_Reinforced_v4_FINAL.gcode">
    <DropdownMenuItem>Print again</DropdownMenuItem>
    <DropdownMenuItem variant="destructive">Delete</DropdownMenuItem>
  </RowActions>
{/snippet}

<Story name="What A Row Can Do" asChild children={fileActions} />

<!--
  Under a coarse pointer the trigger and every item clear this tree's 44px
  floor.

  Read off the computed style rather than the bounding rectangle: the test
  runner renders a story in a scaled frame, so a rectangle returns 41.8px for a
  box whose height is 44, and the thing under test is the rule.
-->
<Story
  name="Everything Clears The Touch Floor Under A Finger"
  tags={['!dev', '!autodocs']}
  asChild
  children={fileActions}
/>
