<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import type { FileDescription } from '../../data';
  import { FileView } from '../../slots/file-view';
  import type { ModuleLoader } from '../../slots/loader.svelte';
  import { SlotRegistry } from '../../slots/registry.svelte';
  import DemoFileView from '../../test/demo-file-view.svelte';
  import { readyLoader } from '../../test/fakes';
  import FileDetailScreen from './file-detail-screen.svelte';
  import type { FileDetailEntry, FilePrint } from './types';

  // A module that draws what a .3mf file holds (ADR-20261027-wvj rule 18).
  const viewRegistry = (registered: boolean): SlotRegistry => {
    const registry = new SlotRegistry();
    registry.declare('reader', {
      pages: [{ path: 'reader', title: 'Project reader', hidden: false }],
      file_view: [{ id: 'project', title: 'Project', file_types: ['.3mf'] }],
    });
    if (registered) {
      registry.registerSection('reader', 'file-view', 'project', DemoFileView);
    }

    return registry;
  };

  const loadingLoader = {
    ready: false,
    activate: () => new Promise<void>(() => undefined),
  } as unknown as ModuleLoader;

  const { Story } = defineMeta({
    title: 'Parts/Screen components/File Detail',
    component: FileDetailScreen,
  });

  const onStorage: FileDetailEntry = {
    name: 'Bracket_Left_Reinforced_v4_FINAL.gcode',
    path: 'Bracket_Left_Reinforced_v4_FINAL.gcode',
    size: 19_293_798,
    modified: '2026-08-18T14:02:00Z',
    sha256: '9f2c1ab4d5e6f70819a2b3c4d5e6f70819a2b3c4d5e6f70819a2b3c4d5e6f708',
  };

  const inAFolder: FileDetailEntry = {
    name: 'urgent_bracket.gcode',
    path: 'q3_rush_jobs/urgent_bracket.gcode',
    size: 12_687_769,
    modified: '2026-08-18T09:30:00Z',
  };

  const model: FileDetailEntry = {
    name: 'benchy.3mf',
    path: 'benchy.3mf',
    size: 2_516_582,
    modified: '2026-08-19T07:41:00Z',
  };

  // The printer reports a short name and the name a person reads.
  const onPrinter: FileDetailEntry = {
    name: 'RETRAC~1.BGC',
    display_name: 'retract_tune.bgcode',
    path: 'usb/calibration/retract_tune.bgcode',
    size: 1_153_434,
    modified: '2026-08-18T12:20:00Z',
    read_only: true,
  };

  const printed: FilePrint[] = [
    {
      id: 'prn-rack-3:1',
      started: '2026-08-19T07:02:00Z',
      state: 'FINISHED',
      printerName: 'MK4S — Print Farm Rack 3, Shelf B',
      printerModel: 'MK4S',
      href: '/jobs/prn-rack-3:1',
      printerHref: '/printers/prn-rack-3',
      durationSeconds: 7680,
    },
    {
      id: 'prn-rack-3:9',
      started: '2026-08-17T11:30:00Z',
      state: 'FINISHED',
      printerName: 'MK4S — Print Farm Rack 3, Shelf B',
      printerModel: 'MK4S',
      href: '/jobs/prn-rack-3:9',
      printerHref: '/printers/prn-rack-3',
      durationSeconds: 13_860,
    },
    {
      id: 'prn-core:14',
      started: '2026-08-16T19:04:00Z',
      state: 'ERROR',
      printerName: 'CORE One',
      printerModel: 'CORE One',
      href: '/jobs/prn-core:14',
      printerHref: '/printers/prn-core',
      durationSeconds: 660,
    },
  ];

  // A plate drawn rather than fetched, so the story needs no server.
  const plate =
    'data:image/svg+xml;utf8,' +
    encodeURIComponent(
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120">' +
        '<rect width="120" height="120" fill="%23222"/>' +
        '<rect x="14" y="18" width="40" height="30" fill="%23f90"/>' +
        '<rect x="64" y="18" width="40" height="30" fill="%23f90"/>' +
        '<rect x="30" y="62" width="60" height="40" fill="%23f90"/>' +
        '</svg>',
    );

  const sliced: FileDescription = {
    details: {
      estimated_seconds: 7260,
      filament_mm: 18_420,
      filament_g: 55.2,
      layer_height_mm: 0.2,
      nozzle_mm: 0.4,
      material: 'PLA',
      printer_model: 'MK4S',
    },
    thumbnailUrl: plate,
  };

  // A file for one filament that asks the printer to check what it is.
  const checked: FileDescription = {
    details: {
      ...sliced.details,
      max_layer_z_mm: 48.2,
      bed: { width_mm: 250, depth_mm: 210, height_mm: 220 },
      checks: {
        printer_model: 'MK4S',
        gcode_level: 2,
        features: ['Input shaper'],
        min_firmware: '6.4.0',
      },
    },
    thumbnailUrl: plate,
  };

  // A file sliced for an XL with five toolheads, three of them used.
  const forFiveTools: FileDescription = {
    details: {
      ...sliced.details,
      printer_model: 'XL5IS',
      max_layer_z_mm: 9.9,
      bed: { width_mm: 360, depth_mm: 360, height_mm: 360 },
      checks: {
        printer_model: 'XL',
        gcode_level: 2,
        features: ['Input shaper'],
        firmware: '6.4.1+12026',
      },
      filaments: [
        {
          index: 0,
          used: true,
          material: 'PLA',
          nozzle_mm: 0.4,
          nozzle_temp_c: 225,
          bed_temp_c: 60,
          filament_g: 0.29,
        },
        {
          index: 1,
          used: true,
          material: 'PETG',
          nozzle_mm: 0.4,
          nozzle_temp_c: 250,
          bed_temp_c: 80,
          filament_g: 0.28,
        },
        {
          index: 2,
          used: true,
          material: 'ASA',
          nozzle_mm: 0.4,
          nozzle_temp_c: 260,
          bed_temp_c: 105,
          filament_g: 0.27,
        },
        { index: 3, used: false, material: 'PLA', nozzle_mm: 0.4 },
        { index: 4, used: false, material: 'PETG', nozzle_mm: 0.4 },
      ],
    },
    thumbnailUrl: plate,
  };

  // The server opened the file and it said nothing.
  const unread: FileDescription = {
    details: undefined,
    thumbnailUrl: undefined,
  };

  const folderHref = (path: string) =>
    `/storage/local/folder/${encodeURIComponent(path)}`;

  const acts = {
    print: () => undefined,
    downloadHref: '/api/v1/storages/local/content?path=bracket.gcode',
    remove: () => undefined,
    rename: () => undefined,
  };
