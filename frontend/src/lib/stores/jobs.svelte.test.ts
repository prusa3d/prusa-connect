// The read across the fleet: every running print, then a bounded number of
// pages of ended ones, grouped by printer and read again when a print starts
// or ends.
import { describe, expect, it, vi } from 'vitest';

import type { StreamEvent } from '@prusa/connect';
import { Fleet } from '@prusa/connect-ui/data';

import { type Call, json, problem, stubClient } from '../../test/stub';
import { Jobs, JOBS_PAGES } from './jobs.svelte';

const deviceOf = (id: string, job?: { id: string; progress: number }) => ({
  id,
  name: `Printer ${id}`,
  kind: 'printer',
  labels: {},
  state: { status: 'IDLE' },
  ...(job === undefined ? {} : { job }),
});

const endedOf = (device: string, number: number) => ({
  id: `${device}-${String(number)}`,
  device_id: device,
  device_job_id: String(number),
  display_name: 'Bracket',
  path: '/usb/bracket.bgcode',
  progress: 100,
  started: '2026-09-01T08:00:00Z',
  ended: '2026-09-01T09:00:00Z',
  state: 'FINISHED',
  time_remaining_ms: 0,
});

const runningOf = (device: string, number: number, progress = 40) => ({
  id: `${device}-${String(number)}`,
  device_id: device,
  device_job_id: String(number),
  display_name: 'Hinge',
  path: '/usb/hinge.bgcode',
  progress,
  started: '2026-08-01T08:00:00Z',
  state: 'PRINTING',
  time_remaining_ms: 60_000,
});

const paramOf = (call: Call, name: string): string =>
  new URLSearchParams(call.query).get(name) ?? '';

const phaseOf = (call: Call): string => paramOf(call, 'phase');

const page = (items: unknown[], cursor = '') =>
  json(200, { items, cursor, limit: 200 });

const built = (
  devices: unknown[],
  answer: (call: Call) => Response | Promise<Response>,
  timing: { settleMs?: number; retryMs?: number; now?: () => number } = {
    settleMs: 0,
  },
) => {
  const { client, calls } = stubClient({
    'GET /api/v1/devices': json(200, { items: devices }),
    'GET /api/v1/jobs': answer,
  });
  const fleet = new Fleet(client);

  return {
    fleet,
    jobs: new Jobs(client, fleet, timing),
    reads: () => calls.filter((call) => call.path === '/api/v1/jobs'),
  };
};

// What the store listens to, driven by hand.
const events = () => {
  let listener: ((event: StreamEvent) => void) | undefined;

  return {
    fleet: {
      subscribe: (next: (event: StreamEvent) => void) => {
        listener = next;

        return () => {
          listener = undefined;
        };
      },
    },
    settle: (deviceId: string) => {
      listener?.(
        // The cast stands in for the narrowed union the SDK yields for one type.
        {
          type: 'com.prusa.connect.device.job.settled',
          id: '1',
          payload: {},
          envelope: { subject: `device:${deviceId}` },
        } as StreamEvent,
      );
    },
  };
};

