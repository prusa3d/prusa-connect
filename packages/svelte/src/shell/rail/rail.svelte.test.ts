// The rail draws the application's own items first and a heading over each
// group, marks the item the address is under, and keeps a label reachable
// when it is collapsed.
import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { NavGroup, NavItem } from '../../slots';
import Rail from './rail.svelte';
import {
  isCurrent,
  isCurrentRow,
  type RailNavItem,
  sectionEntries,
} from './types';

const ITEMS: NavItem[] = [
  { id: 'printers', label: 'Printers', href: '/printers' },
  { id: 'jobs', label: 'Jobs', href: '/jobs' },
  { id: 'spools', label: 'Spools', href: '/m/spools', group: 'extensions' },
];

const GROUPS: NavGroup[] = [{ id: 'extensions', label: 'Extensions' }];

const SETTINGS: RailNavItem = {
  id: 'settings',
  label: 'Settings',
  href: '/settings',
};

const NESTED_ITEMS: RailNavItem[] = [
  {
    id: 'printers',
    label: 'Printers',
    href: '/printers',
    children: [{ id: 'fleet', label: 'Fleet', href: '/printers/fleet' }],
  },
  { id: 'jobs', label: 'Jobs', href: '/jobs' },
];

describe('the rail', () => {
  it('draws the ungrouped items before the group heading', async () => {
    await render(Rail, {
      items: ITEMS,
      groups: GROUPS,
      currentPath: '/printers',
    });

    const links = page.getByRole('link').all();
    const names = links.map((link) => link.element().textContent);

    expect(names.map((name) => name.trim())).toEqual([
      'Printers',
      'Jobs',
      'Spools',
    ]);
    await expect.element(page.getByText('Extensions')).toBeInTheDocument();
  });

  it('marks the item the address is under', async () => {
    await render(Rail, { items: ITEMS, currentPath: '/jobs/42' });

    await expect
      .element(page.getByRole('link', { name: 'Jobs' }))
      .toHaveAttribute('aria-current', 'page');
    await expect
      .element(page.getByRole('link', { name: 'Printers' }))
      .not.toHaveAttribute('aria-current');
  });

  it('keeps the label reachable when it is collapsed', async () => {
    await render(Rail, {
      items: ITEMS,
      currentPath: '/printers',
      collapsed: true,
    });

    await expect
      .element(page.getByRole('link', { name: 'Printers' }))
      .toBeInTheDocument();
  });
});

describe('the rule above Settings', () => {
  it('draws Settings after the rule, never above it', async () => {
    await render(Rail, {
      items: ITEMS,
      groups: GROUPS,
      settings: SETTINGS,
      currentPath: '/printers',
    });

    const links = page.getByRole('link').all();
    const names = links.map((link) => link.element().textContent);

    expect(names.map((name) => name.trim())).toEqual([
      'Printers',
      'Jobs',
      'Spools',
      'Settings',
    ]);
    await expect.element(page.getByRole('separator')).toBeInTheDocument();
  });

  it('draws neither the rule nor a Settings row when there is none', async () => {
    await render(Rail, { items: ITEMS, currentPath: '/printers' });

    expect(page.getByRole('separator').all()).toHaveLength(0);
  });
});

