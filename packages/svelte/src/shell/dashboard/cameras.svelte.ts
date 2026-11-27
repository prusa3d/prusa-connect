// The cameras a tile shows a still frame of, read once for the page.
//
// The library's camera store reads every camera the person may see and moves
// a counter on a timer, which gives each frame a new address. One store
// serves every tile here, and its timer runs only while a tile that shows a
// frame is mounted. A class, because every such tile shares it (svelte-5.md).
//
// A tile reads the list with `load` from `onMount`, and holds the timer with
// `hold` from an effect, so no effect fetches (svelte-data.md). A card's
// settings read it with `reload`, so a camera added since shows there.
import { untrack } from 'svelte';

import type { Camera, Cameras } from '../../data';
import { t } from '../../i18n';
import type { CameraFeed } from '../printer-cameras/types';
import type { FleetViews, Option } from './fleet-views.svelte';

/** What the hub needs of the library's store. A test hands in a fake. */
export type CameraStore = Pick<
  Cameras,
  | 'items'
  | 'loaded'
  | 'loading'
  | 'error'
  | 'load'
  | 'listen'
  | 'stillUrl'
  | 'streamUrl'
  | 'watch'
>;

export class CameraFrames {
  readonly #store: CameraStore;
  #holders = 0;
  #stop: (() => void) | undefined;

  constructor(store: CameraStore) {
    this.#store = store;
  }

  /** Every camera the person may see. Read-only by design. */
  get items(): readonly Camera[] {
    return this.#store.items;
  }

  get loaded(): boolean {
    return this.#store.loaded;
  }

  /** True while a read of the cameras is running. */
  get loading(): boolean {
    return this.#store.loading;
  }

  get error(): unknown {
    return this.#store.error;
  }

  /** The camera of one printer, when it has one. */
  of(deviceId: string): Camera | undefined {
    return this.#store.items.find((camera) => camera.device_id === deviceId);
  }

  /** One camera by its name. */
  named(name: string): Camera | undefined {
    return this.#store.items.find((camera) => camera.name === name);
  }

  /** The address of the newest still frame of one camera, when it keeps stills. */
  frame(camera: Camera): string | undefined {
    return this.#store.stillUrl(camera);
  }

  /** One camera as its picture draws it, with the live view it can play. */
  feed(camera: Camera): CameraFeed {
    return {
      camera,
      pictures: {
        still: this.#store.stillUrl(camera),
        live: camera.stream ? this.#store.streamUrl(camera.name) : undefined,
      },
      watch: (name, offer, signal) => this.#store.watch(name, offer, signal),
    };
  }

  /** Reads the cameras, unless they were read or are being read. */
  load(): void {
    if (!this.#store.loaded && !this.#store.loading) void this.#store.load();
  }

  /**
   * Reads the cameras again, unless they are being read. A camera added since
   * the last read then shows, even when that read found none.
   */
  reload(): void {
    if (!this.#store.loading) void this.#store.load();
  }

  /** Starts the frames for one more tile, and answers the call that lets it go. */
  hold(): () => void {
    // Taken outside any reader's reactive graph, so an effect that holds the
    // frames never reruns because the list of cameras arrived.
    return untrack(() => this.#hold());
  }

  #hold(): () => void {
    this.#holders += 1;
    this.#stop ??= this.#store.listen();

    let held = true;

    return () => {
      if (!held) return;
      held = false;
      this.#holders -= 1;
      if (this.#holders === 0) {
        this.#stop?.();
        this.#stop = undefined;
      }
    };
  }

  /** Every camera, named by the printer it watches when it watches one, for a form. */
  options(printers: FleetViews): Option[] {
    return this.#store.items
      .map((camera) => {
        const printer = printers.view(camera.device_id);

        return {
          value: camera.name,
          label:
            printer === undefined
              ? camera.name
              : t('dashboard.camera.option', {
                  printer: printer.name,
                  camera: camera.name,
                }),
        };
      })
      .sort((a, b) => a.label.localeCompare(b.label));
  }
}
