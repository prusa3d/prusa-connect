import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { NewAccount } from '../../data';
import { expectNoViolations } from '../../test/axe';
import { fakeListingStore } from '../../test/fakes';
import {
  denied,
  fakePeopleSource,
  oneAdmin,
  people,
  SUBJECT,
} from './people.fixtures';
import SettingsPeopleScreen from './settings-people-screen.svelte';

const store = fakeListingStore();

const SELF = 'operator@printfarm.example';
const OTHER = 'jana@printfarm.example';

/** The row menu, which bits-ui portals to the end of the document. */
const menu = (): HTMLElement | null =>
  document.body.querySelector<HTMLElement>(
    '[data-slot="dropdown-menu-content"]',
  );

const openMenu = async (name: string): Promise<HTMLElement> => {
  await page.getByRole('button', { name: `Actions for ${name}` }).click();
  await expect.poll(menu).not.toBeNull();
  const opened = menu();
  if (opened === null) throw new Error('no menu opened');

  return opened;
};

describe('SettingsPeopleScreen', () => {
  it('names itself and offers the one act', async () => {
    await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource(people), subject: SUBJECT, store },
    });

    // The `Settings` heading is the chrome's.
    await expect
      .element(page.getByRole('heading', { level: 2 }))
      .toHaveTextContent('People');
    await expect
      .element(page.getByRole('button', { name: 'Add person' }))
      .toBeVisible();
    await expect
      .element(page.getByRole('combobox', { name: `Change role for ${OTHER}` }))
      .toBeVisible();
  });

  it('draws a skeleton in the listing’s own shape before the first answer', async () => {
    await render(SettingsPeopleScreen, {
      props: {
        people: fakePeopleSource([], { loading: true, loaded: false }),
        subject: SUBJECT,
        store,
      },
    });

    await expect
      .element(page.getByRole('status', { name: 'Loading people' }))
      .toBeVisible();
    expect(page.getByRole('table').query()).toBe(null);
    expect(page.getByText('No one can sign in yet').query()).toBe(null);
  });

  it('draws the refusal the registry carries rather than a sentence of its own', async () => {
    await render(SettingsPeopleScreen, {
      props: {
        people: fakePeopleSource([], { loaded: false, error: denied }),
        store,
      },
    });

    await expect
      .element(
        page.getByText('The caller may not perform that action on that object'),
      )
      .toBeVisible();
    expect(page.getByText('No one can sign in yet').query()).toBe(null);
    expect(page.getByRole('button', { name: 'Add person' }).query()).toBe(null);
  });

  it('reports a refused write over the list it failed to change', async () => {
    await render(SettingsPeopleScreen, {
      props: {
        people: fakePeopleSource(people, { error: denied }),
        subject: SUBJECT,
        store,
      },
    });

    await expect
      .element(
        page.getByText('The caller may not perform that action on that object'),
      )
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: `Actions for ${OTHER}` }))
      .toBeVisible();
  });

  it('offers the act inside the empty state, and only there', async () => {
    const { container } = await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource([]), store },
    });

    await expect
      .element(page.getByText('No one can sign in yet'))
      .toBeVisible();
    expect(container.querySelector('[data-slot="page-header-actions"]')).toBe(
      null,
    );
    await expect
      .element(page.getByRole('button', { name: 'Add person' }))
      .toBeVisible();
  });

  it('refuses to demote or remove the reader’s own account', async () => {
    await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource(people), subject: SUBJECT, store },
    });

    // A role that cannot change is drawn as a reading.
    expect(
      page.getByRole('combobox', { name: `Change role for ${SELF}` }).query(),
    ).toBeNull();

    // The reading and the control stand at one height, so rows do not differ.
    const height = (el: Element) => el.getBoundingClientRect().height;
    const reading = page
      .getByText('Administrator', { exact: true })
      .elements()
      .find((el) => el.tagName === 'SPAN' && el.checkVisibility());
    const control = page
      .getByRole('combobox', { name: `Change role for ${OTHER}` })
      .elements()
      .find((el) => el.checkVisibility());
    expect(reading).toBeDefined();
    expect(control).toBeDefined();
    expect(height(reading as Element)).toBe(height(control as Element));

    const own = await openMenu(SELF);
    await expect
      .element(
        page.elementLocator(own).getByRole('menuitem', { name: 'Remove' }),
      )
      .toBeDisabled();
    await userEvent.keyboard('{Escape}');
    await expect.poll(menu).toBeNull();

    const other = await openMenu(OTHER);
    await expect
      .element(
        page.elementLocator(other).getByRole('menuitem', { name: 'Remove' }),
      )
      .not.toBeDisabled();
  });

  it('says which row is the last administrator, and locks it', async () => {
    await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource(oneAdmin), subject: SUBJECT, store },
    });

    // Both tiers of the listing stay in the document and the container query.
    const said = page
      .getByText('Only administrator')
      .elements()
      .filter((badge) => badge.checkVisibility());

    expect(said).toHaveLength(1);
    expect(
      page.getByRole('combobox', { name: `Change role for ${OTHER}` }).query(),
    ).toBeNull();
  });

  it('asks before it removes somebody, and asks with their name', async () => {
    const removed: string[] = [];
    const remove = (account: string): Promise<void> => {
      removed.push(account);

      return Promise.resolve();
    };
    await render(SettingsPeopleScreen, {
      props: {
        people: fakePeopleSource(people, { remove }),
        subject: SUBJECT,
        store,
      },
    });

    const opened = await openMenu(OTHER);
    await page
      .elementLocator(opened)
      .getByRole('menuitem', { name: 'Remove' })
      .click();

    await expect.element(page.getByText(`Remove ${OTHER}?`)).toBeVisible();
    expect(removed).toEqual([]);

    await page.getByRole('button', { name: 'Remove', exact: true }).click();
    await expect.poll(() => removed).toEqual(['jana']);
  });

  it('refuses a username the server already holds, and sends a good one', async () => {
    const created: NewAccount[] = [];
    const create = (account: NewAccount): Promise<undefined> => {
      created.push(account);

      return Promise.resolve(undefined);
    };
    await render(SettingsPeopleScreen, {
      props: {
        people: fakePeopleSource(people, { create }),
        subject: SUBJECT,
        store,
      },
    });

    await page.getByRole('button', { name: 'Add person' }).click();
    const username = page.getByRole('textbox', { name: 'Username' });
    await username.fill(OTHER);

    await expect
      .element(page.getByText('That username already has an account.'))
      .toBeVisible();

    await username.fill('newcomer@printfarm.example');
    await page.getByLabelText('Password').fill('a-long-enough-one');
    await page.getByRole('button', { name: 'Add person' }).last().click();

    await expect
      .poll(() => created.map((account) => account.username))
      .toEqual(['newcomer@printfarm.example']);
  });

  it('says why a short password keeps Add person disabled', async () => {
    await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource(people), subject: SUBJECT, store },
    });

    await page.getByRole('button', { name: 'Add person' }).click();
    await page.getByRole('textbox', { name: 'Username' }).fill('new@x.example');
    await page.getByLabelText('Password').fill('abc');

    await expect
      .element(page.getByText('A password is at least 8 characters.'))
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Add person' }).last())
      .toBeDisabled();
  });

  // Every role on this server is granted over the whole organisation.
  it('says a role reaches every printer', async () => {
    await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource(people), subject: SUBJECT, store },
    });

    await page.getByRole('button', { name: 'Add person' }).click();

    await expect
      .element(
        page.getByText(
          'A role is held over the whole server. Whoever holds one reaches every printer on it.',
        ),
      )
      .toBeVisible();
  });

  it('gives the table its columns where the container has the room', async () => {
    // The default page is 414px wide, which is the stacked tier.
    await page.viewport(1280, 720);
    const { container } = await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource(people), subject: SUBJECT, store },
    });

    const table = container.querySelector('table');
    if (table === null) throw new Error('no table drawn');
    expect(table.checkVisibility()).toBe(true);

    await expect
      .element(page.getByRole('columnheader', { name: /^Username/ }))
      .toBeVisible();
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource(people), subject: SUBJECT, store },
    });

    await expectNoViolations(container);
  });

  it('opens a person from their username and from the row menu', async () => {
    await render(SettingsPeopleScreen, {
      props: {
        people: fakePeopleSource(people),
        subject: SUBJECT,
        store,
        personHref: (id: string) => `/settings/people/${id}`,
      },
    });

    await expect
      .element(page.getByRole('link', { name: OTHER }))
      .toHaveAttribute('href', '/settings/people/jana');

    const opened = await openMenu(OTHER);
    expect(
      opened.querySelector('a[href="/settings/people/jana"]')?.textContent,
    ).toContain('Printer access');
  });

  it('narrows the people by the search, and says when nothing matches', async () => {
    await render(SettingsPeopleScreen, {
      props: { people: fakePeopleSource(people), subject: SUBJECT, store },
    });

    const search = page.getByRole('searchbox', { name: 'Search people' });
    await search.fill('intern');
    expect(page.getByText(OTHER).query()).toBe(null);

    await search.fill('nobody');
    await expect
      .element(page.getByRole('heading', { name: 'Nothing matches “nobody”' }))
      .toBeVisible();
  });
});
