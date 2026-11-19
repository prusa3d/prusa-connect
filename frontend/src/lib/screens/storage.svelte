<!-- This product's storage page. -->
<script lang="ts">
  import {
    StorageScreen,
    type StoragePlace,
    type StorageRow,
    type StorageRowFold,
  } from '@prusa/connect-ui/shell';
  import { describeFault, translateFor } from '@prusa/connect-ui/i18n';
  import { ABOVE_ROW_LINK, FaultAlert } from '@prusa/connect-ui/primitives';
  import { fileRefOf, SlotActions } from '@prusa/connect-ui/slots';
  import { namespace as fractalNamespace } from 'fractal-client';
  import { untrack } from 'svelte';

  import { SharedStores } from '../fractal/shared-stores';
  import { APP_NAMESPACE } from '../i18n';
  import {
    printedHref,
    sharedPlaceOf,
    storageFileHref,
    storageHref,
    withSharedPlace,
  } from '../routes';
  import type { AppContext } from '../screens';
  import type { SharedPlace } from '../stores/fractal-stores.svelte';

  let { ctx }: { ctx: AppContext } = $props();

  const t = translateFor(APP_NAMESPACE);

  $effect(() => {
    void ctx.fleet.load();
    void ctx.deviceStates.read();
  });

  // A server in a Fractal network shares folders with the servers below it,
  // and sees what the servers above and below share. Without the module, or
  // with its file sharing off, the page is the same as on any other server.
  const fractal = $derived(
    ctx.modules.items.some(
      (instance) => instance.namespace === fractalNamespace && instance.enabled,
    ),
  );

  $effect(() => {
    if (fractal) void ctx.fractalShares.load();
  });

  const sharing = $derived(
    ctx.fractalShares.on === true ? ctx.fractalShares : undefined,
  );

  // A failed read is not a server without sharing, so it is said. A list
  // read before stays on screen beside it.
  const sharesFault = $derived(
    fractal && ctx.fractalShares.error !== undefined
      ? describeFault(ctx.fractalShares.error)
      : undefined,
  );

  // The dialog needs the moves of the kinds this server holds.
  const transitions = $derived(
    ctx.deviceStates.transitionsOf(
      ctx.fleet.devices.map((device) => device.kind),
    ),
  );

  // The shared folder the address names. Each list keeps its own place in
  // the address, so walking one leaves the other where it is, and the back
  // button steps back through both in the order the reader walked them.
  const sharedPlace = $derived(sharedPlaceOf(ctx.search));

  // Each visit reads the stores again, which shows their top, so the place
  // is walked again once they answer.
  $effect(() => {
    const place = sharedPlace;
    if (sharing !== undefined && ctx.fractalStores.stores.length > 0) {
      untrack(() => void ctx.fractalStores.goTo(place));
    }
  });

  // The storage and folder the address names. The bare storage page names
  // none, and the screen shows the folder the reader last left.
  const address = $derived<StoragePlace | undefined>(
    ctx.params.storage === undefined
      ? undefined
      : { storage: ctx.params.storage, directory: ctx.params.path ?? '' },
  );
</script>

<div class="flex flex-col gap-6">
  <StorageScreen
    storage={ctx.storage}
    {address}
    onNavigate={(place: StoragePlace) => {
      ctx.go(
        withSharedPlace(
          storageHref(place.storage, place.directory),
          sharedPlace,
        ),
      );
    }}
    fileHref={(storage: string, row: StorageRow) =>
      storageFileHref(storage, row.path)}
    {fileActions}
    {sharing}
    devices={ctx.fleet.devices}
    {transitions}
    printerStorages={ctx.printerStorages}
    onPrint={ctx.print}
    onPrinted={(printers: readonly string[]) => {
      ctx.go(printedHref(printers));
    }}
  />

  {#if sharesFault}
    <div class="flex flex-col gap-1" data-part="shares-fault">
      <p class="text-sm font-medium">{t('app.fractal.shares.failed')}</p>
      <FaultAlert fault={sharesFault} />
    </div>
  {/if}

  {#if sharing}
    <SharedStores
      stores={ctx.fractalStores}
      onNavigate={(place: SharedPlace | undefined) => {
        ctx.go(withSharedPlace(ctx.path, place));
      }}
    />
  {/if}
</div>

<!-- A module's file-action entries, on each file's row, under the row's own
     acts. -->
{#snippet fileActions(
  storage: string,
  row: StorageRow,
  fold: StorageRowFold | undefined,
)}
  <SlotActions
    name="file-action"
    fileRef={fileRefOf(storage, row.path)}
    label={row.name}
    class={ABOVE_ROW_LINK}
    items={fold?.items}
    itemsOnlyClass={fold?.shown}
  />
{/snippet}
