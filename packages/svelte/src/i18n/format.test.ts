// The two preferences that change a format.
import { afterEach, describe, expect, it } from 'vitest';

import {
  celsiusFor,
  degreesFor,
  formatDateTime,
  formatDuration,
  formatUnit,
} from './format';
import { formatting } from './formatting.svelte';

// Local, so the reader's own timezone never moves it.
const INSTANT = new Date(2026, 7, 19, 17, 5);

afterEach(() => {
  formatting.clock = '24-hour';
  formatting.temperature = 'celsius';
});

describe('the clock preference', () => {
  it('writes the same instant in both cycles', () => {
    formatting.clock = '24-hour';
    const day = formatDateTime(INSTANT);

    formatting.clock = '12-hour';
    const evening = formatDateTime(INSTANT);

    expect(day).toContain('17:05');
    expect(evening).toContain('5:05');
    expect(evening).not.toContain('17:05');
  });
});

describe('the temperature preference', () => {
  it('writes a Celsius reading in Fahrenheit without changing the reading', () => {
    formatting.temperature = 'celsius';
    expect(formatUnit(100, '°C', { maximumFractionDigits: 0 })).toContain(
      '100',
    );

    formatting.temperature = 'fahrenheit';
    const warm = formatUnit(100, '°C', { maximumFractionDigits: 0 });

    expect(warm).toContain('212');
    expect(warm).toContain('F');
  });

  it('turns a figure typed in either unit back into the same Celsius', () => {
    for (const unit of ['celsius', 'fahrenheit'] as const) {
      formatting.temperature = unit;

      expect(celsiusFor(degreesFor(215))).toBeCloseTo(215);
    }
    formatting.temperature = 'fahrenheit';
    expect(celsiusFor(32)).toBe(0);
  });

  it('leaves every other unit alone', () => {
    formatting.temperature = 'fahrenheit';

    expect(formatUnit(42, '%')).toContain('42');
    expect(formatUnit(60, 'mm')).toContain('60');
  });
});

describe('a duration', () => {
  it('leaves out a unit that is zero', () => {
    expect(formatDuration(4 * 3600)).not.toContain('0');
    expect(formatDuration(4 * 3600 + 59)).toBe(formatDuration(4 * 3600));
    expect(formatDuration(4 * 3600 + 120)).toContain('2');
    expect(formatDuration(4 * 3600 + 120)).not.toBe(formatDuration(4 * 3600));
    expect(formatDuration(90)).toBe(formatDuration(60));
  });
});
