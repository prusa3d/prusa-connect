// One pairing at the printer's own screen, read through the SDK.
//
// The server polls the printer once a second, so this store reads the
// server's answer at the same pace while the pairing waits, and stops once it
// ended. The printer joining is not read here: the registration events the
// add-printer page already follows carry the approval and the arrival.
import { ApiError, type components, type PrusaConnect } from '@prusa/connect';
import type {
  PairingRequest,
  PairingState,
  PrinterPairingView,
} from '@prusa/connect-ui/shell';

type PrinterPairing = components['schemas']['PrinterPairing'];

/** What this store needs of the SDK (svelte-data.md). */
export type PrinterPairingClient = Pick<
  PrusaConnect,
  'cancelPrinterPairing' | 'getPrinterPairing' | 'startPrinterPairing'
>;

/** A timer, so a test runs the reads without waiting a second for each. */
export interface PollTimer {
  set(run: () => void, ms: number): unknown;
  clear(handle: unknown): void;
}

const browserTimer: PollTimer = {
  set: (run, ms) => setTimeout(run, ms),
  clear: (handle) => {
    clearTimeout(handle as ReturnType<typeof setTimeout>);
  },
};

const POLL_MS = 1000;

const viewOf = (row: PrinterPairing): PrinterPairingView => ({
  code: row.code,
  printer: row.printer,
  state: row.state,
});

const open = (state: PairingState): boolean =>
  state === 'waiting' || state === 'approved';

export class PrinterPairingStore {
  readonly #client: PrinterPairingClient;
  readonly #timer: PollTimer;
  #id: string | undefined;
  #pairing = $state.raw<PrinterPairingView | undefined>(undefined);
  #busy = $state(false);
  #error = $state<unknown>(undefined);
  #handle: unknown;
  // Counts the resets. A read that started before one records nothing.
  #generation = 0;

  constructor(client: PrinterPairingClient, timer: PollTimer = browserTimer) {
    this.#client = client;
    this.#timer = timer;
  }

  /** The pairing this page started, until the person moves on from it. */
  get pairing(): PrinterPairingView | undefined {
    return this.#pairing;
  }

  /** Whether a request is on its way to the server. */
  get busy(): boolean {
    return this.#busy;
  }

  /** The last refusal, as the SDK threw it. */
  get error(): unknown {
    return this.#error;
  }

  async start(request: PairingRequest): Promise<void> {
    if (this.#busy) return;
    this.reset();

    const generation = this.#generation;
    this.#busy = true;

    try {
      const started = await this.#client.startPrinterPairing({
        body: { printer: request.printer, connect: request.connect },
      });
      if (generation !== this.#generation) return;
      this.#id = started.id;
      this.#record(started, generation);
    } catch (error) {
      if (generation === this.#generation) this.#error = error;
    } finally {
      if (generation === this.#generation) this.#busy = false;
    }
  }

  async cancel(): Promise<void> {
    const id = this.#id;
    if (id === undefined) return;
    this.#stop();

    const generation = this.#generation;

    try {
      const cancelled = await this.#client.cancelPrinterPairing({
        path: { pairing_id: id },
      });
      if (generation === this.#generation) this.#pairing = viewOf(cancelled);
    } catch (error) {
      if (generation === this.#generation) this.#error = error;
    }
  }

  /** Forgets the pairing, so the form shows again. Nothing is sent. */
  reset(): void {
    this.#generation++;
    this.#stop();
    this.#id = undefined;
    this.#pairing = undefined;
    this.#error = undefined;
    this.#busy = false;
  }

  /** Stops reading. The page calls it when it goes away. */
  dispose(): void {
    this.#generation++;
    this.#stop();
  }

  #record(row: PrinterPairing, generation: number): void {
    const next = viewOf(row);
    const now = this.#pairing;
    // A read that changed nothing keeps the same object, so nothing that
    // reads the pairing draws again.
    if (now?.code !== next.code || now.state !== next.state) {
      this.#pairing = next;
    }
    if (open(next.state)) this.#schedule(generation);
  }

  #schedule(generation: number): void {
    this.#stop();
    this.#handle = this.#timer.set(() => {
      this.#handle = undefined;
      void this.#read(generation);
    }, POLL_MS);
  }

  async #read(generation: number): Promise<void> {
    const id = this.#id;
    if (id === undefined || generation !== this.#generation) return;

    try {
      const read = await this.#client.getPrinterPairing({
        path: { pairing_id: id },
      });
      if (generation !== this.#generation) return;
      this.#record(read, generation);
    } catch (error) {
      if (generation !== this.#generation) return;
      // A pairing the server no longer holds ended there. Any other failure
      // is a moment without an answer, and the next read may get one.
      if (error instanceof ApiError && error.status === 404) {
        this.#pairing =
          this.#pairing === undefined
            ? undefined
            : { ...this.#pairing, state: 'expired' };

        return;
      }
      this.#schedule(generation);
    }
  }

  #stop(): void {
    if (this.#handle !== undefined) {
      this.#timer.clear(this.#handle);
      this.#handle = undefined;
    }
  }
}
