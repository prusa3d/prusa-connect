import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import {
  PREHEAT_PRESETS,
  type Device,
  type StateTransition,
} from '../../../domain';
import { formatting } from '../../../i18n';
import { expectNoViolations } from '../../../test/axe';
import { fakeDevice } from '../../../test/fakes';
import { openSurface } from '../../../test/open-surface';
import type { TelemetrySource } from '../types';
import { BED_MAX_CELSIUS, NOZZLE_MAX_CELSIUS } from './machine-controls';
import PrinterControlsPanel from './printer-controls-panel.svelte';
import type { PrinterControlsPanelProps } from './types';

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
  'core.command.set_idle',
  'core.command.set_nozzle_temperature',
  'core.command.set_printer_ready',
  'core.command.set_speed',
];

const SETTINGS_VERBS = [
  'core.command.set_hostname',
  'core.command.set_tool_hardened',
  'core.command.set_tool_high_flow',
  'core.command.set_tool_nozzle_diameter',
];

const READINGS: Readonly<Record<string, number | string>> = {
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
};

const telemetry: TelemetrySource = {
  value: (path) => READINGS[path],
  points: () => [],
};

const printer = (
  state: string,
  capabilities: string[] = EVERY_VERB,
): Device => ({
  ...fakeDevice('printer-1', 'MK4S'),
  capabilities,
  state: { '/state': state },
});

const props = (
  over: Partial<PrinterControlsPanelProps> = {},
): PrinterControlsPanelProps => ({
  device: printer('IDLE'),
  telemetry,
  commands: { onCommand: () => undefined },
  ...over,
});

afterEach(() => {
  formatting.temperature = 'celsius';
});

