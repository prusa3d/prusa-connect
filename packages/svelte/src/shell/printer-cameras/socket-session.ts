// One camera's live view over the live socket: the socket, the decoder, and
// the canvas it draws on. It outlives the picture that started it, so a
// picture that comes back takes over the canvas as it was (live-sessions.ts).

import { pictureOf } from './h264';
import { LiveDecoder } from './live-decoder';
import { type LiveSession, SessionPool } from './live-sessions';

/** The picture that shows a session, and is told what it does. */
export interface SocketViewer {
  /** Called once when the view could not start, or stopped. */
  fail: () => void;
  /** Told whether a moving picture is on the canvas. */
  picture: (showing: boolean) => void;
}

export class SocketSession implements LiveSession {
  /** Where the pictures are drawn. The view that shows it puts it on the page. */
  readonly canvas: HTMLCanvasElement;
  readonly #socket: WebSocket;
  readonly #decoder: LiveDecoder;
  #ended = false;
  #showing = false;
  #viewer: SocketViewer | undefined;

  /** Opens the socket. Throws when it could not be opened. */
  constructor(open: () => WebSocket) {
    this.canvas = document.createElement('canvas');
    this.canvas.className = 'size-full object-contain';
    this.#decoder = new LiveDecoder({
      open: (output, error) => new VideoDecoder({ output, error }),
      chunk: (init) => new EncodedVideoChunk(init),
      draw: (frame) => {
        this.#draw(frame);
      },
      fail: () => {
        this.#fail();
      },
      restart: () => {
        this.#show(false);
      },
    });
    try {
      this.#socket = open();
    } catch (cause) {
      this.#decoder.close();
      throw cause;
    }
    this.#socket.addEventListener('message', (event: MessageEvent<unknown>) => {
      if (!(event.data instanceof ArrayBuffer)) return;
      const picture = pictureOf(event.data);
      if (picture !== undefined) this.#decoder.take(picture, performance.now());
    });
    const fail = (): void => {
      this.#fail();
    };
    this.#socket.addEventListener('close', fail);
    this.#socket.addEventListener('error', fail);
  }

  get ended(): boolean {
    return this.#ended;
  }

  /** Whether the canvas holds a picture drawn since the decoder last started. */
  get showing(): boolean {
    return this.#showing;
  }

  /** Tells a picture what the session does from now on. */
  adopt(viewer: SocketViewer): void {
    this.#viewer = viewer;
    viewer.picture(this.#showing);
  }

  /** The picture went. The session goes on drawing, for the next one. */
  release(): void {
    this.#viewer = undefined;
  }

  close(): void {
    if (this.#ended) return;
    this.#ended = true;
    this.#socket.close();
    this.#decoder.close();
  }

  #draw(frame: VideoFrame): void {
    const drawing = this.canvas.getContext('2d');
    if (!drawing) return;
    if (this.canvas.width !== frame.displayWidth) {
      this.canvas.width = frame.displayWidth;
    }
    if (this.canvas.height !== frame.displayHeight) {
      this.canvas.height = frame.displayHeight;
    }
    drawing.drawImage(frame, 0, 0);
    this.#show(true);
  }

  #show(showing: boolean): void {
    if (this.#showing === showing) return;
    this.#showing = showing;
    this.#viewer?.picture(showing);
  }

  #fail(): void {
    if (this.#ended) return;
    const viewer = this.#viewer;
    this.close();
    viewer?.fail();
  }
}

/** The socket views no picture shows, by camera. */
export const socketSessions = new SessionPool<SocketSession>();
