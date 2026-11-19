<!--
  The title row: a title, an optional description, and an optional action.

  The action shares the title's line only where there is room for both. Below
  24rem of header, which is a card on a 320px phone, a two-column split left
  the title a 40px column and wrapped `API tokens` one letter per line, so
  there the action drops to its own row underneath (see `card-action.svelte`).
-->
<script lang="ts">
  import type { CardHeaderProps } from './types';

  let { class: className, children, ...rest }: CardHeaderProps = $props();
</script>

<div
  data-slot="card-header"
  class={[
    '@container/card-header grid auto-rows-min grid-rows-card-header items-start gap-2 px-4 @sm/card:px-6 @sm/card-header:has-slot-card-action:grid-cols-card-header-action [.border-b]:pb-6',
    className,
  ]}
  {...rest}
>
  {@render children?.()}
</div>
