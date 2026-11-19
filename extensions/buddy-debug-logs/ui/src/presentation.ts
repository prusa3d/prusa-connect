// How the page names and colours what the module answers. Plain functions,
// so a test computes them without drawing anything.
import type { BadgeVariant } from '@prusa/connect-ui/primitives';

import type { LogLine } from './api';

export type Severity = LogLine['severity'];

/** The levels a person filters by, most serious first. */
export const FILTER_LEVELS = ['error', 'warning', 'info', 'debug'] as const;
export type FilterLevel = (typeof FILTER_LEVELS)[number];

/** The look of a severity badge: only warnings and errors draw colour. */
export const severityVariant = (severity: Severity): BadgeVariant => {
  switch (severity) {
    case 'emergency':
    case 'alert':
    case 'critical':
    case 'error':
      return 'destructive';
    case 'warning':
      return 'warning';
    case 'notice':
    case 'info':
    case 'debug':
    case 'unknown':
      return 'outline';
  }
};

/** The time windows a person picks from, in hours. */
export const WINDOWS = [1, 6, 24] as const;
export type Window = (typeof WINDOWS)[number];

/** The start of a window that ends now, as the API spells an instant. */
export const sinceOf = (hours: number, now: number): string =>
  new Date(now - hours * 3_600_000).toISOString();

/**
 * Which printer a filter names: one printer, one MAC no printer is matched
 * to, or every source. Encoded as one string so a select can hold it.
 */
export type Origin =
  | { readonly kind: 'all' }
  | { readonly kind: 'device'; readonly id: string }
  | { readonly kind: 'mac'; readonly mac: string };

export const ALL: Origin = { kind: 'all' };

export const originValue = (origin: Origin): string => {
  switch (origin.kind) {
    case 'all':
      return 'all';
    case 'device':
      return `device:${origin.id}`;
    case 'mac':
      return `mac:${origin.mac}`;
  }
};

export const originOf = (value: string): Origin => {
  if (value.startsWith('device:')) {
    return { kind: 'device', id: value.slice('device:'.length) };
  }
  if (value.startsWith('mac:')) return { kind: 'mac', mac: value.slice(4) };

  return ALL;
};

/** The query keys an origin narrows a request by. */
export const originQuery = (
  origin: Origin,
): { device?: string; mac?: string } => {
  switch (origin.kind) {
    case 'all':
      return {};
    case 'device':
      return { device: origin.id };
    case 'mac':
      return { mac: origin.mac };
  }
};

/**
 * The host to write into a printer's settings: the host part of the address
 * printers dial, else the host this page was opened on. The firmware takes at
 * most 20 characters, so an IPv4 address fits and a long name may not.
 */
export const printerHost = (
  printerAddress: string | undefined,
  pageHost: string,
): string => {
  if (printerAddress !== undefined && printerAddress !== '') {
    try {
      return new URL(printerAddress).hostname;
    } catch {
      // Not an address. The page's own host is the next best guess.
    }
  }

  return pageHost;
};

/** The firmware's own limit on the host it stores. */
export const MAX_HOST = 20;
