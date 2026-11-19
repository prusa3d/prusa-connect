import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { Storage } from '../../data';
import { StubApiError } from '../../data/stubs';
import { START_PRINT_VERB } from '../../data/services/print';
import type { StorageClient } from '../../data/services/storage';
import type { Device, PrintOutcome, PrintRequest } from '../../domain';
import { STATE_PATH } from '../../domain';
import { toast } from '../../primitives';
import { expectNoViolations } from '../../test/axe';
import { fakeDevice, fakeListingStore } from '../../test/fakes';
import StorageScreen from './storage-screen.svelte';
import type {
  FolderSharing,
  StorageChoice,
  StorageRow,
  StorageSource,
} from './types';

/** One printer waiting for work, which is what the print sheet offers. */
const waiting = (id: string, name: string): Device => ({
  ...fakeDevice(id, name),
  capabilities: [START_PRINT_VERB],
  state: { [STATE_PATH]: 'IDLE' },
});

const started: PrintOutcome = {
  commandId: 'cmd-1',
  faultCode: undefined,
  machineReason: undefined,
  ok: true,
  path: 'benchy.3mf',
  transferId: 'transfer-1',
};

const body = () => page.elementLocator(document.body);

const store = fakeListingStore();

const storageOf = (name: string, write = true): StorageChoice => ({
  allowed_extensions: ['.gcode', '.bgcode', '.3mf'],
  browse: true,
  display_name: name,
  download_url: false,
  move: false,
  name,
  ranges: false,
  write,
});

const fileOf = (
  name: string,
  path = name,
  size = 2048,
  modified = '2026-08-18T14:02:00Z',
): StorageRow => ({
  folder: false,
  modified,
  name,
  path,
  size,
});

const folderOf = (name: string, path = name): StorageRow => ({
  folder: true,
  modified: '2026-08-18T09:41:00Z',
  name,
  path,
  size: 0,
});

const source = (over: Partial<StorageSource> = {}): StorageSource => ({
  cancelUpload: () => undefined,
  clearUploads: () => undefined,
  createFolder: () => Promise.resolve(),
  contentUrl: (name: string, path: string) =>
    `/api/v1/storages/${name}/content?path=${path}`,
  describe: () => Promise.resolve(),
  describedFile: () => undefined,
  directory: '',
  entries: [],
  error: undefined,
  filesError: undefined,
  filesLoaded: true,
  filesLoading: false,
  browseFolders: () => Promise.resolve(),
  load: () => Promise.resolve(),
  loaded: true,
  loading: false,
  moveAll: () => Promise.resolve([]),
  open: () => Promise.resolve(),
  pickerError: undefined,
  pickerFolders: [],
  pickerPath: '',
  remove: () => Promise.resolve(),
  storage: 'local',
  storages: [storageOf('local')],
  upload: () => Promise.resolve(undefined),
  uploads: [],
  ...over,
});

const entries: StorageRow[] = [
  fileOf('benchy.3mf', 'benchy.3mf', 4_194_304),
  folderOf('archive'),
  fileOf(
    'gasket_set_tpu.bgcode',
    'gasket_set_tpu.bgcode',
    2_411_724,
    '2026-09-01T08:00:00Z',
  ),
];

/**
 * The real store over a client that holds one folder and moves entries. The
 * move of the entry named `refuse` is refused.
 */
const movingStorage = (refuse?: string) => {
  const moved: [string, string][] = [];
  let held = [...entries];
  const client: StorageClient = {
    url: 'https://core.example',
    deleteStorageEntry: () => Promise.resolve(),
    createStorageFolder: () => Promise.resolve({ path: '' }),
    moveStorageEntry: (request) => {
      if (request.body.from === refuse) {
        return Promise.reject(new StubApiError('core.storage/EXISTS', 409));
      }
      moved.push([request.body.from, request.body.to]);
      held = held.filter((row) => row.path !== request.body.from);

      return Promise.resolve({ path: request.body.to });
    },
    listStorageFiles: (request) =>
      Promise.resolve({
        items: (request.query?.path ?? '') === '' ? [...held] : [],
      }),
    listStorages: () =>
      Promise.resolve({ items: [{ ...storageOf('local'), move: true }] }),
    statStorageEntry: () => Promise.reject(new Error('not asked here')),
    uploadStorageObject: () => Promise.reject(new Error('not asked here')),
  };

  return { storage: new Storage(client), moved };
};

