<!--
  One camera as the printer's Cameras tab draws it: the newest still frame,
  refreshed at the pace the camera declares and never faster than the floor
  the library sets, and the live view a person can play over it.

  The picture is an `img` the browser loads with the page's own cookie, so
  nothing here fetches an image. A camera that stopped serving says so rather
  than keeping its last frame, which would read as a picture of now. A list
  of cameras that could not be read shows why, and never says the camera is
  gone, because nobody knows that.
-->
<script lang="ts">
  import { EmptyState, Skeleton, TilePanel } from '../../../primitives';
  import CameraIcon from '@lucide/svelte/icons/camera';
  import CameraOffIcon from '@lucide/svelte/icons/camera-off';
  import CloudOffIcon from '@lucide/svelte/icons/cloud-off';
  import { onMount } from 'svelte';

  import { describeFault, t } from '../../../i18n';
  import { dashboardContext } from '../context';
  import { cameraSettings } from './config';
  import CameraPicture from '../../printer-cameras/camera-picture.svelte';
  import type { CardProps } from '../../../slots';

  let { config, title }: CardProps = $props();

  const { cameras, printers } = dashboardContext();

  const settings = $derived(cameraSettings(config));
  const camera = $derived(
    settings.camera === '' ? undefined : cameras.named(settings.camera),
  );
  const printer = $derived(
    camera === undefined ? undefined : printers.view(camera.device_id),
  );
  const fallback = $derived(
    printer?.name ?? camera?.name ?? t('dashboard.camera.title'),
  );
  const feed = $derived(
    camera === undefined ? undefined : cameras.feed(camera),
  );
  const fault = $derived(
    cameras.error === undefined ? undefined : describeFault(cameras.error),
  );

  onMount(() => {
    cameras.load();
  });

  // The frames tick while this tile is on the page. The timer is outside
  // the reactive graph, which is what an effect is for (svelte-data.md).
  $effect(() => cameras.hold());
</script>

<TilePanel {title} {fallback}>
  {#if settings.camera === ''}
    <EmptyState
      size="sm"
      icon={CameraIcon}
      heading={t('dashboard.camera.pick')}
      description={t('dashboard.camera.pickNote')}
    />
  {:else if !cameras.loaded && cameras.error === undefined}
    <Skeleton class="w-full flex-1" />
  {:else if camera === undefined && fault !== undefined}
    <EmptyState
      size="sm"
      icon={CloudOffIcon}
      heading={t('dashboard.camera.unreadable')}
      description={fault.title}
    />
  {:else if camera === undefined}
    <EmptyState
      size="sm"
      icon={CameraOffIcon}
      heading={t('dashboard.camera.gone')}
      description={t('dashboard.camera.goneNote')}
    />
  {:else if !camera.serving}
    <EmptyState
      size="sm"
      icon={CameraOffIcon}
      heading={t('dashboard.camera.silent')}
      description={t('dashboard.camera.silentNote')}
    />
  {:else if feed !== undefined}
    <!-- Keyed, so another camera starts as a new picture, playing while it
         is seen (ADR-20261119-pxn). -->
    {#key feed.camera.name}
      <CameraPicture
        {...feed}
        alt={t('dashboard.camera.frame', { name: fallback })}
        class="min-h-0 w-full flex-1 rounded-sm"
        autoplay
      />
    {/key}
  {/if}
</TilePanel>