</script>

<!-- A sliced file on this server's own storage. -->
<Story name="Default" asChild>
  <FileDetailScreen
    path={onStorage.path}
    entry={onStorage}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={printed}
    description={sliced}
  />
</Story>

<!-- A file for one filament lists what the printer checks and no filament table. -->
<Story name="With Checks" asChild>
  <FileDetailScreen
    path={onStorage.path}
    entry={onStorage}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={printed}
    description={checked}
  />
</Story>

<!-- A file for five filaments lists what each filament needs. -->
<Story name="For Five Tools" asChild>
  <FileDetailScreen
    path={onStorage.path}
    entry={onStorage}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={printed}
    description={forFiveTools}
  />
</Story>

<Story name="In A Folder" asChild>
  <FileDetailScreen
    path={inAFolder.path}
    entry={inAFolder}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={printed.slice(0, 1)}
    description={sliced}
  />
</Story>

<!-- A model no printer takes as it is. Offering to slice it is a module's. -->
<Story name="A Model" asChild>
  <FileDetailScreen
    path={model.path}
    entry={model}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={[]}
    description={unread}
  />
</Story>

<!-- A module draws the Contents tab; the header, the actions and the other
     tabs stay the screen's. -->
<Story name="Module View" asChild>
  <FileDetailScreen
    path={model.path}
    entry={model}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={[]}
    description={unread}
  >
    {#snippet contents(own)}
      <FileView
        fileRef="local:benchy.3mf"
        fallback={own}
        registry={viewRegistry(true)}
        loader={readyLoader()}
      />
    {/snippet}
  </FileDetailScreen>
</Story>

<Story name="Module View Loading" asChild>
  <FileDetailScreen
    path={model.path}
    entry={model}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={[]}
    description={unread}
  >
    {#snippet contents(own)}
      <FileView
        fileRef="local:benchy.3mf"
        fallback={own}
        registry={viewRegistry(false)}
        loader={loadingLoader}
      />
    {/snippet}
  </FileDetailScreen>
</Story>

<!-- The module's bundle loaded and drew nothing, or failed: the screen's own
     part, with one line saying why. -->
<Story name="Module View Not Available" asChild>
  <FileDetailScreen
    path={model.path}
    entry={model}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={[]}
    description={unread}
  >
    {#snippet contents(own)}
      <FileView
        fileRef="local:benchy.3mf"
        fallback={own}
        registry={viewRegistry(false)}
        loader={readyLoader()}
      />
    {/snippet}
  </FileDetailScreen>
</Story>

<Story name="On The Printer's Own Drive" asChild>
  <FileDetailScreen
    path={onPrinter.path}
    entry={onPrinter}
    rootLabel="MK3S+ (Storage Room)"
    rootHref="/printers/prn-mk3s/files"
    folderHref={(path: string) => `/printers/prn-mk3s/files?path=${path}`}
    acts={{
      print: () => undefined,
      downloadHref: '/api/v1/printers/prn-mk3s/files/retract_tune.bgcode',
      remove: () => undefined,
      rename: () => undefined,
    }}
    prints={[
      {
        id: 'prn-mk3s:21',
        started: '2026-08-18T12:41:00Z',
        state: 'STOPPED',
        printerName: 'MK3S+ (Storage Room)',
        printerModel: 'MK3S+',
        href: '/jobs/prn-mk3s:21',
        printerHref: '/printers/prn-mk3s',
        durationSeconds: 240,
      },
    ]}
    description={sliced}
  />
</Story>

<!-- A host that answers no "which prints used this file" query at all has
     no Prints tab. -->
<Story name="Prints Are Not Answered" asChild>
  <FileDetailScreen
    path={onStorage.path}
    entry={onStorage}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    description={sliced}
  />
</Story>

<!-- The facts the storage answered, on a tab of their own. -->
<Story name="Details" asChild>
  <FileDetailScreen
    path={onStorage.path}
    entry={onStorage}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    prints={printed}
    description={sliced}
    tab="details"
  />
</Story>

<!-- A module's tab, after the screen's own (ADR-20261029-dgv). -->
<Story name="Module Tab" asChild>
  <FileDetailScreen
    path={model.path}
    entry={model}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    description={unread}
    tab="reader.runs"
    moduleTabs={[{ id: 'reader.runs', label: 'Runs', panel: runsTab }]}
  />
</Story>

{#snippet runsTab()}
  <p class="text-sm text-muted-foreground">
    A module draws what it knows about this file here.
  </p>
{/snippet}

<!-- The trail and the acts are known from the path. -->
<Story name="Loading" asChild>
  <FileDetailScreen
    path={onStorage.path}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    loading
  />
</Story>

<!-- The listing answered and holds nothing at this path. -->
<Story name="No Such File" asChild>
  <FileDetailScreen
    path={onStorage.path}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
  />
</Story>

<Story name="The Server Refused" asChild>
  <FileDetailScreen
    path={onStorage.path}
    entry={onStorage}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    description={unread}
    fault={{
      code: 'core.storage/NOT_FOUND',
      message: 'no such path on storage local',
    }}
  />
</Story>

<Story name="Printable Without Metadata" asChild>
  <FileDetailScreen
    path={onStorage.path}
    entry={onStorage}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    {acts}
    description={unread}
  />
</Story>

<Story name="Short Address And Display Name" asChild>
  <FileDetailScreen
    path="BRACKE~1.GCO"
    entry={{
      name: 'BRACKE~1.GCO',
      display_name: 'Bracket',
      path: 'BRACKE~1.GCO',
      size: 1,
    }}
    rootLabel="USB"
    rootHref="/storage/local"
    {folderHref}
    acts={{ print: () => undefined }}
    description={unread}
  />
</Story>

<Story name="Read Refused" asChild>
  <FileDetailScreen
    path={onStorage.path}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    fault={{ code: 'forbidden', message: 'This file is private.', status: 403 }}
  />
</Story>

<Story name="Read Failed" asChild>
  <FileDetailScreen
    path={onStorage.path}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    fault={{
      code: 'internal',
      message: 'The file listing failed.',
      status: 500,
    }}
  />
</Story>

<Story name="Connection Failed" asChild>
  <FileDetailScreen
    path={onStorage.path}
    rootLabel="Storage"
    rootHref="/storage/local"
    {folderHref}
    fault={new TypeError('The storage connection failed.')}
  />
</Story>
