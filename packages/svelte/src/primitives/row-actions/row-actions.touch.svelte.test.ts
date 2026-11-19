import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import RowActionsProbe from '../../test/row-actions-probe.svelte';

const trigger = () =>
  page.getByRole('button', {
    name: 'Actions for Bracket_Left_Reinforced_v4_FINAL.gcode',
  });

const menu = (): HTMLElement | null =>
  document.body.querySelector<HTMLElement>(
    '[data-slot="dropdown-menu-content"]',
  );

const items = (): HTMLElement[] => [
  ...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]'),
];

describe('RowActions under a finger', () => {
  // Read the computed style and not the bounding rectangle: the runner
  // renders in a scaled frame, and the rule is what is under test.
  it('stands the trigger and every item on the 44px touch floor under a finger', async () => {
    await render(RowActionsProbe);
    const button = parseFloat(getComputedStyle(trigger().element()).height);

    await trigger().click();
    await expect.poll(menu).not.toBeNull();
    expect(items()).toHaveLength(2);

    for (const height of [
      button,
      ...items().map((item) => parseFloat(getComputedStyle(item).height)),
    ]) {
      expect(height).toBeGreaterThanOrEqual(44);
    }
  });
});
