import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import ModuleSettingsForm from './module-settings-form.svelte';

describe('ModuleSettingsForm', () => {
  it('saves the parsed config and refuses a wrong type at its field', async () => {
    const onsave = vi.fn().mockResolvedValue(true);
    await render(ModuleSettingsForm, {
      options: [
        {
          key: 'shelf_size',
          type: 'int',
          summary: 'Spools per shelf.',
          required: true,
        },
      ],
      config: { shelf_size: 4 },
      saving: false,
      onsave,
      onreload: vi.fn(),
    });

    const field = page.getByLabelText('Shelf size (required)');
    await field.fill('1.5');
    await page.getByRole('button', { name: 'Save' }).click();
    expect(onsave).not.toHaveBeenCalled();
    await expect.element(page.getByText('Write a whole number.')).toBeVisible();

    await field.fill('6');
    await page.getByRole('button', { name: 'Save' }).click();
    expect(onsave).toHaveBeenCalledWith({ shelf_size: 6 });
  });

  it('never shows a stored secret and sends the marker back', async () => {
    const onsave = vi.fn().mockResolvedValue(true);
    await render(ModuleSettingsForm, {
      options: [{ key: 'token', type: 'secret', summary: '' }],
      config: { token: 'xxxxx' },
      saving: false,
      onsave,
      onreload: vi.fn(),
    });

    await expect.element(page.getByLabelText('Token')).toHaveValue('');
    await page.getByRole('button', { name: 'Save' }).click();
    expect(onsave).toHaveBeenCalledWith({ token: 'xxxxx' });
  });

  it('keeps the edits and offers a reload on a conflict', async () => {
    const onreload = vi.fn();
    await render(ModuleSettingsForm, {
      options: [{ key: 'name', type: 'string', summary: '' }],
      config: {},
      saving: false,
      conflict: true,
      onsave: vi.fn().mockResolvedValue(true),
      onreload,
    });

    await page
      .getByRole('button', { name: 'Show the current settings' })
      .click();
    expect(onreload).toHaveBeenCalled();
  });

  it('shows no Save button when locked', async () => {
    await render(ModuleSettingsForm, {
      options: [{ key: 'name', type: 'string', summary: '' }],
      config: {},
      locked: 'Set by a file.',
      saving: false,
      onsave: vi.fn().mockResolvedValue(true),
      onreload: vi.fn(),
    });

    await expect.element(page.getByText('Set by a file.')).toBeVisible();
    expect(page.getByRole('button', { name: 'Save' }).elements()).toHaveLength(
      0,
    );
  });

  it('keeps the edits when the save is refused and the config is re-read', async () => {
    const onsave = vi.fn().mockResolvedValue(false);
    const screen = await render(ModuleSettingsForm, {
      options: [{ key: 'name', type: 'string', summary: '' }],
      config: { name: 'old' },
      saving: false,
      onsave,
      onreload: vi.fn(),
    });

    await page.getByLabelText('Name').fill('mine');
    await page.getByRole('button', { name: 'Save' }).click();
    await screen.rerender({ conflict: true, config: { name: 'theirs' } });

    await expect.element(page.getByLabelText('Name')).toHaveValue('mine');
  });

  it('leaves a cleared secret out of the config', async () => {
    const onsave = vi.fn().mockResolvedValue(true);
    await render(ModuleSettingsForm, {
      options: [{ key: 'token', type: 'secret', summary: '' }],
      config: { token: 'xxxxx' },
      saving: false,
      onsave,
      onreload: vi.fn(),
    });

    await page.getByRole('button', { name: 'Clear' }).click();
    await page.getByRole('button', { name: 'Save' }).click();
    expect(onsave).toHaveBeenCalledWith({});
  });
});
