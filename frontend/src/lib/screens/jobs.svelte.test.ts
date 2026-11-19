import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { appContext } from '../../test/app-context';
import { json, type Routes } from '../../test/stub';
import JobsPage from './jobs.svelte';

const deviceOf = (id: string) => ({
  id,
  name: `Printer ${id}`,
  kind: 'printer',
  labels: {},
  state: { status: 'IDLE' },
});

const jobOf = (id: string, name: string) => ({
  id: `${id}-1`,
  device_id: id,
  display_name: name,
  path: `/usb/${name}`,
  progress: 100,
  started: '2026-09-01T08:00:00Z',
  ended: '2026-09-01T09:00:00Z',
  state: 'FINISHED',
  time_remaining_ms: 0,
});

// The fleet's prints as the core pages them: none running, one ended print per
// printer on the first page, and `cursor` naming a page after every ended
// page, which is empty.
const served = (ids: readonly string[], cursor = ''): Routes => ({
  'GET /api/v1/devices': json(200, { items: ids.map(deviceOf) }),
  'GET /api/v1/jobs': (call) => {
    const query = new URLSearchParams(call.query);
    const device = query.get('device_id');
    const items =
      query.get('phase') === 'running' || query.has('cursor')
        ? []
        : ids
            .filter((id) => device === null || id === device)
            .map((id) => jobOf(id, `part-${id}.bgcode`));

    return json(200, {
      items,
      cursor: query.get('phase') === 'running' ? '' : cursor,
    });
  },
});

describe('the jobs page', () => {
  it('lists a print from every printer', async () => {
    await render(JobsPage, { props: { ctx: appContext(served(['a', 'b'])) } });

    await expect
      .element(page.getByRole('button', { name: 'Actions for part-a.bgcode' }))
      .toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Actions for part-b.bgcode' }))
      .toBeInTheDocument();
  });

  // The page reads a bounded number of pages, so it says so rather than
  // letting a person believe they are looking at every print.
  it('says the list is cut when more prints are left', async () => {
    await render(JobsPage, {
      props: { ctx: appContext(served(['a'], 'more')) },
    });

    await expect
      .element(page.getByText('Showing the newest', { exact: false }))
      .toBeInTheDocument();
  });

  it('says nothing about a cut when every print was read', async () => {
    await render(JobsPage, { props: { ctx: appContext(served(['a'])) } });

    await expect
      .element(page.getByRole('button', { name: 'Actions for part-a.bgcode' }))
      .toBeInTheDocument();
    await expect
      .element(page.getByText('Showing the newest', { exact: false }))
      .not.toBeInTheDocument();
  });

  // Print again goes through the print dialog and the print route, so the
  // printer's state is checked and the answer is read.
  it('prints a finished job again through the print dialog', async () => {
    const bodies: unknown[] = [];
    const ctx = appContext({
      ...served(['a']),
      'GET /api/v1/devices': json(200, {
        items: [
          {
            ...deviceOf('a'),
            capabilities: ['core.command.start_print'],
            connected: true,
            state: { '/state': 'IDLE' },
          },
        ],
      }),
      'POST /api/v1/devices/a/print': (call) => {
        bodies.push(JSON.parse(call.body ?? 'null'));

        return json(200, {
          command: { command_id: 'cmd-1', ok: true },
          path: '/usb/part-a.bgcode',
        });
      },
    });
    const went: string[] = [];
    ctx.go = (path) => {
      went.push(path);
    };

    await render(JobsPage, { props: { ctx } });

    await page
      .getByRole('button', { name: 'Actions for part-a.bgcode' })
      .click();
    await page.getByRole('menuitem', { name: 'Print again' }).click();
    await page
      .getByRole('dialog')
      .getByRole('button', { name: /^Print( on \d+ printers?)?$/ })
      .click();

    await expect.poll(() => bodies).toEqual([{ path: '/usb/part-a.bgcode' }]);
    await expect.poll(() => went).toEqual(['/printers/a']);
  });

  // The printer page links here with its printer in the address.
  it('lists the prints of the printer the address names', async () => {
    const ctx = appContext(served(['a', 'b']), { params: { id: 'b' } });

    await render(JobsPage, { props: { ctx } });

    await expect
      .element(page.getByRole('button', { name: 'Actions for part-b.bgcode' }))
      .toBeInTheDocument();
    expect(
      page
        .getByRole('button', { name: 'Actions for part-a.bgcode' })
        .elements(),
    ).toHaveLength(0);
    await expect
      .element(page.getByRole('link', { name: 'Every printer' }))
      .toHaveAttribute('href', '/jobs');
  });
});
