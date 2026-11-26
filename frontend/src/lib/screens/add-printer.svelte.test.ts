import type { StreamEvent } from '@prusa/connect';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { appContext } from '../../test/app-context';
import { json, problem, type Routes, stubFetch } from '../../test/stub';
import DeclaredAddPrinter from '../../../test/declared-add-printer.svelte';
import AddPrinter from './add-printer.svelte';

const open = async (
  ui: Record<string, unknown>,
  routes: Routes = {},
  screen: typeof AddPrinter = AddPrinter,
) => {
  const ctx = appContext({
    'GET /api/v1/ui': json(200, ui),
    'GET /api/v1/registrations': json(200, { items: [] }),
    'GET /api/v1/vocabularies/device-models': json(200, {
      connectors: [],
      format: 'device-models',
      items: [
        {
          connector: 'xbuddy',
          device_type_id: '1.4.0',
          display_name: 'MK4S',
          icon: 'printer-mk4',
          kind: 'printer',
          model: 'MK4S',
          vendor: 'Prusa',
        },
        {
          connector: 'bench',
          display_name: 'Bench Works Tool Changer',
          icon: 'printer-generic',
          kind: 'printer',
          model: 'Bench_One',
          vendor: 'Bench Works',
        },
      ],
      version: '',
      vocabulary: 'device-models',
    }),
    ...routes,
  });
  await ctx.settings.read();
  await render(screen, { props: { ctx } });

  return ctx;
};

const frame = (type: string, payload: Record<string, unknown>): StreamEvent =>
  // The cast stands in for the narrowed union the SDK yields for one type.
  ({ type, id: '1', payload, envelope: undefined }) as unknown as StreamEvent;

const next = async (): Promise<void> => {
  await page.getByRole('button', { name: 'Next' }).click();
};

