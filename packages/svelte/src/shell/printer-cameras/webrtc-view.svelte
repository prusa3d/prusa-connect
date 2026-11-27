<!--
  One camera's H.264 live view, which the server relays over WebRTC. It
  connects when it is drawn. When it goes the connection is held a while, and
  a view of the camera drawn in that time takes it over (live-sessions.ts), so
  a live view runs only while somebody watches it or just did.
-->
<script lang="ts">
  import { onMount } from 'svelte';

  import { keepFrame } from './last-frames';
  import type { CameraWatch } from './types';
  import { WebrtcSession, webrtcSessions } from './webrtc-session';

  let {
    camera,
    label,
    watch,
    onFail,
    onPicture,
  }: {
    camera: string;
    /** What a screen reader says the video is. */
    label: string;
    watch: CameraWatch;
    /** Called once when the live view could not start. */
    onFail: () => void;
    /** Told whether a moving picture is on screen. */
    onPicture: (showing: boolean) => void;
  } = $props();

  let video = $state<HTMLVideoElement | undefined>(undefined);
  // The video that played, held apart from the binding, which is cleared
  // before the teardown that keeps its frame runs.
  let played: HTMLVideoElement | undefined;

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

    const session = webrtcSessions.take(name) ?? new WebrtcSession(name, watch);
    session.adopt({
      fail,
      stream: (stream) => {
        if (video !== undefined) video.srcObject = stream;
      },
    });

    return () => {
      ended = true;
      session.release();
      if (played !== undefined && played.videoWidth > 0) {
        keepFrame(name, played, played.videoWidth, played.videoHeight);
      }
      if (played !== undefined) played.srcObject = null;
      told(false);
      webrtcSessions.hold(name, session);
    };
  });
</script>

<video
  bind:this={video}
  class="absolute inset-0 size-full object-contain"
  aria-label={label}
  autoplay
  muted
  playsinline
  onplaying={(event) => {
    played = event.currentTarget;
    onPicture(true);
  }}
></video>
