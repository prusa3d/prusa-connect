// The cameras a printer declares, and the two URLs a browser loads a frame
// from. A service names the SDK operation and its parameters and keeps no
// state (svelte-data.md).
//
// A frame is an `<img>` or a `<video>` whose `src` the browser loads with the
// page's own cookie, so the SDK builds the URL and the element loads it. No
// `fetch` and no `EventSource` are involved (frontend-structure.md).
import type { PrusaConnect, Operation, OperationRequest } from '@prusa/connect';

/** The cameras this caller may see, as `list-cameras` answers. */
export type CameraPage = Awaited<ReturnType<PrusaConnect['listCameras']>>;

/** One camera: its name, its device, and whether it is serving. */
export type Camera = NonNullable<CameraPage['items']>[number];

/**
 * What builds a URL from an operation. `PrusaConnect.transport` satisfies it,
 * and naming the one method is what lets a test pass a small object.
 */
export interface UrlBuilder {
  urlOf(operation: Operation, request: OperationRequest): string;
}

/** What the cameras store needs of the SDK, and no more. */
export type CamerasClient = Pick<
  PrusaConnect,
  'listCameras' | 'watchCamera'
> & {
  readonly transport: UrlBuilder;
};

// `get-camera-snapshot`, `stream-camera` and `put-camera-snapshot`, as the operation table declares
// them. They are written out here because this package imports the SDK for
// its types alone, and the table is a value.
const SNAPSHOT: Operation = {
  id: 'get-camera-snapshot',
  method: 'GET',
  path: '/api/v1/cameras/{camera}/snapshot',
  response: 'raw',
  anonymous: false,
};

const STREAM: Operation = {
  id: 'stream-camera',
  method: 'GET',
  path: '/api/v1/cameras/{camera}/stream',
  response: 'raw',
  anonymous: false,
};

const LIVE: Operation = {
  id: 'live-camera',
  method: 'GET',
  path: '/api/v1/cameras/{camera}/live',
  response: 'none',
  anonymous: false,
  longRunning: true,
};

const PUSH: Operation = {
  id: 'put-camera-snapshot',
  method: 'PUT',
  path: '/api/v1/cameras/{camera}/snapshot',
  body: 'image/jpeg',
  response: 'none',
  anonymous: false,
};

/** Every camera, or the ones one printer declares. */
export const readCameras = (
  client: CamerasClient,
  deviceId?: string,
): Promise<CameraPage> =>
  client.listCameras(
    deviceId === undefined ? {} : { query: { device: deviceId } },
  );

/**
 * The address of one still frame. `tick` goes onto the query so that a new
 * value gives the browser a new URL, which is what makes it load a new frame
 * instead of the one in its cache.
 */
export const snapshotUrl = (
  client: CamerasClient,
  camera: string,
  tick: number,
): string =>
  client.transport.urlOf(SNAPSHOT, { path: { camera }, query: { tick } });

/** The address a push camera sends its JPEG to, with a PUT and a credential. */
export const pushUrl = (client: CamerasClient, camera: string): string =>
  client.transport.urlOf(PUSH, { path: { camera } });

/**
 * Answers a browser's SDP offer for one camera's H.264 live view, which the
 * server relays over WebRTC. The answer already carries every candidate.
 */
export const watchCamera = (
  client: CamerasClient,
  camera: string,
  offer: string,
  signal?: AbortSignal,
): Promise<string> =>
  client.watchCamera({ path: { camera }, body: offer }, signal);

/** The address of the live view. The browser reads it as a multipart stream. */
export const streamUrl = (client: CamerasClient, camera: string): string =>
  client.transport.urlOf(STREAM, { path: { camera } });

/**
 * The address of the live socket a browser falls back to when WebRTC does not
 * connect. It is the page's own origin with a websocket scheme, because a
 * websocket takes no relative address.
 */
export const socketUrl = (
  client: CamerasClient,
  camera: string,
  base: string,
): string => {
  const url = new URL(client.transport.urlOf(LIVE, { path: { camera } }), base);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';

  return url.href;
};

/**
 * Opens one camera's live socket. The SDK builds the address, as it does for
 * the still frame and the MJPEG stream an element loads. The SDK speaks HTTP
 * and has no socket of its own, so the browser's is opened here, in the one
 * place that may, and with the page's own cookie.
 */
export const openSocket = (
  client: CamerasClient,
  camera: string,
  base: string,
): WebSocket => {
  // eslint-disable-next-line no-restricted-globals
  const socket = new WebSocket(socketUrl(client, camera, base));
  socket.binaryType = 'arraybuffer';

  return socket;
};

// How long a camera says it waits between two frames.
//
// `min_interval_ms` on the published `Camera` schema is the answer, and it is
// milliseconds. A camera that declares no cadence carries no member.
//
// `min_interval` is the driver settings key the core reads that number from,
// and a row is still checked for it. An older core, and a module that answers
// this listing with its own rows, both send the settings spelling, so the
// untyped read is the fallback rather than the first thing tried.
const MIN_INTERVAL = 'min_interval';

const UNIT_MS: Readonly<Record<string, number>> = {
  ms: 1,
  s: 1000,
  m: 60_000,
  h: 3_600_000,
};

// The spelling Go writes a duration in: a number and a unit, repeated. `200ms`
// and `1h30m` are both this shape.
const DURATION_PART = /(\d+(?:\.\d+)?)(ms|s|m|h)/g;

const memberOf = (camera: Camera, name: string): unknown =>
  (camera as unknown as Record<string, unknown>)[name];

/** A wait is only a wait when it is a positive, finite number of milliseconds. */
const waitOf = (value: number): number | undefined =>
  Number.isFinite(value) && value > 0 ? value : undefined;

/**
 * The wait one camera declares, in milliseconds, or undefined when it declares
 * none. `min_interval_ms` answers it, and a row carrying the settings key
 * instead is read too: a number as milliseconds, a string as a Go duration.
 *
 * ```ts
 * declaredIntervalMs({ ...camera, min_interval_ms: 200 }); // 200
 * ```
 */
export const declaredIntervalMs = (camera: Camera): number | undefined => {
  const published = camera.min_interval_ms;
  if (typeof published === 'number') return waitOf(published);

  const given = memberOf(camera, MIN_INTERVAL);

  if (typeof given === 'number') return waitOf(given);
  if (typeof given !== 'string') return undefined;

  let total = 0;
  for (const part of given.matchAll(DURATION_PART)) {
    const value = Number(part[1]);
    const unit = UNIT_MS[part[2] ?? ''];
    if (!Number.isFinite(value) || unit === undefined) return undefined;
    total += value * unit;
  }

  return total > 0 ? total : undefined;
};
