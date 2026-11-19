<script module lang="ts">
  import type { LucideIconData, LucideIconNode } from '@lucide/svelte';

  // Drawn on a 383 by 372 box, so each hexagon is scaled to 20 pixels of
  // lucide's 24 pixel grid and centred; the stroke is 2 pixels after it.
  const transform = 'translate(12 12) scale(0.0558) translate(-192.71 -187.5)';
  const hexagon = (d: string, accent = false): LucideIconNode => [
    'path',
    {
      d,
      transform,
      stroke: accent ? 'var(--primary)' : 'currentColor',
      class: accent ? 'materials-glyph-accent' : undefined,
      'stroke-width': 35.84,
      'stroke-linejoin': 'round',
      fill: 'none',
    },
  ];

  const icon: LucideIconData = {
    name: 'materials-glyph',
    size: 24,
    node: [
      hexagon(
        'M89.71 185.5L165.92 229.5V317.5L89.71 361.5L13.5 317.5V229.5L89.71 185.5Z',
      ),
      hexagon(
        'M295.71 185.5L371.92 229.5V317.5L295.71 361.5L219.5 317.5V229.5L295.71 185.5Z',
      ),
      hexagon(
        'M192.71 13.5L268.92 57.5V145.5L192.71 189.5L116.5 145.5V57.5L192.71 13.5Z',
        true,
      ),
    ],
  };
</script>

<script lang="ts">
  import { Icon, type LucideProps } from '@lucide/svelte';

  let props: LucideProps = $props();
</script>

<Icon {...props} {icon} />

<style>
  :global(.materials-glyph-accent) {
    stroke: currentColor;
  }
</style>
