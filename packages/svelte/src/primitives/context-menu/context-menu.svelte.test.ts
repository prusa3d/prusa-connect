import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import ContextMenuProbe from '../../test/context-menu-probe.svelte';

const trigger = () => page.getByText('Right-click this printer');

const surface = (): HTMLElement | null =>
  document.querySelector<HTMLElement>('[data-slot="context-menu-content"]');

const subSurface = (): HTMLElement | null =>
  document.querySelector<HTMLElement>('[data-slot="context-menu-sub-content"]');

/**
 * The right button press bits-ui opens the menu on, at a point on the page.
 *
 * A real pointer press focuses the region it lands on, and the keyboard walk
 * through the menu starts from there, so the focus is set here too.
 */
const rightClick = (): void => {
  const region = trigger().element() as HTMLElement;
  region.focus();
  region.dispatchEvent(
    new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      clientX: 40,
      clientY: 40,
    }),
  );
};

/** Whether the keyboard is inside the menu, which is where a walk starts. */
const focusIsInMenu = (): boolean =>
  surface()?.contains(document.activeElement) ?? false;

/**
 * Opens the menu and waits for the keyboard to be inside it.
 *
 * bits-ui mounts the surface first and moves focus into it a frame later, once
 * the floating position has settled. An arrow key pressed in between goes to
 * the page and is lost.
 */
const openMenu = async (): Promise<void> => {
  rightClick();
  await expect.poll(focusIsInMenu).toBe(true);
};

describe('ContextMenu', () => {
  it('opens on a context menu event and returns focus to the trigger on Escape', async () => {
    await render(ContextMenuProbe);

    await openMenu();
    expect(surface()?.getAttribute('role')).toBe('menu');

    await userEvent.keyboard('{ArrowDown}');
    await expect
      .element(page.getByRole('menuitem', { name: /Rename/ }))
      .toHaveFocus();

    await userEvent.keyboard('{Escape}');

    await expect.poll(surface).toBe(null);
    await expect.element(trigger()).toHaveFocus();
  });

  it('moves the active item with the arrow keys', async () => {
    await render(ContextMenuProbe);

    await openMenu();

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
    await render(ContextMenuProbe);

    await openMenu();

    const box = page.getByRole('menuitemcheckbox', { name: 'Auto-connect' });
    await expect.element(box).toHaveAttribute('aria-checked', 'false');

    await box.click();
    await expect.poll(surface).toBe(null);

    await openMenu();
    await expect
      .element(page.getByRole('menuitemcheckbox', { name: 'Auto-connect' }))
      .toHaveAttribute('aria-checked', 'true');
  });

  it('runs an item and refuses a disabled one', async () => {
    const onRename = vi.fn();
    const onRecalibrate = vi.fn();
    await render(ContextMenuProbe, { props: { onRename, onRecalibrate } });

    await openMenu();

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
    await render(ContextMenuProbe);

    await openMenu();

    const remove = page.getByRole('menuitem', { name: 'Remove' }).element();
    const rename = page.getByRole('menuitem', { name: /Rename/ }).element();

    expect(remove.getAttribute('data-variant')).toBe('destructive');
    expect(rename.getAttribute('data-variant')).toBe('default');
    expect(
      remove.classList.contains('variant-destructive:text-destructive-text'),
    ).toBe(true);
    expect(getComputedStyle(remove).color).not.toBe(
      getComputedStyle(rename).color,
    );
  });

  // The submenu is floating content of its own, anchored to its row rather
  // than to the pointer. A surface that never reaches the page, or one that
  // reaches it with no items, is what a wrong snippet shape looks like here.
  it('opens a submenu from its row and runs an item in it', async () => {
    const onMove = vi.fn();
    await render(ContextMenuProbe, { props: { onMove } });

    await openMenu();

    const row = page.getByRole('menuitem', { name: 'Move to' });
    await expect.element(row).toHaveAttribute('aria-expanded', 'false');

    await row.click();

    await expect.element(row).toHaveAttribute('aria-expanded', 'true');
    await expect.poll(subSurface).not.toBe(null);
    expect(subSurface()?.getAttribute('role')).toBe('menu');

    await page.getByRole('menuitem', { name: 'Workshop' }).click();
    expect(onMove).toHaveBeenCalledTimes(1);
  });

  // The dot is drawn from the state bits-ui hands the row, so the marked row
  // and the cleared one are the two halves of that snippet.
  it('moves the mark to the radio item that was chosen', async () => {
    await render(ContextMenuProbe);

    await openMenu();

    const draft = page.getByRole('menuitemradio', { name: 'Draft' });
    const fine = page.getByRole('menuitemradio', { name: 'Fine' });
    await expect.element(draft).toHaveAttribute('aria-checked', 'true');
    await expect.element(fine).toHaveAttribute('aria-checked', 'false');

    await fine.click();
    await expect.poll(surface).toBe(null);

    await openMenu();
    await expect
      .element(page.getByRole('menuitemradio', { name: 'Fine' }))
      .toHaveAttribute('aria-checked', 'true');
    await expect
      .element(page.getByRole('menuitemradio', { name: 'Draft' }))
      .toHaveAttribute('aria-checked', 'false');
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(ContextMenuProbe);

    await openMenu();

    await expectNoViolations(container);
    await expectNoViolations(surface()!);
  });
});
