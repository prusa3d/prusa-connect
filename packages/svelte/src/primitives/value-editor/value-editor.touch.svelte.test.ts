import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { NOZZLE_PRESETS } from '../../test/nozzle-presets';
import ValueEditor from './value-editor.svelte';

describe('ValueEditor under a finger', () => {
  it('stands its trigger on the 44px floor under a finger', async () => {
    await render(ValueEditor, {
      props: {
        label: 'Nozzle',
        subject: 'Tool 1',
        currentLabel: '234.6°C',
        target: 235,
        unit: '°C',
        min: 0,
        max: 300,
        step: 5,
        presets: NOZZLE_PRESETS,
        onCommit: vi.fn(),
      },
    });
    const style = getComputedStyle(
      page.getByRole('button', { name: 'Set Tool 1 nozzle target' }).element(),
    );

    expect(parseFloat(style.height)).toBeGreaterThanOrEqual(44);
    expect(parseFloat(style.width)).toBeGreaterThanOrEqual(44);
  });
});
