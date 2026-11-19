<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import { fakeDevice } from '../../test/fakes';
  import PrinterDetailScreen from './printer-detail-screen.svelte';
  import type { PrinterAdministration } from './info/types';
  import type { TelemetrySource } from './types';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Printer Detail',
    component: PrinterDetailScreen,
  });

  const fakePrinter = (
    id: string,
    name: string,
    model: string,
    state: string,
  ) => ({
    ...fakeDevice(id, name),
    model,
    capabilities: [
      'core.command.load_filament',
      'core.command.unload_filament',
    ],
    state: { '/state': state },
  });

  const READINGS: Readonly<Record<string, number | string>> = {
    '/tools/0/nozzle_temp': 214.6,
    '/tools/0/nozzle_target_temp': 215,
    '/temperature/bed': 59.8,
    '/temperature/bed_target': 60,
    '/tools/0/material_package_instance/package/material/abbreviation': 'PLA',
    '/speed': 100,
    '/flow': 95,
  };

  const telemetry: TelemetrySource = {
    value: (path) => READINGS[path],
    points: () => [],
  };

  const printer = fakePrinter('p0', 'MK4S #1042', 'MK4S', 'PRINTING');
  const peer = {
    device: fakePrinter('p1', 'CORE One #7', 'CORE One', 'IDLE'),
    side: 'after' as const,
  };

  // A store that takes every write at once.
  const administration: PrinterAdministration = {
    pending: () => false,
    error: undefined,
    failed: undefined,
    tags: (record) => record,
    editTags: () => Promise.resolve(true),
    rename: () => Promise.resolve(true),
    retire: () => Promise.resolve(true),
  };

  const commands = { onCommand: () => undefined };
  const onTabChange = () => undefined;

  // Two heads in the tool tree: the chips name the active one, and the
  // nozzle editor lists each with its own target.
  const toolChanger = {
    ...fakePrinter('p2', 'XL #12', 'XL', 'PRINTING'),
    capabilities: [
      'core.command.set_nozzle_temperature',
      'core.command.set_nozzle_temperature#tool',
    ],
    state: {
      '/state': 'PRINTING',
      '/tools/1/active': true,
      '/tools/0/enabled': true,
      '/tools/0/nozzle_temp': 170.2,
      '/tools/0/nozzle_target_temp': 170,
      '/tools/0/material_package_instance/package/material/abbreviation': 'PLA',
      '/tools/1/enabled': true,
      '/tools/1/nozzle_temp': 214.6,
      '/tools/1/material_package_instance/package/material/abbreviation':
        'PETG',
    },
  };

  // The live readings of a printer with several tools come from its own
  // record here, so the stream says nothing of a tool.
  const toolTelemetry: TelemetrySource = {
    value: (path) => (path.startsWith('/tools/') ? undefined : READINGS[path]),
    points: () => [],
  };

  // One head with an MMU of five slots, printing from the third.
  const oneHeadWithAFeeder = {
    ...fakePrinter('p3', 'MK4S MMU3 #4', 'MK4S', 'PRINTING'),
    state: {
      '/state': 'PRINTING',
      '/tools/0/active': true,
      '/tools/0/enabled': true,
      '/tools/0/nozzle_temp': 229.8,
      '/tools/0/nozzle_target_temp': 230,
      '/tools/0/feeder/slot_count': 5,
      '/tools/0.2/active': true,
      '/tools/0.0/material_package_instance/package/material/abbreviation':
        'PLA',
      '/tools/0.2/material_package_instance/package/material/abbreviation':
        'ASA',
    },
  };
</script>

{#snippet statusPanel()}
  <p class="text-sm text-muted-foreground">
    The print, the readings and the camera. Filled by its own task.
  </p>
{/snippet}

{#snippet prompt()}
  <p class="rounded-sm border border-border p-4 text-sm">
    The printer is asking whether the first layer went down cleanly.
  </p>
{/snippet}

<Story name="Default" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={printer}
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    panels={{ status: statusPanel }}
    printersHref="/printers"
  />
</Story>

<!-- A read of the drive holds up the printer's other answers. -->
<Story name="Reading The Drive" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={printer}
    {telemetry}
    commands={{ ...commands, readingDrive: true }}
    printerCount={1}
    tab="status"
    {onTabChange}
    panels={{ status: statusPanel }}
    printersHref="/printers"
  />
</Story>

<!-- A caller who may administer the printer gets the pencil beside its name. -->
<Story name="Administrable" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={{ ...printer, actions: ['administer'] }}
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    panels={{ status: statusPanel }}
    {administration}
    printersHref="/printers"
  />
</Story>

<!-- Two printers on one instance: the pair switches in place rather than
     through the listing. -->
<Story name="A Pair" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={printer}
    {telemetry}
    {commands}
    printerCount={2}
    {peer}
    onSelectPeer={() => undefined}
    tab="status"
    {onTabChange}
    panels={{ status: statusPanel }}
    printersHref="/printers"
  />
</Story>

<!-- The header waits in its own shape; the strip is already there, because
     the tabs are known before the record is. -->
<Story name="Loading" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={printer}
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    loading
    printersHref="/printers"
  />
</Story>

<Story name="The Server Refused" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={printer}
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    panels={{ status: statusPanel }}
    fault={{
      code: 'core.device/NOT_FOUND',
      message: 'no device with id p0 on this server',
    }}
    printersHref="/printers"
  />
</Story>

<!-- A machine this instance can no longer hear from reads OFFLINE, whatever
     the last word it sent was. -->
<Story name="A Printer That Is Not Reporting" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={{ ...printer, connected: false }}
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    panels={{ status: statusPanel }}
    printersHref="/printers"
  />
</Story>

<!-- The printer's own question sits between the subject block and the tabs,
     because it outranks whichever tab is open. -->
<Story name="With A Prompt" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={printer}
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    {prompt}
    panels={{ status: statusPanel }}
    printersHref="/printers"
  />
</Story>

<!-- The frame before any tab task has landed: every panel is the skeleton and
     nothing is broken. -->
<Story name="Every Tab Empty" asChild>
  <PrinterDetailScreen
    deviceId={printer.id}
    device={printer}
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    printersHref="/printers"
  />
</Story>

<!-- The path names the printer before the fleet answers, so the trail is
     drawn and the header waits in its own shape. -->
<Story name="The Record Has Not Arrived" asChild>
  <PrinterDetailScreen
    deviceId="p0"
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    loading
    printersHref="/printers"
  />
</Story>

<!-- The fleet answered and holds no such printer, or the read of it was refused.
     Neither a reading on its way nor a fault, so it is named. -->
<Story name="Gone" asChild>
  <PrinterDetailScreen
    deviceId="p0"
    {telemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    printersHref="/printers"
  />
</Story>

<Story name="A Tool Changer" asChild>
  <PrinterDetailScreen
    deviceId={toolChanger.id}
    device={toolChanger}
    telemetry={toolTelemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    panels={{ status: statusPanel }}
    printersHref="/printers"
  />
</Story>

<!-- One head with a feeder: the chips show the slot it prints from. -->
<Story name="One Head With A Feeder" asChild>
  <PrinterDetailScreen
    deviceId={oneHeadWithAFeeder.id}
    device={oneHeadWithAFeeder}
    telemetry={toolTelemetry}
    {commands}
    printerCount={1}
    tab="status"
    {onTabChange}
    panels={{ status: statusPanel }}
    printersHref="/printers"
  />
</Story>
