import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { Device } from '../../../domain';
import { expectNoViolations } from '../../../test/axe';
import { fakeDevice } from '../../../test/fakes';
import { openSurface } from '../../../test/open-surface';
import PrinterTools from './printer-tools.svelte';

type Tools = NonNullable<Device['tools']>;

// The material package instance of a tool, nested as the API serves it.
const packageOf = (abbreviation: string) => ({
  package: { material: { abbreviation } },
});

// What an XL with heads 0 and 1 of its five enabled reports.
const xl: Tools = {
  tool_count: 5,
  tools: {
    '0': {
      type: 'FFF_PRINT_HEAD',
      active: false,
      features: {
        nozzle_diameter: 0.4,
        nozzle_high_flow: false,
        nozzle_hardened: false,
      },
      feeder: null,
      material_package_instance: packageOf('PLA'),
      nozzle_temp: 215,
    },
    '1': {
      type: 'FFF_PRINT_HEAD',
      active: true,
      features: {
        nozzle_diameter: 0.6,
        nozzle_high_flow: true,
        nozzle_hardened: true,
      },
      material_package_instance: null,
    },
    '2': null,
  },
};

// One head with a feeder of three slots, and a slot the printer still holds
// past the feeder's count.
const mmu: Tools = {
  tool_count: 1,
  tools: {
    '0': {
      active: true,
      features: { nozzle_diameter: 0.4 },
      feeder: { slot_count: 3 },
    },
    '0.0': { active: false, material_package_instance: packageOf('PLA') },
    '0.1': { active: true, material_package_instance: packageOf('PETG') },
    '0.2': null,
    '0.3': { active: false, material_package_instance: packageOf('ASA') },
  },
};

// The text of one row, once it is drawn. A unit is formatted with a
// non-breaking space, which the patterns below match as \s.
const rowText = async (name: RegExp): Promise<string> => {
  const row = page.getByRole('row', { name });
  await expect.element(row).toBeVisible();

  return row.element().textContent;
};

