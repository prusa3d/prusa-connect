// The page of one installed module: its state, its settings panes as tabs,
// and what its install granted.
import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { Modules } from '@prusa/connect-ui/data';
import { type ModuleContributes, SlotRegistry } from '@prusa/connect-ui/slots';

import { appContext } from '../../test/app-context';
import PlainPane from '../../test/plain-pane.svelte';
import { type Call, json, problem, stubClient } from '../../test/stub';
import WithSlots from '../../test/with-slots.svelte';
import ExtensionDetail from './extension-detail.svelte';

const NAMESPACE = 'com.github.aux.acme.spools';

const contributes: ModuleContributes = {
  settings: [{ id: 'spools', title: 'Spool shelves' }],
};

const instance = {
  kind: 'sink',
  name: 'spools',
  namespace: NAMESPACE,
  uid: 'uid-spools',
  enabled: true,
  source: 'overlay',
  status: { state: 'ready' },
  module: { version: '1.0.0', asked: { permissions: ['observe'] } },
  ui: {
    entry_url: `/api/v1/modules/${NAMESPACE}/ui/ui/index.js`,
    library: '>=0.1.0 <1.0.0',
    contributes,
  },
};

const open = async (
  registry: SlotRegistry,
  items: readonly unknown[],
  params: Record<string, string>,
  routes: Parameters<typeof appContext>[0] = {},
) => {
  const { client } = stubClient({
    'GET /api/v1/resources': json(200, { items }),
  });
  const modules = new Modules(client);
  await modules.load();
  const ctx = appContext(
    {
      'GET /api/v1/modules/index': json(200, { items: [] }),
      'GET /api/v1/devices': json(200, { items: [] }),
      ...routes,
    },
    { modules, params },
  );

  await render(WithSlots, {
    props: { registry, screen: ExtensionDetail, ctx },
  });

  return ctx;
};

