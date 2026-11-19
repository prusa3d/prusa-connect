// Where a printer's readings live in the addressable state tree.
//
// The path vocabulary belongs to the connector, and every dialect in this
// build writes these (`internal/connector/xbuddy/inbound.go`). A device record
// carries the tree under `state`, keyed by path, with values the core does not
// type. So the two readers below check the type they need rather than trusting
// the map, and a path a printer never wrote reads as absent, which is a
// different fact from a reading of zero.
import type { Device } from './command';

/** The machine's own state word. The listing carries the same value. */
export const STATE_PATH = '/state';

/**
 * The nozzle of the first head. A printer with one nozzle reports it as head
 * `0` of the tool tree, and there is no path for the nozzle outside the tree.
 * A page that shows the head printing now reads `currentHeadOf` and
 * `nozzlePath`.
 */
export const NOZZLE_PATH = '/tools/0/nozzle_temp';
export const NOZZLE_TARGET_PATH = '/tools/0/nozzle_target_temp';
export const BED_PATH = '/temperature/bed';
export const BED_TARGET_PATH = '/temperature/bed_target';
/**
 * The air inside an enclosed printer and the target it is held to. A printer
 * with no chamber never writes them, and a target of 0 is no target.
 */
export const CHAMBER_PATH = '/temperature/chamber';
export const CHAMBER_TARGET_PATH = '/temperature/chamber_target';
/**
 * The chamber fans' target, as a percentage, with -1 while they run on their
 * own, and the chamber light as a percentage.
 */
export const CHAMBER_FAN_PATH = '/chamber/fan_pwm_target';
export const CHAMBER_LIGHT_PATH = '/chamber/led_intensity';

export const AXIS_X_PATH = '/axis/x';
export const AXIS_Y_PATH = '/axis/y';
export const AXIS_Z_PATH = '/axis/z';
export const SPEED_PATH = '/speed';
export const FLOW_PATH = '/flow';

/**
 * The print, addressed apart from the machine. `/state` is the machine and
 * moves on after a print ends; `/job/state` is the print, and the last thing
 * it says is how that print ended.
 */
export const JOB_STATE_PATH = '/job/state';
export const JOB_PATH_PATH = '/job/path';
export const JOB_DISPLAY_PATH = '/job/display';
export const JOB_PROGRESS_PATH = '/job/progress';
export const JOB_TIME_REMAINING_PATH = '/job/time_remaining';

/** A reading the printer sent as a word, or `undefined` when it sent none. */
export const textAt = (device: Device, path: string): string | undefined => {
  const value = device.state[path];

  return typeof value === 'string' && value !== '' ? value : undefined;
};

/** A reading the printer sent as a number, or `undefined` when it sent none. */
export const numberAt = (device: Device, path: string): number | undefined => {
  const value = device.state[path];

  return typeof value === 'number' && Number.isFinite(value)
    ? value
    : undefined;
};

/**
 * The question the machine is waiting on. The id is what
 * `core.command.dialog_action` names, and the whole node is removed once the
 * machine stops waiting.
 */
export const DIALOG_ID_PATH = '/dialog/id';
/**
 * The firmware's own code for the question, as a word and not as a number:
 * the connector passes it through as the printer encoded it
 * (`internal/connector/xbuddy/inbound.go`), and the printer encodes it as a
 * string, zero-padded in codes such as `04502`. Read as a number it would be
 * spelled `4502`, which names no question.
 *
 * It is not the id: the id names this one appearance of the question and is
 * what an answer is addressed to, and the code names which question it is. A
 * firmware that sends no title and no text still sends this, so it is the
 * only thing that tells one wordless question from another.
 */
export const DIALOG_CODE_PATH = '/dialog/code';
export const DIALOG_TITLE_PATH = '/dialog/title';
export const DIALOG_TEXT_PATH = '/dialog/text';
/** The labels on the machine's buttons, as an array in its order. */
export const DIALOG_BUTTONS_PATH = '/dialog/buttons';

