// The readings of every printer a page charts, over one event stream.
//
// `LiveTelemetry` opens a stream per printer, which is right for a printer's
// own page. A dashboard may chart several printers at once, and a browser keeps
// about six connections to one server over HTTP/1.1, while the server holds a
// principal to 64 streams. So this store follows a set
// of printers through one subscription whose origins are that set, and opens
// it again when the set changes (ADR-20261008-sww rule 26).
//
// Each printer also gets the stored readings of the last hour, of the charted
// paths only, read once when it is first followed (ADR-20261008-sww rule 27).
//
// A class, because every chart of a page shares it (svelte-5.md). The client
// and the timers come in through the constructor, so a test drives it with
// no socket and no clock.
import type { EventSubscription, StreamEvent } from '@prusa/connect';

import {
  BED_PATH,
  BED_TARGET_PATH,
  CHAMBER_PATH,
  CHAMBER_TARGET_PATH,
  mergeSeries,
  NOZZLE_PATH,
  NOZZLE_TARGET_PATH,
  type TelemetryPoint,
} from '../domain';
import { DeviceSamples, SAMPLES_PAGE_LIMIT } from './device-samples.svelte';
import { type DataFault, faultOf } from './fault';
import { type HistoryClient, windowEndingNow } from './services/history';
import {
  sampleOf,
  TELEMETRY_EVENTS,
  type TelemetryClient,
} from './services/telemetry';
import { systemTimers, type Timers } from './timers';

/** The most printers one page follows at once (ADR-20261008-sww rule 26). */
export const SHARED_TELEMETRY_MAX_DEVICES = 8;

/** The paths a chart draws, and the only ones kept or read from history. */
export const CHARTED_PATHS: readonly string[] = [
  NOZZLE_PATH,
  NOZZLE_TARGET_PATH,
  BED_PATH,
  BED_TARGET_PATH,
  CHAMBER_PATH,
  CHAMBER_TARGET_PATH,
];

/** How many live points each path keeps: an hour of a reading every two seconds. */
const LIVE_POINTS = 1800;

/** How far back the stored history is read, in hours. */
const HISTORY_HOURS = 1;

/**
 * How long a change of the followed set waits before the stream opens again,
 * so a page drawing six charts at once opens one stream and not six.
 */
const REOPEN_DELAY_MS = 50;

/** What the store needs of the SDK. A `PrusaConnect` is one. */
export type SharedTelemetryClient = TelemetryClient & HistoryClient;

export interface SharedTelemetryOptions {
  /** The person the stream reads for, so a reconnect stays theirs. */
  readonly principal?: string;
  readonly timers?: Timers;
  /** The most printers followed at once. */
  readonly maxDevices?: number;
}

/**
 * The live points of one path of one printer. Each path has its own state, so
 * a frame redraws only the charts that read the path it grew, and not every
 * chart on the page.
 */
class LivePath {
  #points = $state.raw<readonly TelemetryPoint[]>([]);

  /** Oldest first, at most `LIVE_POINTS`. Read-only by design: frames add to it. */
  get points(): readonly TelemetryPoint[] {
    return this.#points;
  }

  add(point: TelemetryPoint): void {
    const held = this.#points;
    const next = held.slice(Math.max(0, held.length + 1 - LIVE_POINTS));
    next.push(point);
    this.#points = next;
  }
}

interface Followed {
  holders: number;
  readonly stored: DeviceSamples;
  /** One live series per charted path. A path no chart draws has none. */
  readonly live: Readonly<Record<string, LivePath>>;
}

const livePaths = (): Readonly<Record<string, LivePath>> =>
  Object.fromEntries(CHARTED_PATHS.map((path) => [path, new LivePath()]));

export class SharedTelemetry {
  readonly #client: SharedTelemetryClient;
  readonly #principal: string | undefined;
  readonly #timers: Timers;
  readonly #max: number;

  /** Which printers are followed, and how many charts hold each one. */
  // A card reads its points through each printer's live paths and stored
  // samples, which are reactive. The map only says who is followed, and a
  // card reads it only after it followed, so it is a plain map.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  readonly #followed = new Map<string, Followed>();
  /** How many printers are followed, as state, so a card past the limit sees a place free up. */
  #count = $state(0);
  #error = $state.raw<DataFault | undefined>(undefined);
  #subscription: EventSubscription | undefined;
  #reopen: number | undefined;
  /** The origins the open stream carries, to skip a reopen that changes nothing. */
  #origins = '';

