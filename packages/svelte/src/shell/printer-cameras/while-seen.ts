// Whether a camera picture is in front of somebody: on screen, in a browser tab
// that is shown. An auto-playing live view runs only while it is, because a
// stream costs the camera, the network and the server for as long as it runs.

/** The page whose visibility counts. `document` in the browser. */
export interface ShownPage extends EventTarget {
  readonly visibilityState: DocumentVisibilityState;
}

/** The part of IntersectionObserver this reads. */
export interface SeenObserver {
  observe(target: Element): void;
  disconnect(): void;
}

/** Builds the observer. `IntersectionObserver` in the browser. */
export type SeenObserverOf = (
  report: (entries: readonly { isIntersecting: boolean }[]) => void,
) => SeenObserver;

const browserObserver: SeenObserverOf = (report) =>
  new IntersectionObserver((entries) => {
    report(entries);
  });

/**
 * An attachment that calls `onChange` each time the box starts or stops being
 * seen. It says nothing before the first answer, so a box starts unseen.
 */
export const whileSeen =
  (
    onChange: (seen: boolean) => void,
    page: ShownPage = document,
    observerOf: SeenObserverOf = browserObserver,
  ) =>
  (box: Element): (() => void) => {
    let onScreen = false;
    let last = false;

    const report = (): void => {
      const seen = onScreen && page.visibilityState === 'visible';
      if (seen === last) return;
      last = seen;
      onChange(seen);
    };

    const observer = observerOf((entries) => {
      // One box is observed, so the newest entry is its state now.
      const newest = entries.at(-1);
      if (newest === undefined) return;
      onScreen = newest.isIntersecting;
      report();
    });
    observer.observe(box);
    page.addEventListener('visibilitychange', report);

    return () => {
      observer.disconnect();
      page.removeEventListener('visibilitychange', report);
    };
  };
