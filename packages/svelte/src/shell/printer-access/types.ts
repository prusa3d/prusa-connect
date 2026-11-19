import type { Snippet } from 'svelte';

import type { AccessGrants, Account, FleetListing } from '../../data';
import type { AccessChoice, AccessState, OrgRole } from '../../domain';
import type { ListingStore, PagerProps } from '../../primitives';

/** One person on one printer, as a row draws it. */
export interface AccessRow {
  key: string;
  deviceId: string;
  subject: string;
  /** The name a row leads with: a username, or a printer's name. */
  primary: string;
  /** A username is drawn in the mono face, as People draws it. */
  mono: boolean;
  /** The facts under the name, already in words. */
  details: readonly string[];
  /** What the row's controls are labelled with: the person, or the printer. */
  name: string;
  orgRole: OrgRole;
  state: AccessState;
  busy: boolean;
}

/** What a list of rows reads and asks of the grants store. */
export type AccessGrantsSource = Pick<
  AccessGrants,
  | 'busy'
  | 'direct'
  | 'error'
  | 'load'
  | 'loaded'
  | 'loading'
  | 'setChoice'
  | 'setOn'
>;

/** What the printer page's card reads of the people on this server. */
export interface AccessPeopleSource {
  readonly items: readonly Account[];
  readonly loaded: boolean;
  readonly loading: boolean;
  readonly error: unknown;
  load(): Promise<void>;
}

export interface AccessRowProps {
  row: AccessRow;
  onOn: (row: AccessRow, on: boolean) => void;
  onChoice: (row: AccessRow, choice: AccessChoice) => void;
}

export interface AccessListProps {
  /** Uses the settings panel frame on a person's access page. */
  settingsPanel?: boolean | undefined;
  title: string;
  description: string;
  rows: readonly AccessRow[];
  /** True until the first read of every list the rows need has answered. */
  waiting: boolean;
  /** A refused read or write, drawn above the rows. */
  fault: unknown;
  search: string;
  onSearch: (next: string) => void;
  searchLabel: string;
  /** What the empty list says, with and without a search. */
  empty: { heading: string; note?: string | undefined };
  pager?: PagerProps | undefined;
  onOn: AccessRowProps['onOn'];
  onChoice: AccessRowProps['onChoice'];
  /** Drawn instead of the rows, for a list that has nothing to change. */
  instead?: Snippet | undefined;
}

export interface PrinterAccessCardProps {
  deviceId: string;
  /** What the caller may do on this printer, from the printer's record. */
  callerActions: readonly string[] | undefined;
  people: AccessPeopleSource;
  grants: AccessGrantsSource;
  /** The signed-in subject, whose own row is locked. */
  self: string | undefined;
  /** Where the page size is kept. A test hands in a map. */
  store?: ListingStore;
}

/** What the person page reads of the server's page of the fleet. */
export type AccessPrintersSource = Pick<
  FleetListing,
  'items' | 'loaded' | 'query' | 'setQuery' | 'total'
>;

export interface PersonAccessScreenProps {
  /** The person, absent until the accounts were read or when there is none. */
  account: Account | undefined;
  /** Tells "no such person" from "not read yet". */
  accountsLoaded: boolean;
  printers: AccessPrintersSource;
  grants: AccessGrantsSource;
  self: string | undefined;
  peopleHref: string;
}
