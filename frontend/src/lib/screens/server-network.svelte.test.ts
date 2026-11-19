// The Network tab of the Server room: the switch that hides this server from
// slicers and apps on its network, sent to the core and drawn from the core's
// answer.
import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { appContext } from '../../test/app-context';
import { json, problem, type Routes } from '../../test/stub';
import { PATHS } from '../routes';
import Server from './server.svelte';

const visible = { hidden: false, announcement: 'announcing' };
const timeOff = { enabled: false, state: 'off', port: 123 };
const noOrigins = { items: [] };

describe('the Network tab of the Server room', () => {
  it('hides the server when the switch is turned on', async () => {
    const sent: (string | undefined)[] = [];
    const routes: Routes = {
      'GET /api/v1/discovery/visibility': json(200, visible),
      'GET /api/v1/time-server': json(200, timeOff),
      'GET /api/v1/allowed-origins': json(200, noOrigins),
      'PUT /api/v1/discovery/visibility': (call) => {
        sent.push(call.body);

        return json(200, { hidden: true, announcement: 'hidden' });
      },
    };

    await render(Server, {
      props: { ctx: appContext(routes, { path: PATHS.settingsServerNetwork }) },
    });

    const control = page.getByRole('switch', { name: 'Hide this server' });
    await expect.element(control).toHaveAttribute('aria-checked', 'false');

    await control.click();

    await expect.element(control).toHaveAttribute('aria-checked', 'true');
    expect(sent).toEqual([JSON.stringify({ hidden: true })]);
  });

  // A viewer reads the setting and the core refuses the change.
  it('leaves the switch off when the core refuses the change', async () => {
    const routes: Routes = {
      'GET /api/v1/discovery/visibility': json(200, visible),
      'GET /api/v1/time-server': json(200, timeOff),
      'GET /api/v1/allowed-origins': json(200, noOrigins),
      'PUT /api/v1/discovery/visibility': problem(
        403,
        'core.auth/ACCESS_DENIED',
      ),
    };

    await render(Server, {
      props: { ctx: appContext(routes, { path: PATHS.settingsServerNetwork }) },
    });

    const control = page.getByRole('switch', { name: 'Hide this server' });
    await control.click();

    await expect.element(page.getByRole('alert')).toBeVisible();
    await expect.element(control).toHaveAttribute('aria-checked', 'false');
  });

  it('starts the time server when its switch is turned on', async () => {
    const sent: (string | undefined)[] = [];
    const routes: Routes = {
      'GET /api/v1/discovery/visibility': json(200, visible),
      'GET /api/v1/time-server': json(200, timeOff),
      'GET /api/v1/allowed-origins': json(200, noOrigins),
      'PUT /api/v1/time-server': (call) => {
        sent.push(call.body);

        return json(200, { enabled: true, state: 'serving', port: 123 });
      },
    };

    await render(Server, {
      props: { ctx: appContext(routes, { path: PATHS.settingsServerNetwork }) },
    });

    const control = page.getByRole('switch', {
      name: 'Answer the time on this network',
    });
    await expect.element(control).toHaveAttribute('aria-checked', 'false');

    await control.click();

    await expect.element(control).toHaveAttribute('aria-checked', 'true');
    expect(sent).toEqual([JSON.stringify({ enabled: true })]);
  });

  it('adds a site to the pages that may call this server', async () => {
    const sent: (string | undefined)[] = [];
    const extension = {
      origin: 'https://slicer.example',
      source: 'extension',
      extension: 'com.github.acme.slicer',
      title: 'Web slicer',
    };
    const routes: Routes = {
      'GET /api/v1/discovery/visibility': json(200, visible),
      'GET /api/v1/time-server': json(200, timeOff),
      'GET /api/v1/allowed-origins': json(200, { items: [extension] }),
      'PUT /api/v1/allowed-origins': (call) => {
        sent.push(call.body);

        return json(200, {
          items: [
            { origin: 'https://dashboard.example', source: 'settings' },
            extension,
          ],
        });
      },
    };

    await render(Server, {
      props: { ctx: appContext(routes, { path: PATHS.settingsServerNetwork }) },
    });

    await expect
      .element(page.getByRole('link', { name: /The extension Web slicer/ }))
      .toHaveAttribute('href', '/settings/extensions/com.github.acme.slicer');

    await page
      .getByRole('textbox', { name: 'Address of the site' })
      .fill('https://dashboard.example');
    await page.getByRole('button', { name: 'Allow' }).click();

    await expect
      .element(
        page.getByRole('button', { name: 'Remove https://dashboard.example' }),
      )
      .toBeVisible();
    expect(sent).toEqual([
      JSON.stringify({ origins: ['https://dashboard.example'] }),
    ]);
  });
});
