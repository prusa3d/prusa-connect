// The requests about the browser origins that may call this server. A service
// names the SDK operation and its parameters and keeps no state
// (svelte-data.md).
import type { PrusaConnect } from '@prusa/connect';

/** What the allowed origins store needs of the SDK, and no more. */
export type AllowedOriginsClient = Pick<
  PrusaConnect,
  'listAllowedOrigins' | 'setAllowedOrigins'
>;

/** The allowed origins, as `list-allowed-origins` answers them. */
export type AllowedOriginsPage = Awaited<
  ReturnType<PrusaConnect['listAllowedOrigins']>
>;

/** One origin a page may call this server from, and where it comes from. */
export type AllowedOrigin = NonNullable<AllowedOriginsPage['items']>[number];

/** Every allowed origin, from the file, the settings and the extensions. */
export const readAllowedOrigins = (
  client: AllowedOriginsClient,
): Promise<AllowedOriginsPage> => client.listAllowedOrigins();

/**
 * Replaces the list a person keeps in Settings. The answer is every allowed
 * origin as the server applies them.
 */
export const writeAllowedOrigins = (
  client: AllowedOriginsClient,
  origins: readonly string[],
): Promise<AllowedOriginsPage> =>
  client.setAllowedOrigins({ body: { origins: [...origins] } });
