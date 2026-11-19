import { describe, expect, it } from 'vitest';

import { chartDashes, chartPalette, lineStyleOf } from './palette';

describe('lineStyleOf', () => {
  it('gives each line within the palette its own colour and no dash', () => {
    const styles = chartPalette.map((_, at) => lineStyleOf(at));

    expect(styles.map((style) => style.hue)).toEqual(chartPalette);
    expect(styles.every((style) => style.dash === undefined)).toBe(true);
  });

  it('dashes a line past the palette, so it differs from the line of its colour', () => {
    const first = lineStyleOf(0);
    const lapped = lineStyleOf(chartPalette.length);

    expect(lapped.hue).toBe(first.hue);
    expect(lapped.dash).toBe(chartDashes[1]);
    expect(lapped.dash).not.toBe(first.dash);
  });

  it('gives every line of three passes its own colour and dash', () => {
    const count = chartPalette.length * 3;
    const looks = Array.from({ length: count }, (_, at) => {
      const style = lineStyleOf(at);

      return `${style.hue?.stroke ?? ''} ${style.dash ?? 'solid'}`;
    });

    expect(new Set(looks).size).toBe(count);
  });

  it('reads a place that is not in the series as the first', () => {
    expect(lineStyleOf(-1)).toEqual(lineStyleOf(0));
  });
});
