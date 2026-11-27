import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { Modules } from '@prusa/connect-ui/data';
import {
  createModuleLoader,
  type ModuleLoader,
  SLOT_FORMS,
  SLOT_SCREENS,
  SlotRegistry,
  type Screen,
  type SlotName,
} from '@prusa/connect-ui/slots';

import { appContext } from '../../test/app-context';
import {
  declareEverySlot,
  EVERY_SLOT_ENTRY,
  EVERY_SLOT_NAMESPACE,
} from '../../test/every-slot';
import { json, type Routes, stubClient, stubFetch } from '../../test/stub';
import WithSlots from '../../test/with-slots.svelte';
// Not exported from the package: the same fixture the registry's own tests use.
import PlainSection from '../../../../packages/svelte/src/test/plain-section.svelte';
import DashboardPage from './dashboard.svelte';
import FileDetail from './file-detail.svelte';
import JobDetailPage from './job-detail.svelte';
import JobsPage from './jobs.svelte';
import ExtensionDetail from './extension-detail.svelte';
import ModulePageScreen from './module-page.svelte';
import PrinterDetail from './printer-detail.svelte';
import Printers from './printers.svelte';
import StorageFiles from './storage.svelte';

const drawModulePage = async (registry: SlotRegistry): Promise<void> => {
  const ctx = appContext(
    { 'GET /api/v1/devices': json(200, { items: [] }) },
    { params: { namespace: EVERY_SLOT_NAMESPACE, path: EVERY_SLOT_ENTRY } },
  );

  await render(WithSlots, {
    props: { registry, screen: ModulePageScreen, ctx },
  });
};

// The settings panes are one tab each on the page of their module.
const drawExtensionDetail = async (registry: SlotRegistry): Promise<void> => {
  const { client } = stubClient({
    'GET /api/v1/resources': json(200, {
      items: [
        {
          kind: 'sink',
          name: 'every-slot',
          namespace: EVERY_SLOT_NAMESPACE,
          enabled: true,
          status: { state: 'ready' },
          module: { version: '1.0.0' },
        },
      ],
    }),
  });
  const modules = new Modules(client);
  await modules.load();
  const ctx = appContext(
    { 'GET /api/v1/devices': json(200, { items: [] }) },
    {
      modules,
      params: {
        namespace: EVERY_SLOT_NAMESPACE,
        tab: `${EVERY_SLOT_NAMESPACE}.${EVERY_SLOT_ENTRY}`,
      },
    },
  );

  await render(WithSlots, {
    props: { registry, screen: ExtensionDetail, ctx },
  });
};

// Task 8 adds a `device-tab` slot on another tab; the outlet the status tab
// already carries stays the default so this draw keeps working for it too.
const drawPrinterDetail = async (
  registry: SlotRegistry,
  tab = 'status',
): Promise<void> => {
  const ctx = appContext(
    {
      'GET /api/v1/devices': json(200, {
        items: [
          {
            id: 'a',
            name: 'Printer a',
            kind: 'printer',
            model: 'MK4S',
            connected: true,
            labels: {},
            actions: ['observe', 'operate'],
            capabilities: [],
            state: { '/state': 'IDLE' },
            // The Printer tab reads the record's link counters.
            link: { counted: false },
          },
        ],
        total: 1,
      }),
    },
    { params: { id: 'a', tab } },
  );

  await render(WithSlots, { props: { registry, screen: PrinterDetail, ctx } });
};

// The device-file-action outlet is in the menu of each file on the Files
// tab, so its draw opens that tab on a printer holding one print file.
const drawPrinterFiles = async (registry: SlotRegistry): Promise<void> => {
  const routes: Routes = {
    'GET /api/v1/devices': json(200, {
      items: [
        {
          id: 'a',
          name: 'Printer a',
          kind: 'printer',
          model: 'MK4S',
          connected: true,
          labels: {},
          actions: ['observe', 'operate'],
          capabilities: ['core.command.start_print'],
          state: { '/state': 'IDLE' },
        },
      ],
      total: 1,
    }),
    'GET /api/v1/devices/a/files': json(200, {
      items: [
        {
          folder: false,
          name: 'bracket.bgcode',
          path: 'usb/bracket.bgcode',
          read_only: false,
          size: 2048,
        },
      ],
    }),
  };
  // The Files tab reads the printer's files through the page's own fetch.
  vi.stubGlobal('fetch', stubFetch(routes).fetch);
  const ctx = appContext(routes, { params: { id: 'a', tab: 'files' } });

  await render(WithSlots, { props: { registry, screen: PrinterDetail, ctx } });
  await page
    .getByRole('button', { name: 'Actions for bracket.bgcode' })
    .click();
};

