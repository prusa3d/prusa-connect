import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { FleetListingQuery } from '../../data';
import { t } from '../../i18n';
import { expectNoViolations } from '../../test/axe';
import {
  fakeDevice,
  fakeListing,
  fakeListingStore,
  searchFilter,
  sortOf,
  stateFilter,
} from '../../test/fakes';
import PrintersScreen from './printers-screen.svelte';
import type { TagFilter } from './types';

const store = fakeListingStore();

// The reader administers every printer, so each card offers the removal.
const devicesOf = (count: number) =>
  Array.from({ length: count }, (_, index) => ({
    ...fakeDevice(`p${String(index)}`, `MK4S #${String(index)}`),
    actions: ['observe', 'operate', 'configure', 'administer'],
  }));

/** A screen over a listing of `count` printers, the whole fleet on one page. */
const propsOf = (
  count: number,
  listing = fakeListing({ items: devicesOf(count) }),
) => ({
  listing,
  printerCount: count,
  onAdd: () => undefined,
  store,
});

/** The `tag` filter the server advertises, spelled as the fleet spells each tag. */
const tagFilterOf = (values: readonly (readonly [string, number])[]) => ({
  param: 'tag' as const,
  kind: 'multi' as const,
  values: values.map(([value, count]) => ({ value, count })),
});

/** A tag filter with no tag chosen, unless the test says otherwise. */
const tagFilter = (over: Partial<TagFilter> = {}): TagFilter => ({
  chosen: [],
  onChange: () => undefined,
  ...over,
});

/** The last query the screen handed the listing. */
const lastAsked = (listing: { asked: Partial<FleetListingQuery>[] }) =>
  listing.asked.at(-1);

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

/** A facet stays open after a pick, so it is shut before the next one opens. */
const pick = async (facet: string, value: string): Promise<void> => {
  await showFilters();
  await page.getByRole('button', { name: new RegExp(facet) }).click();
  await expect.poll(panel).not.toBeNull();
  const surface = panel();
  if (surface === null) throw new Error('no panel opened');

  await page
    .elementLocator(surface)
    .getByRole('button', { name: new RegExp(value) })
    .click();
  await userEvent.keyboard('{Escape}');
  await expect.poll(panel).toBeNull();
};

const sortSurface = (): HTMLElement | null =>
  document.querySelector<HTMLElement>('[data-slot="select-content"]');

