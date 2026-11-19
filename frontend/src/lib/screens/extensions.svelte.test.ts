import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { Modules } from '@prusa/connect-ui/data';
import { type ModuleContributes, SlotRegistry } from '@prusa/connect-ui/slots';

import { appContext } from '../../test/app-context';
import {
  type Handler,
  json,
  problem,
  type Routes,
  stubClient,
} from '../../test/stub';
import WithSlots from '../../test/with-slots.svelte';
import Extensions from './extensions.svelte';

const NAMESPACE = 'com.github.aux.acme.spools';

const contributes: ModuleContributes = {
  pages: [{ path: 'spools', title: 'Spools', hidden: false }],
  settings: [{ id: 'spools', title: 'Spools' }],
};

/** One enabled module with a browser half, as the resources listing carries it. */
const instance = () => ({
  kind: 'sink',
  name: 'spools',
  namespace: NAMESPACE,
  uid: 'uid-spools',
  enabled: true,
  status: { state: 'ready' },
  module: {
    version: '1.0.0',
    summary: 'Tracks the filament spools.',
    asked: {},
  },
  ui: {
    entry_url: `/api/v1/modules/${NAMESPACE}/ui/ui/index.js`,
    library: '>=0.1.0 <1.0.0',
    contributes,
  },
});

const listed = async (items: readonly unknown[]) => {
  const { client } = stubClient({
    'GET /api/v1/resources': json(200, { items }),
  });
  const modules = new Modules(client);
  await modules.load();

  return modules;
};

const open = (
  registry: SlotRegistry,
  modules: Modules,
  over: Parameters<typeof appContext>[1] = {},
  routes: Routes = {},
) =>
  render(WithSlots, {
    props: {
      registry,
      screen: Extensions,
      ctx: appContext(
        { 'GET /api/v1/modules/index': json(200, { items: [] }), ...routes },
        { modules, ...over },
      ),
    },
  });

/**
 * The row of one module, once the listing has drawn it. The locator matches
 * the text the row has now, so take it once the row has stopped changing.
 */
const rowOf = async (module: string) => {
  await expect
    .poll(() => document.querySelector(`[data-extension="${module}"]`))
    .not.toBeNull();

  const found = document.querySelector(`[data-extension="${module}"]`);
  if (!(found instanceof HTMLElement)) throw new Error(`no row for ${module}`);

  return page.elementLocator(found);
};

/** Opens the marketplace settings from the head of the listing. */
const openSettings = async () => {
  await page.getByRole('button', { name: 'Marketplace settings' }).click();
};

/** Opens a row's menu, whose items are drawn outside the row. */
const openMenu = async (name: string) => {
  await page.getByRole('button', { name: `Actions for ${name}` }).click();
};

// The listing is drawn as stacked rows in these cases, so each module is
// drawn once whatever the width of the test's window.
beforeEach(() => {
  localStorage.setItem(
    'listing.settings-extension-modules',
    JSON.stringify({ view: 'rows' }),
  );
});

describe('the Extensions room', () => {
  // A module installed by hand is in no index, and it is still listed, so
  // what runs on this server can always be found and removed here.
  it('lists an installed module no index offers, and opens the page it declares', async () => {
    const go = vi.fn();
    await open(new SlotRegistry(), await listed([instance()]), { go });

    const row = await rowOf(NAMESPACE);
    await expect.element(row).toHaveAttribute('data-state', 'installed');
    await expect.element(row.getByText('spools')).toBeVisible();

    await openMenu('spools');
    await page.getByRole('menuitem', { name: 'Open Spools' }).click();

    expect(go).toHaveBeenCalledWith(`/m/${NAMESPACE}/spools`);
  });

  // ADR-20260911-4p6 rule 26: a refusal is state a person reads, never a console line.
  it('says so when a switch was refused because the module changed meanwhile', async () => {
    const modules = await listed([
      {
        ...instance(),
        version: 3,
        module: { version: '1.0.0', asked: {} },
      },
    ]);
    await open(
      new SlotRegistry(),
      modules,
      {},
      {
        'PUT /api/v1/resources/sink/spools': problem(
          412,
          'core.validation/VERSION_MISMATCH',
        ),
      },
    );

    await (await rowOf(NAMESPACE)).getByRole('switch').click();

    await expect
      .element(page.getByText(/This extension changed meanwhile/u))
      .toBeVisible();
  });

  it('reads a refusal out with its reason and its code', async () => {
    const registry = new SlotRegistry();
    registry.refuse({
      namespace: NAMESPACE,
      reason: 'library-mismatch',
      range: '>=9.0.0',
      version: '0.1.0',
    });

    await open(registry, await listed([instance()]));

    await expect
      .element(page.getByText(/It needs the component library >=9\.0\.0/u))
      .toBeInTheDocument();
    await expect
      .element(page.getByText('library-mismatch'))
      .toBeInTheDocument();
  });
});

const PARTS = 'github.com/prusa3d/parts';
const PARTS_NAMESPACE = 'com.github.aux.prusa3d.parts';

const partsOffer = (over: Record<string, unknown> = {}) => ({
  module: PARTS,
  name: 'parts',
  title: 'Parts',
  description: 'A private catalogue of parts.',
  version: '0.2.0',
  source: `http://localhost:4455/api/connect/v1/extensions/${PARTS_NAMESPACE}/download?version=0.2.0`,
  author: 'Prusa Research',
  trust: 'official',
  origin: 'marketplace',
  namespace: PARTS_NAMESPACE,
  sha256: 'ab'.repeat(32),
  page_url: `http://localhost:4455/connect/extensions/${PARTS_NAMESPACE}`,
  permissions: ['observe'],
  platforms: ['darwin_arm64', 'linux_arm64'],
  installed: false,
  requests_known: true,
  ...over,
});

const marketplace = (over: Record<string, unknown> = {}) => ({
  url: 'http://localhost:4455',
  browse_url: 'http://localhost:4455/connect',
  enabled: true,
  reachable: true,
  read_at: '2026-10-05T12:00:00Z',
  stale: false,
  ...over,
});

/** The room over a server whose index answers from the routes given. */
const openCatalogue = async (routes: Routes) => {
  const ctx = appContext(
    { 'GET /api/v1/resources': json(200, { items: [] }), ...routes },
    {},
  );
  await render(WithSlots, {
    props: { registry: new SlotRegistry(), screen: Extensions, ctx },
  });

  return ctx;
};

