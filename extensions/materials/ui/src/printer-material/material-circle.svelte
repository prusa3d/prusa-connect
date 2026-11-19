<!--
  The material on a tool: a circle in the material's colour with its type
  inside, the text black or white against it. A tool whose material has no
  colour, such as one only the printer reports, gets a dashed outline instead.
  The type shrinks as it grows longer.

  The active tool wears a ring, which turns while the printer prints.
-->
<script lang="ts">
  import { cssColour } from '../css-colour';

  let {
    text,
    colorRgba,
    size,
    ring = 'none',
    title,
  }: {
    text: string;
    /** The colour as #rrggbbaa; none draws the dashed outline. */
    colorRgba: string | undefined;
    size: 'small' | 'large';
    ring?: 'none' | 'still' | 'turning';
    title: string;
  } = $props();

  const colour = $derived(cssColour(colorRgba));
  const coloured = $derived(colour !== undefined);
</script>

<span
  class={['halo', ring === 'turning' && 'turning']}
  data-ring={ring}
  data-size={size}
>
  <span
    data-part="material-circle"
    class={['circle font-semibold', !coloured && 'empty']}
    style="--colour: {colour ?? 'transparent'}; --chars: {Math.max(
      text.length,
      3,
    )};"
    role="img"
    aria-label={text === '' ? title : `${title}: ${text}`}
    {title}
  >
    {text}
  </span>
</span>

<style>
  .halo {
    --size: calc(var(--spacing) * 30);
    --cap: 1.25rem;
    position: relative;
    display: inline-flex;
    border-radius: 50%;
  }

  .halo[data-size='large'] {
    --size: calc(var(--spacing) * 30);
    --cap: 1.5rem;
  }

  .halo:not([data-ring='none']) {
    padding: calc(var(--spacing) * 1.25);
  }

  /* A thin ring of the text colour, faint on one side and strong on the
     other, so its turning shows. */
  .halo:not([data-ring='none'])::before {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background: conic-gradient(
      color-mix(in oklch, var(--color-foreground) 15%, transparent),
      color-mix(in oklch, var(--color-foreground) 70%, transparent)
    );
    mask: radial-gradient(
      farthest-side,
      transparent calc(100% - 2px),
      black calc(100% - 2px)
    );
  }

  .turning::before {
    animation: turn 1.5s linear infinite;
  }

  @media (prefers-reduced-motion: reduce) {
    .turning::before {
      animation: none;
    }
  }

  @keyframes turn {
    to {
      transform: rotate(1turn);
    }
  }

  .circle {
    display: grid;
    place-items: center;
    inline-size: var(--size);
    block-size: var(--size);
    border-radius: 50%;
    overflow: hidden;
    background: var(--colour);
    /* Black on a light colour, white on a dark one. */
    color: oklch(from var(--colour) clamp(0, (0.62 - l) * 1000, 1) 0 0);
    font-size: min(var(--cap), calc(var(--size) / (2.4 + var(--chars) * 0.45)));
    line-height: 1;
    white-space: nowrap;
  }

  .empty {
    border: 2px dashed var(--color-border);
    color: var(--color-foreground);
  }
</style>
