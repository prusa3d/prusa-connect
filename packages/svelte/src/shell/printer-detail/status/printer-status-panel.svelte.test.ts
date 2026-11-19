import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { capabilityFor, LIFECYCLE_VERBS, type Device } from '../../../domain';
import { formatDate, formatDuration } from '../../../i18n';
import { expectNoViolations } from '../../../test/axe';
import { fakeDevice } from '../../../test/fakes';
import type { CameraFeed } from '../../printer-cameras';
import type { PrinterHistorySource } from '../info/types';
import type { TelemetrySource } from '../types';
import PrinterStatusPanel from './printer-status-panel.svelte';

const FEED: CameraFeed = {
  camera: {
    name: 'bed-cam',
    type: 'push',
    device_id: 'p0',
    formats: ['jpeg', 'mjpeg'],
    serving: true,
    snapshot: true,
    stream: true,
    watch: false,
  },
  // A picture that loads, because a still is drawn only once it has.
  pictures: {
    still:
      'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7',
    // A stream that fails to load stops the live view, so this one loads.
    live: 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7#live',
  },
};
import type { PrinterStatusPanelProps } from './types';

const NOW = new Date('2026-08-19T19:09:00Z');

/** The 31 minutes the record below says are left, as the runner's own clock writes them. */
const REMAINING_MS = 1_860_000;
// Through the product's own formatter rather than `Intl` directly.
const FINISH = formatDate(new Date(NOW.getTime() + REMAINING_MS), {
  timeStyle: 'short',
});

const silent: TelemetrySource = {
  value: () => undefined,
  points: () => [],
};

const reading = (values: Record<string, number | string>): TelemetrySource => ({
  value: (path) => values[path],
  points: () => [],
});

const device = (state: string, over: Partial<Device> = {}): Device => ({
  ...fakeDevice('p0', 'MK4S #1042'),
  capabilities: LIFECYCLE_VERBS.map(capabilityFor),
  state: { '/state': state },
  job: {
    display_name: 'Enclosure_Panel_Left.gcode',
    id: '65d10bad-b890-4dc6-8fd2-c1cf626f8ea2',

    device_job_id: '14',
    path: 'usb/Enclosure_Panel_Left.gcode',
    progress: 64,
    running: true,
    state: 'PRINTING',
    time_remaining_ms: REMAINING_MS,
  },
  ...over,
});

/** A printer with nothing on the bed, so only the history block offers a print. */
const idle = (): Device => ({
  ...fakeDevice('p0', 'MK4S #1042'),
  capabilities: LIFECYCLE_VERBS.map(capabilityFor),
  state: { '/state': 'IDLE' },
});

const PRINT_PATH = 'usb/Camera_Mount_Bracket_v3.bgcode';

/** The 3h21m between the two stamps below, which the row reports as its span. */
const PRINT_SECONDS = 12_060;

const history: PrinterHistorySource = {
  jobs: {
    last: {
      display_name: 'Camera_Mount_Bracket_v3.bgcode',
      ended: '2026-08-19T16:52:00Z',
      id: 'c7f4008e-c4d3-4f78-8888-0a6067f93b7f',
      device_id: 'p0',
      device_job_id: '13',
      path: PRINT_PATH,
      progress: 100,
      started: '2026-08-19T13:31:00Z',
      state: 'FINISHED',
      time_remaining_ms: 0,
    },
  },
  jobsError: undefined,
  jobsLoaded: true,
};

/** A native drop carrying real files. */
const drop = (zone: Element, files: File[]): void => {
  const dataTransfer = new DataTransfer();
  for (const file of files) dataTransfer.items.add(file);
  zone.dispatchEvent(
    new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer }),
  );
};

const zoneIn = (container: HTMLElement): Element | null =>
  container.querySelector('[data-slot="file-dropzone"]');

const props = (
  over: Partial<PrinterStatusPanelProps> = {},
): PrinterStatusPanelProps => ({
  device: device('PRINTING'),
  telemetry: silent,
  commands: { onCommand: () => undefined },
  now: NOW,
  ...over,
});

