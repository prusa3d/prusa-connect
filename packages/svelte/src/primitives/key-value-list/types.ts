import type { ClassValue } from 'svelte/elements';

export interface KeyValueRow {
  id: string;
  /** The name of the reading: "Serial", "Enrolment". */
  label: string;
  /**
   * The reading itself, already formatted with its unit. A reading the page
   * could not learn is left absent, and the row says so rather than drawing a
   * zero.
   */
  value?: string | undefined;
  /** A key, a fingerprint or an address reads in the mono face. */
  mono?: boolean | undefined;
  /**
   * A mono reading that breaks at every line feed and inside a long token,
   * rather than being cut to one line.
   */
  wrap?: boolean | undefined;
  /** The row gets a copy button at its end. */
  copyable?: boolean | undefined;
  /** The reading's hierarchy: a strong reading is larger, a muted one smaller. */
  emphasis?: 'strong' | 'muted' | undefined;
}

export interface KeyValueListProps {
  rows: readonly KeyValueRow[];
  /** What a row with no value says. */
  absentText?: string;
  /** The width of the name column, where its names are long: "14rem". */
  labelWidth?: string | undefined;
  class?: ClassValue | undefined;
}
