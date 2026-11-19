import { expect, it } from 'vitest';
import { avatarCrop } from './avatar-crop';

it('centers a square inside landscape and portrait images', () => {
  expect(avatarCrop(1000, 500, 1, 0, 0)).toEqual({ x: 250, y: 0, side: 500 });
  expect(avatarCrop(500, 1000, 1, 0, 0)).toEqual({ x: 0, y: 250, side: 500 });
});

it('clamps pan and zoom so the viewport is always filled', () => {
  for (const [width, height] of [
    [1000, 500],
    [500, 1000],
    [200, 200],
  ]) {
    for (const zoom of [0, 1, 2, 3, 5]) {
      for (const x of [-3, -1, 0, 1, 3]) {
        for (const y of [-3, -1, 0, 1, 3]) {
          const crop = avatarCrop(width!, height!, zoom, x, y);
          expect(crop.x).toBeGreaterThanOrEqual(0);
          expect(crop.y).toBeGreaterThanOrEqual(0);
          expect(crop.x + crop.side).toBeLessThanOrEqual(width!);
          expect(crop.y + crop.side).toBeLessThanOrEqual(height!);
        }
      }
    }
  }
});

it('moves the visible photo in the same direction as dragging', () => {
  expect(avatarCrop(1000, 500, 1, 1, 0).x).toBe(0);
  expect(avatarCrop(1000, 500, 1, -1, 0).x).toBe(500);
  expect(avatarCrop(1000, 500, 2, 0, 0).side).toBe(250);
});
