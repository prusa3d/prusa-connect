<!--
  One camera's picture: its still frame and, once a person presses play, its
  live view over it. Play sits in the middle of the picture while the camera
  can stream, and stop in a corner while it does. The Cameras tab, the
  printer's Now tab, the dashboard's camera tile and the printer list's camera
  view all draw a camera this way.

  With autoplay the live view also starts while the picture is seen and stops
  while it is not (ADR-20261119-pxn). A stop the person pressed, or a view
  that did not start, holds it until they press play.

  An H.264 live view tries WebRTC first and falls back to the live socket
  when WebRTC does not connect. Once one view on the page has fallen back,
  every other one goes to the socket first. A view that stops because it is
  not seen holds its connection for a while (live-sessions.ts).
-->
<script lang="ts">
  import PlayIcon from '@lucide/svelte/icons/play';
  import SquareIcon from '@lucide/svelte/icons/square';
  import { type Snippet, tick } from 'svelte';

  import type { Camera } from '../../data';
  import { t } from '../../i18n';
  import { AspectRatio, Button } from '../../primitives';
  import { carriageFirst, rememberUnreachable } from './carriage';
  import { lastFrame } from './last-frames';
  import { hangUp } from './live-sessions';
  import SocketView from './socket-view.svelte';
  import type { CameraPictures, CameraSocket, CameraWatch } from './types';
  import WebrtcView from './webrtc-view.svelte';
  import { whileSeen } from './while-seen';

  let {
    camera,
    pictures,
    alt,
    watch,
    socket,
    view,
    playing = $bindable(false),
    autoplay = false,
    class: className,
    children,
  }: {
    camera: Camera;
    pictures: CameraPictures;
    /** What a screen reader says the picture is. */
    alt: string;
    watch?: CameraWatch | undefined;
    /** Where the H.264 live view goes when WebRTC does not connect. */
    socket?: CameraSocket | undefined;
    /** A module's own live view of the camera, drawn in place of the core's. */
    view?: Snippet<[Camera]> | undefined;
    /** Whether the live view runs. A stream runs only while somebody asked for it. */
    playing?: boolean;
    /** Play the live view while the picture is on screen in a shown tab. */
    autoplay?: boolean;
    class?: string | undefined;
    /** Drawn over the picture, such as a badge. */
    children?: Snippet;
  } = $props();

  // The last still that loaded. Each one is fetched off screen first, so a
  // camera with no frame yet never shows a broken picture and its alt text.
  let still = $state<string | undefined>(undefined);
  // Derived, so a new pictures object with the same address probes nothing.
  const wanted = $derived(pictures.still);
  $effect(() => {
    const next = wanted;
    if (next === undefined || !camera.serving) {
      still = undefined;

      return;
    }
    // The still is hidden under a playing live view, so it waits for stop.
    if (live) return;
    const probe = new Image();
    probe.onload = () => {
      still = next;
    };
    // A camera with no picture any more answers with none.
    probe.onerror = () => {
      still = undefined;
    };
    probe.src = next;

    return () => {
      probe.onload = null;
      probe.onerror = null;
    };
  });

  // A module's own view says nothing of whether the camera has anything to
  // show, so it is offered once the camera sent a picture.
  const livable = $derived(
    (view !== undefined && still !== undefined) ||
      pictures.live !== undefined ||
      (camera.watch && (watch !== undefined || socket !== undefined)),
  );
  const live = $derived(livable && playing);
  // Whether the last live view did not start, which the picture says until
  // it is played again.
  let stalled = $state(false);
  // Whether the person stopped the live view, or it did not start. Either
  // way it waits for their play, so scrolling never starts it again.
  let held = false;
  const fail = (): void => {
    playing = false;
    stalled = true;
    held = true;
  };

  // Which carriage the H.264 view runs on. It is chosen again on every play,
  // so a press after a failure tries the page's preferred one first.
  let carriage = $derived<'webrtc' | 'socket'>(
    live && watch !== undefined ? carriageFirst() : 'socket',
  );
  const webrtcFailed = (): void => {
    if (socket === undefined) {
      fail();

      return;
    }
    rememberUnreachable();
    carriage = 'socket';
  };

  // Whether an H.264 view has a moving picture on screen. Until it does, the
  // last frame of this camera is shown grayed out, or the still when there is
  // none, so starting a view never blanks the picture.
  let showing = $state(false);
  const onPicture = (now: boolean): void => {
    showing = now;
  };
  const previous = $derived(lastFrame(camera.name));
  const h264 = $derived(view === undefined && pictures.live === undefined);
  const faded = $derived.by((): string | undefined => {
    if (!h264) return undefined;
    if (live) return showing ? undefined : (previous ?? still);
    return still === undefined ? previous : undefined;
  });

  // A view that stops while it is not seen keeps its connection a while, so
  // it plays at once when it is seen again (live-sessions.ts). A stop the
  // person pressed hangs up, once the view has let go of the connection.
  const toggle = (): void => {
    stalled = false;
    playing = !playing;
    held = !playing;
    if (!playing) {
      const name = camera.name;
      void tick().then(() => {
        hangUp(name);
      });
    }
  };

  const follow = (seen: boolean): void => {
    if (!held) playing = seen;
  };