describe('the jobs read', () => {
  it('is not loaded before it has answered', () => {
    const { jobs } = built([deviceOf('a')], () => page([]));

    expect(jobs.loaded).toBe(false);
    expect(jobs.cut).toBe(false);
    expect(jobs.shown).toBe(0);
  });

  it('lists the running prints and the ended ones under their printers', async () => {
    const { fleet, jobs, reads } = built(
      [deviceOf('a'), deviceOf('b')],
      (call) =>
        phaseOf(call) === 'running'
          ? page([runningOf('a', 2)])
          : page([{ ...endedOf('a', 1), outcome: 'finished' }]),
    );

    await fleet.load();
    await jobs.load();

    expect(jobs.loaded).toBe(true);
    expect(jobs.printers.map((row) => row.id)).toEqual(['a', 'b']);
    expect(jobs.printers[0]?.jobs?.map((job) => job.id)).toEqual([
      'a-2',
      'a-1',
    ]);
    expect(jobs.printers[0]?.jobs?.[1]?.outcome).toBe('finished');
    // A printer with no print is one the filter still offers, read as empty.
    expect(jobs.printers[1]?.jobs).toEqual([]);
    expect(reads().map(phaseOf)).toEqual(['running', 'ended']);
  });

  // The running prints are read before any ended page, so one that started
  // before a thousand newer endings is still on the page.
  it('lists a running print older than every ended page', async () => {
    const { fleet, jobs } = built([deviceOf('a'), deviceOf('b')], (call) =>
      phaseOf(call) === 'running'
        ? page([runningOf('b', 9)])
        : page([endedOf('a', 1)], 'more'),
    );

    await fleet.load();
    await jobs.load();

    expect(jobs.printers[1]?.jobs?.map((job) => job.id)).toEqual(['b-9']);
    expect(jobs.cut).toBe(true);
  });

  it('stops after its pages of ended prints and says the list is cut', async () => {
    const { fleet, jobs, reads } = built([deviceOf('a')], (call) =>
      phaseOf(call) === 'running' ? page([]) : page([endedOf('a', 1)], 'more'),
    );

    await fleet.load();
    await jobs.load();

    expect(jobs.cut).toBe(true);
    expect(reads().filter((call) => phaseOf(call) === 'ended')).toHaveLength(
      JOBS_PAGES,
    );
  });

  it('says nothing is cut when the last page came back', async () => {
    const { fleet, jobs } = built([deviceOf('a')], () =>
      page([endedOf('a', 1)]),
    );

    await fleet.load();
    await jobs.load();

    expect(jobs.cut).toBe(false);
  });

  // The fleet row follows the machine's telemetry and the stored row does
  // not, so a running print shows the fleet's figure.
  it('shows a running print at the progress the fleet row carries', async () => {
    const { fleet, jobs } = built(
      [deviceOf('a', { id: 'a-2', progress: 73 })],
      (call) =>
        phaseOf(call) === 'running' ? page([runningOf('a', 2, 40)]) : page([]),
    );

    await fleet.load();
    await jobs.load();

    expect(jobs.printers[0]?.jobs?.[0]?.progress).toBe(73);
  });

  // The two phases are two reads, so a print that ended between them is in
  // both. It is listed once, as ended.
  it('lists a print that ended between the two reads once', async () => {
    const { fleet, jobs } = built([deviceOf('a')], (call) =>
      phaseOf(call) === 'running'
        ? page([runningOf('a', 2)])
        : page([endedOf('a', 2)]),
    );

    await fleet.load();
    await jobs.load();

    const rows = jobs.printers[0]?.jobs ?? [];
    expect(rows.map((job) => job.id)).toEqual(['a-2']);
    expect(rows[0]?.ended).toBeDefined();
  });

  it('reads one printer alone when the page names it', async () => {
    const { fleet, jobs, reads } = built([deviceOf('a'), deviceOf('b')], () =>
      page([endedOf('b', 1)]),
    );

    await fleet.load();
    await jobs.load('b');

    expect(jobs.printers.map((row) => row.id)).toEqual(['b']);
    expect(reads().map((call) => paramOf(call, 'device_id'))).toEqual([
      'b',
      'b',
    ]);
  });

  it('reads the same printer once', async () => {
    const { fleet, jobs, reads } = built([deviceOf('a')], () => page([]));

    await fleet.load();
    await jobs.load();
    await jobs.load();

    expect(reads()).toHaveLength(2);
  });

  it('keeps the refusal it was given', async () => {
    const { fleet, jobs } = built([deviceOf('a')], () =>
      problem(500, 'core.store/UNAVAILABLE'),
    );

    await fleet.load();
    await jobs.load();

    expect(jobs.error).toBeDefined();
    expect(jobs.loaded).toBe(false);
  });

  // A reader who leaves one printer's prints for every printer's must not be
  // left with the one printer's answer, whichever read answers last.
  it('keeps the answer of the read the page asked for last', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const { fleet, jobs } = built(
      [deviceOf('a'), deviceOf('b')],
      async (call) => {
        const one = paramOf(call, 'device_id') === 'a';
        if (one) await held;
        if (phaseOf(call) === 'running') return page([]);

        return page(
          one ? [endedOf('a', 1)] : [endedOf('a', 1), endedOf('b', 1)],
        );
      },
    );
    await fleet.load();

    const one = jobs.load('a');
    await jobs.load();
    release();
    await one;

    expect(jobs.printers.map((row) => row.id)).toEqual(['a', 'b']);
    expect(jobs.printers.every((row) => row.jobs?.length === 1)).toBe(true);
  });
});

