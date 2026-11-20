// Which carriage a camera's H.264 live view tries first on this page.
//
// A browser that could not reach the server over WebRTC once cannot reach it
// for any camera: the server's candidates are the same for all of them. A
// printers page of twelve cameras would otherwise wait out the connect
// deadline twelve times before each one fell back. The page forgets on
// reload, so a network that changed is tried again.

let unreachable = false;

/** The carriage a new live view tries first. */
export const carriageFirst = (): 'webrtc' | 'socket' =>
  unreachable ? 'socket' : 'webrtc';

/** Notes that WebRTC did not connect, so the next view goes to the socket. */
export const rememberUnreachable = (): void => {
  unreachable = true;
};

/** Forgets what the page learned. For tests, which share one module. */
export const forgetCarriage = (): void => {
  unreachable = false;
};