describe('the catalogue on the Extensions room', () => {
  it('lists a marketplace module with who publishes it, its release and its state', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer()],
        marketplace: marketplace(),
      }),
    });

    const row = await rowOf(PARTS);
    await expect.element(row).toHaveAttribute('data-state', 'available');
    await expect.element(row.getByText('Parts')).toBeVisible();
    await expect.element(row.getByText('Prusa Research')).toBeVisible();
    await expect.element(row.getByText('0.2.0', { exact: true })).toBeVisible();
    await expect.element(row.getByText('Marketplace')).toBeVisible();
    await expect.element(row.getByText('Not installed')).toBeVisible();
    await expect
      .element(row.getByRole('button', { name: 'Install' }))
      .toBeVisible();

    // The marketplace's page is one click away, in a window of its own.
    const link = row.getByRole('link', {
      name: 'Open Parts in the marketplace, in a new window',
    });
    await expect
      .element(link)
      .toHaveAttribute(
        'href',
        `http://localhost:4455/connect/extensions/${PARTS_NAMESPACE}`,
      );
    await expect.element(link).toHaveAttribute('target', '_blank');

    await openMenu('Parts');
    await expect
      .element(page.getByRole('menuitem', { name: 'Open in the marketplace' }))
      .toBeVisible();
    // Nothing is installed, so there is nothing to remove yet.
    await expect
      .element(page.getByRole('menuitem', { name: 'Remove' }))
      .toHaveAttribute('aria-disabled', 'true');
  });

  it('offers an update when a newer release is on offer than the one installed', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ installed: true, installed_version: '0.1.0' })],
        marketplace: marketplace(),
      }),
    });

    const row = await rowOf(PARTS);
    await expect.element(row).toHaveAttribute('data-state', 'update');
    await expect.element(row.getByText('Update available')).toBeVisible();
    await expect
      .element(row.getByRole('button', { name: 'Update', exact: true }))
      .toBeVisible();

    await openMenu('Parts');
    await expect
      .element(page.getByRole('menuitem', { name: 'Remove' }))
      .not.toHaveAttribute('aria-disabled', 'true');
  });

  it('asks before installing native code, then installs from the marketplace', async () => {
    let installed = false;
    const ctx = await openCatalogue({
      'GET /api/v1/modules/index': () =>
        json(200, {
          items: [
            partsOffer(
              installed ? { installed: true, installed_version: '0.2.0' } : {},
            ),
          ],
          marketplace: marketplace(),
        }),
      'POST /api/v1/modules': () => {
        installed = true;

        return json(201, { instance: 'sink:parts' });
      },
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();

    const dialog = page.getByRole('alertdialog');
    // An official extension is Prusa Research's own, so it is not warned about.
    await expect
      .element(dialog.getByText(/is an official extension of Prusa Research/u))
      .toBeVisible();
    await expect
      .element(dialog.getByText('Trusted extension'))
      .not.toBeInTheDocument();
    await expect.element(dialog.getByText('Permission observe')).toBeVisible();

    // Nothing is sent before the person confirms.
    expect(installed).toBe(false);

    // An install loses nothing, so it confirms with the primary button.
    await expect
      .element(dialog.getByRole('button', { name: 'Install' }))
      .toHaveAttribute('data-variant', 'default');

    await dialog.getByRole('button', { name: 'Install' }).click();

    // The row can still read Working here, so it is read afresh on each poll.
    await expect
      .poll(() =>
        document
          .querySelector(`[data-extension="${PARTS}"]`)
          ?.getAttribute('data-state'),
      )
      .toBe('installed');
    // The reads that follow the install end after the row has redrawn.
    await expect.poll(() => ctx.moduleCatalogue.acting).toBeUndefined();
  });

  it('says a trusted extension was reviewed, and still runs as native code', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ trust: 'verified', author: 'Acme' })],
        marketplace: marketplace({ community: true }),
      }),
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();

    const dialog = page.getByRole('alertdialog');
    await expect.element(dialog.getByText('Trusted extension')).toBeVisible();
    await expect
      .element(dialog.getByText(/A review does not find every problem/u))
      .toBeVisible();
    await expect
      .element(dialog.getByRole('button', { name: 'Install' }))
      .toBeEnabled();
  });

  // A community extension is a trusted or an unverified one. It is not listed
  // until an organization administrator lists community extensions.
  it('leaves out the community extensions until they are listed', async () => {
    let community = false;
    const nozzle = 'github.com/jane/nozzle-watch';
    const bed = 'github.com/acme/bed-watch';
    await openCatalogue({
      'GET /api/v1/modules/index': () =>
        json(200, {
          items: [
            partsOffer(),
            partsOffer({
              module: nozzle,
              name: 'nozzle-watch',
              title: 'Nozzle watch',
              namespace: 'com.github.aux.jane.nozzle-watch',
              author: 'Jane Doe',
              trust: 'community',
            }),
            partsOffer({
              module: bed,
              name: 'bed-watch',
              title: 'Bed watch',
              namespace: 'com.github.aux.acme.bed-watch',
              author: 'Acme',
              trust: 'verified',
            }),
          ],
          marketplace: marketplace({ community }),
        }),
      'PUT /api/v1/modules/marketplace': () => {
        community = true;

        return json(200, marketplace({ community: true }));
      },
    });

    await rowOf(PARTS);
    expect(document.querySelector(`[data-extension="${nozzle}"]`)).toBeNull();
    expect(document.querySelector(`[data-extension="${bed}"]`)).toBeNull();
    await expect
      .element(page.getByText(/2 community extensions are not listed/u))
      .toBeVisible();

    await page
      .getByRole('button', { name: 'List them in the marketplace settings' })
      .click();
    await page
      .getByRole('switch', { name: 'Show community extensions' })
      .click();

    await rowOf(nozzle);
    await rowOf(bed);
    await expect
      .element(page.getByText(/community extensions? (is|are) not listed/u))
      .not.toBeInTheDocument();
  });

  it('installs a community extension only once the person accepts the risk', async () => {
    let sent = 0;
    const nozzle = 'github.com/jane/nozzle-watch';
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [
          partsOffer({
            module: nozzle,
            name: 'nozzle-watch',
            title: 'Nozzle watch',
            namespace: 'com.github.aux.jane.nozzle-watch',
            author: 'Jane Doe',
            trust: 'community',
          }),
        ],
        marketplace: marketplace({ community: true }),
      }),
      'POST /api/v1/modules': () => {
        sent += 1;

        return json(201, { instance: 'sink:nozzle-watch' });
      },
    });

    await (
      await rowOf(nozzle)
    )
      .getByRole('button', { name: 'Install' })
      .click();

    const dialog = page.getByRole('alertdialog');
    await expect
      .element(dialog.getByText('Unverified extension'))
      .toBeVisible();
    const install = dialog.getByRole('button', { name: 'Install' });
    await expect.element(install).toBeDisabled();

    await dialog
      .getByRole('checkbox', {
        name: 'I understand the risk, and I trust Jane Doe with this server',
      })
      .click();
    await expect.element(install).toBeEnabled();
    await install.click();

    await expect.poll(() => sent).toBe(1);
  });

  it('draws a refused install by its code, beside the name of the module it was for', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer()],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': problem(
        400,
        'core.validation/MODULE_DIGEST_MISMATCH',
      ),
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();
    await page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Install' })
      .click();

    await expect
      .element(page.getByText('core.validation/MODULE_DIGEST_MISMATCH'))
      .toBeVisible();
  });

  it('narrows the listing by what a person types', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [
          partsOffer(),
          partsOffer({
            module: 'github.com/acme/spools',
            namespace: 'com.github.aux.acme.spools',
            title: 'Spools',
            author: 'Acme',
            origin: 'local',
          }),
        ],
        marketplace: marketplace(),
      }),
    });

    await rowOf(PARTS);
    await rowOf('github.com/acme/spools');

    await page
      .getByRole('searchbox', { name: 'Search the extensions' })
      .fill('acme');

    await expect
      .poll(() => document.querySelector(`[data-extension="${PARTS}"]`))
      .toBeNull();
    await rowOf('github.com/acme/spools');

    await page
      .getByRole('searchbox', { name: 'Search the extensions' })
      .fill('nothing like this');
    await expect.element(page.getByText('No extension matches')).toBeVisible();
  });

  it('says when the marketplace did not answer, and still lists the local rows', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ origin: 'local', title: 'Local parts' })],
        marketplace: marketplace({
          enabled: true,
          reachable: false,
          problem: 'reach the marketplace: refused',
        }),
      }),
    });

    // The head of the listing says so while the settings are closed.
    await expect
      .element(page.getByText('Marketplace not answering'))
      .toBeVisible();
    await expect
      .element((await rowOf(PARTS)).getByText('This server'))
      .toBeVisible();

    await openSettings();
    await expect
      .element(page.getByText(/The marketplace has not answered yet/u))
      .toBeVisible();
    await expect
      .element(page.getByText('reach the marketplace: refused'))
      .toBeVisible();
  });

  it('switches the marketplace off, and then lists only what this server offers itself', async () => {
    let enabled = true;
    const sent: unknown[] = [];
    await openCatalogue({
      'GET /api/v1/modules/index': () =>
        json(200, {
          items: enabled ? [partsOffer()] : [],
          marketplace: marketplace({ enabled }),
        }),
      'PUT /api/v1/modules/marketplace': (call) => {
        sent.push(JSON.parse(call.body ?? '{}'));
        enabled = false;

        return json(200, marketplace({ enabled: false, reachable: false }));
      },
    });

    await rowOf(PARTS);
    const headerSwitch = page.getByRole('switch', {
      name: 'Marketplace',
      exact: true,
    });
    await expect.element(headerSwitch).toHaveAttribute('aria-checked', 'true');
    await headerSwitch.click();

    await expect.element(headerSwitch).toHaveAttribute('aria-checked', 'false');
    expect(sent).toEqual([{ enabled: false }]);
    await expect
      .poll(() => document.querySelector(`[data-extension="${PARTS}"]`))
      .toBeNull();

    await openSettings();
    await expect
      .element(page.getByRole('switch', { name: 'Read the marketplace' }))
      .toHaveAttribute('aria-checked', 'false');
    await expect
      .element(page.getByText(/Switched off\. Only the extensions/u))
      .toBeVisible();
    expect(
      page.getByRole('link', { name: 'Browse the marketplace' }).elements(),
    ).toHaveLength(0);
  });

  // A person comes here for the modules, so the marketplace settings are
  // closed until they ask for them, and the button says which way it is.
  it('keeps the marketplace settings closed until they are opened', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer()],
        marketplace: marketplace(),
      }),
    });

    await rowOf(PARTS);
    const settings = page.getByRole('button', { name: 'Marketplace settings' });
    await expect.element(settings).toHaveAttribute('aria-expanded', 'false');
    expect(
      page.getByRole('switch', { name: 'Read the marketplace' }).elements(),
    ).toHaveLength(0);

    await settings.click();
    await expect.element(settings).toHaveAttribute('aria-expanded', 'true');
    await expect
      .element(page.getByRole('switch', { name: 'Read the marketplace' }))
      .toHaveAttribute('aria-checked', 'true');
    await expect
      .element(page.getByRole('link', { name: 'Browse the marketplace' }))
      .toHaveAttribute('href', 'http://localhost:4455/connect');

    await settings.click();
    expect(
      page.getByRole('switch', { name: 'Read the marketplace' }).elements(),
    ).toHaveLength(0);
  });

  // The configuration file reads no marketplace, so the server answers none.
  it('says the configuration switches the marketplace off, and how to switch it on', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, { items: [] }),
    });

    const empty = page.getByRole('heading', { name: 'No extension yet' });
    await expect.element(empty).toBeVisible();
    await expect
      .element(
        page
          .getByText(/configuration file switches the marketplace off/u)
          .first(),
      )
      .toBeVisible();
    expect(document.body.textContent).not.toContain('core.marketplace.url');
    await expect
      .element(page.getByText('Marketplace off in the configuration'))
      .toBeVisible();

    await openSettings();
    await expect
      .element(page.getByText(/Remove that line or write true/u).nth(1))
      .toBeVisible();
    expect(
      page.getByRole('switch', { name: 'Read the marketplace' }).elements(),
    ).toHaveLength(0);
  });

  // A module no list offers, installed by hand or from a marketplace now off,
  // is still listed as Unlisted and removed by its namespace.
  it('removes a module no index offers, by its namespace', async () => {
    let removed = '';
    const ctx = appContext(
      {
        'GET /api/v1/modules/index': json(200, { items: [] }),
        'DELETE /api/v1/modules/com.github.aux.acme.spools': (call) => {
          removed = call.path;

          return new Response(null, { status: 204 });
        },
      },
      { modules: await listed([instance()]) },
    );
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    const row = await rowOf(NAMESPACE);
    await expect.element(row.getByText('Unlisted')).toBeVisible();

    await openMenu('spools');
    await page.getByRole('menuitem', { name: 'Remove' }).click();
    const remove = page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Remove' });
    await expect.element(remove).toHaveAttribute('data-variant', 'destructive');
    await remove.click();

    await expect.poll(() => removed).toBe(`/api/v1/modules/${NAMESPACE}`);
  });
});