describe('the jobs watch', () => {
  // A farm ends prints in bursts. Frames that arrive while a read is on its
  // way cost one more read, never one each.
  it('coalesces frames into one follow-up read', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    let first = true;
    const { fleet, jobs, reads } = built([deviceOf('a')], async () => {
      if (first) {
        first = false;
        await held;
      }

      return page([]);
    });
    await fleet.load();
    const stream = events();
    const stop = jobs.watch(stream.fleet);

    const reading = jobs.load();
    await vi.waitFor(() => {
      expect(reads()).toHaveLength(1);
    });
    for (let at = 0; at < 20; at += 1) stream.settle('a');
    release();
    await reading;

    // One read of each phase, then one follow-up of each.
    expect(reads()).toHaveLength(4);
    stop();
  });

  it('reads again when a print ends while the page is open', async () => {
    const { fleet, jobs, reads } = built([deviceOf('a')], () => page([]));
    await fleet.load();
    const stream = events();
    const stop = jobs.watch(stream.fleet);
    await jobs.load();

    stream.settle('a');
    await vi.waitFor(() => {
      expect(reads()).toHaveLength(4);
    });
    stop();
  });

  // A read that was refused is not retried on every frame of a busy stream.
  it('waits out the retry pause after a refused read', async () => {
    let now = 0;
    let refuse = false;
    const { fleet, jobs, reads } = built(
      [deviceOf('a')],
      () => (refuse ? problem(500, 'core.store/UNAVAILABLE') : page([])),
      { settleMs: 0, retryMs: 30_000, now: () => now },
    );
    await fleet.load();
    const stream = events();
    const stop = jobs.watch(stream.fleet);
    await jobs.load();
    refuse = true;

    stream.settle('a');
    await vi.waitFor(() => {
      expect(jobs.error).toBeDefined();
    });
    const refused = reads().length;
    for (let at = 0; at < 5; at += 1) stream.settle('a');
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(reads()).toHaveLength(refused);

    now = 30_001;
    refuse = false;
    stream.settle('a');
    await vi.waitFor(() => {
      expect(jobs.error).toBeUndefined();
    });
    stop();
  });

  // Frames spaced wider than a read takes, so none of them joins one in
  // flight: only the wait gathers them into a single read.
  it('gathers a burst of frames into one read', async () => {
    const { fleet, jobs, reads } = built([deviceOf('a')], () => page([]), {
      settleMs: 400,
    });
    await fleet.load();
    const stream = events();
    const stop = jobs.watch(stream.fleet);
    await jobs.load();

    for (let at = 0; at < 5; at += 1) {
      stream.settle('a');
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
    expect(reads()).toHaveLength(2);
    await vi.waitFor(() => {
      expect(reads()).toHaveLength(4);
    });
    await new Promise((resolve) => setTimeout(resolve, 200));
    expect(reads()).toHaveLength(4);
    stop();
  });

  it('reads nothing on a frame before the page has read', () => {
    const { jobs, reads } = built([deviceOf('a')], () => page([]));
    const stream = events();
    const stop = jobs.watch(stream.fleet);

    stream.settle('a');

    expect(reads()).toHaveLength(0);
    stop();
  });

  // Nothing follows the prints while no screen shows them, so the next visit
  // reads again.
  it('reads again on the next visit after the page stopped watching', async () => {
    const { fleet, jobs, reads } = built([deviceOf('a')], () => page([]));
    await fleet.load();
    const stop = jobs.watch(events().fleet);
    await jobs.load();
    stop();

    await jobs.load();

    expect(reads()).toHaveLength(4);
  });
});

// A browser can sign out and sign in as somebody else without the page
// reloading. The store then holds nothing of the first person.
describe('the jobs reset', () => {
  it('forgets the prints, and reads them again', async () => {
    const { fleet, jobs, reads } = built([deviceOf('a')], (call) =>
      phaseOf(call) === 'running' ? page([]) : page([endedOf('a', 1)]),
    );
    await fleet.load();
    await jobs.load();

    jobs.reset();

    expect(jobs.loaded).toBe(false);
    expect(jobs.printers[0]?.jobs).toBeUndefined();

    await jobs.load();
    expect(jobs.printers[0]?.jobs).toHaveLength(1);
    expect(reads()).toHaveLength(4);
  });

  it('drops an answer that was asked for before the reset', async () => {
    let answer: (() => void) | undefined;
    const { fleet, jobs } = built([deviceOf('a')], () =>
      answer === undefined
        ? new Promise<Response>((resolve) => {
            answer = () => {
              resolve(page([endedOf('a', 1)]));
            };
          })
        : page([endedOf('a', 1)]),
    );
    await fleet.load();
    const read = jobs.load();
    await vi.waitFor(() => {
      expect(answer).toBeDefined();
    });

    jobs.reset();
    answer?.();
    await read;

    expect(jobs.loaded).toBe(false);
    expect(jobs.printers[0]?.jobs).toBeUndefined();
  });
});
