import { describe, expect, it } from 'vitest';

import { codecOf, pictureOf } from './h264';

const START = [0, 0, 0, 1];

describe('codecOf', () => {
  it('reads the profile, the constraints and the level out of the SPS', () => {
    // High profile (100), no constraints, level 4.0 (40).
    const unit = new Uint8Array([
      ...START,
      0x67,
      100,
      0,
      40,
      0xac,
      ...START,
      0x68,
      0xee,
    ]);

    expect(codecOf(unit)).toBe('avc1.640028');
  });

  it('finds the SPS behind a three-byte start code and another unit', () => {
    const unit = new Uint8Array([
      0, 0, 1, 0x09, 0xf0, 0, 0, 1, 0x67, 66, 0xc0, 30, 0x8c,
    ]);

    expect(codecOf(unit)).toBe('avc1.42c01e');
  });

  it('answers nothing for a picture that carries no SPS', () => {
    expect(
      codecOf(new Uint8Array([...START, 0x41, 0x9a, 0x02])),
    ).toBeUndefined();
  });

  it('answers nothing for an SPS cut short', () => {
    expect(codecOf(new Uint8Array([...START, 0x67, 100]))).toBeUndefined();
  });
});

describe('pictureOf', () => {
  it('splits the flag from the picture', () => {
    const picture = pictureOf(new Uint8Array([1, 0, 0, 0, 1, 0x65]).buffer);

    expect(picture?.key).toBe(true);
    expect([...(picture?.unit ?? [])]).toEqual([0, 0, 0, 1, 0x65]);
  });

  it('reads anything but a 1 as a picture that is not a keyframe', () => {
    expect(pictureOf(new Uint8Array([0, 0x41]).buffer)?.key).toBe(false);
  });

  it('refuses a message with no picture in it', () => {
    expect(pictureOf(new Uint8Array([1]).buffer)).toBeUndefined();
  });
});
