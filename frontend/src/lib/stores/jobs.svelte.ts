// Every printer's prints, gathered for the Jobs page.
//
// The core answers the prints of the whole fleet in one paged read,
// `GET /api/v1/jobs`, over the printers this person may observe. This store
// reads every running print, then a bounded number of pages of ended ones, and
// hands the rows to the screen grouped by printer. A store, because the result
// is server state shared by the screen and its filters (svelte-data.md); an
// application store rather than a library one, because the grouping is this
// product's shape for the screen and not one the library declares.
import type { PrusaConnect, StreamEvent } from '@prusa/connect';
import { DEVICE_JOB_EVENTS, faultOf } from '@prusa/connect-ui/data';
import type { Device, ReadonlyFleet } from '@prusa/connect-ui/domain';
import type { JobRecord, JobsPrinter } from '@prusa/connect-ui/shell';

/** How many prints one page of the read asks for. It is the core's cap. */
export const JOBS_PAGE = 200;

/**
 * How many pages of ended prints one read takes. A farm that prints every day
 * holds more prints than a page needs to show, and the page says when the
 * list is cut.
 */
export const JOBS_PAGES = 5;

/**
 * How many pages of running prints one read takes. A printer runs one print at
 * a time, so this is the fleet of four hundred five times over: the running
 * prints are read whole, and a long print never drops below newer endings.
 */
const RUNNING_PAGES = 10;

/**
 * How long a burst of starts and ends is gathered before one read follows it.
 * A farm ends prints in bursts, and each read is up to fifteen requests.
 */
export const JOBS_SETTLE_MS = 1000;

/**
 * How long a refused read waits before an event may try again. A read that
 * failed is not retried on every frame of a busy stream.
 */
export const JOBS_RETRY_MS = 30_000;

// A gap in the stream may have swallowed a start or an end.
const CONTINUITY_LOST = 'com.prusa.connect.stream.continuity.lost';

/** What this store needs of the SDK, and no more. */
export type JobsClient = Pick<PrusaConnect, 'listJobs'>;

/** What the store reads of a fleet: the printers, how the read went, its events. */
export type JobsFleet = Pick<
  ReadonlyFleet,
  'devices' | 'error' | 'loaded' | 'loading'
> & {
  subscribe(listener: (event: StreamEvent) => void): () => void;
};

/** One print as the fleet listing answers it, which names its printer. */
type FleetJob = JobRecord & { device_id: string };

/**
 * A running print takes the progress of the fleet row, which follows the
 * machine's telemetry; the stored row moves only when this store reads again.
 */
const recordOf = (job: FleetJob, live: number | undefined): JobRecord => ({
  id: job.id,
  display_name: job.display_name,
  path: job.path,
  state: job.state,
  progress: live ?? job.progress,
  started: job.started,
  ...(job.ended === undefined ? {} : { ended: job.ended }),
  ...(job.outcome === undefined ? {} : { outcome: job.outcome }),
});

/**
 * A shared store, so a class (svelte-5.md). The client and the fleet come in
 * through the constructor, which lets a test build one without a network.
 *
 * The rows are `$state.raw` and replaced as a whole: a farm's prints are
 * thousands of rows, and deep proxying each of them would buy nothing
 * (svelte-data.md).
 */
export class Jobs {
  readonly #client: JobsClient;
  readonly #fleet: JobsFleet;

  #rows = $state.raw<readonly FleetJob[]>([]);
  #reading = $state(false);
  #read = $state(false);
  #cut = $state(false);
  #fault = $state<unknown>(undefined);
  #printer = $state<string | undefined>(undefined);
  /** The printer the last read covered, so the same read is not made twice. */
  #covered: string | undefined;
  /** True when a print may have started or ended since the last read began. */
  #stale = false;
  #flight: Promise<void> | undefined;
  /** The number of the read started last. Only that read records its answer. */
  #latest = 0;
  /** The read a burst of events has asked for, waiting for the burst to end. */
  #settling: ReturnType<typeof setTimeout> | undefined;
  /** When the last read was refused, so events do not retry it at once. */
  #refusedAt = 0;
  readonly #settleMs: number;
  readonly #retryMs: number;
  readonly #now: () => number;

  constructor(
    client: JobsClient,
    fleet: JobsFleet,
    timing: { settleMs?: number; retryMs?: number; now?: () => number } = {},
  ) {
    this.#client = client;
    this.#fleet = fleet;
    this.#settleMs = timing.settleMs ?? JOBS_SETTLE_MS;
    this.#retryMs = timing.retryMs ?? JOBS_RETRY_MS;
    this.#now = timing.now ?? Date.now;
  }

  /** The one printer the page lists, or undefined when it lists them all. */
  get printer(): string | undefined {
    return this.#printer;
  }

  /** True when there are more ended prints than one read takes. */
  get cut(): boolean {
    return this.#cut;
  }

  /** How many prints the page holds. */
  get shown(): number {
    return this.#rows.length;
  }