  constructor(
    client: SharedTelemetryClient,
    options: SharedTelemetryOptions = {},
  ) {
    this.#client = client;
    this.#principal = options.principal;
    this.#timers = options.timers ?? systemTimers;
    this.#max = options.maxDevices ?? SHARED_TELEMETRY_MAX_DEVICES;
  }

  /** Why the stream ended, while it is down. */
  get error(): DataFault | undefined {
    return this.#error;
  }

  /**
   * How many printers are followed. A card that found the page full follows
   * again when this drops, and a test reads it for leaks.
   */
  get following(): number {
    return this.#count;
  }

  /**
   * Follows one printer for one more chart, and answers the call that lets it
   * go. A printer past the most a page follows answers undefined, and the
   * chart says so.
   */
  follow(deviceId: string): (() => void) | undefined {
    let held = this.#followed.get(deviceId);
    if (held === undefined) {
      if (this.#followed.size >= this.#max) return undefined;
      const stored = new DeviceSamples(this.#client, deviceId);
      void stored.load({
        ...windowEndingNow(HISTORY_HOURS),
        limit: SAMPLES_PAGE_LIMIT,
        metrics: CHARTED_PATHS,
      });
      held = { holders: 0, stored, live: livePaths() };
      this.#followed.set(deviceId, held);
      this.#count = this.#followed.size;
      this.#schedule();
    }
    held.holders += 1;

    let released = false;

    return () => {
      if (released) return;
      released = true;
      this.#release(deviceId);
    };
  }

  /** Every point of one path of one printer, stored and live, oldest first. */
  points(deviceId: string, path: string): readonly TelemetryPoint[] {
    const held = this.#followed.get(deviceId);
    const stored = held?.stored.series[path] ?? [];
    const live = held?.live[path]?.points ?? [];

    return mergeSeries(stored, live);
  }

  /** True while the stored readings of one printer are being read. */
  loading(deviceId: string): boolean {
    return this.#followed.get(deviceId)?.stored.loading ?? false;
  }

  /**
   * Applies one frame. Public so a test drives the store with no socket. A
   * frame about a printer nobody follows, or a path no chart draws, is
   * dropped. A frame changes the one series it belongs to.
   */
  apply(event: StreamEvent): void {
    const sample = sampleOf(event);
    if (sample === undefined || typeof sample.value !== 'number') return;
    const live = this.#followed.get(sample.deviceId)?.live[sample.path];
    if (live === undefined) return;

    const at = Date.parse(event.envelope?.time ?? '');
    live.add({ at: Number.isNaN(at) ? Date.now() : at, value: sample.value });
  }

  /** Closes the stream and forgets every printer, for a page going away. */
  close(): void {
    if (this.#reopen !== undefined) this.#timers.clear(this.#reopen);
    this.#reopen = undefined;
    this.#subscription?.close();
    this.#subscription = undefined;
    this.#origins = '';
    for (const held of this.#followed.values()) held.stored.abort();
    this.#followed.clear();
    this.#count = 0;
  }

  #release(deviceId: string): void {
    const held = this.#followed.get(deviceId);
    if (held === undefined) return;
    held.holders -= 1;
    if (held.holders > 0) return;

    held.stored.abort();
    this.#followed.delete(deviceId);
    this.#count = this.#followed.size;
    this.#schedule();
  }

  #schedule(): void {
    if (this.#reopen !== undefined) return;
    this.#reopen = this.#timers.set(() => {
      this.#reopen = undefined;
      this.#open();
    }, REOPEN_DELAY_MS);
  }

  #open(): void {
    const devices = [...this.#followed.keys()].sort();
    const origins = devices.map((id) => `device:${id}`);
    const key = origins.join('\n');
    if (key === this.#origins) return;

    this.#subscription?.close();
    this.#subscription = undefined;
    this.#origins = key;
    if (origins.length === 0) return;

    const subscription = this.#client.events({
      types: TELEMETRY_EVENTS,
      origins,
      ...(this.#principal === undefined ? {} : { principal: this.#principal }),
    });
    this.#subscription = subscription;
    this.#error = undefined;
    void this.#consume(subscription);
  }

  // The subscription reconnects a dropped connection itself, with a growing
  // wait, and resumes from the last event it saw. It throws only when the
  // server refused it, such as for a credential that expired. That is for a
  // person to see rather than for a retry loop, as in `LiveTelemetry`, and a
  // change of the printers followed opens the stream again.
  async #consume(subscription: EventSubscription): Promise<void> {
    try {
      for await (const event of subscription) this.apply(event);
    } catch (thrown) {
      if (this.#subscription === subscription) this.#error = faultOf(thrown);
    }
  }
}
