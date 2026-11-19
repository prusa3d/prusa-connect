import type { PrusaConnect, StreamEvent } from '@prusa/connect';
import { flushSync } from 'svelte';
import { describe, expect, it } from 'vitest';

import { NOZZLE_PATH, NOZZLE_TARGET_PATH, readingsOf } from '../domain';
import { DESCRIBED_BED, DESCRIBED_ENCLOSURE } from '../test/readings';
import {
  chartedPaths,
  SharedTelemetry,
  type SharedTelemetryClient,
} from './shared-telemetry.svelte';

const BED = '/temperature/bed';
import { fakeClock, fakeStream, hubEvent, settled } from './stubs';

const sampled = (deviceId: string, path: string, value: unknown): StreamEvent =>
  hubEvent(
    'com.prusa.connect.device.telemetry.sampled',
    { path, value },
    `device:${deviceId}`,
  );

/** A client that records every stream it opened, and answers no history. */
const recording = () => {
  const opened: {
    origins: readonly string[];
    stream: ReturnType<typeof fakeStream>;
  }[] = [];
  const events = ((options: { origins?: readonly string[] }) => {
    const stream = fakeStream();
    opened.push({ origins: options.origins ?? [], stream });

    return stream.subscription;
  }) as unknown as PrusaConnect['events'];
  const asked: unknown[] = [];
  const empty = () =>
    Promise.resolve({ items: [], next_cursor: '', truncated: false });
  const client = {
    events,
    listDeviceHistorySamples: (options: unknown) => {
      asked.push(options);

      return empty();
    },
    listDeviceHistoryFacts: empty,
  } as unknown as SharedTelemetryClient;

  return { client, opened, asked };
};

