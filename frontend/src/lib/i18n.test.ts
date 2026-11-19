// The shell's own catalogues: which languages it ships, and that the rail
// reads them.
import { afterEach, expect, test } from 'vitest';

import { FEATURES } from '@prusa/connect-ui/domain';
import { locale, LOCALES, SOURCE_LOCALE } from '@prusa/connect-ui/i18n';

import { APP_CATALOGUES, startLocale } from './i18n';
import { navGroups, routes, settingsRoute } from './routes';

afterEach(() => {
  locale.set(SOURCE_LOCALE);
});

test('every catalogue of the shell is named for a language the switch offers', () => {
  const offered: readonly string[] = LOCALES;

  expect(
    [...APP_CATALOGUES.keys()].filter((tag) => !offered.includes(tag)),
  ).toEqual([]);
  expect(APP_CATALOGUES.has('cs')).toBe(true);
});

// A key English does not hold is a translation of nothing: the key was renamed
// or removed, and the line will never be read.
test('a translation names only keys the English catalogue holds', () => {
  const english = APP_CATALOGUES.get(SOURCE_LOCALE) ?? {};
  const strays = [...APP_CATALOGUES].flatMap(([tag, messages]) =>
    Object.keys(messages)
      .filter((key) => !(key in english))
      .map((key) => `${tag}: ${key}`),
  );

  expect(strays).toEqual([]);
});

test('the rail reads Czech when the browser asks for it', () => {
  expect(startLocale(['cs-CZ', 'en'])).toBe('cs');

  expect(routes().map((item) => item.label)).toEqual([
    'Nástěnky',
    'Tiskárny',
    'Úlohy',
    'Úložiště',
    ...(FEATURES.tags ? ['Štítky'] : []),
  ]);
  expect(navGroups().map((group) => group.label)).toEqual(['Rozšíření']);

  const settings = settingsRoute();
  expect(settings.label).toBe('Nastavení');
  expect(settings.children?.map((item) => item.label)).toEqual([
    'Účet',
    'Vzhled',
    'Rozšíření',
    'Přístup k API',
    'Uživatelé',
    'Server',
    ...(FEATURES.diagnostics ? ['Diagnostika'] : []),
  ]);
});

test('the rail stays English for a browser that asks for no language the shell ships', () => {
  expect(startLocale(['ja'])).toBe('en');

  expect(routes().map((item) => item.label)).toContain('Printers');
});
