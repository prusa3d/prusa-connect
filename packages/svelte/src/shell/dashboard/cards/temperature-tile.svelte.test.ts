import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { Device } from '../../../domain';
import { fakeDevice } from '../../../test/fakes';
import {
  DESCRIBED_BED,
  DESCRIBED_CHAMBER,
  DESCRIBED_ENCLOSURE,
} from '../../../test/readings';
import { chartedContextOver } from '../fixtures';
import TileStory from '../tile-story.svelte';
import TemperatureTile from './temperature-tile.svelte';

const printer = (state: Record<string, unknown>): Device => ({
  ...fakeDevice('core', 'CORE One bench'),
  state: { '/state': 'PRINTING', ...state },
});

const draw = async (device: Device, config: Record<string, unknown> = {}) =>
  render(TileStory, {
    props: {
      context: chartedContextOver(device),
      tile: TemperatureTile,
      type: 'temperature',
      config: { device: device.id, ...config },
    },
  });

const legend = (container: HTMLElement): string[] =>
  [...container.querySelectorAll('ul:not(.sr-only) > li')].map((item) =>
    item.textContent.trim(),
  );

describe('the temperature card', () => {
  it('draws a chamber the printer describes', async () => {
    const { container } = await draw(
      printer({ ...DESCRIBED_BED, ...DESCRIBED_CHAMBER }),
    );

    await expect.element(page.getByText('Chamber target')).toBeVisible();
    expect(legend(container)).toEqual([
      'Nozzle',
      'Nozzle target',
      'Bed',
      'Bed target',
      'Chamber',
      'Chamber target',
    ]);
  });

  it('draws no chamber for a printer that describes none', async () => {
    const { container } = await draw(printer({ ...DESCRIBED_BED }));

    await expect.element(page.getByText('Bed target')).toBeVisible();
    expect(legend(container)).not.toContain('Chamber');
  });

  it('draws no bed for a printer that describes no readings', async () => {
    const { container } = await draw(
      printer({ '/temperature/bed': 60, '/temperature/bed_target': 60 }),
    );

    await expect.element(page.getByText('Nozzle target')).toBeVisible();
    expect(legend(container)).toEqual(['Nozzle', 'Nozzle target']);
  });

  it('names a reading with no role by the name its driver gave it', async () => {
    const { container } = await draw(
      printer({ ...DESCRIBED_BED, ...DESCRIBED_ENCLOSURE }),
      { heaters: ['bed', 'enclosure'] },
    );

    await expect.element(page.getByText('Enclosure')).toBeVisible();
    expect(legend(container)).toEqual(['Bed', 'Bed target', 'Enclosure']);
  });

  it('keeps drawing the bed of a card saved with the nozzle and the bed', async () => {
    const { container } = await draw(
      printer({ ...DESCRIBED_BED, ...DESCRIBED_CHAMBER }),
      { heaters: ['nozzle', 'bed'] },
    );

    await expect.element(page.getByText('Bed target')).toBeVisible();
    expect(legend(container)).toEqual([
      'Nozzle',
      'Nozzle target',
      'Bed',
      'Bed target',
    ]);
  });
});
