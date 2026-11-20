// The cameras a printer declares, and the addresses a pane loads a frame from.
import type { Camera, CamerasClient } from './services/cameras';
import {
  declaredIntervalMs,
  pushUrl,
  watchCamera,
  readCameras,
  snapshotUrl,
  openSocket,
  streamUrl,
} from './services/cameras';
import { type DataFault, faultOf } from './fault';
import { CAMERA_FRAME_EVENT, deviceIdOf, type Fleet } from './fleet.svelte';
import { systemTimers, type Timers } from './timers';

/** How the store is built beyond its client. */
export interface CamerasOptions {
  /**
   * The wait between still frames, in milliseconds, for a camera that declares
   * none of its own. The floor still applies to it.
   */
  readonly refreshMs?: number;
  readonly timers?: Timers;
}

const DEFAULT_REFRESH_MS = 2000;

/**
 * The fastest a fleet screen asks, whatever a camera says it can do. Every
 * card on the screen loads a frame on the same tick, so a fleet of four
 * hundred is four hundred requests at once and the floor is what bounds them.
 */
export const FLEET_REFRESH_FLOOR_MS = 1000;

/**
 * The fastest one device's page asks. It draws one camera and the person is
 * watching it, so it may run at twice the fleet's rate.
 */
export const DEVICE_REFRESH_FLOOR_MS = 500;

/**
 * How long a camera that announces its frames may stay silent before its
 * still goes back on the clock, the push driver's default hold time.
 */
const FRAME_SILENCE_MS = 5 * 60_000;

/**
 * A shared store, so a class (svelte-5.md). The client comes in through the
 * constructor, which lets a test pass a fake.
 *
 * The store keeps no image. It keeps the addresses, and an `<img>` or a
 * `<video>` loads them with the page's own cookie. A still frame has no event
 * behind it, so the tick moves a counter and the counter goes onto the query,
 * which gives the browser a new address to load.
 */
export class Cameras {
  readonly #client: CamerasClient;
  readonly #timers: Timers;
  readonly #fallbackMs: number;
  readonly #deviceId: string | undefined;

  #floorMs = $state(FLEET_REFRESH_FLOOR_MS);
  #items = $state.raw<readonly Camera[]>([]);
  #loading = $state(false);
  #loaded = $state(false);
  #error = $state<DataFault | undefined>(undefined);
  #tick = $state(0);
  // When each camera last announced a frame. A camera that announces its
  // frames loads its still on each one rather than on the clock, until it
  // falls silent.
  #frames = $state.raw<Readonly<Record<string, number>>>({});
  #flight: Promise<void> | undefined;
  // Counts the resets. A read that started before a reset records nothing.
  #resets = 0;
  #handle: number | undefined;
  // How many panes listen. The tick stops when the last one lets go, so one
  // pane leaving does not stop the frames of a pane still on screen.
  #listeners = 0;

  constructor(
    client: CamerasClient,
    deviceId?: string,
    options: CamerasOptions = {},
  ) {
    this.#client = client;
    this.#deviceId = deviceId;
    this.#timers = options.timers ?? systemTimers;
    this.#fallbackMs = options.refreshMs ?? DEFAULT_REFRESH_MS;
  }

  /** The printer this store is about, or undefined for every camera. */
  get deviceId(): string | undefined {
    return this.#deviceId;
  }

  /** Read-only by design: the collection is the server's (svelte-5.md). */
  get items(): readonly Camera[] {
    return this.#items;
  }

  get loading(): boolean {
    return this.#loading;
  }

  /** True once a read has answered, which tells a printer with no camera from an unread one. */
  get loaded(): boolean {
    return this.#loaded;
  }

  get error(): DataFault | undefined {
    return this.#error;
  }

  /** How many times the still frame has been asked for again. */
  get tick(): number {
    return this.#tick;
  }

  /**
   * The fastest wait the page on screen will ask at. A page sets it through
   * `listen`, and a value that is not a positive number is refused: the state
   * stays and nothing is scheduled at it.
   */
  get floorMs(): number {
    return this.#floorMs;
  }

  set floorMs(next: number) {
    if (!Number.isFinite(next) || next <= 0) return;
    this.#floorMs = next;
  }

  /**
   * The shortest wait any camera here declares, or undefined when none does.
   * One tick serves every pane on the screen, so the fastest camera sets the
   * pace and a slower one is asked more often than it has a new frame.
   */
  get declaredMs(): number | undefined {
    let fastest: number | undefined;

    for (const row of this.#items) {
      const declared = declaredIntervalMs(row);
      if (declared === undefined) continue;
      if (fastest === undefined || declared < fastest) fastest = declared;
    }

    return fastest;
  }