describe('SharedTelemetry', () => {
  it('follows every charted printer through one stream', () => {
    const clock = fakeClock();
    const { client, opened } = recording();
    const hub = new SharedTelemetry(client, { timers: clock });

    hub.follow('a');
    hub.follow('b');
    hub.follow('a');
    clock.advance(100);

    expect(opened).toHaveLength(1);
    expect(opened[0]?.origins).toEqual(['device:a', 'device:b']);
    expect(hub.following).toBe(2);
  });

  it('opens the stream again when the set changes, and closes it when nobody follows', () => {
    const clock = fakeClock();
    const { client, opened } = recording();
    const hub = new SharedTelemetry(client, { timers: clock });

    const releaseA = hub.follow('a');
    const releaseB = hub.follow('b');
    clock.advance(100);

    releaseA?.();
    releaseA?.();
    clock.advance(100);
    expect(opened).toHaveLength(2);
    expect(opened[0]?.stream.closed).toBe(true);
    expect(opened[1]?.origins).toEqual(['device:b']);

    releaseB?.();
    clock.advance(100);
    expect(opened[1]?.stream.closed).toBe(true);
    expect(opened).toHaveLength(2);
    expect(hub.following).toBe(0);
  });

  it('follows no more printers than its limit', () => {
    const { client } = recording();
    const hub = new SharedTelemetry(client, {
      timers: fakeClock(),
      maxDevices: 2,
    });

    expect(hub.follow('a')).toBeDefined();
    expect(hub.follow('b')).toBeDefined();
    expect(hub.follow('c')).toBeUndefined();
    expect(hub.follow('a')).toBeDefined();
  });

  it('keeps the charted readings of a followed printer and drops the rest', async () => {
    const clock = fakeClock();
    const { client, opened } = recording();
    const hub = new SharedTelemetry(client, { timers: clock });
    hub.follow('a');
    clock.advance(100);
    await settled();

    const stream = opened[0]?.stream;
    await stream?.push(sampled('a', NOZZLE_PATH, 215));
    await stream?.push(sampled('a', '/speed', 100));
    await stream?.push(sampled('z', NOZZLE_PATH, 30));

    expect(hub.points('a', NOZZLE_PATH).map((point) => point.value)).toEqual([
      215,
    ]);
    expect(hub.points('a', '/speed')).toEqual([]);
    expect(hub.points('z', NOZZLE_PATH)).toEqual([]);
  });

  it('charts the nozzle and the value and target of every described reading', () => {
    expect(
      chartedPaths(readingsOf({ ...DESCRIBED_BED, ...DESCRIBED_ENCLOSURE })),
    ).toEqual([
      NOZZLE_PATH,
      NOZZLE_TARGET_PATH,
      BED,
      '/temperature/bed_target',
      '/sensors/enclosure',
    ]);
    expect(chartedPaths([])).toEqual([NOZZLE_PATH, NOZZLE_TARGET_PATH]);
  });

  it('reads the history of the paths it was given', async () => {
    const { client, asked } = recording();
    const hub = new SharedTelemetry(client, { timers: fakeClock() });
    hub.follow('a', chartedPaths(readingsOf(DESCRIBED_BED)));
    await settled();

    expect(JSON.stringify(asked[0])).toContain(
      JSON.stringify([
        NOZZLE_PATH,
        NOZZLE_TARGET_PATH,
        BED,
        '/temperature/bed_target',
      ]),
    );
  });

  it('charts a path described after the printer was followed', async () => {
    const clock = fakeClock();
    const { client, opened } = recording();
    const hub = new SharedTelemetry(client, { timers: clock });
    hub.follow('a', chartedPaths([]));
    clock.advance(100);
    await settled();

    const seen: number[][] = [];
    const stop = $effect.root(() => {
      $effect(() => {
        seen.push(
          hub.points('a', '/sensors/enclosure').map((one) => one.value),
        );
      });
    });
    flushSync();
    await opened[0]?.stream.push(sampled('a', '/sensors/enclosure', 31));
    flushSync();

    expect(seen).toEqual([[], [31]]);
    stop();
  });

  it('redraws only what reads the series a frame belongs to', () => {
    const { client } = recording();
    const hub = new SharedTelemetry(client, { timers: fakeClock() });
    hub.follow('a');
    hub.follow('b');
    const counts = { aNozzle: 0, aBed: 0, bNozzle: 0 };

    const stop = $effect.root(() => {
      $effect(() => {
        hub.points('a', NOZZLE_PATH);
        counts.aNozzle += 1;
      });
      $effect(() => {
        hub.points('a', BED);
        counts.aBed += 1;
      });
      $effect(() => {
        hub.points('b', NOZZLE_PATH);
        counts.bNozzle += 1;
      });
    });
    flushSync();
    expect(counts).toEqual({ aNozzle: 1, aBed: 1, bNozzle: 1 });

    hub.apply(sampled('a', NOZZLE_PATH, 215));
    flushSync();
    expect(counts).toEqual({ aNozzle: 2, aBed: 1, bNozzle: 1 });

    // A path no chart draws, and a printer nobody follows, change nothing.
    hub.apply(sampled('a', '/speed', 100));
    hub.apply(sampled('z', NOZZLE_PATH, 30));
    flushSync();
    expect(counts).toEqual({ aNozzle: 2, aBed: 1, bNozzle: 1 });

    stop();
  });

  it('keeps an hour of live points of one path, and drops the oldest', () => {
    const { client } = recording();
    const hub = new SharedTelemetry(client, { timers: fakeClock() });
    hub.follow('a');
    const start = Date.UTC(2026, 9, 8, 9, 0, 0);

    for (let index = 0; index < 1805; index++) {
      const frame = sampled('a', NOZZLE_PATH, index);
      const envelope = frame.envelope;
      if (envelope === undefined)
        throw new Error('a hub event has an envelope');
      hub.apply({
        ...frame,
        envelope: {
          ...envelope,
          time: new Date(start + index * 2000).toISOString(),
        },
      });
    }

    const points = hub.points('a', NOZZLE_PATH);
    expect(points).toHaveLength(1800);
    expect(points[0]?.value).toBe(5);
    expect(points.at(-1)?.value).toBe(1804);
  });

  it('tells a reader when a followed printer is let go', () => {
    const { client } = recording();
    const hub = new SharedTelemetry(client, {
      timers: fakeClock(),
      maxDevices: 1,
    });
    const seen: number[] = [];
    const stop = $effect.root(() => {
      $effect(() => {
        seen.push(hub.following);
      });
    });
    flushSync();

    const release = hub.follow('a');
    flushSync();
    expect(hub.follow('b')).toBeUndefined();
    release?.();
    flushSync();

    expect(seen).toEqual([0, 1, 0]);
    expect(hub.follow('b')).toBeDefined();
    stop();
  });

  it('forgets everything when the page goes away', () => {
    const clock = fakeClock();
    const { client, opened } = recording();
    const hub = new SharedTelemetry(client, { timers: clock });
    hub.follow('a');
    clock.advance(100);

    hub.close();
    expect(opened[0]?.stream.closed).toBe(true);
    expect(hub.following).toBe(0);
    expect(clock.pending).toBe(0);
  });
});
