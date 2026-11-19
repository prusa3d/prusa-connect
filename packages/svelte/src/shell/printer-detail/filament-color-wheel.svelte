<script lang="ts" generics="Color extends readonly [string, string, string]">
  import { t } from '../../i18n';

  let {
    colors,
    outerCount,
    selected,
    labelOf = (color) => t(color[2]),
    onChoose,
  }: {
    colors: readonly Color[];
    outerCount: number;
    selected: Color | undefined;
    labelOf?: (color: Color) => string;
    onChoose: (color: Color) => void;
  } = $props();

  const heading = $derived(t('printerDetailInfo.filament.wheel'));

  const coordinates = (radius: number, angle: number) => ({
    x: String(120 + radius * Math.sin(angle)),
    y: String(120 - radius * Math.cos(angle)),
  });

  const point = (radius: number, angle: number): string => {
    const { x, y } = coordinates(radius, angle);
    return `${x} ${y}`;
  };

  const wedge = (color: Color, index: number) => {
    const outer = index < outerCount;
    const count = outer ? outerCount : colors.length - outerCount;
    const start = ((outer ? index : index - outerCount) * 2 * Math.PI) / count;
    const end = start + (2 * Math.PI) / count;
    const radius = outer ? 111.5 : 46;
    const arc = `M ${point(radius, start)} A ${String(radius)} ${String(radius)} 0 0 1 ${point(radius, end)}`;
    const path = outer
      ? `${arc} L ${point(59, end)} A 59 59 0 0 0 ${point(59, start)} Z`
      : `${arc} L 120 120 Z`;

    const middle = (start + end) / 2;
    const { x, y } = coordinates(outer ? 85 : 23, middle);
    const offset = outer ? 7 : 3;
    const dx = offset * Math.sin(middle);
    const dy = -offset * Math.cos(middle);
    const transform = `translate(${String(dx)} ${String(dy)}) translate(${x} ${y}) scale(1.08) translate(-${x} -${y})`;

    return { color, path, transform };
  };

  const wedges = $derived(colors.map(wedge));
  const active = $derived(
    wedges.find(({ color }) => color[0] === selected?.[0]),
  );
</script>

<svg
  viewBox="-10 -10 260 260"
  class="mx-auto block w-full max-w-color-wheel"
  aria-label={heading}
  role="group"
>
  {#each wedges as { color, path } (color[0])}
    <g
      role="button"
      tabindex="0"
      aria-label={labelOf(color)}
      aria-pressed={selected?.[0] === color[0]}
      onclick={() => onChoose(color)}
      onkeydown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onChoose(color);
        }
      }}
    >
      <title>{labelOf(color)}</title>
      <path
        d={path}
        fill={selected?.[0] === color[0] ? 'transparent' : color[1]}
      />
    </g>
  {/each}
  {#if active}
    <g transform={active.transform} pointer-events="none" aria-hidden="true">
      <path
        d={active.path}
        fill="none"
        stroke="var(--color-control-on)"
        stroke-width="7"
      />
      <path
        d={active.path}
        fill={active.color[1]}
        stroke="var(--color-card)"
        stroke-width="2.5"
      />
    </g>
  {/if}
</svg>

<style>
  g[role='button'] {
    cursor: pointer;
    outline: none;
  }

  g[role='button']:focus-visible path {
    stroke: var(--color-ring);
    stroke-width: 3;
  }
</style>
