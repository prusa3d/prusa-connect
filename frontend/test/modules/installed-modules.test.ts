// What a person sees after a module is installed.
//
// The core this drives has the two proof extensions on it and one fixture that
// must never run: `examples/spools` adds a page, `examples/nozzle-watch` adds a
// section to the printer card, and `test/modules/library-mismatch` declares a
// library range this app is outside of. The install is
// `scripts/check-example-spools.sh`, which starts the demo and runs this file
// against it.
//
// The measures are the ones earlier research sets: nothing moves when
// something loads or appears. A module arrives on the page after the core's own
// screens, which is the worst case for that rule, so each route is read the
// same way the measure lane reads one.
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, beforeAll, describe, expect, inject, it } from 'vitest';

import {
  cls,
  install,
  launch,
  moved,
  settled,
  VIEWPORT,
} from '../ux/measure.ts';

const url = inject('moduleAppUrl');

/** The namespaces the host derives from the module paths the script installs. */
const SPOOLS = 'com.github.aux.acme.spools';
const NOZZLE = 'com.github.aux.acme.nozzle-watch';
const MISMATCH = 'com.github.aux.acme.library-mismatch';

const SPOOLS_PAGE = `/m/${SPOOLS}/spools`;
const SPOOLS_ENTRY = `[data-slot="rail"] a[href="${SPOOLS_PAGE}"]`;
const NOZZLE_CARD = '[data-slot="device-card"][data-section="nozzle"]';
const NOZZLE_SECTION = '[data-slot="device-section"][data-section="nozzle"]';
const NOZZLE_TAB = '[data-slot="device-tab"][data-section="nozzle"]';

/** What a screen that is filling may move. Nothing, to the nearest hundredth. */
const CEILING = 0.02;

/** The five words the nozzle module's own catalogue carries for a heater. */
const HEATER_WORDS = [
  'At temperature',
  'Cooling',
  'Heating',
  'Not reported',
  'Off',
];

let browser: Browser;
let context: BrowserContext;
let sheet: Page;

/** The account the demo printed, which the script reads off its log. */
const account = (): { username: string; password: string } => {
  const password = process.env.PCL_DEMO_PASSWORD ?? '';
  if (password === '') {
    throw new Error('PCL_DEMO_PASSWORD carries the password the demo printed');
  }

  return { username: process.env.PCL_DEMO_USER ?? 'demo', password };
};

/** Opens a route in the one signed-in context and lets the screen settle. */
const open = async (path: string): Promise<void> => {
  await sheet.goto(`${url}${path}`, { waitUntil: 'domcontentloaded' });
  await sheet.waitForSelector('main', { timeout: 60_000 });
  // At most the three seconds every screen used to wait, and less once
  // nothing is on the way and nothing has changed for a moment.
  await settled(sheet, 3000);
};

/** The text of the first element a selector finds, once it is on the screen. */
const textOf = async (selector: string): Promise<string> => {
  await sheet.waitForSelector(selector, { state: 'visible', timeout: 60_000 });

  return (await sheet.locator(selector).first().innerText()).trim();
};

const onScreen = (selector: string): Promise<boolean> =>
  sheet.locator(selector).first().isVisible();

/** Opens the first printer's page and answers its id, read off the URL. */
const openFirstPrinter = async (): Promise<string> => {
  await open('/printers');
  await sheet.locator('a[href^="/printers/"]').first().click();
  await sheet.waitForSelector(NOZZLE_SECTION, { timeout: 60_000 });
  await settled(sheet, 1000);

  return new URL(sheet.url()).pathname.split('/')[2] ?? '';
};

beforeAll(async () => {
  browser = await launch();
  context = await browser.newContext({ viewport: VIEWPORT });
  // Installed before the first navigation, so the first shift is counted too.
  await context.addInitScript(install);
  sheet = await context.newPage();

  const { username, password } = account();
  await sheet.goto(`${url}/`, { waitUntil: 'domcontentloaded' });
  await sheet.getByLabel('Username').fill(username);
  // Exact: the field's reveal toggle is named Show Password, which a
  // substring match on the label finds too.
  await sheet.getByLabel('Password', { exact: true }).fill(password);
  await sheet.getByRole('button', { name: 'Sign in' }).click();
  // The rail is drawn once the session and the module listing have answered.
  await sheet.waitForSelector('[data-slot="rail"]', { timeout: 60_000 });
});

afterAll(async () => {
  await context.close();
  await browser.close();
});