  /**
   * Every printer the page lists, with what it has printed. Every printer is
   * listed, so the filter offers it; one with no print carries an empty list
   * once the read has answered. A print on a printer the fleet no longer
   * lists is grouped under the printer's id.
   */
  get printers(): readonly JobsPrinter[] {
    const printer = this.#printer;
    const devices =
      printer === undefined
        ? this.#fleet.devices
        : this.#fleet.devices.filter((device) => device.id === printer);
    const read = this.#read;

    // Plain records built on every read of this getter: the rows and the
    // fleet are the state, and this only groups them.
    const grouped: Record<string, JobRecord[]> = {};
    for (const device of devices) grouped[device.id] = [];

    const byId: Record<string, Device> = {};
    for (const device of this.#fleet.devices) byId[device.id] = device;

    for (const job of this.#rows) {
      const current = byId[job.device_id]?.job;
      const live =
        job.ended === undefined && current?.id === job.id
          ? current.progress
          : undefined;
      const list = grouped[job.device_id] ?? [];
      list.push(recordOf(job, live));
      grouped[job.device_id] = list;
    }

    return Object.entries(grouped).map(([id, jobs]) => ({
      id,
      name: byId[id]?.name ?? id,
      jobs: read ? jobs : undefined,
    }));
  }

  get loading(): boolean {
    return this.#fleet.loading || this.#reading;
  }

  /** True once both the fleet and the prints have answered. */
  get loaded(): boolean {
    return this.#fleet.loaded && this.#read;
  }

  /** The refusal that left the page with no prints to list. */
  get error(): unknown {
    return this.#fault ?? this.#fleet.error;
  }

  /**
   * Reads the prints again whenever one starts or ends, off the events the
   * fleet applies, while a screen shows them. The returned function stops
   * listening and marks the rows to be read again on the next visit, because
   * nothing follows the prints while no screen does.
   */
  watch(fleet: Pick<JobsFleet, 'subscribe'>): () => void {
    const stop = fleet.subscribe((event) => {
      if (
        !DEVICE_JOB_EVENTS.includes(event.type) &&
        event.type !== CONTINUITY_LOST
      ) {
        return;
      }
      if (!this.#read && this.#flight === undefined) return;

      this.#stale = true;
      if (
        this.#fault !== undefined &&
        this.#now() - this.#refusedAt < this.#retryMs
      ) {
        return;
      }
      // One read in flight, at most one queued behind it (load's follow-up),
      // and a burst of frames gathered into one.
      if (this.#flight !== undefined || this.#settling !== undefined) return;
      this.#settling = setTimeout(() => {
        this.#settling = undefined;
        void this.load(this.#printer);
      }, this.#settleMs);
    });

    return () => {
      stop();
      clearTimeout(this.#settling);
      this.#settling = undefined;
      this.#stale = true;
    };
  }

  /**
   * Forgets every print this store read and the printer the page named. The
   * app calls it when the person signed in changes, so the next person never
   * sees what the last one could read. A read that started before the reset
   * records nothing.
   */
  reset(): void {
    clearTimeout(this.#settling);
    this.#settling = undefined;
    this.#latest += 1;
    this.#flight = undefined;
    this.#covered = undefined;
    this.#rows = [];
    this.#reading = false;
    this.#read = false;
    this.#cut = false;
    this.#stale = false;
    this.#fault = undefined;
    this.#printer = undefined;
  }

  /**
   * Reads the prints of every printer, or of the one printer named. A second
   * call for the same printer while a read is in flight joins it, and a call
   * for prints already read does nothing until a print starts or ends.
   */
  load(printer?: string): Promise<void> {
    this.#printer = printer;
    if (this.#flight !== undefined && this.#covered === printer) {
      return this.#flight;
    }
    if (this.#read && !this.#stale && this.#covered === printer) {
      return Promise.resolve();
    }

    this.#covered = printer;
    this.#stale = false;
    this.#latest += 1;
    const read = this.#latest;
    // A print that starts or ends while this read is on its way may be
    // missing from its answer, so one more read follows it, and never more
    // than one however many frames arrived meanwhile.
    const flight: Promise<void> = this.#walk(printer, read).then(async () => {
      if (this.#flight === flight) this.#flight = undefined;
      // A refused read is not followed: the events wait out the retry pause.
      if (this.#stale && this.#latest === read && this.#fault === undefined) {
        await this.load(printer);
      }
    });
    this.#flight = flight;

    return flight;
  }

  // Up to `pages` pages of one phase. Answers the rows and whether more were
  // left unread.
  async #phase(
    printer: string | undefined,
    phase: 'running' | 'ended',
    pages: number,
  ): Promise<{ rows: FleetJob[]; more: boolean }> {
    const rows: FleetJob[] = [];
    let cursor = '';
    for (let page = 0; page < pages; page += 1) {
      const answer = await this.#client.listJobs({
        query: {
          phase,
          limit: JOBS_PAGE,
          ...(printer === undefined ? {} : { device_id: printer }),
          ...(cursor === '' ? {} : { cursor }),
        },
      });
      rows.push(...(answer.items ?? []));
      cursor = answer.cursor;
      if (cursor === '') return { rows, more: false };
    }

    return { rows, more: true };
  }

  // A read the page has replaced records nothing, so a slow answer never
  // overwrites the answer for the printer the page shows now.
  async #walk(printer: string | undefined, read: number): Promise<void> {
    this.#reading = true;
    try {
      const running = await this.#phase(printer, 'running', RUNNING_PAGES);
      const ended = await this.#phase(printer, 'ended', JOBS_PAGES);

      if (read !== this.#latest) return;

      // A print that ended between the two reads is in both. An end is
      // final, so the ended copy is the one kept.
      // A plain local set, read once here and never rendered.
      // eslint-disable-next-line svelte/prefer-svelte-reactivity
      const endedIds = new Set(ended.rows.map((job) => job.id));
      this.#rows = [
        ...running.rows.filter((job) => !endedIds.has(job.id)),
        ...ended.rows,
      ];
      this.#cut = ended.more;
      this.#read = true;
      this.#fault = undefined;
    } catch (thrown) {
      if (read === this.#latest) {
        this.#fault = faultOf(thrown);
        this.#refusedAt = this.#now();
      }
    } finally {
      if (read === this.#latest) this.#reading = false;
    }
  }
}