// The device-upload-action outlet is a choice in the question a file dropped
// on a printer asks, so its draw drops one file on a printer that takes
// uploads.
const drawPrinterDrop = async (registry: SlotRegistry): Promise<void> => {
  const routes: Routes = {
    'GET /api/v1/devices': json(200, {
      items: [
        {
          id: 'a',
          name: 'Printer a',
          kind: 'printer',
          model: 'MK4S',
          connected: true,
          labels: {},
          actions: ['observe', 'operate'],
          capabilities: ['core.command.start_print'],
          state: { '/state': 'IDLE' },
        },
      ],
      total: 1,
    }),
    'GET /api/v1/devices/a/files': json(200, { items: [] }),
    'GET /api/v1/devices/a/storages': json(200, {
      items: [
        {
          free_space: 1024,
          mountpoint: '/usb',
          name: 'usb',
          read_only: false,
          type: 'USB',
        },
      ],
      reported: true,
    }),
    'GET /api/v1/storages': json(200, {
      items: [
        {
          browse: true,
          display_name: 'Uploads',
          download_url: true,
          move: true,
          name: 'uploads',
          ranges: true,
          write: true,
        },
      ],
    }),
  };
  vi.stubGlobal('fetch', stubFetch(routes).fetch);
  const ctx = appContext(routes, { params: { id: 'a', tab: 'status' } });

  await render(WithSlots, { props: { registry, screen: PrinterDetail, ctx } });
  await expect
    .poll(() => document.querySelector('input[type="file"]'), { timeout: 5000 })
    .not.toBeNull();
  const input = document.querySelector<HTMLInputElement>('input[type="file"]');
  const files = new DataTransfer();
  files.items.add(new File(['G28'], 'bracket.bgcode'));
  if (input !== null) {
    input.files = files.files;
    input.dispatchEvent(new Event('change', { bubbles: true }));
  }
};

// An action slot's outlet is inside the header's kebab menu, closed until a
// person opens it.
const openMenu = async (): Promise<void> => {
  await page.getByRole('button', { name: /^Actions for/ }).click();
};

const drawPrinters = async (registry: SlotRegistry): Promise<void> => {
  const ctx = appContext({
    'GET /api/v1/devices': json(200, {
      items: [
        {
          id: 'a',
          name: 'Printer a',
          kind: 'printer',
          model: 'MK4S',
          labels: {},
          state: { '/state': 'IDLE' },
        },
      ],
    }),
  });

  // The list keeps its view in the browser's storage, and every test file
  // shares that storage. The printers page test leaves it on the camera view,
  // which draws no card, so this draw starts from the default view.
  localStorage.clear();

  await render(WithSlots, { props: { registry, screen: Printers, ctx } });
};

// The dashboard draws each placed card in a cell named for the slot. The
// one dashboard here holds the module's card.
const drawDashboard = async (registry: SlotRegistry): Promise<void> => {
  const dashboard = {
    id: 1,
    name: 'Every slot',
    description: '',
    visibility: 'everyone',
    is_default: true,
    columns: 12,
    density: 'comfortable',
    cards: [
      {
        id: 'c1',
        type: `${EVERY_SLOT_NAMESPACE}/${EVERY_SLOT_ENTRY}`,
        x: 0,
        y: 0,
        w: 2,
        h: 2,
        title: '',
        config: {},
      },
    ],
    revision: 1,
    created: '2026-10-08T09:00:00Z',
    created_by: 'user:demo',
    changed: '2026-10-08T09:00:00Z',
    changed_by: 'user:demo',
  };
  const routes = {
    'GET /api/v1/devices': json(200, { items: [] }),
    'GET /api/v1/dashboards': json(200, {
      items: [{ ...dashboard, cards: 1 }],
      total: 1,
      default_id: 1,
    }),
    'GET /api/v1/dashboards/1': json(200, dashboard),
    'POST /api/v1/access/check': json(200, { allowed: false }),
  };
  const base = appContext(routes);
  const { client } = stubClient(routes);
  const ctx = {
    ...base,
    slots: {
      registry,
      loader: createModuleLoader({ client, registry, fleet: base.fleet }),
    },
  };

  await render(WithSlots, { props: { registry, screen: DashboardPage, ctx } });
};

// One storage holding one printable file, and one printer free to take it.
const storageRoutes = {
  'GET /api/v1/storages': json(200, {
    items: [
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
    ],
  }),
  'GET /api/v1/storages/local/files': json(200, {
    items: [
      {
        folder: false,
        modified: '2026-09-01T10:00:00Z',
        name: 'bracket.bgcode',
        path: 'bracket.bgcode',
        size: 2048,
      },
    ],
  }),
  'GET /api/v1/devices': json(200, {
    items: [
      {
        capabilities: ['core.command.start_print'],
        connected: true,
        id: 'a',
        kind: 'printer',
        labels: {},
        model: 'MK4S',
        name: 'Printer a',
        state: { '/state': 'IDLE' },
      },
    ],
  }),
};