describe('a module that adds a page', () => {
  it('is in the navigation under Extensions, and opens', async () => {
    await open('/');

    // The heading is drawn in capitals, so the text is read without case.
    expect(await textOf('[data-slot="rail"]')).toMatch(/extensions/iu);
    expect(await textOf(SPOOLS_ENTRY)).toBe('Spools');

    await sheet.locator(SPOOLS_ENTRY).click();
    await sheet.waitForSelector(`[data-slot="page"][data-module="${SPOOLS}"]`, {
      timeout: 60_000,
    });

    expect(new URL(sheet.url()).pathname).toBe(SPOOLS_PAGE);
  });

  // frontend-i18n.md rule 7: the module ships its own catalogue and the host
  // merges it under the module's namespace. These two lines are in that file
  // and in no catalogue of the app's.
  it('draws its own texts, from its own catalogue', async () => {
    await open(SPOOLS_PAGE);

    const drawn = await textOf(`[data-slot="page"][data-module="${SPOOLS}"]`);

    expect(drawn).toContain('On the shelf');
    expect(drawn).toContain('Add a spool');
  });

  it('settles without moving anything', async () => {
    await open(SPOOLS_PAGE);

    expect(await cls(sheet), (await moved(sheet)).join(', ')).toBeLessThan(
      CEILING,
    );
  });
});

describe('a module that adds a section to the printer card', () => {
  it('renders with the heading the manifest declares', async () => {
    await openFirstPrinter();

    expect(await textOf(`${NOZZLE_SECTION} h3`)).toBe('Nozzle temperature');
  });

  it('reads the fleet the app already loaded, in its own words', async () => {
    await openFirstPrinter();

    expect(HEATER_WORDS).toContain(
      await textOf(`${NOZZLE_SECTION} [data-part="state"]`),
    );
    expect(await textOf(`${NOZZLE_SECTION} [data-part="target"]`)).not.toBe('');
  });

  it('settles without moving anything', async () => {
    await openFirstPrinter();

    expect(await cls(sheet), (await moved(sheet)).join(', ')).toBeLessThan(
      CEILING,
    );
  });
});

describe('a module that adds a card line, a tab and an action to a printer', () => {
  it("shows one line on each printer's card, in the Printers listing", async () => {
    await open('/printers');

    const line = await textOf(NOZZLE_CARD);
    expect(line).not.toBe('');
    expect(line.split('\n')).toHaveLength(1);
  });

  it("adds a tab that routes under the module's namespace", async () => {
    const id = await openFirstPrinter();

    await sheet.getByRole('tab', { name: 'Nozzle' }).click();
    await sheet.waitForSelector(NOZZLE_TAB, { timeout: 60_000 });

    expect(new URL(sheet.url()).pathname).toBe(
      `/printers/${id}/${NOZZLE}.nozzle`,
    );
  });

  it('runs mark-changed from the action menu, and the tab shows it', async () => {
    await openFirstPrinter();

    // The printer's own menu is in the page header, above the menu of each
    // print the demo's past put on the page.
    await sheet
      .getByRole('button', { name: /^Actions for/u })
      .first()
      .click();
    await sheet.getByRole('menuitem', { name: 'Mark nozzle changed' }).click();
    // The toast the action leaves, if it fails, would still be on screen;
    // its absence is not asserted, because the tab itself is the proof.
    await sheet.waitForTimeout(500);

    await sheet.getByRole('tab', { name: 'Nozzle' }).click();
    await sheet.waitForSelector(NOZZLE_TAB, { timeout: 60_000 });

    const shown = await textOf(NOZZLE_TAB);
    expect(shown).not.toContain('Never marked changed');
    expect(shown).toContain(String(new Date().getUTCFullYear()));
  });
});

describe('a module this app refuses', () => {
  it('is named in Settings with its reason and its code', async () => {
    await open('/settings/extensions');

    const refusal = await textOf(
      `[data-slot="alert"][data-module="${MISMATCH}"]`,
    );

    expect(refusal).toContain('component library');
    expect(refusal).toContain('library-mismatch');
  });

  it('leaves the rest of the app working', async () => {
    await open('/settings/extensions');

    // The module that does load is still in the rail, and the app still
    // navigates.
    expect(await onScreen(SPOOLS_ENTRY)).toBe(true);

    await sheet.locator('[data-slot="rail"] a[href="/printers"]').click();
    await sheet.waitForSelector('a[href^="/printers/"]', { timeout: 60_000 });

    expect(new URL(sheet.url()).pathname).toBe('/printers');
  });
});
