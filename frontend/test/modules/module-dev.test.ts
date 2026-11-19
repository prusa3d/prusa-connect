// A module developed behind its own dev server (ADR-20260924-5nx), in a real
// Chromium. `moduleDev()` keeps `/@vite/client`, `/src/` and every other path
// a Vite dev server answers for itself, so it only works in front of a host
// that serves the built app: a core with the app embedded, or, here, Vite's
// own preview server. A dev server in that spot claims those same paths for
// its own app and the module server never sees them, so the page reloads
// forever chasing a 404 on its own `/src/main.ts`.
//
// The chain this test drives is: the module's dev server, in front of a
// preview server built from this app, in front of the demo the proof
// extensions are installed on. Run by `scripts/check-example-spools.sh`.
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

import type { Browser, Page } from 'playwright';
import {
  build,
  createServer,
  preview,
  type PreviewServer,
  type ViteDevServer,
} from 'vite';
import { afterAll, beforeAll, expect, it } from 'vitest';

import { launch } from '../ux/measure.ts';

const FRONTEND_ROOT = fileURLToPath(new URL('../../', import.meta.url));
const SPOOLS_UI = fileURLToPath(
  new URL('../../../examples/spools/ui/', import.meta.url),
);
const PAGE = `${SPOOLS_UI}src/spools-page/spools-page.svelte`;
const ROUTE = '/m/com.github.aux.acme.spools/spools';

let browser: Browser | undefined;
let sheet: Page;
let app: PreviewServer | undefined;
let dev: ViteDevServer | undefined;
let outDir = '';
let base = '';
let original = '';
let originalRead = false;
// Captured before anything that can throw, so afterAll always restores the
// value this test found rather than the empty string its own declaration
// starts from.
const originalCoreUrl: string | undefined = process.env.PCL_CORE_URL;
const pageErrors: string[] = [];

beforeAll(async () => {
  original = await readFile(PAGE, 'utf8');
  originalRead = true;

  // The demo core this whole lane drives (set by
  // scripts/check-example-spools.sh, read by global-setup.ts for the other
  // test files here too), not the app's own dev server: that is exactly the
  // dev server moduleDev must not sit behind.
  const core = originalCoreUrl;
  if (core === undefined || core === '') {
    throw new Error(
      'PCL_CORE_URL names the demo core this lane drives. scripts/check-example-spools.sh sets it.',
    );
  }

  outDir = await mkdtemp(join(tmpdir(), 'module-dev-app-'));
  await build({
    root: FRONTEND_ROOT,
    logLevel: 'error',
    build: { outDir, emptyOutDir: true },
  });
  app = await preview({
    root: FRONTEND_ROOT,
    logLevel: 'error',
    build: { outDir },
    preview: {
      host: '127.0.0.1',
      port: 0,
      proxy: { '/api': { target: core, changeOrigin: false, ws: true } },
    },
  });
  const appUrl = (app.resolvedUrls?.local[0] ?? '').replace(/\/$/u, '');

  // Read by the module's config as it loads, so it is set first. The dev
  // server is pointed at the built app's preview server, which carries the
  // API through to the demo core.
  process.env.PCL_CORE_URL = appUrl;
  dev = await createServer({
    root: SPOOLS_UI,
    configFile: `${SPOOLS_UI}vite.config.ts`,
    logLevel: 'error',
    server: { host: '127.0.0.1', port: 0 },
  });
  await dev.listen();
  base = (dev.resolvedUrls?.local[0] ?? '').replace(/\/$/u, '');

  browser = await launch();
  sheet = await (await browser.newContext()).newPage();
  // A second Svelte runtime shows up as `effect_orphan` here, never as a
  // rejection, so it is caught by assertion, not a throw.
  sheet.on('pageerror', (error) => pageErrors.push(String(error)));

  await sheet.goto(`${base}/`, { waitUntil: 'domcontentloaded' });
  await sheet.getByLabel('Username').fill(process.env.PCL_DEMO_USER ?? 'demo');
  // Exact: the field's reveal toggle is named Show Password, which a
  // substring match on the label finds too.
  await sheet
    .getByLabel('Password', { exact: true })
    .fill(process.env.PCL_DEMO_PASSWORD ?? '');
  await sheet.getByRole('button', { name: 'Sign in' }).click();
  await sheet.waitForSelector('[data-slot="rail"]', { timeout: 60_000 });
});

afterAll(async () => {
  try {
    // Only what beforeAll actually reached is there to close or undo. A
    // failure partway through (a bad core, a build error) must still leave
    // the fixture, the temp build and the environment as they were found.
    if (originalRead) await writeFile(PAGE, original);
    await browser?.close();
    await dev?.close();
    await app?.close();
  } finally {
    if (outDir !== '') await rm(outDir, { recursive: true, force: true });
    if (originalCoreUrl === undefined) {
      delete process.env.PCL_CORE_URL;
    } else {
      process.env.PCL_CORE_URL = originalCoreUrl;
    }
  }
});

it('replaces an edited component without reloading the page', async () => {
  // Vite sends an update only to the clients connected at the moment of the
  // edit, and its client drops an update for a module that has not run yet.
  // So the edit waits for both: the HMR socket open, and the page's own markup
  // drawn by the module.
  const connected = sheet.waitForEvent('console', {
    predicate: (message) => message.text() === '[vite] connected.',
    timeout: 60_000,
  });
  await sheet.goto(`${base}${ROUTE}`, { waitUntil: 'domcontentloaded' });
  await connected;
  await sheet.waitForSelector('[data-part="principal"]', { timeout: 60_000 });
  // Survives an HMR update and is gone after a reload.
  await sheet.evaluate(() => {
    (window as unknown as { unreloaded: boolean }).unreloaded = true;
  });

  await writeFile(PAGE, `${original}\n<p data-probe>hot</p>\n`);
  await sheet.waitForSelector('[data-probe]', { timeout: 30_000 });

  expect(
    await sheet.evaluate(
      () => (window as unknown as { unreloaded?: boolean }).unreloaded,
    ),
  ).toBe(true);
  expect(pageErrors).toEqual([]);
});
