import { SvelteDate } from 'svelte/reactivity';

import {
  type AppSettingsClient,
  readAppSettings,
  UI_THEMES,
  type UiSettings,
  type UiTheme,
} from './services/app-settings';

/**
 * What the page draws while the read is in flight, and after a read that
 * failed. They are the three values `internal/app/config/ui.go` defaults to,
 * so a server that cannot answer and a server nobody configured look alike.
 */
export const UI_DEFAULTS = {
  title: 'Prusa Connect',
  defaultLanguage: 'en',
  theme: 'system',
} as const;

/**
 * A value the server left empty is a value the page cannot use: an empty tab
 * name would be drawn as nothing.
 */
const orDefault = (value: string | undefined, fallback: string): string =>
  value === undefined || value.trim() === '' ? fallback : value;

const isTheme = (value: string): value is UiTheme =>
  (UI_THEMES as readonly string[]).includes(value);

/**
 * How this deployment wants its page to look: the name in the tab, the
 * language it starts in and the theme.
 *
 * A shared store, so a class (svelte-5.md). The client comes in through the
 * constructor, which is what lets a test hand it a fake and what lets the
 * host hand it its own.
 *
 * Every getter answers the server's own default rather than `undefined`, so a
 * page drawn before the read lands, and a page whose read failed, both carry
 * three usable values.
 */
export class AppSettings {
  readonly #client: AppSettingsClient;
  #settings = $state.raw<UiSettings | undefined>(undefined);
  #asked = $state(false);
  #error = $state<Error | undefined>(undefined);
  #flight: Promise<void> | undefined;

  constructor(client: AppSettingsClient) {
    this.#client = client;
  }

  /** The name in the browser tab and at the head of the sidebar. */
  get title(): string {
    return orDefault(this.#settings?.title, UI_DEFAULTS.title);
  }

  /** The language the page starts in when this browser has chosen none. */
  get defaultLanguage(): string {
    return orDefault(
      this.#settings?.default_language,
      UI_DEFAULTS.defaultLanguage,
    );
  }

  /**
   * The theme the page starts in. A word this library has no palette for
   * comes back as `system`, because a page has to be drawn in something.
   */
  get theme(): UiTheme {
    const named = orDefault(this.#settings?.theme, UI_DEFAULTS.theme);

    return isTheme(named) ? named : UI_DEFAULTS.theme;
  }

  /**
   * Where the server serves its API reference page. Undefined until the read
   * lands, and on a build that leaves the page out.
   */
  get apiReference(): string | undefined {
    return this.#settings?.api_reference;
  }

  /**
   * Whether printers are served a certificate signed by this deployment's own
   * authority, so a printer's settings file asks it to trust connect.der.
   */
  get printerCustomCa(): boolean {
    return this.#settings?.printer_custom_ca ?? false;
  }

  /**
   * How a new printer joins: a person approves each one, or every printer
   * that asks is approved at once. Undefined until the read lands, because
   * the add-printer page must not guess which steps to show.
   */
  get printerAdmission(): 'approval' | 'automatic' | undefined {
    return this.#settings?.printer_admission;
  }

  /**
   * Where the documentation written for this build is read. Undefined until
   * the read lands, and on a build that names none.
   */
  get documentation(): string | undefined {
    return this.#settings?.documentation;
  }

  /**
   * The origin a printer dials, when this deployment names one apart from
   * the address the page was opened on. Undefined until the read lands, and
   * on a server that names none.
   */
  get printerAddress(): string | undefined {
    return this.#settings?.printer_address;
  }

  /**
   * Whether this server keeps dashboards. A build or a configuration may
   * leave them out (ADR-20261008-sww). Until the read lands, and after a read that
   * failed, it answers the default, which is on.
   */
  get dashboards(): boolean {
    return this.#settings?.dashboards ?? true;
  }

  /**
   * Whether this server installs modules and runs the ones it installed. The
   * operator switches them off with `core.modules_enabled: false` (ADR-20261019-h76),
   * and the server refuses every install then. Until the read finishes, and
   * after a read that failed, it answers the default, which is on.
   */
  get modules(): boolean {
    return this.#settings?.modules ?? true;
  }

  /**
   * Whether this server prepares upgrades and keeps backups. The operator
   * switches them off with `core.upgrade.enabled: false`, and the server
   * answers 404 for them then. Until the read finishes, and after a read that
   * failed, it answers the default, which is on.
   */
  get upgrades(): boolean {
    return this.#settings?.upgrades ?? true;
  }

  /**
   * Set while this run is in safe mode: every module is off until the server
   * restarts normally. Undefined otherwise, until the read lands, and after a
   * read that failed.
   */
  get safeMode(): { reason: 'flag' | 'restart'; since: Date } | undefined {
    const state = this.#settings?.safe_mode;

    return state === undefined
      ? undefined
      : { reason: state.reason, since: new SvelteDate(state.since) };
  }

  /**
   * Whether this server serves the diagnostics bundle. The minimal build and
   * `core.diagnostics.enabled: false` leave it out (ADR-20261015-mde). Until the read
   * lands, and after a read that failed, it answers off, so no link leads to
   * an address that answers 404.
   */
  get diagnostics(): boolean {
    return this.#settings?.diagnostics ?? false;
  }

  /**
   * The portal Send to Prusa uploads the diagnostics bundle to (ADR-20261015-mde rule
   * 16). Undefined until the read lands, and where the operator switched
   * sending off, so the button is drawn only once there is somewhere to send.
   */
  get diagnosticsSendUrl(): string | undefined {
    return this.#settings?.diagnostics_send_url;
  }

  /**
   * Where the Support link points, when this deployment names a portal of its
   * own. Undefined until the read lands, and on a server that names none.
   */
  get supportUrl(): string | undefined {
    return this.#settings?.support_url;
  }

  /**
   * True once a read has finished, whether it answered or failed. The page
   * waits on this rather than on the values, so a server that cannot answer
   * draws its own defaults instead of waiting for ever.
   */
  get asked(): boolean {
    return this.#asked;
  }

  /** Why the read failed, when it did. */
  get error(): Error | undefined {
    return this.#error;
  }

  /**
   * Asks this server how its page is to look. A second call while the first
   * is in flight joins it rather than making a second request
   * (svelte-data.md).
   */
  read(): Promise<void> {
    this.#flight ??= this.#read().finally(() => {
      this.#flight = undefined;
    });

    return this.#flight;
  }

  async #read(): Promise<void> {
    try {
      this.#settings = await readAppSettings(this.#client);
      this.#error = undefined;
    } catch (thrown) {
      // A server that cannot answer this still serves a page, so the three
      // defaults stand and the fault is kept for a log rather than shown.
      this.#settings = undefined;
      this.#error =
        thrown instanceof Error ? thrown : new Error(String(thrown));
    } finally {
      this.#asked = true;
    }
  }
}
