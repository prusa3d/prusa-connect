<!--
  What a listing's row can do, behind the one glyph every listing uses for it.

  The touch floor is decided here, once: `icon-lg`, which reaches 44px, this
  tree's size for a control operated with a gloved hand on a wall tablet, under
  a finger or in a narrow shell. Six listings had built this by hand and ended
  with three different hit boxes for one control. The items' own height
  belongs to the menu and is settled there, so no screen writes a floor of its
  own per menu.

  The accessible name is `Actions for {name}`. It is what a screen reader user
  hears once per row, and one listing calling it something else would make the
  whole table read as a different kind of thing.
-->
<script lang="ts">
  import EllipsisVerticalIcon from '@lucide/svelte/icons/ellipsis-vertical';

  import { t } from '../../i18n';
  import Button from '../button/button.svelte';
  import DropdownMenu from '../dropdown-menu/dropdown-menu.svelte';
  import DropdownMenuContent from '../dropdown-menu/dropdown-menu-content.svelte';
  import DropdownMenuTrigger from '../dropdown-menu/dropdown-menu-trigger.svelte';
  import type { RowActionsProps } from './types';

  let {
    name,
    children,
    class: className,
    onOpenChange,
  }: RowActionsProps = $props();

  const label = $derived(t('primitives.rowActions.label', { name }));
</script>

<DropdownMenu onOpenChange={(open: boolean) => onOpenChange?.(open)}>
  <DropdownMenuTrigger>
    {#snippet child({ props })}
      <Button
        {...props}
        variant="ghost"
        size="icon-lg"
        {label}
        class={className}
      >
        <EllipsisVerticalIcon aria-hidden="true" />
      </Button>
    {/snippet}
  </DropdownMenuTrigger>
  <DropdownMenuContent align="end">
    {@render children()}
  </DropdownMenuContent>
</DropdownMenu>
