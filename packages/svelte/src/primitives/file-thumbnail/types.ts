import type { LucideIcon } from '@lucide/svelte';
import type { ClassValue } from 'svelte/elements';

/**
 * The box. `print` holds 56px at every density, and `wide` is 16:9 at the
 * width the caller gives it.
 */
export type FileThumbnailSize = 'md' | 'print' | 'wide';

/** The corner. `none` is a preview drawn to the edges of a card that rounds itself. */
export type FileThumbnailRadius = 'sm' | 'none';

interface FileThumbnailBase {
  /** Names the preview for a screen reader, and captions it. */
  name: string;
  /** Absent for a folder, and for a file this instance has no preview of. */
  thumbnailUrl?: string | undefined;
  /**
   * Whether this row's preview is the one showing. The open state is the
   * caller's: a listing wants one open preview at a time, and only the list
   * knows which row that is.
   */
  open?: boolean;
  size?: FileThumbnailSize | undefined;
  radius?: FileThumbnailRadius | undefined;
  class?: ClassValue | undefined;
}

/**
 * The fallback the box draws without a thumbnail: a design asset when
 * `glyphSrc` is given, which then needs no `glyph`, otherwise the icon.
 */
export type FileThumbnailProps = FileThumbnailBase &
  (
    | { glyph: LucideIcon; glyphSrc?: string | undefined }
    | { glyph?: undefined; glyphSrc: string }
  );
