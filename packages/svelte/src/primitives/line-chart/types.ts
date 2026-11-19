import type { TelemetryPoint } from '../../domain';

/** One line: what to call it, and the points it draws. */
export interface ChartSeries {
  readonly id: string;
  readonly label: string;
  readonly points: readonly TelemetryPoint[];
  /** Named in the legend, left off the chart. */
  readonly hidden?: boolean;
  /** The unit of this line's values, where it differs from the chart's. */
  readonly unit?: string | undefined;
}

/** A horizontal reference line, such as a limit the readings should stay under. */
export interface ChartThreshold {
  readonly value: number;
  readonly label: string;
}

/** An instant worth pointing at, such as a restart. */
export interface ChartMarker {
  readonly at: number;
  readonly label: string;
}

/** A span of time shaded behind the lines. */
export interface ChartRegion {
  readonly from: number;
  readonly to: number;
  readonly label: string;
}

export interface LineChartProps {
  readonly series: readonly ChartSeries[];
  /** What a reader who cannot see the lines is told the chart is. */
  readonly label: string;
  /** The unit the values carry, for the axis. */
  readonly unit?: string | undefined;
  /**
   * The smallest step between two labels on the value axis, in the chart's
   * unit. A reading that varies by less than it is drawn near the middle of
   * the axis and not stretched to its full height.
   */
  readonly minStep?: number | undefined;
  /** The value axis reaches zero, so a small change is not drawn as a large one. */
  readonly zeroBaseline?: boolean | undefined;
  /** Drawn dashed across the plot; the value axis reaches it. */
  readonly threshold?: ChartThreshold | undefined;
  /** A tick at the top edge and a dot on the first line at each instant. */
  readonly markers?: readonly ChartMarker[] | undefined;
  /** Shaded bands behind the lines. All lines break at their edges. */
  readonly regions?: readonly ChartRegion[] | undefined;
  /** The instants the time axis spans, where the data itself is not it. */
  readonly xDomain?: readonly [number, number] | undefined;
  /**
   * Given, every legend entry is a button that turns its line off and on,
   * and the legend says which state each one is in.
   */
  readonly onToggle?: ((id: string) => void) | undefined;
  /** What the legend is, for a reader who reaches it by its role. */
  readonly legendLabel?: string | undefined;
  /** What a legend button offers, shown and hidden. */
  readonly toggleLabels?:
    { readonly shown: string; readonly hidden: string } | undefined;
  readonly class?: string | undefined;
}