/** A refusal carrying the params the server names. */
const refusal = (
  status: number,
  code: string,
  params: Record<string, string>,
): Response =>
  new Response(
    JSON.stringify({
      type: 'about:blank',
      status,
      code,
      title: 'refused',
      detail: `the server refused with ${code}`,
      params,
      retryable: false,
    }),
    { status, headers: { 'content-type': 'application/problem+json' } },
  );

const installation = (over: Record<string, unknown> = {}) => ({
  instance: 'sink:parts',
  type: `${PARTS_NAMESPACE}/parts`,
  namespace: PARTS_NAMESPACE,
  kind: 'sink',
  version: '0.2.0',
  runtime: 'service',
  root: '/srv/modules/parts',
  enabled: true,
  verified: 3,
  running: true,
  restart: false,
  ...over,
});

interface Sent {
  approve?: boolean;
  approve_listen?: boolean;
  listen?: Record<string, number>;
  database?: unknown;
  expect?: unknown;
  version?: string;
}

// The SDK writes every body as JSON, and a test reads back what it wrote.
const sentOf = (body: string | undefined): Sent =>
  JSON.parse(body ?? '{}') as Sent;

/** The access check, answering yes for the kinds `allowed` names. */
const accessFor =
  (allowed: readonly string[]): Handler =>
  (call) => {
    const { object_id: kind } = JSON.parse(call.body ?? '{}') as {
      object_id: string;
    };

    return json(200, {
      allowed: allowed.includes(kind),
      actions: [],
      authorization_epoch: 1,
    });
  };

