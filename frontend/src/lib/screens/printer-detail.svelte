<!-- This product's printer page. -->
<script lang="ts">
  import { onMount, type Component, type Snippet } from 'svelte';

  import {
    AccessGrants,
    type Camera,
    Cameras,
    CameraSettings,
    DEVICE_REFRESH_FLOOR_MS,
    DeviceAdministration,
    DeviceCommandLog,
    DeviceCommands,
    DeviceDetail,
    DeviceHistory,
    EventTypes,
    type HistoryClass,
    type SendOptions,
    LiveTelemetry,
    readDeviceGrants,
    thumbnailByLanding,
    type Transfers,
  } from '@prusa/connect-ui/data';
  import {
    STATE_PATH,
    textAt,
    uploadBlockOf,
    writableStorageOf,
  } from '@prusa/connect-ui/domain';
  import {
    type CameraFeed,
    type CameraPictures,
    CommandAnnouncer,
    CommandLog,
    DropChoice,
    isPrinterDetailTab,
    PrinterControlsPanel,
    PrinterDetailScreen,
    PrinterFilesPanel,
    PrinterAccessCard,
    PrinterCamerasPanel,
    PrinterHistoryPanel,
    PrinterInfoPanel,
    PrinterStatusPanel,
    printerHref,
    printTargetOf,
    ReplaceQuestion,
    TransferAnnouncer,
    TransferLog,
  } from '@prusa/connect-ui/shell';
  import {
    getSlotContext,
    moduleTabs,
    SlotActions,
    SlotOutlet,
  } from '@prusa/connect-ui/slots';

  import { watchCameras } from '../cameras';
  import { client } from '../client';
  import { jobsOfPrinterHref } from '../routes';
  import type { AppContext } from '../screens';

  let { ctx }: { ctx: AppContext } = $props();

  const fleet = $derived(ctx.fleet);
  const deviceId = $derived(ctx.params.id ?? '');

  const slots = getSlotContext();
  const materialsNamespace = 'com.github.aux.prusa3d.materials';
  const MaterialsPositions = $derived(
    slots?.registry.component(materialsNamespace, 'printer-positions') as
      | Component<{
          deviceId: string;
          compact: boolean;
          toolId?: string;
          fallback: Snippet;
        }>
      | undefined,
  );
  // A deep link to a module tab never mounts the Status panel, so its own
  // `SlotOutlet` never calls this; this screen must call it itself, or the
  // tab strip would never learn what a module declared.
  onMount(() => {
    void slots?.loader?.activate();
  });

  // A file dropped here may be printed at once, and the print dialog's own
  // rules, which read the device states, decide whether this printer can.
  onMount(() => {
    void ctx.deviceModels.read();
    void ctx.deviceStates.read();
  });

  // The id and not the principal object, so a session read again for the
  // same person keeps the readings gathered so far.
  const principal = $derived(ctx.session.principal?.id);
  const telemetry = $derived(
    new LiveTelemetry(
      client,
      deviceId,
      principal === undefined ? {} : { principal },
    ),
  );
  const commands = $derived(new DeviceCommands(client, deviceId));
  // Read by the History tab when it opens.
  const commandLog = $derived(new DeviceCommandLog(client, deviceId));
  const detail = $derived(new DeviceDetail(client, deviceId));
  // A write is answered once the fleet row carries it, so the page draws what
  // the server now holds. The row is read on its own, because a list read
  // already on its way may have started before the write.
  const administration = $derived(
    new DeviceAdministration(client, deviceId, {
      afterWrite: () => fleet.readDevice(deviceId),
    }),
  );
  const cameras = $derived(new Cameras(client, deviceId));
  // The declared cameras, read by the Cameras tab for each camera's type and
  // controls. After a write the listing is read until it carries every camera
  // this printer declares, and the save does not wait for it: the tab draws a
  // declared camera that is not running yet as such.
  const cameraSettings: CameraSettings = new CameraSettings(client, {
    afterWrite: () => {
      void cameras.loadUntil((items) =>
        cameraSettings
          .of(deviceId)
          .every((declared) => items.some((row) => row.name === declared.name)),
      );

      return Promise.resolve();
    },
  });
  const cameraPrinters = $derived(
    fleet.devices.map((row) => ({
      id: row.id,
      label: row.name === '' ? row.id : row.name,
    })),
  );
  // Who may use this printer, read by the Printer tab's Access card.
  const access = $derived(
    new AccessGrants(client, (grants, signal) =>
      readDeviceGrants(grants, deviceId, signal),
    ),
  );
  // The one transfers store of this printer, and this server's own storage,
  // are shared with the other pages that show them.
  const transfers = $derived(ctx.transfers(deviceId));
  const storage = $derived(ctx.storage);
  const eventTypes = new EventTypes(client);

  // How far back the History tab reaches from the moment it opens.
  const HISTORY_WINDOW_HOURS = 2;

  // Telemetry is stored as samples, and the chart draws those. The log asks
  // only for the classes a person reads as events.
  const LOG_CLASSES = [
    'state',
    'command',
    'audit',
  ] as const satisfies readonly HistoryClass[];

  // The History tab reads through this when it opens, and not before. The
  // store it opened is kept apart, so closing the tab of a printer the page
  // has already left drops that printer's reads.
  let opened: DeviceHistory | undefined;
  const history = $derived(
    new DeviceHistory(client, deviceId, {
      hours: HISTORY_WINDOW_HOURS,
      classes: LOG_CLASSES,
      eventTypes,
    }),
  );

  $effect(() => {
    void fleet.load();
  });

  // What `my-printer` means on the home page: the printer last looked at.
  $effect(() => {
    if (deviceId !== '') ctx.preferences.setPrinter(deviceId);
  });

  $effect(() => telemetry.listen());
  $effect(() => {
    void ctx.deviceIssues.read();
  });

  $effect(() => {
    void detail.load();
    void commands.loadPresets();
  });

  // The Status and Info tabs draw the print on the bed and the one before it,
  // and both move when a print starts or ends.
  $effect(() => detail.watch(fleet));

  // A read of who may use a printer stops when the page leaves that printer.
  $effect(() => {
    const store = access;

    return () => {
      store.close();
    };
  });

  // A command whose outcome is still being read stops being read when the page
  // leaves the printer.
  $effect(() => {
    const store = commands;

    return () => {
      store.abort();
    };
  });

  // The store is shared, so it may already hold rows from an earlier visit.
  // The landing toasts start from this page's own read of the listing. Each
  // printer the page shows starts without one, and a read that answers after
  // the page moved on changes nothing.
  let readBy = $state.raw<Transfers | undefined>();
  $effect(() => {
    const store = transfers;
    let current = true;
    readBy = undefined;
    void store.load().then(() => {
      if (current) readBy = store;
    });
    const close = store.listen();

    return () => {
      current = false;
      close();
    };
  });

  // A file that lands changes a folder on the Files tab. The store calls this
  // where it handles the transfer event, and the folder is read again there.
  // Both stores belong to the printer on screen, and the listener is removed
  // when the page moves to another printer.
  $effect(() => {
    const folders = detail;

    return transfers.onLanded((path) => {
      void folders.changedAt(path);
    });
  });

  $effect(() => {
    void storage.load();
  });

  // The listing is what says whether this printer has a camera at all.
  $effect(() => {
    void cameras.load();
  });

  // Whether the person may declare cameras, which decides the Cameras tab
  // for a printer that has none yet.
  $effect(() => {
    void cameraSettings.load();
  });
  const mayManageCameras = $derived(cameraSettings.allowed === true);

  // The module that offers a camera's type, as the camera types name it.
  const moduleOf = (type: string): string | undefined =>
    cameraSettings.types.find((row) => row.type === type)?.module;

  // A camera's live view is the module's when the module that offers its
  // type registered one (ADR-20261106-km2).
  const viewOf = (camera: Camera): typeof cameraView | undefined => {
    const owner = moduleOf(camera.type);
    const drawn =
      owner !== undefined &&
      (slots?.registry.sections('camera-view') ?? []).some(
        (row) => row.namespace === owner,
      );

    return drawn ? cameraView : undefined;
  };
  const camerasTab = $derived(cameras.items.length > 0 || mayManageCameras);

  // The store refreshes the still frame on its own clock.
  $effect(() => watchCameras(cameras, DEVICE_REFRESH_FLOOR_MS));
  $effect(() => cameras.follow(fleet));

  // The fleet lists the printers that are here now, so a retired printer's
  // record is read on its own once the fleet has answered without it.
  const listed = $derived(fleet.device(deviceId));
  $effect(() => {
    if (fleet.loaded && listed === undefined) void detail.loadRecord();
  });
  const device = $derived(listed ?? detail.record);

  // An accepted command stays pending until the printer reports a new state.
  // The state is read as the status panel reads it: the live reading first.
  $effect(() => {
    const live = telemetry.value(STATE_PATH);
    commands.observe(
      typeof live === 'string' ? live : device && textAt(device, STATE_PATH),
    );
  });

  // The name shown in the header, and the one a file carries while it goes
  // to this printer.
  const printerName = $derived(device?.name ?? deviceId);
  // A retired printer cannot be renamed, tagged or retired again.
  const administrable = $derived(
    device?.retired === undefined ? administration : undefined,
  );

  const asked = $derived(ctx.params.tab ?? 'status');
  // What the devices selector reads of the record (ADR-20261110-5np):
  // whether there is a record, and whether it is resident. A telemetry value
  // that replaces the record then does not build the tab strip and the
  // outlets' entries again.
  const recorded = $derived(device !== undefined);
  const resident = $derived(device?.resident === true);
  const slotDevice = $derived(recorded ? { resident } : undefined);
  // A tab with a devices selector is kept for a printer that meets it.
  // On any other printer, or before the record arrives, a link to it opens
  // Status, like a link to an unknown tab.
  const declaredTabs = $derived(
    slots ? moduleTabs(slots.registry, 'device-tab', slotDevice) : [],
  );
  const tab = $derived(
    isPrinterDetailTab(asked) || declaredTabs.some((row) => row.id === asked)
      ? asked
      : 'status',
  );
  const tabsForModules = $derived(
    declaredTabs.map((row) => ({ ...row, panel: moduleTab })),
  );

  // Only a transfer says which file on this server a file on the printer is.
  // Every other print file's picture is the one the printer reads out of it.
  const plates = $derived(
    thumbnailByLanding(transfers.rows, (name, path) =>
      storage.thumbnailUrl(name, path),
    ),
  );
  const thumbnailFor = $derived(
    (path: string): string | undefined =>
      plates.get(path) ?? detail.thumbnailUrl(path),
  );

  const landings = new TransferAnnouncer();
  const refusals = new CommandAnnouncer();

  $effect(() => {
    landings.observe(deviceId, transfers.rows, readBy === transfers);
  });

  $effect(() => {
    refusals.observe(deviceId, commands.last);
  });

  // `send` answers what became of the verb, so the Files tab can read its
  // folder again and name the writes that did not happen. The port sends
  // through the store of the printer it was built for. A tab that keeps the
  // port through a switch of printer still sends to the first printer.
  const port = $derived.by(() => {
    const store = commands;

    return {
      sending: store.sending,
      pending: store.pending,
      readingDrive: detail.readingDrive,
      onCommand: (
        verb: string,
        args?: Readonly<Record<string, string>>,
        options?: SendOptions,
      ) => store.send(verb, args, options),
    };
  });

  // Where a camera's pictures are read from. The addresses carry the
  // store's tick.
  const picturesOf = (camera: Camera): CameraPictures => ({
    still: cameras.stillUrl(camera),
    live: camera.stream ? cameras.streamUrl(camera.name) : undefined,
  });

  const watch = (
    camera: string,
    offer: string,
    signal: AbortSignal,
  ): Promise<string> => cameras.watch(camera, offer, signal);

  // The Now tab draws the first camera that serves, as the Cameras tab does.
  const feed = $derived.by((): CameraFeed | undefined => {
    const first = cameras.serving[0];
    if (first === undefined) return undefined;

    return {
      camera: first,
      pictures: picturesOf(first),
      watch,
      view: viewOf(first),
    };
  });

  // Exactly two printers is the one shape with a peer.
  const intake = $derived(storage.writable[0]?.name);

  // Whether a dropped file is kept on this server, and where. Off, it goes
  // up as a temporary copy the server deletes once it is on the printer. The
  // choice lasts while the page is open, on every tab of it.
  let keep = $state(false);
  let keptIn = $state<string>();
  let keptFolder = $state('');
  // A storage that is no longer writable falls back to the first one.
  const keptStorage = $derived(
    storage.writable.some((row) => row.name === keptIn) ? keptIn : intake,
  );

  const keepChoice = $derived(
    keptStorage === undefined
      ? undefined
      : {
          keep,
          onKeepChange: (next: boolean) => {
            keep = next;
          },
          storages: storage.writable.map((row) => ({
            name: row.name,
            label: row.display_name,
          })),
          storage: keptStorage,
          onStorageChange: (name: string) => {
            keptIn = name;
            keptFolder = '';
          },
          folder: keptFolder,
          onFolderChange: (path: string) => {
            keptFolder = path;
          },
          pickerAt: storage.pickerPath,
          pickerFolders: storage.pickerFolders,
          pickerFault: storage.pickerError,
          onPickerWalk: (path: string) => {
            void storage.browseFolders(keptStorage, path);
          },
        },
  );

  // The files dropped here, held while the person says whether to print
  // them or only send them. Nothing goes up before they answer.
  let dropped = $state.raw<
    { files: readonly File[]; directory: string } | undefined
  >(undefined);

  const onUpload = $derived(
    intake === undefined
      ? undefined
      : (files: File[], directory: string) => {
          dropped = { files, directory };
        },
  );

  // Sends what was dropped, and prints it once it lands when `print` is set.
  // The dialog closes, and the files are followed where every send is.
  const deliver = (print: boolean): void => {
    const held = dropped;
    dropped = undefined;
    if (held === undefined || intake === undefined) return;
    const kept = keep && keptStorage !== undefined;
    for (const file of held.files) {
      void ctx.sends.send(
        file,
        deviceId,
        printerName,
        kept ? keptStorage : intake,
        held.directory,
        {
          keep: kept ? keptFolder : undefined,
          print,
          onRefused: (ended) => {
            landings.notPrinted(file.name, ended);
          },
        },
      );
    }
  };

  // Whether this printer can start a print now, graded the way the print
  // dialog grades it, while the drop question is open.
  const dropTarget = $derived(
    dropped === undefined || device === undefined
      ? undefined
      : printTargetOf(device, {
          transitions: ctx.deviceStates.transitionsOf([device.kind]),
          noStorage: ctx.printerStorages.noStorage(deviceId) ? [deviceId] : [],
        }),
  );

  // Why nothing can be uploaded to this printer, said where the drop zone
  // would be rather than leaving the space empty. Nothing is said while this
  // server's own storages are still being read; uploadBlockOf decides the rest.
  const uploadBlock = $derived(
    storage.loaded
      ? uploadBlockOf(
          { storages: detail.storages, error: detail.storagesError },
          intake,
        )
      : undefined,
  );

  // The files still going from the browser up to this server, for this
  // printer alone: the shared store also holds every other printer's.
  const uploads = $derived(
    ctx.sends.rows.filter(
      (row) => row.deviceId === deviceId && row.step === 'uploading',
    ),
  );

  const onCancelUpload = (id: string): void => {
    ctx.sends.cancel(id);
  };

  // The Now tab shows no folder, so a file lands at the top of the drive the
  // printer writes to.
  const drive = $derived(
    uploadBlock === undefined
      ? writableStorageOf(detail.storages?.items ?? [])?.name
      : undefined,
  );

  const onDrop = $derived(
    onUpload === undefined || drive === undefined
      ? undefined
      : (files: File[]) => onUpload(files, drive),
  );

  const recheckStorages = (): void => {
    void detail.readStorages({ refresh: true });
  };

  // The chip on the Printer tab locates the command panel for this printer.
  let picked = $state<{ deviceId: string; verb: string }>();
  const pickedVerb = $derived(
    picked?.deviceId === deviceId ? picked.verb : undefined,
  );

  const peer = $derived(
    fleet.devices.length === 2
      ? fleet.devices.find((one) => one.id !== deviceId)
      : undefined,
  );

  // A file sent onto a name the printer holds waits for the person's answer.
  const takenName = $derived(transfers.taken?.path.split('/').pop());