describe('PrinterTools', () => {
  it('names the feeder and says what its unit is doing', async () => {
    const busy: Tools = {
      ...mmu,
      tools: {
        ...mmu.tools,
        '0': {
          active: true,
          features: { nozzle_diameter: 0.4 },
          feeder: {
            slot_count: 3,
            model: 'MMU3',
            mmu_progress_code: 17,
            mmu_command_in_progress: 'L',
          },
        },
      },
    };

    const drawn = await render(PrinterTools, { props: { tools: busy } });

    expect(await rowText(/Tool 1/)).toMatch(/MMU3 with 3 slots/);
    await expect
      .element(page.getByText('Loading filament, step 17'))
      .toBeVisible();

    await drawn.rerender({
      tools: {
        ...busy,
        tools: {
          ...busy.tools,
          '0': {
            active: true,
            features: { nozzle_diameter: 0.4 },
            feeder: { slot_count: 3, mmu_progress_code: 0 },
          },
        },
      },
    });
    await expect.element(page.getByText('Idle')).toBeVisible();
  });

  it('says nothing of a unit whose feeder reports no progress', async () => {
    await render(PrinterTools, { props: { tools: mmu } });

    await expect.element(page.getByText(/MMU$/)).not.toBeInTheDocument();
  });

  // A printer with one nozzle is head 0 of the tree, with no feeder.
  it('draws the one head of a single-nozzle printer', async () => {
    await render(PrinterTools, {
      props: {
        tools: {
          tool_count: 1,
          tools: {
            '0': {
              type: 'FFF_PRINT_HEAD',
              active: true,
              features: { nozzle_diameter: 0.4, nozzle_hardened: true },
              feeder: null,
              material_package_instance: packageOf('PLA'),
              nozzle_temp: 214.6,
              nozzle_target_temp: null,
            },
          },
        },
      },
    });

    const head = await rowText(/Tool 1/);
    expect(head).toMatch(/0.4\s*mm/);
    expect(head).toMatch(/PLA/);
    expect(head).toMatch(/215\s*°C/);
    await expect.element(page.getByText(/Slot/)).not.toBeInTheDocument();
  });

  it('draws one row per head with its nozzle and filament', async () => {
    await render(PrinterTools, { props: { tools: xl } });

    const second = await rowText(/Tool 2/);
    expect(second).toMatch(/0.6\s*mm/);
    expect(second).toMatch(/Nothing loaded/);
    expect(await rowText(/Tool 1/)).toMatch(/PLA/);
    expect(await rowText(/Tool 1/)).toMatch(/215\s*°C/);
    expect(await rowText(/Tool 3/)).toMatch(/Switched off/);
    await expect.element(page.getByText('Tool count')).toBeVisible();
    await expectNoViolations(page.getByRole('table').element());
  });

  it('lists the slots of a feeder under its head', async () => {
    await render(PrinterTools, { props: { tools: mmu } });

    expect(await rowText(/Tool 1/)).toMatch(/A feeder with 3 slots/);
    expect(await rowText(/Slot 2/)).toMatch(/PETG/);
    expect(await rowText(/Slot 3/)).toMatch(/Switched off/);
    await expect
      .element(page.getByRole('row', { name: /Slot 4/ }))
      .not.toBeInTheDocument();

    const order = [...page.getByRole('row').elements()].map(
      (row) => row.getAttribute('data-tool') ?? '',
    );
    expect(order.filter(Boolean)).toEqual(['0', '0.0', '0.1', '0.2']);
  });

  it('says so when the printer has reported no tools', async () => {
    await render(PrinterTools, { props: { tools: undefined } });

    await expect
      .element(
        page.getByText('This printer has sent no reading of its tools yet.'),
      )
      .toBeVisible();
  });

  it('tells a head with nothing loaded from one never reported', async () => {
    await render(PrinterTools, {
      props: {
        tools: {
          tools: {
            '0': { active: false, material_package_instance: null },
            '1': { active: false },
          },
        },
      },
    });

    expect(await rowText(/Tool 1/)).toMatch(/Nothing loaded/);
    expect(await rowText(/Tool 2/)).not.toMatch(/Nothing loaded/);
  });

  it('shows the build volume the driver reports', async () => {
    await render(PrinterTools, {
      props: { tools: xl, buildVolume: { x_mm: 360, y_mm: 360, z_mm: 360 } },
    });

    await expect.element(page.getByText('Build volume')).toBeVisible();
  });

  it('marks a flag the printer did not send as no reading yet', async () => {
    await render(PrinterTools, {
      props: {
        tools: {
          tools: { '0': { active: false, features: { nozzle_diameter: 0.4 } } },
        },
      },
    });

    expect(await rowText(/Tool 1/)).toMatch(/No reading yet/);
  });

  describe('assigning from the table', () => {
    const device: Device = {
      ...fakeDevice('printer-1', 'XL'),
      connected: true,
      capabilities: [
        'core.command.set_nozzle_temperature',
        'core.command.set_nozzle_temperature#tool',
      ],
      state: { '/state': 'IDLE' },
    };
    const telemetry = { value: () => undefined, points: () => [] };

    it('heats one head with the editor of the nozzle chip', async () => {
      const onCommand = vi.fn();
      await render(PrinterTools, {
        props: {
          tools: xl,
          live: { device, telemetry, commands: { onCommand } },
        },
      });

      await page
        .getByRole('button', { name: 'Set Tool 2 extruder target' })
        .click();
      await (await openSurface()).getByRole('button', { name: 'PETG' }).click();
      await (
        await openSurface()
      )
        .getByRole('button', { name: 'Set', exact: true })
        .click();

      expect(onCommand).toHaveBeenCalledWith(
        'core.command.set_nozzle_temperature',
        { nozzle_temperature: '230', tool: '1' },
      );
    });

    it('offers no editor to a printer that is not reporting', async () => {
      await render(PrinterTools, {
        props: {
          tools: xl,
          live: {
            device: { ...device, connected: false },
            telemetry,
            commands: { onCommand: vi.fn() },
          },
        },
      });

      await expect
        .element(page.getByRole('button', { name: /extruder target/ }))
        .not.toBeInTheDocument();
    });

    it('draws the chip of the header where no extension assigns materials', async () => {
      await render(PrinterTools, {
        props: {
          tools: xl,
          live: { device: { ...device, tools: xl }, telemetry },
        },
      });

      await expect
        .element(page.getByRole('row', { name: /Tool 1/ }).getByRole('button'))
        .toHaveTextContent('PLA');
    });

    it('opens the editor of a refused head with the reason', async () => {
      await render(PrinterTools, {
        props: {
          tools: xl,
          live: {
            device,
            telemetry,
            commands: { onCommand: vi.fn(), locked: true },
          },
        },
      });

      await page
        .getByRole('button', { name: 'Set Tool 1 extruder target' })
        .click();
      await expect
        .element((await openSurface()).getByText('This printer is locked.'))
        .toBeVisible();
    });

    it('hands a head and a slot to the material picker, and a feeder not', async () => {
      const material = createRawSnippet((id: () => string) => ({
        render: () => `<button>Assign ${id()}</button>`,
      }));
      await render(PrinterTools, {
        props: { tools: { ...mmu, tools: { ...mmu.tools } }, material },
      });

      await expect
        .element(page.getByRole('button', { name: 'Assign 0.1' }))
        .toBeVisible();
      await expect
        .element(page.getByRole('button', { name: 'Assign 0', exact: true }))
        .not.toBeInTheDocument();
    });
  });
});