const dialog = () => page.getByRole('alertdialog');

describe('the install confirmation', () => {
  it('lists every request it grants, and sends the same requests as expect', async () => {
    const sent: Sent[] = [];
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [
          partsOffer({
            permissions: ['observe', 'operate'],
            events: { done: 'print.done' },
            check_absent_people: true,
            records_mib: 64,
          }),
        ],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': (call) => {
        sent.push(sentOf(call.body));

        return json(201, installation());
      },
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();

    const list = dialog().getByRole('listitem');
    await expect.element(list.nth(0)).toHaveTextContent('Permission observe');
    await expect.element(list.nth(1)).toHaveTextContent('Permission operate');
    await expect.element(list.nth(2)).toHaveTextContent('Publishes print.done');
    await expect
      .element(list.nth(3))
      .toHaveTextContent('May ask about people it is not serving.');
    await expect
      .element(list.nth(4))
      .toHaveTextContent('64 MiB for its records. The default is 8 MiB.');

    await dialog().getByRole('button', { name: 'Install' }).click();

    await expect.poll(() => sent).toHaveLength(1);
    expect(sent[0]).toMatchObject({
      approve: true,
      expect: {
        permissions: ['observe', 'operate'],
        events: { done: 'print.done' },
        check_absent_people: true,
        records_mib: 64,
      },
    });
    await expect
      .element(page.getByText('Parts 0.2.0 is installed'))
      .toBeVisible();
  });

  // A port takes its own tick, and a person may move it before installing
  // (ADR-20261019-3ns rules 6 and 7).
  it('grants the ports only when ticked, at the number the person typed', async () => {
    const sent: Sent[] = [];
    const syslog = {
      name: 'syslog',
      network: 'udp' as const,
      port: 13514,
      why: 'printer log lines',
    };
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ listen: [syslog], data: true })],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': (call) => {
        sent.push(sentOf(call.body));

        return json(201, installation());
      },
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();
    await expect
      .element(
        dialog().getByText('Listens on UDP port 13514: printer log lines'),
      )
      .toBeVisible();
    await expect
      .element(dialog().getByRole('spinbutton'))
      .not.toBeInTheDocument();

    await dialog()
      .getByRole('checkbox', { name: 'Allow listening on these ports' })
      .click();
    const number = dialog().getByRole('spinbutton', {
      name: 'UDP port for printer log lines',
    });
    await expect.element(number).toHaveValue(13514);

    await number.fill('70000');
    await expect
      .element(
        dialog().getByText('Not a port from 1 to 65535. Port 13514 is used.'),
      )
      .toBeVisible();
    await number.fill('5514');

    await dialog().getByRole('button', { name: 'Install' }).click();

    await expect.poll(() => sent).toHaveLength(1);
    expect(sent[0]).toMatchObject({
      approve: true,
      approve_listen: true,
      listen: { syslog: 5514 },
      expect: { data: true, listen: [syslog] },
    });
  });

  // A database of its own goes where the person picks (ADR-20261021-bqc rule 5).
  it('lists a database of its own and sends the place the person picked', async () => {
    const sent: Sent[] = [];
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ database: true })],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': (call) => {
        sent.push(sentOf(call.body));

        return json(201, installation());
      },
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();
    await expect
      .element(
        dialog().getByText('A database of its own, for the data it keeps.'),
      )
      .toBeVisible();
    await expect
      .element(
        dialog().getByRole('radio', { name: 'A file on this server (SQLite)' }),
      )
      .toBeChecked();

    await dialog()
      .getByRole('radio', { name: 'A Postgres database you made' })
      .click();
    await dialog()
      .getByLabelText('Connection string', { exact: false })
      .fill('postgres://parts:pw@db.lan:5432/parts');

    await dialog().getByRole('button', { name: 'Install' }).click();

    await expect.poll(() => sent).toHaveLength(1);
    expect(sent[0]).toMatchObject({
      approve: true,
      database: {
        mode: 'postgres',
        dsn: 'postgres://parts:pw@db.lan:5432/parts',
      },
      expect: { database: true },
    });
  });

  // A local row whose requests the server has not read: it installs the
  // release switched off, shows what it read, and approves exactly that.
  it('reads the requests first when the row does not know them', async () => {
    const sent: Sent[] = [];
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [
          partsOffer({
            origin: 'local',
            source: '/srv/parts',
            requests_known: false,
          }),
        ],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': (call) => {
        const body = sentOf(call.body);
        sent.push(body);

        return body.approve === true
          ? json(201, installation())
          : json(
              201,
              installation({
                enabled: false,
                running: false,
                requests: ['permission operate', 'records 32 MiB'],
              }),
            );
      },
    });

    await (await rowOf(PARTS)).getByRole('switch').click();
    await expect
      .element(dialog().getByText(/This list does not say what the release/u))
      .toBeVisible();
    // Nothing is sent before the person agrees to the download.
    expect(sent).toEqual([]);

    await dialog()
      .getByRole('button', { name: 'Download and read its requests' })
      .click();

    await expect
      .element(dialog().getByText('Approve what Parts asks for?'))
      .toBeVisible();
    await expect
      .element(dialog().getByText('Permission operate'))
      .toBeVisible();
    await expect
      .element(dialog().getByText(/32 MiB for its records/u))
      .toBeVisible();
    expect(sent[0]?.approve).toBe(false);

    await dialog()
      .getByRole('button', { name: 'Approve and switch on' })
      .click();

    await expect.poll(() => sent).toHaveLength(2);
    expect(sent[1]).toMatchObject({
      approve: true,
      expect: { permissions: ['operate'], records_mib: 32 },
    });
  });

  it('shows what a local release added beyond the list, and asks again', async () => {
    const sent: Sent[] = [];
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ origin: 'local', source: '/srv/parts' })],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': (call) => {
        sent.push(sentOf(call.body));

        return sent.length === 1
          ? refusal(409, 'core.resource/MODULE_REQUESTS_CHANGED', {
              name: PARTS,
              source: 'the request expected',
              found: 'permission operate',
            })
          : json(201, installation());
      },
    });

    await (await rowOf(PARTS)).getByRole('switch').click();
    await dialog().getByRole('button', { name: 'Install' }).click();

    await expect
      .element(dialog().getByText('Parts asks for more than you were shown'))
      .toBeVisible();
    await expect
      .element(dialog().getByText(/Beyond what you were shown/u))
      .toBeVisible();
    // Once beyond what was shown, and once among everything it would get.
    await expect
      .poll(() => dialog().getByText('Permission operate').elements().length)
      .toBe(2);

    await dialog().getByRole('button', { name: 'Approve and install' }).click();

    await expect.poll(() => sent).toHaveLength(2);
    expect(sent[1]).toMatchObject({
      expect: { permissions: ['observe', 'operate'] },
    });
  });

  it('says the update keeps earlier decisions, and lists what the new release added', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ installed: true, installed_version: '0.1.0' })],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': json(
        201,
        installation({ added: ['permission operate'] }),
      ),
    });

    await (
      await rowOf(PARTS)
    )
      .getByRole('button', { name: 'Update', exact: true })
      .click();
    await expect
      .element(dialog().getByText(/Your earlier decisions stay/u))
      .toBeVisible();

    await dialog().getByRole('button', { name: 'Update to 0.2.0' }).click();

    const outcome = page.getByText('Parts is updated to 0.2.0');
    await expect.element(outcome).toBeVisible();
    await expect
      .element(page.getByText(/The new release asked for more/u))
      .toBeVisible();
    await expect.element(page.getByText('Permission operate')).toBeVisible();
  });

  it('says what an install could not grant, and that it is not running', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer()],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': json(
        201,
        installation({
          withheld: ['permission operate'],
          running: false,
        }),
      ),
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();
    await dialog().getByRole('button', { name: 'Install' }).click();

    await expect
      .element(page.getByText(/installed without these/u))
      .toBeVisible();
    await expect.element(page.getByText('Permission operate')).toBeVisible();
    await expect
      .element(page.getByText(/installed and not running yet/u))
      .toBeVisible();

    await page.getByRole('button', { name: 'Close' }).click();
    expect(document.querySelector('[data-part="install-outcome"]')).toBeNull();
  });

  it('marks a beta release on its row and in the question', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ channel: 'beta' })],
        marketplace: marketplace(),
      }),
    });

    const row = await rowOf(PARTS);
    await expect.element(row.getByText('Beta')).toBeVisible();

    await row.getByRole('button', { name: 'Install' }).click();
    await expect
      .element(dialog().getByText(/This is a beta release/u))
      .toBeVisible();
  });

  it('says a removal keeps the records and the files in storages', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer({ installed: true, installed_version: '0.2.0' })],
        marketplace: marketplace(),
      }),
    });

    await rowOf(PARTS);
    await openMenu('Parts');
    await page.getByRole('menuitem', { name: 'Remove' }).click();

    await expect
      .element(
        dialog().getByText(
          /Its records in the record store and its files in storages stay/u,
        ),
      )
      .toBeVisible();
  });

  it('says a name another extension holds in the words of the page', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer()],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules': refusal(409, 'core.resource/MODULE_NAME_TAKEN', {
        resource: 'sink:parts',
        type: 'com.github.aux.acme.parts/parts',
        found: `${PARTS_NAMESPACE}/parts`,
      }),
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();
    await dialog().getByRole('button', { name: 'Install' }).click();

    await expect
      .element(
        page.getByText(
          'Another extension is installed under the same name, as com.github.aux.acme.parts/parts',
        ),
      )
      .toBeVisible();
    await expect
      .element(page.getByText('core.resource/MODULE_NAME_TAKEN'))
      .toBeVisible();
  });
});

