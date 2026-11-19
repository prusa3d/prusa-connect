<!--
  The Connect logo: with `shape="full"` the simple version, the word Connect
  alone, and with `shape="mark"` the orange mark for a slot with no width. The
  default `shape="prusa"` is the PRUSA wordmark, which every screen draws until
  design ships a new Connect logo.

  Drawn, unlike `Logo`, because the word is cut by the designer and is not a
  face the app ships. The mark keeps its orange on every ground, so it reads
  `--color-logo`, which no skin or theme redeclares. The word inherits
  `currentColor`, so it is white on a dark ground and near black on a light
  one, the two versions the designer exported, with no colour prop. The PRUSA
  wordmark follows `currentColor` the same way.
-->
<script lang="ts">
  import { MARK, PRUSA, WORDMARK } from './artwork';
  import type { ConnectLogoProps } from './types';
  import { base, sizes, viewBoxes } from './variants';

  let {
    shape = 'prusa',
    size = 'md',
    label = 'Connect',
    class: className,
  }: ConnectLogoProps = $props();

  const decorative = $derived(label === '');
</script>

<svg
  viewBox={viewBoxes[shape]}
  xmlns="http://www.w3.org/2000/svg"
  data-slot="connect-logo"
  data-variant={shape}
  role={decorative ? undefined : 'img'}
  aria-label={decorative ? undefined : label}
  aria-hidden={decorative ? 'true' : undefined}
  class={[base, sizes[size], className]}
>
  {#if shape === 'prusa'}
    <path class="fill-current" d={PRUSA} />
  {:else if shape === 'full'}
    <path class="fill-current" d={WORDMARK} />
  {:else}
    <path class="fill-logo" d={MARK} />
  {/if}
</svg>
