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
  {
    stroke: 'stroke-muted-foreground',
    fill: 'fill-muted-foreground',
    swatch: 'bg-muted-foreground',
  },
];
