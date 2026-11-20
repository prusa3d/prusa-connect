import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { cleanup, render } from 'vitest-browser-svelte';

import type { Camera } from '../../data';
import { expectNoViolations } from '../../test/axe';
import {
  CAMERAS,
  declaration,
  fakeSettings,
  listingOf,
  NOT_WRITABLE,
  picturesOf,
  PRINTER,
  PRINTER_NAME,
  PRINTERS,
  pushUrlOf,
} from './cameras.fixtures';
import { forgetCarriage } from './carriage';
import { forgetFrames, keepFrame, lastFrame } from './last-frames';
import { hangUpAll } from './live-sessions';
import PrinterCamerasPanel from './printer-cameras-panel.svelte';
import type { PrinterCamerasPanelProps } from './types';

const foot = createRawSnippet((camera: () => string) => ({
  render: () => `<p>section for ${camera()}</p>`,
}));

const draw = (
  settings = fakeSettings(),
  extra: {
    cameras?: readonly Camera[];
    loaded?: boolean;
    withFoot?: boolean;
    watch?: PrinterCamerasPanelProps['watch'];
    socket?: PrinterCamerasPanelProps['socket'];
  } = {},
) =>
  render(PrinterCamerasPanel, {
    props: {
      deviceId: PRINTER,
      printerName: PRINTER_NAME,
      cameras: extra.cameras ?? listingOf(settings.items),
      loaded: extra.loaded ?? true,
      picturesOf,
      settings,
      printers: PRINTERS,
      pushUrlOf,
      ...(extra.withFoot === true ? { foot } : {}),
      ...(extra.watch === undefined ? {} : { watch: extra.watch }),
      ...(extra.socket === undefined ? {} : { socket: extra.socket }),
    },
  });

const cardOf = (name: string) =>
  page
    .getByRole('heading', { name, level: 2 })
    .element()
    .closest('[data-part="camera"]');

// What a camera serves sits on the Information tab, or in plain view on a
// camera with no settings to change.
const openInfo = async (name: string): Promise<void> => {
  const tab = [...(cardOf(name)?.querySelectorAll('[role="tab"]') ?? [])].find(
    (element) => element.textContent.trim() === 'Information',
  );
  if (tab !== undefined) await userEvent.click(tab);
};

/** The text one camera shows beside its picture. */
const shownOf = (name: string): string => {
  const card = cardOf(name);

  return (
    card?.querySelector('[role="tabpanel"]:not([hidden])')?.textContent ??
    card?.textContent ??
    ''
  );
};

/** Shows or hides the browser tab, as switching to another tab does. */
const showPage = (state: DocumentVisibilityState): void => {
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => state,
  });
  document.dispatchEvent(new Event('visibilitychange'));
};

afterEach(() => {
  // A view that goes holds its connection, so the pictures go first and the
  // connections they held after them.
  cleanup();
  hangUpAll();
  Reflect.deleteProperty(document, 'visibilityState');
  forgetCarriage();
  forgetFrames();
});

// A live socket that closes as soon as it opens, which is what a server that
// refused the upgrade looks like to the view.
const deadSocket = (): WebSocket => {
  const socket = Object.assign(new EventTarget(), {
    binaryType: 'arraybuffer',
    close: () => undefined,
  });
  queueMicrotask(() => socket.dispatchEvent(new Event('close')));

  return socket as unknown as WebSocket;
};

// A live socket that stays open and sends nothing, and counts its closes.
const quietSocket = (): WebSocket & { closes: number } => {
  const socket = Object.assign(new EventTarget(), {
    binaryType: 'arraybuffer',
    closes: 0,
    close: () => {
      socket.closes += 1;
    },
  });

  return socket as unknown as WebSocket & { closes: number };
};

