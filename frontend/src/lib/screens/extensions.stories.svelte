<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { userEvent, waitFor, within } from 'storybook/test';

  import { failed, pending, refused } from '../../../test/story/api';
  import AppAt from '../../../test/story/app-at.svelte';
  import {
    instanceOf,
    MARKETPLACE,
    offerOf,
  } from '../../../test/extension-fixtures';
  import fixtures from '../../../test/ux/fixtures/api.json';
  import { json } from '../../test/stub';

  const { Story } = defineMeta({ title: 'Screens/Settings/Extensions' });

  const PATH = '/settings/extensions';
  const INDEX = 'GET /api/v1/modules/index';
  const RESOURCES = 'GET /api/v1/resources';

  const ui = JSON.parse(fixtures['GET /api/v1/ui'].body) as Record<
    string,
    unknown
  >;
  const resources = JSON.parse(fixtures['GET /api/v1/resources'].body) as {
    items: unknown[];
  };

  const NOZZLE_MODULE = 'github.com/prusa3d/nozzle-watch';
  const SPOOLS = 'com.github.prusa3d.spools';
  const NOZZLE = 'com.github.prusa3d.nozzle-watch';

  const spools = offerOf({
    module: 'github.com/prusa3d/spools',
    name: 'spools',
    title: 'Spools',
    namespace: SPOOLS,
    version: '1.1.0',
    installed: true,
    installed_version: '1.0.0',
    instance: 'driver:spools',
  });
  const nozzle = offerOf({
    module: NOZZLE_MODULE,
    name: 'nozzle-watch',
    title: 'Nozzle watch',
    namespace: NOZZLE,
    version: '1.0.0',
    description: 'Adds a nozzle temperature section to each printer.',
  });
  const installed = [
    instanceOf({
      name: 'spools',
      namespace: SPOOLS,
      module: { asked: {}, title: 'Spools', version: '1.0.0' },
    }),
    instanceOf({
      name: 'buddy-logs',
      namespace: 'com.github.prusa3d.buddy-logs',
      enabled: false,
      status: {
        attempt: 0,
        conditions: [],
        dropped: 0,
        observed_generation: 1,
        serving: false,
        state: 'disabled',
      },
      module: {
        asked: {},
        title: 'Buddy logs',
        summary: 'Collects printer logs to help diagnose problems.',
        version: '0.3.0',
      },
    }),
  ];

  // Service modules with a memory budget, for the pool bar and the cells.
  const MIB = 1_048_576;
  const MEMORY = 'GET /api/v1/modules/memory';
  const statusWith = (memory: Record<string, unknown>) => ({
    attempt: 0,
    conditions: [],
    dropped: 0,
    observed_generation: 1,
    serving: true,
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
      ...memory,
    },
  });
  const budgeted = (memory: Record<string, unknown>) =>
    instanceOf({
      name: 'spools',
      namespace: SPOOLS,
      module: { asked: {}, title: 'Spools', version: '1.0.0' },
      status: statusWith(memory),
    } as never);
  const withMemory = (memory: Record<string, unknown>) => () =>
    json(200, { items: [...resources.items, budgeted(memory)] });
  const pool = (body: Record<string, unknown>) => () => json(200, body);
  const AT = '2026-10-28T09:30:00Z';

  const DATABASES = 'GET /api/v1/modules/databases';
  const noLeft = { [DATABASES]: () => json(200, { items: [] }) };

  const withInstalled = () =>
    json(200, { items: [...resources.items, ...installed] });
  const index =
    (items: unknown[], marketplace: Record<string, unknown> = MARKETPLACE) =>
    () =>
      json(200, { items, marketplace });

  const printHistory = offerOf({
    module: 'example.com/extensions/print-history',
    name: 'print-history',
    title: 'Print history',
    description: 'Keeps a history of completed print jobs.',
    author: 'Demo',
    namespace: 'com.example.extensions.print-history',
    version: '1.0.0',
    installed: true,
    installed_version: '1.0.0',
    instance: 'driver:print-history',
  });

  const interactiveMarketplace = (initiallyEnabled: boolean) => {
    let enabled = initiallyEnabled;
    const instances = [
      ...installed,
      instanceOf({
        name: 'print-history',
        uid: 'demo-print-history',
        namespace: 'com.example.extensions.print-history',
        version: '1.0.0',
        module: { asked: {}, title: 'Print history', version: '1.0.0' },
      }),
    ].map((instance) => ({ ...instance }));
    const instanceSwitches = Object.fromEntries(
      instances.map((instance) => [
        `PUT /api/v1/resources/${instance.kind}/${instance.name}`,
        () => {
          instance.enabled = !instance.enabled;
          instance.generation += 1;
          instance.status = {
            ...instance.status,
            serving: instance.enabled,
            state: instance.enabled ? 'ready' : 'disabled',
            observed_generation: instance.generation,
          };
          return json(200, instance);
        },
      ]),
    );
    const marketplace = () => ({ ...MARKETPLACE, enabled, reachable: enabled });
    return {
      ...instanceSwitches,
      [RESOURCES]: () =>
        json(200, { items: [...resources.items, ...instances] }),
      [INDEX]: () =>
        json(200, {
          items: enabled ? [spools, nozzle, printHistory] : [],
          marketplace: marketplace(),
        }),
      'PUT /api/v1/modules/marketplace': () => {
        enabled = !enabled;
        return json(200, marketplace());
      },
    };
  };

  const left = {
    engine: 'sqlite',
    instance: 'driver:old-notes',
    left_at: '2026-10-14T09:00:00Z',
    purge: 'delete',
    type: 'module',
    where: '/var/lib/prusa-connect/old-notes.db',
    why: 'removed',
  };

  const installation = {
    instance: 'driver:nozzle-watch',
    type: `${NOZZLE}/nozzle-watch`,
    namespace: NOZZLE,
    kind: 'driver',
    version: '1.0.0',
    runtime: 'service',
    root: '/srv/modules/nozzle-watch',
    enabled: true,
    verified: 3,
    running: true,
    restart: false,
  };

  // The listing draws stacked rows below a width the table needs, so the
  // story fixes the view it asks about.
  const install = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const found = await waitFor(() => {
      const one = canvasElement.querySelector(
        `[data-extension="${NOZZLE_MODULE}"]`,
      );
      if (!(one instanceof HTMLElement)) throw new Error('no Nozzle watch row');

      return one;
    });
    const button = within(found);
    await userEvent.click(
      await button.findByRole('button', { name: 'Install' }),
    );
    const dialog = within(
      await within(document.body).findByRole('alertdialog'),
    );
    await userEvent.click(
      await dialog.findByRole('button', { name: 'Install' }),
    );
    await waitFor(async () => {
      await canvas.findByText(/is installed/u);
    });
  };

  // An install question the person opens and reads, without confirming it.
  const askInstall = async ({
    canvasElement,
  }: {
    canvasElement: HTMLElement;
  }) => {
    const found = await waitFor(() => {
      const one = canvasElement.querySelector(
        `[data-extension="${NOZZLE_MODULE}"]`,
      );
      if (!(one instanceof HTMLElement)) throw new Error('no Nozzle watch row');

      return one;
    });
    await userEvent.click(
      await within(found).findByRole('button', { name: 'Install' }),
    );
    await within(document.body).findByRole('alertdialog');
  };

  // The same question, confirmed, for a refusal the page then shows.
  const confirmInstall = async (context: { canvasElement: HTMLElement }) => {
    await askInstall(context);
    const dialog = within(
      await within(document.body).findByRole('alertdialog'),
    );
    await userEvent.click(
      await dialog.findByRole('button', { name: 'Install' }),
    );
  };

  // The removal question of the installed Spools.
  const askRemoval = async ({
    canvasElement,
  }: {
    canvasElement: HTMLElement;
  }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole('button', { name: 'Actions for Spools' }),
    );
    await userEvent.click(
      await within(document.body).findByRole('menuitem', { name: 'Remove' }),
    );
    await within(document.body).findByText(/that the install added for it/u);
  };

  // A Nozzle watch that requires a Reader this server does not have
  // (ADR-20261027-e6k).
  const READER = 'com.github.prusa3d.reader';
  const needsReader = offerOf({
    module: NOZZLE_MODULE,
    name: 'nozzle-watch',
    title: 'Nozzle watch',
    namespace: NOZZLE,
    version: '1.0.0',
    requires: [{ module: READER, version: '^1.2' }],
  });
  const readerPlanned = {
    ...offerOf({
      module: 'github.com/prusa3d/reader',
      name: 'reader',
      title: 'Reader',
      namespace: READER,
      version: '1.4.0',
      permissions: ['read_storage'],
    }),
    required_by: [`${NOZZLE} ^1.2`],
  };
  const planned = {
    'POST /api/v1/modules/plan': () =>
      json(200, { dependencies: [readerPlanned] }),
  };
  const refusal = (code: string, params: Record<string, string>) => () =>
    new Response(
      JSON.stringify({
        type: `https://prusa-connect/errors/${code}`,
        status: 409,
        code,
        title: 'refused',
        detail: `the server refused with ${code}`,
        params,
        retryable: false,
      }),
      { status: 409, headers: { 'content-type': 'application/problem+json' } },
    );

  // Spools as Planner requires it, and with a Reader the install added for it.
  const requiredSpools = instanceOf({
    name: 'spools',
    namespace: SPOOLS,
    module: {
      asked: {},
      title: 'Spools',
      version: '1.0.0',
      required_by: [
        {
          instance: 'sink:planner',
          module: 'com.github.prusa3d.planner',
          version: '^1.0',
          enabled: true,
        },
      ],
    },
  });
  const removal = {
    [`GET /api/v1/modules/removals/${SPOOLS}`]: () =>
      json(200, {
        required_by: [],
        unused: [
          {
            instance: 'sink:reader',
            module: READER,
            version: '1.4.0',
            title: 'Reader',
          },
        ],
      }),
  };
