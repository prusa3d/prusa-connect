import { afterEach, describe, expect, it } from 'vitest';

import type { RememberedStorage } from '../remembered';
import {
  DEFAULT_THEME_HREF,
  MODULE_THEME_STORAGE_KEY,
  type ModuleThemeOption,
  ModuleThemeStore,
} from './module-theme.svelte';

const fakeStorage = (seed?: string) => {
  const map = new Map<string, string>();
  if (seed !== undefined) map.set(MODULE_THEME_STORAGE_KEY, seed);
  const storage: RememberedStorage = {
    getItem: (key) => map.get(key) ?? null,
    setItem: (key, value) => {
      map.set(key, value);
    },
    removeItem: (key) => {
      map.delete(key);
    },
  };
  return { map, storage };
};

const OCEAN: ModuleThemeOption = {
  key: 'com.github.aux.acme.themes/ocean',
  title: 'Ocean',
  module: 'Acme themes',
  href: '/api/v1/modules/com.github.aux.acme.themes/ui/ocean.css',
};

const PAPER: ModuleThemeOption = {
  key: 'com.github.aux.acme.themes/paper',
  title: 'Paper',
  href: '/api/v1/modules/com.github.aux.acme.themes/ui/paper.css',
};

const picked = (): string | null | undefined =>
  document.querySelector('link[data-module-theme]')?.getAttribute('href');

const defaulted = (): string | null | undefined =>
  document
    .querySelector('link[data-module-theme-default]')
    ?.getAttribute('href');

afterEach(() => {
  document.querySelector('link[data-module-theme]')?.remove();
  document.querySelector('link[data-module-theme-default]')?.remove();
});

describe('ModuleThemeStore', () => {
  it('follows the server with nothing stored, and links nothing itself', () => {
    const { storage } = fakeStorage();
    const store = new ModuleThemeStore({ storage });

    expect(store.followsDefault).toBe(true);
    expect(store.key).toBeUndefined();
    expect(picked()).toBeUndefined();
    expect(defaulted()).toBeUndefined();
  });

  it('reads the server default as the theme in use until a pick', () => {
    const { storage } = fakeStorage();
    const store = new ModuleThemeStore({ storage });

    store.useDefault(OCEAN.key);

    expect(store.key).toBe(OCEAN.key);
    expect(store.defaultKey).toBe(OCEAN.key);
  });

  it('remembers a pick with its stylesheet, and unlinks the default', () => {
    const { map, storage } = fakeStorage();
    const store = new ModuleThemeStore({ storage });
    store.useDefault(PAPER.key);

    store.choose(OCEAN);

    expect(store.key).toBe(OCEAN.key);
    expect(picked()).toBe(OCEAN.href);
    expect(defaulted()).toBeUndefined();
    expect(JSON.parse(map.get(MODULE_THEME_STORAGE_KEY) ?? '{}')).toEqual({
      key: OCEAN.key,
      href: OCEAN.href,
    });
  });

  it('remembers None over a server default', () => {
    const { map, storage } = fakeStorage();
    const store = new ModuleThemeStore({ storage });
    store.useDefault(OCEAN.key);

    store.choose(undefined);

    expect(store.key).toBeUndefined();
    expect(store.followsDefault).toBe(false);
    expect(picked()).toBeUndefined();
    expect(JSON.parse(map.get(MODULE_THEME_STORAGE_KEY) ?? '{}')).toEqual({
      key: null,
    });
    expect(new ModuleThemeStore({ storage }).followsDefault).toBe(false);
  });

  it('goes back to the default, linking it again', () => {
    const { map, storage } = fakeStorage();
    const store = new ModuleThemeStore({ storage });
    store.choose(OCEAN);

    store.followDefault();

    expect(store.followsDefault).toBe(true);
    expect(map.has(MODULE_THEME_STORAGE_KEY)).toBe(false);
    expect(picked()).toBeUndefined();
    expect(defaulted()).toBe(DEFAULT_THEME_HREF);
  });

  it('reads back a remembered pick before the listing answers', () => {
    const { storage } = fakeStorage(
      JSON.stringify({ key: OCEAN.key, href: OCEAN.href }),
    );

    expect(new ModuleThemeStore({ storage }).key).toBe(OCEAN.key);
  });

  it('refuses a remembered stylesheet from outside the module prefix', () => {
    const { storage } = fakeStorage(
      JSON.stringify({ key: OCEAN.key, href: 'https://example.com/x.css' }),
    );

    expect(new ModuleThemeStore({ storage }).followsDefault).toBe(true);
  });

  it('ignores a stored value it did not write', () => {
    const { storage } = fakeStorage('not json');

    expect(new ModuleThemeStore({ storage }).followsDefault).toBe(true);
  });

  it('follows the default again when the picked theme is gone', () => {
    const { map, storage } = fakeStorage();
    const store = new ModuleThemeStore({ storage });
    store.choose(OCEAN);

    store.reconcile([]);

    expect(store.followsDefault).toBe(true);
    expect(map.has(MODULE_THEME_STORAGE_KEY)).toBe(false);
  });

  it('drops a default no module offers', () => {
    const { storage } = fakeStorage();
    const store = new ModuleThemeStore({ storage });
    store.useDefault(OCEAN.key);

    store.reconcile([PAPER]);

    expect(store.key).toBeUndefined();
  });

  it('follows a stylesheet whose address moved', () => {
    const { storage } = fakeStorage();
    const store = new ModuleThemeStore({ storage });
    store.choose(OCEAN);
    const moved = { ...OCEAN, href: `${OCEAN.href}?v=2` };

    store.reconcile([moved]);

    expect(store.key).toBe(OCEAN.key);
    expect(picked()).toBe(moved.href);
  });
});
