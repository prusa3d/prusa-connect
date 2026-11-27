// The camera wiring on the printers page: the frames view draws a printer's
// camera with its live view, and a printer with no camera keeps its artwork.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { Preferences } from '@prusa/connect-ui/data';
import { FEATURES } from '@prusa/connect-ui/domain';
import { tick } from 'svelte';

import { appContext } from '../../test/app-context';
import { empty, json, type Routes, stubFetch } from '../../test/stub';
import Printers from './printers.svelte';

const deviceOf = (id: string, name: string) => ({
  id,
  name,
  kind: 'printer',
  model: 'MK4S',
  labels: {},
  state: { '/state': 'PRINTING', '/job/progress': 42 },
});

const SERVED: Routes = {
  'GET /api/v1/devices': json(200, {
    items: [deviceOf('a', 'Alpha'), deviceOf('b', 'Beta')],
    total: 2,
  }),
  'GET /api/v1/cameras': json(200, {
    items: [
      {
        device_id: 'a',
        formats: ['image/jpeg'],
        name: 'front',
        serving: true,
        snapshot: true,
        stream: true,
        watch: false,
      },
    ],
  }),
};

/** The frames view, which is the one the cameras are drawn in. */
const openFrames = async () => {
  const view = await render(Printers, { props: { ctx: appContext(SERVED) } });
  await expect.element(page.getByText('Alpha').first()).toBeInTheDocument();
  await page.getByRole('radio', { name: 'Cameras' }).click();

  return view;
};

describe('the printers page', () => {
  // The view plays by itself while the tile is seen (ADR-20261119-pxn), so
  // the tile carries either control.
  it('offers the live view of a printer that has a camera', async () => {
    await openFrames();

    await expect
      .element(
        page.getByRole('button', { name: /^(Play front live|Stop front)$/ }),
      )
      .toBeInTheDocument();
  });

  // One camera is serving and the other printer has none, so exactly one tile
  // carries a camera and the other keeps its artwork.
  it('leaves a printer with no camera its artwork', async () => {
    await openFrames();

    await expect
      .poll(() =>
        page.getByRole('button', { name: /^(Play|Stop) / }).elements(),
      )
      .toHaveLength(1);
  });
});

describe('the printers nearby on the printers page', () => {
  // A printer the server hears and has not connected is offered with the
  // guide that adds it. One that is connected here is already in the
  // listing above, so it is not offered again.
  it('offers the printers not on this server, each with the guide', async () => {
    const routes: Routes = {
      ...SERVED,
      'GET /api/v1/discovery/nearby': json(200, {
        items: [
          {
            address: '192.168.1.20',
            name: 'Alpha',
            port: 80,
            services: ['_prusalink._tcp'],
            tls: false,
            url: 'http://192.168.1.20:80/',
            device_id: 'a',
          },
          {
            address: '192.168.1.21',
            name: 'Workshop MK4',
            port: 80,
            services: ['_prusalink._tcp'],
            tls: false,
            url: 'http://192.168.1.21:80/',
          },
        ],
      }),
    };
    await render(Printers, { props: { ctx: appContext(routes) } });

    await expect
      .element(page.getByRole('link', { name: 'Workshop MK4' }))
      .toHaveAttribute('href', 'http://192.168.1.21:80/');
    // Alpha is connected, so the one offer is the workshop printer's.
    const add = page.getByRole('link', { name: 'Add printer', exact: true });
    await expect.element(add).toHaveAttribute('href', '/printers/add');
    await expect.element(add).toHaveAccessibleDescription('Workshop MK4');
    expect(document.querySelector('a[href="http://192.168.1.20:80/"]')).toBe(
      null,
    );
  });
});

describe('removing a printer from the printers page', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends the retirement once the reader confirms', async () => {
    // The card offers removal only to a reader who may administer the printer.
    const routes: Routes = {
      ...SERVED,
      'GET /api/v1/devices': json(200, {
        items: [
          {
            ...deviceOf('a', 'Alpha'),
            actions: ['observe', 'operate', 'configure', 'administer'],
          },
          deviceOf('b', 'Beta'),
        ],
        total: 2,
      }),
      'DELETE /api/v1/devices/a': empty(204),
    };
    // The store that retires reaches the core through the app's own client.
    const fetcher = stubFetch(routes);
    vi.stubGlobal('fetch', fetcher.fetch);
    await render(Printers, { props: { ctx: appContext(routes) } });
    await expect.element(page.getByText('Alpha').first()).toBeInTheDocument();
    // The view a test before chose is kept, and only a card has the menu.
    await page.getByRole('radio', { name: 'Cards' }).click();

    await page.getByRole('button', { name: 'Actions for Alpha' }).click();
    await page.getByRole('menuitem', { name: 'Remove printer' }).click();
    await page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Remove' })
      .click();

    await expect.poll(() => fetcher.count('DELETE /api/v1/devices/a')).toBe(1);
  });
});