export const DIALOG_PATHS: readonly string[] = [
  DIALOG_ID_PATH,
  DIALOG_CODE_PATH,
  DIALOG_TITLE_PATH,
  DIALOG_TEXT_PATH,
  DIALOG_BUTTONS_PATH,
];

/** A question on the printer's own screen, as the firmware put it there. */
export interface PrinterDialog {
  id: number;
  /** The firmware's code for which question this is. Absent when it sent none. */
  code: string | undefined;
  title: string | undefined;
  text: string | undefined;
  /** Each label is also the answer the printer is sent when it is pressed. */
  buttons: string[];
}

/** The dialog the printer is waiting on, or `undefined` while it waits on none. */
export const dialogOf = (device: Device): PrinterDialog | undefined => {
  const id = numberAt(device, DIALOG_ID_PATH);
  if (id === undefined) return undefined;

  const buttons = device.state[DIALOG_BUTTONS_PATH];

  return {
    id,
    code: textAt(device, DIALOG_CODE_PATH),
    title: textAt(device, DIALOG_TITLE_PATH),
    text: textAt(device, DIALOG_TEXT_PATH),
    buttons: Array.isArray(buttons)
      ? buttons.filter(
          (label): label is string => typeof label === 'string' && label !== '',
        )
      : [],
  };
};

/**
 * One of the firmware's button names as a person reads it. The names arrive in
 * the firmware's own spelling, `Purge_more`, and that spelling is also the
 * answer the printer is sent, so the underscores are taken out for the label
 * and never for the value. It is the label for a name the catalogue has no
 * words for; `printerButtonLabel` in the i18n package labels the rest.
 */
export const dialogButtonLabel = (button: string): string =>
  button.replaceAll('_', ' ');

/** The arguments of `core.command.dialog_action` for one press. */
export const dialogAnswer = (
  dialog: PrinterDialog,
  button: string,
): Record<string, string> => ({ dialog_id: String(dialog.id), button });

/**
 * What a head reports about its nozzle, under `features/` in the tool tree.
 * A fact the printer did not report is left out, and is unknown.
 */
export interface NozzleFeatures {
  /** The nozzle diameter in millimetres. */
  diameter?: number;
  highFlow?: boolean;
  hardened?: boolean;
}

/**
 * One head of a printer: a toolhead on a tool changer, or the one nozzle of a
 * single-tool printer. Heads count from 0, as G-code counts them. The
 * printer's tools are a tree of heads and the feeder slots on them
 * (`sdk/connectorapi/tools.go`, ADR-20261023-gna); the slots are not heads.
 */
export interface ToolReading {
  index: number;
  temperature?: number;
  /**
   * The temperature the head is heating to. It is absent while the heater is
   * off, which the tree says with a target of 0.
   */
  target?: number;
  /**
   * The material in the head, or in the slot of its feeder loaded into it.
   * Absent when nothing is loaded or it is not known.
   */
  material?: string;
  features: NozzleFeatures;
  /** Whether the head is picked: its own `active` member. */
  active: boolean;
}

const HEAD_PATH = /^\/tools\/(0|[1-9]\d*)\//;

/** The active member of a slot of a feeder on a head, such as `/tools/0.2/active`. */
const SLOT_ACTIVE_PATH = /^\/tools\/((0|[1-9]\d*)\.(0|[1-9]\d*))\/active$/;

const MATERIAL_MEMBER =
  'material_package_instance/package/material/abbreviation';

/** The path of one member of one tool, by the tool's id such as `0` or `0.2`. */
export const toolPath = (id: string, member: string): string =>
  `/tools/${id}/${member}`;

/** Tools announced by flattened state, including feeder slots. */
export const reportedToolIds = (
  state: Device['state'],
  paths: readonly string[] = [],
): string[] => [
  ...new Set(
    [...Object.keys(state), ...paths].flatMap((path) => {
      const id = /^\/tools\/((?:0|[1-9]\d*)(?:\.(?:0|[1-9]\d*))*)\//.exec(
        path,
      )?.[1];
      return id === undefined ? [] : [id];
    }),
  ),
];

