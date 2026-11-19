// Where the app opens. The root address decides and then steps aside, so the
// back button never returns to it.
import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { Preferences } from '@prusa/connect-ui/data';
import type { HomePreference } from '@prusa/connect-ui/domain';

import { appContext } from '../../test/app-context';
import { json, problem, type Routes } from '../../test/stub';
import Home from './home.svelte';

const deviceOf = (id: string) => ({
  id,
  name: id,
  kind: 'printer',
  model: 'MK4S',
  labels: {},
  state: { '/state': 'IDLE' },
});

const fleetOf = (count: number): Routes => ({
  'GET /api/v1/devices': json(200, {
    items: ['a', 'b', 'c'].slice(0, count).map(deviceOf),
  }),
});

const landing = async (
  count: number,
  home: HomePreference,
  printer?: string,
) => {
  const preferences = new Preferences(null);
  preferences.setHome(home);
  if (printer !== undefined) preferences.setPrinter(printer);

  const went: { path: string; replace: boolean | undefined }[] = [];
  const ctx = appContext(fleetOf(count), {
    preferences,
    go: (path, options) => {
      went.push({ path, replace: options?.replace });
    },
  });
  await render(Home, { props: { ctx } });

  await expect.poll(() => went.length).toBe(1);

  return went[0];
};

describe('where the app opens', () => {
  it.each([
    [1, 'auto', undefined, '/printers/a'],
    [2, 'auto', undefined, '/printers'],
    [3, 'auto', undefined, '/printers'],
    [1, 'my-printer', undefined, '/printers/a'],
    [3, 'my-printer', 'b', '/printers/b'],
    [3, 'my-printer', 'gone', '/printers'],
    [1, 'all-printers', undefined, '/printers'],
  ] as const)(
    '%i printers under %s (last on %s) opens %s',
    async (count, home, printer, expected) => {
      expect(await landing(count, home, printer)).toEqual({
        path: expected,
        replace: true,
      });
    },
  );

  // A fleet that could not be read has nothing to choose from, and the list
  // is the page that says why.
  it('opens the printers list when the fleet cannot be read', async () => {
    const went: string[] = [];
    const preferences = new Preferences(null);
    preferences.setHome('auto');
    const ctx = appContext(
      { 'GET /api/v1/devices': problem(500, 'core.api/UNEXPECTED') },
      {
        preferences,
        go: (path) => {
          went.push(path);
        },
      },
    );

    await render(Home, { props: { ctx } });

    await expect.poll(() => went).toEqual(['/printers']);
  });

  // A failed read for one stream event leaves an error on a fleet that is
  // still loaded. The fleet can still be chosen from, so the setting decides.
  it('keeps to the setting after a stream error on a loaded fleet', async () => {
    const went: string[] = [];
    const preferences = new Preferences(null);
    preferences.setHome('auto');
    const ctx = appContext(fleetOf(1), {
      preferences,
      go: (path) => {
        went.push(path);
      },
    });
    await ctx.fleet.load();
    await ctx.fleet.apply({
      type: 'com.prusa.connect.device.record.changed',
      id: '1',
      payload: { device_id: 'gone' },
      envelope: {
        specversion: '1.0',
        id: '1',
        source: 'urn:prusa:connect:printer/gone',
        type: 'com.prusa.connect.device.record.changed',
        time: '2026-09-01T08:00:00Z',
        subject: 'device:gone',
        hops: 0,
        evclass: 'state',
      },
    });
    expect(ctx.fleet.error).toBeDefined();

    await render(Home, { props: { ctx } });

    await expect.poll(() => went).toEqual(['/printers/a']);
  });

  // The dashboard is the home page itself, so the root goes nowhere.
  it('draws the dashboard in place when it is the home page', async () => {
    const went: string[] = [];
    const preferences = new Preferences(null);
    preferences.setHome('dashboard');
    const ctx = appContext(
      {
        ...fleetOf(1),
        'GET /api/v1/dashboards': json(200, {
          items: [],
          total: 0,
          default_id: null,
        }),
        'POST /api/v1/access/check': json(200, { allowed: true }),
      },
      {
        preferences,
        go: (path) => {
          went.push(path);
        },
      },
    );

    await render(Home, { props: { ctx } });

    await expect
      .element(page.getByRole('heading', { level: 1, name: /Overview/ }))
      .toBeVisible();
    expect(went).toEqual([]);
  });

  // A server that leaves dashboards out reads the choice as automatic.
  it('opens the printer on a server without dashboards', async () => {
    const went: string[] = [];
    const preferences = new Preferences(null);
    preferences.setHome('dashboard');
    const ctx = appContext(
      {
        ...fleetOf(1),
        'GET /api/v1/ui': json(200, {
          title: 'Prusa Connect',
          default_language: 'en',
          theme: 'system',
          dashboards: false,
        }),
      },
      {
        preferences,
        go: (path) => {
          went.push(path);
        },
      },
    );

    await render(Home, { props: { ctx } });

    await expect.poll(() => went).toEqual(['/printers/a']);
  });
});
