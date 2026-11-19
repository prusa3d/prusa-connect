// Moved from pull request 185, packages/ui/src/theme.svelte.ts, 2026-09-11

import {
  browserStorage,
  readRemembered,
  type RememberedStorage,
  writeRemembered,
} from '../remembered';

/** The theme preference, and whether it resolves to dark right now. */
export const THEME_PREFERENCES = ['light', 'dark', 'system'] as const;

export type ThemePreference = (typeof THEME_PREFERENCES)[number];

/** The one `localStorage` key the preference lives under. */
export const THEME_STORAGE_KEY = 'theme';

/**
 * The key the API reference page (`/docs`, drawn by Scalar on this origin)
 * keeps its colour mode under. The page reads it when it opens, so writing it
 * with the theme makes the reference follow the app.
 */
export const REFERENCE_COLOR_MODE_KEY = 'colorMode';

const DARK_QUERY = '(prefers-color-scheme: dark)';

export type ThemeStorage = RememberedStorage;

/** What the store needs of a `matchMedia` result. */
export interface ThemeMedia {
  readonly matches: boolean;
  addEventListener(
    type: 'change',
    listener: (event: { matches: boolean }) => void,
  ): void;
}

export interface ThemeStoreOptions {
  storage?: ThemeStorage | null;
  /** `null` reads the machine as light. */
  media?: ThemeMedia | null;
}

const isPreference = (value: unknown): value is ThemePreference =>
  typeof value === 'string' &&
  (THEME_PREFERENCES as readonly string[]).includes(value);

// Under SSR there is no `matchMedia`, so the default reads it as optional.
const browser: { matchMedia?: (query: string) => ThemeMedia } = globalThis;

const browserMedia = (): ThemeMedia | null =>
  browser.matchMedia ? browser.matchMedia(DARK_QUERY) : null;

/** What this browser remembers, or `undefined` when it remembers nothing. */
const storedPreference = (
  storage: ThemeStorage | null,
): ThemePreference | undefined => {
  const stored = readRemembered(storage, THEME_STORAGE_KEY);
  return isPreference(stored) ? stored : undefined;
};

/** A shared store, so a class (svelte-5.md). */
export class ThemeStore {
  #preference = $state<ThemePreference>('system');
  #systemDark = $state(false);
  readonly #storage: ThemeStorage | null;
  // Whether a person on this browser has chosen a side.
  #chosen = $state(false);
  readonly #dark = $derived(
    this.#preference === 'dark' ||
      (this.#preference === 'system' && this.#systemDark),
  );

  constructor({
    storage = browserStorage(),
    media = browserMedia(),
  }: ThemeStoreOptions = {}) {
    this.#storage = storage;
    const held = storedPreference(storage);
    this.#chosen = held !== undefined;
    this.#preference = held ?? 'system';
    if (media) {
      this.#systemDark = media.matches;
      // A page-lifetime subscription.
      media.addEventListener('change', (event) => {
        this.#systemDark = event.matches;
      });
    }
  }

  get preference(): ThemePreference {
    return this.#preference;
  }

  /** Rejects anything outside the three values: the state stays, nothing is persisted. */
  set preference(next: ThemePreference) {
    if (!isPreference(next)) return;
    // A write is a choice even when it names the side the page is already on.
    this.#chosen = true;
    this.#preference = next;
    writeRemembered(this.#storage, THEME_STORAGE_KEY, next);
  }

  /** True once somebody on this browser has picked a side. Read-only by design. */
  get chosen(): boolean {
    return this.#chosen;
  }

  /** Takes the theme a deployment configured. */
  useDefault(next: ThemePreference): void {
    if (this.#chosen || !isPreference(next)) return;
    this.#preference = next;
  }

  /** What the preference comes to on this machine at this moment. Read-only by design. */
  get dark(): boolean {
    return this.#dark;
  }
}

export const theme = new ThemeStore();

/** Draws the page light or dark. */
export const applyTheme = (dark: boolean): void => {
  // Under SSR there is no document.
  if (typeof document === 'undefined') return;
  document.documentElement.classList.toggle('dark', dark);
  writeRemembered(
    browserStorage(),
    REFERENCE_COLOR_MODE_KEY,
    dark ? 'dark' : 'light',
  );
};
