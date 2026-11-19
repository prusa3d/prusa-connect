import { describe, expect, it } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { expectNoViolations } from '../../test/axe';
import PasswordInput from './password-input.svelte';
import type { PasswordInputProps } from './types';

const input = () => document.querySelector<HTMLInputElement>('input')!;
const props = {
  label: 'New password',
  'aria-label': 'New password',
  value: 'example-password',
  autocomplete: 'new-password',
} satisfies PasswordInputProps;

describe('PasswordInput', () => {
  it('reveals and hides the same input without changing its value', async () => {
    await render(PasswordInput, { props });
    const original = input();
    expect(original.type).toBe('password');
    await page.getByRole('button', { name: 'Show New password' }).click();
    expect(input()).toBe(original);
    expect(input().type).toBe('text');
    expect(input().value).toBe('example-password');
    expect(input().autocomplete).toBe('new-password');
    await page.getByRole('button', { name: 'Hide New password' }).click();
    expect(input().type).toBe('password');
    expect(input().value).toBe('example-password');
  });

  it('can toggle from the keyboard', async () => {
    await render(PasswordInput, { props });
    await userEvent.click(
      page.getByRole('button', { name: 'Show New password' }),
    );
    await userEvent.keyboard('{Space}');
    expect(input().type).toBe('password');
    await userEvent.keyboard('{Enter}');
    expect(input().type).toBe('text');
  });

  it('conceals the input when the form clears its controlled value', async () => {
    const screen = await render(PasswordInput, { props });
    await page.getByRole('button', { name: 'Show New password' }).click();
    await screen.rerender({ value: '' });
    await expect
      .element(page.getByRole('button', { name: 'Show New password' }))
      .toBeVisible();
    expect(input().type).toBe('password');
    expect(input().value).toBe('');
  });

  it('keeps disabled inputs and their toggle disabled', async () => {
    await render(PasswordInput, { props: { ...props, disabled: true } });
    await expect.element(page.getByRole('button')).toBeDisabled();
    expect(input().disabled).toBe(true);
    expect(input().type).toBe('password');
  });

  it('keeps accessible field naming and error wiring', async () => {
    const { container } = await render(PasswordInput, {
      props: { ...props, id: 'new-password' },
    });
    await expectNoViolations(container);
    expect(
      container.querySelector('button')?.getAttribute('aria-controls'),
    ).toBe('new-password');
  });
});