describe('StorageScreen', () => {
  // Wide enough for every column and every row action; a phone-width pane
  // folds them, which the narrow cases below set up for themselves.
  beforeEach(async () => {
    await page.viewport(1400, 900);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('names itself and lists what the open storage holds', async () => {
    await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });

    await expect
      .element(page.getByRole('heading', { level: 1 }))
      .toHaveTextContent('Storage');
    await expect.element(page.getByText('benchy.3mf')).toBeVisible();
  });

  it('says which files the open storage takes', async () => {
    await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });

    await expect
      .element(page.getByText('Accepted: .gcode, .bgcode, .3mf'))
      .toBeVisible();
  });

  // Neither the order they arrive in nor their dates set the order: the
  // newest file sorts last by name.
  it('puts the folders first, then the files by name', async () => {
    const { container } = await render(StorageScreen, {
      props: {
        storage: source({
          entries: [
            fileOf('cube.gcode', 'cube.gcode', 2048, '2026-09-01T08:00:00Z'),
            folderOf('zeta'),
            fileOf(
              'anchor.gcode',
              'anchor.gcode',
              2048,
              '2026-08-01T08:00:00Z',
            ),
            folderOf('beta'),
          ],
        }),
        store: fakeListingStore(),
      },
    });
    const names = [...container.querySelectorAll('tbody tr')].map((row) =>
      row.textContent.replace(/\s+/g, ' ').trim(),
    );

    expect(names).toHaveLength(4);
    expect(names[0]).toContain('beta');
    expect(names[1]).toContain('zeta');
    expect(names[2]).toContain('anchor.gcode');
    expect(names[3]).toContain('cube.gcode');
  });

  // What the bar does with the ticked rows is the whole point of ticking them.
  it('moves the ticked rows into the folder the picker walked to', async () => {
    const said = vi.spyOn(toast, 'success');
    const { storage, moved } = movingStorage();
    await storage.load();
    await storage.open('local');

    await render(StorageScreen, { props: { storage, store } });

    await page.getByRole('checkbox', { name: 'Select benchy.3mf' }).click();
    await expect.element(page.getByText('1 selected')).toBeVisible();

    await page.getByRole('button', { name: 'Move to…' }).click();
    // Scoped to the dialog: the listing behind it holds a folder of that name
    // too, which is the one the picker was opened over.
    const picker = body().getByRole('dialog');
    // The picker opens at the storage root, so the row steps one folder in.
    await picker.getByRole('button', { name: 'archive' }).click();
    await expect
      .element(picker.getByRole('button', { name: 'Move here' }))
      .toBeEnabled();
    await picker.getByRole('button', { name: 'Move here' }).click();

    await expect
      .poll(() => moved)
      .toEqual([['benchy.3mf', 'archive/benchy.3mf']]);
    await expect.poll(() => said.mock.calls).toEqual([['1 item moved']]);
  });

  // One refused move and one that went through: the count is the one that
  // moved, and the refused entry is named.
  it('reports each entry and keeps the refusal', async () => {
    const said = vi.spyOn(toast, 'success');
    const warned = vi.spyOn(toast, 'error');
    const { storage } = movingStorage('benchy.3mf');
    await storage.load();
    await storage.open('local');

    await render(StorageScreen, { props: { storage, store } });

    await page.getByRole('checkbox', { name: 'Select benchy.3mf' }).click();
    await page
      .getByRole('checkbox', { name: 'Select gasket_set_tpu.bgcode' })
      .click();
    await page.getByRole('button', { name: 'Move to…' }).click();
    const picker = body().getByRole('dialog');
    await picker.getByRole('button', { name: 'archive' }).click();
    await expect
      .element(picker.getByRole('button', { name: 'Move here' }))
      .toBeEnabled();
    await picker.getByRole('button', { name: 'Move here' }).click();

    await expect.poll(() => said.mock.calls).toEqual([['1 item moved']]);
    expect(warned.mock.calls.map((call) => call[0])).toEqual([
      'benchy.3mf was not moved.',
    ]);
    // The refusal stays on the page by its code.
    await expect
      .element(body().getByText('core.storage/EXISTS').first())
      .toBeVisible();
  });

  it('offers no move where the storage has no rename of its own', async () => {
    await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });

    await page.getByRole('checkbox', { name: 'Select benchy.3mf' }).click();
    await expect.element(page.getByText('1 selected')).toBeVisible();
    expect(
      page.getByRole('button', { name: 'Move to…' }).elements(),
    ).toHaveLength(0);
  });

  it('opens a folder when the row is pressed', async () => {
    const opened: string[] = [];
    await render(StorageScreen, {
      props: {
        storage: source({
          entries,
          open: (_name: string, path: string) => {
            opened.push(path);

            return Promise.resolve();
          },
        }),
        store,
      },
    });

    await page.getByRole('button', { name: 'archive' }).click();

    expect(opened).toEqual(['archive']);
  });

  // A host that keeps the folder in the address is told where the reader
  // went, and the screen opens nothing until the address says so.
  it('hands a folder press to the host that moves the address', async () => {
    const opened: string[] = [];
    const onNavigate = vi.fn();
    await render(StorageScreen, {
      props: {
        storage: source({
          entries,
          open: (_name: string, path: string) => {
            opened.push(path);

            return Promise.resolve();
          },
        }),
        store,
        onNavigate,
      },
    });

    await page.getByRole('button', { name: 'archive' }).click();

    expect(onNavigate).toHaveBeenCalledWith({
      storage: 'local',
      directory: 'archive',
    });
    expect(opened).toEqual([]);
  });

  it('opens the storage and the folder the address names', async () => {
    const opened: string[] = [];
    await render(StorageScreen, {
      props: {
        storage: source({
          entries,
          storages: [storageOf('local'), storageOf('usb')],
          open: (name: string, path: string) => {
            opened.push(`${name}:${path}`);

            return Promise.resolve();
          },
        }),
        store,
        address: { storage: 'usb', directory: 'models' },
      },
    });

    await expect.poll(() => opened).toEqual(['usb:models']);
  });

  it('links a file to its page on the storage that holds it', async () => {
    await render(StorageScreen, {
      props: {
        storage: source({ entries, storage: 'usb' }),
        store,
        fileHref: (name: string, row: StorageRow) => `/${name}/${row.path}`,
      },
    });

    await expect
      .element(page.getByRole('link', { name: 'benchy.3mf' }))
      .toHaveAttribute('href', '/usb/benchy.3mf');
  });

  it('walks back up the trail without a second page landmark', async () => {
    const opened: string[] = [];
    const { container } = await render(StorageScreen, {
      props: {
        storage: source({
          entries,
          directory: 'archive/2025_prototypes',
          open: (_name: string, path: string) => {
            opened.push(path);

            return Promise.resolve();
          },
        }),
        store,
      },
    });

    await page.getByRole('button', { name: 'local' }).click();

    expect(opened).toEqual(['']);
    // The trail is a breadcrumb with a name of its own.
    expect(
      container.querySelector('nav[aria-label="Folders in this storage"]'),
    ).not.toBe(null);
  });

  it('says a server holds no storage rather than drawing an empty folder', async () => {
    await render(StorageScreen, {
      props: { storage: source({ storages: [], storage: '' }), store },
    });

    await expect
      .element(page.getByText('Storage is not enabled on this server.'))
      .toBeVisible();
  });

  it('draws a refused listing through the registry, code and all', async () => {
    const refused = {
      traceId: undefined,
      code: 'core.storage/INVALID_PATH',
      message: 'the path is outside the root',
      params: {},
      status: 400,
    };
    await render(StorageScreen, {
      props: { storage: source({ filesError: refused }), store },
    });

    await expect
      .element(page.getByText('core.storage/INVALID_PATH'))
      .toBeVisible();
  });

  it('asks before it removes a file', async () => {
    const removed: string[] = [];
    await render(StorageScreen, {
      props: {
        storage: source({
          entries,
          remove: (_name: string, path: string) => {
            removed.push(path);

            return Promise.resolve();
          },
        }),
        store,
      },
    });

    await page.getByRole('button', { name: 'Delete benchy.3mf' }).click();
    expect(removed).toEqual([]);

    await page
      .elementLocator(document.body)
      .getByRole('button', { name: 'Delete', exact: true })
      .click();

    expect(removed).toEqual(['benchy.3mf']);
  });

  it('creates a folder in the open folder', async () => {
    const made: string[] = [];
    await render(StorageScreen, {
      props: {
        storage: source({
          directory: 'models',
          entries,
          createFolder: (_name: string, path: string) => {
            made.push(path);

            return Promise.resolve();
          },
        }),
        store,
      },
    });

    await page.getByRole('button', { name: 'New folder' }).click();
    await page
      .elementLocator(document.body)
      .getByRole('textbox', { name: 'Folder name' })
      .fill('spares');
    await page
      .elementLocator(document.body)
      .getByRole('button', { name: 'Create' })
      .click();

    expect(made).toEqual(['models/spares']);
  });

  it('renames a file in its own folder and keeps its extension', async () => {
    const moved: [string, string][] = [];
    await render(StorageScreen, {
      props: {
        storage: source({
          directory: 'models',
          entries: [fileOf('benchy.3mf', 'models/benchy.3mf', 4_194_304)],
          moveAll: (
            _name: string,
            moves: readonly { from: string; to: string }[],
          ) => {
            moved.push(
              ...moves.map((move): [string, string] => [move.from, move.to]),
            );

            return Promise.resolve([]);
          },
        }),
        store,
      },
    });

    await page.getByRole('button', { name: 'Rename benchy.3mf' }).click();
    const box = page
      .elementLocator(document.body)
      .getByRole('textbox', { name: 'New name' });
    await expect.element(box).toHaveValue('benchy');
    await box.fill('boat');
    await page
      .elementLocator(document.body)
      .getByRole('button', { name: 'Rename', exact: true })
      .click();

    expect(moved).toEqual([['models/benchy.3mf', 'models/boat.3mf']]);
  });

  // A phone-width pane has room for one control per row: the acts fold into
  // the row's menu and the table never scrolls sideways.
  it('folds the row acts into a menu on a phone-width pane', async () => {
    await page.viewport(375, 800);
    await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });

    const table = page.getByRole('table').element();
    const scroller = table.parentElement!;
    await expect.element(page.getByRole('table')).toBeVisible();
    expect(scroller.scrollWidth).toBeLessThanOrEqual(scroller.clientWidth);
    expect(
      page.getByRole('button', { name: 'Rename benchy.3mf' }).query(),
    ).toBeNull();

    await page.getByRole('button', { name: 'Actions for benchy.3mf' }).click();
    await expect
      .element(page.getByRole('menuitem', { name: 'Rename' }))
      .toBeVisible();
    await expect
      .element(page.getByRole('menuitem', { name: 'Delete' }))
      .toBeVisible();
  });

  it('offers no new folder where the storage is read-only', async () => {
    await render(StorageScreen, {
      props: {
        storage: source({ entries, storages: [storageOf('local', false)] }),
        store,
      },
    });

    await expect
      .element(page.getByRole('button', { name: 'New folder' }))
      .not.toBeInTheDocument();
  });

  it('takes a drop only where the storage takes a write', async () => {
    const writable = await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });

    expect(
      writable.container.querySelector('[data-slot="file-dropzone"]'),
    ).not.toBe(null);
    await expect
      .element(page.getByRole('button', { name: 'Delete benchy.3mf' }))
      .toBeVisible();

    await writable.unmount();
    const locked = await render(StorageScreen, {
      props: {
        storage: source({ entries, storages: [storageOf('local', false)] }),
        store,
      },
    });

    expect(locked.container.querySelector('[data-slot="file-dropzone"]')).toBe(
      null,
    );
    await expect
      .element(page.getByRole('button', { name: 'Delete benchy.3mf' }))
      .not.toBeInTheDocument();
  });

  // The search is over the open folder and never over the storage.
  it('narrows the open folder to the names that match, and says how many', async () => {
    await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });

    await page.getByRole('searchbox', { name: 'Search files' }).fill('gasket');

    await expect.element(page.getByText('1 result')).toBeVisible();
    expect(page.getByText('benchy.3mf').query()).toBeNull();

    await page.getByText('Clear search').click();
    await expect.element(page.getByText('benchy.3mf')).toBeVisible();
  });

  // A search is a new list, so it starts at its first page. Twelve files at
  // five a page, read on the third page, and a search that matches two.
  it('goes back to the first page when the reader searches', async () => {
    const many = Array.from({ length: 12 }, (_, at) =>
      fileOf(`part_${String(at + 10)}.bgcode`),
    );
    const paged = {
      read: () => JSON.stringify({ pageSize: 5 }),
      write: () => undefined,
    };
    await render(StorageScreen, {
      props: { storage: source({ entries: many }), store: paged },
    });

    await page.getByRole('button', { name: 'Next' }).click();
    await page.getByRole('button', { name: 'Next' }).click();
    await expect.element(page.getByText(/Page 3 of 3/)).toBeVisible();

    await page.getByRole('searchbox', { name: 'Search files' }).fill('part_1');

    await expect.element(page.getByText(/Page 1 of 2/)).toBeVisible();
    await expect.element(page.getByText('part_10.bgcode')).toBeVisible();
  });

  // The box is 16px and the column's resize handle sits beside it, so the
  // whole header cell is the target.
  it('ticks the page from anywhere in the header cell', async () => {
    await render(StorageScreen, {
      props: { storage: source({ entries: [fileOf('benchy.3mf')] }), store },
    });
    const box = page.getByRole('checkbox', {
      name: 'Select every row on this page',
    });
    const cell = box.element().closest('label');
    if (cell === null) throw new Error('the header box has no label');

    await userEvent.click(cell, { position: { x: 2, y: 2 } });
    await expect.element(box).toBeChecked();
  });

  // The header box ticks the page on screen. The rest of the folder is a
  // second, deliberate step, so nobody deletes rows they never saw.
  it('ticks the page from the header, then the whole folder from the bar', async () => {
    const many = Array.from({ length: 12 }, (_, at) =>
      fileOf(`part_${String(at + 10)}.bgcode`),
    );
    const paged = {
      read: () => JSON.stringify({ pageSize: 5 }),
      write: () => undefined,
    };
    await render(StorageScreen, {
      props: { storage: source({ entries: many }), store: paged },
    });

    await page
      .getByRole('checkbox', { name: 'Select every row on this page' })
      .click();
    await expect.element(page.getByText('5 selected')).toBeVisible();

    await page.getByRole('button', { name: 'Select all 12' }).click();
    await expect.element(page.getByText('12 selected')).toBeVisible();
    expect(
      page.getByRole('button', { name: 'Select all 12' }).query(),
    ).toBeNull();
  });

  it('keeps the folder trail while the reader searches', async () => {
    await render(StorageScreen, {
      props: { storage: source({ entries, directory: 'archive' }), store },
    });

    await page.getByRole('searchbox', { name: 'Search files' }).fill('gasket');

    await expect.element(page.getByText('1 result')).toBeVisible();
    await expect
      .element(
        page.getByRole('navigation', { name: 'Folders in this storage' }),
      )
      .toBeVisible();
  });

  // A search belongs to the folder it was typed in.
  it('clears the search when the reader opens another folder', async () => {
    await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });

    await page.getByRole('searchbox', { name: 'Search files' }).fill('arch');
    await page.getByRole('button', { name: 'archive' }).click();

    await expect
      .element(page.getByRole('searchbox', { name: 'Search files' }))
      .toHaveValue('');
  });

  it('reports the uploads it has in flight', async () => {
    await render(StorageScreen, {
      props: {
        storage: source({
          entries,
          uploads: [
            {
              fault: undefined,
              path: 'bracket_left_v5.gcode',
              sent: 0,
              state: 'sending',
              storage: 'local',
              total: 18_874_368,
            },
          ],
        }),
        store,
      },
    });

    await expect
      .element(page.getByRole('region', { name: 'Uploads' }))
      .toBeVisible();
  });

  it('switches to the tiles, where a folder is still one press away', async () => {
    const opened: string[] = [];
    const { container } = await render(StorageScreen, {
      props: {
        storage: source({
          entries,
          open: (_name: string, path: string) => {
            opened.push(path);

            return Promise.resolve();
          },
        }),
        store,
      },
    });

    await page.getByRole('radio', { name: 'Cards' }).click();
    await expect.poll(() => container.querySelector('table')).toBe(null);

    // A tile's name carries the date it shows after the folder's name.
    await page.getByRole('button', { name: /^archive / }).click();

    expect(opened).toEqual(['archive']);
  });

  // The print action is the whole point of the page for a person who has just.
  it('offers no print action without a printer or a way to send', async () => {
    const nothing = await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });
    await expect
      .element(
        page.getByRole('button', { name: 'Print gasket_set_tpu.bgcode' }),
      )
      .not.toBeInTheDocument();
    await nothing.unmount();

    const noPrinter = await render(StorageScreen, {
      props: {
        storage: source({ entries }),
        store,
        devices: [],
        onPrint: () => Promise.resolve(started),
      },
    });
    await expect
      .element(
        page.getByRole('button', { name: 'Print gasket_set_tpu.bgcode' }),
      )
      .not.toBeInTheDocument();
    await noPrinter.unmount();
  });

  // A 3MF is a slicer's project and not G-code, so no printer starts a job from it.
  it('offers no print action on a folder or a file that is not G-code', async () => {
    await render(StorageScreen, {
      props: {
        storage: source({ entries }),
        store,
        devices: [waiting('one', 'MK4S #1042')],
        onPrint: () => Promise.resolve(started),
      },
    });

    await expect
      .element(
        page.getByRole('button', { name: 'Print gasket_set_tpu.bgcode' }),
      )
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Print archive' }))
      .not.toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Print benchy.3mf' }))
      .not.toBeInTheDocument();
  });

  it('sends the file the row names, and then says where the print is', async () => {
    const sent: PrintRequest[] = [];
    const onPrinted = vi.fn();
    await render(StorageScreen, {
      props: {
        storage: source({ entries }),
        store,
        devices: [waiting('one', 'MK4S #1042')],
        onPrint: (request: PrintRequest) => {
          sent.push(request);

          return Promise.resolve(started);
        },
        onPrinted,
      },
    });

    await page
      .getByRole('button', { name: 'Print gasket_set_tpu.bgcode' })
      .click();
    await body()
      .getByRole('button', { name: /^Print( on \d+ printers?)?$/ })
      .click();

    await expect
      .poll(() => sent)
      .toEqual([
        {
          deviceId: 'one',
          path: 'gasket_set_tpu.bgcode',
          source: 'gasket_set_tpu.bgcode',
          storage: 'local',
        },
      ]);
    expect(onPrinted).toHaveBeenCalledWith(['one']);
  });

  // The kinds' own table outranks the sheet's rule about a free bed.
  it('hands the print sheet the moves the device kind declared', async () => {
    await render(StorageScreen, {
      props: {
        storage: source({ entries }),
        store,
        devices: [waiting('one', 'MK4S #1042')],
        transitions: [
          { from: 'READY', to: 'PRINTING', verb: START_PRINT_VERB },
        ],
        onPrint: () => Promise.resolve(started),
      },
    });

    await page
      .getByRole('button', { name: 'Print gasket_set_tpu.bgcode' })
      .click();

    await expect
      .element(body().getByText('This works when the printer is READY.'))
      .toBeVisible();
  });

  // A printer another operator is holding is not one to send a file to.
  it('hands the print sheet the printers that are held', async () => {
    await render(StorageScreen, {
      props: {
        storage: source({ entries }),
        store,
        devices: [waiting('one', 'MK4S #1042')],
        locked: ['one'],
        onPrint: () => Promise.resolve(started),
      },
    });

    await page
      .getByRole('button', { name: 'Print gasket_set_tpu.bgcode' })
      .click();

    await expect
      .element(body().getByText('This printer is held by another command.'))
      .toBeVisible();
  });

  // The picture is the server's, read per file.
  it('asks what each file holds and shows the picture that came back', async () => {
    const asked: string[] = [];
    await render(StorageScreen, {
      props: {
        storage: source({
          entries,
          describe: (_name: string, path: string) => {
            asked.push(path);

            return Promise.resolve();
          },
          describedFile: (_name: string, path: string) =>
            path === 'benchy.3mf'
              ? { details: undefined, thumbnailUrl: '/thumb/benchy.png' }
              : undefined,
        }),
        store,
      },
    });

    await expect
      .poll(() => [...asked].sort())
      .toEqual(['benchy.3mf', 'gasket_set_tpu.bgcode']);
    await expect
      .element(page.getByRole('button', { name: 'Preview of benchy.3mf' }))
      .toBeVisible();
  });

  // Every ask opens the file on the server.
  it('asks nothing about a file the server cannot read a picture out of', async () => {
    const asked: string[] = [];
    await render(StorageScreen, {
      props: {
        storage: source({
          entries: [
            fileOf('shell.stl'),
            fileOf('notes.txt'),
            fileOf('benchy.3mf'),
          ],
          describe: (_name: string, path: string) => {
            asked.push(path);

            return Promise.resolve();
          },
        }),
        store,
      },
    });

    await expect.poll(() => asked).toEqual(['benchy.3mf']);
  });

  it('asks the server about each file exactly once', async () => {
    const many: StorageRow[] = Array.from({ length: 9 }, (_, at) =>
      fileOf(`plate-${String(at)}.bgcode`),
    );
    const stated: string[] = [];
    const client: StorageClient = {
      url: 'https://core.example',
      deleteStorageEntry: () => Promise.reject(new Error('no delete here')),
      createStorageFolder: () => Promise.resolve({ path: '' }),
      moveStorageEntry: () => Promise.resolve({ path: '' }),
      listStorageFiles: () => Promise.resolve({ items: [...many] }),
      listStorages: () => Promise.resolve({ items: [storageOf('local')] }),
      statStorageEntry: (request, signal) => {
        const path = request.query.path;
        stated.push(path);

        return new Promise((resolve, reject) => {
          setTimeout(() => {
            if (signal?.aborted === true) {
              reject(new Error('the read was stopped'));

              return;
            }
            resolve({
              folder: false,
              modified: '2026-08-18T14:02:00Z',
              name: path,
              path,
              // A picture, so the answer is written into the state a row reads.
              print: { thumbnail_height: 173, thumbnail_width: 313 },
              size: 2048,
            });
          }, 20);
        });
      },
      uploadStorageObject: () => Promise.reject(new Error('no upload here')),
    };

    await render(StorageScreen, {
      props: { storage: new Storage(client), store },
    });

    await expect
      .poll(() => new Set(stated).size, { timeout: 5000 })
      .toBe(many.length);
    // Room for the rounds the defect used to make.
    await new Promise((resolve) => setTimeout(resolve, 500));

    expect(stated).toHaveLength(many.length);
  });

  it('has no axe violations', async () => {
    const { container } = await render(StorageScreen, {
      props: { storage: source({ entries }), store },
    });

    await expectNoViolations(container);
  });
});

