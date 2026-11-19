import type { Snippet } from 'svelte';

import type { DeviceAdministration, DeviceDetail } from '../../../data';
import type { Device } from '../../../domain';
import type { CommandPort, TelemetrySource } from '../types';

/** What the panel reads of one printer's prints. */
export type PrinterHistorySource = Pick<
  DeviceDetail,
  'jobs' | 'jobsError' | 'jobsLoaded'
>;

/** One print, as the jobs operation answers it. */
export type PrintRow = NonNullable<
  NonNullable<PrinterHistorySource['jobs']>['current']
>;

/** What this tab reads of the store that renames, tags and retires a printer. */
export type PrinterAdministration = Pick<
  DeviceAdministration,
  'editTags' | 'error' | 'failed' | 'pending' | 'rename' | 'retire' | 'tags'
>;

/** What the prints card reads. */
export interface PrintHistoryProps {
  history: PrinterHistorySource;
  /** The prints that have ended, newest first, which is a read of its own. */
  recent?: readonly PrintRow[] | undefined;
  /** Where the whole of this printer's prints are listed. */
  jobsHref?: string | undefined;
  /**
   * The print on the bed as the fleet row reports it. Its progress follows the
   * machine, and the stored row's moves only when the page reads again.
   */
  live?: Pick<NonNullable<Device['job']>, 'id' | 'progress'> | undefined;
  /**
   * The picture of the plate behind one file on the printer, keyed by the path
   * the printer addresses it by. Absent for a file this server never sent.
   */
  thumbnailFor?: ((path: string) => string | undefined) | undefined;
}

export interface PrinterInfoPanelProps {
  /** The record. Absent until it arrives, and the panel waits in its own shape. */
  device?: Device | undefined;
  history: PrinterHistorySource;
  recent?: readonly PrintRow[] | undefined;
  jobsHref?: string | undefined;
  thumbnailFor?: ((path: string) => string | undefined) | undefined;
  /**
   * Renames, tags and retires the printer. Absent, the tab offers none of
   * them, and it offers none to a caller the record says may not administer.
   */
  administration?: PrinterAdministration | undefined;
  /** Called once the server has retired the printer. */
  onRemoved?: (() => void) | undefined;
  /** Takes the reader to this verb on the Advanced tab. */
  onPickCommand?: ((verb: string) => void) | undefined;
  /**
   * Who may use this printer. Drawn above the danger zone, and only for a
   * caller the record says may administer the printer.
   */
  access?: Snippet | undefined;
  /**
   * What the Tools table reads and sends: its material chips need the
   * telemetry, its temperature editors need the commands too.
   */
  telemetry?: TelemetrySource | undefined;
  commands?: CommandPort | undefined;
  /**
   * Draws the material of one tool in the Tools table, handed the tool's id
   * and the chip to fall back on. An extension that assigns materials
   * supplies it.
   */
  material?: Snippet<[string, Snippet]> | undefined;
}
