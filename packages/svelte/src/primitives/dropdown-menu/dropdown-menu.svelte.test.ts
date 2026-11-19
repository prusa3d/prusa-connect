import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import DropdownMenuProbe from '../../test/dropdown-menu-probe.svelte';

const trigger = () => page.getByRole('button', { name: 'Printer actions' });

const surface = (): HTMLElement | null =>
  document.querySelector<HTMLElement>('[data-slot="dropdown-menu-content"]');

describe('DropdownMenu', () => {
  it('opens on Enter and returns focus to the trigger on Escape', async () => {
    await render(DropdownMenuProbe);

    trigger().element().focus();
    await userEvent.keyboard('{Enter}');

    await expect.poll(surface).not.toBe(null);
    expect(surface()?.getAttribute('role')).toBe('menu');
    await expect.element(trigger()).toHaveAttribute('aria-expanded', 'true');

    await userEvent.keyboard('{Escape}');

    await expect.poll(surface).toBe(null);
    await expect.element(trigger()).toHaveFocus();
    await expect.element(trigger()).toHaveAttribute('aria-expanded', 'false');
  });

  it('moves the active item with the arrow keys', async () => {
    await render(DropdownMenuProbe);

    await trigger().click();
    await expect.poll(surface).not.toBe(null);

    await userEvent.keyboard('{ArrowDown}');
    await expect
      .element(page.getByRole('menuitem', { name: /Rename/ }))
      .toHaveFocus();

    await userEvent.keyboard('{ArrowDown}');
    await expect
      .element(page.getByRole('menuitem', { name: 'Duplicate' }))
      .toHaveFocus();
    await expect
      .element(page.getByRole('menuitem', { name: /Rename/ }))
      .not.toHaveFocus();
  });

  it('follows activation with aria-checked on a checkbox item', async () => {
    await render(DropdownMenuProbe);

    await trigger().click();
    await expect.poll(surface).not.toBe(null);

    const box = page.getByRole('menuitemcheckbox', { name: 'Auto-connect' });
    await expect.element(box).toHaveAttribute('aria-checked', 'false');

    await box.click();
    await expect.poll(surface).toBe(null);

    await trigger().click();
    await expect.poll(surface).not.toBe(null);
    await expect
      .element(page.getByRole('menuitemcheckbox', { name: 'Auto-connect' }))
      .toHaveAttribute('aria-checked', 'true');
  });

  it('runs an item and refuses a disabled one', async () => {
    const onRename = vi.fn();
    const onRecalibrate = vi.fn();
    await render(DropdownMenuProbe, { props: { onRename, onRecalibrate } });

    await trigger().click();
    await expect.poll(surface).not.toBe(null);

    const blocked = page.getByRole('menuitem', { name: 'Recalibrate' });
    await expect.element(blocked).toHaveAttribute('aria-disabled', 'true');
    await blocked.click({ force: true });
    expect(onRecalibrate).not.toHaveBeenCalled();

    await page.getByRole('menuitem', { name: /Rename/ }).click();
    expect(onRename).toHaveBeenCalledTimes(1);
  });

  // The two variants have to be told apart on the row itself, because the
  // destructive colour is what warns a person before they press it.
  it('marks a destructive item apart from a plain one', async () => {
    await render(DropdownMenuProbe);

    await trigger().click();
    await expect.poll(surface).not.toBe(null);

    const remove = page.getByRole('menuitem', { name: 'Remove' }).element();
    const rename = page.getByRole('menuitem', { name: /Rename/ }).element();

    expect(remove.getAttribute('data-variant')).toBe('destructive');
    expect(rename.getAttribute('data-variant')).toBe('default');
    expect(remove.getAttribute('class')).toContain(
      'variant-destructive:text-destructive-text',
    );
    expect(getComputedStyle(remove).color).not.toBe(
      getComputedStyle(rename).color,
    );
  });

  // Under a mouse a command row stands on the compact control height, the
  // floor a facet row keeps too. The 44px touch floor is for a coarse pointer
  // only, and the checkbox row carries no floor at all.
  it('stands a command row on the compact floor under a mouse', async () => {
    await render(DropdownMenuProbe);

    await trigger().click();
    await expect.poll(surface).not.toBe(null);

    const command = page.getByRole('menuitem', { name: 'Duplicate' }).element();
    const unraised = page
      .getByRole('menuitemcheckbox', { name: 'Auto-connect' })
      .element();
    const floor = Number.parseFloat(getComputedStyle(command).minHeight);

    expect(floor).toBeGreaterThan(0);
    expect(floor).toBeLessThan(44);
    expect(getComputedStyle(unraised).minHeight).toBe('0px');
  });

  // The exit animation is what keeps the closing surface on the screen long
  // enough to be seen going.
  it('scales the surface out while it closes', async () => {
    await render(DropdownMenuProbe);

    await trigger().click();
    await expect.poll(surface).not.toBe(null);

    const closing = surface();
    if (closing === null) throw new Error('no surface rendered');

    await userEvent.keyboard('{Escape}');

    expect(closing.getAttribute('data-state')).toBe('closed');
    expect(getComputedStyle(closing).animationName).toBe('scale-out');
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(DropdownMenuProbe);

    await trigger().click();
    await expect.poll(surface).not.toBe(null);

    await expectNoViolations(container);
    await expectNoViolations(surface()!);
  });
});
