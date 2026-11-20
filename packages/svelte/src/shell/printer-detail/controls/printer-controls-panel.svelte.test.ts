import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
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
  it('keeps reset behind the maintenance disclosure and confirmation', async () => {
    const onCommand = vi.fn();
    await render(PrinterControlsPanel, {
      props: props({
        device: printer('PRINTING'),
        commands: { onCommand },
      }),
    });

    for (const name of ['Mesh bed leveling', 'Release axes', 'Mark as ready']) {
      await expect.element(page.getByRole('button', { name })).toBeDisabled();
    }

    const disclosure = page.getByText('Maintenance', { exact: true });
    const reset = page.getByRole('button', {
      name: 'Reset printer',
      exact: true,
    });
    await expect.element(reset).not.toBeInTheDocument();
    await disclosure.click();
    await expect.element(reset).toBeEnabled();
    await reset.click();

    const dialog = page.getByRole('alertdialog');
    await expect
      .element(
        dialog.getByText('This stops the print and turns off the heaters.'),
      )
      .toBeVisible();
    expect(onCommand).not.toHaveBeenCalled();
    await dialog.getByRole('button', { name: 'Reset printer' }).click();
    expect(onCommand).toHaveBeenCalledExactlyOnceWith(
      'core.command.reset_printer',
    );
  });

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
      await expect.element(page.getByText('Tool 1')).toBeVisible();
      await expect.element(page.getByText('PLA')).toBeVisible();
      // The active head's material, in its own row and in the extruder's.
      await expect.element(page.getByText('PETG').nth(1)).toBeVisible();
      await expect
        .element(page.getByText('Tool 2'))
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

      await expect.element(page.getByText('Tool 2')).toBeVisible();
      await expect.element(page.getByText('Tool 3')).not.toBeInTheDocument();
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
        .getByRole('button', { name: /^Set Tool 1 extruder target/ })
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

      await page
        .getByRole('combobox', { name: 'Tool 2 nozzle diameter' })
        .click();
      await page.getByRole('option', { name: '0.6 mm' }).click();
      await page
        .getByRole('group', { name: 'Tool 1 high flow nozzle' })
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
        .element(page.getByRole('combobox', { name: 'Tool 2 nozzle diameter' }))
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
      await expect.element(page.getByText('Tool 1')).not.toBeInTheDocument();
    });

    // The Extruder row repeats the active head, so a printer that sets each
    // head alone shows the heads only.
    it('leaves out the extruder row where each head is set alone', async () => {
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...slotted(),
            capabilities: [
              ...EVERY_VERB,
              'core.command.set_nozzle_temperature#tool',
            ],
          },
        }),
      });

      await expect.element(page.getByText('Tool 2')).toBeVisible();
      await expect.element(page.getByText('Extruder')).not.toBeInTheDocument();
    });

    it('hands each head of a tool changer to the material picker', async () => {
      const material = createRawSnippet((id: () => string) => ({
        render: () => `<button>Assign ${id()}</button>`,
      }));
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...slotted(),
            capabilities: [
              ...EVERY_VERB,
              'core.command.set_nozzle_temperature#tool',
            ],
          },
          material,
        }),
      });

      await expect
        .element(page.getByRole('button', { name: 'Assign 0', exact: true }))
        .toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Assign 1', exact: true }))
        .toBeVisible();
    });

    it('hands the one nozzle of a single-head printer to the material picker', async () => {
      const material = createRawSnippet((id: () => string) => ({
        render: () => `<button>Assign ${id()}</button>`,
      }));
      await render(PrinterControlsPanel, { props: props({ material }) });

      await expect
        .element(page.getByRole('button', { name: 'Assign 0', exact: true }))
        .toBeVisible();
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
        .getByRole('button', { name: /^Set Tool 1 extruder target/ })
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
        .element(page.getByRole('combobox', { name: 'Tool 2 nozzle diameter' }))
        .toHaveTextContent('0.6 mm');
      await expect
        .element(
          page
            .getByRole('group', { name: 'Tool 2 hardened nozzle' })
            .getByRole('radio', { name: 'On' }),
        )
        .toBeChecked();
      await expect
        .element(page.getByRole('combobox', { name: 'Tool 1 nozzle diameter' }))
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
    // Beep and reset sit in the Maintenance disclosure.
    await page.getByText('Maintenance', { exact: true }).click();

    for (const name of [
      'Move X left',
      'Move the bed up',
      'Home',
      'Release axes',
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
      .element(page.getByText('Available once the print finishes.').first())
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
          'The printer takes a new chamber target, or Cool down, only while the fans run on Auto.',
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
          'The printer takes a new chamber target, or Cool down, only while the fans run on Auto.',
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
          'The printer takes a new chamber target, or Cool down, only while the fans run on Auto.',
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

    await page.getByText('Maintenance', { exact: true }).click();
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

  describe('coordinates', () => {
    const MOTION_VERBS = [
      'core.command.disable_steppers',
      'core.command.home',
      'core.command.move',
      'core.command.move_z',
    ];

    const plate = (axes: Record<'x' | 'y' | 'z', number>) => {
      const reported = $state({ ...axes });
      const source: TelemetrySource = {
        value: (path) =>
          ({
            '/axis/x': reported.x,
            '/axis/y': reported.y,
            '/axis/z': reported.z,
          })[path],
        points: () => [],
      };
      return { reported, source };
    };

    const renderPlate = async (
      axes: Record<'x' | 'y' | 'z', number>,
      onCommand: PrinterControlsPanelProps['commands']['onCommand'],
    ) => {
      const { reported, source } = plate(axes);
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...printer('IDLE', MOTION_VERBS),
            build_volume: { x_mm: 250, y_mm: 210, z_mm: 220 },
          },
          telemetry: source,
          commands: { onCommand },
        }),
      });
      return reported;
    };

    const TIMEOUT_TEXT =
      'No reading of the requested position yet. Check the printer before you try again.';

    it('frees the controls when the printer never reports the move back', async () => {
      const onCommand = vi.fn(() =>
        Promise.resolve({ kind: 'accepted' as const }),
      );
      await renderPlate({ x: 100, y: 75, z: 25 }, onCommand);

      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
      try {
        await page.getByRole('button', { name: 'X+' }).click();
        await expect
          .element(page.getByText('Command accepted.', { exact: false }))
          .toBeVisible();
        // 1 mm at 3000 mm/min is 20 ms of travel, then the 12 s margin.
        vi.advanceTimersByTime(12_020);
      } finally {
        vi.useRealTimers();
      }

      await expect.element(page.getByText(TIMEOUT_TEXT)).toBeVisible();
      await page.getByRole('button', { name: 'Home axes' }).click();
      expect(onCommand).toHaveBeenLastCalledWith(
        'core.command.home',
        undefined,
      );
      expect(onCommand).toHaveBeenCalledTimes(2);
    });

    // 195 mm of Z at 600 mm/min is 19.5 s of travel, longer than a fixed
    // wait would allow.
    it('waits as long as a long move takes before it gives up', async () => {
      const onCommand = vi.fn(() =>
        Promise.resolve({ kind: 'accepted' as const }),
      );
      const reported = await renderPlate({ x: 100, y: 75, z: 25 }, onCommand);

      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
      try {
        page
          .getByRole('slider', { name: 'Z position in millimeters' })
          .element()
          .focus();
        await userEvent.keyboard('{End}');
        await expect
          .element(page.getByText('Command accepted.', { exact: false }))
          .toBeVisible();
        expect(onCommand).toHaveBeenCalledWith('core.command.move_z', {
          feedrate: '600',
          distance: '195',
        });
        vi.advanceTimersByTime(13_000);
      } finally {
        vi.useRealTimers();
      }

      await expect
        .element(page.getByText('Command accepted.', { exact: false }))
        .toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Z+' }))
        .toHaveAttribute('aria-disabled', 'true');
      reported.z = 220;
      await expect.element(page.getByText('Position confirmed')).toBeVisible();
    });

    it('keeps the controls held while a reading is still on its way to the target', async () => {
      const onCommand = vi.fn(() =>
        Promise.resolve({ kind: 'accepted' as const }),
      );
      const reported = await renderPlate({ x: 100, y: 75, z: 25 }, onCommand);

      await page.getByRole('button', { name: 'X+' }).click();
      await expect
        .element(page.getByText('Command accepted.', { exact: false }))
        .toBeVisible();
      reported.x = 100.5;
      await expect.element(page.getByText('100.5 mm')).toBeVisible();

      await expect
        .element(page.getByRole('button', { name: 'X+' }))
        .toHaveAttribute('aria-disabled', 'true');
      await expect
        .element(page.getByText('Command accepted.', { exact: false }))
        .toBeVisible();

      reported.x = 101;
      await expect.element(page.getByText('Position confirmed')).toBeVisible();
      await page.getByRole('button', { name: 'X+' }).click();
      expect(onCommand).toHaveBeenCalledTimes(2);
    });

    // The plate reads the same hold as Advanced, by the state telemetry
    // reports.
    it('holds the plate on a paused printer, with the one reason', async () => {
      const onCommand = vi.fn();
      const { source } = plate({ x: 100, y: 75, z: 25 });
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...printer('IDLE', MOTION_VERBS),
            build_volume: { x_mm: 250, y_mm: 210, z_mm: 220 },
          },
          telemetry: {
            ...source,
            value: (path) =>
              path === '/state' ? 'PAUSED' : source.value(path),
          },
          commands: { onCommand },
        }),
      });

      for (const name of ['X+', 'Z+', 'Home axes', 'Release axes']) {
        await expect
          .element(page.getByRole('button', { name, exact: true }))
          .toBeDisabled();
      }
      const reason = page.getByText(
        'The printer moves and homes only when it is idle, ready, finished or stopped.',
      );
      await expect.element(reason).toBeVisible();
      // Said once, by the plate, and not again under the axis actions.
      expect(reason.elements()).toHaveLength(1);
      expect(onCommand).not.toHaveBeenCalled();
    });

    // A finished print has left the gantry, so the plate moves again.
    it('lets the plate move on a printer whose print finished', async () => {
      const { source } = plate({ x: 100, y: 75, z: 25 });
      await render(PrinterControlsPanel, {
        props: props({
          device: {
            ...printer('IDLE', MOTION_VERBS),
            build_volume: { x_mm: 250, y_mm: 210, z_mm: 220 },
          },
          telemetry: {
            ...source,
            value: (path) =>
              path === '/state' ? 'FINISHED' : source.value(path),
          },
        }),
      });

      for (const name of ['X+', 'Z+', 'Home axes', 'Release axes']) {
        await expect
          .element(page.getByRole('button', { name, exact: true }))
          .toBeEnabled();
      }
    });

    it('says why X and Y are off when the reported position lies outside the plate', async () => {
      await renderPlate({ x: -2, y: 75, z: 25 }, vi.fn());

      await expect
        .element(page.getByRole('button', { name: 'X+' }))
        .toBeDisabled();
      await expect
        .element(page.getByRole('button', { name: 'Z+' }))
        .toBeEnabled();
      await expect
        .element(
          page.getByText('The printer has no usable position for this move.'),
        )
        .toBeVisible();
    });

    it('says why a drag is dropped when the reported position moved during it', async () => {
      const onCommand = vi.fn();
      const reported = await renderPlate({ x: 100, y: 75, z: 25 }, onCommand);
      const nozzle = page
        .getByRole('button', { name: 'Nozzle position on the build plate' })
        .element() as HTMLElement;
      nozzle.setPointerCapture = () => undefined;
      const { left, top } = nozzle.getBoundingClientRect();
      const pointer = (type: string, dx: number) =>
        new PointerEvent(type, {
          bubbles: true,
          button: 0,
          pointerId: 1,
          isPrimary: true,
          clientX: left + dx,
          clientY: top,
        });

      nozzle.dispatchEvent(pointer('pointerdown', 0));
      nozzle.dispatchEvent(pointer('pointermove', 45));
      reported.x = 100.02;
      await expect.element(page.getByText('100.02 mm')).toBeVisible();
      nozzle.dispatchEvent(pointer('pointerup', 45));

      await expect
        .element(
          page.getByText(
            'The reported position changed during the drag. Try again.',
          ),
        )
        .toBeVisible();
      expect(onCommand).not.toHaveBeenCalled();
    });
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