describe('an item with children', () => {
  it('keeps an item that has none a link', async () => {
    await render(Rail, { items: ITEMS, currentPath: '/printers' });

    expect(page.getByRole('button', { name: 'Printers' }).all()).toHaveLength(
      0,
    );
    await expect
      .element(page.getByRole('link', { name: 'Printers' }))
      .toHaveAttribute('href', '/printers');
  });

  it('reveals its children behind a toggle that never navigates', async () => {
    await render(Rail, { items: NESTED_ITEMS, currentPath: '/jobs' });

    const toggle = page.getByRole('button', { name: 'Printers' });
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(page.getByRole('link', { name: 'Fleet' }).all()).toHaveLength(0);

    await toggle.click();

    await expect.element(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect
      .element(page.getByRole('link', { name: 'Fleet' }))
      .toBeInTheDocument();
  });

  it('opens itself around the section the address is in', async () => {
    await render(Rail, {
      items: NESTED_ITEMS,
      currentPath: '/printers/fleet',
    });

    await expect
      .element(page.getByRole('link', { name: 'Fleet' }))
      .toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Printers' }))
      .toHaveAttribute('aria-expanded', 'true');
  });
});

describe('an item whose children leave out its own page', () => {
  it('lists the item itself first among its sections', async () => {
    await render(Rail, { items: NESTED_ITEMS, currentPath: '/jobs' });

    await page.getByRole('button', { name: 'Printers' }).click();

    const links = page
      .getByRole('link')
      .all()
      .map((link) => link.element().getAttribute('href'));
    expect(links).toEqual(['/printers', '/printers/fleet', '/jobs']);
  });
});

describe('an item with children in a collapsed rail', () => {
  const SETTINGS_SECTIONS: RailNavItem = {
    ...SETTINGS,
    href: '/settings/account',
    children: [
      { id: 'account', label: 'Account', href: '/settings/account' },
      { id: 'server', label: 'Server', href: '/settings/server' },
    ],
  };

  it('opens a menu linking the item and each child', async () => {
    await render(Rail, {
      items: NESTED_ITEMS,
      currentPath: '/printers/fleet',
      collapsed: true,
    });

    const trigger = page.getByRole('button', { name: 'Printers' });
    await expect.element(trigger).toHaveAttribute('aria-haspopup', 'menu');
    await trigger.click();

    const menu = page.getByRole('menu');
    await expect
      .element(menu.getByRole('menuitem', { name: 'Printers' }))
      .toHaveAttribute('href', '/printers');
    await expect
      .element(menu.getByRole('menuitem', { name: 'Fleet' }))
      .toHaveAttribute('href', '/printers/fleet');
    await expect
      .element(menu.getByRole('menuitem', { name: 'Fleet' }))
      .toHaveAttribute('aria-current', 'page');
    await expect
      .element(menu.getByRole('menuitem', { name: 'Printers' }))
      .not.toHaveAttribute('aria-current');
  });

  it("lists only the children when one has the item's own address", async () => {
    await render(Rail, {
      items: ITEMS,
      settings: SETTINGS_SECTIONS,
      currentPath: '/printers',
      collapsed: true,
    });

    await page.getByRole('button', { name: 'Settings' }).click();

    const names = page
      .getByRole('menuitem')
      .all()
      .map((entry) => entry.element().textContent.trim());
    expect(names).toEqual(['Account', 'Server']);
  });

  it('opens with Enter and closes back to its row with Escape', async () => {
    await render(Rail, {
      items: ITEMS,
      settings: SETTINGS_SECTIONS,
      currentPath: '/printers',
      collapsed: true,
    });

    const trigger = page.getByRole('button', { name: 'Settings' });
    trigger.element().focus();
    await userEvent.keyboard('{Enter}');
    await expect.element(page.getByRole('menu')).toBeInTheDocument();
    await expect.element(trigger).toHaveAttribute('aria-expanded', 'true');

    await userEvent.keyboard('{Escape}');
    await expect.element(page.getByRole('menu')).not.toBeInTheDocument();
    await expect.element(trigger).toHaveFocus();
  });

  it('moves between the children with the arrow keys', async () => {
    await render(Rail, {
      items: ITEMS,
      settings: SETTINGS_SECTIONS,
      currentPath: '/printers',
      collapsed: true,
    });

    await page.getByRole('button', { name: 'Settings' }).click();
    await expect.element(page.getByRole('menu')).toBeInTheDocument();

    await userEvent.keyboard('{ArrowDown}');
    await expect
      .element(page.getByRole('menuitem', { name: 'Account' }))
      .toHaveFocus();
    await userEvent.keyboard('{ArrowDown}');
    await expect
      .element(page.getByRole('menuitem', { name: 'Server' }))
      .toHaveFocus();
  });

  it('closes when the address changes', async () => {
    const screen = await render(Rail, {
      items: ITEMS,
      settings: SETTINGS_SECTIONS,
      currentPath: '/printers',
      collapsed: true,
    });

    await page.getByRole('button', { name: 'Settings' }).click();
    await expect.element(page.getByRole('menu')).toBeInTheDocument();

    await screen.rerender({ currentPath: '/settings/server' });
    await expect.element(page.getByRole('menu')).not.toBeInTheDocument();
  });

  it('keeps the selected look on the current entry when it is focused', async () => {
    await render(Rail, {
      items: ITEMS,
      settings: SETTINGS_SECTIONS,
      currentPath: '/settings/server',
      collapsed: true,
    });

    await page.getByRole('button', { name: 'Settings' }).click();

    const server = page.getByRole('menuitem', { name: 'Server' });
    await expect.element(server).toBeInTheDocument();
    const item = server.element();
    item.focus();
    const probe = document.createElement('div');
    probe.className = 'bg-selected-hovered text-selected-foreground';
    document.body.append(probe);

    const focused = getComputedStyle(item);
    const wanted = getComputedStyle(probe);
    expect(focused.backgroundColor).toBe(wanted.backgroundColor);
    expect(focused.color).toBe(wanted.color);
    probe.remove();
  });

  it('keeps an item without children a plain link', async () => {
    await render(Rail, {
      items: NESTED_ITEMS,
      currentPath: '/jobs',
      collapsed: true,
    });

    await expect
      .element(page.getByRole('link', { name: 'Jobs' }))
      .toHaveAttribute('href', '/jobs');
    expect(page.getByRole('button', { name: 'Jobs' }).all()).toHaveLength(0);
  });
});

describe('the places an item leads to', () => {
  it('is the item alone when it has no children', () => {
    expect(sectionEntries(SETTINGS)).toEqual([SETTINGS]);
  });
});

describe('which item the address is under', () => {
  it('matches the item itself', () => {
    expect(isCurrent('/jobs', '/jobs')).toBe(true);
  });

  it('matches a path below the item', () => {
    expect(isCurrent('/jobs', '/jobs/42')).toBe(true);
  });

  it('does not match a sibling that starts with the same letters', () => {
    expect(isCurrent('/jobs', '/jobsearch')).toBe(false);
  });

  it('does not let the root match everything', () => {
    expect(isCurrent('/', '/jobs')).toBe(false);
  });
});

describe('which row is marked', () => {
  const SETTINGS_WITH_ROOMS: RailNavItem = {
    ...SETTINGS,
    children: [{ id: 'account', label: 'Account', href: '/settings/account' }],
  };

  it('leaves the folder unmarked when a room under it is the address', () => {
    expect(isCurrentRow(SETTINGS_WITH_ROOMS, '/settings/account')).toBe(false);
  });

  it('marks the room itself', () => {
    expect(
      isCurrentRow(
        { id: 'account', label: 'Account', href: '/settings/account' },
        '/settings/account',
      ),
    ).toBe(true);
  });

  it('marks the folder when the address is the folder', () => {
    expect(isCurrentRow(SETTINGS_WITH_ROOMS, '/settings')).toBe(true);
  });
});
