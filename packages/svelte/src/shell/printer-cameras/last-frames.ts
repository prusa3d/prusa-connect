// The last picture each camera's live view drew on this page.
//
// A camera that keeps no still has nothing to show between two live views,
// and a live view takes seconds to start: WebRTC may wait out its deadline,
// and the live socket waits for a keyframe. The last frame is shown grayed
// out in that time, so the picture never goes blank. It is held for as long
// as the page is open, and never stored.

import { SvelteMap } from 'svelte/reactivity';

const JPEG_QUALITY = 0.8;

const frames = new SvelteMap<string, string>();

/** The address of the last frame a camera's live view drew, if one did. */
export const lastFrame = (camera: string): string | undefined =>
  frames.get(camera);

/** Keeps what a live view shows now as the camera's last frame. */
export const keepFrame = (
  camera: string,
  source: CanvasImageSource,
  width: number,
  height: number,
): void => {
  if (width === 0 || height === 0) return;
  const copy = document.createElement('canvas');
  copy.width = width;
  copy.height = height;
  const drawing = copy.getContext('2d');
  if (!drawing) return;
  try {
    drawing.drawImage(source, 0, 0, width, height);
  } catch {
    return;
  }
  copy.toBlob(
    (blob) => {
      if (blob === null) return;
      const before = frames.get(camera);
      frames.set(camera, URL.createObjectURL(blob));
      if (before !== undefined) URL.revokeObjectURL(before);
    },
    'image/jpeg',
    JPEG_QUALITY,
  );
};

/** Forgets every frame. For tests, which share one module. */
export const forgetFrames = (): void => {
  for (const address of frames.values()) URL.revokeObjectURL(address);
  frames.clear();
};
