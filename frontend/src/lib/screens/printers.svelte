<!--
  This product's printers page: the library's screen, fed the server's listing,
  this product's cameras and this product's destinations.
-->
<script lang="ts">
  import {
    DeviceAdministration,
    FLEET_REFRESH_FLOOR_MS,
  } from '@prusa/connect-ui/data';
  import { FEATURES, homeOn } from '@prusa/connect-ui/domain';
  import {
    type CameraFeed,
    NearbyPrintersPanel,
    PrintersScreen,
  } from '@prusa/connect-ui/shell';
  import { SlotOutlet } from '@prusa/connect-ui/slots';
  import { onMount } from 'svelte';

  import { watchCameras } from '../cameras';
  import { client } from '../client';
  import { PATHS, printersTaggedHref, tagsOf } from '../routes';
  import type { AppContext } from '../screens';

  let { ctx }: { ctx: AppContext } = $props();

  const fleet = $derived(ctx.fleet);

  // The page of printers the server cut. The fleet store hears the live
  // stream and hands every event on, so the page follows it.
  const listing = $derived(ctx.fleetListing());

  $effect(() => {
    const current = listing;
    const stop = fleet.subscribe((event) => {
      current.hear(event);
    });

    return () => {
      stop();
      current.close();
    };
  });

  // The chosen tags are the address's, so a copied link opens the same list.
  const chosen = $derived(tagsOf(ctx.search));

  // Every camera this person may see, in one request. A tile asks the store
  // for the address of a frame and the browser loads it.
  const cameras = $derived(ctx.cameras);

  // The whole fleet, for how many printers there are. The frame builds the
  // store and never loads it, because the screen that draws the devices is
  // the one that needs them.
  $effect(() => {
    void fleet.load();
  });

  $effect(() => {
    void ctx.deviceIssues.read();
  });

  // The names of the models a driver declared, for a card with no artwork.
  onMount(() => {
    void ctx.deviceModels.read();
  });

  $effect(() => {
    void cameras.load();
  });

  // The printers on the network that are not here yet. A printer joins and
  // leaves that list with no event, so the page reads it again on a tick.
  const nearby = $derived(ctx.nearby);

  $effect(() => {
    void nearby.load();

    return nearby.listen();
  });

  // The tick stops while this tab is hidden. Reading `tick` through
  // `snapshotUrl` is what makes the address change with it.
  $effect(() => watchCameras(cameras, FLEET_REFRESH_FLOOR_MS));
  $effect(() => cameras.follow(fleet));

  // One camera per printer: the first the server lists for it. A printer with
  // none answers undefined, and the tile draws its own artwork.
  const snapshots = {
    feedFor: (deviceId: string): CameraFeed | undefined => {
      const found = cameras.serving.find((row) => row.device_id === deviceId);
      if (found === undefined) return undefined;

      return {
        camera: found,
        pictures: {
          still: cameras.stillUrl(found),
          live: found.stream ? cameras.streamUrl(found.name) : undefined,
        },
        watch: (camera, offer, signal) => cameras.watch(camera, offer, signal),
      };
    },
  };

  // A retired printer leaves the page once the server has taken it, so the
  // page is read again then.
  const actions = {
    retire: async (deviceId: string): Promise<void> => {
      const store = listing;
      if (await new DeviceAdministration(client, deviceId).retire()) {
        store.setQuery({});
        void fleet.load();
      }
    },
  };
</script>

<!-- The device's record decides whether a line with a devices selector is
     drawn on this card (ADR-20261110-5np). -->
{#snippet cardLine(deviceId: string)}
  <SlotOutlet
    screen="printers"
    name="device-card"
    {deviceId}
    device={fleet.device(deviceId)}
  />
{/snippet}

{#snippet nearbyPanel()}
  <NearbyPrintersPanel source={nearby} addHref={PATHS.printersAdd} />
{/snippet}

<PrintersScreen
  {listing}
  thumbnailFor={ctx.deviceThumbnail}
  issues={ctx.deviceIssues}
  printerCount={fleet.devices.length}
  tags={FEATURES.tags
    ? {
        chosen,
        onChange: (next: string[]) => {
          ctx.go(printersTaggedHref(next), { replace: true });
        },
      }
    : undefined}
  cameras={snapshots}
  preference={homeOn(ctx.preferences.home, ctx.settings.dashboards)}
  onAdd={() => {
    ctx.go(PATHS.printersAdd);
  }}
  {actions}
  {cardLine}
  nearby={nearbyPanel}
/>
