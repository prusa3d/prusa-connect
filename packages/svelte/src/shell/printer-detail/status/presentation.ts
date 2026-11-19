// The printer as the Status panel reads it.
import {
  capabilityFor,
  JOB_DISPLAY_PATH,
  JOB_PATH_PATH,
  JOB_PROGRESS_PATH,
  JOB_TIME_REMAINING_PATH,
  LIFECYCLE_TRANSITIONS,
  type LifecycleVerb,
  numberAt,
  printerStateOf,
  projectCommand,
  STATE_PATH,
  textAt,
  type CommandProjection,
  type Device,
  type PrinterState,
} from '../../../domain';
import { blocked, refusalText } from '../controls/machine-controls';
import type { CommandPort, TelemetrySource } from '../types';
import type { StatusJob, StatusVariant } from './types';

export const PRINTER_STATE_VARIANT: Record<PrinterState, StatusVariant> = {
  PRINTING: 'running',
  BUSY: 'running',
  MANIPULATING: 'running',
  PAUSED: 'running',
  ERROR: 'trouble',
  ATTENTION: 'trouble',
  IDLE: 'starter',
  READY: 'starter',
  STOPPED: 'starter',
  FINISHED: 'finished',
  OFFLINE: 'notReporting',
  UNKNOWN: 'notReporting',
};

const number = (
  telemetry: TelemetrySource | undefined,
  device: Device,
  path: string,
): number | undefined => {
  const live = telemetry?.value(path);

  return typeof live === 'number' && Number.isFinite(live)
    ? live
    : numberAt(device, path);
};

const text = (
  telemetry: TelemetrySource | undefined,
  device: Device,
  path: string,
): string | undefined => {
  const live = telemetry?.value(path);

  return typeof live === 'string' && live !== '' ? live : textAt(device, path);
};

/** The machine's own word for what it is doing, as the wire spells it. */
export const wireStateOf = (
  telemetry: TelemetrySource,
  device: Device,
): string =>
  device.connected
    ? (text(telemetry, device, STATE_PATH) ?? 'UNKNOWN')
    : 'OFFLINE';

/** The state this library has rules for, which picks the panel's shape. */
export const stateOf = (
  telemetry: TelemetrySource,
  device: Device,
): PrinterState => printerStateOf(wireStateOf(telemetry, device));

/** How long the print has been running. */
const JOB_TIME_PRINTING_PATH = '/job/time_printing';

/** The record publishes milliseconds; the state tree carries whole seconds. */
const MS_PER_SECOND = 1000;

const emptyToUndefined = (value: string | undefined): string | undefined =>
  value === undefined || value === '' ? undefined : value;

export const jobOf = (
  telemetry: TelemetrySource | undefined,
  device: Device,
): StatusJob => {
  const progress =
    number(telemetry, device, JOB_PROGRESS_PATH) ?? device.job?.progress;
  const remainingMs = device.job?.time_remaining_ms;

  return {
    name: emptyToUndefined(
      text(telemetry, device, JOB_DISPLAY_PATH) ?? device.job?.display_name,
    ),
    path: emptyToUndefined(
      text(telemetry, device, JOB_PATH_PATH) ?? device.job?.path,
    ),
    progress: typeof progress === 'number' ? Math.round(progress) : null,
    remainingSeconds:
      number(telemetry, device, JOB_TIME_REMAINING_PATH) ??
      (typeof remainingMs === 'number' && remainingMs > 0
        ? remainingMs / MS_PER_SECOND
        : undefined),
    elapsedSeconds: number(telemetry, device, JOB_TIME_PRINTING_PATH),
  };
};

/** What the printer answers for one lifecycle verb right now. */
export const commandProjection = (
  device: Device,
  commands: CommandPort,
  state: string,
  verb: LifecycleVerb,
): CommandProjection =>
  projectCommand(capabilityFor(verb), {
    capabilities: device.capabilities,
    // The port's own vocabulary when it fetched one.
    transitions: commands.transitions ?? LIFECYCLE_TRANSITIONS,
    state,
    locked: commands.locked ?? false,
    permitted: device.verbs,
  });

/** A verb the domain has answered for, as a control draws it. */
export interface CommandControl {
  /** Whether the press is refused right now. */
  disabled: boolean;
  /** Why it is refused, in the language on screen. Undefined while it is not. */
  reason: string | undefined;
}

/** What to draw for one verb, and `undefined` for nothing at all. */
export const commandControl = (
  projection: CommandProjection,
  busy = false,
): CommandControl | undefined =>
  projection.kind === 'absent'
    ? undefined
    : {
        disabled: blocked(projection, busy),
        reason:
          projection.kind === 'disabled'
            ? refusalText(projection.reason)
            : undefined,
      };

/**
 * The two columns of the Now tab, stacked in a narrow panel and side by side
 * in a wide one. The Files block uses the same split, so both change together.
 */
export const SPLIT_ROW =
  'flex flex-col @2xl/now:flex-row @2xl/now:items-stretch';

/**
 * The narrow column, and the room after it once the columns stand side by side.
 * The cap holds it while the wide column needs the room for a file name, and
 * a wide panel lifts it, so the camera and the drop target grow with the window.
 */
export const SPLIT_LEADING =
  'pb-6 @2xl/now:w-1/3 @2xl/now:max-w-80 @5xl/now:max-w-none @2xl/now:pe-panel-padding @2xl/now:pb-0';

/** The rule between the two columns, and the room before the wide one. */
export const SPLIT_TRAILING =
  'border-t border-border pt-6 @2xl/now:border-t-0 @2xl/now:border-s @2xl/now:pt-0 @2xl/now:ps-panel-padding';
