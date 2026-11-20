// One camera's H.264 live view over WebRTC: the connection and the stream it
// receives. It outlives the picture that started it, so a picture that comes
// back plays the stream at once (live-sessions.ts).
//
// The negotiation is one request: the offer goes once every candidate of this
// browser is in it, and the answer carries every candidate of the server. A
// connection that is not up a few seconds after the answer has no candidate
// this browser can reach, and the view says it did not start, so the picture
// can fall back to the live socket rather than wait out the browser's own ICE
// timeout.

import { type LiveSession, SessionPool } from './live-sessions';
import type { CameraWatch } from './types';

// How long the browser may take to find its own candidates. On a network with
// no ICE server it finds its host candidates in well under a second, and an
// offer sent without some of them still connects on the others.
const GATHER_MS = 2000;

// How long the connection may take once the answer is in. On the network the
// camera is on it connects in well under a second.
const CONNECT_MS = 5000;

const gathered = (connection: RTCPeerConnection): Promise<void> =>
  new Promise((resolve) => {
    if (connection.iceGatheringState === 'complete') {
      resolve();

      return;
    }

    const timer = setTimeout(resolve, GATHER_MS);
    connection.addEventListener('icegatheringstatechange', () => {
      if (connection.iceGatheringState === 'complete') {
        clearTimeout(timer);
        resolve();
      }
    });
  });

/** The picture that shows a session, and is told what it does. */
export interface WebrtcViewer {
  /** Called once when the view could not start. */
  fail: () => void;
  /** Handed the stream once it arrives, or at once when it already has. */
  stream: (stream: MediaStream) => void;
}

export class WebrtcSession implements LiveSession {
  readonly #connection = new RTCPeerConnection();
  readonly #ended = new AbortController();
  #deadline: ReturnType<typeof setTimeout> | undefined;
  #stream: MediaStream | undefined;
  #viewer: WebrtcViewer | undefined;

  constructor(camera: string, watch: CameraWatch) {
    const connection = this.#connection;
    connection.addTransceiver('video', { direction: 'recvonly' });
    connection.addEventListener('track', (event) => {
      this.#stream = event.streams[0] ?? new MediaStream([event.track]);
      this.#viewer?.stream(this.#stream);
    });
    connection.addEventListener('connectionstatechange', () => {
      if (connection.connectionState === 'connected') {
        clearTimeout(this.#deadline);
      }
      if (connection.connectionState === 'failed') this.#fail();
    });

    const start = async (): Promise<void> => {
      await connection.setLocalDescription(await connection.createOffer());
      await gathered(connection);
      const offer = connection.localDescription?.sdp ?? '';
      const answer = await watch(camera, offer, this.#ended.signal);
      await connection.setRemoteDescription({ type: 'answer', sdp: answer });
      if (connection.connectionState !== 'connected') {
        this.#deadline = setTimeout(() => {
          this.#fail();
        }, CONNECT_MS);
      }
    };

    start().catch(() => {
      this.#fail();
    });
  }

  get ended(): boolean {
    return this.#ended.signal.aborted;
  }

  /** Tells a picture what the session does from now on. */
  adopt(viewer: WebrtcViewer): void {
    this.#viewer = viewer;
    if (this.#stream !== undefined) viewer.stream(this.#stream);
  }

  /** The picture went. The connection stays up, for the next one. */
  release(): void {
    this.#viewer = undefined;
  }

  close(): void {
    if (this.ended) return;
    clearTimeout(this.#deadline);
    this.#ended.abort();
    this.#connection.close();
  }

  #fail(): void {
    if (this.ended) return;
    const viewer = this.#viewer;
    this.close();
    viewer?.fail();
  }
}

/** The WebRTC views no picture shows, by camera. */
export const webrtcSessions = new SessionPool<WebrtcSession>();
