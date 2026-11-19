import type { Snippet } from 'svelte';

import type { SendOutcome } from '../../data';

import type {
  Device,
  HomePreference,
  StateTransition,
  TelemetryPoint,
} from '../../domain';
import type { DetailTab } from '../../primitives';
import type { PrinterAdministration } from './info/types';

export type { TelemetryPoint };

/** The tabs of the printer's detail, in the order the strip draws them. */
export const PRINTER_DETAIL_TABS = [
  'status',
  'controls',
  'files',
  'cameras',
  'info',
  'history',
  'commands',
] as const;

export type PrinterDetailTabId = (typeof PRINTER_DETAIL_TABS)[number];

export const isPrinterDetailTab = (id: string): id is PrinterDetailTabId =>
  (PRINTER_DETAIL_TABS as readonly string[]).includes(id);

/** One reading, as the state tree spells it. */
export type TelemetryValue = number | string | boolean;

/**
 * The live readings, apart from the device record. `Fleet` keeps six paths
 * alive and the record is a snapshot, so the header's chips and the machine
 * tab read from here. `LiveTelemetry` satisfies it.
 */
export interface TelemetrySource {
  value(path: string): TelemetryValue | undefined;
  /** Every path seen so far. A source that cannot list them leaves it out. */
  readonly paths?: readonly string[] | undefined;
  points(path: string): readonly TelemetryPoint[];
}

/**
 * What the screen may ask the printer to do, and why it may not. The screen
 * projects a verb through `projectCommand` and never decides on its own.
 */
export interface CommandPort {
  /** The moves the device kind declared. */
  transitions?: readonly StateTransition[] | undefined;
  /** Held by another operator or a running command. Outranks state. */
  locked?: boolean | undefined;
  sending?: boolean | undefined;
  /** The verb under way, which its button shows a spinner for. */
  pending?: string | undefined;
  /** The printer is reading its drive, and answers nothing else until it ends. */
  readingDrive?: boolean | undefined;
  onCommand: (
    verb: string,
    args?: Readonly<Record<string, string>>,
    // eslint-disable-next-line @typescript-eslint/no-invalid-void-type -- Local story ports can omit acknowledgements.
  ) => void | Promise<SendOutcome>;
}

/**
 * The other printer, on an instance that holds exactly two. `side` is stable,
 * so the pair does not swap places when the reader switches.
 */
export interface PrinterPeer {
  device: Device;
  side: 'before' | 'after';
}

export interface PrinterSwitcherProps {
  /** Exactly two, in the fleet's own order. */
  printers: readonly Device[];
  chosenId: string;
  onChoose: (id: string) => void;
}

export interface PrinterDetailScreenProps {
  /** The printer the path names. Known before the record is. */
  deviceId: string;
  /** The record, once it has arrived. Until then the header waits. */
  device?: Device | undefined;
  telemetry: TelemetrySource;
  commands: CommandPort;
  /** How many printers this instance holds. Two is the one shape with a peer. */
  printerCount: number;
  preference?: HomePreference | undefined;
  peer?: PrinterPeer | undefined;
  onSelectPeer?: (() => void) | undefined;
  /** A core tab id, or `<namespace>.<id>` of an open module tab. */
  tab: string;
  onTabChange: (tab: string) => void;
  /** One panel per tab. A tab with none draws the skeleton. */
  panels?: Partial<Record<PrinterDetailTabId, Snippet | undefined>> | undefined;
  /** The tabs the manifests declare for `device-tab`, after the core's own. */
  moduleTabs?: readonly DetailTab[] | undefined;
  /** The header's action menu, drawn only once a manifest declares an entry. */
  actions?: Snippet | undefined;
  /** The material positions supplied by an installed extension. */
  filamentPositions?: Snippet<[Snippet]> | undefined;
  /** The printer's own question, drawn under the subject block. */
  prompt?: Snippet | undefined;
  /** What the server refused with. Rendered by `describeFault`. */
  fault?: unknown;
  loading?: boolean | undefined;
  /** Renames the printer from the pencil beside its name. */
  administration?: PrinterAdministration | undefined;
  /** Where the trail goes back to. */
  printersHref: string;
}