/** The nozzle temperature of one head, counted from 0. */
export const nozzlePath = (head: number): string =>
  toolPath(String(head), 'nozzle_temp');

/** The temperature one head is heating to, 0 while its heater is off. */
export const nozzleTargetPath = (head: number): string =>
  toolPath(String(head), 'nozzle_target_temp');

/** The active member of a tool, such as `/tools/0.2/active`. */
const ACTIVE_PATH = /^\/tools\/((?:0|[1-9]\d*)(?:\.(?:0|[1-9]\d*))*)\/active$/;

/** The live value of a path first, and the snapshot's second. */
const valueOf = (
  telemetry: { value(path: string): unknown },
  device: Device,
  path: string,
): unknown => telemetry.value(path) ?? device.state[path];

const numberOf = (
  telemetry: { value(path: string): unknown },
  device: Device,
  path: string,
): number | undefined => {
  const value = valueOf(telemetry, device, path);

  return typeof value === 'number' && Number.isFinite(value)
    ? value
    : undefined;
};

const booleanOf = (
  telemetry: { value(path: string): unknown },
  device: Device,
  path: string,
): boolean | undefined => {
  const value = valueOf(telemetry, device, path);

  return typeof value === 'boolean' ? value : undefined;
};

/**
 * The material of one tool, as the tree carries it: a word, `''` for a tool
 * the printer says is empty, and `undefined` when the printer did not say.
 */
export const materialOf = (
  telemetry: { value(path: string): unknown },
  device: Device,
  id: string,
): string | undefined => {
  const value = valueOf(telemetry, device, toolPath(id, MATERIAL_MEMBER));

  return typeof value === 'string' ? value : undefined;
};

/**
 * Every tool whose `active` member is true, from the live values first and
 * the snapshot second.
 */
const activeIdsOf = (
  telemetry: {
    value(path: string): unknown;
    readonly paths?: readonly string[] | undefined;
  },
  device: Device,
): Set<string> => {
  const active = new Set<string>();

  for (const path of [
    ...Object.keys(device.state),
    ...(telemetry.paths ?? []),
  ]) {
    const id = ACTIVE_PATH.exec(path)?.[1];
    if (id !== undefined && valueOf(telemetry, device, path) === true)
      active.add(id);
  }

  return active;
};

/** How deep a tool id is: 1 for a head, 2 for a slot of its feeder. */
const depthOf = (id: string): number => id.split('.').length;

/**
 * The tool the printer prints with now, such as `1` or `0.2`: the deepest
 * active tool whose tools above it are all active, as
 * `connectorapi.ActiveToolOf` derives it. There is no path for it.
 */
export const activeToolOf = (
  telemetry: {
    value(path: string): unknown;
    readonly paths?: readonly string[] | undefined;
  },
  device: Device,
): string | undefined => {
  const active = activeIdsOf(telemetry, device);
  let best: string | undefined;

  for (const id of active) {
    const parts = id.split('.');
    const above = parts
      .slice(1)
      .every((_, i) => active.has(parts.slice(0, i + 1).join('.')));
    if (!above) continue;

    if (
      best === undefined ||
      depthOf(id) > depthOf(best) ||
      (depthOf(id) === depthOf(best) && id < best)
    )
      best = id;
  }

  return best;
};

/**
 * The head the printer prints with now, counted from 0, or `undefined` when
 * none is picked or the printer does not say.
 */
export const activeHeadOf = (
  telemetry: {
    value(path: string): unknown;
    readonly paths?: readonly string[] | undefined;
  },
  device: Device,
): number | undefined => {
  const active = activeToolOf(telemetry, device);
  if (active === undefined) return undefined;

  const head = /^(0|[1-9]\d*)(\.|$)/.exec(active)?.[1];

  return head === undefined ? undefined : Number(head);
};

