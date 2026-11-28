// Which module theme this browser draws the app in, remembered between visits.
//
// A module theme is a stylesheet a module's manifest offers under `ui.themes`.
// It is linked after the app's own styles, so it may override anything the app
// draws. A browser either follows the theme the server names as its default,
// or holds a pick of its own: a theme, or none at all.
//
// The page links the stylesheet before it draws: `index.html` reads the pick
// this store remembers and writes the link itself, or links the server's
// default at `/api/v1/themes/default.css`. This store keeps that link in step
// with what a person chooses afterwards. The storage format is read by that
// script too, so it changes only together with it.
import {
  browserStorage,
  readRemembered,
  type RememberedStorage,
  writeRemembered,
} from '../remembered';

/** The one `localStorage` key the pick lives under. */
export const MODULE_THEME_STORAGE_KEY = 'module-theme';

/** Where the server serves its default theme, or an empty stylesheet. */
export const DEFAULT_THEME_HREF = '/api/v1/themes/default.css';

/** The attribute on the link of a theme a person picked. */
const PICK_ATTRIBUTE = 'data-module-theme';
/** The attribute on the link of the server's default theme. */
const DEFAULT_ATTRIBUTE = 'data-module-theme-default';

/** Only a stylesheet the core serves for a module, never another origin. */
const MODULE_PREFIX = '/api/v1/modules/';

/** One theme a module offers, as the listing names it. */
export interface ModuleThemeOption {
  /** `<namespace>/<id>`, unique on the server. */
  key: string;
  title: string;
  /** The title of the module that offers it. */
  module?: string | undefined;
  /** Where the stylesheet is served. */
  href: string;
}

/** A theme picked, or `null` for the app's own look picked over the default. */
type Pick = { key: string; href: string } | null;

const readPick = (storage: RememberedStorage | null): Pick | undefined => {
  const raw = readRemembered(storage, MODULE_THEME_STORAGE_KEY);
  if (raw === null) return undefined;
  try {
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed === null || !('key' in parsed)) {
      return undefined;
    }
    if (parsed.key === null) return null;
    if (
      typeof parsed.key === 'string' &&
      'href' in parsed &&
      typeof parsed.href === 'string' &&
      parsed.href.startsWith(MODULE_PREFIX)
    ) {
      return { key: parsed.key, href: parsed.href };
    }
  } catch {
    // A value this store did not write is ignored.
  }
  return undefined;
};

const linkWith = (attribute: string): HTMLLinkElement | null =>
  typeof document === 'undefined'
    ? null
    : document.querySelector<HTMLLinkElement>(`link[${attribute}]`);

/** Links `href` under `attribute`, after every stylesheet the app linked. */
const link = (attribute: string, href: string): void => {
  if (typeof document === 'undefined') return;
  const held = linkWith(attribute);
  if (held?.getAttribute('href') === href) return;
  held?.remove();
  const element = document.createElement('link');
  element.rel = 'stylesheet';
  element.setAttribute(attribute, '');
  element.href = href;
  document.head.append(element);
};

const unlink = (attribute: string): void => {
  linkWith(attribute)?.remove();
};

/** Draws what `pick` says: a theme, the app's own look, or the default. */
const applyPick = (pick: Pick | undefined): void => {
  if (pick === undefined) {
    unlink(PICK_ATTRIBUTE);
    link(DEFAULT_ATTRIBUTE, DEFAULT_THEME_HREF);
    return;
  }
  unlink(DEFAULT_ATTRIBUTE);
  if (pick === null) {
    unlink(PICK_ATTRIBUTE);
    return;
  }
  link(PICK_ATTRIBUTE, pick.href);
};

export interface ModuleThemeStoreOptions {
  storage?: RememberedStorage | null;
}

/** A shared store, so a class (svelte-5.md). */
export class ModuleThemeStore {
  readonly #storage: RememberedStorage | null;
  #pick = $state<Pick | undefined>(undefined);
  #defaultKey = $state<string | undefined>(undefined);

  // The constructor draws nothing: the page's own script linked what the
  // stored pick says before the first paint, and a page with no such script,
  // such as a story or a test, has no server to ask for a default.
  constructor({ storage = browserStorage() }: ModuleThemeStoreOptions = {}) {
    this.#storage = storage;
    this.#pick = readPick(storage);
  }

  /** The key of the theme in use, or `undefined` for the app's own look. */
  get key(): string | undefined {
    if (this.#pick === undefined) return this.#defaultKey;
    return this.#pick?.key;
  }

  /** The theme the server names as its default, when a module offers it. */
  get defaultKey(): string | undefined {
    return this.#defaultKey;
  }

  /** True while this browser follows the server's default. */
  get followsDefault(): boolean {
    return this.#pick === undefined;
  }

  /** Takes the theme the server names as its default, as `<namespace>/<id>`. */
  useDefault(key: string | undefined): void {
    this.#defaultKey = key === '' ? undefined : key;
  }

  /** Draws the app in `option`, or in its own look for `undefined`. */
  choose(option: ModuleThemeOption | undefined): void {
    const pick: Pick =
      option === undefined ? null : { key: option.key, href: option.href };
    this.#pick = pick;
    applyPick(pick);
    writeRemembered(
      this.#storage,
      MODULE_THEME_STORAGE_KEY,
      JSON.stringify(pick ?? { key: null }),
    );
  }

  /** Forgets the pick, so the browser follows the server's default again. */
  followDefault(): void {
    this.#pick = undefined;
    applyPick(undefined);
    try {
      this.#storage?.removeItem?.(MODULE_THEME_STORAGE_KEY);
    } catch {
      // A browser that refuses to forget keeps its pick until it is asked again.
    }
  }

  /**
   * Holds the pick to what the server offers now. A theme whose module was
   * removed or switched off is forgotten, and the browser follows the
   * default again. A theme whose address moved is followed.
   */
  reconcile(offered: readonly ModuleThemeOption[]): void {
    const defaultKey = this.#defaultKey;
    if (
      defaultKey !== undefined &&
      !offered.some((option) => option.key === defaultKey)
    ) {
      this.#defaultKey = undefined;
    }
    const pick = this.#pick;
    if (!pick) return;
    const found = offered.find((option) => option.key === pick.key);
    if (found === undefined) {
      this.followDefault();
      return;
    }
    if (found.href !== pick.href) this.choose(found);
  }
}

export const moduleTheme = new ModuleThemeStore();
