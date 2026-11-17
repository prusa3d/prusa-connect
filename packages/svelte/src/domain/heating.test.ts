// Moved from pull request 185, packages/domain/src/heating.test.ts, 2026-09-11
import { describe, expect, it } from 'vitest';

import {
  chamberState,
  COOLING_FLOOR_CELSIUS,
  heaterState,
  HEATING_TOLERANCE_CELSIUS,
  isHeating,
} from './heating';

describe('heaterState', () => {
  it.each([
    // [description, current, target, expected]
    ['no reading at all', undefined, 235, 'not-reported'],
    ['no reading, even with no target', undefined, undefined, 'not-reported'],
    ['no target: cools above the floor', 60, undefined, 'cooling'],
    ['no target: off at the floor', COOLING_FLOOR_CELSIUS, undefined, 'off'],
    ['target zero: cools above the floor', 41, 0, 'cooling'],
    ['target zero: off exactly at the floor', COOLING_FLOOR_CELSIUS, 0, 'off'],
    ['target zero: off below the floor', 23, 0, 'off'],
    ['below the band: heating', 60, 235, 'heating'],
    [
      'lower edge of the band: at-temperature, not heating',
      235 - HEATING_TOLERANCE_CELSIUS,
      235,
      'at-temperature',
    ],
    ['inside the band: at-temperature', 234.6, 235, 'at-temperature'],
    [
      'upper edge of the band: at-temperature, not cooling',
      235 + HEATING_TOLERANCE_CELSIUS,
      235,
      'at-temperature',
    ],
    ['above the band: cooling', 240, 60, 'cooling'],
    ['zero reading is a reading, not a missing value', 0, 200, 'heating'],
  ] as const)('%s', (_description, current, target, expected) => {
    expect(heaterState({ current, target })).toBe(expected);
  });
});

describe('isHeating', () => {
  it('marks a heater still climbing towards its target', () => {
    expect(isHeating({ current: 60, target: 235 })).toBe(true);
  });

  // The case a strict `current < target` gets wrong, and the reason the
  // tolerance exists: a settled heater sits a fraction under its target for
  // the whole print, and a pulse that never stops says nothing.
  it('treats a settled heater as arrived', () => {
    expect(isHeating({ current: 234.6, target: 235 })).toBe(false);
    expect(
      isHeating({ current: 235 - HEATING_TOLERANCE_CELSIUS, target: 235 }),
    ).toBe(false);
  });

  it('says nothing is climbing when the heater is off or unreported', () => {
    expect(isHeating({ current: 23, target: 0 })).toBe(false);
    expect(isHeating({ current: undefined, target: 235 })).toBe(false);
    expect(isHeating({ current: 23, target: undefined })).toBe(false);
  });

  // Cooling down is not something a person waits on, so it carries no
  // motion either.
  it('does not mark a cooling heater', () => {
    expect(isHeating({ current: 240, target: 60 })).toBe(false);
  });

  // `isHeating` reads one branch out of a five-way classification, so it is
  // worth pinning against the single closed-form rule it must equal, over a
  // grid rather than over the handful of cases above.
  it('agrees with the closed-form rule across a spread of inputs', () => {
    const climbing = ({
      current,
      target,
    }: {
      current: number | undefined;
      target: number | undefined;
    }): boolean => {
      if (current === undefined || target === undefined || target <= 0) {
        return false;
      }
      return current < target - HEATING_TOLERANCE_CELSIUS;
    };

    const currents = [
      undefined,
      0,
      -10,
      23,
      40,
      41,
      60,
      200,
      233,
      234.6,
      235,
      236,
      300,
    ];
    const targets = [undefined, -10, 0, 60, 200, 233, 234.6, 235, 236, 300];

    for (const current of currents) {
      for (const target of targets) {
        expect(isHeating({ current, target })).toBe(
          climbing({ current, target }),
        );
      }
    }
  });
});

// A chamber has fans and no heater. A print waits for it to warm to its target,
// and the fans hold it under the target, so it reads as a heater against one.
describe('chamberState', () => {
  it.each([
    { current: undefined, target: 45, want: 'not-reported' },
    { current: 25, target: undefined, want: 'off' },
    { current: 25, target: 0, want: 'off' },
    { current: 50, target: 0, want: 'off' },
    { current: 25, target: 45, want: 'heating' },
    { current: 45, target: 45, want: 'at-temperature' },
    { current: 50, target: 45, want: 'cooling' },
  ] as const)(
    'reads $current against $target as $want',
    ({ current, target, want }) => {
      expect(chamberState({ current, target })).toBe(want);
    },
  );
});
