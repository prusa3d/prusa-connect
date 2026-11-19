<!--
  A header cell.

  `scope` says which cells this one heads. It is `col` by default, which is
  what a cell in the header row is, and a header that names a row passes
  `scope="row"`. Without it a screen reader has to guess the association.

  `h-9` is the shared control height. A button, an input, a select and the tabs
  list are all that tall, so a control dropped into a header cell lines up with
  the column name beside it.

  A header holding a tick box heads a selection column, which has nothing to
  widen, and its padding-free end puts the box under the resize handle. It
  draws no handle.
-->
<script lang="ts">
  import { t } from '../../i18n';
  import type { TableHeadProps } from './types';

  let {
    scope = 'col',
    class: className,
    children,
    ...rest
  }: TableHeadProps = $props();

  const labelId = $props.id();
  const canResize = $derived(scope === 'col');
</script>

<th
  data-slot="table-head"
  {scope}
  class={[
    'h-9 px-2 align-middle text-xs font-semibold whitespace-nowrap text-muted-foreground [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-checkbox-baseline [:where(&)]:text-start',
    className,
    '[&:not(.sr-only)]:relative',
  ]}
  aria-labelledby={canResize && !rest['aria-label'] ? labelId : undefined}
  {...rest}
>
  {#if canResize}
    <span
      id={labelId}
      class="contents [&>[role=checkbox]]:translate-y-checkbox-baseline"
    >
      {@render children?.()}
    </span>
    <button
      type="button"
      data-slot="table-column-resize"
      aria-label={t('table.resizeColumn')}
      aria-describedby={labelId}
      aria-keyshortcuts="ArrowLeft ArrowRight Enter"
      title={t('table.resizeColumn')}
      class="absolute inset-e-0 inset-bs-1 inset-be-1 z-10 w-3 cursor-col-resize touch-none rounded-sm outline-none select-none before:absolute before:inset-y-0 before:inset-e-0 before:w-px before:bg-border hover:before:bg-control-on focus-visible:ring-2 focus-visible:ring-ring active:before:bg-control-on [.sr-only>&]:hidden [:has([role=checkbox])>&]:hidden"
    ></button>
  {:else}
    {@render children?.()}
  {/if}
</th>
