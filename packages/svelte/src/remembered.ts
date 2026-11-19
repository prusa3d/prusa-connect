// What a preference store needs of `localStorage`.

/** What a preference store needs of `localStorage`. */
export interface RememberedStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  // Optional, so a fake that only reads and writes still fits.
  removeItem?(key: string): void;
}

const browser: {
  localStorage?: RememberedStorage;
  sessionStorage?: RememberedStorage;
} = globalThis;

/** The browser's own storage, or null where there is none to reach. */
export const browserStorage = (): RememberedStorage | null => {
  try {
    return browser.localStorage ?? null;
  } catch {
    return null;
  }
};

/**
 * The browser's storage for this tab, or null where there is none. It survives
 * a reload and is gone when the tab is closed, which is where a secret typed
 * into a form may wait (ADR-20261015-ejz rule 30).
 */
export const browserTabStorage = (): RememberedStorage | null => {
  try {
    return browser.sessionStorage ?? null;
  } catch {
    return null;
  }
};

export const readRemembered = (
  storage: RememberedStorage | null | undefined,
  key: string,
): string | null => {
  try {
    return storage?.getItem(key) ?? null;
  } catch {
    return null;
  }
};

export const writeRemembered = (
  storage: RememberedStorage | null | undefined,
  key: string,
  value: string,
): void => {
  try {
    storage?.setItem(key, value);
  } catch {
    // A browser that refuses to remember still draws the choice it was given.
  }
};

export const forgetRemembered = (
  storage: RememberedStorage | null | undefined,
  key: string,
): void => {
  try {
    storage?.removeItem?.(key);
  } catch {
    // A browser that refuses to forget keeps a value nothing reads.
  }
};
