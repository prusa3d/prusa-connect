<!--
  Flows its rows into as many columns as its own width has room for, and stacks
  a lone row.

  A row is a label at one edge and a reading at the other, so its readability is
  a question of measure and not of count. At 1440 a two-row stack puts the label
  and the reading 500px apart with nothing between them, which is a card that
  looks empty while carrying every reading the printer sends. Two 250px columns
  put each reading beside its own label and halve the block's height.

  The columns are `--grid-template-columns-heater-rows` in the theme.
  `auto-fit`, not `auto-fill`: `auto-fill` creates as many tracks as fit
  whatever the content, and a lone row in a 1200px grid is shrunk instead of
  spanning. The `min(.., 100%)` around the floor is load-bearing, because CSS
  Grid overflows a bare `minmax` floor instead of shrinking it. The floor,
  `--container-heater-row` (228px), is the heater row's own: the measured
  break-even is 222px, an icon plus a tool label plus a four-letter material
  plus "Off" plus a three-digit reading plus the 44px editor trigger with the
  gaps, and the floor carries headroom over it. Past that the material line is
  what gives up width, because it is the one piece of content that can lose
  information without losing meaning.
-->
<script lang="ts">
  import type { HeaterRowsProps } from './types';

  let { rows, class: className, children, ...rest }: HeaterRowsProps = $props();

  const COLUMNS_FROM_ROWS = 2;
</script>

<div
  {...rest}
  data-slot="heater-rows"
  class={[
    'grid items-start gap-2',
    rows !== undefined && rows < COLUMNS_FROM_ROWS
      ? 'grid-cols-1'
      : 'grid-cols-heater-rows',
    className,
  ]}
>
  {@render children()}
</div>
