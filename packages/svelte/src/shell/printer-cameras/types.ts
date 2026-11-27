import type { Snippet } from 'svelte';

import type { Camera, CameraSettings, DataFault } from '../../data';

/**
 * The catalogue key that names each core camera type: the one the app
 * declares a new camera as, and the ones a camera declared before it, or in
 * the configuration file, may carry.
 */
export const CAMERA_TYPE_KEYS: Readonly<Record<string, string>> = {
  generic: 'printerCameras.type.generic',
  push: 'printerCameras.type.push',
  mjpeg: 'printerCameras.type.mjpeg',
  rtsp: 'printerCameras.type.rtsp',
};

/** What the tab reads of the camera settings store. */
export type CameraSettingsSource = Pick<
  CameraSettings,
  | 'allowed'
  | 'error'
  | 'failed'
  | 'items'
  | 'loaded'
  | 'of'
  | 'pending'
  | 'remove'
  | 'save'
  | 'types'
>;

/** One printer a camera may be moved to. */
export interface CameraPrinterChoice {
  readonly id: string;
  readonly label: string;
}

/** The addresses one camera is drawn from. */
export interface CameraPictures {
  /** The still frame, with the tick that makes the browser load a new one. */
  still?: string | undefined;
  /** The MJPEG live view, for a camera that declares one. */
  live?: string | undefined;
}

/** Answers a browser's SDP offer for a camera's H.264 live view with the server's answer. */
export type CameraWatch = (
  camera: string,
  offer: string,
  signal: AbortSignal,
) => Promise<string>;

/**
 * Opens a camera's live socket, which carries its H.264 live view when WebRTC
 * does not connect.
 */
export type CameraSocket = (camera: string) => WebSocket;

/** One camera as its picture draws it, with the live view it can play. */
export interface CameraFeed {
  readonly camera: Camera;
  readonly pictures: CameraPictures;
  readonly watch?: CameraWatch | undefined;
  /** Where the H.264 live view goes when WebRTC does not connect. */
  readonly socket?: CameraSocket | undefined;
  /** A module's own live view, for a camera of a type a module offers. */
  readonly view?: Snippet<[Camera]> | undefined;
}

export interface PrinterCamerasPanelProps {
  /** The printer on screen. A new camera watches it. */
  deviceId: string;
  printerName: string;
  /** The cameras that watch this printer, as the camera listing answered. */
  cameras: readonly Camera[];
  /** The listing has answered at least once. */
  loaded: boolean;
  /** Why the listing did not answer, drawn in place of the cameras. */
  error?: DataFault | undefined;
  /** Where each camera's picture is read from. */
  picturesOf: (camera: Camera) => CameraPictures;
  /**
   * The declared cameras. The tab offers to add, change and remove cameras
   * only when the store says the person may.
   */
  settings: CameraSettingsSource;
  /** Every printer a camera may watch, this one included. */
  printers: readonly CameraPrinterChoice[];
  /** The full address a push camera sends its pictures to. */
  pushUrlOf: (camera: string) => string;
  /**
   * Answers a browser's SDP offer for a camera's H.264 live view. Without it
   * a camera that streams H.264 offers no play button.
   */
  watch?: CameraWatch | undefined;
  /** Where the H.264 live view goes when WebRTC does not connect. */
  socket?: CameraSocket | undefined;
  /**
   * The module's live view of a camera, drawn in place of the core's, for a
   * camera of a type whose module registered one (ADR-20261106-km2).
   */
  viewOf?: ((camera: Camera) => Snippet<[Camera]> | undefined) | undefined;
  /** Drawn beside each camera's picture: the sections modules add for it. */
  foot?: Snippet<[string]> | undefined;
}