</script>

<!--
  States the screen branches on: installed and available rows with the
  marketplace on, the marketplace off, an empty marketplace, modules switched
  off by the operator, loading, failed, refused, the install outcome (played
  through the real Install dialog) and the databases extensions left behind.
  Skipped: the marketplace settings panel, an update, a purge and the
  ?install= link notices, which each need a typed or clicked sequence beyond
  the one played here, and the slot registry's refusals, which need a loaded
  module bundle.
-->

<Story name="Installed And Available">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      ...interactiveMarketplace(true),
    }}
  />
</Story>

<!-- A community extension on offer, which nobody chose to list yet. -->
<Story name="Community Extensions Not Listed">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withInstalled,
      [INDEX]: index([
        spools,
        offerOf({
          ...nozzle,
          author: 'Jane Doe',
          trust: 'community',
        }),
      ]),
    }}
  />
</Story>

<Story name="Marketplace Off">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withInstalled,
      [INDEX]: index([], { ...MARKETPLACE, enabled: false, reachable: false }),
    }}
  />
</Story>

<Story name="Empty">
  <AppAt path={PATH} overrides={{ ...noLeft, [INDEX]: index([]) }} />
</Story>

<Story name="Marketplace Unreachable">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [INDEX]: index([], {
        ...MARKETPLACE,
        reachable: false,
        problem: 'the marketplace did not answer',
      }),
    }}
  />
