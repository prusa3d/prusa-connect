import { afterEach, describe, expect, it } from 'vitest';

import { fleetOf } from '../../.storybook/fleet';
import { emptyList, installApi } from './api';

let restore = (): void => undefined;

afterEach(() => {
  restore();
});

describe('the story core', () => {
  it('lists the printers of the chosen fleet size', async () => {
    restore = installApi('two');
    const body = (await (await fetch('/api/v1/devices')).json()) as {
      items: unknown[];
    };
    expect(body.items).toHaveLength(2);
  });

  it('answers a story row over the table', async () => {
    restore = installApi('many', { 'GET /api/v1/jobs': emptyList() });
    const first = (await (await fetch('/api/v1/jobs')).json()) as {
      total: number;
    };
    const again = await fetch('/api/v1/jobs');
    expect(first.total).toBe(0);
    expect(again.status).toBe(200);
  });

  it('answers a recorded row that carries its query', async () => {
    restore = installApi('many');
    const answer = await fetch(
      '/api/v1/storages/local/files?path=q3_rush_jobs',
    );
    expect(answer.status).toBe(200);
  });

  it('serves running and ended jobs for the selected demo printers', async () => {
    restore = installApi('many');
    const ended = (await (
      await fetch('/api/v1/jobs?phase=ended&limit=200')
    ).json()) as { items: { state: string }[] };
    expect(new Set(ended.items.map(({ state }) => state))).toEqual(
      new Set(['FINISHED', 'ERROR', 'STOPPED']),
    );
    const running = (await (
      await fetch('/api/v1/jobs?phase=running&limit=200')
    ).json()) as { items: { device_id: string; state: string }[] };
    const printer = running.items[0]?.device_id ?? '';
    expect(running.items).toHaveLength(2);
    expect(running.items.every(({ state }) => state === 'PRINTING')).toBe(true);
    const single = (await (
      await fetch(`/api/v1/jobs?phase=running&device_id=${printer}`)
    ).json()) as typeof running;
    expect(single.items.map(({ device_id }) => device_id)).toEqual([printer]);
  });

  it('moves a folder with its files within the story session', async () => {
    restore = installApi('many');
    const base = '/api/v1/storages/local';
    const move = await fetch(base + '/moves', {
      method: 'POST',
      body: JSON.stringify({ from: 'examples', to: 'samples' }),
    });
    expect(move.status).toBe(200);
    const answer = await fetch(base + '/files?path=samples');
    const listing = (await answer.json()) as { items: { path: string }[] };
    expect(listing.items.map(({ path }) => path)).toEqual([
      'samples/spacer.bgcode',
    ]);
    expect(
      (await fetch(base + '/files?path=samples', { method: 'DELETE' })).status,
    ).toBe(204);
    const removed = await fetch(base + '/files?path=samples');
    expect(removed.status).toBe(404);
    restore();
    restore = installApi('many');
    const original = await fetch(base + '/files?path=examples');
    expect((await original.json()) as typeof listing).toMatchObject({
      items: [{ path: 'examples/spacer.bgcode' }],
    });
  });

  it('answers a copied printer from the rows of the one it copies', async () => {
    restore = installApi('many');
    const copy = fleetOf('many')[5]?.id ?? '';
    const answer = await fetch(`/api/v1/devices/${copy}/jobs/history`);
    const body = (await answer.json()) as { items: { device_id: string }[] };
    expect(body.items.length).toBeGreaterThan(0);
    expect(body.items.every((job) => job.device_id === copy)).toBe(true);
  });

  it('lists the recorded USB folders for every demo printer', async () => {
    restore = installApi('many');
    for (const printer of fleetOf('many')) {
      const files = `/api/v1/devices/${printer.id}/files`;
      const root = await fetch(files);
      expect(root.status).toBe(200);
      expect(await root.json()).toMatchObject({ items: [{ path: 'usb' }] });
      const folder = await fetch(
        files + '?path=usb%2Fcalibration&refresh=true',
      );
      expect(folder.status).toBe(200);
      expect(await folder.json()).toMatchObject({
        directory: { path: 'usb/calibration' },
      });
      expect((await fetch(files + '?path=usb%2Fmissing')).status).toBe(404);
    }
  });

  it('answers 404 for a request the table does not name', async () => {
    restore = installApi('many');
    expect((await fetch('/api/v1/nowhere')).status).toBe(404);
  });

  it('puts the real fetch back', () => {
    const original = globalThis.fetch;
    installApi('one')();
    expect(globalThis.fetch).toBe(original);
  });

  it('puts the real fetch back after two installs', () => {
    const original = globalThis.fetch;
    installApi('one');
    installApi('two')();
    expect(globalThis.fetch).toBe(original);
  });

  it('holds the event stream open', async () => {
    restore = installApi('many');
    const events = await fetch('/api/v1/events');
    expect(events.status).toBe(200);
    expect(events.headers.get('content-type')).toBe('text/event-stream');

    const reader = events.body?.getReader();
    const read = reader?.read().then(() => 'closed');
    const waited = new Promise((resolve) => setTimeout(resolve, 100, 'open'));
    expect(await Promise.race([read, waited])).toBe('open');
  });
});
