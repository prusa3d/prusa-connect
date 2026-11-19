// What the sections of the Controls panel share.
import {
  numberAt,
  PRINTER_STATE_LABEL_KEY,
  printerStateOf,
  projectCommand,
  STATE_PATH,
  textAt,
  type CommandProjection,
  type CommandRefusal,
  type Device,
} from '../../../domain';
import {
  celsiusFor,
  degreesFor,
  formatList,
  formatting,
  t,
} from '../../../i18n';
import { printerModelOf } from '../../printers/presentation';
import { verbsOf } from '../commands/types';
import type { CommandPort, TelemetrySource } from '../types';

/** The verbs this panel sends. */
export const VERBS = {
  beep: 'core.command.beep',
  cancelPrinterReady: 'core.command.cancel_printer_ready',
  chamberFan: 'core.command.set_chamber_fan_target',
  chamberLight: 'core.command.set_chamber_led_intensity',
  chamberTemp: 'core.command.set_chamber_target_temp',
  disableSteppers: 'core.command.disable_steppers',
  flow: 'core.command.set_flow',
  home: 'core.command.home',
  hostname: 'core.command.set_hostname',
  loadFilament: 'core.command.load_filament',
  meshBedLevelling: 'core.command.mesh_bed_leveling',
  move: 'core.command.move',
  moveZ: 'core.command.move_z',
  nozzle: 'core.command.set_nozzle_temperature',
  bed: 'core.command.set_bed_temperature',
  resetPrinter: 'core.command.reset_printer',
  setIdle: 'core.command.set_idle',
  setPrinterReady: 'core.command.set_printer_ready',
  speed: 'core.command.set_speed',
  toolDiameter: 'core.command.set_tool_nozzle_diameter',
  toolHardened: 'core.command.set_tool_hardened',
  toolHighFlow: 'core.command.set_tool_high_flow',
  unloadFilament: 'core.command.unload_filament',
} as const;

/** The setpoint ceilings the xbuddy dialect declares for these two. */
export const NOZZLE_MAX_CELSIUS = 300;
export const BED_MAX_CELSIUS = 120;
/**
 * The highest chamber target a model takes: 60 °C on a CORE One L and 55 °C
 * on a CORE One, each with its INDX build. The firmware cuts a higher one
 * down without saying so, and the host refuses it.
 */
export const chamberMaxCelsius = (model: string): number =>
  printerModelOf(model).startsWith('core_one_l') ? 60 : 55;

/**
 * A heater's setpoint as its editor shows it. The printer takes Celsius, and
 * the reader may have chosen Fahrenheit, so the editor converts both ways.
 */
export interface DegreeScale {
  /** The symbol the editor writes after a figure. */
  readonly unit: string;
  /** A Celsius figure in the unit the reader chose, as a whole number. */
  shown(celsius: number): number;
  /** A figure in the unit the reader chose, back in Celsius. */
  celsius(shown: number): number;
  /**
   * How far one step of the slider moves. Five degrees Celsius is exactly
   * nine Fahrenheit, so every step shows a whole figure in both units.
   */
  readonly step: number;
}

export const degreeScale = (): DegreeScale =>
  formatting.temperature === 'fahrenheit'
    ? {
        unit: '°F',
        shown: (celsius) => Math.round(degreesFor(celsius)),
        celsius: (shown) => celsiusFor(shown),
        step: 9,
      }
    : {
        unit: '°C',
        shown: (celsius) => celsius,
        celsius: (shown) => shown,
        step: 5,
      };

/** Whether a verb the printer offers declares an argument by this name. */
export const declaresArgument = (
  device: Device,
  verb: string,
  argument: string,
): boolean =>
  verbsOf(device.capabilities)
    .find((entry) => entry.verb === verb)
    ?.args.includes(argument) === true;

/** The nozzle sizes Prusa ships for its toolheads, in millimetres. */
export const NOZZLE_DIAMETERS = [0.25, 0.3, 0.4, 0.5, 0.6, 0.8] as const;

/** The longest hostname the xbuddy dialect accepts. */
export const HOSTNAME_MAX = 35;

/**
 * Why a hostname cannot be sent, as a catalogue key, or `undefined` when it
 * can. The printer announces itself under this one name, so it is held to a
 * single RFC 1123 label: letters, digits and inner hyphens. An empty name is
 * the dialog's own refusal and answers `undefined` here.
 */
export const hostnameProblem = (name: string): string | undefined => {
  if (name.length > HOSTNAME_MAX)
    return 'printerDetailControls.hostname.tooLong';
  if (name !== '' && !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/i.test(name)) {
    return 'printerDetailControls.hostname.invalid';
  }
  return undefined;
};

/** What `chamber.fan_pwm_target` takes on the wire. */
export const FAN_AUTOMATIC = -1;
export const FAN_OFF = 0;
export const PERCENT_MAX = 100;

/** How far a speed or flow override may be nudged. */
export const TUNING_MIN_PERCENT = 50;
export const TUNING_MAX_PERCENT = 200;
export const TUNING_REFERENCE_PERCENT = 100;

/** How far one press of the jog pad moves the gantry, in millimetres. */
export const JOG_STEPS_MM = [0.1, 1, 10] as const;

/** The feedrates a jog travels at, in millimetres per minute. */
export const JOG_XY_FEEDRATE = 3000;
export const JOG_Z_FEEDRATE = 600;

