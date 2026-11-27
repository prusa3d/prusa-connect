<!--
  The Cameras tab of a printer's page: one card per camera that watches the
  printer, with its picture and, beside it on a wide card, what the camera is
  and the sections modules draw for it. A person the server lets declare
  cameras also gets the controls: add and remove, the camera's settings beside
  its picture, and the address a push camera sends its pictures to.

  A camera written in the configuration file offers no control, because the
  server refuses a write to it.
-->
<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';

  import {
    type Camera,
    cameraTypeOf,
    type CameraDeclaration,
    type CameraDraft,
  } from '../../data';
  import { describeFault, t } from '../../i18n';
  import {
    AspectRatio,
    Badge,
    Button,
    Card,
    CardAction,
    CardContent,
    CardHeader,
    CardTitle,
    ConfirmDialog,
    EmptyState,
    FaultAlert,
    Skeleton,
  } from '../../primitives';
  import CameraDetails from './camera-details.svelte';
  import CameraDialog from './camera-dialog.svelte';
  import CameraPicture from './camera-picture.svelte';
  import { CAMERA_TYPE_KEYS, type PrinterCamerasPanelProps } from './types';

  let {
    deviceId,
    printerName,
    cameras,
    loaded,
    error,
    picturesOf,
    settings,
    printers,
    pushUrlOf,
    watch,
    socket,
    viewOf,
    foot,
  }: PrinterCamerasPanelProps = $props();

  let adding = $state(false);
  // The camera whose settings were last saved in place, which shows why a
  // save was refused.
  let saved = $state<string>();

  const manage = $derived(settings.allowed === true);
  const busy = $derived(settings.pending !== undefined);
  const fault = $derived(
    settings.error === undefined ? undefined : describeFault(settings.error),
  );
  const taken = $derived(settings.items.map((camera) => camera.name));

  const declarationOf = (name: string): CameraDeclaration | undefined =>
    settings.items.find((camera) => camera.name === name);

  // The listed cameras, and a card for each declared one the server does not
  // run yet, or could not start, so it can still be changed or removed.
  const rows = $derived<readonly Camera[]>([
    ...cameras,
    ...settings
      .of(deviceId)
      .filter((declared) => !cameras.some((row) => row.name === declared.name))
      .map((declared) => ({
        name: declared.name,
        type: declared.type,
        device_id: deviceId,
        formats: [],
        serving: false,
        snapshot: false,
        stream: false,
        watch: false,
      })),
  ]);

  const typeLabel = (camera: CameraDeclaration): string => {
    const type = cameraTypeOf(camera);
    const key = CAMERA_TYPE_KEYS[type];
    if (key !== undefined) return t(key);

    // A module's type is named by the title its module gave it.
    return (
      settings.types.find((row) => row.type === camera.type)?.title ?? type
    );
  };

  const altOf = (camera: string): string =>
    t('printerDetailCameras.alt', { camera, printer: printerName });

  // The cameras whose live view runs. A stream costs the camera, the network
  // and this server while it runs, so it runs only while its card is seen or
  // somebody asked for it.
  const playing = new SvelteSet<string>();
  const play =
    (name: string) =>
    (on: boolean): void => {
      if (on) playing.add(name);
      else playing.delete(name);
    };

  // A camera that goes is forgotten, so one added again under its name
  // starts on its still frame rather than playing.
  $effect(() => {
    for (const name of playing) {
      if (!cameras.some((camera) => camera.name === name)) playing.delete(name);
    }
  });

  // A camera that takes the pictures it is sent: one of push's type, or a
  // generic one with no address of its own to fetch them from.
  const takesPushes = (camera: CameraDeclaration): boolean => {
    const type = cameraTypeOf(camera);

    return (
      type === 'push' ||
      (type === 'generic' && typeof camera.config.snapshot_url !== 'string')
    );
  };

  const add = async (draft: CameraDraft): Promise<void> => {
    saved = undefined;
    if (await settings.save(draft)) adding = false;
  };

  const change = (draft: CameraDraft): void => {
    saved = draft.name;
    void settings.save(draft);
  };

  // A refused save shows where it was made: in the dialog, beside the
  // camera, or above the cameras for a removal.
  const saveFault = $derived(settings.failed === 'save' ? fault : undefined);
