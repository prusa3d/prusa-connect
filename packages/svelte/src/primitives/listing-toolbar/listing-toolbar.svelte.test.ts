import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import ListingToolbarProbe from '../../test/listing-toolbar-probe.svelte';

/** The panel, which bits-ui portals to the end of the document. */
const panel = (): HTMLElement | null =>
  document.body.querySelector<HTMLElement>('[data-slot="popover-content"]');

/** On a toolbar narrower than its container breakpoint, which the test page
 * is, the facets sit under the Filters disclosure. */
const showFilters = async (): Promise<void> => {
  const filters = page.getByRole('button', { name: /^Filters/ });
  if (filters.element().getAttribute('aria-expanded') !== 'true') {
    await filters.click();
  }
};

const openFacet = async (name: string): Promise<HTMLElement> => {
  await showFilters();
  await page.getByRole('button', { name: new RegExp(name) }).click();
  await expect.poll(panel).not.toBeNull();
  const surface = panel();
  if (surface === null) throw new Error('no panel opened');
  return surface;
};

describe('ListingToolbar', () => {
  it('draws a trigger for each facet the listing declares', async () => {
    await render(ListingToolbarProbe, {});
    await showFilters();

    await expect
      .element(page.getByRole('button', { name: /State/ }))
      .toBeVisible();
  });

  it('reports a chosen value back to the listing', async () => {
    await render(ListingToolbarProbe, {});

    const surface = await openFacet('State');
    await page
      .elementLocator(surface)
      .getByRole('button', { name: /Printing/ })
      .click();

    await expect
      .element(page.getByTestId('picked'))
      .toHaveTextContent('PRINTING');
  });

  it('names the picked value on the trigger', async () => {
    await render(ListingToolbarProbe, {});

    const surface = await openFacet('State');
    await page
      .elementLocator(surface)
      .getByRole('button', { name: /Idle/ })
      .click();

    await expect
      .element(page.getByRole('button', { name: /State: Idle/ }))
      .toBeVisible();
  });

  it('keeps the field picker and the view toggle at the trailing end', async () => {
    await render(ListingToolbarProbe, {});

    const controls = document.querySelector('[data-slot="listing-controls"]');
    const toolbar = document.querySelector('[data-slot="listing-toolbar"]');

    expect(toolbar?.lastElementChild?.lastElementChild).toBe(controls);
  });

  it('wraps the filters under the first line and keeps the view switch on it', async () => {
    const { container } = await render(ListingToolbarProbe, {});
    container.style.width = '14rem';
    await showFilters();

    const toolbar = document.querySelector('[data-slot="listing-toolbar"]');
    const controls = document.querySelector('[data-slot="listing-controls"]');
    const flag = page.getByRole('button', { name: 'Flagged' }).element();
    if (!toolbar || !controls) throw new Error('toolbar not drawn');

    const top = (node: Element): number => node.getBoundingClientRect().top;
    expect(top(controls)).toBe(top(toolbar));
    expect(top(flag)).toBeGreaterThan(top(controls));
    expect(toolbar.scrollWidth).toBeLessThanOrEqual(toolbar.clientWidth);
  });

  it('wraps the actions before the search runs under them', async () => {
    const { container } = await render(ListingToolbarProbe, {
      withSearch: true,
      withActions: true,
    });
    container.style.width = '24rem';

    const search = page.getByRole('searchbox', { name: 'Search' }).element();
    const actions = page.getByTestId('actions').element();
    const a = search.getBoundingClientRect();
    const b = actions.getBoundingClientRect();
    expect(b.width).toBeGreaterThan(150);
    const apart =
      a.right <= b.left ||
      b.right <= a.left ||
      a.bottom <= b.top ||
      b.bottom <= a.top;
    expect(apart).toBe(true);
  });

  it('offers Clear filters only while a filter is on', async () => {
    await render(ListingToolbarProbe, {});
    const clear = page.getByRole('button', { name: 'Clear filters' });

    await expect.element(clear).not.toBeInTheDocument();
    const surface = await openFacet('State');
    await page
      .elementLocator(surface)
      .getByRole('button', { name: /Printing/ })
      .click();

    await expect.element(clear).toBeVisible();
  });

  it('lets every filter go with one press', async () => {
    await render(ListingToolbarProbe, {});
    const surface = await openFacet('State');
    await page
      .elementLocator(surface)
      .getByRole('button', { name: /Printing/ })
      .click();
    await page.getByRole('button', { name: 'Flagged' }).click();
    await expect.element(page.getByTestId('flagged')).toHaveTextContent('true');

    await page.getByRole('button', { name: 'Clear filters' }).click();

    await expect.element(page.getByTestId('picked')).toHaveTextContent('');
    await expect
      .element(page.getByTestId('flagged'))
      .toHaveTextContent('false');
  });

  it('hands the reset to the listing when it reads on the server', async () => {
    const onClearFilters = vi.fn();
    const onSelectedChange = vi.fn();
    await render(ListingToolbarProbe, { onClearFilters, onSelectedChange });
    const surface = await openFacet('State');
    await page
      .elementLocator(surface)
      .getByRole('button', { name: /Printing/ })
      .click();
    onSelectedChange.mockClear();
    await userEvent.keyboard('{Escape}');

    await page.getByRole('button', { name: 'Clear filters' }).click();

    expect(onClearFilters).toHaveBeenCalledTimes(1);
    expect(onSelectedChange).not.toHaveBeenCalled();
  });

  it('shows Clear filters when the listing says it is narrowed', async () => {
    await render(ListingToolbarProbe, { filtersActive: true });

    await expect
      .element(page.getByRole('button', { name: 'Clear filters' }))
      .toBeVisible();
  });

  it('draws a single-choice facet with its All row', async () => {
    const onSelectedChange = vi.fn();
    await render(ListingToolbarProbe, { mode: 'single', onSelectedChange });

    const surface = await openFacet('State');
    await page
      .elementLocator(surface)
      .getByRole('button', { name: /Any state/ })
      .click();

    expect(onSelectedChange).toHaveBeenCalledWith([]);
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(ListingToolbarProbe, {});

    await expectNoViolations(container);
  });
});