// The file-action menu sits on each file's row of the listing.
const drawStorage = async (registry: SlotRegistry): Promise<void> => {
  // The listing keeps its view in the browser's storage, shared by every
  // test file, so this draw starts from the default table view.
  localStorage.clear();

  await render(WithSlots, {
    props: { registry, screen: StorageFiles, ctx: appContext(storageRoutes) },
  });
};

// The file-view outlet is on a file's own page, in place of what the file
// holds (ADR-20261027-wvj rule 16). A file-tab outlet mounts only inside its
// tab, which the query opens (ADR-20261029-dgv).
const drawFileDetail = async (
  registry: SlotRegistry,
  search = '',
): Promise<void> => {
  await render(WithSlots, {
    props: {
      registry,
      screen: FileDetail,
      ctx: appContext(storageRoutes, {
        params: { storage: 'local', path: 'bracket.bgcode' },
        search,
      }),
    },
  });
};

// The print dialog opens from a file's print button on the storage page.
const drawPrintDialog = async (registry: SlotRegistry): Promise<void> => {
  await drawStorage(registry);
  await page.getByRole('button', { name: 'Print bracket.bgcode' }).click();
};

// One printer that ran one print, for the Jobs listing and the print's page.
const JOB_ID = '0199e3a4-5bb5-7aac-8a2c-9c3982e6373f';
const jobRecord = {
  device_id: 'a',
  device_job_id: '412',
  display_name: 'bracket.bgcode',
  ended: '2026-09-01T09:00:00Z',
  id: JOB_ID,
  path: 'usb/bracket.bgcode',
  progress: 100,
  started: '2026-09-01T08:00:00Z',
  state: 'FINISHED',
  time_remaining_ms: 0,
};
const jobRoutes = {
  'GET /api/v1/devices': json(200, {
    items: [
      {
        id: 'a',
        name: 'Printer a',
        kind: 'printer',
        model: 'MK4S',
        labels: {},
      },
    ],
  }),
  'GET /api/v1/jobs': json(200, { items: [jobRecord], cursor: '' }),
  [`GET /api/v1/jobs/${JOB_ID}`]: json(200, jobRecord),
  'GET /api/v1/devices/a/transfers': json(200, { items: [] }),
};

// A job-column cell is in the module's column of each print's row.
const drawJobs = async (registry: SlotRegistry): Promise<void> => {
  localStorage.clear();

  await render(WithSlots, {
    props: { registry, screen: JobsPage, ctx: appContext(jobRoutes) },
  });
};

// A job-section is under the details of a print's own page.
const drawJobDetail = async (registry: SlotRegistry): Promise<void> => {
  await render(WithSlots, {
    props: {
      registry,
      screen: JobDetailPage,
      ctx: appContext(jobRoutes, { params: { id: JOB_ID } }),
    },
  });
};

