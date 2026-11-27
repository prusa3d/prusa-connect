import { afterEach, describe, expect, it, vi } from 'vitest';

import { type LiveSession, SessionPool } from './live-sessions';

class FakeSession implements LiveSession {
  ended = false;
  closes = 0;

  close(): void {
    this.ended = true;
    this.closes += 1;
  }
}

afterEach(() => {
  vi.useRealTimers();
});

describe('SessionPool', () => {
  it('hands a held view of the camera to the next picture of it, once', () => {
    const pool = new SessionPool<FakeSession>(1000);
    const session = new FakeSession();

    pool.hold('bed-cam', session);

    expect(pool.take('shelf-cam')).toBeUndefined();
    expect(pool.take('bed-cam')).toBe(session);
    expect(pool.take('bed-cam')).toBeUndefined();
    expect(session.closes).toBe(0);
  });

  it('closes a view nobody took over once the linger runs out', () => {
    vi.useFakeTimers();
    const pool = new SessionPool<FakeSession>(1000);
    const session = new FakeSession();

    pool.hold('bed-cam', session);
    vi.advanceTimersByTime(999);
    expect(session.closes).toBe(0);
    vi.advanceTimersByTime(1);

    expect(session.closes).toBe(1);
    expect(pool.take('bed-cam')).toBeUndefined();
  });

  it('keeps a view that was taken over open past the linger', () => {
    vi.useFakeTimers();
    const pool = new SessionPool<FakeSession>(1000);
    const session = new FakeSession();

    pool.hold('bed-cam', session);
    pool.take('bed-cam');
    vi.advanceTimersByTime(5000);

    expect(session.closes).toBe(0);
  });

  it('hands over no view that ended while it was held', () => {
    const pool = new SessionPool<FakeSession>(1000);
    const failed = new FakeSession();
    const open = new FakeSession();

    pool.hold('bed-cam', failed);
    pool.hold('bed-cam', open);
    failed.ended = true;

    expect(pool.take('bed-cam')).toBe(open);
  });

  it('closes every held view of a camera at once when asked to hang up', () => {
    const pool = new SessionPool<FakeSession>(1000);
    const first = new FakeSession();
    const second = new FakeSession();
    const other = new FakeSession();

    pool.hold('bed-cam', first);
    pool.hold('bed-cam', second);
    pool.hold('shelf-cam', other);
    pool.hangUp('bed-cam');

    expect([first.closes, second.closes, other.closes]).toEqual([1, 1, 0]);
    expect(pool.take('shelf-cam')).toBe(other);
  });
});