</script>

<AspectRatio
  ratio={16 / 9}
  class={['relative overflow-hidden bg-muted', className]}
  {@attach autoplay ? whileSeen(follow) : undefined}
>
  <!-- The words sit under the pictures, so they show until a picture loads
       over them. A playing live view sits over the still frame: a camera
       that pushes a picture every half minute has an empty stream until its
       next one, and the last still shows through. -->
  {#if still === undefined || live || !camera.serving}
    <p
      class="absolute inset-0 flex items-center justify-center p-4 text-center text-sm text-muted-foreground"
    >
      {#if !camera.serving}
        {t('printerDetailCameras.notRunningNote')}
      {:else if pictures.still === undefined && !livable}
        {t('printerDetailCameras.none')}
      {:else}
        {t('printerDetailCameras.noPicture')}
      {/if}
    </p>
  {/if}
  {#if camera.serving}
    {#if still !== undefined}
      <img
        src={still}
        alt={live ? '' : alt}
        class="absolute inset-0 size-full object-contain"
      />
    {/if}
    {#if faded !== undefined}
      <img
        src={faded}
        alt={live || still !== undefined ? '' : alt}
        class="absolute inset-0 size-full object-contain opacity-60 grayscale"
      />
    {/if}
    {#if live && view !== undefined}
      <div class="absolute inset-0">
        {@render view(camera)}
      </div>
    {:else if live && pictures.live !== undefined}
      <img
        src={pictures.live}
        {alt}
        class="absolute inset-0 size-full object-contain"
        onerror={fail}
      />
    {:else if live && carriage === 'webrtc' && watch !== undefined}
      <WebrtcView
        camera={camera.name}
        label={alt}
        {watch}
        onFail={webrtcFailed}
        {onPicture}
      />
    {:else if live && socket !== undefined}
      <SocketView
        camera={camera.name}
        open={() => socket(camera.name)}
        label={alt}
        onFail={fail}
        {onPicture}
      />
    {/if}
    {#if live}
      <Button
        variant="secondary"
        size="icon-md"
        class="absolute bottom-2 left-2 z-10"
        label={t('printerDetailCameras.stop', { camera: camera.name })}
        aria-pressed="true"
        onclick={toggle}
      >
        <SquareIcon aria-hidden="true" />
      </Button>
    {:else if livable}
      <Button
        variant="secondary"
        size="icon-lg"
        class="absolute top-1/2 left-1/2 z-10 -translate-1/2 shadow-md"
        label={t('printerDetailCameras.play', { camera: camera.name })}
        aria-pressed="false"
        onclick={toggle}
      >
        <PlayIcon aria-hidden="true" />
      </Button>
    {/if}
  {/if}
  {#if stalled}
    <p
      class="absolute inset-x-0 bottom-0 bg-background/80 px-3 py-2 text-sm text-destructive"
    >
      {t('printerDetailCameras.liveFailed')}
    </p>
  {/if}
  {@render children?.()}
</AspectRatio>
