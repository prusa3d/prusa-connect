import type { Snippet } from 'svelte';

import type { PrinterStorages, Storage } from '../../data';
import type { Device, PrintAction, StateTransition } from '../../domain';
import type { ListingStore } from '../../primitives';

/** What this screen reads of the server's own files. */
export type StorageSource = Pick<
  Storage,
  | 'browseFolders'
  | 'cancelUpload'
  | 'clearUploads'
  | 'createFolder'
  | 'contentUrl'
  | 'describe'
  | 'describedFile'
  | 'directory'
  | 'entries'
  | 'error'
  | 'filesError'
  | 'filesLoaded'
  | 'filesLoading'
  | 'load'
  | 'loaded'
  | 'loading'
  | 'moveAll'
  | 'open'
  | 'pickerError'
  | 'pickerFolders'
  | 'pickerPath'
  | 'remove'
  | 'storage'
  | 'storages'
  | 'upload'
  | 'uploads'
>;

/** One file or folder of a storage, as the core answers it. */
export type StorageRow = StorageSource['entries'][number];

/** A file row's own acts, for the one menu the row has. */
export interface StorageRowFold {
  /** The acts, as menu items. */
  items: Snippet;
  /**
   * Shows the menu only where the acts fold off the row, for a menu that
   * holds nothing else.
   */
  shown: string;
}

/** One storage the server declares, with what it allows. */
export type StorageChoice = StorageSource['storages'][number];

/** One file on its way up, as the store tracks it. */
export type StorageUpload = StorageSource['uploads'][number];

/** A column of the listing. A reader puts any of them away. */
export type StorageFieldId = 'modified' | 'name' | 'size';

/**
 * Which folders of this server are shared with the servers below it, and the
 * switch that changes that. A shared folder is shared with everything in it.
 */
export interface FolderSharing {
  /**
   * The shared folder that holds `path` on `storage`: `path` itself when it is
   * shared, a folder above it, or undefined when no share covers it.
   */
  sharedRoot: (storage: string, path: string) => string | undefined;
  /** False for a person who may see the shares but not change them. */
  readonly canChange: boolean;
  /** Shares one folder, or stops sharing it. Rejects with the server's fault. */
  share: (storage: string, path: string, shared: boolean) => Promise<void>;
}

/** One folder of one storage, as an address names it. Empty is the storage's root. */
export interface StoragePlace {
  readonly storage: string;
  readonly directory: string;
}

export interface StorageScreenProps {
  storage: StorageSource;
  /**
   * The folder the page's address names. Without one the screen opens the
   * folder the store already holds, or the first storage.
   */
  address?: StoragePlace | undefined;
  /**
   * Called when the reader opens another folder or storage, so the host moves
   * the address. Without it the screen opens the folder itself.
   */
  onNavigate?: ((place: StoragePlace) => void) | undefined;
  /** Where one file's own page is, on the storage that holds it. */
  fileHref?: ((storage: string, row: StorageRow) => string) | undefined;
  /**
   * Drawn at the end of each file's row in the listing, with the storage that
   * holds the file and the row's own acts, which the menu drawn here holds
   * so the row has one menu: the app puts the modules' `file-action` menu
   * here. A folder's row draws nothing.
   */
  fileActions?:
    Snippet<[string, StorageRow, StorageRowFold | undefined]> | undefined;
  /**
   * The folders this server shares with the servers below. With it the table
   * has a Shared column; without it the screen is the same as on a server
   * that shares nothing.
   */
  sharing?: FolderSharing | undefined;
  /** Where the field and view choices are kept. A test hands in a map. */
  store?: ListingStore;
  /** Every printer this server holds, for the print sheet. */
  devices?: readonly Device[];
  /** The moves between states the device kinds declared. */
  transitions?: readonly StateTransition[] | undefined;
  /** The printers held by another operator or by a command still running. */
  locked?: readonly string[] | undefined;
  /**
   * What each printer said its storages are. Opening the picker asks the
   * printers again, and one with no USB drive is offered with that reason.
   */
  printerStorages?: Pick<PrinterStorages, 'check' | 'noStorage'> | undefined;
  /** Sends one file to one printer. */
  onPrint?: PrintAction | undefined;
  /** Called with the printers' ids once every one of them has taken the file. */
  onPrinted?: ((deviceIds: readonly string[]) => void) | undefined;
}
