import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import Checkbox from './checkbox.svelte';

const label = 'Auto-connect';

describe('Checkbox', () => {
  it('ticks on Space and reports the new state', async () => {
    const onCheckedChange = vi.fn();
    await render(Checkbox, { props: { onCheckedChange, 'aria-label': label } });

    const control = page.getByRole('checkbox', { name: label });
    await expect.element(control).toHaveAttribute('aria-checked', 'false');

    await userEvent.tab();
    await expect.element(control).toHaveFocus();

    await userEvent.keyboard(' ');
    await expect.element(control).toHaveAttribute('aria-checked', 'true');
    expect(onCheckedChange).toHaveBeenLastCalledWith(true);
  });

  // Every colour in this component is keyed on `data-state`. A headless layer
  // that spelled the state differently would leave the box unstyled and say
  // nothing about it.
  it('writes the state the colours are keyed on', async () => {
    await render(Checkbox, { props: { 'aria-label': label } });

    const control = page.getByRole('checkbox');
    await expect.element(control).toHaveAttribute('data-state', 'unchecked');

    await userEvent.tab();
    await userEvent.keyboard(' ');
    await expect.element(control).toHaveAttribute('data-state', 'checked');
  });

  it('blocks the keyboard and the callback while disabled', async () => {
    const onCheckedChange = vi.fn();
    await render(Checkbox, {
      props: { onCheckedChange, disabled: true, 'aria-label': label },
    });

    const control = page.getByRole('checkbox');
    await userEvent.tab();
    await expect.element(control).not.toHaveFocus();

    await control.click({ force: true });
    expect(onCheckedChange).not.toHaveBeenCalled();
    await expect.element(control).toHaveAttribute('aria-checked', 'false');
  });

  // Read from the paint and not from the class string. `disabled:*` and
  // `state-checked:*` tie on specificity, and the disabled colour can
  // lose a tie without any class going missing.
  it('paints a disabled ticked box away from the live colour', async () => {
    const live = await render(Checkbox, {
      props: { checked: true, 'aria-label': label },
    });
    const painted = getComputedStyle(
      page.getByRole('checkbox').element(),
    ).backgroundColor;
    await live.unmount();

    await render(Checkbox, {
      props: { checked: true, disabled: true, 'aria-label': label },
    });
    expect(
      getComputedStyle(page.getByRole('checkbox').element()).backgroundColor,
    ).not.toBe(painted);
  });

  // The disabled fill is the surface's own colour, so the border is what
  // shows where the box is.
  it('outlines a disabled ticked box as it does a disabled empty one', async () => {
    const empty = await render(Checkbox, {
      props: { disabled: true, 'aria-label': label },
    });
    const outline = getComputedStyle(
      page.getByRole('checkbox').element(),
    ).borderTopColor;
    await empty.unmount();

    await render(Checkbox, {
      props: { checked: true, disabled: true, 'aria-label': label },
    });
    expect(outline).not.toBe('rgba(0, 0, 0, 0)');
    expect(
      getComputedStyle(page.getByRole('checkbox').element()).borderTopColor,
    ).toBe(outline);
  });

  it('draws the tick only once the box is ticked', async () => {
    await render(Checkbox, { props: { 'aria-label': label } });

    const mark = () =>
      document.querySelector('[data-slot="checkbox-indicator"]');
    expect(mark()).toBeNull();

    await userEvent.tab();
    await userEvent.keyboard(' ');
    await expect.poll(mark).not.toBeNull();
  });

  // A header box over rows of which only some are ticked must not look like
  // an empty one, or the reader thinks nothing is ticked.
  it('draws a dash and the live colour for a mixed box', async () => {
    await render(Checkbox, {
      props: { indeterminate: true, 'aria-label': label },
    });

    const box = page.getByRole('checkbox').element();
    expect(box.getAttribute('aria-checked')).toBe('mixed');
    expect(
      box.querySelector('[data-slot="checkbox-indicator"]'),
    ).not.toBeNull();
    expect(getComputedStyle(box).backgroundColor).not.toBe(
      getComputedStyle(document.body).backgroundColor,
    );
  });

  it('has nothing axe objects to', async () => {
    const { container } = await render(Checkbox, {
      props: { checked: true, 'aria-label': label },
    });

    await expectNoViolations(container);
  });
});
