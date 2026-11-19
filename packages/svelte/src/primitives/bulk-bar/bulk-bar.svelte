<!--
  The row over a listing that lets a reader tick rows.

  It swaps rather than appends: what a reader does to a selection and what they
  do to the whole listing are different jobs, and showing both at once doubles
  the row's width on a narrow pane. The height is fixed either way, so the
  listing under it does not jump as the first row is ticked.
-->
<script lang="ts">
  import { Button } from '../button';
  import { Separator } from '../separator';
  import type { BulkBarProps } from './types';

  let {
    count,
    label,
    selectAllLabel,
    onSelectAll,
    clearLabel,
    onClear,
    actions,
    idle,
  }: BulkBarProps = $props();

  const offersAll = $derived(
    selectAllLabel !== undefined && onSelectAll !== undefined,
  );
</script>

<div
  class={[
    'flex h-auto min-h-input-height items-center gap-3',
    count > 0 &&
      'overflow-x-auto rounded-md border border-border bg-accent px-3',
  ]}
>
  {#if count > 0}
    <p class="shrink-0 text-xs font-medium">{label}</p>
    <Separator orientation="vertical" class="h-4 shrink-0" />
    {@render actions?.()}
    {#if offersAll}
      <Button
        variant="ghost"
        size="sm"
        class="ms-auto shrink-0"
        onclick={() => {
          onSelectAll?.();
        }}
      >
        {selectAllLabel}
      </Button>
    {/if}
    <Button
      variant="ghost"
      size="sm"
      class={['shrink-0', !offersAll && 'ms-auto']}
      onclick={() => {
        onClear();
      }}
    >
      {clearLabel}
    </Button>
  {:else}
    {@render idle?.()}
  {/if}
</div>