describe('PrinterStatusPanel', () => {
  // The finish time is counted from the prop.
  it('answers when the print finishes, from the moment it was handed', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props(),
    });

    await expect
      .element(page.getByText('Enclosure_Panel_Left.gcode'))
      .toBeVisible();
    expect(container.textContent).toContain('64%');
    expect(container.textContent).toContain(FINISH);
  });

  // A page opened before the print started counted the finish from the moment
  // it was opened, so it read 20 minutes, or two hours, early (CL-228).
  it('counts the finish from when the time left arrived, not from when the page opened', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    try {
      vi.setSystemTime(NOW);
      const { container, rerender } = await render(PrinterStatusPanel, {
        props: props({ device: idle(), now: undefined }),
      });

      const started = new Date(NOW.getTime() + 2 * 60 * 60 * 1000);
      vi.setSystemTime(started);
      await rerender({ device: device('PRINTING') });

      await expect
        .element(page.getByText('Enclosure_Panel_Left.gcode'))
        .toBeVisible();
      expect(container.textContent).toContain(
        formatDate(new Date(started.getTime() + REMAINING_MS), {
          timeStyle: 'short',
        }),
      );
    } finally {
      vi.useRealTimers();
    }
  });

  // The three things a person who has just started a print comes here for.
  it('tracks the print it was handed: the name, the figure and what is left', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props(),
    });

    await expect
      .element(page.getByText('Enclosure_Panel_Left.gcode'))
      .toBeVisible();
    await expect
      .element(
        page.getByRole('progressbar', {
          name: 'Enclosure_Panel_Left.gcode progress',
        }),
      )
      .toHaveAttribute('aria-valuenow', '64');
    expect(container.textContent).toContain('31 min left');
  });

  // The file being printed is drawn the way every other file on this page is.
  it('shows the picture of the file it is printing', async () => {
    const { container } = await render(PrinterStatusPanel, {
      // A picture that loads, so the box keeps it, with the path it was asked
      // for carried in the fragment.
      props: props({
        thumbnailFor: (path) =>
          `data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg"/>#${path}`,
      }),
    });

    await expect
      .element(page.getByRole('button', { name: /Enclosure_Panel_Left/u }))
      .toBeVisible();
    // The picture is asked for once the box is in view and a slot is free.
    await expect
      .poll(() =>
        container
          .querySelector('[data-slot="file-thumbnail"] img')
          ?.getAttribute('src'),
      )
      .toBe(
        'data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg"/>#usb/Enclosure_Panel_Left.gcode',
      );
  });

  // A picture the printer cannot give is a file without one, not a hole.
  it('draws the glyph when the picture of the print fails', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props({ thumbnailFor: () => '/picture-that-fails' }),
    });

    // The box turns inert once the picture failed, and not merely while it
    // waits for its turn with the glyph inside a trigger.
    await expect
      .poll(
        () => container.querySelector('[data-slot="file-thumbnail"]')?.tagName,
      )
      .toBe('DIV');
    expect(
      container.querySelector('[data-slot="file-thumbnail"] svg'),
    ).not.toBeNull();
  });

  it('draws no file box when nothing is printing', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props({ device: idle(), thumbnailFor: () => '/picture' }),
    });

    await expect.element(page.getByText('No print running')).toBeVisible();
    expect(container.querySelector('[data-slot="file-thumbnail"]')).toBeNull();
  });

  // The record is a snapshot and the stream is what moves.
  it('prefers the live reading to the record', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props({
        telemetry: reading({
          '/job/progress': 88,
          '/job/display': 'Hinge_Bracket_v1.bgcode',
        }),
      }),
    });

    expect(container.textContent).toContain('88%');
    await expect
      .element(page.getByText('Hinge_Bracket_v1.bgcode'))
      .toBeVisible();
  });

  it('offers the verb the state allows and sends the capability behind it', async () => {
    const onCommand = vi.fn();
    await render(PrinterStatusPanel, {
      props: props({ commands: { onCommand } }),
    });

    expect(
      page.getByRole('button', { name: 'Resume' }).elements(),
    ).toHaveLength(0);
    await page.getByRole('button', { name: 'Pause' }).click();
    expect(onCommand).toHaveBeenCalledWith(
      'core.command.pause_print',
      undefined,
    );
  });

  it('shows a spinner on the button of the verb under way', async () => {
    await render(PrinterStatusPanel, {
      props: props({
        commands: {
          onCommand: () => undefined,
          sending: false,
          pending: capabilityFor('pause_print'),
        },
      }),
    });

    const pause = page.getByRole('button', { name: 'Pause' });
    await expect.element(pause).toBeDisabled();
    await expect.element(pause).toHaveAttribute('aria-busy', 'true');
  });

  // A verb the printer never published is not drawn at all.
  it('draws no act for a printer that published none', async () => {
    await render(PrinterStatusPanel, {
      props: props({ device: device('PRINTING', { capabilities: [] }) }),
    });

    expect(page.getByRole('button').elements()).toHaveLength(0);
  });

  it('asks before it stops a print, and sends nothing until the answer', async () => {
    const onCommand = vi.fn();
    await render(PrinterStatusPanel, {
      props: props({ commands: { onCommand } }),
    });

    await page.getByRole('button', { name: 'Stop' }).click();
    expect(onCommand).not.toHaveBeenCalled();

    const panel = document.body.querySelector<HTMLElement>(
      '[role="alertdialog"]',
    );
    if (panel === null) throw new Error('no panel opened');
    await page
      .elementLocator(panel)
      .getByRole('button', { name: 'Stop print' })
      .click();
    expect(onCommand).toHaveBeenCalledWith(
      'core.command.stop_print',
      undefined,
    );
  });

  it('names a printer it cannot hear from and offers it nothing', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props({
        device: device('PRINTING', {
          connected: false,
          departed: { at: '2026-08-19T17:44:00Z', reason: 'session_ended' },
        }),
      }),
    });

    await expect.element(page.getByText('Not reporting')).toBeVisible();
    await expect.element(page.getByText('Last known print')).toBeVisible();
    expect(container.textContent).not.toContain(FINISH);
    expect(page.getByRole('button', { name: 'Stop' }).elements()).toHaveLength(
      0,
    );
  });

  // A link that dropped under an open session has no departure, but the
  // printer has reported, so "never" would be false.
  it('does not say a printer that has reported never did', async () => {
    const { container, rerender } = await render(PrinterStatusPanel, {
      props: props({ device: device('PRINTING', { connected: false }) }),
    });
    expect(container.textContent).toContain('not reporting right now');
    expect(container.textContent).not.toContain('never reported');

    await rerender(
      props({ device: device('PRINTING', { connected: false, state: {} }) }),
    );
    expect(container.textContent).toContain('never reported');
  });

  // The panel waits in its own shape.
  it('waits in its own shape', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props({ loading: true }),
    });

    expect(
      container.querySelectorAll('[data-slot="skeleton"]').length,
    ).toBeGreaterThan(0);
    expect(container.textContent).not.toContain('Enclosure_Panel_Left.gcode');
  });

  // A printer with no camera loses the column rather than reserving space for.
  it('draws the camera only when this instance holds one', async () => {
    const { container, rerender } = await render(PrinterStatusPanel, {
      props: props(),
    });
    expect(container.querySelector('img')).toBeNull();

    await rerender(props({ camera: FEED }));
    await expect.poll(() => container.querySelector('img')).not.toBeNull();
  });

  // The picture is the still frame until a person plays it, and only then
  // says it is live; stop brings the still back.
  it('plays the camera only when a person presses play, and says live only then', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props({ camera: FEED }),
    });

    expect(container.textContent).not.toContain('Live');

    await page.getByRole('button', { name: 'Play bed-cam live' }).click();
    await expect.element(page.getByText('Live')).toBeVisible();

    await page.getByRole('button', { name: 'Stop bed-cam' }).click();
    await expect
      .element(page.getByRole('button', { name: 'Play bed-cam live' }))
      .toBeVisible();
    expect(container.textContent).not.toContain('Live');
  });

  // A printer asking for a person between prints has nothing to put beside
  // the camera, so no second column waits there empty.
  it('gives the card to the camera when no print is on record', async () => {
    const rule = '[class*="@2xl/now:border-s"]';
    const { container, rerender } = await render(PrinterStatusPanel, {
      props: props({ device: device('ATTENTION'), camera: FEED }),
    });
    await expect.poll(() => container.querySelector('img')).not.toBeNull();
    expect(container.querySelector(rule)).not.toBeNull();

    await rerender({
      device: { ...idle(), state: { '/state': 'ATTENTION' } },
    });
    await expect.poll(() => container.querySelector(rule)).toBeNull();
    expect(container.querySelector('img')).not.toBeNull();
  });

  // The camera opens at the page's size, on its live view when it has one,
  // and takes that view over from the column, so the camera streams once.
  // One Escape closes it.
  it('enlarges the camera in a dialog on its live view, and closes on Escape', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props({ camera: FEED }),
    });
    await page.getByRole('button', { name: 'Play bed-cam live' }).click();
    await expect.element(page.getByText('Live')).toBeVisible();

    const open = page.getByRole('button', {
      name: 'Open camera stream on MK4S #1042',
    });
    (open.element() as HTMLElement).focus();
    await userEvent.keyboard('{Enter}');

    const dialog = page.getByRole('dialog', { name: 'Camera on MK4S #1042' });
    await expect
      .element(dialog.getByRole('button', { name: 'Close' }))
      .toHaveFocus();
    await expect
      .element(dialog.getByRole('button', { name: 'Stop bed-cam' }))
      .toBeVisible();
    await expect
      .poll(() => dialog.element().querySelector('img[src$="#live"]'))
      .not.toBeNull();
    expect(container.querySelector('img[src$="#live"]')).toBeNull();

    await userEvent.keyboard('{Escape}');
    await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
    await expect.element(open).toHaveFocus();
    await expect
      .element(page.getByRole('button', { name: 'Play bed-cam live' }))
      .toBeVisible();
  });

  it('offers a camera with no live view as a snapshot', async () => {
    await render(PrinterStatusPanel, {
      props: props({
        camera: {
          camera: { ...FEED.camera, stream: false },
          pictures: { still: FEED.pictures.still },
        },
      }),
    });

    await page
      .getByRole('button', { name: 'Open snapshot from MK4S #1042' })
      .click();
    const dialog = page.getByRole('dialog', { name: 'Camera on MK4S #1042' });
    await expect
      .poll(() => dialog.element().querySelector('img'))
      .not.toBeNull();
    expect(dialog.getByRole('button', { name: /bed-cam/ }).query()).toBe(null);
    await expectNoViolations(dialog.element());
  });

  it('starts another camera on its still frame', async () => {
    const { rerender } = await render(PrinterStatusPanel, {
      props: props({ camera: FEED }),
    });
    await page.getByRole('button', { name: 'Play bed-cam live' }).click();
    await expect.element(page.getByText('Live')).toBeVisible();

    await rerender({
      camera: { ...FEED, camera: { ...FEED.camera, name: 'side-cam' } },
    });

    await expect
      .element(page.getByRole('button', { name: 'Play side-cam live' }))
      .toBeVisible();
    expect(page.getByText('Live').query()).toBe(null);
  });

  // A file still going up to this server shows beside the drop target, not
  // only in the files tab.
  it('shows a file still going up to the server, with a bar and a Cancel button', async () => {
    const onCancelUpload = vi.fn();
    await render(PrinterStatusPanel, {
      props: props({
        device: idle(),
        uploads: [
          {
            id: 'job:7',
            deviceId: 'p0',
            printerName: 'MK4S #1042',
            name: 'cube.bgcode',
            step: 'uploading',
            progress: 0.4,
            mine: true,
            startedBy: undefined,
            canCancel: true,
            stopping: false,
          },
        ],
        onCancelUpload,
      }),
    });

    await expect
      .element(page.getByText('Uploading to the server'))
      .toBeVisible();
    await expect
      .element(page.getByRole('progressbar', { name: 'Uploading cube.bgcode' }))
      .toHaveAttribute('aria-valuenow', '40');

    await page
      .getByRole('button', { name: 'Cancel the upload of cube.bgcode' })
      .click();
    expect(onCancelUpload).toHaveBeenCalledWith('job:7');
  });

  it('stops a file on its way to the printer', async () => {
    const stop = vi.fn(() => Promise.resolve());
    await render(PrinterStatusPanel, {
      props: props({
        device: idle(),
        transfers: {
          inFlight: [
            {
              transferId: 't-1',
              storage: 'usb',
              source: 'cube.bgcode',
              path: 'usb/cube.bgcode',
              bytesTotal: 100,
              bytesSent: 40,
              state: 'sending',
              reason: undefined,
            },
          ],
          stop,
        },
      }),
    });

    await page.getByRole('button', { name: 'Stop cube.bgcode' }).click();
    expect(stop).toHaveBeenCalledWith('t-1');
  });

  // A zone that accepts a file and does nothing with it is worse than none.
  it('draws the drop target only where the files have somewhere to go', async () => {
    const taken: string[][] = [];
    const onUpload = (files: File[]): void => {
      taken.push(files.map((file) => file.name));
    };
    const { container, rerender } = await render(PrinterStatusPanel, {
      props: props({ device: idle() }),
    });
    expect(zoneIn(container)).toBeNull();

    await rerender(props({ device: idle(), onUpload }));
    const zone = zoneIn(container);
    if (zone === null) throw new Error('no dropzone rendered');

    drop(zone, [new File(['G1 X10'], 'benchy.bgcode')]);
    expect(taken).toEqual([['benchy.bgcode']]);

    await rerender(
      props({
        device: device('PRINTING', {
          connected: false,
          departed: { at: '2026-08-19T17:44:00Z', reason: 'session_ended' },
        }),
        onUpload,
      }),
    );
    expect(zoneIn(container)).toBeNull();
  });

  // A printer with no USB drive is told so where the drop target would be,
  // and one this server cannot hear has its own story instead.
  it('says why there is no drop target, and offers to ask again', async () => {
    const onRecheck = vi.fn();
    const { container, rerender } = await render(PrinterStatusPanel, {
      props: props({ device: idle(), uploadBlock: 'noStorage', onRecheck }),
    });

    expect(zoneIn(container)).toBeNull();
    await expect
      .element(page.getByText('No USB drive in the printer'))
      .toBeVisible();
    await page.getByRole('button', { name: 'Check again' }).click();
    expect(onRecheck).toHaveBeenCalledOnce();

    await rerender(
      props({
        device: device('PRINTING', {
          connected: false,
          departed: { at: '2026-08-19T17:44:00Z', reason: 'session_ended' },
        }),
        uploadBlock: 'noStorage',
        onRecheck,
      }),
    );
    expect(container.textContent).not.toContain('No USB drive in the printer');
  });

  // The row carries what the print was.
  it('lists what the printer last ran and runs it again', async () => {
    const onCommand = vi.fn();
    const { container } = await render(PrinterStatusPanel, {
      props: props({
        device: idle(),
        commands: { onCommand },
        history,
        historyHref: '#/printers/p0/info',
      }),
    });

    await expect
      .element(page.getByText('Camera_Mount_Bracket_v3.bgcode'))
      .toBeVisible();
    expect(container.textContent).toContain('Finished');
    expect(container.textContent).toContain(formatDuration(PRINT_SECONDS));
    await expect
      .element(page.getByRole('link', { name: 'All prints' }))
      .toBeVisible();

    // The card above holds the one visible Print again; the row's is in its menu.
    expect(
      page.getByRole('button', { name: /Print again/ }).elements(),
    ).toHaveLength(0);
    await page
      .getByRole('button', {
        name: 'Actions for Camera_Mount_Bracket_v3.bgcode',
      })
      .click();
    await page.getByRole('menuitem', { name: 'Print again' }).click();
    expect(onCommand).toHaveBeenCalledWith('core.command.start_print', {
      path: PRINT_PATH,
    });
  });

  // A print's row opens its own page, as every print row in the app does.
  it('links each print to its job page', async () => {
    await render(PrinterStatusPanel, {
      props: props({ device: idle(), history }),
    });

    await expect
      .element(
        page.getByRole('link', { name: 'Camera_Mount_Bracket_v3.bgcode' }),
      )
      .toHaveAttribute('href', '/jobs/c7f4008e-c4d3-4f78-8888-0a6067f93b7f');
  });

  // A printer on its first print has printed something, and the card above
  // already shows it, so the block says that print lands here once it ends.
  it('does not say there are no prints while one is on the bed', async () => {
    const first: PrinterHistorySource = {
      jobs: {
        current: {
          display_name: 'Hinge_v2.bgcode',
          id: '0a1b2c3d-0000-7000-8000-000000000014',
          device_id: 'p0',
          device_job_id: '14',
          path: 'usb/Hinge_v2.bgcode',
          progress: 37,
          started: '2026-08-19T18:31:00Z',
          state: 'PRINTING',
          time_remaining_ms: REMAINING_MS,
        },
      },
      jobsError: undefined,
      jobsLoaded: true,
    };
    const { container } = await render(PrinterStatusPanel, {
      props: props({ history: first, onUpload: () => undefined }),
    });

    await expect
      .element(page.getByText('No finished prints yet'))
      .toBeVisible();
    expect(container.textContent).not.toContain('No prints yet');
  });

  // A refused read is said in the block, never left as half a card.
  it('says the prints could not be read where the rows would be', async () => {
    await render(PrinterStatusPanel, {
      props: props({
        device: idle(),
        history: {
          jobs: undefined,
          jobsError: Object.assign(
            new Error('the record store is not answering'),
            {
              code: 'core.store/UNAVAILABLE',
            },
          ),
          jobsLoaded: false,
        },
        onUpload: () => undefined,
      }),
    });

    await expect
      .element(page.getByText('core.store/UNAVAILABLE'))
      .toBeVisible();
    await expect.element(page.getByText('Recent prints')).toBeVisible();
  });

  // The row keeps the act and refuses it.
  it('refuses the print it ran last while the printer is busy', async () => {
    await render(PrinterStatusPanel, { props: props({ history }) });

    await page
      .getByRole('button', {
        name: 'Actions for Camera_Mount_Bracket_v3.bgcode',
      })
      .click();
    await expect
      .element(page.getByRole('menuitem', { name: /Print again/ }))
      .toHaveAttribute('aria-disabled', 'true');
  });

  // A stopped send leaves the machine naming a file it removed (CL-275).
  // The card says so in place of Print again, and the row refuses it.
  // A printer back in Idle still names the print it ran last, on one line
  // with Print again, and the line is only as tall as its file box.
  it('keeps the print an idle printer ran last on a line of its own height', async () => {
    const { container } = await render(PrinterStatusPanel, {
      props: props({
        device: device('IDLE', {
          job: {
            display_name: 'ball-holder.bgcode',
            id: '0b6f2f4e-1d7c-4f61-9a43-5e2a1c9d8b70',
            device_job_id: '15',
            path: 'usb/ball-holder.bgcode',
            progress: 0,
            running: false,
            state: 'IDLE',
            time_remaining_ms: 0,
          },
        }),
      }),
    });

    const name = container.querySelector('[data-slot="clipped-name"]');
    const plate = container.querySelector('[data-slot="file-thumbnail"]');
    if (name?.parentElement == null || plate === null) {
      throw new Error('no print line rendered');
    }
    expect(name.parentElement.getBoundingClientRect().height).toBe(
      plate.getBoundingClientRect().height,
    );
  });

  it('offers no Print again for a file the printer no longer holds', async () => {
    const checked: string[][] = [];
    const files = {
      checkHeld: (paths: readonly string[]) => {
        checked.push([...paths]);

        return Promise.resolve();
      },
      holds: () => false,
    };
    await render(PrinterStatusPanel, {
      props: props({
        device: device('IDLE', {
          job: {
            display_name: 'sheep_partial.bgcode',
            id: '01a1bfd0-4754-7436-92fa-fc0be5462f87',
            device_job_id: '333',
            path: '/usb/SHEEP_~2.BGC',
            progress: 0,
            running: false,
            state: 'IDLE',
            time_remaining_ms: 0,
          },
        }),
        history,
        files,
      }),
    });

    await expect
      .element(page.getByText('The file is no longer on the printer.'))
      .toBeVisible();
    expect(
      page.getByRole('button', { name: /Print again/ }).elements(),
    ).toHaveLength(0);
    expect(checked.at(-1)).toEqual(['/usb/SHEEP_~2.BGC', PRINT_PATH]);

    await page
      .getByRole('button', {
        name: 'Actions for Camera_Mount_Bracket_v3.bgcode',
      })
      .click();
    const again = page.getByRole('menuitem', { name: /Print again/ });
    await expect.element(again).toHaveAttribute('aria-disabled', 'true');
    expect(again.element().textContent).toContain(
      'The file is no longer on the printer.',
    );
  });

  // Only a print that was stopped or failed can name a partial file the
  // printer removed on its own, so only those have it read the folder again.
  it('names the prints that were stopped or failed for a fresh read', async () => {
    const checked: string[][] = [];
    const files = {
      checkHeld: (
        _: readonly string[],
        interrupted: readonly string[] = [],
      ) => {
        checked.push([...interrupted]);

        return Promise.resolve();
      },
      holds: () => undefined,
    };
    const last = history.jobs?.last;
    if (last === undefined) throw new Error('the fixture has a last print');
    await render(PrinterStatusPanel, {
      props: props({
        device: device('STOPPED', {
          job: {
            display_name: 'sheep_partial.bgcode',
            id: '01a1bfd0-4754-7436-92fa-fc0be5462f87',
            device_job_id: '333',
            path: '/usb/SHEEP_~2.BGC',
            progress: 0,
            running: false,
            state: 'STOPPED',
            time_remaining_ms: 0,
          },
        }),
        history,
        recent: [
          { ...last, id: 'f-1', path: 'usb/failed.bgcode', outcome: 'failed' },
          { ...last, id: 'f-2', path: 'usb/done.bgcode', outcome: 'finished' },
        ],
        files,
      }),
    });

    expect(checked.at(-1)).toEqual(['/usb/SHEEP_~2.BGC', 'usb/failed.bgcode']);
  });

  describe('a dialog on the printer', () => {
    const asking = (actions: string[]): Device =>
      device('ATTENTION', {
        actions,
        capabilities: ['core.command.dialog_action'],
        state: {
          '/state': 'ATTENTION',
          '/dialog/id': 7,
          '/dialog/title': 'Filament runout',
          '/dialog/text': 'Insert filament, then press Retry.',
          '/dialog/buttons': ['Retry', 'Cancel'],
        },
      });

    it('sends the pressed answer for the dialog on the screen', async () => {
      const onCommand = vi.fn();
      await render(PrinterStatusPanel, {
        props: props({
          device: asking(['observe', 'operate']),
          commands: { onCommand },
        }),
      });

      await expect.element(page.getByText('Filament runout')).toBeVisible();
      await page.getByRole('button', { name: 'Retry' }).click();
      expect(onCommand).toHaveBeenCalledWith('core.command.dialog_action', {
        dialog_id: '7',
        button: 'Retry',
      });
    });

    it('shows the question and holds the answers to a caller who may only watch', async () => {
      await render(PrinterStatusPanel, {
        props: props({ device: asking(['observe']) }),
      });

      await expect
        .element(page.getByRole('button', { name: 'Retry' }))
        .toBeDisabled();
      await expect
        .element(page.getByText(/needs permission to operate/))
        .toBeVisible();
    });

    // A stopped upload arrives as a button, a code and no words. The catalogue
    // names the code, so the banner asks the question the printer's own screen
    // asks.
    it('names a dialog that came with a code alone', async () => {
      const { container } = await render(PrinterStatusPanel, {
        props: props({
          device: device('ATTENTION', {
            actions: ['observe', 'operate'],
            driver: 'xbuddy',
            capabilities: ['core.command.dialog_action'],
            state: {
              '/state': 'ATTENTION',
              '/dialog/id': 7,
              '/dialog/code': '13807',
              '/dialog/buttons': ['Ok'],
            },
          }),
        }),
      });

      // The catalogue has "File error" for the title and for the text.
      await expect.element(page.getByText('File error')).toBeVisible();
      expect(container.textContent).not.toContain('not the question');
    });

    // "Identity name: %s" is a sentence the firmware fills in, so the catalogue
    // has the title alone and the reader is still sent to the machine.
    it('sends the reader to the printer for a text the firmware fills in', async () => {
      const { container } = await render(PrinterStatusPanel, {
        props: props({
          device: device('ATTENTION', {
            actions: ['observe', 'operate'],
            driver: 'xbuddy',
            capabilities: ['core.command.dialog_action'],
            state: {
              '/state': 'ATTENTION',
              '/dialog/id': 7,
              '/dialog/code': '13848',
              '/dialog/buttons': ['Yes', 'No'],
            },
          }),
        }),
      });

      await expect.element(page.getByText('UNTRUSTED IDENTITY')).toBeVisible();
      expect(container.textContent).toContain('not the question');
      expect(container.textContent).not.toContain('%s');
    });

    // The firmware sent a title of its own and no text. Its title stands, and
    // the catalogue says the rest.
    it("puts the catalogue's text under a title the printer sent", async () => {
      const { container } = await render(PrinterStatusPanel, {
        props: props({
          device: device('ATTENTION', {
            driver: 'xbuddy',
            actions: ['observe', 'operate'],
            capabilities: ['core.command.dialog_action'],
            state: {
              '/state': 'ATTENTION',
              '/dialog/id': 7,
              '/dialog/code': '13808',
              '/dialog/title': 'Power panic',
              '/dialog/buttons': ['Ok'],
            },
          }),
        }),
      });

      await expect.element(page.getByText('Power panic')).toBeVisible();
      expect(container.textContent).toContain('The heatbed cooled down');
      expect(container.textContent).not.toContain('not the question');
    });

    // The catalogue is the Buddy firmware's. The same digits from another
    // driver are another question, so they stay a number.
    it("leaves another driver's code and answers as they came", async () => {
      const { container } = await render(PrinterStatusPanel, {
        props: props({
          device: device('ATTENTION', {
            actions: ['observe', 'operate'],
            capabilities: ['core.command.dialog_action'],
            state: {
              '/state': 'ATTENTION',
              '/dialog/id': 7,
              '/dialog/code': '13807',
              '/dialog/buttons': ['FS_disable'],
            },
          }),
        }),
      });

      await expect
        .element(page.getByRole('button', { name: 'FS disable' }))
        .toBeVisible();
      expect(container.textContent).not.toContain('File error');
      expect(container.textContent).toContain('13807');
    });

    // The label is the catalogue's and the answer is the firmware's name. A
    // name the catalogue does not hold is shown without its underscores.
    it('labels the answers from the catalogue', async () => {
      await render(PrinterStatusPanel, {
        props: props({
          device: device('ATTENTION', {
            actions: ['observe', 'operate'],
            driver: 'xbuddy',
            capabilities: ['core.command.dialog_action'],
            state: {
              '/state': 'ATTENTION',
              '/dialog/id': 7,
              '/dialog/code': '13807',
              '/dialog/buttons': ['FS_disable', 'Brand_new'],
            },
          }),
        }),
      });

      await expect
        .element(page.getByRole('button', { name: 'Disable FS' }))
        .toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Brand new' }))
        .toBeVisible();
    });

    // The firmware builds some of its texts as it goes, so what the printer
    // said itself is kept over what the catalogue says about the code.
    it('keeps the words the printer sent over the catalogue', async () => {
      const { container } = await render(PrinterStatusPanel, {
        props: props({
          device: device('ATTENTION', {
            actions: ['observe', 'operate'],
            driver: 'xbuddy',
            capabilities: ['core.command.dialog_action'],
            state: {
              '/state': 'ATTENTION',
              '/dialog/id': 7,
              '/dialog/code': '13807',
              '/dialog/title': 'Upload stopped',
              '/dialog/text': 'The file part.gcode was not saved.',
              '/dialog/buttons': ['Ok'],
            },
          }),
        }),
      });

      await expect.element(page.getByText('Upload stopped')).toBeVisible();
      await expect
        .element(page.getByText('The file part.gcode was not saved.'))
        .toBeVisible();
      expect(container.textContent).not.toContain('File error');
    });

    // A newer firmware raises a code this build's catalogue does not hold.
    // Buttons under a bare heading ask somebody away from the machine to
    // answer a question they cannot read, so the banner says the words did not
    // arrive and gives the code the question is on the printer's screen under.
    it('says the printer sent no question and names its code', async () => {
      const onCommand = vi.fn();
      await render(PrinterStatusPanel, {
        props: props({
          device: device('ATTENTION', {
            actions: ['observe', 'operate'],
            capabilities: ['core.command.dialog_action'],
            state: {
              '/state': 'ATTENTION',
              '/dialog/id': 7,
              '/dialog/code': '13999',
              '/dialog/buttons': ['Yes', 'No', 'Purge_more'],
            },
          }),
          commands: { onCommand },
        }),
      });

      await expect.element(page.getByText(/13999/)).toBeVisible();
      // The label loses the firmware's underscores and the answer keeps them.
      await page.getByRole('button', { name: 'Purge more' }).click();
      expect(onCommand).toHaveBeenCalledWith('core.command.dialog_action', {
        dialog_id: '7',
        button: 'Purge_more',
      });
    });

    // Telemetry carries the dialog id alone between the events that describe
    // one, so a dialog reaches this server with no words and no buttons. The
    // sentence about the missing question promises answers, so it must not be
    // drawn where there are none: the reader is sent to the machine instead.
    it('promises no answers for a dialog that arrived as an id alone', async () => {
      const { container } = await render(PrinterStatusPanel, {
        props: props({
          device: device('ATTENTION', {
            actions: ['observe', 'operate'],
            capabilities: ['core.command.dialog_action'],
            state: { '/state': 'ATTENTION', '/dialog/id': 7 },
          }),
        }),
      });

      await expect
        .element(page.getByText("Answer it on the printer's own screen."))
        .toBeVisible();
      expect(container.textContent).not.toContain('only the answers');
    });
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(PrinterStatusPanel, { props: props() });

    await expectNoViolations(container);
  });
});
