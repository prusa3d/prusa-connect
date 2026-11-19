import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { SlotRegistry } from '@prusa/connect-ui/slots';

import { appContext } from '../../test/app-context';
import { json, problem, type Routes } from '../../test/stub';
import type { StoreEntry } from '../stores/fractal-stores.svelte';
import WithSlots from '../../test/with-slots.svelte';
import StorageFiles from './storage.svelte';

const storages = [
  {
    allowed_extensions: [],
    browse: true,
    display_name: 'Local',
    download_url: true,
    move: true,
    name: 'local',
    ranges: true,
    write: true,
  },
];

const files = [
  {
    folder: false,
    modified: '2026-09-01T10:00:00Z',
    name: 'bracket.bgcode',
    path: 'bracket.bgcode',
    size: 2048,
  },
];

const SERVED: Routes = {
  'GET /api/v1/storages': json(200, { items: storages }),
  'GET /api/v1/storages/local/files': json(200, { items: files }),
};

/** One printer waiting for work, which is what the print sheet offers. */
const waiting = {
  capabilities: ['core.command.start_print'],
  connected: true,
  id: 'one',
  kind: 'printer',
  labels: {},
  model: 'MK4S',
  name: 'MK4S #1042',
  state: { '/state': 'IDLE' },
};

describe('the storage page', () => {
  // Wide enough for every file row act: a phone-width pane folds them into
  // the row's menu.
  beforeEach(async () => {
    await page.viewport(1400, 900);
  });

  it('lists the files of the first storage', async () => {
    await render(StorageFiles, { props: { ctx: appContext(SERVED) } });

    await expect.element(page.getByText('bracket.bgcode')).toBeInTheDocument();
  });

  it('offers the upload zone on a storage a person may write to', async () => {
    await render(StorageFiles, { props: { ctx: appContext(SERVED) } });

    await expect
      .element(page.getByRole('button', { name: 'Upload files' }))
      .toBeInTheDocument();
  });

  // A server serving no storage is not a server whose listing failed, and the
  // screen says which of the two this is.
  it('says storage is off when the server declares none', async () => {
    const ctx = appContext({
      'GET /api/v1/storages': json(200, { items: [] }),
    });

    await render(StorageFiles, { props: { ctx } });

    await expect
      .element(page.getByText('Storage is not enabled on this server.'))
      .toBeInTheDocument();
  });

  // The print sheet holds a printer back when the kind's own table says the
  // start verb runs from another state. The page reads that table from the
  // server, so this proves it reaches the sheet: IDLE is a free bed by the
  // sheet's own rule and the kind here says the verb needs READY.
  it('offers the print sheet the states this server declared', async () => {
    const ctx = appContext({
      ...SERVED,
      'GET /api/v1/devices': json(200, { items: [waiting] }),
      'GET /api/v1/vocabularies/device-states': json(200, {
        format: 'device-states',
        items: [
          {
            kind: 'printer',
            namespace: 'core',
            states: [{ display_name: 'Ready', state: 'READY' }],
            transitions: [
              {
                from: 'READY',
                to: 'PRINTING',
                verb: 'core.command.start_print',
              },
            ],
          },
        ],
        version: '',
        vocabulary: 'device-states',
      }),
    });

    await render(StorageFiles, { props: { ctx } });

    await page.getByRole('button', { name: 'Print bracket.bgcode' }).click();

    await expect
      .element(page.getByText('This works when the printer is READY.'))
      .toBeInTheDocument();
  });

  // The file's page is an address the app's own router takes, and it names
  // the storage that holds the file.
  it('links a file to its page on the storage that holds it', async () => {
    await render(StorageFiles, { props: { ctx: appContext(SERVED) } });

    await expect
      .element(page.getByRole('link', { name: /acket.bgcode/ }))
      .toHaveAttribute('href', '/storage/local/file/bracket.bgcode');
  });

  it('opens the storage and the folder the address names', async () => {
    const ctx = appContext(
      {
        'GET /api/v1/storages': json(200, {
          items: [
            ...storages,
            { ...storages[0], display_name: 'USB drive', name: 'usb' },
          ],
        }),
        'GET /api/v1/storages/usb/files': json(200, {
          items: [
            { ...files[0], name: 'hinge.bgcode', path: 'models/hinge.bgcode' },
          ],
        }),
      },
      { params: { storage: 'usb', path: 'models' } },
    );

    await render(StorageFiles, { props: { ctx } });

    await expect.element(page.getByText('hinge.bgcode')).toBeInTheDocument();
    expect(ctx.storage.storage).toBe('usb');
    expect(ctx.storage.directory).toBe('models');
  });

  // Walking into a folder moves the address, so Back and a reload return to
  // the folder the reader was in.
  it('moves the address when the reader opens a folder', async () => {
    const went: string[] = [];
    const ctx = appContext(
      {
        ...SERVED,
        'GET /api/v1/storages/local/files': json(200, {
          items: [
            {
              folder: true,
              modified: '2026-09-01T10:00:00Z',
              name: 'models',
              path: 'models',
              size: 0,
            },
          ],
        }),
      },
      {
        go: (path) => {
          went.push(path);
        },
      },
    );

    await render(StorageFiles, { props: { ctx } });
    await page.getByRole('button', { name: 'models' }).click();

    expect(went).toEqual(['/storage/local/folder/models']);
  });

  // A module's file-action is a menu on each file's row, and never on a
  // folder's.
  it('runs a module file action on a file row with the file reference', async () => {
    localStorage.clear();
    const registry = new SlotRegistry();
    registry.declare('test.later', {
      file_action: [{ id: 'later', title: 'Print later', destructive: false }],
    });
    const run = vi.fn(() => Promise.resolve());
    registry.registerAction('test.later', 'file-action', 'later', run);
    const ctx = appContext({
      ...SERVED,
      'GET /api/v1/storages/local/files': json(200, {
        items: [
          ...files,
          {
            folder: true,
            modified: '2026-09-01T10:00:00Z',
            name: 'models',
            path: 'models',
            size: 0,
          },
        ],
      }),
    });

    await render(WithSlots, { props: { registry, screen: StorageFiles, ctx } });

    await expect
      .element(page.getByRole('button', { name: 'Actions for bracket.bgcode' }))
      .toBeInTheDocument();
    expect(
      page.getByRole('button', { name: 'Actions for models' }).query(),
    ).toBeNull();

    await page
      .getByRole('button', { name: 'Actions for bracket.bgcode' })
      .click();
    await page.getByRole('menuitem', { name: 'Print later' }).click();

    expect(run).toHaveBeenCalledWith('local:bracket.bgcode');
  });

  // A wide pane draws the row's acts on the row, so with no module entry
  // there is nothing for a menu to hold.
  it('draws no row menu on a wide pane when no module adds a file action', async () => {
    await render(StorageFiles, { props: { ctx: appContext(SERVED) } });

    await expect
      .element(page.getByRole('button', { name: 'Rename bracket.bgcode' }))
      .toBeVisible();
    expect(
      page.getByRole('button', { name: 'Actions for bracket.bgcode' }).query(),
    ).toBeNull();
  });

  it('folds the row acts into a menu on a narrow pane with no module', async () => {
    await page.viewport(375, 800);
    await render(StorageFiles, { props: { ctx: appContext(SERVED) } });

    const menu = page.getByRole('button', {
      name: 'Actions for bracket.bgcode',
    });
    await expect.element(menu).toBeVisible();
    expect(
      page.getByRole('button', { name: 'Rename bracket.bgcode' }).query(),
    ).toBeNull();
  });

  // A phone-width pane folds the row's acts into a menu, and a module's file
  // action joins that menu rather than drawing a second one beside it.
  it('keeps one menu on a file row of a narrow pane with a module file action', async () => {
    await page.viewport(375, 800);
    localStorage.clear();
    const registry = new SlotRegistry();
    registry.declare('test.later', {
      file_action: [{ id: 'later', title: 'Print later', destructive: false }],
    });
    registry.registerAction('test.later', 'file-action', 'later', vi.fn());
    const ctx = appContext(SERVED);

    await render(WithSlots, { props: { registry, screen: StorageFiles, ctx } });

    const menu = page.getByRole('button', {
      name: 'Actions for bracket.bgcode',
    });
    await expect.element(menu).toBeInTheDocument();
    expect(menu.elements()).toHaveLength(1);
    const scroller = page.getByRole('table').element().parentElement!;
    expect(scroller.scrollWidth).toBeLessThanOrEqual(scroller.clientWidth);

    await menu.click();
    await expect
      .element(page.getByRole('menuitem', { name: 'Rename' }))
      .toBeInTheDocument();
    await expect
      .element(page.getByRole('menuitem', { name: 'Print later' }))
      .toBeInTheDocument();
  });

  // An operator hides one file action of a module (ADR-20261021-3dp); the module's
  // other action stays in the row's menu.
  it('leaves a hidden module file action out of the row menu', async () => {
    localStorage.clear();
    const registry = new SlotRegistry();
    registry.declare(
      'test.later',
      {
        file_action: [
          { id: 'later', title: 'Print later', destructive: false },
          { id: 'plan', title: 'Plan a print', destructive: false },
        ],
      },
      ['file_action/plan'],
    );
    registry.registerAction('test.later', 'file-action', 'later', vi.fn());
    registry.registerAction('test.later', 'file-action', 'plan', vi.fn());
    const ctx = appContext(SERVED);

    await render(WithSlots, { props: { registry, screen: StorageFiles, ctx } });
    await page
      .getByRole('button', { name: 'Actions for bracket.bgcode' })
      .click();

    await expect
      .element(page.getByRole('menuitem', { name: 'Print later' }))
      .toBeInTheDocument();
    expect(
      page.getByRole('menuitem', { name: 'Plan a print' }).query(),
    ).toBeNull();
  });
});