describe('PrinterCamerasPanel', () => {
  // ADR-20261119-pxn rules 1 and 2.
  it('plays each camera while its card is seen, until a person stops it', async () => {
    await draw();

    const name = `The camera bed-cam on ${PRINTER_NAME}`;
    await expect
      .element(page.getByRole('img', { name }))
      .toHaveAttribute('src', expect.stringContaining('#live-bed-cam'));
    expect(cardOf('bed-cam')?.textContent).toContain('Live');

    await page.getByRole('button', { name: 'Stop bed-cam' }).click();
    await expect
      .element(page.getByRole('img', { name }))
      .toHaveAttribute('src', expect.stringContaining('#still-bed-cam'));

    // A stop holds: the tab coming back does not start it again.
    showPage('hidden');
    showPage('visible');
    await expect
      .element(page.getByRole('button', { name: 'Play bed-cam live' }))
      .toBeVisible();
    expect(cardOf('bed-cam')?.textContent).not.toContain('Live');
  });

  // ADR-20261119-pxn rule 1.
  it('stops a live view while the browser tab is hidden, and plays it again when shown', async () => {
    await draw();

    const name = `The camera bed-cam on ${PRINTER_NAME}`;
    await expect
      .element(page.getByRole('img', { name }))
      .toHaveAttribute('src', expect.stringContaining('#live-bed-cam'));

    showPage('hidden');
    await expect
      .element(page.getByRole('img', { name }))
      .toHaveAttribute('src', expect.stringContaining('#still-bed-cam'));

    showPage('visible');
    await expect
      .element(page.getByRole('img', { name }))
      .toHaveAttribute('src', expect.stringContaining('#live-bed-cam'));
  });

  // ADR-20261119-pxn rule 2. A declared camera keeps its card while the
  // server does not run it, so the stop is still the person's.
  it('keeps a camera a person stopped on its still frame when it stops running and comes back', async () => {
    const settings = fakeSettings();
    const all = listingOf(settings.items);
    const { rerender } = await draw(settings);

    await page.getByRole('button', { name: 'Stop bed-cam' }).click();
    await rerender({
      cameras: all.filter((camera) => camera.name !== 'bed-cam'),
    });
    await rerender({ cameras: all });
    cardOf('bed-cam')?.scrollIntoView();

    await expect
      .element(page.getByRole('button', { name: 'Play bed-cam live' }))
      .toBeVisible();
    expect(cardOf('bed-cam')?.textContent).not.toContain('Live');
  });

  it('offers to play only a camera that has a live view', async () => {
    await draw();

    expect(
      page.getByRole('button', { name: 'Play door-cam live' }).query(),
    ).toBe(null);
  });

  it('stops a live view whose stream does not load, and says so', async () => {
    const pictures = picturesOf;
    await render(PrinterCamerasPanel, {
      props: {
        deviceId: PRINTER,
        printerName: PRINTER_NAME,
        cameras: listingOf([declaration('lone-cam', 'push', {})]),
        loaded: true,
        picturesOf: (row: Camera) => ({
          ...pictures(row),
          live: '/no-such-stream.mjpg',
        }),
        settings: fakeSettings([]),
        printers: PRINTERS,
        pushUrlOf,
      },
    });

    await expect
      .element(
        page.getByText('The camera did not start streaming.', { exact: false }),
      )
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Play lone-cam live' }))
      .toBeVisible();
  });

  it('says why the cameras could not be read, in place of a skeleton', async () => {
    await render(PrinterCamerasPanel, {
      props: {
        deviceId: PRINTER,
        printerName: PRINTER_NAME,
        cameras: [],
        loaded: false,
        error: NOT_WRITABLE,
        picturesOf,
        settings: fakeSettings([]),
        printers: PRINTERS,
        pushUrlOf,
      },
    });

    await expect.element(page.getByRole('alert')).toBeVisible();
    expect(page.getByLabelText('Loading cameras').query()).toBe(null);
  });

  it('says a camera offers no picture this page can show', async () => {
    await render(PrinterCamerasPanel, {
      props: {
        deviceId: PRINTER,
        printerName: PRINTER_NAME,
        cameras: listingOf([declaration('lone-cam', 'mjpeg', {})]),
        loaded: true,
        picturesOf: () => ({}),
        settings: fakeSettings([]),
        printers: PRINTERS,
        pushUrlOf,
      },
    });

    await expect
      .element(
        page.getByText('This camera offers no picture this page can show.'),
      )
      .toBeVisible();
  });

  it('draws no picture for a camera that is not running', async () => {
    await draw();

    await expect.element(page.getByText('Not running')).toBeVisible();
    expect(
      page
        .getByRole('img', { name: `The camera buddy3d on ${PRINTER_NAME}` })
        .query(),
    ).toBe(null);
  });

  it('draws the words in place of a picture that does not load', async () => {
    await render(PrinterCamerasPanel, {
      props: {
        deviceId: PRINTER,
        printerName: PRINTER_NAME,
        cameras: listingOf([declaration('lone-cam', 'mjpeg', {})]),
        loaded: true,
        // A push camera with no frame yet answers 204, which an img reads as
        // a failed load.
        picturesOf: () => ({ still: '/no-such-picture.jpg' }),
        settings: fakeSettings([]),
        printers: PRINTERS,
        pushUrlOf,
      },
    });

    await expect
      .poll(() =>
        page
          .getByRole('img', { name: `The camera lone-cam on ${PRINTER_NAME}` })
          .query(),
      )
      .toBe(null);
  });

  it('names each camera its type', async () => {
    await draw();

    await expect.element(page.getByText('Push').first()).toBeVisible();
  });

  it('hands each camera to the sections modules draw beside it', async () => {
    await draw(fakeSettings(), { withFoot: true });

    for (const camera of CAMERAS) {
      await expect
        .element(page.getByText(`section for ${camera.name}`))
        .toBeVisible();
    }
  });

  it('shows the upload address of a push camera and of no other', async () => {
    await draw();

    await openInfo('bed-cam');
    await expect.poll(() => shownOf('bed-cam')).toContain(pushUrlOf('bed-cam'));
    await expect
      .element(page.getByRole('button', { name: 'Copy Upload address' }))
      .toBeVisible();
    await openInfo('door-cam');
    await expect.poll(() => shownOf('door-cam')).toContain('Stream');
    expect(shownOf('door-cam')).not.toContain('Upload address');
  });

  it('offers the upload address of a generic camera that takes the pictures it is sent', async () => {
    await draw(
      fakeSettings([
        declaration('sent-cam', 'core/generic', {}),
        declaration('fetched-cam', 'core/generic', {
          snapshot_url: 'http://192.168.1.42/snap.jpg',
        }),
      ]),
    );

    await openInfo('sent-cam');
    await expect
      .poll(() => shownOf('sent-cam'))
      .toContain(pushUrlOf('sent-cam'));
    await openInfo('fetched-cam');
    await expect.poll(() => shownOf('fetched-cam')).toContain('Stream');
    expect(shownOf('fetched-cam')).not.toContain('Upload address');
  });

  // ADR-20261119-pxn rule 3: a view that did not start is not tried again.
  it('plays an H.264 camera over WebRTC, and says so once when it does not start', async () => {
    const offers: string[] = [];
    await draw(fakeSettings(), {
      watch: (_camera: string, offer: string) => {
        offers.push(offer);

        return Promise.reject(new Error('no route'));
      },
    });
    cardOf('shelf-cam')?.scrollIntoView();

    await expect
      .element(
        page.getByText(
          'The camera did not start streaming. Press play to try again.',
        ),
      )
      .toBeVisible();
    showPage('hidden');
    showPage('visible');
    expect(offers).toHaveLength(1);
    expect(offers[0]).toContain('m=video');
    await expect
      .element(page.getByRole('button', { name: 'Play shelf-cam live' }))
      .toBeVisible();
  });

  it('falls back to the live socket when WebRTC does not connect, and goes there first after', async () => {
    const offers: string[] = [];
    const sockets: string[] = [];
    await draw(fakeSettings(), {
      watch: (_camera: string, offer: string) => {
        offers.push(offer);

        return Promise.reject(new Error('no route'));
      },
      socket: (camera: string) => {
        sockets.push(camera);

        return deadSocket();
      },
    });
    cardOf('shelf-cam')?.scrollIntoView();

    await expect
      .element(
        page.getByText(
          'The camera did not start streaming. Press play to try again.',
        ),
      )
      .toBeVisible();
    expect(offers).toHaveLength(1);
    expect(sockets).toEqual(['shelf-cam']);

    // WebRTC did not connect on this page, so the next play skips it.
    await userEvent.click(
      page.getByRole('button', { name: 'Play shelf-cam live' }),
    );
    await expect.poll(() => sockets).toHaveLength(2);
    expect(offers).toHaveLength(1);
  });

  it('shows the last frame grayed out while a live view starts', async () => {
    const source = document.createElement('canvas');
    source.width = 16;
    source.height = 9;
    keepFrame('shelf-cam', source, source.width, source.height);
    await expect.poll(() => lastFrame('shelf-cam')).toBeDefined();

    // An answer that never comes, so the view stays starting.
    await draw(fakeSettings(), {
      watch: () => new Promise<string>(() => undefined),
    });
    cardOf('shelf-cam')?.scrollIntoView();

    await expect
      .poll(() =>
        cardOf('shelf-cam')?.querySelector(
          `img.grayscale[src="${lastFrame('shelf-cam') ?? ''}"]`,
        ),
      )
      .not.toBe(null);
  });

  it('keeps the connection of a view that stopped while its tab was hidden, and plays it again on it', async () => {
    const sockets: (WebSocket & { closes: number })[] = [];
    await draw(fakeSettings(), {
      socket: () => {
        const socket = quietSocket();
        sockets.push(socket);

        return socket;
      },
    });
    cardOf('shelf-cam')?.scrollIntoView();
    await expect.poll(() => sockets).toHaveLength(1);

    showPage('hidden');
    await expect
      .element(page.getByRole('button', { name: 'Play shelf-cam live' }))
      .toBeVisible();
    showPage('visible');
    await expect
      .element(page.getByRole('button', { name: 'Stop shelf-cam' }))
      .toBeVisible();

    expect(sockets).toHaveLength(1);
    expect(sockets[0]?.closes).toBe(0);

    // A stop the person pressed hangs up at once.
    await page.getByRole('button', { name: 'Stop shelf-cam' }).click();
    await expect.poll(() => sockets[0]?.closes).toBe(1);
  });

  it('plays an H.264 camera over the live socket alone when nothing answers an offer', async () => {
    const sockets: string[] = [];
    await draw(fakeSettings(), {
      socket: (camera: string) => {
        sockets.push(camera);

        return deadSocket();
      },
    });
    cardOf('shelf-cam')?.scrollIntoView();

    await expect.poll(() => sockets).toEqual(['shelf-cam']);
  });

  it('offers no play button for an H.264 camera when nothing can answer the offer', async () => {
    await draw();

    expect(
      page.getByRole('button', { name: 'Play shelf-cam live' }).query(),
    ).toBe(null);
  });

  it("names a module's type by its title, and offers it when adding a camera", async () => {
    const settings = fakeSettings([
      declaration('bed-cam', 'com.github.aux.acme.cams/buddy3d', {}),
    ]);
    await draw(settings);

    await expect
      .element(page.getByText('Buddy3D camera').first())
      .toBeVisible();

    await page.getByRole('button', { name: 'Add camera' }).click();
    await userEvent.fill(page.getByLabelText('Name'), 'door-cam');
    await page.getByRole('combobox', { name: 'Type' }).click();
    await page.getByRole('option', { name: 'Buddy3D camera' }).click();
    // A module's type takes no address: its module feeds the camera.
    expect(page.getByLabelText('Snapshot address').query()).toBe(null);
    await page
      .getByRole('dialog')
      .getByRole('button', { name: 'Add camera' })
      .click();

    expect(settings.saved).toEqual([
      {
        name: 'door-cam',
        type: 'com.github.aux.acme.cams/buddy3d',
        printer: PRINTER,
        snapshotUrl: '',
        streamUrl: '',
      },
    ]);
  });

  it("plays a module's own live view of a camera of its type", async () => {
    const view = createRawSnippet((camera: () => Camera) => ({
      render: () => `<p>module view of ${camera().name}</p>`,
    }));
    await render(PrinterCamerasPanel, {
      props: {
        deviceId: PRINTER,
        printerName: PRINTER_NAME,
        cameras: listingOf([
          declaration('bed-cam', 'com.github.aux.acme.cams/buddy3d', {}),
        ]),
        loaded: true,
        picturesOf,
        settings: fakeSettings([]),
        printers: PRINTERS,
        pushUrlOf,
        viewOf: (camera: Camera) =>
          camera.type.startsWith('com.github.aux.acme.cams/')
            ? view
            : undefined,
      },
    });

    await expect
      .element(page.getByText('module view of bed-cam'))
      .toBeVisible();
  });

  // A camera just added has sent nothing, so there is nothing to watch yet.
  it("offers no module's live view before the camera sent a picture", async () => {
    const view = createRawSnippet(() => ({ render: () => '<p>view</p>' }));
    await render(PrinterCamerasPanel, {
      props: {
        deviceId: PRINTER,
        printerName: PRINTER_NAME,
        cameras: listingOf([
          declaration('bed-cam', 'com.github.aux.acme.cams/buddy3d', {}),
        ]),
        loaded: true,
        picturesOf: () => ({ still: '/no-such-picture.jpg' }),
        settings: fakeSettings([]),
        printers: PRINTERS,
        pushUrlOf,
        viewOf: () => view,
      },
    });

    await expect
      .element(page.getByText('This camera has sent no picture yet.'))
      .toBeVisible();
    expect(
      page.getByRole('button', { name: 'Play bed-cam live' }).query(),
    ).toBe(null);
    expect(page.getByRole('img').query()).toBe(null);
  });

  it('draws a declared camera the server does not run, so it can be removed', async () => {
    const settings = fakeSettings();
    await draw(settings, {
      cameras: listingOf(settings.items).filter(
        (camera) => camera.name !== 'bed-cam',
      ),
    });

    const card = cardOf('bed-cam');
    expect(card?.textContent).toContain('Not running');
    expect(card?.textContent).toContain('Remove');
  });

  it('drops a still the camera no longer has', async () => {
    const settings = fakeSettings();
    const { rerender } = await draw(settings);
    const name = `The camera bed-cam on ${PRINTER_NAME}`;
    await expect.element(page.getByRole('img', { name })).toBeVisible();

    await rerender({ picturesOf: () => ({ still: '/no-such-picture.jpg' }) });

    await expect.poll(() => page.getByRole('img', { name }).query()).toBe(null);
  });

  it('offers no control to a person who may not declare cameras', async () => {
    await draw(fakeSettings(CAMERAS, { allowed: false }));

    await expect.element(page.getByText('bed-cam')).toBeVisible();
    expect(page.getByRole('button', { name: 'Add camera' }).query()).toBe(null);
    expect(page.getByRole('button', { name: 'Save' }).query()).toBe(null);
    expect(page.getByRole('button', { name: 'Remove' }).query()).toBe(null);
  });

  it('says there are none, apart from a list not read yet', async () => {
    const { unmount } = await draw(fakeSettings([]));
    await expect.element(page.getByText('No cameras')).toBeVisible();
    await unmount();

    await draw(fakeSettings([]), { loaded: false });
    await expect.element(page.getByLabelText('Loading cameras')).toBeVisible();
    expect(page.getByText('No cameras').query()).toBe(null);
  });

  it('adds a generic camera that watches this printer', async () => {
    const settings = fakeSettings([]);
    await draw(settings);

    await page.getByRole('button', { name: 'Add camera' }).click();
    await userEvent.fill(page.getByLabelText('Name'), 'bed-cam');
    await page
      .getByRole('dialog')
      .getByRole('button', { name: 'Add camera' })
      .click();

    expect(settings.saved).toEqual([
      {
        name: 'bed-cam',
        type: 'generic',
        printer: PRINTER,
        snapshotUrl: '',
        streamUrl: '',
      },
    ]);
  });

  it('refuses a name the server would refuse, and one already taken', async () => {
    const settings = fakeSettings();
    await draw(settings);

    await page.getByRole('button', { name: 'Add camera' }).click();
    const submit = page
      .getByRole('dialog')
      .getByRole('button', { name: 'Add camera' });

    await userEvent.fill(page.getByLabelText('Name'), 'Bed Cam');
    await expect
      .element(page.getByText(/Use lowercase letters, digits and dashes/))
      .toBeVisible();
    await expect.element(submit).toBeDisabled();

    await userEvent.fill(page.getByLabelText('Name'), 'bed-cam');
    await expect
      .element(page.getByText('A camera already has this name.'))
      .toBeVisible();
    await expect.element(submit).toBeDisabled();
    expect(settings.saved).toEqual([]);
  });

  it('removes a camera only once the person confirms', async () => {
    const settings = fakeSettings([declaration('bed-cam', 'push', {})]);
    await draw(settings);

    await page.getByRole('button', { name: 'Remove' }).click();
    expect(settings.removed).toEqual([]);
    await page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Remove' })
      .click();

    expect(settings.removed).toEqual(['bed-cam']);
  });

  it('changes a camera in place, beside its picture', async () => {
    const settings = fakeSettings([
      declaration('sent-cam', 'core/generic', {}),
    ]);
    await draw(settings);

    const save = page.getByRole('button', { name: 'Save' });
    await expect.element(save).toBeDisabled();
    await userEvent.fill(
      page.getByLabelText('Snapshot address'),
      'http://192.168.1.42/snap.jpg',
    );
    await save.click();

    expect(settings.saved).toEqual([
      {
        name: 'sent-cam',
        type: 'core/generic',
        printer: PRINTER,
        snapshotUrl: 'http://192.168.1.42/snap.jpg',
        streamUrl: '',
      },
    ]);
  });

  it('holds every control while a change is on its way', async () => {
    await draw(fakeSettings(CAMERAS, { pending: 'save' }));

    await expect
      .element(page.getByRole('button', { name: 'Add camera' }))
      .toBeDisabled();
    await expect
      .element(page.getByRole('button', { name: 'Remove' }).first())
      .toBeDisabled();
  });

  it('has no accessibility violations', async () => {
    const { container } = await draw();

    await expect.element(page.getByText('bed-cam')).toBeVisible();
    await expectNoViolations(container);
  });
});
