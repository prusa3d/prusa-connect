import { describe, expect, it } from 'vitest';

import { plotOf } from './geometry';

const input = (zeroBaseline: boolean) => ({
  drawn: [
    {
      id: 'jobs',
      label: 'Jobs',
      points: [
        { at: 0, value: 40 },
        { at: 1, value: 80 },
      ],
    },
  ],
  unit: undefined,
  minStep: 1,
  zeroBaseline,
  threshold: undefined,
  markers: undefined,
  regions: undefined,
  xDomain: undefined,
});

describe('plotOf', () => {
  it('lets the value axis start above zero by default', () => {
    expect(plotOf(input(false)).yScale.low).toBeGreaterThan(0);
  });

  it('reaches zero when asked, so a small change is not drawn large', () => {
    expect(plotOf(input(true)).yScale.low).toBe(0);
  });
});