// ADR-20260930-rq4.
describe.skipIf(!FEATURES.tags)('the tag filter on the printers page', () => {
  const tagged = (id: string, name: string, tags: string[]) => ({
    ...deviceOf(id, name),
    tags,
  });

  // The server answers the printers carrying the tags the query names, and
  // advertises every tag the fleet carries.
  const taggedRoutes = (asked: string[][]): Routes => ({
    'GET /api/v1/devices': (call) => {
      const rows = [
        tagged('a', 'Alpha', ['workshop']),
        tagged('b', 'Beta', ['night shift']),
      ];
      const tags = new URLSearchParams(call.query).getAll('tag');
      asked.push(tags);
      const items = rows.filter((row) =>
        tags.every((tag) => row.tags.includes(tag)),
      );

      return json(200, {
        items,
        total: items.length,
        limit: 50,
        offset: 0,
        filters: [
          {
            param: 'tag',
            kind: 'multi',
            values: [
              { value: 'workshop', count: 1 },
              { value: 'night shift', count: 1 },
            ],
          },
        ],
        issues: { affected: 0, counts: {}, by_severity: {} },
      });
    },
  });

  // The chosen tags live in the address, so a copied link opens the same list.
  it('reads the chosen tags from the address and asks the server', async () => {
    const asked: string[][] = [];
    const ctx = appContext(taggedRoutes(asked), { search: '?tag=workshop' });
    await render(Printers, { props: { ctx } });

    await expect.poll(() => asked).toContainEqual(['workshop']);
    await expect.element(page.getByText('Alpha').first()).toBeInTheDocument();
    expect(page.getByText('Beta').query()).toBeNull();
    await expect
      .element(
        page.getByText('The list shows only the printers tagged workshop.'),
      )
      .toBeInTheDocument();
  });

  it('writes a chosen tag into the address', async () => {
    const went: { path: string; replace: boolean | undefined }[] = [];
    const ctx = appContext(taggedRoutes([]), {
      go: (path, options) => {
        went.push({ path, replace: options?.replace });
      },
    });
    await render(Printers, { props: { ctx } });
    await expect.element(page.getByText('Beta').first()).toBeInTheDocument();

    await page.getByRole('button', { name: /With all tags/ }).click();
    await page.getByRole('button', { name: /night shift/ }).click();

    expect(went).toEqual([
      { path: '/printers?tag=night+shift', replace: true },
    ]);
  });

  it('clears the chosen tags from the address', async () => {
    const went: string[] = [];
    const ctx = appContext(taggedRoutes([]), {
      search: '?tag=workshop',
      go: (path) => {
        went.push(path);
      },
    });
    await render(Printers, { props: { ctx } });

    await page.getByRole('button', { name: 'Show every printer' }).click();

    expect(went).toEqual(['/printers']);
  });
});

describe('the printers page at every home page setting', () => {
  const fleetOf = (count: number): Routes => ({
    'GET /api/v1/devices': json(200, {
      items: ['a', 'b', 'c'].slice(0, count).map((id) => deviceOf(id, id)),
      total: count,
    }),
    'GET /api/v1/cameras': json(200, { items: [] }),
  });

  // The setting decides where the app opens. The printers page stays the list,
  // so the rail entry and the Add printer button are always reachable.
  it.each([
    [1, 'auto'],
    [2, 'auto'],
    [3, 'auto'],
    [1, 'my-printer'],
    [3, 'my-printer'],
    [1, 'all-printers'],
  ] as const)(
    'lists %i printers under %s and goes nowhere',
    async (count, home) => {
      const preferences = new Preferences(null);
      preferences.setHome(home);
      preferences.setPrinter('b');

      const went: string[] = [];
      const ctx = appContext(fleetOf(count), {
        preferences,
        go: (path) => {
          went.push(path);
        },
      });
      await render(Printers, { props: { ctx } });

      await expect.poll(() => ctx.fleet.loaded).toBe(true);
      await tick();

      await expect
        .element(page.getByRole('button', { name: 'Add printer' }))
        .toBeInTheDocument();
      expect(went).toEqual([]);
    },
  );
});
