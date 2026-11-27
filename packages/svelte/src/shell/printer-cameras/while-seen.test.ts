// Whether a camera picture is seen, over a fake observer and a fake page.
import { describe, expect, it } from 'vitest';

import {
  type SeenObserver,
  type SeenObserverOf,
  type ShownPage,
  whileSeen,
} from './while-seen';

class FakePage extends EventTarget implements ShownPage {
  visibilityState: DocumentVisibilityState = 'visible';

  show(state: DocumentVisibilityState): void {
    this.visibilityState = state;
    this.dispatchEvent(new Event('visibilitychange'));
  }
}

/** An observer whose answers come when the test says so. */
class FakeObserver implements SeenObserver {
  observed: Element | undefined;
  disconnected = false;

  constructor(
    readonly report: (entries: readonly { isIntersecting: boolean }[]) => void,
  ) {}

  observe(target: Element): void {
    this.observed = target;
  }

  disconnect(): void {
    this.disconnected = true;
  }

  scroll(isIntersecting: boolean): void {
    this.report([{ isIntersecting }]);
  }
}

const setUp = (): {
  page: FakePage;
  observer: () => FakeObserver;
  seen: boolean[];
  stop: () => void;
} => {
  const page = new FakePage();
  let made: FakeObserver | undefined;
  const observerOf: SeenObserverOf = (report) => {
    made = new FakeObserver(report);

    return made;
  };
  const seen: boolean[] = [];
  const box = {} as Element;
  const stop = whileSeen((now) => seen.push(now), page, observerOf)(box);
  const observer = (): FakeObserver => {
    if (made === undefined) throw new Error('no observer was made');

    return made;
  };
  expect(observer().observed).toBe(box);

  return { page, observer, seen, stop };
};

describe('whileSeen', () => {
  it('says nothing before the observer answers, so a box starts unseen', () => {
    const { seen } = setUp();

    expect(seen).toEqual([]);
  });

  it('reports a box scrolled into view and out of it', () => {
    const { observer, seen } = setUp();

    observer().scroll(true);
    observer().scroll(false);

    expect(seen).toEqual([true, false]);
  });

  it('reports a box on screen as unseen while its tab is hidden', () => {
    const { page, observer, seen } = setUp();

    observer().scroll(true);
    page.show('hidden');
    page.show('visible');

    expect(seen).toEqual([true, false, true]);
  });

  it('does not report a box off screen as seen when its tab is shown', () => {
    const { page, seen } = setUp();

    page.show('hidden');
    page.show('visible');

    expect(seen).toEqual([]);
  });

  it('reports each change once', () => {
    const { observer, seen } = setUp();

    observer().scroll(true);
    observer().scroll(true);

    expect(seen).toEqual([true]);
  });

  it('stops observing and stops listening to the page when detached', () => {
    const { page, observer, seen, stop } = setUp();

    observer().scroll(true);
    stop();
    page.show('hidden');

    expect(observer().disconnected).toBe(true);
    expect(seen).toEqual([true]);
  });
});