  /**
   * The wait between two still frames, in milliseconds: what the camera
   * declares, never faster than the page's floor, and the fallback wait for a
   * camera that declares nothing. A pane prints it, so a person reads the
   * feed's own cadence rather than reading a slow page.
   */
  get refreshMs(): number {
    return Math.max(this.#floorMs, this.declaredMs ?? this.#fallbackMs);
  }

  /** The cameras that are producing frames now. */
  readonly serving: readonly Camera[] = $derived(
    this.#items.filter((row) => row.serving),
  );

  /** The first camera, which is the one a device pane draws. */
  readonly first: Camera | undefined = $derived(this.#items[0]);

  /**
   * The address of one camera's still frame: new on each frame the camera
   * announced, and on every tick for a camera that announces none.
   */
  snapshotUrl(camera: string): string {
    const at = this.#frames[camera];
    const tick = this.#tick;
    const silent = at === undefined || Date.now() - at > FRAME_SILENCE_MS;

    // Negative, so an announced frame's address never repeats a tick's, and
    // the frame's time, so it never repeats one another store made.
    return snapshotUrl(this.#client, camera, silent ? tick : -at);
  }

  /** Takes each frame a camera announces. The returned function stops it. */
  follow(fleet: Pick<Fleet, 'subscribe'>): () => void {
    return fleet.subscribe((event) => {
      if (event.type !== CAMERA_FRAME_EVENT) return;
      const camera = deviceIdOf(event);
      if (camera === undefined) return;
      this.#frames = { ...this.#frames, [camera]: Date.now() };
    });
  }

  /**
   * The address of a camera's still frame, or undefined for a camera that
   * keeps no still (H.264-only RTSP), which must never be asked for one.
   */
  stillUrl(camera: Camera): string | undefined {
    return camera.snapshot ? this.snapshotUrl(camera.name) : undefined;
  }

  /** The address a push camera sends its pictures to. */
  pushUrl(camera: string): string {
    return pushUrl(this.#client, camera);
  }

  /** Answers an SDP offer for the camera's H.264 live view. */
  watch(camera: string, offer: string, signal?: AbortSignal): Promise<string> {
    return watchCamera(this.#client, camera, offer, signal);
  }

  /** The address of one camera's live view. */
  streamUrl(camera: string): string {
    return streamUrl(this.#client, camera);
  }

  /** Opens the live socket that carries one camera's H.264 live view. */
  socket(camera: string): WebSocket {
    return openSocket(this.#client, camera, document.baseURI);
  }

  /** A second call while one read is in flight is given that read. */
  load(): Promise<void> {
    if (this.#flight) return this.#flight;

    const flight = this.#read().finally(() => {
      if (this.#flight === flight) this.#flight = undefined;
    });
    this.#flight = flight;

    return flight;
  }

  /**
   * Reads until `done` holds of the listing, at most `tries` times, because
   * the server starts a camera after the write that declares it answered.
   */
  async loadUntil(
    done: (items: readonly Camera[]) => boolean,
    tries = 10,
    waitMs = 300,
  ): Promise<void> {
    // A read that started before the write holds the old list.
    await this.#flight;
    for (let left = tries; left > 0; left -= 1) {
      await this.load();
      if (done(this.#items) || left === 1) return;
      await new Promise<void>((resolve) => this.#timers.set(resolve, waitMs));
    }
  }

  /**
   * Asks for a new still frame every `refreshMs`. The returned function lets
   * go, and the pane calls it when it closes (svelte-testing.md). It works
   * once, and the tick stops when every pane that listens has let go.
   *
   * The page passes its own floor, because one store serves the fleet and a
   * device's page and the two ask at different rates. A screen sets it as it
   * opens and the next screen sets its own.
   */
  listen(floorMs?: number): () => void {
    if (floorMs !== undefined) this.floorMs = floorMs;
    this.#listeners += 1;
    this.#schedule();

    let released = false;

    return () => {
      if (released) return;
      released = true;
      this.#listeners -= 1;
      if (this.#listeners === 0) this.stop();
    };
  }

  /**
   * Stops the tick whoever listens. A pane showing the live view calls it,
   * and the next `listen` starts the tick again.
   */
  stop(): void {
    if (this.#handle === undefined) return;

    this.#timers.clear(this.#handle);
    this.#handle = undefined;
  }

  /** Asks for a new still frame now. */
  refresh(): void {
    this.#tick += 1;
  }

  // The wait is read again on every pass, so a listing that arrives after the
  // first tick changes the cadence from the next one. The lag is one wait.
  #schedule(): void {
    this.stop();
    this.#handle = this.#timers.set(() => {
      this.#handle = undefined;
      this.refresh();
      this.#schedule();
    }, this.refreshMs);
  }

  /**
   * Forgets every camera this store read. The app calls it when the person
   * signed in changes, so the next person never sees the cameras of the last
   * one. A read that started before the reset records nothing.
   */
  reset(): void {
    this.#resets += 1;
    this.#flight = undefined;
    this.#items = [];
    this.#frames = {};
    this.#loading = false;
    this.#loaded = false;
    this.#error = undefined;
  }

  async #read(): Promise<void> {
    const resets = this.#resets;
    this.#loading = true;
    try {
      const page = await readCameras(this.#client, this.#deviceId);
      if (resets !== this.#resets) return;
      this.#items = page.items ?? [];
      this.#loaded = true;
      this.#error = undefined;
    } catch (thrown) {
      if (resets !== this.#resets) return;
      this.#items = [];
      this.#error = faultOf(thrown);
    } finally {
      if (resets === this.#resets) this.#loading = false;
    }
  }
}

/** One store per pane, built where the application builds its `PrusaConnect`. */
export const createCameras = (
  client: CamerasClient,
  deviceId?: string,
  options?: CamerasOptions,
): Cameras => new Cameras(client, deviceId, options);
