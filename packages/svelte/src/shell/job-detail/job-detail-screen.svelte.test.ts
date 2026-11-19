import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { START_PRINT_VERB } from '../../data/services/print';
import {
  type Device,
  type PrintOutcome,
  type PrintRequest,
  STATE_PATH,
} from '../../domain';
import { expectNoViolations } from '../../test/axe';
import { fakeDevice } from '../../test/fakes';
import JobDetailScreen from './job-detail-screen.svelte';
import type { JobDetailScreenProps, JobRow, JobSource } from './types';

/** The send button, which names how many printers it sends to once one is ticked. */
const PRINT = /^Print( on \d+ printers?)?$/;

const JOB_ID = '0199e3a4-5bb5-7aac-8a2c-9c3982e6373f';

const finished: JobRow = {
  display_name: 'Bracket_Left_Reinforced_v4_FINAL.gcode',
  ended: '2026-08-19T09:10:00Z',
  id: JOB_ID,
  device_job_id: '412',
  device_id: 'printer-1',
  path: '/usb/examples/bracket.bgcode',
  progress: 100,
  started: '2026-08-19T07:02:00Z',
  state: 'FINISHED',
  time_remaining_ms: 0,
};

const printing: JobRow = {
  display_name: 'Camera_Mount_Bracket_v3.gcode',
  id: JOB_ID,
  device_job_id: '412',
  device_id: 'printer-1',
  path: '/usb/examples/camera-mount.bgcode',
  progress: 42,
  started: '2026-08-19T09:12:00Z',
  state: 'PRINTING',
  time_remaining_ms: 2_280_000,
};

/** The printer the print ran on, in the state a test names. */
const printer = (state: string): Device => ({
  ...fakeDevice('printer-1', 'MK4S #1042'),
  capabilities: [START_PRINT_VERB],
  state: { [STATE_PATH]: state },
});

const started: PrintOutcome = {
  commandId: 'cmd-1',
  faultCode: undefined,
  machineReason: undefined,
  ok: true,
  path: finished.path,
  transferId: undefined,
};

const answered = (job?: JobRow): JobSource => ({
  error: undefined,
  job,
  loaded: true,
});

const props = (
  over: Partial<JobDetailScreenProps> = {},
): JobDetailScreenProps => ({
  jobId: JOB_ID,
  source: answered(finished),
  device: fakeDevice('printer-1', 'MK4S #1042'),
  jobsHref: '/jobs',
  printerHref: '/printers/printer-1',
  fileHref: (path: string) => `/storage/local/file/${encodeURIComponent(path)}`,
  ...over,
});

