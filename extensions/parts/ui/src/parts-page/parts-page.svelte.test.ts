import { afterEach, describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import setup from '../index';
import {
  EMPTY_OVERVIEW,
  MOUNT,
  refusal,
  type StubOptions,
  stubHost,
} from '../test/host';

// The whole module, driven the way the app drives it: the stub host runs the
// bundle's own setup function, the setup registers its page, and each case
// renders the component that was registered. Nothing here imports past the
// entry, so what these cases prove is what a person would see.

const OVERVIEW = {
  ...EMPTY_OVERVIEW,
  folders: [
    {
      v: 1,
      id: 1,
      parent: 0,
      name: 'Garage',
      parts: 1,
      folders: 0,
      created: '',
      changed: '',
      changed_by: 'user:demo',
    },
  ],
  root_parts: 1,
  tags: [{ tag: 'bike', parts: 1 }],
};

const HOOK_ROW = {
  id: 5,
  folder: 0,
  name: 'Bike hook',
  summary: 'Holds a bicycle by its front wheel',
  tags: ['bike'],
  versions: 1,
  comments: 0,
  latest: { number: 1, files: 1, bytes: 4096, created: '2026-10-05T09:00:00Z' },
  changed: '2026-10-05T09:00:00Z',
  changed_by: 'user:demo',
};

const HOOK = {
  v: 1,
  id: 5,
  folder: 0,
  name: 'Bike hook',
  summary: 'Holds a bicycle by its front wheel',
  description: 'Print it in PETG.',
  tags: ['bike'],
  created: '2026-10-05T09:00:00Z',
  created_by: 'user:demo',
  changed: '2026-10-05T09:00:00Z',
  changed_by: 'user:demo',
  revision: 2,
  versions: [
    {
      v: 1,
      part: 5,
      number: 1,
      files: [
        {
          name: 'hook.bgcode',
          storage: 'local',
          path: 'parts/5/a1/hook.bgcode',
          size: 4096,
        },
      ],
      created: '2026-10-05T09:00:00Z',
      created_by: 'user:demo',
      changed: '2026-10-05T09:00:00Z',
      changed_by: 'user:demo',
    },
  ],
  comments: [
    {
      v: 1,
      id: 1,
      part: 5,
      body: 'Brim helps',
      by: 'user:bob',
      created: '2026-10-05T10:00:00Z',
    },
  ],
  dir: 'parts/5',
  storage: 'local',
};

/** The hook with a picture among its files, which a writer could show as the part's picture. */
const HOOK_WITH_PICTURE = {
  ...HOOK,
  versions: [
    {
      ...HOOK.versions[0],
      files: [
        ...(HOOK.versions[0]?.files ?? []),
        {
          name: 'hook.png',
          storage: 'local',
          path: 'parts/5/a1/hook.png',
          size: 2048,
        },
      ],
    },
  ],
};

const LISTING = {
  parts: [HOOK_ROW],
  matched: 1,
  held: 1,
  principal: 'user:demo',
};

/** Builds the module over a stub host and renders its page. */
const open = async (options: StubOptions = {}) => {
  const stub = stubHost(options);
  await setup(stub.host);
  const Page = stub.pages.parts;
  if (Page === undefined)
    throw new Error('the module registered no parts page');
  await render(Page);

  return stub;
};

/** A tree two levels deep: Garage, with Shelves inside it. */
const NESTED = {
  ...OVERVIEW,
  folders: [
    { ...OVERVIEW.folders[0], folders: 1 },
    {
      v: 1,
      id: 2,
      parent: 1,
      name: 'Shelves',
      parts: 0,
      folders: 0,
      created: '',
      changed: '',
      changed_by: 'user:demo',
    },
  ],
};

/** A drag that carries files, as the browser hands one to the page. */
const withFiles = (files: readonly File[]): DataTransfer => {
  const transfer = new DataTransfer();
  for (const file of files) transfer.items.add(file);

  return transfer;
};

const drag = (type: string, dataTransfer: DataTransfer): DragEvent =>
  new DragEvent(type, { bubbles: true, cancelable: true, dataTransfer });

/** Drags files onto an element and lets go, the events a person causes. */
const dropFiles = (target: Element, files: readonly File[]): void => {
  const transfer = withFiles(files);
  for (const type of ['dragenter', 'dragover', 'drop']) {
    target.dispatchEvent(drag(type, transfer));
  }
};

/**
 * Counts the drag listeners added to the window and not taken off again. The
 * page's own drop targets are the only ones that add them.
 */
const countWindowListeners = () => {
  const add = window.addEventListener.bind(window);
  const remove = window.removeEventListener.bind(window);
  let held = 0;
  const DRAG = new Set(['dragend', 'drop']);

  window.addEventListener = (
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: boolean | AddEventListenerOptions,
  ): void => {
    if (DRAG.has(type)) held += 1;
    add(type, listener, options);
  };
  window.removeEventListener = (
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: boolean | EventListenerOptions,
  ): void => {
    if (DRAG.has(type)) held -= 1;
    remove(type, listener, options);
  };

  return {
    count: () => held,
    restore: () => {
      window.addEventListener = add;
      window.removeEventListener = remove;
    },
  };
};

const start = window.location.href;

afterEach(() => {
  window.history.replaceState(null, '', start);
});

describe('the parts page', () => {
  it('asks for the first part in an empty catalogue', async () => {
    const stub = await open();

    await expect
      .element(
        page.getByRole('heading', { name: 'No part is in the catalogue yet' }),
      )
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'New part' }).first())
      .toBeVisible();

    // The page reaches the core through the SDK in session mode, so no call
    // carries a token of its own.
    expect(stub.calls.every((call) => call.authorization === null)).toBe(true);
  });

  it('draws the folders, the tags and the parts of the top', async () => {
    await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: OVERVIEW },
        [`GET ${MOUNT}/parts`]: { status: 200, body: LISTING },
      },
    });

    await expect
      .element(page.getByTestId('folder-tree').getByText('Garage'))
      .toBeVisible();
    await expect
      .element(page.getByTestId('tag-list').getByText('bike'))
      .toBeVisible();
    await expect
      .element(page.getByTestId('cards').getByText('Bike hook'))
      .toBeVisible();
  });

  it('hides every control that changes the catalogue from a person who may only read', async () => {
    await open({
      allow: ['view_resource', 'read_storage'],
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: OVERVIEW },
        [`GET ${MOUNT}/parts`]: { status: 200, body: LISTING },
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: HOOK_WITH_PICTURE, principal: 'user:demo' },
        },
      },
    });

    // The catalogue: no new part, no new folder, no upload.
    await expect.element(page.getByTestId('viewer')).toBeVisible();
    await expect
      .element(page.getByTestId('cards').getByText('Bike hook'))
      .toBeVisible();
    expect(
      page.getByRole('button', { name: 'New part' }).elements(),
    ).toHaveLength(0);
    expect(page.getByTestId('folder-upload').elements()).toHaveLength(0);
    expect(
      page.getByRole('button', { name: 'New folder' }).elements(),
    ).toHaveLength(0);

    // A folder: no menu to rename, move or delete it.
    await userEvent.click(page.getByTestId('folder-tree').getByText('Garage'));
    await expect
      .element(page.getByTestId('cards').getByText('Bike hook'))
      .toBeVisible();
    expect(
      page.getByRole('button', { name: 'Actions for Garage' }).elements(),
    ).toHaveLength(0);

    // A part: no new version, no menu, no description edit, no picture
    // choice, no comment box and no menu on a comment.
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);
    window.dispatchEvent(new PopStateEvent('popstate'));
    await expect
      .element(page.getByRole('heading', { level: 1, name: 'Bike hook' }))
      .toBeVisible();
    expect(page.getByTestId('new-version').elements()).toHaveLength(0);
    expect(
      page.getByRole('button', { name: 'Actions for Bike hook' }).elements(),
    ).toHaveLength(0);
    expect(page.getByTestId('edit-description').elements()).toHaveLength(0);

    await userEvent.click(page.getByRole('tab', { name: /Files/ }));
    await expect.element(page.getByText('hook.png')).toBeVisible();
    expect(
      page.getByRole('button', { name: /as the part's picture/ }).elements(),
    ).toHaveLength(0);

    await userEvent.click(page.getByRole('tab', { name: /Versions/ }));
    await expect.element(page.getByTestId('version')).toBeVisible();
    expect(
      page.getByRole('button', { name: 'Actions for Version 1' }).elements(),
    ).toHaveLength(0);

    await userEvent.click(page.getByRole('tab', { name: /Comments/ }));
    await expect.element(page.getByText('Brim helps')).toBeVisible();
    expect(page.getByLabelText('Add a comment').elements()).toHaveLength(0);
    expect(page.getByTestId('post-comment').elements()).toHaveLength(0);
    expect(
      page.getByTestId('comment').getByRole('button').elements(),
    ).toHaveLength(0);
  });

  it('offers no delete of a part to a person who may change the catalogue and not its files', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);
    await open({
      allow: ['view_resource', 'operate_resource', 'read_storage'],
      routes: {
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: HOOK, principal: 'user:demo' },
        },
      },
    });

    await userEvent.click(
      page.getByRole('button', { name: 'Actions for Bike hook' }),
    );

    await expect
      .element(page.getByRole('menuitem', { name: /Change name/ }))
      .toBeVisible();
    expect(
      page.getByRole('menuitem', { name: 'Delete the part' }).elements(),
    ).toHaveLength(0);
    expect(page.getByTestId('new-version').elements()).toHaveLength(0);
  });

  it('opens a part from its card and reads it once', async () => {
    const stub = await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: OVERVIEW },
        [`GET ${MOUNT}/parts`]: { status: 200, body: LISTING },
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: HOOK, principal: 'user:demo' },
        },
      },
    });

    await userEvent.click(page.getByTestId('cards').getByText('Bike hook'));

    await expect
      .element(page.getByRole('heading', { level: 1, name: 'Bike hook' }))
      .toBeVisible();
    await expect
      .element(page.getByTestId('description'))
      .toHaveTextContent('Print it in PETG.');
    expect(window.location.search).toBe('?part=5');
    expect(stub.made(`GET ${MOUNT}/parts/5`)).toHaveLength(1);
  });

  it('opens the part a link names, and says when the part is gone', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=8`);

    await open({
      routes: {
        [`GET ${MOUNT}/parts/8`]: refusal(404, 'parts/PART_NOT_FOUND'),
      },
    });

    await expect
      .element(
        page.getByRole('heading', {
          name: 'This part is not in the catalogue',
        }),
      )
      .toBeVisible();
  });

  it('draws a catalogue it may not read by the code of the refusal', async () => {
    await open({
      routes: {
        [`GET ${MOUNT}/overview`]: refusal(403, 'core.auth/ACCESS_DENIED'),
      },
    });

    await expect
      .element(
        page.getByRole('heading', { name: 'The catalogue could not be read' }),
      )
      .toBeVisible();
    await expect
      .element(page.getByRole('code'))
      .toHaveTextContent('core.auth/ACCESS_DENIED');
  });

  it('sends a comment and draws it without reading the part again', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);

    const stub = await open({
      routes: {
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: HOOK, principal: 'user:demo' },
        },
        [`POST ${MOUNT}/parts/5/comments`]: (call) => ({
          status: 201,
          body: {
            comment: {
              v: 1,
              id: 2,
              part: 5,
              body: (call.body as { body: string }).body,
              by: 'user:demo',
              created: '2026-10-05T11:00:00Z',
            },
            principal: 'user:demo',
          },
        }),
      },
    });

    await userEvent.click(page.getByRole('tab', { name: /Comments/ }));
    await expect.element(page.getByText('Brim helps')).toBeVisible();

    await userEvent.fill(
      page.getByLabelText('Add a comment'),
      'Use 0.6 nozzle',
    );
    await userEvent.click(page.getByTestId('post-comment'));

    await expect.element(page.getByText('Use 0.6 nozzle')).toBeVisible();
    expect(stub.made(`POST ${MOUNT}/parts/5/comments`)[0]?.body).toEqual({
      body: 'Use 0.6 nozzle',
    });
    expect(stub.made(`GET ${MOUNT}/parts/5`)).toHaveLength(1);
  });

  it('saves a description against the revision it read, and keeps the text when it is refused', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);

    const stub = await open({
      routes: {
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: HOOK, principal: 'user:demo' },
        },
        [`PATCH ${MOUNT}/parts/5`]: refusal(
          409,
          'parts/CHANGED',
          'part 5 is at revision 3',
        ),
      },
    });

    await userEvent.click(page.getByTestId('edit-description'));
    await userEvent.fill(
      page.getByRole('textbox', { name: 'Description' }),
      'Print it in ASA.',
    );
    await userEvent.click(page.getByTestId('save-description'));

    await expect
      .element(page.getByText('Somebody else changed this first'))
      .toBeVisible();
    await expect
      .element(page.getByRole('textbox', { name: 'Description' }))
      .toHaveValue('Print it in ASA.');
    expect(stub.made(`PATCH ${MOUNT}/parts/5`)[0]?.body).toEqual({
      description: 'Print it in ASA.',
      revision: 2,
    });
  });

  it('offers the comment menu only on a comment the person wrote', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);

    await open({
      routes: {
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: {
            part: {
              ...HOOK,
              comments: [
                ...HOOK.comments,
                {
                  v: 1,
                  id: 2,
                  part: 5,
                  body: 'Mine',
                  by: 'user:demo',
                  created: '2026-10-05T11:00:00Z',
                },
              ],
            },
            principal: 'user:demo',
          },
        },
      },
    });

    await userEvent.click(page.getByRole('tab', { name: /Comments/ }));

    const comments = page.getByTestId('comment');
    await expect.element(comments.nth(1).getByText('Mine')).toBeVisible();
    expect(comments.nth(0).getByRole('button').elements()).toHaveLength(0);
    expect(comments.nth(1).getByRole('button').elements()).toHaveLength(1);
  });

  it('keeps the folder tree behind a toggle on a narrow page', async () => {
    await page.viewport(480, 900);
    try {
      await open();
      const tree = page.getByTestId('folder-tree');

      await expect.element(tree).not.toBeVisible();
      await userEvent.click(page.getByTestId('tree-toggle'));
      await expect.element(tree).toBeVisible();
    } finally {
      await page.viewport(1280, 900);
    }
  });

  it('closes and opens a folder of the tree, and every folder at once', async () => {
    const stub = await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: NESTED },
      },
    });
    const tree = page.getByTestId('folder-tree');

    await expect.element(tree.getByText('Shelves')).toBeVisible();

    const garage = tree.getByRole('button', { name: 'Close Garage' });
    await expect.element(garage).toHaveAttribute('aria-expanded', 'true');
    await userEvent.click(garage);

    await expect
      .element(tree.getByRole('button', { name: 'Open Garage' }))
      .toHaveAttribute('aria-expanded', 'false');
    expect(tree.getByText('Shelves').elements()).toHaveLength(0);

    // The header opens every folder again, and closes them all after.
    await userEvent.click(
      page.getByRole('button', { name: 'Open every folder' }),
    );
    await expect.element(tree.getByText('Shelves')).toBeVisible();
    await userEvent.click(
      page.getByRole('button', { name: 'Close every folder' }),
    );
    expect(tree.getByText('Shelves').elements()).toHaveLength(0);
    // The top stays open, so the first level is still there.
    await expect.element(tree.getByText('Garage')).toBeVisible();

    // Opening and closing reads nothing from the server.
    expect(stub.made(`GET ${MOUNT}/overview`)).toHaveLength(1);
  });

  it('makes a part of a file dropped onto a folder of the tree', async () => {
    const stub = await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: NESTED },
        [`POST ${MOUNT}/parts`]: (call) => ({
          status: 201,
          body: {
            part: {
              ...HOOK,
              id: 7,
              folder: (call.body as { folder: number }).folder,
              name: (call.body as { name: string }).name,
              versions: [],
              comments: [],
              dir: 'parts/7',
            },
            principal: 'user:demo',
          },
        }),
        'PUT /api/v1/storages/local/content': {
          status: 201,
          body: { bytes: 3 },
        },
        [`POST ${MOUNT}/parts/7/versions`]: {
          status: 201,
          body: { part: { ...HOOK, id: 7 }, principal: 'user:demo' },
        },
      },
    });

    const row = page
      .getByTestId('folder-tree')
      .getByTestId('folder-row')
      .filter({ hasText: 'Shelves' });
    await expect.element(row).toBeVisible();

    dropFiles(row.element(), [new File(['G28'], 'bracket.bgcode')]);

    await expect
      .element(page.getByTestId('drop-tray').getByText('bracket'))
      .toBeVisible();
    await expect
      .element(page.getByTestId('drop-job'))
      .toHaveAttribute('data-state', 'done');

    expect(stub.made(`POST ${MOUNT}/parts`)[0]?.body).toEqual({
      name: 'bracket',
      folder: 2,
    });
    const upload = stub.made('PUT /api/v1/storages/local/content')[0];
    expect(new URLSearchParams(upload?.search).get('path')).toMatch(
      /^parts\/7\/[a-z0-9]+\/bracket\.bgcode$/,
    );
    expect(
      (
        stub.made(`POST ${MOUNT}/parts/7/versions`)[0]?.body as {
          files: { path: string }[];
        }
      ).files,
    ).toHaveLength(1);
  });

  it('opens the next version with the files dropped onto a part', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);
    const stub = await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: OVERVIEW },
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: HOOK, principal: 'user:demo' },
        },
        'PUT /api/v1/storages/local/content': {
          status: 201,
          body: { bytes: 3 },
        },
      },
    });

    const view = page.getByTestId('part-view');
    await expect
      .element(page.getByRole('heading', { level: 1, name: 'Bike hook' }))
      .toBeVisible();

    dropFiles(view.element(), [new File(['G28'], 'hook-v2.3mf')]);

    await expect.element(page.getByRole('dialog')).toBeVisible();
    await expect
      .element(page.getByRole('dialog').getByText('hook-v2.3mf'))
      .toBeVisible();
    await expect
      .poll(() => stub.made('PUT /api/v1/storages/local/content').length)
      .toBe(1);
  });

  it('marks a row while files are dragged over it', async () => {
    await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: NESTED },
      },
    });

    const row = page
      .getByTestId('folder-tree')
      .getByTestId('folder-row')
      .filter({ hasText: 'Garage' });
    await expect.element(row).toBeVisible();

    const files = withFiles([new File(['x'], 'a.stl')]);
    row.element().dispatchEvent(drag('dragenter', files));
    await expect.element(row).toHaveAttribute('data-dropping', '');
    row.element().dispatchEvent(drag('dragleave', files));
    await expect.element(row).not.toHaveAttribute('data-dropping');
  });

  it('takes no drop from a person who may only read', async () => {
    const stub = await open({
      allow: ['view_resource', 'read_storage'],
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: NESTED },
      },
    });

    const row = page
      .getByTestId('folder-tree')
      .getByTestId('folder-row')
      .filter({ hasText: 'Garage' });
    await expect.element(row).toBeVisible();

    dropFiles(row.element(), [new File(['x'], 'a.stl')]);

    expect(row.element().hasAttribute('data-dropping')).toBe(false);
    expect(stub.made(`POST ${MOUNT}/parts`)).toHaveLength(0);
  });

  // The module counts its limits in UTF-8 bytes, and a name typed with
  // diacritics is longer in bytes than in characters, so the field holds the
  // bytes while the person types instead of the server refusing on save.
  it('holds a name to the bytes the module takes, and names a tag it would refuse', async () => {
    await open();

    await userEvent.click(
      page.getByRole('button', { name: 'New part' }).first(),
    );
    const dialog = page.getByRole('dialog');
    const name = dialog.getByLabelText('Name');

    await userEvent.fill(name, 'ř'.repeat(70));
    await expect.element(name).toHaveValue('ř'.repeat(60));

    await userEvent.fill(
      dialog.getByLabelText('Tags'),
      `bike ${'ž'.repeat(17)}`,
    );
    await expect
      .element(dialog.getByText(/is too long\. A tag is at most 32 bytes/u))
      .toBeVisible();
    await expect
      .element(dialog.getByRole('button', { name: 'Create' }))
      .toBeDisabled();

    await userEvent.fill(dialog.getByLabelText('Tags'), 'bike petg');
    await expect
      .element(dialog.getByRole('button', { name: 'Create' }))
      .toBeEnabled();
  });

  it('leaves no listener behind when the page is drawn and taken down again', async () => {
    const stub = stubHost({
      routes: { [`GET ${MOUNT}/overview`]: { status: 200, body: NESTED } },
    });
    await setup(stub.host);
    const Page = stub.pages.parts;
    if (Page === undefined)
      throw new Error('the module registered no parts page');

    const live = countWindowListeners();
    try {
      for (let round = 0; round < 3; round += 1) {
        const drawn = await render(Page);
        await expect
          .element(page.getByTestId('folder-tree').getByText('Shelves'))
          .toBeVisible();
        // The tree's rows and the listing each listen while drawn.
        expect(live.count()).toBeGreaterThan(0);
        await drawn.unmount();
      }

      expect(live.count()).toBe(0);
    } finally {
      live.restore();
    }
  });

  it('deletes the files of a new version when the person leaves the part with its dialog open', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);
    const stub = await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: OVERVIEW },
        [`GET ${MOUNT}/parts`]: { status: 200, body: LISTING },
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: HOOK, principal: 'user:demo' },
        },
        'PUT /api/v1/storages/local/content': {
          status: 201,
          body: { bytes: 3 },
        },
        'DELETE /api/v1/storages/local/files': { status: 204 },
      },
    });

    await expect
      .element(page.getByRole('heading', { level: 1, name: 'Bike hook' }))
      .toBeVisible();
    dropFiles(page.getByTestId('part-view').element(), [
      new File(['G28'], 'hook-v2.3mf'),
    ]);
    await expect
      .element(page.getByTestId('upload'))
      .toHaveAttribute('data-state', 'done');
    const sent = new URLSearchParams(
      stub.made('PUT /api/v1/storages/local/content')[0]?.search,
    ).get('path');

    // The back button, with the dialog still open.
    window.history.replaceState(null, '', window.location.pathname);
    window.dispatchEvent(new PopStateEvent('popstate'));

    await expect
      .element(page.getByTestId('cards').getByText('Bike hook'))
      .toBeVisible();
    await expect
      .poll(() =>
        stub
          .made('DELETE /api/v1/storages/local/files')
          .map((call) => new URLSearchParams(call.search).get('path')),
      )
      .toEqual([sent]);
  });

  it('holds the version dialog open while it saves, and deletes nothing it recorded', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);
    let answer: () => void = () => undefined;
    const stub = await open({
      routes: {
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: HOOK, principal: 'user:demo' },
        },
        'PUT /api/v1/storages/local/content': {
          status: 201,
          body: { bytes: 3 },
        },
        'DELETE /api/v1/storages/local/files': { status: 204 },
        [`POST ${MOUNT}/parts/5/versions`]: () =>
          new Promise((resolve) => {
            answer = () => {
              resolve({
                status: 201,
                body: { part: HOOK, principal: 'user:demo' },
              });
            };
          }),
      },
    });

    await expect
      .element(page.getByRole('heading', { level: 1, name: 'Bike hook' }))
      .toBeVisible();
    dropFiles(page.getByTestId('part-view').element(), [
      new File(['G28'], 'hook-v2.3mf'),
    ]);
    await expect
      .element(page.getByTestId('upload'))
      .toHaveAttribute('data-state', 'done');

    await userEvent.click(page.getByTestId('save-version'));
    await expect
      .poll(() => stub.made(`POST ${MOUNT}/parts/5/versions`).length)
      .toBe(1);

    // Escape and Cancel do nothing while the version is recorded.
    await expect.element(page.getByTestId('cancel-version')).toBeDisabled();
    await userEvent.keyboard('{Escape}');
    await expect.element(page.getByRole('dialog')).toBeVisible();

    answer();
    await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
    expect(stub.made('DELETE /api/v1/storages/local/files')).toHaveLength(0);
  });

  it('offers no delete of a folder that holds something, and says why', async () => {
    window.history.replaceState(
      null,
      '',
      `${window.location.pathname}?folder=1`,
    );
    await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: OVERVIEW },
        [`GET ${MOUNT}/parts`]: { status: 200, body: LISTING },
      },
    });

    await userEvent.click(
      page.getByRole('button', { name: 'Actions for Garage' }),
    );

    const item = page.getByTestId('delete-folder');
    await expect.element(item).toHaveAttribute('data-disabled');
    await expect
      .element(item.getByTestId('not-empty'))
      .toHaveTextContent('Only an empty folder can be deleted.');
  });

  it('names the words and the tag of a search that has both', async () => {
    window.history.replaceState(
      null,
      '',
      `${window.location.pathname}?q=hook&tag=bike`,
    );
    const stub = await open({
      routes: {
        [`GET ${MOUNT}/overview`]: { status: 200, body: OVERVIEW },
        [`GET ${MOUNT}/parts`]: { status: 200, body: LISTING },
      },
    });

    await expect
      .element(page.getByText('1 part tagged bike matches “hook”'))
      .toBeVisible();
    const query = new URLSearchParams(
      stub.made(`GET ${MOUNT}/parts`)[0]?.search,
    );
    expect([query.get('q'), query.get('tag')]).toEqual(['hook', 'bike']);
  });

  it('makes parts of files picked in the dropzone, for a person who cannot drag', async () => {
    const stub = await open({
      routes: {
        [`POST ${MOUNT}/parts`]: (call) => ({
          status: 201,
          body: {
            part: {
              ...HOOK,
              id: 7,
              folder: 0,
              name: (call.body as { name: string }).name,
              versions: [],
              comments: [],
              dir: 'parts/7',
            },
            principal: 'user:demo',
          },
        }),
        'PUT /api/v1/storages/local/content': {
          status: 201,
          body: { bytes: 3 },
        },
        [`POST ${MOUNT}/parts/7/versions`]: {
          status: 201,
          body: { part: { ...HOOK, id: 7 }, principal: 'user:demo' },
        },
      },
    });

    const zone = page.getByTestId('folder-upload');
    await expect
      .element(zone.getByRole('button', { name: 'Choose files' }))
      .toBeVisible();

    // The picker is the browser's own and a test cannot drive it, so the
    // files are put on the input the button opens.
    const input = zone.element().querySelector('input[type="file"]');
    if (!(input instanceof HTMLInputElement)) throw new Error('no input');
    input.files = withFiles([new File(['G28'], 'bracket.bgcode')]).files;
    input.dispatchEvent(new Event('change', { bubbles: true }));

    await expect
      .element(page.getByTestId('drop-job'))
      .toHaveAttribute('data-state', 'done');
    expect(stub.made(`POST ${MOUNT}/parts`)[0]?.body).toEqual({
      name: 'bracket',
      folder: 0,
    });
  });

  it('numbers a new version after the highest the part ever had, also one deleted since', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?part=5`);
    await open({
      routes: {
        [`GET ${MOUNT}/parts/5`]: {
          status: 200,
          body: { part: { ...HOOK, last_version: 3 }, principal: 'user:demo' },
        },
      },
    });

    await userEvent.click(page.getByTestId('new-version'));

    await expect
      .element(
        page.getByRole('dialog').getByRole('heading', { name: 'Version 4' }),
      )
      .toBeVisible();
  });
});