describe('PrintersScreen', () => {
  it('names itself and offers the one act', async () => {
    await render(PrintersScreen, { props: propsOf(4) });

    await expect
      .element(page.getByRole('heading', { level: 1 }))
      .toHaveTextContent('Printers');
    await expect
      .element(page.getByRole('button', { name: 'Add printer' }))
      .toBeVisible();
  });

  it('draws the toolbar and the pager once there is a fleet', async () => {
    const { container } = await render(PrintersScreen, { props: propsOf(4) });

    expect(container.querySelector('[data-slot="listing-toolbar"]')).not.toBe(
      null,
    );
    await expect
      .element(page.getByRole('button', { name: 'Next' }))
      .toBeVisible();
  });

  it('drops the machinery for one printer when the reader asked it to', async () => {
    const { container } = await render(PrintersScreen, {
      props: { ...propsOf(1), preference: 'auto' },
    });

    expect(container.querySelector('[data-slot="listing-toolbar"]')).toBe(null);
    expect(page.getByRole('button', { name: 'Next' }).query()).toBe(null);
  });

  it('keeps the machinery for one printer by default', async () => {
    const { container } = await render(PrintersScreen, { props: propsOf(1) });

    expect(container.querySelector('[data-slot="listing-toolbar"]')).not.toBe(
      null,
    );
  });

  it('retires a printer from its card once the reader confirms', async () => {
    const retired: string[] = [];
    await render(PrintersScreen, {
      props: {
        ...propsOf(4),
        store: fakeListingStore(),
        actions: {
          retire: (id: string) => {
            retired.push(id);

            return Promise.resolve();
          },
        },
      },
    });

    await page.getByRole('button', { name: 'Actions for MK4S #0' }).click();
    await page.getByRole('menuitem', { name: 'Remove printer' }).click();
    // Picking the item only asks; nothing is sent before the answer.
    expect(retired).toStrictEqual([]);

    await page
      .getByRole('alertdialog')
      .getByRole('button', { name: 'Remove' })
      .click();
    await expect.poll(() => retired).toStrictEqual(['p0']);
  });

  it('holds the card order while a card menu is open', async () => {
    const devices = devicesOf(3);
    const screen = await render(PrintersScreen, {
      props: { ...propsOf(3), store: fakeListingStore() },
    });
    const order = () =>
      page
        .getByRole('button', { name: /^Actions for/ })
        .elements()
        .map((button) => button.getAttribute('aria-label'));
    const first = [
      'Actions for MK4S #0',
      'Actions for MK4S #1',
      'Actions for MK4S #2',
    ];
    const reversed = [...first].reverse();

    await page.getByRole('button', { name: 'Actions for MK4S #1' }).click();
    await screen.rerender({
      listing: fakeListing({ items: [...devices].reverse() }),
    });
    await expect.poll(order).toStrictEqual(first);

    await userEvent.keyboard('{Escape}');
    await expect.poll(order).toStrictEqual(reversed);
  });

  it('lets go of the order when the card with the open menu leaves', async () => {
    const devices = devicesOf(3);
    const screen = await render(PrintersScreen, {
      props: { ...propsOf(3), store: fakeListingStore() },
    });
    const order = () =>
      page
        .getByRole('button', { name: /^Actions for/ })
        .elements()
        .map((button) => button.getAttribute('aria-label'));

    await page.getByRole('button', { name: 'Actions for MK4S #1' }).click();
    await screen.rerender({
      listing: fakeListing({ items: [devices[2]!, devices[0]!] }),
    });

    await expect
      .poll(order)
      .toStrictEqual(['Actions for MK4S #2', 'Actions for MK4S #0']);
  });

  it('offers the act inside the empty state, and only there', async () => {
    await render(PrintersScreen, { props: propsOf(0) });

    await expect.element(page.getByText('No printers yet')).toBeVisible();
    expect(
      page.getByRole('button', { name: 'Add printer' }).all(),
    ).toHaveLength(1);
    await expect
      .element(page.getByRole('button', { name: 'Add printer' }))
      .toBeVisible();
  });

  // Every printer removed leaves the fleet empty, and the removed ones must
  // stay reachable.
  it('offers the retired printers when the fleet is empty', async () => {
    const listing = fakeListing({ items: [] });
    await render(PrintersScreen, { props: propsOf(0, listing) });

    await page
      .getByRole('button', { name: t('printers.retired.only') })
      .click();

    expect(lastAsked(listing)?.retired).toBe('only');
  });

  // The cards of both lists look alike, so the heading is what says which one
  // is on screen. A printer added from the retired list would land in the
  // other one.
  it('names the retired list in its heading and offers no add there', async () => {
    await render(PrintersScreen, { props: propsOf(4) });

    await page
      .getByRole('button', { name: t('printers.retired.only') })
      .click();

    await expect
      .element(page.getByRole('heading', { name: t('printers.retired.only') }))
      .toBeVisible();
    expect(page.getByRole('button', { name: 'Add printer' }).query()).toBe(
      null,
    );

    await page
      .getByRole('button', { name: t('printers.retired.only') })
      .click();

    await expect
      .element(page.getByRole('heading', { name: t('printers.title') }))
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Add printer' }))
      .toBeVisible();
  });

  it('says the filter matched nothing rather than that the fleet is empty', async () => {
    await render(PrintersScreen, {
      props: propsOf(4, fakeListing({ items: [], total: 0 })),
    });

    await expect
      .element(page.getByText(t('printers.filtered.heading')))
      .toBeVisible();
    expect(page.getByText('No printers yet').query()).toBe(null);
  });

  it('draws skeletons while the first page is still arriving', async () => {
    await render(PrintersScreen, {
      props: propsOf(0, fakeListing({ loading: true, loaded: false })),
    });

    await expect
      .element(page.getByRole('status', { name: 'Loading printers' }))
      .toBeVisible();
  });

  it('says the read failed rather than blaming a filter nobody set', async () => {
    await render(PrintersScreen, {
      props: propsOf(
        0,
        fakeListing({ loaded: false, error: new Error('401') }),
      ),
    });

    await expect
      .element(page.getByText('The fleet could not be read'))
      .toBeVisible();
    expect(page.getByText('No printers match these filters').query()).toBe(
      null,
    );
    expect(page.getByText('No printers yet').query()).toBe(null);
  });

  it('draws only the filters the server advertised', async () => {
    await render(PrintersScreen, {
      props: propsOf(
        4,
        fakeListing({
          items: devicesOf(4),
          filters: [stateFilter(['IDLE', 'ERROR'])],
        }),
      ),
    });
    await showFilters();

    await expect
      .element(
        page.getByRole('button', { name: new RegExp(t('printers.state')) }),
      )
      .toBeVisible();
    expect(
      page
        .getByRole('button', { name: new RegExp(t('printers.type')) })
        .query(),
    ).toBe(null);
    expect(page.getByRole('searchbox').query()).toBe(null);
  });

  it('sends a chosen state, the search and the sort to the listing', async () => {
    const listing = fakeListing({
      items: devicesOf(4),
      filters: [searchFilter(), stateFilter(['IDLE'])],
      sorts: [sortOf('urgency'), sortOf('name'), sortOf('last_seen')],
    });
    await render(PrintersScreen, { props: propsOf(4, listing) });

    await userEvent.type(page.getByRole('searchbox'), 'brno');
    await expect
      .poll(() => lastAsked(listing))
      .toEqual(expect.objectContaining({ search: 'brno', page: 0 }));

    await pick(t('printers.state'), 'Idle');
    expect(lastAsked(listing)?.filters?.state).toEqual(['IDLE']);
    expect(lastAsked(listing)?.search).toBe('brno');

    page.getByLabelText(t('printers.sort.label')).element().focus();
    await userEvent.keyboard('{Enter}');
    await expect.poll(sortSurface).not.toBe(null);
    await page
      .getByRole('option', { name: t('printers.sort.last_seen') })
      .click();

    // A field opens in the direction a reader expects of it.
    await expect
      .poll(() => lastAsked(listing))
      .toEqual(expect.objectContaining({ sort: 'last_seen', order: 'desc' }));

    await page
      .getByRole('button', {
        name: `${t('printers.sort.last_seen')}: ${t('printers.sort.asc')}`,
      })
      .click();
    expect(lastAsked(listing)).toEqual(
      expect.objectContaining({ sort: 'last_seen', order: 'asc' }),
    );
  });

  it('caps the search at the length the server accepts', async () => {
    await render(PrintersScreen, {
      props: propsOf(
        4,
        fakeListing({ items: devicesOf(4), filters: [searchFilter(40)] }),
      ),
    });

    await expect
      .element(page.getByRole('searchbox'))
      .toHaveAttribute('maxlength', '40');
  });

  // The stored sort may name a field a newer server no longer offers.
  it('sends no stored sort the server does not advertise', async () => {
    const remembered = {
      read: () => JSON.stringify({ sort: { field: 'gone', order: 'asc' } }),
      write: () => undefined,
    };
    const listing = fakeListing({
      items: devicesOf(4),
      sorts: [sortOf('urgency'), sortOf('name')],
    });
    await render(PrintersScreen, {
      props: { ...propsOf(4, listing), store: remembered },
    });

    expect(lastAsked(listing)).toEqual(
      expect.objectContaining({ sort: undefined, order: undefined }),
    );
  });

  it('asks the server for the page the reader turns to', async () => {
    const listing = fakeListing({ items: devicesOf(4), total: 120 });
    await render(PrintersScreen, { props: propsOf(120, listing) });

    await page.getByRole('button', { name: 'Next' }).click();

    expect(lastAsked(listing)).toEqual(expect.objectContaining({ page: 1 }));
  });

  it('filters to a severity from the issue strip', async () => {
    const listing = fakeListing({
      items: devicesOf(4),
      issues: {
        affected: 2,
        counts: {},
        by_severity: { error: 2, warning: 0, info: 0 },
      },
    });
    await render(PrintersScreen, { props: propsOf(4, listing) });

    await page
      .getByRole('button', { name: t('printers.issues.error', { count: 2 }) })
      .click();

    expect(lastAsked(listing)?.filters?.severity).toEqual(['error']);
  });

  it('lets go of a severity from the no-match state in the hero shape', async () => {
    const listing = fakeListing({
      items: [],
      total: 0,
      issues: {
        affected: 2,
        counts: {},
        by_severity: { error: 2, warning: 0, info: 0 },
      },
    });
    await render(PrintersScreen, {
      props: { ...propsOf(1, listing), preference: 'my-printer' },
    });
    await page
      .getByRole('button', { name: t('printers.issues.error', { count: 2 }) })
      .click();
    expect(lastAsked(listing)?.filters?.severity).toEqual(['error']);

    await page
      .getByRole('button', { name: t('primitives.listing.clearAll') })
      .click();

    expect(lastAsked(listing)?.filters?.severity ?? []).toEqual([]);
  });

  it('offers every tag the server lists and hands the host the pick', async () => {
    const picked: (readonly string[])[] = [];
    await render(PrintersScreen, {
      props: {
        ...propsOf(
          2,
          fakeListing({
            items: devicesOf(2),
            filters: [
              tagFilterOf([
                ['workshop', 2],
                ['night shift', 1],
              ]),
            ],
          }),
        ),
        tags: tagFilter({
          onChange: (next) => {
            picked.push(next);
          },
        }),
      },
    });

    await pick('With all tags', 'night shift');

    expect(picked).toEqual([['night shift']]);
  });

  it('sends the chosen tags to the listing', async () => {
    const listing = fakeListing({ items: devicesOf(2) });
    await render(PrintersScreen, {
      props: {
        ...propsOf(2, listing),
        tags: tagFilter({ chosen: ['workshop'] }),
      },
    });

    expect(lastAsked(listing)?.filters?.tag).toEqual(['workshop']);
  });

  it('offers no tag filter where the host passes none', async () => {
    await render(PrintersScreen, {
      props: propsOf(
        2,
        fakeListing({
          items: devicesOf(2),
          filters: [stateFilter(['IDLE']), tagFilterOf([['workshop', 2]])],
        }),
      ),
    });
    await showFilters();

    await expect
      .element(page.getByRole('button', { name: /State/ }))
      .toBeVisible();
    expect(page.getByRole('button', { name: /With all tags/ }).query()).toBe(
      null,
    );
  });

  // The hero shape draws no facets, so a chosen tag is let go through the
  // toolbar the screen draws for it, and exactly once.
  it('clears a chosen tag once from the hero shape', async () => {
    const picked: (readonly string[])[] = [];
    await render(PrintersScreen, {
      props: {
        ...propsOf(1),
        preference: 'my-printer',
        tags: tagFilter({
          chosen: ['workshop', 'gone'],
          onChange: (next) => {
            picked.push(next);
          },
        }),
      },
    });

    await page
      .getByRole('button', { name: t('primitives.listing.clearFilters') })
      .click();

    expect(picked).toEqual([[]]);
  });

  it('clears a chosen tag once from the fleet toolbar and from the no-match state', async () => {
    const picked: (readonly string[])[] = [];
    const filters = [tagFilterOf([['workshop', 1]])];
    const props = {
      preference: 'auto' as const,
      tags: tagFilter({
        chosen: ['workshop'],
        onChange: (next: readonly string[]) => {
          picked.push(next);
        },
      }),
    };
    const shown = await render(PrintersScreen, {
      props: {
        ...propsOf(2, fakeListing({ items: devicesOf(2), filters })),
        ...props,
      },
    });
    await page
      .getByRole('button', { name: t('primitives.listing.clearFilters') })
      .click();
    expect(picked).toEqual([[]]);

    picked.length = 0;
    await shown.unmount();
    await render(PrintersScreen, {
      props: {
        ...propsOf(2, fakeListing({ items: [], total: 0, filters })),
        ...props,
      },
    });
    await page
      .getByRole('button', { name: t('primitives.listing.clearAll') })
      .click();
    expect(picked).toEqual([[]]);
  });

  // The address may spell a tag another way than the fleet does. The server
  // takes the two as one tag, so the chip stays chosen.
  it('keeps a tag chosen in another spelling selected', async () => {
    const picked: (readonly string[])[] = [];
    await render(PrintersScreen, {
      props: {
        ...propsOf(
          2,
          fakeListing({
            items: devicesOf(2),
            filters: [tagFilterOf([['night shift', 1]])],
          }),
        ),
        tags: tagFilter({
          chosen: ['NIGHT SHIFT'],
          onChange: (next) => {
            picked.push(next);
          },
        }),
      },
    });

    // Pressing the chosen chip takes it off, so it was drawn as chosen.
    await pick('With all tags', 'night shift');

    expect(picked).toEqual([[]]);
  });

  // The printers nearby are what an empty fleet needs most, so the host's
  // panel is drawn under the empty state as well as under a listing.
  it("draws the host's nearby panel under a listing and under an empty fleet", async () => {
    const nearby = createRawSnippet(() => ({
      render: () => '<p data-testid="nearby">nearby</p>',
    }));

    for (const count of [3, 0]) {
      const { unmount } = await render(PrintersScreen, {
        props: { ...propsOf(count), nearby },
      });

      await expect.element(page.getByTestId('nearby')).toBeVisible();
      await unmount();
    }
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(PrintersScreen, {
      props: propsOf(
        4,
        fakeListing({
          items: devicesOf(4),
          filters: [searchFilter(), stateFilter(['IDLE'])],
          sorts: [sortOf('urgency'), sortOf('name')],
          issues: {
            affected: 1,
            counts: {},
            by_severity: { error: 1, warning: 0, info: 0 },
          },
        }),
      ),
    });

    await expectNoViolations(container);
  });
});
