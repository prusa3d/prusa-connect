<!--
  A camera picture whose camera goes away while its live view plays, as a
  dashboard tile's does when the camera is removed or the list reloads. The
  test cannot write the `{#if}` that drops the camera itself, nor the
  transition that tears the picture down after it.
-->
<script lang="ts">
  import { fade } from 'svelte/transition';

  import type { Camera } from '../data';
  import CameraPicture from '../shell/printer-cameras/camera-picture.svelte';
  import type {
    CameraPictures,
    CameraSocket,
    CameraWatch,
  } from '../shell/printer-cameras/types';

  let {
    camera,
    pictures,
    watch,
    socket,
  }: {
    camera: Camera;
    pictures: CameraPictures;
    watch?: CameraWatch | undefined;
    socket?: CameraSocket | undefined;
  } = $props();

  // What the boundary caught, as the app's boundary would show it.
  let caught = $state<string | undefined>(undefined);

  // The button overwrites it, as the tile's list does when it is read again.
  let feed = $derived<{ camera: Camera; pictures: CameraPictures } | undefined>(
    { camera, pictures },
  );
</script>

<button
  type="button"
  onclick={() => {
    feed = undefined;
  }}>Remove the camera</button
>
<svelte:boundary
  onerror={(error) => {
    caught = error instanceof Error ? error.message : String(error);
  }}
>
  {#if feed !== undefined}
    <!-- Faded out, so the picture is torn down in a later update than the
         one that dropped its camera, as a dashboard grid's tile is. -->
    <div out:fade={{ duration: 20 }}>
      <CameraPicture {...feed} alt="The camera" {watch} {socket} playing />
    </div>
  {/if}
  {#snippet failed()}
    <p data-caught>{caught}</p>
  {/snippet}
</svelte:boundary>