describe('the marketplace state on the Extensions room', () => {
  it('says a kept copy is old, with the reason the last read failed', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer()],
        marketplace: marketplace({
          reachable: false,
          stale: true,
          problem: 'reach the marketplace: timeout',
        }),
      }),
    });

    await rowOf(PARTS);
    const badge = page.getByText(/Marketplace not answering, list from/u);
    await expect.element(badge).toBeVisible();

    await openSettings();
    await expect
      .element(page.getByText(/The list shows the catalogue read/u))
      .toBeVisible();
    await expect
      .element(page.getByText('reach the marketplace: timeout'))
      .toBeVisible();
  });

  it('says a marketplace that never answered, with its reason', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [],
        marketplace: marketplace({
          reachable: false,
          problem: 'reach the marketplace: refused',
        }),
      }),
    });

    await expect
      .element(page.getByText(/has not answered yet, so none of its/u))
      .toBeVisible();
    await expect
      .element(page.getByText('reach the marketplace: refused'))
      .toBeVisible();
  });

  it('says a marketplace switched off here, and where to switch it on', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [],
        marketplace: marketplace({ enabled: false, reachable: false }),
      }),
    });

    await expect
      .element(page.getByText(/switched off in the marketplace settings/u))
      .toBeVisible();
  });

  it('says a marketplace that has nothing for this server', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [],
        marketplace: marketplace(),
      }),
    });

    await expect
      .element(page.getByText(/offers no extension for this server's version/u))
      .toBeVisible();
  });
});

describe('what a person may do on the Extensions room', () => {
  // The server never writes config.yaml, so it refuses to remove what the
  // file declares, and the menu does not offer it.
  it('disables removal of a module config.yaml declares', async () => {
    await open(
      new SlotRegistry(),
      await listed([{ ...instance(), source: 'file' }]),
    );

    await rowOf(NAMESPACE);
    await openMenu('spools');
    await expect
      .element(page.getByRole('menuitem', { name: 'Remove' }))
      .toHaveAttribute('data-disabled');
  });

  it('offers no install and no removal to a person the server would refuse', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': problem(403, 'core.authz/ACCESS_DENIED'),
      'POST /api/v1/access/check': accessFor([]),
    });

    await expect
      .element(page.getByText(/Only an administrator can install/u))
      .toBeVisible();
    expect(document.body.textContent).not.toContain('core.authz/ACCESS_DENIED');
  });

  it('disables removal on a row of a kind the person may not configure', async () => {
    const ctx = appContext(
      {
        'GET /api/v1/modules/index': json(200, { items: [] }),
        'POST /api/v1/access/check': accessFor(['driver']),
      },
      { modules: await listed([instance()]) },
    );
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await rowOf(NAMESPACE);
    await expect.poll(() => ctx.moduleCatalogue.mayManage).toBe(true);
    await openMenu('spools');
    await expect
      .element(page.getByRole('menuitem', { name: 'Remove' }))
      .toHaveAttribute('aria-disabled', 'true');
  });

  it('disables the marketplace switch for a person who is no organization administrator', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [partsOffer()],
        marketplace: marketplace(),
      }),
      'POST /api/v1/access/check': accessFor(['sink']),
    });

    await rowOf(PARTS);
    await openSettings();
    await expect
      .element(page.getByRole('switch', { name: 'Read the marketplace' }))
      .toBeDisabled();
    await expect
      .element(page.getByText(/Only an organization administrator/u))
      .toBeVisible();
  });
});