describe('the page of one extension', () => {
  // ADR-20261027-e6k: what it depends on, and that a module another one
  // requires stays until that one is removed.
  it('lists its dependencies, and keeps an extension others require', async () => {
    await open(
      new SlotRegistry(),
      [
        {
          ...instance,
          module: {
            ...instance.module,
            requires: [
              {
                module: 'com.github.aux.acme.reader',
                version: '^1.2',
                met: false,
                installed: '1.1.0',
                instance: 'sink:reader',
                state: 'ready',
              },
            ],
            suggests: [
              {
                module: 'com.github.aux.acme.parts',
                reason: 'Shows the parts of a plan',
              },
            ],
            required_by: [
              {
                instance: 'sink:planner',
                module: 'com.github.aux.acme.planner',
                version: '^1.0',
                enabled: true,
              },
            ],
          },
        },
      ],
      { namespace: NAMESPACE, tab: 'dependencies' },
      {
        [`GET /api/v1/modules/why/${NAMESPACE}`]: json(200, {
          items: [
            {
              by: 'com.github.aux.acme.planner',
              constraint: '^1.0',
              text: `com.github.aux.acme.planner 1.0.0 requires ${NAMESPACE} ^1.0`,
            },
          ],
        }),
        'GET /api/v1/modules/outdated': json(200, {
          items: [
            {
              namespace: NAMESPACE,
              installed: '1.0.0',
              allowed: '1.0.0',
              latest: '2.0.0',
              status: 'blocked',
            },
          ],
        }),
      },
    );

    await expect
      .element(page.getByText('Installed 1.1.0, out of range'))
      .toBeVisible();
    await expect
      .element(page.getByText('Shows the parts of a plan'))
      .toBeVisible();
    await expect
      .element(page.getByText('planner 1.0.0 needs it (^1.0).'))
      .toBeVisible();
    await expect
      .element(page.getByText('Blocked by a constraint'))
      .toBeVisible();
    await expect
      .element(page.getByText('Required by these installed extensions'))
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Remove' }))
      .toBeDisabled();
    await expect
      .element(page.getByText('Required by planner. Remove it first.'))
      .toBeVisible();
  });

  it('disables removal of a module config.yaml declares', async () => {
    await open(new SlotRegistry(), [{ ...instance, source: 'file' }], {
      namespace: NAMESPACE,
      tab: 'permissions',
    });

    await expect
      .element(page.getByRole('button', { name: 'Remove' }))
      .toBeDisabled();
  });

  // ADR-20261019-h76: the operator switched installed modules off, so the switch is
  // locked off and says where, and Remove is still offered. The settings stay
  // writable, because the server accepts a save that leaves the module off.
  it('locks the switch of an installed module while modules are off', async () => {
    const ctx = await open(
      new SlotRegistry(),
      [{ ...instance, ui: undefined }],
      { namespace: NAMESPACE, tab: 'permissions' },
      { 'GET /api/v1/ui': json(200, { modules: false }) },
    );
    await ctx.settings.read();

    await expect
      .element(page.getByText('Locked by core.modules_enabled in config.yaml'))
      .toBeVisible();
    await expect.element(page.getByRole('switch')).toBeDisabled();
    await expect.element(page.getByRole('switch')).not.toBeChecked();
    await expect
      .element(page.getByRole('button', { name: 'Remove' }))
      .toBeEnabled();
  });

  it('locks the switch and the header Restart while the server is in safe mode', async () => {
    const ctx = await open(
      new SlotRegistry(),
      [{ ...instance, ui: undefined }],
      { namespace: NAMESPACE, tab: 'permissions' },
      {
        'GET /api/v1/ui': json(200, {
          modules: false,
          safe_mode: { reason: 'restart', since: '2026-10-30T09:12:00Z' },
        }),
      },
    );
    await ctx.settings.read();

    await expect
      .element(
        page.getByText(
          'Locked in safe mode. Restart normally to switch it on.',
        ),
      )
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Restart', exact: true }))
      .toBeDisabled();
    expect(document.body.textContent).not.toContain('config.yaml');
  });

  // Saving a port installs the release again, which the server refuses while
  // modules are off, so the tab says why instead of pointing at module add.
  it('locks the ports of an installed module while modules are off', async () => {
    const ctx = await open(
      new SlotRegistry(),
      [
        {
          ...instance,
          ui: undefined,
          module: {
            ...instance.module,
            asked: {
              permissions: ['observe'],
              listen: [
                {
                  name: 'syslog',
                  network: 'udp',
                  port: 5514,
                  why: 'log lines',
                },
              ],
            },
          },
        },
      ],
      { namespace: NAMESPACE, tab: 'ports' },
      { 'GET /api/v1/ui': json(200, { modules: false }) },
    );
    await ctx.settings.read();

    await expect
      .element(
        page
          .getByRole('tabpanel')
          .getByText('Locked by core.modules_enabled in config.yaml'),
      )
      .toBeVisible();
    expect(
      document
        .querySelector('[role="tabpanel"]')
        ?.textContent.includes('module add'),
    ).toBe(false);
  });

  it('keeps the settings of an installed module writable while modules are off', async () => {
    const ctx = await open(
      new SlotRegistry(),
      [
        {
          ...instance,
          ui: undefined,
          config: { shelf: 5 },
          module: {
            ...instance.module,
            options: [{ key: 'shelf', type: 'int', default: 1, doc: 'Shelf' }],
          },
        },
      ],
      { namespace: NAMESPACE, tab: 'settings' },
      { 'GET /api/v1/ui': json(200, { modules: false }) },
    );
    await ctx.settings.read();

    await expect
      .element(page.getByRole('tabpanel').getByText('Shelf'))
      .toBeVisible();
    expect(
      document
        .querySelector('[role="tabpanel"]')
        ?.textContent.includes('Locked by core.modules_enabled'),
    ).toBe(false);
  });

  it('draws a pane a module registered under its own tab', async () => {
    const registry = new SlotRegistry();
    registry.declare(NAMESPACE, contributes);
    registry.registerSection(NAMESPACE, 'settings', 'spools', PlainPane);

    await open(registry, [instance], {
      namespace: NAMESPACE,
      tab: `${NAMESPACE}.spools`,
    });

    await expect
      .element(page.getByRole('tab', { name: 'Spool shelves' }))
      .toBeVisible();
    await expect.element(page.getByTestId('plain-pane')).toBeInTheDocument();
  });

  it('has a Dependencies tab for an installed module that depends on nothing', async () => {
    await open(new SlotRegistry(), [instance], { namespace: NAMESPACE });

    await expect
      .element(page.getByRole('tab', { name: 'Dependencies' }))
      .toBeVisible();
  });

  it('lists what the install granted, though no index offers the module', async () => {
    await open(new SlotRegistry(), [{ ...instance, ui: undefined }], {
      namespace: NAMESPACE,
      tab: 'permissions',
    });

    await expect
      .element(page.getByText('What the install granted.'))
      .toBeVisible();
    await expect.element(page.getByText(/observe/iu)).toBeVisible();
  });

  it('says so when no such extension is installed', async () => {
    await open(new SlotRegistry(), [], { namespace: NAMESPACE });

    await expect
      .element(page.getByText('This extension is not here.'))
      .toBeVisible();
    await expect
      .element(page.getByRole('link', { name: 'All extensions' }))
      .toHaveAttribute('href', '/settings/extensions');
  });

  it('keeps the page when a later read of the list fails', async () => {
    let failing = false;
    const { client } = stubClient({
      'GET /api/v1/resources': () =>
        failing
          ? problem(500, 'core.resource/BROKEN')
          : json(200, { items: [instance] }),
    });
    const modules = new Modules(client);
    await modules.load();
    const ctx = appContext(
      {
        'GET /api/v1/modules/index': json(200, { items: [] }),
        'GET /api/v1/devices': json(200, { items: [] }),
      },
      { modules, params: { namespace: NAMESPACE } },
    );
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: ExtensionDetail, ctx },
    });

    failing = true;
    await modules.load({ fresh: true });

    expect(modules.error).toBeDefined();
    expect(document.body.textContent).not.toContain(
      'This extension is not here',
    );
  });

  const configurable = {
    ...instance,
    ui: undefined,
    version: 1,
    config: { shelf_size: 4 },
    module: {
      version: '1.0.0',
      asked: {},
      options: [{ key: 'shelf_size', type: 'int', summary: '' }],
    },
  };

  it('says a refused switch in the header, not in the settings form', async () => {
    await open(
      new SlotRegistry(),
      [configurable],
      { namespace: NAMESPACE },
      {
        'PUT /api/v1/resources/sink/spools': problem(
          412,
          'core.resource/VERSION_CONFLICT',
        ),
      },
    );

    await page.getByRole('switch').click();

    await expect
      .element(
        page.getByText(
          'This extension changed meanwhile. What you see now is current.',
        ),
      )
      .toBeVisible();
    await expect
      .element(page.getByText(/your edits are kept/u))
      .not.toBeInTheDocument();
  });

  it('shows no refusal of a write to another module', async () => {
    const ctx = await open(
      new SlotRegistry(),
      [configurable],
      { namespace: NAMESPACE },
      {
        'PUT /api/v1/resources/sink/other': problem(
          412,
          'core.resource/VERSION_CONFLICT',
        ),
      },
    );

    const other = { ...configurable, name: 'other', namespace: 'other.ns' };
    expect(
      await ctx.moduleCatalogue.writeInstance(other as never, { config: {} }),
    ).toBe(false);
    expect(ctx.moduleCatalogue.writeConflict).toBe(true);

    await expect.element(page.getByLabelText('Shelf size')).toBeVisible();
    expect(
      page.getByText(/This extension changed meanwhile/u).elements(),
    ).toHaveLength(0);
  });

  it('saves against the version the edits started from, and keeps them on a conflict', async () => {
    const withOptions = (version: number) => ({
      ...instance,
      ui: undefined,
      version,
      config: { shelf_size: 4 },
      module: {
        version: '1.0.0',
        asked: {},
        options: [{ key: 'shelf_size', type: 'int', summary: '' }],
      },
    });
    let items: unknown[] = [withOptions(1)];
    const { client } = stubClient({
      'GET /api/v1/resources': () => json(200, { items }),
    });
    const modules = new Modules(client);
    await modules.load();
    const puts: Call[] = [];
    const ctx = appContext(
      {
        'GET /api/v1/modules/index': json(200, { items: [] }),
        'GET /api/v1/devices': json(200, { items: [] }),
        'PUT /api/v1/resources/sink/spools': (call) => {
          puts.push(call);
          return problem(412, 'core.resource/VERSION_CONFLICT');
        },
      },
      { modules, params: { namespace: NAMESPACE } },
    );
    await render(WithSlots, {
      props: { registry: new SlotRegistry(), screen: ExtensionDetail, ctx },
    });

    await page.getByLabelText('Shelf size').fill('9');
    items = [{ ...withOptions(2), config: { shelf_size: 10 } }];
    await modules.load();
    await page.getByRole('button', { name: 'Save' }).click();

    await expect.poll(() => puts.length).toBe(1);
    expect(puts[0]?.ifMatch).toBe('"1"');
    await expect
      .element(page.getByText(/This extension changed meanwhile/u))
      .toBeVisible();
    await expect.element(page.getByLabelText('Shelf size')).toHaveValue('9');
  });

  describe('where it shows', () => {
    interface Sent {
      hidden_contributions: string[];
    }
    const withCard = {
      ...instance,
      version: 1,
      config: {},
      hidden_contributions: ['pages/old'],
      ui: {
        ...instance.ui,
        contributes: {
          device_card: [{ id: 'shelf', title: 'Shelf line' }],
          pages: [{ path: 'spools', title: 'Spools', hidden: false }],
        },
      },
    };
    const open1 = async (item: unknown, puts: Call[]) =>
      open(
        new SlotRegistry(),
        [item],
        { namespace: NAMESPACE, tab: 'visibility' },
        {
          'PUT /api/v1/resources/sink/spools': (call) => {
            puts.push(call);
            return problem(412, 'core.resource/VERSION_CONFLICT');
          },
        },
      );

    it('puts the list with the row switched off, keeping the entries already there', async () => {
      const puts: Call[] = [];
      await open1(withCard, puts);

      await expect.element(page.getByText('Printer card')).toBeVisible();
      await page.getByRole('switch', { name: 'Show Shelf line' }).click();

      await expect.poll(() => puts.length).toBe(1);
      expect(
        (JSON.parse(puts[0]?.body ?? '{}') as Sent).hidden_contributions,
      ).toEqual(['pages/old', 'device_card/shelf']);
    });

    it('shows a hidden row as off and puts it back on without it', async () => {
      const puts: Call[] = [];
      await open1(
        { ...withCard, hidden_contributions: ['device_card/shelf'] },
        puts,
      );

      const row = page.getByRole('switch', { name: 'Show Shelf line' });
      await expect.element(row).not.toBeChecked();
      await row.click();

      await expect.poll(() => puts.length).toBe(1);
      expect(
        (JSON.parse(puts[0]?.body ?? '{}') as Sent).hidden_contributions,
      ).toEqual([]);
    });

    it('says which row the module hides, and never writes it to the list', async () => {
      const puts: Call[] = [];
      await open1(
        {
          ...withCard,
          hidden_contributions: [],
          ui: { ...withCard.ui, hidden: ['pages/spools'] },
        },
        puts,
      );

      await expect
        .element(page.getByText('The extension itself keeps this hidden now.'))
        .toBeVisible();
      const row = page.getByRole('switch', { name: 'Show Shelf line' });
      await expect.element(row).toBeChecked();
      await row.click();

      await expect.poll(() => puts.length).toBe(1);
      expect(
        (JSON.parse(puts[0]?.body ?? '{}') as Sent).hidden_contributions,
      ).toEqual(['device_card/shelf']);
    });

    it('lists a file action and a print section, and hides each one', async () => {
      const withFiles = {
        ...withCard,
        hidden_contributions: [],
        ui: {
          ...withCard.ui,
          contributes: {
            file_action: [
              { id: 'plan', title: 'Plan a print', destructive: false },
            ],
            print_section: [{ id: 'planner', title: 'Planner' }],
          },
        },
      };
      const puts: Call[] = [];
      await open1(withFiles, puts);

      await expect.element(page.getByText('File action')).toBeVisible();
      await expect
        .element(page.getByText('Print dialog section'))
        .toBeVisible();
      await page.getByRole('switch', { name: 'Show Plan a print' }).click();
      await expect.poll(() => puts.length).toBe(1);
      await expect
        .element(page.getByRole('switch', { name: 'Show Planner' }))
        .toBeEnabled();
      await page.getByRole('switch', { name: 'Show Planner' }).click();

      await expect.poll(() => puts.length).toBe(2);
      expect(
        (JSON.parse(puts[0]?.body ?? '{}') as Sent).hidden_contributions,
      ).toEqual(['file_action/plan']);
      expect(
        (JSON.parse(puts[1]?.body ?? '{}') as Sent).hidden_contributions,
      ).toEqual(['print_section/planner']);
    });

    it('is locked for an instance configured in a file', async () => {
      await open1({ ...withCard, source: 'file' }, []);

      await expect
        .element(page.getByRole('switch', { name: 'Show Spools' }))
        .toBeDisabled();
    });
  });
});

