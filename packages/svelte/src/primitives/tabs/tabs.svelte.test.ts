import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import TabsProbe from '../../test/tabs-probe.svelte';

const panels = [
  { value: 'printer', label: 'Printer', body: 'MK4S is ready to print.' },
  { value: 'camera', label: 'Camera', body: 'No camera connected.' },
  { value: 'files', label: 'Files', body: '12 files on USB.' },
];

const withDisabled = panels.map((panel) =>
  panel.value === 'camera' ? { ...panel, disabled: true } : panel,
);

const tab = (name: string) => page.getByRole('tab', { name });

/** The panel a trigger points at, by the `aria-controls` pairing. */
const panelOf = (name: string): HTMLElement => {
  const id = tab(name).element().getAttribute('aria-controls');
  const found = id === null ? null : document.getElementById(id);
  if (found === null) throw new Error(`no panel for ${name}`);
  return found;
};

describe('Tabs', () => {
  it('moves the active tab with an arrow key and shows that panel', async () => {
    await render(TabsProbe, { props: { panels, value: 'printer' } });

    await userEvent.tab();
    await expect.element(tab('Printer')).toHaveFocus();

    await userEvent.keyboard('{ArrowRight}');

    await expect.element(tab('Camera')).toHaveFocus();
    await expect
      .element(tab('Camera'))
      .toHaveAttribute('aria-selected', 'true');
    await expect
      .element(tab('Printer'))
      .toHaveAttribute('aria-selected', 'false');
    expect(panelOf('Camera').textContent).toContain('No camera connected.');
    expect(panelOf('Camera').hidden).toBe(false);
    expect(panelOf('Printer').hidden).toBe(true);
  });

  it('steps over a disabled trigger', async () => {
    await render(TabsProbe, {
      props: { panels: withDisabled, value: 'printer' },
    });

    await userEvent.tab();
    await userEvent.keyboard('{ArrowRight}');

    await expect.element(tab('Files')).toHaveFocus();
    await expect.element(tab('Files')).toHaveAttribute('aria-selected', 'true');
    await expect
      .element(tab('Camera'))
      .toHaveAttribute('aria-selected', 'false');
  });

  it('answers nothing for a disabled trigger', async () => {
    const onValueChange = vi.fn();
    await render(TabsProbe, {
      props: { panels: withDisabled, value: 'printer', onValueChange },
    });

    // Not the locator's click: a disabled control takes no pointer events, so
    // the actionability wait would time out rather than report the block.
    tab('Camera').element().dispatchEvent(new MouseEvent('click'));

    expect(onValueChange).not.toHaveBeenCalled();
    await expect
      .element(tab('Printer'))
      .toHaveAttribute('aria-selected', 'true');
  });

  it('marks the active trigger apart from the rest', async () => {
    await render(TabsProbe, { props: { panels, value: 'printer' } });

    expect(tab('Printer').element().getAttribute('data-state')).toBe('active');
    expect(tab('Camera').element().getAttribute('data-state')).toBe('inactive');
    // The underline is the only mark, and it is drawn by this variant alone.
    expect(tab('Printer').element().getAttribute('class')).toContain(
      'state-active:border-selected',
    );
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(TabsProbe, {
      props: { panels, value: 'printer' },
    });

    await expectNoViolations(container);
  });
});