</Story>

<Story name="Safe Mode">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withInstalled,
      'GET /api/v1/ui': () =>
        json(200, {
          ...ui,
          modules: false,
          safe_mode: { reason: 'restart', since: '2026-10-30T09:12:00Z' },
        }),
    }}
  />
</Story>

<Story name="Modules Switched Off">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withInstalled,
      'GET /api/v1/ui': () => json(200, { ...ui, modules: false }),
    }}
  />
</Story>

<Story name="Install Outcome" play={install}>
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [INDEX]: index([nozzle]),
      'POST /api/v1/modules': () => json(201, installation),
    }}
  />
</Story>

<Story name="Left Databases">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withInstalled,
      [INDEX]: index([spools]),
      'GET /api/v1/modules/databases': () => json(200, { items: [left] }),
    }}
  />
</Story>

<Story name="Loading">
  <AppAt path={PATH} overrides={{ ...noLeft, [INDEX]: pending }} />
</Story>

<Story name="Failed">
  <AppAt path={PATH} overrides={{ ...noLeft, [INDEX]: failed }} />
</Story>

<Story name="Refused">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [INDEX]: refused,
      'POST /api/v1/access/check': () =>
        json(200, { allowed: false, actions: [], authorization_epoch: 1 }),
    }}
  />
</Story>