describe('PrinterControlsPanel', () => {
  it('sends a relative jog of the chosen step, at the pad feedrate', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({ commands: { onCommand } }),
    });

    await page.getByRole('radio', { name: '10 mm' }).click();
    await page.getByRole('button', { name: 'Move X left' }).click();

    expect(onCommand).toHaveBeenCalledWith('core.command.move', {
      feedrate: '3000',
      x: '-10',
    });
  });

  // Z grows as the nozzle and the bed part, whichever of the two moves.
  it('raises the nozzle with the up arrow where the gantry travels on Z', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({
        device: { ...printer('IDLE'), model: 'MK4S' },
        commands: { onCommand },
      }),
    });

    await page.getByRole('button', { name: 'Move Z up' }).click();

    expect(onCommand).toHaveBeenCalledWith('core.command.move_z', {
      feedrate: '600',
      distance: '1',
    });
  });

  it('raises the bed with the up arrow where the bed travels on Z', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({
        device: { ...printer('IDLE'), model: 'COREONE' },
        commands: { onCommand },
      }),
    });

    await page.getByRole('button', { name: 'Move the bed up' }).click();
    await page.getByRole('button', { name: 'Move the bed down' }).click();

    expect(onCommand.mock.calls).toEqual([
      ['core.command.move_z', { feedrate: '600', distance: '-1' }],
      ['core.command.move_z', { feedrate: '600', distance: '1' }],
    ]);
  });

  // The pad sends a delta and the reading is the absolute position.
  it('prints where the gantry is on the label of the control that moves it', async () => {
    await render(PrinterControlsPanel, { props: props() });

    await expect.element(page.getByText('4.20 mm')).toBeVisible();
    await expect.element(page.getByText('120.5 mm · 90.3 mm')).toBeVisible();
  });

  // Slots are toolheads on a tool changer and feeders on an MMU; only the
  // model says which, so the same state tree draws differently.
  describe('tools', () => {
    const slotted = (): Device => ({
      ...printer('IDLE'),
      state: {
        '/state': 'IDLE',
        '/tools/1/active': true,
        '/tools/0/material_package_instance/package/material/abbreviation':
          'PLA',
        '/tools/0/nozzle_temp': 170,
        '/tools/1/material_package_instance/package/material/abbreviation':
          'PETG',
        '/tools/1/nozzle_temp': 215,
      },
    });

    it('lists each head with its material and temperature on a tool changer', async () => {
      await render(PrinterControlsPanel, {
        props: props({ device: slotted() }),
      });

      await expect.element(page.getByText('Tools')).toBeVisible();
      await expect.element(page.getByText('T0')).toBeVisible();
      await expect.element(page.getByText('PLA')).toBeVisible();
      // The active head's material, in its own row and in the extruder's.
      await expect.element(page.getByText('PETG').nth(1)).toBeVisible();
      await expect
        .element(page.getByText('T1'))
        .toHaveAttribute('class', expect.stringContaining('font-semibold'));
    });

    // A head switched off on the printer has no row, and a head that
    // prints shows the nozzle it reports under its own id.
    it('leaves out a head the printer switched off', async () => {
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...slotted(),
            state: {
              ...slotted().state,
              '/tools/2/enabled': false,
              '/tools/2/nozzle_temp': 99,
            },
          },
        }),
      });

      await expect.element(page.getByText('T1')).toBeVisible();
      await expect.element(page.getByText('T2')).not.toBeInTheDocument();
    });

    // A printer with one nozzle is head 0, and the extruder row reads it
    // from the tree.
    it('reads the one nozzle of a single-head printer from head 0', async () => {
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...printer('IDLE'),
            state: {
              '/state': 'IDLE',
              '/tools/0/active': true,
              '/tools/0/nozzle_temp': 199.6,
              '/tools/0/nozzle_target_temp': 200,
            },
          },
          telemetry: { value: () => undefined, points: () => [] },
        }),
      });

      await expect.element(page.getByText('Tools')).not.toBeInTheDocument();
      await expect.element(page.getByText(/200\s*°C/).first()).toBeVisible();
    });

    it('heats one head from its own row where the verb takes a tool', async () => {
      const onCommand = vi.fn();
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...slotted(),
            capabilities: [
              ...EVERY_VERB,
              'core.command.set_nozzle_temperature#tool',
            ],
          },
          commands: { onCommand },
        }),
      });

      await page
        .getByRole('button', { name: 'Set T0 extruder target' })
        .click();
      await (await openSurface()).getByRole('button', { name: 'PETG' }).click();
      await (
        await openSurface()
      )
        .getByRole('button', { name: 'Set', exact: true })
        .click();

      expect(onCommand).toHaveBeenCalledWith(
        'core.command.set_nozzle_temperature',
        { nozzle_temperature: '230', tool: '0' },
      );
    });

    it('sets what is fitted on one head, by its tool number', async () => {
      const onCommand = vi.fn();
      await render(PrinterControlsPanel, {
        props: props({
          device: { ...slotted(), capabilities: SETTINGS_VERBS },
          commands: { onCommand },
        }),
      });

      await page.getByRole('combobox', { name: 'T1 nozzle diameter' }).click();
      await page.getByRole('option', { name: '0.6 mm' }).click();
      await page
        .getByRole('group', { name: 'T0 high flow nozzle' })
        .getByRole('radio', { name: 'On' })
        .click();

      expect(onCommand).toHaveBeenCalledWith(
        'core.command.set_tool_nozzle_diameter',
        { nozzle_diameter: '0.6', tool: '1' },
      );
      expect(onCommand).toHaveBeenCalledWith(
        'core.command.set_tool_high_flow',
        { high_flow: 'true', tool: '0' },
      );
      await expect
        .element(page.getByRole('combobox', { name: 'T1 nozzle diameter' }))
        .toHaveTextContent('0.6 mm');
    });

    it('addresses the one head of a single-tool printer as tool 0', async () => {
      const onCommand = vi.fn();
      await render(PrinterControlsPanel, {
        props: props({
          device: printer('IDLE', SETTINGS_VERBS),
          commands: { onCommand },
        }),
      });

      await page
        .getByRole('group', { name: 'Extruder hardened nozzle' })
        .getByRole('radio', { name: 'Off' })
        .click();

      expect(onCommand).toHaveBeenCalledWith('core.command.set_tool_hardened', {
        hardened: 'false',
        tool: '0',
      });
      await expect
        .element(
          page.getByRole('combobox', { name: 'Extruder nozzle diameter' }),
        )
        .toHaveTextContent('No reading yet');
    });

    // One head with a feeder is not a tool changer, whatever its model
    // says: the tree holds one head (ADR-20261023-gna rule 12).
    it('draws no tools for a printer whose slots are feeders', async () => {
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...printer('IDLE'),
            state: {
              '/state': 'IDLE',
              '/tools/0/active': true,
              '/tools/0.1/active': true,
              '/tools/0/material_package_instance/package/material/abbreviation':
                'PLA',
              '/tools/0/enabled': true,
              '/tools/0/feeder/slot_count': 2,
              '/tools/0.0/material_package_instance/package/material/abbreviation':
                'PLA',
              '/tools/0.1/material_package_instance/package/material/abbreviation':
                'PETG',
            },
          },
        }),
      });

      await expect.element(page.getByText('Extruder')).toBeVisible();
      await expect.element(page.getByText('PETG')).toBeVisible();
      await expect.element(page.getByText('Tools')).not.toBeInTheDocument();
      await expect.element(page.getByText('T0')).not.toBeInTheDocument();
    });

    it('opens a head that is not printing on the target it reports', async () => {
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...slotted(),
            capabilities: [
              ...EVERY_VERB,
              'core.command.set_nozzle_temperature#tool',
            ],
            state: { ...slotted().state, '/tools/0/nozzle_target_temp': 205 },
          },
          // The stream has said nothing of head 0's target yet.
          telemetry: {
            value: (path) =>
              path === '/tools/0/nozzle_target_temp'
                ? undefined
                : READINGS[path],
            points: () => [],
          },
        }),
      });

      await page
        .getByRole('button', { name: 'Set T0 extruder target' })
        .click();

      await expect
        .element((await openSurface()).getByRole('spinbutton'))
        .toHaveValue(205);
    });

    it('shows the nozzle a head reports before anything is sent', async () => {
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...slotted(),
            capabilities: SETTINGS_VERBS,
            state: {
              ...slotted().state,
              '/tools/1/features/nozzle_diameter': 0.6,
              '/tools/1/features/nozzle_hardened': true,
            },
          },
        }),
      });

      await expect
        .element(page.getByRole('combobox', { name: 'T1 nozzle diameter' }))
        .toHaveTextContent('0.6 mm');
      await expect
        .element(
          page
            .getByRole('group', { name: 'T1 hardened nozzle' })
            .getByRole('radio', { name: 'On' }),
        )
        .toBeChecked();
      await expect
        .element(page.getByRole('combobox', { name: 'T0 nozzle diameter' }))
        .toHaveTextContent('No reading yet');
    });
  });

  it('draws no control for a verb the printer never offered', async () => {
    const { container } = await render(PrinterControlsPanel, {
      props: props({
        device: printer('IDLE', ['core.command.home', 'core.command.beep']),
      }),
    });

    await expect
      .element(page.getByRole('button', { name: 'Home' }))
      .toBeVisible();
    expect(
      [...container.querySelectorAll('button')].map((button) =>
        button.textContent.trim(),
      ),
    ).not.toContain('Mesh bed leveling');
  });

  // A verb the state refuses is a dead control plus one sentence.
  it('states why a group refuses, and the group cannot be pressed', async () => {
    const onCommand = vi.fn();
    const transitions: StateTransition[] = [
      { from: 'IDLE', to: 'IDLE', verb: 'core.command.home' },
      { from: 'IDLE', to: 'IDLE', verb: 'core.command.move' },
      { from: 'IDLE', to: 'IDLE', verb: 'core.command.move_z' },
      { from: 'IDLE', to: 'IDLE', verb: 'core.command.disable_steppers' },
      { from: 'IDLE', to: 'IDLE', verb: 'core.command.reset_printer' },
    ];
    await render(PrinterControlsPanel, {
      props: props({
        device: printer('PRINTING'),
        commands: { onCommand, transitions },
      }),
    });

    await expect
      .element(page.getByText('This works when the printer is Idle.'))
      .toBeVisible();

    const home = page.getByRole('button', { name: 'Home' });
    await expect.element(home).toBeDisabled();
    expect(onCommand).not.toHaveBeenCalled();
  });

  // A device kind that declared no transitions leaves every verb executable.
  it('holds the machine controls while a print runs, and says when they return', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({
        device: printer('PRINTING'),
        commands: { onCommand },
      }),
    });

    for (const name of [
      'Move X left',
      'Move the bed up',
      'Home',
      'Disable steppers',
      'Mark as ready',
      'Exit to idle',
      'Mesh bed leveling',
      'Beep',
    ]) {
      await expect.element(page.getByRole('button', { name })).toBeDisabled();
    }

    // Stopping the print is what it is for, so it stays live.
    await expect
      .element(page.getByRole('button', { name: 'Reset printer' }))
      .toBeEnabled();
    await expect
      .element(
        page.getByText('Available once the print is finished and cleared.'),
      )
      .toBeVisible();
    await expect
      .element(page.getByText('Available once the print finishes.'))
      .toBeVisible();
    expect(onCommand).not.toHaveBeenCalled();
  });

  // The fan's mode and its speed are one wire argument.
  it('sends the chamber fan mode as the one argument that carries it', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({ commands: { onCommand } }),
    });

    await page.getByRole('radio', { name: 'Off' }).click();

    expect(onCommand).toHaveBeenCalledWith(
      'core.command.set_chamber_fan_target',
      { 'chamber.fan_pwm_target': '0' },
    );
  });

  // A CORE One reports its fan target and light, so the section opens on what
  // the printer runs rather than on a default.
  it('shows the chamber fan and light the printer reports', async () => {
    const reporting = (
      fan: number,
      light: number,
      target = 45,
    ): TelemetrySource => ({
      value: (path) =>
        ({
          ...READINGS,
          '/chamber/fan_pwm_target': fan,
          '/chamber/led_intensity': light,
          '/temperature/chamber_target': target,
        })[path],
      points: () => [],
    });
    const chamberVerbs = [
      ...EVERY_VERB,
      'core.command.set_chamber_target_temp',
      'core.command.set_chamber_target_temp#chamber.target_temp',
    ];

    const first = await render(PrinterControlsPanel, {
      props: props({
        telemetry: reporting(40, 49),
        device: printer('IDLE', chamberVerbs),
      }),
    });

    await expect
      .element(page.getByRole('radio', { name: 'Manual' }))
      .toBeChecked();
    await expect
      .element(page.getByRole('slider', { name: 'Chamber fan speed' }))
      .toHaveAttribute('aria-valuenow', '40');
    await expect
      .element(page.getByRole('slider', { name: 'Light' }))
      .toHaveAttribute('aria-valuenow', '49');
    await expect
      .element(
        page.getByText(
          'The chamber target applies only while the fans run on Auto.',
        ),
      )
      .toBeVisible();

    await first.unmount();
    const second = await render(PrinterControlsPanel, {
      props: props({
        telemetry: reporting(-1, 100),
        device: printer('IDLE', chamberVerbs),
      }),
    });

    await expect
      .element(page.getByRole('radio', { name: 'Auto' }))
      .toBeChecked();
    await expect
      .element(
        page.getByText(
          'The chamber target applies only while the fans run on Auto.',
        ),
      )
      .not.toBeInTheDocument();

    // With no target there is nothing for the note to warn about.
    await second.unmount();
    await render(PrinterControlsPanel, {
      props: props({
        telemetry: reporting(0, 100, 0),
        device: printer('IDLE', chamberVerbs),
      }),
    });

    await expect
      .element(page.getByRole('radio', { name: 'Off' }))
      .toBeChecked();
    await expect
      .element(
        page.getByText(
          'The chamber target applies only while the fans run on Auto.',
        ),
      )
      .not.toBeInTheDocument();
  });

  // Off and back to Manual returns to the speed the fans last ran at, not to
  // full speed.
  it('returns to the last chamber fan speed after Off', async () => {
    let fan = $state(40);
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({
        commands: { onCommand },
        telemetry: {
          value: (path) =>
            ({ ...READINGS, '/chamber/fan_pwm_target': fan })[path],
          points: () => [],
        },
      }),
    });

    await expect
      .element(page.getByRole('radio', { name: 'Manual' }))
      .toBeChecked();
    fan = 0;
    await expect
      .element(page.getByRole('radio', { name: 'Off' }))
      .toBeChecked();

    await page.getByRole('radio', { name: 'Manual' }).click();

    expect(onCommand).toHaveBeenLastCalledWith(
      'core.command.set_chamber_fan_target',
      { 'chamber.fan_pwm_target': '40' },
    );
  });

  // A preset is a shortcut to a figure.
  it('sends the firmware figure for the material a reader picks', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({ commands: { onCommand } }),
    });

    await page
      .getByRole('button', { name: 'Set extruder target, currently 215°C' })
      .click();
    await (await openSurface()).getByRole('button', { name: 'PETG' }).click();
    await (
      await openSurface()
    )
      .getByRole('button', { name: 'Set', exact: true })
      .click();

    expect(onCommand).toHaveBeenCalledWith(
      'core.command.set_nozzle_temperature',
      { nozzle_temperature: '230' },
    );

    await page
      .getByRole('button', { name: 'Set bed target, currently 60°C' })
      .click();
    await (await openSurface()).getByRole('button', { name: 'PETG' }).click();
    await (
      await openSurface()
    )
      .getByRole('button', { name: 'Set', exact: true })
      .click();

    expect(onCommand).toHaveBeenCalledWith('core.command.set_bed_temperature', {
      bed_temperature: '85',
    });
  });

  // A CORE One reports its chamber and takes a target for it. A printer with
  // neither the reading nor the verb draws no chamber row.
  it('sets the chamber target where the printer has a chamber', async () => {
    const onCommand = vi.fn();
    const chamber: TelemetrySource = {
      value: (path) =>
        ({
          ...READINGS,
          '/temperature/chamber': 38.4,
          '/temperature/chamber_target': 0,
        })[path],
      points: () => [],
    };
    await render(PrinterControlsPanel, {
      props: props({
        telemetry: chamber,
        device: printer('IDLE', [
          ...EVERY_VERB,
          'core.command.set_chamber_target_temp',
          'core.command.set_chamber_target_temp#chamber.target_temp',
        ]),
        commands: { onCommand },
      }),
    });

    await page
      .getByRole('button', { name: 'Set chamber target, currently 0°C' })
      .click();
    const surface = await openSurface();
    await surface.getByRole('spinbutton', { name: 'Target (°C)' }).fill('45');
    await surface.getByRole('button', { name: 'Set', exact: true }).click();

    expect(onCommand).toHaveBeenCalledWith(
      'core.command.set_chamber_target_temp',
      { 'chamber.target_temp': '45' },
    );
  });

  it('draws no chamber row on a printer without one', async () => {
    await render(PrinterControlsPanel, { props: props() });

    await expect
      .element(page.getByRole('button', { name: /^Set chamber target/ }))
      .not.toBeInTheDocument();
  });

  // The row reads in the unit the reader chose, so its editor does too, and
  // the printer is still sent Celsius.
  it('edits a heater in Fahrenheit when the reader chose it', async () => {
    formatting.temperature = 'fahrenheit';
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({ commands: { onCommand } }),
    });

    await page
      .getByRole('button', { name: 'Set extruder target, currently 419°F' })
      .click();
    await expect
      .element(page.getByRole('spinbutton', { name: 'Target (°F)' }))
      .toHaveValue(419);
    await (await openSurface()).getByRole('button', { name: 'PETG' }).click();
    await (
      await openSurface()
    )
      .getByRole('button', { name: 'Set', exact: true })
      .click();

    expect(onCommand).toHaveBeenCalledWith(
      'core.command.set_nozzle_temperature',
      { nozzle_temperature: '230' },
    );
  });

  // The editor clamps silently.
  it('keeps every preheat figure inside the heaters the editor offers', () => {
    for (const { material, nozzleCelsius, bedCelsius } of PREHEAT_PRESETS) {
      expect(nozzleCelsius, material).toBeLessThanOrEqual(NOZZLE_MAX_CELSIUS);
      expect(bedCelsius, material).toBeLessThanOrEqual(BED_MAX_CELSIUS);
    }

    expect(new Set(PREHEAT_PRESETS.map((preset) => preset.material)).size).toBe(
      PREHEAT_PRESETS.length,
    );
  });

  it('offers the unmarking verb to a printer already marked ready', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({
        device: printer('READY'),
        commands: { onCommand },
      }),
    });

    await page.getByRole('button', { name: 'Unmark as ready' }).click();

    expect(onCommand).toHaveBeenCalledWith('core.command.cancel_printer_ready');
  });

  // Every value would be stale and every control would only refuse.
  it('says a printer is not reporting instead of drawing the card', async () => {
    const { container } = await render(PrinterControlsPanel, {
      props: props({ device: { ...printer('OFFLINE'), connected: false } }),
    });

    await expect
      .element(page.getByText('MK4S is not reporting.', { exact: false }))
      .toBeVisible();
    expect(container.querySelectorAll('button')).toHaveLength(0);
  });

  it('waits in its own shape, not as a dead card of controls', async () => {
    const { container } = await render(PrinterControlsPanel, {
      props: props({ loading: true }),
    });

    expect(
      container.querySelectorAll('[data-slot="skeleton"]').length,
    ).toBeGreaterThan(0);
    expect(container.querySelectorAll('button')).toHaveLength(0);
  });

  it('names the absence where the printer accepts no commands at all', async () => {
    await render(PrinterControlsPanel, {
      props: props({ device: printer('IDLE', []) }),
    });

    await expect.element(page.getByText('No controls')).toBeVisible();
  });

  it('draws only the nozzle settings the printer offers', async () => {
    await render(PrinterControlsPanel, {
      props: props({
        device: printer('IDLE', ['core.command.set_tool_high_flow']),
      }),
    });

    await expect.element(page.getByText('High flow')).toBeVisible();
    await expect.element(page.getByText('Diameter')).not.toBeInTheDocument();
    await expect.element(page.getByText('Hardened')).not.toBeInTheDocument();
  });

  it('changes the hostname only to one network label', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({
        device: printer('IDLE', SETTINGS_VERBS),
        commands: { onCommand },
      }),
    });

    await page.getByRole('button', { name: 'Change hostname' }).click();
    const dialog = page.getByRole('dialog');
    const field = dialog.getByRole('textbox', { name: 'Hostname' });
    const confirm = dialog.getByRole('button', { name: 'Change hostname' });

    await field.fill('prusa xl');
    await expect.element(confirm).toBeDisabled();
    await expect
      .element(
        dialog.getByText('Use letters, digits and hyphens.', { exact: false }),
      )
      .toBeVisible();

    await field.fill(' prusa-xl ');
    await confirm.click();

    expect(onCommand).toHaveBeenCalledWith('core.command.set_hostname', {
      hostname: 'prusa-xl',
    });
  });

  it('sends nothing while a command is already on its way', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({ commands: { onCommand, sending: true } }),
    });

    await expect
      .element(page.getByRole('button', { name: 'Home' }))
      .toBeDisabled();
    expect(onCommand).not.toHaveBeenCalled();
  });

  it('has no accessibility violations', async () => {
    const { container } = await render(PrinterControlsPanel, {
      props: props({
        device: printer('IDLE', [...EVERY_VERB, ...SETTINGS_VERBS]),
      }),
    });

    await expectNoViolations(container);
  });
});
