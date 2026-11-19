import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { FileDescription } from '../../data';
import { expectNoViolations } from '../../test/axe';
import FileDetailScreen from './file-detail-screen.svelte';
import type {
  FileDetailEntry,
  FileDetailScreenProps,
  FilePrint,
} from './types';

const entry: FileDetailEntry = {
  name: 'Bracket_Left_Reinforced_v4_FINAL.gcode',
  path: 'q3_rush_jobs/Bracket_Left_Reinforced_v4_FINAL.gcode',
  size: 19_293_798,
  modified: '2026-08-18T14:02:00Z',
  sha256: '9f2c1ab4d5e6f70819a2b3c4d5e6f70819a2b3c4d5e6f70819a2b3c4d5e6f708',
};

const prints: FilePrint[] = [
  {
    id: 'prn-rack-3:1',
    started: '2026-08-19T07:02:00Z',
    state: 'FINISHED',
    printerName: 'MK4S, Rack 3',
    printerModel: 'MK4S',
    href: '/jobs/prn-rack-3:1',
    printerHref: '/printers/prn-rack-3',
    durationSeconds: 7680,
  },
];

const sliced: FileDescription = {
  details: {
    estimated_seconds: 7260,
    filament_mm: 18_420,
    filament_g: 55.2,
    layer_height_mm: 0.2,
    nozzle_mm: 0.4,
    material: 'PLA',
    printer_model: 'MK4S',
  },
  thumbnailUrl:
    'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg"/>',
};

const props = (
  over: Partial<FileDetailScreenProps> = {},
): FileDetailScreenProps => ({
  path: entry.path,
  entry,
  rootLabel: 'Storage',
  rootHref: '/storage/local',
  folderHref: (path) => `/storage/local/folder/${encodeURIComponent(path)}`,
  ...over,
});

