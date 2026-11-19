import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import FieldPickerProbe from '../../test/field-picker-probe.svelte';
import type { ListField } from './types';

/** The panel the trigger opens, once it is there. */
const panel = (): HTMLElement | null =>
  document.querySelector('[data-slot="popover-content"]');

/** A listing whose last field is put away until a reader asks for it. */
const withHidden: ListField[] = [
  { id: 'file', label: 'File', required: true },
  { id: 'printer', label: 'Printer' },
  { id: 'started', label: 'Started', hiddenByDefault: true },
];

const open = async (name: string): Promise<void> => {
  await page.getByRole('button', { name }).click();
  await expect.poll(panel).not.toBeNull();
};

describe('FieldPicker', () => {
  it('names the icon-only trigger and lists every field behind it', async () => {
    await render(FieldPickerProbe, {});

    const trigger = page.getByRole('button', { name: 'Fields' });

    trigger.element().focus();
    await expect.element(page.getByRole('tooltip')).toHaveTextContent('Fields');

    await open('Fields');

    // Every field is offered, the ones drawn now ticked.
    for (const label of ['File', 'Printer', 'State']) {
      await expect
        .element(page.getByRole('button', { name: new RegExp(label) }))
        .toHaveAttribute('aria-pressed', 'true');
    }
  });

  it('draws a square trigger at every size', async () => {
    for (const size of ['sm', 'md', 'lg'] as const) {
      const { unmount } = await render(FieldPickerProbe, { size });
      const box = page.getByRole('button', { name: 'Fields' }).element();
      const style = getComputedStyle(box);

      expect(style.width, size).toBe(style.height);
      await unmount();
    }
  });

  it('offers a required field ticked and unpickable, and says why', async () => {
    await render(FieldPickerProbe, {});
    await open('Fields');

    const file = page.getByRole('button', { name: /File/ });

    await expect.element(file).toBeDisabled();
    await expect.element(file).toHaveAttribute('aria-pressed', 'true');
    // A reader looking for File finds it, and reads why it cannot go.
    // The hint follows the label in the same line, parted by a hyphen.
    expect(file.element().textContent.replace(/\s+/g, ' ').trim()).toBe(
      'File - Always shown',
    );
  });

  it('counts what is put away, on the glyph and in the name', async () => {
    await render(FieldPickerProbe, {});
    await open('Fields');

    await page.getByRole('button', { name: /Printer/ }).click();

    const trigger = page.getByRole('button', { name: 'Fields, 1 hidden' });
    await expect.element(trigger).toBeInTheDocument();
    await expect
      .element(page.getByTestId('visible'))
      .toHaveTextContent('file state');
    // The count rides the glyph, and it is drawn for the eye alone.
    const badge = trigger.element().lastElementChild;
    expect(badge?.getAttribute('aria-hidden')).toBe('true');
    expect(badge?.textContent.trim()).toBe('1');
  });

  it('counts a field hidden by default without calling the listing changed', async () => {
    await render(FieldPickerProbe, { props: { fields: withHidden } });

    // One field is put away, so the trigger says so.
    await expect
      .element(page.getByRole('button', { name: 'Fields, 1 hidden' }))
      .toBeInTheDocument();

    await open('Fields, 1 hidden');

    // Nothing has been changed yet, so there is nothing to go back to.
    expect(page.getByRole('button', { name: 'Reset to default' }).query()).toBe(
      null,
    );
  });

  // The storage table holds Modified but offers no toggle for it.
  it('judges the defaults by the fields it offers alone', async () => {
    await render(FieldPickerProbe, {
      props: {
        fields: [
          { id: 'file', label: 'File', required: true },
          { id: 'printer', label: 'Printer' },
        ],
        initial: ['file', 'printer', 'modified'],
      },
    });
    await open('Fields');

    expect(page.getByRole('button', { name: 'Reset to default' }).query()).toBe(
      null,
    );
  });

  it('offers the way back once something has changed, and takes it', async () => {
    await render(FieldPickerProbe, { props: { fields: withHidden } });
    await open('Fields, 1 hidden');

    await page.getByRole('button', { name: /Started/ }).click();
    await expect
      .element(page.getByTestId('visible'))
      .toHaveTextContent('file printer started');

    const reset = page.getByRole('button', { name: 'Reset to default' });
    await expect.element(reset).toBeInTheDocument();

    await reset.click();

    await expect
      .element(page.getByTestId('visible'))
      .toHaveTextContent('file printer');
    expect(page.getByRole('button', { name: 'Reset to default' }).query()).toBe(
      null,
    );
  });

  it('opens from the keyboard and shuts on Escape', async () => {
    await render(FieldPickerProbe, {});

    await userEvent.tab();
    await expect
      .element(page.getByRole('button', { name: 'Fields' }))
      .toHaveFocus();

    await userEvent.keyboard('{Enter}');
    await expect.poll(panel).not.toBeNull();

    await userEvent.keyboard('{Escape}');
    await expect.poll(panel).toBeNull();
    await expect
      .element(page.getByRole('button', { name: 'Fields' }))
      .toHaveFocus();
  });

  it('has nothing axe refuses, shut and open', async () => {
    const { container } = await render(FieldPickerProbe, {
      props: { fields: withHidden },
    });

    await expectNoViolations(container);

    await open('Fields, 1 hidden');

    await expectNoViolations(panel()!);
  });
});