<!--
  The dependencies between extensions (ADR-20261027-e6k): the extensions an
  install adds, the refusals of a plan, an extension others require, and a
  removal that offers the extensions it leaves unused.
-->

<Story name="Install Plan" play={askInstall}>
  <AppAt
    path={PATH}
    overrides={{ ...noLeft, ...planned, [INDEX]: index([needsReader]) }}
  />
</Story>

<Story name="Dependency Conflict" play={askInstall}>
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [INDEX]: index([needsReader]),
      'POST /api/v1/modules/plan': refusal(
        'core.resource/MODULE_DEPENDENCY_CONFLICT',
        {
          resource: NOZZLE,
          requires: `${NOZZLE} needs ${READER} ^1.2, com.github.prusa3d.planner needs ${READER} ^2.0`,
        },
      ),
    }}
  />
</Story>

<Story name="Dependency Not Offered" play={askInstall}>
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [INDEX]: index([needsReader]),
      'POST /api/v1/modules/plan': refusal(
        'core.resource/MODULE_DEPENDENCY_NOT_OFFERED',
        { resource: NOZZLE, requires: `${NOZZLE} needs ${READER} ^1.2` },
      ),
    }}
  />
</Story>

<Story name="Plan Changed" play={confirmInstall}>
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      ...planned,
      [INDEX]: index([needsReader]),
      'POST /api/v1/modules': refusal('core.resource/MODULE_PLAN_CHANGED', {
        resource: NOZZLE_MODULE,
        found: 'github.com/prusa3d/reader@1.5.0',
      }),
    }}
  />
</Story>

<Story name="Required By">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: () =>
        json(200, { items: [...resources.items, requiredSpools] }),
      [INDEX]: index([]),
    }}
  />
</Story>

<Story name="Removal With Unused" play={askRemoval}>
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      ...removal,
      [RESOURCES]: withInstalled,
      [INDEX]: index([]),
    }}
  />
</Story>

<Story name="Pool Unknown">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withMemory({ used_bytes: 40 * MIB }),
      [MEMORY]: pool({ committed_bytes: 128 * MIB }),
      [INDEX]: index([spools]),
    }}
  />
</Story>

<Story name="Over Committed">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withMemory({ used_bytes: 40 * MIB }),
      [MEMORY]: pool({
        committed_bytes: 300 * MIB,
        pool_bytes: 256 * MIB,
        source: 'configured',
      }),
      [INDEX]: index([spools]),
    }}
  />
</Story>

<Story name="Near Budget">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withMemory({
        used_bytes: 115 * MIB,
        near_budget: true,
        failures_in_window: 1,
      }),
      [MEMORY]: pool({
        committed_bytes: 128 * MIB,
        pool_bytes: 1024 * MIB,
        source: 'detected',
      }),
      [INDEX]: index([spools]),
    }}
  />
</Story>

<Story name="Switched Off">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withMemory({
        held_until_restart: true,
        failures_in_window: 3,
        switched_off: {
          at: AT,
          stops: [
            { at: AT, result: 'over-memory' },
            { at: AT, result: 'over-memory' },
            { at: AT, result: 'over-memory' },
          ],
        },
      }),
      [MEMORY]: pool({
        committed_bytes: 0,
        pool_bytes: 1024 * MIB,
        source: 'detected',
      }),
      [INDEX]: index([spools]),
    }}
  />
</Story>

<Story name="Pool Refused">
  <AppAt
    path={PATH}
    overrides={{
      ...noLeft,
      [RESOURCES]: withMemory({ used_bytes: 40 * MIB }),
      [MEMORY]: refused,
      [INDEX]: index([spools]),
    }}
  />
</Story>
