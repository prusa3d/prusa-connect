import { afterEach, describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { formatting } from '../../i18n';
import { expectNoViolations } from '../../test/axe';
import { fakeDevice } from '../../test/fakes';
import { openSurface } from '../../test/open-surface';
import TelemetryChips from './telemetry-chips.svelte';
import type { TelemetrySource } from './types';

const silent: TelemetrySource = {
  value: () => undefined,
  points: () => [],
};

const device = {
  ...fakeDevice('printer-1', 'MK4S'),
  state: {
    '/tools/0/nozzle_temp': 234.6,
    '/tools/0/nozzle_target_temp': 235,
    '/temperature/bed': 59.8,
    '/temperature/bed_target': 60,
    '/tools/0/material_package_instance/package/material/abbreviation': 'PLA',
    '/speed': 100,
    '/flow': 95,
  },
};

/** The same printer, offering both heaters and in a state that takes them. */
const heating = {
  ...device,
  capabilities: [
    'core.command.set_nozzle_temperature',
    'core.command.set_bed_temperature',
  ],
  state: { ...device.state, '/state': 'IDLE' },
};

/** The same printer, offering both filament verbs and a type to load. */
const filament = {
  ...device,
  capabilities: [
    'core.command.load_filament',
    'core.command.load_filament#filament',
    'core.command.unload_filament',
  ],
  state: { ...device.state, '/state': 'IDLE' },
};

const nozzleChip = () =>
  page.getByRole('button', { name: /^Set extruder target, currently 235°C,/ });

const setButton = () => page.getByRole('button', { name: 'Set', exact: true });

afterEach(() => {
  formatting.temperature = 'celsius';
});

describe('TelemetryChips', () => {
  // One shape for every reading.
  it('draws every reading as one outlined chip', async () => {
    const { container } = await render(TelemetryChips, {
      props: { telemetry: silent, device },
    });

    const chips = [...container.querySelectorAll('[data-slot="badge"]')];
    expect(chips).toHaveLength(5);
    for (const chip of chips) {
      expect(chip.getAttribute('data-variant')).toBe('outline');
      expect(chip.className).toContain('font-mono');
      expect(chip.querySelector('svg')).not.toBeNull();
    }
  });

  // A tool changer is a printer whose tool tree holds more than one head,
  // whatever its model says (ADR-20261023-gna rule 12).
  it('names the active tool on a tool changer and nowhere else', async () => {
    const slotted = {
      ...device,
      state: {
        ...device.state,
        '/tools/1/active': true,
        '/tools/0/enabled': true,
        '/tools/1/enabled': true,
      },
    };
    const first = await render(TelemetryChips, {
      props: { telemetry: silent, device: slotted },
    });

    await expect.element(page.getByText('T1')).toBeVisible();

    await first.unmount();
    await render(TelemetryChips, {
      props: {
        telemetry: silent,
        device: {
          ...device,
          state: {
            ...device.state,
            '/tools/0/active': true,
            '/tools/0/enabled': true,
          },
        },
      },
    });

    await expect.element(page.getByText('T0')).not.toBeInTheDocument();
  });

  // One material for the chip: the one of the tool printing now, from the
  // tree, and the flat alias where the tree does not say.
  it('shows the material of the slot the printer prints from', async () => {
    await render(TelemetryChips, {
      props: {
        telemetry: silent,
        device: {
          ...device,
          state: {
            ...device.state,
            '/tools/0/active': true,
            '/tools/0.2/active': true,
            '/tools/0/feeder/slot_count': 5,
            '/tools/0.2/material_package_instance/package/material/abbreviation':
              'ASA',
          },
        },
      },
    });

    await expect.element(page.getByText('ASA')).toBeVisible();
    await expect.element(page.getByText('PLA')).not.toBeInTheDocument();
  });

  describe('on a tool changer', () => {
    const changer = {
      ...heating,
      capabilities: [
        ...heating.capabilities,
        'core.command.set_nozzle_temperature#tool',
      ],
      state: {
        ...heating.state,
        '/tools/1/active': true,
        '/tools/0/nozzle_temp': 185,
        '/tools/0/nozzle_target_temp': 0,
        '/tools/0/material_package_instance/package/material/abbreviation':
          'PLA',
        '/tools/1/nozzle_temp': 240,
        // The head printing now heats to 235, which the chip shows.
        '/tools/1/nozzle_target_temp': 235,
        '/tools/1/material_package_instance/package/material/abbreviation':
          'PETG',
      },
    };

    it('lists no head the printer switched off', async () => {
      await render(TelemetryChips, {
        props: {
          telemetry: silent,
          device: {
            ...changer,
            state: { ...changer.state, '/tools/2/enabled': false },
          },
          commands: { onCommand: vi.fn() },
        },
      });

      await nozzleChip().click();
      const surface = await openSurface();

      await expect
        .element(surface.getByRole('button', { name: /T1 PETG Active/ }))
        .toBeVisible();
      await expect
        .element(surface.getByRole('button', { name: /T2/ }))
        .not.toBeInTheDocument();
    });

    it('opens on the heads alone, the active one marked', async () => {
      await render(TelemetryChips, {
        props: {
          telemetry: silent,
          device: changer,
          commands: { onCommand: vi.fn() },
        },
      });

      await nozzleChip().click();
      const surface = await openSurface();

      await expect
        .element(surface.getByRole('button', { name: /T0 PLA/ }))
        .toBeVisible();
      await expect
        .element(surface.getByRole('button', { name: /T1 PETG Active/ }))
        .toBeVisible();
      await expect.element(surface.getByRole('slider')).not.toBeInTheDocument();
    });

    it('heats the head picked from the list', async () => {
      const onCommand = vi.fn();
      await render(TelemetryChips, {
        props: {
          telemetry: silent,
          device: changer,
          commands: { onCommand },
        },
      });

      await nozzleChip().click();
      await (
        await openSurface()
      )
        .getByRole('button', { name: /T0 PLA/ })
        .click();
      const step = await openSurface();
      await expect
        .element(step.getByLabelText('Target for T0 (°C)'))
        .toHaveValue(0);
      await step.getByRole('button', { name: 'PETG' }).click();
      await setButton().click();

      expect(onCommand).toHaveBeenCalledWith(
        'core.command.set_nozzle_temperature',
        { nozzle_temperature: '230', tool: '0' },
      );
    });

    // A head that reports its own target opens on it, not on zero.
    it('opens a head on the target it reports', async () => {
      await render(TelemetryChips, {
        props: {
          telemetry: silent,
          device: {
            ...changer,
            state: { ...changer.state, '/tools/0/nozzle_target_temp': 205 },
          },
          commands: { onCommand: vi.fn() },
        },
      });

      await nozzleChip().click();
      await (
        await openSurface()
      )
        .getByRole('button', { name: /T0 PLA/ })
        .click();

      await expect
        .element((await openSurface()).getByLabelText('Target for T0 (°C)'))
        .toHaveValue(205);
    });

    // Without the argument the verb heats only the active head.
    it('keeps the plain editor where the verb takes no tool', async () => {
      await render(TelemetryChips, {
        props: {
          telemetry: silent,
          device: { ...changer, capabilities: heating.capabilities },
          commands: { onCommand: vi.fn() },
        },
      });

      await nozzleChip().click();
      const surface = await openSurface();

      await expect.element(surface.getByRole('slider')).toBeVisible();
    });
  });

  // The live stream is where a reader watches a number move.
  it('prefers the live reading over the record', async () => {
    await render(TelemetryChips, {
      props: {
        telemetry: {
          ...silent,
          value: (path) => (path === '/speed' ? 80 : undefined),
        },
        device,
      },
    });

    await expect.element(page.getByText('80%')).toBeVisible();
  });

  // A filament type is offered, and never a material from a catalogue.
  it('loads the filament type picked from the material chip', async () => {
    const onCommand = vi.fn();
    await render(TelemetryChips, {
      props: { telemetry: silent, device: filament, commands: { onCommand } },
    });

    await page.getByRole('button', { name: 'Material PLA' }).click();
    await (await openSurface()).getByRole('button', { name: 'PETG' }).click();

    expect(onCommand).toHaveBeenCalledWith('core.command.load_filament', {
      filament: 'PETG',
    });
  });

  // A driver whose load verb takes no type loads whatever the printer asks
  // for, so a type picker there would promise what the printer ignores.
  it('offers a plain load where the verb declares no filament argument', async () => {
    const onCommand = vi.fn();
    await render(TelemetryChips, {
      props: {
        telemetry: silent,
        device: {
          ...filament,
          capabilities: [
            'core.command.load_filament',
            'core.command.unload_filament',
          ],
        },
        commands: { onCommand },
      },
    });

    await page.getByRole('button', { name: 'Material PLA' }).click();
    await expect
      .element(page.getByRole('button', { name: 'PETG' }))
      .not.toBeInTheDocument();
    await page.getByRole('button', { name: 'Load filament' }).click();

    expect(onCommand).toHaveBeenCalledWith(
      'core.command.load_filament',
      undefined,
    );
  });

  it('unloads the filament from the material chip', async () => {
    const onCommand = vi.fn();
    await render(TelemetryChips, {
      props: { telemetry: silent, device: filament, commands: { onCommand } },
    });

    await page.getByRole('button', { name: 'Material PLA' }).click();
    await page.getByRole('button', { name: 'Unload filament' }).click();

    expect(onCommand).toHaveBeenCalledWith(
      'core.command.unload_filament',
      undefined,
    );
  });

  // The chevron is a promise that the chip opens something.
  it('leaves the heaters as readings without a port', async () => {
    const { container } = await render(TelemetryChips, {
      props: { telemetry: silent, device: heating },
    });

    expect(container.querySelectorAll('button')).toHaveLength(0);
  });

  // A verb the printer never offered is absence of a feature.
  it('draws a heater the printer never offered as a reading', async () => {
    const { container } = await render(TelemetryChips, {
      props: {
        telemetry: silent,
        device: {
          ...heating,
          capabilities: ['core.command.set_bed_temperature'],
        },
        commands: { onCommand: vi.fn() },
      },
    });

    await expect
      .element(
        page.getByRole('button', { name: /^Set bed target, currently 60°C,/ }),
      )
      .toBeInTheDocument();
    expect(container.querySelectorAll('button')).toHaveLength(1);
  });

  // The same table the Controls tab offers.
  it("sets a heater from the printer's own preheat table", async () => {
    const onCommand = vi.fn();
    await render(TelemetryChips, {
      props: { telemetry: silent, device: heating, commands: { onCommand } },
    });

    await nozzleChip().click();
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

  // The chip reads in the unit the reader chose, so the editor behind it
  // must too, and the printer is still sent Celsius.
  it('edits a heater in Fahrenheit when the reader chose it', async () => {
    formatting.temperature = 'fahrenheit';
    const onCommand = vi.fn();
    await render(TelemetryChips, {
      props: { telemetry: silent, device: heating, commands: { onCommand } },
    });

    await page
      .getByRole('button', { name: /^Set extruder target, currently 455°F,/ })
      .click();
    await expect
      .element(page.getByRole('spinbutton', { name: 'Target (°F)' }))
      .toHaveValue(455);
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

  // One step of the slider is nine Fahrenheit, which is five Celsius. With a
  // one-degree step, the step above 32 °F would be sent as 1 °C.
  it('moves a Fahrenheit editor by a whole Celsius step', async () => {
    formatting.temperature = 'fahrenheit';
    const onCommand = vi.fn();
    await render(TelemetryChips, {
      props: { telemetry: silent, device: heating, commands: { onCommand } },
    });

    await page
      .getByRole('button', { name: /^Set extruder target, currently 455°F,/ })
      .click();
    const surface = await openSurface();
    await surface.getByRole('slider').click();
    await userEvent.keyboard('{Home}');
    await userEvent.keyboard('{ArrowRight}');
    await surface.getByRole('button', { name: 'Set', exact: true }).click();

    expect(onCommand).toHaveBeenCalledWith(
      'core.command.set_nozzle_temperature',
      { nozzle_temperature: '5' },
    );
  });

  it("opens on the printer's own refusal", async () => {
    await render(TelemetryChips, {
      props: {
        telemetry: silent,
        device: heating,
        commands: { onCommand: vi.fn(), locked: true },
      },
    });

    await nozzleChip().click();

    await expect
      .element(page.getByText('This printer is locked.'))
      .toBeVisible();
    await expect.element(setButton()).not.toBeInTheDocument();
  });

  describe('the chamber', () => {
    const coreOne = {
      ...heating,
      model: 'CoreOne',
      capabilities: [
        ...heating.capabilities,
        'core.command.set_chamber_target_temp',
        'core.command.set_chamber_target_temp#chamber.target_temp',
      ],
      state: {
        ...heating.state,
        '/temperature/chamber': 38.4,
        '/temperature/chamber_target': 45,
      },
    };

    // A printer with no chamber has neither the reading nor the verb.
    it('draws no chamber chip on a printer without one', async () => {
      await render(TelemetryChips, {
        props: { telemetry: silent, device: heating },
      });

      await expect
        .element(page.getByText('Chamber', { exact: true }))
        .not.toBeInTheDocument();
    });

    it('sets the chamber target on a CORE One', async () => {
      const onCommand = vi.fn();
      await render(TelemetryChips, {
        props: { telemetry: silent, device: coreOne, commands: { onCommand } },
      });

      await page
        .getByRole('button', {
          name: /^Set chamber target, currently 45°C,/,
        })
        .click();
      const surface = await openSurface();
      await surface.getByRole('spinbutton', { name: 'Target (°C)' }).fill('50');
      await surface.getByRole('button', { name: 'Set', exact: true }).click();

      expect(onCommand).toHaveBeenCalledWith(
        'core.command.set_chamber_target_temp',
        { 'chamber.target_temp': '50' },
      );
    });

    // An XL reports the air of its enclosure and takes no chamber target.
    it('draws an enclosure reading with no verb as a reading', async () => {
      const { container } = await render(TelemetryChips, {
        props: {
          telemetry: silent,
          device: {
            ...heating,
            state: { ...heating.state, '/temperature/chamber': 31 },
          },
          commands: { onCommand: vi.fn() },
        },
      });

      expect(container.textContent).toContain('31°');
      expect(container.querySelectorAll('button')).toHaveLength(2);
    });
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(TelemetryChips, {
      props: { telemetry: silent, device },
    });

    await expectNoViolations(container);
  });

  it('has nothing axe objects to once the heaters are controls', async () => {
    const { container } = await render(TelemetryChips, {
      props: {
        telemetry: silent,
        device: heating,
        commands: { onCommand: vi.fn() },
      },
    });

    await expectNoViolations(container);
  });
});