describe('a link from the marketplace', () => {
  const openLink = async (search: string, routes: Routes = {}) => {
    const go = vi.fn();
    const ctx = appContext(
      {
        'GET /api/v1/resources': json(200, { items: [] }),
        'GET /api/v1/modules/index': json(200, {
          items: [
            partsOffer(),
            partsOffer({
              module: 'github.com/acme/spools',
              namespace: 'com.github.aux.acme.spools',
              title: 'Spools',
            }),
          ],
          marketplace: marketplace(),
        }),
        ...routes,
      },
      { search, path: '/settings/extensions', go },
    );
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    return go;
  };

  it('marks the row and asks the install question, once', async () => {
    const go = await openLink(
      `?install=${encodeURIComponent(PARTS)}&version=0.2.0`,
    );

    await expect.element(dialog().getByText('Install Parts?')).toBeVisible();
    const row = await rowOf(PARTS);
    await expect.element(row).toHaveAttribute('data-highlighted', 'true');
    // The listing is narrowed to the one module.
    expect(
      document.querySelector('[data-extension="github.com/acme/spools"]'),
    ).toBeNull();
    // The query leaves the address, so a reload does not ask again.
    expect(go).toHaveBeenCalledWith('/settings/extensions', { replace: true });
  });

  it('says when the link names another release than the one on offer', async () => {
    await openLink(`?install=${encodeURIComponent(PARTS)}&version=0.1.0`);

    await expect
      .element(dialog().getByText(/The link asks for release 0.1.0/u))
      .toBeVisible();
    await expect
      .element(
        dialog().getByRole('button', {
          name: 'Download and read its requests',
        }),
      )
      .toBeVisible();
  });

  it('says when this server offers no such module', async () => {
    const go = await openLink('?install=github.com%2Fnobody%2Fnothing');

    await expect
      .element(
        page.getByText(/offers no extension github.com\/nobody\/nothing/u),
      )
      .toBeVisible();
    expect(go).toHaveBeenCalledWith('/settings/extensions', { replace: true });
  });

  it('asks nothing of a person who may not install', async () => {
    await openLink(`?install=${encodeURIComponent(PARTS)}`, {
      'GET /api/v1/modules/index': problem(403, 'core.authz/ACCESS_DENIED'),
      'POST /api/v1/access/check': accessFor([]),
    });

    await expect
      .element(page.getByText(/The link asks to install/u))
      .toBeVisible();
    expect(page.getByRole('alertdialog').elements()).toHaveLength(0);
  });
});

// ADR-20261019-h76 rule 15: the operator switched installed modules off. The page
// offers nothing to install and draws no marketplace, and what is installed
// stays listed so it can be removed.
describe('the Extensions room while modules are switched off', () => {
  const openOff = async (search = '', routes: Routes = {}) => {
    let indexReads = 0;
    const go = vi.fn();
    const ctx = appContext(
      {
        'GET /api/v1/ui': json(200, { modules: false }),
        ...routes,
        'GET /api/v1/modules/index': () => {
          indexReads += 1;

          return json(200, {
            items: [partsOffer()],
            marketplace: marketplace(),
          });
        },
      },
      {
        modules: await listed([
          instance(),
          {
            ...instance(),
            name: 'stats',
            namespace: 'com.github.aux.acme.stats',
            source: 'file',
            module: { version: '1.0.0', title: 'Stats', asked: {} },
          },
        ]),
        search,
        path: '/settings/extensions',
        go,
      },
    );
    await ctx.settings.read();
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    return { go, indexReads: () => indexReads };
  };

  it('says it once when nothing is installed', async () => {
    const ctx = appContext(
      { 'GET /api/v1/ui': json(200, { modules: false }) },
      { modules: await listed([]) },
    );
    await ctx.settings.read();
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await expect.element(page.getByText('No extensions')).toBeVisible();
    const text = document.body.textContent;
    expect(
      text.split('installs no extension and runs none it installed').length - 1,
    ).toBe(1);
    expect(text).not.toContain('No extension yet');
    expect(text).not.toContain('can still be removed');
    expect(text).not.toContain('Installed and available');
  });

  it('names safe mode, and never config.yaml, while the server is in it', async () => {
    const ctx = appContext(
      {
        'GET /api/v1/ui': json(200, {
          modules: false,
          safe_mode: { reason: 'restart', since: '2026-10-30T09:12:00Z' },
        }),
      },
      { modules: await listed([instance()]) },
    );
    await ctx.settings.read();
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await expect
      .element(page.getByText(/Safe mode is on, so no extension runs/u))
      .toBeVisible();
    expect(document.body.textContent).not.toContain('config.yaml');
  });

  it('tells a person who may not manage modules nothing about removing', async () => {
    await openOff('', { 'POST /api/v1/access/check': accessFor([]) });

    await expect
      .element(
        page.getByText('Only an administrator can remove extensions.', {
          exact: false,
        }),
      )
      .toBeVisible();
    await expect
      .element(
        page.getByText(/installs no extension and runs none it installed/u),
      )
      .toBeVisible();
    const text = document.body.textContent;
    expect(text).not.toContain('can still be removed');
    expect(text).not.toContain('can install');
    expect(text).not.toContain('To install extensions again');
  });

  it('says nothing about removing to a person who may not remove the installed module', async () => {
    await openOff('', { 'POST /api/v1/access/check': accessFor(['driver']) });

    await rowOf(NAMESPACE);
    await expect
      .element(
        page.getByText(/installs no extension and runs none it installed/u),
      )
      .toBeVisible();
    await expect
      .poll(() => document.body.textContent)
      .not.toContain('can still be removed');
    await expect
      .element(page.getByText(/To install extensions again/u))
      .toBeVisible();
  });

  it('gives a viewer one sentence on an empty page', async () => {
    const ctx = appContext(
      {
        'GET /api/v1/ui': json(200, { modules: false }),
        'POST /api/v1/access/check': accessFor([]),
      },
      { modules: await listed([]) },
    );
    await ctx.settings.read();
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await expect.element(page.getByText('No extensions')).toBeVisible();
    await expect.poll(() => ctx.moduleCatalogue.mayManage).toBe(false);
    expect(document.body.textContent).not.toContain('Only an administrator');
  });

  it('says nothing about removing to a viewer of modules config.yaml declares', async () => {
    const ctx = appContext(
      {
        'GET /api/v1/ui': json(200, { modules: false }),
        'POST /api/v1/access/check': accessFor([]),
      },
      {
        modules: await listed([
          {
            ...instance(),
            name: 'stats',
            namespace: 'com.github.aux.acme.stats',
            source: 'file',
            module: { version: '1.0.0', title: 'Stats', asked: {} },
          },
        ]),
      },
    );
    await ctx.settings.read();
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await rowOf('com.github.aux.acme.stats');
    await expect.poll(() => ctx.moduleCatalogue.mayManage).toBe(false);
    expect(document.body.textContent).not.toContain('Only an administrator');
  });

  it('tells an administrator how to switch them on again', async () => {
    await openOff();

    await expect
      .element(
        page.getByText(/can still be removed\. To install extensions again/u),
      )
      .toBeVisible();
  });

  it('says nothing about removing when every listed module comes from config.yaml', async () => {
    const ctx = appContext(
      { 'GET /api/v1/ui': json(200, { modules: false }) },
      {
        modules: await listed([
          {
            ...instance(),
            name: 'stats',
            namespace: 'com.github.aux.acme.stats',
            source: 'file',
            module: { version: '1.0.0', title: 'Stats', asked: {} },
          },
        ]),
      },
    );
    await ctx.settings.read();
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await rowOf('com.github.aux.acme.stats');
    await expect
      .element(
        page.getByText(/installs no extension and runs none it installed/u),
      )
      .toBeVisible();
    expect(document.body.textContent).not.toContain('can still be removed');
  });

  it('does not promise a reinstall before a removal', async () => {
    await openOff();

    await openMenu('spools');
    await page.getByRole('menuitem', { name: 'Remove' }).click();

    const dialog = page.getByRole('alertdialog');
    await expect
      .element(dialog.getByText(/cannot be installed again/u))
      .toBeVisible();
    expect(dialog.element().textContent).not.toContain('installing it again');
  });

  it('takes a refused index as modules off when the settings could not be read', async () => {
    const ctx = appContext(
      {
        'GET /api/v1/ui': problem(500, 'core.internal/INTERNAL'),
        'GET /api/v1/modules/index': problem(
          409,
          'core.resource/MODULES_DISABLED',
        ),
      },
      { modules: await listed([instance()]) },
    );
    await ctx.settings.read();
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    const row = await rowOf(NAMESPACE);
    await expect
      .element(row.getByText('Locked by core.modules_enabled in config.yaml'))
      .toBeVisible();
    await expect.element(row.getByRole('switch')).toBeDisabled();
    expect(document.body.textContent).not.toContain('MODULES_DISABLED');
  });

  it('says why, reads no catalogue and draws no marketplace', async () => {
    const opened = await openOff();

    await expect
      .element(
        page.getByText(/installs no extension and runs none it installed/u),
      )
      .toBeVisible();
    await rowOf(NAMESPACE);
    expect(
      document.querySelector('[data-part="marketplace-settings"]'),
    ).toBeNull();
    expect(document.querySelector(`[data-extension="${PARTS}"]`)).toBeNull();
    expect(opened.indexReads()).toBe(0);
  });

  it('locks an installed module off and still lets it be removed', async () => {
    await openOff();

    const row = await rowOf(NAMESPACE);
    await expect
      .element(row.getByText('Locked by core.modules_enabled in config.yaml'))
      .toBeVisible();
    await expect.element(row.getByRole('switch')).toBeDisabled();
    await expect.element(row.getByRole('switch')).not.toBeChecked();

    await openMenu('spools');
    await expect
      .element(page.getByRole('menuitem', { name: 'Remove' }))
      .not.toHaveAttribute('data-disabled');

    // The module config.yaml declares keeps the switch the file gives it.
    const stats = await rowOf('com.github.aux.acme.stats');
    await expect
      .element(stats.getByText('Configured in config.yaml'))
      .toBeVisible();
  });

  it('asks nothing for a link from the marketplace, says why, and takes it off the address', async () => {
    const opened = await openOff(
      `?install=${encodeURIComponent(PARTS)}&version=0.2.0`,
    );

    await rowOf(NAMESPACE);
    await expect
      .poll(() => opened.go.mock.calls)
      .toContainEqual(['/settings/extensions', { replace: true }]);
    await expect
      .element(
        page.getByText(`The link asks to install ${PARTS},`, { exact: false }),
      )
      .toBeVisible();
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
  });
});

