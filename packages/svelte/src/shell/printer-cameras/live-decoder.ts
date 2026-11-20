// Feeds the live socket's pictures to a WebCodecs decoder, and survives the
// decoder failing.
//
// A camera on a poor link sends pictures with pieces missing, and the server
// drops what it cannot read. WebRTC's decoder hides a damaged picture. A
// WebCodecs decoder closes itself on the first one it cannot decode, so this
// opens a new decoder and waits for the next keyframe, as a viewer who just
// joined would. Only a camera whose pictures fail again and again with
// nothing drawn between them ends the view.

import { codecOf, type SocketPicture } from './h264';

/** The part of a WebCodecs decoder this uses, so a test can pass its own. */
export interface PictureDecoder {
  readonly state: CodecState;
  readonly decodeQueueSize: number;
  configure(config: VideoDecoderConfig): void;
  decode(chunk: EncodedVideoChunk): void;
  close(): void;
}

/** Opens a decoder that reports to these two callbacks. */
export type OpenDecoder = (
  output: (frame: VideoFrame) => void,
  error: (cause: unknown) => void,
) => PictureDecoder;

/** Builds the chunk a decoder takes. A test passes one that needs no browser. */
export type ChunkOf = (init: EncodedVideoChunkInit) => EncodedVideoChunk;

/**
 * How many decoders may fail in a row, with no picture drawn between them,
 * before the view ends. Every keyframe is another chance, so five is about
 * ten seconds of a camera sending nothing that decodes.
 */
export const DECODER_RETRIES = 5;

/**
 * How many pictures may wait in the decoder before the ones that are not
 * keyframes are dropped: about a second at the frame rates cameras run at.
 */
export const BEHIND = 30;

export class LiveDecoder {
  readonly #open: OpenDecoder;
  readonly #chunk: ChunkOf;
  readonly #draw: (frame: VideoFrame) => void;
  readonly #fail: () => void;
  readonly #restart: () => void;

  #decoder: PictureDecoder;
  #configured = false;
  #skipping = false;
  #failures = 0;
  #ended = false;

  constructor(options: {
    open: OpenDecoder;
    chunk: ChunkOf;
    /** Draws one decoded frame. The frame is closed after it returns. */
    draw: (frame: VideoFrame) => void;
    /** Called once, when the camera's pictures no longer decode at all. */
    fail: () => void;
    /** Called when a decoder failed and the next keyframe is awaited. */
    restart?: () => void;
  }) {
    this.#open = options.open;
    this.#chunk = options.chunk;
    this.#draw = options.draw;
    this.#fail = options.fail;
    this.#restart = options.restart ?? (() => undefined);
    this.#decoder = this.#fresh();
  }

  /** Decodes one picture off the socket. */
  take(picture: SocketPicture, now: number): void {
    if (this.#ended) return;
    if (this.#decoder.state === 'closed' && !this.#lost()) return;

    if (!this.#configured) {
      if (!picture.key) return;
      const codec = codecOf(picture.unit);
      if (codec === undefined) {
        this.#lost();

        return;
      }
      // No description, so the decoder reads the pictures as Annex B.
      this.#decoder.configure({ codec, optimizeForLatency: true });
      this.#configured = true;
    }

    if (this.#decoder.decodeQueueSize > BEHIND) this.#skipping = true;
    if (this.#skipping && !picture.key) return;
    this.#skipping = false;

    try {
      this.#decoder.decode(
        this.#chunk({
          type: picture.key ? 'key' : 'delta',
          timestamp: Math.round(now * 1000),
          data: picture.unit,
        }),
      );
    } catch {
      this.#lost();
    }
  }

  /** Ends the decoding. Nothing is reported after this. */
  close(): void {
    this.#ended = true;
    if (this.#decoder.state !== 'closed') this.#decoder.close();
  }

  // A failed decoder can still report after it was replaced, so each one
  // reports only while it is the current one.
  #fresh(): PictureDecoder {
    const decoder: PictureDecoder = this.#open(
      (frame) => {
        try {
          if (decoder !== this.#decoder || this.#ended) return;
          this.#failures = 0;
          this.#draw(frame);
        } finally {
          frame.close();
        }
      },
      () => {
        if (decoder === this.#decoder) this.#lost();
      },
    );

    return decoder;
  }

  // A decoder that failed is closed by WebCodecs itself. A new one starts
  // from the next keyframe. It reports whether the view goes on.
  #lost(): boolean {
    if (this.#ended) return false;
    this.#failures += 1;
    if (this.#failures > DECODER_RETRIES) {
      this.close();
      this.#fail();

      return false;
    }
    if (this.#decoder.state !== 'closed') this.#decoder.close();
    this.#decoder = this.#fresh();
    this.#configured = false;
    this.#skipping = false;
    this.#restart();

    return true;
  }
}