/**
 * Whether the bed, not the nozzle, travels on Z. On these machines Z grows as
 * the bed goes down, so the pad draws the bed's own motion.
 */
export const bedTravelsOnZ = (model: string): boolean => {
  const known = printerModelOf(model);

  return known.startsWith('core_one') || known.startsWith('xl');
};

/** One reading, on the surface a section states it on. */
export const READING_TILE = 'rounded-sm bg-surface px-3 py-2';

/** The heading a section wears, with room for the glyph on the same line. */
export const SECTION_TITLE =
  'flex items-center gap-2 text-sm font-medium text-foreground';

export type Resolve = (verb: string) => CommandProjection;

/** The wire word the printer last reported, or `undefined` while it has none. */
export const stateOf = (device: Device): string | undefined =>
  textAt(device, STATE_PATH);

/** The verbs that move the machine, which run only in `MOTION_STATES`. */
const MOTION_VERBS: ReadonlySet<string> = new Set([
  VERBS.disableSteppers,
  VERBS.home,
  VERBS.move,
  VERBS.moveZ,
  'core.command.z_power_move',
]);

/**
 * The states with nothing on the machine. A print that finished or stopped
 * has left the gantry, so homing and jogging after it are safe.
 */
const MOTION_STATES: ReadonlySet<string> = new Set([
  'IDLE',
  'READY',
  'FINISHED',
  'STOPPED',
]);

/** The verbs a print holds while it runs. */
const PRINT_HOLDS: Readonly<Record<string, string>> = {
  [VERBS.beep]: 'printerDetailControls.refused.printing',
  [VERBS.cancelPrinterReady]: 'printerDetailControls.refused.cleared',
  [VERBS.loadFilament]: 'printerDetailControls.refused.printing',
  [VERBS.meshBedLevelling]: 'printerDetailControls.refused.cleared',
  'core.command.move_e': 'printerDetailControls.refused.printing',
  [VERBS.setIdle]: 'printerDetailControls.refused.cleared',
  [VERBS.setPrinterReady]: 'printerDetailControls.refused.cleared',
  [VERBS.unloadFilament]: 'printerDetailControls.refused.printing',
};

/**
 * A projection with the page's own holds applied: the machine moves only
 * in `MOTION_STATES`, and a running print holds the verbs above. The
 * coordinate plate, the step pad and the Advanced catalogue all read it, so
 * they agree on the one page they share.
 */
export const withHolds = (
  verb: string,
  projection: CommandProjection,
  state: string | undefined,
): CommandProjection => {
  if (projection.kind !== 'available') return projection;
  const observed = printerStateOf(state ?? 'UNKNOWN');
  if (MOTION_VERBS.has(verb))
    return MOTION_STATES.has(observed)
      ? projection
      : {
          kind: 'disabled',
          reason: { key: 'printerDetailControls.coordinates.idleRequired' },
        };
  const held = PRINT_HOLDS[verb];
  return held !== undefined && observed === 'PRINTING'
    ? { kind: 'disabled', reason: { key: held } }
    : projection;
};

/**
 * `state` is the wire word the page observes, telemetry first, and
 * undefined for a printer that is not reporting.
 */
export const resolverFor =
  (device: Device, commands: CommandPort, state: string | undefined): Resolve =>
  (verb) => {
    const projection = projectCommand(verb, {
      capabilities: device.capabilities,
      transitions: commands.transitions ?? [],
      state,
      locked: commands.locked === true,
      permitted: device.verbs,
    });

    return withHolds(verb, projection, state);
  };

/** A reading from the live stream first and the device record after it. */
export const readingOf = (
  telemetry: TelemetrySource,
  device: Device,
  path: string,
): number | undefined => {
  const live = telemetry.value(path);

  return typeof live === 'number' && Number.isFinite(live)
    ? live
    : numberAt(device, path);
};

export const textOf = (
  telemetry: TelemetrySource,
  device: Device,
  path: string,
): string | undefined => {
  const live = telemetry.value(path);

  return typeof live === 'string' && live !== '' ? live : textAt(device, path);
};

/** Why a verb is refused, in the language on screen. */
export const refusalText = (reason: CommandRefusal): string =>
  reason.states === undefined
    ? t(reason.key)
    : t(reason.key, {
        states: formatList(
          reason.states.map((wire) =>
            t(PRINTER_STATE_LABEL_KEY[printerStateOf(wire)]),
          ),
        ),
      });

/** A section's own refusal, and only where every verb it offers refuses. */
export const groupReason = (
  projections: readonly CommandProjection[],
): string | undefined => {
  const present = projections.filter(
    (projection) => projection.kind !== 'absent',
  );
  const first = present[0];

  if (
    first === undefined ||
    present.some((projection) => projection.kind === 'available')
  ) {
    return undefined;
  }

  return first.kind === 'disabled' ? refusalText(first.reason) : undefined;
};

/** Whether a control that is drawn can be pressed right now. */
export const blocked = (
  projection: CommandProjection,
  busy: boolean,
): boolean => busy || projection.kind !== 'available';

/** What every section is handed. */
export interface SectionProps {
  resolve: Resolve;
  reading: (path: string) => number | undefined;
  /** A command is on its way, so nothing new may be sent. */
  busy: boolean;
  onCommand: CommandPort['onCommand'];
}
