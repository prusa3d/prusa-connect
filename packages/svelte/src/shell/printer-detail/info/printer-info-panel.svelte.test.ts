import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { FEATURES } from '../../../domain';
import { expectNoViolations } from '../../../test/axe';
import { fakeDevice } from '../../../test/fakes';
import PrinterInfoPanel from './printer-info-panel.svelte';
import type { PrinterAdministration, PrinterHistorySource } from './types';

const unread: PrinterHistorySource = {
  jobs: undefined,
  jobsError: undefined,
  jobsLoaded: false,
};

const printed: PrinterHistorySource = {
  jobs: {
    last: {
      display_name: 'benchy.bgcode',
      ended: '2026-09-16T11:20:00Z',
      id: '3fe9fea7-d1bc-4cd7-8eca-e8b8db1aa7a3',
      device_id: 'printer-1',
      device_job_id: '41',
      path: '/usb/benchy.bgcode',
      progress: 100,
      started: '2026-09-16T10:20:00Z',
      state: 'FINISHED',
      time_remaining_ms: 0,
    },
  },
  jobsError: undefined,
  jobsLoaded: true,
};

const never: PrinterHistorySource = {
  jobs: {},
  jobsError: undefined,
  jobsLoaded: true,
};

const refused: PrinterHistorySource = {
  jobs: undefined,
  jobsError: Object.assign(new Error('the provider is not configured'), {
    code: 'core.jobs/unavailable',
  }),
  jobsLoaded: false,
};

const device = {
  ...fakeDevice('printer-1', 'MK4S #1042'),
  actions: ['observe', 'operate', 'configure', 'administer'],
};

/** A store that answers every act at once, and records what it was sent. */
const administration = (
  over: Partial<PrinterAdministration> = {},
): PrinterAdministration & { names: string[]; retired: number } => {
  const sent = { names: [] as string[], retired: 0 };

  return Object.assign(sent, {
    pending: () => false,
    error: undefined,
    failed: undefined,
    tags: (record: readonly string[]) => record,
    editTags: () => Promise.resolve(true),
    rename: (name: string) => {
      sent.names.push(name);

      return Promise.resolve(true);
    },
    retire: () => {
      sent.retired += 1;

      return Promise.resolve(true);
    },
    ...over,
  });
};

