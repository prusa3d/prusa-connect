// How this deployment wants its own page to look. A service names the SDK
// operation and its parameters and holds no state (svelte-data.md).
import type { PrusaConnect } from '@prusa/connect';

/**
 * What the store needs of the SDK. A `PrusaConnect` satisfies it, and naming
 * the one operation is what lets a test hand in a fake without writing out
 * every operation the core declares (svelte-data.md).
 */
export type AppSettingsClient = Pick<PrusaConnect, 'getUiSettings'>;

/** How the page is to look, as `get-ui-settings` answers it. */
export type UiSettings = Awaited<ReturnType<PrusaConnect['getUiSettings']>>;

/** The three values `core.frontend.theme` takes. */
export const UI_THEMES = ['system', 'light', 'dark'] as const;

export type UiTheme = (typeof UI_THEMES)[number];

/**
 * Reads the name, the fallback language and the theme this server draws its
 * page with. `get-ui-settings` is `GET /api/v1/ui`, and it
 * takes no credential, because the page reads it before it paints and before
 * anybody has signed in.
 */
export const readAppSettings = (
  client: AppSettingsClient,
): Promise<UiSettings> => client.getUiSettings({});