// One way to draw each screen with a registry. A screen added to the table
// without a way to draw it here fails the typecheck, and a slot added to a
// screen without an outlet fails the test below.
const DRAW: Record<Screen, (registry: SlotRegistry) => Promise<void>> = {
  app: drawModulePage,
  dashboard: drawDashboard,
  settings: drawExtensionDetail,
  'printer-detail': drawPrinterDetail,
  printers: drawPrinters,
  storage: drawStorage,
  'print-dialog': drawPrintDialog,
  jobs: drawJobs,
  'job-detail': drawJobDetail,
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('every slot has an outlet on its screen', () => {
  // Wide enough for the file rows to draw their acts on the row: a
  // phone-width pane folds them into the row's menu.
  beforeEach(async () => {
    await page.viewport(1400, 900);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // `Object.entries` widens keys to string, and `SLOT_SCREENS` is a closed
  // literal.
  for (const [slot, screen] of Object.entries(SLOT_SCREENS) as [
    SlotName,
    Screen,
  ][]) {
    // A slot the host derives rather than mounts, such as `sidebar`, has no
    // outlet to find.
    if (SLOT_FORMS[slot] === 'derived') continue;

    it(`${slot} on ${screen}`, async () => {
      const registry = new SlotRegistry();
      declareEverySlot(registry, screen);
      // A device-tab outlet mounts only inside the module tab it belongs to,
      // so its draw must open that tab rather than the default one.
      if (slot === 'device-tab') {
        await drawPrinterDetail(
          registry,
          `${EVERY_SLOT_NAMESPACE}.${EVERY_SLOT_ENTRY}`,
        );
      } else if (slot === 'camera-section' || slot === 'camera-view') {
        // A camera section mounts under each camera in the Cameras tab, and a
        // camera view over a camera of the module's type while it plays.
        // The page reads its cameras through the app's own client, so the
        // one camera that puts the Cameras tab on the page is answered there,
        // with the camera types that name its module.
        vi.stubGlobal(
          'fetch',
          stubFetch({
            'GET /api/v1/camera-types': json(200, {
              items: [
                { type: 'generic' },
                {
                  type: `${EVERY_SLOT_NAMESPACE}/cam`,
                  title: 'Cam',
                  module: EVERY_SLOT_NAMESPACE,
                },
              ],
            }),
            'GET /api/v1/cameras': json(200, {
              items: [
                {
                  name: 'bed-cam',
                  type: `${EVERY_SLOT_NAMESPACE}/cam`,
                  device_id: 'a',
                  formats: ['jpeg'],
                  serving: true,
                  snapshot: true,
                  stream: false,
                  watch: false,
                },
              ],
            }),
          }).fetch,
        );
        // A module's view is offered once the camera's still has loaded, and
        // no server answers the still here, so every picture loads at once.
        vi.stubGlobal(
          'Image',
          class {
            onload: (() => void) | null = null;
            set src(_: string) {
              queueMicrotask(() => this.onload?.());
            }
          },
        );
        // The camera's view plays by itself while its card is seen
        // (ADR-20261119-pxn), so the outlet mounts with no press.
        await drawPrinterDetail(registry, 'cameras');
      } else if (slot === 'file-view') {
        await drawFileDetail(registry);
      } else if (slot === 'file-tab') {
        await drawFileDetail(
          registry,
          `?tab=${EVERY_SLOT_NAMESPACE}.${EVERY_SLOT_ENTRY}`,
        );
      } else if (slot === 'device-info-section') {
        // Its outlet is on the Printer tab, under the Connection card.
        await drawPrinterDetail(registry, 'info');
      } else if (slot === 'device-file-action') {
        await drawPrinterFiles(registry);
      } else if (slot === 'device-upload-action') {
        await drawPrinterDrop(registry);
      } else {
        await DRAW[screen](registry);
      }

      // An action slot's outlet mounts inside the kebab menu, which starts
      // closed.
      if (
        SLOT_FORMS[slot] === 'action' &&
        slot !== 'device-file-action' &&
        slot !== 'device-upload-action'
      ) {
        await openMenu();
      }

      // The first case in the file also waits for the browser to compile
      // the screen, which on a CI runner takes longer than the default second.
      await expect
        .poll(() => document.querySelector(`[data-slot="${slot}"]`), {
          timeout: 5000,
        })
        .not.toBeNull();
    });
  }
});

// ADR-20261110-5np: the printers page hands each card's line the printer's
// record from the fleet, so a line with a devices selector is on the resident
// printer's card only.
// ADR-20261123-wfw rule 3: the Jobs listing reads its module columns from the
// declarations. Opened first, before any other screen read them, it reads the
// listing itself, or it would draw no column and so no cell to load a bundle.
describe('the Jobs listing opened first', () => {
  it('reads the declarations and draws the module column', async () => {
    localStorage.clear();
    const registry = new SlotRegistry();
    const discover = vi.fn(() => {
      registry.declare(EVERY_SLOT_NAMESPACE, {
        job_column: [{ id: 'used', title: 'Filament' }],
      });

      return Promise.resolve();
    });
    const loader = {
      discover,
      activate: () => Promise.resolve(),
    } as unknown as ModuleLoader;

    await render(WithSlots, {
      props: { registry, loader, screen: JobsPage, ctx: appContext(jobRoutes) },
    });

    expect(discover).toHaveBeenCalled();
    await expect
      .element(page.getByRole('columnheader', { name: 'Filament' }))
      .toBeVisible();
  });
});

describe('a device-card line only for the printer the server runs on', () => {
  it('is drawn on the resident card and on no other', async () => {
    const registry = new SlotRegistry();
    registry.declare('display', {
      device_card: [{ id: 'line', title: 'Line', devices: { resident: true } }],
    });
    registry.registerSection('display', 'device-card', 'line', PlainSection);
    const row = (id: string) => ({
      id,
      name: `Printer ${id}`,
      kind: 'printer',
      model: 'MK4S',
      labels: {},
      state: { '/state': 'IDLE' },
    });
    const ctx = appContext({
      'GET /api/v1/devices': json(200, {
        items: [{ ...row('host'), resident: true }, row('mk4')],
      }),
    });
    localStorage.clear();

    await render(WithSlots, { props: { registry, screen: Printers, ctx } });

    await expect
      .element(page.getByText('plain section for host'))
      .toBeInTheDocument();
    await expect
      .element(page.getByText('Printer mk4', { exact: true }))
      .toBeInTheDocument();
    expect(page.getByText('plain section for mk4').query()).toBeNull();
  });
});
