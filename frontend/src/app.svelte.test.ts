// A screen that throws must not take the navigation with it, moving between
// addresses one screen serves must not remount it, and a failure links to
// Diagnostics only where the person can follow the link.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import fixtures from '../test/ux/fixtures/api.json';
import { screen } from '../test/screen-probe';

vi.mock('./lib/screens', async () => {
  const { default: Counting } = await import('../test/counting-screen.svelte');
  const { default: Diagnostics } =
    await import('./lib/screens/diagnostics.svelte');
  const { default: Extensions } =
    await import('./lib/screens/extensions.svelte');
  const { default: Server } = await import('./lib/screens/server.svelte');

  return {
    screenFor: (pattern: string | undefined) => {
      if (pattern === '/settings/diagnostics') return Diagnostics;
      if (pattern === '/settings/extensions') return Extensions;
      if (pattern?.startsWith('/settings/server/') === true) return Server;

      return Counting;
    },
  };
});

interface Fixture {
  status: number;
  type: string;
  body: string;
}

const go = (path: string): void => {
  history.pushState(null, '', path);
  dispatchEvent(new PopStateEvent('popstate'));
};

// Answers from the recorded fixtures, with some rows replaced.
const serve = (replaced: Record<string, Fixture> = {}): void => {
  const table = { ...(fixtures as Record<string, Fixture>), ...replaced };
  vi.stubGlobal(
    'fetch',
    vi.fn((input: Request | string, init?: RequestInit) => {
      const href = typeof input === 'string' ? input : input.url;
      const method =
        typeof input === 'string' ? (init?.method ?? 'GET') : input.method;
      const row = table[`${method} ${new URL(href, location.href).pathname}`];

      return Promise.resolve(
        new Response(row?.body ?? '{}', {
          status: row?.status ?? 404,
          headers: { 'content-type': row?.type ?? 'application/json' },
        }),
      );
    }),
  );
};

const refused: Fixture = {
  status: 401,
  type: 'application/json',
  body: '{"title":"Unauthorized","status":401}',
};

const report = (): Element | null =>
  document.querySelector('a[href="/settings/diagnostics"][class~="underline"]');

