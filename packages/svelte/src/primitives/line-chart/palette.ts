/**
 * The tokens that tell one line from the next, as the stroke of a line, the
 * fill of a lone point and the swatch in the legend. The theme names no
 * chart palette, so these are accent tokens picked for contrast.
 * `NON_TEXT_PAIRS` in `contrast.test.ts` measures each one against every
 * background at 3:1, because a line carries meaning.
 */
export const chartPalette = [
  { stroke: 'stroke-primary', fill: 'fill-primary', swatch: 'bg-primary' },
  { stroke: 'stroke-info', fill: 'fill-info', swatch: 'bg-info' },
  {
    stroke: 'stroke-success-text',
    fill: 'fill-success-text',
    swatch: 'bg-success-text',
  },
  {
    stroke: 'stroke-warning-text',
    fill: 'fill-warning-text',
    swatch: 'bg-warning-text',
  },
  {
    stroke: 'stroke-destructive-text',
    fill: 'fill-destructive-text',
    swatch: 'bg-destructive-text',
  },
];

/**
 * The dash a line wears on each pass through the palette: solid on the first,
 * so a line past the last colour still differs from the line of its colour.
 */
export const chartDashes = [undefined, '6 4', '2 3', '8 3 2 3'] as const;

export interface LineStyle {
  readonly hue: (typeof chartPalette)[number] | undefined;
  readonly dash: (typeof chartDashes)[number];
}

/** The colour and dash of the line at one place in a chart's series. */
export const lineStyleOf = (at: number): LineStyle => {
  const place = Math.max(0, Math.trunc(at));
  const lap = Math.floor(place / chartPalette.length);

  return {
    hue: chartPalette[place % chartPalette.length],
    dash: chartDashes[lap % chartDashes.length],
  };
};
