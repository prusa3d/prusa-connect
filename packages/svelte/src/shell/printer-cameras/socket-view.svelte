<!--
  One camera's H.264 live view over the live socket, for a browser that could
  not reach the server over WebRTC. It connects when it is drawn. When it goes
  the connection is held a while, and a view of the camera drawn in that time
  takes it over (live-sessions.ts).

  Each message is one picture, which WebCodecs decodes and the canvas draws.
  A picture that does not decode costs the pictures up to the next keyframe,
  not the view (live-decoder.ts).
-->
<script lang="ts">
  import { onMount } from 'svelte';

  import { keepFrame } from './last-frames';
  import { SocketSession, socketSessions } from './socket-session';

  let {
    camera,
    open,
    label,
    onFail,
    onPicture,
  }: {
    camera: string;
    /** Opens the live socket. */
    open: () => WebSocket;
    /** What a screen reader says the video is. */
    label: string;
    /** Called once when the live view could not start, or stopped. */
    onFail: () => void;
    /** Told whether a moving picture is on screen. */
    onPicture: (showing: boolean) => void;
  } = $props();

  // The session's canvas, which the wrapper holds while this view shows it.
  let canvas = $state<HTMLCanvasElement | undefined>(undefined);
  const holding =
    (shown: HTMLCanvasElement | undefined) =>
    (box: HTMLDivElement): (() => void) | undefined => {
      if (shown === undefined) return undefined;
      box.append(shown);

      // A view that took the canvas over may hold it already.
      return () => {
        if (shown.parentNode === box) shown.remove();
      };
    };
  // Whether the canvas holds a picture from before the decoder last started,
  // which is drawn grayed out until a new one covers it.
  let waiting = $state(true);

  onMount(() => {
    // Read once. A teardown can run after the parent let go of the camera,
    // and reading the prop then reads the parent's camera that is gone.
    const name = camera;
    const told = onPicture;
    let ended = false;
    let failed = false;
    const fail = (): void => {
      if (ended || failed) return;
      failed = true;
      onFail();
    };

    if (typeof VideoDecoder === 'undefined') {
      fail();

      return;
    }

    let session = socketSessions.take(name);
    if (session === undefined) {
      try {
        session = new SocketSession(open);
      } catch {
        fail();

        return;
      }
    }
    const shown = session;
    canvas = shown.canvas;
    shown.adopt({
      fail,
      picture: (showing) => {
        waiting = !showing;
        onPicture(showing);
      },
    });

    return () => {
      ended = true;
      shown.release();
      if (shown.showing) {
        keepFrame(name, shown.canvas, shown.canvas.width, shown.canvas.height);
      }
      told(false);
      socketSessions.hold(name, shown);
    };
  });
</script>

<!-- A canvas is not an image a screen reader names, so the label sits on a
     wrapper that is. The session's canvas goes inside it. -->
<div
  {@attach holding(canvas)}
  class={['absolute inset-0', waiting && 'opacity-60 grayscale']}
  role="img"
  aria-label={label}
></div>
