import { describe, expect, it, vi } from 'vitest';

import { slots } from './slots';

const setup = (): {
  take: (name: string) => () => void;
  started: string[];
} => {
  const take = slots(2);
  const started: string[] = [];
  return {
    take: (name) =>
      take(() => {
        started.push(name);
      }),
    started,
  };
};

describe('slots', () => {
  it('starts two at once and the rest in the order they asked', () => {
    const { take, started } = setup();
    const a = take('a');
    take('b');
    take('c');
    take('d');

    expect(started).toEqual(['a', 'b']);

    a();
    expect(started).toEqual(['a', 'b', 'c']);
  });

  it('frees a slot once, however often it is given back', () => {
    const { take, started } = setup();
    const a = take('a');
    take('b');
    take('c');
    take('d');

    a();
    a();
    expect(started).toEqual(['a', 'b', 'c']);
  });

  // A row unmounted, or scrolled away, before its turn came.
  it('drops a job that leaves the queue before it started', () => {
    const { take, started } = setup();
    const a = take('a');
    take('b');
    const c = take('c');
    take('d');

    c();
    a();
    expect(started).toEqual(['a', 'b', 'd']);

    c();
    take('e');
    expect(started).toEqual(['a', 'b', 'd']);
  });

  // A picture a printer never answers must not hold the page's queue.
  it('gives a stalled slot back after the bound, and only once', () => {
    vi.useFakeTimers();
    try {
      const take = slots(2, 1000);
      const started: string[] = [];
      const stalled: string[] = [];
      const start = (name: string) =>
        take(
          () => {
            started.push(name);
          },
          () => {
            stalled.push(name);
          },
        );
      const a = start('a');
      start('b');
      start('c');
      start('d');
      start('e');

      vi.advanceTimersByTime(999);
      expect(started).toEqual(['a', 'b']);
      expect(stalled).toEqual([]);
      vi.advanceTimersByTime(1);
      expect(started).toEqual(['a', 'b', 'c', 'd']);
      // Told, so the job can cancel what it started.
      expect(stalled).toEqual(['a', 'b']);

      // The stalled picture answers late, and the slot is already free.
      a();
      expect(started).toEqual(['a', 'b', 'c', 'd']);
    } finally {
      vi.useRealTimers();
    }
  });

  it('never calls a job stalled once it gave its slot back', () => {
    vi.useFakeTimers();
    try {
      const take = slots(2, 1000);
      let stalled = 0;
      const give = take(
        () => undefined,
        () => {
          stalled += 1;
        },
      );
      give();
      vi.advanceTimersByTime(1000);
      expect(stalled).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it('never has more than two running through any churn', () => {
    const take = slots(2);
    let running = 0;
    let peak = 0;
    const jobs = Array.from({ length: 20 }, () => {
      let started = false;
      const release = take(() => {
        started = true;
        running += 1;
        peak = Math.max(peak, running);
      });
      return (): void => {
        if (started) running -= 1;
        release();
      };
    });

    // A scrambled order, so some finish and some leave before their turn.
    for (let step = 0; step < jobs.length; step += 1) {
      jobs[(step * 7) % jobs.length]?.();
    }

    expect(peak).toBe(2);
    expect(running).toBe(0);
  });
});
