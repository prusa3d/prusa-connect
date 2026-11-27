// Live views that a camera picture let go of, held open for a while.
//
// A live view stops when its picture leaves the screen, its browser tab is
// hidden, or the person switches to another tab of the page. Starting it again
// costs seconds: WebRTC negotiates, and the live socket waits for a keyframe.
// So a view that stops for any of those reasons keeps its connection for a
// short while, and a picture of the same camera that starts in that time takes
// it over and plays at once. A stop the person pressed hangs up at once.

/** A connection to one camera's live view. */
export interface LiveSession {
  /** Whether it is closed, or failed, and so cannot be taken over. */
  readonly ended: boolean;
  close(): void;
}

/**
 * How long a live view nobody watches stays open. About long enough to look
 * at another tab and come back, and short enough that a page left on another
 * tab gives the camera's stream slot back soon.
 */
export const LINGER_MS = 30_000;

interface Held<S> {
  session: S;
  timer: ReturnType<typeof setTimeout>;
}

const pools = new Set<SessionPool<LiveSession>>();

/** The held live views of one kind, by camera. */
export class SessionPool<S extends LiveSession> {
  readonly #held = new Map<string, Held<S>[]>();
  readonly #linger: number;

  constructor(linger = LINGER_MS) {
    this.#linger = linger;
    pools.add(this);
  }

  /** Takes over a held view of the camera that is still open, if one is. */
  take(camera: string): S | undefined {
    const held = this.#held.get(camera) ?? [];
    this.#held.delete(camera);
    let taken: S | undefined;
    const rest: Held<S>[] = [];
    for (const entry of held) {
      if (entry.session.ended || taken === undefined) {
        clearTimeout(entry.timer);
        if (!entry.session.ended) taken = entry.session;
      } else {
        rest.push(entry);
      }
    }
    if (rest.length > 0) this.#held.set(camera, rest);

    return taken;
  }

  /** Holds a view nobody watches, and closes it once the linger runs out. */
  hold(camera: string, session: S): void {
    if (session.ended) return;
    const entry: Held<S> = {
      session,
      timer: setTimeout(() => {
        const held = this.#held.get(camera) ?? [];
        const rest = held.filter((other) => other !== entry);
        if (rest.length > 0) this.#held.set(camera, rest);
        else this.#held.delete(camera);
        session.close();
      }, this.#linger),
    };
    this.#held.set(camera, [...(this.#held.get(camera) ?? []), entry]);
  }

  /** Closes every held view of the camera now. */
  hangUp(camera: string): void {
    for (const entry of this.#held.get(camera) ?? []) {
      clearTimeout(entry.timer);
      entry.session.close();
    }
    this.#held.delete(camera);
  }

  /** Closes every held view. */
  hangUpAll(): void {
    for (const camera of [...this.#held.keys()]) this.hangUp(camera);
  }
}

/** Closes every held view of the camera, of every kind. */
export const hangUp = (camera: string): void => {
  for (const pool of pools) pool.hangUp(camera);
};

/** Closes every held view. For tests, which share one module. */
export const hangUpAll = (): void => {
  for (const pool of pools) pool.hangUpAll();
};