</script>

<div class="flex flex-col gap-6">
  {#if manage}
    <div class="flex items-center justify-end gap-4">
      <Button
        variant="secondary"
        size="sm"
        disabled={busy}
        onclick={() => {
          adding = true;
        }}
      >
        {t('printerCameras.add')}
      </Button>
    </div>
    {#if fault && settings.failed !== 'save'}
      <FaultAlert {fault} />
    {/if}
  {/if}

  {#if error !== undefined}
    <FaultAlert fault={describeFault(error)} />
  {:else if !loaded}
    <Card>
      <CardContent>
        <div aria-busy="true" aria-label={t('printerDetailCameras.loading')}>
          <AspectRatio ratio={16 / 9}>
            <Skeleton class="size-full" />
          </AspectRatio>
        </div>
      </CardContent>
    </Card>
  {:else if rows.length === 0}
    <EmptyState
      size="sm"
      heading={t('printerDetailCameras.empty.heading')}
      description={t('printerDetailCameras.empty.note')}
    />
  {/if}

  {#each rows as camera (camera.name)}
    {@const declared = declarationOf(camera.name)}
    {@const moduleView = viewOf?.(camera)}
    {@const writable = manage && declared !== undefined}
    {#snippet removeTrigger({ props }: { props: Record<string, unknown> })}
      <Button {...props} variant="ghost" size="sm" disabled={busy}>
        {t('printerCameras.remove')}
      </Button>
    {/snippet}
    <Card data-part="camera">
      <CardHeader>
        <CardTitle class="flex flex-wrap items-center gap-2">
          <h2 class="contents">{camera.name}</h2>
          {#if declared !== undefined}
            <Badge variant="secondary" data-part="type">
              {typeLabel(declared)}
            </Badge>
          {/if}
          {#if !camera.serving}
            <Badge variant="secondary">
              {t('printerDetailCameras.notServing')}
            </Badge>
          {:else if playing.has(camera.name)}
            <Badge variant="outline">{t('printerDetailCameras.live')}</Badge>
          {/if}
        </CardTitle>
        {#if manage && declared !== undefined}
          <CardAction>
            <ConfirmDialog
              trigger={removeTrigger}
              title={t('printerCameras.removeTitle', { name: camera.name })}
              description={t('printerCameras.removeDetail')}
              confirmLabel={t('printerCameras.remove')}
              onConfirm={() => {
                void settings.remove(declared);
              }}
            />
          </CardAction>
        {/if}
      </CardHeader>
      <CardContent>
        <!-- Picture and facts side by side on a wide card, one above the
             other on a narrow one. -->
        <div class="flex flex-col gap-4 @3xl/card:flex-row">
          <div class="@3xl/card:w-3/5 @3xl/card:shrink-0">
            <CameraPicture
              {camera}
              class="rounded-md"
              pictures={picturesOf(camera)}
              alt={altOf(camera.name)}
              {watch}
              {socket}
              view={moduleView}
              autoplay
              bind:playing={() => playing.has(camera.name), play(camera.name)}
            />
          </div>
          <div class="flex min-w-0 flex-1 flex-col gap-4 text-sm">
            <CameraDetails
              {camera}
              {declared}
              {deviceId}
              pushUrl={manage && declared !== undefined && takesPushes(declared)
                ? pushUrlOf(camera.name)
                : undefined}
              {writable}
              {busy}
              fault={saved === camera.name ? saveFault : undefined}
              onSave={change}
            />
            {@render foot?.(camera.name)}
          </div>
        </div>
      </CardContent>
    </Card>
  {/each}
</div>

<CameraDialog
  open={adding}
  {deviceId}
  {printers}
  types={settings.types}
  {taken}
  {busy}
  fault={adding && saved === undefined ? saveFault : undefined}
  onOpenChange={(next: boolean) => {
    if (!next && !busy) adding = false;
  }}
  onSave={(draft: CameraDraft) => {
    void add(draft);
  }}
/>
