// What a file on a printer's drive is.
import { isPrintFile } from '../../../domain';

import type { PrinterFileEntry } from './types';

export const SPLIT_ROW =
  'flex flex-col @4xl/printer-files:flex-row @4xl/printer-files:items-stretch';

export const SPLIT_LEADING =
  'order-first border-b border-border pb-6 @4xl/printer-files:w-(--file-upload-pane-width) @4xl/printer-files:max-w-2/5 @4xl/printer-files:min-w-0 @4xl/printer-files:border-b-0 @4xl/printer-files:pb-0 @4xl/printer-files:pe-panel-padding';

export const SPLIT_TRAILING =
  'border-border pt-6 @4xl/printer-files:border-s @4xl/printer-files:pt-0 @4xl/printer-files:ps-panel-padding';

/** A firmware image the printer can write to itself. */
const FIRMWARE = /\.bbf$/i;
/** A model the printer stores and cannot start. */
const MODEL = /\.(?:3mf|stl|obj)$/i;

export const nameOf = (entry: PrinterFileEntry): string =>
  entry.display_name ?? entry.name;

export const isPrintable = (entry: PrinterFileEntry): boolean =>
  !entry.folder && isPrintFile(nameOf(entry));

/** Whether a file of this name is a firmware image. */
export const isFirmwareName = (name: string): boolean => FIRMWARE.test(name);

export const isFirmware = (entry: PrinterFileEntry): boolean =>
  !entry.folder && isFirmwareName(nameOf(entry));

export const isModel = (entry: PrinterFileEntry): boolean =>
  !entry.folder && MODEL.test(nameOf(entry));
