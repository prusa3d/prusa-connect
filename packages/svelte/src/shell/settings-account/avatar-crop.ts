/** Canvas source coordinates; CSS owns the visible viewport size. */
export function avatarCrop(
  width: number,
  height: number,
  zoom: number,
  x: number,
  y: number,
) {
  const side = Math.min(width, height) / Math.min(3, Math.max(1, zoom));
  const clamp = (value: number): number => Math.min(1, Math.max(-1, value));
  return {
    x: ((width - side) * (1 - clamp(x))) / 2,
    y: ((height - side) * (1 - clamp(y))) / 2,
    side,
  };
}

export const AVATAR_PIXELS = 512;