describe('FileDetailScreen', () => {
  it('names the file as the page heading and every folder above it', async () => {
    await render(FileDetailScreen, { props: props() });

    expect(document.querySelector('h1')?.textContent.trim()).toBe(entry.name);
    await expect
      .element(page.getByRole('link', { name: 'Storage' }))
      .toHaveAttribute('href', '/storage/local');
    await expect
      .element(page.getByRole('link', { name: 'q3_rush_jobs' }))
      .toHaveAttribute('href', '/storage/local/folder/q3_rush_jobs');
  });

  // The file name is the page's subject and the only breadcrumb on it.
  it('draws one trail and no second one', async () => {
    const { container } = await render(FileDetailScreen, { props: props() });

    expect(container.querySelectorAll('nav')).toHaveLength(1);
  });

  // The size and the date are what a person reads first, so they sit under
  // the name. The rest of what the storage answered is a tab of its own.
  it('draws the size and the date under the name, and the rest on a tab', async () => {
    const { container } = await render(FileDetailScreen, { props: props() });

    expect(container.textContent).not.toContain('19293798');
    await expect.element(page.getByText(/MB · /)).toBeVisible();
    expect(container.textContent).not.toContain('Checksum');

    await page.getByRole('tab', { name: 'Details' }).click();
    await expect.element(page.getByText('Checksum')).toBeVisible();
    await expect
      .element(page.getByText('Storage / q3_rush_jobs'))
      .toBeVisible();
  });

  it('opens the tab the app asks for and reports the one a person picks', async () => {
    const onTabChange = vi.fn();
    await render(FileDetailScreen, {
      props: props({ tab: 'details', onTabChange, prints }),
    });

    await expect.element(page.getByText('Checksum')).toBeVisible();
    await page.getByRole('tab', { name: 'Prints' }).click();
    expect(onTabChange).toHaveBeenCalledWith('prints');
  });

  // A module tab for another file type, or a link to one that was removed.
  it('opens the contents for a tab the page does not have', async () => {
    await render(FileDetailScreen, {
      props: props({ tab: 'com.example.gone.tab', description: sliced }),
    });

    await expect
      .element(page.getByRole('tab', { name: 'Contents' }))
      .toHaveAttribute('aria-selected', 'true');
    await expect
      .element(page.getByRole('heading', { name: 'Slicer settings' }))
      .toBeVisible();
  });

  // ADR-20261029-dgv: a module's tabs follow the page's own.
  it('draws a module tab after its own', async () => {
    const panel = createRawSnippet(() => ({
      render: () => '<p data-testid="module-tab">a module tab</p>',
    }));
    await render(FileDetailScreen, {
      props: props({
        tab: 'com.example.history.runs',
        moduleTabs: [{ id: 'com.example.history.runs', label: 'Runs', panel }],
      }),
    });

    const labels = page
      .getByRole('tab')
      .elements()
      .map((tab) => tab.textContent.trim());
    expect(labels).toEqual(['Contents', 'Details', 'Runs']);
    await expect.element(page.getByTestId('module-tab')).toBeVisible();
  });

  it('drops a fact the storage did not answer', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({
        path: 'benchy.3mf',
        entry: { name: 'benchy.3mf', path: 'benchy.3mf', size: 2_516_582 },
        tab: 'details',
      }),
    });

    await expect.element(page.getByText('Location')).toBeVisible();
    expect(container.textContent).not.toContain('No reading yet');
    expect(container.textContent).not.toContain('Modified');
    expect(container.textContent).not.toContain('Checksum');
  });

  it('offers Print for a file a printer takes as it is', async () => {
    const print = vi.fn();
    await render(FileDetailScreen, { props: props({ acts: { print } }) });

    await page.getByRole('button', { name: 'Print' }).click();
    expect(print).toHaveBeenCalledTimes(1);
  });

  // A model has to go through a slicer first. Offering that is a slicer
  // module's to do, so the page names no slicing of its own.
  it('leaves Print out for a file that has to be sliced first', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({
        path: 'benchy.3mf',
        entry: { name: 'benchy.3mf', path: 'benchy.3mf', size: 2_516_582 },
        acts: { print: () => undefined },
      }),
    });

    expect(container.textContent).not.toContain('slicing');
    expect(page.getByRole('button', { name: 'Print' }).elements()).toHaveLength(
      0,
    );
  });

  it('offers nothing that writes to a printer-managed file', async () => {
    await render(FileDetailScreen, {
      props: props({
        entry: { ...entry, read_only: true },
        acts: {
          downloadHref: '/content',
          remove: () => undefined,
          rename: () => undefined,
        },
      }),
    });

    await expect.element(page.getByText('Read-only')).toBeVisible();
    await page
      .getByRole('button', { name: `Actions for ${entry.name}` })
      .click();

    // The bytes may still be read. Everything that would change them is gone.
    await expect
      .element(page.getByRole('menuitem', { name: 'Download' }))
      .toBeVisible();
    expect(
      page.getByRole('menuitem', { name: 'Rename' }).elements(),
    ).toHaveLength(0);
    expect(
      page.getByRole('menuitem', { name: 'Delete' }).elements(),
    ).toHaveLength(0);
  });

  it('asks before it removes the file', async () => {
    const remove = vi.fn();
    await render(FileDetailScreen, { props: props({ acts: { remove } }) });

    await page
      .getByRole('button', { name: `Actions for ${entry.name}` })
      .click();
    await page.getByRole('menuitem', { name: 'Delete' }).click();
    expect(remove).not.toHaveBeenCalled();

    await page.getByRole('button', { name: 'Delete' }).click();
    expect(remove).toHaveBeenCalledTimes(1);
  });

  it('takes a new name from the rename panel', async () => {
    const rename = vi.fn();
    await render(FileDetailScreen, { props: props({ acts: { rename } }) });

    await page
      .getByRole('button', { name: `Actions for ${entry.name}` })
      .click();
    await page.getByRole('menuitem', { name: 'Rename' }).click();
    await page.getByRole('textbox', { name: 'New name' }).fill('brace.gcode');
    await page.getByRole('button', { name: 'Rename' }).click();

    expect(rename).toHaveBeenCalledWith('brace.gcode');
  });

  // A job records the name a file had and never its path.
  it('lists every print that recorded this name, and says what that means', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({ prints, tab: 'prints' }),
    });

    await expect
      .element(page.getByRole('link', { name: /2026/ }).first())
      .toHaveAttribute('href', '/jobs/prn-rack-3:1');
    // The machine's artwork names itself inside the link.
    await expect
      .element(page.getByRole('link', { name: /MK4S, Rack 3$/ }))
      .toHaveAttribute('href', '/printers/prn-rack-3');
    await expect.element(page.getByText('Finished')).toBeVisible();
    expect(container.textContent).toContain('Matched by file name.');
  });

  // The printer was back at idle when the print ended; the outcome says how.
  it('reads an ended print by its recorded outcome', async () => {
    const ended: FilePrint = {
      ...prints[0]!,
      state: 'IDLE',
      ended: '2026-08-19T09:10:00Z',
      outcome: 'failed',
      // Its own field says it ended, not the span beside it.
      durationSeconds: undefined,
    };
    await render(FileDetailScreen, {
      props: props({ prints: [ended], tab: 'prints' }),
    });

    await expect
      .element(page.getByText('Error', { exact: true }))
      .toBeVisible();
  });

  // Answered and empty is not the same fact as never asked.
  it('tells a file nothing has printed from a query nothing answers', async () => {
    await render(FileDetailScreen, {
      props: props({ prints: [], tab: 'prints' }),
    });

    await expect
      .element(page.getByText('No prints under this name'))
      .toBeVisible();
  });

  // An app that lists no prints gets no tab that says it lists none.
  it('leaves the Prints tab out where the app lists no prints', async () => {
    await render(FileDetailScreen, { props: props() });

    await expect
      .element(page.getByRole('tab', { name: 'Contents' }))
      .toBeVisible();
    expect(page.getByRole('tab', { name: 'Prints' }).query()).toBeNull();
  });

  it('lists what the slicer wrote, and drops what it did not', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({
        description: { ...sliced, details: { material: 'PETG' } },
      }),
    });

    expect(container.textContent).toContain('PETG');
    expect(container.textContent).not.toContain('Estimated time');
    expect(container.textContent).not.toContain('Nozzle');
  });

  it('shows the plate the file carries', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({ description: sliced }),
    });

    expect(container.textContent).toContain('Estimated time');
    expect(container.textContent).toContain('MK4S');
    // The picture is asked for once the box is in view and a slot is free.
    await expect.poll(() => container.querySelector('img')).not.toBeNull();
  });

  // A file the server opened and found nothing in.
  it('says so when the file carries nothing', async () => {
    await render(FileDetailScreen, {
      props: props({
        description: { details: undefined, thumbnailUrl: undefined },
      }),
    });

    await expect
      .element(page.getByText('Nothing inside to read'))
      .toBeVisible();
  });

  // The card waits in the shape of what it will hold.
  it('draws a skeleton for every part while the listing is on its way', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({ entry: undefined, loading: true }),
    });

    expect(document.querySelector('h1')).toBeNull();
    expect(
      container.querySelectorAll('[data-slot="skeleton"]').length,
    ).toBeGreaterThan(0);
    expect(container.textContent).not.toContain('Checksum');
    await expect
      .element(page.getByRole('link', { name: 'Storage' }))
      .toHaveAttribute('href', '/storage/local');
  });

  // The listing answered and holds nothing at this path.
  it('names a file the server does not hold', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({ entry: undefined }),
    });

    await expect.element(page.getByText('No such file')).toBeVisible();
    expect(container.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(
      0,
    );
    expect(container.textContent).not.toContain('Checksum');
  });

  it('names a fault the server answered with', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({
        fault: { code: 'core.storage/NOT_FOUND', message: 'no such path' },
      }),
    });

    expect(container.textContent).toContain('core.storage/NOT_FOUND');
  });

  // ADR-20261027-wvj rule 16: a module's view takes the place of what the
  // file holds, and the rest of the page stays the screen's.
  it('draws a view in place of the contents and keeps the rest', async () => {
    const contents = createRawSnippet(() => ({
      render: () => '<p data-testid="module-view">a module view</p>',
    }));

    await render(FileDetailScreen, {
      props: props({ contents, acts: { print: () => undefined }, prints }),
    });

    await expect.element(page.getByTestId('module-view')).toBeVisible();
    await expect
      .element(page.getByRole('heading', { name: 'Slicer settings' }))
      .not.toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Print' }))
      .toBeVisible();
    await expect
      .element(page.getByRole('tab', { name: 'Details' }))
      .toBeVisible();
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(FileDetailScreen, {
      props: props({
        acts: {
          print: () => undefined,
          downloadHref: '/content',
          remove: () => undefined,
          rename: () => undefined,
        },
        prints,
      }),
    });

    await expectNoViolations(container);
  });
});
