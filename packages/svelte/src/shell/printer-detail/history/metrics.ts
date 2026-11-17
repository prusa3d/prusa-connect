// What a metric path is called, and what its readings are measured in.
//
// A stored sample carries the connector's own path and nothing else, and
// `/tools/1/nozzle_target_temp` is an address rather than a name. The paths are
// the ones every dialect in this build writes (`domain/state-paths.ts`); one a
// connector added since is drawn under its path, which is still truer than a
// guess at what it means.
import {
  AXIS_X_PATH,
  AXIS_Y_PATH,
  AXIS_Z_PATH,
  BED_PATH,
  BED_TARGET_PATH,
  CHAMBER_PATH,
  CHAMBER_TARGET_PATH,
  FLOW_PATH,
  nozzlePath,
  SPEED_PATH,
} from '../../../domain';
import { t } from '../../../i18n';

const CELSIUS = '°C';
const MILLIMETRE = 'mm';
const PERCENT = '%';

interface Reading {
  readonly key: string;
  readonly unit: string;
}

const READINGS: Readonly<Record<string, Reading>> = {
  [BED_PATH]: { key: 'printerDetailHistory.metric.bed', unit: CELSIUS },
  [BED_TARGET_PATH]: {
    key: 'printerDetailHistory.metric.bedTarget',
    unit: CELSIUS,
  },
  [CHAMBER_PATH]: { key: 'printerDetailHistory.metric.chamber', unit: CELSIUS },
  [CHAMBER_TARGET_PATH]: {
    key: 'printerDetailHistory.metric.chamberTarget',
    unit: CELSIUS,
  },
  [AXIS_X_PATH]: { key: 'printerDetailHistory.metric.axisX', unit: MILLIMETRE },
  [AXIS_Y_PATH]: { key: 'printerDetailHistory.metric.axisY', unit: MILLIMETRE },
  [AXIS_Z_PATH]: { key: 'printerDetailHistory.metric.axisZ', unit: MILLIMETRE },
  [SPEED_PATH]: { key: 'printerDetailHistory.metric.speed', unit: PERCENT },
  [FLOW_PATH]: { key: 'printerDetailHistory.metric.flow', unit: PERCENT },
};

/**
 * What the chart draws before a reader has chosen. Two readings in one unit,
 * which is what makes the axis mean something: a chart holding a temperature
 * and a millimetre at once has a y axis that measures neither.
 */
export const DEFAULT_CHART_METRICS: readonly string[] = [
  nozzlePath(0),
  BED_PATH,
];

/** The nozzle reading or target of one head, such as `/tools/1/nozzle_temp`. */
const HEAD_READING =
  /^\/tools\/(0|[1-9]\d*)\/(nozzle_temp|nozzle_target_temp)$/;

/** The words and the unit of a head's reading, keyed by the head. */
const headReading = (
  metric: string,
): { label: string; unit: string } | undefined => {
  const match = HEAD_READING.exec(metric);
  if (match?.[1] === undefined) return undefined;

  return {
    label: t(
      match[2] === 'nozzle_temp'
        ? 'printerDetailHistory.metric.headNozzle'
        : 'printerDetailHistory.metric.headNozzleTarget',
      { n: match[1] },
    ),
    unit: CELSIUS,
  };
};

export const metricLabel = (metric: string): string => {
  const head = headReading(metric);
  if (head !== undefined) return head.label;

  const reading = READINGS[metric];

  return reading === undefined ? metric : t(reading.key);
};

export const metricUnit = (metric: string): string | undefined =>
  headReading(metric)?.unit ?? READINGS[metric]?.unit;

/**
 * The smallest axis step a unit reads well at. A temperature sensor's noise
 * is a fraction of a degree, and an axis labelled in tenths of a degree is
 * noise a reader cannot use.
 */
const MIN_STEPS: Readonly<Record<string, number>> = { [CELSIUS]: 5 };

/** The smallest axis step for a chart drawn in `unit`, where one is set. */
export const minStepOf = (unit: string | undefined): number | undefined =>
  unit === undefined ? undefined : MIN_STEPS[unit];

/** The unit every one of these shares, where they share one. */
export const sharedUnit = (metrics: readonly string[]): string | undefined => {
  const units = new Set(metrics.map(metricUnit));

  return units.size === 1 ? [...units][0] : undefined;
};
