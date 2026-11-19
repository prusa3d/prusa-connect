import type { Snippet } from 'svelte';
import type { StatusJob } from '../status/types';
import type { TelemetrySource } from '../types';
import type { DeviceCommands } from '../../../data';
import type {
  CommandProjection,
  Device,
  StateTransition,
  PrinterState,
} from '../../../domain';

/** What this tab reads of the command store. */
export type CommandsSource = Pick<
  DeviceCommands,
  | 'answer'
  | 'clear'
  | 'error'
  | 'limit'
  | 'loadPresets'
  | 'log'
  | 'pending'
  | 'presets'
  | 'refused'
  | 'runPreset'
  | 'send'
  | 'sendGcode'
  | 'sending'
>;

/** One verb the printer accepts, with the arguments its capability declares. */
export interface CommandVerb {
  /** The capability key as the wire spells it: `core.command.set_speed`. */
  verb: string;
  /** The argument names, from the `verb#name` entries of the same list. */
  args: readonly string[];
  /** Irreversible, so the send is confirmed first. */
  destructive: boolean;
}

/** The verbs a send cannot be taken back. */
const IRREVERSIBLE: ReadonlySet<string> = new Set([
  'core.command.delete_file',
  'core.command.delete_folder',
  'core.command.factory_reset',
  'core.command.flash',
  'core.command.reset',
  'core.command.reset_printer',
  'core.command.stop_print',
  'core.command.upgrade',
]);

/** The character the capability list puts between a verb and one argument. */
const ARGUMENT = '#';

/** The verbs a printer accepts, each with its arguments, sorted by the verb. */
export const verbsOf = (
  capabilities: Device['capabilities'],
): CommandVerb[] => {
  const args = new Map<string, string[]>();
  const verbs: string[] = [];

  for (const entry of capabilities ?? []) {
    const cut = entry.indexOf(ARGUMENT);
    if (cut === -1) {
      verbs.push(entry);
      continue;
    }

    const held = args.get(entry.slice(0, cut));
    const name = entry.slice(cut + 1);
    if (held === undefined) args.set(entry.slice(0, cut), [name]);
    else held.push(name);
  }

  return verbs.sort().map((verb) => ({
    verb,
    args: args.get(verb)?.sort() ?? [],
    destructive: IRREVERSIBLE.has(verb),
  }));
};

export type TuningArgument = 'speed' | 'flow';
export type TuningReadings = Record<TuningArgument, number | undefined>;

export interface CommandRowProps {
  tuning?: TuningReadings | undefined;
  entry: CommandVerb;
  projection: CommandProjection;
  /** The row the reader came here to find, while the mark lasts. */
  marked: boolean;
  /** A send is in flight, so no row offers a second one. */
  sending: boolean;
  /** The verb under way, which its row shows a spinner for. */
  pending?: string | undefined;
  /** The printer is reading its drive, which holds up the verb under way. */
  readingDrive?: boolean | undefined;
  onSend: (verb: string, args?: Readonly<Record<string, string>>) => void;
}

export interface PrinterCommandsPanelProps {
  /** The commands already available in the Controls panel above this one. */
  basicCommands?: ReadonlySet<string> | undefined;
  telemetry?: TelemetrySource;
  commands: CommandsSource;
  /** The record, once it has arrived. Its `capabilities` is the verb list. */
  device?: Device | undefined;
  /** The moves the device kind declared, from the state vocabulary. */
  transitions?: readonly StateTransition[] | undefined;
  /** The command the reader arrived here to find. The row is brought into view and marked. */
  highlight?: string | undefined;
  /** Held by another operator or a running command. Outranks state. */
  locked?: boolean | undefined;
  loading?: boolean | undefined;
  /** The printer is reading its drive, which holds up the verb under way. */
  readingDrive?: boolean | undefined;
}

/** One row of the catalogue: the verb, and what this printer says about it. */
export interface CommandListing {
  entry: CommandVerb;
  projection: CommandProjection;
}

export interface CommandCatalogueProps {
  embedded?: boolean;
  tuning?: TuningReadings | undefined;
  printerState?: PrinterState;
  activeJob?: Pick<StatusJob, 'name' | 'path' | 'progress'> | undefined;
  motion?: Snippet | undefined;
  rows: readonly CommandListing[];
  /** The verb whose row is lit, while the mark lasts. */
  marked: string | undefined;
  sending: boolean;
  pending?: string | undefined;
  readingDrive?: boolean | undefined;
  onSend: (verb: string, args?: Readonly<Record<string, string>>) => void;
}

export interface GcodeConsoleProps {
  commands: CommandsSource;
}