describe('the storage page on a Fractal server', () => {
  // Wide enough for every file row act: a phone-width pane folds them into
  // the row's menu.
  beforeEach(async () => {
    await page.viewport(1400, 900);
  });

  const MOUNT = '/api/v1/modules/com.github.aux.prusa3d.fractal-link';

  const fractal = {
    config: {},
    enabled: true,
    generation: 1,
    kind: 'link',
    name: 'fractal',
    namespace: 'com.github.aux.prusa3d.fractal-link',
    required: false,
    runtime: 'process',
    source: 'file',
    status: {
      attempt: 0,
      conditions: [],
      dropped: 0,
      observed_generation: 1,
      serving: true,
      state: 'ready',
    },
    type: 'fractal',
    uid: 'uid-fractal',
    version: '1.0.0',
  };

  const folders = {
    'GET /api/v1/storages/local/files': json(200, {
      items: [
        ...files,
        {
          folder: true,
          modified: '2026-09-01T10:00:00Z',
          name: 'projects',
          path: 'projects',
          size: 0,
        },
      ],
    }),
  };

  const sharing: Routes = {
    ...SERVED,
    ...folders,
    'GET /api/v1/resources': json(200, { items: [fractal] }),
    [`GET ${MOUNT}/files/shares`]: json(200, {
      items: [{ storage: 'local', path: 'projects' }],
    }),
    [`GET ${MOUNT}/files/stores`]: json(200, {
      node: 'hq/brno',
      items: [],
      total: 0,
      truncated: false,
    }),
  };

  it('switches the share of a folder and lists what other servers share', async () => {
    const ctx = appContext(sharing);
    await ctx.modules.load();

    await render(StorageFiles, { props: { ctx } });

    await expect
      .element(
        page.getByRole('switch', {
          name: 'Share projects with the servers below',
        }),
      )
      .toBeChecked();
    await expect
      .element(page.getByRole('heading', { name: 'Shared by other servers' }))
      .toBeVisible();
  });

  it('is the page of any server where the link shares no files', async () => {
    const ctx = appContext({
      ...sharing,
      [`GET ${MOUNT}/files/shares`]: problem(
        404,
        'com.prusa.fractal/FILES_OFF',
      ),
    });
    await ctx.modules.load();

    await render(StorageFiles, { props: { ctx } });

    await expect.element(page.getByText('projects')).toBeVisible();
    expect(page.getByRole('switch').query()).toBeNull();
    expect(
      page.getByRole('heading', { name: 'Shared by other servers' }).query(),
    ).toBeNull();
  });

  it('says the shares could not be read, which is not a server without them', async () => {
    const ctx = appContext({
      ...sharing,
      [`GET ${MOUNT}/files/shares`]: problem(
        503,
        'core.internal/RESOURCE_NOT_READY',
      ),
    });
    await ctx.modules.load();

    await render(StorageFiles, { props: { ctx } });

    await expect
      .element(
        page.getByText('The shared folders of this server could not be read.'),
      )
      .toBeVisible();
  });

  it('asks the module nothing on a server without it', async () => {
    const ctx = appContext({
      ...SERVED,
      ...folders,
      'GET /api/v1/resources': json(200, { items: [] }),
    });
    await ctx.modules.load();

    await render(StorageFiles, { props: { ctx } });

    await expect.element(page.getByText('projects')).toBeVisible();
    expect(
      page.getByRole('columnheader', { name: 'Shared' }).query(),
    ).toBeNull();
  });

  it('keeps the shared folder in the address, so the back button walks it', async () => {
    const ROOT = 'sha256:aa';
    const ORDERS = 'sha256:bb';
    const listing = (identity: string, entries: StoreEntry[]) =>
      json(200, { node: 'hq', instance: 'local', identity, entries });
    let search = $state('');
    const asked: string[] = [];
    const ctx = appContext(
      {
        ...sharing,
        [`GET ${MOUNT}/files/stores`]: json(200, {
          node: 'hq/brno',
          items: [
            {
              node: 'hq',
              interface: 'ContentStore1',
              instance: 'local',
              status: 'OK',
              updated_at: 1_793_700_000_000,
              props: { rootHash: ROOT },
            },
          ],
          total: 1,
          truncated: false,
        }),
        [`GET ${MOUNT}/files/stores/hq/local/${ROOT}`]: listing(ROOT, [
          { name: 'orders', kind: 'directory', size: 0, identity: ORDERS },
        ]),
        [`GET ${MOUNT}/files/stores/hq/local/${ORDERS}`]: listing(ORDERS, [
          {
            name: 'gear.bgcode',
            kind: 'file',
            size: 2048,
            identity: 'sha256:cc',
          },
        ]),
      },
      {
        path: '/storage',
        go: (path) => {
          asked.push(path);
          search = new URL(path, 'http://app.invalid').search;
        },
      },
    );
    // The router's query changes under the screen, as on a real navigation.
    Object.defineProperty(ctx, 'search', { get: () => search });
    await ctx.modules.load();

    await render(StorageFiles, { props: { ctx } });

    await page.getByRole('button', { name: 'orders' }).click();
    expect(asked.at(-1)).toBe('/storage?node=hq&store=local&folder=orders');
    await expect.element(page.getByText('gear.bgcode')).toBeVisible();

    // A step in this server's own folders keeps the shared one where it is.
    await page.getByRole('button', { name: 'projects' }).click();
    expect(asked.at(-1)).toBe(
      '/storage/local/folder/projects?node=hq&store=local&folder=orders',
    );

    // The back button takes the query away, and the list goes back to the top.
    search = '';
    await expect
      .element(page.getByRole('button', { name: 'orders' }))
      .toBeVisible();
    expect(page.getByText('gear.bgcode').query()).toBeNull();
  });
});
