import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import Toggle from './toggle.svelte';

const label = (text: string) =>
  createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

describe('Toggle', () => {
  it('flips aria-pressed on Space, on Enter and on a click', async () => {
    const onPressedChange = vi.fn();
    await render(Toggle, {
      props: { onPressedChange, children: label('Bold') },
    });

    const toggle = page.getByRole('button', { name: 'Bold' });
    await expect.element(toggle).toHaveAttribute('aria-pressed', 'false');
    await expect.element(toggle).toHaveAttribute('data-state', 'off');

    await userEvent.tab();
    await expect.element(toggle).toHaveFocus();

    await userEvent.keyboard(' ');
    await expect.element(toggle).toHaveAttribute('aria-pressed', 'true');
    await expect.element(toggle).toHaveAttribute('data-state', 'on');

    await userEvent.keyboard('{Enter}');
    await expect.element(toggle).toHaveAttribute('aria-pressed', 'false');

    await toggle.click();
    await expect.element(toggle).toHaveAttribute('aria-pressed', 'true');
    expect(onPressedChange).toHaveBeenCalledTimes(3);
  });

  it('starts pressed when it is given a starting state', async () => {
    await render(Toggle, { props: { pressed: true, children: label('Bold') } });

    await expect
      .element(page.getByRole('button', { name: 'Bold' }))
      .toHaveAttribute('aria-pressed', 'true');
  });

  it('blocks the callback while disabled', async () => {
    const onPressedChange = vi.fn();
    await render(Toggle, {
      props: { onPressedChange, disabled: true, children: label('Bold') },
    });

    const toggle = page.getByRole('button', { name: 'Bold' });
    await expect.element(toggle).toBeDisabled();
    await toggle.click({ force: true });
    expect(onPressedChange).not.toHaveBeenCalled();
    await expect.element(toggle).toHaveAttribute('aria-pressed', 'false');
  });

  // The compound `disabled:state-on:*` selectors. A plain
  // `disabled:bg-disabled` ties with `state-on:bg-selected` on
  // specificity and loses, and a dead control keeps the brand fill.
  it('drops the pressed fill when a pressed toggle is disabled', async () => {
    const live = await render(Toggle, {
      props: { pressed: true, children: label('Bold') },
    });
    const pressedFill = getComputedStyle(
      page.getByRole('button', { name: 'Bold' }).element(),
    ).backgroundColor;
    await live.unmount();

    await render(Toggle, {
      props: { pressed: true, disabled: true, children: label('Bold') },
    });
    const deadFill = getComputedStyle(
      page.getByRole('button', { name: 'Bold' }).element(),
    ).backgroundColor;

    expect(deadFill).not.toBe(pressedFill);
  });

  it('renders each variant with its own edge', async () => {
    const edges = new Set<string>();

    for (const variant of ['default', 'outline'] as const) {
      const { unmount } = await render(Toggle, {
        props: { variant, children: label(variant) },
      });
      const toggle = page.getByRole('button', { name: variant });
      edges.add(getComputedStyle(toggle.element()).borderTopColor);
      await unmount();
    }

    // `border-transparent` against `border-border`. One recipe falling back to
    // the other collapses this set.
    expect(edges.size).toBe(2);
  });

  it('grows with its size', async () => {
    const heights: number[] = [];

    for (const size of ['sm', 'md', 'lg'] as const) {
      const { unmount } = await render(Toggle, {
        props: { size, children: label(size) },
      });
      const toggle = page.getByRole('button', { name: size });
      heights.push(toggle.element().getBoundingClientRect().height);
      await unmount();
    }

    expect(heights[0]).toBeLessThan(heights[1]!);
    expect(heights[1]).toBeLessThan(heights[2]!);
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(Toggle, {
      props: { pressed: true, children: label('Bold') },
    });

    await expectNoViolations(container);
  });
});
