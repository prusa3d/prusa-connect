import { createRawSnippet } from 'svelte';
import { expect, test, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import ServerSettingsShell from './server-settings-shell.svelte';

const children = createRawSnippet(() => ({
  render: () => '<div data-testid="content"></div>',
}));

test('marks the open tab, with Overview first, and opens the address of a pressed one', async () => {
  const onNavigate = vi.fn();
  await render(ServerSettingsShell, {
    props: {
      active: 'backups',
      overviewHref: '/settings/server/overview',
      healthHref: '/settings/server/health',
      updatesHref: '/settings/server/updates',
      backupsHref: '/settings/server/backups',
      networkHref: '/settings/server/network',
      onNavigate,
      children,
    },
  });

  await expect
    .element(page.getByRole('tab', { name: 'Backups' }))
    .toHaveAttribute('aria-selected', 'true');
  await expect
    .element(page.getByRole('tab').first())
    .toHaveAccessibleName('Overview');
  await expect.element(page.getByTestId('content')).toBeInTheDocument();

  await page.getByRole('tab', { name: 'Network' }).click();
  expect(onNavigate).toHaveBeenCalledWith('/settings/server/network');
});

test('draws no Updates and no Backups tab where the server prepares no upgrade', async () => {
  await render(ServerSettingsShell, {
    props: {
      active: 'overview',
      overviewHref: '/settings/server/overview',
      healthHref: '/settings/server/health',
      networkHref: '/settings/server/network',
      onNavigate: () => undefined,
      children,
    },
  });

  await expect
    .element(page.getByRole('tab', { name: 'Network' }))
    .toBeVisible();
  expect(page.getByRole('tab', { name: 'Updates' }).elements()).toEqual([]);
  expect(page.getByRole('tab', { name: 'Backups' }).elements()).toEqual([]);
});
