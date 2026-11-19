import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import FacetPopoverProbe from '../../test/facet-popover-probe.svelte';
import Input from '../input/input.svelte';
import { facetListSummary } from './summary';
import type { FacetPopoverProps } from './types';

const draw = async (
  props: Partial<Omit<FacetPopoverProps, 'children'>> = {},
): Promise<HTMLButtonElement> => {
  const { container } = await render(FacetPopoverProbe, {
    props: { label: 'State', width: 240, ...props },
  });

  const trigger = container.querySelector<HTMLButtonElement>(
    '[data-slot="facet-popover-trigger"]',
  );
  if (trigger === null) throw new Error('no facet rendered');
  return trigger;
};

/** The height an Input of one size stands at, the scale a facet shares. */
const inputHeight = async (size: 'sm' | 'lg'): Promise<number> => {
  const { container, unmount } = await render(Input, {
    props: { size, 'aria-label': size },
  });
  const height = parseFloat(
    getComputedStyle(container.querySelector('input')!).height,
  );
  await unmount();
  return height;
};

/** The panel, which bits-ui portals to the end of the document. */
const panel = (): HTMLElement | null =>
  document.body.querySelector<HTMLElement>('[data-slot="popover-content"]');

describe('FacetPopover', () => {
  it('names the dimension until a value is picked, then both', async () => {
    const empty = await draw();

    expect(empty.textContent.trim()).toBe('State');
    expect(empty.hasAttribute('data-on')).toBe(false);

    const picked = await draw({ summary: 'Ready' });

    expect(picked.textContent.replace(/\s+/g, ' ').trim()).toBe('State: Ready');
    expect(picked.hasAttribute('data-on')).toBe(true);
    // The fill is what says a facet is filtering, and it is the same fill a
    // toggle wears when it is on.
    expect(getComputedStyle(picked).backgroundColor).not.toBe(
      getComputedStyle(empty).backgroundColor,
    );
  });

  it('counts the values the summary could not list', async () => {
    const trigger = await draw({ summary: 'Ready', count: 4 });

    expect(trigger.textContent.replace(/\s+/g, ' ').trim()).toBe(
      'State: Ready +3',
    );

    // A summary that already counts the rest is left alone.
    const counted = await draw({ summary: 'Ready +2', count: 3 });

    expect(counted.textContent.replace(/\s+/g, ' ').trim()).toBe(
      'State: Ready +2',
    );
  });

  it('carries swatches for a facet words cannot summarise', async () => {
    const trigger = await draw({
      label: 'Colour',
      active: true,
      swatches: ['rgb(245, 166, 35)', 'rgb(27, 27, 31)'],
    });

    const drawn = trigger.querySelectorAll('[style*="background"]');
    expect(drawn).toHaveLength(2);
    expect(getComputedStyle(drawn[0]!).backgroundColor).toBe(
      'rgb(245, 166, 35)',
    );
    // `active` forces the on state that a summary would otherwise decide.
    expect(trigger.hasAttribute('data-on')).toBe(true);
  });

  it('stands at the height of the control scale it shares', async () => {
    const small = parseFloat(
      getComputedStyle(await draw({ size: 'sm' })).height,
    );
    const large = parseFloat(
      getComputedStyle(await draw({ size: 'lg' })).height,
    );

    expect(small).toBe(await inputHeight('sm'));
    expect(large).toBe(await inputHeight('lg'));
    expect(small).toBeLessThanOrEqual(large);
    expect(small).toBeGreaterThanOrEqual(24);
  });

  it('opens a panel of the width the caller asked for', async () => {
    const trigger = await draw({ width: 240 });

    await page.elementLocator(trigger).click();
    await expect.poll(panel).not.toBeNull();

    const surface = panel();
    if (surface === null) throw new Error('no panel opened');
    expect(getComputedStyle(surface).width).toBe('240px');
    expect(surface.getAttribute('aria-label')).toBe('State options');
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    await expectNoViolations(surface);
  });

  it('lets the list inside shut the panel', async () => {
    const trigger = await draw();

    await page.elementLocator(trigger).click();
    await expect.poll(panel).not.toBeNull();

    const row = panel()?.querySelector('button');
    if (row === undefined || row === null) throw new Error('no row rendered');
    row.click();

    await expect.poll(panel).toBeNull();
  });

  it('opens from the keyboard and shuts on Escape', async () => {
    const trigger = await draw();

    await userEvent.tab();
    expect(document.activeElement).toBe(trigger);
    await userEvent.keyboard('{Enter}');
    await expect.poll(panel).not.toBeNull();

    await userEvent.keyboard('{Escape}');
    await expect.poll(panel).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it('has nothing axe objects to', async () => {
    // A picked facet is the hard case: the dimension is told from the value by
    // weight, because dimming it over the primary fill measures 3.04:1.
    const trigger = await draw({ summary: 'Ready', count: 4 });

    await expectNoViolations(trigger);
  });
});

describe('facetListSummary', () => {
  it('says nothing when nothing is picked', () => {
    expect(facetListSummary([])).toBeUndefined();
  });

  it('names one value, and counts the rest past the first', () => {
    expect(facetListSummary(['Ready'])).toBe('Ready');
    expect(facetListSummary(['Ready', 'Printing', 'Attention'])).toBe(
      'Ready +2',
    );
  });
});