/**
 * The head a page shows when it shows one nozzle: the head printing now, and
 * head 0 when none is picked or the printer does not say.
 */
export const currentHeadOf = (
  telemetry: {
    value(path: string): unknown;
    readonly paths?: readonly string[] | undefined;
  },
  device: Device,
): number => activeHeadOf(telemetry, device) ?? 0;

/**
 * The material the printer prints with now, for a page that shows one: the
 * material of the tool printing now in the tree. A page that shows each tool
 * reads `toolsOf` instead. `undefined` is nothing loaded, or not known.
 */
export const activeMaterialOf = (
  telemetry: {
    value(path: string): unknown;
    readonly paths?: readonly string[] | undefined;
  },
  device: Device,
): string | undefined => {
  const active = activeToolOf(telemetry, device) ?? '0';
  const material = materialOf(telemetry, device, active);

  return material === '' ? undefined : material;
};

/**
 * The `tool` argument of the published nozzle commands counts from 0, as the
 * state tree and G-code do, so a head is sent as its own index.
 */
export const toolArgument = (index: number): string => String(index);

/**
 * The heads a printer reports, ordered by index. A head exists once any path
 * under it has been written, by the snapshot or by the live stream, and the
 * driver has not said it is switched off. Each reading takes the live value
 * first and the snapshot's second. A material the printer reports as an
 * empty word is a head with nothing loaded, and is left out here so a reader
 * shows its own word for it. A head with a feeder holds no material of its
 * own, so its material is the one of the slot it prints from now.
 */
export const toolsOf = (
  telemetry: {
    value(path: string): unknown;
    readonly paths?: readonly string[] | undefined;
  },
  device: Device,
): ToolReading[] => {
  const indexes = new Set<number>();
  const paths = [...Object.keys(device.state), ...(telemetry.paths ?? [])];

  for (const path of paths) {
    const match = HEAD_PATH.exec(path);
    if (match?.[1] !== undefined) indexes.add(Number(match[1]));
  }

  // The slot loaded into a head is the one whose active member is true. A
  // head that is not picked can have one too.
  const loaded = new Map<string, string>();
  for (const path of paths) {
    const match = SLOT_ACTIVE_PATH.exec(path);
    if (
      match?.[1] !== undefined &&
      match[2] !== undefined &&
      valueOf(telemetry, device, path) === true
    ) {
      loaded.set(match[2], match[1]);
    }
  }

  return [...indexes]
    .filter(
      (index) =>
        valueOf(telemetry, device, toolPath(String(index), 'enabled')) !==
        false,
    )
    .sort((left, right) => left - right)
    .map((index) => {
      const id = String(index);
      const temperature = numberOf(
        telemetry,
        device,
        toolPath(id, 'nozzle_temp'),
      );
      const target = numberOf(
        telemetry,
        device,
        toolPath(id, 'nozzle_target_temp'),
      );
      const picked = booleanOf(telemetry, device, toolPath(id, 'active'));
      const slot = loaded.get(id);
      const material =
        materialOf(telemetry, device, id) ??
        (slot === undefined ? undefined : materialOf(telemetry, device, slot));
      const diameter = numberOf(
        telemetry,
        device,
        toolPath(id, 'features/nozzle_diameter'),
      );
      const highFlow = booleanOf(
        telemetry,
        device,
        toolPath(id, 'features/nozzle_high_flow'),
      );
      const hardened = booleanOf(
        telemetry,
        device,
        toolPath(id, 'features/nozzle_hardened'),
      );

      return {
        index,
        active: picked ?? false,
        ...(temperature === undefined ? {} : { temperature }),
        ...(target === undefined || target === 0 ? {} : { target }),
        ...(material === undefined || material === '' ? {} : { material }),
        features: {
          ...(diameter === undefined ? {} : { diameter }),
          ...(highFlow === undefined ? {} : { highFlow }),
          ...(hardened === undefined ? {} : { hardened }),
        },
      };
    });
};