describe('StorageScreen sharing', () => {
  // Wide enough for every column and every row action; a phone-width pane
  // folds them, which the narrow cases below set up for themselves.
  beforeEach(async () => {
    await page.viewport(1400, 900);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const folders: StorageRow[] = [
    folderOf('projects'),
    folderOf('archive'),
    fileOf('benchy.3mf'),
  ];

  /** Shares `held` on the storage `local`, and records every change asked for. */
  const sharingOf = (
    held: readonly string[],
    over: Partial<FolderSharing> = {},
  ) => {
    const asked: [string, string, boolean][] = [];
    const sharing: FolderSharing = {
      sharedRoot: (storage: string, path: string) =>
        storage === 'local'
          ? held.find((root) => path === root || path.startsWith(`${root}/`))
          : undefined,
      canChange: true,
      share: (storage: string, path: string, shared: boolean) => {
        asked.push([storage, path, shared]);

        return Promise.resolve();
      },
      ...over,
    };

    return { sharing, asked };
  };

  it('has no Shared column on a server that shares nothing', async () => {
    await render(StorageScreen, {
      props: { storage: source({ entries: folders }), store },
    });

    await expect.element(page.getByText('projects')).toBeVisible();
    await expect
      .element(page.getByRole('columnheader', { name: 'Shared' }))
      .not.toBeInTheDocument();
  });

  it('asks before it shares a folder, and shares it once confirmed', async () => {
    const { sharing, asked } = sharingOf([]);
    await render(StorageScreen, {
      props: { storage: source({ entries: folders }), store, sharing },
    });

    const flip = page.getByRole('switch', {
      name: 'Share projects with the servers below',
    });
    await flip.click();
    expect(asked).toEqual([]);
    await expect
      .element(body().getByText(/full path, local · \/projects/))
      .toBeVisible();

    await body().getByRole('button', { name: 'Share', exact: true }).click();

    expect(asked).toEqual([['local', 'projects', true]]);
  });

  it('leaves the switch off when the reader does not confirm', async () => {
    const { sharing, asked } = sharingOf([]);
    await render(StorageScreen, {
      props: { storage: source({ entries: folders }), store, sharing },
    });

    const flip = page.getByRole('switch', {
      name: 'Share projects with the servers below',
    });
    await flip.click();
    await body().getByRole('button', { name: 'Cancel' }).click();

    expect(asked).toEqual([]);
    await expect.element(flip).not.toBeChecked();
  });

  it('stops sharing at once, with no question', async () => {
    const { sharing, asked } = sharingOf(['projects']);
    await render(StorageScreen, {
      props: { storage: source({ entries: folders }), store, sharing },
    });

    const flip = page.getByRole('switch', {
      name: 'Share projects with the servers below',
    });
    await expect.element(flip).toBeChecked();
    await flip.click();

    expect(asked).toEqual([['local', 'projects', false]]);
  });

  it('says which shared folder holds a folder, and offers it no switch', async () => {
    const { sharing } = sharingOf(['projects']);
    await render(StorageScreen, {
      props: {
        storage: source({
          directory: 'projects',
          entries: [folderOf('fixtures', 'projects/fixtures')],
        }),
        store,
        sharing,
      },
    });

    await expect
      .element(page.getByText('Inside shared projects'))
      .toBeVisible();
    await expect.element(page.getByRole('switch')).not.toBeInTheDocument();
  });

  it('shows a share it may not change as text', async () => {
    const { sharing } = sharingOf(['projects'], { canChange: false });
    await render(StorageScreen, {
      props: { storage: source({ entries: folders }), store, sharing },
    });

    await expect
      .element(page.getByRole('cell', { name: 'Shared', exact: true }))
      .toBeVisible();
    await expect.element(page.getByRole('switch')).not.toBeInTheDocument();
  });

  it('marks a shared folder on its tile too', async () => {
    const { sharing } = sharingOf(['projects']);
    const { container } = await render(StorageScreen, {
      props: {
        storage: source({ entries: folders }),
        store: fakeListingStore(),
        sharing,
      },
    });

    await page.getByRole('radio', { name: 'Cards' }).click();
    await expect.poll(() => container.querySelector('table')).toBe(null);

    await expect
      .element(
        page.getByRole('button', { name: /^projects/ }).getByText('Shared'),
      )
      .toBeVisible();
    expect(
      page
        .getByRole('button', { name: /^archive/ })
        .getByText('Shared')
        .query(),
    ).toBeNull();
  });

  it('says a share the server refused, with the reason', async () => {
    const failed = vi.spyOn(toast, 'error');
    const { sharing } = sharingOf(['projects'], {
      share: () =>
        Promise.reject(
          new StubApiError('com.prusa.fractal/SHARE_NOT_SAVED', 500),
        ),
    });
    await render(StorageScreen, {
      props: { storage: source({ entries: folders }), store, sharing },
    });

    await page
      .getByRole('switch', { name: 'Share projects with the servers below' })
      .click();

    await expect
      .poll(() => failed.mock.calls[0]?.[0])
      .toBe('projects is still shared.');
  });

  it('has no axe violations', async () => {
    const { sharing } = sharingOf(['projects']);
    const { container } = await render(StorageScreen, {
      props: { storage: source({ entries: folders }), store, sharing },
    });

    await expectNoViolations(container);
  });
});
