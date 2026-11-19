// A stub host: everything the module receives, and nothing it does not.
//
// The registry is the library's own `SlotRegistry`, declared with the same
// `contributes` block `module.yaml` carries, so a registration outside the
// block is refused here exactly as the app refuses it. The client is a real
// `PrusaConnect` over a fetch that answers the allowed origins a test hands
// it and holds every other call open.
import { PrusaConnect } from '@prusa/connect';
import { registerCatalogue, translateFor } from '@prusa/connect-ui/i18n';
import type { ModuleContributes, ModuleHost } from '@prusa/connect-ui/slots';
import {
  buildRegister,
  modulePageHref,
  noComponents,
  SlotRegistry,
} from '@prusa/connect-ui/slots';

import english from '../locales/en.json';

/** Where the host would install this module. A module never writes one. */
export const NAMESPACE = 'com.github.aux.prusa3d.easyprint';

/** The ceiling, written the way `../../../module.yaml` declares it. */
export const CONTRIBUTES: ModuleContributes = {
  settings: [{ id: 'easyprint', title: 'EasyPrint' }],
};

/** What the server answers for the allowed origins, or a refusal. */
export type OriginsAnswer =
  { readonly origins: readonly string[] } | { readonly refused: number };

const addressOf = (input: RequestInfo | URL): string => {
  if (typeof input === 'string') return input;

  return input instanceof URL ? input.href : input.url;
};

export interface Stub {
  host: ModuleHost;
  registry: SlotRegistry;
  /** Every request the module made. */
  requests: string[];
}

/** The host a module receives, over a server that answers `answer`. */
export const stubHost = (answer: OriginsAnswer): Stub => {
  const registry = new SlotRegistry();
  registry.declare(NAMESPACE, CONTRIBUTES);

  // What the loader does before it imports a bundle (frontend-i18n.md rule 7).
  registerCatalogue(NAMESPACE, 'en', english);

  const requests: string[] = [];
  const records = new Map<string, string>();

  const fetcher: typeof globalThis.fetch = (input) => {
    const address = addressOf(input);
    requests.push(address);
    if (!address.includes('/api/v1/allowed-origins')) {
      return new Promise<Response>(() => undefined);
    }

    if ('refused' in answer) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            code: 'core.auth/ACCESS_DENIED',
            title: 'refused',
            status: answer.refused,
          }),
          {
            status: answer.refused,
            headers: { 'content-type': 'application/problem+json' },
          },
        ),
      );
    }

    return Promise.resolve(
      new Response(
        JSON.stringify({
          items: answer.origins.map((origin) => ({
            origin,
            source: 'extension',
            extension: NAMESPACE,
            title: 'EasyPrint',
          })),
        }),
        { headers: { 'content-type': 'application/json' } },
      ),
    );
  };

  const host: ModuleHost = {
    client: new PrusaConnect({
      auth: { kind: 'session' },
      fetch: fetcher,
      retryDelay: 0,
    }),
    principal: { id: 'user:demo', grants: [] },
    fleet: {
      devices: [],
      loading: false,
      loaded: true,
      error: undefined,
      device: () => undefined,
    },
    t: translateFor(NAMESPACE),
    // A test never ends the module's time on the page, so nothing aborts it.
    signal: new AbortController().signal,
    pageHref: (path) => modulePageHref(NAMESPACE, path),
    components: noComponents,
    store: {
      get: (key) => records.get(key),
      set: (key, value) => {
        records.set(key, value);
      },
      delete: (key) => {
        records.delete(key);
      },
      keys: () => [...records.keys()].sort(),
    },
    register: buildRegister({
      page: (path, component) => {
        registry.registerPage(NAMESPACE, path, component);
      },
      section: (slot, id, component) => {
        registry.registerSection(NAMESPACE, slot, id, component);
      },
      action: (slot, id, run) => {
        registry.registerAction(NAMESPACE, slot, id, run);
      },
      card: (id, component, options) => {
        registry.registerCard(NAMESPACE, id, component, options);
      },
    }),
  };

  return { host, registry, requests };
};