// A module depends on other modules by a range of their releases
// (ADR-20261027-e6k). An install adds what the release requires, a module
// another one requires stays, and a removal names what it leaves unused.
describe('the dependencies between extensions', () => {
  const READER = 'github.com/acme/reader';
  const READER_NAMESPACE = 'com.github.aux.acme.reader';

  const readerPlanned = (over: Record<string, unknown> = {}) => ({
    module: READER,
    name: 'reader',
    title: 'Reader',
    version: '1.4.0',
    source: 'https://example.test/reader-1.4.0.tar.gz',
    origin: 'marketplace',
    namespace: READER_NAMESPACE,
    installed: false,
    requests_known: true,
    permissions: ['read_storage'],
    required_by: [`${PARTS_NAMESPACE} ^1.2`],
    ...over,
  });

  it('lists the extensions an install adds, and sends them as the approved plan', async () => {
    const sent: { dependencies?: unknown }[] = [];
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [
          partsOffer({
            requires: [{ module: READER_NAMESPACE, version: '^1.2' }],
          }),
        ],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules/plan': json(200, {
        dependencies: [readerPlanned()],
      }),
      'POST /api/v1/modules': (call) => {
        sent.push(JSON.parse(call.body ?? '{}') as { dependencies?: unknown });

        return json(201, installation({ dependencies: [`${READER}@1.4.0`] }));
      },
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();

    const plan = page.elementLocator(
      await expect
        .poll(() => document.querySelector('[data-part="plan"]'))
        .not.toBeNull()
        .then(() => document.querySelector('[data-part="plan"]')!),
    );
    await expect
      .element(
        plan.getByText(
          /also needs 1 more extension, which is installed with it/u,
        ),
      )
      .toBeVisible();
    await expect.element(plan.getByText('Reader')).toBeVisible();
    await expect
      .element(plan.getByText('Required by parts ^1.2'))
      .toBeVisible();
    await expect
      .element(plan.getByText('Permission read_storage'))
      .toBeVisible();

    await dialog().getByRole('button', { name: 'Install' }).click();

    await expect.poll(() => sent).toHaveLength(1);
    expect(sent[0]?.dependencies).toEqual([
      {
        module: READER,
        version: '1.4.0',
        expect: expect.objectContaining({
          permissions: ['read_storage'],
        }) as unknown,
      },
    ]);
  });

  it('holds the Install back while the plan is read', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [
          partsOffer({
            requires: [{ module: READER_NAMESPACE, version: '^1.2' }],
          }),
        ],
        marketplace: marketplace(),
      }),
      // The plan never answers, so the question stays on the read.
      'POST /api/v1/modules/plan': () => new Promise<Response>(() => undefined),
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();

    await expect
      .element(dialog().getByRole('button', { name: 'Install' }))
      .toBeDisabled();
  });

  it('lists the ports of the extensions the plan adds beside the approval', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [
          partsOffer({
            requires: [{ module: READER_NAMESPACE, version: '^1.2' }],
          }),
        ],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules/plan': json(200, {
        dependencies: [
          readerPlanned({
            listen: [
              { name: 'feed', port: 9100, network: 'tcp', why: 'the feed' },
            ],
          }),
        ],
      }),
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();

    await expect
      .element(dialog().getByText('Reader: TCP port 9100 for the feed'))
      .toBeVisible();
  });

  it('says who needs what when no release fits every extension', async () => {
    await openCatalogue({
      'GET /api/v1/modules/index': json(200, {
        items: [
          partsOffer({
            requires: [{ module: READER_NAMESPACE, version: '^2' }],
          }),
        ],
        marketplace: marketplace(),
      }),
      'POST /api/v1/modules/plan': refusal(
        409,
        'core.resource/MODULE_DEPENDENCY_CONFLICT',
        {
          resource: PARTS_NAMESPACE,
          requires: `${PARTS_NAMESPACE} needs ${READER_NAMESPACE} ^2, planner needs ${READER_NAMESPACE} ^1`,
        },
      ),
    });

    await (await rowOf(PARTS)).getByRole('button', { name: 'Install' }).click();

    await expect
      .element(
        dialog().getByText(
          `No release fits every extension that needs it: ${PARTS_NAMESPACE} needs ${READER_NAMESPACE} ^2, planner needs ${READER_NAMESPACE} ^1`,
        ),
      )
      .toBeVisible();
  });

  it('disables removal and switching off of an extension another one requires', async () => {
    const required = {
      ...instance(),
      module: {
        ...instance().module,
        required_by: [
          {
            instance: 'sink:planner',
            module: 'com.github.aux.acme.planner',
            version: '^1.0',
            enabled: true,
          },
        ],
      },
    };
    await open(new SlotRegistry(), await listed([required]));

    const row = await rowOf(NAMESPACE);
    await expect
      .element(row.getByText('Required by planner. Remove it first.'))
      .toBeVisible();
    await expect.element(row.getByRole('switch')).toBeDisabled();

    await openMenu('spools');
    await expect
      .element(page.getByRole('menuitem', { name: 'Remove' }))
      .toHaveAttribute('aria-disabled', 'true');
  });

  it('holds the Remove back until what the removal leaves is read', async () => {
    const ctx = appContext(
      {
        'GET /api/v1/modules/index': json(200, { items: [] }),
        [`GET /api/v1/modules/removals/${NAMESPACE}`]: () =>
          new Promise<Response>(() => undefined),
      },
      { modules: await listed([instance()]) },
    );
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await rowOf(NAMESPACE);
    await openMenu('spools');
    await page.getByRole('menuitem', { name: 'Remove' }).click();

    await expect
      .element(dialog().getByRole('button', { name: 'Remove' }))
      .toBeDisabled();
  });

  it('says Remove keeps the database of an extension that has one', async () => {
    const withDatabase = {
      ...instance(),
      module: { ...instance().module, asked: { database: true } },
    };
    const ctx = appContext(
      {
        'GET /api/v1/modules/index': json(200, { items: [] }),
        [`GET /api/v1/modules/removals/${NAMESPACE}`]: json(200, {
          required_by: [],
          unused: [],
        }),
      },
      { modules: await listed([withDatabase]) },
    );
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await rowOf(NAMESPACE);
    await openMenu('spools');
    await page.getByRole('menuitem', { name: 'Remove' }).click();

    await expect
      .element(dialog().getByText(/Its database stays on this server/u))
      .toBeVisible();
  });

  it('removes the extensions an install added only when the person ticks it', async () => {
    const queries: string[] = [];
    const ctx = appContext(
      {
        'GET /api/v1/modules/index': json(200, { items: [] }),
        [`GET /api/v1/modules/removals/${NAMESPACE}`]: json(200, {
          required_by: [],
          unused: [
            {
              instance: 'sink:reader',
              module: READER_NAMESPACE,
              version: '1.4.0',
              title: 'Reader',
            },
          ],
        }),
        [`DELETE /api/v1/modules/${NAMESPACE}`]: (call) => {
          queries.push(call.query);

          return new Response(null, { status: 204 });
        },
      },
      { modules: await listed([instance()]) },
    );
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: Extensions, ctx },
    });

    await rowOf(NAMESPACE);
    await openMenu('spools');
    await page.getByRole('menuitem', { name: 'Remove' }).click();

    const unused = dialog().getByRole('checkbox');
    await expect.element(unused).not.toBeChecked();
    await expect
      .element(
        dialog().getByText(
          'Also remove 1 extension that the install added for it and nothing else needs',
        ),
      )
      .toBeVisible();
    await unused.click();
    await dialog().getByRole('button', { name: 'Remove' }).click();

    await expect.poll(() => queries).toEqual(['with_unused=true']);
  });
});

