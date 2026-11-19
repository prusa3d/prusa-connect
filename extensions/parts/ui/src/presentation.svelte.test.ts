import { describe, expect, it } from 'vitest';

import { fileThumbnail } from './presentation';

const files = {
  thumbnailUrl: (storage: string, path: string) => `thumb:${storage}/${path}`,
  contentUrl: (storage: string, path: string) => `file:${storage}/${path}`,
};
const file = (name: string, thumbnail?: boolean) => ({
  name,
  storage: 'local',
  path: name,
  size: 1,
  ...(thumbnail === undefined ? {} : { thumbnail }),
});

describe('the picture of one file', () => {
  it('asks for the picture inside a print file unless the storage found none', () => {
    expect(fileThumbnail(files, file('a.bgcode'))).toBe('thumb:local/a.bgcode');
    expect(fileThumbnail(files, file('a.bgcode', true))).toBe(
      'thumb:local/a.bgcode',
    );
    expect(fileThumbnail(files, file('a.bgcode', false))).toBeUndefined();
  });

  it('draws an image as it is, and nothing for another file', () => {
    expect(fileThumbnail(files, file('p.png'))).toBe('file:local/p.png');
    expect(fileThumbnail(files, file('notes.txt'))).toBeUndefined();
  });
});
