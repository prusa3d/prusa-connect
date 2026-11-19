import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import SelectProbe from '../../test/select-probe.svelte';

const trigger = () => page.getByLabelText('Printer model');

/** What the caller's `bind:value` last carried back, as the probe draws it. */
const chosen = (): string =>
  document.querySelector<HTMLElement>('[data-testid="chosen"]')?.textContent ??
  '';

const surface = (): HTMLElement | null =>
  document.querySelector<HTMLElement>('[data-slot="select-content"]');

const option = (label: string): HTMLElement | null =>
  document.querySelector<HTMLElement>(
    `[data-slot="select-item"][data-label="${label}"]`,
  );

const open = async (): Promise<void> => {
  trigger().element().focus();
  await userEvent.keyboard('{Enter}');
  await expect.poll(surface).not.toBe(null);
};

describe('Select', () => {
  it('opens on Enter, walks the options with the arrows and keeps the one Enter picks', async () => {
    await render(SelectProbe);

    await expect.element(trigger()).toHaveAttribute('aria-expanded', 'false');

    await open();

    await expect.element(trigger()).toHaveAttribute('aria-expanded', 'true');
    // bits-ui leaves focus on the surface and points at the live option with
    // `aria-activedescendant`, where Radix moves focus onto the option itself.
    await expect.poll(() => option('MK4S')?.dataset.highlighted).toBe('');
    expect(trigger().element().getAttribute('aria-activedescendant')).toBe(
      option('MK4S')?.id,
    );

    await userEvent.keyboard('{ArrowDown}');
    await expect.poll(() => option('MINI+')?.dataset.highlighted).toBe('');

    await userEvent.keyboard('{Enter}');

    await expect.poll(surface).toBe(null);
    await expect.element(trigger()).toHaveTextContent('MINI+');
    // The value travelled back out through the caller's binding.
    await expect.poll(chosen).toBe('mini');
  });

  // bits-ui reads an option's text off the option's own element, which is
  // unmounted while the list is shut, and falls back to the raw value. The
  // `items` the caller passes are what keep the label there, and a select that
  // reads `mk4s` where it should read `MK4S` is what this catches.
  it('keeps the label of the chosen option in the shut trigger', async () => {
    await render(SelectProbe);

    await expect.element(trigger()).toHaveTextContent('MK4S');

    await open();
    await userEvent.keyboard('{ArrowDown}{Enter}');
    await expect.poll(surface).toBe(null);

    await expect.element(trigger()).toHaveTextContent('MINI+');
  });

  it('jumps to the option whose label is typed', async () => {
    await render(SelectProbe);

    await open();
    await userEvent.keyboard('sl');

    await expect.poll(() => option('SL1S SPEED')?.dataset.highlighted).toBe('');

    await userEvent.keyboard('{Enter}');

    await expect.poll(chosen).toBe('sl1s');
  });

  it('marks the chosen option and moves the mark when another is picked', async () => {
    await render(SelectProbe);

    await open();
    expect(option('MK4S')?.getAttribute('aria-selected')).toBe('true');
    expect(option('MINI+')?.getAttribute('aria-selected')).toBe(null);

    await userEvent.keyboard('{ArrowDown}{Enter}');
    await expect.poll(surface).toBe(null);

    await open();
    expect(option('MINI+')?.getAttribute('aria-selected')).toBe('true');
    expect(option('MK4S')?.getAttribute('aria-selected')).toBe(null);
  });

  it('walks past a disabled option and never picks it', async () => {
    await render(SelectProbe, { props: { value: 'mini' } });

    await open();
    await expect.poll(() => option('MINI+')?.dataset.highlighted).toBe('');

    // XL sits between MINI+ and SL1S SPEED and is disabled. The keyboard skips
    // it, and `pointer-events-none` is what keeps a press off it as well.
    await userEvent.keyboard('{ArrowDown}');

    expect(option('XL')?.dataset.highlighted).toBeUndefined();
    expect(getComputedStyle(option('XL')!).pointerEvents).toBe('none');
    await expect.poll(() => option('SL1S SPEED')?.dataset.highlighted).toBe('');

    await userEvent.keyboard('{Enter}');
    await expect.poll(chosen).toBe('sl1s');
  });

  // `data-placeholder` is the hook the trigger's two placeholder classes hang
  // off, so the attribute and the text are asserted together.
  it('shows the placeholder while nothing is chosen', async () => {
    await render(SelectProbe, { props: { value: '' } });

    await expect.element(trigger()).toHaveTextContent('Select a printer');
    await expect.element(trigger()).toHaveAttribute('data-placeholder');
  });

  it('closes on Escape and gives the focus back to the trigger', async () => {
    await render(SelectProbe);

    await open();
    await userEvent.keyboard('{Escape}');

    await expect.poll(surface).toBe(null);
    await expect.element(trigger()).toHaveFocus();
    await expect.element(trigger()).toHaveAttribute('aria-expanded', 'false');
  });

  it('does not reach a disabled trigger by keyboard', async () => {
    await render(SelectProbe, { props: { disabled: true } });

    await expect.element(trigger()).toBeDisabled();

    await userEvent.tab();

    await expect.element(trigger()).not.toHaveFocus();
    expect(surface()).toBe(null);
  });

  // Every step of the control scale is drawn at one height. A larger step
  // is never the shorter one.
  it('never shrinks from one step of the control scale to the next', async () => {
    const heightOf = async (size: 'sm' | 'md' | 'lg'): Promise<number> => {
      const { container } = await render(SelectProbe, { props: { size } });
      const found = container.querySelector(`[data-size=${size}]`);
      if (found === null) throw new Error('no trigger rendered');
      return found.getBoundingClientRect().height;
    };

    const small = await heightOf('sm');
    const medium = await heightOf('md');
    const large = await heightOf('lg');

    expect(small).toBeGreaterThan(0);
    expect(small).toBeLessThanOrEqual(medium);
    expect(medium).toBeLessThanOrEqual(large);
  });

  // The React classes read Radix custom properties. bits-ui publishes the same
  // measurements under its own names, and the surface declares the Radix names
  // from the bits-ui ones. A rename on either side leaves the class reading
  // nothing and reports no error, so each pair is compared here.
  it('declares the Radix measurements from the bits-ui ones', async () => {
    await render(SelectProbe);
    await open();

    const styles = getComputedStyle(surface()!);
    const read = (name: string): string => styles.getPropertyValue(name).trim();
    const pairs: [string, string][] = [
      [
        '--radix-select-content-available-height',
        '--bits-select-content-available-height',
      ],
      [
        '--radix-select-content-transform-origin',
        '--bits-select-content-transform-origin',
      ],
      ['--radix-select-trigger-height', '--bits-select-anchor-height'],
      ['--radix-select-trigger-width', '--bits-select-anchor-width'],
    ];
    for (const [radix, bits] of pairs) {
      expect(read(bits), `${bits} is gone`).not.toBe('');
      expect(read(radix), `${radix} does not read ${bits}`).toBe(read(bits));
    }
  });

  it('holds the surface to the width of the trigger', async () => {
    await render(SelectProbe);
    await open();

    const viewport = surface()!.querySelector<HTMLElement>(
      '[data-select-viewport]',
    )!;
    expect(parseFloat(getComputedStyle(viewport).minWidth)).toBeCloseTo(
      trigger().element().getBoundingClientRect().width,
      1,
    );
  });

  // The viewport carries React's popper-mode `h-(--radix-select-trigger-height)`.
  // bits-ui's inline `flex: 1` decides the height instead; were that to change,
  // the list would stand one row tall and every option but the first would be
  // out of reach.
  it('draws the whole list rather than one row of it', async () => {
    await render(SelectProbe);
    await open();

    const viewport = surface()!.querySelector<HTMLElement>(
      '[data-select-viewport]',
    )!;
    expect(document.querySelectorAll('[data-slot="select-item"]').length).toBe(
      4,
    );
    expect(viewport.getBoundingClientRect().height).toBeGreaterThan(
      trigger().element().getBoundingClientRect().height * 3,
    );
  });

  // The closed control is what axe judges here, which is the scope the React
  // design system settled on: it ships no open-state story because the open
  // listbox is flagged. bits-ui flags two rules, both from its own structure.
  // `aria-input-field-name`, because it gives the trigger's name to no one but
  // the trigger, and `aria-required-children`, because the viewport it puts
  // inside the listbox carries `role="presentation"`. Neither is reachable
  // from here.
  it('has nothing axe objects to', async () => {
    const { container } = await render(SelectProbe);

    await expectNoViolations(container);
  });
});
