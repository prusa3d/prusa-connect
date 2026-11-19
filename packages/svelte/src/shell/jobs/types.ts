import type { components } from '@prusa/connect';
import type { Snippet } from 'svelte';

import type {
  Device,
  PrintAction,
  PrinterState,
  StateTransition,
} from '../../domain';
import type { ListingStore } from '../../primitives';

/** A column of the listing. A reader puts any of them away. */
export type JobFieldId =
  | 'preview'
  | 'file'
  | 'printer'
  | 'state'
  | 'started'
  | 'duration'
  | 'progress';

/** One print, as the core answers it. */
export interface JobRecord {
  /** The print's identity, a UUID the core gives each print. */
  id: string;
  /** What the machine shows on its screen for the file. */
  display_name: string;
  path: string;
  /** The machine's own word. Open: the states belong to the device kind. */
  state: string;
  /** Whole percent when the row was last written. */
  progress: number;
  /** When this server first saw the print. */
  started: string;
  /** When the print stopped. Absent while it runs, which is what tells them apart. */
  ended?: string;
  /** The reported outcome of an ended print. */
  outcome?: components['schemas']['DeviceJob']['outcome'];
}

/** One printer and what it has printed. */
export interface JobsPrinter {
  id: string;
  name: string;
  jobs: readonly JobRecord[] | undefined;
}

export interface JobsSource {
  /** Every printer, whether or not it has printed. The filter offers them all. */
  printers: readonly JobsPrinter[];
  /** True while a read is in flight. */
  loading: boolean;
  /** True once a read has answered, which tells an empty list from an unread one. */
  loaded: boolean;
  /** The refusal, as it was thrown. Drawn through `describeFault`. */
  error: unknown;
}

/** A print as the listing draws it: the record flattened onto its printer. */
export interface JobRow {
  id: string;
  fileName: string;
  path: string;
  state: PrinterState;
  printerId: string;
  printerName: string;
  /** Whole percent, as the machine last reported it. */
  progress: number;
  /** When the print started, as the core spells it. */
  started: string;
  /** How long it ran, in seconds. Absent while it is still running. */
  durationSeconds: number | undefined;
  running: boolean;
  detailHref: string;
  printerHref: string;
}

/** The columns a reader may order the list by. */
export type JobSortColumn =
  'file' | 'printer' | 'state' | 'started' | 'duration' | 'progress';

export interface JobSort {
  column: JobSortColumn;
  ascending: boolean;
}

/** Which values each dimension is narrowed to. Empty means every value. */
export interface JobFacetSelection {
  state: readonly string[];
  printer: readonly string[];
}

export interface JobsScreenProps {
  jobs: JobsSource;
  store?: ListingStore;
  /** Every printer this server holds. Print again offers the one that ran the print. */
  devices?: readonly Device[] | undefined;
  /** The moves between states the device kinds declared, for the print dialog. */
  transitions?: readonly StateTransition[] | undefined;
  /** The printers held by another operator or by a command still running. */
  locked?: readonly string[] | undefined;
  /** Starts a print. Without it no row offers Print again. */
  onPrint?: PrintAction | undefined;
  /** Called with the printers' ids once every one of them has taken the file. */
  onPrinted?: ((deviceIds: readonly string[]) => void) | undefined;
  /** Where the picture inside a row's file is served from. Without it every row draws the glyph. */
  thumbnailFor?: ((row: JobRow) => string | undefined) | undefined;
  /** A line under the heading, such as how much of the list is shown. */
  notice?: Snippet | undefined;
}
