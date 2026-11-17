<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import type { Device, StateTransition } from '../../../domain';
  import { fakeDevice } from '../../../test/fakes';
  import type { TelemetrySource } from '../types';
  import PrinterControlsPanel from './printer-controls-panel.svelte';

  const { Story } = defineMeta({
    title: 'Parts/Screen components/Printer Detail/Controls',
    component: PrinterControlsPanel,
  });

  const EVERY_VERB = [
    'core.command.beep',
    'core.command.cancel_printer_ready',
    'core.command.disable_steppers',
    'core.command.home',
    'core.command.mesh_bed_leveling',
    'core.command.move',
    'core.command.move_z',
    'core.command.reset_printer',
    'core.command.set_bed_temperature',
    'core.command.set_chamber_fan_target',
    'core.command.set_chamber_led_intensity',
    'core.command.set_flow',
    'core.command.set_hostname',
    'core.command.set_idle',
    'core.command.set_nozzle_temperature',
    'core.command.set_printer_ready',
    'core.command.set_speed',
    'core.command.set_tool_hardened',
    'core.command.set_tool_high_flow',
    'core.command.set_tool_nozzle_diameter',
  ];

  // A machine with no pad and no bed levelling.
  const FEW_VERBS = [
    'core.command.set_nozzle_temperature',
    'core.command.set_bed_temperature',
    'core.command.beep',
  ];

  const printer = (state: string, capabilities = EVERY_VERB): Device => ({
    ...fakeDevice('p0', 'MK4S #1042'),
    model: 'MK4S',
    capabilities,
    state: { '/state': state },
  });

  const readings = (
    over: Readonly<Record<string, number | string>> = {},
  ): TelemetrySource => {
    const values: Readonly<Record<string, number | string>> = {
      '/axis/x': 120.5,
      '/axis/y': 90.25,
      '/axis/z': 4.2,
      '/flow': 95,
      '/tools/0/material_package_instance/package/material/abbreviation': 'PLA',
      '/speed': 100,
      '/temperature/bed': 59.8,
      '/temperature/bed_target': 60,
      '/tools/0/nozzle_temp': 214.6,
      '/tools/0/nozzle_target_temp': 215,
      ...over,
    };

    return { value: (path) => values[path], points: () => [] };
  };

  // Nothing is home yet and both heaters are climbing.
  const heatingUp = readings({
    '/axis/x': 0,
    '/axis/y': 0,
    '/axis/z': 0.2,
    '/temperature/bed': 32.4,
    '/tools/0/nozzle_temp': 96.2,
  });

  const idle = readings({
    '/temperature/bed': 24.1,
    '/temperature/bed_target': 0,
    '/tools/0/nozzle_temp': 26.8,
    '/tools/0/nozzle_target_temp': 0,
  });

  // While a print runs the gantry is not a person's to drive.
  const WHILE_IDLE: StateTransition[] = [
    'core.command.home',
    'core.command.move',
    'core.command.move_z',
    'core.command.disable_steppers',
    'core.command.mesh_bed_leveling',
  ].map((verb) => ({ from: 'IDLE', to: 'IDLE', verb }));

  const commands = { onCommand: () => undefined };

  // A CORE One printing with a chamber target: the bed warms the air towards
  // it, and the fans run on their own.
  const coreOne: Device = {
    ...fakeDevice('p3', 'CORE One #7'),
    model: 'CoreOne',
    capabilities: [
      ...EVERY_VERB,
      'core.command.set_chamber_target_temp',
      'core.command.set_chamber_target_temp#chamber.target_temp',
    ],
    state: { '/state': 'PRINTING' },
  };

  const chamberWarming = readings({
    '/temperature/chamber': 38.4,
    '/temperature/chamber_target': 45,
    '/chamber/fan_pwm_target': -1,
    '/chamber/led_intensity': 100,
  });

  const MATERIAL = 'material_package_instance/package/material/abbreviation';

  // Two heads in the tool tree. Head 1 prints and head 0 heats to its own
  // target, and each reports its nozzle.
  const toolChanger: Device = {
    ...fakeDevice('p2', 'XL #12'),
    model: 'XL',
    capabilities: [...EVERY_VERB, 'core.command.set_nozzle_temperature#tool'],
    state: {
      '/state': 'PRINTING',
      '/tools/1/active': true,
      '/tools/0/enabled': true,
      '/tools/0/nozzle_temp': 170.2,
      '/tools/0/nozzle_target_temp': 170,
      '/tools/0/features/nozzle_diameter': 0.4,
      '/tools/0/features/nozzle_high_flow': false,
      '/tools/0/features/nozzle_hardened': false,
      [`/tools/0/${MATERIAL}`]: 'PLA',
      '/tools/1/enabled': true,
      '/tools/1/nozzle_temp': 244.8,
      '/tools/1/features/nozzle_diameter': 0.6,
      '/tools/1/features/nozzle_high_flow': true,
      '/tools/1/features/nozzle_hardened': true,
      [`/tools/1/${MATERIAL}`]: 'PETG',
    },
  };

  // The same tool changer with head 0 switched off on the printer. The
  // architecture writes it as null, which the tree holds as enabled false.
  const headOff: Device = {
    ...toolChanger,
    state: {
      ...Object.fromEntries(
        Object.entries(toolChanger.state).filter(
          ([path]) => !path.startsWith('/tools/0/'),
        ),
      ),
      '/tools/0/enabled': false,
      '/tools/2/enabled': true,
      '/tools/2/nozzle_temp': 31.5,
      '/tools/2/features/nozzle_diameter': 0.4,
      [`/tools/2/${MATERIAL}`]: 'ASA',
    },
  };

  // One head with a feeder of five slots, printing from the third.
  const feeder: Device = {
    ...printer('PRINTING'),
    state: {
      '/state': 'PRINTING',
      '/tools/0/active': true,
      '/tools/0.2/active': true,
      '/tools/0/enabled': true,
      '/tools/0/feeder/slot_count': 5,
      '/tools/0/features/nozzle_diameter': 0.4,
      [`/tools/0.0/${MATERIAL}`]: 'PLA',
      [`/tools/0.1/${MATERIAL}`]: 'PLA',
      [`/tools/0.2/${MATERIAL}`]: 'ASA',
      [`/tools/0.3/${MATERIAL}`]: '',
      [`/tools/0.4/${MATERIAL}`]: 'PETG',
    },
  };
