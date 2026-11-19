<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import type { RunningTransferRow } from '../../data/running-transfers.svelte';
  import { START_PRINT_VERB } from '../../data/services/print';
  import type { Device, PrintOutcome, PrintRequest } from '../../domain';
  import { STATE_PATH } from '../../domain';
  import { fakeDevice } from '../../test/fakes';
  import PrintDialog from './print-dialog.svelte';
  import type { PrintableFile } from './types';

  const { Story } = defineMeta({
    title: 'Shell/PrintDialog',
    component: PrintDialog,
  });

  const printer = (
    id: string,
    name: string,
    state: string,
    model = 'MK4IS',
  ): Device => ({
    ...fakeDevice(id, name),
    capabilities: [START_PRINT_VERB],
    connected: state !== 'OFFLINE',
    model,
    state: { [STATE_PATH]: state },
  });

  // A farm with several free machines, one of them another model.
  const farm: Device[] = [
    printer('f0', 'MK4S #1042', 'IDLE'),
    printer('f1', 'MK4S #1043', 'IDLE'),
    printer('f2', 'MK4S #1044', 'IDLE'),
    printer('f3', 'XL #7', 'IDLE', 'XL'),
    printer('f4', 'MK4S #1045', 'PRINTING'),
  ];

  const devices: Device[] = [
    printer('p0', 'MK4S #1042', 'IDLE'),
    printer('p1', 'MK4S #1043', 'PRINTING'),
    printer('p2', 'CoreOne #7', 'OFFLINE', 'COREONE'),
  ];

  const file: PrintableFile = {
    details: {
      estimated_seconds: 7_260,
      filament_g: 31.4,
      material: 'PLA',
      printer_model: 'MK4IS',
    },
    name: 'bracket_left_v5.bgcode',
    source: 'models/bracket_left_v5.bgcode',
    storage: 'local',
  };

  // The file on its way to two of the farm's printers: the bytes still moving
  // to #1042, and every byte out to #1043, which is writing it to its drive.
  const onItsWay = (
    deviceId: string,
    bytesSent: number,
  ): RunningTransferRow => ({
    transferId: `transfer-${deviceId}`,
    deviceId,
    printerName: deviceId,
    storage: 'local',
    source: file.source,
    path: `usb/${file.name}`,
    bytesTotal: 3_145_728,
    bytesSent,
    state: 'sending',
    mine: true,
    startedBy: undefined,
    canStop: true,
  });
  const transfers = [onItsWay('f0', 1_415_577), onItsWay('f1', 3_145_728)];

  const started: PrintOutcome = {
    commandId: 'cmd-1',
    faultCode: undefined,
    machineReason: undefined,
    ok: true,
    path: 'bracket_left_v5.bgcode',
    transferId: 'transfer-1',
  };

  const refused: PrintOutcome = {
    commandId: 'cmd-2',
    faultCode: 'core.printer/COMMAND_REFUSED',
    machineReason: 'the printer is busy with another job',
    ok: false,
    path: 'bracket_left_v5.bgcode',
    transferId: undefined,
  };
</script>

<!-- The ordinary case: one printer is waiting, the others say why they are
     not offered. -->
<Story name="Default" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    {file}
    {devices}
    onPrint={() => Promise.resolve(started)}
  />
</Story>

<!-- The free printer answered with no USB drive, so the file has nowhere to
     land on it. -->
<Story name="A Printer With No USB Drive" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    {file}
    {devices}
    noStorage={['p0']}
    onPrint={() => Promise.resolve(started)}
  />
</Story>

<!-- The file said nothing about the print, so there are no facts to draw. -->
<Story name="A File That Said Nothing" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    file={{ ...file, details: undefined }}
    {devices}
    onPrint={() => Promise.resolve(started)}
  />
</Story>

<!-- Every machine is busy or away, so there is nothing to send to. -->
<Story name="No Printer Is Free" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    {file}
    devices={devices.slice(1)}
    onPrint={() => Promise.resolve(started)}
  />
</Story>

<!-- The printer answered and did not take the command. -->
<Story name="The Printer Refused" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    {file}
    {devices}
    onPrint={() => Promise.resolve(refused)}
  />
</Story>

<!-- Several printers are free, so the reader ticks the ones the file goes to.
     The XL is offered with a line saying the file was sliced for another model. -->
<Story name="Several Printers Are Free" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    {file}
    devices={farm}
    onPrint={() => Promise.resolve(started)}
  />
</Story>

<!-- Tick #1042 and #1043 and press Print: #1042 starts, #1043 refuses. The
     panel stays open naming the one that failed and why, with it still ticked. -->
<Story name="One Of Several Refused" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    {file}
    devices={farm}
    onPrint={(request: PrintRequest) =>
      Promise.resolve(request.deviceId === 'f1' ? refused : started)}
  />
</Story>

<!-- The file on its way: #1042 shows how far the bytes got, and #1043, which
     has every byte, says the printer is writing the file to its drive. -->
<Story name="On Its Way To The Printers" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    {file}
    devices={farm}
    {transfers}
    onPrint={() => Promise.resolve(started)}
  />
</Story>

<Story name="Choose from a large farm" asChild>
  <PrintDialog
    open
    onOpenChange={() => undefined}
    {file}
    devices={[
      ...farm,
      ...Array.from({ length: 5 }, (_, index) =>
        printer(
          `extra-${String(index)}`,
          `CoreOne #${String(index + 1)}`,
          'IDLE',
          'COREONE',
        ),
      ),
    ]}
    {transfers}
    onPrint={() => Promise.resolve(started)}
  />
</Story>
