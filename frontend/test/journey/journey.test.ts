// The whole customer journey, in one browser, against a core that is really
// running: sign in, upload a print file, see its picture and what the slicer
// wrote, press Print on its row, pick a printer, confirm, land on that
// printer's page and watch the print being tracked.
//
// Every other lane here proves one screen against an answer written by hand.
// This one proves the requests: `scripts/check-journey.sh` starts `cmd/demo`
// with simulated printers and a storage on disk, and the core answers every
// call the page makes. So a route the application asks for and the server does
// not serve fails here and nowhere else.
//
// What the script puts in the environment:
//
//   PCL_DEMO_USER, PCL_DEMO_PASSWORD  the account the demo printed
//   PCL_JOURNEY_FILE                  the G-code file on disk, with a picture
//   PCL_JOURNEY_DEVICE                the printer that is free to take it
//   PCL_JOURNEY_DEVICE_NAME           what that printer is called
//   PCL_JOURNEY_STORAGE               the storage the file is uploaded to
import { basename } from 'node:path';

import type { Browser, Page } from 'playwright';
import { chromium } from 'playwright';
import { afterAll, beforeAll, expect, inject, it } from 'vitest';

/** The size the measure lane uses, so both lanes see one layout. */
const VIEWPORT = { width: 1280, height: 900 };

/** The picture the file carries, as the fixture writes it. */
const PICTURE = { width: 96, height: 64 };

const need = (name: string): string => {
  const value = process.env[name];
  if (value === undefined || value === '') {
    throw new Error(`${name} is not set: run \`make journey-check\``);
  }

  return value;
};

const account = {
  user: need('PCL_DEMO_USER'),
  password: need('PCL_DEMO_PASSWORD'),
};
const filePath = need('PCL_JOURNEY_FILE');
const fileName = basename(filePath);
const deviceId = need('PCL_JOURNEY_DEVICE');
const deviceName = need('PCL_JOURNEY_DEVICE_NAME');
const storageName = need('PCL_JOURNEY_STORAGE');

/** What `GET /api/v1/devices/{id}` says about the print on the machine. */
interface DeviceJob {
  readonly display_name?: string;
  readonly path?: string;
  readonly progress?: number;
  readonly running?: boolean;
  readonly state?: string;
}

let browser: Browser;
let page: Page;
let base: string;

/** The step number, so a failure says how far the journey got. */
let step = 0;
const say = (what: string): void => {
  step += 1;
  process.stdout.write(`    ${String(step).padStart(2)}. ${what}\n`);
};

beforeAll(async () => {
  base = inject('journeyBaseUrl');
  browser = await chromium.launch();
  const context = await browser.newContext({ viewport: VIEWPORT });
  page = await context.newPage();
}, 180_000);

afterAll(async () => {
  await browser.close();
});

/** Asks the core the same way the page does, over the session the browser holds. */
const askTheCore = async <T>(path: string): Promise<T> => {
  const body = await page.evaluate(async (where: string) => {
    const answer = await fetch(where, {
      headers: { accept: 'application/json' },
    });
    if (!answer.ok) {
      throw new Error(`${where} answered ${String(answer.status)}`);
    }

    return await answer.text();
  }, path);

  return JSON.parse(body) as T;
};

