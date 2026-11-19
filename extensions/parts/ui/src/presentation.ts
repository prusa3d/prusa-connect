// What the page reads off a file or a part to draw it: which kind of file it
// is, which glyph stands for it, and where its picture is. Plain functions,
// with no state, so every component and every test reads one answer.
import BoxIcon from '@lucide/svelte/icons/box';
import FileIcon from '@lucide/svelte/icons/file';
import FileCode2Icon from '@lucide/svelte/icons/file-code-2';
import FileTextIcon from '@lucide/svelte/icons/file-text';
import ImageIcon from '@lucide/svelte/icons/image';
import type { LucideIcon } from '@lucide/svelte';

import type { FilesService } from './files';
import type { PartFile, Picture } from './parts';

/** The kinds of file a part holds, as a person tells them apart. */
export type FileKind = 'print' | 'model' | 'image' | 'document' | 'other';

/** What a printer starts as it is. */
const PRINT = /\.(?:bgcode|gcode|gco|g)$/i;
/** What a slicer opens. */
const MODEL = /\.(?:3mf|stl|obj|step|stp|amf|ply|f3d|scad)$/i;
/** What a browser draws as it is. */
const IMAGE = /\.(?:png|jpe?g|webp|gif)$/i;
/** What a person reads. */
const DOCUMENT = /\.(?:pdf|txt|md|csv|dxf|svg|docx?|xlsx?|odt)$/i;

/** Which kind one file is, by its name. */
export const kindOf = (name: string): FileKind => {
  if (PRINT.test(name)) return 'print';
  if (MODEL.test(name)) return 'model';
  if (IMAGE.test(name)) return 'image';
  if (DOCUMENT.test(name)) return 'document';

  return 'other';
};

/** True for a file a printer starts as it is. */
export const isPrintable = (name: string): boolean => PRINT.test(name);

/** The catalogue key that names one kind. */
export const KIND_KEY: Record<FileKind, string> = {
  print: 'parts.kind.print',
  model: 'parts.kind.model',
  image: 'parts.kind.image',
  document: 'parts.kind.document',
  other: 'parts.kind.other',
};

const GLYPH: Record<FileKind, LucideIcon> = {
  print: FileCode2Icon,
  model: BoxIcon,
  image: ImageIcon,
  document: FileTextIcon,
  other: FileIcon,
};

/** The glyph that stands for one file. */
export const glyphOf = (name: string): LucideIcon => GLYPH[kindOf(name)];

/** True for a file that can stand for its part: a picture, or a print file with one inside. */
export const canCover = (name: string): boolean =>
  IMAGE.test(name) || /\.(?:bgcode|gcode|3mf)$/i.test(name);

/** The address of the picture that stands for a part, or undefined for none. */
export const pictureUrl = (
  files: Pick<FilesService, 'thumbnailUrl' | 'contentUrl'>,
  picture: Picture | undefined,
): string | undefined => {
  if (picture === undefined) return undefined;

  return picture.kind === 'image'
    ? files.contentUrl(picture.storage, picture.path)
    : files.thumbnailUrl(picture.storage, picture.path);
};

/** The address of the picture inside one file, or undefined for a file with none. */
export const fileThumbnail = (
  files: Pick<FilesService, 'thumbnailUrl' | 'contentUrl'>,
  file: PartFile,
): string | undefined => {
  if (IMAGE.test(file.name)) return files.contentUrl(file.storage, file.path);
  // The storage said it found no picture inside the file: no request is made.
  if (canCover(file.name) && file.thumbnail !== false) {
    return files.thumbnailUrl(file.storage, file.path);
  }

  return undefined;
};

/** The tags a person typed, as the module keeps them: lower case, one each. */
export const tagsOf = (written: string): string[] => {
  const out: string[] = [];

  for (const raw of written.split(/[,\s]+/)) {
    const tag = raw.trim().toLowerCase();
    if (tag !== '' && !out.includes(tag)) out.push(tag);
  }

  return out;
};

// Every limit below is the module's, in UTF-8 bytes as it counts them. A field
// carries its limit in `data-byte-limit` and the page holds it there while the
// person types (parts-page/byte-limits.ts); code that writes a value itself
// clips it with clipBytes.

/** The longest name a folder or a part takes. */
export const MAX_NAME = 120;
/** The longest file name the module records, in UTF-8 bytes. */
export const MAX_FILE_NAME_BYTES = 200;

const UTF8 = new TextEncoder();

/** How many bytes a text takes in UTF-8, as the module counts its limits. */
export const byteLength = (text: string): number => UTF8.encode(text).length;

/**
 * The longest start of a text that fits in `limit` UTF-8 bytes, cut between
 * two characters and never inside one.
 */
export const clipBytes = (text: string, limit: number): string => {
  if (byteLength(text) <= limit) return text;

  let out = '';
  let used = 0;
  for (const character of text) {
    const size = byteLength(character);
    if (used + size > limit) break;
    out += character;
    used += size;
  }

  return out;
};
/** The longest summary. */
export const MAX_SUMMARY = 280;
/** The longest description of a part. */
export const MAX_DESCRIPTION = 16384;
/** The longest description of a folder. */
export const MAX_FOLDER_DESCRIPTION = 2048;
/** The longest note on a version. */
export const MAX_NOTE = 2048;
/** The longest comment. */
export const MAX_COMMENT = 4096;
/** The longest tag. */
export const MAX_TAG = 32;
/** How many tags one part carries. */
export const MAX_TAGS = 16;

/** What the module would refuse about a set of tags, or undefined when nothing. */
export type TagProblem =
  | { readonly kind: 'many'; readonly limit: number }
  | { readonly kind: 'long'; readonly tag: string; readonly limit: number };

/** The first thing the module would refuse about the tags, in the order it checks. */
export const tagProblemOf = (
  tags: readonly string[],
): TagProblem | undefined => {
  const long = tags.find((tag) => byteLength(tag) > MAX_TAG);
  if (long !== undefined) return { kind: 'long', tag: long, limit: MAX_TAG };
  if (tags.length > MAX_TAGS) return { kind: 'many', limit: MAX_TAGS };

  return undefined;
};
