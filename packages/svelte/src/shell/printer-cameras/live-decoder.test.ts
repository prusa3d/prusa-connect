import { describe, expect, it } from 'vitest';

import type { SocketPicture } from './h264';
import {
  DECODER_RETRIES,
  LiveDecoder,
  type PictureDecoder,
} from './live-decoder';

const START = [0, 0, 0, 1];
const KEY: SocketPicture = {
  key: true,
  unit: new Uint8Array([
    ...START,
    0x67,
    66,
    0xc0,
    40,
    ...START,
    0x68,
    0xce,
    ...START,
    0x65,
    0x88,
  ]),
};
const DELTA: SocketPicture = {
  key: false,
  unit: new Uint8Array([...START, 0x41, 0x9a]),
};

/** A decoder that fails when told to, as a WebCodecs one does: it closes and reports. */
class FakeDecoder implements PictureDecoder {
  state: CodecState = 'unconfigured';
  decodeQueueSize = 0;
  readonly decoded: string[] = [];
  constructor(
    readonly output: (frame: VideoFrame) => void,
    readonly error: (cause: unknown) => void,
  ) {}
  configure(): void {
    this.state = 'configured';
  }
  decode(chunk: EncodedVideoChunk): void {
    this.decoded.push(chunk.type);
    this.output({ close: () => undefined } as VideoFrame);
  }
  close(): void {
    this.state = 'closed';
  }
  breaks(): void {
    this.state = 'closed';
    this.error(new Error('Decoding error.'));
  }
}

const harness = () => {
  const decoders: FakeDecoder[] = [];
  let drawn = 0;
  let failed = 0;
  const live = new LiveDecoder({
    open: (output, error) => {
      const made = new FakeDecoder(output, error);
      decoders.push(made);

      return made;
    },
    chunk: (init) => ({ type: init.type }) as EncodedVideoChunk,
    draw: () => {
      drawn += 1;
    },
    fail: () => {
      failed += 1;
    },
  });

  return {
    live,
    decoders,
    drawn: () => drawn,
    failed: () => failed,
    current: () => decoders.at(-1)!,
  };
};

describe('LiveDecoder', () => {
  it('waits for a keyframe before it decodes anything', () => {
    const h = harness();
    h.live.take(DELTA, 1);
    h.live.take(KEY, 2);
    h.live.take(DELTA, 3);

    expect(h.current().decoded).toEqual(['key', 'delta']);
  });

  it('opens a new decoder after one fails, and goes on from the next keyframe', () => {
    const h = harness();
    h.live.take(KEY, 1);
    h.current().breaks();

    h.live.take(DELTA, 2);
    h.live.take(KEY, 3);
    h.live.take(DELTA, 4);

    expect(h.decoders).toHaveLength(2);
    expect(h.current().decoded).toEqual(['key', 'delta']);
    expect(h.failed()).toBe(0);
  });

  it('ends the view only after decoders fail again and again with nothing drawn', () => {
    const h = harness();
    for (let round = 0; round <= DECODER_RETRIES; round += 1) {
      h.current().breaks();
    }

    expect(h.failed()).toBe(1);
    h.live.take(KEY, 1);
    expect(h.drawn()).toBe(0);
  });

  it('counts the failures again from zero once a picture is drawn', () => {
    const h = harness();
    for (let round = 0; round < 3 * DECODER_RETRIES; round += 1) {
      h.current().breaks();
      h.live.take(KEY, round);
    }

    expect(h.failed()).toBe(0);
  });

  it('says it waits for a keyframe again after a decoder failed', () => {
    let restarts = 0;
    let current: FakeDecoder | undefined;
    const live = new LiveDecoder({
      open: (output, error) => (current = new FakeDecoder(output, error)),
      chunk: (init) => ({ type: init.type }) as EncodedVideoChunk,
      draw: () => undefined,
      fail: () => undefined,
      restart: () => {
        restarts += 1;
      },
    });
    live.take(KEY, 1);
    expect(restarts).toBe(0);

    current?.breaks();
    expect(restarts).toBe(1);
  });

  it('ignores a failed decoder that reports after it was replaced', () => {
    const h = harness();
    const first = h.current();
    first.breaks();
    first.error(new Error('late'));

    expect(h.decoders).toHaveLength(2);
  });
});