describe("a service module's history and restart", () => {
  const service = {
    ...instance,
    runtime: 'service',
    status: {
      state: 'ready',
      memory: {
        budget_bytes: 128 * 1_048_576,
        budget_source: 'default',
        held_until_restart: false,
        near_budget: false,
        on_over_memory: 'stop',
        over_budget: false,
        failures_in_window: 0,
        switch_off_failures: 3,
        switch_off_window_seconds: 3600,
        switch_off_source: 'default',
      },
    },
  };
  const history = 'GET /api/v1/resources/sink/spools/memory';
  const log = 'GET /api/v1/resources/sink/spools/log';

  it('says the log needs permission to configure it when the server refuses', async () => {
    await open(
      new SlotRegistry(),
      [service],
      { namespace: NAMESPACE, tab: 'log' },
      {
        [log]: problem(403, 'core.authz/ACCESS_DENIED'),
        [history]: json(200, { items: [], stops: [] }),
      },
    );

    await expect
      .element(
        page.getByText(
          "Reading this module's log takes permission to configure it.",
        ),
      )
      .toBeVisible();
  });

  it('shows a refused restart beside the header', async () => {
    await open(
      new SlotRegistry(),
      [service],
      { namespace: NAMESPACE, tab: 'memory' },
      {
        'POST /api/v1/resources/sink/spools/restart': problem(
          403,
          'core.authz/ACCESS_DENIED',
        ),
        [history]: json(200, { items: [], stops: [] }),
        [log]: json(200, { lines: [] }),
      },
    );

    await page.getByRole('button', { name: 'Restart' }).click();

    await expect
      .poll(() => document.querySelector('[data-part="restart-fault"]'))
      .not.toBeNull();
  });

  it('stops reading the history when the page goes away', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    try {
      const { client } = stubClient({
        'GET /api/v1/resources': json(200, { items: [service] }),
        [history]: json(200, { items: [], stops: [] }),
        [log]: json(200, { lines: [] }),
      });
      const modules = new Modules(client);
      await modules.load();
      const ctx = appContext(
        {
          'GET /api/v1/modules/index': json(200, { items: [] }),
          'GET /api/v1/devices': json(200, { items: [] }),
          [history]: json(200, { items: [], stops: [] }),
          [log]: json(200, { lines: [] }),
        },
        { modules, params: { namespace: NAMESPACE, tab: 'memory' } },
      );
      const screen = await render(WithSlots, {
        props: { registry: new SlotRegistry(), screen: ExtensionDetail, ctx },
      });
      const reads = () => ctx.moduleOperations.history(service as never);
      await expect.poll(reads).toBeDefined();

      await screen.unmount();
      const before = ctx.moduleOperations.history(service as never);
      await vi.advanceTimersByTimeAsync(20_000);

      expect(ctx.moduleOperations.history(service as never)).toBe(before);
    } finally {
      vi.useRealTimers();
    }
  });
});
