// Moved from pull request 185, packages/domain/src/heating.ts, 2026-09-11

/**
 * A heater's state relative to its target. In the domain rather than beside
 * any one component: three screens ask about four different heaters, every
 * tool head and the bed, and three copies of `current < target` would
 * eventually disagree about the edge cases below. This module only names the
 * state; a component turns a state into a word or a colour on screen.
 */
export type HeaterState =
  'heating' | 'at-temperature' | 'cooling' | 'off' | 'not-reported';

export interface HeaterReading {
  /** The reading, or `undefined` when the printer has not reported one. */
  current: number | undefined;
  /** The target, or `undefined`. `0` means the heater is off, not "cold target". */
  target: number | undefined;
}

/**
 * How close counts as arrived. A heater holding 234.6 against a 235 target
 * has arrived: firmware settles a degree or so either side of a target and
 * stays there, so a strict comparison would pulse for the whole print. Two
 * degrees is the band this project treats as "at temperature" for display.
 * It is a rendering threshold, never a safety one, and nothing decides
 * whether to print from it.
 */
export const HEATING_TOLERANCE_CELSIUS = 2;

/**
 * Below this, a heater with no target (or a target of zero) reads as `off`
 * rather than `cooling`. A heater told to go to zero is being switched off;
 * above the floor it is still hot enough to matter to a person reaching
 * into the machine, which is what the floor is for.
 */
export const COOLING_FLOOR_CELSIUS = 40;

export const heaterState = ({
  current,
  target,
}: HeaterReading): HeaterState => {
  if (current === undefined) {
    return 'not-reported';
  }
  if (target === undefined || target <= 0) {
    return current > COOLING_FLOOR_CELSIUS ? 'cooling' : 'off';
  }
  if (current < target - HEATING_TOLERANCE_CELSIUS) {
    return 'heating';
  }
  if (current > target + HEATING_TOLERANCE_CELSIUS) {
    return 'cooling';
  }
  return 'at-temperature';
};

/**
 * What a chamber is doing. A chamber has fans and no heater, and its target
 * works both ways: the fans hold the air under it, and a print waits while the
 * bed warms the air up to it. So against a target it is a heater like any
 * other, and with no target it is off whatever the air, because nothing is holding the
 * air anywhere and a warm chamber is no hazard the way a warm nozzle is.
 */
export const chamberState = ({
  current,
  target,
}: HeaterReading): HeaterState => {
  if (current === undefined) {
    return 'not-reported';
  }
  if (target === undefined || target <= 0) {
    return 'off';
  }
  return heaterState({ current, target });
};

// Whether a heater is still climbing towards its target: the one condition
// the interface marks with continuous motion (a slow pulse on the reading),
// because a monitoring product with no sign of life on screen reads as a
// picture of itself.
export const isHeating = (reading: HeaterReading): boolean =>
  heaterState(reading) === 'heating';