</script>

<!-- Asked for every drop before anything goes up. -->
<DropChoice
  names={dropped?.files.map((file) => file.name)}
  printer={printerName}
  printTarget={dropTarget}
  onConfirm={deliver}
  onCancel={() => {
    dropped = undefined;
  }}
/>

<!-- Asked once for the page, whichever tab sent the file. -->
<ReplaceQuestion
  name={takenName}
  print={transfers.taken?.print}
  onPrintHeld={() => {
    // Nothing is sent, so a failed request is said here as well as a refusal.
    const name = takenName ?? '';
    void transfers.printTaken().then((ended) => {
      if (ended !== undefined) landings.notPrinted(name, ended);
    });
  }}
  onReplace={() => {
    // A transfer that fails is said by its own row; only the start is said here.
    const name = takenName ?? '';
    void transfers.replaceTaken().then((ended) => {
      if (ended?.answer !== undefined) landings.notPrinted(name, ended);
    });
  }}
  onKeep={() => {
    transfers.keepTaken();
  }}
/>

{#snippet materialHeader(fallback: Snippet)}
  {#if MaterialsPositions}
    <svelte:boundary>
      <div data-module={materialsNamespace} class="contents">
        <MaterialsPositions {deviceId} compact {fallback} />
      </div>
      {#snippet failed()}
        {@render fallback()}
      {/snippet}
    </svelte:boundary>
  {/if}
{/snippet}

<PrinterDetailScreen
  filamentPositions={MaterialsPositions ? materialHeader : undefined}
  {deviceId}
  {device}
  {telemetry}
  preference={ctx.preferences.home}
  commands={port}
  printerCount={fleet.devices.length}
  peer={peer === undefined
    ? undefined
    : {
        device: peer,
        side: fleet.devices[0]?.id === deviceId ? 'after' : 'before',
      }}
  onSelectPeer={() => {
    // The asked tab, not the one shown: the other printer may show a tab
    // this one does not have.
    if (peer) ctx.go(printerHref(peer.id, asked));
  }}
  {tab}
  onTabChange={(next: string) => {
    picked = undefined;
    ctx.go(printerHref(deviceId, next));
  }}
  fault={fleet.error ?? (listed === undefined ? detail.recordError : undefined)}
  loading={!fleet.loaded || (listed === undefined && !detail.recordLoaded)}
  administration={administrable}
  printersHref="/printers"
  panels={{
    status,
    controls,
    files,
    cameras: camerasTab ? camerasPanel : undefined,
    info,
    history: historyPanel,
  }}
  moduleTabs={tabsForModules}
  actions={headerActions}
/>

{#snippet status()}
  {#if device}
    <!-- A module's own sections come after the core's. -->
    <div class="flex flex-col gap-6">
      <PrinterStatusPanel
        {device}
        {telemetry}
        commands={port}
        camera={feed}
        issues={ctx.deviceIssues}
        problemHref={printerHref(deviceId, 'history')}
        history={detail}
        recent={detail.recent}
        files={detail}
        {transfers}
        {thumbnailFor}
        historyHref={jobsOfPrinterHref(deviceId)}
        onUpload={onDrop}
        {uploadBlock}
        keep={keepChoice}
        checkingStorages={detail.storagesChecking}
        onRecheck={recheckStorages}
        {uploads}
        {onCancelUpload}
        loading={!fleet.loaded}
      />
      <SlotOutlet
        screen="printer-detail"
        name="device-section"
        deviceId={device.id}
        device={slotDevice}
      />
    </div>
  {/if}
{/snippet}

{#snippet controls()}
  {#key deviceId}
    <PrinterControlsPanel
      {device}
      {telemetry}
      commands={port}
      loading={!fleet.loaded}
      material={MaterialsPositions ? materialCell : undefined}
      advanced={{
        commands,
        requested: tab === 'commands',
        highlight: pickedVerb,
      }}
    />
  {/key}
{/snippet}

{#snippet files()}
  {#if device}
    <!-- Another printer is another storage, so the panel starts again. -->
    {#key deviceId}
      <PrinterFilesPanel
        {device}
        files={detail}
        commands={port}
        {transfers}
        {thumbnailFor}
        {onUpload}
        {uploadBlock}
        keep={keepChoice}
        checkingStorages={detail.storagesChecking}
        onRecheck={recheckStorages}
        {uploads}
        {onCancelUpload}
      />
    {/key}
  {/if}
{/snippet}

{#snippet info()}
  <PrinterInfoPanel
    {device}
    history={detail}
    recent={detail.recent}
    {thumbnailFor}
    administration={administrable}
    onRemoved={() => {
      ctx.go('/printers');
    }}
    jobsHref={jobsOfPrinterHref(deviceId)}
    onPickCommand={(verb: string) => {
      picked = { deviceId, verb };
      ctx.go(printerHref(deviceId, 'commands'));
    }}
    access={accessCard}
    {telemetry}
    commands={port}
    material={MaterialsPositions ? materialCell : undefined}
  />
{/snippet}

<!-- The Tools table assigns a material through the picker of the header chip. -->
{#snippet materialCell(toolId: string, fallback: Snippet)}
  {#if MaterialsPositions}
    <svelte:boundary>
      <div data-module={materialsNamespace} class="contents">
        <MaterialsPositions {deviceId} compact {toolId} {fallback} />
      </div>
      {#snippet failed()}
        {@render fallback()}
      {/snippet}
    </svelte:boundary>
  {/if}
{/snippet}

{#snippet camerasPanel()}
  <PrinterCamerasPanel
    {deviceId}
    {printerName}
    cameras={cameras.items}
    loaded={cameras.loaded}
    error={cameras.error}
    {picturesOf}
    settings={cameraSettings}
    printers={cameraPrinters}
    pushUrlOf={(name: string) =>
      new URL(cameras.pushUrl(name), document.baseURI).href}
    {watch}
    foot={cameraFoot}
    {viewOf}
  />
{/snippet}

{#snippet cameraView(camera: Camera)}
  <SlotOutlet
    screen="printer-detail"
    name="camera-view"
    camera={camera.name}
    namespace={moduleOf(camera.type)}
    class="size-full"
  />
{/snippet}

{#snippet cameraFoot(camera: string)}
  <!-- A module's own section under each camera; one that does not know the
       camera draws nothing. -->
  <SlotOutlet screen="printer-detail" name="camera-section" {camera} />
{/snippet}

{#snippet accessCard()}
  <!-- Keyed, so a move to another printer mounts the card and reads again. -->
  {#key deviceId}
    <PrinterAccessCard
      {deviceId}
      callerActions={device?.actions ?? undefined}
      people={ctx.accounts}
      grants={access}
      self={ctx.session.principal?.id}
    />
  {/key}
{/snippet}

{#snippet historyPanel()}
  <!-- Another printer is another window, so the panel opens again. -->
  {#key deviceId}
    <div class="flex flex-col gap-6">
      <PrinterHistoryPanel
        facts={history.facts}
        samples={history.samples}
        live={telemetry}
        reported={device?.state}
        span={history.span}
        drawn={ctx.preferences.chartMetrics}
        describeType={(type: string) => eventTypes.summaryOf(type)}
        onOpen={(metrics: readonly string[]) => {
          opened = history;
          void history.open(metrics);
        }}
        onClose={() => {
          opened?.close();
        }}
        onDrawnChange={(next: readonly string[]) => {
          ctx.preferences.setChartMetrics(next);
          void history.draw(next);
        }}
        onMore={() => {
          void history.more();
        }}
      />
      <CommandLog source={commandLog} />
      <TransferLog source={transfers} />
    </div>
  {/key}
{/snippet}

{#snippet headerActions()}
  <SlotActions
    name="device-action"
    {deviceId}
    device={slotDevice}
    label={printerName}
  />
{/snippet}

{#snippet moduleTab()}
  {#key deviceId}
    <SlotOutlet
      screen="printer-detail"
      name="device-tab"
      {deviceId}
      device={slotDevice}
      entry={tab}
    />
  {/key}
{/snippet}
