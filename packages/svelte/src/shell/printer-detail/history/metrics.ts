// What a metric path is called, and what its readings are measured in.
//
// A stored sample carries the connector's own path and nothing else, and
// `/tools/1/nozzle_target_temp` is an address and not a name. A reading
// the printer describes is named by its description (`domain/readings.ts`),
// and the other paths are the ones every dialect in this build writes; one a
// connector added since is drawn under its path, which is still truer than a
// guess at what it means.
import {
  AXIS_X_PATH,
  AXIS_Y_PATH,
  AXIS_Z_PATH,
  FLOW_PATH,
  nozzlePath,
  type Reading,
  readingName,
  readingWithRole,
  type ReadingRole,
  type ReadingUnit,
  SPEED_PATH,
} from '../../../domain';
import { t } from '../../../i18n';

const CELSIUS = '°C';
const MILLIMETRE = 'mm';
const PERCENT = '%';

interface Metric {
  readonly key: string;
  readonly unit: string;
}

const METRICS: Readonly<Record<string, Metric>> = {
  [AXIS_X_PATH]: { key: 'printerDetailHistory.metric.axisX', unit: MILLIMETRE },
  [AXIS_Y_PATH]: { key: 'printerDetailHistory.metric.axisY', unit: MILLIMETRE },
  [AXIS_Z_PATH]: { key: 'printerDetailHistory.metric.axisZ', unit: MILLIMETRE },
  [SPEED_PATH]: { key: 'printerDetailHistory.metric.speed', unit: PERCENT },
  [FLOW_PATH]: { key: 'printerDetailHistory.metric.flow', unit: PERCENT },
};

const UNIT: Readonly<Record<ReadingUnit, string>> = { celsius: CELSIUS };

const ROLE_KEY: Readonly<Record<ReadingRole, string>> = {
  bed: 'printerDetailHistory.metric.bed',
  chamber: 'printerDetailHistory.metric.chamber',
};

const ROLE_TARGET_KEY: Readonly<Record<ReadingRole, string>> = {
  bed: 'printerDetailHistory.metric.bedTarget',
  chamber: 'printerDetailHistory.metric.chamberTarget',
};

/**
 * What the chart draws before a reader has chosen: the first nozzle and the
 * bed. Two readings in one unit, which is what makes the axis mean something:
 * a chart holding a temperature and a millimetre at once has a y axis that
 * measures neither.
 */
export const defaultChartMetrics = (readings: readonly Reading[]): string[] => {
  const bed = readingWithRole(readings, 'bed');

  return bed === undefined ? [nozzlePath(0)] : [nozzlePath(0), bed.valuePath];
};

/** The reading a path is the value or the target of. */
const describedBy = (
  metric: string,
  readings: readonly Reading[],
): { reading: Reading; target: boolean } | undefined => {
  for (const reading of readings) {
    if (reading.valuePath === metric) return { reading, target: false };
    if (reading.targetPath === metric) return { reading, target: true };
  }

  return undefined;
};

const describedLabel = (reading: Reading, target: boolean): string => {
  if (reading.role !== undefined) {
    return t((target ? ROLE_TARGET_KEY : ROLE_KEY)[reading.role]);
  }

  return target
    ? t('printerDetailHistory.metric.readingTarget', {
        name: readingName(reading),
      })
    : readingName(reading);
};

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
      { n: Number(match[1]) + 1 },
    ),
    unit: CELSIUS,
  };
};

export const metricLabel = (
  metric: string,
  readings: readonly Reading[],
): string => {
  const head = headReading(metric);
  if (head !== undefined) return head.label;

  const described = describedBy(metric, readings);
  if (described !== undefined) {
    return describedLabel(described.reading, described.target);
  }

  const known = METRICS[metric];

  return known === undefined ? metric : t(known.key);
};

export const metricUnit = (
  metric: string,
  readings: readonly Reading[],
): string | undefined => {
  const described = describedBy(metric, readings);

  return (
    headReading(metric)?.unit ??
    (described === undefined ? undefined : UNIT[described.reading.unit]) ??
    METRICS[metric]?.unit
  );
};

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
export const sharedUnit = (
  metrics: readonly string[],
  readings: readonly Reading[],
): string | undefined => {
  const units = new Set(metrics.map((metric) => metricUnit(metric, readings)));

  return units.size === 1 ? [...units][0] : undefined;
};
