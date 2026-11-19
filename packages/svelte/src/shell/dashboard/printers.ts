// A printer as a card reads it: the row the Printers screen draws, with the
// time left and the tags a dashboard filters by. Reading the printer through
// the same row keeps a card and the listing in agreement about every state.
import type { DeviceIssueEntry } from '../../data';
import type { Device, Reading } from '../../domain';
import { PRINTER_STATE_SEVERITY, readingsOf } from '../../domain';
import { printerRowOf } from '../printers/presentation';
import type { PrinterRow } from '../printers/types';

/** One printer, as every card reads it. */
export interface PrinterView extends PrinterRow {
  /** Seconds until the print ends, absent when nothing is printing. */
  readonly remaining: number | undefined;
  readonly tags: readonly string[];
  /** The readings the printer describes, for a chart of them. */
  readonly readings: readonly Reading[];
}

/**
 * The words of an issue's reason, from the server's vocabulary, or undefined
 * for a reason it does not list. The Printers screen reads the same one.
 */
export type IssueWords = (reason: string) => DeviceIssueEntry | undefined;

/**
 * Reads one printer for a card. With the vocabulary, an issue reads in its
 * words, the way the Printers screen shows it; without it, in the machine's.
 */
export const viewOf = (device: Device, issueOf?: IssueWords): PrinterView => {
  const row = printerRowOf(device, issueOf);
  const job = device.job;
  const left = job?.time_remaining_ms;

  return {
    ...row,
    remaining:
      job?.running === true && typeof left === 'number' && left > 0
        ? Math.round(left / 1000)
        : undefined,
    tags: device.tags ?? [],
    readings: readingsOf(device.state),
  };
};

/**
 * True when a printer needs attention: a state in the critical or attention
 * tier (an error, a question on its screen, a paused print), or an issue it
 * reports as a warning or an error. A finished or stopped printer is done, not
 * waiting on a person, as Prusa Connect counts it. An informational issue does
 * not count, and neither does a printer that is not reporting, whatever it
 * last said. The scope of a card of many, the issues card and the fleet
 * summary all count by this one rule, so the three agree on every wall.
 */
export const needsPerson = (view: PrinterView): boolean =>
  view.state !== 'OFFLINE' &&
  (view.tier === 'critical' ||
    view.tier === 'attention' ||
    view.severity === 'warning' ||
    view.severity === 'error');

/** The printers a tile may show: every one the fleet holds that is not retired. */
export const activeOf = (devices: readonly Device[]): Device[] =>
  devices.filter((device) => device.retired === undefined);

/**
 * True when a card of many printers shows this one: it is one of the printers
 * the person chose, or they chose none, and it carries the tag asked for.
 */
export const selected = (
  view: PrinterView,
  selection: { readonly devices: readonly string[]; readonly tag: string },
): boolean =>
  (selection.devices.length === 0 || selection.devices.includes(view.id)) &&
  tagged(view, selection.tag);

/** True when a printer carries a tag, or when no tag is asked for. */
export const tagged = (view: PrinterView, tag: string): boolean =>
  tag === '' ||
  view.tags.some((held) => held.toLowerCase() === tag.toLowerCase());

/** Every tag the fleet uses, once each, sorted. */
export const tagsOf = (devices: readonly Device[]): string[] => {
  const seen = new Map<string, string>();
  for (const device of devices) {
    for (const tag of device.tags ?? []) {
      const key = tag.toLowerCase();
      if (!seen.has(key)) seen.set(key, tag);
    }
  }

  return [...seen.values()].sort((a, b) => a.localeCompare(b));
};

/** The orders a list of printers can be read in. */
export type PrinterOrder = 'name' | 'state' | 'remaining';

export const PRINTER_ORDERS: readonly PrinterOrder[] = [
  'name',
  'state',
  'remaining',
];

/**
 * Sorts printers for a tile. `state` puts the worst first, the order a farm
 * reads them in. `remaining` puts the print that ends soonest first and every
 * printer that is not printing after them.
 */
export const ordered = (
  views: readonly PrinterView[],
  order: PrinterOrder,
): PrinterView[] => {
  const byName = (a: PrinterView, b: PrinterView): number =>
    a.name.localeCompare(b.name);

  return [...views].sort((a, b) => {
    if (order === 'state') {
      return (
        PRINTER_STATE_SEVERITY[a.state] - PRINTER_STATE_SEVERITY[b.state] ||
        byName(a, b)
      );
    }
    if (order === 'remaining') {
      const left = a.remaining ?? Number.POSITIVE_INFINITY;
      const right = b.remaining ?? Number.POSITIVE_INFINITY;

      return left - right || byName(a, b);
    }

    return byName(a, b);
  });
};