describe('the add-printer page', () => {
  // A printer may reach this server by another name than the browser did,
  // and a settings file naming the browser's host fails TLS on the printer.
  it('writes the address the server names for printers', async () => {
    await open({
      printer_address: 'https://shop.example.net:8443',
      printer_admission: 'approval',
    });

    await expect
      .element(page.getByText(/hostname = shop\.example\.net/))
      .toBeInTheDocument();
    await expect.element(page.getByText(/port = 8443/)).toBeInTheDocument();
  });

  it('writes the address the page was opened on when the server names none', async () => {
    await open({ printer_admission: 'approval' });

    await expect
      .element(
        page.getByText(`hostname = ${window.location.hostname}`, {
          exact: false,
        }),
      )
      .toBeInTheDocument();
  });

  it('draws no step until the server says how printers join', async () => {
    await open({});

    await expect
      .element(
        page.getByRole('status', {
          name: 'Loading how printers join this server',
        }),
      )
      .toBeVisible();
  });

  // The whole journey on the real stores: the printer asks, a person
  // approves it, and it dials in, each through what the stream delivers.
  it('follows a printer from its code to its own page, without a reload', async () => {
    const ctx = await open(
      { printer_admission: 'approval' },
      {
        'POST /api/v1/registrations/K7QF2RXA9C/approve': json(200, {
          code: 'K7QF2RXA9C',
          connector: 'xbuddy',
          fingerprint: 'a1b2c3d4e5f60718',
          approved: true,
          device_id: 'd-1',
          model: '1.4.0',
        }),
        'GET /api/v1/devices/d-1': json(200, {
          id: 'd-1',
          name: 'MK4S 12345',
          model: 'MK4S',
          connected: true,
        }),
      },
    );
    await next();
    await next();

    await expect
      .element(page.getByText('Waiting for the printer'))
      .toBeVisible();

    ctx.registrations.apply(
      frame('com.prusa.connect.device.registration.announced', {
        code: 'K7QF2RXA9C',
        connector: 'xbuddy',
        fingerprint: 'a1b2c3d4e5f60718',
        model: '1.4.0',
        serial: '12345',
      }),
    );
    await expect.element(page.getByText('K7QF2RXA9C')).toBeVisible();
    // The printer reported a code, and the guide draws the model the code
    // names. The row's icon key names a family and no drawing.
    await expect
      .element(page.getByRole('img', { name: 'Original Prusa MK4S' }))
      .toHaveAttribute('src', '/img/printers/mk4s.svg');

    await page
      .getByRole('button', { name: 'Approve MK4S, code K7QF2RXA9C' })
      .click();
    await expect
      .element(page.getByText('MK4S is approved and is connecting.'))
      .toBeVisible();

    await ctx.fleet.apply(
      frame('com.prusa.connect.device.arrived', { device_id: 'd-1' }),
    );

    await expect
      .element(page.getByText('MK4S 12345 is connected'))
      .toBeVisible();
    await expect
      .element(page.getByRole('img', { name: 'Original Prusa MK4S' }))
      .toHaveAttribute('src', '/img/printers/mk4s.svg');
    await expect
      .element(page.getByRole('link', { name: 'Open the printer' }))
      .toHaveAttribute('href', '/printers/d-1');
  });

  it('says why a printer was turned away while the page waited', async () => {
    const ctx = await open({ printer_admission: 'automatic' });
    await next();
    await next();

    // A caller with no identity is not a printer, and says nothing here.
    ctx.registrations.apply(
      frame('com.prusa.connect.device.registration.refused', {
        connector: 'xbuddy',
        reason: 'not_admitted',
        admission: 'automatic',
      }),
    );
    ctx.registrations.apply(
      frame('com.prusa.connect.device.registration.refused', {
        connector: 'xbuddy',
        reason: 'unknown_credentials',
        admission: 'automatic',
        fingerprint_digest: 'abc',
        model: '1.4.0',
      }),
    );

    await expect
      .element(page.getByText('This server refused MK4S'))
      .toBeVisible();
    expect(page.getByText('This server refused a printer').query()).toBe(null);
  });

  // On a server that approves at once, the printer is let in while the person
  // still reads the printer step, and the arrival can be delivered before the
  // approval. Once in, it stays shown as joined when it drops again.
  it('follows a printer approved before the waiting step, and keeps it joined', async () => {
    let connected = true;
    const ctx = await open(
      { printer_admission: 'automatic' },
      {
        'GET /api/v1/devices/d-1': () =>
          json(200, {
            id: 'd-1',
            name: 'MK4S 12345',
            model: 'MK4S',
            connected,
          }),
      },
    );
    await next();

    await ctx.fleet.apply(
      frame('com.prusa.connect.device.arrived', { device_id: 'd-1' }),
    );
    ctx.registrations.apply(
      frame('com.prusa.connect.device.registration.approved', {
        code: 'K7QF2RXA9C',
        device_id: 'd-1',
        model: '1.4.0',
      }),
    );

    // The printer joined while the printer step was open, so the guide
    // moves to the last step by itself, with no Next to press.
    await expect
      .element(page.getByText('MK4S 12345 is connected'))
      .toBeVisible();

    connected = false;
    await ctx.fleet.apply(
      frame('com.prusa.connect.device.presence.changed', {
        device_id: 'd-1',
        connected: false,
      }),
    );
    expect(ctx.fleet.devices[0]?.connected).toBe(false);
    await expect
      .element(page.getByText('MK4S 12345 is connected'))
      .toBeVisible();
  });

  // The guide names a printer by the vocabulary's name for its model, and the
  // icon beside the name looks the model up by the spelling the printer
  // reported. A driver declared this one, and the printer spells it in its
  // own way, so the name and the icon have to find the row by one rule.
  it('names a declared printer on its icon while it asks and once it joins', async () => {
    const ctx = await open(
      { printer_admission: 'approval' },
      {
        'POST /api/v1/registrations/B3NCH0NE00/approve': json(200, {
          code: 'B3NCH0NE00',
          connector: 'bench',
          fingerprint: '0011223344556677',
          approved: true,
          device_id: 'd-9',
          model: 'bench one',
        }),
        'GET /api/v1/devices/d-9': json(200, {
          id: 'd-9',
          name: 'Bench 9',
          model: 'bench one',
          connected: true,
        }),
      },
      DeclaredAddPrinter,
    );
    await next();
    await next();

    ctx.registrations.apply(
      frame('com.prusa.connect.device.registration.announced', {
        code: 'B3NCH0NE00',
        connector: 'bench',
        fingerprint: '0011223344556677',
        model: 'bench one',
      }),
    );
    await expect
      .element(page.getByRole('img', { name: 'Bench Works Tool Changer' }))
      .toBeVisible();

    await page
      .getByRole('button', {
        name: 'Approve Bench Works Tool Changer, code B3NCH0NE00',
      })
      .click();
    await ctx.fleet.apply(
      frame('com.prusa.connect.device.arrived', { device_id: 'd-9' }),
    );

    await expect.element(page.getByText('Bench 9 is connected')).toBeVisible();
    await expect
      .element(page.getByRole('img', { name: 'Bench Works Tool Changer' }))
      .toBeVisible();
  });

  describe('connect.der', () => {
    afterEach(() => {
      vi.unstubAllGlobals();
    });

    const AUTHORITY = 'GET /api/v1/printer-authority';

    // The page reaches the authority through the app's own client, which
    // reads the global fetch.
    const openWith = async (ui: Record<string, unknown>, routes: Routes) => {
      const table: Routes = {
        'GET /api/v1/ui': json(200, ui),
        'GET /api/v1/registrations': json(200, { items: [] }),
        ...routes,
      };
      const fetcher = stubFetch(table);
      vi.stubGlobal('fetch', fetcher.fetch);
      await open(ui, table);

      return fetcher;
    };

    it('is fetched and offered when printers trust our own authority over TLS', async () => {
      const fetcher = await openWith(
        {
          printer_address: 'https://shop.example.net',
          printer_admission: 'approval',
          printer_custom_ca: true,
        },
        { [AUTHORITY]: new Response(new Blob(['DER'])) },
      );

      await expect
        .element(page.getByRole('link', { name: 'Download connect.der' }))
        .toHaveAttribute('download', 'connect.der');
      expect(fetcher.count(AUTHORITY)).toBe(1);
    });

    it('is not fetched when the authority is not our own', async () => {
      const fetcher = await openWith(
        {
          printer_address: 'https://shop.example.net',
          printer_admission: 'approval',
          printer_custom_ca: false,
        },
        {},
      );

      await expect
        .element(page.getByText(/hostname = shop\.example\.net/))
        .toBeInTheDocument();
      expect(fetcher.count(AUTHORITY)).toBe(0);
    });
  });
  describe('pairing at the printer', () => {
    afterEach(() => {
      vi.unstubAllGlobals();
    });

    const START = 'POST /api/v1/printer-pairings';
    const ui = {
      printer_address: 'http://192.168.1.50:8080',
      printer_admission: 'approval',
    };

    const openWith = async (routes: Routes) => {
      const table: Routes = {
        'GET /api/v1/ui': json(200, ui),
        'GET /api/v1/registrations': json(200, { items: [] }),
        ...routes,
      };
      const fetcher = stubFetch(table);
      vi.stubGlobal('fetch', fetcher.fetch);
      await open(ui, table);

      return fetcher;
    };

    it('asks the printer through this server, with the address printers dial', async () => {
      const fetcher = await openWith({
        [START]: json(201, {
          id: 'p1',
          code: 'K7QX-29FM',
          printer: '192.168.1.20:80',
          connect: { host: '192.168.1.50', port: 8080, tls: false },
          state: 'waiting',
          started_at: '2026-11-26T10:00:00Z',
        }),
        'GET /api/v1/printer-pairings/p1': json(200, {
          id: 'p1',
          code: 'K7QX-29FM',
          printer: '192.168.1.20:80',
          connect: { host: '192.168.1.50', port: 8080, tls: false },
          state: 'waiting',
          started_at: '2026-11-26T10:00:00Z',
        }),
      });

      await page.getByLabelText('Printer address').fill('192.168.1.20');
      await page.getByRole('button', { name: 'Ask the printer' }).click();

      await expect.element(page.getByText('K7QX-29FM')).toBeInTheDocument();

      const sent = fetcher.calls.find(
        (call) => `${call.method} ${call.path}` === START,
      );
      expect(JSON.parse(sent?.body ?? '{}')).toEqual({
        printer: '192.168.1.20',
        connect: { host: '192.168.1.50', port: 8080, tls: false },
      });
    });

    it('explains a printer whose firmware cannot pair, and keeps the file', async () => {
      await openWith({
        [START]: problem(422, 'core.printer/PAIRING_UNSUPPORTED'),
      });

      await page.getByLabelText('Printer address').fill('192.168.1.20');
      await page.getByRole('button', { name: 'Ask the printer' }).click();

      await expect
        .element(page.getByText(/add the printer with the settings file/))
        .toBeInTheDocument();
      await expect
        .element(
          page.getByRole('link', {
            name: 'Download prusa_printer_settings.ini',
          }),
        )
        .toBeInTheDocument();
    });
  });
});
