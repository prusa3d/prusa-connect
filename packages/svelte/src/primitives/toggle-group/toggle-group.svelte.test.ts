import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import ToggleGroupProbe from '../../test/toggle-group-probe.svelte';

/** The items of the rendered group, in the order they are drawn. */
const items = (): HTMLElement[] => [
  ...document.querySelectorAll<HTMLElement>('[data-slot="toggle-group-item"]'),
];

describe('ToggleGroup', () => {
  // A group that picks one option reports its items as radios. The arrow keys
  // move the tab stop between them, and neither the move nor the arrow key
  // changes what is picked.
  it('moves focus with the arrow keys and picks with Space when one option wins', async () => {
    await render(ToggleGroupProbe, {});

    const left = page.getByRole('radio', { name: 'left' });
    const center = page.getByRole('radio', { name: 'center' });

    left.element().focus();
    await expect.element(left).toHaveAttribute('aria-checked', 'true');

    await userEvent.keyboard('{ArrowRight}');
    await expect.element(center).toHaveFocus();
    await expect.element(center).toHaveAttribute('aria-checked', 'false');
    await expect.element(left).toHaveAttribute('aria-checked', 'true');

    await userEvent.keyboard(' ');
    await expect.element(center).toHaveAttribute('aria-checked', 'true');
    await expect.element(center).toHaveAttribute('data-state', 'on');
    await expect.element(left).toHaveAttribute('aria-checked', 'false');
    await expect.element(left).toHaveAttribute('data-state', 'off');
  });

  // Several options at once is a different control: the items are plain
  // buttons that report `aria-pressed`, and one going on leaves the rest be.
  it('toggles each item on its own when several options can win', async () => {
    await render(ToggleGroupProbe, { props: { type: 'multiple' } });

    const left = page.getByRole('button', { name: 'left' });
    const center = page.getByRole('button', { name: 'center' });

    await expect.element(left).toHaveAttribute('aria-pressed', 'true');
    await expect.element(center).toHaveAttribute('aria-pressed', 'false');

    left.element().focus();
    await userEvent.keyboard('{ArrowRight}');
    await expect.element(center).toHaveFocus();

    await userEvent.keyboard(' ');
    await expect.element(center).toHaveAttribute('aria-pressed', 'true');
    await expect.element(center).toHaveAttribute('data-state', 'on');
    await expect.element(left).toHaveAttribute('aria-pressed', 'true');
  });

  it('leaves the selection alone when a disabled item is clicked', async () => {
    await render(ToggleGroupProbe, { props: { disabled: 'center' } });

    const center = page.getByRole('radio', { name: 'center' });
    await expect.element(center).toBeDisabled();

    await center.click({ force: true });
    await expect.element(center).toHaveAttribute('aria-checked', 'false');
    await expect
      .element(page.getByRole('radio', { name: 'left' }))
      .toHaveAttribute('aria-checked', 'true');
  });

  it('keeps the picked item when it is clicked again', async () => {
    await render(ToggleGroupProbe, {});

    const left = page.getByRole('radio', { name: 'left' });
    await left.click();
    await expect.element(left).toHaveAttribute('aria-checked', 'true');
  });

  it('gives every item the variant the group was given', async () => {
    const edges = new Set<string>();

    for (const variant of ['default', 'outline'] as const) {
      const { unmount } = await render(ToggleGroupProbe, {
        props: { variant },
      });
      // The middle item, because the first one is the picked one and the
      // pressed look paints its own edge over the variant's.
      const [, middle] = items();
      expect(middle!.dataset.variant).toBe(variant);
      edges.add(getComputedStyle(middle!).borderTopColor);
      await unmount();
    }

    // `border-transparent` against `border-border`. One recipe falling back to
    // the other collapses this set.
    expect(edges.size).toBe(2);
  });

  // A choice that sets the machine is picked in the primary button's orange,
  // and one that only changes the page keeps the selected blue.
  it('paints the picked item in the primary fill under the primary tone', async () => {
    const probe = document.createElement('div');
    probe.style.backgroundColor = 'var(--color-button-accent)';
    document.body.append(probe);
    const accent = getComputedStyle(probe).backgroundColor;
    probe.remove();

    const fills = new Map<string, string>();
    for (const tone of ['default', 'primary'] as const) {
      const { unmount } = await render(ToggleGroupProbe, {
        props: { tone },
      });
      const [picked] = items();
      expect(picked!.dataset.tone).toBe(tone);
      fills.set(tone, getComputedStyle(picked!).backgroundColor);
      await unmount();
    }

    expect(fills.get('primary')).toBe(accent);
    expect(fills.get('default')).not.toBe(accent);
  });

  // Under `outline` a shared edge would be drawn by both its neighbours, so
  // every item but the first drops its left border.
  it('draws one hairline between two outlined items', async () => {
    await render(ToggleGroupProbe, { props: { variant: 'outline' } });

    const [first, middle] = items();
    expect(getComputedStyle(first!).borderLeftWidth).not.toBe('0px');
    expect(getComputedStyle(middle!).borderLeftWidth).toBe('0px');
  });

  it('gives every item the size the group was given, and grows with it', async () => {
    const heights: number[] = [];
    const paddings = new Set<string>();

    for (const size of ['sm', 'md', 'lg'] as const) {
      const { unmount } = await render(ToggleGroupProbe, { props: { size } });
      const [first] = items();
      expect(first!.dataset.size).toBe(size);
      heights.push(first!.getBoundingClientRect().height);
      paddings.add(getComputedStyle(first!).paddingLeft);
      await unmount();
    }

    expect(heights[0]).toBeLessThan(heights[1]!);
    expect(heights[1]).toBeLessThan(heights[2]!);
    // An item's inline padding is the same at every size: a segmented control
    // whose options are padded differently reads as a row of loose buttons.
    expect(paddings.size).toBe(1);
  });

  // The items sit edge to edge, so only the outer two are rounded.
  it('rounds the group at its ends and nowhere between them', async () => {
    await render(ToggleGroupProbe, {});

    const [first, middle, last] = items();
    const corners = (box: HTMLElement) => getComputedStyle(box);

    expect(corners(first!).borderTopLeftRadius).not.toBe('0px');
    expect(corners(first!).borderTopRightRadius).toBe('0px');
    expect(corners(middle!).borderTopLeftRadius).toBe('0px');
    expect(corners(middle!).borderTopRightRadius).toBe('0px');
    expect(corners(last!).borderTopRightRadius).not.toBe('0px');
  });

  it('has nothing axe refuses, picking one option or several', async () => {
    const single = await render(ToggleGroupProbe, {});
    await expectNoViolations(single.container);
    await single.unmount();

    const many = await render(ToggleGroupProbe, {
      props: { type: 'multiple' },
    });
    await expectNoViolations(many.container);
  });
});
