// What the Status panel is handed, and the reading it draws.
import type { DeviceDetail, DeviceIssueEntry, Transfers } from '../../../data';
import type { Device, UploadBlock } from '../../../domain';
import type { CameraFeed } from '../../printer-cameras';
import type { SendsIndicatorRow } from '../../sends-indicator';
import type { SaveToStorageProps } from '../save-to-storage';
import type { PrinterHistorySource, PrintRow } from '../info/types';
import type { CommandPort, TelemetrySource } from '../types';

/** What the panel reads of the files on their way to this printer, and how it stops one. */
export type PrinterTransferSource = Pick<Transfers, 'inFlight' | 'stop'>;

/** One file on its way to the printer, as the store keeps it. */
export type PrinterTransferRow = PrinterTransferSource['inFlight'][number];

/** Which of the files the prints name the printer still holds. */
export type PrinterHeldFiles = Pick<DeviceDetail, 'checkHeld' | 'holds'>;

export interface PrinterStatusPanelProps {
  /** The printer's record. The panel reads the job and the capabilities off it. */
  device: Device;
  /** The live readings, which outrank the record for a path both carry. */
  telemetry: TelemetrySource;
  commands: CommandPort;
  issues?:
    { entryOf(reason: string): DeviceIssueEntry | undefined } | undefined;
  problemHref?: string | undefined;
  /** The moment the finish time is counted from. Absent, it is the moment the time left arrived. */
  now?: Date | undefined;
  /** The camera drawn beside a running print, with the live view it can play. */
  camera?: CameraFeed | undefined;
  loading?: boolean | undefined;
  /** The files on their way to this printer. */
  transfers?: PrinterTransferSource | undefined;
  /** The files this tab is still sending from the browser to this server, for this printer. */
  uploads?: readonly SendsIndicatorRow[] | undefined;
  /** Stops one of those uploads. */
  onCancelUpload?: ((id: string) => void) | undefined;
  /** What this printer has printed. */
  history?: PrinterHistorySource | undefined;
  /** The prints that have ended, newest first, which is a read of its own. */
  recent?: readonly PrintRow[] | undefined;
  /**
   * Which files the printer still holds. Print again is offered only for
   * those, as Prusa Connect does. Absent, it is offered for every file.
   */
  files?: PrinterHeldFiles | undefined;
  /** Where the whole history is, which is the Info tab. */
  historyHref?: string | undefined;
  /**
   * The picture of the plate behind one file on the printer, keyed by the path
   * the printer addresses it by. Absent for a file this server never sent.
   */
  thumbnailFor?: ((path: string) => string | undefined) | undefined;
  /** Takes the files a reader drops or picks. */
  onUpload?: ((files: File[]) => void) | undefined;
  /**
   * Why an upload cannot start, shown where the drop zone would be. Read only
   * while `onUpload` is absent.
   */
  uploadBlock?: UploadBlock | undefined;
  /**
   * Whether a dropped file is kept on this server, and where, drawn under
   * the drop zone. Absent, nothing is drawn and the host decides.
   */
  keep?: SaveToStorageProps | undefined;
  /** The printer is being asked again for its storages. */
  checkingStorages?: boolean | undefined;
  /** Asks the printer again for its storages. */
  onRecheck?: (() => void) | undefined;
}

export interface StatusJob {
  /** What the machine shows on its screen, or undefined while it has said nothing. */
  name: string | undefined;
  /** The file to print again, empty until the machine has named one. */
  path: string | undefined;
  /** Whole percent, or null when the printer has reported none. */
  progress: number | null;
  /** Seconds the machine says are left, or undefined when it says nothing. */
  remainingSeconds: number | undefined;
  /** Seconds the machine says it has been printing. */
  elapsedSeconds: number | undefined;
}

/** Five shapes for twelve states. */
export type StatusVariant =
  'running' | 'trouble' | 'starter' | 'finished' | 'notReporting';
