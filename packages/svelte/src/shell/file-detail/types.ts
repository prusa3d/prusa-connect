import type { Snippet } from 'svelte';

import type { FileDescription } from '../../data';
import type { DetailTab } from '../../primitives';

/** One file, wherever it is kept. */
export interface FileDetailEntry {
  /** What the storage addresses the file by. On a FAT drive this is the short name. */
  name: string;
  /** The path inside its root, "/"-separated. It is what every act here takes. */
  path: string;
  size: number;
  /** When it last changed, RFC 3339. Absent from a board that reported no timestamp. */
  modified?: string | undefined;
  /** The name written for a person, where the printer reports a second one. */
  display_name?: string | undefined;
  /** The printer refuses to change this file. */
  read_only?: boolean | undefined;
  /** The digest the storage took when the file was written. Empty where it kept none. */
  sha256?: string | undefined;
}

/** What may be done to the file. */
export interface FileActs {
  /** Sends the file to a printer. Absent where nothing takes it. */
  print?: (() => void) | undefined;
  /** Where the bytes are. Absent where this core serves no content route for the root. */
  downloadHref?: string | undefined;
  /** Absent for a file this instance may not write to. */
  remove?: (() => void) | undefined;
  /** Takes the new name. Absent for a file this instance may not write to. */
  rename?: ((name: string) => void) | undefined;
  /** An act is already on its way, which is what stops a second press. */
  sending?: boolean | undefined;
}

/** One print that named this file. */
export interface FilePrint {
  id: string;
  /** When the print started, RFC 3339. */
  started: string;
  /** The machine's own word, read through the shared state vocabulary. */
  state: string;
  printerName: string;
  /** The machine's own model string. Anything unrecognised draws the unknown one. */
  printerModel?: string | undefined;
  href: string;
  printerHref: string;
  /** How long it ran. Absent while it is still on the bed. */
  durationSeconds?: number | undefined;
  /** When the print ended, RFC 3339. Absent while it is still on the bed. */
  ended?: string | undefined;
  /** How an ended print ended, as the core records it. */
  outcome?: string | undefined;
}

export interface FileDetailScreenProps {
  /** The path the route names. The page has a subject before the record lands. */
  path: string;
  /** The file, once the listing it sits in has answered. */
  entry?: FileDetailEntry | undefined;
  /** What holds the file, as a person reads it: "Storage", "USB Drive". */
  rootLabel: string;
  /** Where the root crumb leads: the storage listing, or the printer's files. */
  rootHref: string;
  /** Where a folder above the file leads. */
  folderHref: (path: string) => string;
  acts?: FileActs | undefined;
  /**
   * Every print that recorded this file's name, newest first. Absent where
   * the app lists none, which leaves the Prints tab out.
   */
  prints?: readonly FilePrint[] | undefined;
  /**
   * What the file turned out to hold, once the server has opened it. Undefined
   * while that read is on its way, which is what separates "not asked yet"
   * from "asked, and the file said nothing".
   */
  description?: FileDescription | undefined;
  /** What the server refused with. Rendered by `describeFault`. */
  fault?: unknown;
  loading?: boolean | undefined;
  /**
   * Draws the file's action menu in place of the screen's own, handed the
   * screen's items, or undefined when the screen has none to offer. The app
   * uses it to add the modules' `file-action` entries to the same menu.
   */
  menu?: Snippet<[Snippet | undefined]> | undefined;
  /**
   * Draws what the file holds in place of the screen's own part, handed that
   * part to fall back on. The app uses it for a module's `file-view`
   * (ADR-20261027-wvj rule 16). It draws in the Contents tab, with no card
   * around it (ADR-20261029-dgv). The header, the actions and the other tabs
   * stay the screen's.
   */
  contents?: Snippet<[Snippet]> | undefined;
  /**
   * The open tab: `contents`, `prints`, `details`, or `<namespace>.<id>` of a
   * module tab. Absent, the screen keeps it. A tab not on the page opens the
   * contents.
   */
  tab?: string | undefined;
  /** Told which tab a person opened, so the app can keep it in the address. */
  onTabChange?: ((tab: string) => void) | undefined;
  /** The tabs the manifests declare for `file-tab`, after the screen's own. */
  moduleTabs?: readonly DetailTab[] | undefined;
}