</script>

<Story name="Printing" asChild>
  <PrinterControlsPanel
    device={printer('PRINTING')}
    telemetry={readings()}
    commands={{ ...commands, transitions: WHILE_IDLE }}
  />
</Story>

<Story name="Printing Without A Transition Table" asChild>
  <PrinterControlsPanel
    device={printer('PRINTING')}
    telemetry={readings()}
    {commands}
  />
</Story>

<Story name="Chamber" asChild>
  <PrinterControlsPanel
    device={coreOne}
    telemetry={chamberWarming}
    {commands}
  />
</Story>

<Story name="Idle" asChild>
  <PrinterControlsPanel device={printer('IDLE')} telemetry={idle} {commands} />
</Story>

<Story name="Heating" asChild>
  <PrinterControlsPanel
    device={printer('BUSY')}
    telemetry={heatingUp}
    {commands}
  />
</Story>

<!-- Every value would be stale and every control would only refuse. -->
<Story name="Not Reporting" asChild>
  <PrinterControlsPanel
    device={{ ...printer('OFFLINE'), connected: false }}
    telemetry={readings()}
    {commands}
  />
</Story>

<Story name="Loading" asChild>
  <PrinterControlsPanel telemetry={readings()} {commands} loading />
</Story>

<Story name="Only Some Of The Verbs" asChild>
  <PrinterControlsPanel
    device={printer('IDLE', FEW_VERBS)}
    telemetry={idle}
    {commands}
  />
</Story>

<!-- A row per head, each with its own target and the nozzle it reports. -->
<Story name="Tool Changer" asChild>
  <PrinterControlsPanel
    device={toolChanger}
    telemetry={readings({
      '/tools/0/nozzle_temp': 170.2,
      '/tools/0/nozzle_target_temp': 170,
      '/tools/1/nozzle_temp': 244.8,
      '/tools/1/nozzle_target_temp': 245,
    })}
    {commands}
  />
</Story>

<!-- One head with a feeder is not a tool changer. The extruder shows the
     material of the slot it prints from. -->
<Story name="One Head With A Feeder" asChild>
  <PrinterControlsPanel
    device={feeder}
    telemetry={readings({
      '/tools/0.2/material_package_instance/package/material/abbreviation':
        'ASA',
    })}
    {commands}
  />
</Story>

<!-- A head switched off on the printer has no row: the rows are the heads
     the printer can print with. -->
<Story name="Tool Changer With A Head Switched Off" asChild>
  <PrinterControlsPanel
    device={headOff}
    telemetry={readings({
      '/tools/1/nozzle_temp': 244.8,
      '/tools/1/nozzle_target_temp': 245,
    })}
    {commands}
  />
</Story>
