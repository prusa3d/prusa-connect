import type { FileThumbnailRadius, FileThumbnailSize } from './types';

// `md`: 56px is the smaller of the two sizes measured against a real render,
// 56 and 64, in both themes. At 40px a thumbnail reads as a colour blob; at
// 56px the plate's own layout, rows of parts and their rough shapes, is
// legible enough to tell one print's preview from another's, and 64px adds no
// legibility to earn the extra row height. `print` is the same 56px held
// whatever the density, for the job a printer or a jobs row shows. `wide` is
// a 16:9 box as wide as the caller makes it.
export const sizes: Record<FileThumbnailSize, string> = {
  md: 'size-14',
  print: 'size-printer-print-preview',
  wide: 'aspect-video h-auto',
};

export const radii: Record<FileThumbnailRadius, string> = {
  sm: 'rounded-sm',
  none: 'rounded-none',
};

export const THUMBNAIL_GLYPH = 'size-6';

export const THUMBNAIL_BASE =
  'flex shrink-0 items-center justify-center overflow-hidden bg-surface';

/**
 * The box height plus a row's own vertical padding. The box is the tallest
 * thing in a row that carries one, so 72px is that row's real floor rather
 * than the shared one a data row defaults to.
 */
export const THUMBNAIL_ROW_MIN_HEIGHT = 'min-h-18';

/** Preview sizes shared by the two file browsers. */
const FILE_ROW_WIDTHS = ['w-36', 'w-44', 'w-52', 'w-60'] as const;

export const fileRowPreview = (step: number) => [
  'max-w-file-preview border border-border',
  FILE_ROW_WIDTHS[step],
];

export const FILE_CARD_GRID = 'file-preview-grid';

export const FILE_CARD_DETAILS =
  'flex w-full min-w-0 flex-col gap-1 border-t border-border bg-card px-4 py-3 text-start';
