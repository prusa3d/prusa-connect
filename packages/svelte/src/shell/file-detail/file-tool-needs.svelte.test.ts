import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { PrintDetails } from '../../data';
import { expectNoViolations } from '../../test/axe';
import FileToolNeeds from './file-tool-needs.svelte';

// What the server reads out of an XL file sliced for five tools, three used.
const xl: PrintDetails = {
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
    { index: 2, used: false, material: 'ASA', nozzle_mm: 0.4 },
  ],
};

// The text of one row, once it is drawn. A unit is formatted with a
// non-breaking space, which the patterns below match as \s.
const rowText = async (name: RegExp): Promise<string> => {
  const row = page.getByRole('row', { name });
  await expect.element(row).toBeVisible();

  return row.element().textContent;
};

describe('FileToolNeeds', () => {
  it('draws one row per tool and says which the print uses', async () => {
    await render(FileToolNeeds, { props: { details: xl } });

    const second = await rowText(/Tool 2/);
    expect(second).toMatch(/PETG/);
    expect(second).toMatch(/250\s*°C/);
    expect(await rowText(/Tool 3/)).toMatch(/Not used/);
    await expectNoViolations(page.getByRole('table').element());
  });

  it('lists what the G-code asks the printer to check', async () => {
    await render(FileToolNeeds, { props: { details: xl } });

    await expect
      .element(page.getByText('What the printer checks'))
      .toBeVisible();
    await expect.element(page.getByText('Input shaper')).toBeVisible();
    await expect.element(page.getByText('360 mm by 360 mm')).toBeVisible();
  });

  it('draws no table for a file sliced for one filament', async () => {
    await render(FileToolNeeds, {
      props: {
        details: { filaments: [{ index: 0, used: true, material: 'PLA' }] },
      },
    });

    await expect.element(page.getByRole('table')).not.toBeInTheDocument();
  });
});
