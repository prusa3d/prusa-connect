import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import SettingsAccountScreen from './settings-account-screen.svelte';
import type { AccountSource } from './types';

const signedIn: AccountSource = {
  principal: { id: 'user:alice', grants: [] },
  status: 'signed-in',
};

const type = async (label: string, text: string): Promise<void> => {
  await userEvent.click(page.getByLabelText(label));
  await userEvent.keyboard(text);
};

const fill = async (
  current: string,
  next: string,
  confirm = next,
): Promise<void> => {
  await type('Current password', current);
  await type('New password', next);
  await type('Confirm new password', confirm);
};

describe('SettingsAccountScreen', () => {
  it('names the room and who this browser is signed in as', async () => {
    await render(SettingsAccountScreen, {
      props: { source: signedIn, onChangePassword: () => undefined },
    });

    await expect
      .element(page.getByRole('heading', { level: 2, name: 'Account' }))
      .toBeVisible();
    await expect.element(page.getByText('user:alice')).toBeVisible();
  });

  // The record is `list-accounts`' own row.
  it('draws the account behind the subject where the server hands one over', async () => {
    const { container } = await render(SettingsAccountScreen, {
      props: {
        source: signedIn,
        identity: {
          username: 'alice',
          displayName: 'Alice Doe',
          role: 'admin',
          createdAt: '2026-08-19T17:05:00Z',
        },
        onChangePassword: () => undefined,
      },
    });

    await expect.element(page.getByText('Alice Doe')).toBeVisible();
    await expect.element(page.getByText('alice')).toBeVisible();
    await expect.element(page.getByText('Administrator')).toBeVisible();
    expect(container.textContent).not.toContain('Not built yet');
  });

  it('draws the subject alone for a reader the listing refuses', async () => {
    const { container } = await render(SettingsAccountScreen, {
      props: { source: signedIn, onChangePassword: () => undefined },
    });

    await expect.element(page.getByText('user:alice')).toBeVisible();
    expect(container.textContent).not.toContain('Username');
  });

  it('draws the subject as a shape rather than as a blank while the read is open', async () => {
    await render(SettingsAccountScreen, {
      props: {
        source: { principal: undefined, status: 'unknown' },
        onChangePassword: () => undefined,
      },
    });

    await expect
      .element(page.getByRole('status', { name: 'Loading the account' }))
      .toBeVisible();
  });

  it('hands the two passwords over exactly as they were typed', async () => {
    const onChangePassword = vi.fn();
    await render(SettingsAccountScreen, {
      props: { source: signedIn, onChangePassword },
    });

    await fill('workshop-floor-9', 'correct-horse-battery');
    await page.getByRole('button', { name: 'Change password' }).click();

    expect(onChangePassword).toHaveBeenCalledWith(
      'workshop-floor-9',
      'correct-horse-battery',
    );
  });

  it('refuses to send two new passwords that differ', async () => {
    const onChangePassword = vi.fn();
    await render(SettingsAccountScreen, {
      props: { source: signedIn, onChangePassword },
    });

    await fill('workshop-floor-9', 'correct-horse', 'correct-hose');

    await expect
      .element(page.getByText('These two passwords do not match.'))
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Change password' }))
      .toBeDisabled();
    expect(onChangePassword).not.toHaveBeenCalled();
  });

  it('refuses a new password shorter than the core accepts', async () => {
    const onChangePassword = vi.fn();
    await render(SettingsAccountScreen, {
      props: { source: signedIn, onChangePassword },
    });

    await fill('workshop-floor-9', 'short');

    await expect
      .element(page.getByText('Use 8 to 72 characters.'))
      .toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Change password' }))
      .toBeDisabled();
    expect(onChangePassword).not.toHaveBeenCalled();
  });

  it('sends on Enter from inside a field', async () => {
    const onChangePassword = vi.fn();
    await render(SettingsAccountScreen, {
      props: { source: signedIn, onChangePassword },
    });

    await type('Current password', 'workshop-floor-9');
    await type('New password', 'correct-horse-battery');
    await type('Confirm new password', 'correct-horse-battery{Enter}');

    expect(onChangePassword).toHaveBeenCalledWith(
      'workshop-floor-9',
      'correct-horse-battery',
    );
  });

  it('holds a second attempt back while the first is in flight', async () => {
    const onChangePassword = vi.fn();
    await render(SettingsAccountScreen, {
      props: { source: signedIn, changing: true, onChangePassword },
    });

    await fill('workshop-floor-9', 'correct-horse-battery');

    await expect
      .element(page.getByRole('button', { name: 'Change password' }))
      .toBeDisabled();
    expect(onChangePassword).not.toHaveBeenCalled();
  });

  it('draws the refusal in the words the core registered for it', async () => {
    await render(SettingsAccountScreen, {
      props: {
        source: signedIn,
        fault: { code: 'core.auth/LOGIN_REFUSED', status: 401 },
        onChangePassword: () => undefined,
      },
    });

    await expect
      .element(page.getByText('The username or password is wrong'))
      .toBeVisible();
  });

  it('keeps what was typed when the server refused it', async () => {
    await render(SettingsAccountScreen, {
      props: {
        source: signedIn,
        fault: { code: 'core.auth/LOGIN_REFUSED', status: 401 },
        onChangePassword: () => undefined,
      },
    });

    await fill('wrong-one', 'correct-horse-battery');

    await expect
      .element(page.getByLabelText('Current password'))
      .toHaveValue('wrong-one');
  });

  it('empties the form once the change was accepted', async () => {
    const { rerender } = await render(SettingsAccountScreen, {
      props: { source: signedIn, onChangePassword: () => undefined },
    });

    await fill('workshop-floor-9', 'correct-horse-battery');
    await rerender({
      source: signedIn,
      changed: true,
      onChangePassword: () => undefined,
    });

    await expect
      .element(page.getByText('Your password was changed.'))
      .toBeVisible();
    await expect
      .element(page.getByLabelText('Current password'))
      .toHaveValue('');
    await expect.element(page.getByLabelText('New password')).toHaveValue('');
    await expect
      .element(page.getByLabelText('Confirm new password'))
      .toHaveValue('');
  });

  // A person signed in at an identity provider has no password here to change.
  it('offers no password change to a passwordless account', async () => {
    await render(SettingsAccountScreen, {
      props: {
        source: signedIn,
        passwordless: true,
        onChangePassword: () => undefined,
      },
    });

    await expect.element(page.getByText('user:alice')).toBeVisible();
    await expect
      .element(page.getByLabelText('Current password'))
      .not.toBeInTheDocument();
  });

  it('has nothing axe refuses', async () => {
    const { container } = await render(SettingsAccountScreen, {
      props: {
        source: signedIn,
        fault: { code: 'core.auth/LOGIN_REFUSED', status: 401 },
        onChangePassword: () => undefined,
      },
    });

    await expectNoViolations(container);
  });
});

describe('Password visibility', () => {
  it('toggles each field independently without submitting the form', async () => {
    const onChangePassword = vi.fn();
    const { container } = await render(SettingsAccountScreen, {
      props: { source: signedIn, onChangePassword },
    });
    await page.getByRole('button', { name: 'Show Current password' }).click();
    expect(
      container.querySelectorAll<HTMLInputElement>('[data-slot="input"]')[0]
        ?.type,
    ).toBe('text');
    expect(
      container.querySelectorAll<HTMLInputElement>('[data-slot="input"]')[1]
        ?.type,
    ).toBe('password');
    expect(
      container.querySelectorAll<HTMLInputElement>('[data-slot="input"]')[2]
        ?.type,
    ).toBe('password');
    await page.getByRole('button', { name: 'Show New password' }).click();
    expect(
      container.querySelectorAll<HTMLInputElement>('[data-slot="input"]')[1]
        ?.type,
    ).toBe('text');
    expect(
      container.querySelectorAll<HTMLInputElement>('[data-slot="input"]')[2]
        ?.type,
    ).toBe('password');
    expect(onChangePassword).not.toHaveBeenCalled();
  });
});
