// One pairing at the printer's screen: what the store sends, how it follows
// the server while the pairing waits, and when it stops reading.
import { describe, expect, it } from 'vitest';

import { json, problem, stubClient } from '../../test/stub';
import { type PollTimer, PrinterPairingStore } from './printer-pairing.svelte';

const ID = '4f2a9c1e7b3d4a6f8e0c2b5d7a9f1e3c';
const READ = `GET /api/v1/printer-pairings/${ID}`;

const row = (state: string) => ({
  id: ID,
  code: 'K7QX-29FM',
  printer: '192.168.1.20:80',
  connect: { host: '192.168.1.50', port: 8080, tls: false },
  state,
  started_at: '2026-11-26T10:00:00Z',
});

/** A timer the case runs by hand: one pending read at most. */
const manualTimer = () => {
  let due: (() => void) | undefined;
  const timer: PollTimer = {
    set: (run) => {
      due = run;

      return 1;
    },
    clear: () => {
      due = undefined;
    },
  };

  return {
    timer,
    pending: () => due !== undefined,
    tick: async () => {
      const run = due;
      due = undefined;
      run?.();
      // Lets the read the tick started settle: the SDK reads the answer
      // through several promises.
      for (let turn = 0; turn < 20; turn++) {
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    },
  };
};

const request = {
  printer: '192.168.1.20',
  connect: { host: '192.168.1.50', port: 8080, tls: false },
};

describe('the printer pairing store', () => {
  it('starts with nothing open and nothing failed', () => {
    const { client } = stubClient({});
    const store = new PrinterPairingStore(client, manualTimer().timer);

    expect(store.pairing).toBeUndefined();
    expect(store.busy).toBe(false);
    expect(store.error).toBeUndefined();
  });

  it('sends the request and follows the pairing until the printer joined', async () => {
    const answers = ['waiting', 'approved', 'admitted'];
    const { client, calls } = stubClient({
      'POST /api/v1/printer-pairings': json(201, row('waiting')),
      [READ]: () => json(200, row(answers.shift() ?? 'admitted')),
    });
    const clock = manualTimer();
    const store = new PrinterPairingStore(client, clock.timer);

    await store.start(request);

    expect(store.pairing).toEqual({
      code: 'K7QX-29FM',
      printer: '192.168.1.20:80',
      state: 'waiting',
    });
    expect(JSON.parse(calls[0]?.body ?? '{}')).toEqual(request);

    await clock.tick();
    expect(store.pairing?.state).toBe('waiting');
    await clock.tick();
    expect(store.pairing?.state).toBe('approved');
    await clock.tick();
    expect(store.pairing?.state).toBe('admitted');
    expect(clock.pending()).toBe(false);
  });

  it('keeps the same object for a read that changed nothing', async () => {
    const { client } = stubClient({
      'POST /api/v1/printer-pairings': json(201, row('waiting')),
      [READ]: json(200, row('waiting')),
    });
    const clock = manualTimer();
    const store = new PrinterPairingStore(client, clock.timer);

    await store.start(request);
    const first = store.pairing;
    await clock.tick();

    expect(store.pairing).toBe(first);
    expect(clock.pending()).toBe(true);
  });

  it('keeps the refusal it was given and reads nothing more', async () => {
    const { client } = stubClient({
      'POST /api/v1/printer-pairings': problem(
        409,
        'core.printer/PAIRING_BUSY',
      ),
    });
    const clock = manualTimer();
    const store = new PrinterPairingStore(client, clock.timer);

    await store.start(request);

    expect(store.pairing).toBeUndefined();
    expect(store.error).toMatchObject({
      status: 409,
      code: 'core.printer/PAIRING_BUSY',
    });
    expect(store.busy).toBe(false);
    expect(clock.pending()).toBe(false);
  });

  it('withdraws the pairing and stops reading', async () => {
    const stub = stubClient({
      'POST /api/v1/printer-pairings': json(201, row('waiting')),
      [`DELETE /api/v1/printer-pairings/${ID}`]: json(200, row('withdrawn')),
    });
    const clock = manualTimer();
    const store = new PrinterPairingStore(stub.client, clock.timer);

    await store.start(request);
    await store.cancel();

    expect(store.pairing?.state).toBe('withdrawn');
    expect(clock.pending()).toBe(false);
    expect(stub.count(READ)).toBe(0);
  });

  it('ends a pairing the server no longer holds', async () => {
    const { client } = stubClient({
      'POST /api/v1/printer-pairings': json(201, row('waiting')),
      [READ]: problem(404, 'core.validation/RESOURCE_NOT_FOUND'),
    });
    const clock = manualTimer();
    const store = new PrinterPairingStore(client, clock.timer);

    await store.start(request);
    await clock.tick();

    expect(store.pairing?.state).toBe('expired');
    expect(clock.pending()).toBe(false);
  });

  it('drops an answer that came after the person moved on', async () => {
    let answer: (() => void) | undefined;
    const { client } = stubClient({
      'POST /api/v1/printer-pairings': () =>
        new Promise((resolve) => {
          answer = () => {
            resolve(json(201, row('waiting')));
          };
        }),
    });
    const clock = manualTimer();
    const store = new PrinterPairingStore(client, clock.timer);

    const started = store.start(request);
    await new Promise((resolve) => setTimeout(resolve, 0));
    store.reset();
    answer?.();
    await started;

    expect(store.pairing).toBeUndefined();
    expect(clock.pending()).toBe(false);
  });

  it('stops reading when the page goes away', async () => {
    const { client } = stubClient({
      'POST /api/v1/printer-pairings': json(201, row('waiting')),
    });
    const clock = manualTimer();
    const store = new PrinterPairingStore(client, clock.timer);

    await store.start(request);
    store.dispose();

    expect(clock.pending()).toBe(false);
  });
});
