import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import RowActionsProbe from '../../test/row-actions-probe.svelte';

const NAME = 'Bracket_Left_Reinforced_v4_FINAL.gcode';

const trigger = () => page.getByRole('button', { name: `Actions for ${NAME}` });

const menu = (): HTMLElement | null =>
  document.body.querySelector<HTMLElement>(
    '[data-slot="dropdown-menu-content"]',
  );

const items = (): HTMLElement[] => [
  ...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]'),
];

describe('RowActions', () => {
  it('names the trigger after the row it acts on', async () => {
    await render(RowActionsProbe);

    const button = trigger().element();
    expect(button.getAttribute('aria-expanded')).toBe('false');
    // One glyph and no words, so the name is the only thing a screen reader
    // has to tell one row's menu from the next.
    expect(button.textContent.trim()).toBe('');
  });

  // Read the computed style and not the bounding rectangle: the runner
  // renders in a scaled frame, and the rule is what is under test.
  const heights = async (): Promise<number[]> => {
    await render(RowActionsProbe);
    const button = parseFloat(getComputedStyle(trigger().element()).height);

    await trigger().click();
    await expect.poll(menu).not.toBeNull();
    expect(items()).toHaveLength(2);

    return [
      button,
      ...items().map((item) => parseFloat(getComputedStyle(item).height)),
    ];
  };

  it('keeps the trigger and every item at least 24px under a mouse', async () => {
    for (const height of await heights()) {
      expect(height).toBeGreaterThanOrEqual(24);
    }
  });

  it('opens from the keyboard and returns focus to the row on Escape', async () => {
    await render(RowActionsProbe);

    trigger().element().focus();
    await userEvent.keyboard('{Enter}');

    await expect.poll(menu).not.toBeNull();
    await expect.element(trigger()).toHaveAttribute('aria-expanded', 'true');

    // bits-ui lands on the first item, so the menu is usable without a mouse
    // from the moment it opens.
    // It moves focus a frame after the menu mounts, so this waits for it.
    await expect
      .poll(() => document.activeElement?.textContent.trim())
      .toBe('Print again');

    await userEvent.keyboard('{Escape}');
    await expect.poll(menu).toBeNull();
    await expect.element(trigger()).toHaveFocus();
  });

  it('calls back the item that was picked and shuts', async () => {
    const picked: string[] = [];
    await render(RowActionsProbe, {
      props: { onPick: (it: string) => picked.push(it) },
    });

    await trigger().click();
    await expect.poll(menu).not.toBeNull();

    items()[0]?.click();

    await expect.poll(menu).toBeNull();
    expect(picked).toStrictEqual(['again']);
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(RowActionsProbe);

    await expectNoViolations(container);

    await trigger().click();
    await expect.poll(menu).not.toBeNull();

    await expectNoViolations(menu()!);
  });
});
