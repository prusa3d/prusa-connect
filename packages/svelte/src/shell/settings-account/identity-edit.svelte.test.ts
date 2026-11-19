import { expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import SettingsAccountScreen from './settings-account-screen.svelte';

const props = {
  source: {
    principal: { id: 'user:alice', grants: [] },
    status: 'signed-in' as const,
  },
  identity: { displayName: 'Alice Doe', username: 'alice', role: 'admin' },
  onChangePassword: () => undefined,
};

it('submits the two identity fields and blocks an empty username', async () => {
  const save = vi.fn();
  await render(SettingsAccountScreen, {
    props: { ...props, onSaveIdentity: save },
  });
  await page.getByRole('button', { name: 'Edit', exact: true }).click();
  await page
    .getByRole('textbox', { name: 'Name', exact: true })
    .fill('Alice Smith');
  await page.getByRole('textbox', { name: 'Username', exact: true }).fill('');
  await expect
    .element(page.getByRole('button', { name: 'Save', exact: true }))
    .toBeDisabled();
  await page
    .getByRole('textbox', { name: 'Username', exact: true })
    .fill('alice.smith');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  expect(save).toHaveBeenCalledWith({
    displayName: 'Alice Smith',
    username: 'alice.smith',
  });
  await expect
    .element(page.getByRole('button', { name: 'Edit', exact: true }))
    .toBeVisible();
});

it('discards drafts on cancel without calling the host', async () => {
  const save = vi.fn();
  await render(SettingsAccountScreen, {
    props: { ...props, onSaveIdentity: save },
  });
  await page.getByRole('button', { name: 'Edit', exact: true }).click();
  await page
    .getByRole('textbox', { name: 'Name', exact: true })
    .fill('Discard me');
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  expect(save).not.toHaveBeenCalled();
  await page.getByRole('button', { name: 'Edit', exact: true }).click();
  await expect
    .element(page.getByRole('textbox', { name: 'Name', exact: true }))
    .toHaveValue('Alice Doe');
});
