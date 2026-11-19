// The bar carries no destinations of its own. Everything the rail holds is
// behind one control, so the test opens it and looks for the last item.
import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import type { NavItem } from '../../slots';
import type { RailNavItem } from '../rail';
import PhoneBar from './phone-bar.svelte';

const ITEMS: NavItem[] = [
  { id: 'printers', label: 'Printers', href: '/printers' },
  { id: 'jobs', label: 'Jobs', href: '/jobs' },
];
const SETTINGS: RailNavItem = {
  id: 'settings',
  label: 'Settings',
  href: '/settings/account',
  children: [
    { id: 'account', label: 'Account', href: '/settings/account' },
    { id: 'server', label: 'Server', href: '/settings/server' },
  ],
};

describe('the phone bar', () => {
  it('draws no destination until the menu is opened', async () => {
    await render(PhoneBar, {
      productName: 'Prusa Connect',
      items: ITEMS,
      settings: SETTINGS,
      currentPath: '/printers',
      connection: 'online',
    });

    expect(page.getByRole('link').all()).toHaveLength(0);
  });

  it('opens a menu holding every destination', async () => {
    await render(PhoneBar, {
      productName: 'Prusa Connect',
      items: ITEMS,
      settings: SETTINGS,
      currentPath: '/printers',
      connection: 'online',
    });

    await userEvent.click(page.getByRole('button', { name: /menu/i }));
    await userEvent.click(page.getByRole('button', { name: 'Settings' }));

    await expect
      .element(page.getByRole('link', { name: 'Server' }))
      .toBeInTheDocument();
  });

  it('closes the menu when a row is followed', async () => {
    await render(PhoneBar, {
      productName: 'Prusa Connect',
      items: ITEMS,
      settings: SETTINGS,
      currentPath: '/printers',
      connection: 'online',
    });

    await userEvent.click(page.getByRole('button', { name: /menu/i }));
    const row = page.getByRole('link', { name: 'Jobs' });
    row.element().addEventListener('click', (event) => {
      event.preventDefault();
    });
    await userEvent.click(row);

    await expect.element(row).not.toBeInTheDocument();
  });

  it('lists each destination once, in the rail order', async () => {
    await render(PhoneBar, {
      productName: 'Prusa Connect',
      items: ITEMS,
      settings: SETTINGS,
      currentPath: '/printers',
      connection: 'online',
    });

    await userEvent.click(page.getByRole('button', { name: /menu/i }));
    await userEvent.click(page.getByRole('button', { name: 'Settings' }));

    const labels = page
      .getByRole('link')
      .all()
      .map((link) => link.element().textContent.trim());
    expect(labels).toEqual([
      ...ITEMS.map((item) => item.label),
      'Account',
      'Server',
    ]);
  });

  it('marks Server current on a child route', async () => {
    await render(PhoneBar, {
      productName: 'Prusa Connect',
      items: ITEMS,
      settings: SETTINGS,
      currentPath: '/settings/server/backups',
      connection: 'online',
    });

    await userEvent.click(page.getByRole('button', { name: /menu/i }));
    await userEvent.click(page.getByRole('button', { name: 'Settings' }));

    await expect
      .element(page.getByRole('link', { name: 'Server' }))
      .toHaveAttribute('aria-current', 'page');
  });

  it('lists the children of an item after it, as the rail does', async () => {
    await render(PhoneBar, {
      productName: 'Prusa Connect',
      items: [
        {
          id: 'printers',
          label: 'Printers',
          href: '/printers',
          children: [{ id: 'fleet', label: 'Fleet', href: '/printers/fleet' }],
        },
      ],
      currentPath: '/printers/fleet',
      connection: 'online',
    });

    await userEvent.click(page.getByRole('button', { name: /menu/i }));
    await userEvent.click(page.getByRole('button', { name: 'Printers' }));

    await expect
      .element(page.getByRole('link', { name: 'Fleet' }))
      .toHaveAttribute('aria-current', 'page');
    await expect
      .element(page.getByRole('link', { name: 'Printers' }))
      .not.toHaveAttribute('aria-current');
  });
});
