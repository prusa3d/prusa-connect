// The home choice on the Appearance page, on a server with and without
// dashboards (ADR-20261008-sww), and the choices the page offers.
import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

import { Preferences } from '../../data';
import { moduleTheme } from '../../styles/module-theme.svelte';
import SettingsAppearanceScreen from './settings-appearance-screen.svelte';

describe('the look', () => {
  it('offers a theme and a text size and no skin', async () => {
    await render(SettingsAppearanceScreen, {
      props: { home: new Preferences(null) },
    });

    const themes = page.getByRole('group', { name: 'Theme' });
    for (const name of ['System', 'Light', 'Dark'])
      await expect.element(themes.getByText(name)).toBeVisible();
    const sizes = page.getByRole('group', { name: 'Text size' });
    for (const name of ['Small', 'Normal', 'Large'])
      await expect.element(sizes.getByText(name)).toBeVisible();
    await expect.element(page.getByLabelText('Skin')).not.toBeInTheDocument();
  });
});

describe('the home choice', () => {
  it('offers the dashboard where the server keeps dashboards', async () => {
    await render(SettingsAppearanceScreen, {
      props: { home: new Preferences(null) },
    });

    await expect
      .element(page.getByRole('radio', { name: 'Dashboard' }))
      .toBeChecked();
  });

  it('leaves the dashboard out, and shows automatic, where the server has none', async () => {
    await render(SettingsAppearanceScreen, {
      props: { home: new Preferences(null), dashboards: false },
    });

    await expect
      .element(page.getByRole('radio', { name: 'Automatic' }))
      .toBeChecked();
    await expect
      .element(page.getByRole('radio', { name: 'Dashboard' }))
      .not.toBeInTheDocument();
  });
});

describe('the extension theme', () => {
  const OCEAN = {
    key: 'com.github.aux.acme.themes/ocean',
    title: 'Ocean',
    module: 'Acme themes',
    href: '/api/v1/modules/com.github.aux.acme.themes/ui/ocean.css',
  };

  const linked = () =>
    document.querySelector('link[data-module-theme]')?.getAttribute('href');

  afterEach(() => {
    moduleTheme.choose(undefined);
  });

  it('is not offered when no module offers a theme', async () => {
    await render(SettingsAppearanceScreen, {
      props: { home: new Preferences(null) },
    });

    await expect
      .element(page.getByRole('combobox', { name: 'Extension theme' }))
      .not.toBeInTheDocument();
  });

  it('links the stylesheet of the theme picked, and unlinks it on None', async () => {
    await render(SettingsAppearanceScreen, {
      props: { home: new Preferences(null), themes: [OCEAN] },
    });
    const picker = page.getByRole('combobox', { name: 'Extension theme' });

    await expect.element(picker).toHaveTextContent('None');

    await picker.click();
    await expect.element(page.getByText('From Acme themes')).toBeVisible();
    await page.getByRole('option', { name: /Ocean/ }).click();
    await expect.element(picker).toHaveTextContent('Ocean');
    expect(linked()).toBe(OCEAN.href);

    await picker.click();
    await page.getByRole('option', { name: 'None' }).click();
    await expect.element(picker).toHaveTextContent('None');
    expect(linked()).toBeUndefined();
  });
});
