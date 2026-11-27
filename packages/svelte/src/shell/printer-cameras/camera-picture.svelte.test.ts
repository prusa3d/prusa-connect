import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { cleanup, render } from 'vitest-browser-svelte';

import CameraGoneProbe from '../../test/camera-gone-probe.svelte';
import { listingOf, picturesOf } from './cameras.fixtures';
import { forgetCarriage } from './carriage';
import { forgetFrames } from './last-frames';
import { hangUpAll } from './live-sessions';

const shelf = listingOf().find((row) => row.name === 'shelf-cam');
if (shelf === undefined) throw new Error('the fixtures lost shelf-cam');

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

afterEach(() => {
  cleanup();
  hangUpAll();
  forgetCarriage();
  forgetFrames();
});

describe('CameraPicture', () => {
  // A dashboard tile drops its camera while the view plays when the camera is
  // removed or the list is read again. The view's teardown must not read the
  // camera it no longer has.
  it('lets go of a live socket view whose camera went away', async () => {
    const sockets: (WebSocket & { closes: number })[] = [];
    await render(CameraGoneProbe, {
      props: {
        camera: shelf,
        pictures: picturesOf(shelf),
        socket: () => {
          const socket = quietSocket();
          sockets.push(socket);

          return socket;
        },
      },
    });
    await expect.poll(() => sockets).toHaveLength(1);

    await page.getByRole('button', { name: 'Remove the camera' }).click();

    await expect
      .element(page.getByRole('button', { name: 'Stop shelf-cam' }))
      .not.toBeInTheDocument();
    expect(
      document.querySelector('[data-caught]')?.textContent,
    ).toBeUndefined();
    // Held for the next picture of the camera, not closed.
    expect(sockets[0]?.closes).toBe(0);
    hangUpAll();
    expect(sockets[0]?.closes).toBe(1);
  });

  it('lets go of a WebRTC view whose camera went away', async () => {
    const offers: string[] = [];
    await render(CameraGoneProbe, {
      props: {
        camera: shelf,
        pictures: picturesOf(shelf),
        // An answer that never comes, so the view stays starting.
        watch: (_camera: string, offer: string) => {
          offers.push(offer);

          return new Promise<string>(() => undefined);
        },
      },
    });
    await expect.poll(() => offers).toHaveLength(1);

    await page.getByRole('button', { name: 'Remove the camera' }).click();

    await expect
      .element(page.getByRole('button', { name: 'Stop shelf-cam' }))
      .not.toBeInTheDocument();
    expect(
      document.querySelector('[data-caught]')?.textContent,
    ).toBeUndefined();
  });
});