beforeEach(() => {
  serve();
  screen.mounts = 0;
  screen.failing = false;
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('the application boundary', () => {
  it('keeps the navigation standing when one screen cannot be drawn', async () => {
    screen.failing = true;
    const { default: App } = await import('./app.svelte');

    await render(App);

    await expect.element(page.getByText('Something broke')).toBeVisible();
    await expect.element(page.getByRole('navigation').first()).toBeVisible();
  });

  it('keeps the screen mounted across addresses it serves', async () => {
    const { default: App } = await import('./app.svelte');

    await render(App);
    await expect
      .element(page.getByTestId('counting-screen'))
      .toBeInTheDocument();
    const mounts = screen.mounts;

    go('/printers/one/telemetry');
    go('/printers/one/cameras');

    await expect
      .element(page.getByTestId('counting-screen'))
      .toBeInTheDocument();
    expect(screen.mounts).toBe(mounts);
  });

  it('draws a failed screen again after a move to another address', async () => {
    screen.failing = true;
    const { default: App } = await import('./app.svelte');

    await render(App);
    await expect.element(page.getByText('Something broke')).toBeVisible();

    screen.failing = false;
    go('/printers/two');

    await expect
      .element(page.getByTestId('counting-screen'))
      .toBeInTheDocument();
    await expect
      .element(page.getByText('Something broke'))
      .not.toBeInTheDocument();
  });

  it('offers no report link before sign-in', async () => {
    serve({
      'GET /api/v1/principal': refused,
      'POST /api/v1/auth/login': refused,
    });
    const { default: App } = await import('./app.svelte');

    await render(App);
    await page.getByLabelText('Username').fill('demo');
    await page.getByLabelText('Password').fill('wrong');
    await page.getByRole('button', { name: 'Sign in' }).click();

    await expect.element(page.getByRole('alert')).toBeVisible();
    expect(report()).toBeNull();
  });

  it('offers no report link on Diagnostics itself', async () => {
    const { default: App } = await import('./app.svelte');

    await render(App);
    go('/settings/diagnostics');
    await page.getByRole('checkbox', { name: /agree to share it/ }).click();
    await page.getByRole('button', { name: 'Download diagnostics' }).click();

    await expect.element(page.getByRole('alert')).toBeVisible();
    expect(report()).toBeNull();
  });

  // ADR-20261019-h76 rule 15, through the app's own wiring rather than a test copy of
  // it: the catalogue reads the key from GET /api/v1/ui.
  it('tells the Extensions page that the operator switched modules off', async () => {
    const ui = (fixtures as Record<string, Fixture>)['GET /api/v1/ui'];
    serve({
      'GET /api/v1/ui': {
        status: 200,
        type: 'application/json',
        body: JSON.stringify({
          ...(JSON.parse(ui?.body ?? '{}') as Record<string, unknown>),
          modules: false,
        }),
      },
    });
    const { default: App } = await import('./app.svelte');

    await render(App);
    go('/settings/extensions');

    await expect
      .element(
        page
          .getByText(/installs no extension and runs none it installed/u)
          .first(),
      )
      .toBeVisible();
  });

  // The banner shows a restart wait on every page, so moving to another page
  // must leave the wait running.
  it('keeps a restart wait across a change of page', async () => {
    const ui = (fixtures as Record<string, Fixture>)['GET /api/v1/ui'];
    const json = (body: unknown, status = 200): Fixture => ({
      status,
      type: 'application/json',
      body: JSON.stringify(body),
    });
    serve({
      'GET /api/v1/ui': json({
        ...(JSON.parse(ui?.body ?? '{}') as Record<string, unknown>),
        modules: false,
        safe_mode: { reason: 'restart', since: '2026-10-30T09:12:00Z' },
      }),
      'GET /api/v1/server/version': json({ version: '1.2.0' }),
      'GET /api/v1/server/upgrade/history': json([]),
      'POST /api/v1/server/restart': json({}, 202),
    });
    const { default: App } = await import('./app.svelte');

    await render(App);
    await page.getByRole('button', { name: 'Restart normally' }).click();
    await expect
      .element(page.getByRole('button', { name: 'Restarting…' }))
      .toBeVisible();

    go('/settings/extensions');
    await new Promise((resolve) => setTimeout(resolve, 300));

    await expect
      .element(page.getByRole('button', { name: 'Restarting…' }))
      .toBeVisible();
  });

  // The Server room reads core.upgrade.enabled from GET /api/v1/ui, through
  // the app's own wiring rather than a test copy of it.
  describe('with upgrades switched off', () => {
    const asked = (): string[] =>
      vi
        .mocked(fetch)
        .mock.calls.map(
          ([input]) =>
            new URL(
              typeof input === 'string' ? input : (input as Request).url,
              location.href,
            ).pathname,
        );

    beforeEach(() => {
      const ui = (fixtures as Record<string, Fixture>)['GET /api/v1/ui'];
      const json = (body: unknown): Fixture => ({
        status: 200,
        type: 'application/json',
        body: JSON.stringify(body),
      });
      serve({
        'GET /api/v1/ui': json({
          ...(JSON.parse(ui?.body ?? '{}') as Record<string, unknown>),
          upgrades: false,
        }),
        'GET /api/v1/server/version': json({ version: '1.2.0' }),
      });
    });

    it('draws no Updates and no Backups tab, and keeps the restart', async () => {
      const { default: App } = await import('./app.svelte');

      await render(App);
      go('/settings/server/overview');

      await expect.element(page.getByText('1.2.0')).toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Restart in safe mode' }))
        .toBeVisible();
      await expect
        .element(page.getByRole('tab', { name: 'Overview', exact: true }))
        .toBeVisible();
      expect(
        page.getByRole('tab', { name: 'Updates', exact: true }).elements(),
      ).toEqual([]);
      expect(
        page.getByRole('tab', { name: 'Backups', exact: true }).elements(),
      ).toEqual([]);
      expect(
        asked().filter(
          (path) =>
            path.startsWith('/api/v1/server/upgrade') ||
            path.startsWith('/api/v1/server/backups') ||
            path.startsWith('/api/v1/server/beta-portal-pairing'),
        ),
      ).toEqual([]);
    });

    it.each(['/settings/server/updates', '/settings/server/backups'])(
      'answers %s with the not-found screen',
      async (address) => {
        const { default: App } = await import('./app.svelte');

        await render(App);
        go(address);

        await expect.element(page.getByText(address)).toBeVisible();
        expect(
          document.querySelector('[data-slot="not-found"]'),
        ).not.toBeNull();
      },
    );
  });
});

describe('the skin', () => {
  afterEach(() => {
    localStorage.removeItem('skin');
  });

  it('forgets a skin an older build stored', async () => {
    localStorage.setItem('skin', 'slicer');
    history.pushState(null, '', '/printers/one');
    const { default: App } = await import('./app.svelte');

    await render(App, { props: { useServerAppearance: true } });
    await expect
      .element(page.getByTestId('counting-screen'))
      .toBeInTheDocument();

    expect(localStorage.getItem('skin')).toBeNull();
  });
});