describe('PrinterInfoPanel', () => {
  it('draws the record and copies the values a person compares', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: { device, history: printed },
    });

    await expect.element(page.getByText('Prusa CORE One')).toBeVisible();
    await expect.element(page.getByText('serial-printer-1')).toBeVisible();
    await expect.element(page.getByText('unit-printer-1')).toBeVisible();
    await expect.element(page.getByText('1.0.0')).toBeVisible();

    // Serial, unit id, firmware and the device id each earn a copy.
    expect(
      container.querySelectorAll('button[aria-label^="Copy "]'),
    ).toHaveLength(4);
  });

  // `counted: false` is the driver declining to measure.
  it('leaves every link reading absent when the driver counts nothing', async () => {
    const uncounted = {
      ...device,
      link: { ...device.link, counted: false },
    };
    const { container } = await render(PrinterInfoPanel, {
      props: { device: uncounted, history: printed },
    });

    const absent = [...container.querySelectorAll('dd')].filter((cell) =>
      cell.textContent.includes('No reading yet'),
    );
    expect(absent.length).toBeGreaterThanOrEqual(5);
    await expect.element(page.getByText('Open')).toBeVisible();
  });

  it('says nothing special about a printer that reported no firmware', async () => {
    const quiet = { ...device, firmware: '' };
    const { container } = await render(PrinterInfoPanel, {
      props: { device: quiet, history: printed },
    });

    expect(container.textContent).not.toContain('1.0.0');
    // The telemetry rows draw the same fallback.
    expect(container.textContent).toContain('No reading yet');
  });

  it('draws the prints skeleton before the read answers', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: { device, history: unread },
    });

    expect(
      container.querySelector('[aria-label="Reading the prints"]'),
    ).not.toBeNull();
    expect(container.textContent).not.toContain('No prints yet');
  });

  // A printer that has never printed is not a printer whose prints failed to.
  it('tells a printer that never printed from one still being read', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: { device, history: never },
    });

    await expect.element(page.getByText('No prints yet')).toBeVisible();
    expect(
      container.querySelector('[aria-label="Reading the prints"]'),
    ).toBeNull();
  });

  // The third answer: the registry code beside the server's own English.
  it('renders a refused history through describeFault', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: { device, history: refused },
    });

    expect(container.textContent).toContain('core.jobs/unavailable');
    expect(container.textContent).toContain('the provider is not configured');
  });

  it('names a model it does not know as the printer spells it', async () => {
    await render(PrinterInfoPanel, {
      props: { device: { ...device, model: 'ZX-42' }, history: printed },
    });

    await expect.element(page.getByText('ZX-42')).toBeVisible();
  });

  it('waits in its own shape while the record is on its way', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: { device: undefined, history: unread },
    });

    expect(
      container.querySelector('[aria-label="Reading this printer"]'),
    ).not.toBeNull();
    expect(container.textContent).not.toContain('CORE One');
  });

  it('names an issue in words as well as in colour', async () => {
    const troubled = {
      ...device,
      issues: [
        {
          detail: 'The filament sensor saw nothing.',
          reason: 'filament_missing',
          severity: 'warning' as const,
        },
      ],
    };
    const { container } = await render(PrinterInfoPanel, {
      props: { device: troubled, history: printed },
    });

    await expect.element(page.getByText('Warning')).toBeVisible();
    await expect.element(page.getByText('filament_missing')).toBeVisible();
    await expectNoViolations(container);
  });

  // The print on the bed and the page of ended prints are two reads side by
  // side. A print that ended between them is in both, and is listed once.
  it('lists a print that ended between the two reads once', async () => {
    const hinge = {
      display_name: 'Hinge_v2.bgcode',
      id: '0a1b2c3d-0000-7000-8000-000000000014',
      device_id: 'printer-1',
      device_job_id: '14',
      path: '/usb/Hinge_v2.bgcode',
      progress: 99,
      started: '2026-08-19T18:31:00Z',
      state: 'PRINTING',
      time_remaining_ms: 1000,
    };
    const { container } = await render(PrinterInfoPanel, {
      props: {
        device,
        history: {
          jobs: { current: hinge },
          jobsError: undefined,
          jobsLoaded: true,
        },
        recent: [
          {
            ...hinge,
            progress: 100,
            state: 'FINISHED',
            ended: '2026-08-19T18:35:00Z',
          },
        ],
      },
    });

    await expect
      .element(page.getByRole('link', { name: 'Hinge_v2.bgcode' }))
      .toHaveAttribute('href', `/jobs/${hinge.id}`);
    expect(container.textContent.split('Hinge_v2.bgcode')).toHaveLength(2);
    expect(container.textContent).toContain('Finished');
  });

  // The stored row of a running print is the reading taken when the page read
  // it; the fleet row follows the machine.
  it('shows the print on the bed at the progress the fleet row carries', async () => {
    const onBed = {
      display_name: 'Hinge_v2.bgcode',
      id: '0a1b2c3d-0000-7000-8000-000000000015',
      device_id: 'printer-1',
      device_job_id: '15',
      path: '/usb/Hinge_v2.bgcode',
      progress: 4,
      started: '2026-08-19T18:31:00Z',
      state: 'PRINTING',
      time_remaining_ms: 1000,
    };
    const { container } = await render(PrinterInfoPanel, {
      props: {
        device: {
          ...device,
          job: { ...onBed, progress: 64, running: true },
        },
        history: {
          jobs: { current: onBed },
          jobsError: undefined,
          jobsLoaded: true,
        },
      },
    });

    await expect.element(page.getByText('Hinge_v2.bgcode')).toBeVisible();
    expect(container.textContent).toContain('64%');
    expect(container.textContent).not.toMatch(/(^|[^0-9])4%/);
  });

  // The page of ended prints is a read of its own.
  it('lists the paged prints in place of the single last one', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: {
        device,
        history: printed,
        recent: [
          {
            display_name: 'spool-holder.bgcode',
            ended: '2026-09-15T09:48:00Z',
            id: 'f4730659-f514-4b81-8e7b-f17c8607fee9',
            device_id: 'printer-1',
            device_job_id: '39',
            path: '/usb/spool-holder.bgcode',
            progress: 100,
            started: '2026-09-15T08:05:00Z',
            state: 'FINISHED',
            time_remaining_ms: 0,
          },
        ],
      },
    });

    await expect.element(page.getByText('spool-holder.bgcode')).toBeVisible();
    expect(container.textContent).not.toContain('benchy.bgcode');
  });

  it('takes a chip to the Advanced tab rather than running the command', async () => {
    const picked: string[] = [];
    const accepting = {
      ...device,
      capabilities: [
        'core.command.pause_print',
        // An argument of the verb above, on the same run of strings.
        'core.command.pause_print#id',
        // The console owns this one, so the Advanced tab does not list it.
        'core.command.send_gcode',
      ],
    };

    const { container } = await render(PrinterInfoPanel, {
      props: {
        device: accepting,
        history: printed,
        onPickCommand: (verb: string) => {
          picked.push(verb);
        },
      },
    });

    await expect.element(page.getByText('2 commands')).toBeVisible();
    // Only the verb that tab offers is a control.
    expect(
      container.querySelectorAll('[data-slot="badge"][type="button"]'),
    ).toHaveLength(1);

    await page.getByRole('button', { name: 'Find the Pause command' }).click();
    expect(picked).toEqual(['core.command.pause_print']);
  });

  // A control that cannot do anything is worse than no control.
  it('draws no Danger zone where the host offers no removal', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: { device, history: printed },
    });

    expect(container.textContent).not.toContain('Danger zone');
  });

  // The dialog opens on the name the printer carries, and trailing space is
  // invisible, so what the core is sent is what a reader sees.
  it('renames a printer from the name it already carries', async () => {
    const store = administration();

    await render(PrinterInfoPanel, {
      props: { device, history: printed, administration: store },
    });

    await page.getByRole('button', { name: 'Rename printer' }).click();

    const field = page.getByRole('textbox', { name: 'Name' });
    await expect.element(field).toHaveValue('MK4S #1042');

    await field.fill('  Workshop bench  ');
    await page.getByRole('button', { name: 'Rename', exact: true }).click();

    expect(store.names).toEqual(['Workshop bench']);
  });

  // The server takes an empty name as a request to show the model and the
  // serial again.
  it('sends an empty name, which takes the operator name off', async () => {
    const store = administration();

    await render(PrinterInfoPanel, {
      props: { device, history: printed, administration: store },
    });

    await page.getByRole('button', { name: 'Rename printer' }).click();
    await page.getByRole('textbox', { name: 'Name' }).fill('');
    await page.getByRole('button', { name: 'Rename', exact: true }).click();

    expect(store.names).toEqual(['']);
  });

  // The dialog waits for the answer, and it cannot be sent twice.
  it('holds the rename dialog shut to a second press while it waits', async () => {
    await render(PrinterInfoPanel, {
      props: {
        device,
        history: printed,
        administration: administration({ pending: (act) => act === 'rename' }),
      },
    });

    await page.getByRole('button', { name: 'Rename printer' }).click();
    await expect
      .element(page.getByRole('button', { name: 'Rename', exact: true }))
      .toBeDisabled();
  });

  // A rename on its way cannot be abandoned, or its answer would land on a
  // dialog nobody is looking at.
  it('keeps the rename dialog open while the rename is on its way', async () => {
    let pending = $state(false);
    await render(PrinterInfoPanel, {
      props: {
        device,
        history: printed,
        administration: administration({
          pending: (act) => act === 'rename' && pending,
          rename: () => {
            pending = true;

            return new Promise<boolean>(() => undefined);
          },
        }),
      },
    });

    await page.getByRole('button', { name: 'Rename printer' }).click();
    await page.getByRole('button', { name: 'Rename', exact: true }).click();
    await userEvent.keyboard('{Escape}');

    await expect.element(page.getByRole('dialog')).toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Cancel' }))
      .toBeDisabled();
  });

  it('draws a refused rename by its code on the Details card', async () => {
    await render(PrinterInfoPanel, {
      props: {
        device,
        history: printed,
        administration: administration({
          failed: 'rename',
          error: {
            code: 'core.authz/FORBIDDEN',
            traceId: undefined,
            status: 403,
            params: {},
            message: 'the caller may not administer this device',
          },
        }),
      },
    });

    await expect.element(page.getByText('core.authz/FORBIDDEN')).toBeVisible();
    await expect
      .element(page.getByText('the caller may not administer this device'))
      .toBeVisible();
  });

  // ADR-20260930-rq4.
  it.skipIf(!FEATURES.tags)(
    'draws the tags the printer carries on the Details card',
    async () => {
      await render(PrinterInfoPanel, {
        props: {
          device: { ...device, tags: ['workshop', 'night shift'] },
          history: printed,
          administration: administration(),
        },
      });

      await expect.element(page.getByText('night shift')).toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Remove the tag workshop' }))
        .toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Add a tag' }))
        .toBeVisible();
    },
  );

  it('draws a refused tag write by its code on the Details card', async () => {
    await render(PrinterInfoPanel, {
      props: {
        device,
        history: printed,
        administration: administration({
          failed: 'tags',
          error: {
            code: 'core.validation/TAG_TOO_LONG',
            traceId: undefined,
            status: 422,
            params: { limit_characters: 64, location: 'body.tags[0]' },
            message: 'tag is longer than 64 characters',
          },
        }),
      },
    });

    await expect
      .element(page.getByText('core.validation/TAG_TOO_LONG'))
      .toBeVisible();
    await expect
      .element(page.getByText('A tag is longer than this server accepts'))
      .toBeVisible();
  });

  // The tags are read by anyone who may read the printer. Only a caller who
  // may administer it is offered a control that changes them.
  // ADR-20260930-rq4.
  it.skipIf(!FEATURES.tags)(
    'offers no tag control to a caller who may not administer',
    async () => {
      await render(PrinterInfoPanel, {
        props: {
          device: {
            ...device,
            actions: ['observe', 'operate'],
            tags: ['workshop'],
          },
          history: printed,
          administration: administration(),
        },
      });

      await expect.element(page.getByText('workshop')).toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Add a tag' }))
        .not.toBeInTheDocument();
      await expect
        .element(page.getByRole('button', { name: 'Remove the tag workshop' }))
        .not.toBeInTheDocument();
    },
  );

  // The record says what the caller may do, and a control the server would
  // refuse is not drawn.
  it('offers no rename and no removal to a caller who may not administer', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: {
        device: { ...device, actions: ['observe', 'operate'] },
        history: printed,
        administration: administration(),
      },
    });

    await expect
      .element(page.getByRole('button', { name: 'Rename printer' }))
      .not.toBeInTheDocument();
    expect(container.textContent).not.toContain('Danger zone');
  });

  // The server always sends a list, so a record without one says nothing is
  // allowed.
  it('offers no rename and no removal where the record carries no action list', async () => {
    const { container } = await render(PrinterInfoPanel, {
      props: {
        device: { ...device, actions: null },
        history: printed,
        administration: administration(),
      },
    });

    await expect
      .element(page.getByRole('button', { name: 'Rename printer' }))
      .not.toBeInTheDocument();
    expect(container.textContent).not.toContain('Danger zone');
  });

  // Retiring a printer that runs this server would bring it back at once under
  // a new id, so it keeps its rename, and the Danger zone says why it offers
  // no removal.
  it('says why a printer that runs this server cannot be removed', async () => {
    await render(PrinterInfoPanel, {
      props: {
        device: { ...device, resident: true },
        history: printed,
        administration: administration(),
      },
    });

    await expect
      .element(page.getByRole('button', { name: 'Rename printer' }))
      .toBeVisible();
    await expect
      .element(page.getByRole('heading', { name: 'Danger zone' }))
      .toBeVisible();
    await expect
      .element(
        page.getByText(
          'MK4S #1042 runs this server, so it cannot be removed from it.',
        ),
      )
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Remove this printer' }))
      .not.toBeInTheDocument();
  });

  it('moves on only once the server has retired the printer', async () => {
    const store = administration();
    let removed = 0;

    await render(PrinterInfoPanel, {
      props: {
        device,
        history: printed,
        administration: store,
        onRemoved: () => {
          removed += 1;
        },
      },
    });

    await page.getByRole('button', { name: 'Remove this printer' }).click();
    await page.getByRole('button', { name: 'Remove', exact: true }).click();

    await vi.waitFor(() => {
      expect(removed).toBe(1);
    });
    expect(store.retired).toBe(1);
  });

  it('stays and says why when the server refuses the removal', async () => {
    let removed = 0;

    await render(PrinterInfoPanel, {
      props: {
        device,
        history: printed,
        administration: administration({
          retire: () => Promise.resolve(false),
          failed: 'retire',
          error: {
            code: 'core.authz/FORBIDDEN',
            traceId: undefined,
            status: 403,
            params: {},
            message: undefined,
          },
        }),
        onRemoved: () => {
          removed += 1;
        },
      },
    });

    await expect.element(page.getByText('core.authz/FORBIDDEN')).toBeVisible();
    expect(removed).toBe(0);
  });

  // The core counts characters, not bytes, and refuses a longer name. The
  // dialog says so before the request, and its confirm cannot be pressed.
  it('blocks a name past ninety-six characters', async () => {
    await render(PrinterInfoPanel, {
      props: { device, history: printed, administration: administration() },
    });

    await page.getByRole('button', { name: 'Rename printer' }).click();
    await page.getByRole('textbox', { name: 'Name' }).fill('é'.repeat(97));

    await expect
      .element(page.getByText('A name is at most 96 characters.'))
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Rename', exact: true }))
      .toBeDisabled();
  });
});
