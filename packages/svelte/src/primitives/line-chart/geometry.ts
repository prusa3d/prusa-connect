// What the line chart draws and says, worked out from its props and kept
// apart from the markup so a test can check it.
import { formatDate, formatNumber, formatUnit, t } from '../../i18n';
import type { TelemetryPoint } from '../../domain';
import { valueScale, type TimeTick } from './scale';
import { segmentsOf } from './segments';
import type {
  ChartMarker,
  ChartRegion,
  ChartSeries,
  ChartThreshold,
} from './types';

// The side of the box the lines are drawn in, in SVG units.
export const BOX = 1000;

// `Math.min` and `Math.max` would turn a gap (`NaN`) into the answer.
export const lowestOf = (one: ChartSeries): number =>
  one.points.reduce(
    (low, point) => (point.value < low ? point.value : low),
    Infinity,
  );

export const highestOf = (one: ChartSeries): number =>
  one.points.reduce(
    (high, point) => (point.value > high ? point.value : high),
    -Infinity,
  );

export const latestOf = (one: ChartSeries): number =>
  one.points.filter((point) => !Number.isNaN(point.value)).at(-1)?.value ?? 0;

/** The lines that have a reading to draw: not hidden, and not all gaps. */
export const drawnOf = (series: readonly ChartSeries[]): ChartSeries[] =>
  series.filter(
    (one) =>
      one.hidden !== true &&
      one.points.some((point) => !Number.isNaN(point.value)),
  );

export const xLabel = (tick: TimeTick): string =>
  formatDate(
    tick.at,
    tick.day
      ? { day: 'numeric', month: 'short' }
      : { hour: 'numeric', minute: '2-digit' },
  );

export const valueText = (
  value: number,
  valueUnit: string | undefined,
): string =>
  valueUnit === undefined
    ? formatNumber(value, { maximumFractionDigits: 1 })
    : formatUnit(value, valueUnit, { maximumFractionDigits: 1 });

export interface PlotInput {
  readonly drawn: readonly ChartSeries[];
  readonly unit: string | undefined;
  readonly minStep: number | undefined;
  readonly zeroBaseline?: boolean | undefined;
  readonly threshold: ChartThreshold | undefined;
  readonly markers: readonly ChartMarker[] | undefined;
  readonly regions: readonly ChartRegion[] | undefined;
  readonly xDomain: readonly [number, number] | undefined;
}

const nearestReading = (
  one: ChartSeries | undefined,
  at: number,
): TelemetryPoint | undefined =>
  one?.points
    .filter((point) => !Number.isNaN(point.value))
    .reduce<TelemetryPoint | undefined>(
      (best, point) =>
        best === undefined || Math.abs(point.at - at) < Math.abs(best.at - at)
          ? point
          : best,
      undefined,
    );

export const plotOf = (input: PlotInput) => {
  const { drawn, unit, threshold, regions } = input;

  const yScale = valueScale(
    Math.min(
      ...drawn.map(lowestOf),
      threshold?.value ?? Infinity,
      input.zeroBaseline ? 0 : Infinity,
    ),
    Math.max(...drawn.map(highestOf), threshold?.value ?? -Infinity),
    undefined,
    input.minStep,
  );

  // The time axis spans the window the caller asked for, or else the points.
  let xRange: readonly [number, number] = input.xDomain ?? [
    Infinity,
    -Infinity,
  ];
  if (input.xDomain === undefined) {
    for (const one of drawn) {
      for (const point of one.points) {
        xRange = [Math.min(xRange[0], point.at), Math.max(xRange[1], point.at)];
      }
    }
  }

  // Each position is a share of the plot, from 0 to 1. A time axis with no
  // length puts everything in its middle.
  const xAt = (at: number): number => {
    const [from, to] = xRange;

    return to > from ? (at - from) / (to - from) : 0.5;
  };

  // Measured from the top, because SVG and CSS both count downward.
  const yAt = (value: number): number =>
    (yScale.high - value) / (yScale.high - yScale.low);

  const coordinate = (share: number): number =>
    Math.round(share * BOX * 10) / 10;

  // A marker outside the axis is left out, as a region is cut to it.
  const markers = (input.markers ?? []).filter((one) => {
    const share = xAt(one.at);

    return share >= 0 && share <= 1;
  });

  const breaks = [
    ...markers.map((one) => one.at),
    ...(regions ?? []).flatMap((one) => [one.from, one.to]),
  ];

  // A run of one point has no line to draw and is drawn as a dot.
  const lines = drawn.map((one) => {
    const runs = segmentsOf(one.points, breaks);

    return {
      id: one.id,
      path: runs
        .filter((run) => run.length > 1)
        .map((run) =>
          run
            .map(
              (point, index) =>
                `${index === 0 ? 'M' : 'L'}${String(coordinate(xAt(point.at)))},${String(coordinate(yAt(point.value)))}`,
            )
            .join(''),
        )
        .join(''),
      dots: runs.flatMap((run) => (run.length === 1 ? run : [])),
    };
  });

  const bands = (regions ?? []).map((one) => {
    const from = Math.min(1, Math.max(0, xAt(one.from)));
    const to = Math.min(1, Math.max(0, xAt(one.to)));

    return { left: from, width: to - from };
  });

  // Each marker sits on the first drawn line, at the reading nearest its instant.
  const flags = markers.map((one) => ({
    at: one.at,
    nearest: nearestReading(drawn[0], one.at),
  }));

  // What a reader who cannot see the chart is told of what is drawn on it.
  const extras = [
    ...(threshold === undefined
      ? []
      : [
          t('primitives.lineChart.threshold', {
            label: threshold.label,
            value: valueText(threshold.value, unit),
          }),
        ]),
    ...markers.map((one) =>
      t('primitives.lineChart.marker', {
        label: one.label,
        at: xLabel({ at: one.at, day: false }),
      }),
    ),
    ...(regions ?? []).map((one) =>
      t('primitives.lineChart.region', {
        label: one.label,
        from: xLabel({ at: one.from, day: false }),
        to: xLabel({ at: one.to, day: false }),
      }),
    ),
  ];

  return { yScale, xRange, xAt, yAt, lines, bands, flags, extras };
};
