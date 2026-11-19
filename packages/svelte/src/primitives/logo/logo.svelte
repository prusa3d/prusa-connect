<!--
  The product's wordmark: PRUSA over its product name.

  Set, not drawn. The mark is real text in Atlas Grotesk Bold, the typeface the
  theme already names and the one the reference wordmark is cut from, so it
  needs no asset, it scales with the type scale, and a screen reader reads a
  product name rather than an image.

  Monochrome by construction. Every part inherits `currentColor`, so the caller
  decides black or white by setting a text colour on an ancestor and the mark
  follows. There is deliberately no colour prop and no muted token on the
  second line: a lockup that resolves to two greys stops being a black-or-white
  mark the moment it lands on a coloured surface. The second line is quieter
  because it is smaller, never because it is dimmer.

  The name matches the mark's width by its size, not by its spacing. `Connect
  Local` set in this face at `0.49em` of the wordmark is naturally as wide as
  `PRUSA`. That is the whole trick: no letter-spacing, no justification, and
  nothing measured at run time. Spreading the letters to fill the width instead
  was tried and is worse at every size, because it reads as a caption that has
  been pulled apart rather than as the second line of a mark.

  The gap is optical, not a box gap. `leading-none` already puts the two boxes
  flush, but a line box is taller than the caps inside it, so flush boxes still
  leave visible air. The small negative margin takes that air back out. It is
  `em` of the second line, which is itself `0.49em` of the mark, so the pull
  stays the same share of the lockup at every size. `0.22em` was chosen by
  rendering the lockup at `0.09`, `0.14`, `0.18`, `0.22`, `0.26` and `0.30em`
  side by side and reading them: below `0.18em` the two lines still read as two
  stacked labels, and past `0.26em` the name crowds the wordmark's baseline.
  `0.22em` is the tightest pull that leaves a hairline of ground between them.
-->
<script lang="ts">
  import type { LogoLines, LogoProps } from './types';
  import { base, sizes } from './variants';

  let {
    subtitle = 'Connect',
    size = 'md',
    lines = 'two',
    class: className,
  }: LogoProps = $props();

  // Split on whitespace rather than on a fixed pair, so a one-word product
  // name gets a single line instead of an empty second one, and an empty name
  // draws the bare wordmark.
  const linesOf = (name: string, shape: LogoLines): string[] => {
    const words = name.split(/\s+/u).filter(Boolean);

    if (shape === 'three') return words;
    return words.length > 0 ? [name] : [];
  };

  const nameLines = $derived(linesOf(subtitle, lines));
</script>

<span
  data-slot="logo"
  data-variant={size}
  class={[base, sizes[size], className]}
>
  <span class="font-bold tracking-tight">PRUSA</span>
  {#each nameLines as line, index (index)}
    <span class="-mt-logo-name-pull text-logo-name font-medium">{line}</span>
  {/each}
</span>