describe('the Extensions room re-reading memory', () => {
  const MIB = 1_048_576;

  const withMemory = (used?: number) => ({
    ...instance(),
    runtime: 'service',
    status: {
      state: 'ready',
      memory: {
        budget_bytes: 128 * MIB,
        budget_source: 'default',
        failures_in_window: 0,
        held_until_restart: false,
        near_budget: false,
        on_over_memory: 'stop',
        over_budget: false,
        switch_off_failures: 3,
        switch_off_source: 'default',
        switch_off_window_seconds: 3600,
        ...(used === undefined ? {} : { used_bytes: used }),
      },
    },
  });

  it('turns an unknown memory cell into a figure at the next read', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    try {
      let reads = 0;
      const { client } = stubClient({
        'GET /api/v1/resources': () =>
          json(200, {
            items: [withMemory(reads++ === 0 ? undefined : 40 * MIB)],
          }),
      });
      const modules = new Modules(client);
      await modules.load();
      // Memory is a field a reader has to ask for.
      localStorage.setItem(
        'listing.settings-extension-modules',
        JSON.stringify({
          view: 'rows',
          fields: ['module', 'version', 'state', 'memory', 'actions'],
        }),
      );
      await open(new SlotRegistry(), modules);

      await expect.element(page.getByText('Unknown / 128 MiB')).toBeVisible();
      await vi.advanceTimersByTimeAsync(5000);
      await expect.element(page.getByText('40 MiB / 128 MiB')).toBeVisible();
    } finally {
      vi.useRealTimers();
    }
  });

  it('stops reading when the room goes away', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    try {
      let reads = 0;
      const { client } = stubClient({
        'GET /api/v1/resources': () => {
          reads += 1;

          return json(200, { items: [withMemory()] });
        },
      });
      const modules = new Modules(client);
      await modules.load();
      const screen = await open(new SlotRegistry(), modules);
      await screen.unmount();
      const before = reads;
      await vi.advanceTimersByTimeAsync(20_000);

      expect(reads).toBe(before);
    } finally {
      vi.useRealTimers();
    }
  });
});