it('takes a file from the storage page onto a printer', async () => {
  await page.goto(`${base}/`, { waitUntil: 'domcontentloaded' });

  // ── Sign in ──────────────────────────────────────────────────────
  await page.getByRole('textbox', { name: 'Username' }).fill(account.user);
  await page.getByRole('textbox', { name: 'Password' }).fill(account.password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.waitForSelector('main', { timeout: 60_000 });
  say('the demo account signs in on the sign-in page');

  // ── The storage page ─────────────────────────────────────────────
  await page.getByRole('link', { name: 'Storage' }).first().click();
  await expect.poll(() => new URL(page.url()).pathname).toBe('/storage');
  await page.getByRole('heading', { name: 'Storage' }).first().waitFor();
  say('the rail opens the storage page');

  // ── The upload ───────────────────────────────────────────────────
  // The drop zone's own input, hidden from sight and not from the browser.
  await page.locator('input[type="file"]').setInputFiles(filePath);
  const row = page.locator(`[data-slot="clipped-name"][title="${fileName}"]`);
  await row.first().waitFor({ timeout: 60_000 });
  say(`the file is uploaded and ${fileName} is listed`);

  // The server has it, which is the claim the row alone does not make. The
  // row is drawn while the upload is still written under a `.part` name, so
  // the listing is read until the file stands under its own.
  await expect
    .poll(
      async () => {
        const listing = await askTheCore<{ items: { path: string }[] }>(
          `/api/v1/storages/${storageName}/files`,
        );

        return listing.items.map((item) => item.path);
      },
      { timeout: 60_000, interval: 500 },
    )
    .toContain(fileName);

  // ── The picture the slicer stored in the file ────────────────────
  const thumbnail = page
    .locator('button[data-slot="file-thumbnail"] img')
    .first();
  await thumbnail.waitFor({ timeout: 60_000 });
  await expect
    .poll(
      () =>
        thumbnail.evaluate((image: HTMLImageElement) => ({
          width: image.naturalWidth,
          height: image.naturalHeight,
        })),
      { timeout: 30_000 },
    )
    .toEqual(PICTURE);
  expect(await thumbnail.getAttribute('src')).toContain(
    `/api/v1/storages/${storageName}/thumbnail`,
  );
  say('the row draws the picture the file carries, at its own size');

  // ── The print sheet ──────────────────────────────────────────────
  await page.getByRole('button', { name: `Print ${fileName}` }).click();
  const sheet = page.getByRole('dialog');
  await sheet.getByText('Print this file').waitFor();

  // What the slicer wrote, read off the file by the server and drawn here.
  for (const fact of ['Estimated time', 'Filament', 'Material', 'Sliced for']) {
    await sheet.getByText(fact, { exact: true }).first().waitFor();
  }
  say('the print sheet names the file and what the slicer wrote');

  // ── The printer ──────────────────────────────────────────────────
  // The only free printer arrives ticked, so a click would untick it.
  await sheet.getByRole('checkbox', { name: new RegExp(deviceName) }).check();
  say(`${deviceName} is ticked in the sheet`);

  await sheet
    .getByRole('button', { name: 'Print on 1 printer', exact: true })
    .click();

  // ── The printer's page ───────────────────────────────────────────
  await expect
    .poll(() => new URL(page.url()).pathname, { timeout: 60_000 })
    .toBe(`/printers/${deviceId}`);
  say('the sheet closes and the browser lands on the printer page');

  // The server is what decides whether a print is running. The page is read
  // after it, so a page drawing a print the machine never took fails here.
  let job: DeviceJob | undefined;
  await expect
    .poll(
      async () => {
        const answer = await askTheCore<{ job?: DeviceJob }>(
          `/api/v1/devices/${deviceId}`,
        );
        job = answer.job;

        return job?.running === true;
      },
      { timeout: 60_000, interval: 1000 },
    )
    .toBe(true);

  expect(job?.display_name).toBe(fileName);
  expect(job?.path).toContain(fileName);
  expect(job?.state).toBe('PRINTING');
  expect(job?.progress).toBeGreaterThanOrEqual(0);
  say(`the core reports the print running on ${deviceName}`);

  // ── What a person sees on that page ──────────────────────────────
  await page
    .locator(`[data-slot="clipped-name"][title="${fileName}"]`)
    .first()
    .waitFor({ timeout: 60_000 });

  // The bar is named after the print it measures.
  const progress = page.getByRole('progressbar', {
    name: `${fileName} progress`,
  });
  await progress.waitFor({ timeout: 60_000 });

  // The bar reads a number rather than sitting indeterminate, and the figure
  // over it says the same. Both are read in one pass, because the print
  // advances while the page is open.
  await expect
    .poll(
      () =>
        progress.evaluate((bar: Element) => {
          const now = bar.getAttribute('aria-valuenow') ?? '';
          // A running print sets its figure large at the end of the row above
          // the bar, formatted with a space before the unit.
          const figure = (
            bar.parentElement?.previousElementSibling?.lastElementChild
              ?.textContent ?? ''
          ).replace(/\s/g, '');

          return /^\d+$/.test(now) && figure === `${now}%`;
        }),
      { timeout: 60_000 },
    )
    .toBe(true);
  say('the printer page shows the file name and a progress figure');
}, 180_000);