describe('JobDetailScreen', () => {
  it('names the print as the page heading and the trail leads back', async () => {
    await render(JobDetailScreen, { props: props() });

    expect(document.querySelector('h1')?.textContent.trim()).toBe(
      finished.display_name,
    );
    await expect
      .element(page.getByRole('link', { name: 'Jobs' }))
      .toHaveAttribute('href', '/jobs');
  });

  it('draws the state the print ended in, and the printer it ran on', async () => {
    await render(JobDetailScreen, { props: props() });

    await expect.element(page.getByText('Finished')).toBeVisible();
    await expect.element(page.getByText('MK4S #1042')).toBeVisible();
  });

  // The printer had gone back to idle when the print ended, and the outcome
  // says how it ended, the same as the Jobs list reads it.
  it('reads an ended print by its recorded outcome', async () => {
    await render(JobDetailScreen, {
      props: props({
        source: answered({ ...finished, state: 'IDLE', outcome: 'failed' }),
      }),
    });

    await expect
      .element(page.getByText('Error', { exact: true }))
      .toBeVisible();
  });

  // The job record holds no serial or firmware, and the printer's current
  // ones may not be what the print ran on.
  it('names the model the print ran on and no serial or firmware', async () => {
    await render(JobDetailScreen, { props: props() });

    await expect.element(page.getByText('Model')).toBeVisible();
    await expect.element(page.getByText('CORE One')).toBeVisible();
    expect(page.getByText('serial-printer-1').elements()).toHaveLength(0);
    expect(page.getByText('1.0.0', { exact: true }).elements()).toHaveLength(0);
  });

  // The fleet may not hold the printer this print ran on.
  it('draws the origin without the printer where the fleet has none', async () => {
    await render(JobDetailScreen, { props: props({ device: undefined }) });

    await expect.element(page.getByText('Print id')).toBeVisible();
    expect(page.getByText('Model').elements()).toHaveLength(0);
    expect(page.getByText('Not built yet').elements()).toHaveLength(0);
  });

  // A print that ended has a span and no estimate.
  it('shows the span of a print that ended and no estimate', async () => {
    await render(JobDetailScreen, { props: props() });

    await expect.element(page.getByText('Duration')).toBeVisible();
    expect(page.getByText('Time left').elements()).toHaveLength(0);
  });

  it('shows the estimate of a print still running and no end', async () => {
    await render(JobDetailScreen, {
      props: props({ source: answered(printing) }),
    });

    await expect.element(page.getByText('Time left')).toBeVisible();
    expect(page.getByText('Ended').elements()).toHaveLength(0);
    expect(page.getByText('Duration').elements()).toHaveLength(0);
  });

  // `DeviceJob` carries no reason on any code path.
  it('names what a failed print does not say', async () => {
    await render(JobDetailScreen, {
      props: props({ source: answered({ ...finished, state: 'ERROR' }) }),
    });

    await expect.element(page.getByRole('alert')).toBeVisible();
    await expect
      .element(
        page.getByText('This server does not record why a print failed.'),
      )
      .toBeVisible();
  });

  it('puts no alert over a print somebody stopped', async () => {
    await render(JobDetailScreen, {
      props: props({
        source: answered({ ...finished, progress: 18, state: 'STOPPED' }),
      }),
    });

    expect(page.getByRole('alert').elements()).toHaveLength(0);
    await expect.element(page.getByText('18%')).toBeVisible();
  });

  // Every part waits in its own shape.
  it('waits in the shape of the page, not as a grey rectangle', async () => {
    const { container } = await render(JobDetailScreen, {
      props: props({
        source: {
          error: undefined,
          job: undefined,
          loaded: false,
        },
      }),
    });

    expect(document.querySelector('h1')).toBeNull();
    expect(page.getByText('Duration').elements()).toHaveLength(0);
    expect(
      container.querySelectorAll('[data-slot="skeleton"]').length,
    ).toBeGreaterThan(0);
    await expect
      .element(page.getByRole('link', { name: 'Jobs' }))
      .toHaveAttribute('href', '/jobs');
  });

  // The server answered and holds no such print.
  it('names a print the server does not hold', async () => {
    const { container } = await render(JobDetailScreen, {
      props: props({ source: answered() }),
    });

    await expect.element(page.getByText('No such print')).toBeVisible();
    expect(container.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(
      0,
    );
  });

  // A refusal renders by its code, with the server's own line under it.
  it('names a fault the server answered with', async () => {
    const { container } = await render(JobDetailScreen, {
      props: props({
        source: {
          error: {
            code: 'core.device/NOT_FOUND',
            traceId: undefined,
            message: 'no such device',
            params: {},
            status: 404,
          },
          job: undefined,
          loaded: false,
        },
      }),
    });

    expect(container.textContent).toContain('core.device/NOT_FOUND');
    expect(page.getByText('Duration').elements()).toHaveLength(0);
  });

  it('draws the run, the origin and the file in one card', async () => {
    await render(JobDetailScreen, { props: props() });

    await expect.element(page.getByText('Run', { exact: true })).toBeVisible();
    await expect
      .element(page.getByText('Origin', { exact: true }))
      .toBeVisible();
    await expect.element(page.getByText('Path on the printer')).toBeVisible();
    // A strip over three parts that already fit is a control that hides them.
    expect(page.getByRole('tab').elements()).toHaveLength(0);
  });

  // The id the machine issued is the tail of the job's own address.
  it('reads the number the machine issued out of the address', async () => {
    await render(JobDetailScreen, { props: props() });

    await expect.element(page.getByText('Printer job id')).toBeVisible();
    await expect.element(page.getByText('412', { exact: true })).toBeVisible();
  });

  it('leads to the file on the printer where the caller draws one', async () => {
    const { unmount } = await render(JobDetailScreen, { props: props() });
    await expect
      .element(page.getByRole('link', { name: 'Open this file' }))
      .toHaveAttribute(
        'href',
        '/storage/local/file/%2Fusb%2Fexamples%2Fbracket.bgcode',
      );
    await unmount();

    // A host that has no page for this file draws no link to a wrong one.
    await render(JobDetailScreen, {
      props: props({ fileHref: () => undefined }),
    });
    expect(
      page.getByRole('link', { name: 'Open this file' }).elements(),
    ).toHaveLength(0);
  });

  // The printer beside the title is the one way to its page.
  it('leads to the printer the print ran on', async () => {
    await render(JobDetailScreen, { props: props() });

    await expect
      .element(page.getByRole('link', { name: /MK4S #1042/ }))
      .toHaveAttribute('href', '/printers/printer-1');
    expect(
      page.getByRole('link', { name: 'Open printer' }).elements(),
    ).toHaveLength(0);
  });

  it('offers the print again only where the caller can send it', async () => {
    const { unmount } = await render(JobDetailScreen, { props: props() });
    expect(
      page.getByRole('button', { name: 'Print again' }).elements(),
    ).toHaveLength(0);
    await unmount();

    const sent: PrintRequest[] = [];
    await render(JobDetailScreen, {
      props: props({
        device: printer('IDLE'),
        onPrint: (request: PrintRequest) => {
          sent.push(request);

          return Promise.resolve(started);
        },
      }),
    });
    await page.getByRole('button', { name: 'Print again' }).click();
    await page.getByRole('dialog').getByRole('button', { name: PRINT }).click();

    expect(sent).toEqual([
      { deviceId: 'printer-1', path: '/usb/examples/bracket.bgcode' },
    ]);
  });

  // The rules are the print dialog's, so a busy printer is not sent the file.
  it('does not send the print to a printer that is busy', async () => {
    const onPrint = vi.fn();
    await render(JobDetailScreen, {
      props: props({ device: printer('PRINTING'), onPrint }),
    });
    await page.getByRole('button', { name: 'Print again' }).click();

    const dialog = page.getByRole('dialog');
    await expect
      .element(dialog.getByText('This printer is printing.'))
      .toBeVisible();
    await expect
      .element(dialog.getByRole('button', { name: PRINT }))
      .toBeDisabled();
    expect(onPrint).not.toHaveBeenCalled();
  });

  it('offers no Print again on a print that is still running', async () => {
    await render(JobDetailScreen, {
      props: props({
        device: printer('PRINTING'),
        source: answered(printing),
        onPrint: vi.fn(),
      }),
    });

    expect(
      page.getByRole('button', { name: 'Print again' }).elements(),
    ).toHaveLength(0);
  });

  // The stored row is written as the machine reports, so it can lag behind
  // the live reading.
  it('draws the live reading over the stored row while the print runs', async () => {
    const device = {
      ...fakeDevice('printer-1', 'MK4S #1042'),
      job: {
        display_name: printing.display_name,
        id: JOB_ID,
        device_job_id: '412',
        device_id: 'printer-1',
        path: printing.path,
        progress: 77,
        running: true,
        state: 'PRINTING',
        time_remaining_ms: 600_000,
      },
    };
    await render(JobDetailScreen, {
      props: props({ device, source: answered(printing) }),
    });

    await expect.element(page.getByText('77%')).toBeVisible();
    expect(page.getByText('42%').elements()).toHaveLength(0);
  });

  // A finished print is a record and never moves again.
  it('leaves a finished print on its stored figure', async () => {
    const device = {
      ...fakeDevice('printer-1', 'MK4S #1042'),
      job: {
        display_name: finished.display_name,
        id: JOB_ID,
        device_job_id: '412',
        device_id: 'printer-1',
        path: finished.path,
        progress: 3,
        running: false,
        state: 'FINISHED',
        time_remaining_ms: 0,
      },
    };
    await render(JobDetailScreen, {
      props: props({ device, source: answered(finished) }),
    });

    await expect.element(page.getByText('100%')).toBeVisible();
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(JobDetailScreen, {
      props: props({ device: printer('IDLE'), onPrint: vi.fn() }),
    });

    await expectNoViolations(container);
  });
});
