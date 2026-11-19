import { expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import SettingsAccountScreen from './settings-account-screen.svelte';

const note = 'What this account may do. Only an administrator can change it.';
const mount = () =>
  render(SettingsAccountScreen, {
    props: {
      source: {
        status: 'signed-in',
        principal: { id: 'user:alice', grants: [] },
      },
      identity: { username: 'alice', role: 'admin' },
      onChangePassword: () => undefined,
    },
  });

it('reveals role help on keyboard focus and closes it with Escape', async () => {
  await mount();
  await expect.element(page.getByText(note)).not.toBeVisible();
  await userEvent.tab();
  await userEvent.tab();
  const trigger = page.getByRole('button', { name: 'Role', exact: true });
  await expect.element(trigger).toHaveFocus();
  await expect.element(page.getByRole('tooltip')).toHaveTextContent(note);
  const tooltip = page.getByRole('tooltip').element();
  expect(trigger.element().getAttribute('aria-describedby')).toBe(tooltip.id);
  await userEvent.keyboard('{Escape}');
  await expect.element(page.getByRole('tooltip')).not.toBeInTheDocument();
  await expect.element(trigger).toHaveFocus();
});

it('opens role help when the icon is clicked', async () => {
  await mount();
  await userEvent.click(
    page.getByRole('button', { name: 'Role', exact: true }),
  );
  await expect.element(page.getByRole('tooltip')).toHaveTextContent(note);
});
