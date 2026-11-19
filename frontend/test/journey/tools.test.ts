// The tool tree, end to end, in one browser against a core that is really
// running (ADR-20261025-pze): the printer page of a simulated XL and of a simulated
// MK4S with an MMU3 draws each head, each slot, the material in each, the
// temperatures and the nozzle, all read from the tool tree. A nozzle command
// for one head goes out with its tool counted from 0, and the machine heats
// that head.
//
// `scripts/check-journey.sh` starts `cmd/demo`, whose fleet holds an XL and
// an MK4S with an MMU3.
import type { Browser, Page } from 'playwright';
import { chromium } from 'playwright';
import { afterAll, beforeAll, expect, inject, it } from 'vitest';

const VIEWPORT = { width: 1280, height: 900 };

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

/** What the test reads of a printer in the listing. */
interface Listed {
  readonly id: string;
  readonly model: string;
  readonly connected: boolean;
  readonly state: Readonly<Record<string, unknown>>;
}

let browser: Browser;
let page: Page;
let base: string;

beforeAll(async () => {
  base = inject('journeyBaseUrl');
  browser = await chromium.launch();
  const context = await browser.newContext({ viewport: VIEWPORT });
  page = await context.newPage();

  await page.goto(`${base}/`, { waitUntil: 'domcontentloaded' });
  await page.getByRole('textbox', { name: 'Username' }).fill(account.user);
  await page.getByRole('textbox', { name: 'Password' }).fill(account.password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.waitForSelector('main', { timeout: 60_000 });
}, 180_000);

afterAll(async () => {
  await browser.close();
});

/** Asks the core over the session the browser holds. */
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

/** The first connected printer of a model whose tool tree has arrived. */
const printerOf = async (
  model: string,
  ready: (state: Listed['state']) => boolean,
): Promise<Listed> => {
  let found: Listed | undefined;

  await expect
    .poll(
      async () => {
        const listing = await askTheCore<{ items: Listed[] }>(
          '/api/v1/devices',
        );
        found = listing.items.find(
          (item) => item.model === model && item.connected && ready(item.state),
        );

        return found !== undefined;
      },
      { timeout: 60_000, interval: 500 },
    )
    .toBe(true);

  if (found === undefined) throw new Error(`no ${model} in the fleet`);

  return found;
};

/** The paths an earlier release wrote beside the tree. */
const OUTSIDE_THE_TREE = [
  '/material',
  '/temperature/nozzle',
  '/temperature/nozzle_target',
  '/active_tool',
];

it("draws an XL's heads from the tree and heats one by its 0-based tool", async () => {
  const xl = await printerOf(
    'XL',
    (state) => typeof state['/tools/4/nozzle_temp'] === 'number',
  );

  for (const path of OUTSIDE_THE_TREE) {
    expect(Object.keys(xl.state)).not.toContain(path);
  }

  await page.goto(`${base}/printers/${encodeURIComponent(xl.id)}/info`, {
    waitUntil: 'domcontentloaded',
  });

  const tools = page.getByRole('heading', { name: 'Tools' });
  await tools.waitFor({ timeout: 60_000 });

  // Each head has a row with its nozzle and the filament the simulator loads.
  // The filament chips in the page header carry the same `data-tool`, so a
  // row is asked for as a table row.
  for (const [head, material] of [
    ['0', 'PLA'],
    ['1', 'PETG'],
    ['2', 'ASA'],
    ['3', 'PLA'],
    ['4', 'PVA'],
  ] as const) {
    const row = page.locator(`tr[data-tool="${head}"]`);
    await expect.poll(() => row.textContent()).toContain(material);
    await expect.poll(() => row.textContent()).toMatch(/0\.4\s*mm/);
    await expect.poll(() => row.textContent()).toMatch(/°C/);
  }

  // The nozzle chip beside the name: Tool 1, which is head 0, heats to the
  // PETG figure. The editor is asked for its own buttons. The material chip in
  // the header is a button too, and it reads PETG while the print runs on head
  // 1, which holds PETG. A press on that chip opens the filament surface and
  // closes this one.
  await page
    .getByRole('button', { name: /^Set extruder target/ })
    .first()
    .click();
  const editor = page
    .locator('[data-slot="popover-content"]')
    .filter({ hasText: 'Extruder' });
  await editor.getByRole('button', { name: /^Tool 1 / }).click();
  await editor.getByRole('button', { name: 'PETG', exact: true }).click();
  await editor.getByRole('button', { name: 'Set', exact: true }).click();

  // The machine heats head 0 and no other: the simulator holds the target an
  // M104 sets on the head its T word names, counted from 0.
  await expect
    .poll(
      async () => {
        const device = await askTheCore<{ state: Listed['state'] }>(
          `/api/v1/devices/${encodeURIComponent(xl.id)}`,
        );

        return device.state['/tools/0/nozzle_temp'];
      },
      { timeout: 60_000, interval: 500 },
    )
    .toBe(230);

  const record = await askTheCore<{
    items: { verb: string; ok: boolean }[] | null;
  }>(`/api/v1/devices/${encodeURIComponent(xl.id)}/commands`);
  expect(
    (record.items ?? []).some(
      (command) =>
        command.verb === 'core.command.set_nozzle_temperature' && command.ok,
    ),
  ).toBe(true);
});

it('draws the slots of an MMU3 under its one head', async () => {
  const mmu = await printerOf(
    'MK4S',
    (state) => state['/tools/0/feeder/slot_count'] === 5,
  );

  for (const path of OUTSIDE_THE_TREE) {
    expect(Object.keys(mmu.state)).not.toContain(path);
  }

  await page.goto(`${base}/printers/${encodeURIComponent(mmu.id)}/info`, {
    waitUntil: 'domcontentloaded',
  });
  await page
    .getByRole('heading', { name: 'Tools' })
    .waitFor({ timeout: 60_000 });

  const head = page.locator('tr[data-tool="0"]');
  await expect
    .poll(() => head.textContent())
    .toMatch(/MMU3 with 5 slots|5 slots/);
  await expect.poll(() => head.textContent()).toMatch(/0\.4\s*mm/);

  for (const slot of ['0.0', '0.1', '0.2', '0.3', '0.4']) {
    await expect
      .poll(() => page.locator(`tr[data-tool="${slot}"]`).count())
      .toBe(1);
  }

  await expect
    .poll(() => page.locator('tr[data-tool="0.1"]').textContent())
    .toContain('PETG');
  await expect
    .poll(() => page.locator('tr[data-tool="0.4"]').textContent())
    .toContain('Nothing loaded');
});
